package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitea-agents/internal/admin"
	"gitea-agents/internal/agents"
	"gitea-agents/internal/config"
	"gitea-agents/internal/gitea"
	redisqueue "gitea-agents/internal/queue/redis"
	"gitea-agents/internal/reviewconfig"
	"gitea-agents/internal/store"
	"gitea-agents/internal/webhook"
	"gitea-agents/internal/worker"
)

func main() {
	cfg := config.Load()
	logOutput := io.Writer(os.Stdout)
	var workerLog *os.File
	if cfg.AppMode == "worker" {
		if err := os.MkdirAll("/logs", 0o755); err == nil {
			workerLog, err = os.OpenFile("/logs/worker.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				logOutput = io.MultiWriter(os.Stdout, workerLog)
				defer workerLog.Close()
			}
		}
	}
	logger := log.New(logOutput, "["+cfg.ServiceName+"] ", log.LstdFlags)
	if err := cfg.Validate(); err != nil {
		logger.Fatalf("config invalida: %v", err)
	}
	configurationStore, err := store.Open(cfg)
	if err != nil {
		logger.Fatalf("erro ao abrir SQLite de configuracao: %v", err)
	}
	defer configurationStore.Close()
	if cfg.AppMode == "api" {
		if err := configurationStore.Initialize(context.Background()); err != nil {
			logger.Fatalf("erro ao inicializar SQLite: %v", err)
		}
		if err := configurationStore.Seed(context.Background(), cfg); err != nil {
			logger.Fatalf("erro ao criar catalogo inicial: %v", err)
		}
	} else if cfg.AppMode == "worker" {
		if err := configurationStore.RequireSchema(context.Background()); err != nil {
			logger.Fatalf("erro ao validar SQLite: %v", err)
		}
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
		runAPI(ctx, cfg, logger, reviewQueue, configurationStore)
	case "worker":
		runWorker(ctx, cfg, logger, reviewQueue, configurationStore)
	default:
		logger.Fatalf("APP_MODE inválido: %s", cfg.AppMode)
	}
}

func runAPI(ctx context.Context, cfg config.Config, logger *log.Logger, reviewQueue *redisqueue.Queue, configurationStore *store.Store) {
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
	admin.Register(mux, configurationStore, cfg.AdminUsername, cfg.AdminPassword, reviewQueue, cfg.DiffLogDir)
	mux.Handle("/", http.FileServer(http.Dir("./web")))

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

func runWorker(ctx context.Context, cfg config.Config, logger *log.Logger, reviewQueue *redisqueue.Queue, configurationStore *store.Store) {
	if err := reviewQueue.EnsureGroup(ctx); err != nil {
		logger.Fatalf("erro ao criar consumer group: %v", err)
	}

	giteaClient := gitea.NewClient(cfg.GiteaURL, cfg.GiteaToken)
	giteaResolver := gitea.NewClientResolver(configurationStore.DB, cfg.GiteaURL, cfg.GiteaToken)
	registry := agents.NewRegistry()
	registry.Register(agents.NewReviewerAgent(logger, giteaClient, nil, agents.ReviewerOptions{
		DiffLogDir:    cfg.DiffLogDir,
		MaxBlockChars: 1, MaxFilesPerBlock: 1, ReviewConcurrency: 1,
		ReviewFinalRetries:     5,
		PublishManualReviews:   false,
		AllowAutonomousReject:  false,
		OllamaTimeoutSeconds:   900,
		ReviewPromptConfigPath: cfg.ReviewPromptConfigPath,
		LogStore:               reviewQueue,
		GiteaResolver:          giteaResolver,
		ConfigProvider:         reviewconfig.SQLiteProvider{Store: configurationStore},
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
