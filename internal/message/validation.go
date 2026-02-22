package message

import (
	"fmt"
	"strings"
	"time"
)

// Validation constants
const (
	MaxContentLength   = 10000 // Maximum content length in characters
	MaxMetadataLength  = 1000  // Maximum metadata value length
	MaxFileIDLength    = 255   // Maximum file ID length
	MaxFileURLLength   = 2048  // Maximum file URL length
	MaxModelIDLength   = 100   // Maximum model ID length
	MaxSessionIDLength = 128   // Maximum session ID length
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
}

// Validate validates a message according to the protocol specification
func (m *Message) Validate() error {
	// Validate required fields for all messages
	if err := m.validateRequiredFields(); err != nil {
		return err
	}

	// Validate message-type-specific required fields
	if err := m.validateTypeSpecificFields(); err != nil {
		return err
	}

	// Validate field lengths
	if err := m.validateFieldLengths(); err != nil {
		return err
	}

	return nil
}

// validateRequiredFields validates that all required fields are present
func (m *Message) validateRequiredFields() error {
	// Type is required
	if m.Type == "" {
		return &ValidationError{Field: "type", Message: "type is required"}
	}

	// Validate type is a known message type
	if !isValidMessageType(m.Type) {
		return &ValidationError{Field: "type", Message: fmt.Sprintf("invalid message type: %s", m.Type)}
	}

	// Sender is required
	if m.Sender == "" {
		return &ValidationError{Field: "sender", Message: "sender is required"}
	}

	// Validate sender is a known sender type
	if !isValidSenderType(m.Sender) {
		return &ValidationError{Field: "sender", Message: fmt.Sprintf("invalid sender type: %s", m.Sender)}
	}

	// Timestamp is required and must not be zero
	if m.Timestamp.IsZero() {
		return &ValidationError{Field: "timestamp", Message: "timestamp is required"}
	}

	// Timestamp should not be in the future (with 1 minute tolerance for clock skew)
	if m.Timestamp.After(time.Now().Add(1 * time.Minute)) {
		return &ValidationError{Field: "timestamp", Message: "timestamp cannot be in the future"}
	}

	return nil
}

// validateTypeSpecificFields validates required fields based on message type
func (m *Message) validateTypeSpecificFields() error {
	switch m.Type {
	case TypeUserMessage:
		if m.Content == "" {
			return &ValidationError{Field: "content", Message: "content is required for user_message"}
		}

	case TypeAIResponse:
		if m.Content == "" {
			return &ValidationError{Field: "content", Message: "content is required for ai_response"}
		}

	case TypeFileUpload:
		if m.FileID == "" {
			return &ValidationError{Field: "file_id", Message: "file_id is required for file_upload"}
		}
		if m.FileURL == "" {
			return &ValidationError{Field: "file_url", Message: "file_url is required for file_upload"}
		}

	case TypeVoiceMessage:
		if m.FileID == "" {
			return &ValidationError{Field: "file_id", Message: "file_id is required for voice_message"}
		}
		if m.FileURL == "" {
			return &ValidationError{Field: "file_url", Message: "file_url is required for voice_message"}
		}

	case TypeError:
		if m.Error == nil {
			return &ValidationError{Field: "error", Message: "error is required for error message type"}
		}
		if m.Error.Code == "" {
			return &ValidationError{Field: "error.code", Message: "error code is required"}
		}
		if m.Error.Message == "" {
			return &ValidationError{Field: "error.message", Message: "error message is required"}
		}

	case TypeModelSelect:
		if m.ModelID == "" {
			return &ValidationError{Field: "model_id", Message: "model_id is required for model_select"}
		}

	case TypeAdminJoin, TypeAdminLeave:
		// Admin messages should have admin sender
		if m.Sender != SenderAdmin {
			return &ValidationError{Field: "sender", Message: fmt.Sprintf("sender must be 'admin' for %s", m.Type)}
		}

	case TypeHelpRequest:
		// Help request should come from user
		if m.Sender != SenderUser {
			return &ValidationError{Field: "sender", Message: "sender must be 'user' for help_request"}
		}
	}

	return nil
}

