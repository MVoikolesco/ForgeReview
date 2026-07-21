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

func TestRunFiltersFetchedFilesAndGroupsDeterministically(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/repos/acme/api/pulls/12":
			_, _ = writer.Write([]byte(`{"number":12}`))
		case "/api/v1/repos/acme/api/pulls/12/files":
			_, _ = writer.Write([]byte(`[
				{"filename":"src/z.go","patch":"1234567"},
				{"filename":"web/app.ts","patch":"1234"},
				{"filename":"vendor/lib.go","patch":"1234"},
				{"filename":"src/a.go","patch":"1234"},
				{"filename":"web/app.min.js","patch":"1234"},
				{"filename":"docs/readme.md","patch":"1234"}
			]`))
		case "/api/v1/repos/acme/api/pulls/12.diff":
			_, _ = writer.Write([]byte("diff"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	config, _ := json.Marshal(map[string]string{"base_url": server.URL})
	t.Setenv("GITEA_FILTER_TOKEN", "secret")
	definition := Definition{Key: "filter-group", Name: "Filter group", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea", "owner": "acme", "repo": "api", "pull_request": 12}},
		{Key: "filter", Type: "filter", Name: "Filter", Config: map[string]any{"include_extensions": []any{"go", ".ts", ".js"}, "exclude_extensions": []any{".js"}, "ignore_generated": true}},
		{Key: "group", Type: "group", Name: "Group", Config: map[string]any{"max_files": 2, "max_characters": 8, "group_by_extension": true}},
	}, Edges: []Edge{
		{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"},
		{Key: "files", FromNode: "fetch", FromPort: "files", ToNode: "filter", ToPort: "files"},
		{Key: "filtered", FromNode: "filter", FromPort: "files", ToNode: "group", ToPort: "files"},
	}}
	adapters := Adapters{Integrations: memoryIntegrations{"gitea": {Key: "gitea", Type: integration.TypeGitea, Config: config, SecretReference: "GITEA_FILTER_TOKEN", Status: integration.StatusActive}}, Gitea: integration.HTTPGiteaClient{Client: server.Client()}}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, adapters)
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}
	filtered := outputFor(t, report, "filter", "files").([]map[string]any)
	if len(filtered) != 3 || filtered[0]["filename"] != "src/a.go" || filtered[1]["filename"] != "src/z.go" || filtered[2]["filename"] != "web/app.ts" {
		t.Fatalf("filtered files = %#v", filtered)
	}
	groups := outputFor(t, report, "group", "groups").([]FileGroup)
	if len(groups) != 3 || groups[0].Extension != ".go" || groups[0].Files[0]["filename"] != "src/a.go" || groups[1].Files[0]["filename"] != "src/z.go" || groups[2].Extension != ".ts" || groups[2].Files[0]["filename"] != "web/app.ts" {
		t.Fatalf("groups = %#v", groups)
	}
}

func TestRunRoutesInvalidModelResponseToValidateInvalid(t *testing.T) {
	definition := Definition{Key: "invalid", Name: "Invalid", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review"}},
		{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"integration": "model"}},
		{Key: "validate", Type: "validate", Name: "Validate"},
	}, Edges: []Edge{
		{Key: "context", FromNode: "start", FromPort: "event", ToNode: "template", ToPort: "context"},
		{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
		{Key: "response", FromNode: "model", FromPort: "response", ToNode: "validate", ToPort: "response"},
	}}
	t.Setenv("MODEL_INVALID_TOKEN", "secret")
	adapters := Adapters{Integrations: memoryIntegrations{"model": modelIntegration(t, "MODEL_INVALID_TOKEN")}, OpenAI: responseModel("not JSON")}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, adapters)
	if err != nil {
		t.Fatalf("invalid response must be routed, not fail the workflow: %v", err)
	}
	invalid := outputFor(t, report, "validate", "invalid").(ValidationFailure)
	if len(invalid.Errors) == 0 {
		t.Fatalf("invalid response was accepted: %#v", report)
	}
	if hasOutput(report, "validate", "valid") {
		t.Fatalf("invalid response emitted validate.valid: %#v", report)
	}
}

func TestValidateResponseEnforcesFindingFieldsAndFetchedPaths(t *testing.T) {
	files := []any{[]map[string]any{{"filename": "api/main.go"}}}
	valid, port := validateResponse([]any{`[{"path":"api/main.go","line":1,"comment":"handle error","severity":"high"}]`}, files, map[string]any{"validate_paths": true})
	if port != "valid" || len(valid.([]Finding)) != 1 {
		t.Fatalf("valid response = %#v on %s", valid, port)
	}
	invalid, port := validateResponse([]any{`[{"path":"missing.go","line":0,"comment":"","severity":"urgent"}]`}, files, map[string]any{"validate_paths": true})
	failure, ok := invalid.(ValidationFailure)
	if port != "invalid" || !ok || len(failure.Errors) != 4 {
		t.Fatalf("invalid response = %#v on %s", invalid, port)
	}
}

