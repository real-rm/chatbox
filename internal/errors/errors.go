// Package errors provides error handling functionality for the WebSocket chat application.
// It defines error categories, error types, and error message generation.
package errors

import (
	"fmt"

	"github.com/real-rm/chatbox/internal/message"
)

// ErrorCategory represents the category of an error
type ErrorCategory string

const (
	// CategoryAuth represents authentication and authorization errors
	CategoryAuth ErrorCategory = "auth"
	// CategoryValidation represents input validation errors
	CategoryValidation ErrorCategory = "validation"
	// CategoryService represents service-level errors (LLM, database, S3)
	CategoryService ErrorCategory = "service"
	// CategoryRateLimit represents rate limiting errors
	CategoryRateLimit ErrorCategory = "rate_limit"
)

// ErrorCode represents specific error codes
type ErrorCode string

const (
	// Authentication errors
	ErrCodeInvalidToken      ErrorCode = "INVALID_TOKEN"
	ErrCodeExpiredToken      ErrorCode = "EXPIRED_TOKEN"
	ErrCodeInsufficientPerms ErrorCode = "INSUFFICIENT_PERMISSIONS"

	// Validation errors
	ErrCodeInvalidFormat   ErrorCode = "INVALID_FORMAT"
	ErrCodeMissingField    ErrorCode = "MISSING_FIELD"
	ErrCodeInvalidFileType ErrorCode = "INVALID_FILE_TYPE"
	ErrCodeInvalidFileSize ErrorCode = "INVALID_FILE_SIZE"

	// Service errors
	ErrCodeLLMUnavailable ErrorCode = "LLM_UNAVAILABLE"
	ErrCodeDatabaseError  ErrorCode = "DATABASE_ERROR"
	ErrCodeStorageError   ErrorCode = "STORAGE_ERROR"
	ErrCodeServiceError   ErrorCode = "SERVICE_ERROR"

	// Rate limiting errors
	ErrCodeTooManyRequests ErrorCode = "TOO_MANY_REQUESTS"
	ErrCodeConnectionLimit ErrorCode = "CONNECTION_LIMIT_EXCEEDED"
)

// ChatError represents an application error with category and recoverability information
type ChatError struct {
	Category    ErrorCategory
	Code        ErrorCode
	Message     string
	Recoverable bool
	RetryAfter  int // milliseconds, only for rate limit errors
	Cause       error
}

// Error implements the error interface
func (e *ChatError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause error
func (e *ChatError) Unwrap() error {
	return e.Cause
}

// IsFatal returns true if the error is fatal and requires connection closure
func (e *ChatError) IsFatal() bool {
	return !e.Recoverable
}

// ToErrorInfo converts a ChatError to a message.ErrorInfo for wire protocol
func (e *ChatError) ToErrorInfo() *message.ErrorInfo {
	return &message.ErrorInfo{
		Code:        string(e.Code),
		Message:     e.Message,
		Recoverable: e.Recoverable,
		RetryAfter:  e.RetryAfter,
	}
}

// NewAuthError creates a new authentication error (fatal)
func NewAuthError(code ErrorCode, message string, cause error) *ChatError {
	return &ChatError{
		Category:    CategoryAuth,
		Code:        code,
		Message:     message,
		Recoverable: false, // Auth errors are fatal
		Cause:       cause,
	}
}

// NewValidationError creates a new validation error (recoverable)
func NewValidationError(code ErrorCode, message string, cause error) *ChatError {
	return &ChatError{
		Category:    CategoryValidation,
		Code:        code,
		Message:     message,
		Recoverable: true, // Validation errors are recoverable
		Cause:       cause,
	}
}

// NewServiceError creates a new service error (recoverable with retry)
func NewServiceError(code ErrorCode, message string, cause error) *ChatError {
	return &ChatError{
		Category:    CategoryService,
		Code:        code,
		Message:     message,
		Recoverable: true, // Service errors are recoverable
		Cause:       cause,
	}
}

// NewRateLimitError creates a new rate limit error (recoverable with retry after)
func NewRateLimitError(code ErrorCode, message string, retryAfter int, cause error) *ChatError {
	return &ChatError{
		Category:    CategoryRateLimit,
		Code:        code,
		Message:     message,
		Recoverable: true,
		RetryAfter:  retryAfter,
		Cause:       cause,
	}
}

// Common error constructors for convenience

// ErrInvalidToken creates an invalid token error
func ErrInvalidToken(cause error) *ChatError {
	return NewAuthError(ErrCodeInvalidToken, "Invalid authentication token", cause)
}

// ErrExpiredToken creates an expired token error
func ErrExpiredToken(cause error) *ChatError {
	return NewAuthError(ErrCodeExpiredToken, "Authentication token has expired", cause)
}

// ErrInsufficientPermissions creates an insufficient permissions error
func ErrInsufficientPermissions(cause error) *ChatError {
	return NewAuthError(ErrCodeInsufficientPerms, "Insufficient permissions for this operation", cause)
}

// ErrInvalidMessageFormat creates an invalid message format error
func ErrInvalidMessageFormat(details string, cause error) *ChatError {
	return NewValidationError(ErrCodeInvalidFormat, fmt.Sprintf("Invalid message format: %s", details), cause)
}

// ErrMissingField creates a missing field error
func ErrMissingField(fieldName string) *ChatError {
	return NewValidationError(ErrCodeMissingField, fmt.Sprintf("Required field missing: %s", fieldName), nil)
}

// ErrInvalidFileType creates an invalid file type error
func ErrInvalidFileType(fileType string) *ChatError {
	return NewValidationError(ErrCodeInvalidFileType, fmt.Sprintf("Invalid file type: %s", fileType), nil)
}

// ErrInvalidFileSize creates an invalid file size error
func ErrInvalidFileSize(size int64, maxSize int64) *ChatError {
	return NewValidationError(ErrCodeInvalidFileSize,
		fmt.Sprintf("File size %d bytes exceeds maximum %d bytes", size, maxSize), nil)
}

// ErrLLMUnavailable creates an LLM unavailable error
func ErrLLMUnavailable(cause error) *ChatError {
	return NewServiceError(ErrCodeLLMUnavailable, "AI service is temporarily unavailable", cause)
}

// ErrDatabaseError creates a database error
func ErrDatabaseError(cause error) *ChatError {
	return NewServiceError(ErrCodeDatabaseError, "Database operation failed", cause)
}

// ErrStorageError creates a storage error
func ErrStorageError(cause error) *ChatError {
	return NewServiceError(ErrCodeStorageError, "File storage operation failed", cause)
}

// ErrTooManyRequests creates a too many requests error
func ErrTooManyRequests(retryAfter int) *ChatError {
	return NewRateLimitError(ErrCodeTooManyRequests,
		"Too many requests, please slow down", retryAfter, nil)
}

// ErrConnectionLimitExceeded creates a connection limit exceeded error
func ErrConnectionLimitExceeded(retryAfter int) *ChatError {
	return NewRateLimitError(ErrCodeConnectionLimit,
		"Connection limit exceeded, please try again later", retryAfter, nil)
}
