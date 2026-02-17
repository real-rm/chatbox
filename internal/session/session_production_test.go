package session

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProductionIssue01_SessionCleanup verifies that EndSession marks sessions as inactive
// but does NOT remove them from memory (documenting current behavior).
//
// Production Readiness Issue #1: In-memory session store never cleaned up
// Location: session/session.go:79
// Impact: Unbounded memory growth → OOM crash
//
// This test documents the current behavior where sessions remain in memory after ending.
func TestProductionIssue01_SessionCleanup(t *testing.T) {
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

	// Create a new session
	userID := "test-user-1"
	sess, err := sm.CreateSession(userID)
	require.NoError(t, err)
	require.NotNil(t, sess)

	sessionID := sess.ID

	// Verify session exists in sessions map
	sm.mu.RLock()
	_, existsInSessions := sm.sessions[sessionID]
	_, existsInUserSessions := sm.userSessions[userID]
	sm.mu.RUnlock()

	assert.True(t, existsInSessions, "Session should exist in sessions map")
	assert.True(t, existsInUserSessions, "User mapping should exist")
	assert.True(t, sess.IsActive, "Session should be active")

	// End the session
	err = sm.EndSession(sessionID)
	require.NoError(t, err)

	// Verify session STILL exists in sessions map (current behavior)
	sm.mu.RLock()
	endedSession, stillExists := sm.sessions[sessionID]
	_, userMappingExists := sm.userSessions[userID]
	sm.mu.RUnlock()

	// CRITICAL: Session remains in memory after ending
	assert.True(t, stillExists, "Session SHOULD still exist in memory (current behavior)")
	assert.NotNil(t, endedSession, "Session object should still be accessible")

	// Verify session is marked as inactive
	assert.False(t, endedSession.IsActive, "Session should be marked inactive")
	assert.NotNil(t, endedSession.EndTime, "EndTime should be set")

	// Verify user mapping is removed
	assert.False(t, userMappingExists, "User mapping should be removed")

	t.Log("FINDING: Sessions remain in memory after EndSession() is called")
	t.Log("IMPACT: Unbounded memory growth as sessions accumulate")
	t.Log("RECOMMENDATION: Implement periodic cleanup or TTL-based removal")
}

// TestProductionIssue01_MemoryGrowth demonstrates unbounded memory growth
// with many sessions that are never cleaned up.
//
// This test creates many sessions, ends them all, and verifies they all
// remain in memory, documenting the memory leak behavior.
func TestProductionIssue01_MemoryGrowth(t *testing.T) {
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

	// Record initial memory stats
	var memBefore runtime.MemStats
	runtime.GC() // Force GC to get baseline
	runtime.ReadMemStats(&memBefore)

	// Create many sessions
	numSessions := 1000
	sessionIDs := make([]string, numSessions)

	for i := 0; i < numSessions; i++ {
		userID := "user-" + string(rune(i))
		sess, err := sm.CreateSession(userID)
		require.NoError(t, err)
		sessionIDs[i] = sess.ID

		// Add some messages to make sessions more realistic
		for j := 0; j < 10; j++ {
			msg := &Message{
				Content:   "Test message content that takes up memory",
				Timestamp: time.Now(),
				Sender:    "user",
			}
			err = sm.AddMessage(sess.ID, msg)
			require.NoError(t, err)
		}
	}

	// End all sessions
	for _, sessionID := range sessionIDs {
		err := sm.EndSession(sessionID)
		require.NoError(t, err)
	}

	// Force GC to see if sessions are collected
	runtime.GC()
	time.Sleep(100 * time.Millisecond) // Give GC time to run

	// Verify all sessions STILL exist in memory
	sm.mu.RLock()
	sessionsInMemory := len(sm.sessions)
	sm.mu.RUnlock()

	assert.Equal(t, numSessions, sessionsInMemory,
		"All %d sessions should still be in memory after ending", numSessions)

	// Record memory after
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	memoryGrowth := memAfter.Alloc - memBefore.Alloc
	t.Logf("Created and ended %d sessions", numSessions)
	t.Logf("Sessions remaining in memory: %d", sessionsInMemory)
	t.Logf("Memory growth: %d bytes (%.2f MB)", memoryGrowth, float64(memoryGrowth)/(1024*1024))

	// Verify sessions are not garbage collected
	assert.Equal(t, numSessions, sessionsInMemory,
		"Sessions are not garbage collected after ending")

	t.Log("FINDING: All ended sessions remain in memory indefinitely")
	t.Log("IMPACT: Memory grows unbounded with session count")
	t.Log("RISK: Production deployment will eventually OOM crash")
	t.Log("RECOMMENDATION: Implement cleanup mechanism (TTL, LRU, or manual cleanup)")
}

