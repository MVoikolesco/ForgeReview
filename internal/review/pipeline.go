package review

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// pipelineFile is the prepared representation used by every pipeline stage.
type pipelineFile struct{ Path, Patch string }
type pipelineGroup struct {
	ID          string   `json:"id"`
	Purpose     string   `json:"purpose"`
	Files       []string `json:"files"`
	RiskLevel   string   `json:"risk_level"`
	ReviewFocus []string `json:"review_focus"`
}
type pipelinePlan struct {
	Summary     string          `json:"pr_summary"`
	RiskLevel   string          `json:"risk_level"`
	RiskAreas   []string        `json:"risk_areas"`
	Groups      []pipelineGroup `json:"groups"`
	Assumptions []string        `json:"assumptions"`
}
type pipelineFinding struct {
	ID               string   `json:"id"`
	File             string   `json:"file"`
	Line             int      `json:"line"`
	Severity         string   `json:"severity"`
	Category         string   `json:"category"`
	Confidence       float64  `json:"confidence"`
	DecisionReason   string   `json:"decision_reason"`
	Comment          string   `json:"comment"`
	Title            string   `json:"title"`
	Evidence         string   `json:"evidence"`
	FailureScenario  string   `json:"failure_scenario"`
	SuggestedFix     string   `json:"suggested_fix"`
	SourceGroupID    string   `json:"source_group_id"`
	SourceStage      string   `json:"source_stage,omitempty"`
	SourceFindingIDs []string `json:"source_finding_ids"`
	Introduced       bool     `json:"introduced_by_pr"`
}
type pipelineGroupReview struct {
	GroupID       string            `json:"group_id"`
	ReviewedFiles []string          `json:"reviewed_files"`
	Findings      []pipelineFinding `json:"findings"`
	ReviewSummary string            `json:"review_summary"`
}
type pipelineConsolidated struct {
	Findings          []pipelineFinding `json:"findings"`
	Summary           string            `json:"pr_summary"`
	OverallRisk       string            `json:"overall_risk"`
	DiscardedFindings []struct {
		SourceFindingID string `json:"source_finding_id"`
		Reason          string `json:"reason"`
	} `json:"discarded_findings"`
}
type pipelineVerification struct {
	Results []struct {
		ID                 string           `json:"finding_id"`
		Status             string           `json:"status"`
		Confidence         float64          `json:"confidence"`
		VerificationReason string           `json:"verification_reason"`
		AdjustedFinding    *pipelineFinding `json:"adjusted_finding"`
	} `json:"results"`
}
type pipelineFormatted struct {
	Comments    []pipelineComment `json:"comments"`
	FinalReview FinalReview       `json:"final_review"`
	Metadata    map[string]any    `json:"metadata"`
}
type pipelineComment struct {
	File           string `json:"file"`
	Line           int    `json:"line"`
	Severity       string `json:"severity"`
	Type           string `json:"type"`
	DecisionReason string `json:"decision_reason"`
	Comment        string `json:"comment"`
}

// queueInput avoids coupling the pipeline mechanics to queue transport fields.
type queueInput struct {
	ID, Owner, Repository string
	PullRequest           int
}

