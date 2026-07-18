package responses

import "github.com/gin-gonic/gin"

// Legacy writes an unwrapped JSON response required by the existing admin UI.
func Legacy(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

// LegacyError writes the legacy {error:string} response expected by the admin UI.
func LegacyError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}
