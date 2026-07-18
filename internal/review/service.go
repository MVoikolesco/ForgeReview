package review

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitea-agents/internal/config"
	"gitea-agents/internal/integrations/gitea"
	"gitea-agents/internal/providers"
	"gitea-agents/internal/queue"
)

type providerFactory func(context.Context) (providers.LLMProvider, error)
type giteaFactory func(context.Context, queue.ReviewJob) (*gitea.Client, error)
type promptLoader func(context.Context) string

// Service coordinates review persistence, queueing, provider execution, and
// publication to Gitea.
type Service struct {
	cfg             config.Config
	repo            *Repository
	publisher       queue.Publisher
	providerFactory providerFactory
	giteaFactory    giteaFactory
	promptLoader    promptLoader
}

// NewService creates a review service with queue and environment-based Gitea
// defaults. Provider and resolver factories can be supplied before processing.
func NewService(cfg config.Config, repo *Repository, publisher queue.Publisher) *Service {
	return &Service{
		cfg:       cfg,
		repo:      repo,
		publisher: publisher,
		providerFactory: func(context.Context) (providers.LLMProvider, error) {
			return nil, errors.New("AI provider is not configured")
		},
		giteaFactory: func(context.Context, queue.ReviewJob) (*gitea.Client, error) {
			return gitea.New(cfg.GiteaURL, cfg.GiteaToken), nil
		},
	}
}

// SetFactories configures lazy AI provider and per-job Gitea client factories.
// Call it during application startup before invoking Process.
func (s *Service) SetFactories(provider providerFactory, giteaClient giteaFactory) {
	s.providerFactory = provider
	s.giteaFactory = giteaClient
}

// SetPromptLoader configures the function used to load the active review
// prompt. Call it during startup before processing jobs.
func (s *Service) SetPromptLoader(loader promptLoader) {
	s.promptLoader = loader
}

// Enqueue persists a new review, records its queued step, publishes its job,
// and returns the current persisted review.
func (s *Service) Enqueue(
	ctx context.Context,
	job queue.ReviewJob,
	source string,
) (Review, error) {
	if job.ReviewID == "" {
		job.ReviewID = NewID()
	}
	if err := s.repo.Create(ctx, job.ReviewID, job, source); err != nil {
		return Review{}, err
	}
	if err := s.repo.SetStatus(ctx, job.ReviewID, StatusQueued, ""); err != nil {
		return Review{}, err
	}

	_ = s.repo.AddStep(
		ctx,
		job.ReviewID,
		"enfileirado",
		"concluido",
		"Review enfileirado",
		nil,
		time.Now().UTC(),
		nil,
		0,
		"",
	)
	if err := s.publisher.Publish(ctx, job); err != nil {
		_ = s.repo.SetStatus(ctx, job.ReviewID, StatusFailed, err.Error())
		return Review{}, err
	}

	return s.repo.Get(ctx, job.ReviewID)
}

// EnqueueExisting republishes a persisted job after setting its review status
// back to queued. The job must contain a review ID.
func (s *Service) EnqueueExisting(ctx context.Context, job queue.ReviewJob) error {
	if job.ReviewID == "" {
		return errors.New("review id is required")
	}
	if err := s.repo.SetStatus(ctx, job.ReviewID, StatusQueued, ""); err != nil {
		return err
	}
	return s.publisher.Publish(ctx, job)
}

// NewID returns a time-based review identifier prefixed with "rev-".
func NewID() string {
	return fmt.Sprintf("rev-%d", time.Now().UnixNano())
}
