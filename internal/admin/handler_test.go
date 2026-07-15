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
