package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"forgereview/backend/internal/integration"
)

type memoryIntegrations map[string]integration.Integration
type memoryModelProfiles map[string]integration.ModelProfile
type memoryWorkflows struct {
	definitions map[int64]Definition
	statuses    map[int64]string
}

func (items memoryWorkflows) Load(_ context.Context, id int64) (Definition, error) {
	item, ok := items.definitions[id]
	if !ok {
		return Definition{}, errors.New("not found")
	}
	return item, nil
}

func (items memoryWorkflows) VersionStatus(_ context.Context, id int64) (string, error) {
	status, ok := items.statuses[id]
	if !ok {
		return "", errors.New("not found")
	}
	return status, nil
}

type memoryCache struct {
	values  map[string][]byte
	ttls    map[string]time.Duration
	deleted []string
}

func (c *memoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, found := c.values[key]
	return value, found, nil
}

func (c *memoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if c.values == nil {
		c.values = map[string][]byte{}
		c.ttls = map[string]time.Duration{}
	}
	c.values[key] = value
	c.ttls[key] = ttl
	return nil
}

func (c *memoryCache) Delete(_ context.Context, key string) error {
	delete(c.values, key)
	c.deleted = append(c.deleted, key)
	return nil
}

func TestRuntimeObservesRunningAndTerminalNodeProgress(t *testing.T) {
	definition := Definition{Key: "progress", Name: "Progress", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "transform", Type: "transform", Name: "Transform"},
	}, Edges: []Edge{{Key: "event", FromNode: "start", FromPort: "event", ToNode: "transform", ToPort: "input"}}}
	observed := []NodeRun{}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), map[string]any{"value": 1}, Adapters{Progress: ProgressObserverFunc(func(_ context.Context, run NodeRun) error {
		observed = append(observed, run)
		return nil
	})})
	if err != nil || report.Status != "completed" {
		t.Fatalf("run = %#v, %v", report, err)
	}
	if len(observed) != 4 {
		t.Fatalf("observed = %#v", observed)
	}
	for index, want := range []struct{ node, status, scope string }{{"start", "running", "root"}, {"start", "completed", "root"}, {"transform", "running", "root"}, {"transform", "completed", "root"}} {
		if observed[index].NodeKey != want.node || observed[index].Status != want.status || observed[index].ScopeKey != want.scope {
			t.Fatalf("progress %d = %#v; want %#v", index, observed[index], want)
		}
	}
}

