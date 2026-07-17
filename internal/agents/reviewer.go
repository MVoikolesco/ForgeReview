package agents

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gitea-agents/internal/ai"
	"gitea-agents/internal/diff"
	"gitea-agents/internal/gitea"
	"gitea-agents/internal/ollama"
	"gitea-agents/internal/openrouter"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"
	"gitea-agents/internal/review/pipeline"
	"gitea-agents/internal/review/promptconfig"
	"gitea-agents/internal/reviewconfig"
)

const ReviewerAgentName = "reviewer"
const defaultDiffLogDir = "/tmp/gitea-ai-reviewer-diffs"

type GiteaClient interface {
	GetPullRequestDiff(ctx context.Context, owner string, repo string, prNumber int) (string, error)
	CreatePullRequestReview(ctx context.Context, owner string, repo string, prNumber int, options gitea.CreatePullReviewOptions) (gitea.PullReview, error)
}

type GiteaClientResolver interface {
	Resolve(ctx context.Context, instanceID int64, owner, repo string) (*gitea.Client, error)
}

type AIReviewerClient interface {
	Chat(ctx context.Context, prompt string) (string, error)
	Model() string
}

type ollamaMetadataClient interface {
	ChatWithMetadata(ctx context.Context, prompt string) (ai.ChatResult, error)
}

type ollamaMaxTokenClient interface {
	ChatWithMaxTokens(ctx context.Context, prompt string, maxOutputTokens int) (ai.ChatResult, error)
}

type textMaxTokenClient interface {
	ChatWithMaxTokens(ctx context.Context, prompt string, maxOutputTokens int) (string, error)
}

type ReviewerOptions struct {
	DiffLogDir             string
	MaxBlockChars          int
	MaxFilesPerBlock       int
	ReviewConcurrency      int
	OllamaTimeoutSeconds   int
	ReviewFinalRetries     int
	ReviewPromptConfigPath string
	PublishManualReviews   bool
	AllowAutonomousReject  bool
	ConfigProvider         reviewconfig.Provider
	GiteaResolver          GiteaClientResolver
	LogSensitiveData       bool
	UnloadAfterReview      bool
	ContextWindow          int
	MaxOutputTokens        int
	PipelineConfig         pipeline.Config
	PipelineConfigSet      bool
}

type ReviewerAgent struct {
	logger       *log.Logger
	giteaClient  GiteaClient
	ollamaClient AIReviewerClient
	options      ReviewerOptions
}

func NewReviewerAgent(logger *log.Logger, giteaClient GiteaClient, ollamaClient AIReviewerClient, options ReviewerOptions) *ReviewerAgent {
	// Directly constructed agents are used by local unit tests. Production always
	// supplies ConfigProvider and then takes this flag from the SQLite policy.
	if options.ConfigProvider == nil {
		options.LogSensitiveData = true
	}
	if options.DiffLogDir == "" {
		options.DiffLogDir = defaultDiffLogDir
	}

	if options.MaxBlockChars <= 0 {
		options.MaxBlockChars = 4000
	}

	if options.MaxFilesPerBlock <= 0 {
		options.MaxFilesPerBlock = 2
	}

	if options.ReviewConcurrency <= 0 {
		options.ReviewConcurrency = 1
	}

	if options.OllamaTimeoutSeconds <= 0 {
		options.OllamaTimeoutSeconds = 900
	}
	if options.ReviewFinalRetries <= 0 {
		options.ReviewFinalRetries = 5
	}
	if options.ReviewPromptConfigPath == "" {
		options.ReviewPromptConfigPath = "./config/review-prompts.yaml"
	}

	return &ReviewerAgent{
		logger:       logger,
		giteaClient:  giteaClient,
		ollamaClient: ollamaClient,
		options:      options,
	}
}

func (a *ReviewerAgent) Name() string {
	return ReviewerAgentName
}

