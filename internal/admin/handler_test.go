package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gitea-agents/internal/config"
	"gitea-agents/internal/store"
)

func TestGiteaTokenIsEncryptedAndNeverReturned(t *testing.T) {
	os.Setenv("GITEA_TOKEN_ENCRYPTION_KEY", "01234567890123456789012345678901")
	defer os.Unsetenv("GITEA_TOKEN_ENCRYPTION_KEY")
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, s, "admin", "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/admin/gitea/instances", strings.NewReader(`{"name":"main","base_url":"https://gitea.example","bot_username":"bot","token":"do-not-return"}`))
	req.SetBasicAuth("admin", "secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != 201 {
		t.Fatalf("create=%d %s", res.Code, res.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(res.Body.Bytes(), &body)
	if _, ok := body["token"]; ok {
		t.Fatal("token returned")
	}
	if _, ok := body["token_ciphertext"]; ok {
		t.Fatal("ciphertext returned")
	}
	var ciphertext string
	if err = s.DB.QueryRow("SELECT token_ciphertext FROM gitea_instances").Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if ciphertext == "do-not-return" || ciphertext == "" {
		t.Fatal("token was not encrypted")
	}
}

func TestAIKeyIsEncryptedWriteOnlyAndFailsClosed(t *testing.T) {
	t.Setenv("GITEA_TOKEN_ENCRYPTION_KEY", "01234567890123456789012345678901")
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.Seed(context.Background(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, s, "admin", "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/admin/ai/connections", strings.NewReader(`{"provider_id":2,"name":"private","base_url":"https://example.test","api_key":"not-returned"}`))
	req.SetBasicAuth("admin", "secret")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "not-returned") || strings.Contains(res.Body.String(), "ciphertext") {
		t.Fatalf("secret leaked: %s", res.Body.String())
	}
	var ciphertext string
	if err = s.DB.QueryRow("SELECT api_key_ciphertext FROM ai_connections WHERE id=1").Scan(&ciphertext); err != nil || ciphertext == "" || ciphertext == "not-returned" {
		t.Fatalf("ciphertext=%q err=%v", ciphertext, err)
	}
	if _, err = s.DB.Exec("UPDATE ai_connections SET api_key_ciphertext='invalid'"); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/admin/ai/connections/1/test", nil)
	req.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest || strings.Contains(res.Body.String(), "not-returned") {
		t.Fatalf("fail closed=%d %s", res.Code, res.Body.String())
	}
}

func TestAuthenticatedProvidersRejectConnectionsWithoutAPIKey(t *testing.T) {
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.Seed(context.Background(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, s, "admin", "secret")

	for _, provider := range []string{"google_gemini", "groq"} {
		t.Run(provider, func(t *testing.T) {
			var providerID int64
			if err := s.DB.QueryRow("SELECT id FROM ai_providers WHERE name=?", provider).Scan(&providerID); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/admin/ai/connections", strings.NewReader(fmt.Sprintf(`{"provider_id":%d,"name":"%s","base_url":"https://example.test"}`, providerID, provider)))
			req.SetBasicAuth("admin", "secret")
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("create=%d %s", res.Code, res.Body.String())
			}
			var count int
			if err := s.DB.QueryRow("SELECT COUNT(*) FROM ai_connections WHERE provider_id=?", providerID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("connection created without API key: %d", count)
			}
		})
	}
}

func TestGiteaHasOneConnectionAndDeletesItsRepositories(t *testing.T) {
	os.Setenv("GITEA_TOKEN_ENCRYPTION_KEY", "01234567890123456789012345678901")
	defer os.Unsetenv("GITEA_TOKEN_ENCRYPTION_KEY")
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, s, "admin", "secret")
	create := func(name string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/gitea/instances", strings.NewReader(fmt.Sprintf(`{"name":%q,"base_url":"https://gitea.example","bot_username":"bot","token":"secret"}`, name)))
		req.SetBasicAuth("admin", "secret")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	if res := create("main"); res.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", res.Code, res.Body.String())
	}
	if res := create("second"); res.Code != http.StatusConflict {
		t.Fatalf("duplicate=%d %s", res.Code, res.Body.String())
	}
	if _, err = s.DB.Exec(`INSERT INTO repositories(gitea_instance_id,owner,name,full_name) VALUES(1,'org','repo','org/repo')`); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/gitea/instances/1", nil)
	req.SetBasicAuth("admin", "secret")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", res.Code, res.Body.String())
	}
	var count int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM repositories").Scan(&count); err != nil || count != 0 {
		t.Fatalf("repositories remain: count=%d err=%v", count, err)
	}
}

func TestRepositoriesRouteAndAuthentication(t *testing.T) {
	s, e := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	Register(mux, s, "admin", "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/repositories", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/admin/repositories", nil)
	req.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
}

func TestSetDefaultModelIsScopedToItsConnectionAndPreservesOnMissingTarget(t *testing.T) {
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`INSERT INTO ai_providers(id,name,display_name,base_url,auth_type) VALUES(1,'ollama','Ollama','http://ollama','none');
INSERT INTO ai_connections(id,provider_id,name) VALUES(1,1,'one'),(2,1,'two');
INSERT INTO ai_models(id,connection_id,provider_model_name,display_name,is_default) VALUES(1,1,'a','A',1),(2,1,'b','B',0),(3,2,'c','C',1);`)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, s, "admin", "secret")
	setDefault := func(id int) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/ai/models/%d/set-default", id), nil)
		req.SetBasicAuth("admin", "secret")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	if res := setDefault(2); res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var first, second, other int
	if err = s.DB.QueryRow("SELECT is_default FROM ai_models WHERE id=1").Scan(&first); err != nil {
		t.Fatal(err)
	}
	_ = s.DB.QueryRow("SELECT is_default FROM ai_models WHERE id=2").Scan(&second)
	_ = s.DB.QueryRow("SELECT is_default FROM ai_models WHERE id=3").Scan(&other)
	if first != 0 || second != 1 || other != 1 {
		t.Fatalf("defaults after update: first=%d second=%d other=%d", first, second, other)
	}
	if res := setDefault(999); res.Code != http.StatusNotFound {
		t.Fatalf("missing target status=%d body=%s", res.Code, res.Body.String())
	}
	_ = s.DB.QueryRow("SELECT is_default FROM ai_models WHERE id=2").Scan(&second)
	if second != 1 {
		t.Fatal("missing target cleared the previous default")
	}
}

