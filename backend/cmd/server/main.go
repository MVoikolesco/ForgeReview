package main

import (
	"log"
	"os"

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
		OpenAI:       integration.HTTPOpenAIClient{},
		Ollama:       integration.HTTPOllamaClient{},
	}
	if err = httpapi.New(workflow.DefaultCatalog(), workflows, adapters).Run(address); err != nil {
		log.Fatal(err)
	}
}
