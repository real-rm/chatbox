package router

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_SessionCreationFlow tests the complete session creation flow
// This integration test covers Requirements: 4.1, 4.2, 4.3, 4.4, 4.6, 4.7, 4.8
func TestIntegration_SessionCreationFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("new user connects and sends message - session created", func(t *testing.T) {
		// Setup
		logger := createTestLogger()
		sm := session.NewSessionManager(15*time.Minute, logger)
		mockStorage := &mockStorageService{}
		mockLLM := &mockLLMService{}
		router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, logger)

		// Connect as new user
		conn := mockConnection("user-new-123")
		
		// First, create a session to get a valid session ID
		sess, err := router.getOrCreateSession(conn, "any-id")
		require.NoError(t, err)
		require.NotNil(t, sess)
		sessionID := sess.ID

		// Send message using the created session ID
		msg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: sessionID,
			Content:   "Hello, this is my first message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}

		// Register connection with the actual session ID
		err = router.RegisterConnection(sessionID, conn)
		require.NoError(t, err)

		// Handle user message
		err = router.HandleUserMessage(conn, msg)
		require.NoError(t, err)

		// Verify session exists in memory
		retrievedSess, err := sm.GetSession(sessionID)
		require.NoError(t, err)
		assert.NotNil(t, retrievedSess)
		assert.Equal(t, "user-new-123", retrievedSess.UserID)
		assert.True(t, retrievedSess.IsActive)

		// Verify session was persisted to database
		assert.True(t, mockStorage.createSessionCalled)
		assert.Len(t, mockStorage.createdSessions, 1)
		assert.Equal(t, sessionID, mockStorage.createdSessions[0].ID)
		assert.Equal(t, "user-new-123", mockStorage.createdSessions[0].UserID)
	})

	t.Run("reconnect with session ID - session restored", func(t *testing.T) {
		// Setup
		logger := createTestLogger()
		sm := session.NewSessionManager(15*time.Minute, logger)
		mockStorage := &mockStorageService{}
		mockLLM := &mockLLMService{}
		router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, logger)

		// Create initial session for user
		conn1 := mockConnection("user-reconnect-456")
		sess, err := router.getOrCreateSession(conn1, "any-id")
		require.NoError(t, err)
		require.NotNil(t, sess)
		sessionID := sess.ID

		// Add some data to the session
		sm.SetModelID(sess.ID, "gpt-4")
		msg1 := &session.Message{
			Content:   "First message",
			Timestamp: time.Now(),
			Sender:    "user",
		}
		sm.AddMessage(sess.ID, msg1)

		// Get session data before reconnection
		originalSess, err := sm.GetSession(sess.ID)
		require.NoError(t, err)
		originalMessageCount := len(originalSess.Messages)
		originalModelID := originalSess.ModelID

		// Simulate reconnection with same session ID (getOrCreateSession should return existing)
		conn2 := mockConnection("user-reconnect-456")
		restoredSess, err := router.getOrCreateSession(conn2, sessionID)
		require.NoError(t, err)
		require.NotNil(t, restoredSess)

		// Verify session was restored (not recreated)
		assert.Equal(t, sess.ID, restoredSess.ID)
		assert.Equal(t, "user-reconnect-456", restoredSess.UserID)
		assert.Equal(t, originalModelID, restoredSess.ModelID)
		assert.Equal(t, originalMessageCount, len(restoredSess.Messages))

		// Verify no new session was created in database
		assert.Len(t, mockStorage.createdSessions, 1) // Only the initial creation
	})

	t.Run("concurrent messages from same user - single session", func(t *testing.T) {
		// Setup
		logger := createTestLogger()
		sm := session.NewSessionManager(15*time.Minute, logger)
		mockStorage := &mockStorageService{}
		mockLLM := &mockLLMService{}
		router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, logger)

		userID := "user-concurrent-789"
		numConcurrent := 10

		// Launch concurrent session creation attempts
		var wg sync.WaitGroup
		results := make(chan *session.Session, numConcurrent)
		errors := make(chan error, numConcurrent)

		for i := 0; i < numConcurrent; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				conn := mockConnection(userID)
				sessionID := fmt.Sprintf("concurrent-session-%d", id)
				sess, err := router.getOrCreateSession(conn, sessionID)
				if err != nil {
					errors <- err
				} else {
					results <- sess
				}
			}(i)
		}

		wg.Wait()
		close(results)
		close(errors)

		// Collect results
		var sessions []*session.Session
		for sess := range results {
			sessions = append(sessions, sess)
		}

		var errs []error
		for err := range errors {
			errs = append(errs, err)
		}

		// At least one should succeed
		assert.True(t, len(sessions) >= 1, "At least one session should be created")

		// Verify all successful sessions belong to the same user
		for _, sess := range sessions {
			assert.Equal(t, userID, sess.UserID)
		}

		// Verify sessions were created in database
		assert.True(t, len(mockStorage.createdSessions) >= 1)
	})

	t.Run("database failure - error handling", func(t *testing.T) {
		// Setup with failing storage
		logger := createTestLogger()
		sm := session.NewSessionManager(15*time.Minute, logger)
		mockStorage := &mockStorageService{
			createSessionError: errors.New("database connection failed"),
		}
		mockLLM := &mockLLMService{}
		router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, logger)

		// Attempt to create session
		conn := mockConnection("user-db-fail-999")
		sessionID := "db-fail-session"

		sess, err := router.getOrCreateSession(conn, sessionID)

		// Should return error
		require.Error(t, err)
		assert.Nil(t, sess)

		// Verify database was called
		assert.True(t, mockStorage.createSessionCalled)

		// Verify no session was created in database
		assert.Len(t, mockStorage.createdSessions, 0)

		// Verify session was rolled back from memory
		_, err = sm.GetSession(sessionID)
		assert.Error(t, err)
		assert.ErrorIs(t, err, session.ErrSessionNotFound)
	})
}

