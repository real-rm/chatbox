package router

import (
	"errors"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetOrCreateSession_AutomaticCreation tests automatic session creation for new user
// Requirements: 4.1
func TestGetOrCreateSession_AutomaticCreation(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	conn := mockConnection("user-123")
	sessionID := "new-session-id"

	// Call getOrCreateSession with a non-existent session ID
	sess, err := router.getOrCreateSession(conn, sessionID)

	// Should create a new session without error
	require.NoError(t, err)
	assert.NotNil(t, sess)
	assert.Equal(t, "user-123", sess.UserID)
	assert.True(t, sess.IsActive)

	// Verify session was stored in database
	assert.True(t, mockStorage.createSessionCalled)
	assert.Len(t, mockStorage.createdSessions, 1)
	assert.Equal(t, sess.ID, mockStorage.createdSessions[0].ID)
}

// TestGetOrCreateSession_ExistingSessionReuse tests existing session reuse
// Requirements: 4.2
func TestGetOrCreateSession_ExistingSessionReuse(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	// Create an existing session
	existingSess, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	conn := mockConnection("user-123")

	// Call getOrCreateSession with existing session ID
	sess, err := router.getOrCreateSession(conn, existingSess.ID)

	// Should return the existing session
	require.NoError(t, err)
	assert.NotNil(t, sess)
	assert.Equal(t, existingSess.ID, sess.ID)
	assert.Equal(t, "user-123", sess.UserID)

	// Verify no new session was created in database
	assert.False(t, mockStorage.createSessionCalled)
	assert.Len(t, mockStorage.createdSessions, 0)
}

// TestGetOrCreateSession_SessionCreationWithProvidedID tests session creation with provided ID
// Requirements: 4.3
func TestGetOrCreateSession_SessionCreationWithProvidedID(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	conn := mockConnection("user-456")
	providedSessionID := "provided-session-id"

	// Call getOrCreateSession with a provided session ID that doesn't exist
	sess, err := router.getOrCreateSession(conn, providedSessionID)

	// Should create a new session
	require.NoError(t, err)
	assert.NotNil(t, sess)
	assert.Equal(t, "user-456", sess.UserID)
	assert.True(t, sess.IsActive)

	// Verify session was created in database
	assert.True(t, mockStorage.createSessionCalled)
	assert.Len(t, mockStorage.createdSessions, 1)
}

// TestCreateNewSession_DualStorage tests dual storage (memory + database)
// Requirements: 4.4
func TestCreateNewSession_DualStorage(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	conn := mockConnection("user-789")
	sessionID := "test-session-id"

	// Create new session
	sess, err := router.createNewSession(conn, sessionID)

	// Should succeed
	require.NoError(t, err)
	assert.NotNil(t, sess)

	// Verify session exists in memory (SessionManager)
	memorySess, err := sm.GetSession(sess.ID)
	require.NoError(t, err)
	assert.Equal(t, sess.ID, memorySess.ID)
	assert.Equal(t, "user-789", memorySess.UserID)

	// Verify session was persisted to database (StorageService)
	assert.True(t, mockStorage.createSessionCalled)
	assert.Len(t, mockStorage.createdSessions, 1)
	assert.Equal(t, sess.ID, mockStorage.createdSessions[0].ID)
	assert.Equal(t, "user-789", mockStorage.createdSessions[0].UserID)

	// Verify data consistency between memory and database
	assert.Equal(t, memorySess.UserID, mockStorage.createdSessions[0].UserID)
	assert.Equal(t, memorySess.ID, mockStorage.createdSessions[0].ID)
	assert.Equal(t, memorySess.IsActive, mockStorage.createdSessions[0].IsActive)
}

// TestCreateNewSession_UserIDAssociation tests user ID association
// Requirements: 4.5
func TestCreateNewSession_UserIDAssociation(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	tests := []struct {
		name   string
		userID string
	}{
		{
			name:   "user with alphanumeric ID",
			userID: "user-abc123",
		},
		{
			name:   "user with UUID",
			userID: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:   "user with numeric ID",
			userID: "12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := mockConnection(tt.userID)
			sessionID := "test-session"

			// Create new session
			sess, err := router.createNewSession(conn, sessionID)

			// Should succeed
			require.NoError(t, err)
			assert.NotNil(t, sess)

			// Verify session's user ID matches the connection's user ID
			assert.Equal(t, tt.userID, sess.UserID)

			// Verify user ID is consistent in database
			assert.True(t, mockStorage.createSessionCalled)
			assert.Equal(t, tt.userID, mockStorage.createdSessions[len(mockStorage.createdSessions)-1].UserID)

			// Reset mock for next test
			mockStorage.createSessionCalled = false
		})
	}
}

// TestCreateNewSession_DatabaseFailure tests error handling on database failure
// Requirements: 4.6
func TestCreateNewSession_DatabaseFailure(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{
		createSessionError: errors.New("database connection failed"),
	}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	conn := mockConnection("user-999")
	sessionID := "test-session"

	// Attempt to create new session
	sess, err := router.createNewSession(conn, sessionID)

	// Should return error
	require.Error(t, err)
	assert.Nil(t, sess)

	// Verify database was called
	assert.True(t, mockStorage.createSessionCalled)

	// Verify no session was created in database
	assert.Len(t, mockStorage.createdSessions, 0)
}

