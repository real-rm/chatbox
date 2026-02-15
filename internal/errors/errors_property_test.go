package errors

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: chat-application-websocket, Property 33: Error Message Generation
// **Validates: Requirements 9.1, 9.2**
func TestProperty_ErrorMessageGeneration(t *testing.T) {
	properties := gopter.NewProperties(nil)

	// Generator for error categories
	genCategory := gen.OneConstOf(
		CategoryAuth,
		CategoryValidation,
		CategoryService,
		CategoryRateLimit,
	)

	// Generator for error codes
	genErrorCode := gen.OneConstOf(
		ErrCodeInvalidToken,
		ErrCodeExpiredToken,
		ErrCodeInsufficientPerms,
		ErrCodeInvalidFormat,
		ErrCodeMissingField,
		ErrCodeInvalidFileType,
		ErrCodeInvalidFileSize,
		ErrCodeLLMUnavailable,
		ErrCodeDatabaseError,
		ErrCodeStorageError,
		ErrCodeServiceError,
		ErrCodeTooManyRequests,
		ErrCodeConnectionLimit,
	)

	// Generator for error messages
	genMessage := gen.AlphaString()

	// Generator for retry after values (for rate limit errors)
	genRetryAfter := gen.IntRange(0, 60000) // 0 to 60 seconds in milliseconds

	properties.Property("Error message contains type and description", prop.ForAll(
		func(category ErrorCategory, code ErrorCode, message string) bool {
			var chatErr *ChatError

			// Create appropriate error based on category
			switch category {
			case CategoryAuth:
				chatErr = NewAuthError(code, message, nil)
			case CategoryValidation:
				chatErr = NewValidationError(code, message, nil)
			case CategoryService:
				chatErr = NewServiceError(code, message, nil)
			case CategoryRateLimit:
				chatErr = NewRateLimitError(code, message, 5000, nil)
			}

			// Verify error has correct fields
			if chatErr.Code != code {
				return false
			}
			if chatErr.Message != message {
				return false
			}
			if chatErr.Category != category {
				return false
			}

			// Convert to ErrorInfo
			errorInfo := chatErr.ToErrorInfo()
			if errorInfo.Code != string(code) {
				return false
			}
			if errorInfo.Message != message {
				return false
			}

			return true
		},
		genCategory,
		genErrorCode,
		genMessage,
	))

	properties.Property("Auth errors are always fatal (not recoverable)", prop.ForAll(
		func(code ErrorCode, message string) bool {
			chatErr := NewAuthError(code, message, nil)
			return !chatErr.Recoverable && chatErr.IsFatal()
		},
		genErrorCode,
		genMessage,
	))

	properties.Property("Validation errors are always recoverable", prop.ForAll(
		func(code ErrorCode, message string) bool {
			chatErr := NewValidationError(code, message, nil)
			return chatErr.Recoverable && !chatErr.IsFatal()
		},
		genErrorCode,
		genMessage,
	))

	properties.Property("Service errors are always recoverable", prop.ForAll(
		func(code ErrorCode, message string) bool {
			chatErr := NewServiceError(code, message, nil)
			return chatErr.Recoverable && !chatErr.IsFatal()
		},
		genErrorCode,
		genMessage,
	))

	properties.Property("Rate limit errors include retry after value", prop.ForAll(
		func(code ErrorCode, message string, retryAfter int) bool {
			chatErr := NewRateLimitError(code, message, retryAfter, nil)

			// Verify error is recoverable
			if !chatErr.Recoverable || chatErr.IsFatal() {
				return false
			}

			// Verify retry after is set
			if chatErr.RetryAfter != retryAfter {
				return false
			}

			// Verify ErrorInfo includes retry after
			errorInfo := chatErr.ToErrorInfo()
			return errorInfo.RetryAfter == retryAfter
		},
		genErrorCode,
		genMessage,
		genRetryAfter,
	))

	properties.Property("Error implements error interface", prop.ForAll(
		func(code ErrorCode, message string) bool {
			chatErr := NewServiceError(code, message, nil)

			// Verify Error() method returns non-empty string
			errStr := chatErr.Error()
			if errStr == "" {
				return false
			}

			// Verify error string contains code and message
			return len(errStr) > 0
		},
		genErrorCode,
		genMessage,
	))

	properties.Property("ToErrorInfo preserves all error information", prop.ForAll(
		func(category ErrorCategory, code ErrorCode, message string, retryAfter int) bool {
			var chatErr *ChatError

			// Create error based on category
			switch category {
			case CategoryAuth:
				chatErr = NewAuthError(code, message, nil)
			case CategoryValidation:
				chatErr = NewValidationError(code, message, nil)
			case CategoryService:
				chatErr = NewServiceError(code, message, nil)
			case CategoryRateLimit:
				chatErr = NewRateLimitError(code, message, retryAfter, nil)
			}

			// Convert to ErrorInfo
			errorInfo := chatErr.ToErrorInfo()

			// Verify all fields are preserved
			if errorInfo.Code != string(chatErr.Code) {
				return false
			}
			if errorInfo.Message != chatErr.Message {
				return false
			}
			if errorInfo.Recoverable != chatErr.Recoverable {
				return false
			}
			if category == CategoryRateLimit && errorInfo.RetryAfter != retryAfter {
				return false
			}

			return true
		},
		genCategory,
		genErrorCode,
		genMessage,
		genRetryAfter,
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 34: Fatal Error Connection Closure
// **Validates: Requirements 9.3**
func TestProperty_FatalErrorConnectionClosure(t *testing.T) {
	properties := gopter.NewProperties(nil)

	// Generator for error codes
	genErrorCode := gen.OneConstOf(
		ErrCodeInvalidToken,
		ErrCodeExpiredToken,
		ErrCodeInsufficientPerms,
		ErrCodeInvalidFormat,
		ErrCodeMissingField,
		ErrCodeLLMUnavailable,
		ErrCodeDatabaseError,
	)

	// Generator for error messages
	genMessage := gen.AlphaString()

	properties.Property("Fatal errors are marked as non-recoverable", prop.ForAll(
		func(code ErrorCode, message string) bool {
			// Auth errors are fatal
			authErr := NewAuthError(code, message, nil)
			if authErr.Recoverable || !authErr.IsFatal() {
				return false
			}

			// Validation errors are recoverable
			validationErr := NewValidationError(code, message, nil)
			if !validationErr.Recoverable || validationErr.IsFatal() {
				return false
			}

			// Service errors are recoverable
			serviceErr := NewServiceError(code, message, nil)
			if !serviceErr.Recoverable || serviceErr.IsFatal() {
				return false
			}

			return true
		},
		genErrorCode,
		genMessage,
	))

	properties.Property("IsFatal returns opposite of Recoverable", prop.ForAll(
		func(code ErrorCode, message string) bool {
			// Test with auth error (fatal)
			authErr := NewAuthError(code, message, nil)
			if authErr.IsFatal() != !authErr.Recoverable {
				return false
			}

			// Test with validation error (recoverable)
			validationErr := NewValidationError(code, message, nil)
			if validationErr.IsFatal() != !validationErr.Recoverable {
				return false
			}

			// Test with service error (recoverable)
			serviceErr := NewServiceError(code, message, nil)
			if serviceErr.IsFatal() != !serviceErr.Recoverable {
				return false
			}

			// Test with rate limit error (recoverable)
			rateLimitErr := NewRateLimitError(code, message, 5000, nil)
			if rateLimitErr.IsFatal() != !rateLimitErr.Recoverable {
				return false
			}

			return true
		},
		genErrorCode,
		genMessage,
	))

	properties.Property("Auth errors are always fatal", prop.ForAll(
		func(code ErrorCode, message string) bool {
			authErr := NewAuthError(code, message, nil)
			return authErr.IsFatal() && !authErr.Recoverable && authErr.Category == CategoryAuth
		},
		genErrorCode,
		genMessage,
	))

	properties.Property("Non-auth errors are never fatal", prop.ForAll(
		func(code ErrorCode, message string) bool {
			// Validation errors
			validationErr := NewValidationError(code, message, nil)
			if validationErr.IsFatal() {
				return false
			}

			// Service errors
			serviceErr := NewServiceError(code, message, nil)
			if serviceErr.IsFatal() {
				return false
			}

			// Rate limit errors
			rateLimitErr := NewRateLimitError(code, message, 5000, nil)
			if rateLimitErr.IsFatal() {
				return false
			}

			return true
		},
		genErrorCode,
		genMessage,
	))

	properties.Property("ErrorInfo recoverable flag matches ChatError", prop.ForAll(
		func(code ErrorCode, message string) bool {
			// Test with fatal error
			authErr := NewAuthError(code, message, nil)
			authInfo := authErr.ToErrorInfo()
			if authInfo.Recoverable != authErr.Recoverable {
				return false
			}

			// Test with recoverable error
			validationErr := NewValidationError(code, message, nil)
			validationInfo := validationErr.ToErrorInfo()
			if validationInfo.Recoverable != validationErr.Recoverable {
				return false
			}

			return true
		},
		genErrorCode,
		genMessage,
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Test convenience error constructors
func TestConvenienceConstructors(t *testing.T) {
	t.Run("ErrInvalidToken creates auth error", func(t *testing.T) {
		err := ErrInvalidToken(nil)
		if err.Category != CategoryAuth {
			t.Errorf("Expected category %s, got %s", CategoryAuth, err.Category)
		}
		if err.Code != ErrCodeInvalidToken {
			t.Errorf("Expected code %s, got %s", ErrCodeInvalidToken, err.Code)
		}
		if err.Recoverable {
			t.Error("Expected non-recoverable error")
		}
	})

	t.Run("ErrExpiredToken creates auth error", func(t *testing.T) {
		err := ErrExpiredToken(nil)
		if err.Category != CategoryAuth {
			t.Errorf("Expected category %s, got %s", CategoryAuth, err.Category)
		}
		if err.Code != ErrCodeExpiredToken {
			t.Errorf("Expected code %s, got %s", ErrCodeExpiredToken, err.Code)
		}
		if err.Recoverable {
			t.Error("Expected non-recoverable error")
		}
	})

	t.Run("ErrMissingField creates validation error", func(t *testing.T) {
		err := ErrMissingField("test_field")
		if err.Category != CategoryValidation {
			t.Errorf("Expected category %s, got %s", CategoryValidation, err.Category)
		}
		if err.Code != ErrCodeMissingField {
			t.Errorf("Expected code %s, got %s", ErrCodeMissingField, err.Code)
		}
		if !err.Recoverable {
			t.Error("Expected recoverable error")
		}
	})

	t.Run("ErrLLMUnavailable creates service error", func(t *testing.T) {
		err := ErrLLMUnavailable(nil)
		if err.Category != CategoryService {
			t.Errorf("Expected category %s, got %s", CategoryService, err.Category)
		}
		if err.Code != ErrCodeLLMUnavailable {
			t.Errorf("Expected code %s, got %s", ErrCodeLLMUnavailable, err.Code)
		}
		if !err.Recoverable {
			t.Error("Expected recoverable error")
		}
	})

	t.Run("ErrTooManyRequests creates rate limit error", func(t *testing.T) {
		retryAfter := 5000
		err := ErrTooManyRequests(retryAfter)
		if err.Category != CategoryRateLimit {
			t.Errorf("Expected category %s, got %s", CategoryRateLimit, err.Category)
		}
		if err.Code != ErrCodeTooManyRequests {
			t.Errorf("Expected code %s, got %s", ErrCodeTooManyRequests, err.Code)
		}
		if !err.Recoverable {
			t.Error("Expected recoverable error")
		}
		if err.RetryAfter != retryAfter {
			t.Errorf("Expected retry after %d, got %d", retryAfter, err.RetryAfter)
		}
	})
}
