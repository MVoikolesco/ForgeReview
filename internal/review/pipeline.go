package review

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gitea-agents/internal/providers"
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

// runPipeline restores the staged review workflow while retaining the current
// repository result and Gitea contracts. Each stage has a deterministic fallback.
func (s *Service) runPipeline(ctx context.Context, provider providers.LLMProvider, jobInput queueInput, rawDiff, prompt string, policy Policy, progress func(string, string, string, map[string]any, time.Time, error)) (Result, error) {
	started := time.Now()
	files, ignored := preparePipelineFiles(rawDiff)
	meta := map[string]any{"pipeline_version": "2", "total_files": len(files) + ignored, "reviewed_files": len(files), "ignored_files": ignored, "stage_metrics": []map[string]any{}}
	progress("preparacao", "concluido", fmt.Sprintf("%d arquivos preparados; %d ignorados", len(files), ignored), map[string]any{"files": filePaths(files)}, started, nil)
	if len(files) == 0 {
		return deterministicPipelineResult(jobInput.ID, provider.Name(), nil, "Nenhum arquivo revisavel encontrado no diff.", false, policy, meta), nil
	}
	call := func(stage, text string, max int) (string, error) {
		at := time.Now()
		response, usage, err := provider.Chat(ctx, text, safeStageTokens(text, max, policy))
		metric := map[string]any{"stage": stage, "duration_ms": time.Since(at).Milliseconds(), "input_chars": len(text), "requested_output_tokens": max, "actual_prompt_tokens": usage.PromptTokens, "actual_completion_tokens": usage.CompletionTokens, "response_chars": len(response), "failed": err != nil}
		meta["stage_metrics"] = append(meta["stage_metrics"].([]map[string]any), metric)
		return response, err
	}
	planningStarted := time.Now()
	progress("planejamento", "processando", "Agrupando arquivos para review", nil, planningStarted, nil)
	plan, fallback := deterministicPlan(files, policy)
	if policy.PlannerEnabled {
		var candidate pipelinePlan
		if raw, err := call("planner", stagePrompt("planner", prompt, files, nil), policy.PlannerMaxTokens); err == nil && decodeStage(raw, &candidate) == nil && validPipelinePlan(candidate, files) {
			plan = candidate
			fallback = false
		}
	}
	meta["planner_fallback"] = fallback
	meta["review_groups"] = len(plan.Groups)
	progress("planejamento", "concluido", fmt.Sprintf("%d grupos de review", len(plan.Groups)), map[string]any{"fallback": fallback, "total_groups": len(plan.Groups)}, planningStarted, nil)

	progress("revisao", "processando", "Revisando grupos de arquivos", map[string]any{"total_groups": len(plan.Groups)}, time.Now(), nil)
	reviews, failed := s.reviewPipelineGroups(ctx, call, prompt, files, plan, policy, progress)
	meta["successful_groups"] = len(reviews)
	meta["failed_groups"] = len(failed)
	meta["partial_review"] = len(failed) > 0
	if len(reviews) == 0 {
		return Result{}, fmt.Errorf("nenhum grupo retornou uma review válida")
	}

	consolidationStarted := time.Now()
	progress("consolidacao", "processando", "Consolidando achados", nil, consolidationStarted, nil)
	consolidated := fallbackConsolidation(reviews)
	consolidatorFallback := true
	if policy.ConsolidatorEnabled {
		var candidate pipelineConsolidated
		if raw, err := call("consolidator", stagePrompt("consolidator", prompt, files, reviews), policy.ConsolidatorMaxTokens); err == nil && decodeStage(raw, &candidate) == nil {
			consolidated = candidate
			consolidatorFallback = false
		}
	}
	consolidated.Findings = validatePipelineFindings(consolidated.Findings, files)
	meta["consolidator_fallback"] = consolidatorFallback
	meta["consolidated_findings"] = len(consolidated.Findings)
	progress("consolidacao", "concluido", fmt.Sprintf("%d achados consolidados", len(consolidated.Findings)), map[string]any{"fallback": consolidatorFallback}, consolidationStarted, nil)

	approved := consolidated.Findings
	verificationStarted := time.Now()
	progress("verificacao", "processando", "Validando relevância dos achados", nil, verificationStarted, nil)
	verifierFallback := false
	if len(approved) > 0 && policy.VerifierEnabled {
		var verification pipelineVerification
		if raw, err := call("verifier", stagePrompt("verifier", prompt, files, consolidated), policy.VerifierMaxTokens); err == nil && decodeStage(raw, &verification) == nil {
			approved = verifiedFindings(consolidated.Findings, verification, policy.MinimumConfidence)
		} else {
			verifierFallback = true
			approved = confidenceFindings(approved, policy.MinimumConfidence)
		}
	} else if !policy.VerifierEnabled {
		verifierFallback = true
		approved = confidenceFindings(approved, policy.MinimumConfidence)
	}
	meta["verifier_fallback"] = verifierFallback
	meta["confirmed_findings"] = len(approved)
	meta["rejected_findings"] = len(consolidated.Findings) - len(approved)
	progress("verificacao", "concluido", fmt.Sprintf("%d achados confirmados", len(approved)), map[string]any{"fallback": verifierFallback}, verificationStarted, nil)

	result := deterministicPipelineResult(jobInput.ID, provider.Name(), approved, consolidated.Summary, len(failed) > 0, policy, meta)
	formattingStarted := time.Now()
	progress("formatacao", "processando", "Montando comentário final", nil, formattingStarted, nil)
	formatterFallback := true
	if policy.FormatterEnabled {
		var formatted pipelineFormatted
		if raw, err := call("formatter", stagePrompt("formatter", prompt, files, approved), policy.FormatterMaxTokens); err == nil && decodeStage(raw, &formatted) == nil && formatted.FinalReview.Summary != "" {
			formatted.Comments = validFormattedComments(formatted.Comments, approved)
			if formatted.Comments != nil {
				result.Comments = make([]Comment, 0, len(formatted.Comments))
				for _, comment := range formatted.Comments {
					result.Comments = append(result.Comments, Comment{File: comment.File, Line: comment.Line, Severity: comment.Severity, DecisionReason: comment.DecisionReason, Comment: comment.Comment})
				}
			}
			result.FinalReview = formatted.FinalReview
			formatterFallback = false
		}
	}
	applyPipelineDecision(&result, len(failed) > 0, policy)
	meta["formatter_fallback"] = formatterFallback
	meta["total_duration_ms"] = time.Since(started).Milliseconds()
	progress("formatacao", "concluido", fmt.Sprintf("%d comentários formatados", len(result.Comments)), map[string]any{"fallback": formatterFallback}, formattingStarted, nil)
	return result, validateResult(result)
}

