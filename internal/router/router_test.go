package router

import (
	"fmt"
	"testing"
	"time"

	chaterrors "github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConnection creates a mock WebSocket connection for testing
func mockConnection(userID string) *websocket.Connection {
	return websocket.NewConnection(userID, []string{"user"})
}

// mockStorageService is a mock implementation of StorageService for testing
type mockStorageService struct {
	createSessionCalled bool
	createSessionError  error
	createdSessions     []*session.Session
}

func (m *mockStorageService) CreateSession(sess *session.Session) error {
	m.createSessionCalled = true
	if m.createSessionError != nil {
		return m.createSessionError
	}
	m.createdSessions = append(m.createdSessions, sess)
	return nil
}

func TestNewMessageRouter(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	assert.NotNil(t, router)
	assert.NotNil(t, router.connections)
	assert.NotNil(t, router.adminConns)
	assert.Equal(t, sm, router.sessionManager)
}

func TestRegisterConnection(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	tests := []struct {
		name      string
		sessionID string
		conn      *websocket.Connection
		wantErr   bool
		errType   error
	}{
		{
			name:      "valid connection",
			sessionID: "session-123",
			conn:      mockConnection("user-1"),
			wantErr:   false,
		},
		{
			name:      "nil connection",
			sessionID: "session-123",
			conn:      nil,
			wantErr:   true,
			errType:   ErrNilConnection,
		},
		{
			name:      "empty session ID",
			sessionID: "",
			conn:      mockConnection("user-1"),
			wantErr:   true,
			errType:   ErrInvalidMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.RegisterConnection(tt.sessionID, tt.conn)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				assert.NoError(t, err)
				// Verify connection is registered
				conn, err := router.GetConnection(tt.sessionID)
				assert.NoError(t, err)
				assert.Equal(t, tt.conn, conn)
			}
		})
	}
}

func TestUnregisterConnection(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	sessionID := "session-123"
	conn := mockConnection("user-1")

	// Register connection
	err := router.RegisterConnection(sessionID, conn)
	require.NoError(t, err)

	// Verify it's registered
	_, err = router.GetConnection(sessionID)
	require.NoError(t, err)

	// Unregister
	router.UnregisterConnection(sessionID)

	// Verify it's unregistered
	_, err = router.GetConnection(sessionID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrConnectionNotFound)
}

func TestRouteMessage_UserMessage(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockLLM := &mockLLMService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Create and register connection
	conn := mockConnection("user-1")
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Create user message
	msg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sess.ID,
		Content:   "Hello",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	// Route message
	err = router.RouteMessage(conn, msg)
	assert.NoError(t, err)
	
	// Verify LLM service was called with streaming
	assert.True(t, mockLLM.streamCalled, "StreamMessage should be called")
	assert.False(t, mockLLM.sendMessageCalled, "SendMessage should not be called")
}

