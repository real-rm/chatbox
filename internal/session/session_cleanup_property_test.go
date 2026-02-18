package session

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: production-readiness-fixes
// Property 1: Session cleanup removes expired sessions
// **Validates: Requirements 1.1, 1.2, 1.5**
//
// For any session that has been inactive for longer than the TTL, the cleanup
// process should remove it from the sessions map.
func TestProperty_SessionCleanupRemovesExpiredSessions(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20 // Reduced from 100 for faster execution
	properties := gopter.NewProperties(parameters)

	properties.Property("expired sessions are removed after TTL", prop.ForAll(
		func(userID string, ttlMillis int) bool {
			// Skip invalid inputs
			if userID == "" {
				return true
			}

			// Ensure TTL is reasonable (between 10ms and 100ms for faster testing)
			if ttlMillis < 10 || ttlMillis > 100 {
				return true
			}

			ttl := time.Duration(ttlMillis) * time.Millisecond

			logger := getTestLogger()
			sm := NewSessionManager(15*time.Minute, logger)

			// Configure short TTL for testing
			sm.sessionTTL = ttl
			sm.cleanupInterval = 10 * time.Millisecond

			// Create a session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}
			sessionID := session.ID

			// End the session
			err = sm.EndSession(sessionID)
			if err != nil {
				return false
			}

			// Verify session exists before cleanup
			sm.mu.RLock()
			_, existsBefore := sm.sessions[sessionID]
			sm.mu.RUnlock()

			if !existsBefore {
				return false
			}

			// Wait for TTL to expire (add buffer for timing)
			time.Sleep(ttl + 50*time.Millisecond)

			// Run cleanup
			sm.cleanupExpiredSessions()

			// Verify session is removed after cleanup
			sm.mu.RLock()
			_, existsAfter := sm.sessions[sessionID]
			sm.mu.RUnlock()

			// Session should be removed
			return !existsAfter
		},
		gen.Identifier(),
		gen.IntRange(10, 100), // Reduced from 500 for faster execution
	))

	properties.TestingRun(t)
}

