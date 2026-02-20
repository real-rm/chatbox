package session

import (
	"testing"
	"time"

	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestLogger creates a logger for testing
func getTestLogger() *golog.Logger {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            "/tmp/chatbox-test-logs", // Use temp directory for test logs
		Level:          "error",                  // Only log errors in tests
		StandardOutput: false,                    // Don't output to console during tests
	})
	if err != nil {
		panic("Failed to initialize test logger: " + err.Error())
	}
	return logger
}

func TestNewSessionManager(t *testing.T) {
	timeout := 15 * time.Minute
	logger := getTestLogger()
	sm := NewSessionManager(timeout, logger)

	require.NotNil(t, sm)
	assert.Equal(t, timeout, sm.reconnectTimeout)
	assert.NotNil(t, sm.sessions)
	assert.NotNil(t, sm.userSessions)
}

func TestCreateSession_ValidUser(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	session, err := sm.CreateSession("user-123")

	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "user-123", session.UserID)
	assert.NotEmpty(t, session.ID)
	assert.True(t, session.IsActive)
	assert.False(t, session.StartTime.IsZero())
	assert.False(t, session.LastActivity.IsZero())
	assert.NotNil(t, session.Messages)
	assert.Empty(t, session.Messages)
	assert.Equal(t, 0, session.TotalTokens)
	assert.NotNil(t, session.ResponseTimes)
	assert.Empty(t, session.ResponseTimes)
}

func TestCreateSession_EmptyUserID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	session, err := sm.CreateSession("")

	require.Error(t, err)
	assert.Nil(t, session)
	assert.Contains(t, err.Error(), "user ID")
}

func TestCreateSession_SingleActiveSessionConstraint(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create first session
	session1, err := sm.CreateSession("user-123")
	require.NoError(t, err)
	require.NotNil(t, session1)

	// Attempt to create second session for same user
	session2, err := sm.CreateSession("user-123")

	require.Error(t, err)
	assert.Nil(t, session2)
	assert.Contains(t, err.Error(), "active session")
}

func TestCreateSession_AfterEndingPreviousSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create first session
	session1, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// End the session
	err = sm.EndSession(session1.ID)
	require.NoError(t, err)

	// Should be able to create new session now
	session2, err := sm.CreateSession("user-123")
	require.NoError(t, err)
	require.NotNil(t, session2)
	assert.NotEqual(t, session1.ID, session2.ID)
}

func TestGetSession_ExistingSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	created, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	retrieved, err := sm.GetSession(created.ID)

	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, created.UserID, retrieved.UserID)
}

func TestGetSession_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	session, err := sm.GetSession("non-existent-id")

	require.Error(t, err)
	assert.Nil(t, session)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetSession_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	session, err := sm.GetSession("")

	require.Error(t, err)
	assert.Nil(t, session)
	assert.Contains(t, err.Error(), "session ID")
}

func TestRestoreSession_WithinTimeout(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	created, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Simulate disconnection by ending session
	err = sm.EndSession(created.ID)
	require.NoError(t, err)

	// Restore within timeout
	restored, err := sm.RestoreSession("user-123", created.ID)

	require.NoError(t, err)
	require.NotNil(t, restored)
	assert.Equal(t, created.ID, restored.ID)
	assert.True(t, restored.IsActive)
}

func TestRestoreSession_AfterTimeout(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(100*time.Millisecond, logger) // Very short timeout for testing

	// Create session
	created, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// End session
	err = sm.EndSession(created.ID)
	require.NoError(t, err)

	// Wait for timeout to expire
	time.Sleep(150 * time.Millisecond)

	// Attempt to restore after timeout
	restored, err := sm.RestoreSession("user-123", created.ID)

	require.Error(t, err)
	assert.Nil(t, restored)
	assert.Contains(t, err.Error(), "timeout")
}

func TestRestoreSession_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	restored, err := sm.RestoreSession("user-123", "non-existent-id")

	require.Error(t, err)
	assert.Nil(t, restored)
	assert.Contains(t, err.Error(), "not found")
}

func TestRestoreSession_WrongUser(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session for user-123
	created, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// End session
	err = sm.EndSession(created.ID)
	require.NoError(t, err)

	// Attempt to restore with different user
	restored, err := sm.RestoreSession("user-456", created.ID)

	require.Error(t, err)
	assert.Nil(t, restored)
	assert.Contains(t, err.Error(), "does not belong")
}