// validateFieldLengths validates that field values don't exceed maximum lengths
func (m *Message) validateFieldLengths() error {
	if len(m.SessionID) > MaxSessionIDLength {
		return &ValidationError{
			Field:   "session_id",
			Message: fmt.Sprintf("session_id exceeds maximum length of %d characters", MaxSessionIDLength),
		}
	}

	if len(m.Content) > MaxContentLength {
		return &ValidationError{
			Field:   "content",
			Message: fmt.Sprintf("content exceeds maximum length of %d characters", MaxContentLength),
		}
	}

	if len(m.FileID) > MaxFileIDLength {
		return &ValidationError{
			Field:   "file_id",
			Message: fmt.Sprintf("file_id exceeds maximum length of %d characters", MaxFileIDLength),
		}
	}

	if len(m.FileURL) > MaxFileURLLength {
		return &ValidationError{
			Field:   "file_url",
			Message: fmt.Sprintf("file_url exceeds maximum length of %d characters", MaxFileURLLength),
		}
	}

	if len(m.ModelID) > MaxModelIDLength {
		return &ValidationError{
			Field:   "model_id",
			Message: fmt.Sprintf("model_id exceeds maximum length of %d characters", MaxModelIDLength),
		}
	}

	// Validate metadata lengths
	for key, value := range m.Metadata {
		if len(value) > MaxMetadataLength {
			return &ValidationError{
				Field:   fmt.Sprintf("metadata.%s", key),
				Message: fmt.Sprintf("metadata value exceeds maximum length of %d characters", MaxMetadataLength),
			}
		}
	}

	return nil
}

// Sanitize sanitizes user input to prevent injection attacks
func (m *Message) Sanitize() {
	// Sanitize content (HTML escape)
	m.Content = sanitizeString(m.Content)

	// Sanitize session ID
	m.SessionID = sanitizeString(m.SessionID)

	// Sanitize file ID and URL
	m.FileID = sanitizeString(m.FileID)
	m.FileURL = sanitizeString(m.FileURL)

	// Sanitize model ID
	m.ModelID = sanitizeString(m.ModelID)

	// Sanitize metadata
	if m.Metadata != nil {
		sanitizedMetadata := make(map[string]string)
		for key, value := range m.Metadata {
			sanitizedMetadata[sanitizeString(key)] = sanitizeString(value)
		}
		m.Metadata = sanitizedMetadata
	}

	// Sanitize error info if present
	if m.Error != nil {
		m.Error.Code = sanitizeString(m.Error.Code)
		m.Error.Message = sanitizeString(m.Error.Message)
	}
}

// sanitizeString sanitizes a string by removing null bytes and trimming whitespace.
// HTML escaping is NOT applied here — it belongs at render time only.
// Applying html.EscapeString at ingestion garbles content sent to the LLM
// (e.g., "<" becomes "&lt;"), degrading AI responses.
func sanitizeString(s string) string {
	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")

	// Trim whitespace
	s = strings.TrimSpace(s)

	return s
}

// isValidMessageType checks if the message type is valid
func isValidMessageType(t MessageType) bool {
	switch t {
	case TypeUserMessage, TypeAIResponse, TypeFileUpload, TypeVoiceMessage,
		TypeError, TypeConnectionStatus, TypeTypingIndicator, TypeHelpRequest,
		TypeAdminJoin, TypeAdminLeave, TypeModelSelect, TypeLoading,
		TypeNotification:
		return true
	default:
		return false
	}
}

// isValidSenderType checks if the sender type is valid
func isValidSenderType(s SenderType) bool {
	switch s {
	case SenderUser, SenderAI, SenderAdmin, SenderSystem:
		return true
	default:
		return false
	}
}