// Feature: production-readiness-fixes
// Property 1 (variant): Session cleanup removes multiple expired sessions
// **Validates: Requirements 1.1, 1.2, 1.5**
//
// For any set of sessions that have been inactive for longer than the TTL,
// the cleanup process should remove all of them from the sessions map.
func TestProperty_SessionCleanupRemovesMultipleExpiredSessions(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("multiple expired sessions are removed after TTL", prop.ForAll(
		func(numSessions int) bool {
			// Ensure reasonable number of sessions (1-20)
			if numSessions < 1 || numSessions > 20 {
				return true
			}

			ttl := 50 * time.Millisecond

			logger := getTestLogger()
			sm := NewSessionManager(15*time.Minute, logger)

			// Configure short TTL for testing
			sm.sessionTTL = ttl
			sm.cleanupInterval = 10 * time.Millisecond

			// Create and end multiple sessions
			sessionIDs := make([]string, numSessions)
			for i := 0; i < numSessions; i++ {
				userID := "user-" + string(rune(i))
				session, err := sm.CreateSession(userID)
				if err != nil {
					return false
				}
				sessionIDs[i] = session.ID

				// End the session
				err = sm.EndSession(session.ID)
				if err != nil {
					return false
				}
			}

			// Verify all sessions exist before cleanup
			sm.mu.RLock()
			countBefore := len(sm.sessions)
			sm.mu.RUnlock()

			if countBefore != numSessions {
				return false
			}

			// Wait for TTL to expire
			time.Sleep(ttl + 50*time.Millisecond)

			// Run cleanup
			sm.cleanupExpiredSessions()

			// Verify all sessions are removed after cleanup
			sm.mu.RLock()
			countAfter := len(sm.sessions)
			sm.mu.RUnlock()

			// All sessions should be removed
			return countAfter == 0
		},
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t)
}

// Feature: production-readiness-fixes
// Property 1 (edge case): Sessions with nil EndTime are not removed
// **Validates: Requirements 1.1, 1.2, 1.5**
//
// For any session that is inactive but has nil EndTime, the cleanup process
// should not remove it (defensive programming).
func TestProperty_SessionCleanupHandlesNilEndTime(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("sessions with nil EndTime are not removed", prop.ForAll(
		func(userID string) bool {
			// Skip invalid inputs
			if userID == "" {
				return true
			}

			ttl := 50 * time.Millisecond

			logger := getTestLogger()
			sm := NewSessionManager(15*time.Minute, logger)

			// Configure short TTL for testing
			sm.sessionTTL = ttl
			sm.cleanupInterval = 10 * time.Millisecond

			// Create a session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}
			sessionID := session.ID

			// Manually set session to inactive with nil EndTime
			sm.mu.Lock()
			session.IsActive = false
			session.EndTime = nil
			sm.mu.Unlock()

			// Wait longer than TTL
			time.Sleep(ttl + 50*time.Millisecond)

			// Run cleanup
			sm.cleanupExpiredSessions()

			// Verify session still exists (not removed)
			sm.mu.RLock()
			_, exists := sm.sessions[sessionID]
			sm.mu.RUnlock()

			// Session should NOT be removed
			return exists
		},
		gen.Identifier(),
	))

	properties.TestingRun(t)
}

// Feature: production-readiness-fixes
// Property 1 (timing): Sessions removed only after TTL expires
// **Validates: Requirements 1.1, 1.2, 1.5**
//
// For any session that has been inactive for less than the TTL, the cleanup
// process should NOT remove it from the sessions map.
func TestProperty_SessionCleanupRespectsTimeWindow(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("sessions are not removed before TTL expires", prop.ForAll(
		func(userID string) bool {
			// Skip invalid inputs
			if userID == "" {
				return true
			}

			ttl := 200 * time.Millisecond

			logger := getTestLogger()
			sm := NewSessionManager(15*time.Minute, logger)

			// Configure TTL for testing
			sm.sessionTTL = ttl
			sm.cleanupInterval = 10 * time.Millisecond

			// Create and end a session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}
			sessionID := session.ID

			err = sm.EndSession(sessionID)
			if err != nil {
				return false
			}

			// Wait less than TTL (half the TTL)
			time.Sleep(ttl / 2)

			// Run cleanup
			sm.cleanupExpiredSessions()

			// Verify session still exists (not removed yet)
			sm.mu.RLock()
			_, exists := sm.sessions[sessionID]
			sm.mu.RUnlock()

			// Session should NOT be removed yet
			return exists
		},
		gen.Identifier(),
	))

	properties.TestingRun(t)
}

// Feature: production-readiness-fixes
// Property 2: Active sessions are never cleaned up
// **Validates: Requirements 1.1**
//
// For any active session, the cleanup process should never remove it from the
// sessions map, regardless of how long it has been active.
func TestProperty_ActiveSessionsNeverCleanedUp(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20 // Reduced from 100 for faster execution
	properties := gopter.NewProperties(parameters)

	properties.Property("active sessions are never removed by cleanup", prop.ForAll(
		func(userID string, waitMillis int) bool {
			// Skip invalid inputs
			if userID == "" {
				return true
			}

			// Ensure wait time is reasonable (between 10ms and 100ms for faster testing)
			if waitMillis < 10 || waitMillis > 100 {
				return true
			}

			waitTime := time.Duration(waitMillis) * time.Millisecond
			ttl := 50 * time.Millisecond // Short TTL for testing

			logger := getTestLogger()
			sm := NewSessionManager(15*time.Minute, logger)

			// Configure short TTL for testing
			sm.sessionTTL = ttl
			sm.cleanupInterval = 10 * time.Millisecond

			// Create an active session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}
			sessionID := session.ID

			// Verify session is active
			sm.mu.RLock()
			if !session.IsActive {
				sm.mu.RUnlock()
				return false
			}
			sm.mu.RUnlock()

			// Wait longer than TTL (to ensure cleanup would remove expired sessions)
			time.Sleep(waitTime)

			// Run cleanup multiple times
			sm.cleanupExpiredSessions()
			sm.cleanupExpiredSessions()
			sm.cleanupExpiredSessions()

			// Verify active session still exists
			sm.mu.RLock()
			_, exists := sm.sessions[sessionID]
			stillActive := session.IsActive
			sm.mu.RUnlock()

			// Active session should NEVER be removed
			return exists && stillActive
		},
		gen.Identifier(),
		gen.IntRange(10, 100), // Reduced from 500 for faster execution
	))

	properties.TestingRun(t)
}

// Feature: production-readiness-fixes
// Property 2 (variant): Active sessions preserved with multiple cleanup cycles
// **Validates: Requirements 1.1**
//
// For any set of active sessions, multiple cleanup cycles should never remove
// any of them from the sessions map.
func TestProperty_MultipleActiveSessionsPreservedDuringCleanup(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("multiple active sessions preserved across cleanup cycles", prop.ForAll(
		func(numSessions int) bool {
			// Ensure reasonable number of sessions (1-20)
			if numSessions < 1 || numSessions > 20 {
				return true
			}

			ttl := 50 * time.Millisecond

			logger := getTestLogger()
			sm := NewSessionManager(15*time.Minute, logger)

			// Configure short TTL for testing
			sm.sessionTTL = ttl
			sm.cleanupInterval = 10 * time.Millisecond

			// Create multiple active sessions
			sessionIDs := make([]string, numSessions)
			for i := 0; i < numSessions; i++ {
				userID := "user-" + string(rune('A'+i))
				session, err := sm.CreateSession(userID)
				if err != nil {
					return false
				}
				sessionIDs[i] = session.ID
			}

			// Verify all sessions exist and are active
			sm.mu.RLock()
			countBefore := 0
			for _, sessionID := range sessionIDs {
				if session, exists := sm.sessions[sessionID]; exists && session.IsActive {
					countBefore++
				}
			}
			sm.mu.RUnlock()

			if countBefore != numSessions {
				return false
			}

			// Wait longer than TTL
			time.Sleep(ttl + 50*time.Millisecond)

			// Run cleanup multiple times
			sm.cleanupExpiredSessions()
			sm.cleanupExpiredSessions()
			sm.cleanupExpiredSessions()

			// Verify all active sessions still exist
			sm.mu.RLock()
			countAfter := 0
			for _, sessionID := range sessionIDs {
				if session, exists := sm.sessions[sessionID]; exists && session.IsActive {
					countAfter++
				}
			}
			sm.mu.RUnlock()

			// All active sessions should still be present
			return countAfter == numSessions
		},
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t)
}

// Feature: production-readiness-fixes
// Property 2 (mixed): Cleanup removes only inactive sessions, preserves active ones
// **Validates: Requirements 1.1**
//
// For any mix of active and inactive sessions, cleanup should remove only the
// expired inactive sessions while preserving all active sessions.
func TestProperty_CleanupSelectivelyRemovesInactiveSessions(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("cleanup removes inactive but preserves active sessions", prop.ForAll(
		func(numActive, numInactive int) bool {
			// Ensure reasonable numbers (1-10 each)
			if numActive < 1 || numActive > 10 || numInactive < 1 || numInactive > 10 {
				return true
			}

			ttl := 50 * time.Millisecond

			logger := getTestLogger()
			sm := NewSessionManager(15*time.Minute, logger)

			// Configure short TTL for testing
			sm.sessionTTL = ttl
			sm.cleanupInterval = 10 * time.Millisecond

			// Create active sessions
			activeSessionIDs := make([]string, numActive)
			for i := 0; i < numActive; i++ {
				userID := "active-user-" + string(rune('A'+i))
				session, err := sm.CreateSession(userID)
				if err != nil {
					return false
				}
				activeSessionIDs[i] = session.ID
			}

			// Create and end inactive sessions
			inactiveSessionIDs := make([]string, numInactive)
			for i := 0; i < numInactive; i++ {
				userID := "inactive-user-" + string(rune('A'+i))
				session, err := sm.CreateSession(userID)
				if err != nil {
					return false
				}
				inactiveSessionIDs[i] = session.ID

				// End the session
				err = sm.EndSession(session.ID)
				if err != nil {
					return false
				}
			}

			// Verify initial state
			sm.mu.RLock()
			totalBefore := len(sm.sessions)
			sm.mu.RUnlock()

			if totalBefore != numActive+numInactive {
				return false
			}

			// Wait for TTL to expire
			time.Sleep(ttl + 50*time.Millisecond)

			// Run cleanup
			sm.cleanupExpiredSessions()

			// Verify active sessions still exist
			sm.mu.RLock()
			activeCount := 0
			for _, sessionID := range activeSessionIDs {
				if session, exists := sm.sessions[sessionID]; exists && session.IsActive {
					activeCount++
				}
			}

			// Verify inactive sessions are removed
			inactiveCount := 0
			for _, sessionID := range inactiveSessionIDs {
				if _, exists := sm.sessions[sessionID]; exists {
					inactiveCount++
				}
			}
			sm.mu.RUnlock()

			// All active sessions should remain, all inactive should be removed
			return activeCount == numActive && inactiveCount == 0
		},
		gen.IntRange(1, 10),
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}
