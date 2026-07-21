package main

import (
	"log"
	"os"

	"forgereview/backend/internal/httpapi"
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
	if err = httpapi.New(workflow.DefaultCatalog(), workflows).Run(address); err != nil {
		log.Fatal(err)
	}
}
