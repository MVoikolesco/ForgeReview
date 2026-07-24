package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"

	"forgereview/backend/internal/auth"
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

func TestPublishValidatesPinnedSubpipelineInterface(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/subpipeline.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	child := workflow.Definition{Key: "child", Name: "Child", Interface: &workflow.WorkflowInterface{
		Inputs:  []workflow.InterfaceField{{Key: "payload", Contract: "any"}},
		Outputs: []workflow.InterfaceField{{Key: "result", Contract: "event", Required: true, NodeKey: "start", PortKey: "event"}},
	}, Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}}
	childID, err := database.Save(context.Background(), child)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), childID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	// Publishing a replacement archives the referenced immutable version; it
	// remains a valid pin for a parent pipeline.
	replacementID, _ := database.Save(context.Background(), child)
	if _, err = database.Publish(context.Background(), replacementID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	parent := workflow.Definition{Key: "parent", Name: "Parent", Nodes: []workflow.Node{{
		Key: "child", Type: "workflow", Name: "Child", Config: map[string]any{"workflow_key": "child", "workflow_version_id": childID},
	}}}
	parentID, err := database.Save(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), parentID, workflow.DefaultCatalog()); err != nil {
		t.Fatalf("publish parent with archived pin: %v", err)
	}
	missing := parent
	missing.Key = "missing-parent"
	missing.Nodes[0].Config = map[string]any{"workflow_version_id": 999999}
	missingID, _ := database.Save(context.Background(), missing)
	if _, err = database.Publish(context.Background(), missingID, workflow.DefaultCatalog()); !errors.Is(err, ErrInvalidWorkflowVersion) {
		t.Fatalf("missing subpipeline error = %v", err)
	}
}

