package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forgereview/backend/internal/integration"
)

type staticPullRequestReader struct {
	files    []map[string]any
	metadata map[string]any
}

func (reader staticPullRequestReader) ReadPullRequest(_ context.Context, _ integration.Integration, _ string, _ integration.PullRequestRequest) (integration.PullRequest, error) {
	return integration.PullRequest{Files: reader.files, Metadata: reader.metadata}, nil
}

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
	definition := Definition{Key: "filter-group", Name: "Filter group", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea"}},
		{Key: "filter", Type: "filter", Name: "Filter", Config: map[string]any{"include_extensions": []any{"go", ".ts", ".js"}, "exclude_extensions": []any{".js"}, "ignore_generated": true}},
		{Key: "group", Type: "group", Name: "Group", Config: map[string]any{"max_files": 2, "max_characters": 8, "group_by_extension": true}},
	}, Edges: []Edge{
		{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"},
		{Key: "files", FromNode: "fetch", FromPort: "files", ToNode: "filter", ToPort: "files"},
		{Key: "filtered", FromNode: "filter", FromPort: "files", ToNode: "group", ToPort: "files"},
	}}
	adapters := Adapters{Integrations: memoryIntegrations{"gitea": encryptedIntegration(t, integration.Integration{Key: "gitea", Name: "Gitea", Type: integration.TypeGitea, Config: config, Status: integration.StatusActive}, "secret")}, Secrets: testSecrets(t), Gitea: integration.HTTPGiteaClient{Client: server.Client()}}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), map[string]any{"pull_request": map[string]any{"owner": "acme", "repo": "api", "number": 12}}, adapters)
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

func TestRunReviewSendsFetchedUnifiedDiffToModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/repos/acme/api/pulls/12":
			_, _ = writer.Write([]byte(`{"number":12}`))
		case "/api/v1/repos/acme/api/pulls/12/files":
			_, _ = writer.Write([]byte(`[{"filename":"src/service.go","status":"modified"}]`))
		case "/api/v1/repos/acme/api/pulls/12.diff":
			_, _ = writer.Write([]byte("diff --git a/src/service.go b/src/service.go\n--- a/src/service.go\n+++ b/src/service.go\n@@ -4,2 +4,3 @@\n func run() {\n+\tvalidate()\n }"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	config, _ := json.Marshal(map[string]string{"base_url": server.URL})
	definition := Definition{Key: "diff-prompt", Name: "Diff prompt", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea"}},
		{Key: "group", Type: "group", Name: "Group", Config: map[string]any{"max_files": 8, "max_characters": 12000}},
		{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 2, "concurrency": 1}},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review"}},
		{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"integration": "model"}},
	}, Edges: []Edge{
		{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"},
		{Key: "files", FromNode: "fetch", FromPort: "files", ToNode: "group", ToPort: "files"},
		{Key: "groups", FromNode: "group", FromPort: "groups", ToNode: "loop", ToPort: "items"},
		{Key: "context", FromNode: "loop", FromPort: "item", ToNode: "template", ToPort: "context"},
		{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
	}}
	model := &sequentialModel{responses: []string{"[]"}}
	adapters := Adapters{
		Integrations: memoryIntegrations{
			"gitea": encryptedIntegration(t, integration.Integration{Key: "gitea", Name: "Gitea", Type: integration.TypeGitea, Config: config, Status: integration.StatusActive}, "gitea-secret"),
			"model": modelIntegration(t, "model-secret"),
		},
		Secrets: testSecrets(t),
		Gitea:   integration.HTTPGiteaClient{Client: server.Client()},
		OpenAI:  model,
	}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), map[string]any{"pull_request": map[string]any{"owner": "acme", "repo": "api", "number": 12}}, adapters)
	if err != nil || report.Status != "completed" {
		t.Fatalf("run = %#v, %v", report, err)
	}
	if len(model.prompts) != 1 {
		t.Fatalf("model calls = %d, prompts = %#v", model.calls, model.prompts)
	}
	for _, expected := range []string{"diff --git a/src/service.go b/src/service.go", "@@ -4,2 +4,3 @@", `+\tvalidate()`} {
		if !strings.Contains(model.prompts[0], expected) {
			t.Fatalf("model prompt is missing %q: %s", expected, model.prompts[0])
		}
	}
	for _, expected := range []string{"introduzida por uma linha '+'", "nunca uma linha de contexto", "não repita a mesma observação", "riscos meramente hipotéticos", "auxiliam um revisor humano"} {
		if !strings.Contains(model.prompts[0], expected) {
			t.Fatalf("model prompt is missing review guidance %q: %s", expected, model.prompts[0])
		}
	}
}

