package review

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gitea-agents/internal/providers"
	"gitea-agents/internal/queue"
)

type stepRecorder func(name, status, message string, startedAt time.Time, stepErr error)

// Process executes one queued review from diff retrieval through validation,
// persistence, optional manual approval, and Gitea publication.
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
	recordStep := s.stepRecorder(ctx, id)
	giteaClient, err := s.giteaFactory(ctx, job)
	if err != nil {
		return s.fail(ctx, id, recordStep, "buscando_diff", err)
	}

	stepStarted := time.Now()
	rawDiff, err := giteaClient.PullRequestDiff(ctx, job.Owner, job.Repository, job.PullRequest)
	if err != nil {
		return s.fail(ctx, id, recordStep, "buscando_diff", err)
	}
	recordStep("buscando_diff", "concluido", "Diff obtido", stepStarted, nil)

	policy, policyErr := s.repo.Policy(ctx, job)
	if policyErr != nil {
		policy.MaxBlockChars = s.cfg.ReviewMaxBlockChars
		policy.MaxFilesPerBlock = s.cfg.ReviewMaxFilesPerBlock
	}
	blocks := splitDiff(rawDiff, policy.MaxBlockChars, policy.MaxFilesPerBlock)
	if len(blocks) == 0 {
		blocks = []string{""}
	}

	provider, err := s.providerFactory(ctx)
	if err != nil {
		return s.fail(ctx, id, recordStep, "enviando_para_ia", err)
	}

	result := Result{
		ReviewID: id,
		Provider: provider.Name(),
		Agent:    "reviewer",
		Comments: []Comment{},
		Metadata: map[string]any{
			"partial_reviews":   len(blocks),
			"failed_blocks":     0,
			"response_chars":    0,
			"total_duration_ms": 0,
		},
	}

	for index, block := range blocks {
		if status, statusErr := s.repo.Status(ctx, id); statusErr == nil && status == StatusCancelled {
			return nil
		}

		stepStarted = time.Now()
		partial, callErr := provider.Review(ctx, providers.Input{
			Owner:       job.Owner,
			Repository:  job.Repository,
			PullRequest: job.PullRequest,
			Diff:        block,
			Prompt:      s.promptFor(ctx, job),
		})
		if callErr != nil {
			incrementFailedBlocks(&result)
			recordStep(
				"enviando_para_ia",
				"falhou",
				fmt.Sprintf("Bloco %d/%d", index+1, len(blocks)),
				stepStarted,
				callErr,
			)
			continue
		}
		if validationErr := validateResult(partial); validationErr != nil {
			incrementFailedBlocks(&result)
			recordStep(
				"recebendo_resposta_parcial",
				"falhou",
				fmt.Sprintf("Bloco %d/%d", index+1, len(blocks)),
				stepStarted,
				validationErr,
			)
			continue
		}

		recordStep(
			"recebendo_resposta_parcial",
			"concluido",
			fmt.Sprintf("Bloco %d/%d", index+1, len(blocks)),
			stepStarted,
			nil,
		)
		result.Comments = mergeComments(result.Comments, partial.Comments)
		if result.Summary == "" {
			result.Summary = partial.Summary
		}
		result.FinalReview = partial.FinalReview
	}

	if result.Metadata["failed_blocks"].(int) == len(blocks) {
		return s.fail(
			ctx,
			id,
			recordStep,
			"recebendo_resposta_parcial",
			errors.New("nenhum bloco retornou uma review válida"),
		)
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

	stepStarted = time.Now()
	if err := s.repo.SaveResult(ctx, id, result); err != nil {
		return s.fail(ctx, id, recordStep, "agregando_resultado", err)
	}
	recordStep(
		"agregando_resultado",
		"concluido",
		fmt.Sprintf("%d comentários", len(result.Comments)),
		stepStarted,
		nil,
	)

	if status, statusErr := s.repo.Status(ctx, id); statusErr == nil && status == StatusCancelled {
		return nil
	}
	if job.Manual && !policy.PublishManualReviews {
		if err := s.repo.SavePending(ctx, id, job, result); err != nil {
			return s.fail(ctx, id, recordStep, "pre-publicacao", err)
		}
		recordStep(
			"pre-publicacao",
			"aguardando",
			"Aguardando autorização para publicar no Gitea",
			time.Now(),
			nil,
		)
		_ = s.repo.SetStatus(ctx, id, StatusAwaitingApproval, "")
		return nil
	}

	if giteaClient.BaseURL != "" {
		stepStarted = time.Now()
		if err := giteaClient.Publish(ctx, job.Owner, job.Repository, job.PullRequest, result); err != nil {
			return s.fail(ctx, id, recordStep, "publicando_comentario", err)
		}
		recordStep(
			"publicando_comentario",
			"concluido",
			"Resultado publicado no Gitea",
			stepStarted,
			nil,
		)
	}

	_ = s.repo.SetStatus(ctx, id, StatusCompleted, "")
	return nil
}

// stepRecorder returns a callback that appends completed or failed steps for a
// review ID.
func (s *Service) stepRecorder(ctx context.Context, id string) stepRecorder {
	return func(name, status, message string, startedAt time.Time, stepErr error) {
		finished := time.Now()
		errorText := ""
		if stepErr != nil {
			errorText = stepErr.Error()
		}
		_ = s.repo.AddStep(
			ctx,
			id,
			name,
			status,
			message,
			nil,
			startedAt,
			&finished,
			finished.Sub(startedAt).Milliseconds(),
			errorText,
		)
	}
}

// fail records a failed step, marks the review failed, and returns err.
func (s *Service) fail(
	ctx context.Context,
	id string,
	recordStep stepRecorder,
	name string,
	err error,
) error {
	recordStep(name, "falhou", "A etapa falhou", time.Now(), err)
	_ = s.repo.SetStatus(ctx, id, StatusFailed, err.Error())
	return err
}

// incrementFailedBlocks updates the failed block count in result metadata.
func incrementFailedBlocks(result *Result) {
	result.Metadata["failed_blocks"] = result.Metadata["failed_blocks"].(int) + 1
}
