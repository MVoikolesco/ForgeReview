package admin

import (
	"context"
	"gitea-agents/internal/config"
	"gitea-agents/internal/secrets"
	"gitea-agents/internal/store"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaCatalogUsesConfiguredBearerOnlyForCloud(t *testing.T) {
	for _, test := range []struct {
		name, key, wantAuth string
		cloud               bool
	}{
		{name: "local ignores submitted key", key: "local-secret"},
		{name: "cloud", key: "cloud-secret", wantAuth: "Bearer cloud-secret", cloud: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/tags" {
					t.Fatalf("path=%s", r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != test.wantAuth {
					t.Fatalf("authorization=%q want=%q", got, test.wantAuth)
				}
				_, _ = w.Write([]byte(`{"models":[{"name":"qwen"}]}`))
			}))
			defer server.Close()
			h := Handler{http: server.Client()}
			w := httptest.NewRecorder()
			h.ollamaCatalog(w, httptest.NewRequest(http.MethodPost, "/", nil), setupConnection{BaseURL: server.URL, APIKey: test.key}, test.cloud)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestProviderFailuresDoNotEchoCredentialsOrCiphertext(t *testing.T) {
	t.Setenv("GITEA_TOKEN_ENCRYPTION_KEY", "01234567890123456789012345678901")
	const apiKey = "provider-secret-that-must-not-leak"
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
	ciphertext, err := secrets.Encrypt(apiKey, secrets.ConnectionKeyAAD(1))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("provider reflected " + r.Header.Get("Authorization") + " ciphertext=" + ciphertext))
	}))
	defer server.Close()
	if _, err = s.DB.Exec(`INSERT INTO ai_connections(id,provider_id,name,base_url,api_key_ciphertext,requires_auth) VALUES(1,2,'remote',?,?,1)`, server.URL, ciphertext); err != nil {
		t.Fatal(err)
	}
	h := Handler{db: s.DB, http: server.Client()}
	for _, test := range []struct {
		name string
		call func(*httptest.ResponseRecorder)
	}{
		{
			name: "catalog",
			call: func(w *httptest.ResponseRecorder) {
				h.openRouterCatalog(w, httptest.NewRequest(http.MethodPost, "/", nil), setupConnection{BaseURL: server.URL, APIKey: apiKey})
			},
		},
		{
			name: "connection test",
			call: func(w *httptest.ResponseRecorder) {
				h.testConnection(w, httptest.NewRequest(http.MethodPost, "/", nil), 1)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			test.call(w)
			if w.Code != http.StatusBadGateway {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), apiKey) || strings.Contains(w.Body.String(), ciphertext) {
				t.Fatalf("provider credentials leaked: %s", w.Body.String())
			}
		})
	}
}

func TestOllamaAuthenticationUsesEndpointNotConnectionName(t *testing.T) {
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

	remote := httptest.NewRequest(http.MethodPost, "/api/admin/ai/connections", strings.NewReader(`{"provider_id":1,"name":"Ollama local","base_url":"https://remote.example"}`))
	remote.SetBasicAuth("admin", "secret")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, remote)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("remote local-name create=%d %s", res.Code, res.Body.String())
	}
	setup := httptest.NewRequest(http.MethodPost, "/api/admin/setup/complete", strings.NewReader(`{"provider":"ollama","connection":{"name":"Ollama local","base_url":"https://remote.example"},"model":{"id":"remote","name":"Remote"},"parameters":{},"profile":{"name":"Padrão"},"policy":{}}`))
	setup.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, setup)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("remote local-name setup=%d %s", res.Code, res.Body.String())
	}
	catalog := httptest.NewRequest(http.MethodPost, "/api/admin/setup/catalog", strings.NewReader(`{"provider":"ollama","connection":{"name":"Ollama local","base_url":"https://remote.example"}}`))
	catalog.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, catalog)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("remote local-name catalog=%d %s", res.Code, res.Body.String())
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("loopback authorization=%q", got)
		}
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()
	loopbackURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	local := httptest.NewRequest(http.MethodPost, "/api/admin/ai/connections", strings.NewReader(`{"provider_id":1,"name":"any name","base_url":"`+loopbackURL+`"}`))
	local.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, local)
	if res.Code != http.StatusCreated {
		t.Fatalf("loopback create=%d %s", res.Code, res.Body.String())
	}
	var requiresAuth int
	if err = s.DB.QueryRow("SELECT requires_auth FROM ai_connections WHERE id=1").Scan(&requiresAuth); err != nil || requiresAuth != 0 {
		t.Fatalf("loopback requires_auth=%d err=%v", requiresAuth, err)
	}
	test := httptest.NewRequest(http.MethodPost, "/api/admin/ai/connections/1/test", nil)
	test.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, test)
	if res.Code != http.StatusOK {
		t.Fatalf("loopback test=%d %s", res.Code, res.Body.String())
	}

	update := httptest.NewRequest(http.MethodPatch, "/api/admin/ai/connections/1", strings.NewReader(`{"base_url":"https://remote.example"}`))
	update.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, update)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("remote update without key=%d %s", res.Code, res.Body.String())
	}
}

