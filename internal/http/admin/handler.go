package admin

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
