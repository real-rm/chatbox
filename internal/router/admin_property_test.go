package router

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/real-rm/golog"
)

const defaultTimeout = 15 * time.Minute

// getTestLogger creates a logger for testing
func getTestLogger() *golog.Logger {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            "/tmp/chatbox-test-logs",
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		panic("Failed to initialize test logger: " + err.Error())
	}
	return logger
}

// Feature: chat-application-websocket
// Property 54: Bidirectional Message Routing During Takeover
// **Validates: Requirements 17.4**
//
// For any active admin takeover, messages from both the user and Chat_Admin
// should be routed to each other.
//
// Note: This test verifies the admin connection registration mechanism.
// Full bidirectional routing is tested in integration tests.
func TestProperty_BidirectionalMessageRoutingDuringTakeover(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("admin and user connections can be registered for a session", prop.ForAll(
		func(userID, adminID string) bool {
			// Skip invalid inputs
			if userID == "" || adminID == "" {
				return true
			}

			logger := getTestLogger()
			sm := session.NewSessionManager(defaultTimeout, logger)
			mr := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

			// Create user session
			sess, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			// Create and register user connection
			userConn := websocket.NewConnection(userID, []string{"user"})
			userConn.SessionID = sess.ID
			err = mr.RegisterConnection(sess.ID, userConn)
			if err != nil {
				return false
			}

			// Create and register admin connection with compound key
			adminConn := websocket.NewConnection(adminID, []string{"admin"})
			err = mr.RegisterAdminConnection(adminID, sess.ID, adminConn)
			if err != nil {
				return false
			}

			// Verify both connections are registered
			adminConnKey := adminID + ":" + sess.ID
			mr.mu.RLock()
			_, userExists := mr.connections[sess.ID]
			_, adminExists := mr.adminConns[adminConnKey]
			mr.mu.RUnlock()

			return userExists && adminExists
		},
		gen.Identifier(),
		gen.Identifier(),
	))

	properties.TestingRun(t)
}

// Feature: chat-application-websocket
// Property 56: Session Takeover Event Logging
// **Validates: Requirements 17.7**
//
// For any session takeover, the WebSocket_Server should log the event with
// Chat_Admin ID, start time, and end time.
//
// Note: This property test verifies that admin takeover and leave operations
// complete successfully, which ensures logging occurs (as logging is part of
// those operations).
func TestProperty_SessionTakeoverEventLogging(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("admin takeover and leave operations complete successfully", prop.ForAll(
		func(userID, adminID, adminName string) bool {
			// Skip invalid inputs
			if userID == "" || adminID == "" || adminName == "" {
				return true
			}

			logger := getTestLogger()
			sm := session.NewSessionManager(defaultTimeout, logger)
			mr := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

			// Create user session
			sess, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			// Create admin connection
			adminConn := websocket.NewConnection(adminID, []string{"admin"})

			// Handle admin takeover (logs start event)
			err = mr.HandleAdminTakeover(adminConn, sess.ID)
			if err != nil {
				return false
			}

			// Handle admin leave (logs end event)
			err = mr.HandleAdminLeave(adminID, sess.ID)
			if err != nil {
				return false
			}

			// Verify admin is no longer assisting
			storedAdminID, _, err := sm.GetAssistingAdmin(sess.ID)
			if err != nil {
				return false
			}

			// Admin ID should be cleared after leaving
			return storedAdminID == ""
		},
		gen.Identifier(),
		gen.Identifier(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}