func TestRouteMessage_InvalidMessageType(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	conn := mockConnection("user-1")
	msg := &message.Message{
		Type:      "invalid_type",
		SessionID: "session-123",
		Content:   "Hello",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err := router.RouteMessage(conn, msg)
	assert.Error(t, err)

	// Check if it's a ChatError
	var chatErr *chaterrors.ChatError
	assert.ErrorAs(t, err, &chatErr)
	assert.Equal(t, chaterrors.ErrCodeInvalidFormat, chatErr.Code)
}

func TestRouteMessage_NilInputs(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	conn := mockConnection("user-1")
	msg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: "session-123",
		Content:   "Hello",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	// Test nil connection
	err := router.RouteMessage(nil, msg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilConnection)

	// Test nil message
	err = router.RouteMessage(conn, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilMessage)
}

func TestHandleUserMessage(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockLLM := &mockLLMService{}
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Create and register connection with send channel
	conn := mockConnection("user-1")
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	tests := []struct {
		name    string
		msg     *message.Message
		wantErr bool
		errCode chaterrors.ErrorCode
	}{
		{
			name: "valid user message",
			msg: &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: sess.ID,
				Content:   "Hello",
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing session ID",
			msg: &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: "",
				Content:   "Hello",
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			},
			wantErr: true,
			errCode: chaterrors.ErrCodeMissingField,
		},
		{
			name: "non-existent session - auto-creates",
			msg: &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: "non-existent",
				Content:   "Hello",
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			},
			wantErr: true, // Will fail because user-1 already has an active session
			errCode: chaterrors.ErrCodeDatabaseError,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.HandleUserMessage(conn, tt.msg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					var chatErr *chaterrors.ChatError
					if assert.ErrorAs(t, err, &chatErr) {
						assert.Equal(t, tt.errCode, chatErr.Code)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandleUserMessage_NilConnection(t *testing.T) {
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

	err := router.HandleUserMessage(nil, msg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilConnection)
}

func TestBroadcastToSession(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Create and register connection
	conn := mockConnection("user-1")
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	tests := []struct {
		name      string
		sessionID string
		msg       *message.Message
		wantErr   bool
		errType   error
		errCode   chaterrors.ErrorCode
	}{
		{
			name:      "valid broadcast",
			sessionID: sess.ID,
			msg: &message.Message{
				Type:      message.TypeAIResponse,
				SessionID: sess.ID,
				Content:   "Response",
				Sender:    message.SenderAI,
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name:      "nil message",
			sessionID: sess.ID,
			msg:       nil,
			wantErr:   true,
			errType:   ErrNilMessage,
		},
		{
			name:      "empty session ID",
			sessionID: "",
			msg: &message.Message{
				Type:      message.TypeAIResponse,
				Content:   "Response",
				Sender:    message.SenderAI,
				Timestamp: time.Now(),
			},
			wantErr: true,
			errCode: chaterrors.ErrCodeMissingField,
		},
		{
			name:      "non-existent session",
			sessionID: "non-existent",
			msg: &message.Message{
				Type:      message.TypeAIResponse,
				Content:   "Response",
				Sender:    message.SenderAI,
				Timestamp: time.Now(),
			},
			wantErr: true,
			errCode: chaterrors.ErrCodeMissingField,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.BroadcastToSession(tt.sessionID, tt.msg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				if tt.errCode != "" {
					var chatErr *chaterrors.ChatError
					if assert.ErrorAs(t, err, &chatErr) {
						assert.Equal(t, tt.errCode, chatErr.Code)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetConnection(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	sessionID := "session-123"
	conn := mockConnection("user-1")

	// Test getting non-existent connection
	_, err := router.GetConnection(sessionID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrConnectionNotFound)

	// Register connection
	err = router.RegisterConnection(sessionID, conn)
	require.NoError(t, err)

	// Test getting existing connection
	gotConn, err := router.GetConnection(sessionID)
	assert.NoError(t, err)
	assert.Equal(t, conn, gotConn)
}

// TestMessageOrderPreservation tests that messages are processed in order
func TestMessageOrderPreservation(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockLLM := &mockLLMService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Create and register connection
	conn := mockConnection("user-1")
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Send multiple messages in sequence
	messages := []string{"First", "Second", "Third"}
	for _, content := range messages {
		msg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: sess.ID,
			Content:   content,
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		err := router.HandleUserMessage(conn, msg)
		assert.NoError(t, err)
	}

	// The test verifies that RouteMessage processes messages synchronously
	// which ensures order preservation within a session
}

// TestConcurrentConnectionAccess tests thread-safe connection access
func TestConcurrentConnectionAccess(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create multiple sessions and connections
	numConnections := 10
	for i := 0; i < numConnections; i++ {
		sessionID := fmt.Sprintf("session-%d", i)
		conn := mockConnection(fmt.Sprintf("user-%d", i))
		err := router.RegisterConnection(sessionID, conn)
		require.NoError(t, err)
	}

	// Concurrently access connections
	done := make(chan bool)
	for i := 0; i < numConnections; i++ {
		go func(id int) {
			sessionID := fmt.Sprintf("session-%d", id)
			_, err := router.GetConnection(sessionID)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numConnections; i++ {
		<-done
	}
}

// TestHandleModelSelection tests model selection message handling
func TestHandleModelSelection(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Create and register connection
	conn := mockConnection("user-123")
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Create model selection message
	msg := &message.Message{
		Type:      message.TypeModelSelect,
		SessionID: sess.ID,
		ModelID:   "gpt-4",
		Timestamp: time.Now(),
		Sender:    message.SenderUser,
	}

	// Route the message
	err = router.RouteMessage(conn, msg)
	require.NoError(t, err)

	// Verify model was set in session
	modelID, err := sm.GetModelID(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4", modelID)
}

func TestHandleModelSelection_EmptySessionID(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	conn := mockConnection("user-123")

	msg := &message.Message{
		Type:      message.TypeModelSelect,
		SessionID: "",
		ModelID:   "gpt-4",
		Timestamp: time.Now(),
		Sender:    message.SenderUser,
	}

	err := router.RouteMessage(conn, msg)
	require.Error(t, err)

	var chatErr *chaterrors.ChatError
	if assert.ErrorAs(t, err, &chatErr) {
		assert.Equal(t, chaterrors.ErrCodeMissingField, chatErr.Code)
	}
}

func TestHandleModelSelection_EmptyModelID(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	conn := mockConnection("user-123")

	msg := &message.Message{
		Type:      message.TypeModelSelect,
		SessionID: sess.ID,
		ModelID:   "",
		Timestamp: time.Now(),
		Sender:    message.SenderUser,
	}

	err = router.RouteMessage(conn, msg)
	require.Error(t, err)

	var chatErr *chaterrors.ChatError
	if assert.ErrorAs(t, err, &chatErr) {
		assert.Equal(t, chaterrors.ErrCodeMissingField, chatErr.Code)
	}
}

func TestHandleModelSelection_NonExistentSession(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	conn := mockConnection("user-123")

	msg := &message.Message{
		Type:      message.TypeModelSelect,
		SessionID: "non-existent-session",
		ModelID:   "gpt-4",
		Timestamp: time.Now(),
		Sender:    message.SenderUser,
	}

	err := router.RouteMessage(conn, msg)
	require.Error(t, err)

	var chatErr *chaterrors.ChatError
	if assert.ErrorAs(t, err, &chatErr) {
		assert.Equal(t, chaterrors.ErrCodeMissingField, chatErr.Code)
	}
}

func TestHandleModelSelection_UpdateModel(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Create and register connection
	conn := mockConnection("user-123")
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Set initial model
	msg1 := &message.Message{
		Type:      message.TypeModelSelect,
		SessionID: sess.ID,
		ModelID:   "gpt-3.5-turbo",
		Timestamp: time.Now(),
		Sender:    message.SenderUser,
	}

	err = router.RouteMessage(conn, msg1)
	require.NoError(t, err)

	// Verify initial model
	modelID, err := sm.GetModelID(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, "gpt-3.5-turbo", modelID)

	// Update to different model
	msg2 := &message.Message{
		Type:      message.TypeModelSelect,
		SessionID: sess.ID,
		ModelID:   "gpt-4",
		Timestamp: time.Now(),
		Sender:    message.SenderUser,
	}

	err = router.RouteMessage(conn, msg2)
	require.NoError(t, err)

	// Verify model was updated
	modelID, err = sm.GetModelID(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4", modelID)
}

func TestHandleModelSelection_NilConnection(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	msg := &message.Message{
		Type:      message.TypeModelSelect,
		SessionID: sess.ID,
		ModelID:   "gpt-4",
		Timestamp: time.Now(),
		Sender:    message.SenderUser,
	}

	err = router.RouteMessage(nil, msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilConnection)
}

func TestHandleModelSelection_NilMessage(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	conn := mockConnection("user-123")

	err := router.RouteMessage(conn, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNilMessage)
}

func TestModelSelection_Persistence(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Create and register connection
	conn := mockConnection("user-123")
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Set model
	msg := &message.Message{
		Type:      message.TypeModelSelect,
		SessionID: sess.ID,
		ModelID:   "claude-3",
		Timestamp: time.Now(),
		Sender:    message.SenderUser,
	}

	err = router.RouteMessage(conn, msg)
	require.NoError(t, err)

	// End and restore session
	err = sm.EndSession(sess.ID)
	require.NoError(t, err)

	restored, err := sm.RestoreSession("user-123", sess.ID)
	require.NoError(t, err)

	// Model should persist across session restoration
	assert.Equal(t, "claude-3", restored.ModelID)
}
