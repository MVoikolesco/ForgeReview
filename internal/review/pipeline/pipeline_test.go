package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitea-agents/internal/diff"
)

func TestSafeOutputTokensClampsConfiguredLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ContextWindow = 1000
	cfg.SafetyMarginTokens = 100
	got, err := SafeOutputTokens(cfg, strings.Repeat("a", 1200), 900)
	if err != nil {
		t.Fatal(err)
	}
	if got >= 900 || got <= 0 {
		t.Fatalf("expected clamped positive output, got %d", got)
	}
}

func TestSafeOutputTokensRejectsOversizedInput(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ContextWindow = 100
	cfg.SafetyMarginTokens = 50
	if _, err := SafeOutputTokens(cfg, strings.Repeat("a", 1000), 100); err == nil {
		t.Fatal("expected oversized input error")
	}
}

func TestParseJSONStageStripsMarkdownFence(t *testing.T) {
	var review GroupReview
	err := parseJSONStage("reviewer", "```json\n{\"group_id\":\"g\",\"reviewed_files\":[],\"findings\":[],\"review_summary\":\"ok\"}\n```", &review)
	if err != nil || review.GroupID != "g" {
		t.Fatalf("expected fenced json to parse, got %#v err=%v", review, err)
	}
}

func TestFallbackPlanCoversFilesOnce(t *testing.T) {
	input := Input{Files: sampleFiles()}
	plan := FallbackPlan(input, DefaultConfig(), "test")
	seen := map[string]int{}
	for _, group := range plan.Groups {
		for _, file := range group.Files {
			seen[file]++
		}
	}
	if len(seen) != len(input.Files) {
		t.Fatalf("expected all files, got %#v", seen)
	}
	for file, count := range seen {
		if count != 1 {
			t.Fatalf("file %s count=%d", file, count)
		}
	}
}

func TestValidPlanRejectsOmittedDuplicatedAndInventedFiles(t *testing.T) {
	input := Input{Files: []diff.ChangedFile{{Path: "a.go"}, {Path: "b.go"}}}
	cases := []ReviewPlan{
		{Groups: []ReviewGroup{{ID: "g1", Files: []string{"a.go"}}}},
		{Groups: []ReviewGroup{{ID: "g1", Files: []string{"a.go", "a.go"}}, {ID: "g2", Files: []string{"b.go"}}}},
		{Groups: []ReviewGroup{{ID: "g1", Files: []string{"a.go", "b.go", "c.go"}}}},
	}
	for _, plan := range cases {
		if validPlan(plan, input) {
			t.Fatalf("expected invalid plan %#v", plan)
		}
	}
}

func TestValidateFindingsNormalizesAndFilters(t *testing.T) {
	findings := validateFindings([]ReviewFinding{{ID: "f1", File: "app.go", Line: 10, Severity: "HIGH", Confidence: 2, DecisionReason: "reason", Comment: "comment", IntroducedByPR: true}, {ID: "f2", File: "app.go", Line: 11, Severity: "grave", Confidence: .5, DecisionReason: "reason", Comment: "comment", IntroducedByPR: true}, {ID: "f3", File: "app.go", Line: 10, Severity: "alta", Confidence: .9, DecisionReason: "O diff nao mostra contrato declarado.", Comment: "comment", IntroducedByPR: true}}, Input{Files: sampleFiles()}, "group-1")
	if len(findings) != 1 || findings[0].Severity != "alta" || findings[0].Confidence != 1 || findings[0].SourceGroupID != "group-1" {
		t.Fatalf("unexpected findings %#v", findings)
	}
}

func TestEnforceDecisionDoesNotBlockWhenDiffOnlyCommentIsFiltered(t *testing.T) {
	cfg := DefaultConfig()
	response := enforceDecision(FinalResponse{Comments: []FinalComment{{File: "app.go", Line: 10, Severity: "alta", DecisionReason: "O diff nao mostra a validacao anterior.", Comment: "Sem evidencia concreta."}}, FinalReview: FinalReview{Summary: "Resumo"}}, nil, Metadata{}, cfg, "Resumo")
	if response.FinalReview.GiteaEvent != "APPROVED" || len(response.Comments) != 0 {
		t.Fatalf("expected diff-only finding to be filtered and approved, got %#v", response)
	}
}

