package store

import (
	"context"
	"errors"
	"testing"

	"forgereview/backend/internal/workflow"
)

func TestPublishArchivesPriorPublishedVersionAtomically(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/versions.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	firstID, err := database.Save(context.Background(), versionedDefinition("Review v1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), firstID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	secondID, err := database.Save(context.Background(), versionedDefinition("Review v2"))
	if err != nil {
		t.Fatal(err)
	}
	published, err := database.Publish(context.Background(), secondID, workflow.DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if published.Version != 2 || published.Status != workflow.VersionStatusPublished || published.CreatedAt == "" {
		t.Fatalf("published summary = %#v", published)
	}

	items, err := database.ListDefinitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Versions) != 2 {
		t.Fatalf("workflow summaries = %#v", items)
	}
	versions := items[0].Versions
	if versions[0].Version != 1 || versions[0].Status != workflow.VersionStatusArchived || versions[1].Version != 2 || versions[1].Status != workflow.VersionStatusPublished {
		t.Fatalf("status transitions = %#v", versions)
	}
	if _, err = database.Publish(context.Background(), secondID, workflow.DefaultCatalog()); !errors.Is(err, ErrWorkflowVersionNotDraft) {
		t.Fatalf("republish error = %v, want non-draft error", err)
	}
}

func TestPublishRejectsInvalidDraftWithoutArchivingPublishedVersion(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/invalid-version.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	publishedID, err := database.Save(context.Background(), versionedDefinition("Published"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), publishedID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	invalidID, err := database.Save(context.Background(), workflow.Definition{Key: "review", Name: "Invalid", Nodes: []workflow.Node{{Key: "unknown", Type: "unknown", Name: "Unknown"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), invalidID, workflow.DefaultCatalog()); !errors.Is(err, ErrInvalidWorkflowVersion) {
		t.Fatalf("publish invalid draft error = %v", err)
	}
	if _, err = database.Publish(context.Background(), 999, workflow.DefaultCatalog()); !errors.Is(err, ErrWorkflowVersionNotFound) {
		t.Fatalf("publish missing draft error = %v", err)
	}

	items, err := database.ListDefinitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	statuses := items[0].Versions
	if statuses[0].Status != workflow.VersionStatusPublished || statuses[1].Status != workflow.VersionStatusDraft {
		t.Fatalf("invalid publication changed statuses: %#v", statuses)
	}
}

func versionedDefinition(name string) workflow.Definition {
	return workflow.Definition{Key: "review", Name: name, Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}}
}