func (a *ReviewerAgent) Process(ctx context.Context, job queue.ReviewJob) error {
	if a.options.ConfigProvider != nil {
		runtime := *a
		if a.options.GiteaResolver != nil {
			client, err := a.options.GiteaResolver.Resolve(ctx, job.GiteaInstanceID, job.Owner, job.Repo)
			if err != nil {
				return err
			}
			runtime.giteaClient = client
		}
		cfg, err := a.options.ConfigProvider.GetConfig(ctx, job.Owner+"/"+job.Repo)
		if err != nil {
			return fmt.Errorf("review configuration is required for %s/%s: %w", job.Owner, job.Repo, err)
		}
		client, err := reviewerClientFromConfig(cfg)
		if err != nil {
			return err
		}
		runtime.ollamaClient = client
		runtime.options.MaxBlockChars = cfg.Policy.MaxBlockChars
		runtime.options.MaxFilesPerBlock = cfg.Policy.MaxFilesPerBlock
		runtime.options.ReviewConcurrency = cfg.Policy.ReviewConcurrency
		runtime.options.OllamaTimeoutSeconds = cfg.Parameters.TimeoutSeconds
		runtime.options.ReviewFinalRetries = cfg.Policy.ReviewFinalRetries
		runtime.options.PublishManualReviews = cfg.Policy.PublishManualReviews
		runtime.options.AllowAutonomousReject = cfg.Policy.AllowAutonomousRejection
		runtime.options.LogSensitiveData = cfg.Policy.LogSensitiveData
		runtime.options.UnloadAfterReview = cfg.Policy.UnloadModelAfterReview || cfg.Parameters.UnloadModelAfterReview
		runtime.options.ContextWindow = cfg.Model.ContextWindow
		runtime.options.MaxOutputTokens = cfg.Model.MaxOutputTokens
		runtime.options.PipelineConfig = pipeline.Config{
			PlannerEnabled:              cfg.Pipeline.PlannerEnabled,
			ConsolidatorEnabled:         cfg.Pipeline.ConsolidatorEnabled,
			VerifierEnabled:             cfg.Pipeline.VerifierEnabled,
			FormatterEnabled:            cfg.Pipeline.FormatterEnabled,
			PlannerMaxOutputTokens:      cfg.Pipeline.PlannerMaxOutputTokens,
			ReviewerMaxOutputTokens:     cfg.Pipeline.GroupMaxOutputTokens,
			ConsolidatorMaxOutputTokens: cfg.Pipeline.ConsolidatorMaxOutputTokens,
			VerifierMaxOutputTokens:     cfg.Pipeline.VerifierMaxOutputTokens,
			FormatterMaxOutputTokens:    cfg.Pipeline.FormatterMaxOutputTokens,
			SafetyMarginTokens:          cfg.Pipeline.ContextSafetyMarginTokens,
			MinimumPublishConfidence:    cfg.Pipeline.MinimumPublishConfidence,
			MaxParallelReviewGroups:     cfg.Pipeline.MaxParallelGroups,
			MediumSeverityEvent:         cfg.Pipeline.MediumSeverityEvent,
			PartialEvent:                cfg.Pipeline.PartialEvent,
		}
		runtime.options.PipelineConfigSet = true
		return runtime.process(ctx, job)
	}
	if a.options.GiteaResolver != nil {
		runtime := *a
		client, err := a.options.GiteaResolver.Resolve(ctx, job.GiteaInstanceID, job.Owner, job.Repo)
		if err != nil {
			return err
		}
		runtime.giteaClient = client
		return runtime.process(ctx, job)
	}
	return a.process(ctx, job)
}

func reviewerClientFromConfig(cfg *reviewconfig.ReviewConfig) (AIReviewerClient, error) {
	switch cfg.Provider.Name {
	case "ollama":
		key := cfg.Connection.APIKey
		return ollama.NewClient(ollama.Config{URL: cfg.Connection.BaseURL, Model: cfg.Model.Name, APIKey: key, Options: ollama.Options{Temperature: cfg.Parameters.Temperature, TopP: cfg.Parameters.TopP, RepeatPenalty: cfg.Parameters.RepeatPenalty, NumCtx: cfg.Parameters.NumCtx, NumThread: cfg.Parameters.NumThreads, NumPredict: cfg.Parameters.NumPredict}, KeepAlive: cfg.Parameters.KeepAlive, TimeoutSeconds: cfg.Parameters.TimeoutSeconds}), nil
	case "openrouter":
		key := cfg.Connection.APIKey
		if key == "" {
			return nil, fmt.Errorf("OpenRouter stored API key is unavailable")
		}
		return openrouter.NewClient(openrouter.Config{URL: cfg.Connection.BaseURL, Model: cfg.Model.Name, APIKey: key, HTTPReferer: cfg.Connection.HTTPReferer, AppTitle: cfg.Connection.AppTitle, Temperature: cfg.Parameters.Temperature, TopP: cfg.Parameters.TopP, ContextWindow: cfg.Model.ContextWindow, MaxTokens: cfg.Model.MaxOutputTokens, TimeoutSeconds: cfg.Parameters.TimeoutSeconds}), nil
	default:
		return nil, fmt.Errorf("provider %q is not implemented", cfg.Provider.Name)
	}
}

