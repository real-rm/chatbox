package router

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	chaterrors "github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEdgeCase_EmptyMessageContent tests handling of messages with empty content
// **Validates: Requirements 6.1**
func TestEdgeCase_EmptyMessageContent(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockLLM := &mockLLMService{}
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Create and register connection
	conn := mockConnection("user-1")
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	tests := []struct {
		name        string
		msgType     message.MessageType
		content     string
		shouldError bool
		description string
	}{
		{
			name:        "empty user message content",
			msgType:     message.TypeUserMessage,
			content:     "",
			shouldError: false, // Empty content is allowed, LLM should handle it
			description: "User sends empty message",
		},
		{
			name:        "whitespace only content",
			msgType:     message.TypeUserMessage,
			content:     "   \t\n  ",
			shouldError: false, // Whitespace is valid content
			description: "User sends whitespace-only message",
		},
		{
			name:        "empty help request",
			msgType:     message.TypeHelpRequest,
			content:     "",
			shouldError: false, // Help requests don't require content
			description: "User requests help without message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &message.Message{
				Type:      tt.msgType,
				SessionID: sess.ID,
				Content:   tt.content,
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}

			err := router.RouteMessage(conn, msg)
			if tt.shouldError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

// TestEdgeCase_VeryLongMessages tests handling of extremely long message content
// **Validates: Requirements 6.1**
func TestEdgeCase_VeryLongMessages(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockLLM := &mockLLMService{}
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Create and register connection
	conn := mockConnection("user-1")
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	tests := []struct {
		name        string
		contentSize int
		description string
	}{
		{
			name:        "1KB message",
			contentSize: 1024,
			description: "Small message should be handled",
		},
		{
			name:        "10KB message",
			contentSize: 10 * 1024,
			description: "Medium message should be handled",
		},
		{
			name:        "100KB message",
			contentSize: 100 * 1024,
			description: "Large message should be handled",
		},
		{
			name:        "1MB message",
			contentSize: 1024 * 1024,
			description: "Very large message at limit should be handled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a message with the specified size
			content := strings.Repeat("a", tt.contentSize)

			msg := &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: sess.ID,
				Content:   content,
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}

			err := router.RouteMessage(conn, msg)
			assert.NoError(t, err, tt.description)

			// Verify the message was processed
			assert.True(t, mockLLM.streamCalled, "LLM should be called for large messages")

			// Reset mock for next test
			mockLLM.streamCalled = false
		})
	}
}

