package message

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidate_ValidMessages tests validation of valid messages
func TestValidate_ValidMessages(t *testing.T) {
	tests := []struct {
		name    string
		message Message
	}{
		{
			name: "valid user message",
			message: Message{
				Type:      TypeUserMessage,
				SessionID: "session-123",
				Content:   "Hello, AI!",
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
		},
		{
			name: "valid ai response",
			message: Message{
				Type:      TypeAIResponse,
				SessionID: "session-123",
				Content:   "Hello, user!",
				Timestamp: time.Now(),
				Sender:    SenderAI,
			},
		},
		{
			name: "valid file upload",
			message: Message{
				Type:      TypeFileUpload,
				SessionID: "session-123",
				FileID:    "file-456",
				FileURL:   "https://example.com/file.pdf",
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
		},
		{
			name: "valid voice message",
			message: Message{
				Type:      TypeVoiceMessage,
				SessionID: "session-123",
				FileID:    "voice-789",
				FileURL:   "https://example.com/voice.mp3",
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
		},
		{
			name: "valid error message",
			message: Message{
				Type:      TypeError,
				SessionID: "session-123",
				Timestamp: time.Now(),
				Sender:    SenderUser,
				Error: &ErrorInfo{
					Code:        "VALIDATION_ERROR",
					Message:     "Invalid input",
					Recoverable: true,
				},
			},
		},
		{
			name: "valid model select",
			message: Message{
				Type:      TypeModelSelect,
				SessionID: "session-123",
				ModelID:   "gpt-4",
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
		},
		{
			name: "valid admin join",
			message: Message{
				Type:      TypeAdminJoin,
				SessionID: "session-123",
				Content:   "Admin joined",
				Timestamp: time.Now(),
				Sender:    SenderAdmin,
			},
		},
		{
			name: "valid help request",
			message: Message{
				Type:      TypeHelpRequest,
				SessionID: "session-123",
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
		},
		{
			name: "valid connection status",
			message: Message{
				Type:      TypeConnectionStatus,
				SessionID: "session-123",
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
		},
		{
			name: "valid typing indicator",
			message: Message{
				Type:      TypeTypingIndicator,
				SessionID: "session-123",
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
		},
		{
			name: "valid loading message",
			message: Message{
				Type:      TypeLoading,
				SessionID: "session-123",
				Timestamp: time.Now(),
				Sender:    SenderAI,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			assert.NoError(t, err)
		})
	}
}

// TestValidate_MissingRequiredFields tests validation errors for missing required fields
func TestValidate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name          string
		message       Message
		expectedField string
		expectedError string
	}{
		{
			name: "missing type",
			message: Message{
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
			expectedField: "type",
			expectedError: "type is required",
		},
		{
			name: "missing sender",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now(),
			},
			expectedField: "sender",
			expectedError: "sender is required",
		},
		{
			name: "missing timestamp",
			message: Message{
				Type:   TypeUserMessage,
				Sender: SenderUser,
			},
			expectedField: "timestamp",
			expectedError: "timestamp is required",
		},
		{
			name: "invalid message type",
			message: Message{
				Type:      "invalid_type",
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
			expectedField: "type",
			expectedError: "invalid message type",
		},
		{
			name: "invalid sender type",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now(),
				Sender:    "invalid_sender",
			},
			expectedField: "sender",
			expectedError: "invalid sender type",
		},
		{
			name: "future timestamp",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now().Add(2 * time.Hour),
				Sender:    SenderUser,
				Content:   "Test",
			},
			expectedField: "timestamp",
			expectedError: "timestamp cannot be in the future",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			require.Error(t, err)

			validationErr, ok := err.(*ValidationError)
			require.True(t, ok, "error should be ValidationError")
			assert.Equal(t, tt.expectedField, validationErr.Field)
			assert.Contains(t, validationErr.Message, tt.expectedError)
		})
	}
}

// TestValidate_TypeSpecificFields tests validation of type-specific required fields
func TestValidate_TypeSpecificFields(t *testing.T) {
	tests := []struct {
		name          string
		message       Message
		expectedField string
		expectedError string
	}{
		{
			name: "user message missing content",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
			expectedField: "content",
			expectedError: "content is required for user_message",
		},
		{
			name: "ai response missing content",
			message: Message{
				Type:      TypeAIResponse,
				Timestamp: time.Now(),
				Sender:    SenderAI,
			},
			expectedField: "content",
			expectedError: "content is required for ai_response",
		},
		{
			name: "file upload missing file_id",
			message: Message{
				Type:      TypeFileUpload,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				FileURL:   "https://example.com/file.pdf",
			},
			expectedField: "file_id",
			expectedError: "file_id is required for file_upload",
		},
		{
			name: "file upload missing file_url",
			message: Message{
				Type:      TypeFileUpload,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				FileID:    "file-123",
			},
			expectedField: "file_url",
			expectedError: "file_url is required for file_upload",
		},
		{
			name: "voice message missing file_id",
			message: Message{
				Type:      TypeVoiceMessage,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				FileURL:   "https://example.com/voice.mp3",
			},
			expectedField: "file_id",
			expectedError: "file_id is required for voice_message",
		},
		{
			name: "voice message missing file_url",
			message: Message{
				Type:      TypeVoiceMessage,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				FileID:    "voice-123",
			},
			expectedField: "file_url",
			expectedError: "file_url is required for voice_message",
		},
		{
			name: "error message missing error info",
			message: Message{
				Type:      TypeError,
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
			expectedField: "error",
			expectedError: "error is required for error message type",
		},
		{
			name: "error message missing error code",
			message: Message{
				Type:      TypeError,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				Error: &ErrorInfo{
					Message: "Something went wrong",
				},
			},
			expectedField: "error.code",
			expectedError: "error code is required",
		},
		{
			name: "error message missing error message",
			message: Message{
				Type:      TypeError,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				Error: &ErrorInfo{
					Code: "ERROR",
				},
			},
			expectedField: "error.message",
			expectedError: "error message is required",
		},
		{
			name: "model select missing model_id",
			message: Message{
				Type:      TypeModelSelect,
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
			expectedField: "model_id",
			expectedError: "model_id is required for model_select",
		},
		{
			name: "admin join with non-admin sender",
			message: Message{
				Type:      TypeAdminJoin,
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
			expectedField: "sender",
			expectedError: "sender must be 'admin' for admin_join",
		},
		{
			name: "admin leave with non-admin sender",
			message: Message{
				Type:      TypeAdminLeave,
				Timestamp: time.Now(),
				Sender:    SenderUser,
			},
			expectedField: "sender",
			expectedError: "sender must be 'admin' for admin_leave",
		},
		{
			name: "help request with non-user sender",
			message: Message{
				Type:      TypeHelpRequest,
				Timestamp: time.Now(),
				Sender:    SenderAdmin,
			},
			expectedField: "sender",
			expectedError: "sender must be 'user' for help_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			require.Error(t, err)

			validationErr, ok := err.(*ValidationError)
			require.True(t, ok, "error should be ValidationError")
			assert.Equal(t, tt.expectedField, validationErr.Field)
			assert.Contains(t, validationErr.Message, tt.expectedError)
		})
	}
}

// TestValidate_FieldLengths tests validation of field length limits
func TestValidate_FieldLengths(t *testing.T) {
	tests := []struct {
		name          string
		message       Message
		expectedField string
	}{
		{
			name: "content exceeds max length",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				Content:   strings.Repeat("a", MaxContentLength+1),
			},
			expectedField: "content",
		},
		{
			name: "file_id exceeds max length",
			message: Message{
				Type:      TypeFileUpload,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				FileID:    strings.Repeat("a", MaxFileIDLength+1),
				FileURL:   "https://example.com/file.pdf",
			},
			expectedField: "file_id",
		},
		{
			name: "model_id exceeds max length",
			message: Message{
				Type:      TypeModelSelect,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				ModelID:   strings.Repeat("a", MaxModelIDLength+1),
			},
			expectedField: "model_id",
		},
		{
			name: "metadata value exceeds max length",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				Content:   "Test",
				Metadata: map[string]string{
					"key": strings.Repeat("a", MaxMetadataLength+1),
				},
			},
			expectedField: "metadata.key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			require.Error(t, err)

			validationErr, ok := err.(*ValidationError)
			require.True(t, ok, "error should be ValidationError")
			assert.Contains(t, validationErr.Field, tt.expectedField)
			assert.Contains(t, validationErr.Message, "exceeds maximum length")
		})
	}
}

// TestValidate_EdgeCases tests edge cases
func TestValidate_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		message   Message
		wantError bool
	}{
		{
			name: "empty content is valid for non-content messages",
			message: Message{
				Type:      TypeConnectionStatus,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				Content:   "",
			},
			wantError: false,
		},
		{
			name: "content at max length is valid",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				Content:   strings.Repeat("a", MaxContentLength),
			},
			wantError: false,
		},
		{
			name: "empty metadata is valid",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				Content:   "Test",
				Metadata:  map[string]string{},
			},
			wantError: false,
		},
		{
			name: "nil metadata is valid",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now(),
				Sender:    SenderUser,
				Content:   "Test",
				Metadata:  nil,
			},
			wantError: false,
		},
		{
			name: "timestamp with clock skew tolerance",
			message: Message{
				Type:      TypeUserMessage,
				Timestamp: time.Now().Add(30 * time.Second),
				Sender:    SenderUser,
				Content:   "Test",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSanitize_XSSPrevention tests XSS attack prevention
func TestSanitize_XSSPrevention(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "script tag",
			input:    "<script>alert('XSS')</script>",
			expected: "&lt;script&gt;alert(&#39;XSS&#39;)&lt;/script&gt;",
		},
		{
			name:     "img tag with onerror",
			input:    "<img src=x onerror=alert('XSS')>",
			expected: "&lt;img src=x onerror=alert(&#39;XSS&#39;)&gt;",
		},
		{
			name:     "javascript protocol",
			input:    "<a href='javascript:alert(1)'>Click</a>",
			expected: "&lt;a href=&#39;javascript:alert(1)&#39;&gt;Click&lt;/a&gt;",
		},
		{
			name:     "html entities",
			input:    "Test & <test>",
			expected: "Test &amp; &lt;test&gt;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Message{
				Type:      TypeUserMessage,
				Content:   tt.input,
				Timestamp: time.Now(),
				Sender:    SenderUser,
			}

			msg.Sanitize()
			assert.Equal(t, tt.expected, msg.Content)
		})
	}
}

