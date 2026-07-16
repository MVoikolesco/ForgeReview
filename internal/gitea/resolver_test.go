package gitea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitea-agents/internal/config"
	"gitea-agents/internal/store"
)

func TestClientResolverUsesRepositoryInstanceAndLegacyEnvToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token configured-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("diff"))
	}))
	defer server.Close()
	t.Setenv("GITEA_INSTANCE_TOKEN", "configured-token")

	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`INSERT INTO gitea_instances(name,base_url,bot_username,token_ciphertext,token_env_name) VALUES('configured',?,?, '', 'GITEA_INSTANCE_TOKEN')`, server.URL, "bot")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`INSERT INTO repositories(gitea_instance_id,owner,name,full_name) VALUES(1,'acme','portal','acme/portal')`)
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewClientResolver(s.DB, "", "").Resolve(context.Background(), 0, "acme", "portal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetPullRequestDiff(context.Background(), "acme", "portal", 1); err != nil {
		t.Fatal(err)
	}
}

func TestClientResolverFallsBackToEnvironmentClientForLegacyJobs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token legacy-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("diff"))
	}))
	defer server.Close()
	client, err := NewClientResolver(nil, server.URL, "legacy-token").Resolve(context.Background(), 0, "acme", "portal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetPullRequestDiff(context.Background(), "acme", "portal", 1); err != nil {
		t.Fatal(err)
	}
}

func TestClientResolverUsesEnabledDefaultWhenRepositoryIsUnmapped(t *testing.T) {
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}

	_, err = s.DB.Exec(`INSERT INTO gitea_instances(id,name,base_url,token_env_name,is_enabled,is_default) VALUES
		(1,'disabled','http://disabled','GITEA_DISABLED_TOKEN',0,0),
		(2,'enabled','http://enabled','GITEA_ENABLED_TOKEN',1,0),
		(3,'default','http://default','GITEA_DEFAULT_TOKEN',1,1)`)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITEA_DEFAULT_TOKEN", "default-token")

	client, err := NewClientResolver(s.DB, "", "").Resolve(context.Background(), 0, "acme", "unmapped")
	if err != nil {
		t.Fatal(err)
	}
	if client.baseURL != "http://default" {
		t.Fatalf("base URL = %q, want default instance", client.baseURL)
	}
}