func TestTemplateIncludesItsIncomingContextInPrompt(t *testing.T) {
	outputs, err := execute(context.Background(), Node{Key: "template", Type: "template", Config: map[string]any{"template": "Revise {{context}} e responda JSON."}}, map[string][]any{"context": {map[string]any{"filename": "api.go", "diff": "+ fix"}}}, nil, Adapters{}, "root")
	if err != nil {
		t.Fatal(err)
	}
	prompt := outputs["prompt"].(string)
	for _, want := range []string{"Contexto real para a tarefa", "api.go", "+ fix", "responda JSON"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
	if strings.Contains(prompt, "{{context}}") {
		t.Fatalf("unexpanded context placeholder: %s", prompt)
	}
}

const workflowTestEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func testSecrets(t *testing.T) *integration.EncryptedSecrets {
	t.Helper()
	secrets, err := integration.NewEncryptedSecrets(workflowTestEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	return secrets
}

func encryptedIntegration(t *testing.T, item integration.Integration, value string) integration.Integration {
	t.Helper()
	ciphertext, err := testSecrets(t).Encrypt(item.Key, value)
	if err != nil {
		t.Fatal(err)
	}
	item.SecretCiphertext = ciphertext
	return item
}

func (items memoryIntegrations) Integration(_ context.Context, key string) (integration.Integration, error) {
	item, ok := items[key]
	if !ok {
		return integration.Integration{}, errors.New("not found")
	}
	return item, nil
}

func (profiles memoryModelProfiles) ModelProfile(_ context.Context, key string) (integration.ModelProfile, error) {
	profile, ok := profiles[key]
	if !ok {
		return integration.ModelProfile{}, errors.New("not found")
	}
	return profile, nil
}

func TestRunExecutesReadyNodesAndRoutesTokens(t *testing.T) {
	definition := Definition{
		Key: "test", Name: "Test",
		Nodes: []Node{
			{Key: "trigger", Type: "trigger", Name: "Trigger"},
			{Key: "transform", Type: "transform", Name: "Transform"},
			{Key: "log", Type: "log", Name: "Log"},
		},
		Edges: []Edge{
			{Key: "event", FromNode: "trigger", FromPort: "event", ToNode: "transform", ToPort: "input"},
			{Key: "output", FromNode: "transform", FromPort: "output", ToNode: "log", ToPort: "input"},
		},
	}
	report, err := Run(context.Background(), definition, DefaultCatalog(), map[string]any{"pull_request": 42})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Status != "completed" || len(report.Runs) != 3 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.Runs[2].Outputs[0].Value.(map[string]any)["pull_request"] != 42 {
		t.Fatalf("expected original event to reach log")
	}
}

func TestMergeWaitsForAndReturnsAllIncomingValues(t *testing.T) {
	definition := Definition{
		Key: "join", Name: "Join",
		Nodes: []Node{{Key: "trigger", Type: "trigger", Name: "Trigger"}, {Key: "left", Type: "transform", Name: "Left"}, {Key: "right", Type: "transform", Name: "Right"}, {Key: "merge", Type: "merge", Name: "Merge"}},
		Edges: []Edge{
			{Key: "left-in", FromNode: "trigger", FromPort: "event", ToNode: "left", ToPort: "input"},
			{Key: "right-in", FromNode: "trigger", FromPort: "event", ToNode: "right", ToPort: "input"},
			{Key: "left-join", FromNode: "left", FromPort: "output", ToNode: "merge", ToPort: "inputs"},
			{Key: "right-join", FromNode: "right", FromPort: "output", ToNode: "merge", ToPort: "inputs"},
		},
	}
	report, err := Run(context.Background(), definition, DefaultCatalog(), map[string]any{"value": 1})
	if err != nil {
		t.Fatalf("run merge: %v", err)
	}
	last := report.Runs[len(report.Runs)-1]
	joined, ok := last.Outputs[0].Value.([]any)
	if !ok || len(joined) != 2 {
		t.Fatalf("merge output = %#v", last.Outputs[0].Value)
	}
}

func TestRunRoutesConditionToSelectedPort(t *testing.T) {
	definition := Definition{
		Key: "condition", Name: "Condition",
		Nodes: []Node{
			{Key: "trigger", Type: "trigger", Name: "Trigger", Config: map[string]any{"event": "php"}},
			{Key: "condition", Type: "condition", Name: "Condition", Config: map[string]any{"equals": "php"}},
			{Key: "log", Type: "log", Name: "Selected"},
		},
		Edges: []Edge{
			{Key: "in", FromNode: "trigger", FromPort: "event", ToNode: "condition", ToPort: "input"},
			{Key: "true", FromNode: "condition", FromPort: "true", ToNode: "log", ToPort: "input"},
		},
	}
	report, err := Run(context.Background(), definition, DefaultCatalog(), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(report.Runs) != 3 || report.Runs[2].NodeKey != "log" {
		t.Fatalf("true branch did not run: %#v", report.Runs)
	}
}

func TestDeclarativeTransformAndNamespacedVariableExecution(t *testing.T) {
	transform := Node{Key: "transform", Type: "transform", Config: map[string]any{"operations": []any{
		map[string]any{"op": "rename", "path": "pull.owner", "to": "repository.owner"},
		map[string]any{"op": "set", "path": "policy.severity", "value": "high"},
		map[string]any{"op": "remove", "path": "secret"},
	}}}
	outputs, err := execute(context.Background(), transform, map[string][]any{"input": {map[string]any{"pull": map[string]any{"owner": "acme"}, "secret": "remove"}}}, nil, Adapters{}, rootScope)
	if err != nil {
		t.Fatal(err)
	}
	value := outputs["output"].(map[string]any)
	if value["repository"].(map[string]any)["owner"] != "acme" || value["policy"].(map[string]any)["severity"] != "high" {
		t.Fatalf("transformed value = %#v", value)
	}
	if _, exists := value["secret"]; exists {
		t.Fatalf("remove operation kept secret: %#v", value)
	}

	definition := Definition{Key: "variables", Name: "Variables", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "set", Type: "variable", Name: "Set", Config: map[string]any{"action": "set", "namespace": "execution", "name": "decision"}},
		{Key: "get", Type: "variable", Name: "Get", Config: map[string]any{"action": "get", "namespace": "execution", "name": "decision"}},
	}, Edges: []Edge{
		{Key: "set-value", FromNode: "start", FromPort: "event", ToNode: "set", ToPort: "value"},
		{Key: "get-after-set", FromNode: "set", FromPort: "value", ToNode: "get", ToPort: "value"},
	}}
	report, err := Run(context.Background(), definition, DefaultCatalog(), map[string]any{"result": "safe"})
	if err != nil {
		t.Fatal(err)
	}
	if got := outputFor(t, report, "get", "value").(map[string]any)["result"]; got != "safe" {
		t.Fatalf("namespaced variable = %#v", got)
	}
}

func TestConditionMultipleBranchesAndMergePolicies(t *testing.T) {
	t.Run("multiple branch skips non-selected route", func(t *testing.T) {
		definition := Definition{Key: "branches", Name: "Branches", Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": map[string]any{"language": "go"}}},
			{Key: "condition", Type: "condition", Name: "Condition", Config: map[string]any{"branches": []any{
				map[string]any{"port": "match_1", "path": "language", "operator": "equals", "value": "go"},
				map[string]any{"port": "match_2", "path": "language", "operator": "equals", "value": "typescript"},
			}}},
			{Key: "go", Type: "transform", Name: "Go"},
			{Key: "typescript", Type: "transform", Name: "TypeScript"},
		}, Edges: []Edge{
			{Key: "input", FromNode: "start", FromPort: "event", ToNode: "condition", ToPort: "input"},
			{Key: "go", FromNode: "condition", FromPort: "match_1", ToNode: "go", ToPort: "input"},
			{Key: "ts", FromNode: "condition", FromPort: "match_2", ToNode: "typescript", ToPort: "input"},
		}}
		report, err := Run(context.Background(), definition, DefaultCatalog(), nil)
		if err != nil || len(nodeScopes(report, "go")) != 1 || len(nodeScopes(report, "typescript")) != 0 {
			t.Fatalf("branch report = %#v, %v", report, err)
		}
	})

	t.Run("any returns first arrival", func(t *testing.T) {
		definition := mergePolicyDefinition("any", 0, 0)
		report, err := Run(context.Background(), definition, DefaultCatalog(), map[string]any{"value": 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := outputFor(t, report, "merge", "output").(map[string]any); !ok {
			t.Fatalf("any output = %#v", outputFor(t, report, "merge", "output"))
		}
	})

	t.Run("quorum runs at threshold", func(t *testing.T) {
		definition := mergePolicyDefinition("quorum", 2, 0)
		report, err := Run(context.Background(), definition, DefaultCatalog(), map[string]any{"value": 1})
		if err != nil {
			t.Fatal(err)
		}
		values := outputFor(t, report, "merge", "output").([]any)
		if len(values) != 2 {
			t.Fatalf("quorum output = %#v", values)
		}
	})

	t.Run("timeout releases partial all join", func(t *testing.T) {
		definition := Definition{Key: "timeout", Name: "Timeout", Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": "go"}},
			{Key: "condition", Type: "condition", Name: "Condition", Config: map[string]any{"branches": []any{map[string]any{"port": "match_1", "operator": "equals", "value": "go"}}}},
			{Key: "merge", Type: "merge", Name: "Merge", Config: map[string]any{"mode": "all", "timeout_ms": 1}},
		}, Edges: []Edge{
			{Key: "input", FromNode: "start", FromPort: "event", ToNode: "condition", ToPort: "input"},
			{Key: "one", FromNode: "condition", FromPort: "match_1", ToNode: "merge", ToPort: "inputs"},
			{Key: "two", FromNode: "condition", FromPort: "match_2", ToNode: "merge", ToPort: "inputs"},
		}}
		report, err := Run(context.Background(), definition, DefaultCatalog(), nil)
		if err != nil {
			t.Fatal(err)
		}
		run := nodeRunFor(t, report, "merge", rootScope)
		if run.Metadata["timed_out"] != true || len(run.Outputs[0].Value.([]any)) != 1 {
			t.Fatalf("timeout merge = %#v", run)
		}
	})
}

func mergePolicyDefinition(mode string, quorum, timeout int) Definition {
	config := map[string]any{"mode": mode}
	if quorum > 0 {
		config["quorum"] = quorum
	}
	if timeout > 0 {
		config["timeout_ms"] = timeout
	}
	return Definition{Key: mode, Name: mode, Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "one", Type: "transform", Name: "One"},
		{Key: "two", Type: "transform", Name: "Two"},
		{Key: "three", Type: "transform", Name: "Three"},
		{Key: "merge", Type: "merge", Name: "Merge", Config: config},
	}, Edges: []Edge{
		{Key: "one-in", FromNode: "start", FromPort: "event", ToNode: "one", ToPort: "input"},
		{Key: "two-in", FromNode: "start", FromPort: "event", ToNode: "two", ToPort: "input"},
		{Key: "three-in", FromNode: "start", FromPort: "event", ToNode: "three", ToPort: "input"},
		{Key: "one-out", FromNode: "one", FromPort: "output", ToNode: "merge", ToPort: "inputs"},
		{Key: "two-out", FromNode: "two", FromPort: "output", ToNode: "merge", ToPort: "inputs"},
		{Key: "three-out", FromNode: "three", FromPort: "output", ToNode: "merge", ToPort: "inputs"},
	}}
}

