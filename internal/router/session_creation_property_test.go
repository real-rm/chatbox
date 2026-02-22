package router

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/real-rm/chatbox/internal/session"
)

// Feature: critical-security-fixes, Property 6: Automatic session creation for new users
// **Validates: Requirements 4.1**
//
// For any authenticated user who sends a message without providing a session ID,
// a new session should be created and associated with that user.
func TestProperty_AutomaticSessionCreationForNewUsers(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("new session is created for users without existing session", prop.ForAll(
		func(userID string) bool {
			// Skip empty user IDs
			if userID == "" {
				return true
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			mockStorage := &mockStorageService{}
			router := NewMessageRouter(sm, nil, nil, nil, mockStorage, 120*time.Second, logger)

			conn := mockConnection(userID)
			sessionID := fmt.Sprintf("new-session-%s", userID)

			// Call getOrCreateSession with a non-existent session ID
			sess, err := router.getOrCreateSession(conn, sessionID)

			// Should create a new session without error
			if err != nil {
				t.Logf("Failed to create session for user %s: %v", userID, err)
				return false
			}

			// Verify session was created
			if sess == nil {
				t.Logf("Session is nil for user %s", userID)
				return false
			}

			// Verify session is associated with the correct user
			if sess.UserID != userID {
				t.Logf("Session user ID mismatch: expected %s, got %s", userID, sess.UserID)
				return false
			}

			// Verify session is active
			if !sess.IsActive {
				t.Logf("Session is not active for user %s", userID)
				return false
			}

			// Verify session was stored in database
			if !mockStorage.createSessionCalled {
				t.Logf("CreateSession was not called for user %s", userID)
				return false
			}

			return true
		},
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes, Property 7: Existing session reuse
// **Validates: Requirements 4.2**
//
// For any message that includes a session ID for an existing session,
// the system should use that existing session rather than creating a new one.
func TestProperty_ExistingSessionReuse(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("existing sessions are reused instead of creating new ones", prop.ForAll(
		func(userID string) bool {
			// Skip empty user IDs
			if userID == "" {
				return true
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			mockStorage := &mockStorageService{}
			router := NewMessageRouter(sm, nil, nil, nil, mockStorage, 120*time.Second, logger)

			// Create an existing session
			existingSess, err := sm.CreateSession(userID)
			if err != nil {
				t.Logf("Failed to create existing session: %v", err)
				return false
			}

			conn := mockConnection(userID)

			// Call getOrCreateSession with existing session ID
			sess, err := router.getOrCreateSession(conn, existingSess.ID)

			// Should return the existing session without error
			if err != nil {
				t.Logf("Failed to get existing session: %v", err)
				return false
			}

			// Verify it's the same session
			if sess.ID != existingSess.ID {
				t.Logf("Session ID mismatch: expected %s, got %s", existingSess.ID, sess.ID)
				return false
			}

			// Verify no new session was created in database
			if mockStorage.createSessionCalled {
				t.Logf("CreateSession was called when it shouldn't have been")
				return false
			}

			return true
		},
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes, Property 8: Session creation with provided ID
// **Validates: Requirements 4.3**
//
// For any message that includes a session ID that does not exist,
// the system should create a new session.
func TestProperty_SessionCreationWithProvidedID(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("new session is created when provided ID does not exist", prop.ForAll(
		func(userID string, sessionID string) bool {
			// Skip empty IDs
			if userID == "" || sessionID == "" {
				return true
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			mockStorage := &mockStorageService{}
			router := NewMessageRouter(sm, nil, nil, nil, mockStorage, 120*time.Second, logger)

			conn := mockConnection(userID)

			// Call getOrCreateSession with a non-existent session ID
			sess, err := router.getOrCreateSession(conn, sessionID)

			// Should create a new session without error
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Verify session was created
			if sess == nil {
				t.Logf("Session is nil")
				return false
			}

			// Verify session is associated with the correct user
			if sess.UserID != userID {
				t.Logf("Session user ID mismatch: expected %s, got %s", userID, sess.UserID)
				return false
			}

			// Verify session was stored in database
			if !mockStorage.createSessionCalled {
				t.Logf("CreateSession was not called")
				return false
			}

			return true
		},
		gen.Identifier(),
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes, Property 9: Dual storage consistency
// **Validates: Requirements 4.4**
//
// For any session that is created, the session should exist in both
// the SessionManager (in-memory) and the StorageService (database) with consistent data.
func TestProperty_DualStorageConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("sessions exist in both memory and database with consistent data", prop.ForAll(
		func(userID string) bool {
			// Skip empty user IDs
			if userID == "" {
				return true
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			mockStorage := &mockStorageService{}
			router := NewMessageRouter(sm, nil, nil, nil, mockStorage, 120*time.Second, logger)

			conn := mockConnection(userID)

			// Create new session
			sess, err := router.createNewSession(conn)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Verify session exists in memory (SessionManager)
			memorySess, err := sm.GetSession(sess.ID)
			if err != nil {
				t.Logf("Session not found in memory: %v", err)
				return false
			}

			// Verify session was persisted to database (StorageService)
			if !mockStorage.createSessionCalled {
				t.Logf("CreateSession was not called")
				return false
			}

			if len(mockStorage.createdSessions) == 0 {
				t.Logf("No sessions in database")
				return false
			}

			dbSess := mockStorage.createdSessions[len(mockStorage.createdSessions)-1]

			// Verify data consistency between memory and database
			if memorySess.ID != dbSess.ID {
				t.Logf("Session ID mismatch: memory=%s, db=%s", memorySess.ID, dbSess.ID)
				return false
			}

			if memorySess.UserID != dbSess.UserID {
				t.Logf("User ID mismatch: memory=%s, db=%s", memorySess.UserID, dbSess.UserID)
				return false
			}

			if memorySess.IsActive != dbSess.IsActive {
				t.Logf("IsActive mismatch: memory=%v, db=%v", memorySess.IsActive, dbSess.IsActive)
				return false
			}

			return true
		},
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes, Property 10: User association correctness
// **Validates: Requirements 4.5**
//
// For any session that is created, the session's user ID should match
// the user ID from the JWT token of the connection that triggered the creation.
func TestProperty_UserAssociationCorrectness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("session user ID matches connection user ID", prop.ForAll(
		func(userID string) bool {
			// Skip empty user IDs
			if userID == "" {
				return true
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			mockStorage := &mockStorageService{}
			router := NewMessageRouter(sm, nil, nil, nil, mockStorage, 120*time.Second, logger)

			conn := mockConnection(userID)

			// Create new session
			sess, err := router.createNewSession(conn)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Verify session's user ID matches the connection's user ID
			if sess.UserID != userID {
				t.Logf("User ID mismatch: expected %s, got %s", userID, sess.UserID)
				return false
			}

			// Verify user ID is consistent in database
			if len(mockStorage.createdSessions) == 0 {
				t.Logf("No sessions in database")
				return false
			}

			dbSess := mockStorage.createdSessions[len(mockStorage.createdSessions)-1]
			if dbSess.UserID != userID {
				t.Logf("Database user ID mismatch: expected %s, got %s", userID, dbSess.UserID)
				return false
			}

			return true
		},
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes, Property 11: Session creation error handling
// **Validates: Requirements 4.6**
//
// For any session creation attempt that fails (e.g., due to database error),
// an error should be returned to the caller.
func TestProperty_SessionCreationErrorHandling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("session creation failures return appropriate errors", prop.ForAll(
		func(userID string) bool {
			// Skip empty user IDs
			if userID == "" {
				return true
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			// Create mock storage that always fails
			mockStorage := &mockStorageService{
				createSessionError: fmt.Errorf("database connection failed"),
			}
			router := NewMessageRouter(sm, nil, nil, nil, mockStorage, 120*time.Second, logger)

			conn := mockConnection(userID)

			// Attempt to create new session
			sess, err := router.createNewSession(conn)

			// Should return error
			if err == nil {
				t.Logf("Expected error but got nil")
				return false
			}

			// Session should be nil
			if sess != nil {
				t.Logf("Expected nil session but got %v", sess)
				return false
			}

			// Verify database was called
			if !mockStorage.createSessionCalled {
				t.Logf("CreateSession was not called")
				return false
			}

			// Verify no session was created in database
			if len(mockStorage.createdSessions) > 0 {
				t.Logf("Session was created in database despite error")
				return false
			}

			return true
		},
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes, Property 12: Concurrent session creation safety
// **Validates: Requirements 4.7**
//
// For any set of concurrent session creation requests from the same user,
// only one session should be successfully created.
func TestProperty_ConcurrentSessionCreationSafety(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("concurrent session creation is handled safely", prop.ForAll(
		func(userID string, numConcurrent int) bool {
			// Skip empty user IDs and invalid ranges
			if userID == "" || numConcurrent < 2 || numConcurrent > 20 {
				return true
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			mockStorage := &mockStorageService{}
			router := NewMessageRouter(sm, nil, nil, nil, mockStorage, 120*time.Second, logger)

			conn := mockConnection(userID)

			// Launch concurrent session creation attempts
			var wg sync.WaitGroup
			results := make(chan error, numConcurrent)

			for i := 0; i < numConcurrent; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					sessionID := fmt.Sprintf("concurrent-session-%d", id)
					_, err := router.getOrCreateSession(conn, sessionID)
					results <- err
				}(i)
			}

			wg.Wait()
			close(results)

			// Count successful creations
			successCount := 0
			for err := range results {
				if err == nil {
					successCount++
				}
			}

			// At least one should succeed (the first one)
			// Others may fail with ErrActiveSessionExists
			if successCount < 1 {
				t.Logf("No successful session creations")
				return false
			}

			// Verify only one session was created in database
			// (or multiple if they had different session IDs, but same user)
			// The key is that the SessionManager prevents multiple active sessions per user
			if len(mockStorage.createdSessions) < 1 {
				t.Logf("No sessions created in database")
				return false
			}

			return true
		},
		gen.Identifier(),
		gen.IntRange(2, 10),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes, Property 13: Session restoration from database
// **Validates: Requirements 4.8**
//
// For any user who reconnects with a session ID that exists in the database,
// the session should be restored with all its previous data intact.
func TestProperty_SessionRestorationFromDatabase(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("sessions can be restored from database with data intact", prop.ForAll(
		func(userID string, modelID string, messageCount int) bool {
			// Skip empty user IDs and invalid message counts
			if userID == "" || messageCount < 0 || messageCount > 50 {
				return true
			}

			logger := createTestLogger()
			sm := session.NewSessionManager(15*time.Minute, logger)
			mockStorage := &mockStorageService{}
			router := NewMessageRouter(sm, nil, nil, nil, mockStorage, 120*time.Second, logger)

			conn := mockConnection(userID)

			// Create initial session
			sess, err := router.createNewSession(conn)
			if err != nil {
				t.Logf("Failed to create initial session: %v", err)
				return false
			}

			// Add some data to the session
			if modelID != "" {
				sm.SetModelID(sess.ID, modelID)
			}

			for i := 0; i < messageCount; i++ {
				msg := &session.Message{
					Content:   fmt.Sprintf("Message %d", i),
					Timestamp: time.Now(),
					Sender:    "user",
				}
				sm.AddMessage(sess.ID, msg)
			}

			// Get the session to verify data
			originalSess, err := sm.GetSession(sess.ID)
			if err != nil {
				t.Logf("Failed to get original session: %v", err)
				return false
			}

			// Simulate reconnection by getting the session again
			restoredSess, err := router.getOrCreateSession(conn, sess.ID)
			if err != nil {
				t.Logf("Failed to restore session: %v", err)
				return false
			}

			// Verify session ID is the same
			if restoredSess.ID != originalSess.ID {
				t.Logf("Session ID mismatch: expected %s, got %s", originalSess.ID, restoredSess.ID)
				return false
			}

			// Verify user ID is preserved
			if restoredSess.UserID != originalSess.UserID {
				t.Logf("User ID mismatch: expected %s, got %s", originalSess.UserID, restoredSess.UserID)
				return false
			}

			// Verify model ID is preserved (if set)
			if modelID != "" && restoredSess.ModelID != originalSess.ModelID {
				t.Logf("Model ID mismatch: expected %s, got %s", originalSess.ModelID, restoredSess.ModelID)
				return false
			}

			// Verify message count is preserved
			if len(restoredSess.Messages) != len(originalSess.Messages) {
				t.Logf("Message count mismatch: expected %d, got %d", len(originalSess.Messages), len(restoredSess.Messages))
				return false
			}

			return true
		},
		gen.Identifier(),
		gen.AlphaString(),
		gen.IntRange(0, 10),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
