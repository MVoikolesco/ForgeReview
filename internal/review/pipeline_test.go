package review

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitea-agents/internal/contracts"
	"gitea-agents/internal/providers"
)

type pipelineProvider struct {
	calls        map[string]int
	invalidFirst bool
}

func (p *pipelineProvider) Name() string { return "test" }
func (p *pipelineProvider) Review(context.Context, providers.Input) (contracts.Result, error) {
	return contracts.Result{}, nil
}
func (p *pipelineProvider) Chat(_ context.Context, prompt string, _ int) (string, providers.Usage, error) {
	stage := ""
	for _, candidate := range []string{"planner", "reviewer", "consolidator", "verifier", "formatter"} {
		if strings.Contains(prompt, "<stage>"+candidate+"</stage>") {
			stage = candidate
			break
		}
	}
	p.calls[stage]++
	switch stage {
	case "planner":
		return `{"pr_summary":"planned","groups":[{"id":"group-1","files":["app.go"]}]}`, providers.Usage{}, nil
	case "reviewer":
		if p.invalidFirst && p.calls[stage] == 1 {
			return `{"findings":[],"unknown":true}`, providers.Usage{}, nil
		}
		return `{"findings":[{"id":"f1","file":"app.go","line":10,"severity":"alta","confidence":0.9,"decision_reason":"new behavior fails","comment":"handle the error","introduced_by_pr":true}]}`, providers.Usage{}, nil
	case "consolidator":
		return `{"findings":[{"id":"f1","file":"app.go","line":10,"severity":"alta","confidence":0.9,"decision_reason":"new behavior fails","comment":"handle the error","introduced_by_pr":true}],"pr_summary":"consolidated"}`, providers.Usage{}, nil
	case "verifier":
		return `{"results":[{"finding_id":"f1","status":"confirmed","confidence":0.9}]}`, providers.Usage{}, nil
	case "formatter":
		return `{"comments":[{"file":"app.go","line":10,"severity":"alta","decision_reason":"new behavior fails","comment":"handle the error"}],"final_review":{"gitea_event":"COMMENT","status":"comentado","summary":"formatted","observations":""}}`, providers.Usage{}, nil
	}
	return "", providers.Usage{}, nil
}

func TestPipelineRunsStagesAndRetriesInvalidGroupContract(t *testing.T) {
	p := &pipelineProvider{calls: map[string]int{}, invalidFirst: true}
	policy := Policy{PlannerEnabled: true, ConsolidatorEnabled: true, VerifierEnabled: true, FormatterEnabled: true, PlannerMaxTokens: 100, GroupMaxTokens: 100, ConsolidatorMaxTokens: 100, VerifierMaxTokens: 100, FormatterMaxTokens: 100, ContractMaxAttempts: 2, MinimumConfidence: .75, MaxParallelGroups: 1, MediumSeverityEvent: "COMMENT", PartialEvent: "COMMENT"}
	var events []string
	result, err := (&Service{}).runPipeline(context.Background(), p, queueInput{ID: "rev-1"}, "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n", "review", policy, func(stage, status, _ string, _ map[string]any, _ time.Time, _ error) {
		events = append(events, stage+":"+status)
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.calls["reviewer"] != 2 || len(result.Comments) != 1 || result.FinalReview.GiteaEvent != "REQUEST_CHANGES" {
		t.Fatalf("unexpected result=%#v calls=%#v", result, p.calls)
	}
	if result.Metadata["planner_fallback"] != false || result.Metadata["formatter_fallback"] != false {
		t.Fatalf("expected stage output, got %#v", result.Metadata)
	}
	if !containsEvent(events, "revisao:retentando") {
		t.Fatalf("retry was not persisted: %#v", events)
	}
	for _, stage := range []string{"preparacao", "planejamento", "revisao", "consolidacao", "verificacao", "formatacao"} {
		if !containsEvent(events, stage+":concluido") {
			t.Fatalf("stage %s was not completed: %#v", stage, events)
		}
	}
}

func TestPipelineFallsBackDeterministicallyWhenOptionalStagesFail(t *testing.T) {
	p := &pipelineProvider{calls: map[string]int{}}
	policy := Policy{PlannerEnabled: false, ConsolidatorEnabled: false, VerifierEnabled: false, FormatterEnabled: false, MaxFilesPerBlock: 1, ContractMaxAttempts: 1, MinimumConfidence: .75, PartialEvent: "COMMENT"}
	result, err := (&Service{}).runPipeline(context.Background(), p, queueInput{ID: "rev-1"}, "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n", "review", policy, func(string, string, string, map[string]any, time.Time, error) {})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Metadata["planner_fallback"].(bool) || !result.Metadata["formatter_fallback"].(bool) || len(result.Comments) != 1 {
		t.Fatalf("unexpected fallback=%#v", result)
	}
}

func containsEvent(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}
