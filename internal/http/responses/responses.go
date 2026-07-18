package responses

import "github.com/gin-gonic/gin"

// Envelope is the stable response wrapper used by versioned API endpoints.
type Envelope struct {
	Success bool      `json:"success"`
	Data    any       `json:"data"`
	Error   *APIError `json:"error"`
}

// APIError describes a machine-readable versioned API failure.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// OK writes a successful versioned API envelope with status and data.
func OK(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{Success: true, Data: data})
}

// Fail writes a failed versioned API envelope with status, code, and message.
func Fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, Envelope{Success: false, Error: &APIError{Code: code, Message: message}})
}