func TestPublishedSubpipelineRunsByPinnedInterface(t *testing.T) {
	child := Definition{Key: "child", Name: "Child", Interface: &WorkflowInterface{
		Inputs:  []InterfaceField{{Key: "payload", Contract: "any", Required: true}},
		Outputs: []InterfaceField{{Key: "result", Contract: "any", Required: true, NodeKey: "transform", PortKey: "output"}},
	}, Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "transform", Type: "transform", Name: "Transform"},
	}, Edges: []Edge{{Key: "input", FromNode: "start", FromPort: "event", ToNode: "transform", ToPort: "input"}}}
	parent := Definition{Key: "parent", Name: "Parent", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "child", Type: "workflow", Name: "Child", Config: map[string]any{"workflow_key": "child", "workflow_version_id": 2}},
	}, Edges: []Edge{{Key: "input", FromNode: "start", FromPort: "event", ToNode: "child", ToPort: "input"}}}
	resolver := memoryWorkflows{definitions: map[int64]Definition{2: child}, statuses: map[int64]string{2: VersionStatusPublished}}
	report, err := RunWithAdapters(context.Background(), parent, DefaultCatalog(), map[string]any{"payload": "ok"}, Adapters{Workflows: resolver, Execution: ExecutionContext{VersionID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	result := outputFor(t, report, "child", "output").(map[string]any)
	if result["payload"] != "ok" {
		t.Fatalf("subpipeline output = %#v", result)
	}
	parent.Nodes[1].Config["workflow_version_id"] = 1
	if _, err = RunWithAdapters(context.Background(), parent, DefaultCatalog(), nil, Adapters{Workflows: resolver, Execution: ExecutionContext{VersionID: 1}}); err == nil {
		t.Fatal("expected recursive subpipeline to be rejected")
	}
}

func TestRunLoopExecutesGroupsInDistinctScopesAndAggregatesTerminalBranches(t *testing.T) {
	files := []map[string]any{
		{"filename": "a.go", "patch": "a"},
		{"filename": "b.go", "patch": "b"},
	}
	definition := Definition{Key: "scoped-loop", Name: "Scoped loop", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": files}},
		{Key: "prepare", Type: "transform", Name: "Prepare"},
		{Key: "group", Type: "group", Name: "Group", Config: map[string]any{"max_files": 1, "max_characters": 10}},
		{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 2, "concurrency": 1}},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review group"}},
		{Key: "transform", Type: "transform", Name: "Transform"},
	}, Edges: []Edge{
		{Key: "event", FromNode: "start", FromPort: "event", ToNode: "prepare", ToPort: "input"},
		{Key: "files", FromNode: "prepare", FromPort: "output", ToNode: "group", ToPort: "files"},
		{Key: "groups", FromNode: "group", FromPort: "groups", ToNode: "loop", ToPort: "items"},
		{Key: "context", FromNode: "loop", FromPort: "item", ToNode: "template", ToPort: "context"},
		{Key: "input", FromNode: "loop", FromPort: "item", ToNode: "transform", ToPort: "input"},
	}}
	report, err := Run(context.Background(), definition, DefaultCatalog(), nil)
	if err != nil {
		t.Fatalf("run scoped loop: %v", err)
	}
	if report.Status != "completed" {
		t.Fatalf("report status = %q", report.Status)
	}
	templateScopes := nodeScopes(report, "template")
	transformScopes := nodeScopes(report, "transform")
	if len(templateScopes) != 2 || len(transformScopes) != 2 || templateScopes[0] != "loop:000001" || templateScopes[1] != "loop:000002" || transformScopes[0] != "loop:000001" || transformScopes[1] != "loop:000002" {
		t.Fatalf("scoped runs = %#v", report.Runs)
	}
	loopRun := nodeRunFor(t, report, "loop", "root")
	if loopRun.Metadata["concurrency"] != 1 || loopRun.Metadata["completed_iterations"] != 2 {
		t.Fatalf("loop metadata = %#v", loopRun.Metadata)
	}
	results := outputFor(t, report, "loop", "results").([]any)
	if len(results) != 4 || !strings.HasPrefix(results[0].(string), "review group\n\nContexto real para a tarefa") || !strings.HasPrefix(results[2].(string), "review group\n\nContexto real para a tarefa") {
		t.Fatalf("loop results = %#v", results)
	}
	if first, ok := results[1].(FileGroup); !ok || first.Files[0]["filename"] != "a.go" {
		t.Fatalf("first scoped group = %#v", results[1])
	}
	if second, ok := results[3].(FileGroup); !ok || second.Files[0]["filename"] != "b.go" {
		t.Fatalf("second scoped group = %#v", results[3])
	}
}

