package review

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gitea-agents/internal/config"
	"gitea-agents/internal/contracts"
	databasepkg "gitea-agents/internal/database"
	"gitea-agents/internal/providers"
	"gitea-agents/internal/queue"
)

type pipelineProvider struct {
	mu           sync.Mutex
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
	p.mu.Lock()
	p.calls[stage]++
	call := p.calls[stage]
	p.mu.Unlock()
	switch stage {
	case "planner":
		return `{"pr_summary":"planned","groups":[{"id":"group-1","files":["app.go"]}]}`, providers.Usage{}, nil
	case "reviewer":
		if p.invalidFirst && call == 1 {
			return `{"findings":[],"unknown":true}`, providers.Usage{}, nil
		}
		return `{"findings":[{"id":"f1","file":"app.go","line":10,"severity":"alta","confidence":0.9,"decision_reason":"new behavior fails","comment":"handle the error","introduced_by_pr":true}]}`, providers.Usage{}, nil
	case "consolidator":
		return `{"findings":[{"id":"f1","file":"app.go","line":10,"end_line":10,"severity":"alta","confidence":0.9,"decision_reason":"new behavior fails","comment":"handle the error","introduced_by_pr":true}],"pr_summary":"consolidated"}`, providers.Usage{}, nil
	case "verifier":
		return `{"results":[{"finding_id":"f1","status":"confirmed","confidence":0.9}]}`, providers.Usage{}, nil
	case "formatter":
		return `{"comments":[{"file":"app.go","line":10,"severity":"alta","decision_reason":"new behavior fails","comment":"handle the error"}],"final_review":{"gitea_event":"COMMENT","status":"comentado","summary":"formatted","observations":""}}`, providers.Usage{}, nil
	}
	return "", providers.Usage{}, nil
}

type failingPipelineProvider struct{}

func (*failingPipelineProvider) Name() string { return "failing" }
func (*failingPipelineProvider) Review(context.Context, providers.Input) (contracts.Result, error) {
	return contracts.Result{}, errors.New("provider unavailable")
}

func (*failingPipelineProvider) Chat(context.Context, string, int) (string, providers.Usage, error) {
	return "", providers.Usage{}, errors.New("provider unavailable")
}

type emptyConsolidatorProvider struct{ base *pipelineProvider }

func (*emptyConsolidatorProvider) Name() string { return "test" }
func (*emptyConsolidatorProvider) Review(context.Context, providers.Input) (contracts.Result, error) {
	return contracts.Result{}, nil
}
func (p *emptyConsolidatorProvider) Chat(ctx context.Context, prompt string, max int) (string, providers.Usage, error) {
	if strings.Contains(prompt, "<stage>consolidator</stage>") {
		return `{"findings":[],"pr_summary":"no valid findings"}`, providers.Usage{}, nil
	}
	return p.base.Chat(ctx, prompt, max)
}

