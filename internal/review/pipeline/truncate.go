package pipeline

import (
	"fmt"
	"strings"

	"gitea-agents/internal/diff"
)

func truncateInputDiff(input Input, cfg Config) (Input, bool) {
	budgetTokens := cfg.MaxInputTokens
	if budgetTokens <= 0 && cfg.ContextWindow > 0 {
		budgetTokens = cfg.ContextWindow - cfg.SafetyMarginTokens - maxConfiguredOutput(cfg)
	}
	if budgetTokens <= 0 {
		return input, false
	}
	budgetChars := budgetTokens * 3
	if len(formatFilesDiff(input.Files)) <= budgetChars {
		return input, false
	}
	out := input
	out.Files = make([]diff.ChangedFile, 0, len(input.Files))
	used := 0
	truncated := false
	for _, file := range input.Files {
		if used >= budgetChars {
			truncated = true
			minimal := file
			minimal.Patch = fmt.Sprintf("diff truncado antes do arquivo path=%s por limite de contexto\n", file.Path)
			out.Files = append(out.Files, minimal)
			continue
		}
		remaining := budgetChars - used
		patch := file.Patch
		if len(patch) > remaining {
			patch = truncatePatchLines(file.Patch, remaining, file.Path)
			truncated = true
		}
		truncatedFile := file
		truncatedFile.Patch = patch
		out.Files = append(out.Files, truncatedFile)
		used += len(patch)
	}
	return out, truncated
}

func truncatePatchLines(patch string, maxChars int, path string) string {
	if maxChars <= 0 {
		return fmt.Sprintf("diff truncado path=%s por limite de contexto\n", path)
	}
	var b strings.Builder
	for _, line := range strings.Split(patch, "\n") {
		lineWithBreak := line + "\n"
		if b.Len()+len(lineWithBreak) > maxChars {
			break
		}
		b.WriteString(lineWithBreak)
	}
	if b.Len() == 0 {
		b.WriteString(fmt.Sprintf("diff truncado path=%s por limite de contexto\n", path))
	}
	if !strings.Contains(b.String(), "@@") && strings.Contains(patch, "@@") {
		b.WriteString("@@ hunk omitido por truncamento @@\n")
	}
	b.WriteString(fmt.Sprintf("[TRUNCADO: diff do arquivo %s foi cortado por limite de contexto]\n", path))
	return b.String()
}

func maxConfiguredOutput(cfg Config) int {
	max := cfg.PlannerMaxOutputTokens
	for _, value := range []int{cfg.ReviewerMaxOutputTokens, cfg.ConsolidatorMaxOutputTokens, cfg.VerifierMaxOutputTokens, cfg.FormatterMaxOutputTokens} {
		if value > max {
			max = value
		}
	}
	return max
}
