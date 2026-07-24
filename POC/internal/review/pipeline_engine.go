package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gitea-agents/internal/providers"
)

type stageProgress func(string, string, string, map[string]any, time.Time, error)
type stageProviderResolver func(context.Context, *int64) (providers.LLMProvider, error)

// StageOutcome is the validated artifact and state produced by one executor.
type StageOutcome struct {
	Status       string
	ArtifactType string
	Artifact     any
	Metadata     map[string]any
}

// StageExecutor executes one registered behavior for a configured stage.
type StageExecutor interface {
	Execute(context.Context, *pipelineRuntime, PipelineStage) (StageOutcome, error)
}

// PipelineExecutionInput contains runtime dependencies that are not pipeline configuration.
type PipelineExecutionInput struct {
	Job           queueInput
	TriggerSource string
	RawDiff       string
	BasePrompt    string
	Policy        Policy
	Provider      stageProviderResolver
	Progress      stageProgress
	Publish       func(context.Context, Result) (StageOutcome, error)
}

// PipelineEngine resolves configured stage types through a controlled registry.
type PipelineEngine struct {
	repo      *Repository
	executors map[string]StageExecutor
}

type pipelineRuntime struct {
	definition        PipelineDefinition
	input             PipelineExecutionInput
	files             []pipelineFile
	plan              pipelinePlan
	reviews           []pipelineGroupReview
	failed            []string
	consolidated      pipelineConsolidated
	consolidatedReady bool
	approved          []pipelineFinding
	result            Result
	providerName      string
	meta              map[string]any
	metricsMu         sync.Mutex
	providerMu        sync.Mutex
	providers         map[string]providers.LLMProvider
	repo              *Repository
	executionID       int64
	currentError      map[string]any
	currentArtifacts  []pipelineArtifact
	currentSources    []pipelineArtifact
}

type pipelineArtifact struct {
	ID      int64
	Type    string
	Payload any
}

type schedulerEvent struct {
	stage      PipelineStage
	via        *PipelineTransition
	artifact   *pipelineArtifact
	state      *pipelineRuntime
	closed     bool
	errPayload map[string]any
	originals  []pipelineArtifact
}

type stageCallResult struct {
	Prompt                string
	Response              string
	Usage                 providers.Usage
	Provider              string
	Model                 string
	RequestedOutputTokens int
	DurationMS            int64
}

// NewPipelineEngine creates the only orchestration runtime used by reviews.
func NewPipelineEngine(repo *Repository) *PipelineEngine {
	return &PipelineEngine{repo: repo, executors: map[string]StageExecutor{
		"preparation":     preparationExecutor{},
		"planner":         plannerExecutor{},
		"reviewer":        reviewerExecutor{},
		"consolidator":    consolidatorExecutor{},
		"verification":    verificationExecutor{},
		"formatting":      formattingExecutor{},
		"publication":     publicationExecutor{},
		"error_log":       errorLogExecutor{},
		"rule_filter":     ruleFilterExecutor{},
		"transform_merge": transformMergeExecutor{},
	}}
}

