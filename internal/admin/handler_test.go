package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitea-agents/internal/config"
	"gitea-agents/internal/store"
)

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
