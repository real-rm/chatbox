package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/message"
)

// Test all error constructors

func TestNewAuthError(t *testing.T) {
	cause := errors.New("underlying auth error")
	err := NewAuthError(ErrCodeInvalidToken, "test auth error", cause)

	if err.Category != CategoryAuth {
		t.Errorf("Expected category %s, got %s", CategoryAuth, err.Category)
	}
	if err.Code != ErrCodeInvalidToken {
		t.Errorf("Expected code %s, got %s", ErrCodeInvalidToken, err.Code)
	}
	if err.Message != "test auth error" {
		t.Errorf("Expected message 'test auth error', got '%s'", err.Message)
	}
	if err.Recoverable {
		t.Error("Expected auth error to be non-recoverable")
	}
	if err.Cause != cause {
		t.Error("Expected cause to be set")
	}
}

func TestNewValidationError(t *testing.T) {
	cause := errors.New("underlying validation error")
	err := NewValidationError(ErrCodeInvalidFormat, "test validation error", cause)

	if err.Category != CategoryValidation {
		t.Errorf("Expected category %s, got %s", CategoryValidation, err.Category)
	}
	if err.Code != ErrCodeInvalidFormat {
		t.Errorf("Expected code %s, got %s", ErrCodeInvalidFormat, err.Code)
	}
	if err.Message != "test validation error" {
		t.Errorf("Expected message 'test validation error', got '%s'", err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected validation error to be recoverable")
	}
	if err.Cause != cause {
		t.Error("Expected cause to be set")
	}
}

func TestNewServiceError(t *testing.T) {
	cause := errors.New("underlying service error")
	err := NewServiceError(ErrCodeDatabaseError, "test service error", cause)

	if err.Category != CategoryService {
		t.Errorf("Expected category %s, got %s", CategoryService, err.Category)
	}
	if err.Code != ErrCodeDatabaseError {
		t.Errorf("Expected code %s, got %s", ErrCodeDatabaseError, err.Code)
	}
	if err.Message != "test service error" {
		t.Errorf("Expected message 'test service error', got '%s'", err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected service error to be recoverable")
	}
	if err.Cause != cause {
		t.Error("Expected cause to be set")
	}
}

func TestNewRateLimitError(t *testing.T) {
	cause := errors.New("underlying rate limit error")
	retryAfter := 5000
	err := NewRateLimitError(ErrCodeTooManyRequests, "test rate limit error", retryAfter, cause)

	if err.Category != CategoryRateLimit {
		t.Errorf("Expected category %s, got %s", CategoryRateLimit, err.Category)
	}
	if err.Code != ErrCodeTooManyRequests {
		t.Errorf("Expected code %s, got %s", ErrCodeTooManyRequests, err.Code)
	}
	if err.Message != "test rate limit error" {
		t.Errorf("Expected message 'test rate limit error', got '%s'", err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected rate limit error to be recoverable")
	}
	if err.RetryAfter != retryAfter {
		t.Errorf("Expected retry after %d, got %d", retryAfter, err.RetryAfter)
	}
	if err.Cause != cause {
		t.Error("Expected cause to be set")
	}
}

// Test convenience error constructors

func TestErrInvalidToken(t *testing.T) {
	cause := errors.New("token parse error")
	err := ErrInvalidToken(cause)

	if err.Category != CategoryAuth {
		t.Errorf("Expected category %s, got %s", CategoryAuth, err.Category)
	}
	if err.Code != ErrCodeInvalidToken {
		t.Errorf("Expected code %s, got %s", ErrCodeInvalidToken, err.Code)
	}
	if err.Message != "Invalid authentication token" {
		t.Errorf("Expected standard message, got '%s'", err.Message)
	}
	if err.Recoverable {
		t.Error("Expected non-recoverable error")
	}
	if err.Cause != cause {
		t.Error("Expected cause to be set")
	}
}

func TestErrExpiredToken(t *testing.T) {
	cause := errors.New("token expired")
	err := ErrExpiredToken(cause)

	if err.Category != CategoryAuth {
		t.Errorf("Expected category %s, got %s", CategoryAuth, err.Category)
	}
	if err.Code != ErrCodeExpiredToken {
		t.Errorf("Expected code %s, got %s", ErrCodeExpiredToken, err.Code)
	}
	if err.Message != "Authentication token has expired" {
		t.Errorf("Expected standard message, got '%s'", err.Message)
	}
	if err.Recoverable {
		t.Error("Expected non-recoverable error")
	}
}

func TestErrInsufficientPermissions(t *testing.T) {
	cause := errors.New("permission denied")
	err := ErrInsufficientPermissions(cause)

	if err.Category != CategoryAuth {
		t.Errorf("Expected category %s, got %s", CategoryAuth, err.Category)
	}
	if err.Code != ErrCodeInsufficientPerms {
		t.Errorf("Expected code %s, got %s", ErrCodeInsufficientPerms, err.Code)
	}
	if err.Message != "Insufficient permissions for this operation" {
		t.Errorf("Expected standard message, got '%s'", err.Message)
	}
	if err.Recoverable {
		t.Error("Expected non-recoverable error")
	}
}

func TestErrInvalidMessageFormat(t *testing.T) {
	cause := errors.New("json parse error")
	err := ErrInvalidMessageFormat("missing required field", cause)

	if err.Category != CategoryValidation {
		t.Errorf("Expected category %s, got %s", CategoryValidation, err.Category)
	}
	if err.Code != ErrCodeInvalidFormat {
		t.Errorf("Expected code %s, got %s", ErrCodeInvalidFormat, err.Code)
	}
	expectedMsg := "Invalid message format: missing required field"
	if err.Message != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
}

func TestErrMissingField(t *testing.T) {
	err := ErrMissingField("username")

	if err.Category != CategoryValidation {
		t.Errorf("Expected category %s, got %s", CategoryValidation, err.Category)
	}
	if err.Code != ErrCodeMissingField {
		t.Errorf("Expected code %s, got %s", ErrCodeMissingField, err.Code)
	}
	expectedMsg := "Required field missing: username"
	if err.Message != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
	if err.Cause != nil {
		t.Error("Expected no cause")
	}
}

func TestErrInvalidFileType(t *testing.T) {
	err := ErrInvalidFileType("application/exe")

	if err.Category != CategoryValidation {
		t.Errorf("Expected category %s, got %s", CategoryValidation, err.Category)
	}
	if err.Code != ErrCodeInvalidFileType {
		t.Errorf("Expected code %s, got %s", ErrCodeInvalidFileType, err.Code)
	}
	expectedMsg := "Invalid file type: application/exe"
	if err.Message != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
}

func TestErrInvalidFileSize(t *testing.T) {
	err := ErrInvalidFileSize(2000000, 1000000)

	if err.Category != CategoryValidation {
		t.Errorf("Expected category %s, got %s", CategoryValidation, err.Category)
	}
	if err.Code != ErrCodeInvalidFileSize {
		t.Errorf("Expected code %s, got %s", ErrCodeInvalidFileSize, err.Code)
	}
	expectedMsg := "File size 2000000 bytes exceeds maximum 1000000 bytes"
	if err.Message != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
}

func TestErrLLMUnavailable(t *testing.T) {
	cause := errors.New("connection refused")
	err := ErrLLMUnavailable(cause)

	if err.Category != CategoryService {
		t.Errorf("Expected category %s, got %s", CategoryService, err.Category)
	}
	if err.Code != ErrCodeLLMUnavailable {
		t.Errorf("Expected code %s, got %s", ErrCodeLLMUnavailable, err.Code)
	}
	if err.Message != "AI service is temporarily unavailable" {
		t.Errorf("Expected standard message, got '%s'", err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
	if err.Cause != cause {
		t.Error("Expected cause to be set")
	}
}

func TestErrLLMTimeout(t *testing.T) {
	timeout := 30 * time.Second
	err := ErrLLMTimeout(timeout)

	if err.Category != CategoryService {
		t.Errorf("Expected category %s, got %s", CategoryService, err.Category)
	}
	if err.Code != ErrCodeLLMTimeout {
		t.Errorf("Expected code %s, got %s", ErrCodeLLMTimeout, err.Code)
	}
	expectedMsg := fmt.Sprintf("AI service request timed out after %v", timeout)
	if err.Message != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
}

func TestErrDatabaseError(t *testing.T) {
	cause := errors.New("connection lost")
	err := ErrDatabaseError(cause)

	if err.Category != CategoryService {
		t.Errorf("Expected category %s, got %s", CategoryService, err.Category)
	}
	if err.Code != ErrCodeDatabaseError {
		t.Errorf("Expected code %s, got %s", ErrCodeDatabaseError, err.Code)
	}
	if err.Message != "Database operation failed" {
		t.Errorf("Expected standard message, got '%s'", err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
	if err.Cause != cause {
		t.Error("Expected cause to be set")
	}
}

func TestErrStorageError(t *testing.T) {
	cause := errors.New("S3 error")
	err := ErrStorageError(cause)

	if err.Category != CategoryService {
		t.Errorf("Expected category %s, got %s", CategoryService, err.Category)
	}
	if err.Code != ErrCodeStorageError {
		t.Errorf("Expected code %s, got %s", ErrCodeStorageError, err.Code)
	}
	if err.Message != "File storage operation failed" {
		t.Errorf("Expected standard message, got '%s'", err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
	if err.Cause != cause {
		t.Error("Expected cause to be set")
	}
}

func TestErrTooManyRequests(t *testing.T) {
	retryAfter := 10000
	err := ErrTooManyRequests(retryAfter)

	if err.Category != CategoryRateLimit {
		t.Errorf("Expected category %s, got %s", CategoryRateLimit, err.Category)
	}
	if err.Code != ErrCodeTooManyRequests {
		t.Errorf("Expected code %s, got %s", ErrCodeTooManyRequests, err.Code)
	}
	if err.Message != "Too many requests, please slow down" {
		t.Errorf("Expected standard message, got '%s'", err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
	if err.RetryAfter != retryAfter {
		t.Errorf("Expected retry after %d, got %d", retryAfter, err.RetryAfter)
	}
}

func TestErrConnectionLimitExceeded(t *testing.T) {
	retryAfter := 15000
	err := ErrConnectionLimitExceeded(retryAfter)

	if err.Category != CategoryRateLimit {
		t.Errorf("Expected category %s, got %s", CategoryRateLimit, err.Category)
	}
	if err.Code != ErrCodeConnectionLimit {
		t.Errorf("Expected code %s, got %s", ErrCodeConnectionLimit, err.Code)
	}
	if err.Message != "Connection limit exceeded, please try again later" {
		t.Errorf("Expected standard message, got '%s'", err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
	if err.RetryAfter != retryAfter {
		t.Errorf("Expected retry after %d, got %d", retryAfter, err.RetryAfter)
	}
}

func TestErrNotFound(t *testing.T) {
	err := ErrNotFound("Session")

	if err.Category != CategoryValidation {
		t.Errorf("Expected category %s, got %s", CategoryValidation, err.Category)
	}
	if err.Code != ErrCodeNotFound {
		t.Errorf("Expected code %s, got %s", ErrCodeNotFound, err.Code)
	}
	expectedMsg := "Session not found"
	if err.Message != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, err.Message)
	}
	if !err.Recoverable {
		t.Error("Expected recoverable error")
	}
}

func TestErrUnauthorized(t *testing.T) {
	msg := "Access denied"
	err := ErrUnauthorized(msg)

	if err.Category != CategoryAuth {
		t.Errorf("Expected category %s, got %s", CategoryAuth, err.Category)
	}
	if err.Code != ErrCodeUnauthorized {
		t.Errorf("Expected code %s, got %s", ErrCodeUnauthorized, err.Code)
	}
	if err.Message != msg {
		t.Errorf("Expected message '%s', got '%s'", msg, err.Message)
	}
	if err.Recoverable {
		t.Error("Expected non-recoverable error")
	}
}

// Test error code validation

func TestErrorCodeConstants(t *testing.T) {
	tests := []struct {
		name string
		code ErrorCode
		want string
	}{
		{"InvalidToken", ErrCodeInvalidToken, "INVALID_TOKEN"},
		{"ExpiredToken", ErrCodeExpiredToken, "EXPIRED_TOKEN"},
		{"InsufficientPerms", ErrCodeInsufficientPerms, "INSUFFICIENT_PERMISSIONS"},
		{"Unauthorized", ErrCodeUnauthorized, "UNAUTHORIZED"},
		{"InvalidFormat", ErrCodeInvalidFormat, "INVALID_FORMAT"},
		{"MissingField", ErrCodeMissingField, "MISSING_FIELD"},
		{"InvalidFileType", ErrCodeInvalidFileType, "INVALID_FILE_TYPE"},
		{"InvalidFileSize", ErrCodeInvalidFileSize, "INVALID_FILE_SIZE"},
		{"NotFound", ErrCodeNotFound, "NOT_FOUND"},
		{"LLMUnavailable", ErrCodeLLMUnavailable, "LLM_UNAVAILABLE"},
		{"LLMTimeout", ErrCodeLLMTimeout, "LLM_TIMEOUT"},
		{"DatabaseError", ErrCodeDatabaseError, "DATABASE_ERROR"},
		{"StorageError", ErrCodeStorageError, "STORAGE_ERROR"},
		{"ServiceError", ErrCodeServiceError, "SERVICE_ERROR"},
		{"TooManyRequests", ErrCodeTooManyRequests, "TOO_MANY_REQUESTS"},
		{"ConnectionLimit", ErrCodeConnectionLimit, "CONNECTION_LIMIT_EXCEEDED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.code) != tt.want {
				t.Errorf("Expected code %s, got %s", tt.want, string(tt.code))
			}
		})
	}
}

func TestErrorCategoryConstants(t *testing.T) {
	tests := []struct {
		name     string
		category ErrorCategory
		want     string
	}{
		{"Auth", CategoryAuth, "auth"},
		{"Validation", CategoryValidation, "validation"},
		{"Service", CategoryService, "service"},
		{"RateLimit", CategoryRateLimit, "rate_limit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.category) != tt.want {
				t.Errorf("Expected category %s, got %s", tt.want, string(tt.category))
			}
		})
	}
}

// Test error message formatting

func TestChatErrorError(t *testing.T) {
	t.Run("Error without cause", func(t *testing.T) {
		err := NewValidationError(ErrCodeMissingField, "field is required", nil)
		expected := "MISSING_FIELD: field is required"
		if err.Error() != expected {
			t.Errorf("Expected error string '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("Error with cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := NewServiceError(ErrCodeDatabaseError, "database failed", cause)
		expected := "DATABASE_ERROR: database failed (caused by: underlying error)"
		if err.Error() != expected {
			t.Errorf("Expected error string '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("Error with formatted message", func(t *testing.T) {
		err := ErrInvalidFileSize(2000, 1000)
		if err.Error() == "" {
			t.Error("Expected non-empty error string")
		}
		// Should contain code and message
		errStr := err.Error()
		if len(errStr) < 10 {
			t.Errorf("Error string too short: '%s'", errStr)
		}
	})
}

func TestChatErrorUnwrap(t *testing.T) {
	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("root cause")
		err := NewAuthError(ErrCodeInvalidToken, "token error", cause)

		unwrapped := err.Unwrap()
		if unwrapped != cause {
			t.Error("Expected Unwrap to return the cause")
		}
	})

	t.Run("Unwrap returns nil when no cause", func(t *testing.T) {
		err := NewValidationError(ErrCodeMissingField, "field error", nil)

		unwrapped := err.Unwrap()
		if unwrapped != nil {
			t.Error("Expected Unwrap to return nil when no cause")
		}
	})
}

func TestChatErrorIsFatal(t *testing.T) {
	tests := []struct {
		name    string
		err     *ChatError
		isFatal bool
	}{
		{
			name:    "Auth error is fatal",
			err:     NewAuthError(ErrCodeInvalidToken, "test", nil),
			isFatal: true,
		},
		{
			name:    "Validation error is not fatal",
			err:     NewValidationError(ErrCodeInvalidFormat, "test", nil),
			isFatal: false,
		},
		{
			name:    "Service error is not fatal",
			err:     NewServiceError(ErrCodeDatabaseError, "test", nil),
			isFatal: false,
		},
		{
			name:    "Rate limit error is not fatal",
			err:     NewRateLimitError(ErrCodeTooManyRequests, "test", 5000, nil),
			isFatal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.IsFatal() != tt.isFatal {
				t.Errorf("Expected IsFatal() to be %v, got %v", tt.isFatal, tt.err.IsFatal())
			}
		})
	}
}

func TestChatErrorToErrorInfo(t *testing.T) {
	t.Run("Auth error conversion", func(t *testing.T) {
		err := NewAuthError(ErrCodeInvalidToken, "invalid token", nil)
		info := err.ToErrorInfo()

		if info.Code != string(ErrCodeInvalidToken) {
			t.Errorf("Expected code %s, got %s", ErrCodeInvalidToken, info.Code)
		}
		if info.Message != "invalid token" {
			t.Errorf("Expected message 'invalid token', got '%s'", info.Message)
		}
		if info.Recoverable {
			t.Error("Expected non-recoverable")
		}
		if info.RetryAfter != 0 {
			t.Errorf("Expected retry after 0, got %d", info.RetryAfter)
		}
	})

	t.Run("Rate limit error conversion", func(t *testing.T) {
		retryAfter := 5000
		err := NewRateLimitError(ErrCodeTooManyRequests, "too many requests", retryAfter, nil)
		info := err.ToErrorInfo()

		if info.Code != string(ErrCodeTooManyRequests) {
			t.Errorf("Expected code %s, got %s", ErrCodeTooManyRequests, info.Code)
		}
		if info.Message != "too many requests" {
			t.Errorf("Expected message 'too many requests', got '%s'", info.Message)
		}
		if !info.Recoverable {
			t.Error("Expected recoverable")
		}
		if info.RetryAfter != retryAfter {
			t.Errorf("Expected retry after %d, got %d", retryAfter, info.RetryAfter)
		}
	})

	t.Run("Validation error conversion", func(t *testing.T) {
		err := NewValidationError(ErrCodeMissingField, "field missing", nil)
		info := err.ToErrorInfo()

		if info.Code != string(ErrCodeMissingField) {
			t.Errorf("Expected code %s, got %s", ErrCodeMissingField, info.Code)
		}
		if info.Message != "field missing" {
			t.Errorf("Expected message 'field missing', got '%s'", info.Message)
		}
		if !info.Recoverable {
			t.Error("Expected recoverable")
		}
	})

	t.Run("Service error conversion", func(t *testing.T) {
		err := NewServiceError(ErrCodeLLMUnavailable, "service down", nil)
		info := err.ToErrorInfo()

		if info.Code != string(ErrCodeLLMUnavailable) {
			t.Errorf("Expected code %s, got %s", ErrCodeLLMUnavailable, info.Code)
		}
		if info.Message != "service down" {
			t.Errorf("Expected message 'service down', got '%s'", info.Message)
		}
		if !info.Recoverable {
			t.Error("Expected recoverable")
		}
	})
}

// Serialization Tests

func TestToErrorInfoConversion(t *testing.T) {
	t.Run("Auth error with all fields", func(t *testing.T) {
		cause := errors.New("token expired")
		err := NewAuthError(ErrCodeExpiredToken, "Your session has expired", cause)

		info := err.ToErrorInfo()

		if info.Code != string(ErrCodeExpiredToken) {
			t.Errorf("Expected code %s, got %s", ErrCodeExpiredToken, info.Code)
		}
		if info.Message != "Your session has expired" {
			t.Errorf("Expected message 'Your session has expired', got '%s'", info.Message)
		}
		if info.Recoverable {
			t.Error("Expected non-recoverable for auth error")
		}
		if info.RetryAfter != 0 {
			t.Errorf("Expected RetryAfter 0, got %d", info.RetryAfter)
		}
	})

	t.Run("Rate limit error with retry after", func(t *testing.T) {
		retryAfter := 30000
		err := NewRateLimitError(ErrCodeTooManyRequests, "Rate limit exceeded", retryAfter, nil)

		info := err.ToErrorInfo()

		if info.Code != string(ErrCodeTooManyRequests) {
			t.Errorf("Expected code %s, got %s", ErrCodeTooManyRequests, info.Code)
		}
		if info.Message != "Rate limit exceeded" {
			t.Errorf("Expected message 'Rate limit exceeded', got '%s'", info.Message)
		}
		if !info.Recoverable {
			t.Error("Expected recoverable for rate limit error")
		}
		if info.RetryAfter != retryAfter {
			t.Errorf("Expected RetryAfter %d, got %d", retryAfter, info.RetryAfter)
		}
	})

	t.Run("Validation error without retry after", func(t *testing.T) {
		err := NewValidationError(ErrCodeInvalidFormat, "Invalid JSON format", nil)

		info := err.ToErrorInfo()

		if info.Code != string(ErrCodeInvalidFormat) {
			t.Errorf("Expected code %s, got %s", ErrCodeInvalidFormat, info.Code)
		}
		if info.Message != "Invalid JSON format" {
			t.Errorf("Expected message 'Invalid JSON format', got '%s'", info.Message)
		}
		if !info.Recoverable {
			t.Error("Expected recoverable for validation error")
		}
		if info.RetryAfter != 0 {
			t.Errorf("Expected RetryAfter 0, got %d", info.RetryAfter)
		}
	})

	t.Run("Service error conversion", func(t *testing.T) {
		cause := errors.New("connection timeout")
		err := NewServiceError(ErrCodeLLMTimeout, "LLM request timed out", cause)

		info := err.ToErrorInfo()

		if info.Code != string(ErrCodeLLMTimeout) {
			t.Errorf("Expected code %s, got %s", ErrCodeLLMTimeout, info.Code)
		}
		if info.Message != "LLM request timed out" {
			t.Errorf("Expected message 'LLM request timed out', got '%s'", info.Message)
		}
		if !info.Recoverable {
			t.Error("Expected recoverable for service error")
		}
	})
}

func TestJSONMarshaling(t *testing.T) {
	t.Run("Marshal auth error", func(t *testing.T) {
		err := NewAuthError(ErrCodeInvalidToken, "Token is invalid", nil)
		info := err.ToErrorInfo()

		data, marshalErr := json.Marshal(info)
		if marshalErr != nil {
			t.Fatalf("Failed to marshal ErrorInfo: %v", marshalErr)
		}

		// Verify JSON contains expected fields
		jsonStr := string(data)
		if !contains(jsonStr, `"code":"INVALID_TOKEN"`) {
			t.Errorf("Expected JSON to contain code field, got: %s", jsonStr)
		}
		if !contains(jsonStr, `"message":"Token is invalid"`) {
			t.Errorf("Expected JSON to contain message field, got: %s", jsonStr)
		}
		if !contains(jsonStr, `"recoverable":false`) {
			t.Errorf("Expected JSON to contain recoverable:false, got: %s", jsonStr)
		}
	})

	t.Run("Marshal rate limit error with retry after", func(t *testing.T) {
		err := NewRateLimitError(ErrCodeTooManyRequests, "Too many requests", 5000, nil)
		info := err.ToErrorInfo()

		data, marshalErr := json.Marshal(info)
		if marshalErr != nil {
			t.Fatalf("Failed to marshal ErrorInfo: %v", marshalErr)
		}

		jsonStr := string(data)
		if !contains(jsonStr, `"code":"TOO_MANY_REQUESTS"`) {
			t.Errorf("Expected JSON to contain code field, got: %s", jsonStr)
		}
		if !contains(jsonStr, `"message":"Too many requests"`) {
			t.Errorf("Expected JSON to contain message field, got: %s", jsonStr)
		}
		if !contains(jsonStr, `"recoverable":true`) {
			t.Errorf("Expected JSON to contain recoverable:true, got: %s", jsonStr)
		}
		if !contains(jsonStr, `"retry_after":5000`) {
			t.Errorf("Expected JSON to contain retry_after field, got: %s", jsonStr)
		}
	})

	t.Run("Marshal validation error without retry after", func(t *testing.T) {
		err := NewValidationError(ErrCodeMissingField, "Field required", nil)
		info := err.ToErrorInfo()

		data, marshalErr := json.Marshal(info)
		if marshalErr != nil {
			t.Fatalf("Failed to marshal ErrorInfo: %v", marshalErr)
		}

		jsonStr := string(data)
		if !contains(jsonStr, `"code":"MISSING_FIELD"`) {
			t.Errorf("Expected JSON to contain code field, got: %s", jsonStr)
		}
		if !contains(jsonStr, `"recoverable":true`) {
			t.Errorf("Expected JSON to contain recoverable:true, got: %s", jsonStr)
		}
		// retry_after should be omitted when 0 due to omitempty tag
		if contains(jsonStr, `"retry_after"`) && !contains(jsonStr, `"retry_after":0`) {
			// If retry_after appears, it should be 0 or omitted
			t.Logf("Note: retry_after field present in JSON: %s", jsonStr)
		}
	})

	t.Run("Unmarshal JSON to ErrorInfo", func(t *testing.T) {
		jsonData := `{"code":"DATABASE_ERROR","message":"Connection failed","recoverable":true,"retry_after":2000}`

		var info message.ErrorInfo
		err := json.Unmarshal([]byte(jsonData), &info)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		if info.Code != "DATABASE_ERROR" {
			t.Errorf("Expected code DATABASE_ERROR, got %s", info.Code)
		}
		if info.Message != "Connection failed" {
			t.Errorf("Expected message 'Connection failed', got '%s'", info.Message)
		}
		if !info.Recoverable {
			t.Error("Expected recoverable to be true")
		}
		if info.RetryAfter != 2000 {
			t.Errorf("Expected retry_after 2000, got %d", info.RetryAfter)
		}
	})

	t.Run("Round-trip marshal and unmarshal", func(t *testing.T) {
		original := NewRateLimitError(ErrCodeConnectionLimit, "Connection limit reached", 10000, nil)
		originalInfo := original.ToErrorInfo()

		// Marshal
		data, err := json.Marshal(originalInfo)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		// Unmarshal
		var decoded message.ErrorInfo
		err = json.Unmarshal(data, &decoded)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		// Compare
		if decoded.Code != originalInfo.Code {
			t.Errorf("Code mismatch: expected %s, got %s", originalInfo.Code, decoded.Code)
		}
		if decoded.Message != originalInfo.Message {
			t.Errorf("Message mismatch: expected %s, got %s", originalInfo.Message, decoded.Message)
		}
		if decoded.Recoverable != originalInfo.Recoverable {
			t.Errorf("Recoverable mismatch: expected %v, got %v", originalInfo.Recoverable, decoded.Recoverable)
		}
		if decoded.RetryAfter != originalInfo.RetryAfter {
			t.Errorf("RetryAfter mismatch: expected %d, got %d", originalInfo.RetryAfter, decoded.RetryAfter)
		}
	})
}

func TestErrorWrapping(t *testing.T) {
	t.Run("Wrap error with cause", func(t *testing.T) {
		rootCause := errors.New("network timeout")
		err := NewServiceError(ErrCodeDatabaseError, "Database connection failed", rootCause)

		if err.Cause != rootCause {
			t.Error("Expected cause to be preserved")
		}

		unwrapped := err.Unwrap()
		if unwrapped != rootCause {
			t.Error("Expected Unwrap to return root cause")
		}
	})

	t.Run("Error string includes cause", func(t *testing.T) {
		rootCause := errors.New("connection refused")
		err := NewServiceError(ErrCodeLLMUnavailable, "LLM service unavailable", rootCause)

		errStr := err.Error()
		if !contains(errStr, "LLM_UNAVAILABLE") {
			t.Errorf("Expected error string to contain code, got: %s", errStr)
		}
		if !contains(errStr, "LLM service unavailable") {
			t.Errorf("Expected error string to contain message, got: %s", errStr)
		}
		if !contains(errStr, "connection refused") {
			t.Errorf("Expected error string to contain cause, got: %s", errStr)
		}
		if !contains(errStr, "caused by") {
			t.Errorf("Expected error string to contain 'caused by', got: %s", errStr)
		}
	})

	t.Run("Error without cause", func(t *testing.T) {
		err := NewValidationError(ErrCodeMissingField, "Username is required", nil)

		if err.Cause != nil {
			t.Error("Expected cause to be nil")
		}

		unwrapped := err.Unwrap()
		if unwrapped != nil {
			t.Error("Expected Unwrap to return nil")
		}

		errStr := err.Error()
		if contains(errStr, "caused by") {
			t.Errorf("Expected error string to not contain 'caused by', got: %s", errStr)
		}
	})

	t.Run("Nested error wrapping", func(t *testing.T) {
		rootCause := errors.New("disk full")
		wrappedCause := fmt.Errorf("write failed: %w", rootCause)
		err := NewServiceError(ErrCodeStorageError, "Failed to save file", wrappedCause)

		// Test direct unwrap
		if err.Unwrap() != wrappedCause {
			t.Error("Expected first Unwrap to return wrapped cause")
		}

		// Test errors.Is with root cause
		if !errors.Is(err, wrappedCause) {
			t.Error("Expected errors.Is to find wrapped cause")
		}
	})

	t.Run("Convenience constructors preserve cause", func(t *testing.T) {
		rootCause := errors.New("jwt parse error")
		err := ErrInvalidToken(rootCause)

		if err.Cause != rootCause {
			t.Error("Expected convenience constructor to preserve cause")
		}
		if err.Unwrap() != rootCause {
			t.Error("Expected Unwrap to return root cause")
		}
	})

	t.Run("ToErrorInfo does not expose cause", func(t *testing.T) {
		rootCause := errors.New("internal database error")
		err := NewServiceError(ErrCodeDatabaseError, "Database operation failed", rootCause)

		info := err.ToErrorInfo()

		// ErrorInfo should not expose internal error details
		if contains(info.Message, "internal database error") {
			t.Error("ErrorInfo should not expose internal cause details")
		}
		if info.Message != "Database operation failed" {
			t.Errorf("Expected sanitized message, got: %s", info.Message)
		}
	})
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
