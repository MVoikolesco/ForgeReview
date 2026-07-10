package webhook

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gitea-agents/internal/queue"
)

func TestReceivePublishesReviewJob(t *testing.T) {
	mux, logs, publisher := newTestWebhook("ia-reviewer")

	payload := `{
		"action":"review_requested",
		"number":12,
		"requested_reviewer":{"login":"ia-reviewer","username":"ia-reviewer"},
		"repository":{"full_name":"Qualyagro/wiki"},
		"sender":{"login":"marcio"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, res.Code)
	}

	if !strings.Contains(res.Body.String(), "accepted") {
		t.Fatalf("expected accepted response, got %q", res.Body.String())
	}

	assertJob(t, publisher.jobs[0], queue.ReviewJob{
		Owner:             "Qualyagro",
		Repo:              "wiki",
		PRNumber:          12,
		RequestedReviewer: "ia-reviewer",
		Sender:            "marcio",
	})

	assertLogContains(t, logs.String(), "Webhook válido recebido")
	assertLogContains(t, logs.String(), "Reviewer: ia-reviewer")
	assertLogContains(t, logs.String(), "Solicitado por: marcio")
	assertLogContains(t, logs.String(), "Repositório: Qualyagro/wiki")
	assertLogContains(t, logs.String(), "PR: #12")
	assertLogContains(t, logs.String(), "Ação: review_requested")
	assertLogContains(t, logs.String(), "Job criado: {Owner:Qualyagro Repo:wiki PRNumber:12 RequestedReviewer:ia-reviewer Sender:marcio}")
}

func TestReceivePublishesReviewJobFromPullRequestReviewers(t *testing.T) {
	mux, logs, publisher := newTestWebhook("victor.brutti")

	payload := `{
		"action":"review_requested",
		"pull_request":{
			"number":12,
			"requested_reviewers":[
				{"id":17,"login":"victor.brutti","username":"victor.brutti"}
			]
		},
		"repository":{"full_name":"mvoikolesco/teste-bot"},
		"sender":{"login":"marcioj"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, res.Code)
	}

	assertJob(t, publisher.jobs[0], queue.ReviewJob{
		Owner:             "mvoikolesco",
		Repo:              "teste-bot",
		PRNumber:          12,
		RequestedReviewer: "victor.brutti",
		Sender:            "marcioj",
	})

	assertLogContains(t, logs.String(), "Reviewer: victor.brutti")
}

func TestReceiveReadsPayloadFromQueryString(t *testing.T) {
	mux, logs, publisher := newTestWebhook("victor.brutti")

	payload := `{
		"action":"review_requested",
		"number":1,
		"pull_request":{
			"requested_reviewers":[
				{"id":17,"login":"victor.brutti","username":"victor.brutti"}
			]
		},
		"repository":{"full_name":"mvoikolesco/teste-bot"},
		"sender":{"login":"marcioj"}
	}`

	req := httptest.NewRequest(http.MethodGet, "/webhook?payload="+url.QueryEscape(payload), nil)
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, res.Code)
	}

	assertJob(t, publisher.jobs[0], queue.ReviewJob{
		Owner:             "mvoikolesco",
		Repo:              "teste-bot",
		PRNumber:          1,
		RequestedReviewer: "victor.brutti",
		Sender:            "marcioj",
	})
	assertLogContains(t, logs.String(), "Reviewer: victor.brutti")
}

func TestReceiveReadsPayloadFromForm(t *testing.T) {
	mux, logs, publisher := newTestWebhook("victor.brutti")

	payload := `{
		"action":"review_requested",
		"number":1,
		"requested_reviewer":{"login":"victor.brutti","username":"victor.brutti"},
		"repository":{"full_name":"mvoikolesco/teste-bot"},
		"sender":{"login":"marcioj"}
	}`

	form := url.Values{}
	form.Set("payload", payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, res.Code)
	}

	assertJob(t, publisher.jobs[0], queue.ReviewJob{
		Owner:             "mvoikolesco",
		Repo:              "teste-bot",
		PRNumber:          1,
		RequestedReviewer: "victor.brutti",
		Sender:            "marcioj",
	})
	assertLogContains(t, logs.String(), "Reviewer: victor.brutti")
}

