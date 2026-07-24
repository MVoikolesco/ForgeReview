package main

import (
	"context"
	"log"
	"os"
	"time"

	"forgereview/backend/internal/auth"
	"forgereview/backend/internal/dispatch"
	"forgereview/backend/internal/executionlog"
	"forgereview/backend/internal/httpapi"
	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/store"
	"forgereview/backend/internal/workflow"

	"github.com/gin-gonic/gin"
)

func main() {
	if os.Getenv("FORGEREVIEW_GIN_MODE") == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	secrets, err := integration.NewEncryptedSecretsFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}
	path := os.Getenv("FORGEREVIEW_DATABASE_URL")
	if path == "" {
		path = "file:forgereview.db"
	}
	address := os.Getenv("FORGEREVIEW_HTTP_ADDR")
	if address == "" {
		address = ":8088"
	}
	workflows, err := store.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer workflows.Close()
	if err = workflows.PruneExecutionSensitiveData(context.Background()); err != nil {
		log.Fatal(err)
	}
	sessions, err := auth.New(workflows, auth.Config{SigningKey: os.Getenv("FORGEREVIEW_SESSION_SIGNING_KEY")})
	if err != nil {
		log.Fatal(err)
	}
	if err = sessions.Bootstrap(context.Background(), os.Getenv("FORGEREVIEW_BOOTSTRAP_ADMIN_EMAIL"), os.Getenv("FORGEREVIEW_BOOTSTRAP_ADMIN_PASSWORD")); err != nil {
		log.Fatal(err)
	}
	if _, _, err = workflows.EnsureOfficialReviewWorkflow(context.Background(), workflow.DefaultCatalog()); err != nil {
		log.Fatal(err)
	}
	logDirectory := os.Getenv("FORGEREVIEW_LOG_DIR")
	if logDirectory == "" {
		logDirectory = "logs"
	}
	executionLogs, err := executionlog.New(logDirectory)
	if err != nil {
		log.Fatal(err)
	}
	adapters := workflow.Adapters{
		Integrations:  workflows,
		ModelProfiles: workflows,
		Secrets:       secrets,
		Gitea:         integration.HTTPGiteaClient{},
		GiteaWriter:   integration.HTTPGiteaClient{},
		OpenAI:        integration.HTTPOpenAIClient{},
		Ollama:        integration.HTTPOllamaClient{},
		Publications:  workflows,
		Workflows:     workflows,
		Logs:          executionLogs,
	}
	if redisURL := os.Getenv("FORGEREVIEW_REDIS_URL"); redisURL != "" {
		queue, queueErr := dispatch.NewRedisQueue(redisURL)
		if queueErr != nil {
			if os.Getenv("FORGEREVIEW_ALLOW_IN_PROCESS_QUEUE") != "true" {
				log.Fatal(queueErr)
			}
			queue = nil
			log.Printf("Redis unavailable; using explicitly enabled in-process queue")
		}
		var executionQueue dispatch.Queue
		if queue != nil {
			executionQueue = queue
			defer queue.Close()
			cache, cacheErr := dispatch.NewRedisCache(redisURL)
			if cacheErr != nil {
				log.Printf("Redis cache unavailable; cache cards are disabled: %v", cacheErr)
			} else {
				adapters.Cache = cache
				defer cache.Close()
			}
		} else {
			executionQueue = dispatch.NewInProcessQueue()
		}
		adapters.Dispatcher = executionQueue
		queuedIDs, recoveryErr := workflows.RecoverableExecutionIDs(context.Background(), 15*time.Minute)
		if recoveryErr != nil {
			log.Fatal(recoveryErr)
		}
		for _, executionID := range queuedIDs {
			if recoveryErr = executionQueue.Enqueue(context.Background(), executionID); recoveryErr != nil {
				log.Fatal(recoveryErr)
			}
		}
		if len(queuedIDs) > 0 {
			log.Printf("requeued %d durable SQLite execution(s)", len(queuedIDs))
		}
		go func() {
			if workerErr := (dispatch.Worker{Queue: executionQueue, Store: workflows, Catalog: workflow.DefaultCatalog(), Adapters: adapters}).Run(context.Background()); workerErr != nil {
				log.Printf("execution worker stopped: %v", workerErr)
			}
		}()
		go recoverExecutionQueue(workflows, executionQueue)
	}
	if err = httpapi.NewWithAuth(workflow.DefaultCatalog(), workflows, sessions, adapters).Run(address); err != nil {
		log.Fatal(err)
	}
}

func recoverExecutionQueue(workflows *store.SQLite, queue dispatch.Queue) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ids, err := workflows.RecoverableExecutionIDs(context.Background(), 15*time.Minute)
		if err != nil {
			log.Printf("execution recovery scan failed: %v", err)
			continue
		}
		for _, id := range ids {
			if err = queue.Enqueue(context.Background(), id); err != nil {
				log.Printf("execution recovery enqueue failed: %v", err)
			}
		}
	}
}
