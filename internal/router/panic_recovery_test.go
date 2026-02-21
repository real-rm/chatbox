package router

import (
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// panicNotificationService is a mock that panics when SendHelpRequestAlert is called.
type panicNotificationService struct{}

func (p *panicNotificationService) SendHelpRequestAlert(userID, sessionID string) error {
	panic("intentional panic in notification service")
}

// TestPanicRecovery_HelpRequestNotification verifies that a panic in the
// notification service goroutine does not crash the router.
func TestPanicRecovery_HelpRequestNotification(t *testing.T) {
	logger := createTestLogger()
	defer logger.Close()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}

	router := NewMessageRouter(sm, nil, nil, &panicNotificationService{}, mockStorage, 120*time.Second, logger)
	defer router.Shutdown()

	// Create a session for the user
	conn := websocket.NewConnection("user-panic", []string{"user"})
	sess, err := sm.CreateSession("user-panic")
	require.NoError(t, err)

	// Register connection
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Send help request - the notification goroutine will panic
	// but the router should survive
	msg := &message.Message{
		Type:      message.TypeHelpRequest,
		SessionID: sess.ID,
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.RouteMessage(conn, msg)
	assert.NoError(t, err, "router should not return error despite notification panic")

	// Give goroutine time to panic and recover
	time.Sleep(200 * time.Millisecond)

	// Verify router is still functional by registering a new connection
	err = router.RegisterConnection("new-session", websocket.NewConnection("user2", []string{"user"}))
	assert.NoError(t, err, "router should still be functional after panic recovery")
}

// TestSendToConnection_ClosingConnection verifies that sendToConnection does not
// panic when the connection is marked as closing.
func TestSendToConnection_ClosingConnection(t *testing.T) {
	logger := createTestLogger()
	defer logger.Close()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}

	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, 120*time.Second, logger)
	defer router.Shutdown()

	// Create and register a connection
	conn := websocket.NewConnection("user-close", []string{"user"})
	err := router.RegisterConnection("session-close", conn)
	require.NoError(t, err)

	// Mark connection as closing
	conn.SetClosing()

	// Try to send - should not panic, should return error
	msg := &message.Message{
		Type:      message.TypeConnectionStatus,
		SessionID: "session-close",
		Content:   "test",
		Sender:    message.SenderAI,
		Timestamp: time.Now(),
	}

	err = router.sendToConnection("session-close", msg)
	assert.Error(t, err, "should return error when connection is closing")
}
