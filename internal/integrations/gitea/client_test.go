package gitea

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitea-agents/internal/contracts"
)

func TestPublishMarkerAllowsReconciliation(t *testing.T) {
	var publishedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var payload struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			publishedBody = payload.Body
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]string{{"body": publishedBody}})
	}))
	defer server.Close()

	client := New(server.URL, "token")
	result := contracts.Result{
		ReviewID: "rev-1", Model: "gpt-oss:120b", Comments: []contracts.Comment{},
		FinalReview: contracts.FinalReview{GiteaEvent: "COMMENT", Status: "aprovado", Summary: "review", Observations: "Nenhum problema relevante foi confirmado."},
		Metadata: map[string]any{
			"total_duration_ms": 43501,
			"stage_metrics":     []map[string]any{{"actual_prompt_tokens": 12, "actual_completion_tokens": 8}},
		},
	}
	if err := client.Publish(context.Background(), "acme", "app", 1, result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(publishedBody, "<!-- forgereview:rev-1 -->") {
		t.Fatalf("publication marker missing: %q", publishedBody)
	}
	for _, expected := range []string{"> status: aprovado", "> elapsed time: 43.501s", "> model: gpt-oss:120b", "> tokens: 20 (prompt: 12, completion: 8)", "Nenhum problema relevante foi confirmado."} {
		if !strings.Contains(publishedBody, expected) {
			t.Fatalf("publication envelope missing %q: %q", expected, publishedBody)
		}
	}
	found, err := client.HasPublishedReview(context.Background(), "acme", "app", 1, "rev-1")
	if err != nil || !found {
		t.Fatalf("publication was not reconciled: found=%v err=%v", found, err)
	}
}

func TestOnlyClientHTTPFailuresAreDefinitive(t *testing.T) {
	if !IsDefinitiveHTTPRejection(&HTTPStatusError{StatusCode: http.StatusBadRequest}) {
		t.Fatal("expected 400 to be definitive")
	}
	if IsDefinitiveHTTPRejection(&HTTPStatusError{StatusCode: http.StatusBadGateway}) {
		t.Fatal("proxy errors are ambiguous")
	}
	if IsDefinitiveHTTPRejection(errors.New("timeout")) {
		t.Fatal("transport errors are ambiguous")
	}
}