func TestEndSession_ExistingSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	err = sm.EndSession(session.ID)

	require.NoError(t, err)

	// Verify session is marked as inactive
	retrieved, err := sm.GetSession(session.ID)
	require.NoError(t, err)
	assert.False(t, retrieved.IsActive)
	assert.NotNil(t, retrieved.EndTime)
}

func TestEndSession_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.EndSession("non-existent-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEndSession_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.EndSession("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID")
}

func TestSessionTimeout_DefaultValue(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	assert.Equal(t, 15*time.Minute, sm.reconnectTimeout)
}

func TestSessionTimeout_CustomValue(t *testing.T) {
	customTimeout := 30 * time.Minute
	logger := getTestLogger()
	sm := NewSessionManager(customTimeout, logger)

	assert.Equal(t, customTimeout, sm.reconnectTimeout)
}

func TestUserToSessionMapping(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session for user-123
	session1, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Verify mapping exists
	sm.mu.RLock()
	mappedSessionID, exists := sm.userSessions["user-123"]
	sm.mu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, session1.ID, mappedSessionID)
}

func TestUserToSessionMapping_RemovedAfterEnd(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// End session
	err = sm.EndSession(session.ID)
	require.NoError(t, err)

	// Verify mapping is removed
	sm.mu.RLock()
	_, exists := sm.userSessions["user-123"]
	sm.mu.RUnlock()

	assert.False(t, exists)
}

func TestSession_InitialState(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Verify all initial fields
	assert.NotEmpty(t, session.ID)
	assert.Equal(t, "user-123", session.UserID)
	assert.Empty(t, session.Name)
	assert.Empty(t, session.ModelID)
	assert.NotNil(t, session.Messages)
	assert.Empty(t, session.Messages)
	assert.False(t, session.StartTime.IsZero())
	assert.False(t, session.LastActivity.IsZero())
	assert.Nil(t, session.EndTime)
	assert.True(t, session.IsActive)
	assert.False(t, session.AdminAssisted)
	assert.Empty(t, session.AssistingAdminID)
	assert.Empty(t, session.AssistingAdminName)
	assert.False(t, session.HelpRequested)
	assert.Equal(t, 0, session.TotalTokens)
	assert.NotNil(t, session.ResponseTimes)
	assert.Empty(t, session.ResponseTimes)
}

func TestConcurrentSessionCreation(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create sessions for different users concurrently
	done := make(chan bool, 3)

	for i := 1; i <= 3; i++ {
		userID := "user-" + string(rune('0'+i))
		go func(uid string) {
			session, err := sm.CreateSession(uid)
			assert.NoError(t, err)
			assert.NotNil(t, session)
			done <- true
		}(userID)
	}

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify all sessions were created
	sm.mu.RLock()
	assert.Len(t, sm.sessions, 3)
	assert.Len(t, sm.userSessions, 3)
	sm.mu.RUnlock()
}

func TestConcurrentGetSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create a session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Read session concurrently
	done := make(chan bool, 5)

	for i := 0; i < 5; i++ {
		go func() {
			retrieved, err := sm.GetSession(session.ID)
			assert.NoError(t, err)
			assert.NotNil(t, retrieved)
			assert.Equal(t, session.ID, retrieved.ID)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestRestoreSession_ReactivatesSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create and end session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	err = sm.EndSession(session.ID)
	require.NoError(t, err)

	// Verify session is inactive
	retrieved, err := sm.GetSession(session.ID)
	require.NoError(t, err)
	assert.False(t, retrieved.IsActive)

	// Restore session
	restored, err := sm.RestoreSession("user-123", session.ID)
	require.NoError(t, err)

	// Verify session is active again
	assert.True(t, restored.IsActive)
	assert.False(t, restored.LastActivity.IsZero())
}

func TestRestoreSession_UpdatesUserMapping(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create and end session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	err = sm.EndSession(session.ID)
	require.NoError(t, err)

	// Verify mapping is removed
	sm.mu.RLock()
	_, exists := sm.userSessions["user-123"]
	sm.mu.RUnlock()
	assert.False(t, exists)

	// Restore session
	_, err = sm.RestoreSession("user-123", session.ID)
	require.NoError(t, err)

	// Verify mapping is restored
	sm.mu.RLock()
	mappedSessionID, exists := sm.userSessions["user-123"]
	sm.mu.RUnlock()
	assert.True(t, exists)
	assert.Equal(t, session.ID, mappedSessionID)
}

// TestGenerateSessionName tests session name generation from first message
func TestGenerateSessionName(t *testing.T) {
	tests := []struct {
		name         string
		firstMessage string
		expectedName string
		maxLength    int
	}{
		{
			name:         "short message",
			firstMessage: "Hello world",
			expectedName: "Hello world",
			maxLength:    50,
		},
		{
			name:         "long message gets truncated",
			firstMessage: "This is a very long message that should be truncated to fit within the maximum length limit",
			expectedName: "This is a very long message that should be...",
			maxLength:    50,
		},
		{
			name:         "message with multiple sentences uses first sentence",
			firstMessage: "What is the weather today? I need to know if I should bring an umbrella.",
			expectedName: "What is the weather today?",
			maxLength:    50,
		},
		{
			name:         "empty message returns default",
			firstMessage: "",
			expectedName: "New Chat",
			maxLength:    50,
		},
		{
			name:         "whitespace only returns default",
			firstMessage: "   \n\t  ",
			expectedName: "New Chat",
			maxLength:    50,
		},
		{
			name:         "message with newlines uses first line",
			firstMessage: "First line here\nSecond line here\nThird line here",
			expectedName: "First line here",
			maxLength:    50,
		},
		{
			name:         "message with special characters",
			firstMessage: "How do I use @mentions and #hashtags?",
			expectedName: "How do I use @mentions and #hashtags?",
			maxLength:    50,
		},
		{
			name:         "very short max length",
			firstMessage: "Hello world",
			expectedName: "Hello...",
			maxLength:    10,
		},
		{
			name:         "message exactly at max length",
			firstMessage: "Exactly fifty characters in this message here!",
			expectedName: "Exactly fifty characters in this message here!",
			maxLength:    50,
		},
		{
			// When maxLength <= len("...") == 3, GenerateSessionName returns
			// just the ellipsis without trying to truncate.
			name:         "maxLength at ellipsis boundary returns ellipsis only",
			firstMessage: "Hello world",
			expectedName: "...",
			maxLength:    3,
		},
		{
			// Long word with no spaces — truncateAtWordBoundary falls back to
			// hard truncation at maxLen since no space is found.
			name:         "long word with no spaces hard-truncates",
			firstMessage: "Supercalifragilisticexpialidocious",
			expectedName: "Supercalif...",
			maxLength:    13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSessionName(tt.firstMessage, tt.maxLength)
			assert.Equal(t, tt.expectedName, result)
			assert.LessOrEqual(t, len(result), tt.maxLength)
		})
	}
}

func TestGenerateSessionName_DefaultMaxLength(t *testing.T) {
	// Test with default max length (should be 50)
	longMessage := "This is a very long message that should be truncated to fit within the default maximum length limit"
	result := GenerateSessionName(longMessage, 50)

	assert.LessOrEqual(t, len(result), 50)
	assert.Contains(t, result, "...")
}

func TestSetSessionNameFromMessage(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)
	require.Empty(t, session.Name)

	// Set name from first message
	err = sm.SetSessionNameFromMessage(session.ID, "What is the weather today?")
	require.NoError(t, err)

	// Verify name was set
	retrieved, err := sm.GetSession(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "What is the weather today?", retrieved.Name)
}

func TestSetSessionNameFromMessage_OnlyFirstMessage(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Set name from first message
	err = sm.SetSessionNameFromMessage(session.ID, "First message")
	require.NoError(t, err)

	// Try to set name again with different message
	err = sm.SetSessionNameFromMessage(session.ID, "Second message should not change name")
	require.NoError(t, err)

	// Verify name is still from first message
	retrieved, err := sm.GetSession(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "First message", retrieved.Name)
}

func TestSetSessionNameFromMessage_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.SetSessionNameFromMessage("non-existent-id", "Test message")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSetSessionNameFromMessage_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.SetSessionNameFromMessage("", "Test message")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID")
}