// Execute runs a bounded, deterministic worklist over the immutable DAG.
func (e *PipelineEngine) Execute(ctx context.Context, definition PipelineDefinition, input PipelineExecutionInput) (Result, error) {
	if err := validatePipelineDefinition(definition); err != nil {
		return Result{}, err
	}
	if input.Progress == nil {
		input.Progress = func(string, string, string, map[string]any, time.Time, error) {}
	}
	baseRuntime := &pipelineRuntime{
		definition: definition,
		input:      input,
		meta: map[string]any{
			"pipeline_definition": definition.Key,
			"pipeline_version":    definition.Version,
			"pipeline_version_id": definition.VersionID,
			"trigger_source":      input.TriggerSource,
			"stage_metrics":       []map[string]any{},
		},
		providers: map[string]providers.LLMProvider{},
	}
	started := time.Now()
	executionID := int64(0)
	if e.repo != nil {
		var err error
		executionID, err = e.repo.BeginPipelineExecution(ctx, input.Job.ID, PipelineExecutionSnapshot{Definition: definition, BasePrompt: input.BasePrompt, Policy: input.Policy})
		if err != nil {
			return Result{}, err
		}
	}
	baseRuntime.repo = e.repo
	baseRuntime.executionID = executionID
	executionStatus := "completed"
	byID := map[int64]PipelineStage{}
	byKey := map[string]PipelineStage{}
	for _, stage := range definition.Stages {
		byID[stage.ID] = stage
		byKey[stage.Key] = stage
	}
	work := []schedulerEvent{}
	triggerSource := input.TriggerSource
	if triggerSource == "" {
		triggerSource = "webhook"
	}
	for _, trigger := range definition.Triggers {
		if trigger.Source != triggerSource {
			continue
		}
		if !trigger.Enabled || trigger.TargetStageKey == "" {
			return Result{}, fmt.Errorf("pipeline entrypoint %s is disabled or disconnected", triggerSource)
		}
		stage, ok := byKey[trigger.TargetStageKey]
		if !ok {
			return Result{}, fmt.Errorf("pipeline entrypoint %s targets unknown stage %s", triggerSource, trigger.TargetStageKey)
		}
		work = append(work, schedulerEvent{stage: stage, state: baseRuntime})
		break
	}
	if len(work) == 0 {
		return Result{}, fmt.Errorf("pipeline entrypoint %s not found", triggerSource)
	}
	maxRuns := definition.SchedulerMaxRuns
	if maxRuns < 1 {
		maxRuns = 256
	}
	runs, published, publicationScheduled := 0, false, false
	var completedResult Result
	transitionTraversals := make(map[int]int, len(definition.Transitions))
	incoming := map[int64][]int{}
	outgoing := map[int64][]int{}
	for index, edge := range definition.Transitions {
		outgoing[edge.FromStageID] = append(outgoing[edge.FromStageID], index)
		if edge.ToStageID != nil {
			incoming[*edge.ToStageID] = append(incoming[*edge.ToStageID], index)
		}
	}
	type joinBucket struct {
		signals map[int][]schedulerEvent
		fired   bool
	}
	joins := map[int64]*joinBucket{}
	propagateClose := func(stage PipelineStage, source schedulerEvent) {
		for _, edgeIndex := range outgoing[stage.ID] {
			edge := &definition.Transitions[edgeIndex]
			if edge.MaxTraversals > 0 && transitionTraversals[edgeIndex] >= edge.MaxTraversals {
				continue
			}
			if edge.ToStageID != nil {
				transitionTraversals[edgeIndex]++
				work = append(work, schedulerEvent{stage: byID[*edge.ToStageID], via: edge, state: source.state, closed: true, originals: source.originals})
			}
		}
	}
	for len(work) > 0 {
		if runs >= maxRuns {
			err := errors.New("pipeline scheduler bound exceeded")
			if e.repo != nil {
				_ = e.repo.FinishPipelineExecution(ctx, executionID, "failed", err)
			}
			return Result{}, err
		}
		runs++
		item := work[0]
		work = work[1:]
		stage := item.stage
		joinMode := stage.JoinMode
		if joinMode == "" {
			joinMode = "each_arrival"
		}
		if item.via != nil && joinMode != "each_arrival" {
			bucket := joins[stage.ID]
			if bucket == nil {
				bucket = &joinBucket{signals: map[int][]schedulerEvent{}}
				joins[stage.ID] = bucket
			}
			edgeIndex := transitionIndex(definition.Transitions, item.via)
			bucket.signals[edgeIndex] = append(bucket.signals[edgeIndex], item)
			if joinMode == "any" {
				if item.closed || bucket.fired {
					allClosed := true
					for _, index := range incoming[stage.ID] {
						if len(bucket.signals[index]) == 0 {
							allClosed = false
						}
					}
					if allClosed {
						if !bucket.fired {
							propagateClose(stage, item)
						}
						for _, index := range incoming[stage.ID] {
							bucket.signals[index] = bucket.signals[index][1:]
						}
						bucket.fired = false
					}
					continue
				}
				bucket.fired = true
				if len(incoming[stage.ID]) == 1 {
					bucket.signals[edgeIndex] = bucket.signals[edgeIndex][1:]
					bucket.fired = false
				}
			} else {
				ready := true
				for _, index := range incoming[stage.ID] {
					if len(bucket.signals[index]) == 0 {
						ready = false
						break
					}
				}
				if !ready {
					continue
				}
				joined := []pipelineArtifact{}
				sources := []pipelineArtifact{}
				for _, index := range incoming[stage.ID] {
					signal := bucket.signals[index][0]
					bucket.signals[index] = bucket.signals[index][1:]
					if signal.artifact != nil {
						joined = append(joined, *signal.artifact)
						if len(signal.originals) > 0 {
							sources = append(sources, signal.originals...)
						} else {
							sources = append(sources, *signal.artifact)
						}
					}
				}
				if len(joined) == 0 {
					propagateClose(stage, item)
					continue
				}
				artifact, joinErr := mergePipelineArtifacts(joined)
				if joinErr != nil {
					return Result{}, fmt.Errorf("join %s: %w", stage.Key, joinErr)
				}
				item.artifact = &artifact
				item.originals = sources
			}
		}
		if item.closed {
			propagateClose(stage, item)
			continue
		}
		// Multiple branches may converge on the single publication stage. Its
		// external effect is intentionally scheduled once; repository-level
		// publication reservation remains the second line of defense.
		if stage.ExecutorKey == "publication" {
			if publicationScheduled {
				continue
			}
			publicationScheduled = true
		}
		runtime := runtimeForEvent(baseRuntime, item)
		executor, ok := e.executors[stage.ExecutorKey]
		if !ok {
			err := fmt.Errorf("executor %q não registrado", stage.ExecutorKey)
			if e.repo != nil {
				_ = e.repo.FinishPipelineExecution(ctx, executionID, "failed", err)
			}
			return Result{}, err
		}
		stageExecutionID := int64(0)
		stageStarted := time.Now().UTC()
		if e.repo != nil {
			var err error
			stageExecutionID, stageStarted, err = e.repo.beginStageExecution(ctx, executionID, stage)
			if err != nil {
				_ = e.repo.FinishPipelineExecution(ctx, executionID, "failed", err)
				return Result{}, err
			}
		}
		stageCtx := ctx
		cancel := func() {}
		if stage.TimeoutSeconds > 0 {
			stageCtx, cancel = context.WithTimeout(ctx, time.Duration(stage.TimeoutSeconds)*time.Second)
		}
		if item.artifact != nil && stage.InputContract != nil && stage.ExecutorKey != "error_log" {
			if contractErr := validateContractPayload(stage.InputContract.SchemaJSON, item.artifact.Payload); contractErr != nil {
				stageErr := fmt.Errorf("contract_invalid: %w", contractErr)
				cancel()
				outcome := StageOutcome{Status: "failed"}
				if e.repo != nil {
					_, _ = e.repo.finishStageExecution(ctx, executionID, stageExecutionID, stageStarted, outcome.Status, "", nil, nil, stageErr, item.via, item.originals)
				}
				work, stageErr = e.routeStage(definition, byID, outgoing, transitionTraversals, work, item, stage, outcome, stageErr, runtime)
				if stageErr != nil {
					if e.repo != nil {
						_ = e.repo.FinishPipelineExecution(ctx, executionID, "failed", stageErr)
					}
					return Result{}, fmt.Errorf("etapa %s: %w", stage.Key, stageErr)
				}
				continue
			}
		}
		outcome, stageErr := executor.Execute(stageCtx, runtime, stage)
		cancel()
		if outcome.Status == "" {
			if stageErr != nil {
				outcome.Status = "failed"
			} else {
				outcome.Status = "completed"
			}
		}
		if stageErr == nil && stage.OutputContract != nil && outcome.Artifact != nil {
			if contractErr := validateContractPayload(stage.OutputContract.SchemaJSON, outcome.Artifact); contractErr != nil {
				stageErr = fmt.Errorf("contract_invalid: %w", contractErr)
				outcome.Status = "failed"
			}
		}
		artifactID := int64(0)
		if e.repo != nil {
			var persistErr error
			artifactID, persistErr = e.repo.finishStageExecution(ctx, executionID, stageExecutionID, stageStarted, outcome.Status, outcome.ArtifactType, outcome.Artifact, outcome.Metadata, stageErr, item.via, item.originals)
			if stageErr == nil && persistErr != nil {
				stageErr = persistErr
			}
		}
		if stage.ExecutorKey == "publication" && stageErr == nil {
			published = true
		}
		if stageErr == nil && outcome.Artifact != nil {
			item.artifact = &pipelineArtifact{ID: artifactID, Type: outcome.ArtifactType, Payload: outcome.Artifact}
			item.originals = []pipelineArtifact{*item.artifact}
		}
		if stage.ExecutorKey == "formatting" && stageErr == nil {
			completedResult = runtime.result
		}
		var unhandled error
		work, unhandled = e.routeStage(definition, byID, outgoing, transitionTraversals, work, item, stage, outcome, stageErr, runtime)
		if unhandled != nil {
			if e.repo != nil {
				_ = e.repo.FinishPipelineExecution(ctx, executionID, "failed", unhandled)
			}
			return Result{}, fmt.Errorf("etapa %s: %w", stage.Key, unhandled)
		}
		if outcome.Status == "waiting" {
			executionStatus = "waiting"
		} else if outcome.Status == "cancelled" {
			executionStatus = "cancelled"
		}
	}
	baseRuntime.meta["total_duration_ms"] = time.Since(started).Milliseconds()
	if completedResult.Metadata != nil {
		completedResult.Metadata["total_duration_ms"] = time.Since(started).Milliseconds()
	}
	if completedResult.Metadata == nil {
		completedResult.Metadata = baseRuntime.meta
	}
	if !published {
		if e.repo != nil {
			if err := e.repo.FinishPipelineExecution(ctx, executionID, executionStatus, nil); err != nil {
				return Result{}, err
			}
		}
		return Result{}, nil
	}
	if err := validateResult(completedResult); err != nil {
		if e.repo != nil {
			_ = e.repo.FinishPipelineExecution(ctx, executionID, "failed", err)
		}
		return Result{}, err
	}
	if e.repo != nil {
		if err := e.repo.FinishPipelineExecution(ctx, executionID, executionStatus, nil); err != nil {
			return Result{}, err
		}
	}
	return completedResult, nil
}