func TestRunFetchRejectsFilesWithoutReviewableContent(t *testing.T) {
	definition := Definition{Key: "missing-diff", Name: "Missing diff", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea"}},
	}, Edges: []Edge{{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"}}}
	config, _ := json.Marshal(map[string]string{"base_url": "https://gitea.example"})
	adapters := Adapters{
		Integrations: memoryIntegrations{"gitea": encryptedIntegration(t, integration.Integration{Key: "gitea", Name: "Gitea", Type: integration.TypeGitea, Config: config, Status: integration.StatusActive}, "secret")},
		Secrets:      testSecrets(t),
		Gitea:        staticPullRequestReader{files: []map[string]any{{"filename": "main.go"}}},
	}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), map[string]any{"pull_request": map[string]any{"owner": "acme", "repo": "api", "number": 12}}, adapters)
	var failure *ExecutionFailure
	if err == nil || !errors.As(err, &failure) || !strings.Contains(failure.Cause.Error(), "without reviewable patch content") || report.Status != "failed" {
		t.Fatalf("run = %#v, %v", report, err)
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
	adapters := Adapters{Integrations: memoryIntegrations{"model": modelIntegration(t, "secret")}, Secrets: testSecrets(t), OpenAI: responseModel("not JSON")}
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

func TestRunCorrectsInvalidModelResponseBeforeValidateRouting(t *testing.T) {
	definition := correctiveRetryDefinition(map[string]any{"integration": "model", "retry_limit": 1})
	model := &sequentialModel{responses: []string{"not JSON", `[{"path":"a.go","line":1,"comment":"fixed","severity":"high"}]`}}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, Adapters{Integrations: memoryIntegrations{"model": modelIntegration(t, "secret")}, Secrets: testSecrets(t), OpenAI: model})
	if err != nil || model.calls != 2 {
		t.Fatalf("corrective run = %#v, %v; calls=%d", report, err, model.calls)
	}
	if !hasOutput(report, "validate", "valid") || hasOutput(report, "validate", "invalid") {
		t.Fatalf("corrective validation routing = %#v", report)
	}
	validate := nodeRunFor(t, report, "validate", rootScope)
	if validate.Metadata["attempt_count"] != 2 || len(validate.Metadata["provider_calls"].([]any)) != 1 {
		t.Fatalf("retry metadata = %#v", validate.Metadata)
	}
	if !strings.Contains(model.prompts[1], "Motivo exato: model response must be a JSON finding list") || !strings.Contains(model.prompts[1], `"line":12`) {
		t.Fatalf("corrective prompt does not reinforce the contract: %s", model.prompts[1])
	}
}

func TestRunExhaustedCorrectiveRetryPreservesInvalidRoute(t *testing.T) {
	definition := correctiveRetryDefinition(map[string]any{"integration": "model", "retry_limit": 1})
	definition.Nodes = append(definition.Nodes, Node{Key: "invalid-log", Type: "log", Name: "Invalid log"})
	definition.Edges = append(definition.Edges, Edge{Key: "invalid", FromNode: "validate", FromPort: "invalid", ToNode: "invalid-log", ToPort: "input"})
	model := &sequentialModel{responses: []string{"not JSON", "still not JSON"}}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, Adapters{Integrations: memoryIntegrations{"model": modelIntegration(t, "secret")}, Secrets: testSecrets(t), OpenAI: model})
	if err != nil || model.calls != 2 || !hasOutput(report, "validate", "invalid") {
		t.Fatalf("exhausted retry = %#v, %v; calls=%d", report, err, model.calls)
	}
	if _, ok := outputFor(t, report, "invalid-log", "output").(ValidationFailure); !ok {
		t.Fatalf("invalid route was not preserved: %#v", report)
	}
}

