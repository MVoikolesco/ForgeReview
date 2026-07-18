package review

import (
	"context"
	"testing"
	"time"

	"gitea-agents/internal/config"
	databasepkg "gitea-agents/internal/database"
	"gitea-agents/internal/queue"
)

func TestRepositoryPersistsReviewAndSteps(t *testing.T) {
	db, err := databasepkg.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db.SQL)
	job := queue.ReviewJob{ReviewID: "rev-test", Owner: "acme", Repository: "app", PullRequest: 7}
	if err := repo.Create(context.Background(), job.ReviewID, job, "test"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetStatus(context.Background(), job.ReviewID, StatusQueued, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddStep(context.Background(), job.ReviewID, "buscando_diff", "concluido", "ok", nil, time.Now().UTC(), nil, 12, ""); err != nil {
		t.Fatal(err)
	}
	item, err := repo.Get(context.Background(), job.ReviewID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != StatusQueued || len(item.Steps) != 1 || item.Steps[0].Step != "buscando_diff" {
		t.Fatalf("unexpected review: %#v", item)
	}
}