func TestRunLoopRejectsItemsAboveConfiguredMaximum(t *testing.T) {
	definition := Definition{Key: "loop-limit", Name: "Loop limit", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": []any{"one", "two"}}},
		{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 1}},
	}, Edges: []Edge{{Key: "items", FromNode: "start", FromPort: "event", ToNode: "loop", ToPort: "items"}}}
	report, err := Run(context.Background(), definition, DefaultCatalog(), nil)
	if err == nil || err.Error() != `card "loop" execution failed` {
		t.Fatalf("loop limit error = %v", err)
	}
	if report.Status != "failed" || len(report.Runs) != 2 || report.Runs[1].ScopeKey != "root" {
		t.Fatalf("loop limit report = %#v", report)
	}
}

func TestRunRecordsDiagnosticErrorForExecutionLogs(t *testing.T) {
	base := func(policy string) Definition {
		return Definition{Key: policy, Name: policy, Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start"},
			{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"on_error": policy}},
		}, Edges: []Edge{{Key: "context", FromNode: "start", FromPort: "event", ToNode: "template", ToPort: "context"}}}
	}
	t.Run("fail", func(t *testing.T) {
		report, err := Run(context.Background(), base("fail"), DefaultCatalog(), nil)
		if err == nil || err.Error() != `card "template" execution failed` || report.Status != "failed" {
			t.Fatalf("fail policy = %#v, %v", report, err)
		}
		run := nodeRunFor(t, report, "template", "root")
		if run.Error != `template card "template" requires config.template` || run.Metadata["error_code"] != "execution_failed" || run.Metadata["error_scope"] != "root" {
			t.Fatalf("diagnostic failure report = %#v", run)
		}
	})
	t.Run("continue", func(t *testing.T) {
		report, err := Run(context.Background(), base("continue"), DefaultCatalog(), nil)
		if err != nil || report.Status != "completed" {
			t.Fatalf("continue policy = %#v, %v", report, err)
		}
		run := nodeRunFor(t, report, "template", "root")
		if run.Status != "failed" || run.Metadata["error_action"] != "continued" {
			t.Fatalf("continue run = %#v", run)
		}
	})
}

func TestRunRoutesTypedErrorToFallback(t *testing.T) {
	definition := Definition{Key: "routed", Name: "Routed", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"on_error": "route"}},
		{Key: "control", Type: "error_control", Name: "Control", Config: map[string]any{"on_error": "fallback", "fallback_result": "safe result"}},
	}, Edges: []Edge{
		{Key: "context", FromNode: "start", FromPort: "event", ToNode: "template", ToPort: "context"},
		{Key: "error", FromNode: "template", FromPort: "error", ToNode: "control", ToPort: "error"},
	}}
	report, err := Run(context.Background(), definition, DefaultCatalog(), nil)
	if err != nil || report.Status != "completed" {
		t.Fatalf("routed fallback = %#v, %v", report, err)
	}
	template := nodeRunFor(t, report, "template", "root")
	if template.Metadata["error_action"] != "routed" || template.Metadata["error_scope"] != "root" || template.Outputs[0].Value.(ErrorToken).ScopeKey != "root" {
		t.Fatalf("routed token = %#v", template)
	}
	if got := outputFor(t, report, "control", "recovered"); got != "safe result" {
		t.Fatalf("fallback output = %#v", got)
	}
}

