package review

import (
	"context"
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
	Job        queueInput
	RawDiff    string
	BasePrompt string
	Policy     Policy
	Provider   stageProviderResolver
	Progress   stageProgress
	Publish    func(context.Context, Result) (StageOutcome, error)
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
		"preparation":  preparationExecutor{},
		"planner":      plannerExecutor{},
		"reviewer":     reviewerExecutor{},
		"consolidator": consolidatorExecutor{},
		"verification": verificationExecutor{},
		"formatting":   formattingExecutor{},
		"publication":  publicationExecutor{},
		"error_log":    errorLogExecutor{},
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
	runtime := &pipelineRuntime{
		definition: definition,
		input:      input,
		meta: map[string]any{
			"pipeline_definition": definition.Key,
			"pipeline_version":    definition.Version,
			"pipeline_version_id": definition.VersionID,
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
	runtime.repo = e.repo
	runtime.executionID = executionID
	executionStatus := "completed"
	byID := map[int64]PipelineStage{}
	incoming := map[int64]int{}
	for _, stage := range definition.Stages {
		byID[stage.ID] = stage
	}
	for _, edge := range definition.Transitions {
		if (edge.Type == "success" || edge.Type == "skip") && edge.ToStageID != nil {
			incoming[*edge.ToStageID]++
		}
	}
	type arrival struct {
		stage      PipelineStage
		via        *PipelineTransition
		errPayload map[string]any
	}
	work := []arrival{}
	for _, stage := range definition.Stages {
		if stage.ExecutorKey == "preparation" && incoming[stage.ID] == 0 {
			work = append(work, arrival{stage: stage})
		}
	}
	maxRuns := len(definition.Stages) * (len(definition.Transitions) + 1) * 4
	if maxRuns < 16 {
		maxRuns = 16
	}
	runs, published, publicationScheduled := 0, false, false
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
		// Multiple branches may converge on the single publication stage. Its
		// external effect is intentionally scheduled once; repository-level
		// publication reservation remains the second line of defense.
		if stage.ExecutorKey == "publication" {
			if publicationScheduled {
				continue
			}
			publicationScheduled = true
		}
		runtime.currentError = item.errPayload
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
		outcome, stageErr := executor.Execute(stageCtx, runtime, stage)
		cancel()
		if outcome.Status == "" {
			if stageErr != nil {
				outcome.Status = "failed"
			} else {
				outcome.Status = "completed"
			}
		}
		if e.repo != nil {
			persistErr := e.repo.finishStageExecution(ctx, executionID, stageExecutionID, stageStarted, outcome.Status, outcome.ArtifactType, outcome.Artifact, outcome.Metadata, stageErr, item.via)
			if stageErr == nil && persistErr != nil {
				stageErr = persistErr
			}
		}
		if stage.ExecutorKey == "publication" && stageErr == nil {
			published = true
		}
		matched := false
		for _, edge := range definition.Transitions {
			if edge.FromStageID != stage.ID || edge.ToStageID == nil {
				continue
			}
			isError := stageErr != nil
			if (edge.Type == "failure" || edge.Type == "fallback") && isError {
				payload := normalizedStageError(stage, edge, stageErr)
				work = append(work, arrival{stage: byID[*edge.ToStageID], via: &edge, errPayload: payload})
				matched = true
			} else if (edge.Type == "success" || edge.Type == "skip") && !isError && conditionMatches(edge.ConditionKey, outcome, runtime) {
				work = append(work, arrival{stage: byID[*edge.ToStageID], via: &edge})
				matched = true
			}
		}
		if stageErr != nil && !matched {
			if e.repo != nil {
				_ = e.repo.FinishPipelineExecution(ctx, executionID, "failed", stageErr)
			}
			return Result{}, fmt.Errorf("etapa %s: %w", stage.Key, stageErr)
		}
		if outcome.Status == "waiting" {
			executionStatus = "waiting"
		} else if outcome.Status == "cancelled" {
			executionStatus = "cancelled"
		}
	}
	runtime.meta["total_duration_ms"] = time.Since(started).Milliseconds()
	if runtime.result.Metadata == nil {
		runtime.result.Metadata = runtime.meta
	}
	if !published || len(runtime.result.Comments) == 0 && runtime.result.FinalReview.Summary == "" {
		err := errors.New("pipeline ended without a valid published formatted result")
		if e.repo != nil {
			_ = e.repo.FinishPipelineExecution(ctx, executionID, "failed", err)
		}
		return Result{}, err
	}
	if err := validateResult(runtime.result); err != nil {
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
	return runtime.result, nil
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