func (a *ReviewerAgent) process(ctx context.Context, job queue.ReviewJob) error {
	if a.options.UnloadAfterReview {
		defer func() {
			unloadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if client, ok := a.ollamaClient.(interface {
				Unload(context.Context, string) error
			}); ok {
				if err := client.Unload(unloadCtx, a.ollamaClient.Model()); err != nil {
					a.logger.Printf("erro ao descarregar modelo model=%s err=%v", a.ollamaClient.Model(), err)
				}
			}
		}()
	}
	return a.processPipeline(ctx, job)
}

func formatManualReviewMarkdown(finalReview review.FinalReview) string {
	var builder strings.Builder
	builder.WriteString("# Review final\n\n")
	builder.WriteString("- **Status:** ")
	builder.WriteString(finalReview.Status)
	builder.WriteString("\n- **Evento Gitea:** ")
	builder.WriteString(finalReview.Event)
	builder.WriteString("\n\n")
	builder.WriteString(finalReview.ReviewBody())

	if len(finalReview.InlineComments) > 0 {
		builder.WriteString("\n\n## Comentarios inline\n")
		for _, comment := range finalReview.InlineComments {
			builder.WriteString("\n### ")
			builder.WriteString(comment.Path)
			if comment.NewPosition > 0 {
				builder.WriteString(":")
				builder.WriteString(strconv.Itoa(comment.NewPosition))
			}
			builder.WriteString("\n\n")
			if comment.Severity != "" {
				builder.WriteString("**Severidade:** ")
				builder.WriteString(comment.Severity)
				builder.WriteString("\n\n")
			}
			builder.WriteString(comment.Body)
			builder.WriteString("\n")
		}
	}

	return strings.TrimSpace(builder.String()) + "\n"
}

func (a *ReviewerAgent) requestValidatedFinalReview(ctx context.Context, prompt string, runLog *reviewRunLog) (string, review.FinalReview, ai.ChatResult, error) {
	currentPrompt := prompt
	var validationErrors []string
	for attempt := 1; attempt <= a.options.ReviewFinalRetries; attempt++ {
		response, usage, err := a.chatWithMetadata(ctx, currentPrompt)
		if err != nil {
			return "", review.FinalReview{}, ai.ChatResult{}, fmt.Errorf("erro ao consolidar review com ollama: %w", err)
		}
		parsed, validationErr := review.ValidateFinalReviewResponse(response)
		if validationErr == nil {
			return response, parsed, usage, nil
		}

		validationErrors = append(validationErrors, validationErr.Error())
		if appendErr := runLog.AppendProcess("resposta final invalida tentativa=%d de %d erros=%s", attempt, a.options.ReviewFinalRetries, validationErr.Error()); appendErr != nil {
			return "", review.FinalReview{}, ai.ChatResult{}, appendErr
		}
		if attempt == a.options.ReviewFinalRetries {
			break
		}
		currentPrompt = prompt + "\n\nCORRECAO OBRIGATORIA DA TENTATIVA ANTERIOR:\nA ultima resposta nao veio no padrao obrigatorio. Refaça a resposta completa e retorne exclusivamente um objeto JSON valido, sem Markdown e sem qualquer texto antes ou depois. Preserve exatamente as propriedades comments e final_review, incluindo todos os campos obrigatorios. Utilize somente os valores de severidade, status e evento definidos pelo projeto. Erros detectados: " + validationErr.Error()
	}

	return "", review.FinalReview{}, ai.ChatResult{}, fmt.Errorf("review final invalido apos %d tentativas: %s", a.options.ReviewFinalRetries, strings.Join(validationErrors, " | "))
}

func (a *ReviewerAgent) chatWithMetadata(ctx context.Context, prompt string) (string, ai.ChatResult, error) {
	if client, ok := a.ollamaClient.(ollamaMetadataClient); ok {
		result, err := client.ChatWithMetadata(ctx, prompt)
		return result.Content, result, err
	}
	content, err := a.ollamaClient.Chat(ctx, prompt)
	return content, ai.ChatResult{Content: content}, err
}

