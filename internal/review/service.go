package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"gitea-agents/internal/config"
	"gitea-agents/internal/integrations/gitea"
	"gitea-agents/internal/providers"
	"gitea-agents/internal/queue"
	"strings"
	"time"
)

type Service struct {
	cfg             config.Config
	repo            *Repository
	publisher       queue.Publisher
	providerFactory func(context.Context) (providers.LLMProvider, error)
	giteaFactory    func(context.Context, queue.ReviewJob) (*gitea.Client, error)
	promptLoader    func(context.Context) string
}

func NewService(cfg config.Config, repo *Repository, publisher queue.Publisher) *Service {
	return &Service{cfg: cfg, repo: repo, publisher: publisher, providerFactory: func(context.Context) (providers.LLMProvider, error) {
		return nil, errors.New("AI provider is not configured")
	}, giteaFactory: func(context.Context, queue.ReviewJob) (*gitea.Client, error) {
		return gitea.New(cfg.GiteaURL, cfg.GiteaToken), nil
	}}
}
func (s *Service) SetFactories(provider func(context.Context) (providers.LLMProvider, error), giteaClient func(context.Context, queue.ReviewJob) (*gitea.Client, error)) {
	s.providerFactory = provider
	s.giteaFactory = giteaClient
}
func (s *Service) SetPromptLoader(loader func(context.Context) string) { s.promptLoader = loader }
func (s *Service) Enqueue(ctx context.Context, job queue.ReviewJob, source string) (Review, error) {
	if job.ReviewID == "" {
		job.ReviewID = NewID()
	}
	if err := s.repo.Create(ctx, job.ReviewID, job, source); err != nil {
		return Review{}, err
	}
	if err := s.repo.SetStatus(ctx, job.ReviewID, StatusQueued, ""); err != nil {
		return Review{}, err
	}
	_ = s.repo.AddStep(ctx, job.ReviewID, "enfileirado", "concluido", "Review enfileirado", nil, time.Now().UTC(), nil, 0, "")
	if err := s.publisher.Publish(ctx, job); err != nil {
		_ = s.repo.SetStatus(ctx, job.ReviewID, StatusFailed, err.Error())
		return Review{}, err
	}
	return s.repo.Get(ctx, job.ReviewID)
}
func (s *Service) EnqueueExisting(ctx context.Context, job queue.ReviewJob) error {
	if job.ReviewID == "" {
		return errors.New("review id is required")
	}
	if err := s.repo.SetStatus(ctx, job.ReviewID, StatusQueued, ""); err != nil {
		return err
	}
	return s.publisher.Publish(ctx, job)
}
func NewID() string { return fmt.Sprintf("rev-%d", time.Now().UnixNano()) }
func (s *Service) Process(ctx context.Context, job queue.ReviewJob) error {
	id := job.ReviewID
	if id == "" {
		id = NewID()
		job.ReviewID = id
	}
	if status, statusErr := s.repo.Status(ctx, id); errors.Is(statusErr, sql.ErrNoRows) {
		if err := s.repo.Create(ctx, id, job, "queue"); err != nil {
			return err
		}
	} else if statusErr != nil {
		return statusErr
	} else if status == StatusCancelled {
		return nil
	}
	_ = s.repo.SetStatus(ctx, id, StatusProcessing, "")
	started := time.Now()
	step := func(name, status, message string, startedAt time.Time, stepErr error) {
		finished := time.Now()
		errText := ""
		if stepErr != nil {
			errText = stepErr.Error()
		}
		_ = s.repo.AddStep(ctx, id, name, status, message, nil, startedAt, &finished, finished.Sub(startedAt).Milliseconds(), errText)
	}
	g, err := s.giteaFactory(ctx, job)
	if err != nil {
		return s.fail(ctx, id, step, "buscando_diff", err)
	}
	st := time.Now()
	rawDiff, err := g.PullRequestDiff(ctx, job.Owner, job.Repository, job.PullRequest)
	if err != nil {
		return s.fail(ctx, id, step, "buscando_diff", err)
	}
	step("buscando_diff", "concluido", "Diff obtido", st, nil)
	policy, policyErr := s.repo.Policy(ctx, job)
	if policyErr != nil {
		policy.MaxBlockChars = s.cfg.ReviewMaxBlockChars
		policy.MaxFilesPerBlock = s.cfg.ReviewMaxFilesPerBlock
	}
	blocks := splitDiff(rawDiff, policy.MaxBlockChars, policy.MaxFilesPerBlock)
	if len(blocks) == 0 {
		blocks = []string{""}
	}
	p, err := s.providerFactory(ctx)
	if err != nil {
		return s.fail(ctx, id, step, "enviando_para_ia", err)
	}
	result := Result{ReviewID: id, Provider: p.Name(), Agent: "reviewer", Comments: []Comment{}, Metadata: map[string]any{"partial_reviews": len(blocks), "failed_blocks": 0, "response_chars": 0, "total_duration_ms": 0}}
	for i, block := range blocks {
		if status, statusErr := s.repo.Status(ctx, id); statusErr == nil && status == StatusCancelled {
			return nil
		}
		st = time.Now()
		prompt := s.promptFor(ctx, job)
		partial, callErr := p.Review(ctx, providers.Input{Owner: job.Owner, Repository: job.Repository, PullRequest: job.PullRequest, Diff: block, Prompt: prompt})
		if callErr != nil {
			result.Metadata["failed_blocks"] = result.Metadata["failed_blocks"].(int) + 1
			step("enviando_para_ia", "falhou", fmt.Sprintf("Bloco %d/%d", i+1, len(blocks)), st, callErr)
			continue
		}
		if validationErr := validateResult(partial); validationErr != nil {
			result.Metadata["failed_blocks"] = result.Metadata["failed_blocks"].(int) + 1
			step("recebendo_resposta_parcial", "falhou", fmt.Sprintf("Bloco %d/%d", i+1, len(blocks)), st, validationErr)
			continue
		}
		step("recebendo_resposta_parcial", "concluido", fmt.Sprintf("Bloco %d/%d", i+1, len(blocks)), st, nil)
		result.Comments = mergeComments(result.Comments, partial.Comments)
		if result.Summary == "" {
			result.Summary = partial.Summary
		}
		result.FinalReview = partial.FinalReview
	}
	if result.Metadata["failed_blocks"].(int) == len(blocks) {
		return s.fail(ctx, id, step, "recebendo_resposta_parcial", errors.New("nenhum bloco retornou uma review válida"))
	}
	result.Metadata["response_chars"] = len(rawDiff)
	result.Metadata["total_duration_ms"] = time.Since(started).Milliseconds()
	if result.FinalReview.GiteaEvent == "" {
		result.FinalReview.GiteaEvent = "COMMENT"
	}
	if result.FinalReview.Status == "" {
		result.FinalReview.Status = "comentado"
	}
	if !policy.AllowAutonomousReject && result.FinalReview.GiteaEvent == "REQUEST_CHANGES" {
		result.FinalReview.GiteaEvent = "COMMENT"
	}
	st = time.Now()
	if err := s.repo.SaveResult(ctx, id, result); err != nil {
		return s.fail(ctx, id, step, "agregando_resultado", err)
	}
	step("agregando_resultado", "concluido", fmt.Sprintf("%d comentários", len(result.Comments)), st, nil)
	if status, statusErr := s.repo.Status(ctx, id); statusErr == nil && status == StatusCancelled {
		return nil
	}
	if job.Manual && !policy.PublishManualReviews {
		if err := s.repo.SavePending(ctx, id, job, result); err != nil {
			return s.fail(ctx, id, step, "pre-publicacao", err)
		}
		step("pre-publicacao", "aguardando", "Aguardando autorização para publicar no Gitea", time.Now(), nil)
		_ = s.repo.SetStatus(ctx, id, StatusAwaitingApproval, "")
		return nil
	} else if g.BaseURL != "" {
		st = time.Now()
		if err := g.Publish(ctx, job.Owner, job.Repository, job.PullRequest, result); err != nil {
			return s.fail(ctx, id, step, "publicando_comentario", err)
		}
		step("publicando_comentario", "concluido", "Resultado publicado no Gitea", st, nil)
	}
	_ = s.repo.SetStatus(ctx, id, StatusCompleted, "")
	return nil
}

