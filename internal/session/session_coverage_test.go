package session

// session_coverage_test.go adds targeted tests for functions that were at
// 0% coverage: StartCleanup, StopCleanup, and the Session mutex helpers
// (RLock/RUnlock/Lock/Unlock). All tests use only in-memory state — no
// MongoDB, Redis, or external services are required.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// StartCleanup / StopCleanup
// ---------------------------------------------------------------------------

// TestStartCleanup_StartsGoroutine verifies that StartCleanup actually
// launches a goroutine (the WaitGroup counter increases) and that sessions
// are cleaned up on the configured interval.
func TestStartCleanup_StartsGoroutine(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Use a very short interval so we can observe cleanup without long waits.
	sm.cleanupInterval = 20 * time.Millisecond
	sm.sessionTTL = 10 * time.Millisecond

	// Create and end a session so there is something to clean up.
	sess, err := sm.CreateSession("user-cleanup-1")
	require.NoError(t, err)
	require.NoError(t, sm.EndSession(sess.ID))

	// Verify session still exists before cleanup runs.
	sm.mu.RLock()
	_, exists := sm.sessions[sess.ID]
	sm.mu.RUnlock()
	assert.True(t, exists, "session should exist before cleanup")

	// Start the background goroutine.
	sm.StartCleanup()

	// Allow the TTL to expire and at least one cleanup tick to fire.
	time.Sleep(100 * time.Millisecond)

	// Session should now be removed by the background goroutine.
	sm.mu.RLock()
	_, exists = sm.sessions[sess.ID]
	sm.mu.RUnlock()
	assert.False(t, exists, "session should be removed after background cleanup fires")

	// Shut down cleanly so the goroutine doesn't leak.
	sm.StopCleanup()
}

// TestStopCleanup_StopsGoroutine verifies that StopCleanup signals the
// goroutine to exit and waits for it to finish before returning.
func TestStopCleanup_StopsGoroutine(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)
	sm.cleanupInterval = 10 * time.Millisecond
	sm.sessionTTL = 5 * time.Millisecond

	sm.StartCleanup()

	// Give the goroutine time to start.
	time.Sleep(20 * time.Millisecond)

	// StopCleanup must return without deadlocking and within a short timeout.
	done := make(chan struct{})
	go func() {
		sm.StopCleanup()
		close(done)
	}()

	select {
	case <-done:
		// Success — StopCleanup returned.
	case <-time.After(2 * time.Second):
		t.Fatal("StopCleanup did not return within the expected timeout")
	}
}

// TestStopCleanup_IdempotentWithoutStart verifies that calling StopCleanup
// without a prior StartCleanup does not panic or block.
func TestStopCleanup_IdempotentWithoutStart(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	// Should not panic even though no goroutine was started.
	assert.NotPanics(t, func() {
		sm.StopCleanup()
	})
}

// TestStopCleanup_CalledTwice verifies that calling StopCleanup a second
// time does not panic (double-close guard).
func TestStopCleanup_CalledTwice(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)
	sm.cleanupInterval = 50 * time.Millisecond
	sm.sessionTTL = 50 * time.Millisecond

	sm.StartCleanup()
	sm.StopCleanup()

	// Second call must not panic.
	assert.NotPanics(t, func() {
		sm.StopCleanup()
	})
}

// TestStartCleanup_ActiveSessionsPreserved confirms that active sessions
// are NOT removed by the background cleanup goroutine.
func TestStartCleanup_ActiveSessionsPreserved(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)
	sm.cleanupInterval = 20 * time.Millisecond
	sm.sessionTTL = 10 * time.Millisecond

	sess, err := sm.CreateSession("user-active-cleanup")
	require.NoError(t, err)

	sm.StartCleanup()

	// Wait well beyond the TTL.
	time.Sleep(100 * time.Millisecond)

	// Active session must still be present.
	sm.mu.RLock()
	_, exists := sm.sessions[sess.ID]
	sm.mu.RUnlock()
	assert.True(t, exists, "active session must not be removed by cleanup")

	sm.StopCleanup()
}

// ---------------------------------------------------------------------------
// Session mutex helpers (RLock, RUnlock, Lock, Unlock)
// ---------------------------------------------------------------------------

// TestSession_RLock_RUnlock verifies that the read-lock helpers do not panic
// and that a session can be safely read through them.
func TestSession_RLock_RUnlock(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	sess, err := sm.CreateSession("user-mutex-1")
	require.NoError(t, err)

	// Should not panic.
	assert.NotPanics(t, func() {
		sess.RLock()
		// Read a field while holding the lock.
		_ = sess.IsActive
		sess.RUnlock()
	})
}

// TestSession_Lock_Unlock verifies that the write-lock helpers do not panic
// and that a session field can be safely modified through them.
func TestSession_Lock_Unlock(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	sess, err := sm.CreateSession("user-mutex-2")
	require.NoError(t, err)

	// Should not panic.
	assert.NotPanics(t, func() {
		sess.Lock()
		// Modify a field while holding the write lock.
		sess.Name = "updated-name"
		sess.Unlock()
	})

	// Verify the write was applied.
	assert.Equal(t, "updated-name", sess.Name)
}

