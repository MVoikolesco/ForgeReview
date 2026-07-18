package admin

import (
	"context"
	"net/http/httptest"
	"testing"

	"gitea-agents/internal/config"
	"gitea-agents/internal/store"
)

func TestResolvePendingGiteaInstanceUsesDefaultForLegacyJob(t *testing.T) {
	s, err := store.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO gitea_instances(name,base_url,is_default) VALUES('main','https://gitea.example',1)`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	instanceID, err := (Handler{db: s.DB}).resolvePendingGiteaInstance(req, 0, "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if instanceID != 1 {
		t.Fatalf("expected default instance 1, got %d", instanceID)
	}
}