func TestSetDefaultProviderAndConnectionAreScoped(t *testing.T) {
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`INSERT INTO ai_providers(id,name,display_name,base_url,auth_type,is_default) VALUES(1,'ollama','Ollama','http://ollama','none',1),(2,'openrouter','OpenRouter','https://openrouter.ai/api/v1','bearer',0);
INSERT INTO ai_connections(id,provider_id,name,is_default) VALUES(1,1,'ollama-one',1),(2,1,'ollama-two',0),(3,2,'router-one',1),(4,2,'router-two',0);`)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, s, "admin", "secret")
	post := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.SetBasicAuth("admin", "secret")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	if res := post("/api/admin/ai/providers/2/set-default"); res.Code != http.StatusOK {
		t.Fatalf("provider status=%d body=%s", res.Code, res.Body.String())
	}
	if res := post("/api/admin/ai/connections/2/set-default"); res.Code != http.StatusOK {
		t.Fatalf("connection status=%d body=%s", res.Code, res.Body.String())
	}
	var ollamaOne, ollamaTwo, routerOne, routerTwo, providerOne, providerTwo int
	_ = s.DB.QueryRow("SELECT is_default FROM ai_connections WHERE id=1").Scan(&ollamaOne)
	_ = s.DB.QueryRow("SELECT is_default FROM ai_connections WHERE id=2").Scan(&ollamaTwo)
	_ = s.DB.QueryRow("SELECT is_default FROM ai_connections WHERE id=3").Scan(&routerOne)
	_ = s.DB.QueryRow("SELECT is_default FROM ai_connections WHERE id=4").Scan(&routerTwo)
	_ = s.DB.QueryRow("SELECT is_default FROM ai_providers WHERE id=1").Scan(&providerOne)
	_ = s.DB.QueryRow("SELECT is_default FROM ai_providers WHERE id=2").Scan(&providerTwo)
	if providerOne != 0 || providerTwo != 1 || ollamaOne != 0 || ollamaTwo != 1 || routerOne != 1 || routerTwo != 0 {
		t.Fatalf("provider defaults %d/%d connection defaults %d/%d/%d/%d", providerOne, providerTwo, ollamaOne, ollamaTwo, routerOne, routerTwo)
	}
}

func TestAddingAndRemovingModelMaintainsReferences(t *testing.T) {
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.Seed(context.Background(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`INSERT INTO ai_connections(id,provider_id,name,is_default) VALUES(1,1,'main',1); INSERT INTO ai_models(id,connection_id,provider_model_name,display_name,is_default) VALUES(1,1,'old','Old',1); INSERT INTO model_parameters(model_id) VALUES(1); INSERT INTO review_profiles(id,name,model_id,is_default) VALUES(1,'default',1,1);`)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, s, "admin", "secret")
	post := httptest.NewRequest(http.MethodPost, "/api/admin/setup/add-model", strings.NewReader(`{"connection_id":1,"model":{"id":"new","name":"New"},"parameters":{},"make_default":true}`))
	post.SetBasicAuth("admin", "secret")
	post.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, post)
	if res.Code != http.StatusCreated {
		t.Fatalf("add=%d %s", res.Code, res.Body.String())
	}
	var newDefault int
	_ = s.DB.QueryRow("SELECT is_default FROM ai_models WHERE provider_model_name='new'").Scan(&newDefault)
	if newDefault != 1 {
		t.Fatal("new model was not made default")
	}
	del := httptest.NewRequest(http.MethodDelete, "/api/admin/ai/models/2", nil)
	del.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, del)
	if res.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", res.Code, res.Body.String())
	}
	var modelID int
	if err = s.DB.QueryRow("SELECT model_id FROM review_profiles WHERE id=1").Scan(&modelID); err != nil || modelID != 1 {
		t.Fatalf("profile model=%d err=%v", modelID, err)
	}
}

func TestDeletingDefaultConnectionPromotesSameProvider(t *testing.T) {
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`INSERT INTO ai_providers(id,name,display_name) VALUES(1,'ollama','Ollama'); INSERT INTO ai_connections(id,provider_id,name,is_default) VALUES(1,1,'one',1),(2,1,'two',0); INSERT INTO ai_models(id,connection_id,provider_model_name,display_name,is_default) VALUES(1,1,'old','Old',1),(2,2,'new','New',1); INSERT INTO model_parameters(model_id) VALUES(1),(2);`)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, s, "admin", "secret")
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/ai/connections/1", nil)
	req.SetBasicAuth("admin", "secret")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", res.Code, res.Body.String())
	}
	var got int
	if err = s.DB.QueryRow("SELECT is_default FROM ai_connections WHERE id=2").Scan(&got); err != nil || got != 1 {
		t.Fatalf("promoted=%d err=%v", got, err)
	}
}