func TestRunRoutesFallbackInsideLoopScope(t *testing.T) {
	definition := Definition{Key: "scoped-route", Name: "Scoped route", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": []any{"one", "two"}}},
		{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 2}},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"on_error": "route"}},
		{Key: "control", Type: "error_control", Name: "Control", Config: map[string]any{"on_error": "fallback", "fallback_result": "fallback"}},
	}, Edges: []Edge{
		{Key: "items", FromNode: "start", FromPort: "event", ToNode: "loop", ToPort: "items"},
		{Key: "context", FromNode: "loop", FromPort: "item", ToNode: "template", ToPort: "context"},
		{Key: "error", FromNode: "template", FromPort: "error", ToNode: "control", ToPort: "error"},
	}}
	report, err := Run(context.Background(), definition, DefaultCatalog(), nil)
	if err != nil || report.Status != "completed" {
		t.Fatalf("scoped fallback = %#v, %v", report, err)
	}
	if scopes := nodeScopes(report, "control"); len(scopes) != 2 || scopes[0] != "loop:000001" || scopes[1] != "loop:000002" {
		t.Fatalf("fallback scopes = %#v", report.Runs)
	}
	if run := nodeRunFor(t, report, "template", "loop:000001"); run.Metadata["error_scope"] != "loop:000001" {
		t.Fatalf("scoped error metadata = %#v", run.Metadata)
	}
	if results := outputFor(t, report, "loop", "results").([]any); len(results) != 2 || results[0] != "fallback" || results[1] != "fallback" {
		t.Fatalf("scoped fallback results = %#v", results)
	}
}

func TestRunLoopPartialFailureContinuesRemainingScopesAndReturnsAggregate(t *testing.T) {
	definition := Definition{Key: "loop-partial", Name: "Loop partial", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": []any{"one", "two"}}},
		{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 2, "on_error": "partial"}},
		{Key: "template", Type: "template", Name: "Template"},
	}, Edges: []Edge{
		{Key: "items", FromNode: "start", FromPort: "event", ToNode: "loop", ToPort: "items"},
		{Key: "context", FromNode: "loop", FromPort: "item", ToNode: "template", ToPort: "context"},
	}}
	report, err := Run(context.Background(), definition, DefaultCatalog(), nil)
	if err != nil || report.Status != "completed" {
		t.Fatalf("partial loop = %#v, %v", report, err)
	}
	if scopes := nodeScopes(report, "template"); len(scopes) != 2 || scopes[0] != "loop:000001" || scopes[1] != "loop:000002" {
		t.Fatalf("partial failure did not run both scopes: %#v", report.Runs)
	}
	loopRun := nodeRunFor(t, report, "loop", "root")
	if loopRun.Metadata["failed_iterations"] != 2 || loopRun.Metadata["completed_iterations"] != 0 {
		t.Fatalf("partial loop metadata = %#v", loopRun.Metadata)
	}
	if results := outputFor(t, report, "loop", "results").([]any); len(results) != 0 {
		t.Fatalf("partial loop results = %#v", results)
	}
}

type trackingModel struct {
	mu          sync.Mutex
	inFlight    int
	maxInFlight int
	calls       int
	block       time.Duration
}

type deadlineModel struct {
	deadline time.Time
	config   map[string]string
}

type profileFallbackModel struct {
	calls []string
}

func (model *profileFallbackModel) Chat(_ context.Context, item integration.Integration, _ string, _ string) (integration.ChatResult, error) {
	config, _ := item.ConfigValues()
	model.calls = append(model.calls, config["model"])
	if config["model"] == "primary" {
		return integration.ChatResult{}, errors.New("provider unavailable")
	}
	return integration.ChatResult{Content: "fallback response", Model: config["model"], Usage: integration.TokenUsage{Prompt: 10, Completion: 5, Total: 15}}, nil
}

func (m *deadlineModel) Chat(ctx context.Context, item integration.Integration, _ string, _ string) (integration.ChatResult, error) {
	m.deadline, _ = ctx.Deadline()
	m.config, _ = item.ConfigValues()
	return integration.ChatResult{Content: "ok"}, nil
}

func TestModelSettingsValidateSamplingTimeoutAndKeepAlive(t *testing.T) {
	valid := Node{Key: "model", Config: map[string]any{
		"temperature":     0.3,
		"top_p":           0.8,
		"timeout_seconds": 45,
		"keep_alive":      "10m",
	}}
	settings, err := modelSettingsFor(valid)
	if err != nil || *settings.Temperature != 0.3 || *settings.TopP != 0.8 || settings.Timeout != 45 || settings.KeepAlive != "10m" {
		t.Fatalf("settings = %#v, %v", settings, err)
	}
	for key, value := range map[string]any{
		"temperature":     2.1,
		"top_p":           0,
		"timeout_seconds": 3601,
		"keep_alive":      "25h",
	} {
		node := Node{Key: "model", Config: map[string]any{key: value}}
		if _, err := modelSettingsFor(node); err == nil {
			t.Fatalf("expected %s=%v to be rejected", key, value)
		}
	}
}

