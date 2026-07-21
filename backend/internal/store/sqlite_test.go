package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
			"gitea": encryptedStoreIntegration(t, integration.Integration{Key: "gitea", Name: "Gitea", Type: integration.TypeGitea, Config: giteaConfig, Status: integration.StatusActive}, "gitea-secret"),
			"model": encryptedStoreIntegration(t, integration.Integration{Key: "model", Name: "Model", Type: integration.TypeOpenAI, Config: modelConfig, Status: integration.StatusActive}, "model-secret"),
		},
		Secrets:      storeTestSecrets(t),
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

const storeTestEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func storeTestSecrets(t *testing.T) *integration.EncryptedSecrets {
	t.Helper()
	secrets, err := integration.NewEncryptedSecrets(storeTestEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	return secrets
}

func encryptedStoreIntegration(t *testing.T, item integration.Integration, value string) integration.Integration {
	t.Helper()
	ciphertext, err := storeTestSecrets(t).Encrypt(item.Key, value)
	if err != nil {
		t.Fatal(err)
	}
	item.SecretCiphertext = ciphertext
	return item
}

func TestIntegrationPersistsOnlyCiphertextAndMigratesLegacyReferences(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/integrations.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config := json.RawMessage(`{"base_url":"https://gitea.example"}`)
	item := encryptedStoreIntegration(t, integration.Integration{Key: "gitea", Name: "Gitea", Type: integration.TypeGitea, Config: config, Status: integration.StatusActive}, "raw-secret")
	if err = database.CreateIntegration(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	var ciphertext string
	if err = database.db.QueryRow(`SELECT secret_ciphertext FROM integrations WHERE integration_key='gitea'`).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if ciphertext == "raw-secret" || strings.Contains(ciphertext, "raw-secret") {
		t.Fatalf("plaintext persisted in database: %q", ciphertext)
	}
	stored, err := database.Integration(context.Background(), "gitea")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := storeTestSecrets(t).Resolve(stored); err != nil || value != "raw-secret" {
		t.Fatalf("stored cipher resolution = %q, %v", value, err)
	}
}

func TestOpenMigratesLegacyIntegrationReferencesWithoutRetainingThem(t *testing.T) {
	path := "file:" + t.TempDir() + "/legacy-integrations.db"
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE integrations (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 integration_key TEXT NOT NULL UNIQUE,
 name TEXT NOT NULL,
 type TEXT NOT NULL,
 config_json TEXT NOT NULL,
 secret_reference TEXT NOT NULL,
 status TEXT NOT NULL,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
); INSERT INTO integrations(integration_key,name,type,config_json,secret_reference,status)
VALUES('legacy','Legacy','gitea','{"base_url":"https://gitea.example"}','OLD_TOKEN','active')`)
	if err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	items, err := database.Integrations(context.Background())
	if err != nil || len(items) != 1 || items[0].SecretCiphertext != "" || items[0].Summary().SecretConfigured {
		t.Fatalf("legacy migration = %#v, %v", items, err)
	}
	var referenceColumns int
	if err = database.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('integrations') WHERE name='secret_reference'`).Scan(&referenceColumns); err != nil {
		t.Fatal(err)
	}
	if referenceColumns != 0 {
		t.Fatal("legacy secret reference column was retained")
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
