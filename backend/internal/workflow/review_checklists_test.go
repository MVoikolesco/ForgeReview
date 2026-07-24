package workflow

import (
	"context"
	"strings"
	"testing"
)

func TestOfficialReviewChecklistIsClosedVersionedAndValid(t *testing.T) {
	items := ReviewChecklists()
	if len(items) != 1 || items[0].Key != "official.pull-request.v1" || items[0].Version != 1 || items[0].Editable || len(items[0].Items) != 6 {
		t.Fatalf("checklists = %#v", items)
	}
	if err := validateReviewChecklist(officialReviewChecklistSnapshot()); err != nil {
		t.Fatalf("official checklist: %v", err)
	}
}

func TestReviewPromptIncludesOnlyConfiguredChecklistSnapshot(t *testing.T) {
	prompt, err := reviewPromptWithChecklist("review this diff", map[string]any{"review_checklist": officialReviewChecklistSnapshot()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Checklist fechada e versionada") || !strings.Contains(prompt, `"check_id":"security.authorization"`) {
		t.Fatalf("prompt = %s", prompt)
	}
}

func TestCandidateOutsideChecklistBecomesNotApplicableWithoutModelCall(t *testing.T) {
	model := &sequentialModel{}
	runner := &scopedRunner{adapters: Adapters{OpenAI: model}}
	node := Node{Key: "validator", Type: "candidate_validator", Config: map[string]any{"review_checklist": officialReviewChecklistSnapshot()}}
	candidate := candidateFixture("custom.unlisted", "app.go", 2)
	outputs, metadata, err := runner.runCandidateValidator(context.Background(), node, map[string][]any{
		"candidates": []any{[]CandidateFinding{candidate}},
		"files":      []any{FileGroup{Files: []map[string]any{{"filename": "app.go", "patch": "@@ -1 +1,2 @@\n package app\n+unsafe()"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	decisions := outputs["decisions"].([]CandidateFinding)
	if len(decisions) != 1 || decisions[0].Status != CandidateNotApplicable || metadata["not_applicable_count"] != 1 || model.calls != 0 {
		t.Fatalf("decisions/metadata/calls = %#v / %#v / %d", decisions, metadata, model.calls)
	}
}

func TestReviewChecklistRejectsDuplicateCheckIDs(t *testing.T) {
	checklist := officialReviewChecklistSnapshot()
	checklist.Items = append(checklist.Items, checklist.Items[0])
	if err := validateReviewChecklist(checklist); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
}