func TestRunFiltersDeduplicatesConsolidatesAndFormatsFindings(t *testing.T) {
	definition := Definition{Key: "findings", Name: "Findings", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "template_low", Type: "template", Name: "Low", Config: map[string]any{"template": "low"}},
		{Key: "template_high", Type: "template", Name: "High", Config: map[string]any{"template": "high"}},
		{Key: "model_low", Type: "model", Name: "Model low", Config: map[string]any{"integration": "model"}},
		{Key: "model_high", Type: "model", Name: "Model high", Config: map[string]any{"integration": "model"}},
		{Key: "validate_low", Type: "validate", Name: "Validate low"},
		{Key: "validate_high", Type: "validate", Name: "Validate high"},
		{Key: "filter_low", Type: "response_filter", Name: "Filter low", Config: map[string]any{"minimum_severity": "medium"}},
		{Key: "filter_high", Type: "response_filter", Name: "Filter high", Config: map[string]any{"minimum_severity": "medium"}},
		{Key: "consolidate", Type: "consolidate", Name: "Consolidate"},
		{Key: "format", Type: "format", Name: "Format"},
	}, Edges: []Edge{
		{Key: "low-context", FromNode: "start", FromPort: "event", ToNode: "template_low", ToPort: "context"},
		{Key: "high-context", FromNode: "start", FromPort: "event", ToNode: "template_high", ToPort: "context"},
		{Key: "low-prompt", FromNode: "template_low", FromPort: "prompt", ToNode: "model_low", ToPort: "prompt"},
		{Key: "high-prompt", FromNode: "template_high", FromPort: "prompt", ToNode: "model_high", ToPort: "prompt"},
		{Key: "low-response", FromNode: "model_low", FromPort: "response", ToNode: "validate_low", ToPort: "response"},
		{Key: "high-response", FromNode: "model_high", FromPort: "response", ToNode: "validate_high", ToPort: "response"},
		{Key: "low-valid", FromNode: "validate_low", FromPort: "valid", ToNode: "filter_low", ToPort: "response"},
		{Key: "high-valid", FromNode: "validate_high", FromPort: "valid", ToNode: "filter_high", ToPort: "response"},
		{Key: "low-comments", FromNode: "filter_low", FromPort: "comments", ToNode: "consolidate", ToPort: "comments"},
		{Key: "high-comments", FromNode: "filter_high", FromPort: "comments", ToNode: "consolidate", ToPort: "comments"},
		{Key: "review", FromNode: "consolidate", FromPort: "review", ToNode: "format", ToPort: "review"},
	}}
	t.Setenv("MODEL_FINDINGS_TOKEN", "secret")
	responses := map[string]string{
		"low":  `[{"path":"a.go","line":2,"comment":"low","severity":"low"},{"path":"a.go","line":4,"comment":"duplicate","severity":"high"},{"path":"a.go","line":4,"comment":"duplicate","severity":"high"}]`,
		"high": `[{"path":"b.go","line":1,"comment":"critical","severity":"critical"},{"path":"a.go","line":4,"comment":"duplicate","severity":"high"}]`,
	}
	adapters := Adapters{Integrations: memoryIntegrations{"model": modelIntegration(t, "MODEL_FINDINGS_TOKEN")}, OpenAI: responseModels(responses)}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, adapters)
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}
	formatted := outputFor(t, report, "format", "formatted").(FormattedReview)
	if formatted.Summary != (ReviewSummary{Total: 2, High: 1, Critical: 1}) {
		t.Fatalf("summary = %#v", formatted.Summary)
	}
	if len(formatted.Findings) != 2 || formatted.Findings[0].Path != "a.go" || formatted.Findings[1].Path != "b.go" {
		t.Fatalf("formatted findings = %#v", formatted.Findings)
	}
}

type responseModel string

func (model responseModel) Chat(context.Context, integration.Integration, string, string) (string, error) {
	return string(model), nil
}

type responseModels map[string]string

func (models responseModels) Chat(_ context.Context, _ integration.Integration, _ string, prompt string) (string, error) {
	response, ok := models[prompt]
	if !ok {
		return "", errors.New("unexpected prompt")
	}
	return response, nil
}

func modelIntegration(t *testing.T, secretReference string) integration.Integration {
	t.Helper()
	config, err := json.Marshal(map[string]string{"base_url": "https://model.example", "model": "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	return integration.Integration{Key: "model", Type: integration.TypeOpenAI, Config: config, SecretReference: secretReference, Status: integration.StatusActive}
}

func outputFor(t *testing.T, report RunReport, nodeKey, portKey string) any {
	t.Helper()
	for _, run := range report.Runs {
		if run.NodeKey != nodeKey {
			continue
		}
		for _, output := range run.Outputs {
			if output.PortKey == portKey {
				return output.Value
			}
		}
	}
	t.Fatalf("output %s.%s not found in %#v", nodeKey, portKey, report)
	return nil
}

func hasOutput(report RunReport, nodeKey, portKey string) bool {
	for _, run := range report.Runs {
		if run.NodeKey != nodeKey {
			continue
		}
		for _, output := range run.Outputs {
			if output.PortKey == portKey {
				return true
			}
		}
	}
	return false
}