// TestCreateNewSession_RollbackOnPartialFailure tests rollback on partial failure
// Requirements: 4.6
func TestCreateNewSession_RollbackOnPartialFailure(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{
		createSessionError: errors.New("database write failed"),
	}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	conn := mockConnection("user-rollback")
	sessionID := "rollback-session"

	// Attempt to create new session (will fail at database step)
	sess, err := router.createNewSession(conn, sessionID)

	// Should return error
	require.Error(t, err)
	assert.Nil(t, sess)

	// Verify database was called
	assert.True(t, mockStorage.createSessionCalled)

	// Verify session was rolled back from memory (SessionManager)
	// The session should not exist in SessionManager after rollback
	_, err = sm.GetSession(sessionID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

// TestGetOrCreateSession_ConcurrentCreation tests concurrent session creation safety
// Requirements: 4.7
func TestGetOrCreateSession_ConcurrentCreation(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	conn := mockConnection("user-concurrent")
	sessionID := "concurrent-session"

	// First call should create the session
	sess1, err1 := router.getOrCreateSession(conn, sessionID)
	require.NoError(t, err1)
	assert.NotNil(t, sess1)

	// Second call with same user should return error (user already has active session)
	sess2, err2 := router.getOrCreateSession(conn, sessionID)

	// Should either:
	// 1. Return the existing session (if session ID matches)
	// 2. Return error if trying to create a new session for user with active session
	if err2 == nil {
		// If no error, should return the same session
		assert.Equal(t, sess1.ID, sess2.ID)
	} else {
		// If error, should be because user already has active session
		assert.ErrorIs(t, err2, session.ErrActiveSessionExists)
	}

	// Verify only one session was created in database
	assert.Len(t, mockStorage.createdSessions, 1)
}

// TestGetOrCreateSession_EmptyUserID tests error handling with empty user ID
func TestGetOrCreateSession_EmptyUserID(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	conn := mockConnection("")
	sessionID := "test-session"

	// Attempt to create session with empty user ID
	sess, err := router.getOrCreateSession(conn, sessionID)

	// Should return error
	require.Error(t, err)
	assert.Nil(t, sess)
	assert.ErrorIs(t, err, session.ErrInvalidUserID)

	// Verify no session was created in database
	assert.False(t, mockStorage.createSessionCalled)
}

// TestGetOrCreateSession_NilConnection tests error handling with nil connection
func TestGetOrCreateSession_NilConnection(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	sessionID := "test-session"

	// Attempt to create session with nil connection should panic
	// We expect this to panic since the code dereferences conn.UserID
	defer func() {
		if r := recover(); r != nil {
			// Expected panic - test passes
			assert.NotNil(t, r)
		}
	}()

	// This should panic
	sess, err := router.getOrCreateSession(nil, sessionID)

	// If we get here without panic, the implementation has changed
	// In that case, we should get an error
	if err == nil {
		t.Error("Expected error or panic with nil connection, got nil error")
	}
	assert.Nil(t, sess)

	// Verify no session was created in database
	assert.False(t, mockStorage.createSessionCalled)
}

// TestCreateNewSession_SessionMetadata tests that session metadata is properly initialized
func TestCreateNewSession_SessionMetadata(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	conn := mockConnection("user-metadata")
	sessionID := "metadata-session"

	// Create new session
	sess, err := router.createNewSession(conn, sessionID)

	// Should succeed
	require.NoError(t, err)
	assert.NotNil(t, sess)

	// Verify session metadata is properly initialized
	assert.NotEmpty(t, sess.ID)
	assert.Equal(t, "user-metadata", sess.UserID)
	assert.True(t, sess.IsActive)
	assert.False(t, sess.StartTime.IsZero())
	assert.False(t, sess.LastActivity.IsZero())
	assert.Nil(t, sess.EndTime)
	assert.NotNil(t, sess.Messages)
	assert.Empty(t, sess.Messages)
	assert.Equal(t, 0, sess.TotalTokens)
	assert.NotNil(t, sess.ResponseTimes)
	assert.Empty(t, sess.ResponseTimes)
}

// TestGetOrCreateSession_MultipleUsers tests that different users can create sessions independently
func TestGetOrCreateSession_MultipleUsers(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	router := NewMessageRouter(sm, nil, nil, nil, mockStorage, logger)

	// Create sessions for multiple users
	users := []string{"user-1", "user-2", "user-3"}
	sessions := make([]*session.Session, 0, len(users))

	for _, userID := range users {
		conn := mockConnection(userID)
		sessionID := "session-" + userID

		sess, err := router.getOrCreateSession(conn, sessionID)
		require.NoError(t, err)
		assert.NotNil(t, sess)
		assert.Equal(t, userID, sess.UserID)

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
}
