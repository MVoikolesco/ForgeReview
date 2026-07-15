package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitea-agents/internal/agents"
	"gitea-agents/internal/config"
	"gitea-agents/internal/gitea"
	"gitea-agents/internal/ollama"
	redisqueue "gitea-agents/internal/queue/redis"
	"gitea-agents/internal/webhook"
	"gitea-agents/internal/worker"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "["+cfg.ServiceName+"] ", log.LstdFlags)
	if err := cfg.Validate(); err != nil {
		logger.Fatalf("config invalida: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reviewQueue := redisqueue.New(cfg)
	defer func() {
		if err := reviewQueue.Close(); err != nil {
			logger.Printf("erro ao fechar redis: %v", err)
		}
	}()

	switch cfg.AppMode {
	case "api":
		runAPI(ctx, cfg, logger, reviewQueue)
	case "worker":
		runWorker(ctx, cfg, logger, reviewQueue)
	default:
		logger.Fatalf("APP_MODE inválido: %s", cfg.AppMode)
	}
}

func runAPI(ctx context.Context, cfg config.Config, logger *log.Logger, reviewQueue *redisqueue.Queue) {
	mux := http.NewServeMux()
	webhook.RegisterRoutes(
		mux,
		logger,
		reviewQueue,
		cfg.GiteaBotUsername,
		cfg.ServiceName,
		cfg.Version,
		time.Now(),
	)

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("erro ao desligar servidor: %v", err)
		}
	}()

	logger.Printf("api listening on %s", cfg.Address())

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server stopped: %v", err)
	}
}

func runWorker(ctx context.Context, cfg config.Config, logger *log.Logger, reviewQueue *redisqueue.Queue) {
	if err := reviewQueue.EnsureGroup(ctx); err != nil {
		logger.Fatalf("erro ao criar consumer group: %v", err)
	}

	giteaClient := gitea.NewClient(cfg.GiteaURL, cfg.GiteaToken)
	ollamaClient := ollama.NewClient(ollama.Config{
		URL:   cfg.OllamaURL,
		Model: cfg.OllamaModel,
		Options: ollama.Options{
			Temperature:   cfg.OllamaTemperature,
			TopP:          cfg.OllamaTopP,
			RepeatPenalty: cfg.OllamaRepeatPenalty,
			NumCtx:        cfg.OllamaNumCtx,
			NumThread:     cfg.OllamaNumThread,
			NumPredict:    cfg.OllamaNumPredict,
		},
		KeepAlive:      cfg.OllamaKeepAlive,
		TimeoutSeconds: cfg.OllamaTimeoutSeconds,
	})
	registry := agents.NewRegistry()
	registry.Register(agents.NewReviewerAgent(logger, giteaClient, ollamaClient, agents.ReviewerOptions{
		DiffLogDir:             cfg.DiffLogDir,
		MaxBlockChars:          cfg.ReviewMaxBlockChars,
		MaxFilesPerBlock:       cfg.ReviewMaxFilesPerBlock,
		ReviewConcurrency:      cfg.ReviewConcurrency,
		ReviewFinalRetries:     cfg.ReviewFinalRetries,
		PublishManualReviews:   cfg.ReviewPublishManual,
		AllowAutonomousReject:  cfg.ReviewAllowRejection,
		OllamaTimeoutSeconds:   cfg.OllamaTimeoutSeconds,
		ReviewPromptConfigPath: cfg.ReviewPromptConfigPath,
	}))

	agent, err := registry.Get(agents.ReviewerAgentName)
	if err != nil {
		logger.Fatalf("erro ao carregar agent: %v", err)
	}

	reviewWorker := worker.New(logger, reviewQueue, cfg.RedisConsumer, agent)
	if err := reviewWorker.Run(ctx); err != nil {
		logger.Fatalf("worker stopped: %v", err)
	}
}
