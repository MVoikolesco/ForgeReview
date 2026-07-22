package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testIntegration(t *testing.T, kind, baseURL string) Integration {
	t.Helper()
	config := map[string]string{"base_url": baseURL}
	if kind == TypeOpenAI || kind == TypeOllama {
		config["model"] = "reviewer"
	}
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	return Integration{Key: kind, Name: kind, Type: kind, Config: payload, SecretCiphertext: "test-ciphertext", Status: StatusActive}
}

func TestHTTPGiteaClientReadPullRequestContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "token gitea-secret" {
			t.Fatalf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/api/v1/repos/acme/review/pulls/7":
			if request.Method != http.MethodGet {
				t.Fatalf("method = %s", request.Method)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"number":7,"title":"Improve review"}`))
		case "/api/v1/repos/acme/review/pulls/7/files":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`[{"filename":"main.go","status":"modified"}]`))
		case "/api/v1/repos/acme/review/pulls/7.diff":
			_, _ = writer.Write([]byte("diff --git a/main.go b/main.go"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	result, err := (HTTPGiteaClient{Client: server.Client()}).ReadPullRequest(context.Background(), testIntegration(t, TypeGitea, server.URL), "gitea-secret", PullRequestRequest{Owner: "acme", Repo: "review", Number: 7})
	if err != nil {
		t.Fatalf("read pull request: %v", err)
	}
	if result.Metadata["title"] != "Improve review" || result.Diff == "" || result.Files[0]["filename"] != "main.go" {
		t.Fatalf("unexpected pull request: %#v", result)
	}
}

func TestHTTPGiteaClientPublishReviewContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/repos/acme/review/pulls/7/reviews" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "token gitea-secret" || request.Header.Get("X-ForgeReview-Idempotency-Key") != "forgereview:publication:1:2:publish" {
			t.Fatalf("unexpected controlled headers")
		}
		var body struct {
			Body     string               `json:"body"`
			Event    string               `json:"event"`
			Comments []GiteaReviewComment `json:"comments"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Body != "review body\n\n<!-- forgereview:idempotency=forgereview:publication:1:2:publish -->" || body.Event != "REQUEST_CHANGES" || len(body.Comments) != 1 || body.Comments[0] != (GiteaReviewComment{Path: "main.go", Body: "inline", NewPosition: 4}) {
			t.Fatalf("body = %#v", body)
		}
		_, _ = writer.Write([]byte(`{"id":42,"html_url":"https://gitea.example/comments/42"}`))
	}))
	defer server.Close()

	receipt, err := (HTTPGiteaClient{Client: server.Client()}).PublishReview(context.Background(), testIntegration(t, TypeGitea, server.URL), "gitea-secret", GiteaReviewRequest{Owner: "acme", Repo: "review", Number: 7, Body: "review body", Event: "REQUEST_CHANGES", Comments: []GiteaReviewComment{{Path: "main.go", Body: "inline", NewPosition: 4}}, IdempotencyKey: "forgereview:publication:1:2:publish"})
	if err != nil {
		t.Fatalf("publish review: %v", err)
	}
	if receipt.CommentID != 42 || receipt.URL == "" || receipt.Status != "completed" {
		t.Fatalf("receipt = %#v", receipt)
	}
}

func TestHTTPChatClientRequestContracts(t *testing.T) {
	tests := []struct {
		name, kind, path, response string
		client                     ChatClient
	}{
		{"openai", TypeOpenAI, "/v1/chat/completions", `{"choices":[{"message":{"content":"openai response"}}]}`, HTTPOpenAIClient{}},
		{"ollama", TypeOllama, "/api/chat", `{"message":{"content":"ollama response"}}`, HTTPOllamaClient{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != test.path {
					t.Fatalf("request = %s %s", request.Method, request.URL.Path)
				}
				if request.Header.Get("Authorization") != "Bearer chat-secret" {
					t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["model"] != "reviewer" {
					t.Fatalf("model = %#v", body["model"])
				}
				messages := body["messages"].([]any)
				if messages[0].(map[string]any)["content"] != "review this" {
					t.Fatalf("messages = %#v", messages)
				}
				if test.kind == TypeOllama && body["stream"] != false {
					t.Fatalf("stream = %#v", body["stream"])
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.response))
			}))
			defer server.Close()
			client := test.client
			switch value := client.(type) {
			case HTTPOpenAIClient:
				client = HTTPOpenAIClient{Client: server.Client()}
			case HTTPOllamaClient:
				client = HTTPOllamaClient{Client: server.Client()}
			case *HTTPOpenAIClient:
				value.Client = server.Client()
				client = value
			}
			response, err := client.Chat(context.Background(), testIntegration(t, test.kind, server.URL), "chat-secret", "review this")
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if response != test.name+" response" {
				t.Fatalf("response = %q", response)
			}
		})
	}
}

func TestDiscoveryUsesProviderContractsAndOpenRouterVersionedBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/models" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing authorization")
		}
		_, _ = writer.Write([]byte(`{"data":[{"id":"openai/gpt"},{"id":"meta/llama"}]}`))
	}))
	defer server.Close()
	item := testIntegration(t, TypeOpenAI, server.URL+"/api/v1")
	adapter := HTTPDiscoveryAdapter{Client: server.Client()}
	if err := adapter.Validate(context.Background(), item, "secret"); err != nil {
		t.Fatalf("validate: %v", err)
	}
	models, err := adapter.Models(context.Background(), item, "secret")
	if err != nil || len(models) != 2 || models[0] != "openai/gpt" {
		t.Fatalf("models = %#v, %v", models, err)
	}
	if endpoint := openAIEndpoint("https://openrouter.ai/api/v1", "chat/completions"); endpoint != "https://openrouter.ai/api/v1/chat/completions" {
		t.Fatalf("endpoint = %s", endpoint)
	}
}
