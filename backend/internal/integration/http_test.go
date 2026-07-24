package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClassifyFailureDistinguishesRetrySafety(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want FailureClass
	}{
		{name: "rate limited", err: HTTPStatusError{StatusCode: http.StatusTooManyRequests}, want: FailureTransient},
		{name: "server failure", err: HTTPStatusError{StatusCode: http.StatusBadGateway}, want: FailureTransient},
		{name: "validation failure", err: HTTPStatusError{StatusCode: http.StatusBadRequest}, want: FailurePermanent},
		{name: "network failure", err: &net.DNSError{IsTimeout: true}, want: FailureTransient},
		{name: "publication uncertain", err: PublicationError{Uncertain: true}, want: FailureUncertain},
		{name: "ordinary failure", err: errors.New("invalid workflow"), want: FailurePermanent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifyFailure(test.err); got != test.want {
				t.Fatalf("ClassifyFailure(%v) = %q, want %q", test.err, got, test.want)
			}
		})
	}
}

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

func TestGiteaPublicationAmbiguityAndMarkerLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`[{"id":9,"html_url":"https://gitea/reviews/9","body":"ok <!-- forgereview:idempotency=key -->"}]`))
	}))
	defer server.Close()
	client := HTTPGiteaClient{Client: server.Client()}
	item := testIntegration(t, TypeGitea, server.URL)
	_, err := client.PublishReview(context.Background(), item, "secret", GiteaReviewRequest{Owner: "o", Repo: "r", Number: 1, Body: "body", Event: "COMMENT", IdempotencyKey: "key"})
	var publicationErr PublicationError
	if !errors.As(err, &publicationErr) || !publicationErr.Uncertain {
		t.Fatalf("5xx classification = %v", err)
	}
	receipt, found, err := client.FindReviewByMarker(context.Background(), item, "secret", PullRequestRequest{Owner: "o", Repo: "r", Number: 1}, "key")
	if err != nil || !found || receipt.CommentID != 9 {
		t.Fatalf("marker lookup = %#v %v %v", receipt, found, err)
	}
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
			_, _ = writer.Write([]byte(`[
				{"filename":"main.go","status":"modified"},
				{"filename":"old.go","status":"deleted"}
			]`))
		case "/api/v1/repos/acme/review/pulls/7.diff":
			_, _ = writer.Write([]byte("diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1,2 @@\n package main\n+func added() {}\ndiff --git a/old.go b/old.go\n--- a/old.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-package old"))
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
	if patch, _ := result.Files[0]["patch"].(string); !strings.Contains(patch, "@@ -1 +1,2 @@") || !strings.Contains(patch, "+func added() {}") {
		t.Fatalf("main.go patch was not attached: %#v", result.Files[0])
	}
	if patch, _ := result.Files[1]["patch"].(string); !strings.Contains(patch, "+++ /dev/null") || !strings.Contains(patch, "-package old") {
		t.Fatalf("deleted file patch was not attached: %#v", result.Files[1])
	}
}

func TestHTTPGiteaClientRejectsFilesWithoutReviewableDiff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/repos/acme/review/pulls/7":
			_, _ = writer.Write([]byte(`{"number":7}`))
		case "/api/v1/repos/acme/review/pulls/7/files":
			_, _ = writer.Write([]byte(`[{"filename":"main.go","status":"modified"}]`))
		case "/api/v1/repos/acme/review/pulls/7.diff":
			_, _ = writer.Write([]byte(""))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	_, err := (HTTPGiteaClient{Client: server.Client()}).ReadPullRequest(context.Background(), testIntegration(t, TypeGitea, server.URL), "gitea-secret", PullRequestRequest{Owner: "acme", Repo: "review", Number: 7})
	if err == nil || !strings.Contains(err.Error(), "1 changed file(s) have no reviewable patch content") {
		t.Fatalf("error = %v", err)
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
		{"openai", TypeOpenAI, "/v1/chat/completions", `{"model":"gpt-review","choices":[{"message":{"content":"openai response"}}],"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`, HTTPOpenAIClient{}},
		{"ollama", TypeOllama, "/api/chat", `{"model":"qwen-review","message":{"content":"ollama response"},"prompt_eval_count":7,"eval_count":5}`, HTTPOllamaClient{}},
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
				if test.kind == TypeOpenAI && body["max_tokens"] != float64(4096) {
					t.Fatalf("max_tokens = %#v", body["max_tokens"])
				}
				if body["temperature"] != nil && test.kind == TypeOllama {
					t.Fatalf("temperature must be inside Ollama options: %#v", body)
				}
				if test.kind == TypeOpenAI && (body["temperature"] != 0.3 || body["top_p"] != 0.8) {
					t.Fatalf("sampling parameters = %#v", body)
				}
				if test.kind == TypeOllama {
					options := body["options"].(map[string]any)
					if options["num_predict"] != float64(4096) || options["temperature"] != 0.3 || options["top_p"] != 0.8 {
						t.Fatalf("options = %#v", options)
					}
					if body["keep_alive"] != "10m" {
						t.Fatalf("keep_alive = %#v", body["keep_alive"])
					}
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
			item := testIntegration(t, test.kind, server.URL)
			config, _ := item.ConfigValues()
			config["max_tokens"] = "4096"
			config["temperature"] = "0.3"
			config["top_p"] = "0.8"
			config["keep_alive"] = "10m"
			item.Config, _ = json.Marshal(config)
			response, err := client.Chat(context.Background(), item, "chat-secret", "review this")
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if response.Content != test.name+" response" || response.Model == "" || response.Usage.Total <= 0 {
				t.Fatalf("response = %#v", response)
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

func TestGiteaDiscoveryListsOrganizationsThenScopesRepositories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "token secret" {
			t.Fatal("missing authorization")
		}
		switch request.URL.Path {
		case "/api/v1/user/orgs":
			_, _ = writer.Write([]byte(`[{"username":"acme"},{"username":"other"}]`))
		case "/api/v1/orgs/acme/repos":
			_, _ = writer.Write([]byte(`[{"name":"api","owner":{"username":"acme"}}]`))
		case "/api/v1/orgs/other/repos":
			t.Fatal("repository discovery must not enumerate unselected organizations")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter := HTTPDiscoveryAdapter{Client: server.Client()}
	item := testIntegration(t, TypeGitea, server.URL)
	organizations, err := adapter.Organizations(context.Background(), item, "secret")
	if err != nil || len(organizations) != 2 || organizations[0] != "acme" {
		t.Fatalf("organizations = %#v, %v", organizations, err)
	}
	repositories, err := adapter.Repositories(context.Background(), item, "secret", "acme")
	if err != nil || len(repositories) != 1 || repositories[0] != (Repository{IntegrationKey: TypeGitea, Owner: "acme", Name: "api"}) {
		t.Fatalf("repositories = %#v, %v", repositories, err)
	}
}