func (e *PipelineEngine) routeStage(definition PipelineDefinition, byID map[int64]PipelineStage, outgoing map[int64][]int, traversals map[int]int, work []schedulerEvent, item schedulerEvent, stage PipelineStage, outcome StageOutcome, stageErr error, runtime *pipelineRuntime) ([]schedulerEvent, error) {
	matched := false
	selected := -1
	projected := map[int]any{}
	if stageErr == nil {
		for _, edgeIndex := range outgoing[stage.ID] {
			edge := &definition.Transitions[edgeIndex]
			if edge.ToStageID == nil || edge.Type != "success" && edge.Type != "skip" {
				continue
			}
			edgeMatched := false
			payload := outcome.Artifact
			if edge.Rule != nil {
				schema := ""
				if stage.OutputContract != nil {
					schema = stage.OutputContract.SchemaJSON
				}
				var err error
				edgeMatched, payload, err = EvaluateRuleArtifact(*edge.Rule, schema, outcome.Artifact)
				if err != nil {
					return e.routeStage(definition, byID, outgoing, traversals, work, item, stage, StageOutcome{}, fmt.Errorf("contract_invalid: %w", err), runtime)
				}
			} else {
				edgeMatched = conditionMatches(edge.ConditionKey, outcome, runtime)
			}
			if edgeMatched {
				selected = edgeIndex
				projected[edgeIndex] = payload
				if stage.RouteMode == "first_match" {
					break
				}
			}
		}
	}
	for _, edgeIndex := range outgoing[stage.ID] {
		edge := &definition.Transitions[edgeIndex]
		if edge.ToStageID == nil {
			continue
		}
		edgeMatched := false
		exhausted := false
		if stageErr != nil {
			edgeMatched = (edge.Type == "failure" || edge.Type == "fallback") && edge.ConditionKey == technicalErrorKind(stageErr)
		} else if edge.Type == "success" || edge.Type == "skip" {
			_, edgeMatched = projected[edgeIndex]
			if selected >= 0 && stage.RouteMode != "first_match" {
				_, edgeMatched = projected[edgeIndex]
			}
		}
		if edgeMatched {
			traversals[edgeIndex]++
			if edge.MaxTraversals > 0 && traversals[edgeIndex] > edge.MaxTraversals {
				edgeMatched = false
				exhausted = true
			}
		}
		if !edgeMatched {
			if exhausted {
				continue
			}
			traversals[edgeIndex]++
			if edge.MaxTraversals > 0 && traversals[edgeIndex] > edge.MaxTraversals {
				continue
			}
			work = append(work, schedulerEvent{stage: byID[*edge.ToStageID], via: edge, state: runtime, closed: true, originals: item.originals})
			continue
		}
		matched = true
		if stageErr != nil {
			work = append(work, schedulerEvent{stage: byID[*edge.ToStageID], via: edge, artifact: item.artifact, state: runtime, originals: item.originals, errPayload: normalizedStageError(stage, *edge, stageErr)})
			continue
		}
		payload := outcome.Artifact
		if value, ok := projected[edgeIndex]; ok {
			payload = value
		}
		artifact := pipelineArtifact{Type: outcome.ArtifactType, Payload: payload}
		if item.artifact != nil {
			artifact.ID = item.artifact.ID
		}
		work = append(work, schedulerEvent{stage: byID[*edge.ToStageID], via: edge, artifact: &artifact, state: runtime, originals: []pipelineArtifact{artifact}})
	}
	if stageErr != nil && !matched {
		return work, stageErr
	}
	return work, nil
}