func TestPipelineRunsStagesAndRetriesInvalidGroupContract(t *testing.T) {
	p := &pipelineProvider{calls: map[string]int{}, invalidFirst: true}
	repo, definition, cleanup := seededPipeline(t)
	defer cleanup()
	for i := range definition.Stages {
		definition.Stages[i].MaxOutputTokens = 100
		if definition.Stages[i].ExecutorKey == "reviewer" {
			definition.Stages[i].RetryLimit = 2
		}
	}
	policy := Policy{MinimumConfidence: .75, MaxParallelGroups: 1, MediumSeverityEvent: "COMMENT", PartialEvent: "COMMENT"}
	var events []string
	result, err := NewPipelineEngine(repo).Execute(context.Background(), definition, PipelineExecutionInput{
		Job: queueInput{ID: "rev-1"}, RawDiff: "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n", BasePrompt: "review", Policy: policy,
		Provider: func(context.Context, *int64) (providers.LLMProvider, error) { return p, nil },
		Progress: func(stage, status, _ string, _ map[string]any, _ time.Time, _ error) {
			events = append(events, stage+":"+status)
		},
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
	repo, definition, cleanup := seededPipeline(t)
	defer cleanup()
	for i := range definition.Stages {
		if definition.Stages[i].ExecutorKey != "reviewer" {
			definition.Stages[i].UseLLM = false
		}
	}
	policy := Policy{MaxFilesPerBlock: 1, MinimumConfidence: .75, PartialEvent: "COMMENT"}
	result, err := NewPipelineEngine(repo).Execute(context.Background(), definition, PipelineExecutionInput{
		Job: queueInput{ID: "rev-1"}, RawDiff: "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n", BasePrompt: "review", Policy: policy,
		Provider: func(context.Context, *int64) (providers.LLMProvider, error) { return p, nil },
		Progress: func(string, string, string, map[string]any, time.Time, error) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Metadata["planner_fallback"].(bool) || !result.Metadata["formatter_fallback"].(bool) || len(result.Comments) != 1 {
		t.Fatalf("unexpected fallback=%#v", result)
	}
}

func TestPipelineAllowsRepeatedReviewExecutorInstances(t *testing.T) {
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()
	for i := range definition.Stages {
		if definition.Stages[i].Position >= 4 {
			definition.Stages[i].Position++
		}
	}
	repeated := definition.Stages[2]
	repeated.Key = "revisao-seguranca"
	repeated.Name = "Revisão de segurança"
	repeated.Position = 4
	repeated.PromptTemplate = "Revise apenas segurança."
	stages := make([]PipelineStage, 0, len(definition.Stages)+1)
	stages = append(stages, definition.Stages[:3]...)
	stages = append(stages, repeated)
	stages = append(stages, definition.Stages[3:]...)
	definition.Stages = stages
	if err := validatePipelineDefinition(definition); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineDefinitionAllowsMultipleCompatibleSuccessTransitions(t *testing.T) {
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()

	definition = pipelineWithPlannerFanOut(definition)
	if err := validatePipelineDefinition(definition); err != nil {
		t.Fatalf("expected compatible success fan-out to be valid: %v", err)
	}
}

func TestPipelineSchedulesAllCompatibleSuccessFanOutDestinations(t *testing.T) {
	p := &pipelineProvider{calls: map[string]int{}}
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()

	definition = pipelineWithPlannerFanOut(definition)
	publications := 0
	result, err := NewPipelineEngine(nil).Execute(context.Background(), definition, PipelineExecutionInput{
		Job:     queueInput{ID: "rev-1"},
		RawDiff: "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n",
		Policy:  Policy{MinimumConfidence: .75, MaxParallelGroups: 1},
		Provider: func(context.Context, *int64) (providers.LLMProvider, error) {
			return p, nil
		},
		Publish: func(context.Context, Result) (StageOutcome, error) {
			publications++
			return StageOutcome{ArtifactType: "publication_result"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.calls["planner"] != 2 {
		t.Fatalf("expected both planner destinations to run, got calls=%#v", p.calls)
	}
	if publications != 1 {
		t.Fatalf("expected publication to be scheduled once, got %d", publications)
	}
	if len(result.Comments) != 1 {
		t.Fatalf("unexpected fan-out result: %#v", result)
	}
}

func TestPipelineFirstMatchUsesPriorityOrderedRule(t *testing.T) {
	p := &pipelineProvider{calls: map[string]int{}}
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()
	definition = pipelineWithPlannerFanOut(definition)
	definition.Stages[0].RouteMode = "first_match"
	for index := range definition.Transitions {
		if definition.Transitions[index].FromStageID == definition.Stages[0].ID {
			definition.Transitions[index].Rule = &Rule{
				Operator: RuleGT,
				Scope:    &CollectionScope{Kind: CollectionCount, Path: "files"},
				Value:    0,
			}
		}
	}
	_, err := NewPipelineEngine(nil).Execute(context.Background(), definition, PipelineExecutionInput{
		Job:      queueInput{ID: "first-match"},
		RawDiff:  "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n",
		Policy:   Policy{MinimumConfidence: .75, MaxParallelGroups: 1},
		Provider: func(context.Context, *int64) (providers.LLMProvider, error) { return p, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.calls["planner"] != 1 {
		t.Fatalf("first_match scheduled %d matching branches", p.calls["planner"])
	}
}

func TestPipelineBranchWithNoRuleMatchClosesNormally(t *testing.T) {
	p := &pipelineProvider{calls: map[string]int{}}
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()
	definition.Stages[0].RouteMode = "first_match"
	definition.Transitions[0].Rule = &Rule{Operator: RuleExists, Path: "missing", Value: true}
	result, err := NewPipelineEngine(nil).Execute(context.Background(), definition, PipelineExecutionInput{
		Job: queueInput{ID: "closed-branch"}, RawDiff: "diff --git a/app.go b/app.go\n",
		Provider: func(context.Context, *int64) (providers.LLMProvider, error) { return p, nil },
	})
	if err != nil {
		t.Fatalf("unmatched branch must close normally: %v", err)
	}
	if len(result.Comments) != 0 || len(p.calls) != 0 {
		t.Fatalf("closed branch produced work: result=%#v calls=%#v", result, p.calls)
	}
}

func TestPipelineCyclesRequireEveryTraversalToBeBounded(t *testing.T) {
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()
	loop := definition.Stages[1]
	loop.ID = 999
	loop.Key = "bounded-loop"
	loop.Position = len(definition.Stages) + 1
	loop.InputContract = loop.OutputContract
	definition.Stages = append(definition.Stages, loop)
	loopID := loop.ID
	definition.Transitions = append(definition.Transitions, PipelineTransition{
		ID: 999, FromStageID: loop.ID, ToStageID: &loopID, Type: "success", ConditionKey: "always", MaxTraversals: 2,
	})
	if err := validatePipelineDefinition(definition); err != nil {
		t.Fatalf("explicitly bounded cycle was rejected: %v", err)
	}
	definition.Transitions[len(definition.Transitions)-1].MaxTraversals = 0
	if err := validatePipelineDefinition(definition); err == nil {
		t.Fatal("expected unbounded cyclic traversal to be rejected")
	}
}

func TestPipelineRejectsCatalogedButUnimplementedJoinModes(t *testing.T) {
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()
	for _, mode := range []string{"any", "wait_all"} {
		candidate := definition
		candidate.Stages = append([]PipelineStage(nil), definition.Stages...)
		candidate.Stages[1].JoinMode = mode
		if err := validatePipelineDefinition(candidate); err == nil {
			t.Fatalf("expected join mode %s to be rejected", mode)
		}
	}
}

func TestPipelinePersistsFailedStageStatus(t *testing.T) {
	repo, definition, cleanup := seededPipeline(t)
	defer cleanup()
	p := &failingPipelineProvider{}
	_, err := NewPipelineEngine(repo).Execute(context.Background(), definition, PipelineExecutionInput{
		Job: queueInput{ID: "rev-1"}, RawDiff: "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n",
		Policy:   Policy{MinimumConfidence: .75, MaxParallelGroups: 1},
		Provider: func(context.Context, *int64) (providers.LLMProvider, error) { return p, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatal("expected pipeline failure")
	}
	var status string
	if err := repo.db.QueryRowContext(context.Background(), `SELECT se.status FROM stage_executions se
		JOIN pipeline_executions pe ON pe.id=se.pipeline_execution_id
		WHERE pe.review_id='rev-1' AND se.stage_key='revisao' ORDER BY se.id DESC LIMIT 1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("expected failed stage audit, got %s", status)
	}
}

func TestPipelineRejectsFormattingBeforeVerification(t *testing.T) {
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()
	definition.Stages[4], definition.Stages[5] = definition.Stages[5], definition.Stages[4]
	definition.Stages[4].Position = 5
	definition.Stages[5].Position = 6
	if err := validatePipelineDefinition(definition); err != nil {
		t.Fatalf("stage positions are layout only: %v", err)
	}
}

func TestBoundPipelineRemainsLoadableAfterArchival(t *testing.T) {
	repo, definition, cleanup := seededPipeline(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, `UPDATE pipeline_versions SET status='archived' WHERE id=?`, definition.VersionID); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.Pipeline(ctx, "rev-1", queue.ReviewJob{Owner: "acme", Repository: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VersionID != definition.VersionID {
		t.Fatalf("bound version changed: got %d want %d", loaded.VersionID, definition.VersionID)
	}
}

func TestVerificationConsumesReviewsWithoutConsolidator(t *testing.T) {
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()
	stages := make([]PipelineStage, 0, len(definition.Stages)-1)
	for _, stage := range definition.Stages {
		if stage.ExecutorKey == "consolidator" {
			continue
		}
		stage.Position = len(stages) + 1
		stages = append(stages, stage)
	}
	definition.Stages = stages
	if err := validatePipelineDefinition(definition); err == nil {
		t.Fatal("expected missing edge target to be rejected")
	}
}

func TestVerificationDoesNotRestoreFindingsDiscardedByConsolidator(t *testing.T) {
	base := &pipelineProvider{calls: map[string]int{}}
	provider := &emptyConsolidatorProvider{base: base}
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()
	result, err := NewPipelineEngine(nil).Execute(context.Background(), definition, PipelineExecutionInput{
		Job: queueInput{ID: "rev-1"}, RawDiff: "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n",
		Policy:   Policy{MinimumConfidence: .75, MaxParallelGroups: 1},
		Provider: func(context.Context, *int64) (providers.LLMProvider, error) { return provider, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Comments) != 0 || result.FinalReview.GiteaEvent != "APPROVE" {
		t.Fatalf("discarded findings were restored: %#v", result)
	}
}

func TestDecodeStageRepairsUnexpectedEOF(t *testing.T) {
	raw := `{"group_id":"G1","reviewed_files":["app.go"],"findings":[{"id":"f1","file":"app.go","line":10,"severity":"alta","category":"correctness","confidence":0.9,"decision_reason":"quebra o fluxo","comment":"corrija a validação","introduced_by_pr":true}],"review_summary":"ok"`
	var review pipelineGroupReview

	if err := decodeStage(raw, &review); err != nil {
		t.Fatalf("expected truncated JSON to be repaired: %v", err)
	}
	if review.GroupID != "G1" || len(review.Findings) != 1 {
		t.Fatalf("unexpected repaired review: %#v", review)
	}
}

func TestApplyPipelineDecisionOverridesContradictoryApprovalText(t *testing.T) {
	result := Result{
		Summary: "Nenhum defeito bloqueante encontrado no diff.",
		FinalReview: FinalReview{
			Summary:      "Erro de sintaxe crítico encontrado e necessidade de corrigir antes da aprovação.",
			Observations: "Corrija o campo antes de aprovar.",
		},
	}

	applyPipelineDecision(&result, false, Policy{})

	if result.FinalReview.GiteaEvent != "APPROVE" || result.FinalReview.Status != "aprovado" {
		t.Fatalf("unexpected approval decision: %#v", result.FinalReview)
	}
	if result.FinalReview.Summary != "Nenhum problema relevante foi confirmado." {
		t.Fatalf("unexpected normalized approval summary: %q", result.FinalReview.Summary)
	}
	if strings.Contains(result.FinalReview.Observations, "Corrija") {
		t.Fatalf("approval observations kept contradictory text: %q", result.FinalReview.Observations)
	}
	if !strings.Contains(result.FinalReview.Observations, "Resumo técnico") {
		t.Fatalf("approval observations lost deterministic context: %q", result.FinalReview.Observations)
	}
}

func TestApplyPipelineDecisionOverridesContradictoryRejectionText(t *testing.T) {
	result := Result{
		Summary: "O diff introduz uma falha concreta.",
		Comments: []Comment{{
			File:           "component.tsx",
			Line:           14,
			Severity:       "alta",
			DecisionReason: "o atributo min ficou inválido e quebra o HTML",
			Comment:        "corrija o atributo min",
		}},
		FinalReview: FinalReview{
			Summary:      "Aprovado sem ressalvas.",
			Observations: "Tudo certo.",
		},
	}

	applyPipelineDecision(&result, false, Policy{})

	if result.FinalReview.GiteaEvent != "REQUEST_CHANGES" || result.FinalReview.Status != "reprovado" {
		t.Fatalf("unexpected rejection decision: %#v", result.FinalReview)
	}
	if !strings.Contains(result.FinalReview.Summary, "precisa de correção") {
		t.Fatalf("unexpected normalized rejection summary: %q", result.FinalReview.Summary)
	}
	if !strings.Contains(result.FinalReview.Observations, "component.tsx:14") {
		t.Fatalf("rejection observations lost finding context: %q", result.FinalReview.Observations)
	}
}

func seededPipeline(t *testing.T) (*Repository, PipelineDefinition, func()) {
	t.Helper()
	db, err := databasepkg.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = db.Migrate(ctx); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Seed(ctx); err != nil {
		db.Close()
		t.Fatal(err)
	}
	repo := NewRepository(db.SQL)
	job := queue.ReviewJob{ReviewID: "rev-1", Owner: "acme", Repository: "app", PullRequest: 1}
	if err = repo.Create(ctx, job.ReviewID, job, "test"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	definition, err := repo.Pipeline(ctx, job.ReviewID, job)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	return repo, definition, func() { _ = db.Close() }
}

func pipelineWithPlannerFanOut(definition PipelineDefinition) PipelineDefinition {
	branch := definition.Stages[1]
	branch.ID = 999
	branch.Key = "planejamento-secundario"
	branch.Name = "Planejamento secundário"
	branch.Position = len(definition.Stages) + 1
	definition.Stages = append(definition.Stages, branch)

	branchID := branch.ID
	definition.Transitions = append(definition.Transitions, PipelineTransition{
		FromStageID:  definition.Stages[0].ID,
		ToStageID:    &branchID,
		Type:         "success",
		ConditionKey: "always",
		Priority:     1,
	})
	definition.Transitions = append(definition.Transitions, PipelineTransition{
		FromStageID:  branch.ID,
		ToStageID:    definition.Transitions[1].ToStageID,
		Type:         "success",
		ConditionKey: "always",
	})
	return definition
}

func containsEvent(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}
