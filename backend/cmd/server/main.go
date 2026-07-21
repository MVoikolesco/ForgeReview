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
	if path == "" { path = "file:forgereview.db" }
	workflows, err := store.Open(path)
	if err != nil { log.Fatal(err) }
	defer workflows.Close()
	if err = httpapi.New(workflow.DefaultCatalog(), workflows).Run(":8080"); err != nil { log.Fatal(err) }
}