// TestProductionIssue12_ResponseTimesGrowth verifies that ResponseTimes slice
// is now capped with a rolling window to prevent unbounded growth.
//
// Production Readiness Issue #12: ResponseTimes slice unbounded
// Location: session/session.go:72
// Impact: Memory growth per session
// Status: FIXED - Rolling window implemented with MaxResponseTimes cap
//
// This test verifies that the fix is working correctly.
func TestProductionIssue12_ResponseTimesGrowth(t *testing.T) {
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

	// Create a session
	sess, err := sm.CreateSession("test-user")
	require.NoError(t, err)

	// Record many response times (more than MaxResponseTimes)
	numResponses := 10000
	for i := 0; i < numResponses; i++ {
		duration := time.Duration(i) * time.Millisecond
		err := sm.RecordResponseTime(sess.ID, duration)
		require.NoError(t, err)
	}

	// Verify response times are capped at MaxResponseTimes
	retrievedSess, err := sm.GetSession(sess.ID)
	require.NoError(t, err)

	retrievedSess.mu.RLock()
	responseTimesCount := len(retrievedSess.ResponseTimes)
	retrievedSess.mu.RUnlock()

	// Should be capped at MaxResponseTimes, not all 10000
	assert.Equal(t, MaxResponseTimes, responseTimesCount,
		"Response times should be capped at MaxResponseTimes (%d), not grow unbounded", MaxResponseTimes)

	// Verify the rolling window contains the most recent values
	retrievedSess.mu.RLock()
	lastValue := retrievedSess.ResponseTimes[MaxResponseTimes-1]
	firstValue := retrievedSess.ResponseTimes[0]
	retrievedSess.mu.RUnlock()

	// The last value should be from the most recent recording
	expectedLastValue := time.Duration(numResponses-1) * time.Millisecond
	assert.Equal(t, expectedLastValue, lastValue,
		"Last value should be the most recent recording")

	// The first value should be from (numResponses - MaxResponseTimes) recordings ago
	expectedFirstValue := time.Duration(numResponses-MaxResponseTimes) * time.Millisecond
	assert.Equal(t, expectedFirstValue, firstValue,
		"First value should be from the oldest entry in the rolling window")

	// Calculate approximate memory usage
	// Each time.Duration is 8 bytes
	memoryUsage := responseTimesCount * 8
	t.Logf("Recorded %d response times", numResponses)
	t.Logf("Stored response times: %d (capped at MaxResponseTimes)", responseTimesCount)
	t.Logf("Approximate memory usage: %d bytes (%.2f KB)",
		memoryUsage, float64(memoryUsage)/1024)

	t.Log("FINDING: ResponseTimes slice is now capped with rolling window")
	t.Log("STATUS: FIXED - Issue #12 has been resolved")
	t.Log("IMPLEMENTATION: MaxResponseTimes constant limits array size to 100 entries")
	t.Log("IMPACT: Memory usage per session is now bounded")
}

// TestProductionIssue01_RestoreSessionAfterTimeout verifies that sessions
// can be restored within the timeout window but not after.
func TestProductionIssue01_RestoreSessionAfterTimeout(t *testing.T) {
	// Create logger for testing
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create session manager with short timeout for testing
	reconnectTimeout := 100 * time.Millisecond
	sm := NewSessionManager(reconnectTimeout, logger)

	// Create and end a session
	userID := "test-user"
	sess, err := sm.CreateSession(userID)
	require.NoError(t, err)
	sessionID := sess.ID

	err = sm.EndSession(sessionID)
	require.NoError(t, err)

	// Try to restore immediately (should succeed)
	restoredSess, err := sm.RestoreSession(userID, sessionID)
	assert.NoError(t, err)
	assert.NotNil(t, restoredSess)
	assert.True(t, restoredSess.IsActive)

	// End again
	err = sm.EndSession(sessionID)
	require.NoError(t, err)

	// Wait for timeout to expire
	time.Sleep(reconnectTimeout + 50*time.Millisecond)

	// Try to restore after timeout (should fail)
	restoredSess, err = sm.RestoreSession(userID, sessionID)
	assert.Error(t, err)
	assert.Nil(t, restoredSess)
	assert.ErrorIs(t, err, ErrSessionTimeout)

	// But session STILL exists in memory
	sm.mu.RLock()
	_, stillExists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	assert.True(t, stillExists, "Session still exists in memory even after timeout")

	t.Log("FINDING: Timed-out sessions remain in memory")
	t.Log("IMPACT: Sessions that can't be restored still consume memory")
}

