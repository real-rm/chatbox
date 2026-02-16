// Package httperrors provides generic error responses for HTTP endpoints.
// It ensures that internal implementation details are not leaked to clients.
package httperrors

import (
	"github.com/gin-gonic/gin"
)

// ErrorResponse represents a generic error response for clients
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// Generic error messages that don't expose internal details
const (
	MsgUnauthorized          = "Authentication required"
	MsgInvalidToken          = "Invalid or expired authentication token"
	MsgInvalidAuthHeader     = "Invalid authorization header"
	MsgForbidden             = "Insufficient permissions"
	MsgInvalidRequest        = "Invalid request parameters"
	MsgInternalError         = "An internal error occurred"
	MsgServiceUnavailable    = "Service temporarily unavailable"
	MsgResourceNotFound      = "Resource not found"
	MsgBadRequest            = "Bad request"
	MsgInvalidTimeFormat     = "Invalid time format, expected RFC3339"
	MsgSessionNotFound       = "Session not found"
	MsgOperationFailed       = "Operation failed"
)

// Error codes for client-side handling
const (
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeInvalidToken       = "INVALID_TOKEN"
	CodeForbidden          = "FORBIDDEN"
	CodeInvalidRequest     = "INVALID_REQUEST"
	CodeInternalError      = "INTERNAL_ERROR"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeNotFound           = "NOT_FOUND"
	CodeBadRequest         = "BAD_REQUEST"
)

// RespondUnauthorized sends a 401 response with a generic message
func RespondUnauthorized(c *gin.Context, message string) {
	if message == "" {
		message = MsgUnauthorized
	}
	c.JSON(401, ErrorResponse{
		Error: message,
		Code:  CodeUnauthorized,
	})
}

// RespondInvalidToken sends a 401 response for invalid tokens
func RespondInvalidToken(c *gin.Context) {
	c.JSON(401, ErrorResponse{
		Error: MsgInvalidToken,
		Code:  CodeInvalidToken,
	})
}

// RespondForbidden sends a 403 response with a generic message
func RespondForbidden(c *gin.Context) {
	c.JSON(403, ErrorResponse{
		Error: MsgForbidden,
		Code:  CodeForbidden,
	})
}

// RespondBadRequest sends a 400 response with a generic message
func RespondBadRequest(c *gin.Context, message string) {
	if message == "" {
		message = MsgBadRequest
	}
	c.JSON(400, ErrorResponse{
		Error: message,
		Code:  CodeBadRequest,
	})
}

// RespondInternalError sends a 500 response with a generic message
func RespondInternalError(c *gin.Context) {
	c.JSON(500, ErrorResponse{
		Error: MsgInternalError,
		Code:  CodeInternalError,
	})
}

// RespondServiceUnavailable sends a 503 response
func RespondServiceUnavailable(c *gin.Context) {
	c.JSON(503, ErrorResponse{
		Error: MsgServiceUnavailable,
		Code:  CodeServiceUnavailable,
	})
}

// RespondNotFound sends a 404 response
func RespondNotFound(c *gin.Context, message string) {
	if message == "" {
		message = MsgResourceNotFound
	}
	c.JSON(404, ErrorResponse{
		Error: message,
		Code:  CodeNotFound,
	})
}
