package workflow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	ContextDiff         = "diff"
	ContextFile         = "file"
	ContextSymbol       = "symbol"
	ContextDependencies = "dependencies"
	ContextTests        = "tests"
	ContextContracts    = "contracts"
	ContextRepository   = "repository"
)

var checkIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

type ReviewCheck struct {
	CheckID        string `json:"check_id"`
	Description    string `json:"description"`
	Category       string `json:"category"`
	MinimumContext string `json:"minimum_context"`
}

// ReviewChecklist is an immutable catalog item. ReviewChecklistSnapshot is the
// copy embedded in a workflow version so historical executions never drift.
type ReviewChecklist struct {
	Key         string        `json:"key"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Version     int           `json:"version"`
	Items       []ReviewCheck `json:"items"`
	Editable    bool          `json:"editable"`
}

type ReviewChecklistSnapshot struct {
	Key     string        `json:"key"`
	Name    string        `json:"name"`
	Version int           `json:"version"`
	Items   []ReviewCheck `json:"items"`
}

func ReviewChecklists() []ReviewChecklist {
	return []ReviewChecklist{{
		Key: "official.pull-request.v1", Name: "Review de pull request",
		Description: "Checklist fechada para segurança, corretude, contratos, performance, arquitetura e observabilidade.",
		Version:     1,
		Items: []ReviewCheck{
			{CheckID: "security.authorization", Description: "Verificar bypass de autenticação, autorização ou isolamento de dados introduzido pela alteração.", Category: "security", MinimumContext: ContextFile},
			{CheckID: "correctness.behavior", Description: "Verificar comportamento incorreto reproduzível no fluxo alterado, incluindo limites e tratamento de erro.", Category: "correctness", MinimumContext: ContextDiff},
			{CheckID: "contracts.compatibility", Description: "Verificar quebra concreta de contrato público, formato persistido, API ou integração.", Category: "contracts", MinimumContext: ContextContracts},
			{CheckID: "performance.hot_path", Description: "Verificar regressão material em caminho executado com frequência ou crescimento não limitado.", Category: "performance", MinimumContext: ContextSymbol},
			{CheckID: "architecture.boundaries", Description: "Verificar violação funcional de fronteira que cria acoplamento ou efeito externo indevido.", Category: "architecture", MinimumContext: ContextDependencies},
			{CheckID: "observability.failures", Description: "Verificar falha operacional nova que fica silenciosa ou perde sinal necessário para diagnóstico.", Category: "observability", MinimumContext: ContextFile},
		},
		Editable: false,
	}}
}

func officialReviewChecklistSnapshot() ReviewChecklistSnapshot {
	item := ReviewChecklists()[0]
	return ReviewChecklistSnapshot{Key: item.Key, Name: item.Name, Version: item.Version, Items: append([]ReviewCheck(nil), item.Items...)}
}

func reviewChecklistFromConfig(config map[string]any) (ReviewChecklistSnapshot, bool, error) {
	if config == nil {
		return ReviewChecklistSnapshot{}, false, nil
	}
	value, exists := config["review_checklist"]
	if !exists {
		return ReviewChecklistSnapshot{}, false, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return ReviewChecklistSnapshot{}, true, fmt.Errorf("review checklist is not valid JSON")
	}
	var checklist ReviewChecklistSnapshot
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&checklist); err != nil {
		return ReviewChecklistSnapshot{}, true, fmt.Errorf("review checklist is invalid: %w", err)
	}
	if err = validateReviewChecklist(checklist); err != nil {
		return ReviewChecklistSnapshot{}, true, err
	}
	return checklist, true, nil
}

func validateReviewChecklist(checklist ReviewChecklistSnapshot) error {
	if strings.TrimSpace(checklist.Key) == "" || strings.TrimSpace(checklist.Name) == "" || checklist.Version < 1 {
		return fmt.Errorf("review checklist requires key, name, and positive version")
	}
	if len(checklist.Items) == 0 || len(checklist.Items) > 64 {
		return fmt.Errorf("review checklist must contain between 1 and 64 checks")
	}
	contexts := map[string]bool{ContextDiff: true, ContextFile: true, ContextSymbol: true, ContextDependencies: true, ContextTests: true, ContextContracts: true, ContextRepository: true}
	categories := map[string]bool{"security": true, "correctness": true, "contracts": true, "performance": true, "architecture": true, "observability": true}
	seen := map[string]bool{}
	for _, item := range checklist.Items {
		if !checkIDPattern.MatchString(item.CheckID) || seen[item.CheckID] {
			return fmt.Errorf("review checklist contains invalid or duplicate check_id %q", item.CheckID)
		}
		if strings.TrimSpace(item.Description) == "" || len([]rune(item.Description)) > 500 {
			return fmt.Errorf("review checklist check %q requires a description of at most 500 characters", item.CheckID)
		}
		if !categories[item.Category] {
			return fmt.Errorf("review checklist check %q has invalid category %q", item.CheckID, item.Category)
		}
		if !contexts[item.MinimumContext] {
			return fmt.Errorf("review checklist check %q has invalid minimum_context %q", item.CheckID, item.MinimumContext)
		}
		seen[item.CheckID] = true
	}
	return nil
}

func checklistCheckIDs(config map[string]any) (map[string]bool, bool, error) {
	checklist, configured, err := reviewChecklistFromConfig(config)
	if err != nil || !configured {
		return nil, configured, err
	}
	ids := make(map[string]bool, len(checklist.Items))
	for _, item := range checklist.Items {
		ids[item.CheckID] = true
	}
	return ids, true, nil
}

func reviewPromptWithChecklist(prompt string, config map[string]any) (string, error) {
	checklist, configured, err := reviewChecklistFromConfig(config)
	if err != nil || !configured {
		return prompt, err
	}
	payload, err := json.Marshal(checklist)
	if err != nil {
		return "", fmt.Errorf("serialize review checklist: %w", err)
	}
	return strings.TrimSpace(prompt) + "\n\nChecklist fechada e versionada. Execute somente estes checks e use exatamente um check_id listado em cada candidato. Não crie checks adicionais. Se nenhum check produzir candidato sustentado, responda [].\n" + string(payload), nil
}
