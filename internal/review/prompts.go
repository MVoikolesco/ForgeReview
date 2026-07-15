package review

import (
	"fmt"
	"strings"
)

type PartialReview struct {
	Block   ReviewBlock
	Content string
	Failed  bool
	Error   string
}

const partialResponseTemplate = `Responda agora somente com este modelo, sem cabecalho extra:
STATUS: aprovado|aprovado_com_observacao|reprovado|comentario
BLOCO: N/M
RESUMO: uma frase objetiva
ACHADOS_CONCRETOS:
- nenhum

CONTRATOS_DECLARADOS:
- nenhum

REFERENCIAS_A_VERIFICAR:
- nenhum

REGRAS_DE_VALIDACAO:
- nenhum

OBSERVACOES:
- nenhum

Se houver achados concretos, use:
ACHADOS_CONCRETOS:
- SEVERIDADE: alta|media|baixa
  ARQUIVO: caminho
  LINHA_REFERENCIA: numero da linha nova quando possivel, ou 0
  TRECHO_REFERENCIA: trecho curto do diff que ancora o comentario
  TITULO: titulo curto
  IMPACTO: consequencia concreta em uma frase
  CORRECAO: ajuste recomendado em uma frase
  COMENTARIO_PR: comentario curto e acionavel para publicar na linha referente do PR

Use CONTRATOS_DECLARADOS para metodos, funcoes, props, configs, envs, rotas, eventos ou regras que este bloco declara.
Use REFERENCIAS_A_VERIFICAR para chamadas, imports, props obrigatorias, configs, envs, rotas ou eventos que dependem de outro arquivo.
Use REGRAS_DE_VALIDACAO para campos obrigatorios, defaults, permissoes, sanitizacao e regras que possam divergir entre camadas; quando houver mudanca, escreva antes => depois.
Nao reprove por uma referencia que ainda depende de outro arquivo; registre a pendencia para cruzamento final.`

func finalResponseTemplate() string {
	return fmt.Sprintf(`A ultima resposta deve conter exclusivamente um objeto JSON valido, sem Markdown, sem bloco de codigo e sem texto antes ou depois.
Valide internamente a estrutura antes de responder e use um parser JSON estrito.
As propriedades de primeiro nivel devem ser exatamente comments e final_review; rejeite qualquer propriedade desconhecida.
Todos os campos abaixo sao obrigatorios. comments deve ser um array, inclusive quando nao houver problemas.
Schema esperado:
{"comments":[{"file":"string nao vazia","line":"inteiro positivo","severity":"%s","decision_reason":"string nao vazia","comment":"string nao vazia"}],"final_review":{"gitea_event":"%s","status":"%s","summary":"string nao vazia","observations":"string"}}
Use somente os valores listados de severity, status e gitea_event. Retorne a resposta completa.`,
		strings.Join(AllowedSeverities(), "|"), strings.Join(AllowedGiteaEvents(), "|"), strings.Join(AllowedStatuses(), "|"))
}

func BuildPartialReviewPrompt(block ReviewBlock, resolvedPrompt string) string {
	return BuildPartialReviewPromptWithMemory(block, resolvedPrompt, nil)
}

func BuildPartialReviewPromptWithMemory(block ReviewBlock, resolvedPrompt string, previousReviews []PartialReview) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(resolvedPrompt))
	if memory := buildAccumulatedReviewMemory(previousReviews); memory != "" {
		builder.WriteString("\n\n## Memoria tecnica acumulada\n")
		builder.WriteString("Use estas anotacoes apenas para cruzar contratos entre arquivos. Nao transforme pendencia em achado sem evidencia direta no bloco atual ou nos reviews parciais.\n\n")
		builder.WriteString(memory)
	}
	if signals := buildRegressionSignals(block.Content); signals != "" {
		builder.WriteString("\n\n## Sinais automaticos de regressao\n")
		builder.WriteString("Use estes sinais como checklist. Eles nao sao achados por si so; confirme no diff antes de reprovar.\n\n")
		builder.WriteString(signals)
	}

	builder.WriteString(fmt.Sprintf(`

## Bloco %d/%d

Este e o bloco %d de %d de um Pull Request.
Arquivos neste bloco: %s

Diff:
`+"```diff\n%s\n```\n\n%s", block.Index, block.Total, block.Index, block.Total, strings.Join(block.Files, ", "), block.Content, partialResponseTemplate))

	return builder.String()
}

