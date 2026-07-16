package pipeline

import (
	"fmt"
	"strings"
)

func deterministicFinal(findings []ReviewFinding, meta Metadata, cfg Config, summary string) FinalResponse {
	comments := make([]FinalComment, 0, len(findings))
	for _, finding := range findings {
		comments = append(comments, FinalComment{File: finding.File, Line: finding.Line, Severity: finding.Severity, Type: finding.Category, DecisionReason: finding.DecisionReason, Comment: finding.Comment})
	}
	if strings.TrimSpace(summary) == "" {
		summary = "Review automatizado concluido."
	}
	response := FinalResponse{Comments: comments, FinalReview: FinalReview{Summary: summary}, Metadata: meta}
	return enforceDecision(response, findings, meta, cfg, summary)
}

func alignFinalCommentLines(response FinalResponse, findings []ReviewFinding) FinalResponse {
	if len(response.Comments) == 0 || len(findings) == 0 {
		return response
	}

	used := make([]bool, len(findings))
	for commentIndex := range response.Comments {
		comment := &response.Comments[commentIndex]
		findingIndex := findMatchingFindingIndex(*comment, findings, used)
		if findingIndex < 0 {
			continue
		}
		comment.Line = findings[findingIndex].Line
		used[findingIndex] = true
	}

	return response
}

func findMatchingFindingIndex(comment FinalComment, findings []ReviewFinding, used []bool) int {
	for index, finding := range findings {
		if used[index] || finding.File != comment.File {
			continue
		}
		if comment.Severity != "" && normalizeSeverity(comment.Severity) != normalizeSeverity(finding.Severity) {
			continue
		}
		return index
	}
	return -1
}

func enforceDecision(response FinalResponse, _ []ReviewFinding, meta Metadata, cfg Config, summary string) FinalResponse {
	response.Comments = normalizeFinalComments(response.Comments)
	if response.Comments == nil {
		response.Comments = []FinalComment{}
	}
	if response.FinalReview.Summary == "" {
		response.FinalReview.Summary = summary
	}
	if response.FinalReview.Summary == "" {
		response.FinalReview.Summary = "Review automatizado concluido."
	}
	if meta.PartialReview {
		response.FinalReview.GiteaEvent = cfg.PartialEvent
		if response.FinalReview.GiteaEvent == "" {
			response.FinalReview.GiteaEvent = "COMMENT"
		}
		response.FinalReview.Status = "parcial"
		obs := fmt.Sprintf("Analise parcial: %d grupo(s) falharam e parte dos arquivos pode nao ter sido revisada.", meta.FailedGroups)
		if response.FinalReview.Observations != "" {
			response.FinalReview.Observations += "\n\n" + obs
		} else {
			response.FinalReview.Observations = obs
		}
		return response
	}
	maxSeverity := highestCommentRisk(response.Comments)
	switch maxSeverity {
	case "critica", "alta":
		response.FinalReview.GiteaEvent = "REQUEST_CHANGES"
		response.FinalReview.Status = "reprovado"
	case "media":
		response.FinalReview.GiteaEvent = cfg.MediumSeverityEvent
		if response.FinalReview.GiteaEvent == "REQUEST_CHANGES" {
			response.FinalReview.Status = "reprovado"
		} else {
			response.FinalReview.Status = "comentado"
		}
	case "baixa":
		response.FinalReview.GiteaEvent = "COMMENT"
		response.FinalReview.Status = "comentado"
	default:
		response.FinalReview.GiteaEvent = "APPROVED"
		response.FinalReview.Status = "aprovado"
		if response.FinalReview.Observations == "" {
			response.FinalReview.Observations = "Nenhum problema relevante foi confirmado."
		}
	}
	return response
}

func highestCommentRisk(comments []FinalComment) string {
	findings := make([]ReviewFinding, 0, len(comments))
	for _, comment := range comments {
		findings = append(findings, ReviewFinding{Severity: comment.Severity})
	}
	return highestRisk(findings)
}

func normalizeFinalComments(comments []FinalComment) []FinalComment {
	var out []FinalComment
	seen := map[string]bool{}
	for _, comment := range comments {
		comment.Severity = normalizeSeverity(comment.Severity)
		comment.Type = normalizeCommentType(comment.Type, comment.DecisionReason+" "+comment.Comment)
		if comment.File == "" || comment.Line <= 0 || comment.Severity == "" || strings.TrimSpace(comment.DecisionReason) == "" || strings.TrimSpace(comment.Comment) == "" {
			continue
		}
		if unsupportedByDiffOnly(comment.DecisionReason) {
			continue
		}
		key := fmt.Sprintf("%s:%d:%s:%s", comment.File, comment.Line, comment.Severity, comment.Comment)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, comment)
	}
	return out
}

func unsupportedByDiffOnly(reason string) bool {
	lower := strings.ToLower(strings.TrimSpace(reason))
	if lower == "" || !strings.Contains(lower, "diff") {
		return false
	}
	return strings.Contains(lower, "nao mostra") || strings.Contains(lower, "não mostra") || strings.Contains(lower, "nao ha evidencia") || strings.Contains(lower, "não há evidência") || strings.Contains(lower, "sem evidencia") || strings.Contains(lower, "sem evidência")
}

func normalizeCommentType(value string, text string) string {
	n := strings.ToLower(strings.TrimSpace(value))
	n = strings.ReplaceAll(n, "á", "a")
	n = strings.ReplaceAll(n, "â", "a")
	n = strings.ReplaceAll(n, "ã", "a")
	n = strings.ReplaceAll(n, "é", "e")
	n = strings.ReplaceAll(n, "ê", "e")
	n = strings.ReplaceAll(n, "í", "i")
	n = strings.ReplaceAll(n, "ó", "o")
	n = strings.ReplaceAll(n, "ô", "o")
	n = strings.ReplaceAll(n, "õ", "o")
	n = strings.ReplaceAll(n, "ú", "u")
	n = strings.ReplaceAll(n, "ç", "c")
	switch n {
	case "performance":
		return "performance"
	case "security", "seguranca":
		return "seguranca"
	case "contract", "contracts", "contrato", "compatibility", "api":
		return "contrato"
	case "syntax", "sintaxe", "compile", "compilacao", "chamada", "import":
		return "sintaxe"
	case "semantic", "semantica", "semantics", "correctness", "logic", "logica", "reliability":
		return "semantica"
	case "validation", "validacao":
		return "validacao"
	case "data", "dados":
		return "dados"
	case "test", "tests", "teste", "testes":
		return "teste"
	}

	lowerText := strings.ToLower(text)
	switch {
	case strings.Contains(lowerText, "contrato") || strings.Contains(lowerText, "api") || strings.Contains(lowerText, "assinatura"):
		return "contrato"
	case strings.Contains(lowerText, "performance") || strings.Contains(lowerText, "n+1") || strings.Contains(lowerText, "lento"):
		return "performance"
	case strings.Contains(lowerText, "sintaxe") || strings.Contains(lowerText, "compila") || strings.Contains(lowerText, "metodo inexistente") || strings.Contains(lowerText, "import"):
		return "sintaxe"
	case strings.Contains(lowerText, "validacao") || strings.Contains(lowerText, "validação"):
		return "validacao"
	case strings.Contains(lowerText, "dados") || strings.Contains(lowerText, "perda"):
		return "dados"
	default:
		return "semantica"
	}
}
