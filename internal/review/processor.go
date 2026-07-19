package review

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gitea-agents/internal/integrations/gitea"
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

	definition, err := s.repo.Pipeline(ctx, id, job)
	if err != nil {
		return s.fail(ctx, id, recordStep, "preparacao", err)
	}
	policy, policyErr := s.repo.PolicyForProfile(ctx, definition.ProfileID)
	if policyErr != nil {
		policy = defaultPolicy(s.cfg.ReviewMaxBlockChars, s.cfg.ReviewMaxFilesPerBlock)
	}
	basePrompt, _ := s.repo.Prompt(ctx, definition.ProfileID)

	progress := func(stage, status, message string, metadata map[string]any, began time.Time, stageErr error) {
		finished := time.Now()
		errorText := ""
		if stageErr != nil {
			errorText = stageErr.Error()
		}
		_ = s.repo.AddStep(ctx, id, stage, status, message, metadata, began, &finished, finished.Sub(began).Milliseconds(), errorText)
	}
	engine := NewPipelineEngine(s.repo)
	_, err = engine.Execute(ctx, definition, PipelineExecutionInput{
		Job:        queueInput{ID: id, Owner: job.Owner, Repository: job.Repository, PullRequest: job.PullRequest},
		RawDiff:    rawDiff,
		BasePrompt: basePrompt,
		Policy:     policy,
		Provider: func(callCtx context.Context, modelID *int64) (providers.LLMProvider, error) {
			return s.providerFactory(callCtx, definition.ProfileID, modelID)
		},
		Progress: progress,
		Publish: func(publishCtx context.Context, result Result) (StageOutcome, error) {
			result.Metadata["response_chars"] = len(rawDiff)
			result.Metadata["total_duration_ms"] = time.Since(started).Milliseconds()
			if !policy.AllowAutonomousReject && result.FinalReview.GiteaEvent == "REQUEST_CHANGES" {
				result.FinalReview.GiteaEvent = "COMMENT"
				result.FinalReview.Status = "comentado"
			}

			resultStarted := time.Now()
			if saveErr := s.repo.SaveResult(publishCtx, id, result); saveErr != nil {
				return StageOutcome{}, saveErr
			}
			recordStep("agregando_resultado", "concluido", fmt.Sprintf("%d comentários", len(result.Comments)), resultStarted, nil)
			if status, statusErr := s.repo.Status(publishCtx, id); statusErr == nil && status == StatusCancelled {
				return StageOutcome{Status: "cancelled", ArtifactType: "publication_result", Artifact: map[string]string{"status": "cancelled"}}, nil
			}
			if job.Manual && !policy.PublishManualReviews {
				saved, pendingErr := s.repo.SavePending(publishCtx, id, job, result)
				if pendingErr != nil {
					return StageOutcome{}, pendingErr
				}
				if !saved {
					return StageOutcome{Status: "cancelled", ArtifactType: "publication_result", Artifact: map[string]string{"status": "cancelled"}}, nil
				}
				recordStep("pre-publicacao", "aguardando", "Aguardando autorização para publicar no Gitea", time.Now(), nil)
				return StageOutcome{Status: "waiting", ArtifactType: "publication_result", Artifact: map[string]string{"status": "waiting"}}, nil
			}

			if giteaClient.BaseURL != "" {
				reserved, reserveErr := s.repo.BeginPublication(publishCtx, id, result)
				if reserveErr != nil {
					return StageOutcome{}, reserveErr
				}
				if !reserved {
					if status, statusErr := s.repo.Status(publishCtx, id); statusErr == nil && status == StatusCancelled {
						return StageOutcome{Status: "cancelled", ArtifactType: "publication_result", Artifact: map[string]string{"status": "cancelled"}}, nil
					}
					publicationStatus, _ := s.repo.PublicationStatus(publishCtx, id)
					if publicationStatus == "publishing" || publicationStatus == "uncertain" {
						allowed, allowedErr := s.repo.PublicationReconciliationAllowed(publishCtx, id)
						if allowedErr != nil {
							return StageOutcome{}, allowedErr
						}
						if !allowed {
							return StageOutcome{}, fmt.Errorf("publicação %s ainda está dentro da janela de segurança", publicationStatus)
						}
						found, reconcileErr := giteaClient.HasPublishedReview(publishCtx, job.Owner, job.Repository, job.PullRequest, id)
						if reconcileErr != nil {
							return StageOutcome{}, fmt.Errorf("não foi possível reconciliar publicação %s: %w", publicationStatus, reconcileErr)
						}
						if found {
							if finishErr := s.repo.FinishPublication(publishCtx, id, nil); finishErr != nil {
								return StageOutcome{}, finishErr
							}
							publicationStatus = "published"
						} else {
							if resetErr := s.repo.ResetPublicationForRetry(publishCtx, id); resetErr != nil {
								return StageOutcome{}, resetErr
							}
							reserved, reserveErr = s.repo.BeginPublication(publishCtx, id, result)
							if reserveErr != nil || !reserved {
								return StageOutcome{}, fmt.Errorf("não foi possível reabrir publicação: %w", reserveErr)
							}
						}
					}
					if publicationStatus != "published" && !reserved {
						return StageOutcome{}, fmt.Errorf("publicação já reservada com status %s", publicationStatus)
					}
				}
				if reserved {
					if status, statusErr := s.repo.Status(publishCtx, id); statusErr == nil && status == StatusCancelled {
						return StageOutcome{Status: "cancelled", ArtifactType: "publication_result", Artifact: map[string]string{"status": "cancelled"}}, nil
					}
					publishStarted := time.Now()
					publishErr := giteaClient.Publish(publishCtx, job.Owner, job.Repository, job.PullRequest, result)
					var finishErr error
					if publishErr != nil && !gitea.IsDefinitiveHTTPRejection(publishErr) {
						finishErr = s.repo.MarkPublicationUncertain(publishCtx, id, publishErr)
					} else {
						finishErr = s.repo.FinishPublication(publishCtx, id, publishErr)
					}
					if publishErr == nil && finishErr != nil {
						publishErr = finishErr
					}
					if publishErr != nil {
						return StageOutcome{}, publishErr
					}
					recordStep("publicando_comentario", "concluido", "Resultado publicado no Gitea", publishStarted, nil)
				}
			}
			if statusErr := s.repo.SetStatus(publishCtx, id, StatusCompleted, ""); statusErr != nil {
				return StageOutcome{}, statusErr
			}
			return StageOutcome{ArtifactType: "publication_result", Artifact: map[string]string{"status": "published"}}, nil
		},
	})
	if err != nil {
		return s.fail(ctx, id, recordStep, "revisao", err)
	}
	return nil
}

func defaultPolicy(maxBlockChars, maxFilesPerBlock int) Policy {
	return Policy{
		MaxBlockChars: maxBlockChars, MaxFilesPerBlock: maxFilesPerBlock,
		PlannerEnabled: true, ConsolidatorEnabled: true, VerifierEnabled: true, FormatterEnabled: true,
		PlannerMaxTokens: 2000, GroupMaxTokens: 3500, ConsolidatorMaxTokens: 3500,
		VerifierMaxTokens: 2500, FormatterMaxTokens: 2500, MinimumConfidence: .75,
		MaxParallelGroups: 1, MediumSeverityEvent: "REQUEST_CHANGES", PartialEvent: "COMMENT", ContractMaxAttempts: 5,
	}
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
