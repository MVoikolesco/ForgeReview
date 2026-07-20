package admin

import (
	"database/sql"
	"net/http"

	"gitea-agents/internal/config"
	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes attaches administrative routes to an authenticated router
// group owned by the caller.
func RegisterRoutes(
	router gin.IRoutes,
	db *sql.DB,
	cfg config.Config,
	reviews *review.Service,
	repo *review.Repository,
	observer queue.Observer,
) {
	handler := NewAdminHandler(db, cfg, reviews, repo, observer)
	router.GET("/status", handler.status)
	router.Any("/setup/:action", handler.setup)
	router.GET("/observability/metrics", handler.metrics)
	router.GET("/observability/logs", func(c *gin.Context) {
		responses.OK(c, http.StatusOK, gin.H{"lines": []string{}, "available": false})
	})
	router.GET("/observability/reviews", handler.observabilityReviews)
	router.GET("/observability/progress", handler.observabilityProgress)
	router.GET("/observability/stage-logs", handler.observabilityStageLogs)
	router.POST("/reviews/manual", handler.manual)
	router.GET("/review/settings", handler.reviewSettings)
	router.GET("/review/pipelines", handler.pipelines)
	router.POST("/review/pipelines", handler.createPipeline)
	router.GET("/review/pipelines/select", handler.selectPipeline)
	router.GET("/review/pipelines/:id", handler.pipeline)
	router.GET("/review/pipelines/:id/drafts/:versionID", handler.pipelineDraft)
	router.PUT("/review/pipelines/:id/drafts/:versionID", handler.updatePipelineDraft)
	router.POST("/review/pipelines/:id/drafts/:versionID/validate", handler.validatePipelineDraft)
	router.POST("/review/pipelines/:id/drafts/:versionID/publish", handler.publishPipelineDraft)
	router.POST("/review/pipelines/:id/clone", handler.clonePipeline)
	router.POST("/review/pipelines/:id/rollback", handler.clonePipeline)
	router.POST("/review/pipelines/:id/select", handler.selectPipelineForProfile)
	router.GET("/reviews/pending", handler.pending)
	router.Any("/reviews/pending/:action", handler.pending)
	router.POST("/gitea/instances/test", handler.testGitea)
	router.POST("/gitea/instances/:id/test", handler.testGitea)
	router.POST("/gitea/instances/:id/organizations", handler.organizations)
	router.POST("/gitea/instances/:id/repositories", handler.giteaRepositories)
	router.POST("/gitea/instances/:id/pull-requests", handler.pullRequests)

	for path := range resources {
		registerResourceRoutes(router, handler, path)
	}
}

// registerResourceRoutes attaches the CRUD endpoints for an allow-listed
// administrative resource.
func registerResourceRoutes(router gin.IRoutes, handler *AdminHandler, path string) {
	router.GET("/"+path, handler.list)
	router.POST("/"+path, handler.create)
	router.GET("/"+path+"/:id", handler.one)
	router.PUT("/"+path+"/:id", handler.update)
	router.PATCH("/"+path+"/:id", handler.update)
	router.DELETE("/"+path+"/:id", handler.delete)
	router.POST("/"+path+"/:id/set-default", handler.setDefault)

	if path == "ai/connections" {
		router.POST("/"+path+"/:id/test", handler.testConnection)
	}
}
