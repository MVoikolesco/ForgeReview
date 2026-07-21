package main

import (
	"context"
	"log"
	"os"

	"forgereview/backend/internal/dispatch"
	"forgereview/backend/internal/httpapi"
	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/store"
	"forgereview/backend/internal/workflow"
)

func main() {
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
	adapters := workflow.Adapters{
		Integrations: workflows,
		Gitea:        integration.HTTPGiteaClient{},
		GiteaWriter:  integration.HTTPGiteaClient{},
		OpenAI:       integration.HTTPOpenAIClient{},
		Ollama:       integration.HTTPOllamaClient{},
		Publications: workflows,
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
		} else {
			executionQueue = dispatch.NewInProcessQueue()
		}
		adapters.Dispatcher = executionQueue
		go func() {
			if workerErr := (dispatch.Worker{Queue: executionQueue, Store: workflows, Catalog: workflow.DefaultCatalog(), Adapters: adapters}).Run(context.Background()); workerErr != nil {
				log.Printf("execution worker stopped: %v", workerErr)
			}
		}()
	}
	if err = httpapi.New(workflow.DefaultCatalog(), workflows, adapters).Run(address); err != nil {
		log.Fatal(err)
	}
}