func preparePipelineFiles(raw string) ([]pipelineFile, int) {
	parts := strings.Split(raw, "diff --git ")
	files := []pipelineFile{}
	ignored := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		patch := "diff --git " + part
		line := strings.SplitN(part, "\n", 2)[0]
		fields := strings.Fields(line)
		if len(fields) < 2 {
			ignored++
			continue
		}
		path := strings.TrimPrefix(fields[1], "b/")
		if path == fields[1] {
			path = strings.TrimPrefix(fields[0], "a/")
		}
		if shouldIgnorePipelineFile(path, patch) {
			ignored++
			continue
		}
		files = append(files, pipelineFile{path, patch})
	}
	return files, ignored
}
func shouldIgnorePipelineFile(path, patch string) bool {
	lower := strings.ToLower(path)
	return !strings.Contains(patch, "@@") || strings.HasPrefix(lower, "vendor/") || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".lock")
}
func deterministicPlan(files []pipelineFile, p Policy) (pipelinePlan, bool) {
	maxFiles := p.MaxFilesPerBlock
	if maxFiles < 1 {
		maxFiles = 2
	}
	maxChars := p.MaxBlockChars
	if maxChars < 1 {
		maxChars = 4000
	}
	sorted := append([]pipelineFile(nil), files...)
	sort.SliceStable(sorted, func(i, j int) bool { return filepath.Dir(sorted[i].Path) < filepath.Dir(sorted[j].Path) })
	plan := pipelinePlan{Summary: "Plano determinístico gerado localmente."}
	var group pipelineGroup
	chars := 0
	for _, file := range sorted {
		if len(group.Files) > 0 && (len(group.Files) >= maxFiles || chars+len(file.Patch) > maxChars) {
			group.ID = fmt.Sprintf("group-%d", len(plan.Groups)+1)
			plan.Groups = append(plan.Groups, group)
			group = pipelineGroup{}
			chars = 0
		}
		group.Files = append(group.Files, file.Path)
		chars += len(file.Patch)
	}
	if len(group.Files) > 0 {
		group.ID = fmt.Sprintf("group-%d", len(plan.Groups)+1)
		plan.Groups = append(plan.Groups, group)
	}
	return plan, true
}
func validPipelinePlan(plan pipelinePlan, files []pipelineFile) bool {
	seen := map[string]bool{}
	exists := map[string]bool{}
	for _, f := range files {
		exists[f.Path] = true
	}
	for _, g := range plan.Groups {
		if g.ID == "" || len(g.Files) == 0 {
			return false
		}
		for _, path := range g.Files {
			if !exists[path] || seen[path] {
				return false
			}
			seen[path] = true
		}
	}
	return len(seen) == len(files)
}
func stagePrompt(stage, basePrompt, stagePrompt, contract string, files []pipelineFile, data any) string {
	b := strings.Builder{}
	if strings.TrimSpace(basePrompt) != "" {
		b.WriteString(basePrompt)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "<stage>%s</stage>\n%s\n%s\n<diff>\n", stage, stagePrompt, contract)
	for _, f := range files {
		fmt.Fprintf(&b, "===== FILE %s =====\n%s\n", f.Path, f.Patch)
	}
	b.WriteString("</diff>")
	if data != nil {
		v, _ := json.Marshal(data)
		fmt.Fprintf(&b, "\n<context>%s</context>", v)
	}
	return b.String()
}
func decodeStage(raw string, out any) error {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	cleaned := strings.TrimSpace(raw)
	if err := decodeStageJSON(cleaned, out); err == nil {
		return nil
	} else if repaired, ok := repairTruncatedJSONObject(cleaned, err); ok {
		if decodeErr := decodeStageJSON(repaired, out); decodeErr == nil {
			return nil
		}
	}
	start, end := strings.Index(cleaned, "{"), strings.LastIndex(cleaned, "}")
	if start >= 0 && end > start {
		candidate := cleaned[start : end+1]
		if err := decodeStageJSON(candidate, out); err == nil {
			return nil
		}
	} else if start >= 0 {
		candidate := cleaned[start:]
		if repaired, ok := repairTruncatedJSONObject(candidate, fmt.Errorf("unexpected EOF")); ok {
			if err := decodeStageJSON(repaired, out); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("resposta não contém um objeto JSON")
}

func repairTruncatedJSONObject(raw string, decodeErr error) (string, bool) {
	if !strings.Contains(strings.ToLower(decodeErr.Error()), "unexpected eof") {
		return "", false
	}
	stack := make([]rune, 0, 8)
	inString := false
	escaped := false
	for _, ch := range raw {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, ch)
		case '}':
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				stack = stack[:len(stack)-1]
			}
		case ']':
			if len(stack) > 0 && stack[len(stack)-1] == '[' {
				stack = stack[:len(stack)-1]
			}
		}
	}
	var repaired strings.Builder
	repaired.WriteString(raw)
	if inString {
		repaired.WriteByte('"')
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == '{' {
			repaired.WriteByte('}')
		} else {
			repaired.WriteByte(']')
		}
	}
	return repaired.String(), true
}