func buildAccumulatedReviewMemory(reviews []PartialReview) string {
	var builder strings.Builder
	for _, partial := range reviews {
		if partial.Failed {
			continue
		}

		memory := extractReviewMemory(partial.Content)
		if memory == "" {
			continue
		}

		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("===== MEMORIA BLOCO %d/%d arquivos=%s =====\n", partial.Block.Index, partial.Block.Total, strings.Join(partial.Block.Files, ", ")))
		builder.WriteString(memory)
		if !strings.HasSuffix(memory, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("===== FIM MEMORIA BLOCO %d/%d =====\n", partial.Block.Index, partial.Block.Total))
	}

	return strings.TrimSpace(builder.String())
}

func extractReviewMemory(content string) string {
	targetSections := map[string]struct{}{
		"CONTRATOS_DECLARADOS:":    {},
		"REFERENCIAS_A_VERIFICAR:": {},
		"REGRAS_DE_VALIDACAO:":     {},
	}
	knownSections := map[string]struct{}{
		"STATUS:":                  {},
		"BLOCO:":                   {},
		"RESUMO:":                  {},
		"ACHADOS:":                 {},
		"ACHADOS_CONCRETOS:":       {},
		"CONTRATOS_DECLARADOS:":    {},
		"REFERENCIAS_A_VERIFICAR:": {},
		"REGRAS_DE_VALIDACAO:":     {},
		"OBSERVACOES:":             {},
	}

	type section struct {
		header string
		lines  []string
	}

	var sections []section
	currentIndex := -1
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if _, ok := targetSections[trimmed]; ok {
			sections = append(sections, section{header: trimmed})
			currentIndex = len(sections) - 1
			continue
		}

		if _, ok := knownSections[trimmed]; ok {
			currentIndex = -1
			continue
		}

		if currentIndex >= 0 {
			sections[currentIndex].lines = append(sections[currentIndex].lines, line)
		}
	}

	var builder strings.Builder
	for _, section := range sections {
		if !hasUsefulMemoryLines(section.lines) {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(section.header)
		builder.WriteByte('\n')
		for _, line := range section.lines {
			builder.WriteString(line)
			builder.WriteByte('\n')
		}
	}

	return strings.TrimSpace(builder.String())
}

func hasUsefulMemoryLines(lines []string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && trimmed != "- nenhum" {
			return true
		}
	}

	return false
}

func BuildFinalReviewPrompt(reviews []PartialReview, resolvedPrompt string) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(resolvedPrompt))
	if memory := buildAccumulatedReviewMemory(reviews); memory != "" {
		builder.WriteString("\n\n## Memoria tecnica consolidada para cruzamento\n")
		builder.WriteString("Antes do veredito, cruze declaracoes, referencias e regras abaixo. So gere achado quando houver evidencia direta de incompatibilidade entre blocos ou em um review parcial.\n\n")
		builder.WriteString(memory)
	}
	if signals := buildConsolidatedRegressionSignals(reviews); signals != "" {
		builder.WriteString("\n\n## Sinais automaticos de regressao consolidados\n")
		builder.WriteString("Use estes sinais para conferir se algum review parcial deixou passar regressao concreta. Nao gere achado sem evidencia do diff, memoria ou review parcial.\n\n")
		builder.WriteString(signals)
	}

	builder.WriteString(fmt.Sprintf("\n\nQuantidade de blocos: %d", len(reviews)))
	failedCount := countFailedReviews(reviews)
	builder.WriteString(fmt.Sprintf("\nBlocos com falha: %d", failedCount))
	if failedCount > 0 {
		builder.WriteString("\n\nA analise e parcial. O veredito final nao pode ser aprovado.")
		builder.WriteString("\nBlocos que falharam:")
		for _, partial := range reviews {
			if partial.Failed {
				builder.WriteString(fmt.Sprintf("\n- Bloco %d/%d arquivos=%s", partial.Block.Index, partial.Block.Total, strings.Join(partial.Block.Files, ", ")))
				if partial.Error != "" {
					builder.WriteString(fmt.Sprintf(" erro=%s", partial.Error))
				}
			}
		}
	}
	builder.WriteString("\n\nReviews parciais:\n")

	for _, partial := range reviews {
		builder.WriteString(fmt.Sprintf("\n===== REVIEW PARCIAL BLOCO %d/%d arquivos=%s =====\n", partial.Block.Index, partial.Block.Total, strings.Join(partial.Block.Files, ", ")))
		if partial.Failed {
			builder.WriteString("STATUS: FALHA AO REVISAR ESTE BLOCO\n")
			if partial.Error != "" {
				builder.WriteString("ERRO: ")
				builder.WriteString(partial.Error)
				builder.WriteByte('\n')
			}
		}
		builder.WriteString(partial.Content)
		if !strings.HasSuffix(partial.Content, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("===== FIM REVIEW PARCIAL BLOCO %d/%d =====\n", partial.Block.Index, partial.Block.Total))
	}

	builder.WriteString("\n")
	builder.WriteString(finalResponseTemplate())
	builder.WriteByte('\n')

	return builder.String()
}