func TestVerifierRejectsBelowThresholdAndRejectedStatus(t *testing.T) {
	input := Input{Files: sampleFiles()}
	consolidated := ConsolidatedReview{Findings: []ReviewFinding{{ID: "c1", File: "app.go", Line: 10, Severity: "alta", Confidence: .9, DecisionReason: "r", Comment: "c", IntroducedByPR: true}, {ID: "c2", File: "app.go", Line: 10, Severity: "alta", Confidence: .9, DecisionReason: "r", Comment: "c", IntroducedByPR: true}}}
	runner := Runner{Config: DefaultConfig(), Chat: func(ctx context.Context, stage string, prompt string, maxOutputTokens int) (string, StageUsage, error) {
		return `{"results":[{"finding_id":"c1","status":"rejected","confidence":0.99,"verification_reason":"sem evidencia","adjusted_finding":null},{"finding_id":"c2","status":"confirmed","confidence":0.2,"verification_reason":"baixa confianca","adjusted_finding":null}]}`, StageUsage{}, nil
	}}
	approved, rejected, fallback := runner.verify(context.Background(), DefaultConfig(), input, consolidated)
	if fallback || len(approved) != 0 || rejected != 2 {
		t.Fatalf("unexpected verifier result approved=%#v rejected=%d fallback=%t", approved, rejected, fallback)
	}
}

func TestDeterministicFinalDoesNotApprovePartialReview(t *testing.T) {
	cfg := DefaultConfig()
	meta := Metadata{PartialReview: true, FailedGroups: 1}
	response := deterministicFinal(nil, meta, cfg, "Resumo")
	if response.FinalReview.Status != "parcial" || response.FinalReview.GiteaEvent != "COMMENT" {
		t.Fatalf("unexpected partial response %#v", response.FinalReview)
	}
}

func TestAlignFinalCommentLinesUsesValidatedFindingLines(t *testing.T) {
	response := FinalResponse{Comments: []FinalComment{{File: "app.go", Line: 11, Severity: "alta", DecisionReason: "r", Comment: "c"}}}
	findings := []ReviewFinding{{File: "app.go", Line: 10, Severity: "alta"}}

	aligned := alignFinalCommentLines(response, findings)

	if aligned.Comments[0].Line != 10 {
		t.Fatalf("expected formatter line to be replaced by validated finding line, got %d", aligned.Comments[0].Line)
	}
}

func TestTruncateInputDiffMarksFilesWithoutCuttingMidLine(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxInputTokens = 20
	input := Input{Files: []diff.ChangedFile{{Path: "big.go", Patch: "diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -1,1 +1,4 @@\n-old\n+line1\n+line2\n+line3\n+line4\n"}}}
	out, truncated := truncateInputDiff(input, cfg)
	if !truncated || !strings.Contains(out.Files[0].Patch, "TRUNCADO") || !strings.HasSuffix(out.Files[0].Patch, "\n") {
		t.Fatalf("expected marked line-safe truncation %#v", out.Files[0].Patch)
	}
}

func TestRunnerFullFlowAndFallbacks(t *testing.T) {
	input := Input{Owner: "o", Repository: "r", PullRequestNumber: 1, Files: sampleFiles(), Stacks: []string{"go"}}
	calls := 0
	runner := Runner{Config: DefaultConfig(), Chat: func(ctx context.Context, stage string, prompt string, maxOutputTokens int) (string, StageUsage, error) {
		calls++
		switch stage {
		case "planner":
			return `{"pr_summary":"Resumo","risk_level":"medio","risk_areas":[],"groups":[{"id":"group-1","purpose":"Go","files":["app.go"],"relevant_stacks":["go"],"risk_level":"medio","review_focus":["contrato"]}],"assumptions":[]}`, StageUsage{}, nil
		case "reviewer":
			return `{"group_id":"group-1","reviewed_files":["app.go"],"findings":[{"id":"f1","file":"app.go","line":10,"severity":"alta","category":"correctness","confidence":0.9,"title":"Bug","decision_reason":"Quebra contrato","comment":"Corrija o contrato.","evidence":"diff","failure_scenario":"falha","suggested_fix":"corrigir","introduced_by_pr":true}],"review_summary":"um achado"}`, StageUsage{}, nil
		case "consolidator":
			return "invalid", StageUsage{}, nil
		case "verifier":
			return `{"results":[{"finding_id":"consolidated-1","status":"confirmed","confidence":0.95,"verification_reason":"ok","adjusted_finding":null}]}`, StageUsage{}, nil
		case "formatter":
			return "invalid", StageUsage{}, nil
		default:
			return "", StageUsage{}, errors.New("unexpected stage")
		}
	}, StackRules: func(context.Context, []string) (string, []string, error) { return "go rules", []string{"go"}, nil }}
	raw, final, meta, err := runner.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 5 || len(meta.StageMetrics) != 5 || !meta.ConsolidatorFallback || !meta.FormatterFallback || meta.ConfirmedFindings != 1 || len(final.InlineComments) != 1 || !strings.Contains(raw, "metadata") {
		t.Fatalf("unexpected result calls=%d meta=%#v final=%#v raw=%s", calls, meta, final, raw)
	}
}

func sampleFiles() []diff.ChangedFile {
	return []diff.ChangedFile{{Path: "app.go", Additions: 1, Deletions: 1, Patch: "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10,1 +10,1 @@\n-old\n+new\n"}}
}