func TestCompleteOllamaCloudSetupStoresCiphertextAndTestsWithBearer(t *testing.T) {
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
	body := `{"provider":"ollama-cloud","connection":{"name":"Ollama local","base_url":"https://remote.example","api_key":"cloud-secret"},"model":{"id":"cloud","name":"Cloud"},"parameters":{},"profile":{"name":"Padrão"},"policy":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup/complete", strings.NewReader(body))
	req.SetBasicAuth("admin", "secret")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", res.Code, res.Body.String())
	}
	var ciphertext string
	var requiresAuth int
	if err = s.DB.QueryRow("SELECT api_key_ciphertext,requires_auth FROM ai_connections WHERE id=1").Scan(&ciphertext, &requiresAuth); err != nil || requiresAuth != 1 {
		t.Fatalf("ciphertext=%q requires_auth=%d err=%v", ciphertext, requiresAuth, err)
	}
	if key, decryptErr := secrets.Decrypt(ciphertext, secrets.ConnectionKeyAAD(1)); decryptErr != nil || key != "cloud-secret" {
		t.Fatalf("decrypted key=%q err=%v", key, decryptErr)
	}
	h := Handler{db: s.DB, http: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer cloud-secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"models":[]}`)), Header: make(http.Header)}, nil
	})}}
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"provider":"ollama","connection":{"id":1}}`))
	res = httptest.NewRecorder()
	h.providerCatalog(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("catalog=%d %s", res.Code, res.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	res = httptest.NewRecorder()
	h.testConnection(res, req, 1)
	if res.Code != http.StatusOK {
		t.Fatalf("test=%d %s", res.Code, res.Body.String())
	}
	if _, err = s.DB.Exec("UPDATE ai_connections SET api_key_ciphertext='' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	res = httptest.NewRecorder()
	h.testConnection(res, req, 1)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("missing ciphertext test=%d %s", res.Code, res.Body.String())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCompleteSetupCreatesAtomicReviewConfiguration(t *testing.T) {
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
	body := `{"provider":"ollama","connection":{"name":"Local","base_url":"http://localhost:11434"},"model":{"id":"qwen:test","name":"Qwen Test","context_length":32768,"supported_parameters":["temperature"]},"parameters":{"temperature":0.2,"top_p":0.9,"timeout_seconds":30},"profile":{"name":"Padrão","description":"Teste"},"policy":{"max_block_chars":8000,"max_files_per_block":3,"review_concurrency":1,"review_final_retries":4}}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup/complete", strings.NewReader(body))
	req.SetBasicAuth("admin", "secret")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	for table, want := range map[string]int{"ai_connections": 1, "ai_models": 1, "model_parameters": 1, "review_profiles": 1, "review_policies": 1, "review_prompts": 0} {
		var got int
		if err = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil || got != want {
			t.Fatalf("%s=%d want=%d err=%v", table, got, want, err)
		}
	}
	var profileModelID int
	if err = s.DB.QueryRow("SELECT model_id FROM review_profiles WHERE is_default=1").Scan(&profileModelID); err != nil || profileModelID == 0 {
		t.Fatalf("default profile model_id=%d err=%v", profileModelID, err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"configured":true`) {
		t.Fatalf("status response=%d %s", res.Code, res.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/admin/review/prompts", strings.NewReader(`{"content":"não permitido"}`))
	req.SetBasicAuth("admin", "secret")
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("prompt write status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestCompleteSetupReusesDefaultProfileForAnotherConnection(t *testing.T) {
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

	for _, body := range []string{
		`{"provider":"ollama","connection":{"name":"Local A"},"model":{"id":"model:a","name":"A"},"profile":{"name":"Padrão"},"parameters":{},"policy":{}}`,
		`{"provider":"ollama","connection":{"name":"Local B"},"model":{"id":"model:b","name":"B"},"profile":{"name":"Padrão"},"parameters":{},"policy":{}}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/setup/complete", strings.NewReader(body))
		req.SetBasicAuth("admin", "secret")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusCreated {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
	}

	var profiles, policies int
	var activeModel string
	_ = s.DB.QueryRow("SELECT count(*) FROM review_profiles").Scan(&profiles)
	_ = s.DB.QueryRow("SELECT count(*) FROM review_policies").Scan(&policies)
	err = s.DB.QueryRow(`SELECT m.provider_model_name FROM review_profiles p JOIN ai_models m ON m.id=p.model_id WHERE p.is_default=1`).Scan(&activeModel)
	if err != nil || profiles != 1 || policies != 1 || activeModel != "model:b" {
		t.Fatalf("profiles=%d policies=%d active_model=%q err=%v", profiles, policies, activeModel, err)
	}
}

func TestCompleteOpenRouterSetupDoesNotUseCatalogMaximumAsRequestLimit(t *testing.T) {
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
	t.Setenv("GITEA_TOKEN_ENCRYPTION_KEY", "01234567890123456789012345678901")
	body := `{"provider":"openrouter","connection":{"name":"OpenRouter","base_url":"https://openrouter.ai/api/v1","api_key":"test-key"},"model":{"id":"qwen/qwen3-coder-next","name":"Qwen","context_length":262144,"max_completion_tokens":262144},"parameters":{"temperature":0.2,"timeout_seconds":30},"profile":{"name":"Padrão"},"policy":{"max_block_chars":8000,"max_files_per_block":3,"review_concurrency":1,"review_final_retries":4}}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/setup/complete", strings.NewReader(body))
	req.SetBasicAuth("admin", "secret")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var contextWindow, maxOutputTokens int
	if err = s.DB.QueryRow("SELECT context_window,max_output_tokens FROM ai_models WHERE provider_model_name=?", "qwen/qwen3-coder-next").Scan(&contextWindow, &maxOutputTokens); err != nil {
		t.Fatal(err)
	}
	if contextWindow != 262144 || maxOutputTokens != 4096 {
		t.Fatalf("context_window=%d max_output_tokens=%d", contextWindow, maxOutputTokens)
	}
}