// TestSession_ConcurrentRLock verifies that multiple goroutines can hold
// read locks simultaneously without deadlocking.
func TestSession_ConcurrentRLock(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	sess, err := sm.CreateSession("user-concurrent-read")
	require.NoError(t, err)

	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		go func() {
			sess.RLock()
			_ = sess.UserID
			sess.RUnlock()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("goroutine did not complete within timeout")
		}
	}
}

// TestSession_LockExclusion verifies that the write lock prevents concurrent
// writes from racing (no data race when used correctly).
func TestSession_LockExclusion(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)

	sess, err := sm.CreateSession("user-exclusive-write")
	require.NoError(t, err)

	done := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		idx := i
		go func() {
			sess.Lock()
			sess.TotalTokens += idx
			sess.Unlock()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 5; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("goroutine did not complete within timeout")
		}
	}

	// Sum of 0+1+2+3+4 = 10 (order may vary, but total must be deterministic).
	assert.Equal(t, 10, sess.TotalTokens)
}

// TestSession_MutexHelpers_TableDriven exercises all four mutex methods
// through a table so each gets explicit coverage.
func TestSession_MutexHelpers_TableDriven(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)
	sess, err := sm.CreateSession("user-table-mutex")
	require.NoError(t, err)

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "RLock and RUnlock",
			fn: func() {
				sess.RLock()
				_ = sess.IsActive
				sess.RUnlock()
			},
		},
		{
			name: "Lock and Unlock",
			fn: func() {
				sess.Lock()
				sess.HelpRequested = true
				sess.Unlock()
			},
		},
		{
			name: "multiple RLock/RUnlock cycles",
			fn: func() {
				for i := 0; i < 5; i++ {
					sess.RLock()
					_ = sess.UserID
					sess.RUnlock()
				}
			},
		},
		{
			name: "multiple Lock/Unlock cycles",
			fn: func() {
				for i := 0; i < 5; i++ {
					sess.Lock()
					sess.TotalTokens++
					sess.Unlock()
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, tt.fn)
		})
	}
}

// ---------------------------------------------------------------------------
// Thread-safe Session accessor methods
// ---------------------------------------------------------------------------

func TestSession_GetModelID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)
	sess, err := sm.CreateSession("user-getmodel")
	require.NoError(t, err)

	// Initially empty
	assert.Equal(t, "", sess.GetModelID())

	// Set via SessionManager (acquires locks properly)
	err = sm.SetModelID(sess.ID, "gpt-4")
	require.NoError(t, err)

	// Getter returns updated value
	assert.Equal(t, "gpt-4", sess.GetModelID())
}

func TestSession_GetAssistingAdminID(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)
	sess, err := sm.CreateSession("user-getadmin")
	require.NoError(t, err)

	// Initially empty
	assert.Equal(t, "", sess.GetAssistingAdminID())

	// Set via SessionManager
	err = sm.MarkAdminAssisted(sess.ID, "admin-1", "Admin One")
	require.NoError(t, err)

	assert.Equal(t, "admin-1", sess.GetAssistingAdminID())
}

func TestSession_GetAssistingAdminName(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)
	sess, err := sm.CreateSession("user-getadminname")
	require.NoError(t, err)

	assert.Equal(t, "", sess.GetAssistingAdminName())

	err = sm.MarkAdminAssisted(sess.ID, "admin-2", "Admin Two")
	require.NoError(t, err)

	assert.Equal(t, "Admin Two", sess.GetAssistingAdminName())
}

func TestSession_GetAdminAssistance(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)
	sess, err := sm.CreateSession("user-getadminassist")
	require.NoError(t, err)

	// Initially empty
	adminID, adminName := sess.GetAdminAssistance()
	assert.Equal(t, "", adminID)
	assert.Equal(t, "", adminName)

	// Set admin
	err = sm.MarkAdminAssisted(sess.ID, "admin-3", "Admin Three")
	require.NoError(t, err)

	adminID, adminName = sess.GetAdminAssistance()
	assert.Equal(t, "admin-3", adminID)
	assert.Equal(t, "Admin Three", adminName)

	// Clear admin
	err = sm.ClearAdminAssistance(sess.ID)
	require.NoError(t, err)

	adminID, adminName = sess.GetAdminAssistance()
	assert.Equal(t, "", adminID)
	assert.Equal(t, "", adminName)
}

func TestSession_GetModelID_Concurrent(t *testing.T) {
	logger := getTestLogger()
	sm := NewSessionManager(15*time.Minute, logger)
	sess, err := sm.CreateSession("user-concurrent-model")
	require.NoError(t, err)

	err = sm.SetModelID(sess.ID, "gpt-4")
	require.NoError(t, err)

	// Concurrent reads should not race
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				_ = sess.GetModelID()
			}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
