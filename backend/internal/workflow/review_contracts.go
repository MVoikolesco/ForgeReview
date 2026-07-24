package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const OfficialPullRequestContractKey = "official.pull-request"

// ReviewContractVersion is the immutable backend model selected by a Template.
// Prompt text remains workflow-owned; the response shape and closed checklist
// remain contract-owned and are versioned independently from the graph.
type ReviewContractVersion struct {
	ID             int64                   `json:"id,omitempty"`
	Key            string                  `json:"key"`
	Version        int                     `json:"version"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	ResponseSchema map[string]any          `json:"response_schema"`
	Checklist      ReviewChecklistSnapshot `json:"checklist"`
	Official       bool                    `json:"official"`
}

type ReviewContractLookup interface {
	ReviewContract(context.Context, string, int) (ReviewContractVersion, error)
}

type ReviewContractRef struct {
	Key     string `json:"key"`
	Version int    `json:"version"`
}

// ReviewTask is carried from Template to Model. Downstream cards cannot
// silently replace the contract selected by the pipeline author.
type ReviewTask struct {
	Prompt   string                `json:"prompt"`
	Contract ReviewContractVersion `json:"contract"`
}

type ReviewModelResponse struct {
	Content  string                `json:"content"`
	Contract ReviewContractVersion `json:"contract"`
}

type ValidatedReviewResponse struct {
	Value    any                   `json:"value"`
	Contract ReviewContractVersion `json:"contract"`
}

func BuiltInReviewContracts() []ReviewContractVersion {
	checklist := officialReviewChecklistSnapshot()
	contracts := []ReviewContractVersion{{
		Key:            OfficialPullRequestContractKey,
		Version:        1,
		Name:           "Review de pull request",
		Description:    "Contrato oficial de candidatos e checklist fechado para revisão de pull requests.",
		ResponseSchema: responseContractSchema("review.candidate-findings.v1"),
		Checklist:      checklist,
		Official:       true,
	}}
	names := map[string]string{
		"security":      "Segurança",
		"correctness":   "Corretude",
		"contracts":     "Contratos",
		"performance":   "Performance",
		"architecture":  "Arquitetura",
		"observability": "Observabilidade",
	}
	for _, check := range checklist.Items {
		name := names[check.Category]
		contracts = append(contracts, ReviewContractVersion{
			Key:            "review." + check.Category,
			Version:        1,
			Name:           "Review de " + name,
			Description:    check.Description,
			ResponseSchema: responseContractSchema("review.candidate-findings.v1"),
			Checklist: ReviewChecklistSnapshot{
				Key:     "official." + check.Category,
				Name:    name,
				Version: 1,
				Items:   []ReviewCheck{check},
			},
			Official: true,
		})
	}
	return contracts
}

func ReviewContractReference(config map[string]any) (ReviewContractRef, bool, error) {
	if config == nil {
		return ReviewContractRef{}, false, nil
	}
	keyValue, hasKey := config["review_contract_key"]
	versionValue, hasVersion := config["review_contract_version"]
	if !hasKey && !hasVersion {
		return ReviewContractRef{}, false, nil
	}
	key, keyOK := keyValue.(string)
	version, versionOK := integer(versionValue)
	if !hasKey || !hasVersion || !keyOK || strings.TrimSpace(key) == "" || len(key) > 128 || !versionOK || version < 1 {
		return ReviewContractRef{}, true, fmt.Errorf("review contract requires a key and positive version")
	}
	return ReviewContractRef{Key: strings.TrimSpace(key), Version: version}, true, nil
}

func ValidateReviewContract(contract ReviewContractVersion) error {
	if !checkIDPattern.MatchString(contract.Key) || contract.Version < 1 || strings.TrimSpace(contract.Name) == "" {
		return fmt.Errorf("review contract requires a valid key, name, and positive version")
	}
	if _, err := validateResponseSchema(contract.ResponseSchema); err != nil {
		return fmt.Errorf("review contract response schema: %w", err)
	}
	if err := validateReviewChecklist(contract.Checklist); err != nil {
		return fmt.Errorf("review contract checklist: %w", err)
	}
	return nil
}

func cloneReviewContract(contract ReviewContractVersion) (ReviewContractVersion, error) {
	payload, err := json.Marshal(contract)
	if err != nil {
		return ReviewContractVersion{}, err
	}
	var clone ReviewContractVersion
	if err = json.Unmarshal(payload, &clone); err != nil {
		return ReviewContractVersion{}, err
	}
	return clone, nil
}

func contractConfigured(contract ReviewContractVersion) bool {
	return strings.TrimSpace(contract.Key) != "" && contract.Version > 0
}