func TestModelResponseAppliesRequestSettingsAndDeadline(t *testing.T) {
	config, _ := json.Marshal(map[string]string{"base_url": "https://model.example", "model": "reviewer"})
	client := &deadlineModel{}
	node := Node{Key: "model", Type: "model", Config: map[string]any{
		"integration":     "model",
		"temperature":     0.4,
		"top_p":           0.7,
		"timeout_seconds": 2,
		"keep_alive":      "5m",
	}}
	_, err := modelResponse(context.Background(), node, "review", Adapters{
		Integrations: memoryIntegrations{"model": encryptedIntegration(t, integration.Integration{Key: "model", Name: "Model", Type: integration.TypeOllama, Config: config, Status: integration.StatusActive}, "secret")},
		Secrets:      testSecrets(t),
		Ollama:       client,
	})
	if err != nil {
		t.Fatal(err)
	}
	remaining := time.Until(client.deadline)
	if remaining <= 0 || remaining > 2*time.Second {
		t.Fatalf("deadline remaining = %s", remaining)
	}
	for key, want := range map[string]string{"temperature": "0.4", "top_p": "0.7", "keep_alive": "5m", "max_tokens": "2000"} {
		if client.config[key] != want {
			t.Fatalf("config[%s] = %q, want %q", key, client.config[key], want)
		}
	}
}

func TestModelFallbackAndCostBudget(t *testing.T) {
	config, _ := json.Marshal(map[string]string{"base_url": "https://model.example"})
	client := &profileFallbackModel{}
	telemetry := NewExecutionTelemetry(time.Now())
	node := Node{Key: "model", Type: "model", Config: map[string]any{
		"model_profile":               "primary",
		"fallback_model_profile":      "fallback",
		"max_tokens":                  100,
		"input_cost_per_million_usd":  1.0,
		"output_cost_per_million_usd": 2.0,
		"max_cost_usd":                0.001,
	}}
	result, err := modelResponse(context.Background(), node, "review", Adapters{
		Integrations: memoryIntegrations{"provider": encryptedIntegration(t, integration.Integration{Key: "provider", Name: "Provider", Type: integration.TypeOpenAI, Config: config, Status: integration.StatusActive}, "secret")},
		ModelProfiles: memoryModelProfiles{
			"primary":  {Key: "primary", Name: "Primary", IntegrationKey: "provider", Model: "primary", Status: integration.StatusActive},
			"fallback": {Key: "fallback", Name: "Fallback", IntegrationKey: "provider", Model: "fallback", Status: integration.StatusActive},
		},
		Secrets: testSecrets(t), OpenAI: client, Telemetry: telemetry,
	})
	if err != nil || result.Content != "fallback response" || !result.FallbackUsed || len(client.calls) != 2 {
		t.Fatalf("fallback result = %#v, calls = %#v, err = %v", result, client.calls, err)
	}
	if result.CostUSD < 0.0000199 || result.CostUSD > 0.0000201 || telemetry.LastCallMetadata()["fallback_used"] != true {
		t.Fatalf("cost telemetry = %#v, result = %#v", telemetry.LastCallMetadata(), result)
	}

	node.Config["max_cost_usd"] = 0.000001
	client.calls = nil
	if _, err = modelResponse(context.Background(), node, "review", Adapters{}); err == nil || len(client.calls) != 0 {
		t.Fatalf("expected pre-call budget rejection, calls = %#v, err = %v", client.calls, err)
	}
}

func (m *trackingModel) Chat(ctx context.Context, _ integration.Integration, _ string, _ string) (integration.ChatResult, error) {
	m.mu.Lock()
	m.inFlight++
	m.calls++
	if m.inFlight > m.maxInFlight {
		m.maxInFlight = m.inFlight
	}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.inFlight--
		m.mu.Unlock()
	}()
	select {
	case <-time.After(m.block):
		return integration.ChatResult{Content: "ok"}, nil
	case <-ctx.Done():
		return integration.ChatResult{}, ctx.Err()
	}
}

func TestParallelLoopBoundsAndDeterministicallyAggregatesProviderWork(t *testing.T) {
	model := &trackingModel{block: 10 * time.Millisecond}
	config, _ := json.Marshal(map[string]string{"base_url": "https://model.example", "model": "reviewer"})
	definition := Definition{Key: "parallel", Name: "Parallel", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": []any{"one", "two", "three", "four"}}},
		{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 4, "concurrency": 4}},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review"}},
		{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"integration": "model"}},
	}, Edges: []Edge{
		{Key: "items", FromNode: "start", FromPort: "event", ToNode: "loop", ToPort: "items"},
		{Key: "context", FromNode: "loop", FromPort: "item", ToNode: "template", ToPort: "context"},
		{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
	}}
	if runner := newScopedRunner(definition, DefaultCatalog(), nil, Adapters{}, map[string]bool{"start": true, "loop": true, "template": true, "model": true}); cap(runner.limits.global) != 8 {
		t.Fatalf("global execution limit = %d, want 8", cap(runner.limits.global))
	}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, Adapters{Integrations: memoryIntegrations{"model": encryptedIntegration(t, integration.Integration{Key: "model", Name: "Model", Type: integration.TypeOpenAI, Config: config, Status: integration.StatusActive}, "secret")}, Secrets: testSecrets(t), OpenAI: model})
	if err != nil || report.Status != "completed" {
		t.Fatalf("parallel run = %#v, %v", report, err)
	}
	model.mu.Lock()
	maxInFlight, calls := model.maxInFlight, model.calls
	model.mu.Unlock()
	if calls != 4 || maxInFlight != 1 {
		t.Fatalf("provider calls = %d, max concurrent = %d; provider work must serialize", calls, maxInFlight)
	}
	if scopes := nodeScopes(report, "model"); len(scopes) != 4 || scopes[0] != "loop:000001" || scopes[3] != "loop:000004" {
		t.Fatalf("non-deterministic aggregate order: %#v", scopes)
	}
}

