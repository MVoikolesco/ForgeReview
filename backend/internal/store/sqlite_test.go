package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/workflow"
)

type integrationLookup map[string]integration.Integration

func (items integrationLookup) Integration(_ context.Context, key string) (integration.Integration, error) {
	item, ok := items[key]
	if !ok {
		return integration.Integration{}, errors.New("not found")
	}
	return item, nil
}

type modelResponse string

func (m modelResponse) Chat(context.Context, integration.Integration, string, string) (string, error) {
	return string(m), nil
}

func TestPublicationIsIdempotentAcrossDuplicateExecutionRun(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/publication.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var mu sync.Mutex
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/repos/acme/review/issues/7/comments" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "token gitea-secret" {
			t.Fatal("missing Gitea authorization")
		}
		mu.Lock()
		posts++
		mu.Unlock()
		_, _ = writer.Write([]byte(`{"id":99,"html_url":"https://gitea.example/comments/99"}`))
	}))
	defer server.Close()
	t.Setenv("GITEA_PUBLISH_TOKEN", "gitea-secret")
	t.Setenv("MODEL_PUBLISH_TOKEN", "model-secret")
	giteaConfig, _ := json.Marshal(map[string]string{"base_url": server.URL})
	modelConfig, _ := json.Marshal(map[string]string{"base_url": "https://model.example", "model": "reviewer"})
	definition := publicationDefinition()
	versionID, err := database.Save(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	executionID, err := database.CreateExecution(context.Background(), versionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	adapters := workflow.Adapters{
		Integrations: integrationLookup{
			"gitea": {Key: "gitea", Type: integration.TypeGitea, Config: giteaConfig, SecretReference: "GITEA_PUBLISH_TOKEN", Status: integration.StatusActive},
			"model": {Key: "model", Type: integration.TypeOpenAI, Config: modelConfig, SecretReference: "MODEL_PUBLISH_TOKEN", Status: integration.StatusActive},
		},
		GiteaWriter:  integration.HTTPGiteaClient{Client: server.Client()},
		OpenAI:       modelResponse(`[{"path":"api/main.go","line":2,"comment":"handle error","severity":"high"}]`),
		Publications: database,
		Execution:    workflow.ExecutionContext{ID: executionID, VersionID: versionID},
	}
	for run := 0; run < 2; run++ {
		report, runErr := workflow.RunWithAdapters(context.Background(), definition, workflow.DefaultCatalog(), nil, adapters)
		if runErr != nil || report.Status != "completed" {
			t.Fatalf("run %d = %#v, %v", run, report, runErr)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 {
		t.Fatalf("Gitea publications = %d, want 1", posts)
	}
}

func TestPublicationRetryableAttemptCanBeClaimedAgain(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/retry.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	definition := workflow.Definition{Key: "retry", Name: "Retry", Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}}
	versionID, err := database.Save(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	executionID, err := database.CreateExecution(context.Background(), versionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := "forgereview:publication:" + "1:2:publish"
	attempt := workflow.PublicationAttempt{IdempotencyKey: key, ExecutionID: executionID, VersionID: versionID, NodeKey: "publish"}
	if _, shouldPublish, err := database.BeginPublication(context.Background(), attempt); err != nil || !shouldPublish {
		t.Fatalf("initial claim = %t, %v", shouldPublish, err)
	}
	if err = database.RetryPublication(context.Background(), key, errors.New("provider unavailable")); err != nil {
		t.Fatal(err)
	}
	retried, shouldPublish, err := database.BeginPublication(context.Background(), attempt)
	if err != nil || !shouldPublish || retried.Status != "pending" {
		t.Fatalf("retry claim = %#v, %t, %v", retried, shouldPublish, err)
	}
}

func publicationDefinition() workflow.Definition {
	return workflow.Definition{Key: "publication", Name: "Publication", Nodes: []workflow.Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review"}},
		{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"integration": "model"}},
		{Key: "validate", Type: "validate", Name: "Validate"},
		{Key: "filter", Type: "response_filter", Name: "Filter"},
		{Key: "consolidate", Type: "consolidate", Name: "Consolidate"},
		{Key: "format", Type: "format", Name: "Format"},
		{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"integration": "gitea", "owner": "acme", "repo": "review", "pull_request": 7}},
	}, Edges: []workflow.Edge{
		{Key: "context", FromNode: "start", FromPort: "event", ToNode: "template", ToPort: "context"},
		{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
		{Key: "response", FromNode: "model", FromPort: "response", ToNode: "validate", ToPort: "response"},
		{Key: "valid", FromNode: "validate", FromPort: "valid", ToNode: "filter", ToPort: "response"},
		{Key: "comments", FromNode: "filter", FromPort: "comments", ToNode: "consolidate", ToPort: "comments"},
		{Key: "review", FromNode: "consolidate", FromPort: "review", ToNode: "format", ToPort: "review"},
		{Key: "formatted", FromNode: "format", FromPort: "formatted", ToNode: "publish", ToPort: "formatted_review"},
	}}
}
