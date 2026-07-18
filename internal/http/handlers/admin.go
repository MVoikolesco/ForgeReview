package handlers

import (
	"database/sql"
	"strings"

	"gitea-agents/internal/config"
	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

// AdminHandler coordinates authenticated administrative endpoints. Its
// dependencies provide configuration, persistence, review execution, and queue
// observability; handler methods write their result directly to Gin's context.
type AdminHandler struct {
	db         *sql.DB
	cfg        config.Config
	reviews    *review.Service
	reviewRepo *review.Repository
	observer   queue.Observer
}

var resources = map[string]string{
	"ai/providers":        "ai_providers",
	"ai/connections":      "ai_connections",
	"ai/models":           "ai_models",
	"ai/model-parameters": "model_parameters",
	"review/profiles":     "review_profiles",
	"review/prompts":      "review_prompts",
	"review/policies":     "review_policies",
	"gitea/instances":     "gitea_instances",
	"repositories":        "repositories",
}

// NewAdminHandler builds an administrative handler from its database,
// configuration, review service, review repository, and optional queue
// observer. It returns a handler ready to register on a protected Gin group.
func NewAdminHandler(
	db *sql.DB,
	cfg config.Config,
	reviews *review.Service,
	repo *review.Repository,
	observer queue.Observer,
) *AdminHandler {
	return &AdminHandler{
		db:         db,
		cfg:        cfg,
		reviews:    reviews,
		reviewRepo: repo,
		observer:   observer,
	}
}

// Register attaches all administrative routes to the supplied Gin router
// group. Authentication must be configured by the caller on that group.
func (h *AdminHandler) Register(router *gin.RouterGroup) {
	router.GET("/status", h.status)
	router.Any("/setup/:action", h.setup)
	router.GET("/observability/metrics", h.metrics)
	router.GET("/observability/logs", func(c *gin.Context) {
		responses.OK(c, 200, gin.H{"lines": []string{}, "available": false})
	})
	router.GET("/observability/reviews", h.observabilityReviews)
	router.GET("/observability/progress", h.observabilityProgress)
	router.POST("/reviews/manual", h.manual)
	router.GET("/reviews/pending", h.pending)
	router.Any("/reviews/pending/:action", h.pending)
	router.POST("/gitea/instances/test", h.testGitea)
	router.POST("/gitea/instances/:id/test", h.testGitea)
	router.POST("/gitea/instances/:id/organizations", h.organizations)
	router.POST("/gitea/instances/:id/repositories", h.giteaRepositories)
	router.POST("/gitea/instances/:id/pull-requests", h.pullRequests)

	for path := range resources {
		h.registerResource(router, path)
	}
}

// registerResource attaches the generic CRUD endpoints for one allow-listed
// administrative resource.
func (h *AdminHandler) registerResource(router *gin.RouterGroup, path string) {
	router.GET("/"+path, h.list)
	router.POST("/"+path, h.create)
	router.GET("/"+path+"/:id", h.one)
	router.PUT("/"+path+"/:id", h.update)
	router.PATCH("/"+path+"/:id", h.update)
	router.DELETE("/"+path+"/:id", h.delete)
	router.POST("/"+path+"/:id/set-default", h.setDefault)

	if path == "ai/connections" {
		router.POST("/"+path+"/:id/test", h.testConnection)
	}
}

// table resolves an administrative resource path to its allow-listed SQLite
// table. The boolean return is false for unknown resources.
func (h *AdminHandler) table(path string) (string, bool) {
	table, ok := resources[strings.Trim(path, "/")]
	return table, ok
}

// tableFromRequest resolves the current request path to an allow-listed table
// and writes a not-found response when no resource matches.
func (h *AdminHandler) tableFromRequest(c *gin.Context) (string, bool) {
	path := strings.TrimPrefix(c.Request.URL.Path, "/api/admin/")
	for key, table := range resources {
		if path == key || strings.HasPrefix(path, key+"/") {
			return table, true
		}
	}

	responses.Fail(c, 404, "NOT_FOUND", "resource not found")
	return "", false
}