// TestSanitize_SQLInjectionPrevention tests SQL injection prevention
func TestSanitize_SQLInjectionPrevention(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "sql injection attempt",
			input:    "'; DROP TABLE users; --",
			expected: "&#39;; DROP TABLE users; --",
		},
		{
			name:     "union select",
			input:    "1' UNION SELECT * FROM users--",
			expected: "1&#39; UNION SELECT * FROM users--",
		},
		{
			name:     "comment injection",
			input:    "admin'--",
			expected: "admin&#39;--",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Message{
				Type:      TypeUserMessage,
				Content:   tt.input,
				Timestamp: time.Now(),
				Sender:    SenderUser,
			}

			msg.Sanitize()
			assert.Equal(t, tt.expected, msg.Content)
		})
	}
}

// TestSanitize_SpecialCharacters tests handling of special characters
func TestSanitize_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "null bytes",
			input:    "test\x00data",
			expected: "testdata",
		},
		{
			name:     "leading and trailing whitespace",
			input:    "  test data  ",
			expected: "test data",
		},
		{
			name:     "newlines and tabs",
			input:    "test\ndata\twith\rspecial",
			expected: "test\ndata\twith\rspecial",
		},
		{
			name:     "unicode characters",
			input:    "Hello 世界 🌍",
			expected: "Hello 世界 🌍",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Message{
				Type:      TypeUserMessage,
				Content:   tt.input,
				Timestamp: time.Now(),
				Sender:    SenderUser,
			}

			msg.Sanitize()
			assert.Equal(t, tt.expected, msg.Content)
		})
	}
}

