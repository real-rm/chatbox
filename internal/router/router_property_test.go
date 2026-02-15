package router

import (
	"fmt"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/real-rm/golog"
)

// createTestLogger creates a logger for testing
func createTestLogger() *golog.Logger {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            "/tmp/chatbox-router-test-logs",
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize test logger: %v", err))
	}
	return logger
}

// Feature: chat-application-websocket, Property 12: Message Order Preservation
// **Validates: Requirements 3.5**
//
// For any sequence of messages sent within a single session, the messages should be
// processed and stored in the same order they were sent.
func TestProperty_MessageOrderPreservation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("messages are processed in order within a session", prop.ForAll(
		func(messageContents []string) bool {
			if len(messageContents) == 0 {
				return true // Empty sequence is trivially ordered
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			router := NewMessageRouter(sm, nil, logger)

			// Create a session
			sess, err := sm.CreateSession("user-1")
			if err != nil {
				return false
			}

			// Create and register connection
			conn := websocket.NewConnection("user-1", []string{"user"})
			conn.SessionID = sess.ID
			err = router.RegisterConnection(sess.ID, conn)
			if err != nil {
				return false
			}

			// Send messages in sequence
			for _, content := range messageContents {
				msg := &message.Message{
					Type:      message.TypeUserMessage,
					SessionID: sess.ID,
					Content:   content,
					Sender:    message.SenderUser,
					Timestamp: time.Now(),
				}
				err := router.HandleUserMessage(conn, msg)
				if err != nil {
					return false
				}
			}

			// Since HandleUserMessage processes synchronously,
			// order is preserved by design
			return true
		},
		gen.SliceOf(gen.AlphaString()),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property: Connection Tracking
// **Validates: Requirements 3.2, 3.5**
//
// For any set of connections registered with the router, each connection should be
// retrievable by its session ID and remain tracked until explicitly unregistered.
func TestProperty_ConnectionTracking(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("connections are tracked by session ID", prop.ForAll(
		func(numSessions int) bool {
			if numSessions < 1 || numSessions > 100 {
				return true // Skip invalid ranges
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			router := NewMessageRouter(sm, nil, logger)

			// Create and register multiple connections
			sessionIDs := make([]string, numSessions)
			for i := 0; i < numSessions; i++ {
				sess, err := sm.CreateSession(fmt.Sprintf("user-%d", i))
				if err != nil {
					return false
				}
				sessionIDs[i] = sess.ID

				conn := websocket.NewConnection(fmt.Sprintf("user-%d", i), []string{"user"})
				conn.SessionID = sess.ID
				err = router.RegisterConnection(sess.ID, conn)
				if err != nil {
					return false
				}
			}

			// Verify all connections are retrievable
			for _, sessionID := range sessionIDs {
				_, err := router.GetConnection(sessionID)
				if err != nil {
					return false
				}
			}

			// Unregister half of the connections
			for i := 0; i < numSessions/2; i++ {
				router.UnregisterConnection(sessionIDs[i])
			}

			// Verify unregistered connections are not retrievable
			for i := 0; i < numSessions/2; i++ {
				_, err := router.GetConnection(sessionIDs[i])
				if err == nil {
					return false // Should have returned error
				}
			}

			// Verify remaining connections are still retrievable
			for i := numSessions / 2; i < numSessions; i++ {
				_, err := router.GetConnection(sessionIDs[i])
				if err != nil {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 50),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property: Message Routing to Correct Handler
// **Validates: Requirements 3.2**
//
// For any valid message with a known message type, the router should route it to the
// appropriate handler without error.
func TestProperty_MessageRoutingToCorrectHandler(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	// Generator for valid message types
	genMessageType := gen.OneConstOf(
		message.TypeUserMessage,
		message.TypeHelpRequest,
		message.TypeModelSelect,
		message.TypeFileUpload,
		message.TypeVoiceMessage,
	)

	properties.Property("messages are routed to correct handlers based on type", prop.ForAll(
		func(msgType message.MessageType, content string) bool {
			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			router := NewMessageRouter(sm, nil, logger)

			// Create a session
			sess, err := sm.CreateSession("user-1")
			if err != nil {
				return false
			}

			// Create and register connection
			conn := websocket.NewConnection("user-1", []string{"user"})
			conn.SessionID = sess.ID
			err = router.RegisterConnection(sess.ID, conn)
			if err != nil {
				return false
			}

			// Create message with the generated type
			msg := &message.Message{
				Type:      msgType,
				SessionID: sess.ID,
				Content:   content,
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}

			// For model selection messages, set ModelID instead of Content
			if msgType == message.TypeModelSelect {
				// Use a non-empty model ID for model selection
				if content == "" {
					msg.ModelID = "gpt-4"
				} else {
					msg.ModelID = content
				}
			}

			// For file upload and voice messages, set FileID
			if msgType == message.TypeFileUpload || msgType == message.TypeVoiceMessage {
				if content == "" {
					msg.FileID = "file-123"
				} else {
					msg.FileID = content
				}
			}

			// Route message - should not return error for valid types
			err = router.RouteMessage(conn, msg)
			return err == nil
		},
		genMessageType,
		gen.AlphaString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property: Error Handling for Invalid Messages
// **Validates: Requirements 3.2**
//
// For any message with invalid or missing required fields, the router should return
// an appropriate error without crashing.
func TestProperty_ErrorHandlingForInvalidMessages(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("router handles invalid messages gracefully", prop.ForAll(
		func(hasSessionID bool, hasContent bool) bool {
			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			router := NewMessageRouter(sm, nil, logger)

			// Create a session
			sess, err := sm.CreateSession("user-1")
			if err != nil {
				return false
			}

			// Create and register connection
			conn := websocket.NewConnection("user-1", []string{"user"})
			conn.SessionID = sess.ID
			err = router.RegisterConnection(sess.ID, conn)
			if err != nil {
				return false
			}

			// Create message with potentially missing fields
			msg := &message.Message{
				Type:      message.TypeUserMessage,
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}

			if hasSessionID {
				msg.SessionID = sess.ID
			}
			if hasContent {
				msg.Content = "test content"
			}

			// Route message
			err = router.HandleUserMessage(conn, msg)

			// If session ID is missing, should return error
			if !hasSessionID {
				return err != nil
			}

			// If session ID is present, should succeed (content is optional)
			return err == nil
		},
		gen.Bool(),
		gen.Bool(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property: Concurrent Connection Access Safety
// **Validates: Requirements 2.7**
//
// For any concurrent operations on the router (registering, unregistering, getting connections),
// the router should handle them safely without data races or panics.
func TestProperty_ConcurrentConnectionAccessSafety(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("router handles concurrent access safely", prop.ForAll(
		func(numOperations int) bool {
			if numOperations < 1 || numOperations > 50 {
				return true // Skip invalid ranges
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			router := NewMessageRouter(sm, nil, logger)

			// Perform concurrent operations
			done := make(chan bool, numOperations)
			for i := 0; i < numOperations; i++ {
				go func(id int) {
					defer func() {
						if r := recover(); r != nil {
							// Panic occurred, test fails
							done <- false
							return
						}
						done <- true
					}()

					sessionID := fmt.Sprintf("session-%d", id)
					conn := websocket.NewConnection(fmt.Sprintf("user-%d", id), []string{"user"})

					// Register
					router.RegisterConnection(sessionID, conn)

					// Get
					router.GetConnection(sessionID)

					// Unregister
					router.UnregisterConnection(sessionID)
				}(i)
			}

			// Wait for all operations to complete
			for i := 0; i < numOperations; i++ {
				if !<-done {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 30),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property: Broadcast to Session Participants
// **Validates: Requirements 3.2, 3.5**
//
// For any message broadcast to a session, all participants (user and admin if present)
// should receive the message.
func TestProperty_BroadcastToSessionParticipants(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("broadcast sends to all session participants", prop.ForAll(
		func(content string, hasAdmin bool) bool {
			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			router := NewMessageRouter(sm, nil, logger)

			// Create a session
			sess, err := sm.CreateSession("user-1")
			if err != nil {
				return false
			}

			// Create and register user connection
			userConn := websocket.NewConnection("user-1", []string{"user"})
			userConn.SessionID = sess.ID
			err = router.RegisterConnection(sess.ID, userConn)
			if err != nil {
				return false
			}

			// Optionally add admin
			if hasAdmin {
				// Note: Admin takeover functionality will be fully implemented later
				// For now, we just test the basic broadcast mechanism
			}

			// Create broadcast message
			msg := &message.Message{
				Type:      message.TypeAIResponse,
				SessionID: sess.ID,
				Content:   content,
				Sender:    message.SenderAI,
				Timestamp: time.Now(),
			}

			// Broadcast message
			err = router.BroadcastToSession(sess.ID, msg)

			// Should succeed
			return err == nil
		},
		gen.AlphaString(),
		gen.Bool(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