func transitionIndex(edges []PipelineTransition, target *PipelineTransition) int {
	for index := range edges {
		if &edges[index] == target || edges[index].ID != 0 && edges[index].ID == target.ID {
			return index
		}
	}
	return -1
}

func runtimeForEvent(base *pipelineRuntime, item schedulerEvent) *pipelineRuntime {
	if item.state != nil {
		base = item.state
	}
	consumed := item.originals
	if item.artifact != nil {
		consumed = []pipelineArtifact{*item.artifact}
	}
	runtime := &pipelineRuntime{
		definition: base.definition, input: base.input, repo: base.repo, executionID: base.executionID,
		providers: base.providers, currentError: item.errPayload, currentArtifacts: append([]pipelineArtifact(nil), consumed...),
		currentSources: append([]pipelineArtifact(nil), item.originals...),
		meta:           map[string]any{}, files: append([]pipelineFile(nil), base.files...), plan: base.plan,
		reviews: append([]pipelineGroupReview(nil), base.reviews...), failed: append([]string(nil), base.failed...),
		consolidated: base.consolidated, consolidatedReady: base.consolidatedReady,
		approved: append([]pipelineFinding(nil), base.approved...), result: base.result, providerName: base.providerName,
	}
	for key, value := range base.meta {
		if metrics, ok := value.([]map[string]any); ok {
			runtime.meta[key] = append([]map[string]any(nil), metrics...)
		} else {
			runtime.meta[key] = value
		}
	}
	if item.artifact != nil {
		hydrateRuntime(runtime, *item.artifact)
	}
	return runtime
}

func hydrateRuntime(runtime *pipelineRuntime, artifact pipelineArtifact) {
	switch artifact.Type {
	case "prepared_diff":
		var value struct {
			Files   []pipelineFile `json:"files"`
			Ignored int            `json:"ignored"`
		}
		if decodeArtifactPayload(artifact.Payload, &value) == nil {
			runtime.files = value.Files
		}
	case "review_plan":
		_ = decodeArtifactPayload(artifact.Payload, &runtime.plan)
	case "review_findings":
		_ = decodeArtifactPayload(artifact.Payload, &runtime.reviews)
	case "consolidated_findings":
		if decodeArtifactPayload(artifact.Payload, &runtime.consolidated) == nil {
			runtime.consolidatedReady = true
		}
	case "verified_findings":
		_ = decodeArtifactPayload(artifact.Payload, &runtime.approved)
	case "formatted_review":
		_ = decodeArtifactPayload(artifact.Payload, &runtime.result)
	}
}

func decodeArtifactPayload(payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func validateContractPayload(schemaJSON string, payload any) error {
	if schemaJSON == "" {
		return nil
	}
	var schema any
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return err
	}
	normalized, err := normalizeJSON(payload)
	if err != nil {
		return err
	}
	return validateSchemaValue(schema, normalized, "$", true)
}

func mergePipelineArtifacts(items []pipelineArtifact) (pipelineArtifact, error) {
	if len(items) == 0 {
		return pipelineArtifact{}, errors.New("no artifacts to merge")
	}
	out := pipelineArtifact{Type: items[0].Type}
	for _, item := range items[1:] {
		if item.Type != out.Type {
			return pipelineArtifact{}, fmt.Errorf("incompatible artifact types %s and %s", out.Type, item.Type)
		}
	}
	switch out.Type {
	case "prepared_diff":
		files := []pipelineFile{}
		ignored := 0
		for _, item := range items {
			var value struct {
				Files   []pipelineFile `json:"files"`
				Ignored int            `json:"ignored"`
			}
			if err := decodeArtifactPayload(item.Payload, &value); err != nil {
				return out, err
			}
			files = append(files, value.Files...)
			ignored += value.Ignored
		}
		out.Payload = map[string]any{"files": files, "ignored": ignored}
	case "review_findings":
		values := []pipelineGroupReview{}
		for _, item := range items {
			var value []pipelineGroupReview
			if err := decodeArtifactPayload(item.Payload, &value); err != nil {
				return out, err
			}
			values = append(values, value...)
		}
		out.Payload = values
	default:
		if len(items) == 1 {
			out.Payload = items[0].Payload
		} else {
			values := make([]any, len(items))
			for index := range items {
				values[index] = items[index].Payload
			}
			out.Payload = values
		}
	}
	return out, nil
}

func technicalErrorKind(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	if strings.Contains(err.Error(), "contract_invalid") {
		return "contract_invalid"
	}
	return "error"
}