// queueInput avoids coupling the pipeline mechanics to queue transport fields.
type queueInput struct {
	ID, Owner, Repository string
	PullRequest           int
}

func (s *Service) reviewPipelineGroups(ctx context.Context, call func(string, string, int) (string, error), prompt string, files []pipelineFile, plan pipelinePlan, policy Policy, progress func(string, string, string, map[string]any, time.Time, error)) ([]pipelineGroupReview, []string) {
	parallel := policy.MaxParallelGroups
	if parallel < 1 {
		parallel = 1
	}
	attempts := policy.ContractMaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	type outcome struct {
		index  int
		review pipelineGroupReview
		failed string
	}
	out := make(chan outcome, len(plan.Groups))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, group := range plan.Groups {
		wg.Add(1)
		sem <- struct{}{}
		go func(index int, group pipelineGroup) {
			defer wg.Done()
			defer func() { <-sem }()
			stageStarted := time.Now()
			groupFiles := selectPipelineFiles(files, group.Files)
			progress("revisao", "processando", "Revisando grupo "+group.ID, map[string]any{"group_id": group.ID, "group_index": index + 1, "total_groups": len(plan.Groups), "files": filePaths(groupFiles)}, stageStarted, nil)
			base := stagePrompt("reviewer", prompt, groupFiles, group)
			var review pipelineGroupReview
			var err error
			for attempt := 1; attempt <= attempts; attempt++ {
				raw, callErr := call("reviewer", base, policy.GroupMaxTokens)
				if callErr != nil {
					err = callErr
					break
				}
				if err = decodeStage(raw, &review); err == nil {
					review.Findings = validatePipelineFindings(review.Findings, groupFiles)
					out <- outcome{index: index, review: review}
					progress("revisao", "concluido", "Grupo "+group.ID+" revisado", map[string]any{"group_id": group.ID, "attempt": attempt, "max_attempts": attempts}, stageStarted, nil)
					return
				}
				if attempt < attempts {
					progress("revisao", "retentando", "Contrato inválido no grupo "+group.ID, map[string]any{"group_id": group.ID, "attempt": attempt + 1, "max_attempts": attempts}, stageStarted, err)
					base += "\n\nRetorne exclusivamente JSON válido conforme o contrato."
				}
			}
			progress("revisao", "falhou", "Grupo "+group.ID+" falhou", map[string]any{"group_id": group.ID, "attempt": attempts, "max_attempts": attempts}, stageStarted, err)
			out <- outcome{index: index, failed: group.ID}
		}(i, group)
	}
	wg.Wait()
	close(out)
	byIndex := make([]pipelineGroupReview, len(plan.Groups))
	good := make([]bool, len(plan.Groups))
	failed := []string{}
	for item := range out {
		if item.failed != "" {
			failed = append(failed, item.failed)
		} else {
			byIndex[item.index] = item.review
			good[item.index] = true
		}
	}
	reviews := []pipelineGroupReview{}
	for i := range byIndex {
		if good[i] {
			reviews = append(reviews, byIndex[i])
		}
	}
	return reviews, failed
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
func stagePrompt(stage, prompt string, files []pipelineFile, data any) string {
	b := strings.Builder{}
	fmt.Fprintf(&b, "%s\n\n<stage>%s</stage>\n%s\n<diff>\n", prompt, stage, pipelineStageContract(stage))
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

func pipelineStageContract(stage string) string {
	switch stage {
	case "planner":
		return `Responda somente JSON: {"pr_summary":"...","risk_level":"...","risk_areas":["..."],"groups":[{"id":"...","purpose":"...","files":["path"],"risk_level":"...","review_focus":["..."]}],"assumptions":["..."]}. Cada arquivo do diff deve aparecer exatamente uma vez.`
	case "reviewer":
		return `Responda somente JSON: {"group_id":"...","reviewed_files":["path"],"findings":[{"id":"...","file":"path","line":1,"severity":"critica|alta|media|baixa","category":"...","confidence":0.0,"decision_reason":"...","comment":"...","title":"...","evidence":"...","failure_scenario":"...","suggested_fix":"...","source_group_id":"...","introduced_by_pr":true}],"review_summary":"..."}. Reporte somente linhas adicionadas no diff.`
	case "consolidator":
		return `Responda somente JSON: {"findings":[...],"pr_summary":"...","overall_risk":"...","discarded_findings":[{"source_finding_id":"...","reason":"..."}]}. Preserve apenas achados comprovados no diff.`
	case "verifier":
		return `Responda somente JSON: {"results":[{"finding_id":"...","status":"confirmed|adjusted|rejected","confidence":0.0,"verification_reason":"...","adjusted_finding":null}]}. Confirme somente problemas com evidência suficiente.`
	case "formatter":
		return `Responda somente JSON: {"comments":[{"file":"path","line":1,"severity":"critica|alta|media|baixa","type":"...","decision_reason":"...","comment":"..."}],"final_review":{"gitea_event":"APPROVE|COMMENT|REQUEST_CHANGES","status":"...","summary":"...","observations":"..."}}.`
	default:
		return "Responda somente JSON válido conforme o contrato desta etapa."
	}
}
func decodeStage(raw string, out any) error {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	if err := decodeStageJSON(strings.TrimSpace(raw), out); err == nil {
		return nil
	}
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return decodeStageJSON(raw[start:end+1], out)
	}
	return fmt.Errorf("resposta não contém um objeto JSON")
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
	}
}
