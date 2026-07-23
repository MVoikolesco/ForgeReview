package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

func (m modelResponse) Chat(context.Context, integration.Integration, string, string) (integration.ChatResult, error) {
	return integration.ChatResult{Content: string(m)}, nil
}

func TestNodeProgressUpsertsRunningToTerminalWithoutDuplicateRows(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/progress.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	versionID, err := database.Save(context.Background(), workflow.Definition{Key: "progress", Name: "Progress", Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}})
	if err != nil {
		t.Fatal(err)
	}
	executionID, err := database.CreateExecution(context.Background(), versionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, claimErr := database.ClaimExecution(context.Background(), executionID); claimErr != nil || !claimed {
		t.Fatalf("claim = %t, %v", claimed, claimErr)
	}
	if err = database.SaveNodeProgress(context.Background(), executionID, workflow.NodeRun{NodeKey: "start", ScopeKey: "root", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	live, err := database.Execution(context.Background(), executionID)
	if err != nil || live.Status != "running" || len(live.Runs) != 1 || live.Runs[0].Status != "running" {
		t.Fatalf("live report = %#v, %v", live, err)
	}
	final := workflow.RunReport{Status: "completed", Runs: []workflow.NodeRun{{NodeKey: "start", ScopeKey: "root", Status: "completed", DurationMS: 2}}}
	if err = database.CompleteExecution(context.Background(), executionID, final); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err = database.db.QueryRow(`SELECT COUNT(*) FROM workflow_node_runs WHERE workflow_execution_id=?`, executionID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	completed, err := database.Execution(context.Background(), executionID)
	if err != nil || rows != 1 || len(completed.Runs) != 1 || completed.Runs[0].Status != "completed" || completed.Runs[0].ScopeKey != "root" {
		t.Fatalf("completed = %#v, rows=%d, err=%v", completed, rows, err)
	}
}

func TestExecutionStatusAndReplayEventsExcludeSensitivePayloads(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/safe-status.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	versionID, err := database.Save(context.Background(), workflow.Definition{Key: "safe", Name: "Safe", Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := database.CreateExecution(context.Background(), versionID, map[string]any{"token": "must-not-leak"})
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := database.ClaimExecution(context.Background(), id); err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	run := workflow.NodeRun{NodeKey: "start", ScopeKey: "root", Status: "completed", Inputs: map[string][]any{"event": {"sensitive"}}, Outputs: []workflow.Token{{NodeKey: "start", PortKey: "event", Value: "sensitive"}}}
	if err = database.SaveNodeProgress(context.Background(), id, run); err != nil {
		t.Fatal(err)
	}
	status, err := database.Execution(context.Background(), id)
	if err != nil || status.Status != "running" || len(status.Runs) != 1 || status.Runs[0].NodeKey != "start" {
		t.Fatalf("safe status = %#v, %v", status, err)
	}
	payload, _ := json.Marshal(status)
	if strings.Contains(string(payload), "sensitive") || strings.Contains(string(payload), "token") {
		t.Fatalf("status leaked payload: %s", payload)
	}
	events, err := database.ExecutionEvents(context.Background(), 0, id, 10)
	if err != nil || len(events) < 3 {
		t.Fatalf("events = %#v, %v", events, err)
	}
	payload, _ = json.Marshal(events)
	if strings.Contains(string(payload), "sensitive") || strings.Contains(string(payload), "token") {
		t.Fatalf("events leaked payload: %s", payload)
	}
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
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/repos/acme/review/pulls/7/reviews" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "token gitea-secret" {
			t.Fatal("missing Gitea authorization")
		}
		var payload struct {
			Event    string `json:"event"`
			Comments []struct {
				Path        string `json:"path"`
				NewPosition int    `json:"new_position"`
			} `json:"comments"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Event != "COMMENT" || len(payload.Comments) != 1 || payload.Comments[0].Path != "api/main.go" || payload.Comments[0].NewPosition != 2 {
			t.Fatalf("review payload = %#v", payload)
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

func TestModelProfileRequiresAnLLMConnection(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/profiles.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config := json.RawMessage(`{"base_url":"https://models.example"}`)
	item := encryptedStoreIntegration(t, integration.Integration{Key: "models", Name: "Models", Type: integration.TypeOpenAI, Config: config, Status: integration.StatusActive}, "secret")
	if err = database.CreateIntegration(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	profile := integration.ModelProfile{Key: "reviewer", Name: "Reviewer", IntegrationKey: "models", Model: "qwen2.5-coder", Status: integration.StatusActive}
	if err = database.CreateModelProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	profiles, err := database.ModelProfiles(context.Background())
	if err != nil || len(profiles) != 1 || profiles[0] != profile {
		t.Fatalf("profiles = %#v, %v", profiles, err)
	}
}

func TestReplaceSelectedResourcesIsTransactional(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/resources.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config := json.RawMessage(`{"base_url":"https://models.example"}`)
	item := encryptedStoreIntegration(t, integration.Integration{Key: "models", Name: "Models", Type: integration.TypeOpenAI, Config: config, Status: integration.StatusActive}, "secret")
	if err = database.CreateIntegration(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	profiles, err := database.ReplaceModelProfiles(context.Background(), "models", []string{"one", "two", "one"})
	if err != nil || len(profiles) != 2 {
		t.Fatalf("first replace = %#v, %v", profiles, err)
	}
	profiles, err = database.ReplaceModelProfiles(context.Background(), "models", []string{"three"})
	if err != nil || len(profiles) != 1 || profiles[0].Model != "three" {
		t.Fatalf("replacement = %#v, %v", profiles, err)
	}
	stored, err := database.ModelProfiles(context.Background())
	if err != nil || len(stored) != 1 || stored[0].Model != "three" {
		t.Fatalf("stored = %#v, %v", stored, err)
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

func TestOpenMigratesFixedPullRequestCoordinatesWithoutChangingVersionIdentity(t *testing.T) {
	path := "file:" + t.TempDir() + "/fixed-coordinates.db"
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Exec(`CREATE TABLE workflow_versions (
 id INTEGER PRIMARY KEY AUTOINCREMENT, workflow_key TEXT NOT NULL, version INTEGER NOT NULL,
 name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', status TEXT NOT NULL,
 definition_json TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(workflow_key,version));`); err != nil {
		t.Fatal(err)
	}
	sole := workflow.Definition{Key: "sole", Name: "Sole", Nodes: []workflow.Node{
		{Key: "fetch", Type: "fetch", Name: "Fetch", Config: map[string]any{"integration": "gitea", "owner": "acme", "repo": "api", "pull_request": 4}},
		{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"integration": "gitea", "owner": "acme", "repo": "api", "pull_request": 4}},
	}}
	matched := workflow.Definition{Key: "matched", Name: "Matched", Nodes: []workflow.Node{
		{Key: "fetch-a", Type: "fetch", Name: "Fetch A", Config: map[string]any{"owner": "acme", "repo": "api", "pull_request": 1}},
		{Key: "fetch-b", Type: "fetch", Name: "Fetch B", Config: map[string]any{"owner": "acme", "repo": "api", "pull_request": 2}},
		{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"owner": "acme", "repo": "api", "pull_request": 2}},
	}}
	soleJSON, _ := json.Marshal(sole)
	matchedJSON, _ := json.Marshal(matched)
	if _, err = legacy.Exec(`INSERT INTO workflow_versions(id,workflow_key,version,name,status,definition_json) VALUES(41,'sole',3,'Sole','archived',?),(42,'matched',7,'Matched','published',?)`, string(soleJSON), string(matchedJSON)); err != nil {
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
	for _, expectation := range []struct {
		id, version int64
		status      string
		source      string
	}{{41, 3, "archived", "fetch"}, {42, 7, "published", "fetch-b"}} {
		definition, loadErr := database.Load(context.Background(), expectation.id)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		for _, node := range definition.Nodes {
			if node.Type == "fetch" || node.Type == "publish" {
				for _, key := range []string{"owner", "repo", "pull_request"} {
					if _, exists := node.Config[key]; exists {
						t.Fatalf("version %d retained %s on %s: %#v", expectation.id, key, node.Key, node.Config)
					}
				}
			}
		}
		if len(definition.Edges) != 1 || definition.Edges[0].FromNode != expectation.source || definition.Edges[0].FromPort != "pull_request" || definition.Edges[0].ToNode != "publish" || definition.Edges[0].ToPort != "pull_request" {
			t.Fatalf("version %d migrated edges = %#v", expectation.id, definition.Edges)
		}
		var version int64
		var status string
		if err = database.db.QueryRow(`SELECT version,status FROM workflow_versions WHERE id=?`, expectation.id).Scan(&version, &status); err != nil || version != expectation.version || status != expectation.status {
			t.Fatalf("version identity %d = %d/%s, %v", expectation.id, version, status, err)
		}
	}
}

func TestOpenRejectsAmbiguousFixedCoordinateMigrationAtomically(t *testing.T) {
	path := "file:" + t.TempDir() + "/ambiguous-coordinates.db"
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Exec(`CREATE TABLE workflow_versions (
 id INTEGER PRIMARY KEY AUTOINCREMENT, workflow_key TEXT NOT NULL, version INTEGER NOT NULL,
 name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', status TEXT NOT NULL,
 definition_json TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(workflow_key,version));`); err != nil {
		t.Fatal(err)
	}
	definition := workflow.Definition{Key: "ambiguous", Name: "Ambiguous", Nodes: []workflow.Node{
		{Key: "fetch-a", Type: "fetch", Name: "Fetch A", Config: map[string]any{"owner": "acme", "repo": "api", "pull_request": 1}},
		{Key: "fetch-b", Type: "fetch", Name: "Fetch B", Config: map[string]any{"owner": "acme", "repo": "api", "pull_request": 1}},
		{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"owner": "acme", "repo": "api", "pull_request": 1}},
	}}
	payload, _ := json.Marshal(definition)
	if _, err = legacy.Exec(`INSERT INTO workflow_versions(id,workflow_key,version,name,status,definition_json) VALUES(9,'ambiguous',1,'Ambiguous','draft',?)`, string(payload)); err != nil {
		t.Fatal(err)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if database, openErr := Open(path); openErr == nil || !strings.Contains(openErr.Error(), `workflow version 9`) || !strings.Contains(openErr.Error(), `publish card "publish"`) || !strings.Contains(openErr.Error(), "ambiguous") {
		if database != nil {
			database.Close()
		}
		t.Fatalf("ambiguous migration error = %v", openErr)
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var stored string
	if err = check.QueryRow(`SELECT definition_json FROM workflow_versions WHERE id=9`).Scan(&stored); err != nil || !strings.Contains(stored, `"owner":"acme"`) {
		t.Fatalf("failed migration changed payload: %s, %v", stored, err)
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

func TestQueuedExecutionRecoveryRetainsSelectedTriggerAndInput(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/queued-recovery.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	versionID, err := database.Save(context.Background(), workflow.Definition{Key: "recovery", Name: "Recovery", Nodes: []workflow.Node{{Key: "api", Type: "trigger", Name: "API", Config: map[string]any{"mode": "api"}}}})
	if err != nil {
		t.Fatal(err)
	}
	executionID, err := database.CreateTriggeredExecution(context.Background(), versionID, "api", map[string]any{"request": "durable"})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := database.QueuedExecutionIDs(context.Background())
	if err != nil || len(queued) != 1 || queued[0] != executionID {
		t.Fatalf("queued recovery IDs = %#v, %v", queued, err)
	}
	execution, claimed, err := database.ClaimExecution(context.Background(), executionID)
	if err != nil || !claimed || execution.TriggerNodeKey != "api" || execution.Input["request"] != "durable" {
		t.Fatalf("recovered execution = %#v, %t, %v", execution, claimed, err)
	}
}

func TestExecutionFailureRetriesAreBoundedAndPermanentOrUncertainFailuresDeadLetter(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/failure-policy.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	versionID, err := database.Save(context.Background(), workflow.Definition{Key: "retry-policy", Name: "Retry policy", Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}})
	if err != nil {
		t.Fatal(err)
	}
	newRunning := func() int64 {
		id, createErr := database.CreateExecution(context.Background(), versionID, map[string]any{"payload": "private"})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, claimed, claimErr := database.ClaimExecution(context.Background(), id); claimErr != nil || !claimed {
			t.Fatalf("claim %d = %t, %v", id, claimed, claimErr)
		}
		return id
	}
	id := newRunning()
	for attempt := 1; attempt <= maxExecutionRetries; attempt++ {
		retry, failureErr := database.HandleExecutionFailure(context.Background(), id, "transient")
		if failureErr != nil || !retry {
			t.Fatalf("attempt %d = retry %t, err %v", attempt, retry, failureErr)
		}
		var retries int
		var next string
		if err = database.db.QueryRow(`SELECT retry_count,next_retry_at FROM workflow_executions WHERE id=?`, id).Scan(&retries, &next); err != nil || retries != attempt || next == "" {
			t.Fatalf("retry state %d = %d/%q, %v", attempt, retries, next, err)
		}
		if _, err = database.db.Exec(`UPDATE workflow_executions SET next_retry_at=NULL WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		if _, claimed, claimErr := database.ClaimExecution(context.Background(), id); claimErr != nil || !claimed {
			t.Fatalf("reclaim %d = %t, %v", attempt, claimed, claimErr)
		}
	}
	if retry, failureErr := database.HandleExecutionFailure(context.Background(), id, "transient"); failureErr != nil || retry {
		t.Fatalf("exhausted retry = %t, %v", retry, failureErr)
	}
	for _, class := range []string{"permanent", "uncertain"} {
		terminal := newRunning()
		if retry, failureErr := database.HandleExecutionFailure(context.Background(), terminal, class); failureErr != nil || retry {
			t.Fatalf("%s = retry %t, err %v", class, retry, failureErr)
		}
		var status, persistedClass string
		if err = database.db.QueryRow(`SELECT status,last_failure_class FROM workflow_executions WHERE id=?`, terminal).Scan(&status, &persistedClass); err != nil || status != "dead_letter" || persistedClass != class {
			t.Fatalf("%s terminal state = %s/%s, %v", class, status, persistedClass, err)
		}
	}
}

func TestRecoverStaleExecutionAndReplayDeadLetterAreSafeAndAuditable(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/recovery-replay.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	versionID, err := database.Save(context.Background(), workflow.Definition{Key: "recovery-replay", Name: "Recovery replay", Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}})
	if err != nil {
		t.Fatal(err)
	}
	stale, _ := database.CreateExecution(context.Background(), versionID, map[string]any{"secret_payload": "never public"})
	fresh, _ := database.CreateExecution(context.Background(), versionID, nil)
	for _, id := range []int64{stale, fresh} {
		if _, claimed, claimErr := database.ClaimExecution(context.Background(), id); claimErr != nil || !claimed {
			t.Fatalf("claim = %t, %v", claimed, claimErr)
		}
	}
	if _, err = database.db.Exec(`UPDATE workflow_executions SET started_at=datetime('now','-20 minutes') WHERE id=?`, stale); err != nil {
		t.Fatal(err)
	}
	ids, err := database.RecoverableExecutionIDs(context.Background(), 15*time.Minute)
	if err != nil || len(ids) != 1 || ids[0] != stale {
		t.Fatalf("recovery = %#v, %v", ids, err)
	}
	if _, err = database.RecoverableExecutionIDs(context.Background(), 0); err == nil {
		t.Fatal("zero stale threshold was accepted")
	}
	if _, claimed, claimErr := database.ClaimExecution(context.Background(), stale); claimErr != nil || !claimed {
		t.Fatalf("claim recovered = %t, %v", claimed, claimErr)
	}
	if _, err = database.HandleExecutionFailure(context.Background(), stale, "permanent"); err != nil {
		t.Fatal(err)
	}
	replay, err := database.ReplayDeadLetterExecution(context.Background(), stale)
	if err != nil || replay == stale {
		t.Fatalf("replay = %d, %v", replay, err)
	}
	actor, err := database.CreateUser(context.Background(), "admin@example.test", "test-hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Audit(context.Background(), actor.ID, "execution.dead_letter_replayed", "execution:"+strconv.FormatInt(replay, 10), map[string]any{"source_execution_id": stale, "token": "must-not-store"}); err != nil {
		t.Fatal(err)
	}
	audit, err := database.AuditEntries(context.Background(), 1)
	if err != nil || len(audit) != 1 || audit[0].Action != "execution.dead_letter_replayed" || audit[0].Metadata["token"] != nil || audit[0].Metadata["source_execution_id"] != float64(stale) {
		t.Fatalf("audit = %#v, %v", audit, err)
	}
}

func TestMetricsAreAggregateOnly(t *testing.T) {
	database, err := Open("file:" + t.TempDir() + "/metrics.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	versionID, err := database.Save(context.Background(), workflow.Definition{Key: "metrics", Name: "Metrics", Nodes: []workflow.Node{{Key: "start", Type: "trigger", Name: "Start"}}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := database.CreateExecution(context.Background(), versionID, map[string]any{"secret": "do-not-expose"})
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := database.Metrics(context.Background())
	payload, _ := json.Marshal(metrics)
	if err != nil || metrics.QueueDepth != 1 || strings.Contains(string(payload), "secret") || strings.Contains(string(payload), "payload") {
		t.Fatalf("metrics = %s, %v", payload, err)
	}
	_ = id
}

func publicationDefinition() workflow.Definition {
	return workflow.Definition{Key: "publication", Name: "Publication", Nodes: []workflow.Node{
		{Key: "start", Type: "trigger", Name: "Start", Config: map[string]any{"event": map[string]any{"pull_request": map[string]any{"owner": "acme", "repo": "review", "number": 7}}}},
		{Key: "target", Type: "transform", Name: "Target"},
		{Key: "template", Type: "template", Name: "Template", Config: map[string]any{"template": "review"}},
		{Key: "model", Type: "model", Name: "Model", Config: map[string]any{"integration": "model"}},
		{Key: "validate", Type: "validate", Name: "Validate"},
		{Key: "filter", Type: "response_filter", Name: "Filter"},
		{Key: "consolidate", Type: "consolidate", Name: "Consolidate"},
		{Key: "format", Type: "format", Name: "Format"},
		{Key: "publish", Type: "publish", Name: "Publish", Config: map[string]any{"integration": "gitea"}},
	}, Edges: []workflow.Edge{
		{Key: "context", FromNode: "start", FromPort: "event", ToNode: "template", ToPort: "context"},
		{Key: "target-input", FromNode: "start", FromPort: "event", ToNode: "target", ToPort: "input"},
		{Key: "target-output", FromNode: "target", FromPort: "output", ToNode: "publish", ToPort: "pull_request"},
		{Key: "prompt", FromNode: "template", FromPort: "prompt", ToNode: "model", ToPort: "prompt"},
		{Key: "response", FromNode: "model", FromPort: "response", ToNode: "validate", ToPort: "response"},
		{Key: "valid", FromNode: "validate", FromPort: "valid", ToNode: "filter", ToPort: "response"},
		{Key: "comments", FromNode: "filter", FromPort: "comments", ToNode: "consolidate", ToPort: "comments"},
		{Key: "review", FromNode: "consolidate", FromPort: "review", ToNode: "format", ToPort: "review"},
		{Key: "formatted", FromNode: "format", FromPort: "formatted", ToNode: "publish", ToPort: "formatted_review"},
	}}
}
