package reviews

import (
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes attaches the versioned review API to an authenticated router
// group owned by the caller.
func RegisterRoutes(router gin.IRoutes, service *review.Service, repo *review.Repository) {
	handler := NewReviewHandler(service, repo)
	router.GET("/reviews", handler.List)
	router.POST("/reviews", handler.Create)
	router.GET("/reviews/:id", handler.Get)
	router.GET("/reviews/:id/status", handler.Get)
	router.GET("/reviews/:id/steps", handler.Steps)
	router.GET("/reviews/:id/result", handler.Result)
	router.POST("/reviews/:id/reprocess", handler.Reprocess)
	router.POST("/reviews/:id/cancel", handler.Cancel)
}
