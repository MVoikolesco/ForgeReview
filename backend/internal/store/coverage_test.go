package store

import (
	"context"
	"testing"

	"forgereview/backend/internal/workflow"
)

func TestCoverageLedgerPersistsPlannedAndTerminalStates(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/coverage.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	versionID, err := database.Save(context.Background(), workflow.Definition{
		Key: "coverage", Name: "Coverage",
		Nodes: []workflow.Node{{Key: "trigger", Type: "trigger", Name: "Trigger"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	executionID, err := database.CreateExecution(context.Background(), versionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	planned := workflow.CoverageRecord{
		ExecutionID: executionID, ScopeKey: "loop:000001", NodeKey: "template",
		ContractKey: "review.security", ContractVersion: 1,
		CheckID: "security.authorization", Category: "security", MinimumContext: "file",
		Planned: true, Status: workflow.CoveragePlanned,
	}
	if err = database.RecordCoverage(context.Background(), []workflow.CoverageRecord{planned}); err != nil {
		t.Fatal(err)
	}
	if complete, completeErr := database.CoverageComplete(context.Background(), executionID); completeErr != nil || complete {
		t.Fatalf("planned coverage complete = %t, %v", complete, completeErr)
	}

	terminal := planned
	terminal.NodeKey = "candidate-validator"
	terminal.Status = workflow.CoverageConfirmed
	terminal.CandidatesGenerated = 1
	terminal.CandidatesValidated = 1
	terminal.Confirmed = 1
	terminal.Attempts = 1
	terminal.DurationMS = 12
	if err = database.RecordCoverage(context.Background(), []workflow.CoverageRecord{terminal}); err != nil {
		t.Fatal(err)
	}
	summary, err := database.Coverage(context.Background(), executionID)
	if err != nil || summary.Planned != 1 || summary.Completed != 1 || summary.Incomplete != 0 || summary.Confirmed != 1 {
		t.Fatalf("coverage summary = %#v, %v", summary, err)
	}
	status, err := database.Execution(context.Background(), executionID)
	if err != nil || status.Coverage == nil || status.Coverage.Completed != 1 {
		t.Fatalf("execution coverage = %#v, %v", status.Coverage, err)
	}
}