func (a *ReviewerAgent) chatStageWithMetadata(ctx context.Context, stage string, prompt string, maxOutputTokens int) (string, pipeline.StageUsage, error) {
	if client, ok := a.ollamaClient.(ollamaMaxTokenClient); ok {
		result, err := client.ChatWithMaxTokens(ctx, prompt, maxOutputTokens)
		return result.Content, pipeline.StageUsage{PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens}, err
	}
	if client, ok := a.ollamaClient.(textMaxTokenClient); ok {
		content, err := client.ChatWithMaxTokens(ctx, prompt, maxOutputTokens)
		return content, pipeline.StageUsage{}, err
	}
	content, usage, err := a.chatWithMetadata(ctx, prompt)
	return content, pipeline.StageUsage{PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens}, err
}

func (a *ReviewerAgent) processPipeline(ctx context.Context, job queue.ReviewJob) error {
	rawDiff, err := a.giteaClient.GetPullRequestDiff(ctx, job.Owner, job.Repo, job.PRNumber)
	if err != nil {
		return err
	}
	a.logger.Printf("diff obtido owner=%s repo=%s pr=%d size=%d", job.Owner, job.Repo, job.PRNumber, len(rawDiff))

	runLog, err := newReviewRunLog(a.options.DiffLogDir, job, a.options.LogSensitiveData)
	if err != nil {
		return err
	}
	a.logger.Printf("logs fisicos do review dir=%s", runLog.Dir())
	progress := func(event pipeline.ProgressEvent) {
		if err := runLog.AppendProgress(event); err != nil {
			a.logger.Printf("erro ao gravar progresso do review: %v", err)
		}
	}
	progress(pipeline.ProgressEvent{Stage: "preparacao", Status: "running", Percent: 5, Message: "Baixando e analisando diff"})

	files := diff.Parse(rawDiff)
	a.logger.Printf("diff parseado owner=%s repo=%s pr=%d files=%d", job.Owner, job.Repo, job.PRNumber, len(files))
	for _, file := range files {
		a.logger.Printf("arquivo alterado path=%s additions=%d deletions=%d patch_size=%d", file.Path, file.Additions, file.Deletions, len(file.Patch))
	}
	if err := runLog.AppendProcess("pipeline v2 iniciado files=%d diff_chars=%d", len(files), len(rawDiff)); err != nil {
		return err
	}
	if _, err := runLog.Write("01-diff-completo.log", formatFullDiffLog(rawDiff, files)); err != nil {
		return err
	}

	promptResolver := promptconfig.NewPromptResolver(a.options.ReviewPromptConfigPath)
	if _, err := promptResolver.Load(ctx); err != nil {
		return err
	}
	a.logger.Printf("prompt config carregado path=%s", a.options.ReviewPromptConfigPath)
	resolvedBlockFilter, err := promptResolver.ResolveBlockFilter(ctx, job.Owner, job.Repo)
	if err != nil {
		return err
	}
	blockFilter := reviewFileFilterFromConfig(resolvedBlockFilter)
	var reviewable []diff.ChangedFile
	for _, file := range files {
		if review.ShouldIgnoreReviewFileWithFilter(file.Path, blockFilter) {
			continue
		}
		reviewable = append(reviewable, file)
	}
	if len(reviewable) == 0 {
		progress(pipeline.ProgressEvent{Stage: "preparacao", Status: "done", Percent: 100, Message: "Nenhum arquivo revisavel encontrado"})
		finalReview := review.FinalReview{Event: review.GiteaEventComment, Status: "comentado", Summary: "Nenhum arquivo revisavel encontrado no diff.", Structured: true}
		if job.Manual && !a.options.PublishManualReviews {
			_, err := runLog.Write("final-review.md", formatManualReviewMarkdown(finalReview))
			return err
		}
		created, err := a.giteaClient.CreatePullRequestReview(ctx, job.Owner, job.Repo, job.PRNumber, buildCreatePullReviewOptions(finalReview, a.options.AllowAutonomousReject))
		if err != nil {
			return fmt.Errorf("erro ao publicar review no gitea: %w", err)
		}
		a.logger.Printf("review publicado no gitea owner=%s repo=%s pr=%d review_id=%d state=%s event=%s comments=%d", job.Owner, job.Repo, job.PRNumber, created.ID, created.State, finalReview.Event, 0)
		return nil
	}

	prompts, err := promptResolver.ResolvePrompts(ctx)
	if err != nil {
		return err
	}
	author := job.Author
	if author == "" {
		author = job.Sender
	}
	input := pipeline.Input{Owner: job.Owner, Repository: job.Repo, PullRequestNumber: job.PRNumber, Title: job.Title, Description: job.Description, Author: author, BaseBranch: job.BaseBranch, HeadBranch: job.HeadBranch, Files: reviewable, Prompts: prompts}
	cfg := pipelineConfigFromEnv()
	if a.options.PipelineConfigSet {
		cfg = mergePipelineConfig(cfg, a.options.PipelineConfig)
	}
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = a.options.ContextWindow
	}
	if a.options.MaxOutputTokens > 0 {
		cfg.PlannerMaxOutputTokens = minPositive(cfg.PlannerMaxOutputTokens, a.options.MaxOutputTokens)
		cfg.ReviewerMaxOutputTokens = minPositive(cfg.ReviewerMaxOutputTokens, a.options.MaxOutputTokens)
		cfg.ConsolidatorMaxOutputTokens = minPositive(cfg.ConsolidatorMaxOutputTokens, a.options.MaxOutputTokens)
		cfg.VerifierMaxOutputTokens = minPositive(cfg.VerifierMaxOutputTokens, a.options.MaxOutputTokens)
		cfg.FormatterMaxOutputTokens = minPositive(cfg.FormatterMaxOutputTokens, a.options.MaxOutputTokens)
	}
	cfg.MaxGroupChars = a.options.MaxBlockChars
	cfg.MaxFilesPerGroup = a.options.MaxFilesPerBlock
	cfg.ContractMaxAttempts = a.options.ReviewFinalRetries
	if cfg.MaxParallelReviewGroups <= 0 {
		cfg.MaxParallelReviewGroups = 1
	}

	runner := pipeline.Runner{Config: cfg, Logger: a.logger, Chat: a.chatStageWithMetadata, Progress: progress, ProcessLog: func(format string, args ...any) {
		if err := runLog.AppendProcess(format, args...); err != nil {
			a.logger.Printf("erro ao gravar log de processo do review: %v", err)
		}
	}}
	started := time.Now()
	finalResponse, finalReview, metadata, err := runner.Run(ctx, input)
	if err != nil {
		progress(pipeline.ProgressEvent{Stage: "erro", Status: "failed", Percent: 100, Message: err.Error()})
		if appendErr := runLog.AppendProcess("pipeline v2 falhou err=%v", err); appendErr != nil {
			return appendErr
		}
		return err
	}
	finalReview.Metadata = review.ReviewMetadata{Model: a.ollamaClient.Model(), Elapsed: time.Since(started).Round(time.Millisecond).String()}
	if _, err := runLog.Write("final-resposta.log", formatPipelineFinalResponseLog(finalResponse, time.Since(started).Round(time.Millisecond), metadata)); err != nil {
		return err
	}
	if err := runLog.AppendProcess("pipeline v2 concluido groups=%d successful=%d failed=%d raw_findings=%d confirmed=%d partial=%t", metadata.ReviewGroups, metadata.SuccessfulGroups, metadata.FailedGroups, metadata.RawFindings, metadata.ConfirmedFindings, metadata.PartialReview); err != nil {
		return err
	}
	parsedFinalReview := review.ResolveFinalReviewCommentPositions(finalReview, files)
	if job.Manual && !a.options.PublishManualReviews {
		if _, err := runLog.Write("final-review.md", formatManualReviewMarkdown(parsedFinalReview)); err != nil {
			return err
		}
		progress(pipeline.ProgressEvent{Stage: "publicacao", Status: "done", Percent: 100, Message: "Review manual salvo sem publicar"})
		a.logger.Printf("review manual salvo sem publicar no gitea dir=%s", runLog.Dir())
		return nil
	}
	progress(pipeline.ProgressEvent{Stage: "publicacao", Status: "running", Percent: 98, Message: "Publicando review no Gitea"})
	createReviewOptions := buildCreatePullReviewOptions(parsedFinalReview, a.options.AllowAutonomousReject)
	createdReview, err := a.giteaClient.CreatePullRequestReview(ctx, job.Owner, job.Repo, job.PRNumber, createReviewOptions)
	if err != nil {
		progress(pipeline.ProgressEvent{Stage: "publicacao", Status: "failed", Percent: 100, Message: err.Error()})
		if appendErr := runLog.AppendProcess("erro ao publicar review no gitea event=%s comments=%d err=%v", createReviewOptions.Event, len(createReviewOptions.Comments), err); appendErr != nil {
			return appendErr
		}
		return fmt.Errorf("erro ao publicar review no gitea: %w", err)
	}
	a.logger.Printf("review publicado no gitea owner=%s repo=%s pr=%d review_id=%d state=%s event=%s comments=%d", job.Owner, job.Repo, job.PRNumber, createdReview.ID, createdReview.State, createReviewOptions.Event, len(createReviewOptions.Comments))
	progress(pipeline.ProgressEvent{Stage: "publicacao", Status: "done", Percent: 100, Message: fmt.Sprintf("Review publicado com %d comentarios", len(createReviewOptions.Comments))})
	return nil
}

