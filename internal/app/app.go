package app

import (
	"context"
	"database/sql"
	"fmt"
	"gitea-agents/internal/config"
	databasepkg "gitea-agents/internal/database"
	httpapi "gitea-agents/internal/http"
	"gitea-agents/internal/integrations/gitea"
	"gitea-agents/internal/providers"
	"gitea-agents/internal/queue"
	redisqueue "gitea-agents/internal/queue/redis"
	"gitea-agents/internal/review"
	"gitea-agents/internal/security"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

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
	q := redisqueue.New(cfg)
	defer q.Close()
	repo := review.NewRepository(db.SQL)
	service := review.NewService(cfg, repo, q)
	giteaResolver := gitea.NewResolver(db.SQL, gitea.New(cfg.GiteaURL, cfg.GiteaToken))
	service.SetFactories(providerFactory(db.SQL, cfg), func(ctx context.Context, job queue.ReviewJob) (*gitea.Client, error) {
		return giteaResolver.Resolve(ctx, job.GiteaInstanceID, job.Owner, job.Repository)
	})
	service.SetPromptLoader(func(ctx context.Context) string { value, _ := repo.DefaultPrompt(ctx); return value })
	if cfg.AppMode == "worker" {
		return runWorker(ctx, cfg, q, service, logger)
	}
	router := httpapi.NewRouter(cfg, db.SQL, repo, service, q, logger)
	server := &http.Server{Addr: cfg.Address(), Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	logger.Printf("api listening on %s", cfg.Address())
	err = server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
func runWorker(ctx context.Context, cfg config.Config, q *redisqueue.Queue, service *review.Service, logger *log.Logger) error {
	if err := q.EnsureGroup(ctx); err != nil {
		return err
	}
	logger.Printf("worker listening on stream=%s group=%s consumer=%s", cfg.RedisStream, cfg.RedisGroup, cfg.RedisConsumer)
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var current string
	var stateMu sync.RWMutex
	setHeartbeat := func() {
		stateMu.RLock()
		job := current
		stateMu.RUnlock()
		state := "idle"
		if job != "" {
			state = "processing"
		}
		_ = q.Heartbeat(workerCtx, cfg.RedisConsumer, state, job)
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				setHeartbeat()
			}
		}
	}()
	setHeartbeat()
	return q.Consume(workerCtx, cfg.RedisConsumer, func(jobCtx context.Context, job queue.ReviewJob) error {
		stateMu.Lock()
		current = job.ReviewID
		stateMu.Unlock()
		setHeartbeat()
		err := service.Process(jobCtx, job)
		stateMu.Lock()
		current = ""
		stateMu.Unlock()
		setHeartbeat()
		_ = q.RecordJob(jobCtx, cfg.RedisConsumer, err == nil)
		if err != nil {
			logger.Printf("review job failed without stopping worker: err=%v", err)
		}
		return nil
	})
}
func providerFactory(db *sql.DB, cfg config.Config) func(context.Context) (providers.LLMProvider, error) {
	return func(ctx context.Context) (providers.LLMProvider, error) {
		var provider, baseURL, model, ciphertext string
		var timeout int
		if err := db.QueryRowContext(ctx, `SELECT p.name,c.base_url,m.provider_model_name,c.api_key_ciphertext,COALESCE(mp.timeout_seconds,?) FROM review_profiles rp JOIN ai_models m ON m.id=rp.model_id JOIN ai_connections c ON c.id=m.connection_id JOIN ai_providers p ON p.id=c.provider_id LEFT JOIN model_parameters mp ON mp.model_id=m.id WHERE rp.is_default=1 AND rp.is_enabled=1 AND m.is_enabled=1 AND c.is_enabled=1 AND p.is_enabled=1 LIMIT 1`, cfg.ReviewRequestTimeoutSeconds).Scan(&provider, &baseURL, &model, &ciphertext, &timeout); err != nil {
			return nil, fmt.Errorf("review provider is not configured")
		}
		key := ""
		if ciphertext != "" {
			key, _ = security.Decrypt(ciphertext)
		}
		return providers.New(providers.Config{Name: provider, BaseURL: baseURL, APIKey: key, Model: model, Timeout: time.Duration(timeout) * time.Second}), nil
	}
}
