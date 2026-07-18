package httpapi

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"gitea-agents/internal/config"
	"gitea-agents/internal/http/admin"
	"gitea-agents/internal/http/health"
	"gitea-agents/internal/http/middlewares"
	"gitea-agents/internal/http/reviews"
	"gitea-agents/internal/http/webhook"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

// NewRouter builds the Gin engine and its global middleware. Route composition
// is delegated to RegisterRoutes so domain handlers own their endpoints.
func NewRouter(
	cfg config.Config,
	db *sql.DB,
	repo *review.Repository,
	service *review.Service,
	observer queue.Observer,
	logger *log.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	if cfg.Debug() {
		gin.SetMode(gin.DebugMode)
	}
	r := gin.New()
	r.Use(middlewares.Recovery(), middlewares.CORS(), middlewares.AccessLog(logger))

	RegisterRoutes(r, cfg, db, repo, service, observer)
	return r
}

// RegisterRoutes is the sole HTTP route composition point. Domain handlers
// register their own endpoints; this function only establishes shared groups.
func RegisterRoutes(
	r *gin.Engine,
	cfg config.Config,
	db *sql.DB,
	repo *review.Repository,
	service *review.Service,
	observer queue.Observer,
) {
	health.RegisterRoutes(r, cfg, time.Now())

	webhook.RegisterRoutes(r, service, cfg)

	api := r.Group("/api/v1", middlewares.BasicAuth(cfg.AdminUsername, cfg.AdminPassword))
	reviews.RegisterRoutes(api, service, repo)

	adminRoutes := r.Group("/api/admin", middlewares.BasicAuth(cfg.AdminUsername, cfg.AdminPassword))
	admin.RegisterRoutes(adminRoutes, db, cfg, service, repo, observer)

	fileServer := http.FileServer(http.Dir("./web"))
	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.Status(http.StatusNotFound)
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
