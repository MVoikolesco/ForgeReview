package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"forgereview/backend/internal/auth"
	"forgereview/backend/internal/integration"
	"forgereview/backend/internal/store"
	"forgereview/backend/internal/workflow"
)

func TestCORSAndTrustedProxyConfigurationFailSafely(t *testing.T) {
	t.Setenv("FORGEREVIEW_CORS_ALLOWED_ORIGINS", "https://studio.example, http://localhost:3010")
	t.Setenv("FORGEREVIEW_TRUSTED_PROXIES", "10.0.0.0/24")
	db, err := store.Open("file:" + t.TempDir() + "/cors.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	router := New(workflow.DefaultCatalog(), db)
	allowed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/health", nil)
	request.Header.Set("Origin", "https://studio.example")
	router.ServeHTTP(allowed, request)
	if allowed.Code != http.StatusNoContent || allowed.Header().Get("Access-Control-Allow-Origin") != "https://studio.example" {
		t.Fatalf("allowed CORS = %d %#v", allowed.Code, allowed.Header())
	}
	denied := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodOptions, "/health", nil)
	request.Header.Set("Origin", "https://evil.example")
	router.ServeHTTP(denied, request)
	if denied.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected CORS origin: %#v", denied.Header())
	}
	t.Setenv("FORGEREVIEW_TRUSTED_PROXIES", "not an address")
	if _, err := trustedProxies(); err == nil {
		t.Fatal("invalid proxy accepted")
	}
}

func TestUserAdminConstraintsRevokeSessionsAndAuditSafely(t *testing.T) {
	db, err := store.Open("file:" + t.TempDir() + "/users.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := auth.New(db, auth.Config{SigningKey: strings.Repeat("s", 32)})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := manager.CreateUser(context.Background(), "admin@example.test", "correct horse battery staple", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	user, err := manager.CreateUser(context.Background(), "editor@example.test", "correct horse battery staple", auth.RoleEditor)
	if err != nil {
		t.Fatal(err)
	}
	_, token, _, err := manager.Login(context.Background(), user.Email, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.UpdateUser(context.Background(), user.ID, auth.RoleViewer, true); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Authenticate(context.Background(), token); err == nil {
		t.Fatal("role change did not revoke session")
	}
	if _, err = manager.UpdateUser(context.Background(), admin.ID, auth.RoleViewer, true); !errors.Is(err, store.ErrLastActiveAdmin) {
		t.Fatalf("last admin lockout = %v", err)
	}
	if err = db.Audit(context.Background(), admin.ID, "integration.created", "integration:main", map[string]any{"token": "secret", "status": "active"}); err != nil {
		t.Fatal(err)
	}
	entries, err := db.AuditEntries(context.Background(), 10)
	if err != nil || len(entries) != 1 || entries[0].Metadata["token"] != nil || entries[0].Metadata["status"] != "active" {
		t.Fatalf("unsafe audit = %#v, %v", entries, err)
	}
}

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

func TestIntegrationDetailEditDisableAndHistoricalDeleteConflict(t *testing.T) {
	database, err := store.Open("file:" + t.TempDir() + "/connection-life.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	secrets, err := integration.NewEncryptedSecrets("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatal(err)
	}
	router := New(workflow.DefaultCatalog(), database, workflow.Adapters{Secrets: secrets})
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/integrations", bytes.NewBufferString(`{"key":"life","name":"Original","type":"gitea","config":{"base_url":"https://gitea.example"},"status":"active","secret":"secret-value"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", create.Code, create.Body.String())
	}
	before, err := database.Integration(context.Background(), "life")
	if err != nil {
		t.Fatal(err)
	}
	edit := httptest.NewRecorder()
	router.ServeHTTP(edit, httptest.NewRequest(http.MethodPatch, "/api/integrations/life", bytes.NewBufferString(`{"name":"Renamed","config":{"base_url":"https://gitea.example"},"status":"active"}`)))
	if edit.Code != http.StatusOK || strings.Contains(edit.Body.String(), "secret-value") {
		t.Fatalf("edit = %d %s", edit.Code, edit.Body.String())
	}
	after, err := database.Integration(context.Background(), "life")
	if err != nil || after.SecretCiphertext != before.SecretCiphertext {
		t.Fatalf("secret replacement = %#v, %v", after, err)
	}
	if _, err = database.Save(context.Background(), workflow.Definition{Key: "history", Name: "History", Nodes: []workflow.Node{{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "life"}}}}); err != nil {
		t.Fatal(err)
	}
	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(http.MethodDelete, "/api/integrations/life", nil))
	if remove.Code != http.StatusConflict {
		t.Fatalf("delete = %d %s", remove.Code, remove.Body.String())
	}
	disable := httptest.NewRecorder()
	router.ServeHTTP(disable, httptest.NewRequest(http.MethodPost, "/api/integrations/life/disable", nil))
	if disable.Code != http.StatusNoContent {
		t.Fatalf("disable = %d", disable.Code)
	}
}

