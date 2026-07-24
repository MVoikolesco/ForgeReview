package review

import (
	"context"
	"errors"
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

func TestPublicationReservationDistinguishesRetryableAndUncertainFailures(t *testing.T) {
	db, err := databasepkg.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db.SQL)
	job := queue.ReviewJob{ReviewID: "rev-publication", Owner: "acme", Repository: "app", PullRequest: 7}
	if err = repo.Create(ctx, job.ReviewID, job, "test"); err != nil {
		t.Fatal(err)
	}
	result := Result{Comments: []Comment{}, FinalReview: FinalReview{GiteaEvent: "COMMENT", Summary: "review"}}
	reserved, err := repo.BeginPublication(ctx, job.ReviewID, result)
	if err != nil || !reserved {
		t.Fatalf("first reservation failed: reserved=%v err=%v", reserved, err)
	}
	if err = repo.MarkPublicationUncertain(ctx, job.ReviewID, errors.New("timeout")); err != nil {
		t.Fatal(err)
	}
	reserved, err = repo.BeginPublication(ctx, job.ReviewID, result)
	if err != nil || reserved {
		t.Fatalf("uncertain publication must remain blocked: reserved=%v err=%v", reserved, err)
	}
	if _, err = db.SQL.ExecContext(ctx, `UPDATE review_publications SET status='failed' WHERE review_id=?`, job.ReviewID); err != nil {
		t.Fatal(err)
	}
	result.FinalReview.Summary = "updated review"
	reserved, err = repo.BeginPublication(ctx, job.ReviewID, result)
	if err != nil || !reserved {
		t.Fatalf("definitive failure should allow a new result: reserved=%v err=%v", reserved, err)
	}
	cancelled, err := repo.Cancel(ctx, job.ReviewID, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled {
		t.Fatal("publication reservation must prevent cancellation")
	}
}