func TestEnsureOfficialReviewWorkflowSeedsOnceWithoutChangingUserWorkflows(t *testing.T) {
	path := "file:" + t.TempDir() + "/official-review.db"
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	userID, err := database.Save(context.Background(), versionedDefinition("User review"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), userID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}

	first, seeded, err := database.EnsureOfficialReviewWorkflow(context.Background(), workflow.DefaultCatalog())
	if err != nil || !seeded || first.Version != workflow.OfficialReviewWorkflowVersion || first.Status != workflow.VersionStatusPublished {
		t.Fatalf("first official seed = %#v, %t, %v", first, seeded, err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	second, seeded, err := database.EnsureOfficialReviewWorkflow(context.Background(), workflow.DefaultCatalog())
	if err != nil || seeded || second != first {
		t.Fatalf("second official seed = %#v, %t, %v", second, seeded, err)
	}

	items, err := database.ListDefinitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Key != workflow.OfficialReviewWorkflowKey || len(items[0].Versions) != 1 || items[0].Versions[0] != first || items[1].Key != "review" || items[1].Versions[0].Status != workflow.VersionStatusPublished {
		t.Fatalf("workflow summaries = %#v", items)
	}
}

func TestEnsureOfficialReviewWorkflowAppendOnlyUpgradesUntouchedLegacySeed(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/official-upgrade.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	legacyID, err := database.Save(context.Background(), workflow.PreviousOfficialReviewDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), legacyID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	upgraded, seeded, err := database.EnsureOfficialReviewWorkflow(context.Background(), workflow.DefaultCatalog())
	if err != nil || !seeded || upgraded.Version != 2 || upgraded.Status != workflow.VersionStatusPublished {
		t.Fatalf("official upgrade = %#v, %t, %v", upgraded, seeded, err)
	}
	items, err := database.ListDefinitions(context.Background())
	if err != nil || len(items) != 1 || len(items[0].Versions) != 2 || items[0].Versions[0].Status != workflow.VersionStatusArchived || items[0].Versions[1].Status != workflow.VersionStatusPublished {
		t.Fatalf("append-only versions = %#v, %v", items, err)
	}
	definition, err := database.Load(context.Background(), upgraded.ID)
	if err != nil || len(definition.Edges) != len(workflow.OfficialReviewDefinition().Edges) || definition.Nodes[0].Config["mode"] != "webhook" {
		t.Fatalf("upgraded definition = %#v, %v", definition, err)
	}
}

func TestEnsureOfficialReviewWorkflowAppendOnlyAddsCandidateValidation(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/candidate-upgrade.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previousID, err := database.Save(context.Background(), workflow.PreviousVerifiableReviewDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), previousID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	upgraded, seeded, err := database.EnsureOfficialReviewWorkflow(context.Background(), workflow.DefaultCatalog())
	if err != nil || !seeded || upgraded.Version != 2 {
		t.Fatalf("candidate upgrade = %#v, %t, %v", upgraded, seeded, err)
	}
	definition, err := database.Load(context.Background(), upgraded.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundValidator := false
	for _, node := range definition.Nodes {
		foundValidator = foundValidator || node.Type == "candidate_validator"
	}
	if !foundValidator {
		t.Fatalf("upgraded definition has no candidate validator: %#v", definition)
	}
}

func TestEnsureOfficialReviewWorkflowAppendOnlyMovesContractToTemplate(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/checklist-upgrade.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previousID, err := database.Save(context.Background(), workflow.PreviousChecklistReviewDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), previousID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	upgraded, seeded, err := database.EnsureOfficialReviewWorkflow(context.Background(), workflow.DefaultCatalog())
	if err != nil || !seeded || upgraded.Version != 2 {
		t.Fatalf("review contract upgrade = %#v, %t, %v", upgraded, seeded, err)
	}
	definition, err := database.Load(context.Background(), upgraded.ID)
	if err != nil {
		t.Fatal(err)
	}
	var templateConfig map[string]any
	for _, node := range definition.Nodes {
		if node.Type == "template" {
			templateConfig = node.Config
		}
		if (node.Type == "model" || node.Type == "candidate_validator") && node.Config["review_checklist"] != nil {
			t.Fatalf("downstream checklist duplicate remains in %#v", node)
		}
		if node.Type == "validate" && node.Config["response_schema"] != nil {
			t.Fatalf("validate schema duplicate remains in %#v", node)
		}
	}
	if templateConfig["review_contract_key"] != workflow.OfficialPullRequestContractKey || templateConfig["review_contract_version"] != float64(1) {
		t.Fatalf("template contract reference = %#v", templateConfig)
	}
}

func TestSaveRejectsFixedPullRequestCoordinates(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/reject-fixed.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	definition := workflow.Definition{Key: "fixed", Name: "Fixed", Nodes: []workflow.Node{{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"owner": "acme"}}}}
	if _, err = database.Save(context.Background(), definition); err == nil || !strings.Contains(err.Error(), "does not support fixed PR coordinate") {
		t.Fatalf("fixed coordinate save error = %v", err)
	}
}

func TestDeleteWorkflowVersionBlocksEveryRetainedDependency(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/delete-dependencies.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	versionID, err := database.Save(context.Background(), versionedDefinition("Retained"))
	if err != nil {
		t.Fatal(err)
	}
	executionID, err := database.CreateExecution(context.Background(), versionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = database.BeginPublication(context.Background(), workflow.PublicationAttempt{IdempotencyKey: "retained-publication", ExecutionID: executionID, VersionID: versionID, NodeKey: "publish"}); err != nil {
		t.Fatal(err)
	}
	if err = database.UpsertWebhookRegistration(context.Background(), workflow.WebhookRegistration{Key: "retained-hook", Name: "Retained hook", WorkflowKey: "review", TriggerNodeKey: "start", SecretCiphertext: "ciphertext", Active: true}); err != nil {
		t.Fatal(err)
	}
	admin, err := database.CreateUser(context.Background(), "admin@example.test", "hash", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Audit(context.Background(), admin.ID, "workflow.published", "workflow_version:"+strconv.FormatInt(versionID, 10), nil); err != nil {
		t.Fatal(err)
	}
	err = database.DeleteWorkflowVersion(context.Background(), versionID, admin.ID)
	if !errors.Is(err, ErrWorkflowVersionDeletionBlocked) || !strings.Contains(err.Error(), "execution history") || !strings.Contains(err.Error(), "publication attempts") || !strings.Contains(err.Error(), "webhook registrations") || !strings.Contains(err.Error(), "audit history") {
		t.Fatalf("delete dependency error = %v", err)
	}
	if _, err = database.Load(context.Background(), versionID); err != nil {
		t.Fatalf("blocked deletion removed version: %v", err)
	}
}

func TestDeleteWorkflowVersionOnlyRemovesEligibleVersionAndAuditsIt(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/delete-version.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	admin, err := database.CreateUser(context.Background(), "admin@example.test", "hash", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	published, err := database.Save(context.Background(), versionedDefinition("Published"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), published, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	draft, err := database.Save(context.Background(), versionedDefinition("Draft"))
	if err != nil {
		t.Fatal(err)
	}
	if err = database.DeleteWorkflowVersion(context.Background(), published, admin.ID); !errors.Is(err, ErrWorkflowVersionNotDeletable) {
		t.Fatalf("delete published = %v", err)
	}
	if err = database.DeleteWorkflowVersion(context.Background(), draft, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Load(context.Background(), draft); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted draft load = %v", err)
	}
	if _, err = database.Load(context.Background(), published); err != nil {
		t.Fatalf("published version removed = %v", err)
	}
	replacement, err := database.Save(context.Background(), versionedDefinition("Replacement"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), replacement, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	if err = database.DeleteWorkflowVersion(context.Background(), published, admin.ID); err != nil {
		t.Fatalf("delete archived = %v", err)
	}
	if _, err = database.Load(context.Background(), published); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted archived load = %v", err)
	}
	audit, err := database.AuditEntries(context.Background(), 2)
	if err != nil || len(audit) != 2 || audit[0].Action != "workflow_version.deleted" || audit[0].Target != "workflow_version:"+strconv.FormatInt(published, 10) || audit[1].Action != "workflow_version.deleted" || audit[1].Target != "workflow_version:"+strconv.FormatInt(draft, 10) || audit[0].ActorID != admin.ID || audit[1].ActorID != admin.ID {
		t.Fatalf("deletion audit = %#v, %v", audit, err)
	}
}

func versionedDefinition(name string) workflow.Definition {
	return workflow.Definition{Key: "review", Name: name, Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}}
}
