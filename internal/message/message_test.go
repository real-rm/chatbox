package message

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessage_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		wantErr bool
	}{
		{
			name: "user message with all fields",
			message: Message{
				Type:      TypeUserMessage,
				SessionID: "session-123",
				Content:   "Hello, AI!",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Sender:    SenderUser,
				Metadata: map[string]string{
					"key": "value",
				},
			},
			wantErr: false,
		},
		{
			name: "ai response message",
			message: Message{
				Type:      TypeAIResponse,
				SessionID: "session-123",
				Content:   "Hello, user!",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 1, 0, time.UTC),
				Sender:    SenderAI,
			},
			wantErr: false,
		},
		{
			name: "file upload message",
			message: Message{
				Type:      TypeFileUpload,
				SessionID: "session-123",
				FileID:    "file-456",
				FileURL:   "https://example.com/file.pdf",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 2, 0, time.UTC),
				Sender:    SenderUser,
			},
			wantErr: false,
		},
		{
			name: "error message",
			message: Message{
				Type:      TypeError,
				SessionID: "session-123",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 3, 0, time.UTC),
				Sender:    SenderUser,
				Error: &ErrorInfo{
					Code:        "VALIDATION_ERROR",
					Message:     "Invalid message format",
					Recoverable: true,
					RetryAfter:  1000,
				},
			},
			wantErr: false,
		},
		{
			name: "model select message",
			message: Message{
				Type:      TypeModelSelect,
				SessionID: "session-123",
				ModelID:   "gpt-4",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 4, 0, time.UTC),
				Sender:    SenderUser,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(&tt.message)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, data)

			// Verify JSON is valid
			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			// Verify required fields
			assert.Equal(t, string(tt.message.Type), result["type"])
			assert.Equal(t, string(tt.message.Sender), result["sender"])
			assert.NotEmpty(t, result["timestamp"])
		})
	}
}

func TestMessage_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    Message
		wantErr bool
	}{
		{
			name: "valid user message",
			json: `{
				"type": "user_message",
				"session_id": "session-123",
				"content": "Hello, AI!",
				"timestamp": "2024-01-01T12:00:00Z",
				"sender": "user"
			}`,
			want: Message{
				Type:      TypeUserMessage,
				SessionID: "session-123",
				Content:   "Hello, AI!",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Sender:    SenderUser,
			},
			wantErr: false,
		},
		{
			name: "valid ai response",
			json: `{
				"type": "ai_response",
				"session_id": "session-123",
				"content": "Hello, user!",
				"timestamp": "2024-01-01T12:00:01Z",
				"sender": "ai"
			}`,
			want: Message{
				Type:      TypeAIResponse,
				SessionID: "session-123",
				Content:   "Hello, user!",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 1, 0, time.UTC),
				Sender:    SenderAI,
			},
			wantErr: false,
		},
		{
			name: "file upload with metadata",
			json: `{
				"type": "file_upload",
				"session_id": "session-123",
				"file_id": "file-456",
				"file_url": "https://example.com/file.pdf",
				"timestamp": "2024-01-01T12:00:02Z",
				"sender": "user",
				"metadata": {
					"filename": "document.pdf",
					"size": "1024"
				}
			}`,
			want: Message{
				Type:      TypeFileUpload,
				SessionID: "session-123",
				FileID:    "file-456",
				FileURL:   "https://example.com/file.pdf",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 2, 0, time.UTC),
				Sender:    SenderUser,
				Metadata: map[string]string{
					"filename": "document.pdf",
					"size":     "1024",
				},
			},
			wantErr: false,
		},
		{
			name: "error message with error info",
			json: `{
				"type": "error",
				"session_id": "session-123",
				"timestamp": "2024-01-01T12:00:03Z",
				"sender": "user",
				"error": {
					"code": "VALIDATION_ERROR",
					"message": "Invalid message format",
					"recoverable": true,
					"retry_after": 1000
				}
			}`,
			want: Message{
				Type:      TypeError,
				SessionID: "session-123",
				Timestamp: time.Date(2024, 1, 1, 12, 0, 3, 0, time.UTC),
				Sender:    SenderUser,
				Error: &ErrorInfo{
					Code:        "VALIDATION_ERROR",
					Message:     "Invalid message format",
					Recoverable: true,
					RetryAfter:  1000,
				},
			},
			wantErr: false,
		},
		{
			name:    "invalid json",
			json:    `{invalid}`,
			want:    Message{},
			wantErr: true,
		},
		{
			name: "invalid timestamp format",
			json: `{
				"type": "user_message",
				"timestamp": "invalid-time",
				"sender": "user"
			}`,
			want:    Message{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg Message
			err := json.Unmarshal([]byte(tt.json), &msg)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.Type, msg.Type)
			assert.Equal(t, tt.want.SessionID, msg.SessionID)
			assert.Equal(t, tt.want.Content, msg.Content)
			assert.Equal(t, tt.want.FileID, msg.FileID)
			assert.Equal(t, tt.want.FileURL, msg.FileURL)
			assert.Equal(t, tt.want.Sender, msg.Sender)
			assert.True(t, tt.want.Timestamp.Equal(msg.Timestamp))

			if tt.want.Metadata != nil {
				assert.Equal(t, tt.want.Metadata, msg.Metadata)
			}

			if tt.want.Error != nil {
				require.NotNil(t, msg.Error)
				assert.Equal(t, tt.want.Error.Code, msg.Error.Code)
				assert.Equal(t, tt.want.Error.Message, msg.Error.Message)
				assert.Equal(t, tt.want.Error.Recoverable, msg.Error.Recoverable)
				assert.Equal(t, tt.want.Error.RetryAfter, msg.Error.RetryAfter)
			}
		})
	}
}

