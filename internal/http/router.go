package httpapi

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"gitea-agents/internal/config"
	"gitea-agents/internal/http/handlers"
	"gitea-agents/internal/http/middlewares"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

// NewRouter builds the Gin engine, middleware chain, public integration routes,
// authenticated APIs, administrative routes, and static frontend fallback.
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

	started := time.Now()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":         "ok",
			"service":        cfg.ServiceName,
			"version":        cfg.Version,
			"uptime_seconds": int(time.Since(started).Seconds()),
		})
	})

	webhook := handlers.NewWebhookHandler(service, cfg)
	r.Any("/webhook", webhook.Receive)
	r.POST("/review", webhook.Manual)
	reviewHandler := handlers.NewReviewHandler(service, repo)
	api := r.Group("/api/v1", middlewares.BasicAuth(cfg.AdminUsername, cfg.AdminPassword))
	api.GET("/reviews", reviewHandler.List)
	api.POST("/reviews", reviewHandler.Create)
	api.GET("/reviews/:id", reviewHandler.Get)
	api.GET("/reviews/:id/status", reviewHandler.Get)
	api.GET("/reviews/:id/steps", reviewHandler.Steps)
	api.GET("/reviews/:id/result", reviewHandler.Result)
	api.POST("/reviews/:id/reprocess", reviewHandler.Reprocess)
	api.POST("/reviews/:id/cancel", reviewHandler.Cancel)

	admin := r.Group("/api/admin", middlewares.BasicAuth(cfg.AdminUsername, cfg.AdminPassword))
	handlers.NewAdminHandler(db, cfg, service, repo, observer).Register(admin)

	fileServer := http.FileServer(http.Dir("./web"))
	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.Status(http.StatusNotFound)
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	return r
}