func TestReceiveIgnoresDifferentReviewer(t *testing.T) {
	mux, logs, publisher := newTestWebhook("ia-reviewer")

	payload := `{
		"action":"review_requested",
		"number":1,
		"requested_reviewer":{"login":"victor.brutti","username":"victor.brutti"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	if logs.String() != "" {
		t.Fatalf("expected no log, got %q", logs.String())
	}

	if len(publisher.jobs) != 0 {
		t.Fatalf("expected no published job, got %+v", publisher.jobs)
	}
}

func TestReceiveDoesNothingWhenReviewRequestIsRemoved(t *testing.T) {
	mux, logs, publisher := newTestWebhook("victor.brutti")

	payload := `{
		"action":"review_request_removed",
		"number":1,
		"requested_reviewer":{"login":"victor.brutti","username":"victor.brutti"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	if logs.String() != "" {
		t.Fatalf("expected no log, got %q", logs.String())
	}

	if !strings.Contains(res.Body.String(), "ignored") {
		t.Fatalf("expected ignored response, got %q", res.Body.String())
	}

	if len(publisher.jobs) != 0 {
		t.Fatalf("expected no published job, got %+v", publisher.jobs)
	}
}

func TestReceiveReturnsServerErrorWhenPublishFails(t *testing.T) {
	var logs bytes.Buffer
	publisher := &fakePublisher{err: errors.New("redis down")}
	mux := http.NewServeMux()
	RegisterRoutes(mux, log.New(&logs, "", 0), publisher, "ia-reviewer", "test", "0.1.0", time.Now())

	payload := `{
		"action":"review_requested",
		"number":12,
		"requested_reviewer":{"login":"ia-reviewer","username":"ia-reviewer"},
		"repository":{"full_name":"Qualyagro/wiki"},
		"sender":{"login":"marcio"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, res.Code)
	}
}

func TestReceiveIgnoresUnsupportedMethodWithoutPayload(t *testing.T) {
	mux, _, publisher := newTestWebhook("victor.brutti")

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	if !strings.Contains(res.Body.String(), "ignored") {
		t.Fatalf("expected ignored response, got %q", res.Body.String())
	}

	if len(publisher.jobs) != 0 {
		t.Fatalf("expected no published job, got %+v", publisher.jobs)
	}
}

func TestReceiveRejectsInvalidJSON(t *testing.T) {
	mux, _, _ := newTestWebhook("victor.brutti")

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{invalid json`))
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
}

func TestHealth(t *testing.T) {
	mux, _, _ := newTestWebhook("victor.brutti")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
}

func newTestWebhook(botUsername string) (*http.ServeMux, *bytes.Buffer, *fakePublisher) {
	var logs bytes.Buffer
	publisher := &fakePublisher{}
	mux := http.NewServeMux()
	RegisterRoutes(mux, log.New(&logs, "", 0), publisher, botUsername, "test", "0.1.0", time.Now())
	return mux, &logs, publisher
}

type fakePublisher struct {
	jobs []queue.ReviewJob
	err  error
}

func (p *fakePublisher) Publish(ctx context.Context, job queue.ReviewJob) error {
	if p.err != nil {
		return p.err
	}

	p.jobs = append(p.jobs, job)
	return nil
}

func assertJob(t *testing.T, got queue.ReviewJob, want queue.ReviewJob) {
	t.Helper()

	if got != want {
		t.Fatalf("unexpected job\nwant: %+v\n got: %+v", want, got)
	}
}

func assertLogContains(t *testing.T, logs string, want string) {
	t.Helper()

	if !strings.Contains(logs, want) {
		t.Fatalf("expected log to contain %q, got %q", want, logs)
	}
}