func TestMessage_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		message Message
	}{
		{
			name: "user message round trip",
			message: Message{
				Type:      TypeUserMessage,
				SessionID: "session-123",
				Content:   "Test message",
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Sender:    SenderUser,
			},
		},
		{
			name: "admin message round trip",
			message: Message{
				Type:      TypeAdminJoin,
				SessionID: "session-456",
				Content:   "Admin joined",
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Sender:    SenderAdmin,
				Metadata: map[string]string{
					"admin_name": "John Doe",
				},
			},
		},
		{
			name: "voice message round trip",
			message: Message{
				Type:      TypeVoiceMessage,
				SessionID: "session-789",
				FileID:    "voice-123",
				FileURL:   "https://example.com/voice.mp3",
				Timestamp: time.Now().UTC().Truncate(time.Second),
				Sender:    SenderUser,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(&tt.message)
			require.NoError(t, err)

			// Unmarshal back
			var result Message
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			// Compare
			assert.Equal(t, tt.message.Type, result.Type)
			assert.Equal(t, tt.message.SessionID, result.SessionID)
			assert.Equal(t, tt.message.Content, result.Content)
			assert.Equal(t, tt.message.FileID, result.FileID)
			assert.Equal(t, tt.message.FileURL, result.FileURL)
			assert.Equal(t, tt.message.Sender, result.Sender)
			assert.True(t, tt.message.Timestamp.Equal(result.Timestamp))
			assert.Equal(t, tt.message.Metadata, result.Metadata)
		})
	}
}

func TestMessageTypes(t *testing.T) {
	tests := []struct {
		name        string
		messageType MessageType
		expected    string
	}{
		{"user message", TypeUserMessage, "user_message"},
		{"ai response", TypeAIResponse, "ai_response"},
		{"file upload", TypeFileUpload, "file_upload"},
		{"voice message", TypeVoiceMessage, "voice_message"},
		{"error", TypeError, "error"},
		{"connection status", TypeConnectionStatus, "connection_status"},
		{"typing indicator", TypeTypingIndicator, "typing_indicator"},
		{"help request", TypeHelpRequest, "help_request"},
		{"admin join", TypeAdminJoin, "admin_join"},
		{"admin leave", TypeAdminLeave, "admin_leave"},
		{"model select", TypeModelSelect, "model_select"},
		{"loading", TypeLoading, "loading"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.messageType))
		})
	}
}

func TestSenderTypes(t *testing.T) {
	tests := []struct {
		name       string
		senderType SenderType
		expected   string
	}{
		{"user sender", SenderUser, "user"},
		{"ai sender", SenderAI, "ai"},
		{"admin sender", SenderAdmin, "admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.senderType))
		})
	}
}

func TestErrorInfo(t *testing.T) {
	tests := []struct {
		name      string
		errorInfo ErrorInfo
	}{
		{
			name: "recoverable error",
			errorInfo: ErrorInfo{
				Code:        "RATE_LIMIT",
				Message:     "Too many requests",
				Recoverable: true,
				RetryAfter:  5000,
			},
		},
		{
			name: "fatal error",
			errorInfo: ErrorInfo{
				Code:        "AUTH_FAILED",
				Message:     "Authentication failed",
				Recoverable: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Message{
				Type:      TypeError,
				Timestamp: time.Now().UTC(),
				Sender:    SenderUser,
				Error:     &tt.errorInfo,
			}

			data, err := json.Marshal(&msg)
			require.NoError(t, err)

			var result Message
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			require.NotNil(t, result.Error)
			assert.Equal(t, tt.errorInfo.Code, result.Error.Code)
			assert.Equal(t, tt.errorInfo.Message, result.Error.Message)
			assert.Equal(t, tt.errorInfo.Recoverable, result.Error.Recoverable)
			assert.Equal(t, tt.errorInfo.RetryAfter, result.Error.RetryAfter)
		})
	}
}
