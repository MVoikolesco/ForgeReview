package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	basereview "gitea-agents/internal/review"
)

type Runner struct {
	Config     Config
	Logger     *log.Logger
	Chat       ChatFunc
	StackRules func(context.Context, []string) (string, []string, error)
}

func (r Runner) Run(ctx context.Context, input Input) (string, basereview.FinalReview, Metadata, error) {
	started := time.Now()
	cfg := normalizeConfig(r.Config)
	meta := Metadata{PipelineVersion: Version, PlannerUsed: cfg.PlannerEnabled, TotalFiles: len(input.Files)}
	if truncatedInput, truncated := truncateInputDiff(input, cfg); truncated {
		input = truncatedInput
		meta.DiffTruncated = true
		meta.PartialReview = true
		r.logf("pipeline diff truncado owner=%s repo=%s pr=%d files=%d", input.Owner, input.Repository, input.PullRequestNumber, len(input.Files))
	}
	var metricsMu sync.Mutex
	originalChat := r.Chat
	r.Chat = func(ctx context.Context, stage string, prompt string, maxOutputTokens int) (string, StageUsage, error) {
		stageStarted := time.Now()
		response, usage, err := originalChat(ctx, stage, prompt, maxOutputTokens)
		metric := StageMetric{Stage: stage, DurationMillis: time.Since(stageStarted).Milliseconds(), InputChars: len(prompt), EstimatedInputTokens: EstimateTokens(prompt), RequestedOutputTokens: maxOutputTokens, ActualPromptTokens: usage.PromptTokens, ActualCompletionTokens: usage.CompletionTokens, ResponseChars: len(response), Failed: err != nil}
		metricsMu.Lock()
		meta.StageMetrics = append(meta.StageMetrics, metric)
		metricsMu.Unlock()
		return response, usage, err
	}
	r.logf("pipeline iniciado owner=%s repo=%s pr=%d files=%d", input.Owner, input.Repository, input.PullRequestNumber, len(input.Files))

	plan, plannerFallback := r.plan(ctx, cfg, input)
	meta.PlannerFallback = plannerFallback
	meta.ReviewGroups = len(plan.Groups)
	if len(plan.Groups) == 0 {
		plan = FallbackPlan(input, cfg, "planner sem grupos")
		meta.PlannerFallback = true
		meta.ReviewGroups = len(plan.Groups)
	}

	reviews, failedGroups, err := r.reviewGroups(ctx, cfg, input, plan)
	if err != nil {
		return "", basereview.FinalReview{}, meta, err
	}
	meta.SuccessfulGroups = len(reviews)
	meta.FailedGroups = len(failedGroups)
	meta.PartialReview = meta.PartialReview || len(failedGroups) > 0
	for _, review := range reviews {
		meta.RawFindings += len(review.Findings)
		meta.ReviewedFiles += len(review.ReviewedFiles)
	}
	if len(reviews) == 0 {
		return "", basereview.FinalReview{}, meta, fmt.Errorf("todos os grupos falharam")
	}

	consolidated, consolidatedFallback := r.consolidate(ctx, cfg, input, plan, reviews, failedGroups)
	meta.ConsolidatorFallback = consolidatedFallback
	meta.ConsolidatedFindings = len(consolidated.Findings)

	approved, rejected, verifierFallback := r.verify(ctx, cfg, input, consolidated)
	meta.VerifierFallback = verifierFallback
	meta.ConfirmedFindings = len(approved)
	meta.RejectedFindings = rejected

	response, formatterFallback := r.format(ctx, cfg, consolidated, approved, meta)
	meta.FormatterFallback = formatterFallback
	response.Metadata = meta
	raw, _ := json.Marshal(response)
	parsed, err := basereview.ValidateFinalReviewResponse(string(raw))
	if err != nil {
		return "", basereview.FinalReview{}, meta, err
	}
	r.logf("pipeline concluido duration=%s partial=%t comments=%d", time.Since(started).Round(time.Millisecond), meta.PartialReview, len(response.Comments))
	return string(raw), parsed, meta, nil
}

func (r Runner) plan(ctx context.Context, cfg Config, input Input) (ReviewPlan, bool) {
	if !cfg.PlannerEnabled {
		return FallbackPlan(input, cfg, "planner desabilitado"), true
	}
	prompt := plannerPrompt(input)
	max, err := SafeOutputTokens(cfg, prompt, cfg.PlannerMaxOutputTokens)
	if err != nil {
		return FallbackPlan(input, cfg, err.Error()), true
	}
	started := time.Now()
	raw, _, err := r.Chat(ctx, "planner", prompt, max)
	if err != nil {
		r.logf("planner fallback reason=%v", err)
		return FallbackPlan(input, cfg, err.Error()), true
	}
	var plan ReviewPlan
	if err := parseJSONStage("planner", raw, &plan); err != nil || !validPlan(plan, input) {
		if err == nil {
			err = fmt.Errorf("agrupamento invalido")
		}
		r.logf("planner fallback reason=%v", err)
		return FallbackPlan(input, cfg, err.Error()), true
	}
	normalizePlan(&plan)
	r.logf("planner concluido groups=%d duration=%s", len(plan.Groups), time.Since(started).Round(time.Millisecond))
	return plan, false
}

