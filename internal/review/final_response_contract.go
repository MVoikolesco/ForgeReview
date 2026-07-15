package review

import (
	"encoding/json"
	"fmt"
	"strings"
)

var allowedSeverities = []string{"alta", "media", "baixa"}
var allowedStatuses = []string{"aprovado", "aprovado_com_observacao", "reprovado", "comentario"}

type finalResponseJSON struct {
	Comments    []finalCommentJSON `json:"comments"`
	FinalReview finalReviewJSON    `json:"final_review"`
}

type finalCommentJSON struct {
	File           string `json:"file"`
	Line           int    `json:"line"`
	Severity       string `json:"severity"`
	DecisionReason string `json:"decision_reason"`
	Comment        string `json:"comment"`
}

type finalReviewJSON struct {
	GiteaEvent   string `json:"gitea_event"`
	Status       string `json:"status"`
	Summary      string `json:"summary"`
	Observations string `json:"observations"`
}

// ValidateFinalReviewResponse parses and validates the complete final contract.
// It rejects wrappers, trailing content, unknown properties and partial objects.
func ValidateFinalReviewResponse(content string) (FinalReview, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &top); err != nil {
		return FinalReview{}, fmt.Errorf("JSON invalido")
	}
	if top == nil {
		return FinalReview{}, fmt.Errorf("resposta deve ser um objeto JSON")
	}
	if len(top) != 2 {
		return FinalReview{}, fmt.Errorf("propriedades de primeiro nivel devem ser exatamente comments e final_review")
	}
	for key := range top {
		if key != "comments" && key != "final_review" {
			return FinalReview{}, fmt.Errorf("propriedade desconhecida: %s", key)
		}
	}

	var response finalResponseJSON
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return FinalReview{}, fmt.Errorf("estrutura JSON invalida")
	}

	errors := make([]string, 0)
	if _, ok := top["comments"]; !ok {
		errors = append(errors, "campo comments ausente")
	}
	if _, ok := top["final_review"]; !ok {
		errors = append(errors, "campo final_review ausente")
	}
	if raw, ok := top["comments"]; ok && string(raw) == "null" {
		errors = append(errors, "comments deve ser um array")
	}
	if raw, ok := top["final_review"]; ok && string(raw) == "null" {
		errors = append(errors, "final_review deve ser um objeto")
	}
	if raw, ok := top["final_review"]; ok && string(raw) != "null" {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err == nil {
			for _, field := range []string{"gitea_event", "status", "summary", "observations"} {
				value, exists := fields[field]
				if !exists {
					errors = append(errors, "final_review."+field+" ausente")
				} else if string(value) == "null" {
					errors = append(errors, "final_review."+field+" nao pode ser null")
				}
			}
		}
	}

	for index, comment := range response.Comments {
		prefix := fmt.Sprintf("comments[%d]", index)
		if strings.TrimSpace(comment.File) == "" {
			errors = append(errors, prefix+".file vazio")
		}
		if comment.Line <= 0 {
			errors = append(errors, prefix+".line deve ser inteiro positivo")
		}
		if !contains(allowedSeverities, comment.Severity) {
			errors = append(errors, prefix+".severity nao permitida")
		}
		if strings.TrimSpace(comment.DecisionReason) == "" {
			errors = append(errors, prefix+".decision_reason vazio")
		}
		if strings.TrimSpace(comment.Comment) == "" {
			errors = append(errors, prefix+".comment vazio")
		}
	}
	if !contains([]string{GiteaEventApproved, GiteaEventComment, GiteaEventRequestChanges}, response.FinalReview.GiteaEvent) {
		errors = append(errors, "final_review.gitea_event nao permitido")
	}
	if !contains(allowedStatuses, response.FinalReview.Status) {
		errors = append(errors, "final_review.status nao permitido")
	}
	if strings.TrimSpace(response.FinalReview.Summary) == "" {
		errors = append(errors, "final_review.summary vazio")
	}
	if len(errors) > 0 {
		return FinalReview{}, fmt.Errorf("contrato de review invalido: %s", strings.Join(errors, "; "))
	}

	parsed := FinalReview{
		Event: response.FinalReview.GiteaEvent, Status: response.FinalReview.Status,
		Summary: response.FinalReview.Summary, Observations: response.FinalReview.Observations,
		Raw: strings.TrimSpace(content), Structured: true,
	}
	for _, comment := range response.Comments {
		parsed.InlineComments = append(parsed.InlineComments, InlineComment{
			Severity: comment.Severity, Path: comment.File, NewPosition: comment.Line,
			DecisionReason: comment.DecisionReason, Body: comment.Comment,
		})
	}
	return parsed, nil
}

func AllowedSeverities() []string { return append([]string(nil), allowedSeverities...) }

func AllowedStatuses() []string { return append([]string(nil), allowedStatuses...) }

func AllowedGiteaEvents() []string {
	return []string{GiteaEventApproved, GiteaEventRequestChanges, GiteaEventComment}
}

func contains(values []string, value string) bool {
	for _, allowed := range values {
		if value == allowed {
			return true
		}
	}
	return false
}
