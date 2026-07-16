package admin

import (
	"context"
	"gitea-agents/internal/config"
	"gitea-agents/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	body := `{"provider":"ollama","connection":{"name":"Local","base_url":"http://ollama:11434"},"model":{"id":"qwen:test","name":"Qwen Test","context_length":32768,"supported_parameters":["temperature"]},"parameters":{"temperature":0.2,"top_p":0.9,"timeout_seconds":30},"profile":{"name":"Padrão","description":"Teste"},"policy":{"max_block_chars":8000,"max_files_per_block":3,"review_concurrency":1,"review_final_retries":4}}`
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
	body := `{"provider":"openrouter","connection":{"name":"OpenRouter","base_url":"https://openrouter.ai/api/v1","api_key_env_name":"OPENROUTER_API_KEY"},"model":{"id":"qwen/qwen3-coder-next","name":"Qwen","context_length":262144,"max_completion_tokens":262144},"parameters":{"temperature":0.2,"timeout_seconds":30},"profile":{"name":"Padrão"},"policy":{"max_block_chars":8000,"max_files_per_block":3,"review_concurrency":1,"review_final_retries":4}}`
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
