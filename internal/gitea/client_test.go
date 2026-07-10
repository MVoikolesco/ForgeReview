package gitea

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPullRequestDiff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/Qualyagro/wiki/pulls/12.diff" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}

		if r.Header.Get("Authorization") != "token secret-token" {
			t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}

		if r.Header.Get("Accept") != "text/plain" {
			t.Fatalf("unexpected accept header %q", r.Header.Get("Accept"))
		}

		_, _ = w.Write([]byte("diff --git a/file.go b/file.go"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-token")

	diff, err := client.GetPullRequestDiff(context.Background(), "Qualyagro", "wiki", 12)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if diff != "diff --git a/file.go b/file.go" {
		t.Fatalf("unexpected diff %q", diff)
	}
}

func TestGetPullRequestDiffReturnsErrorForNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-token")

	_, err := client.GetPullRequestDiff(context.Background(), "Qualyagro", "wiki", 12)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreatePullRequestReview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/Qualyagro/wiki/pulls/12/reviews" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %q", r.Method)
		}
		if r.Header.Get("Authorization") != "token secret-token" {
			t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("unexpected accept header %q", r.Header.Get("Accept"))
		}

		var payload CreatePullReviewOptions
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("unexpected json payload: %v", err)
		}
		if payload.Event != "REQUEST_CHANGES" || payload.Body != "review body" {
			t.Fatalf("unexpected payload %#v", payload)
		}
		if len(payload.Comments) != 1 {
			t.Fatalf("expected one comment, got %#v", payload.Comments)
		}
		if payload.Comments[0].Path != "app/file.go" || payload.Comments[0].NewPosition != 42 {
			t.Fatalf("unexpected comment %#v", payload.Comments[0])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123,"state":"REQUEST_CHANGES"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-token")

	review, err := client.CreatePullRequestReview(context.Background(), "Qualyagro", "wiki", 12, CreatePullReviewOptions{
		Event: "REQUEST_CHANGES",
		Body:  "review body",
		Comments: []CreatePullReviewComment{
			{Path: "app/file.go", NewPosition: 42, Body: "fix this"},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if review.ID != 123 || review.State != "REQUEST_CHANGES" {
		t.Fatalf("unexpected review %#v", review)
	}
}
