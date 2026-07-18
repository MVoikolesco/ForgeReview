package middlewares

import (
	"crypto/subtle"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Recovery returns Gin middleware that converts panics to the versioned
// internal-error response without exposing implementation details.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": "internal server error"}})
	})
}

// CORS returns permissive development CORS middleware and handles preflight
// OPTIONS requests before they reach route handlers.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// AccessLog returns middleware that writes method, path, status, and duration
// to out after each request completes.
func AccessLog(out *log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		out.Printf("method=%s path=%s status=%d duration_ms=%d", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(started).Milliseconds())
	}
}

// BasicAuth returns middleware that validates one username/password pair using
// constant-time comparison and aborts unauthorized requests.
func BasicAuth(username, password string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, p, ok := c.Request.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
			c.Header("WWW-Authenticate", `Basic realm="ForgeReview admin"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		c.Next()
	}
}
