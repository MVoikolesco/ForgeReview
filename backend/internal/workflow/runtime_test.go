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

func (items memoryIntegrations) Integration(_ context.Context, key string) (integration.Integration, error) {
	item, ok := items[key]
	if !ok {
		return integration.Integration{}, errors.New("not found")
	}
	return item, nil
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

func TestRunWithAdaptersExecutesFetchAndModelCards(t *testing.T) {
	t.Run("fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
		t.Setenv("GITEA_TEST_TOKEN", "secret")
		definition := Definition{Key: "fetch", Name: "Fetch", Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start"},
			{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea", "owner": "acme", "repo": "api", "pull_request": 12}},
		}, Edges: []Edge{{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"}}}
		adapters := Adapters{Integrations: memoryIntegrations{"gitea": {Key: "gitea", Type: integration.TypeGitea, Config: config, SecretReference: "GITEA_TEST_TOKEN", Status: integration.StatusActive}}, Gitea: integration.HTTPGiteaClient{Client: server.Client()}}
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
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"review complete"}}]}`))
		}))
		defer server.Close()
		config, _ := json.Marshal(map[string]string{"base_url": server.URL, "model": "reviewer"})
		t.Setenv("OPENAI_TEST_TOKEN", "secret")
		definition := Definition{Key: "model", Name: "Model", Nodes: []Node{
			{Key: "start", Type: "trigger", Name: "Start"},
			{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review this"}},
			{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"integration": "openai"}},
		}, Edges: []Edge{
			{Key: "context", FromNode: "start", FromPort: "event", ToNode: "template", ToPort: "context"},
			{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
		}}
		adapters := Adapters{Integrations: memoryIntegrations{"openai": {Key: "openai", Type: integration.TypeOpenAI, Config: config, SecretReference: "OPENAI_TEST_TOKEN", Status: integration.StatusActive}}, OpenAI: integration.HTTPOpenAIClient{Client: server.Client()}}
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
	if err == nil || err.Error() != `fetch card "fetch" requires config.integration` {
		t.Fatalf("unexpected error: %v", err)
	}
}
