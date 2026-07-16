package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func plannerPrompt(input Input) string {
	return fixedHeader("planner") + `
Regras da etapa: produza um plano global do PR em JSON. Agrupe arquivos por relacao funcional/semantica. Use somente arquivos listados. Cada arquivo deve aparecer exatamente uma vez.
Schema: {"pr_summary":"string","risk_level":"baixo|medio|alto|critico","risk_areas":["string"],"groups":[{"id":"group-1","purpose":"string","files":["path"],"relevant_stacks":["string"],"risk_level":"baixo|medio|alto|critico","review_focus":["string"]}],"assumptions":["string"]}
Contexto do PR:
` + inputContext(input, true)
}

func reviewerPrompt(input Input, plan ReviewPlan, group ReviewGroup, stackRules string) string {
	return fixedHeader("reviewer") + `
Regras da etapa: revise somente este grupo. Encontre defeitos concretos introduzidos ou expostos pelo PR. Nao reporte estilo, preferencias, duplicacoes, hipoteticos sem cenario concreto ou codigo preexistente fora do escopo.
Para cada achado cite arquivo e linha alterada, evidência do diff, cenario de falha, motivo de ter sido introduzido pelo PR, severidade e confianca 0..1.
Nao reporte como erro apenas porque "o diff nao mostra" uma referencia/contrato/validacao. Ausencia de evidencia no diff deve gerar rejeicao do achado, salvo se o proprio diff mostrar antes/depois concreto que quebra o contrato.
Use category com um destes tipos quando aplicavel: performance, contrato, sintaxe, semantica, validacao, seguranca, dados, teste.
Schema: {"group_id":"string","reviewed_files":["path"],"findings":[{"id":"string","file":"path","line":1,"end_line":1,"severity":"baixa|media|alta|critica","category":"correctness|security|performance|reliability|maintainability|test","confidence":0.9,"title":"string","decision_reason":"string","comment":"string","evidence":"string","failure_scenario":"string","suggested_fix":"string","introduced_by_pr":true}],"review_summary":"string"}
Regras da stack relevantes:
` + strings.TrimSpace(stackRules) + `
Resumo global do PR: ` + plan.PRSummary + `
Objetivo do grupo: ` + group.Purpose + `
Focos de risco: ` + strings.Join(group.ReviewFocus, ", ") + `
Arquivos do grupo: ` + strings.Join(group.Files, ", ") + "\nDiff do grupo:\n" + "```diff\n" + formatFilesDiff(selectFiles(input.Files, group.Files)) + "\n```"
}

func consolidatorPrompt(input Input, plan ReviewPlan, reviews []GroupReview, failedGroups []string) string {
	reviewsJSON, _ := json.Marshal(reviews)
	return fixedHeader("consolidator") + `
Regras da etapa: consolide criticamente os achados. Remova duplicados, una mesma causa-raiz, descarte estilo/sem evidencia/fora do escopo, ajuste severidade apenas com justificativa e preserve source_finding_ids.
Descarte qualquer achado cujo motivo central seja apenas "o diff nao mostra" uma referencia/contrato/validacao. Falta de contexto no diff nao e erro do PR.
Schema: {"pr_summary":"string","overall_risk":"baixo|medio|alto|critico","findings":[{"id":"consolidated-1","source_finding_ids":["id"],"file":"path","line":1,"end_line":1,"severity":"baixa|media|alta|critica","category":"string","confidence":0.9,"title":"string","decision_reason":"string","comment":"string","evidence":"string","failure_scenario":"string","suggested_fix":"string","introduced_by_pr":true}],"discarded_findings":[{"source_finding_id":"id","reason":"string"}]}
Resumo do PR: ` + plan.PRSummary + `
Arquivos: ` + strings.Join(sortedFilePaths(input.Files), ", ") + `
Grupos com falha: ` + strings.Join(failedGroups, ", ") + `
Achados por grupo em JSON:
` + string(reviewsJSON)
}

func verifierPrompt(input Input, consolidated ConsolidatedReview) string {
	findingsJSON, _ := json.Marshal(consolidated.Findings)
	return fixedHeader("verifier") + `
Regras da etapa: voce e um segundo revisor adversarial. Tente provar que cada achado esta incorreto, exagerado, duplicado, fora do escopo ou sem evidencia. Confirme somente com evidencia no diff/contexto, linha compativel, cenario concreto e severidade proporcional.
Rejeite achado que dependa de "o diff nao mostra" uma referencia/contrato/validacao. Isso e ausencia de contexto, nao defeito confirmado.
Use status confirmed, rejected ou adjusted.
Schema: {"results":[{"finding_id":"consolidated-1","status":"confirmed|rejected|adjusted","confidence":0.9,"verification_reason":"string","adjusted_finding":null}]}
Resumo consolidado: ` + consolidated.PRSummary + `
Achados consolidados:
` + string(findingsJSON) + "\nDiff disponivel para verificacao:\n" + "```diff\n" + formatFilesDiff(input.Files) + "\n```"
}

func formatterPrompt(consolidated ConsolidatedReview, findings []ReviewFinding, metadata Metadata) string {
	findingsJSON, _ := json.Marshal(findings)
	metadataJSON, _ := json.Marshal(metadata)
	return fixedHeader("formatter") + `
Regras da etapa: nao descubra novos bugs. Apenas transforme achados confirmados no JSON final. Preserve os campos obrigatorios e use metadata opcional. Se nao houver achados, aprove somente quando partial_review=false.
Nao inclua comentario cujo decision_reason dependa de "o diff nao mostra"; falta de contexto no diff nao e erro.
Escreva comment de forma direta e tao facil de entender quanto decision_reason: explique a consequencia pratica e o ajuste esperado, sem frases vagas.
Use type com um destes tipos quando aplicavel: performance, contrato, sintaxe, semantica, validacao, seguranca, dados, teste.
Schema: {"comments":[{"file":"path","line":1,"severity":"baixa|media|alta|critica","type":"performance|contrato|sintaxe|semantica|validacao|seguranca|dados|teste","decision_reason":"string","comment":"string"}],"final_review":{"gitea_event":"APPROVED|REQUEST_CHANGES|COMMENT","status":"aprovado|reprovado|comentado|parcial","summary":"string","observations":"string"},"metadata":{}}
Resumo consolidado: ` + consolidated.PRSummary + `
Achados aprovados:
` + string(findingsJSON) + `
Metadata:
` + string(metadataJSON)
}

func fixedHeader(stage string) string {
	return fmt.Sprintf("Voce e o ForgeReview pipeline v2, etapa %s. Responda exclusivamente JSON valido, sem Markdown fora de campos string, sem texto antes ou depois. Use somente dados fornecidos. Nao invente arquivos, APIs ou regras de negocio.\n", stage)
}

func inputContext(input Input, includeDiff bool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("owner=%s repo=%s pr=%d autor=%s\n", input.Owner, input.Repository, input.PullRequestNumber, input.Author))
	b.WriteString(fmt.Sprintf("titulo=%s\ndescricao=%s\nbase=%s head=%s\n", input.Title, input.Description, input.BaseBranch, input.HeadBranch))
	b.WriteString("stacks=" + strings.Join(input.Stacks, ", ") + "\n")
	for _, file := range input.Files {
		b.WriteString(fmt.Sprintf("- %s additions=%d deletions=%d\n", file.Path, file.Additions, file.Deletions))
	}
	if includeDiff {
		b.WriteString("Diff:\n```diff\n")
		b.WriteString(formatFilesDiff(input.Files))
		b.WriteString("\n```\n")
	}
	return b.String()
}