func TestLoopCancellationWinsOverPartialPolicy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := &trackingModel{block: time.Second}
	config, _ := json.Marshal(map[string]string{"base_url": "https://model.example", "model": "reviewer"})
	definition := Definition{Key: "cancel", Name: "Cancel", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": []any{"one", "two"}}},
		{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 2, "concurrency": 2, "on_error": "partial"}},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review"}},
		{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"integration": "model"}},
	}, Edges: []Edge{{Key: "items", FromNode: "start", FromPort: "event", ToNode: "loop", ToPort: "items"}, {Key: "context", FromNode: "loop", FromPort: "item", ToNode: "template", ToPort: "context"}, {Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"}}}
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	report, err := RunWithAdapters(ctx, definition, DefaultCatalog(), nil, Adapters{Integrations: memoryIntegrations{"model": encryptedIntegration(t, integration.Integration{Key: "model", Name: "Model", Type: integration.TypeOpenAI, Config: config, Status: integration.StatusActive}, "secret")}, Secrets: testSecrets(t), OpenAI: model})
	if !errors.Is(err, context.Canceled) || report.Status != "cancelled" {
		t.Fatalf("cancellation = %#v, %v", report, err)
	}
}

func nodeScopes(report RunReport, nodeKey string) []string {
	scopes := []string{}
	for _, run := range report.Runs {
		if run.NodeKey == nodeKey {
			scopes = append(scopes, run.ScopeKey)
		}
	}
	return scopes
}

func nodeRunFor(t *testing.T, report RunReport, nodeKey, scopeKey string) NodeRun {
	t.Helper()
	for _, run := range report.Runs {
		if run.NodeKey == nodeKey && run.ScopeKey == scopeKey {
			return run
		}
	}
	t.Fatalf("run %s in scope %s not found in %#v", nodeKey, scopeKey, report)
	return NodeRun{}
}

func TestRunWithAdaptersExecutesFetchAndModelCards(t *testing.T) {
	t.Run("fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "token secret" {
				t.Fatalf("fetch adapter did not receive decrypted secret")
			}
			switch request.URL.Path {
			case "/api/v1/repos/acme/api/pulls/12":
				_, _ = writer.Write([]byte(`{"number":12}`))
			case "/api/v1/repos/acme/api/pulls/12/files":
				_, _ = writer.Write([]byte(`[{"filename":"api.go"}]`))
			case "/api/v1/repos/acme/api/pulls/12.diff":
				_, _ = writer.Write([]byte("diff --git a/api.go b/api.go\n--- a/api.go\n+++ b/api.go\n@@ -1 +1,2 @@\n package api\n+func Added() {}"))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()
		config, _ := json.Marshal(map[string]string{"base_url": server.URL})
		definition := Definition{Key: "fetch", Name: "Fetch", Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start"},
			{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea"}},
		}, Edges: []Edge{{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"}}}
		adapters := Adapters{Integrations: memoryIntegrations{"gitea": encryptedIntegration(t, integration.Integration{Key: "gitea", Name: "Gitea", Type: integration.TypeGitea, Config: config, Status: integration.StatusActive}, "secret")}, Secrets: testSecrets(t), Gitea: integration.HTTPGiteaClient{Client: server.Client()}}
		report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), map[string]any{"pull_request": map[string]any{"owner": "acme", "repo": "api", "number": 12}}, adapters)
		if err != nil {
			t.Fatalf("run fetch: %v", err)
		}
		var result integration.PullRequest
		for _, output := range report.Runs[1].Outputs {
			if output.PortKey == "pull_request" {
				result = output.Value.(integration.PullRequest)
			}
		}
		if result.Diff == "" || result.Files[0]["filename"] != "api.go" || !strings.Contains(result.Files[0]["patch"].(string), "+func Added() {}") {
			t.Fatalf("unexpected fetch result: %#v", result)
		}
	})

	t.Run("model", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/v1/chat/completions" {
				http.NotFound(writer, request)
				return
			}
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || payload["model"] != "reviewer" || payload["max_tokens"] != float64(4096) {
				t.Fatalf("model profile payload = %#v, %v", payload, err)
			}
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"review complete"}}]}`))
		}))
		defer server.Close()
		config, _ := json.Marshal(map[string]string{"base_url": server.URL})
		definition := Definition{Key: "model", Name: "Model", Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start"},
			{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review this"}},
			{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"model_profile": "reviewer", "max_tokens": 4096}},
		}, Edges: []Edge{
			{Key: "context", FromNode: "start", FromPort: "event", ToNode: "template", ToPort: "context"},
			{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
		}}
		adapters := Adapters{Integrations: memoryIntegrations{"openai": encryptedIntegration(t, integration.Integration{Key: "openai", Name: "OpenAI", Type: integration.TypeOpenAI, Config: config, Status: integration.StatusActive}, "secret")}, ModelProfiles: memoryModelProfiles{"reviewer": {Key: "reviewer", Name: "Reviewer", IntegrationKey: "openai", Model: "reviewer", Status: integration.StatusActive}}, Secrets: testSecrets(t), OpenAI: integration.HTTPOpenAIClient{Client: server.Client()}}
		report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, adapters)
		if err != nil {
			t.Fatalf("run model: %v", err)
		}
		if response := report.Runs[2].Outputs[0].Value; response != "review complete" {
			t.Fatalf("response = %#v", response)
		}
	})
}

func TestRunFetchRequiresConfiguredIntegration(t *testing.T) {
	definition := Definition{Key: "fetch", Name: "Fetch", Nodes: []Node{{Key: "start", Type: "trigger", Name: "Start"}, {Key: "fetch", Type: "fetch", Name: "Fetch"}}, Edges: []Edge{{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"}}}
	_, err := Run(context.Background(), definition, DefaultCatalog(), nil)
	if err == nil || err.Error() != `card "fetch" execution failed` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheCardReadsWritesAndDeletesJSONValues(t *testing.T) {
	cache := &memoryCache{}
	write := Node{Key: "cache", Type: "cache", Name: "Cache", Config: map[string]any{"key": "review:42", "mode": "write", "ttl_seconds": 90}}
	outputs, err := executeCache(context.Background(), write, map[string][]any{"value": {map[string]any{"review": "ready"}}}, Adapters{Cache: cache})
	if err != nil || outputs["value"].(map[string]any)["review"] != "ready" {
		t.Fatalf("write output = %#v, %v", outputs, err)
	}
	if string(cache.values["review:42"]) != `{"review":"ready"}` || cache.ttls["review:42"] != 90*time.Second {
		t.Fatalf("cache write = %q with TTL %s", cache.values["review:42"], cache.ttls["review:42"])
	}

	read := write
	read.Config = map[string]any{"key": "review:42", "mode": "read"}
	outputs, err = executeCache(context.Background(), read, nil, Adapters{Cache: cache})
	if err != nil || outputs["value"].(map[string]any)["review"] != "ready" {
		t.Fatalf("read output = %#v, %v", outputs, err)
	}
	read.Config["key"] = "missing"
	outputs, err = executeCache(context.Background(), read, nil, Adapters{Cache: cache})
	if err != nil || outputs["value"] != nil {
		t.Fatalf("cache miss = %#v, %v", outputs, err)
	}

	deleteNode := write
	deleteNode.Config = map[string]any{"key": "review:42", "mode": "delete"}
	outputs, err = executeCache(context.Background(), deleteNode, nil, Adapters{Cache: cache})
	if err != nil || len(outputs) != 0 || len(cache.deleted) != 1 || cache.deleted[0] != "review:42" {
		t.Fatalf("delete output = %#v, deleted = %#v, err = %v", outputs, cache.deleted, err)
	}
}

func TestRunCacheWriteWaitsForValue(t *testing.T) {
	cache := &memoryCache{}
	definition := Definition{Key: "cache-write", Name: "Cache write", Nodes: []Node{
		{Key: "trigger", Type: "trigger", Name: "Trigger", Config: map[string]any{"event": map[string]any{"review": "ready"}}},
		{Key: "cache", Type: "cache", Name: "Cache", Config: map[string]any{"key": "review:42", "mode": "write", "ttl_seconds": 90}},
	}, Edges: []Edge{{Key: "value", FromNode: "trigger", FromPort: "event", ToNode: "cache", ToPort: "value"}}}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, Adapters{Cache: cache})
	if err != nil || len(report.Runs) != 2 || string(cache.values["review:42"]) != `{"review":"ready"}` {
		t.Fatalf("cache write report = %#v, stored = %q, err = %v", report, cache.values["review:42"], err)
	}
}

func TestCacheCardRequiresInjectedAdapter(t *testing.T) {
	_, err := executeCache(context.Background(), Node{Key: "cache", Type: "cache", Name: "Cache", Config: map[string]any{"key": "review:42", "mode": "read"}}, nil, Adapters{})
	if err == nil || err.Error() != `cache card "cache" requires an injected cache adapter` {
		t.Fatalf("cache adapter error = %v", err)
	}
}

func TestRunCacheCardEmitsNilForMissAndNothingForDelete(t *testing.T) {
	cache := &memoryCache{}
	for _, testCase := range []struct {
		name    string
		mode    string
		outputs int
	}{
		{name: "miss", mode: "read", outputs: 1},
		{name: "delete", mode: "delete", outputs: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			definition := Definition{Key: testCase.name, Name: testCase.name, Nodes: []Node{{Key: "cache", Type: "cache", Name: "Cache", Config: map[string]any{"key": "review:missing", "mode": testCase.mode}}}}
			report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, Adapters{Cache: cache})
			if err != nil || len(report.Runs) != 1 || len(report.Runs[0].Outputs) != testCase.outputs {
				t.Fatalf("cache report = %#v, err = %v", report, err)
			}
			if testCase.mode == "read" && report.Runs[0].Outputs[0].Value != nil {
				t.Fatalf("cache miss output = %#v", report.Runs[0].Outputs)
			}
		})
	}
}
