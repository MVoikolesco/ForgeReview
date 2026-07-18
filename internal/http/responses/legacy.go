package responses

import "github.com/gin-gonic/gin"

func Legacy(c *gin.Context, status int, data any)            { c.JSON(status, data) }
func LegacyError(c *gin.Context, status int, message string) { c.JSON(status, gin.H{"error": message}) }