func decodeStageJSON(raw string, out any) error {
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func selectPipelineFiles(files []pipelineFile, paths []string) []pipelineFile {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	out := []pipelineFile{}
	for _, f := range files {
		if set[f.Path] {
			out = append(out, f)
		}
	}
	return out
}
func filePaths(files []pipelineFile) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}
func safeStageTokens(text string, max int, p Policy) int {
	if max < 1 {
		max = 2000
	}
	if p.ContextSafetyTokens > 0 && p.MaxBlockChars > 0 {
		available := p.MaxBlockChars/3 - p.ContextSafetyTokens
		if available > 0 && available < max {
			return available
		}
	}
	return max
}
func validatePipelineFindings(in []pipelineFinding, files []pipelineFile) []pipelineFinding {
	valid := map[string]map[int]bool{}
	for _, f := range files {
		valid[f.Path] = addedPipelineLines(f.Patch)
	}
	out := []pipelineFinding{}
	seen := map[string]bool{}
	for _, f := range in {
		f.Severity = normalizePipelineSeverity(f.Severity)
		if !f.Introduced || f.File == "" || !valid[f.File][f.Line] || f.Severity == "" || f.DecisionReason == "" || f.Comment == "" {
			continue
		}
		key := fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Comment)
		if !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}
func addedPipelineLines(patch string) map[int]bool {
	out := map[int]bool{}
	line := 0
	for _, text := range strings.Split(patch, "\n") {
		if strings.HasPrefix(text, "@@") {
			plus := strings.Index(text, "+")
			if plus >= 0 {
				line = 0
				for _, digit := range text[plus+1:] {
					if digit < '0' || digit > '9' {
						break
					}
					line = line*10 + int(digit-'0')
				}
			}
			continue
		}
		if line == 0 {
			continue
		}
		if strings.HasPrefix(text, "+") && !strings.HasPrefix(text, "+++") {
			out[line] = true
			line++
		} else if !strings.HasPrefix(text, "-") {
			line++
		}
	}
	return out
}
func normalizePipelineSeverity(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "critical", "critica", "crítica":
		return "critica"
	case "high", "alta":
		return "alta"
	case "medium", "media", "média":
		return "media"
	case "low", "baixa":
		return "baixa"
	}
	return ""
}
func fallbackConsolidation(reviews []pipelineGroupReview) pipelineConsolidated {
	out := pipelineConsolidated{Summary: "Review consolidada por fallback determinístico."}
	seen := map[string]bool{}
	for _, r := range reviews {
		for _, f := range r.Findings {
			k := fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Comment)
			if !seen[k] {
				seen[k] = true
				out.Findings = append(out.Findings, f)
			}
		}
	}
	return out
}
func confidenceFindings(in []pipelineFinding, min float64) []pipelineFinding {
	if min == 0 {
		min = .75
	}
	out := []pipelineFinding{}
	for _, f := range in {
		if f.Confidence >= min {
			out = append(out, f)
		}
	}
	return out
}
func verifiedFindings(in []pipelineFinding, v pipelineVerification, min float64) []pipelineFinding {
	byID := map[string]pipelineFinding{}
	for _, f := range in {
		byID[f.ID] = f
	}
	out := []pipelineFinding{}
	for _, r := range v.Results {
		if f, ok := byID[r.ID]; ok && strings.EqualFold(r.Status, "confirmed") && r.Confidence >= min {
			f.Confidence = r.Confidence
			out = append(out, f)
		}
	}
	return out
}
func validFormattedComments(in []pipelineComment, findings []pipelineFinding) []pipelineComment {
	allowed := map[string]bool{}
	for _, f := range findings {
		allowed[fmt.Sprintf("%s:%d", f.File, f.Line)] = true
	}
	out := []pipelineComment{}
	for _, c := range in {
		if allowed[fmt.Sprintf("%s:%d", c.File, c.Line)] && normalizePipelineSeverity(c.Severity) != "" && c.Comment != "" && c.DecisionReason != "" {
			c.Severity = normalizePipelineSeverity(c.Severity)
			out = append(out, c)
		}
	}
	return out
}
func deterministicPipelineResult(id, provider string, findings []pipelineFinding, summary string, partial bool, p Policy, meta map[string]any) Result {
	comments := []Comment{}
	for _, f := range findings {
		comments = append(comments, Comment{File: f.File, Line: f.Line, Severity: f.Severity, DecisionReason: f.DecisionReason, Comment: f.Comment})
	}
	if summary == "" {
		summary = "Review automatizada concluída."
	}
	r := Result{ReviewID: id, Provider: provider, Agent: "reviewer", Summary: summary, Comments: comments, FinalReview: FinalReview{Summary: summary}, Metadata: meta}
	applyPipelineDecision(&r, partial, p)
	return r
}
func applyPipelineDecision(r *Result, partial bool, p Policy) {
	if partial {
		r.FinalReview.GiteaEvent = p.PartialEvent
		if r.FinalReview.GiteaEvent == "" {
			r.FinalReview.GiteaEvent = "COMMENT"
		}
		r.FinalReview.Status = "parcial"
		return
	}
	rank := 0
	for _, c := range r.Comments {
		n := map[string]int{"baixa": 1, "media": 2, "alta": 3, "critica": 4}[c.Severity]
		if n > rank {
			rank = n
		}
	}
	if rank >= 3 {
		r.FinalReview.GiteaEvent = "REQUEST_CHANGES"
		r.FinalReview.Status = "reprovado"
	} else if rank == 2 {
		r.FinalReview.GiteaEvent = p.MediumSeverityEvent
		if r.FinalReview.GiteaEvent == "" {
			r.FinalReview.GiteaEvent = "REQUEST_CHANGES"
		}
		r.FinalReview.Status = "comentado"
	} else if rank == 1 {
		r.FinalReview.GiteaEvent = "COMMENT"
		r.FinalReview.Status = "comentado"
	} else {
		r.FinalReview.GiteaEvent = "APPROVE"
		r.FinalReview.Status = "aprovado"
		if r.FinalReview.Observations == "" {
			r.FinalReview.Observations = "Nenhum problema relevante foi confirmado."
		}
	}
	normalizeFinalReviewText(r, partial)
}

