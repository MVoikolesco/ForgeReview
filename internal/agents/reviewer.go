package agents

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"gitea-agents/internal/diff"
	"gitea-agents/internal/gitea"
	"gitea-agents/internal/ollama"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"
	"gitea-agents/internal/review/promptconfig"
)

const ReviewerAgentName = "reviewer"
const defaultDiffLogDir = "/tmp/gitea-ai-reviewer-diffs"

type GiteaClient interface {
	GetPullRequestDiff(ctx context.Context, owner string, repo string, prNumber int) (string, error)
	CreatePullRequestReview(ctx context.Context, owner string, repo string, prNumber int, options gitea.CreatePullReviewOptions) (gitea.PullReview, error)
}

type OllamaClient interface {
	Chat(ctx context.Context, prompt string) (string, error)
	Model() string
}

type ollamaMetadataClient interface {
	ChatWithMetadata(ctx context.Context, prompt string) (ollama.ChatResult, error)
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
}

type ReviewerAgent struct {
	logger       *log.Logger
	giteaClient  GiteaClient
	ollamaClient OllamaClient
	options      ReviewerOptions
}

func NewReviewerAgent(logger *log.Logger, giteaClient GiteaClient, ollamaClient OllamaClient, options ReviewerOptions) *ReviewerAgent {
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
	rawDiff, err := a.giteaClient.GetPullRequestDiff(ctx, job.Owner, job.Repo, job.PRNumber)
	if err != nil {
		return err
	}

	a.logger.Printf("diff obtido owner=%s repo=%s pr=%d size=%d", job.Owner, job.Repo, job.PRNumber, len(rawDiff))

	runLog, err := newReviewRunLog(a.options.DiffLogDir, job)
	if err != nil {
		return err
	}
	a.logger.Printf("logs fisicos do review dir=%s", runLog.Dir())

	files := diff.Parse(rawDiff)
	a.logger.Printf("diff parseado owner=%s repo=%s pr=%d files=%d", job.Owner, job.Repo, job.PRNumber, len(files))
	if err := runLog.AppendProcess("diff obtido owner=%s repo=%s pr=%d size=%d files=%d", job.Owner, job.Repo, job.PRNumber, len(rawDiff), len(files)); err != nil {
		return err
	}

	for _, file := range files {
		a.logger.Printf(
			"arquivo alterado path=%s additions=%d deletions=%d patch_size=%d",
			file.Path,
			file.Additions,
			file.Deletions,
			len(file.Patch),
		)
	}

	if _, err := runLog.Write("01-diff-completo.log", formatFullDiffLog(rawDiff, files)); err != nil {
		return err
	}

	promptResolver := promptconfig.NewPromptResolver(a.options.ReviewPromptConfigPath)
	if _, err := promptResolver.Load(ctx); err != nil {
		return err
	}
	a.logger.Printf("prompt config carregado path=%s", a.options.ReviewPromptConfigPath)
	if err := runLog.AppendProcess("prompt config carregado path=%s", a.options.ReviewPromptConfigPath); err != nil {
		return err
	}

	resolvedBlockFilter, err := promptResolver.ResolveBlockFilter(ctx, job.Owner, job.Repo)
	if err != nil {
		return err
	}
	blockFilter := reviewFileFilterFromConfig(resolvedBlockFilter)
	includeDocs := blockFilter.IncludeDocs
	ignoredDocs := countIgnoredDocs(files, blockFilter)
	if ignoredDocs > 0 {
		a.logger.Printf("docs ignorados owner=%s repo=%s pr=%d files=%d include_docs=%t", job.Owner, job.Repo, job.PRNumber, ignoredDocs, includeDocs)
		if err := runLog.AppendProcess("docs ignorados files=%d include_docs=%t", ignoredDocs, includeDocs); err != nil {
			return err
		}
	}
	ignoredFiles := countIgnoredReviewFiles(files, blockFilter)
	if ignoredFiles > 0 {
		a.logger.Printf("arquivos ignorados por filtro owner=%s repo=%s pr=%d files=%d include_docs=%t ignore_patterns=%d force_include_patterns=%d", job.Owner, job.Repo, job.PRNumber, ignoredFiles, includeDocs, len(blockFilter.IgnoreFilePatterns), len(blockFilter.ForceIncludeFilePatterns))
		if err := runLog.AppendProcess("arquivos ignorados por filtro files=%d include_docs=%t ignore_patterns=%d force_include_patterns=%d", ignoredFiles, includeDocs, len(blockFilter.IgnoreFilePatterns), len(blockFilter.ForceIncludeFilePatterns)); err != nil {
			return err
		}
	}

	basePartialPrompt, err := promptResolver.ResolvePartialPrompt(ctx, job.Owner, job.Repo, nil)
	if err != nil {
		return err
	}
	baseFinalPrompt, err := promptResolver.ResolveFinalPrompt(ctx, job.Owner, job.Repo)
	if err != nil {
		return err
	}

	if _, err := runLog.Write("02-prompt-base.log", review.BasePromptsLog(basePartialPrompt.Content, baseFinalPrompt.Content)); err != nil {
		return err
	}

	blocks := review.BuildReviewBlocksWithFilter(files, a.options.MaxBlockChars, a.options.MaxFilesPerBlock, blockFilter)
	reviewableFiles := countReviewableFiles(blocks)
	a.logger.Printf("arquivos revisaveis owner=%s repo=%s pr=%d files=%d", job.Owner, job.Repo, job.PRNumber, reviewableFiles)
	a.logger.Printf("blocos de review gerados owner=%s repo=%s pr=%d blocks=%d max_chars=%d max_files_per_block=%d", job.Owner, job.Repo, job.PRNumber, len(blocks), a.options.MaxBlockChars, a.options.MaxFilesPerBlock)
	if a.options.ReviewConcurrency > 1 {
		a.logger.Printf("review_concurrency configurado=%d processamento=sequencial", a.options.ReviewConcurrency)
	}
	if err := runLog.AppendProcess("blocos gerados reviewable_files=%d blocks=%d max_chars=%d max_files_per_block=%d", reviewableFiles, len(blocks), a.options.MaxBlockChars, a.options.MaxFilesPerBlock); err != nil {
		return err
	}

	if len(blocks) == 0 {
		finalReview := "Nenhum arquivo revisavel encontrado no diff."
		if _, err := runLog.Write("final-resposta.log", finalReview); err != nil {
			return err
		}
		if job.Manual && !a.options.PublishManualReviews {
			if _, err := runLog.Write("final-review.md", finalReview); err != nil {
				return err
			}
		}
		a.logger.Printf("review final gerado chars=%d", len(finalReview))
		a.logger.Printf("review final:\n%s", finalReview)
		return nil
	}

	partialReviews := make([]review.PartialReview, 0, len(blocks))
	failedBlocks := 0
	successfulBlocks := 0
	var blockErrors []error
	var ollamaStartedAt time.Time
	var totalUsage ollama.ChatResult
	for _, block := range blocks {
		if err := ctx.Err(); err != nil {
			a.logger.Printf("review cancelado antes de enviar bloco para ollama block=%d total=%d err=%v", block.Index, block.Total, err)
			if appendErr := runLog.AppendProcess("review cancelado antes de enviar bloco para ollama block=%d total=%d err=%v", block.Index, block.Total, err); appendErr != nil {
				return appendErr
			}
			return fmt.Errorf("review cancelado antes do bloco %d/%d: %w", block.Index, block.Total, err)
		}

		resolvedPrompt, err := promptResolver.ResolvePartialPrompt(ctx, job.Owner, job.Repo, block.Files)
		if err != nil {
			return err
		}
		a.logger.Printf("prompt parcial resolvido owner=%s repo=%s stacks=%s files=%d prompt_chars=%d", job.Owner, job.Repo, strings.Join(resolvedPrompt.Stacks, ","), len(block.Files), len(resolvedPrompt.Content))
		if err := runLog.AppendProcess("prompt parcial resolvido block=%d stacks=%s files=%d prompt_chars=%d", block.Index, strings.Join(resolvedPrompt.Stacks, ","), len(block.Files), len(resolvedPrompt.Content)); err != nil {
			return err
		}
		prompt := review.BuildPartialReviewPromptWithMemory(block, resolvedPrompt.Content, partialReviews)
		if _, err := runLog.Write(fmt.Sprintf("block-%03d-prompt.log", block.Index), prompt); err != nil {
			return err
		}

		timeoutLabel := fmt.Sprintf("%ds", a.options.OllamaTimeoutSeconds)
		a.logger.Printf("enviando bloco para ollama block=%d total=%d files=%d chars=%d model=%s timeout=%s", block.Index, block.Total, len(block.Files), len(block.Content), a.ollamaClient.Model(), timeoutLabel)
		if err := runLog.AppendProcess("enviando bloco para ollama block=%d total=%d files=%d chars=%d model=%s timeout=%s prompt_chars=%d", block.Index, block.Total, len(block.Files), len(block.Content), a.ollamaClient.Model(), timeoutLabel, len(prompt)); err != nil {
			return err
		}

		startedAt := time.Now()
		if ollamaStartedAt.IsZero() {
			ollamaStartedAt = startedAt
			if err := runLog.AppendProcess("inicio chamadas ollama started_at=%s", ollamaStartedAt.Format(time.RFC3339)); err != nil {
				return err
			}
		}
		result, usage, err := a.chatWithMetadata(ctx, prompt)
		duration := time.Since(startedAt).Round(time.Millisecond)
		totalElapsed := time.Since(ollamaStartedAt).Round(time.Millisecond)
		if err != nil {
			failedBlocks++
			blockErr := fmt.Errorf("bloco %d/%d: %w", block.Index, block.Total, err)
			blockErrors = append(blockErrors, blockErr)
			failureContent := fmt.Sprintf("Falha ao revisar este bloco: %v", err)

			if _, writeErr := runLog.Write(fmt.Sprintf("block-%03d-erro.log", block.Index), formatBlockErrorLog(block, failureContent, duration, totalElapsed, err)); writeErr != nil {
				return writeErr
			}

			a.logger.Printf("erro ao revisar bloco com ollama block=%d total=%d files=%d chars=%d prompt_chars=%d timeout=%s duration=%s total_elapsed=%s err=%v", block.Index, block.Total, len(block.Files), len(block.Content), len(prompt), timeoutLabel, duration, totalElapsed, err)
			if appendErr := runLog.AppendProcess("erro ao revisar bloco com ollama block=%d total=%d files=%d chars=%d prompt_chars=%d timeout=%s duration=%s total_elapsed=%s err=%v", block.Index, block.Total, len(block.Files), len(block.Content), len(prompt), timeoutLabel, duration, totalElapsed, err); appendErr != nil {
				return appendErr
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				a.logger.Printf("review cancelado durante bloco block=%d total=%d err=%v", block.Index, block.Total, ctxErr)
				if appendErr := runLog.AppendProcess("review cancelado durante bloco block=%d total=%d err=%v", block.Index, block.Total, ctxErr); appendErr != nil {
					return appendErr
				}
				return fmt.Errorf("review cancelado durante bloco %d/%d: %w", block.Index, block.Total, ctxErr)
			}

			partialReviews = append(partialReviews, review.PartialReview{
				Block:   block,
				Content: failureContent,
				Failed:  true,
				Error:   err.Error(),
			})
			continue
		}
		totalUsage.PromptTokens += usage.PromptTokens
		totalUsage.CompletionTokens += usage.CompletionTokens

		if _, err := runLog.Write(fmt.Sprintf("block-%03d-resposta.log", block.Index), formatBlockResponseLog(block, result, duration, totalElapsed)); err != nil {
			return err
		}

		successfulBlocks++
		a.logger.Printf("resposta ollama recebida block=%d chars=%d duration=%s total_elapsed=%s", block.Index, len(result), duration, totalElapsed)
		if err := runLog.AppendProcess("resposta ollama recebida block=%d chars=%d duration=%s total_elapsed=%s", block.Index, len(result), duration, totalElapsed); err != nil {
			return err
		}

		partialReviews = append(partialReviews, review.PartialReview{
			Block:   block,
			Content: result,
		})
	}

	if successfulBlocks == 0 {
		return fmt.Errorf("todos os %d blocos falharam ao chamar o ollama: %w", len(blocks), errors.Join(blockErrors...))
	}

	a.logger.Printf("gerando review final partial_reviews=%d failed_blocks=%d", len(partialReviews), failedBlocks)
	if err := runLog.AppendProcess("gerando review final partial_reviews=%d failed_blocks=%d", len(partialReviews), failedBlocks); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		a.logger.Printf("review cancelado antes da consolidacao final err=%v", err)
		if appendErr := runLog.AppendProcess("review cancelado antes da consolidacao final err=%v", err); appendErr != nil {
			return appendErr
		}
		return fmt.Errorf("review cancelado antes da consolidacao final: %w", err)
	}
	resolvedFinalPrompt, err := promptResolver.ResolveFinalPrompt(ctx, job.Owner, job.Repo)
	if err != nil {
		return err
	}
	a.logger.Printf("prompt final resolvido owner=%s repo=%s prompt_chars=%d", job.Owner, job.Repo, len(resolvedFinalPrompt.Content))
	if err := runLog.AppendProcess("prompt final resolvido prompt_chars=%d", len(resolvedFinalPrompt.Content)); err != nil {
		return err
	}
	finalPrompt := review.BuildFinalReviewPrompt(partialReviews, resolvedFinalPrompt.Content)
	if _, err := runLog.Write("final-prompt.log", finalPrompt); err != nil {
		return err
	}

	finalStartedAt := time.Now()
	if ollamaStartedAt.IsZero() {
		ollamaStartedAt = finalStartedAt
	}
	finalResponse, finalReview, finalUsage, err := a.requestValidatedFinalReview(ctx, finalPrompt, runLog)
	finalDuration := time.Since(finalStartedAt).Round(time.Millisecond)
	finalTotalElapsed := time.Since(ollamaStartedAt).Round(time.Millisecond)
	if err != nil {
		return err
	}
	totalUsage.PromptTokens += finalUsage.PromptTokens
	totalUsage.CompletionTokens += finalUsage.CompletionTokens
	finalReview.Metadata = review.ReviewMetadata{
		Model:            a.ollamaClient.Model(),
		Elapsed:          finalTotalElapsed.String(),
		PromptTokens:     totalUsage.PromptTokens,
		CompletionTokens: totalUsage.CompletionTokens,
	}

	if _, err := runLog.Write("final-resposta.log", formatFinalResponseLog(finalResponse, finalDuration, finalTotalElapsed, len(partialReviews), failedBlocks)); err != nil {
		return err
	}

	a.logger.Printf("review final gerado chars=%d duration=%s total_elapsed=%s", len(finalResponse), finalDuration, finalTotalElapsed)
	if err := runLog.AppendProcess("review final gerado chars=%d duration=%s total_elapsed=%s failed_blocks=%d", len(finalResponse), finalDuration, finalTotalElapsed, failedBlocks); err != nil {
		return err
	}
	a.logger.Printf("review final:\n%s", finalResponse)

	parsedFinalReview := finalReview
	parsedFinalReview = review.ResolveFinalReviewCommentPositions(parsedFinalReview, files)
	if job.Manual && !a.options.PublishManualReviews {
		if _, err := runLog.Write("final-review.md", formatManualReviewMarkdown(parsedFinalReview)); err != nil {
			return err
		}
		a.logger.Printf("review manual salvo sem publicar no gitea dir=%s", runLog.Dir())
		return nil
	}
	createReviewOptions := buildCreatePullReviewOptions(parsedFinalReview, a.options.AllowAutonomousReject)
	createdReview, err := a.giteaClient.CreatePullRequestReview(ctx, job.Owner, job.Repo, job.PRNumber, createReviewOptions)
	if err != nil {
		if appendErr := runLog.AppendProcess("erro ao publicar review no gitea event=%s comments=%d err=%v", createReviewOptions.Event, len(createReviewOptions.Comments), err); appendErr != nil {
			return appendErr
		}
		return fmt.Errorf("erro ao publicar review no gitea: %w", err)
	}
	a.logger.Printf("review publicado no gitea owner=%s repo=%s pr=%d review_id=%d state=%s event=%s comments=%d", job.Owner, job.Repo, job.PRNumber, createdReview.ID, createdReview.State, createReviewOptions.Event, len(createReviewOptions.Comments))
	if err := runLog.AppendProcess("review publicado no gitea review_id=%d state=%s event=%s comments=%d", createdReview.ID, createdReview.State, createReviewOptions.Event, len(createReviewOptions.Comments)); err != nil {
		return err
	}

	return nil
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