func TestRunCorrectiveRetriesRemainInEachLoopScope(t *testing.T) {
	definition := Definition{Key: "scoped-retry", Name: "Scoped retry", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": []any{"one", "two"}}},
		{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 2}},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review"}},
		{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"integration": "model", "retry_limit": 1}},
		{Key: "validate", Type: "validate", Name: "Validate"},
	}, Edges: []Edge{
		{Key: "items", FromNode: "start", FromPort: "event", ToNode: "loop", ToPort: "items"},
		{Key: "context", FromNode: "loop", FromPort: "item", ToNode: "template", ToPort: "context"},
		{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
		{Key: "response", FromNode: "model", FromPort: "response", ToNode: "validate", ToPort: "response"},
	}}
	model := &sequentialModel{responses: []string{"bad first", `[]`, "bad second", `[]`}}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, Adapters{Integrations: memoryIntegrations{"model": modelIntegration(t, "secret")}, Secrets: testSecrets(t), OpenAI: model})
	if err != nil || model.calls != 4 {
		t.Fatalf("scoped retries = %#v, %v; calls=%d", report, err, model.calls)
	}
	for _, scope := range []string{"loop:000001", "loop:000002"} {
		validate := nodeRunFor(t, report, "validate", scope)
		if validate.Metadata["attempt_count"] != 2 || !hasOutputInScope(report, "validate", scope, "valid") {
			t.Fatalf("scope %s retry = %#v", scope, validate)
		}
	}
}

func correctiveRetryDefinition(modelConfig map[string]any) Definition {
	return Definition{Key: "corrective", Name: "Corrective", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review original prompt"}},
		{Key: "model", Type: "model", Name: "Model", Config: modelConfig},
		{Key: "validate", Type: "validate", Name: "Validate"},
	}, Edges: []Edge{
		{Key: "context", FromNode: "start", FromPort: "event", ToNode: "template", ToPort: "context"},
		{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
		{Key: "response", FromNode: "model", FromPort: "response", ToNode: "validate", ToPort: "response"},
	}}
}

func hasOutputInScope(report RunReport, nodeKey, scopeKey, portKey string) bool {
	for _, run := range report.Runs {
		if run.NodeKey == nodeKey && run.ScopeKey == scopeKey {
			for _, output := range run.Outputs {
				if output.PortKey == portKey {
					return true
				}
			}
		}
	}
	return false
}

func TestValidateResponseEnforcesFindingFieldsAndFetchedPaths(t *testing.T) {
	files := []any{[]map[string]any{{"filename": "api/main.go", "patch": "@@ -0,0 +1 @@\n+handle error"}}}
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
	responses := map[string]string{
		"low":  `[{"path":"a.go","line":2,"comment":"low","severity":"low"},{"path":"a.go","line":4,"comment":"duplicate","severity":"high"},{"path":"a.go","line":4,"comment":"duplicate","severity":"high"}]`,
		"high": `[{"path":"b.go","line":1,"comment":"critical","severity":"critical"},{"path":"a.go","line":4,"comment":"duplicate","severity":"high"}]`,
	}
	adapters := Adapters{Integrations: memoryIntegrations{"model": modelIntegration(t, "secret")}, Secrets: testSecrets(t), OpenAI: responseModels(responses)}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, adapters)
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}
	formatted := outputFor(t, report, "format", "formatted").(FormattedReview)
	if formatted.Summary != (ReviewSummary{Total: 2, High: 1, Critical: 1, Event: "REQUEST_CHANGES", Status: "changes_requested"}) {
		t.Fatalf("summary = %#v", formatted.Summary)
	}
	if len(formatted.Findings) != 2 || formatted.Findings[0].Path != "a.go" || formatted.Findings[1].Path != "b.go" {
		t.Fatalf("formatted findings = %#v", formatted.Findings)
	}
	if len(formatted.Observations) != 2 || formatted.Observations[0] != (ReviewObservation{Path: "a.go", Body: "duplicate", NewPosition: 4}) {
		t.Fatalf("formatted observations = %#v", formatted.Observations)
	}
}