func TestGiteaDiscoveryRequiresOrganizationBeforeScopedRepositories(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/user/orgs":
			_, _ = writer.Write([]byte(`[{"username":"acme"}]`))
		case "/api/v1/orgs/acme/repos":
			_, _ = writer.Write([]byte(`[{"name":"api"}]`))
		default:
			if request.URL.Path == "/api/v1/user" {
				_, _ = writer.Write([]byte(`{"login":"admin"}`))
				return
			}
			http.NotFound(writer, request)
		}
	}))
	defer provider.Close()
	database, err := store.Open("file:" + t.TempDir() + "/scoped-discovery.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	secrets, err := integration.NewEncryptedSecrets("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatal(err)
	}
	router := New(workflow.DefaultCatalog(), database, workflow.Adapters{Secrets: secrets})
	candidate := `{"key":"candidate","name":"Candidate","type":"gitea","config":{"base_url":"` + provider.URL + `"},"status":"active","secret":"secret"}`
	validate := httptest.NewRecorder()
	router.ServeHTTP(validate, httptest.NewRequest(http.MethodPost, "/api/integrations/validate", bytes.NewBufferString(candidate)))
	if validate.Code != http.StatusOK || !strings.Contains(validate.Body.String(), `"organizations":["acme"]`) {
		t.Fatalf("validate = %d %s", validate.Code, validate.Body.String())
	}
	candidateRepositories := httptest.NewRecorder()
	router.ServeHTTP(candidateRepositories, httptest.NewRequest(http.MethodPost, "/api/integrations/discover-repositories", bytes.NewBufferString(strings.TrimSuffix(candidate, "}")+`,"organization":"acme"}`)))
	if candidateRepositories.Code != http.StatusOK || !strings.Contains(candidateRepositories.Body.String(), `"name":"api"`) {
		t.Fatalf("candidate repositories = %d %s", candidateRepositories.Code, candidateRepositories.Body.String())
	}
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/integrations", bytes.NewBufferString(`{"key":"gitea","name":"Gitea","type":"gitea","config":{"base_url":"`+provider.URL+`"},"status":"active","secret":"secret"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", create.Code, create.Body.String())
	}
	organizations := httptest.NewRecorder()
	router.ServeHTTP(organizations, httptest.NewRequest(http.MethodGet, "/api/integrations/gitea/discover", nil))
	if organizations.Code != http.StatusOK || !strings.Contains(organizations.Body.String(), `"organizations":["acme"]`) {
		t.Fatalf("organizations = %d %s", organizations.Code, organizations.Body.String())
	}
	repositories := httptest.NewRecorder()
	router.ServeHTTP(repositories, httptest.NewRequest(http.MethodGet, "/api/integrations/gitea/discover?organization=acme", nil))
	if repositories.Code != http.StatusOK || !strings.Contains(repositories.Body.String(), `"owner":"acme"`) || !strings.Contains(repositories.Body.String(), `"name":"api"`) {
		t.Fatalf("repositories = %d %s", repositories.Code, repositories.Body.String())
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

func TestSignedGiteaWebhookRegistersQueuesDeduplicatesAndRejectsCollision(t *testing.T) {
	database, err := store.Open("file:" + t.TempDir() + "/webhook.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	definition := workflow.Definition{Key: "review", Name: "Review", Nodes: []workflow.Node{{Key: "gitea", Type: "trigger", Name: "Gitea", Config: map[string]any{"mode": "webhook"}}}}
	versionID, err := database.Save(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Publish(context.Background(), versionID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	secrets, err := integration.NewEncryptedSecrets("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := &recordingDispatcher{}
	router := New(workflow.DefaultCatalog(), database, workflow.Adapters{Secrets: secrets, Dispatcher: dispatcher})
	registration := httptest.NewRecorder()
	router.ServeHTTP(registration, httptest.NewRequest(http.MethodPost, "/api/webhook-registrations", bytes.NewBufferString(`{"key":"gitea-review","name":"Gitea Review","workflow_key":"review","trigger_node_key":"gitea","secret":"signing-secret","active":true}`)))
	if registration.Code != http.StatusCreated || strings.Contains(registration.Body.String(), "signing-secret") || strings.Contains(registration.Body.String(), "secret_ciphertext") {
		t.Fatalf("registration = %d %s", registration.Code, registration.Body.String())
	}
	stored, err := database.WebhookRegistration(context.Background(), "gitea-review")
	if err != nil || stored.SecretCiphertext == "" || strings.Contains(stored.SecretCiphertext, "signing-secret") {
		t.Fatalf("stored registration = %#v, %v", stored, err)
	}

	body := []byte(`{"action":"synchronized","number":42,"repository":{"name":"api","owner":{"login":"acme"}},"pull_request":{"number":42}}`)
	requestWebhook := func(delivery string, payload []byte, signature string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/webhooks/gitea/gitea-review", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Gitea-Event", "pull_request")
		request.Header.Set("X-Gitea-Delivery", delivery)
		request.Header.Set("X-Gitea-Signature", signature)
		router.ServeHTTP(response, request)
		return response
	}
	sign := func(payload []byte) string {
		mac := hmac.New(sha256.New, []byte("signing-secret"))
		_, _ = mac.Write(payload)
		return hex.EncodeToString(mac.Sum(nil))
	}
	if response := requestWebhook("delivery-1", body, strings.Repeat("0", 64)); response.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d %s", response.Code, response.Body.String())
	}
	first := requestWebhook("delivery-1", body, sign(body))
	if first.Code != http.StatusAccepted || len(dispatcher.ids) != 1 || !strings.Contains(first.Body.String(), `"duplicate":false`) {
		t.Fatalf("first delivery = %d %s; IDs = %#v", first.Code, first.Body.String(), dispatcher.ids)
	}
	duplicate := requestWebhook("delivery-1", body, sign(body))
	if duplicate.Code != http.StatusAccepted || len(dispatcher.ids) != 1 || !strings.Contains(duplicate.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate delivery = %d %s; IDs = %#v", duplicate.Code, duplicate.Body.String(), dispatcher.ids)
	}
	collisionBody := bytes.Replace(body, []byte(`"number":42`), []byte(`"number":43`), 2)
	collision := requestWebhook("delivery-1", collisionBody, sign(collisionBody))
	if collision.Code != http.StatusConflict || len(dispatcher.ids) != 1 {
		t.Fatalf("collision = %d %s; IDs = %#v", collision.Code, collision.Body.String(), dispatcher.ids)
	}
	missingHeaders := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webhooks/gitea/gitea-review", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(missingHeaders, request)
	if missingHeaders.Code != http.StatusBadRequest {
		t.Fatalf("missing headers = %d %s", missingHeaders.Code, missingHeaders.Body.String())
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
		{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea-secret-key"}},
		{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"integration": "gitea-secret-key"}},
	}}
	versionID, err := database.Save(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.CreateExecution(context.Background(), versionID, map[string]any{"token": "must-not-leak", "pull_request": map[string]any{"owner": "dynamic", "repo": "service", "number": 73}, "ignored": map[string]any{"secret": "hidden"}}); err != nil {
		t.Fatal(err)
	}
	router := New(workflow.DefaultCatalog(), database)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/executions?limit=1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"owner":"dynamic"`) || !strings.Contains(response.Body.String(), `"repo":"service"`) || !strings.Contains(response.Body.String(), `"pull_request":73`) {
		t.Fatalf("missing review context: %s", response.Body.String())
	}
	for _, forbidden := range []string{"token", "must-not-leak", "hidden", "ignored", "gitea-secret-key", "nodes", "error"} {
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