// TestUpdateTokenUsage tests updating token usage for a session
func TestUpdateTokenUsage(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)
	require.Equal(t, 0, session.TotalTokens)

	// Update token usage
	err = sm.UpdateTokenUsage(session.ID, 100)
	require.NoError(t, err)

	// Verify tokens were added
	retrieved, err := sm.GetSession(session.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, retrieved.TotalTokens)

	// Update again
	err = sm.UpdateTokenUsage(session.ID, 50)
	require.NoError(t, err)

	// Verify tokens were accumulated
	retrieved, err = sm.GetSession(session.ID)
	require.NoError(t, err)
	assert.Equal(t, 150, retrieved.TotalTokens)
}

func TestUpdateTokenUsage_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.UpdateTokenUsage("non-existent-id", 100)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateTokenUsage_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.UpdateTokenUsage("", 100)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID")
}

func TestUpdateTokenUsage_NegativeTokens(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	err = sm.UpdateTokenUsage(session.ID, -50)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative")
}

// TestRecordResponseTime tests recording response times for a session
func TestRecordResponseTime(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)
	require.Empty(t, session.ResponseTimes)

	// Record response time
	duration := 500 * time.Millisecond
	err = sm.RecordResponseTime(session.ID, duration)
	require.NoError(t, err)

	// Verify response time was recorded
	retrieved, err := sm.GetSession(session.ID)
	require.NoError(t, err)
	assert.Len(t, retrieved.ResponseTimes, 1)
	assert.Equal(t, duration, retrieved.ResponseTimes[0])

	// Record another response time
	duration2 := 750 * time.Millisecond
	err = sm.RecordResponseTime(session.ID, duration2)
	require.NoError(t, err)

	// Verify both response times are recorded
	retrieved, err = sm.GetSession(session.ID)
	require.NoError(t, err)
	assert.Len(t, retrieved.ResponseTimes, 2)
	assert.Equal(t, duration, retrieved.ResponseTimes[0])
	assert.Equal(t, duration2, retrieved.ResponseTimes[1])
}

func TestRecordResponseTime_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.RecordResponseTime("non-existent-id", 500*time.Millisecond)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRecordResponseTime_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.RecordResponseTime("", 500*time.Millisecond)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID")
}

func TestRecordResponseTime_NegativeDuration(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	err = sm.RecordResponseTime(session.ID, -100*time.Millisecond)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative")
}

// TestGetMaxResponseTime tests calculating maximum response time
func TestGetMaxResponseTime(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Record multiple response times
	times := []time.Duration{
		300 * time.Millisecond,
		750 * time.Millisecond,
		500 * time.Millisecond,
		1200 * time.Millisecond,
		400 * time.Millisecond,
	}

	for _, duration := range times {
		err = sm.RecordResponseTime(session.ID, duration)
		require.NoError(t, err)
	}

	// Get max response time
	maxTime, err := sm.GetMaxResponseTime(session.ID)
	require.NoError(t, err)
	assert.Equal(t, 1200*time.Millisecond, maxTime)
}

func TestGetMaxResponseTime_NoResponseTimes(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session with no response times
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	maxTime, err := sm.GetMaxResponseTime(session.ID)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), maxTime)
}

func TestGetMaxResponseTime_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	_, err := sm.GetMaxResponseTime("non-existent-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetMaxResponseTime_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	_, err := sm.GetMaxResponseTime("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID")
}

// TestGetAverageResponseTime tests calculating average response time
func TestGetAverageResponseTime(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Record multiple response times
	times := []time.Duration{
		300 * time.Millisecond,
		500 * time.Millisecond,
		700 * time.Millisecond,
	}

	for _, duration := range times {
		err = sm.RecordResponseTime(session.ID, duration)
		require.NoError(t, err)
	}

	// Get average response time
	avgTime, err := sm.GetAverageResponseTime(session.ID)
	require.NoError(t, err)

	// Average should be (300 + 500 + 700) / 3 = 500ms
	assert.Equal(t, 500*time.Millisecond, avgTime)
}

func TestGetAverageResponseTime_NoResponseTimes(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session with no response times
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	avgTime, err := sm.GetAverageResponseTime(session.ID)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), avgTime)
}

