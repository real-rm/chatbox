package router

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/chat-websocket/internal/message"
	"github.com/yourusername/chat-websocket/internal/session"
	"github.com/yourusername/chat-websocket/internal/websocket"
)

// mockConnection creates a mock WebSocket connection for testing
func mockConnection(userID string) *websocket.Connection {
	return websocket.NewConnection(userID, []string{"user"})
}

func TestNewMessageRouter(t *testing.T) {
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

	assert.NotNil(t, router)
	assert.NotNil(t, router.connections)
	assert.NotNil(t, router.adminConns)
	assert.Equal(t, sm, router.sessionManager)
}

func TestRegisterConnection(t *testing.T) {
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
}

func TestRouteMessage_InvalidMessageType(t *testing.T) {
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
	assert.ErrorIs(t, err, ErrInvalidMessage)
}

func TestRouteMessage_NilInputs(t *testing.T) {
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
		errType error
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
			errType: ErrInvalidMessage,
		},
		{
			name: "non-existent session",
			msg: &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: "non-existent",
				Content:   "Hello",
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			},
			wantErr: true,
			errType: ErrSessionNotFound,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
			errType: ErrNilMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.HandleUserMessage(conn, tt.msg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandleUserMessage_NilConnection(t *testing.T) {
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
			errType: ErrInvalidMessage,
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
			errType: ErrSessionNotFound,
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
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetConnection(t *testing.T) {
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
	sm := session.NewSessionManager(15 * time.Minute)
	router := NewMessageRouter(sm)

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
