package message

import (
	"encoding/json"
	"time"
)

// MessageType represents the type of WebSocket message
type MessageType string

const (
	TypeUserMessage       MessageType = "user_message"
	TypeAIResponse        MessageType = "ai_response"
	TypeFileUpload        MessageType = "file_upload"
	TypeVoiceMessage      MessageType = "voice_message"
	TypeError             MessageType = "error"
	TypeConnectionStatus  MessageType = "connection_status"
	TypeTypingIndicator   MessageType = "typing_indicator"
	TypeHelpRequest       MessageType = "help_request"
	TypeAdminJoin         MessageType = "admin_join"
	TypeAdminLeave        MessageType = "admin_leave"
	TypeModelSelect       MessageType = "model_select"
	TypeLoading           MessageType = "loading"
)

// SenderType represents who sent the message
type SenderType string

const (
	SenderUser  SenderType = "user"
	SenderAI    SenderType = "ai"
	SenderAdmin SenderType = "admin"
)

// ErrorInfo contains error details
type ErrorInfo struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	RetryAfter  int    `json:"retry_after,omitempty"` // milliseconds
}

// Message represents a WebSocket message
type Message struct {
	Type      MessageType       `json:"type"`
	SessionID string            `json:"session_id,omitempty"`
	Content   string            `json:"content,omitempty"`
	FileID    string            `json:"file_id,omitempty"`
	FileURL   string            `json:"file_url,omitempty"`
	ModelID   string            `json:"model_id,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Sender    SenderType        `json:"sender"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Error     *ErrorInfo        `json:"error,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for Message
func (m *Message) MarshalJSON() ([]byte, error) {
	type Alias Message
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"timestamp"`
	}{
		Alias:     (*Alias)(m),
		Timestamp: m.Timestamp.Format(time.RFC3339),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for Message
func (m *Message) UnmarshalJSON(data []byte) error {
	type Alias Message
	aux := &struct {
		*Alias
		Timestamp string `json:"timestamp"`
	}{
		Alias: (*Alias)(m),
	}
	
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	if aux.Timestamp != "" {
		t, err := time.Parse(time.RFC3339, aux.Timestamp)
		if err != nil {
			return err
		}
		m.Timestamp = t
	}
	
	return nil
}
