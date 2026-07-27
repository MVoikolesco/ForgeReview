package workflow

import (
	"context"
	"testing"
)

func TestCandidateValidatorPublishesOnlyConfirmedCandidates(t *testing.T) {
	model := &sequentialModel{responses: []string{
		`{"decision":"CONFIRMED","reason":"the added line demonstrates the failure"}`,
		`{"decision":"REJECTED","reason":"the guard in the same patch prevents the scenario"}`,
	}}
	runner := &scopedRunner{adapters: Adapters{
		Integrations: memoryIntegrations{"validator": modelIntegration(t, "secret")},
		Secrets:      testSecrets(t),
		OpenAI:       model,
	}}
	node := Node{Key: "validator", Type: "candidate_validator", Name: "Validator", Config: map[string]any{"integration": "validator", "max_tokens": 300}}
	candidates := []CandidateFinding{
		candidateFixture("security.auth", "app.go", 2),
		candidateFixture("correctness.guard", "app.go", 2),
		candidateFixture("contracts.missing", "missing.go", 1),
		candidateFixture("security.auth", "app.go", 2),
	}
	candidates[3].Comment = "same issue with different wording"
	outputs, metadata, err := runner.runCandidateValidator(context.Background(), node, rootScope, map[string][]any{
		"candidates": []any{candidates},
		"files":      []any{FileGroup{Files: []map[string]any{{"filename": "app.go", "patch": "@@ -1 +1,2 @@\n package app\n+unsafe()"}}}},
	})
	if err != nil {
		t.Fatalf("validate candidates: %v", err)
	}
	confirmed := outputs["confirmed"].([]Finding)
	decisions := outputs["decisions"].([]CandidateFinding)
	if len(confirmed) != 1 || confirmed[0].Path != "app.go" {
		t.Fatalf("confirmed = %#v", confirmed)
	}
	if decisions[0].Status != CandidateConfirmed || decisions[1].Status != CandidateRejected || decisions[2].Status != CandidateNotObservable || decisions[3].Status != CandidateRejected {
		t.Fatalf("decisions = %#v", decisions)
	}
	if model.calls != 2 || metadata["confirmed_count"] != 1 || metadata["rejected_count"] != 2 || metadata["not_observable_count"] != 1 || metadata["duplicate_count"] != 1 {
		t.Fatalf("calls/metadata = %d / %#v", model.calls, metadata)
	}
	if decisions[0].Fingerprint == "" || decisions[3].Fingerprint != decisions[0].Fingerprint {
		t.Fatalf("fingerprints = %q / %q", decisions[0].Fingerprint, decisions[3].Fingerprint)
	}
}

func TestCandidateDecisionContractIsClosed(t *testing.T) {
	if _, err := parseCandidateDecision(`{"decision":"CONFIRMED","reason":"evidence holds","extra":true}`); err == nil {
		t.Fatal("unknown fields must be rejected")
	}
	if _, err := parseCandidateDecision(`{"decision":"NOT_OBSERVABLE","reason":"missing"}`); err == nil {
		t.Fatal("model must not assign system-only states")
	}
}

func TestCandidateValidatorRequestsMissingSemanticContextWithoutProviderCall(t *testing.T) {
	model := &sequentialModel{}
	runner := &scopedRunner{adapters: Adapters{OpenAI: model}}
	candidate := candidateFixture("security.auth", "app.go", 2)
	candidate.RequiredContext = []string{ContextRepository}
	unit := SemanticUnit{
		UnitID: "unit", AvailableContext: []string{ContextDiff, ContextFile},
		file: map[string]any{
			"filename": "app.go", "patch": "@@ -1 +1,2 @@\n package app\n+unsafe()",
			"_forgereview_available_context": []string{ContextDiff, ContextFile},
		},
	}
	outputs, metadata, err := runner.runCandidateValidator(context.Background(), Node{
		Key: "validator", Type: "candidate_validator",
	}, "loop:000001", map[string][]any{"candidates": {[]CandidateFinding{candidate}}, "files": {unit}})
	if err != nil {
		t.Fatal(err)
	}
	decisions := outputs["decisions"].([]CandidateFinding)
	if len(decisions) != 1 || decisions[0].Status != CandidateNeedsContext || model.calls != 0 || metadata["needs_context_count"] != 1 {
		t.Fatalf("semantic context decision = %#v, calls=%d metadata=%#v", decisions, model.calls, metadata)
	}
}

func candidateFixture(checkID, path string, line int) CandidateFinding {
	return CandidateFinding{
		CheckID: checkID, Claim: "claim", Scenario: "scenario", Impact: "impact",
		Evidence: []string{"added call"}, Confidence: 0.9, RequiredContext: []string{},
		Symbol: "run", IssueType: checkID, AffectedEntity: "request",
		Path: path, Line: line, Comment: "actionable finding", Severity: "high",
	}
}

func TestCandidateFingerprintIgnoresGeneratedWording(t *testing.T) {
	file := map[string]any{
		"_forgereview_repository":  "acme/review",
		"_forgereview_base_commit": "abc123",
	}
	first := candidateFixture("security.auth", "app.go", 2)
	second := first
	second.Claim = "different wording"
	second.Comment = "different publication text"
	second.Line = 99
	if candidateFingerprint(first, file) != candidateFingerprint(second, file) {
		t.Fatal("wording and line changes must not change semantic identity")
	}
	second.AffectedEntity = "admin request"
	if candidateFingerprint(first, file) == candidateFingerprint(second, file) {
		t.Fatal("affected entity must participate in semantic identity")
	}
}

func TestFindingDeduplicationPrefersFingerprintOverText(t *testing.T) {
	findings := deduplicateFindings([]Finding{
		{Path: "app.go", Line: 2, Comment: "first wording", Severity: "high", Fingerprint: "sha256:same"},
		{Path: "app.go", Line: 9, Comment: "second wording", Severity: "critical", Fingerprint: "sha256:same"},
		{Path: "app.go", Line: 2, Comment: "another issue", Severity: "high", Fingerprint: "sha256:other"},
	})
	if len(findings) != 2 {
		t.Fatalf("deduplicated findings = %#v", findings)
	}
	for _, finding := range findings {
		if finding.Fingerprint == "sha256:same" && finding.Comment != "first wording" {
			t.Fatalf("deduplication did not retain the first semantic finding: %#v", findings)
		}
	}
}
