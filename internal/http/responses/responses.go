package responses

import "github.com/gin-gonic/gin"

type Envelope struct {
	Success bool      `json:"success"`
	Data    any       `json:"data"`
	Error   *APIError `json:"error"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func OK(c *gin.Context, status int, data any) { c.JSON(status, Envelope{Success: true, Data: data}) }
func Fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, Envelope{Success: false, Error: &APIError{Code: code, Message: message}})
}