func (r Runner) reviewGroups(ctx context.Context, cfg Config, input Input, plan ReviewPlan) ([]GroupReview, []string, error) {
	parallel := cfg.MaxParallelReviewGroups
	if parallel <= 0 {
		parallel = 1
	}
	sem := make(chan struct{}, parallel)
	type result struct {
		index   int
		review  GroupReview
		groupID string
		err     error
	}
	results := make(chan result, len(plan.Groups))
	var wg sync.WaitGroup
	for index, group := range plan.Groups {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(index int, group ReviewGroup) {
			defer wg.Done()
			defer func() { <-sem }()
			res := result{index: index, groupID: group.ID}
			stackRules := ""
			if r.StackRules != nil {
				rules, stacks, err := r.StackRules(ctx, group.Files)
				if err != nil {
					res.err = err
					results <- res
					return
				}
				stackRules = rules
				if len(group.RelevantStacks) == 0 {
					group.RelevantStacks = stacks
				}
			}
			prompt := reviewerPrompt(input, plan, group, stackRules)
			max, err := SafeOutputTokens(cfg, prompt, cfg.ReviewerMaxOutputTokens)
			if err != nil {
				res.err = err
				results <- res
				return
			}
			started := time.Now()
			raw, _, err := r.Chat(ctx, "reviewer", prompt, max)
			if err != nil {
				res.err = err
				results <- res
				return
			}
			var review GroupReview
			if err := parseJSONStage("reviewer", raw, &review); err != nil {
				res.err = err
				results <- res
				return
			}
			review.GroupID = group.ID
			review.ReviewedFiles = filterExistingPaths(review.ReviewedFiles, group.Files)
			if len(review.ReviewedFiles) == 0 {
				review.ReviewedFiles = append([]string(nil), group.Files...)
			}
			review.Findings = validateFindings(review.Findings, input, group.ID)
			r.logf("review group concluido group=%s findings=%d duration=%s", group.ID, len(review.Findings), time.Since(started).Round(time.Millisecond))
			res.review = review
			results <- res
		}(index, group)
	}
	wg.Wait()
	close(results)
	reviewsByIndex := make([]GroupReview, len(plan.Groups))
	okByIndex := make([]bool, len(plan.Groups))
	var failed []string
	for res := range results {
		if res.err != nil {
			r.logf("review group falhou group=%s err=%v", res.groupID, res.err)
			failed = append(failed, res.groupID)
			continue
		}
		reviewsByIndex[res.index] = res.review
		okByIndex[res.index] = true
	}
	var reviews []GroupReview
	for i := range reviewsByIndex {
		if okByIndex[i] {
			reviews = append(reviews, reviewsByIndex[i])
		}
	}
	return reviews, failed, nil
}

func (r Runner) consolidate(ctx context.Context, cfg Config, input Input, plan ReviewPlan, reviews []GroupReview, failed []string) (ConsolidatedReview, bool) {
	if !cfg.ConsolidatorEnabled {
		return fallbackConsolidate(reviews), true
	}
	prompt := consolidatorPrompt(input, plan, reviews, failed)
	max, err := SafeOutputTokens(cfg, prompt, cfg.ConsolidatorMaxOutputTokens)
	if err != nil {
		return fallbackConsolidate(reviews), true
	}
	raw, _, err := r.Chat(ctx, "consolidator", prompt, max)
	if err != nil {
		return fallbackConsolidate(reviews), true
	}
	var consolidated ConsolidatedReview
	if err := parseJSONStage("consolidator", raw, &consolidated); err != nil {
		return fallbackConsolidate(reviews), true
	}
	consolidated.Findings = validateFindings(consolidated.Findings, input, "")
	if consolidated.PRSummary == "" {
		consolidated.PRSummary = plan.PRSummary
	}
	if consolidated.OverallRisk == "" {
		consolidated.OverallRisk = highestRisk(consolidated.Findings)
	}
	return consolidated, false
}

