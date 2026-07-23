package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"forgereview/backend/internal/auth"
	"forgereview/backend/internal/store"
	"forgereview/backend/internal/workflow"
)

func TestAuthenticationAndRolesProtectWrites(t *testing.T) {
	db, err := store.Open("file:" + t.TempDir() + "/roles.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := auth.New(db, auth.Config{SigningKey: strings.Repeat("k", 32)})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := manager.CreateUser(context.Background(), "admin@example.test", "a secure admin password", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := manager.CreateUser(context.Background(), "viewer@example.test", "a secure viewer password", auth.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	editor, err := manager.CreateUser(context.Background(), "editor@example.test", "a secure editor password", auth.RoleEditor)
	if err != nil {
		t.Fatal(err)
	}
	router := NewWithAuth(workflow.DefaultCatalog(), db, manager)
	request := func(method, path string, body []byte, email, password string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if email != "" {
			_, token, _, loginErr := manager.Login(context.Background(), email, password)
			if loginErr != nil {
				t.Fatal(loginErr)
			}
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		router.ServeHTTP(r, req)
		return r
	}
	if response := request(http.MethodGet, "/api/workflows", nil, "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", response.Code)
	}
	workflowBody := []byte(`{"key":"role-test","name":"Role test","nodes":[{"key":"start","type":"trigger","name":"Start"}]}`)
	if response := request(http.MethodPost, "/api/workflows", workflowBody, viewer.Email, "a secure viewer password"); response.Code != http.StatusForbidden {
		t.Fatalf("viewer write = %d", response.Code)
	}
	if response := request(http.MethodPost, "/api/workflows", workflowBody, editor.Email, "a secure editor password"); response.Code != http.StatusCreated {
		t.Fatalf("editor workflow = %d: %s", response.Code, response.Body.String())
	}
	integrationBody := []byte(`{"key":"gitea","name":"Gitea","type":"gitea","config":{"base_url":"https://gitea.example"},"status":"active","secret":"secret"}`)
	if response := request(http.MethodPost, "/api/integrations", integrationBody, editor.Email, "a secure editor password"); response.Code != http.StatusForbidden {
		t.Fatalf("editor integration = %d", response.Code)
	}
	if response := request(http.MethodPost, "/api/integrations", integrationBody, admin.Email, "a secure admin password"); response.Code != http.StatusInternalServerError {
		t.Fatalf("admin reached integration handler = %d", response.Code)
	}
}

func TestAPITriggerRequiresAuthenticationAndQueuesSelectedPublishedTrigger(t *testing.T) {
	db, err := store.Open("file:" + t.TempDir() + "/api-trigger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := auth.New(db, auth.Config{SigningKey: strings.Repeat("k", 32)})
	if err != nil {
		t.Fatal(err)
	}
	editor, err := manager.CreateUser(context.Background(), "editor@example.test", "a secure editor password", auth.RoleEditor)
	if err != nil {
		t.Fatal(err)
	}
	definition := workflow.Definition{Key: "automation", Name: "Automation", Nodes: []workflow.Node{
		{Key: "manual", Type: "trigger", Name: "Manual", Config: map[string]any{"mode": "manual"}},
		{Key: "api", Type: "trigger", Name: "API", Config: map[string]any{"mode": "api"}},
	}}
	versionID, err := db.Save(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Publish(context.Background(), versionID, workflow.DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
	dispatcher := &recordingDispatcher{}
	router := NewWithAuth(workflow.DefaultCatalog(), db, manager, workflow.Adapters{Dispatcher: dispatcher})
	path := "/api/workflows/automation/triggers/api/executions"
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"pull_request":{"owner":"acme","repo":"api","number":42}}`)))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated trigger = %d", unauthenticated.Code)
	}
	_, token, _, err := manager.Login(context.Background(), editor.Email, "a secure editor password")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"pull_request":{"owner":"acme","repo":"api","number":42}}`))
	request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || len(dispatcher.ids) != 1 {
		t.Fatalf("authenticated trigger = %d %s; IDs = %#v", response.Code, response.Body.String(), dispatcher.ids)
	}
	execution, claimed, err := db.ClaimExecution(context.Background(), dispatcher.ids[0])
	if err != nil || !claimed || execution.TriggerNodeKey != "api" || execution.Input["pull_request"] == nil {
		t.Fatalf("persisted selected trigger = %#v, %t, %v", execution, claimed, err)
	}
}

func TestSessionCookieSurvivesRepeatedAuthenticatedExecutionPolls(t *testing.T) {
	db, err := store.Open("file:" + t.TempDir() + "/execution-polls.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager, err := auth.New(db, auth.Config{SigningKey: strings.Repeat("k", 32)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.CreateUser(context.Background(), "editor@example.test", "a secure editor password", auth.RoleEditor); err != nil {
		t.Fatal(err)
	}
	versionID, err := db.Save(context.Background(), workflow.Definition{Key: "polls", Name: "Polls", Nodes: []workflow.Node{{Key: "manual", Type: "trigger", Name: "Manual", Config: map[string]any{"mode": "manual"}}}})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := &recordingDispatcher{}
	router := NewWithAuth(workflow.DefaultCatalog(), db, manager, workflow.Adapters{Dispatcher: dispatcher})
	server := httptest.NewServer(router)
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	login, err := client.Post(server.URL+"/api/auth/login", "application/json", bytes.NewBufferString(`{"email":"editor@example.test","password":"a secure editor password"}`))
	if err != nil {
		t.Fatal(err)
	}
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", login.StatusCode)
	}
	login.Body.Close()
	cookies := jar.Cookies(login.Request.URL)
	if len(cookies) != 1 || cookies[0].Name != auth.CookieName || cookies[0].Value == "" {
		t.Fatalf("cookie jar did not retain the login session: %#v", cookies)
	}
	start, err := http.NewRequest(http.MethodPost, server.URL+"/api/workflow-versions/"+strconv.FormatInt(versionID, 10)+"/executions", bytes.NewBufferString(`{"trigger_node":"manual","payload":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	start.Header.Set("Content-Type", "application/json")
	started, err := client.Do(start)
	if err != nil {
		t.Fatal(err)
	}
	if started.StatusCode != http.StatusAccepted || len(dispatcher.ids) != 1 {
		t.Fatalf("start = %d; IDs = %#v", started.StatusCode, dispatcher.ids)
	}
	started.Body.Close()
	for attempt := 0; attempt < 3; attempt++ {
		poll, err := client.Get(server.URL + "/api/executions/" + strconv.FormatInt(dispatcher.ids[0], 10))
		if err != nil {
			t.Fatal(err)
		}
		if poll.StatusCode != http.StatusOK {
			poll.Body.Close()
			t.Fatalf("poll %d = %d", attempt+1, poll.StatusCode)
		}
		if attempt == 0 && !strings.Contains(readBody(t, poll), `"runs":[]`) {
			t.Fatalf("queued report must provide an empty runs array")
		}
		poll.Body.Close()
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	var body bytes.Buffer
	if _, err := body.ReadFrom(response.Body); err != nil {
		t.Fatal(err)
	}
	return body.String()
}
