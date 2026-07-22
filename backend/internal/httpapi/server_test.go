package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/store"
	"forgereview/backend/internal/workflow"
)

type recordingDispatcher struct{ ids []int64 }

func (d *recordingDispatcher) Enqueue(_ context.Context, id int64) error {
	d.ids = append(d.ids, id)
	return nil
}

func TestIntegrationsNeverExposeSecretMaterial(t *testing.T) {
	database, err := store.Open("file:" + t.TempDir() + "/integrations.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	secrets, err := integration.NewEncryptedSecrets("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatal(err)
	}
	router := New(workflow.DefaultCatalog(), database, workflow.Adapters{Secrets: secrets})
	legacy := httptest.NewRecorder()
	router.ServeHTTP(legacy, httptest.NewRequest(http.MethodPost, "/api/integrations", bytes.NewReader([]byte(`{"key":"legacy","name":"Legacy","type":"gitea","config":{"base_url":"https://gitea.example"},"secret_reference":"GITEA_TOKEN","status":"active","secret":"raw-secret-must-not-persist"}`))))
	if legacy.Code != http.StatusBadRequest || strings.Contains(legacy.Body.String(), "raw-secret-must-not-persist") {
		t.Fatalf("legacy secret reference handling = %d: %s", legacy.Code, legacy.Body.String())
	}
	create := httptest.NewRecorder()
	body := []byte(`{"key":"gitea-main","name":"Gitea","type":"gitea","config":{"base_url":"https://gitea.example"},"status":"active","secret":"raw-secret-must-not-persist"}`)
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/integrations", bytes.NewReader(body)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", create.Code, create.Body.String())
	}
	if strings.Contains(create.Body.String(), "raw-secret-must-not-persist") || strings.Contains(create.Body.String(), "secret_ciphertext") {
		t.Fatalf("create exposed secret data: %s", create.Body.String())
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/integrations", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), "raw-secret-must-not-persist") || strings.Contains(list.Body.String(), "secret_ciphertext") {
		t.Fatalf("list exposed secret data: %s", list.Body.String())
	}
	if !strings.Contains(list.Body.String(), `"secret_configured":true`) {
		t.Fatalf("list did not report safe secret state: %s", list.Body.String())
	}
}

func TestModelProfilesUseExistingLLMConnection(t *testing.T) {
	database, err := store.Open("file:" + t.TempDir() + "/profiles.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	secrets, err := integration.NewEncryptedSecrets("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatal(err)
	}
	router := New(workflow.DefaultCatalog(), database, workflow.Adapters{Secrets: secrets})
	connection := httptest.NewRecorder()
	router.ServeHTTP(connection, httptest.NewRequest(http.MethodPost, "/api/integrations", bytes.NewBufferString(`{"key":"models","name":"Models","type":"openai","config":{"base_url":"https://models.example"},"status":"active","secret":"secret"}`)))
	if connection.Code != http.StatusCreated {
		t.Fatalf("connection status = %d: %s", connection.Code, connection.Body.String())
	}
	profile := httptest.NewRecorder()
	router.ServeHTTP(profile, httptest.NewRequest(http.MethodPost, "/api/model-profiles", bytes.NewBufferString(`{"key":"reviewer","name":"Reviewer","integration_key":"models","model":"qwen2.5-coder","status":"active"}`)))
	if profile.Code != http.StatusCreated || strings.Contains(profile.Body.String(), "secret") {
		t.Fatalf("profile status = %d: %s", profile.Code, profile.Body.String())
	}
}