func pipelineConfigFromEnv() pipeline.Config {
	cfg := pipeline.DefaultConfig()
	cfg.PlannerEnabled = getEnvBool("REVIEW_PLANNER_ENABLED", true)
	cfg.ConsolidatorEnabled = getEnvBool("REVIEW_CONSOLIDATOR_ENABLED", true)
	cfg.VerifierEnabled = getEnvBool("REVIEW_VERIFIER_ENABLED", true)
	cfg.FormatterEnabled = getEnvBool("REVIEW_FORMATTER_ENABLED", true)
	cfg.PlannerMaxOutputTokens = getEnvIntLocal("REVIEW_PLANNER_MAX_OUTPUT_TOKENS", cfg.PlannerMaxOutputTokens)
	cfg.ReviewerMaxOutputTokens = getEnvIntLocal("REVIEW_GROUP_MAX_OUTPUT_TOKENS", cfg.ReviewerMaxOutputTokens)
	cfg.ConsolidatorMaxOutputTokens = getEnvIntLocal("REVIEW_CONSOLIDATOR_MAX_OUTPUT_TOKENS", cfg.ConsolidatorMaxOutputTokens)
	cfg.VerifierMaxOutputTokens = getEnvIntLocal("REVIEW_VERIFIER_MAX_OUTPUT_TOKENS", cfg.VerifierMaxOutputTokens)
	cfg.FormatterMaxOutputTokens = getEnvIntLocal("REVIEW_FORMATTER_MAX_OUTPUT_TOKENS", cfg.FormatterMaxOutputTokens)
	cfg.SafetyMarginTokens = getEnvIntLocal("REVIEW_CONTEXT_SAFETY_MARGIN_TOKENS", cfg.SafetyMarginTokens)
	cfg.MaxParallelReviewGroups = getEnvIntLocal("REVIEW_MAX_PARALLEL_GROUPS", cfg.MaxParallelReviewGroups)
	cfg.ContextWindow = getEnvIntLocal("REVIEW_CONTEXT_WINDOW", cfg.ContextWindow)
	cfg.MaxInputTokens = getEnvIntLocal("REVIEW_MAX_INPUT_TOKENS", cfg.MaxInputTokens)
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = 0
	}
	cfg.MinimumPublishConfidence = getEnvFloatLocal("REVIEW_MIN_PUBLISH_CONFIDENCE", cfg.MinimumPublishConfidence)
	cfg.MediumSeverityEvent = getEnvStringLocal("REVIEW_MEDIUM_SEVERITY_EVENT", cfg.MediumSeverityEvent)
	cfg.PartialEvent = getEnvStringLocal("REVIEW_PARTIAL_EVENT", cfg.PartialEvent)
	return cfg
}

