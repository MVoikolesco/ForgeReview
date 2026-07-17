package agents

import (
	"context"
	"strings"
	"testing"

	"gitea-agents/internal/queue"
	"gitea-agents/internal/review/pipeline"
)

type memoryReviewLogStore struct {
	files map[string]string
}

func (s *memoryReviewLogStore) CreateRun(_ context.Context, baseName string) (string, error) {
	return baseName, nil
}

func (s *memoryReviewLogStore) Write(_ context.Context, name, file, content string) error {
	s.files[name+":"+file] = content
	return nil
}

func (s *memoryReviewLogStore) Append(_ context.Context, name, file, content string) error {
	s.files[name+":"+file] += content
	return nil
}

func (s *memoryReviewLogStore) Read(_ context.Context, name, file string) (string, error) {
	return s.files[name+":"+file], nil
}

func (s *memoryReviewLogStore) Exists(_ context.Context, name, file string) (bool, error) {
	_, ok := s.files[name+":"+file]
	return ok, nil
}

func (s *memoryReviewLogStore) Delete(_ context.Context, name, file string) error {
	delete(s.files, name+":"+file)
	return nil
}

func (s *memoryReviewLogStore) Runs(context.Context) ([]string, error) { return []string{}, nil }

func TestReviewRunLogWritesArtifactsToStore(t *testing.T) {
	store := &memoryReviewLogStore{files: map[string]string{}}
	log, err := newReviewRunLogWithStore(context.Background(), "/read-only/logs", store, queue.ReviewJob{Owner: "acme", Repo: "app", PRNumber: 7})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Write("final-review.md", "review"); err != nil {
		t.Fatal(err)
	}
	if err := log.AppendProgress(pipeline.ProgressEvent{Stage: "revisao", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if store.files["acme_app_pr-7:final-review.md"] != "review" {
		t.Fatalf("unexpected stored review %q", store.files["acme_app_pr-7:final-review.md"])
	}
	if !strings.Contains(store.files["acme_app_pr-7:00-progress.jsonl"], `"stage":"revisao"`) {
		t.Fatalf("expected progress in store, got %q", store.files["acme_app_pr-7:00-progress.jsonl"])
	}
}