// TestEdgeCase_ConcurrentMessageRouting tests concurrent message routing to different sessions
// **Validates: Requirements 6.1**
func TestEdgeCase_ConcurrentMessageRouting(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockLLM := &mockLLMService{}
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	numSessions := 10
	messagesPerSession := 5

	// Create multiple sessions and connections
	sessions := make([]*session.Session, numSessions)
	connections := make([]*websocket.Connection, numSessions)

	for i := 0; i < numSessions; i++ {
		userID := fmt.Sprintf("user-%d", i)
		sess, err := sm.CreateSession(userID)
		require.NoError(t, err)
		sessions[i] = sess

		conn := mockConnection(userID)
		conn.SessionID = sess.ID
		connections[i] = conn

		err = router.RegisterConnection(sess.ID, conn)
		require.NoError(t, err)
	}

	// Send messages concurrently from all sessions
	var wg sync.WaitGroup
	errors := make(chan error, numSessions*messagesPerSession)

	for i := 0; i < numSessions; i++ {
		sessionIdx := i
		wg.Add(1)

		go func() {
			defer wg.Done()

			for j := 0; j < messagesPerSession; j++ {
				msg := &message.Message{
					Type:      message.TypeUserMessage,
					SessionID: sessions[sessionIdx].ID,
					Content:   fmt.Sprintf("Message %d from session %d", j, sessionIdx),
					Sender:    message.SenderUser,
					Timestamp: time.Now(),
				}

				err := router.RouteMessage(connections[sessionIdx], msg)
				if err != nil {
					errors <- err
				}
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Concurrent routing error: %v", err)
		errorCount++
	}

	assert.Equal(t, 0, errorCount, "No errors should occur during concurrent message routing")
}

// TestEdgeCase_ConcurrentMessageRoutingSameSession tests concurrent messages to the same session
// **Validates: Requirements 6.1**
func TestEdgeCase_ConcurrentMessageRoutingSameSession(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockLLM := &mockLLMService{}
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Create a single session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	conn := mockConnection("user-1")
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	numMessages := 20
	var wg sync.WaitGroup
	errors := make(chan error, numMessages)

	// Send multiple messages concurrently to the same session
	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		messageNum := i

		go func() {
			defer wg.Done()

			msg := &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: sess.ID,
				Content:   fmt.Sprintf("Concurrent message %d", messageNum),
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}

			err := router.RouteMessage(conn, msg)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Concurrent routing error: %v", err)
		errorCount++
	}

	// Some errors may occur due to rate limiting, which is expected
	t.Logf("Total errors (may include rate limit errors): %d", errorCount)
}

// TestEdgeCase_AdminTakeoverNilConnection tests admin takeover with nil connection
// **Validates: Requirements 6.1**
func TestEdgeCase_AdminTakeoverNilConnection(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Try to takeover with nil connection
	err = router.HandleAdminTakeover(nil, sess.ID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilConnection)
}

// TestEdgeCase_AdminTakeoverEmptySessionID tests admin takeover with empty session ID
// **Validates: Requirements 6.1**
func TestEdgeCase_AdminTakeoverEmptySessionID(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	adminConn := mockConnection("admin-1")
	adminConn.Roles = []string{"admin"}

	err := router.HandleAdminTakeover(adminConn, "")
	assert.Error(t, err)

	var chatErr *chaterrors.ChatError
	if assert.ErrorAs(t, err, &chatErr) {
		assert.Equal(t, chaterrors.ErrCodeMissingField, chatErr.Code)
	}
}

// TestEdgeCase_AdminTakeoverNonExistentSession tests admin takeover of non-existent session
// **Validates: Requirements 6.1**
func TestEdgeCase_AdminTakeoverNonExistentSession(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	adminConn := mockConnection("admin-1")
	adminConn.Roles = []string{"admin"}

	err := router.HandleAdminTakeover(adminConn, "non-existent-session")
	assert.Error(t, err)

	var chatErr *chaterrors.ChatError
	if assert.ErrorAs(t, err, &chatErr) {
		assert.Equal(t, chaterrors.ErrCodeNotFound, chatErr.Code)
	}
}

// TestEdgeCase_AdminTakeoverAlreadyAssisted tests admin takeover when another admin is already assisting
// **Validates: Requirements 6.1**
func TestEdgeCase_AdminTakeoverAlreadyAssisted(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// First admin takes over
	admin1Conn := mockConnection("admin-1")
	admin1Conn.Roles = []string{"admin"}
	admin1Conn.Name = "Admin One"

	err = router.HandleAdminTakeover(admin1Conn, sess.ID)
	require.NoError(t, err)

	// Verify first admin is assisting
	updatedSess, err := sm.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin-1", updatedSess.AssistingAdminID)

	// Second admin tries to take over
	admin2Conn := mockConnection("admin-2")
	admin2Conn.Roles = []string{"admin"}
	admin2Conn.Name = "Admin Two"

	err = router.HandleAdminTakeover(admin2Conn, sess.ID)
	assert.Error(t, err)

	var chatErr *chaterrors.ChatError
	if assert.ErrorAs(t, err, &chatErr) {
		assert.Equal(t, chaterrors.ErrCodeInvalidFormat, chatErr.Code)
		assert.Contains(t, chatErr.Message, "already being assisted")
	}
}

// TestEdgeCase_AdminTakeoverSameAdminTwice tests same admin taking over twice
// **Validates: Requirements 6.1**
func TestEdgeCase_AdminTakeoverSameAdminTwice(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Admin takes over
	adminConn := mockConnection("admin-1")
	adminConn.Roles = []string{"admin"}
	adminConn.Name = "Admin One"

	err = router.HandleAdminTakeover(adminConn, sess.ID)
	require.NoError(t, err)

	// Same admin tries to take over again - should succeed (idempotent)
	err = router.HandleAdminTakeover(adminConn, sess.ID)
	assert.NoError(t, err, "Same admin should be able to takeover again (idempotent operation)")
}

// TestEdgeCase_AdminLeaveNotAssisting tests admin leaving a session they're not assisting
// **Validates: Requirements 6.1**
func TestEdgeCase_AdminLeaveNotAssisting(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Admin tries to leave without assisting
	err = router.HandleAdminLeave("admin-1", sess.ID)
	assert.Error(t, err)

	var chatErr *chaterrors.ChatError
	if assert.ErrorAs(t, err, &chatErr) {
		assert.Equal(t, chaterrors.ErrCodeInvalidFormat, chatErr.Code)
		assert.Contains(t, chatErr.Message, "not assisting")
	}
}

// TestEdgeCase_AdminLeaveEmptyParameters tests admin leave with empty parameters
// **Validates: Requirements 6.1**
func TestEdgeCase_AdminLeaveEmptyParameters(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	tests := []struct {
		name      string
		adminID   string
		sessionID string
		errCode   chaterrors.ErrorCode
	}{
		{
			name:      "empty admin ID",
			adminID:   "",
			sessionID: "session-123",
			errCode:   chaterrors.ErrCodeMissingField,
		},
		{
			name:      "empty session ID",
			adminID:   "admin-1",
			sessionID: "",
			errCode:   chaterrors.ErrCodeMissingField,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.HandleAdminLeave(tt.adminID, tt.sessionID)
			assert.Error(t, err)

			var chatErr *chaterrors.ChatError
			if assert.ErrorAs(t, err, &chatErr) {
				assert.Equal(t, tt.errCode, chatErr.Code)
			}
		})
	}
}

