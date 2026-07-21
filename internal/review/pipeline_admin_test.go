package review

import (
	"context"
	"errors"
	"testing"

	"gitea-agents/internal/config"
	"gitea-agents/internal/providers"
	"gitea-agents/internal/queue"
)

type pipelineNoopPublisher struct{}

func (pipelineNoopPublisher) Publish(context.Context, queue.ReviewJob) error { return nil }

func TestDraftValidatePublishAndExecutePipeline(t *testing.T) {
	repo, published, cleanup := seededPipeline(t)
	defer cleanup()
	ctx := context.Background()
	created, err := repo.CreatePipelineDraft(ctx, PipelineCreateInput{
		Key: "admin-global", IsDefault: true,
		PipelineDraftInput: PipelineDraftInput{Name: "Admin global", Stages: stageInputs(published), Transitions: transitionInputPtr(linearTransitions(published)), Triggers: []PipelineTrigger{{Source: "webhook", Enabled: true}, {Source: "api", Enabled: true}, {Source: "manual", Enabled: true}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	draft := created.Versions[0]
	if len(draft.Triggers) != 3 {
		t.Fatalf("administrative draft omitted triggers: %#v", draft)
	}
	if draft.Version != 1 || draft.Status != "draft" {
		t.Fatalf("unexpected draft: %#v", draft)
	}
	if err = repo.ValidatePipelineDraft(ctx, created.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.PublishPipelineDraft(ctx, created.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	cloned, err := repo.ClonePublishedPipeline(ctx, created.ID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	nextDraft := cloned.Versions[0]
	if nextDraft.Version != 2 || nextDraft.Status != "draft" {
		t.Fatalf("unexpected cloned draft: %#v", nextDraft)
	}
	if _, err = repo.PublishPipelineDraft(ctx, created.ID, nextDraft.ID); err != nil {
		t.Fatal(err)
	}
	var oldStatus string
	if err = repo.db.QueryRowContext(ctx, `SELECT status FROM pipeline_versions WHERE id=?`, draft.ID).Scan(&oldStatus); err != nil || oldStatus != "archived" {
		t.Fatalf("previous publication was not archived atomically: status=%s err=%v", oldStatus, err)
	}
	job := queue.ReviewJob{ReviewID: "admin-pipeline-run", Owner: "acme", Repository: "app", PullRequest: 2}
	if err = repo.Create(ctx, job.ReviewID, job, "test"); err != nil {
		t.Fatal(err)
	}
	definition, err := repo.Pipeline(ctx, job.ReviewID, job)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Key != "admin-global" || definition.VersionID != nextDraft.ID {
		t.Fatalf("review did not bind the published draft: %#v", definition)
	}
	provider := &pipelineProvider{calls: map[string]int{}}
	result, err := NewPipelineEngine(repo).Execute(ctx, definition, PipelineExecutionInput{
		Job: queueInput{ID: job.ReviewID}, RawDiff: "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n",
		Policy:   Policy{MinimumConfidence: .75, MaxParallelGroups: 1},
		Provider: func(context.Context, *int64) (providers.LLMProvider, error) { return provider, nil },
	})
	if err != nil || len(result.Comments) != 1 {
		t.Fatalf("published draft did not execute: result=%#v err=%v", result, err)
	}
}

func TestPipelineDefinitionRejectsCyclesAndIncompatibleContracts(t *testing.T) {
	_, definition, cleanup := seededPipeline(t)
	defer cleanup()
	cycle := definition
	cycle.Transitions = append(cycle.Transitions, PipelineTransition{FromStageID: cycle.Stages[len(cycle.Stages)-1].ID, ToStageID: &cycle.Stages[0].ID, Type: "success", ConditionKey: "always"})
	if err := validatePipelineDefinition(cycle); err == nil {
		t.Fatal("expected cycle rejection")
	}
	bad := definition
	bad.Transitions[0].ToStageID = &bad.Stages[2].ID
	if err := validatePipelineDefinition(bad); err == nil {
		t.Fatal("expected contract rejection")
	}
}

func TestDisabledTriggerRejectsBeforeReviewPersistence(t *testing.T) {
	repo, published, cleanup := seededPipeline(t)
	defer cleanup()
	ctx := context.Background()
	created, err := repo.CreatePipelineDraft(ctx, PipelineCreateInput{Key: "manual-disabled", IsDefault: true, PipelineDraftInput: PipelineDraftInput{Name: "Disabled", Stages: stageInputs(published), Triggers: []PipelineTrigger{{Source: "webhook", Enabled: true}, {Source: "api", Enabled: true}, {Source: "manual", Enabled: false}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.PublishPipelineDraft(ctx, created.ID, created.Versions[0].ID); err != nil {
		t.Fatal(err)
	}
	service := NewService(config.Config{}, repo, pipelineNoopPublisher{})
	_, err = service.Enqueue(ctx, queue.ReviewJob{ReviewID: "blocked", Owner: "acme", Repository: "app", PullRequest: 3}, "manual")
	if err == nil || !errors.Is(err, ErrNotFound) && err.Error() == "" {
		t.Fatalf("expected trigger rejection, got %v", err)
	}
	var count int
	if err = repo.db.QueryRowContext(ctx, `SELECT count(*) FROM reviews WHERE id='blocked'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("disabled trigger persisted a review: count=%d err=%v", count, err)
	}
}

func TestEntrypointTargetsAndPositionsSurviveClone(t *testing.T) {
	repo, published, cleanup := seededPipeline(t)
	defer cleanup()
	ctx := context.Background()
	triggers := []PipelineTrigger{
		{Source: "webhook", Enabled: true, TargetStageKey: published.Stages[0].Key, Config: map[string]any{"x": -420, "y": -180}},
		{Source: "api", Enabled: true, TargetStageKey: published.Stages[1].Key, Config: map[string]any{"x": -120, "y": 80}},
		{Source: "manual", Enabled: true, TargetStageKey: published.Stages[2].Key, Config: map[string]any{"x": 240, "y": 320}},
	}
	created, err := repo.CreatePipelineDraft(ctx, PipelineCreateInput{
		Key: "custom-entrypoints",
		PipelineDraftInput: PipelineDraftInput{
			Name: "Custom entrypoints", Stages: stageInputs(published), Triggers: triggers,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	draft := created.Versions[0]
	assertEntrypoints := func(items []PipelineTrigger) {
		t.Helper()
		for _, expected := range triggers {
			var got *PipelineTrigger
			for index := range items {
				if items[index].Source == expected.Source {
					got = &items[index]
					break
				}
			}
			if got == nil || got.TargetStageKey != expected.TargetStageKey || got.Config["x"] != float64(expected.Config["x"].(int)) || got.Config["y"] != float64(expected.Config["y"].(int)) {
				t.Fatalf("entrypoint mismatch: got=%#v want=%#v", got, expected)
			}
		}
	}
	assertEntrypoints(draft.Triggers)
	if _, err = repo.PublishPipelineDraft(ctx, created.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	cloned, err := repo.ClonePublishedPipeline(ctx, created.ID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEntrypoints(cloned.Versions[0].Triggers)
}

func TestDynamicWorkflowFieldsSurviveAdminRoundTripAndClone(t *testing.T) {
	repo, published, cleanup := seededPipeline(t)
	defer cleanup()
	ctx := context.Background()
	stages := stageInputs(published)
	stages[0].RouteMode = "first_match"
	stages[1].JoinMode = "each_arrival"
	transitions := linearTransitions(published)
	transitions[0].Rule = &Rule{Operator: RuleExists, Path: "files", Value: true}
	transitions[0].MaxTraversals = 3
	created, err := repo.CreatePipelineDraft(ctx, PipelineCreateInput{
		Key: "dynamic-roundtrip",
		PipelineDraftInput: PipelineDraftInput{
			Name: "Dynamic roundtrip", SchedulerMaxRuns: 41, Stages: stages, Transitions: &transitions,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	draft := created.Versions[0]
	if draft.SchedulerMaxRuns != 41 || draft.Stages[0].RouteMode != "first_match" || draft.Stages[1].JoinMode != "each_arrival" {
		t.Fatalf("workflow modes did not roundtrip: %#v", draft)
	}
	if draft.Transitions[0].Rule == nil || draft.Transitions[0].Rule.Operator != RuleExists || draft.Transitions[0].MaxTraversals != 3 {
		t.Fatalf("transition rule did not roundtrip: %#v", draft.Transitions[0])
	}
	if _, err = repo.PublishPipelineDraft(ctx, created.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	cloned, err := repo.ClonePublishedPipeline(ctx, created.ID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	clone := cloned.Versions[0]
	if clone.SchedulerMaxRuns != 41 || clone.Stages[0].RouteMode != "first_match" || clone.Transitions[0].Rule == nil || clone.Transitions[0].MaxTraversals != 3 {
		t.Fatalf("dynamic fields did not survive clone: %#v", clone)
	}
}

func TestWorkflowCatalogIsSystemControlledAndMarksUnsupportedModes(t *testing.T) {
	repo, _, cleanup := seededPipeline(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, `INSERT INTO stage_contracts(key,version,is_system) VALUES('user-contract',1,0);
		INSERT INTO workflow_entrypoint_catalog(key,display_name,adapter_key) VALUES('custom','Custom','unregistered');`); err != nil {
		t.Fatal(err)
	}
	catalog, err := repo.WorkflowCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Contracts) != 9 || len(catalog.Processors) != 4 || len(catalog.Entrypoints) != 3 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	for _, contract := range catalog.Contracts {
		if contract.Key == "user-contract" {
			t.Fatal("non-system contract was exposed")
		}
	}
	for _, processor := range catalog.Processors {
		if (processor.Key == "rule_filter" || processor.Key == "transform_merge") && processor.Executable {
			t.Fatalf("unsupported processor was exposed as executable: %#v", processor)
		}
	}
	if catalog.JoinModes[0].Key != "each_arrival" || !catalog.JoinModes[0].Executable || catalog.JoinModes[1].Executable || catalog.JoinModes[2].Executable {
		t.Fatalf("unexpected join mode capabilities: %#v", catalog.JoinModes)
	}
}

func TestPipelineSelectionUsesProfileThenGlobalFallback(t *testing.T) {
	repo, global, cleanup := seededPipeline(t)
	defer cleanup()
	ctx := context.Background()
	result, err := repo.db.ExecContext(ctx, `INSERT INTO review_profiles(name,is_default,is_enabled) VALUES('Team',0,1)`)
	if err != nil {
		t.Fatal(err)
	}
	profileID, _ := result.LastInsertId()
	profilePipeline, err := repo.CreatePipelineDraft(ctx, PipelineCreateInput{
		Key: "team-pipeline", IsDefault: true,
		PipelineDraftInput: PipelineDraftInput{Name: "Team pipeline", ProfileID: &profileID, Stages: stageInputs(global)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.PublishPipelineDraft(ctx, profilePipeline.ID, profilePipeline.Versions[0].ID); err != nil {
		t.Fatal(err)
	}
	selected, err := repo.SelectedPipeline(ctx, &profileID)
	if err != nil || selected.ID != profilePipeline.ID {
		t.Fatalf("profile selection failed: %#v %v", selected, err)
	}
	orphan := int64(9999)
	selected, err = repo.SelectedPipeline(ctx, &orphan)
	if err != nil || selected.ID != global.ID {
		t.Fatalf("global fallback failed: %#v %v", selected, err)
	}
}

func TestDraftValidationRejectsRequiredStageOrder(t *testing.T) {
	repo, definition, cleanup := seededPipeline(t)
	defer cleanup()
	ctx := context.Background()
	inputs := stageInputs(definition)
	inputs[4], inputs[5] = inputs[5], inputs[4]
	created, err := repo.CreatePipelineDraft(ctx, PipelineCreateInput{Key: "invalid-order", PipelineDraftInput: PipelineDraftInput{Name: "Invalid", Stages: inputs}})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.ValidatePipelineDraft(ctx, created.ID, created.Versions[0].ID); err == nil {
		t.Fatal("expected formatting before verification to be rejected")
	}
}

func TestDraftProfileChangeOnlyTakesEffectWhenPublished(t *testing.T) {
	repo, global, cleanup := seededPipeline(t)
	defer cleanup()
	ctx := context.Background()
	result, err := repo.db.ExecContext(ctx, `INSERT INTO review_profiles(name,is_default,is_enabled) VALUES('Moved',0,1)`)
	if err != nil {
		t.Fatal(err)
	}
	profileID, _ := result.LastInsertId()
	draft, err := repo.ClonePublishedPipeline(ctx, global.ID, global.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	version := draft.Versions[0]
	if _, err = repo.UpdatePipelineDraft(ctx, global.ID, version.ID, PipelineDraftInput{Name: global.Name, Description: global.Name, ProfileID: &profileID, Stages: stageInputs(global)}); err != nil {
		t.Fatal(err)
	}
	if selected, err := repo.SelectedPipeline(ctx, &profileID); err != nil || selected.ID != global.ID {
		t.Fatalf("draft retargeted a published pipeline: %#v %v", selected, err)
	}
	if _, err = repo.PublishPipelineDraft(ctx, global.ID, version.ID); err != nil {
		t.Fatal(err)
	}
	var selectedProfile int64
	if err = repo.db.QueryRowContext(ctx, `SELECT profile_id FROM pipeline_definitions WHERE id=?`, global.ID).Scan(&selectedProfile); err != nil || selectedProfile != profileID {
		t.Fatalf("published draft did not move pipeline profile: profile=%d err=%v", selectedProfile, err)
	}
}

func stageInputs(definition PipelineDefinition) []PipelineStageInput {
	inputs := make([]PipelineStageInput, len(definition.Stages))
	for i, stage := range definition.Stages {
		inputs[i] = PipelineStageInput{StageTypeKey: stage.StageTypeKey, Key: stage.Key, Name: stage.Name, Prompt: stage.PromptTemplate, ModelID: stage.ModelID, MaxTokens: stage.MaxOutputTokens, RetryLimit: stage.RetryLimit, Timeout: stage.TimeoutSeconds, UseLLM: stage.UseLLM, Required: stage.Required, RouteMode: stage.RouteMode, JoinMode: stage.JoinMode, Config: stage.Config}
	}
	return inputs
}

func linearTransitions(definition PipelineDefinition) []PipelineTransitionInput {
	out := make([]PipelineTransitionInput, 0, len(definition.Stages)-1)
	for i := 0; i+1 < len(definition.Stages); i++ {
		out = append(out, PipelineTransitionInput{FromStageKey: definition.Stages[i].Key, ToStageKey: definition.Stages[i+1].Key, Type: "success", ConditionKey: "always"})
	}
	return out
}
func transitionInputPtr(items []PipelineTransitionInput) *[]PipelineTransitionInput { return &items }
