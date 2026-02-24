package router

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/metrics"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastToSession_AdminDropIncrementsMetric verifies that when an admin
// connection's send channel is full or closing, metrics.AdminMessagesDropped
// is incremented and a Warn is logged (best-effort semantics are documented).
func TestBroadcastToSession_AdminDropIncrementsMetric(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mr := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session owned by a regular user.
	userConn := websocket.NewConnection("user-drop-test", []string{"user"})
	sess, err := sm.CreateSession(userConn.UserID)
	require.NoError(t, err)
	err = mr.RegisterConnection(sess.ID, userConn)
	require.NoError(t, err)

	// Register admin, marking session as admin-assisted.
	adminConn := websocket.NewConnection("admin-drop-test", []string{"admin"})
	err = mr.HandleAdminTakeover(adminConn, sess.ID)
	require.NoError(t, err)

	// Mark adminConn as closing so SafeSend returns false (simulates full/closed channel).
	adminConn.SetClosing()

	before := testutil.ToFloat64(metrics.AdminMessagesDropped)

	// Broadcast a message; the admin channel is unavailable → drop must increment metric.
	msg := &message.Message{
		Type:      message.TypeAIResponse,
		SessionID: sess.ID,
		Content:   "hello",
		Sender:    message.SenderAI,
		Timestamp: time.Now(),
	}

	_ = mr.BroadcastToSession(sess.ID, msg)

	after := testutil.ToFloat64(metrics.AdminMessagesDropped)
	assert.Greater(t, after, before, "AdminMessagesDropped must increment when admin SafeSend fails")
}