func mergePipelineConfig(base, override pipeline.Config) pipeline.Config {
	base.PlannerEnabled = override.PlannerEnabled
	base.ConsolidatorEnabled = override.ConsolidatorEnabled
	base.VerifierEnabled = override.VerifierEnabled
	base.FormatterEnabled = override.FormatterEnabled
	if override.PlannerMaxOutputTokens > 0 {
		base.PlannerMaxOutputTokens = override.PlannerMaxOutputTokens
	}
	if override.ReviewerMaxOutputTokens > 0 {
		base.ReviewerMaxOutputTokens = override.ReviewerMaxOutputTokens
	}
	if override.ConsolidatorMaxOutputTokens > 0 {
		base.ConsolidatorMaxOutputTokens = override.ConsolidatorMaxOutputTokens
	}
	if override.VerifierMaxOutputTokens > 0 {
		base.VerifierMaxOutputTokens = override.VerifierMaxOutputTokens
	}
	if override.FormatterMaxOutputTokens > 0 {
		base.FormatterMaxOutputTokens = override.FormatterMaxOutputTokens
	}
	if override.SafetyMarginTokens > 0 {
		base.SafetyMarginTokens = override.SafetyMarginTokens
	}
	if override.MinimumPublishConfidence > 0 {
		base.MinimumPublishConfidence = override.MinimumPublishConfidence
	}
	if override.MaxParallelReviewGroups > 0 {
		base.MaxParallelReviewGroups = override.MaxParallelReviewGroups
	}
	if override.MediumSeverityEvent != "" {
		base.MediumSeverityEvent = override.MediumSeverityEvent
	}
	if override.PartialEvent != "" {
		base.PartialEvent = override.PartialEvent
	}
	return base
}