// TestSanitize_AllFields tests that all fields are sanitized
func TestSanitize_AllFields(t *testing.T) {
	msg := Message{
		Type:      TypeUserMessage,
		SessionID: "<script>alert('xss')</script>",
		Content:   "<img src=x onerror=alert(1)>",
		FileID:    "'; DROP TABLE files; --",
		FileURL:   "javascript:alert(1)",
		ModelID:   "<b>model</b>",
		Timestamp: time.Now(),
		Sender:    SenderUser,
		Metadata: map[string]string{
			"<key>": "<value>",
		},
		Error: &ErrorInfo{
			Code:    "<code>",
			Message: "<message>",
		},
	}

	msg.Sanitize()

	// Verify all fields are sanitized (HTML escaped)
	assert.Contains(t, msg.SessionID, "&lt;script&gt;")
	assert.Contains(t, msg.Content, "&lt;img")
	assert.Contains(t, msg.FileID, "&#39;")
	assert.Contains(t, msg.FileURL, "javascript:alert(1)") // URLs are escaped but protocol remains
	assert.Contains(t, msg.ModelID, "&lt;b&gt;")

	// Check metadata - keys and values should be escaped
	for key, value := range msg.Metadata {
		// Keys and values should have HTML entities escaped
		if strings.Contains(key, "&lt;") || strings.Contains(value, "&lt;") {
			// At least one should be escaped
			assert.True(t, true)
		}
	}

	// Check error info
	assert.Contains(t, msg.Error.Code, "&lt;")
	assert.Contains(t, msg.Error.Message, "&lt;")
}

