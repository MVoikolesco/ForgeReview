package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type memoryReviewContracts map[string]ReviewContractVersion

func (items memoryReviewContracts) ReviewContract(_ context.Context, key string, version int) (ReviewContractVersion, error) {
	item, ok := items[key]
	if !ok || item.Version != version {
		return ReviewContractVersion{}, errors.New("not found")
	}
	return item, nil
}

func TestTemplateResolvesAndCarriesImmutableReviewContract(t *testing.T) {
	contract := BuiltInReviewContracts()[0]
	coverage := &recordingCoverageLedger{}
	unit := SemanticUnit{UnitID: "unit-1", Path: "api.go", Kind: "hunk", Symbol: "<file>", Diff: "@@ -1 +1 @@\n+fix"}
	outputs, err := execute(context.Background(), Node{
		Key: "template", Type: "template",
		Config: map[string]any{
			"template":                "Revise {{context}}",
			"review_contract_key":     contract.Key,
			"review_contract_version": contract.Version,
		},
	}, map[string][]any{"context": {unit}}, nil, Adapters{
		ReviewContracts: memoryReviewContracts{contract.Key: contract},
		Coverage:        coverage,
		Execution:       ExecutionContext{ID: 8, VersionID: 2},
	}, rootScope)
	if err != nil {
		t.Fatal(err)
	}
	task, ok := outputs["prompt"].(ReviewTask)
	if !ok || task.Contract.Key != contract.Key || task.Contract.Version != contract.Version {
		t.Fatalf("review task = %#v", outputs["prompt"])
	}
	if !strings.Contains(task.Prompt, "security.authorization") {
		t.Fatalf("contract checklist was not attached to prompt: %q", task.Prompt)
	}
	if len(coverage.records) != 6 || coverage.records[0].Status != CoveragePlanned || coverage.records[0].UnitID != "unit-1" {
		t.Fatalf("planned coverage = %#v", coverage.records)
	}

	modelOutput := ReviewModelResponse{Content: `[]`, Contract: task.Contract}
	files := []any{[]map[string]any{{"filename": "api.go", "patch": "@@ -1 +1 @@\n+fix"}}}
	validated, port := validateResponse([]any{modelOutput}, files, map[string]any{"validate_paths": true})
	envelope, ok := validated.(ValidatedReviewResponse)
	if port != "valid" || !ok || envelope.Contract.Key != contract.Key {
		t.Fatalf("validated contract = %#v, %s", validated, port)
	}
}

func TestReviewContractReferenceIsAtomic(t *testing.T) {
	if _, _, err := ReviewContractReference(map[string]any{"review_contract_key": "missing-version"}); err == nil {
		t.Fatal("partial review contract reference should be rejected")
	}
}
