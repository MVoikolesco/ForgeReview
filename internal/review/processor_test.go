package review

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitea-agents/internal/config"
	databasepkg "gitea-agents/internal/database"
	"gitea-agents/internal/integrations/gitea"
	"gitea-agents/internal/providers"
	"gitea-agents/internal/queue"
)

func TestProcessUsesSeededDatabasePipeline(t *testing.T) {
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

	published := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -10 +10 @@\n-old\n+new\n"))
			return
		}
		published++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	repo := NewRepository(db.SQL)
	job := queue.ReviewJob{ReviewID: "rev-process", Owner: "acme", Repository: "app", PullRequest: 1}
	if err = repo.Create(ctx, job.ReviewID, job, "test"); err != nil {
		t.Fatal(err)
	}
	provider := &pipelineProvider{calls: map[string]int{}}
	service := NewService(config.Config{ReviewMaxBlockChars: 4000, ReviewMaxFilesPerBlock: 2}, repo, nil)
	service.SetFactories(
		func(context.Context, *int64, *int64) (providers.LLMProvider, error) { return provider, nil },
		func(context.Context, queue.ReviewJob) (*gitea.Client, error) {
			return gitea.New(server.URL, "token"), nil
		},
	)
	if err = service.Process(ctx, job); err != nil {
		t.Fatal(err)
	}
	item, err := repo.Get(ctx, job.ReviewID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != StatusCompleted || item.Result == nil || len(item.Result.Comments) != 1 || published != 1 {
		t.Fatalf("unexpected processed review: status=%s result=%#v published=%d", item.Status, item.Result, published)
	}
	var executions, stages, artifacts int
	if err = db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_executions WHERE review_id=? AND status='completed'`, job.ReviewID).Scan(&executions); err != nil {
		t.Fatal(err)
	}
	if err = db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM stage_executions se JOIN pipeline_executions pe ON pe.id=se.pipeline_execution_id WHERE pe.review_id=?`, job.ReviewID).Scan(&stages); err != nil {
		t.Fatal(err)
	}
	if err = db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM stage_artifacts sa JOIN pipeline_executions pe ON pe.id=sa.pipeline_execution_id WHERE pe.review_id=?`, job.ReviewID).Scan(&artifacts); err != nil {
		t.Fatal(err)
	}
	if executions != 1 || stages != 8 || artifacts != 7 {
		t.Fatalf("unexpected execution audit: executions=%d stages=%d artifacts=%d", executions, stages, artifacts)
	}
}