func validateResult(result Result) error {
	for _, comment := range result.Comments {
		if strings.TrimSpace(comment.File) == "" || comment.Line <= 0 || strings.TrimSpace(comment.Comment) == "" || strings.TrimSpace(comment.DecisionReason) == "" {
			return errors.New("comentário de review inválido")
		}
		if !containsValue([]string{"critica", "alta", "media", "baixa"}, comment.Severity) {
			return fmt.Errorf("severidade inválida: %s", comment.Severity)
		}
	}
	if strings.TrimSpace(result.FinalReview.Summary) == "" {
		return errors.New("resumo final vazio")
	}
	if !containsValue([]string{"APPROVE", "COMMENT", "REQUEST_CHANGES"}, result.FinalReview.GiteaEvent) {
		return fmt.Errorf("evento Gitea inválido: %s", result.FinalReview.GiteaEvent)
	}
	return nil
}

func containsValue(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func (s *Service) promptFor(ctx context.Context, job queue.ReviewJob) string {
	if s.promptLoader != nil {
		if value := strings.TrimSpace(s.promptLoader(ctx)); value != "" {
			return value
		}
	}
	return reviewPrompt(job)
}
func (s *Service) fail(ctx context.Context, id string, step func(string, string, string, time.Time, error), name string, err error) error {
	step(name, "falhou", "A etapa falhou", time.Now(), err)
	_ = s.repo.SetStatus(ctx, id, StatusFailed, err.Error())
	return err
}
func reviewPrompt(job queue.ReviewJob) string {
	return fmt.Sprintf("Você é um revisor de código. Analise apenas o diff. Responda somente JSON no formato {\"comments\":[{\"file\":\"path\",\"line\":1,\"severity\":\"alta\",\"decision_reason\":\"...\",\"comment\":\"...\"}],\"summary\":\"...\",\"final_review\":{\"gitea_event\":\"COMMENT\",\"status\":\"comentado\",\"summary\":\"...\",\"observations\":\"\"}}. PR %d de %s/%s.", job.PullRequest, job.Owner, job.Repository)
}
func splitDiff(raw string, maxChars, maxFiles int) []string {
	if maxChars <= 0 {
		maxChars = 2000
	}
	if maxFiles <= 0 {
		maxFiles = 2
	}
	files := strings.Split(raw, "diff --git ")
	blocks := []string{}
	current := ""
	count := 0
	for _, file := range files {
		if file == "" {
			continue
		}
		piece := "diff --git " + file
		if len(piece) > maxChars {
			piece = piece[:maxChars]
		}
		if current != "" && (len(current)+len(piece) > maxChars || count >= maxFiles) {
			blocks = append(blocks, current)
			current = ""
			count = 0
		}
		current += piece
		count++
	}
	if current != "" {
		blocks = append(blocks, current)
	}
	return blocks
}
func mergeComments(existing, incoming []Comment) []Comment {
	seen := map[string]bool{}
	for _, c := range existing {
		seen[c.File+fmt.Sprint(c.Line)+c.Comment] = true
	}
	for _, c := range incoming {
		key := c.File + fmt.Sprint(c.Line) + c.Comment
		if c.File != "" && !seen[key] {
			existing = append(existing, c)
			seen[key] = true
		}
	}
	return existing
}

var _ = json.Valid
