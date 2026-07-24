package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitea-agents/internal/config"
	databasepkg "gitea-agents/internal/database"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"
)

type stubPublisher struct{}

func (stubPublisher) Publish(context.Context, queue.ReviewJob) error {
	return nil
}

func TestRouterKeepsHealthAndAdminContracts(t *testing.T) {
	db, err := databasepkg.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	repo := review.NewRepository(db.SQL)
	service := review.NewService(config.Config{}, repo, stubPublisher{})
	cfg := config.Config{ServiceName: "test", Version: "test", AdminUsername: "admin", AdminPassword: "secret", GiteaBotUsername: "ia-reviewer"}
	router := NewRouter(cfg, db.SQL, repo, service, nil, log.Default())
	for _, request := range []*http.Request{httptest.NewRequest(http.MethodGet, "/health", nil), httptest.NewRequest(http.MethodGet, "/api/admin/ai/providers", nil)} {
		if request.URL.Path != "/health" {
			request.SetBasicAuth("admin", "secret")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", request.URL.Path, response.Code, response.Body.String())
		}
	}
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/reviews", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("versioned API must require auth, got %d", unauthenticated.Code)
	}
}

func TestRouterRegistersDomainRoutes(t *testing.T) {
	db, err := databasepkg.Open(config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	repo := review.NewRepository(db.SQL)
	service := review.NewService(config.Config{}, repo, stubPublisher{})
	router := NewRouter(config.Config{}, db.SQL, repo, service, nil, log.Default())
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	for _, route := range []string{
		"GET /health",
		"POST /review",
		"GET /webhook",
		"POST /webhook",
		"GET /api/v1/reviews",
		"POST /api/v1/reviews",
		"POST /api/v1/reviews/:id/reprocess",
		"GET /api/admin/status",
	} {
		if !routes[route] {
			t.Errorf("expected route %s to be registered", route)
		}
	}
}

func TestPipelineAPIProcessorConfigsSurvivePublishRoundTrip(t *testing.T) {
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
	repo := review.NewRepository(db.SQL)
	pipelines, err := repo.AdminPipelines(ctx)
	if err != nil || len(pipelines) == 0 || len(pipelines[0].Versions) == 0 {
		t.Fatalf("load seeded pipeline: pipelines=%d err=%v", len(pipelines), err)
	}
	base := pipelines[0].Versions[0].Stages
	inputs := make([]review.PipelineStageInput, len(base))
	for index, stage := range base {
		inputs[index] = review.PipelineStageInput{StageTypeKey: stage.StageTypeKey, Key: stage.Key, Name: stage.Name, Prompt: stage.PromptTemplate, ModelID: stage.ModelID, MaxTokens: stage.MaxOutputTokens, RetryLimit: stage.RetryLimit, Timeout: stage.TimeoutSeconds, UseLLM: stage.UseLLM, Required: stage.Required, RouteMode: stage.RouteMode, JoinMode: stage.JoinMode, Config: stage.Config}
	}
	filter := inputs[1]
	filter.StageTypeKey, filter.Key, filter.Name, filter.UseLLM = "file_filter", "extension-filter", "Extension filter", false
	filter.Config = map[string]any{"include_extensions": []string{".tsx"}, "x": 100, "y": 200, "input_side": "left", "output_side": "right"}
	reviewer := inputs[2]
	reviewer.StageTypeKey, reviewer.Key = "llm_review", "llm-review"
	merge := inputs[3]
	merge.StageTypeKey, merge.Key, merge.Name, merge.UseLLM = "findings_merge", "findings-merge", "Findings merge", false
	merge.Config = map[string]any{"operation": "dedupe_findings", "x": 300, "y": 400, "input_side": "top", "output_side": "bottom"}
	inputs = append([]review.PipelineStageInput{inputs[0], filter, reviewer, merge}, inputs[3:]...)
	payload, _ := json.Marshal(review.PipelineCreateInput{Key: "api-processors", PipelineDraftInput: review.PipelineDraftInput{Name: "API processors", Stages: inputs}})

	service := review.NewService(config.Config{}, repo, stubPublisher{})
	cfg := config.Config{AdminUsername: "admin", AdminPassword: "secret"}
	router := NewRouter(cfg, db.SQL, repo, service, nil, log.Default())
	request := httptest.NewRequest(http.MethodPost, "/api/admin/review/pipelines", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth("admin", "secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create pipeline returned %d: %s", response.Code, response.Body.String())
	}
	var created review.PipelineInfo
	if err = json.Unmarshal(response.Body.Bytes(), &created); err != nil || len(created.Versions) != 1 {
		t.Fatalf("decode created pipeline: %#v err=%v", created, err)
	}
	publishPath := fmt.Sprintf("/api/admin/review/pipelines/%d/drafts/%d/publish", created.ID, created.Versions[0].ID)
	request = httptest.NewRequest(http.MethodPost, publishPath, nil)
	request.SetBasicAuth("admin", "secret")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("publish pipeline returned %d: %s", response.Code, response.Body.String())
	}
	var published review.PipelineInfo
	if err = json.Unmarshal(response.Body.Bytes(), &published); err != nil || len(published.Versions) != 1 {
		t.Fatalf("decode published pipeline: %#v err=%v", published, err)
	}
	stages := published.Versions[0].Stages
	if stages[1].StageTypeKey != "file_filter" || stages[1].Config["include_extensions"] == nil || stages[1].Config["input_side"] != "left" || stages[3].StageTypeKey != "findings_merge" || stages[3].Config["operation"] != "dedupe_findings" || stages[3].Config["output_side"] != "bottom" {
		t.Fatalf("processor configs did not survive API publication: %#v", stages)
	}
}
