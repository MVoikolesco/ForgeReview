package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"forgereview/backend/internal/integration"
)

type memoryIntegrations map[string]integration.Integration
type memoryModelProfiles map[string]integration.ModelProfile

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
	if len(results) != 4 || results[0] != "review group" || results[2] != "review group" {
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

func TestRunAppliesGenericErrorPoliciesWithoutLeakingExecutionDetails(t *testing.T) {
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
		if run.Error != "execution failed" || run.Metadata["error_code"] != "execution_failed" || run.Metadata["error_scope"] != "root" {
			t.Fatalf("unsafe failure report = %#v", run)
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
				_, _ = writer.Write([]byte("diff --git a/api.go b/api.go"))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()
		config, _ := json.Marshal(map[string]string{"base_url": server.URL})
		definition := Definition{Key: "fetch", Name: "Fetch", Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start"},
			{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea", "owner": "acme", "repo": "api", "pull_request": 12}},
		}, Edges: []Edge{{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"}}}
		adapters := Adapters{Integrations: memoryIntegrations{"gitea": encryptedIntegration(t, integration.Integration{Key: "gitea", Name: "Gitea", Type: integration.TypeGitea, Config: config, Status: integration.StatusActive}, "secret")}, Secrets: testSecrets(t), Gitea: integration.HTTPGiteaClient{Client: server.Client()}}
		report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, adapters)
		if err != nil {
			t.Fatalf("run fetch: %v", err)
		}
		var result integration.PullRequest
		for _, output := range report.Runs[1].Outputs {
			if output.PortKey == "pull_request" {
				result = output.Value.(integration.PullRequest)
			}
		}
		if result.Diff == "" || result.Files[0]["filename"] != "api.go" {
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
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || payload["model"] != "reviewer" {
				t.Fatalf("model profile payload = %#v, %v", payload, err)
			}
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"review complete"}}]}`))
		}))
		defer server.Close()
		config, _ := json.Marshal(map[string]string{"base_url": server.URL})
		definition := Definition{Key: "model", Name: "Model", Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start"},
			{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review this"}},
			{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"model_profile": "reviewer"}},
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
