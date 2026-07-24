package app

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitea-agents/internal/config"
	databasepkg "gitea-agents/internal/database"
	httpapi "gitea-agents/internal/http"
	"gitea-agents/internal/integrations/gitea"
	"gitea-agents/internal/queue"
	redisqueue "gitea-agents/internal/queue/redis"
	"gitea-agents/internal/review"
)

// Run loads and validates configuration, initializes shared dependencies, and
// starts either the HTTP API or Redis worker selected by APP_MODE. It blocks
// until shutdown and returns startup or runtime errors to the command entrypoint.
func Run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}

	logger := log.New(os.Stdout, "[forgereview] ", log.LstdFlags)
	db, err := databasepkg.Open(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.AppMode == "api" {
		if err := db.Migrate(ctx); err != nil {
			return err
		}
		if err := db.Seed(ctx); err != nil {
			return err
		}
	} else if err := db.RequireSchema(ctx); err != nil {
		return err
	}

	queueClient := redisqueue.New(cfg)
	defer queueClient.Close()

	repository := review.NewRepository(db.SQL)
	service := review.NewService(cfg, repository, queueClient)
	giteaResolver := gitea.NewResolver(db.SQL, gitea.New(cfg.GiteaURL, cfg.GiteaToken))
	service.SetFactories(
		providerFactory(db.SQL, cfg),
		func(ctx context.Context, job queue.ReviewJob) (*gitea.Client, error) {
			return giteaResolver.Resolve(ctx, job.GiteaInstanceID, job.Owner, job.Repository)
		},
	)
	if cfg.AppMode == "worker" {
		return runWorker(ctx, cfg, queueClient, service, logger)
	}

	return runAPI(ctx, cfg, db.SQL, repository, service, queueClient, logger)
}

// runAPI configures graceful shutdown and serves the Gin router until the
// process context is cancelled or the HTTP server fails.
func runAPI(
	ctx context.Context,
	cfg config.Config,
	db *sql.DB,
	repository *review.Repository,
	service *review.Service,
	observer queue.Observer,
	logger *log.Logger,
) error {
	router := httpapi.NewRouter(cfg, db, repository, service, observer, logger)
	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Printf("api listening on %s", cfg.Address())
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