func TestGetAverageResponseTime_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	_, err := sm.GetAverageResponseTime("non-existent-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetAverageResponseTime_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	_, err := sm.GetAverageResponseTime("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID")
}

// TestGetSessionDuration tests calculating session duration
func TestGetSessionDuration(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// End session
	err = sm.EndSession(session.ID)
	require.NoError(t, err)

	// Get duration
	duration, err := sm.GetSessionDuration(session.ID)
	require.NoError(t, err)

	// Duration should be at least 100ms
	assert.GreaterOrEqual(t, duration, 100*time.Millisecond)
}

func TestGetSessionDuration_ActiveSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Get duration for active session (should calculate from start to now)
	duration, err := sm.GetSessionDuration(session.ID)
	require.NoError(t, err)

	// Duration should be at least 50ms
	assert.GreaterOrEqual(t, duration, 50*time.Millisecond)
}

func TestGetSessionDuration_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	_, err := sm.GetSessionDuration("non-existent-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetSessionDuration_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	_, err := sm.GetSessionDuration("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID")
}

// TestConcurrentMetricsUpdates tests concurrent updates to session metrics
func TestConcurrentMetricsUpdates(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Update metrics concurrently
	done := make(chan bool, 20)

	// 10 goroutines updating tokens
	for i := 0; i < 10; i++ {
		go func() {
			err := sm.UpdateTokenUsage(session.ID, 10)
			assert.NoError(t, err)
			done <- true
		}()
	}

	// 10 goroutines recording response times
	for i := 0; i < 10; i++ {
		go func(idx int) {
			duration := time.Duration(100+idx*50) * time.Millisecond
			err := sm.RecordResponseTime(session.ID, duration)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify final state
	retrieved, err := sm.GetSession(session.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, retrieved.TotalTokens) // 10 * 10
	assert.Len(t, retrieved.ResponseTimes, 10)
}

// TestSetModelID tests setting the model ID for a session
func TestSetModelID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create a session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Set model ID
	err = sm.SetModelID(session.ID, "gpt-4")
	require.NoError(t, err)

	// Verify model ID was set
	modelID, err := sm.GetModelID(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4", modelID)
}

func TestSetModelID_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.SetModelID("", "gpt-4")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID")
}

func TestSetModelID_EmptyModelID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	err = sm.SetModelID(session.ID, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "model ID")
}

func TestSetModelID_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	err := sm.SetModelID("non-existent-session", "gpt-4")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSetModelID_UpdateExistingModel(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create a session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Set initial model
	err = sm.SetModelID(session.ID, "gpt-3.5-turbo")
	require.NoError(t, err)

	// Update to different model
	err = sm.SetModelID(session.ID, "gpt-4")
	require.NoError(t, err)

	// Verify model was updated
	modelID, err := sm.GetModelID(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4", modelID)
}

func TestGetModelID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create a session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Set model ID
	err = sm.SetModelID(session.ID, "claude-3")
	require.NoError(t, err)

	// Get model ID
	modelID, err := sm.GetModelID(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "claude-3", modelID)
}

func TestGetModelID_EmptySessionID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	modelID, err := sm.GetModelID("")

	require.Error(t, err)
	assert.Empty(t, modelID)
	assert.Contains(t, err.Error(), "session ID")
}

func TestGetModelID_NonExistentSession(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	modelID, err := sm.GetModelID("non-existent-session")

	require.Error(t, err)
	assert.Empty(t, modelID)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetModelID_NoModelSet(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create a session without setting model
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Get model ID should return empty string
	modelID, err := sm.GetModelID(session.ID)
	require.NoError(t, err)
	assert.Empty(t, modelID)
}

func TestModelSelection_Persistence(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Create a session
	session, err := sm.CreateSession("user-123")
	require.NoError(t, err)

	// Set model ID
	err = sm.SetModelID(session.ID, "gpt-4")
	require.NoError(t, err)

	// End the session
	err = sm.EndSession(session.ID)
	require.NoError(t, err)

	// Restore the session
	restored, err := sm.RestoreSession("user-123", session.ID)
	require.NoError(t, err)

	// Model ID should still be set
	assert.Equal(t, "gpt-4", restored.ModelID)
}
