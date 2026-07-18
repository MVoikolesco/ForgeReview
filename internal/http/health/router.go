package health

import (
	"net/http"
	"time"

	"gitea-agents/internal/config"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes attaches the public service health endpoint.
func RegisterRoutes(router gin.IRoutes, cfg config.Config, started time.Time) {
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":         "ok",
			"service":        cfg.ServiceName,
			"version":        cfg.Version,
			"uptime_seconds": int(time.Since(started).Seconds()),
		})
	})
}