func (r Runner) verify(ctx context.Context, cfg Config, input Input, consolidated ConsolidatedReview) ([]ReviewFinding, int, bool) {
	if len(consolidated.Findings) == 0 {
		return nil, 0, false
	}
	if !cfg.VerifierEnabled {
		return filterByConfidence(consolidated.Findings, cfg.MinimumPublishConfidence+0.1), 0, true
	}
	prompt := verifierPrompt(input, consolidated)
	max, err := SafeOutputTokens(cfg, prompt, cfg.VerifierMaxOutputTokens)
	if err != nil {
		return filterByConfidence(consolidated.Findings, cfg.MinimumPublishConfidence+0.1), 0, true
	}
	raw, _, err := r.Chat(ctx, "verifier", prompt, max)
	if err != nil {
		return filterByConfidence(consolidated.Findings, cfg.MinimumPublishConfidence+0.1), 0, true
	}
	var response VerificationResponse
	if err := parseJSONStage("verifier", raw, &response); err != nil {
		return filterByConfidence(consolidated.Findings, cfg.MinimumPublishConfidence+0.1), 0, true
	}
	byID := map[string]ReviewFinding{}
	for _, f := range consolidated.Findings {
		byID[f.ID] = f
	}
	var approved []ReviewFinding
	rejected := 0
	for _, result := range response.Results {
		base, ok := byID[result.FindingID]
		if !ok {
			continue
		}
		confidence := clampConfidence(result.Confidence)
		if confidence < cfg.MinimumPublishConfidence {
			rejected++
			continue
		}
		switch strings.ToLower(strings.TrimSpace(result.Status)) {
		case "confirmed":
			base.Confidence = confidence
			approved = append(approved, base)
		case "adjusted":
			if result.AdjustedFinding != nil {
				adjusted := *result.AdjustedFinding
				if adjusted.ID == "" {
					adjusted.ID = base.ID
				}
				approved = append(approved, validateFindings([]ReviewFinding{adjusted}, input, base.SourceGroupID)...)
			} else {
				rejected++
			}
		default:
			rejected++
		}
	}
	return approved, rejected, false
}

func (r Runner) format(ctx context.Context, cfg Config, consolidated ConsolidatedReview, findings []ReviewFinding, meta Metadata) (FinalResponse, bool) {
	if cfg.FormatterEnabled {
		prompt := formatterPrompt(consolidated, findings, meta)
		if max, err := SafeOutputTokens(cfg, prompt, cfg.FormatterMaxOutputTokens); err == nil {
			if raw, _, err := r.Chat(ctx, "formatter", prompt, max); err == nil {
				var response FinalResponse
				if err := parseJSONStage("formatter", raw, &response); err == nil && response.FinalReview.Summary != "" {
					response = enforceDecision(response, findings, meta, cfg, consolidated.PRSummary)
					return response, false
				}
			}
		}
	}
	return deterministicFinal(findings, meta, cfg, consolidated.PRSummary), true
}

func normalizeConfig(cfg Config) Config {
	d := DefaultConfig()
	if cfg.PlannerMaxOutputTokens == 0 {
		cfg.PlannerMaxOutputTokens = d.PlannerMaxOutputTokens
	}
	if cfg.ReviewerMaxOutputTokens == 0 {
		cfg.ReviewerMaxOutputTokens = d.ReviewerMaxOutputTokens
	}
	if cfg.ConsolidatorMaxOutputTokens == 0 {
		cfg.ConsolidatorMaxOutputTokens = d.ConsolidatorMaxOutputTokens
	}
	if cfg.VerifierMaxOutputTokens == 0 {
		cfg.VerifierMaxOutputTokens = d.VerifierMaxOutputTokens
	}
	if cfg.FormatterMaxOutputTokens == 0 {
		cfg.FormatterMaxOutputTokens = d.FormatterMaxOutputTokens
	}
	if cfg.SafetyMarginTokens == 0 {
		cfg.SafetyMarginTokens = d.SafetyMarginTokens
	}
	if cfg.MinimumPublishConfidence == 0 {
		cfg.MinimumPublishConfidence = d.MinimumPublishConfidence
	}
	if cfg.MaxParallelReviewGroups == 0 {
		cfg.MaxParallelReviewGroups = d.MaxParallelReviewGroups
	}
	if cfg.MediumSeverityEvent == "" {
		cfg.MediumSeverityEvent = d.MediumSeverityEvent
	}
	if cfg.PartialEvent == "" {
		cfg.PartialEvent = d.PartialEvent
	}
	if cfg.MaxGroupChars == 0 {
		cfg.MaxGroupChars = d.MaxGroupChars
	}
	if cfg.MaxFilesPerGroup == 0 {
		cfg.MaxFilesPerGroup = d.MaxFilesPerGroup
	}
	return cfg
}

func (r Runner) logf(format string, args ...any) {
	if r.Logger != nil {
		r.Logger.Printf(format, args...)
	}
}
