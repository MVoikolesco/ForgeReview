package store

import (
	"context"
	"testing"

	"forgereview/backend/internal/workflow"
)

func TestOpenSeedsImmutableReviewContractVersions(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/contracts.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	items, err := database.ReviewContracts(context.Background())
	if err != nil || len(items) != 7 {
		t.Fatalf("review contracts = %#v, %v", items, err)
	}
	item, err := database.ReviewContract(context.Background(), workflow.OfficialPullRequestContractKey, 1)
	if err != nil || len(item.Checklist.Items) != 6 || item.ResponseSchema["type"] != "array" {
		t.Fatalf("official review contract = %#v, %v", item, err)
	}
}

func TestSaveRejectsUnknownReviewContractVersion(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/unknown-contract.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	definition := workflow.Definition{
		Key: "unknown-contract", Name: "Unknown contract",
		Nodes: []workflow.Node{{
			Key: "template", Type: "template", Name: "Template",
			Config: map[string]any{"template": "review", "review_contract_key": "missing", "review_contract_version": 9},
		}},
	}
	if _, err = database.Save(context.Background(), definition); err == nil {
		t.Fatal("unknown review contract should not be saved")
	}
}
