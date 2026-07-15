package store

import (
	"context"
	"gitea-agents/internal/config"
	"testing"
)

func TestMigrationsAndSeedAreIdempotent(t *testing.T) {
	s, e := Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	c := config.Config{}
	if e = s.Seed(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	if e = s.Seed(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow("SELECT count(*) FROM ai_providers").Scan(&n); e != nil || n != 4 {
		t.Fatalf("providers=%d err=%v", n, e)
	}
}
