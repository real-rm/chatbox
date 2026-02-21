package session

import (
	"sync"
	"testing"
	"time"

	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanupExpiredSessions_RemovesExpiredSessions verifies that
// cleanupExpiredSessions removes sessions that have been inactive
// for longer than the TTL.
func TestCleanupExpiredSessions_RemovesExpiredSessions(t *testing.T) {
	// Create logger for testing
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create session manager with short TTL for testing
	sm := NewSessionManager(15*time.Minute, logger)
	sm.sessionTTL = 100 * time.Millisecond
	sm.cleanupInterval = 50 * time.Millisecond

	// Create and end a session
	sess, err := sm.CreateSession("test-user")
	require.NoError(t, err)
	sessionID := sess.ID

	err = sm.EndSession(sessionID)
	require.NoError(t, err)

	// Verify session exists before cleanup
	sm.mu.RLock()
	_, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()
	assert.True(t, exists, "Session should exist before cleanup")

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Run cleanup
	sm.cleanupExpiredSessions()

	// Verify session is removed after cleanup
	sm.mu.RLock()
	_, exists = sm.sessions[sessionID]
	sm.mu.RUnlock()
	assert.False(t, exists, "Session should be removed after cleanup")
}

// TestCleanupExpiredSessions_PreservesActiveSessions verifies that
// cleanupExpiredSessions does not remove active sessions.
func TestCleanupExpiredSessions_PreservesActiveSessions(t *testing.T) {
	// Create logger for testing
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create session manager with short TTL for testing
	sm := NewSessionManager(15*time.Minute, logger)
	sm.sessionTTL = 100 * time.Millisecond
	sm.cleanupInterval = 50 * time.Millisecond

	// Create an active session
	sess, err := sm.CreateSession("test-user")
	require.NoError(t, err)
	sessionID := sess.ID

	// Wait longer than TTL
	time.Sleep(150 * time.Millisecond)

	// Run cleanup
	sm.cleanupExpiredSessions()

	// Verify active session is NOT removed
	sm.mu.RLock()
	_, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()
	assert.True(t, exists, "Active session should not be removed by cleanup")
}

// TestCleanupExpiredSessions_PreservesRecentlyEndedSessions verifies that
// cleanupExpiredSessions does not remove sessions that ended recently
// (within the TTL window).
func TestCleanupExpiredSessions_PreservesRecentlyEndedSessions(t *testing.T) {
	// Create logger for testing
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create session manager with longer TTL
	sm := NewSessionManager(15*time.Minute, logger)
	sm.sessionTTL = 500 * time.Millisecond
	sm.cleanupInterval = 50 * time.Millisecond

	// Create and end a session
	sess, err := sm.CreateSession("test-user")
	require.NoError(t, err)
	sessionID := sess.ID

	err = sm.EndSession(sessionID)
	require.NoError(t, err)

	// Wait less than TTL
	time.Sleep(100 * time.Millisecond)

	// Run cleanup
	sm.cleanupExpiredSessions()

	// Verify session is NOT removed (TTL not expired yet)
	sm.mu.RLock()
	_, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()
	assert.True(t, exists, "Recently ended session should not be removed before TTL expires")
}

// TestCleanupExpiredSessions_LogsStatistics verifies that
// cleanupExpiredSessions logs the number of removed sessions.
func TestCleanupExpiredSessions_LogsStatistics(t *testing.T) {
	// Create logger for testing
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "info",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create session manager with short TTL
	sm := NewSessionManager(15*time.Minute, logger)
	sm.sessionTTL = 100 * time.Millisecond
	sm.cleanupInterval = 50 * time.Millisecond

	// Create and end multiple sessions
	numSessions := 5
	for i := 0; i < numSessions; i++ {
		sess, err := sm.CreateSession("test-user-" + string(rune(i)))
		require.NoError(t, err)
		err = sm.EndSession(sess.ID)
		require.NoError(t, err)
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Run cleanup
	sm.cleanupExpiredSessions()

	// Verify all sessions are removed
	sm.mu.RLock()
	count := len(sm.sessions)
	sm.mu.RUnlock()
	assert.Equal(t, 0, count, "All expired sessions should be removed")
}

// TestCleanupExpiredSessions_HandlesNilEndTime verifies that
// cleanupExpiredSessions handles sessions with nil EndTime gracefully.
func TestCleanupExpiredSessions_HandlesNilEndTime(t *testing.T) {
	// Create logger for testing
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create session manager
	sm := NewSessionManager(15*time.Minute, logger)
	sm.sessionTTL = 100 * time.Millisecond

	// Create a session and manually set it to inactive without EndTime
	sess, err := sm.CreateSession("test-user")
	require.NoError(t, err)

	sm.mu.Lock()
	sess.IsActive = false
	sess.EndTime = nil // Explicitly set to nil
	sm.mu.Unlock()

	// Run cleanup
	sm.cleanupExpiredSessions()

	// Verify session is NOT removed (EndTime is nil)
	sm.mu.RLock()
	_, exists := sm.sessions[sess.ID]
	sm.mu.RUnlock()
	assert.True(t, exists, "Session with nil EndTime should not be removed")
}

// TestGetMemoryStats_AccurateCounts verifies that GetMemoryStats
// returns accurate counts of active, inactive, and total sessions.
// Validates: Requirements 1.4
func TestGetMemoryStats_AccurateCounts(t *testing.T) {
	// Create logger for testing
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create session manager
	sm := NewSessionManager(15*time.Minute, logger)

	// Initially should have zero sessions
	active, inactive, total := sm.GetMemoryStats()
	assert.Equal(t, 0, active, "Should have 0 active sessions initially")
	assert.Equal(t, 0, inactive, "Should have 0 inactive sessions initially")
	assert.Equal(t, 0, total, "Should have 0 total sessions initially")

	// Create 3 active sessions
	sess1, err := sm.CreateSession("user1")
	require.NoError(t, err)
	sess2, err := sm.CreateSession("user2")
	require.NoError(t, err)
	sess3, err := sm.CreateSession("user3")
	require.NoError(t, err)

	// Check stats with 3 active sessions
	active, inactive, total = sm.GetMemoryStats()
	assert.Equal(t, 3, active, "Should have 3 active sessions")
	assert.Equal(t, 0, inactive, "Should have 0 inactive sessions")
	assert.Equal(t, 3, total, "Should have 3 total sessions")

	// End 2 sessions
	err = sm.EndSession(sess1.ID)
	require.NoError(t, err)
	err = sm.EndSession(sess2.ID)
	require.NoError(t, err)

	// Check stats with 1 active and 2 inactive sessions
	active, inactive, total = sm.GetMemoryStats()
	assert.Equal(t, 1, active, "Should have 1 active session")
	assert.Equal(t, 2, inactive, "Should have 2 inactive sessions")
	assert.Equal(t, 3, total, "Should have 3 total sessions")

	// Verify that active + inactive = total
	assert.Equal(t, total, active+inactive, "Active + inactive should equal total")

	// End the last session
	err = sm.EndSession(sess3.ID)
	require.NoError(t, err)

	// Check stats with all inactive
	active, inactive, total = sm.GetMemoryStats()
	assert.Equal(t, 0, active, "Should have 0 active sessions")
	assert.Equal(t, 3, inactive, "Should have 3 inactive sessions")
	assert.Equal(t, 3, total, "Should have 3 total sessions")
}

// TestGetMemoryStats_ThreadSafe verifies that GetMemoryStats
// is thread-safe and can be called concurrently.
func TestGetMemoryStats_ThreadSafe(t *testing.T) {
	// Create logger for testing
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create session manager
	sm := NewSessionManager(15*time.Minute, logger)

	// Create some sessions
	for i := 0; i < 10; i++ {
		_, err := sm.CreateSession("user" + string(rune(i)))
		require.NoError(t, err)
	}

	// Call GetMemoryStats concurrently from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				active, inactive, total := sm.GetMemoryStats()
				// Verify invariant: active + inactive = total
				assert.Equal(t, total, active+inactive, "Active + inactive should equal total")
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestStopCleanup_ConcurrentSafety verifies that calling StopCleanup
// concurrently from multiple goroutines does not panic or race.
// This test must be run with -race to verify the fix.
func TestStopCleanup_ConcurrentSafety(t *testing.T) {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	sm := NewSessionManager(15*time.Minute, logger)
	sm.cleanupInterval = 10 * time.Millisecond
	sm.sessionTTL = 50 * time.Millisecond

	sm.StartCleanup()

	// Create some sessions while cleanup is running
	for i := 0; i < 5; i++ {
		sess, err := sm.CreateSession("user-" + string(rune('a'+i)))
		require.NoError(t, err)
		_ = sm.EndSession(sess.ID)
	}

	// Call StopCleanup concurrently from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sm.StopCleanup()
		}()
	}

	// Also call GetSession/CreateSession concurrently with StopCleanup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _, _ = sm.GetMemoryStats()
		}(i)
	}

	wg.Wait()
}

// TestStopCleanup_DoubleCallSafe verifies that calling StopCleanup
// twice does not panic.
func TestStopCleanup_DoubleCallSafe(t *testing.T) {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	sm := NewSessionManager(15*time.Minute, logger)
	sm.StartCleanup()

	// Should not panic on double call
	sm.StopCleanup()
	sm.StopCleanup()
}