func normalizedStageError(stage PipelineStage, edge PipelineTransition, err error) map[string]any {
	return map[string]any{"error": err.Error(), "source_stage": stage.Key, "transition_type": edge.Type, "condition": edge.ConditionKey}
}
func conditionMatches(key string, outcome StageOutcome, runtime *pipelineRuntime) bool {
	if key == "always" {
		return true
	}
	findings := 0
	switch value := outcome.Artifact.(type) {
	case []pipelineGroupReview:
		for _, review := range value {
			findings += len(review.Findings)
		}
	case pipelineConsolidated:
		findings = len(value.Findings)
	case []pipelineFinding:
		findings = len(value)
	}
	switch key {
	case "has_findings":
		return findings > 0
	case "no_findings":
		return findings == 0
	case "partial_result":
		return len(runtime.failed) > 0
	case "contract_invalid":
		return false
	case "confidence_below_threshold":
		for _, finding := range runtime.approved {
			if finding.Confidence < runtime.input.Policy.MinimumConfidence {
				return true
			}
		}
	}
	return false
}

func (r *pipelineRuntime) call(ctx context.Context, stage PipelineStage, marker string, files []pipelineFile, data any) (stageCallResult, error) {
	if r.input.Provider == nil {
		return stageCallResult{}, errors.New("AI provider is not configured")
	}
	providerKey := "profile"
	if stage.ModelID != nil {
		providerKey = fmt.Sprintf("model:%d", *stage.ModelID)
	}
	r.providerMu.Lock()
	provider := r.providers[providerKey]
	if provider == nil {
		var err error
		provider, err = r.input.Provider(ctx, stage.ModelID)
		if err != nil {
			r.providerMu.Unlock()
			return stageCallResult{}, err
		}
		r.providers[providerKey] = provider
	}
	r.providerMu.Unlock()
	prompt := stagePrompt(marker, r.input.BasePrompt, stage.PromptTemplate, stageContractInstruction(stage), files, data)
	call := stageCallResult{Prompt: prompt, Provider: provider.Name(), RequestedOutputTokens: stage.MaxOutputTokens}
	r.metricsMu.Lock()
	r.providerName = provider.Name()
	if model, ok := provider.(providers.ModelName); ok {
		r.meta["model"] = model.Model()
		call.Model = model.Model()
	}
	r.metricsMu.Unlock()
	at := time.Now()
	response, usage, err := provider.Chat(ctx, prompt, safeStageTokens(prompt, stage.MaxOutputTokens, r.input.Policy))
	call.Response = response
	call.Usage = usage
	call.DurationMS = time.Since(at).Milliseconds()
	metric := map[string]any{
		"stage": stage.Key, "duration_ms": call.DurationMS, "input_chars": len(prompt),
		"requested_output_tokens": stage.MaxOutputTokens, "actual_prompt_tokens": usage.PromptTokens,
		"actual_completion_tokens": usage.CompletionTokens, "response_chars": len(response), "failed": err != nil,
		"provider": provider.Name(), "model_id": stage.ModelID,
	}
	r.metricsMu.Lock()
	r.meta["stage_metrics"] = append(r.meta["stage_metrics"].([]map[string]any), metric)
	r.metricsMu.Unlock()
	return call, err
}

func callMetadata(base map[string]any, call stageCallResult) map[string]any {
	metadata := map[string]any{}
	for key, value := range base {
		metadata[key] = value
	}
	metadata["prompt"] = call.Prompt
	metadata["response"] = call.Response
	metadata["provider"] = call.Provider
	metadata["requested_output_tokens"] = call.RequestedOutputTokens
	metadata["actual_prompt_tokens"] = call.Usage.PromptTokens
	metadata["actual_completion_tokens"] = call.Usage.CompletionTokens
	metadata["input_chars"] = len(call.Prompt)
	metadata["response_chars"] = len(call.Response)
	metadata["duration_ms"] = call.DurationMS
	if call.Model != "" {
		metadata["model"] = call.Model
	}
	return metadata
}

func stageContractInstruction(stage PipelineStage) string {
	if stage.OutputContract == nil {
		return ""
	}
	return stage.OutputContract.ResponseInstruction
}