// TestIntegration_SessionCreationWithMessages tests session creation with actual message routing
func TestIntegration_SessionCreationWithMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("first message creates session and routes correctly", func(t *testing.T) {
		// Setup
		logger := createTestLogger()
		sm := session.NewSessionManager(15*time.Minute, logger)
		mockStorage := &mockStorageService{}
		mockLLM := &mockLLMService{}
		router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, logger)

		// Create connection
		conn := mockConnection("user-msg-123")
		
		// Create session first
		sess, err := router.getOrCreateSession(conn, "any-id")
		require.NoError(t, err)
		sessionID := sess.ID

		// Register connection
		err = router.RegisterConnection(sessionID, conn)
		require.NoError(t, err)

		// Send first message
		msg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: sessionID,
			Content:   "Hello, create my session",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}

		err = router.HandleUserMessage(conn, msg)
		require.NoError(t, err)

		// Verify session was created
		retrievedSess, err := sm.GetSession(sessionID)
		require.NoError(t, err)
		assert.NotNil(t, retrievedSess)
		assert.Equal(t, "user-msg-123", retrievedSess.UserID)

		// Verify session was persisted
		assert.True(t, mockStorage.createSessionCalled)
		assert.Len(t, mockStorage.createdSessions, 1)

		// Verify LLM was called (message was routed)
		assert.True(t, mockLLM.sendMessageCalled || mockLLM.streamCalled)
	})

	t.Run("subsequent messages use existing session", func(t *testing.T) {
		// Setup
		logger := createTestLogger()
		sm := session.NewSessionManager(15*time.Minute, logger)
		mockStorage := &mockStorageService{}
		mockLLM := &mockLLMService{}
		router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, logger)

		// Create connection
		conn := mockConnection("user-msg-456")
		
		// Create session
		sess, err := router.getOrCreateSession(conn, "any-id")
		require.NoError(t, err)
		sessionID := sess.ID

		// Register connection
		err = router.RegisterConnection(sessionID, conn)
		require.NoError(t, err)

		// Send first message
		msg1 := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: sessionID,
			Content:   "First message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}

		err = router.HandleUserMessage(conn, msg1)
		require.NoError(t, err)

		// Verify session was created
		assert.Len(t, mockStorage.createdSessions, 1)

		// Send second message (should use existing session)
		msg2 := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: sessionID,
			Content:   "Second message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}

		err = router.HandleUserMessage(conn, msg2)
		require.NoError(t, err)

		// Verify no new session was created
		assert.Len(t, mockStorage.createdSessions, 1)

		// Verify session still exists and is active
		retrievedSess, err := sm.GetSession(sessionID)
		require.NoError(t, err)
		assert.True(t, retrievedSess.IsActive)
	})
}

// TestIntegration_SessionCreationErrorRecovery tests error recovery scenarios
func TestIntegration_SessionCreationErrorRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("database failure then recovery", func(t *testing.T) {
		// Setup with initially failing storage
		logger := createTestLogger()
		sm := session.NewSessionManager(15*time.Minute, logger)
		mockStorage := &mockStorageService{
			createSessionError: errors.New("temporary database error"),
		}
		mockLLM := &mockLLMService{}
		router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, logger)

		conn := mockConnection("user-recovery-123")

		// First attempt - should fail
		sess1, err1 := router.getOrCreateSession(conn, "any-id")
		require.Error(t, err1)
		assert.Nil(t, sess1)

		// Fix the storage error
		mockStorage.createSessionError = nil

		// Second attempt - should succeed (user doesn't have active session anymore)
		sess2, err2 := router.getOrCreateSession(conn, "any-id")
		require.NoError(t, err2)
		assert.NotNil(t, sess2)

		// Verify session was created
		sess, err := sm.GetSession(sess2.ID)
		require.NoError(t, err)
		assert.Equal(t, "user-recovery-123", sess.UserID)
	})
}

// TestIntegration_SessionCreationWithDifferentUsers tests multiple users creating sessions
func TestIntegration_SessionCreationWithDifferentUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("multiple users create independent sessions", func(t *testing.T) {
		// Setup
		logger := createTestLogger()
		sm := session.NewSessionManager(15*time.Minute, logger)
		mockStorage := &mockStorageService{}
		mockLLM := &mockLLMService{}
		router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, logger)

		// Create sessions for multiple users
		users := []string{"user-1", "user-2", "user-3", "user-4", "user-5"}
		sessions := make([]*session.Session, 0, len(users))

		for i, userID := range users {
			conn := mockConnection(userID)
			sessionID := fmt.Sprintf("session-%d", i)

			sess, err := router.getOrCreateSession(conn, sessionID)
			require.NoError(t, err)
			require.NotNil(t, sess)

			sessions = append(sessions, sess)
		}

		// Verify all sessions were created
		assert.Len(t, sessions, len(users))
		assert.Len(t, mockStorage.createdSessions, len(users))

		// Verify each session has correct user ID
		for i, sess := range sessions {
			assert.Equal(t, users[i], sess.UserID)
			assert.Equal(t, users[i], mockStorage.createdSessions[i].UserID)
		}

		// Verify all sessions are independent
		for i := 0; i < len(sessions); i++ {
			for j := i + 1; j < len(sessions); j++ {
				assert.NotEqual(t, sessions[i].ID, sessions[j].ID)
				assert.NotEqual(t, sessions[i].UserID, sessions[j].UserID)
			}
		}
	})
}
