package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func plannerPrompt(input Input) string {
	return renderPrompt(input.Prompts.TechnicalReview, "planner", dynamicBlock("pr", inputContext(input)))
}

func reviewerPrompt(input Input, plan ReviewPlan, group ReviewGroup) string {
	return renderPrompt(strings.Join([]string{input.Prompts.TechnicalReview, input.Prompts.SecurityPerformance, input.Prompts.ImportDivergence}, "\n\n"), "reviewer",
		dynamicBlock("resumo_pr", plan.PRSummary), dynamicBlock("grupo", group), dynamicBlock("diff_canonico", formatFilesDiff(input.Files)))
}

func consolidatorPrompt(input Input, plan ReviewPlan, reviews []GroupReview, failedGroups []string) string {
	reviewsJSON, _ := json.Marshal(reviews)
	return renderPrompt(strings.Join([]string{input.Prompts.TechnicalReview, input.Prompts.SecurityPerformance, input.Prompts.ImportDivergence}, "\n\n"), "consolidator",
		dynamicBlock("plano", plan), dynamicBlock("grupos_com_falha", failedGroups), dynamicBlock("achados_por_grupo", string(reviewsJSON)), dynamicBlock("diff_canonico", formatFilesDiff(input.Files)))
}

func verifierPrompt(input Input, consolidated ConsolidatedReview) string {
	findingsJSON, _ := json.Marshal(consolidated.Findings)
	return renderPrompt(strings.Join([]string{input.Prompts.TechnicalReview, input.Prompts.SecurityPerformance, input.Prompts.ImportDivergence}, "\n\n"), "verifier",
		dynamicBlock("resumo_consolidado", consolidated.PRSummary), dynamicBlock("achados_consolidados", string(findingsJSON)), dynamicBlock("diff_canonico", formatFilesDiff(input.Files)))
}

func formatterPrompt(input Input, consolidated ConsolidatedReview, findings []ReviewFinding, metadata Metadata) string {
	findingsJSON, _ := json.Marshal(findings)
	metadataJSON, _ := json.Marshal(metadata)
	return renderPrompt(input.Prompts.FinalResponse, "formatter",
		dynamicBlock("resumo_consolidado", consolidated.PRSummary), dynamicBlock("achados_aprovados", string(findingsJSON)), dynamicBlock("metadata", string(metadataJSON)), dynamicBlock("diff_canonico", formatFilesDiff(input.Files)))
}

func renderPrompt(instructions, stage string, blocks ...string) string {
	return strings.TrimSpace(instructions) + "\n\n<etapa>" + stage + "</etapa>\n" + strings.Join(blocks, "\n")
}

func dynamicBlock(name string, value any) string {
	return fmt.Sprintf("<%s>\n%v\n</%s>", name, value, name)
}

func inputContext(input Input) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("owner=%s repo=%s pr=%d autor=%s\n", input.Owner, input.Repository, input.PullRequestNumber, input.Author))
	b.WriteString(fmt.Sprintf("titulo=%s\ndescricao=%s\nbase=%s head=%s\n", input.Title, input.Description, input.BaseBranch, input.HeadBranch))
	for _, file := range input.Files {
		b.WriteString(fmt.Sprintf("- %s additions=%d deletions=%d\n", file.Path, file.Additions, file.Deletions))
	}
	b.WriteString(formatFilesDiff(input.Files))
	return b.String()
}