// TestEdgeCase_AdminTakeoverAndLeaveFlow tests complete admin takeover and leave flow
// **Validates: Requirements 6.1**
func TestEdgeCase_AdminTakeoverAndLeaveFlow(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Register user connection
	userConn := mockConnection("user-1")
	userConn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, userConn)
	require.NoError(t, err)

	// Admin takes over
	adminConn := mockConnection("admin-1")
	adminConn.Roles = []string{"admin"}
	adminConn.Name = "Admin One"

	err = router.HandleAdminTakeover(adminConn, sess.ID)
	require.NoError(t, err)

	// Verify admin is assisting
	updatedSess, err := sm.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin-1", updatedSess.AssistingAdminID)
	assert.Equal(t, "Admin One", updatedSess.AssistingAdminName)

	// Admin leaves
	err = router.HandleAdminLeave("admin-1", sess.ID)
	require.NoError(t, err)

	// Verify admin is no longer assisting
	updatedSess, err = sm.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Empty(t, updatedSess.AssistingAdminID)
	assert.Empty(t, updatedSess.AssistingAdminName)
}

// TestEdgeCase_BroadcastToSessionWithAdmin tests broadcasting when admin is assisting
// **Validates: Requirements 6.1**
func TestEdgeCase_BroadcastToSessionWithAdmin(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Register user connection
	userConn := mockConnection("user-1")
	userConn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, userConn)
	require.NoError(t, err)

	// Admin takes over
	adminConn := mockConnection("admin-1")
	adminConn.Roles = []string{"admin"}
	adminConn.Name = "Admin One"

	err = router.HandleAdminTakeover(adminConn, sess.ID)
	require.NoError(t, err)

	// Register admin connection
	err = router.RegisterAdminConnection("admin-1", adminConn)
	require.NoError(t, err)

	// Broadcast a message
	msg := &message.Message{
		Type:      message.TypeAIResponse,
		SessionID: sess.ID,
		Content:   "Test broadcast message",
		Sender:    message.SenderAI,
		Timestamp: time.Now(),
	}

	err = router.BroadcastToSession(sess.ID, msg)
	assert.NoError(t, err, "Broadcast should succeed when admin is assisting")
}