func (a *ReviewerAgent) requestValidatedFinalReview(ctx context.Context, prompt string, runLog *reviewRunLog) (string, review.FinalReview, ollama.ChatResult, error) {
	currentPrompt := prompt
	var validationErrors []string
	for attempt := 1; attempt <= a.options.ReviewFinalRetries; attempt++ {
		response, usage, err := a.chatWithMetadata(ctx, currentPrompt)
		if err != nil {
			return "", review.FinalReview{}, ollama.ChatResult{}, fmt.Errorf("erro ao consolidar review com ollama: %w", err)
		}
		parsed, validationErr := review.ValidateFinalReviewResponse(response)
		if validationErr == nil {
			return response, parsed, usage, nil
		}

		validationErrors = append(validationErrors, validationErr.Error())
		if appendErr := runLog.AppendProcess("resposta final invalida tentativa=%d de %d erros=%s", attempt, a.options.ReviewFinalRetries, validationErr.Error()); appendErr != nil {
			return "", review.FinalReview{}, ollama.ChatResult{}, appendErr
		}
		if attempt == a.options.ReviewFinalRetries {
			break
		}
		currentPrompt = prompt + "\n\nCORRECAO OBRIGATORIA DA TENTATIVA ANTERIOR:\nA ultima resposta nao veio no padrao obrigatorio. Refaça a resposta completa e retorne exclusivamente um objeto JSON valido, sem Markdown e sem qualquer texto antes ou depois. Preserve exatamente as propriedades comments e final_review, incluindo todos os campos obrigatorios. Utilize somente os valores de severidade, status e evento definidos pelo projeto. Erros detectados: " + validationErr.Error()
	}

	return "", review.FinalReview{}, ollama.ChatResult{}, fmt.Errorf("review final invalido apos %d tentativas: %s", a.options.ReviewFinalRetries, strings.Join(validationErrors, " | "))
}

func (a *ReviewerAgent) chatWithMetadata(ctx context.Context, prompt string) (string, ollama.ChatResult, error) {
	if client, ok := a.ollamaClient.(ollamaMetadataClient); ok {
		result, err := client.ChatWithMetadata(ctx, prompt)
		return result.Content, result, err
	}
	content, err := a.ollamaClient.Chat(ctx, prompt)
	return content, ollama.ChatResult{Content: content}, err
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
			Body:        comment.Body,
			NewPosition: comment.NewPosition,
			Path:        comment.Path,
		})
	}

	return options
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