func BasePromptsLog(partialPrompt string, finalPrompt string) string {
	return "===== PROMPT BASE REVIEW PARCIAL =====\n" +
		strings.TrimSpace(partialPrompt) +
		"\n\n===== PROMPT BASE REVIEW FINAL =====\n" +
		strings.TrimSpace(finalPrompt) +
		"\n"
}

func countFailedReviews(reviews []PartialReview) int {
	total := 0
	for _, partial := range reviews {
		if partial.Failed {
			total++
		}
	}

	return total
}

func buildConsolidatedRegressionSignals(reviews []PartialReview) string {
	var builder strings.Builder
	for _, partial := range reviews {
		if partial.Failed {
			continue
		}

		signals := buildRegressionSignals(partial.Block.Content)
		if signals == "" {
			continue
		}

		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("===== SINAIS BLOCO %d/%d arquivos=%s =====\n", partial.Block.Index, partial.Block.Total, strings.Join(partial.Block.Files, ", ")))
		builder.WriteString(signals)
		if !strings.HasSuffix(signals, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("===== FIM SINAIS BLOCO %d/%d =====\n", partial.Block.Index, partial.Block.Total))
	}

	return strings.TrimSpace(builder.String())
}

func buildRegressionSignals(diffContent string) string {
	const maxSignals = 12

	var signals []string
	seen := map[string]struct{}{}
	addSignal := func(category string, code string) {
		code = compactCodeLine(code)
		if code == "" {
			return
		}

		signal := fmt.Sprintf("- %s: `%s`", category, code)
		if _, ok := seen[signal]; ok {
			return
		}

		seen[signal] = struct{}{}
		if len(signals) < maxSignals {
			signals = append(signals, signal)
		}
	}

	for _, line := range strings.Split(diffContent, "\n") {
		if len(line) < 2 || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}

		prefix := line[0]
		if prefix != '+' && prefix != '-' {
			continue
		}

		code := strings.TrimSpace(line[1:])
		lower := strings.ToLower(code)
		switch prefix {
		case '-':
			if containsAny(lower, "empty(", "isset(", "required", "validator", "validate", "permission", "authorize", "auth", "sanitize", "escape") {
				addSignal("validacao/permissao/sanitizacao removida", code)
			}
			if containsAny(lower, "random_bytes", "random_int", "password_hash", "hash_equals", "csrf", "nonce") {
				addSignal("seguranca ou segredo removido", code)
			}
			if containsAny(lower, "invalidate", "revoke", "logout", "cleanup", "close(", "rollback", "commit") {
				addSignal("invalidacao ou cleanup removido", code)
			}
			if looksLikeContractLine(lower) {
				addSignal("contrato/chamada removido", code)
			}
		case '+':
			if containsAny(lower, "md5(", "sha1(", "time()", "microtime(", "rand(", "mt_rand(") {
				addSignal("geracao previsivel adicionada", code)
			}
			if containsAny(lower, "status") && containsAny(lower, "=> 200", ": 200", "= 200", "(200", "http_200") {
				addSignal("codigo/status de sucesso em resposta", code)
			}
			if looksLikeContractLine(lower) {
				addSignal("contrato/chamada adicionado", code)
			}
		}
	}

	if len(signals) == 0 {
		return ""
	}

	return strings.Join(signals, "\n")
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}

	return false
}

func looksLikeContractLine(lower string) bool {
	return strings.Contains(lower, "function ") ||
		hasObjectMethodCall(lower) ||
		strings.Contains(lower, "::") ||
		strings.Contains(lower, "interface ") ||
		strings.Contains(lower, "implements ") ||
		strings.Contains(lower, "export ")
}

func hasObjectMethodCall(lower string) bool {
	searchFrom := 0
	for {
		arrowIndex := strings.Index(lower[searchFrom:], "->")
		if arrowIndex < 0 {
			return false
		}

		nameStart := searchFrom + arrowIndex + len("->")
		nameEnd := nameStart
		for nameEnd < len(lower) && isIdentifierByte(lower[nameEnd]) {
			nameEnd++
		}

		if nameEnd > nameStart && nameEnd < len(lower) && lower[nameEnd] == '(' {
			return true
		}

		searchFrom = nameStart
	}
}

func isIdentifierByte(value byte) bool {
	return (value >= 'a' && value <= 'z') ||
		(value >= '0' && value <= '9') ||
		value == '_'
}

func compactCodeLine(code string) string {
	code = strings.Join(strings.Fields(strings.TrimSpace(code)), " ")
	if len(code) <= 180 {
		return code
	}

	return strings.TrimSpace(code[:177]) + "..."
}