func (r *pipelineRuntime) recordAttempt(ctx context.Context, stage PipelineStage, attempt int, started time.Time, metadata any, attemptErr error) error {
	if r.repo == nil {
		return nil
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return r.repo.recordStageAttempt(auditCtx, r.executionID, stage, attempt, started, metadata, attemptErr)
}

type preparationExecutor struct{}

func (preparationExecutor) Execute(_ context.Context, r *pipelineRuntime, stage PipelineStage) (StageOutcome, error) {
	started := time.Now()
	files, ignored := preparePipelineFiles(r.input.RawDiff)
	r.files = files
	r.meta["total_files"] = len(files) + ignored
	r.meta["reviewed_files"] = len(files)
	r.meta["ignored_files"] = ignored
	r.input.Progress(stage.Key, "concluido", fmt.Sprintf("%d arquivos preparados; %d ignorados", len(files), ignored), map[string]any{"files": filePaths(files)}, started, nil)
	return StageOutcome{ArtifactType: "prepared_diff", Artifact: map[string]any{"files": files, "ignored": ignored}}, nil
}

type plannerExecutor struct{}

func (plannerExecutor) Execute(ctx context.Context, r *pipelineRuntime, stage PipelineStage) (StageOutcome, error) {
	started := time.Now()
	r.input.Progress(stage.Key, "processando", "Agrupando arquivos para review", nil, started, nil)
	plan, fallback := deterministicPlan(r.files, r.input.Policy)
	var plannerCall stageCallResult
	if len(r.files) > 0 && stage.UseLLM {
		var candidate pipelinePlan
		if call, err := r.call(ctx, stage, "planner", r.files, nil); err == nil && decodeStage(call.Response, &candidate) == nil && validPipelinePlan(candidate, r.files) {
			plan = candidate
			fallback = false
			plannerCall = call
		}
	}
	r.plan = plan
	r.meta["planner_fallback"] = fallback
	r.meta["review_groups"] = len(plan.Groups)
	metadata := map[string]any{"fallback": fallback, "total_groups": len(plan.Groups)}
	if r.input.Policy.DetailedStageLogs && plannerCall.Prompt != "" {
		metadata = callMetadata(metadata, plannerCall)
	}
	r.input.Progress(stage.Key, "concluido", fmt.Sprintf("%d grupos de review", len(plan.Groups)), metadata, started, nil)
	return StageOutcome{ArtifactType: "review_plan", Artifact: plan, Metadata: metadata}, nil
}

type reviewerExecutor struct{}

func (reviewerExecutor) Execute(ctx context.Context, r *pipelineRuntime, stage PipelineStage) (StageOutcome, error) {
	if len(r.files) == 0 {
		r.input.Progress(stage.Key, "concluido", "Nenhum arquivo revisável", map[string]any{"total_groups": 0}, time.Now(), nil)
		return StageOutcome{ArtifactType: "review_findings", Artifact: []pipelineGroupReview{}}, nil
	}
	plan := r.plan
	if len(plan.Groups) == 0 {
		plan, _ = deterministicPlan(r.files, r.input.Policy)
	}
	r.input.Progress(stage.Key, "processando", "Revisando grupos de arquivos", map[string]any{"total_groups": len(plan.Groups)}, time.Now(), nil)
	reviews, failed := reviewGroups(ctx, r, stage, plan)
	r.reviews = append(r.reviews, reviews...)
	r.failed = append(r.failed, failed...)
	r.meta["successful_groups"] = len(r.reviews)
	r.meta["failed_groups"] = len(r.failed)
	r.meta["partial_review"] = len(r.failed) > 0
	if len(reviews) == 0 {
		return StageOutcome{}, fmt.Errorf("nenhum grupo retornou uma review válida: %s", strings.Join(failed, "; "))
	}
	metadata := map[string]any{"successful_groups": len(reviews), "failed_groups": len(failed), "source_stage": stage.Key}
	return StageOutcome{ArtifactType: "review_findings", Artifact: reviews, Metadata: metadata}, nil
}

func reviewGroups(ctx context.Context, r *pipelineRuntime, stage PipelineStage, plan pipelinePlan) ([]pipelineGroupReview, []string) {
	parallel := r.input.Policy.MaxParallelGroups
	if parallel < 1 {
		parallel = 1
	}
	attempts := stage.RetryLimit
	if attempts < 1 {
		attempts = 1
	}
	type outcome struct {
		index  int
		review pipelineGroupReview
		failed string
	}
	out := make(chan outcome, len(plan.Groups))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, group := range plan.Groups {
		wg.Add(1)
		sem <- struct{}{}
		go func(index int, group pipelineGroup) {
			defer wg.Done()
			defer func() { <-sem }()
			stageStarted := time.Now()
			groupFiles := selectPipelineFiles(r.files, group.Files)
			r.input.Progress(stage.Key, "processando", "Revisando grupo "+group.ID, map[string]any{"group_id": group.ID, "group_index": index + 1, "total_groups": len(plan.Groups), "files": filePaths(groupFiles)}, stageStarted, nil)
			var review pipelineGroupReview
			var err error
			for attempt := 1; attempt <= attempts; attempt++ {
				attemptStarted := time.Now().UTC()
				call, callErr := r.call(ctx, stage, "reviewer", groupFiles, group)
				attemptMetadata := map[string]any{"group_id": group.ID}
				if r.input.Policy.DetailedStageLogs {
					attemptMetadata = callMetadata(attemptMetadata, call)
				}
				if callErr != nil {
					err = callErr
					if auditErr := r.recordAttempt(ctx, stage, attempt, attemptStarted, attemptMetadata, err); auditErr != nil {
						err = fmt.Errorf("%v; persistir tentativa: %w", err, auditErr)
						break
					}
					if attempt < attempts {
						r.input.Progress(stage.Key, "retentando", fmt.Sprintf("Falha ao consultar o provider no grupo %s: %v", group.ID, err), map[string]any{"group_id": group.ID, "attempt": attempt + 1, "max_attempts": attempts}, stageStarted, err)
						continue
					}
					continue
				}
				if err = decodeStage(call.Response, &review); err == nil {
					review.GroupID = group.ID
					review.ReviewedFiles = group.Files
					review.Findings = validatePipelineFindings(review.Findings, groupFiles)
					for i := range review.Findings {
						review.Findings[i].SourceStage = stage.Key
					}
					if auditErr := r.recordAttempt(ctx, stage, attempt, attemptStarted, attemptMetadata, nil); auditErr != nil {
						err = fmt.Errorf("persistir tentativa: %w", auditErr)
						break
					}
					out <- outcome{index: index, review: review}
					r.input.Progress(stage.Key, "concluido", "Grupo "+group.ID+" revisado", map[string]any{"group_id": group.ID, "attempt": attempt, "max_attempts": attempts}, stageStarted, nil)
					return
				}
				if auditErr := r.recordAttempt(ctx, stage, attempt, attemptStarted, attemptMetadata, err); auditErr != nil {
					err = fmt.Errorf("%v; persistir tentativa: %w", err, auditErr)
					break
				}
				if attempt < attempts {
					r.input.Progress(stage.Key, "retentando", "Contrato inválido no grupo "+group.ID, map[string]any{"group_id": group.ID, "attempt": attempt + 1, "max_attempts": attempts}, stageStarted, err)
				}
			}
			r.input.Progress(stage.Key, "falhou", "Grupo "+group.ID+" falhou", map[string]any{"group_id": group.ID, "attempt": attempts, "max_attempts": attempts}, stageStarted, err)
			out <- outcome{index: index, failed: fmt.Sprintf("%s: %v", group.ID, err)}
		}(i, group)
	}
	wg.Wait()
	close(out)
	byIndex := make([]pipelineGroupReview, len(plan.Groups))
	good := make([]bool, len(plan.Groups))
	failed := []string{}
	for item := range out {
		if item.failed != "" {
			failed = append(failed, item.failed)
		} else {
			byIndex[item.index] = item.review
			good[item.index] = true
		}
	}
	reviews := []pipelineGroupReview{}
	for i := range byIndex {
		if good[i] {
			reviews = append(reviews, byIndex[i])
		}
	}
	return reviews, failed
}

type consolidatorExecutor struct{}

func (consolidatorExecutor) Execute(ctx context.Context, r *pipelineRuntime, stage PipelineStage) (StageOutcome, error) {
	started := time.Now()
	r.input.Progress(stage.Key, "processando", "Consolidando achados", nil, started, nil)
	consolidated := fallbackConsolidation(r.reviews)
	fallback := true
	var consolidatorCall stageCallResult
	if len(r.files) > 0 && stage.UseLLM {
		var candidate pipelineConsolidated
		if call, err := r.call(ctx, stage, "consolidator", r.files, r.reviews); err == nil && decodeStage(call.Response, &candidate) == nil {
			consolidated = candidate
			fallback = false
			consolidatorCall = call
		}
	}
	consolidated.Findings = validatePipelineFindings(consolidated.Findings, r.files)
	r.consolidated = consolidated
	r.consolidatedReady = true
	r.meta["consolidator_fallback"] = fallback
	r.meta["consolidated_findings"] = len(consolidated.Findings)
	metadata := map[string]any{"fallback": fallback, "findings": len(consolidated.Findings)}
	if r.input.Policy.DetailedStageLogs && consolidatorCall.Prompt != "" {
		metadata = callMetadata(metadata, consolidatorCall)
	}
	r.input.Progress(stage.Key, "concluido", fmt.Sprintf("%d achados consolidados", len(consolidated.Findings)), metadata, started, nil)
	return StageOutcome{ArtifactType: "consolidated_findings", Artifact: consolidated, Metadata: metadata}, nil
}

type verificationExecutor struct{}

func (verificationExecutor) Execute(ctx context.Context, r *pipelineRuntime, stage PipelineStage) (StageOutcome, error) {
	started := time.Now()
	r.input.Progress(stage.Key, "processando", "Validando relevância dos achados", nil, started, nil)
	if !r.consolidatedReady && len(r.reviews) > 0 {
		r.consolidated = fallbackConsolidation(r.reviews)
	}
	approved := r.consolidated.Findings
	fallback := false
	var verifierCall stageCallResult
	if len(approved) > 0 && stage.UseLLM {
		var verification pipelineVerification
		if call, err := r.call(ctx, stage, "verifier", r.files, r.consolidated); err == nil && decodeStage(call.Response, &verification) == nil {
			approved = verifiedFindings(r.consolidated.Findings, verification, r.input.Policy.MinimumConfidence)
			verifierCall = call
		} else {
			fallback = true
			approved = confidenceFindings(approved, r.input.Policy.MinimumConfidence)
		}
	} else if !stage.UseLLM {
		fallback = true
		approved = confidenceFindings(approved, r.input.Policy.MinimumConfidence)
	}
	r.approved = approved
	r.meta["verifier_fallback"] = fallback
	r.meta["confirmed_findings"] = len(approved)
	r.meta["rejected_findings"] = len(r.consolidated.Findings) - len(approved)
	metadata := map[string]any{"fallback": fallback, "confirmed": len(approved)}
	if r.input.Policy.DetailedStageLogs && verifierCall.Prompt != "" {
		metadata = callMetadata(metadata, verifierCall)
	}
	r.input.Progress(stage.Key, "concluido", fmt.Sprintf("%d achados confirmados", len(approved)), metadata, started, nil)
	return StageOutcome{ArtifactType: "verified_findings", Artifact: approved, Metadata: metadata}, nil
}

type formattingExecutor struct{}

func (formattingExecutor) Execute(ctx context.Context, r *pipelineRuntime, stage PipelineStage) (StageOutcome, error) {
	started := time.Now()
	r.input.Progress(stage.Key, "processando", "Montando comentário final", nil, started, nil)
	summary := r.consolidated.Summary
	if len(r.files) == 0 {
		summary = "Nenhum arquivo revisavel encontrado no diff."
	}
	result := deterministicPipelineResult(r.input.Job.ID, r.providerName, r.approved, summary, len(r.failed) > 0, r.input.Policy, r.meta)
	if model, ok := r.meta["model"].(string); ok {
		result.Model = model
	}
	fallback := true
	var formatterCall stageCallResult
	if len(r.files) > 0 && stage.UseLLM {
		var formatted pipelineFormatted
		if call, err := r.call(ctx, stage, "formatter", r.files, r.approved); err == nil && decodeStage(call.Response, &formatted) == nil && formatted.FinalReview.Summary != "" {
			formatted.Comments = validFormattedComments(formatted.Comments, r.approved)
			if formatted.Comments != nil {
				result.Comments = make([]Comment, 0, len(formatted.Comments))
				for _, comment := range formatted.Comments {
					result.Comments = append(result.Comments, Comment{File: comment.File, Line: comment.Line, Severity: comment.Severity, DecisionReason: comment.DecisionReason, Comment: comment.Comment})
				}
			}
			result.FinalReview = formatted.FinalReview
			fallback = false
			formatterCall = call
		}
	}
	applyPipelineDecision(&result, len(r.failed) > 0, r.input.Policy)
	r.meta["formatter_fallback"] = fallback
	result.Metadata = r.meta
	if err := validateResult(result); err != nil {
		return StageOutcome{}, err
	}
	r.result = result
	metadata := map[string]any{"fallback": fallback, "comments": len(result.Comments)}
	if r.input.Policy.DetailedStageLogs && formatterCall.Prompt != "" {
		metadata = callMetadata(metadata, formatterCall)
	}
	r.input.Progress(stage.Key, "concluido", fmt.Sprintf("%d comentários formatados", len(result.Comments)), metadata, started, nil)
	return StageOutcome{ArtifactType: "formatted_review", Artifact: result, Metadata: metadata}, nil
}

type publicationExecutor struct{}

func (publicationExecutor) Execute(ctx context.Context, r *pipelineRuntime, _ PipelineStage) (StageOutcome, error) {
	if r.input.Publish == nil {
		return StageOutcome{ArtifactType: "publication_result", Artifact: map[string]string{"status": "skipped"}}, nil
	}
	return r.input.Publish(ctx, r.result)
}

type errorLogExecutor struct{}

func (errorLogExecutor) Execute(_ context.Context, r *pipelineRuntime, _ PipelineStage) (StageOutcome, error) {
	payload := r.currentError
	if payload == nil {
		payload = map[string]any{"error": "unknown pipeline error"}
	}
	return StageOutcome{Status: "completed", ArtifactType: "error_log", Artifact: payload, Metadata: payload}, nil
}

type ruleFilterExecutor struct{}

func (ruleFilterExecutor) Execute(_ context.Context, r *pipelineRuntime, stage PipelineStage) (StageOutcome, error) {
	if len(r.currentArtifacts) == 0 {
		return StageOutcome{}, errors.New("rule_filter requires an input artifact")
	}
	input := r.currentArtifacts[0]
	payload := input.Payload
	if rawRule, ok := stage.Config["rule"]; ok {
		body, err := json.Marshal(rawRule)
		if err != nil {
			return StageOutcome{}, err
		}
		var rule Rule
		if err = json.Unmarshal(body, &rule); err != nil {
			return StageOutcome{}, fmt.Errorf("invalid rule_filter rule: %w", err)
		}
		schema := ""
		if stage.InputContract != nil {
			schema = stage.InputContract.SchemaJSON
		}
		matched, projected, err := EvaluateRuleArtifact(rule, schema, payload)
		if err != nil {
			return StageOutcome{}, err
		}
		if !matched {
			projected = emptyFilteredArtifact(input.Type, payload)
		}
		payload = projected
	}
	if extensions, ok := stringList(stage.Config["include_extensions"]); ok && input.Type == "prepared_diff" {
		var value struct {
			Files   []pipelineFile `json:"files"`
			Ignored int            `json:"ignored"`
		}
		if err := decodeArtifactPayload(payload, &value); err != nil {
			return StageOutcome{}, err
		}
		filtered := value.Files[:0]
		for _, file := range value.Files {
			for _, extension := range extensions {
				if strings.HasSuffix(strings.ToLower(file.Path), strings.ToLower(extension)) {
					filtered = append(filtered, file)
					break
				}
			}
		}
		payload = map[string]any{"files": filtered, "ignored": value.Ignored + len(value.Files) - len(filtered)}
	}
	return StageOutcome{ArtifactType: input.Type, Artifact: payload}, nil
}

func emptyFilteredArtifact(artifactType string, payload any) any {
	if artifactType == "prepared_diff" {
		return map[string]any{"files": []pipelineFile{}, "ignored": 0}
	}
	normalized, _ := normalizeJSON(payload)
	if _, ok := normalized.([]any); ok {
		return []any{}
	}
	return normalized
}

func stringList(value any) ([]string, bool) {
	if value == nil {
		return nil, false
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var out []string
	if json.Unmarshal(body, &out) != nil {
		return nil, false
	}
	return out, len(out) > 0
}

type transformMergeExecutor struct{}

func (transformMergeExecutor) Execute(_ context.Context, r *pipelineRuntime, stage PipelineStage) (StageOutcome, error) {
	if len(r.currentArtifacts) == 0 {
		return StageOutcome{}, errors.New("transform_merge requires input artifacts")
	}
	merged, err := mergePipelineArtifacts(r.currentArtifacts)
	if err != nil {
		return StageOutcome{}, err
	}
	operation, _ := stage.Config["operation"].(string)
	if operation == "" || operation == "identity" || operation == "merge" {
		return StageOutcome{ArtifactType: merged.Type, Artifact: merged.Payload}, nil
	}
	if operation == "dedupe_findings" && merged.Type == "review_findings" {
		var groups []pipelineGroupReview
		if err = decodeArtifactPayload(merged.Payload, &groups); err != nil {
			return StageOutcome{}, err
		}
		seen := map[string]bool{}
		for groupIndex := range groups {
			findings := groups[groupIndex].Findings[:0]
			for _, finding := range groups[groupIndex].Findings {
				key := finding.ID
				if key == "" {
					key = fmt.Sprintf("%s:%d:%s", finding.File, finding.Line, finding.Comment)
				}
				if !seen[key] {
					seen[key] = true
					findings = append(findings, finding)
				}
			}
			groups[groupIndex].Findings = findings
		}
		return StageOutcome{ArtifactType: merged.Type, Artifact: groups}, nil
	}
	return StageOutcome{}, fmt.Errorf("unsupported transform_merge operation %q", operation)
}
