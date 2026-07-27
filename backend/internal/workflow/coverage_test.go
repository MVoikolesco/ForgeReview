package workflow

import (
	"context"
	"testing"
)

type recordingCoverageLedger struct {
	records  []CoverageRecord
	complete bool
}

func (ledger *recordingCoverageLedger) RecordCoverage(_ context.Context, records []CoverageRecord) error {
	ledger.records = append(ledger.records, records...)
	return nil
}

func (ledger *recordingCoverageLedger) CoverageComplete(context.Context, int64) (bool, error) {
	return ledger.complete, nil
}

func TestCoveragePlansEveryContractCheckAndClosesByDecision(t *testing.T) {
	contract := BuiltInReviewContracts()[0]
	planned := plannedCoverage(41, "template", "loop:000001", contract)
	if len(planned) != 6 {
		t.Fatalf("planned checks = %#v", planned)
	}
	for _, item := range planned {
		if !item.Planned || item.Status != CoveragePlanned {
			t.Fatalf("invalid planned item = %#v", item)
		}
	}

	confirmed := candidateFixture(contract.Checklist.Items[0].CheckID, "app.go", 2)
	confirmed.Status = CandidateConfirmed
	confirmed.ValidationAttempted = true
	needsContext := candidateFixture(contract.Checklist.Items[1].CheckID, "app.go", 3)
	needsContext.Status = CandidateNeedsContext
	needsContext.ValidationAttempted = true
	outside := candidateFixture("custom.outside", "app.go", 4)
	outside.Status = CandidateNotApplicable

	completed := completedCoverage(41, "candidate-validator", "loop:000001", contract, []CandidateFinding{confirmed, needsContext, outside}, 25)
	if len(completed) != 7 {
		t.Fatalf("completed checks = %#v", completed)
	}
	byCheck := map[string]CoverageRecord{}
	for _, item := range completed {
		byCheck[item.CheckID] = item
	}
	if byCheck[confirmed.CheckID].Status != CoverageConfirmed || byCheck[confirmed.CheckID].Attempts != 1 {
		t.Fatalf("confirmed coverage = %#v", byCheck[confirmed.CheckID])
	}
	if byCheck[needsContext.CheckID].Status != CoverageNeedsContext {
		t.Fatalf("needs-context coverage = %#v", byCheck[needsContext.CheckID])
	}
	if byCheck[outside.CheckID].Status != CoverageNotApplicable || byCheck[outside.CheckID].Planned {
		t.Fatalf("outside coverage = %#v", byCheck[outside.CheckID])
	}
}

func TestCoverageWithNoCandidatesStillCompletesPlannedChecks(t *testing.T) {
	contract := BuiltInReviewContracts()[1]
	items := completedCoverage(7, "validator", rootScope, contract, nil, 4)
	if len(items) != 1 || items[0].Status != CoverageCompleted || items[0].CandidatesGenerated != 0 {
		t.Fatalf("empty candidate coverage = %#v", items)
	}
}

func TestCandidateValidatorClosesCoverageForAnEmptyContractResponse(t *testing.T) {
	contract := BuiltInReviewContracts()[1]
	ledger := &recordingCoverageLedger{}
	runner := &scopedRunner{adapters: Adapters{
		Coverage:  ledger,
		Execution: ExecutionContext{ID: 12, VersionID: 3},
	}}
	outputs, metadata, err := runner.runCandidateValidator(context.Background(), Node{
		Key: "validator", Type: "candidate_validator",
	}, "loop:000001", map[string][]any{
		"candidates": {ValidatedReviewResponse{Value: []CandidateFinding{}, Contract: contract}},
		"files": {SemanticUnit{
			UnitID: "unit-12", AvailableContext: []string{ContextDiff, ContextFile},
			file: map[string]any{"filename": "app.go", "patch": "@@ -1 +1 @@\n+ok", "_forgereview_available_context": []string{ContextDiff, ContextFile}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs["confirmed"].([]Finding)) != 0 || metadata["coverage_checks"] != 1 {
		t.Fatalf("empty validator output = %#v, %#v", outputs, metadata)
	}
	if len(ledger.records) != 1 || ledger.records[0].Status != CoverageCompleted || ledger.records[0].UnitID != "unit-12" {
		t.Fatalf("recorded coverage = %#v", ledger.records)
	}
}

func TestSemanticCoverageMarksUnavailableMinimumContext(t *testing.T) {
	var contract ReviewContractVersion
	for _, item := range BuiltInReviewContracts() {
		if item.Key == "review.performance" {
			contract = item
			break
		}
	}
	records := completedCoverage(9, "validator", "loop:000001", contract, nil, 1)
	records = coverageWithSemanticObservability(records, SemanticUnit{
		AvailableContext: []string{ContextDiff, ContextFile},
	})
	if len(records) != 1 || records[0].Status != CoverageNotObservable || records[0].NotObservable != 1 {
		t.Fatalf("semantic observability coverage = %#v", records)
	}
}