func normalizeFinalReviewText(r *Result, partial bool) {
	technicalSummary := strings.TrimSpace(r.Summary)
	switch r.FinalReview.GiteaEvent {
	case "APPROVE":
		r.FinalReview.Summary = "Nenhum problema relevante foi confirmado."
		if technicalSummary != "" {
			r.FinalReview.Observations = "Resumo técnico: " + technicalSummary
		} else {
			r.FinalReview.Observations = "Review pronta para aprovação."
		}
	case "REQUEST_CHANGES":
		if len(r.Comments) == 1 {
			r.FinalReview.Summary = "Foi identificado 1 problema que precisa de correção antes da aprovação."
		} else {
			r.FinalReview.Summary = fmt.Sprintf("Foram identificados %d problemas que precisam de correção antes da aprovação.", len(r.Comments))
		}
		r.FinalReview.Observations = finalReviewObservations(technicalSummary, r.Comments, "Ajustes obrigatórios:")
	case "COMMENT":
		if partial {
			r.FinalReview.Summary = "A revisão foi concluída parcialmente e exige validação manual."
			r.FinalReview.Observations = finalReviewObservations(technicalSummary, r.Comments, "Resultado parcial:")
			return
		}
		if len(r.Comments) == 0 {
			r.FinalReview.Summary = "A revisão não encontrou bloqueios, mas o resultado deve ser publicado como comentário."
			r.FinalReview.Observations = finalReviewObservations(technicalSummary, nil, "Observações:")
			return
		}
		if len(r.Comments) == 1 {
			r.FinalReview.Summary = "Foi identificado 1 ponto de atenção sem bloqueio automático."
		} else {
			r.FinalReview.Summary = fmt.Sprintf("Foram identificados %d pontos de atenção sem bloqueio automático.", len(r.Comments))
		}
		r.FinalReview.Observations = finalReviewObservations(technicalSummary, r.Comments, "Pontos de atenção:")
	}
}

func finalReviewObservations(technicalSummary string, comments []Comment, heading string) string {
	parts := []string{}
	if technicalSummary != "" {
		parts = append(parts, technicalSummary)
	}
	if len(comments) == 0 {
		if len(parts) == 0 {
			return "Sem observações adicionais."
		}
		return strings.Join(parts, "\n\n")
	}
	var findings strings.Builder
	findings.WriteString(heading)
	limit := len(comments)
	if limit > 3 {
		limit = 3
	}
	for i := 0; i < limit; i++ {
		comment := comments[i]
		message := strings.TrimSpace(comment.DecisionReason)
		if message == "" {
			message = strings.TrimSpace(comment.Comment)
		}
		fmt.Fprintf(&findings, "\n- %s:%d - %s", comment.File, comment.Line, message)
	}
	if len(comments) > limit {
		fmt.Fprintf(&findings, "\n- ... e mais %d achado(s).", len(comments)-limit)
	}
	parts = append(parts, findings.String())
	return strings.Join(parts, "\n\n")
}
