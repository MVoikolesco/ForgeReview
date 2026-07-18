package pipeline

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gitea-agents/internal/diff"
)

func FallbackPlan(input Input, cfg Config, reason string) ReviewPlan {
	maxChars := cfg.MaxGroupChars
	if maxChars <= 0 {
		maxChars = 4000
	}
	maxFiles := cfg.MaxFilesPerGroup
	if maxFiles <= 0 {
		maxFiles = 2
	}

	files := append([]diff.ChangedFile(nil), input.Files...)
	sort.SliceStable(files, func(i, j int) bool {
		ki := groupAffinityKey(files[i].Path)
		kj := groupAffinityKey(files[j].Path)
		if ki == kj {
			return files[i].Path < files[j].Path
		}
		return ki < kj
	})

	var groups []ReviewGroup
	var current []diff.ChangedFile
	currentChars := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		paths := make([]string, 0, len(current))
		for _, file := range current {
			paths = append(paths, file.Path)
		}
		groups = append(groups, ReviewGroup{ID: fmt.Sprintf("group-%d", len(groups)+1), Purpose: "Grupo deterministico por afinidade de diretorio e nome", Files: paths, RiskLevel: "medio", ReviewFocus: []string{"bugs funcionais", "contratos entre arquivos", "validacao", "seguranca"}})
		current = nil
		currentChars = 0
	}

	for _, file := range files {
		fileChars := len(file.Patch)
		if len(current) > 0 && (len(current) >= maxFiles || currentChars+fileChars > maxChars) {
			flush()
		}
		current = append(current, file)
		currentChars += fileChars
	}
	flush()
	return ReviewPlan{PRSummary: "Plano gerado por fallback deterministico: " + reason, RiskLevel: "medio", RiskAreas: []string{"alteracoes do PR"}, Groups: groups, Assumptions: []string{"planner indisponivel ou retornou agrupamento invalido"}}
}

func groupAffinityKey(path string) string {
	dir := filepath.Dir(path)
	base := strings.ToLower(filepath.Base(path))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	for _, suffix := range []string{"controller", "model", "view", "service", "component", "test", "spec"} {
		base = strings.TrimSuffix(base, suffix)
	}
	return dir + "/" + base
}

func fallbackConsolidate(reviews []GroupReview) ConsolidatedReview {
	byKey := map[string]ReviewFinding{}
	for _, group := range reviews {
		for _, finding := range group.Findings {
			key := dedupeKey(finding)
			existing, ok := byKey[key]
			if !ok || severityRank(finding.Severity) > severityRank(existing.Severity) || finding.Confidence > existing.Confidence {
				if finding.SourceFindingIDs == nil {
					finding.SourceFindingIDs = []string{finding.ID}
				}
				finding.ID = fmt.Sprintf("consolidated-%d", len(byKey)+1)
				byKey[key] = finding
			}
		}
	}
	findings := make([]ReviewFinding, 0, len(byKey))
	for _, finding := range byKey {
		findings = append(findings, finding)
	}
	sort.SliceStable(findings, func(i, j int) bool {
		return findings[i].File < findings[j].File || (findings[i].File == findings[j].File && findings[i].Line < findings[j].Line)
	})
	return ConsolidatedReview{Findings: findings, PRSummary: "Review consolidado por fallback deterministico.", OverallRisk: highestRisk(findings)}
}

func dedupeKey(f ReviewFinding) string {
	return strings.ToLower(fmt.Sprintf("%s:%d:%s:%s", f.File, f.Line, f.Category, strings.Join(strings.Fields(f.Title), " ")))
}

func severityRank(s string) int {
	switch normalizeSeverity(s) {
	case "critica":
		return 4
	case "alta":
		return 3
	case "media":
		return 2
	case "baixa":
		return 1
	default:
		return 0
	}
}

func highestRisk(findings []ReviewFinding) string {
	best := "baixo"
	for _, finding := range findings {
		if severityRank(finding.Severity) > severityRank(best) {
			best = finding.Severity
		}
	}
	return best
}
