package router

import (
	"context"
	"testing"
	"time"

	chaterrors "github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLLMServiceForErrorTests is a simple mock for error handling tests
type mockLLMServiceForErrorTests struct{}

func (m *mockLLMServiceForErrorTests) SendMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{
		Content:    "Mock response",
		TokensUsed: 10,
		Duration:   100 * time.Millisecond,
	}, nil
}

func (m *mockLLMServiceForErrorTests) StreamMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (<-chan *llm.LLMChunk, error) {
	ch := make(chan *llm.LLMChunk, 1)
	ch <- &llm.LLMChunk{Content: "Mock chunk", Done: true}
	close(ch)
	return ch, nil
}

func (m *mockLLMServiceForErrorTests) ValidateModel(modelID string) error { return nil }

// mockStorageServiceForErrorTests is a simple mock for error handling tests
type mockStorageServiceForErrorTests struct{}

func (m *mockStorageServiceForErrorTests) CreateSession(sess *session.Session) error {
	return nil
}

func (m *mockStorageServiceForErrorTests) AddMessage(sessionID string, msg *session.Message) error {
	return nil
}

// TestErrorHandling_NilConnection tests that nil connection is properly handled
// **Validates: Requirements 6.1**
func TestErrorHandling_NilConnection(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	msg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: "session-123",
		Content:   "Hello",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	// Test RouteMessage with nil connection
	err := router.RouteMessage(nil, msg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilConnection)

	// Test HandleUserMessage with nil connection
	err = router.HandleUserMessage(nil, msg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilConnection)
}

// TestErrorHandling_NilMessage tests that nil message is properly handled
// **Validates: Requirements 6.1**
func TestErrorHandling_NilMessage(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	conn := mockConnection("user-1")

	// Test RouteMessage with nil message
	err := router.RouteMessage(conn, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilMessage)
}

// TestErrorHandling_InvalidSessionID tests handling of invalid/non-existent session IDs
// **Validates: Requirements 6.1**
func TestErrorHandling_InvalidSessionID(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	conn := mockConnection("user-123")

	tests := []struct {
		name      string
		sessionID string
		msgType   message.MessageType
		wantErr   bool
		errCode   chaterrors.ErrorCode
	}{
		{
			name:      "empty session ID",
			sessionID: "",
			msgType:   message.TypeUserMessage,
			wantErr:   true,
			errCode:   chaterrors.ErrCodeMissingField,
		},
		{
			name:      "non-existent session ID for model selection",
			sessionID: "non-existent-session",
			msgType:   message.TypeModelSelect,
			wantErr:   true,
			errCode:   chaterrors.ErrCodeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &message.Message{
				Type:      tt.msgType,
				SessionID: tt.sessionID,
				Content:   "Test content",
				ModelID:   "gpt-4",
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}

			err := router.RouteMessage(conn, msg)

			if tt.wantErr {
				require.Error(t, err)
				var chatErr *chaterrors.ChatError
				if assert.ErrorAs(t, err, &chatErr) {
					assert.Equal(t, tt.errCode, chatErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestErrorHandling_RateLimitExceeded tests rate limit enforcement
// **Validates: Requirements 6.1**
func TestErrorHandling_RateLimitExceeded(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockLLM := &mockLLMServiceForErrorTests{}
	mockStorage := &mockStorageServiceForErrorTests{}

	// Create router with mocks
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Replace the rate limiter with one that has a very low limit for testing
	router.messageLimiter = ratelimit.NewMessageLimiter(1*time.Minute, 2)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	conn := mockConnection("user-1")
	conn.SessionID = sess.ID

	// Register the connection
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Send messages up to the limit
	for i := 0; i < 2; i++ {
		msg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: sess.ID,
			Content:   "Test message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}

		err := router.RouteMessage(conn, msg)
		// First 2 messages should succeed (or fail for other reasons, but not rate limit)
		if err != nil {
			var chatErr *chaterrors.ChatError
			if assert.ErrorAs(t, err, &chatErr) {
				assert.NotEqual(t, chaterrors.ErrCodeTooManyRequests, chatErr.Code,
					"Message %d should not be rate limited", i+1)
			}
		}
	}

	// The 3rd message should be rate limited
	msg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sess.ID,
		Content:   "This should be rate limited",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.RouteMessage(conn, msg)
	require.Error(t, err)

	var chatErr *chaterrors.ChatError
	if assert.ErrorAs(t, err, &chatErr) {
		assert.Equal(t, chaterrors.ErrCodeTooManyRequests, chatErr.Code)
		assert.True(t, chatErr.Recoverable, "Rate limit error should be recoverable")
		assert.Greater(t, chatErr.RetryAfter, 0, "RetryAfter should be set")
	}
}

// TestErrorHandling_RateLimitOnlyAppliesToUserMessages tests that rate limiting
// only applies to user messages, not other message types
// **Validates: Requirements 6.1**
func TestErrorHandling_RateLimitOnlyAppliesToUserMessages(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockLLM := &mockLLMServiceForErrorTests{}
	mockStorage := &mockStorageServiceForErrorTests{}

	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Replace with a very restrictive rate limiter
	router.messageLimiter = ratelimit.NewMessageLimiter(1*time.Minute, 1)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	conn := mockConnection("user-1")
	conn.SessionID = sess.ID

	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Send one user message to exhaust the rate limit
	userMsg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sess.ID,
		Content:   "Test message",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	_ = router.RouteMessage(conn, userMsg)

	// Now try to send a model selection message - should NOT be rate limited
	modelMsg := &message.Message{
		Type:      message.TypeModelSelect,
		SessionID: sess.ID,
		ModelID:   "gpt-4",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.RouteMessage(conn, modelMsg)
	// Should fail with NotFound (session doesn't have model), not rate limit
	if err != nil {
		var chatErr *chaterrors.ChatError
		if assert.ErrorAs(t, err, &chatErr) {
			assert.NotEqual(t, chaterrors.ErrCodeTooManyRequests, chatErr.Code,
				"Model selection should not be rate limited")
		}
	}
}
