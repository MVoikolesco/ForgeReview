package httpapi

import (
	"context"
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
