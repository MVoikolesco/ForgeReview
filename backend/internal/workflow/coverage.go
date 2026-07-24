package workflow

import (
	"context"
	"fmt"
	"strings"
)

const (
	CoveragePlanned       = "PLANNED"
	CoverageCompleted     = "COMPLETED"
	CoverageConfirmed     = "CONFIRMED"
	CoverageRejected      = "REJECTED"
	CoverageNeedsContext  = "NEEDS_CONTEXT"
	CoverageNotObservable = "NOT_OBSERVABLE"
	CoverageNotApplicable = "NOT_APPLICABLE"
)

type CoverageRecord struct {
	ExecutionID         int64  `json:"execution_id"`
	ScopeKey            string `json:"scope_key"`
	NodeKey             string `json:"node_key"`
	ContractKey         string `json:"contract_key"`
	ContractVersion     int    `json:"contract_version"`
	CheckID             string `json:"check_id"`
	Category            string `json:"category"`
	MinimumContext      string `json:"minimum_context"`
	Planned             bool   `json:"planned"`
	Status              string `json:"status"`
	CandidatesGenerated int    `json:"candidates_generated"`
	CandidatesValidated int    `json:"candidates_validated"`
	Confirmed           int    `json:"confirmed"`
	Rejected            int    `json:"rejected"`
	NeedsContext        int    `json:"needs_context"`
	NotObservable       int    `json:"not_observable"`
	NotApplicable       int    `json:"not_applicable"`
	Attempts            int    `json:"attempts"`
	DurationMS          int64  `json:"duration_ms"`
}

type CoverageSummary struct {
	Planned       int              `json:"planned"`
	Completed     int              `json:"completed"`
	Incomplete    int              `json:"incomplete"`
	Confirmed     int              `json:"confirmed"`
	NeedsContext  int              `json:"needs_context"`
	NotObservable int              `json:"not_observable"`
	Items         []CoverageRecord `json:"items"`
}

type CoverageLedger interface {
	RecordCoverage(context.Context, []CoverageRecord) error
	CoverageComplete(context.Context, int64) (bool, error)
}

func plannedCoverage(executionID int64, nodeKey, scopeKey string, contract ReviewContractVersion) []CoverageRecord {
	if executionID < 1 || !contractConfigured(contract) {
		return nil
	}
	items := make([]CoverageRecord, 0, len(contract.Checklist.Items))
	for _, check := range contract.Checklist.Items {
		items = append(items, CoverageRecord{
			ExecutionID: executionID, ScopeKey: normalizedScope(scopeKey), NodeKey: nodeKey,
			ContractKey: contract.Key, ContractVersion: contract.Version,
			CheckID: check.CheckID, Category: check.Category, MinimumContext: check.MinimumContext,
			Planned: true, Status: CoveragePlanned,
		})
	}
	return items
}

func completedCoverage(executionID int64, nodeKey, scopeKey string, contract ReviewContractVersion, candidates []CandidateFinding, durationMS int64) []CoverageRecord {
	if executionID < 1 || !contractConfigured(contract) {
		return nil
	}
	records := map[string]*CoverageRecord{}
	order := make([]string, 0, len(contract.Checklist.Items))
	for _, check := range contract.Checklist.Items {
		order = append(order, check.CheckID)
		records[check.CheckID] = &CoverageRecord{
			ExecutionID: executionID, ScopeKey: normalizedScope(scopeKey), NodeKey: nodeKey,
			ContractKey: contract.Key, ContractVersion: contract.Version,
			CheckID: check.CheckID, Category: check.Category, MinimumContext: check.MinimumContext,
			Planned: true, Status: CoverageCompleted,
		}
	}
	for _, candidate := range candidates {
		record := records[candidate.CheckID]
		if record == nil {
			order = append(order, candidate.CheckID)
			record = &CoverageRecord{
				ExecutionID: executionID, ScopeKey: normalizedScope(scopeKey), NodeKey: nodeKey,
				ContractKey: contract.Key, ContractVersion: contract.Version,
				CheckID: candidate.CheckID, Planned: false, Status: CoverageNotApplicable,
			}
			records[candidate.CheckID] = record
		}
		record.CandidatesGenerated++
		record.CandidatesValidated++
		if candidate.ValidationAttempted {
			record.Attempts++
		}
		switch candidate.Status {
		case CandidateConfirmed:
			record.Confirmed++
		case CandidateRejected:
			record.Rejected++
		case CandidateNeedsContext:
			record.NeedsContext++
		case CandidateNotObservable:
			record.NotObservable++
		case CandidateNotApplicable:
			record.NotApplicable++
		}
	}
	result := make([]CoverageRecord, 0, len(order))
	for _, checkID := range order {
		record := records[checkID]
		record.Status = coverageStatus(*record)
		record.DurationMS = durationMS
		result = append(result, *record)
	}
	return result
}

func coverageStatus(record CoverageRecord) string {
	switch {
	case !record.Planned:
		return CoverageNotApplicable
	case record.NeedsContext > 0:
		return CoverageNeedsContext
	case record.Confirmed > 0:
		return CoverageConfirmed
	case record.NotObservable > 0:
		return CoverageNotObservable
	case record.CandidatesGenerated > 0 && record.Rejected == record.CandidatesGenerated:
		return CoverageRejected
	default:
		return CoverageCompleted
	}
}

func normalizedScope(scopeKey string) string {
	if strings.TrimSpace(scopeKey) == "" {
		return rootScope
	}
	return scopeKey
}

func ValidateCoverageRecord(record CoverageRecord) error {
	statuses := map[string]bool{
		CoveragePlanned: true, CoverageCompleted: true, CoverageConfirmed: true,
		CoverageRejected: true, CoverageNeedsContext: true,
		CoverageNotObservable: true, CoverageNotApplicable: true,
	}
	if record.ExecutionID < 1 || strings.TrimSpace(record.ScopeKey) == "" ||
		strings.TrimSpace(record.NodeKey) == "" || !checkIDPattern.MatchString(record.CheckID) ||
		!statuses[record.Status] {
		return fmt.Errorf("invalid coverage record")
	}
	return nil
}
