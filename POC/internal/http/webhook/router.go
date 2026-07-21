package webhook

import (
	"gitea-agents/internal/config"
	"gitea-agents/internal/review"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes attaches public Gitea webhook and legacy manual-review routes.
func RegisterRoutes(router gin.IRoutes, service *review.Service, cfg config.Config) {
	handler := NewWebhookHandler(service, cfg)
	router.Any("/webhook", handler.Receive)
	router.POST("/review", handler.Manual)
}
