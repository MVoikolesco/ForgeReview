package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientUsesSelectedModelAndBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization missing")
		}
		if r.Header.Get("HTTP-Referer") != "https://forgereview.example" || r.Header.Get("X-OpenRouter-Title") != "ForgeReview" {
			t.Errorf("attribution headers missing")
		}
		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "openai/test" {
			t.Errorf("model=%s", body.Model)
		}
		if body.MaxTokens != 2048 {
			t.Errorf("max_tokens=%d", body.MaxTokens)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	c := NewClient(Config{URL: server.URL, Model: "openai/test", APIKey: "secret", HTTPReferer: "https://forgereview.example", AppTitle: "ForgeReview", MaxTokens: 2048, TimeoutSeconds: 2})
	got, e := c.Chat(context.Background(), "review")
	if e != nil {
		t.Fatal(e)
	}
	if got != "ok" {
		t.Fatalf("got %q", got)
	}
}

func TestClientRepairsLegacyCatalogLimitAndReservesPromptContext(t *testing.T) {
	var gotMaxTokens int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		gotMaxTokens = body.MaxTokens
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		URL:            server.URL,
		Model:          "qwen/qwen3-coder-next",
		APIKey:         "secret",
		ContextWindow:  262144,
		MaxTokens:      262144, // legacy value copied from max_completion_tokens
		TimeoutSeconds: 2,
	})
	if _, err := client.Chat(context.Background(), "review this diff"); err != nil {
		t.Fatal(err)
	}
	if gotMaxTokens != defaultReviewMaxTokens {
		t.Fatalf("max_tokens=%d want=%d", gotMaxTokens, defaultReviewMaxTokens)
	}
}

func TestClientClampsOutputToAvailableContext(t *testing.T) {
	client := NewClient(Config{ContextWindow: 5000, MaxTokens: 4096})
	got, err := client.effectiveMaxTokens(string(make([]byte, 3000)))
	if err != nil {
		t.Fatal(err)
	}
	if want := 5000 - 3000 - 1024; got != want {
		t.Fatalf("max_tokens=%d want=%d", got, want)
	}
}