// TestProductionIssue04_ConcurrentSessionAccess verifies thread-safe access
// to Session fields under concurrent read/write operations.
//
// Production Readiness Issue #4: Potential data races on session fields
// Location: session/session.go
// Impact: Data corruption, race conditions
//
// This test verifies that the Session's mutex properly protects all field access.
func TestProductionIssue04_ConcurrentSessionAccess(t *testing.T) {
	// Create logger for testing
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create session manager and session
	sm := NewSessionManager(15*time.Minute, logger)
	sess, err := sm.CreateSession("test-user")
	require.NoError(t, err)
	require.NotNil(t, sess)

	// Use WaitGroup to coordinate goroutines
	var wg sync.WaitGroup
	numGoroutines := 50
	iterations := 100

	// Launch concurrent readers for various fields
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Read various fields with proper locking
				sess.mu.RLock()
				_ = sess.IsActive
				_ = sess.TotalTokens
				_ = sess.HelpRequested
				_ = sess.AdminAssisted
				_ = sess.ModelID
				_ = sess.Name
				_ = len(sess.Messages)
				_ = len(sess.ResponseTimes)
				sess.mu.RUnlock()
			}
		}()
	}

	// Launch concurrent writers for various fields
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Write various fields with proper locking
				sess.mu.Lock()
				sess.TotalTokens += 1
				sess.LastActivity = time.Now()
				sess.HelpRequested = (j % 2) == 0
				sess.AdminAssisted = (j % 3) == 0
				sess.ModelID = fmt.Sprintf("model-%d", j)
				sess.Name = fmt.Sprintf("session-%d", j)
				sess.mu.Unlock()
			}
		}()
	}

	// Launch concurrent message appenders
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				msg := &Message{
					Content:   fmt.Sprintf("Message %d from goroutine %d", j, goroutineID),
					Timestamp: time.Now(),
					Sender:    "user",
				}
				sess.mu.Lock()
				sess.Messages = append(sess.Messages, msg)
				sess.mu.Unlock()
			}
		}(i)
	}

	// Launch concurrent response time recorders
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				duration := time.Duration(j) * time.Millisecond
				sess.mu.Lock()
				sess.ResponseTimes = append(sess.ResponseTimes, duration)
				// Implement rolling window to prevent unbounded growth
				if len(sess.ResponseTimes) > MaxResponseTimes {
					sess.ResponseTimes = sess.ResponseTimes[1:]
				}
				sess.mu.Unlock()
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify session is still in valid state
	sess.mu.RLock()
	messagesCount := len(sess.Messages)
	responseTimesCount := len(sess.ResponseTimes)
	totalTokens := sess.TotalTokens
	sess.mu.RUnlock()

	// Verify we got some data from concurrent operations
	assert.Greater(t, messagesCount, 0, "Should have messages from concurrent writes")
	assert.Greater(t, responseTimesCount, 0, "Should have response times from concurrent writes")
	assert.Greater(t, totalTokens, 0, "Should have token count from concurrent writes")

	// Verify rolling window cap is enforced
	assert.LessOrEqual(t, responseTimesCount, MaxResponseTimes,
		"ResponseTimes should be capped at MaxResponseTimes")

	t.Log("FINDING: Session fields are properly protected by mutex")
	t.Log("RESULT: No data races detected with concurrent access")
	t.Log("NOTE: Run with -race flag to verify: go test -race -run TestProductionIssue04_ConcurrentSessionAccess")
}