func TestExecutionStartQueuesWhenDispatcherConfigured(t *testing.T) {
	database, err := store.Open("file:" + t.TempDir() + "/queue.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	definition := workflow.Definition{Key: "queued", Name: "Queued", Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}}
	versionID, err := database.Save(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := &recordingDispatcher{}
	router := New(workflow.DefaultCatalog(), database, workflow.Adapters{Dispatcher: dispatcher})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/workflow-versions/"+strconv.FormatInt(versionID, 10)+"/executions", bytes.NewReader([]byte(`{"event":"queued"}`))))
	if response.Code != http.StatusAccepted || len(dispatcher.ids) != 1 || dispatcher.ids[0] < 1 {
		t.Fatalf("queue response = %d %s; IDs = %#v", response.Code, response.Body.String(), dispatcher.ids)
	}
	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/executions/"+strconv.FormatInt(dispatcher.ids[0], 10), nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"queued"`) {
		t.Fatalf("execution status = %d: %s", status.Code, status.Body.String())
	}
}

func TestWorkflowPublishLifecycleAndListContract(t *testing.T) {
	database, err := store.Open("file:" + t.TempDir() + "/workflow-lifecycle.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	router := New(workflow.DefaultCatalog(), database)

	firstID := createWorkflow(t, router, `{"key":"review","name":"Review v1","nodes":[{"key":"start","type":"trigger","name":"Start"}]}`)
	publishWorkflow(t, router, firstID, http.StatusOK)
	secondID := createWorkflow(t, router, `{"key":"review","name":"Review v2","nodes":[{"key":"start","type":"trigger","name":"Start"}]}`)
	publishWorkflow(t, router, secondID, http.StatusOK)
	publishWorkflow(t, router, 999, http.StatusNotFound)

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/workflows", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body.String())
	}
	var items []workflow.DefinitionSummary
	if err := json.NewDecoder(list.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Key != "review" || items[0].Name != "Review v2" || len(items[0].Versions) != 2 {
		t.Fatalf("list shape = %#v", items)
	}
	versions := items[0].Versions
	if versions[0].Version != 1 || versions[0].Status != workflow.VersionStatusArchived || versions[1].Version != 2 || versions[1].Status != workflow.VersionStatusPublished || versions[1].ID != secondID || versions[1].CreatedAt == "" {
		t.Fatalf("listed lifecycle = %#v", versions)
	}
}

func TestExecutionListProvidesOnlySafeSummaryAndValidatesLimit(t *testing.T) {
	database, err := store.Open("file:" + t.TempDir() + "/execution-list.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	definition := workflow.Definition{Key: "review", Name: "Review", Nodes: []workflow.Node{
		{Key: "start", Type: "trigger", Name: "Start"},
		{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea-secret-key", "owner": "acme", "repo": "api", "pull_request": 42}},
		{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"integration": "gitea-secret-key", "owner": "acme", "repo": "api", "pull_request": 42}},
	}}
	versionID, err := database.Save(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.CreateExecution(context.Background(), versionID, map[string]any{"token": "must-not-leak"}); err != nil {
		t.Fatal(err)
	}
	router := New(workflow.DefaultCatalog(), database)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/executions?limit=1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"owner":"acme"`) || !strings.Contains(response.Body.String(), `"pull_request":42`) {
		t.Fatalf("missing review context: %s", response.Body.String())
	}
	for _, forbidden := range []string{"token", "must-not-leak", "gitea-secret-key", "nodes", "error"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("execution summary exposed %q: %s", forbidden, response.Body.String())
		}
	}
	for _, path := range []string{"/api/executions?limit=0", "/api/executions?limit=101", "/api/executions?limit=invalid"} {
		invalid := httptest.NewRecorder()
		router.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, path, nil))
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d: %s", path, invalid.Code, invalid.Body.String())
		}
	}
}

func createWorkflow(t *testing.T, router http.Handler, body string) int64 {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/workflows", bytes.NewBufferString(body)))
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		VersionID int64 `json:"version_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result.VersionID
}

func publishWorkflow(t *testing.T, router http.Handler, id int64, wantStatus int) {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/workflow-versions/"+strconv.FormatInt(id, 10)+"/publish", nil))
	if response.Code != wantStatus {
		t.Fatalf("publish %d status = %d: %s", id, response.Code, response.Body.String())
	}
}
