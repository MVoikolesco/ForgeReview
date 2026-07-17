package pipeline

import "strings"

func validPlan(plan ReviewPlan, input Input) bool {
	if len(plan.Groups) == 0 {
		return false
	}
	existing := map[string]bool{}
	for _, file := range input.Files {
		existing[file.Path] = true
	}
	seen := map[string]bool{}
	for _, group := range plan.Groups {
		if strings.TrimSpace(group.ID) == "" || len(group.Files) == 0 {
			return false
		}
		for _, path := range group.Files {
			if !existing[path] || seen[path] {
				return false
			}
			seen[path] = true
		}
	}
	return len(seen) == len(input.Files)
}

func normalizePlan(plan *ReviewPlan) {
	plan.RiskLevel = normalizeRisk(plan.RiskLevel)
	for i := range plan.Groups {
		plan.Groups[i].RiskLevel = normalizeRisk(plan.Groups[i].RiskLevel)
		if plan.Groups[i].ID == "" {
			plan.Groups[i].ID = "group-" + string(rune(i+1))
		}
	}
}

func validateFindings(findings []ReviewFinding, input Input, groupID string) []ReviewFinding {
	byPath := filesByPath(input.Files)
	valid := make([]ReviewFinding, 0, len(findings))
	seen := map[string]bool{}
	for _, finding := range findings {
		if groupID != "" {
			finding.SourceGroupID = groupID
		}
		finding.File = strings.TrimSpace(finding.File)
		file, ok := byPath[finding.File]
		if !ok || !validAddedLine(file, finding.Line) {
			continue
		}
		finding.Severity = normalizeSeverity(finding.Severity)
		if finding.Severity == "" {
			continue
		}
		finding.Confidence = clampConfidence(finding.Confidence)
		if strings.TrimSpace(finding.ID) == "" || strings.TrimSpace(finding.DecisionReason) == "" || strings.TrimSpace(finding.Comment) == "" || !finding.IntroducedByPR {
			continue
		}
		if unsupportedByDiffOnly(finding.DecisionReason) || unsupportedImportDivergence(finding, input) {
			continue
		}
		key := dedupeKey(finding)
		if seen[key] {
			continue
		}
		seen[key] = true
		valid = append(valid, finding)
	}
	return valid
}

func unsupportedImportDivergence(finding ReviewFinding, input Input) bool {
	text := importDivergenceText(finding)
	if !isImportDivergenceFinding(finding) {
		return false
	}
	for _, phrase := range []string{"nao existe", "não existe", "ausente", "nao encontrado", "não encontrado", "nao declarado", "não declarado", "fora do diff", "diff nao mostra", "diff não mostra", "sem contexto"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	file, ok := filesByPath(input.Files)[finding.File]
	if !ok {
		return true
	}
	return !evidenceProvesChangedImportUseMismatch(finding.Evidence, file.Patch)
}

func isImportDivergenceFinding(finding ReviewFinding) bool {
	return strings.Contains(importDivergenceText(finding), "import") || strings.Contains(importDivergenceText(finding), "alias")
}

func importDivergenceText(finding ReviewFinding) string {
	return strings.ToLower(strings.Join([]string{finding.Category, finding.Title, finding.DecisionReason, finding.Comment, finding.Evidence}, " "))
}

func evidenceProvesChangedImportUseMismatch(evidence, patch string) bool {
	evidence = strings.ToLower(evidence)
	if strings.TrimSpace(evidence) == "" {
		return false
	}
	hasImport, hasUse := false, false
	for _, line := range strings.Split(patch, "\n") {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		changed := strings.TrimSpace(line[1:])
		if changed == "" || !strings.Contains(evidence, strings.ToLower(changed)) {
			continue
		}
		lower := strings.ToLower(changed)
		if strings.Contains(lower, "import ") || strings.HasPrefix(lower, "from ") || strings.Contains(lower, "require(") {
			hasImport = true
			continue
		}
		if strings.Contains(changed, ".") || strings.Contains(changed, "(") {
			hasUse = true
		}
	}
	return hasImport && hasUse
}

func filterExistingPaths(values []string, allowed []string) []string {
	allow := map[string]bool{}
	for _, value := range allowed {
		allow[value] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		if allow[value] && !seen[value] {
			out = append(out, value)
			seen[value] = true
		}
	}
	return out
}

func filterByConfidence(findings []ReviewFinding, minimum float64) []ReviewFinding {
	var out []ReviewFinding
	for _, finding := range findings {
		if finding.Confidence >= minimum {
			out = append(out, finding)
		}
	}
	return out
}
