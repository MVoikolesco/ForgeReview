package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientChatSendsPayloadAndReturnsContent(t *testing.T) {
	var got chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("expected /api/chat, got %s", r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"review final"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		URL:            server.URL,
		Model:          "deepseek-coder:6.7b",
		KeepAlive:      "5m",
		TimeoutSeconds: 900,
		Options: Options{
			Temperature:   0.1,
			TopP:          0.85,
			RepeatPenalty: 1.1,
			NumCtx:        4096,
			NumThread:     2,
			NumPredict:    400,
		},
	})

	if client.httpClient.Timeout != 900*time.Second {
		t.Fatalf("unexpected timeout %s", client.httpClient.Timeout)
	}

	content, err := client.Chat(context.Background(), "analise este diff")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if content != "review final" {
		t.Fatalf("unexpected content %q", content)
	}

	if got.Model != "deepseek-coder:6.7b" {
		t.Fatalf("unexpected model %q", got.Model)
	}

	if got.Stream {
		t.Fatal("expected stream false")
	}

	if got.KeepAlive != "5m" {
		t.Fatalf("unexpected keep_alive %q", got.KeepAlive)
	}

	if len(got.Messages) != 1 || got.Messages[0].Role != "user" || got.Messages[0].Content != "analise este diff" {
		t.Fatalf("unexpected messages %#v", got.Messages)
	}

	if got.Options.Temperature != 0.1 || got.Options.TopP != 0.85 || got.Options.RepeatPenalty != 1.1 || got.Options.NumCtx != 4096 || got.Options.NumThread != 2 || got.Options.NumPredict != 400 {
		t.Fatalf("unexpected options %#v", got.Options)
	}
}

func TestNewClientUsesDefaultTimeoutWhenInvalid(t *testing.T) {
	client := NewClient(Config{URL: "http://localhost:11434", Model: "deepseek-coder:6.7b", TimeoutSeconds: -1})

	if client.httpClient.Timeout != 900*time.Second {
		t.Fatalf("expected default timeout, got %s", client.httpClient.Timeout)
	}
}

func TestClientChatReturnsErrorForHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, Model: "deepseek-coder:6.7b"})

	_, err := client.Chat(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "status=500") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestClientChatReturnsErrorForTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"late"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, Model: "deepseek-coder:6.7b", TimeoutSeconds: 900})
	client.httpClient.Timeout = time.Nanosecond

	_, err := client.Chat(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "erro ao chamar ollama") {
		t.Fatalf("unexpected timeout error, got %v", err)
	}
}

func TestClientChatReturnsErrorWhenContentIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, Model: "deepseek-coder:6.7b"})

	_, err := client.Chat(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "message.content") {
		t.Fatalf("expected missing content error, got %v", err)
	}
}