func getEnvStringLocal(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getEnvIntLocal(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvFloatLocal(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func minPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

func formatPipelineFinalResponseLog(response string, duration time.Duration, metadata pipeline.Metadata) string {
	stageCount := len(metadata.StageMetrics)
	inputChars := 0
	estimatedInputTokens := 0
	requestedOutputTokens := 0
	actualPromptTokens := 0
	actualCompletionTokens := 0
	for _, metric := range metadata.StageMetrics {
		inputChars += metric.InputChars
		estimatedInputTokens += metric.EstimatedInputTokens
		requestedOutputTokens += metric.RequestedOutputTokens
		actualPromptTokens += metric.ActualPromptTokens
		actualCompletionTokens += metric.ActualCompletionTokens
	}
	return fmt.Sprintf(`===== METADADOS RESPOSTA PROVIDER =====
tipo=final
pipeline_version=%s
review_groups=%d
successful_groups=%d
failed_groups=%d
raw_findings=%d
consolidated_findings=%d
confirmed_findings=%d
rejected_findings=%d
partial_review=%t
diff_truncated=%t
stage_calls=%d
input_chars=%d
estimated_input_tokens=%d
requested_output_tokens=%d
actual_prompt_tokens=%d
actual_completion_tokens=%d
response_chars=%d
response_duration=%s
===== RESPOSTA =====
%s`, metadata.PipelineVersion, metadata.ReviewGroups, metadata.SuccessfulGroups, metadata.FailedGroups, metadata.RawFindings, metadata.ConsolidatedFindings, metadata.ConfirmedFindings, metadata.RejectedFindings, metadata.PartialReview, metadata.DiffTruncated, stageCount, inputChars, estimatedInputTokens, requestedOutputTokens, actualPromptTokens, actualCompletionTokens, len(response), duration, response)
}

func buildCreatePullReviewOptions(finalReview review.FinalReview, allowAutonomousReject bool) gitea.CreatePullReviewOptions {
	event := finalReview.Event
	if !allowAutonomousReject && (event == review.GiteaEventApproved || event == review.GiteaEventRequestChanges) {
		event = review.GiteaEventComment
	}
	if event == review.GiteaEventRequestChanges && !hasBlockingComment(finalReview.InlineComments) {
		event = review.GiteaEventComment
	}

	body := strings.TrimSpace(finalReview.ReviewBody())
	if commentsWithoutPosition := finalReview.CommentsWithoutPosition(); len(commentsWithoutPosition) > 0 {
		if body != "" {
			body += "\n\n"
		}
		body += "Comentarios sem linha especifica:\n"
		for _, comment := range commentsWithoutPosition {
			body += comment.SummaryLine() + "\n"
		}
		body = strings.TrimSpace(body)
	}

	options := gitea.CreatePullReviewOptions{
		Event: event,
		Body:  body,
	}
	for _, comment := range finalReview.InlineComments {
		if comment.Path == "" || comment.Body == "" {
			continue
		}
		if comment.NewPosition <= 0 {
			continue
		}
		options.Comments = append(options.Comments, gitea.CreatePullReviewComment{
			Body:        formatInlineReviewComment(comment),
			NewPosition: comment.NewPosition,
			Path:        comment.Path,
		})
	}

	return options
}

func formatInlineReviewComment(comment review.InlineComment) string {
	var builder strings.Builder
	severity := strings.TrimSpace(comment.Severity)
	if severity == "" {
		severity = "nao informada"
	}
	commentType := strings.TrimSpace(comment.Type)
	if commentType == "" {
		commentType = "semantica"
	}
	builder.WriteString("> severity: ")
	builder.WriteString(severity)
	builder.WriteString("\n> tipo: ")
	builder.WriteString(commentType)

	if reason := strings.TrimSpace(comment.DecisionReason); reason != "" {
		builder.WriteString("\n\n")
		builder.WriteString(reason)
	}
	if body := strings.TrimSpace(comment.Body); body != "" {
		builder.WriteString("\n\n")
		builder.WriteString(body)
	}

	return strings.TrimSpace(builder.String())
}

func hasBlockingComment(comments []review.InlineComment) bool {
	for _, comment := range comments {
		if strings.EqualFold(comment.Severity, "alta") || strings.EqualFold(comment.Severity, "media") {
			return true
		}
		lowerReason := strings.ToLower(comment.DecisionReason)
		if isExplicitlyNonBlockingReason(lowerReason) {
			continue
		}
		if strings.Contains(lowerReason, "bloque") || strings.Contains(lowerReason, "request_changes") {
			return true
		}
	}

	return false
}

func isExplicitlyNonBlockingReason(reason string) bool {
	reason = strings.ReplaceAll(reason, "\u00e3", "a")
	return strings.Contains(reason, "nao bloque") ||
		strings.Contains(reason, "nao-bloque") ||
		strings.Contains(reason, "non-block")
}

func formatFullDiffLog(rawDiff string, files []diff.ChangedFile) string {
	if len(files) == 0 {
		return rawDiff
	}

	var builder strings.Builder
	for index, file := range files {
		builder.WriteString(fmt.Sprintf("===== INICIO DIFF ARQUIVO %d/%d path=%s additions=%d deletions=%d =====\n", index+1, len(files), file.Path, file.Additions, file.Deletions))
		builder.WriteString(file.Patch)
		if !strings.HasSuffix(file.Patch, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("===== FIM DIFF ARQUIVO %d/%d path=%s =====\n", index+1, len(files), file.Path))
		if index < len(files)-1 {
			builder.WriteByte('\n')
		}
	}

	return builder.String()
}

func formatBlockResponseLog(block review.ReviewBlock, response string, duration time.Duration, totalElapsed time.Duration) string {
	return fmt.Sprintf(`===== METADADOS RESPOSTA OLLAMA =====
tipo=bloco
block=%d
total=%d
files=%s
response_chars=%d
response_duration=%s
total_desde_inicio_ollama=%s
===== RESPOSTA =====
%s`, block.Index, block.Total, strings.Join(block.Files, ","), len(response), duration, totalElapsed, response)
}

func formatBlockErrorLog(block review.ReviewBlock, response string, duration time.Duration, totalElapsed time.Duration, err error) string {
	return fmt.Sprintf(`===== METADADOS RESPOSTA OLLAMA =====
tipo=bloco
status=erro
block=%d
total=%d
files=%s
response_chars=%d
response_duration=%s
total_desde_inicio_ollama=%s
error=%v
===== RESPOSTA =====
%s`, block.Index, block.Total, strings.Join(block.Files, ","), len(response), duration, totalElapsed, err, response)
}

func formatFinalResponseLog(response string, duration time.Duration, totalElapsed time.Duration, partialReviews int, failedBlocks int) string {
	return fmt.Sprintf(`===== METADADOS RESPOSTA OLLAMA =====
tipo=final
partial_reviews=%d
failed_blocks=%d
response_chars=%d
response_duration=%s
total_desde_inicio_ollama=%s
===== RESPOSTA =====
%s`, partialReviews, failedBlocks, len(response), duration, totalElapsed, response)
}

func countReviewableFiles(blocks []review.ReviewBlock) int {
	total := 0
	for _, block := range blocks {
		total += len(block.Files)
	}

	return total
}

func reviewFileFilterFromConfig(filter promptconfig.ResolvedBlockFilter) review.ReviewFileFilter {
	return review.NormalizeReviewFileFilter(review.ReviewFileFilter{
		IncludeDocs:              filter.IncludeDocs,
		DocFilePatterns:          filter.DocFilePatterns,
		IgnoreFilePatterns:       filter.IgnoreFilePatterns,
		ForceIncludeFilePatterns: filter.ForceIncludeFilePatterns,
	})
}

func countIgnoredReviewFiles(files []diff.ChangedFile, filter review.ReviewFileFilter) int {
	total := 0
	for _, file := range files {
		if review.ShouldIgnoreReviewFileWithFilter(file.Path, filter) {
			total++
		}
	}

	return total
}

func countIgnoredDocs(files []diff.ChangedFile, filter review.ReviewFileFilter) int {
	if filter.IncludeDocs {
		return 0
	}

	total := 0
	for _, file := range files {
		if review.IsDocumentationFileWithFilter(file.Path, filter) && review.ShouldIgnoreReviewFileWithFilter(file.Path, filter) {
			total++
		}
	}

	return total
}
