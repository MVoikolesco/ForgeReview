package database

import (
	"context"
	"testing"

	"gitea-agents/internal/config"
)

func TestMigrateAndSeed(t *testing.T) {
	db, err := Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"ai_providers", "review_profiles", "reviews", "review_steps", "pending_reviews"} {
		var name string
		if err := db.SQL.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Fatalf("missing %s: %v", table, err)
		}
	}
	var providers int
	if err := db.SQL.QueryRowContext(ctx, "SELECT count(*) FROM ai_providers").Scan(&providers); err != nil {
		t.Fatal(err)
	}
	if providers < 4 {
		t.Fatalf("expected seeded providers, got %d", providers)
	}
}