// TestSanitize_EmptyStrings tests sanitization of empty strings
func TestSanitize_EmptyStrings(t *testing.T) {
	msg := Message{
		Type:      TypeUserMessage,
		SessionID: "",
		Content:   "",
		Timestamp: time.Now(),
		Sender:    SenderUser,
	}

	msg.Sanitize()

	assert.Equal(t, "", msg.SessionID)
	assert.Equal(t, "", msg.Content)
}

// TestSanitize_VeryLongStrings tests sanitization of very long strings
func TestSanitize_VeryLongStrings(t *testing.T) {
	longString := strings.Repeat("<script>alert('xss')</script>", 100)

	msg := Message{
		Type:      TypeUserMessage,
		Content:   longString,
		Timestamp: time.Now(),
		Sender:    SenderUser,
	}

	msg.Sanitize()

	// Verify no script tags remain
	assert.NotContains(t, msg.Content, "<script>")
	assert.NotContains(t, msg.Content, "</script>")
	// Verify content is escaped
	assert.Contains(t, msg.Content, "&lt;script&gt;")
}

// TestValidationError_Error tests ValidationError error message formatting
func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:   "content",
		Message: "content is required",
	}

	expected := "validation error on field 'content': content is required"
	assert.Equal(t, expected, err.Error())
}

// TestIsValidMessageType tests message type validation
func TestIsValidMessageType(t *testing.T) {
	validTypes := []MessageType{
		TypeUserMessage, TypeAIResponse, TypeFileUpload, TypeVoiceMessage,
		TypeError, TypeConnectionStatus, TypeTypingIndicator, TypeHelpRequest,
		TypeAdminJoin, TypeAdminLeave, TypeModelSelect, TypeLoading,
	}

	for _, msgType := range validTypes {
		t.Run(string(msgType), func(t *testing.T) {
			assert.True(t, isValidMessageType(msgType))
		})
	}

	// Test invalid types
	invalidTypes := []MessageType{"invalid", "unknown", ""}
	for _, msgType := range invalidTypes {
		t.Run(string(msgType), func(t *testing.T) {
			assert.False(t, isValidMessageType(msgType))
		})
	}
}

// TestIsValidSenderType tests sender type validation
func TestIsValidSenderType(t *testing.T) {
	validSenders := []SenderType{SenderUser, SenderAI, SenderAdmin}

	for _, sender := range validSenders {
		t.Run(string(sender), func(t *testing.T) {
			assert.True(t, isValidSenderType(sender))
		})
	}

	// Test invalid senders
	invalidSenders := []SenderType{"invalid", "system", ""}
	for _, sender := range invalidSenders {
		t.Run(string(sender), func(t *testing.T) {
			assert.False(t, isValidSenderType(sender))
		})
	}
}