func TestRunAggregatesScopedReviewFindingsAndPublishesOnceAtRoot(t *testing.T) {
	files := []map[string]any{
		{"filename": "a.go", "patch": "@@ -1,2 +1,2 @@\n package a\n+first"},
		{"filename": "b.go", "patch": "@@ -1,3 +1,3 @@\n package b\n+first\n+second"},
	}
	definition := Definition{Key: "scoped-review", Name: "Scoped review", Nodes: []Node{
		{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": map[string]any{"pull_request": map[string]any{"owner": "acme", "repo": "review", "number": 7}}}},
		{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea"}},
		{Key: "group", Type: "group", Name: "Group", Config: map[string]any{"max_files": 1, "max_characters": 100}},
		{Key: "loop", Type: "loop", Name: "Loop", Config: map[string]any{"max_iterations": 2, "concurrency": 1}},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review"}},
		{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"integration": "model"}},
		{Key: "validate", Type: "validate", Name: "Validate", Config: map[string]any{"validate_paths": true}},
		{Key: "response-filter", Type: "response_filter", Name: "Filter", Config: map[string]any{"minimum_severity": "medium"}},
		{Key: "consolidate", Type: "consolidate", Name: "Consolidate"},
		{Key: "format", Type: "format", Name: "Format"},
		{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"integration": "gitea"}},
	}, Edges: []Edge{
		{Key: "event", FromNode: "start", FromPort: "event", ToNode: "fetch", ToPort: "event"},
		{Key: "files", FromNode: "fetch", FromPort: "files", ToNode: "group", ToPort: "files"},
		{Key: "groups", FromNode: "group", FromPort: "groups", ToNode: "loop", ToPort: "items"},
		{Key: "context", FromNode: "loop", FromPort: "item", ToNode: "template", ToPort: "context"},
		{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
		{Key: "response", FromNode: "model", FromPort: "response", ToNode: "validate", ToPort: "response"},
		{Key: "files-in-group", FromNode: "loop", FromPort: "item", ToNode: "validate", ToPort: "files"},
		{Key: "valid", FromNode: "validate", FromPort: "valid", ToNode: "response-filter", ToPort: "response"},
		{Key: "results", FromNode: "loop", FromPort: "results", ToNode: "consolidate", ToPort: "comments"},
		{Key: "review", FromNode: "consolidate", FromPort: "review", ToNode: "format", ToPort: "review"},
		{Key: "formatted", FromNode: "format", FromPort: "formatted", ToNode: "publish", ToPort: "formatted_review"},
		{Key: "target", FromNode: "fetch", FromPort: "pull_request", ToNode: "publish", ToPort: "pull_request"},
	}}
	giteaConfig, err := json.Marshal(map[string]string{"base_url": "https://gitea.example"})
	if err != nil {
		t.Fatal(err)
	}
	model := &sequentialModel{responses: []string{
		`[{"path":"a.go","line":2,"comment":"first","severity":"high"}]`,
		`[{"path":"b.go","line":3,"comment":"second","severity":"medium"}]`,
	}}
	publisher := &recordingPublisher{}
	ledger := &recordingPublicationLedger{}
	adapters := Adapters{
		Integrations: memoryIntegrations{
			"model": modelIntegration(t, "model-secret"),
			"gitea": encryptedIntegration(t, integration.Integration{Key: "gitea", Name: "Gitea", Type: integration.TypeGitea, Config: giteaConfig, Status: integration.StatusActive}, "gitea-secret"),
		},
		Secrets:      testSecrets(t),
		Gitea:        staticPullRequestReader{files: files, metadata: map[string]any{"title": "feat: revisa processamento em grupos"}},
		OpenAI:       model,
		GiteaWriter:  publisher,
		Publications: ledger,
		Execution:    ExecutionContext{ID: 31, VersionID: 17},
	}
	report, err := RunWithAdapters(context.Background(), definition, DefaultCatalog(), nil, adapters)
	if err != nil || report.Status != "completed" {
		t.Fatalf("run = %#v, %v", report, err)
	}
	if model.calls != 2 {
		t.Fatalf("model calls = %d, want 2", model.calls)
	}
	for _, nodeKey := range []string{"template", "model", "validate", "response-filter"} {
		if scopes := nodeScopes(report, nodeKey); len(scopes) != 2 || scopes[0] != "loop:000001" || scopes[1] != "loop:000002" {
			t.Fatalf("%s scopes = %#v", nodeKey, scopes)
		}
	}
	for _, nodeKey := range []string{"consolidate", "format", "publish"} {
		if scopes := nodeScopes(report, nodeKey); len(scopes) != 1 || scopes[0] != rootScope {
			t.Fatalf("%s must run once at root, got %#v", nodeKey, scopes)
		}
	}
	loopResults := outputFor(t, report, "loop", "results").([]any)
	if len(loopResults) != 2 {
		t.Fatalf("loop results = %#v", loopResults)
	}
	consolidateRun := nodeRunFor(t, report, "consolidate", rootScope)
	aggregated, ok := consolidateRun.Inputs["comments"][0].([]any)
	if !ok || len(aggregated) != 2 {
		t.Fatalf("consolidate inputs = %#v", consolidateRun.Inputs)
	}
	review := outputFor(t, report, "consolidate", "review").(Review)
	if len(review.Findings) != 2 || review.Findings[0].Path != "a.go" || review.Findings[1].Path != "b.go" {
		t.Fatalf("aggregated review = %#v", review)
	}
	formatted := outputFor(t, report, "format", "formatted").(FormattedReview)
	if formatted.Summary != (ReviewSummary{Total: 2, Medium: 1, High: 1, Event: "REQUEST_CHANGES", Status: "changes_requested"}) {
		t.Fatalf("formatted review = %#v", formatted)
	}
	if len(publisher.requests) != 1 || len(ledger.attempts) != 1 {
		t.Fatalf("publications = %#v, attempts = %#v", publisher.requests, ledger.attempts)
	}
	request := publisher.requests[0]
	if request.Event != "COMMENT" || len(request.Comments) != 2 || request.Comments[0] != (integration.GiteaReviewComment{Path: "a.go", Body: "first", NewPosition: 2}) {
		t.Fatalf("published review = %#v", request)
	}
	for _, expected := range []string{"Resumo da implementação: revisa processamento em grupos.", "Os 2 pontos destacados passaram pelas validações configuradas"} {
		if !strings.Contains(request.Body, expected) {
			t.Fatalf("published review body missing %q: %s", expected, request.Body)
		}
	}
}

func TestValidateResponseAcceptsOnlyAddedLines(t *testing.T) {
	files := []any{[]map[string]any{{"filename": "app/main.go", "patch": "@@ -10,2 +10,3 @@\n context\n+added\n context"}}}
	value, port := validateResponse([]any{`[{"path":"app/main.go","line":4,"comment":"wrong line","severity":"medium"}]`}, files, map[string]any{"validate_paths": true})
	if port != "invalid" {
		t.Fatalf("port = %q, value = %#v", port, value)
	}
	failure := value.(ValidationFailure)
	if len(failure.Errors) != 1 || !strings.Contains(failure.Errors[0], "line 4 is not an added line") {
		t.Fatalf("failure = %#v", failure)
	}

	value, port = validateResponse([]any{`[{"path":"app/main.go","line":10,"comment":"context line","severity":"medium"}]`}, files, map[string]any{"validate_paths": true})
	if port != "invalid" {
		t.Fatalf("context line was accepted: %#v", value)
	}
	failure = value.(ValidationFailure)
	if len(failure.Errors) != 1 || !strings.Contains(failure.Errors[0], "line 10 is not an added line") {
		t.Fatalf("context failure = %#v", failure)
	}

	value, port = validateResponse([]any{`[{"path":"app/main.go","line":11,"comment":"right line","severity":"medium"}]`}, files, map[string]any{"validate_paths": true})
	if port != "valid" {
		t.Fatalf("valid line rejected: %#v", value)
	}
}

func TestAttachReviewIdentityUsesRepositoryAndBaseCommit(t *testing.T) {
	files := []map[string]any{{"filename": "app.go"}}
	attachReviewIdentity(files, map[string]any{"base": map[string]any{"sha": "base123"}}, integration.PullRequestRequest{Owner: "acme", Repo: "review", Number: 7})
	if files[0]["_forgereview_repository"] != "acme/review" || files[0]["_forgereview_base_commit"] != "base123" {
		t.Fatalf("identity = %#v", files[0])
	}
}

func TestPublishEventUsesSafeSeverityDefaults(t *testing.T) {
	high := FormattedReview{Summary: ReviewSummary{High: 1}}
	medium := FormattedReview{Summary: ReviewSummary{Medium: 1}}
	if got := publishEvent(high, nil); got != "COMMENT" {
		t.Fatalf("high event = %q", got)
	}
	if got := publishEvent(high, map[string]any{"allow_autonomous_rejection": true}); got != "REQUEST_CHANGES" {
		t.Fatalf("allowed high event = %q", got)
	}
	if got := publishEvent(medium, nil); got != "COMMENT" {
		t.Fatalf("medium event = %q", got)
	}
	if got := publishEvent(medium, map[string]any{"medium_severity_event": "REQUEST_CHANGES"}); got != "REQUEST_CHANGES" {
		t.Fatalf("configured medium event = %q", got)
	}
}

func TestFormattedReviewBodyUsesSafeOptionalTelemetryInPortuguese(t *testing.T) {
	review := FormattedReview{Summary: ReviewSummary{Total: 2, High: 1, Medium: 1}}
	body := formattedReviewBody(review, "COMMENT", TelemetrySnapshot{ElapsedMS: 43501, Models: []string{"gpt-review"}, Prompt: 12, Completion: 8, Total: 20}, "adiciona validação de MIME")
	for _, expected := range []string{"> status: comentado", "> tempo decorrido: 43.501s", "> modelo: gpt-review", "> tokens: 20 (prompt: 12, completion: 8)", "Foram identificados 2 achados relevantes", "Resumo da implementação: adiciona validação de MIME.", "Os 2 pontos destacados passaram pelas validações configuradas, servem como apoio e devem ser avaliados pelo revisor."} {
		if !strings.Contains(body, expected) {
			t.Fatalf("body missing %q: %s", expected, body)
		}
	}
	if strings.Contains(body, "Review automatizada concluída.") {
		t.Fatalf("body still presents the automation as a conclusive review: %s", body)
	}
	withoutUnknowns := formattedReviewBody(FormattedReview{}, "COMMENT", TelemetrySnapshot{ElapsedMS: 1}, "")
	if strings.Contains(withoutUnknowns, "> modelo:") || strings.Contains(withoutUnknowns, "> tokens:") {
		t.Fatalf("unknown telemetry was invented: %s", withoutUnknowns)
	}
	for _, expected := range []string{"Não foram destacados achados relevantes", "Resumo da implementação: alterações apresentadas no diff.", "a decisão final permanece com o revisor."} {
		if !strings.Contains(withoutUnknowns, expected) {
			t.Fatalf("empty body missing %q: %s", expected, withoutUnknowns)
		}
	}
}

func TestPullRequestImplementationSummaryUsesSafeCompactTitle(t *testing.T) {
	value := integration.PullRequest{Metadata: map[string]any{"title": "feat:   adiciona suporte a MIME\r\nnos currículos. "}}
	if got := pullRequestImplementationSummary(value); got != "adiciona suporte a MIME nos currículos" {
		t.Fatalf("summary = %q", got)
	}
}

type responseModel string

func (model responseModel) Chat(context.Context, integration.Integration, string, string) (integration.ChatResult, error) {
	return integration.ChatResult{Content: string(model)}, nil
}

type responseModels map[string]string

func (models responseModels) Chat(_ context.Context, _ integration.Integration, _ string, prompt string) (integration.ChatResult, error) {
	response, ok := models[prompt]
	if !ok {
		for prefix, candidate := range models {
			if strings.HasPrefix(prompt, prefix+"\n\nContexto real para a tarefa") {
				response, ok = candidate, true
				break
			}
		}
	}
	if !ok {
		return integration.ChatResult{}, errors.New("unexpected prompt")
	}
	return integration.ChatResult{Content: response}, nil
}

type sequentialModel struct {
	responses []string
	calls     int
	prompts   []string
}

func (model *sequentialModel) Chat(_ context.Context, _ integration.Integration, _ string, prompt string) (integration.ChatResult, error) {
	if model.calls >= len(model.responses) {
		return integration.ChatResult{}, errors.New("unexpected model call")
	}
	response := model.responses[model.calls]
	model.calls++
	model.prompts = append(model.prompts, prompt)
	return integration.ChatResult{Content: response}, nil
}

type recordingPublisher struct {
	requests []integration.GiteaReviewRequest
}

func (publisher *recordingPublisher) PublishReview(_ context.Context, _ integration.Integration, _ string, request integration.GiteaReviewRequest) (integration.PublicationReceipt, error) {
	publisher.requests = append(publisher.requests, request)
	return integration.PublicationReceipt{CommentID: int64(len(publisher.requests))}, nil
}

type recordingPublicationLedger struct {
	attempts map[string]PublicationAttempt
}

func (ledger *recordingPublicationLedger) BeginPublication(_ context.Context, attempt PublicationAttempt) (PublicationAttempt, bool, error) {
	if ledger.attempts == nil {
		ledger.attempts = map[string]PublicationAttempt{}
	}
	if existing, ok := ledger.attempts[attempt.IdempotencyKey]; ok {
		return existing, false, nil
	}
	attempt.Status = "pending"
	ledger.attempts[attempt.IdempotencyKey] = attempt
	return attempt, true, nil
}

func (ledger *recordingPublicationLedger) CompletePublication(_ context.Context, key string, receipt integration.PublicationReceipt) error {
	attempt := ledger.attempts[key]
	attempt.Status, attempt.Receipt = "completed", receipt
	ledger.attempts[key] = attempt
	return nil
}

func (ledger *recordingPublicationLedger) RetryPublication(_ context.Context, key string, _ error) error {
	attempt := ledger.attempts[key]
	attempt.Status = "retryable"
	ledger.attempts[key] = attempt
	return nil
}

func modelIntegration(t *testing.T, secret string) integration.Integration {
	t.Helper()
	config, err := json.Marshal(map[string]string{"base_url": "https://model.example", "model": "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	return encryptedIntegration(t, integration.Integration{Key: "model", Name: "Model", Type: integration.TypeOpenAI, Config: config, Status: integration.StatusActive}, secret)
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
