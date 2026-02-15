package session

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: chat-application-websocket
// Property 6: Session Continuity Timeout
// **Validates: Requirements 2.4, 2.5**
//
// For any session, if a connection reconnects within the configured timeout period,
// the previous session should be restored; if reconnecting after the timeout,
// a new session should be created.
func TestProperty_SessionContinuityTimeout(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("session restored within timeout, new session after timeout", prop.ForAll(
		func(userID string, timeoutMs int64, waitMs int64) bool {
			// Skip invalid inputs
			if userID == "" || timeoutMs <= 0 || waitMs < 0 {
				return true
			}

			// Use reasonable timeout values (between 100ms and 5000ms for testing)
			timeout := time.Duration(timeoutMs%4900+100) * time.Millisecond
			waitTime := time.Duration(waitMs % 6000) * time.Millisecond

			// Add a buffer for timing imprecision (50ms)
			const timingBuffer = 50 * time.Millisecond

			logger := getTestLogger()
			sm := NewSessionManager(timeout, logger)

			// Create initial session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			originalSessionID := session.ID

			// End the session (simulating disconnection)
			err = sm.EndSession(session.ID)
			if err != nil {
				return false
			}

			// Wait for the specified time
			time.Sleep(waitTime)

			// Attempt to restore the session
			restored, err := sm.RestoreSession(userID, originalSessionID)

			// If wait time is clearly less than timeout (with buffer), session should be restored
			if waitTime < timeout-timingBuffer {
				if err != nil {
					return false
				}
				// Should restore the same session
				if restored.ID != originalSessionID {
					return false
				}
				if !restored.IsActive {
					return false
				}
				return true
			}

			// If wait time is clearly greater than timeout (with buffer), restore should fail
			if waitTime > timeout+timingBuffer {
				if err == nil {
					return false
				}
				// Error should indicate timeout
				if restored != nil {
					return false
				}

				// Should be able to create a new session after timeout
				newSession, err := sm.CreateSession(userID)
				if err != nil {
					return false
				}
				// New session should have different ID
				return newSession.ID != originalSessionID && newSession.IsActive
			}

			// If we're in the buffer zone, accept either outcome
			return true
		},
		gen.Identifier(),                           // userID
		gen.Int64Range(100, 5000),                 // timeoutMs
		gen.Int64Range(0, 6000),                   // waitMs
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket
// Property 17: Multiple Sessions Per User
// **Validates: Requirements 4.5**
//
// For any user, the Storage_Service should support creating and storing
// multiple distinct session records.
func TestProperty_MultipleSessionsPerUser(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("user can have multiple session records over time", prop.ForAll(
		func(userID string, sessionCount int) bool {
			// Skip invalid inputs
			if userID == "" || sessionCount <= 0 {
				return true
			}

			// Limit session count to reasonable number (1-10)
			count := (sessionCount % 10) + 1

			logger := getTestLogger()
			sm := NewSessionManager(15 * time.Minute, logger)
			sessionIDs := make([]string, 0, count)

			// Create multiple sessions by creating and ending each one
			for i := 0; i < count; i++ {
				// Create session
				session, err := sm.CreateSession(userID)
				if err != nil {
					return false
				}

				// Verify session belongs to user
				if session.UserID != userID {
					return false
				}

				// Store session ID
				sessionIDs = append(sessionIDs, session.ID)

				// End session to allow creating next one
				err = sm.EndSession(session.ID)
				if err != nil {
					return false
				}
			}

			// Verify all sessions are distinct
			uniqueIDs := make(map[string]bool)
			for _, id := range sessionIDs {
				if uniqueIDs[id] {
					// Duplicate ID found
					return false
				}
				uniqueIDs[id] = true
			}

			// Verify all sessions can be retrieved
			for _, id := range sessionIDs {
				session, err := sm.GetSession(id)
				if err != nil {
					return false
				}
				if session.UserID != userID {
					return false
				}
			}

			return len(sessionIDs) == count
		},
		gen.Identifier(),      // userID
		gen.IntRange(1, 15),   // sessionCount
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket
// Property 48: Single Active Session Constraint
// **Validates: Requirements 15.4**
//
// For any user, attempting to create or activate a second concurrent session
// should be prevented while an active session exists.
func TestProperty_SingleActiveSessionConstraint(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("user cannot have multiple concurrent active sessions", prop.ForAll(
		func(userID string) bool {
			// Skip empty user IDs
			if userID == "" {
				return true
			}

			logger := getTestLogger()
			sm := NewSessionManager(15 * time.Minute, logger)

			// Create first session
			session1, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			// Verify first session is active
			if !session1.IsActive {
				return false
			}

			// Attempt to create second session while first is active
			session2, err := sm.CreateSession(userID)

			// Should fail with error
			if err == nil {
				return false
			}

			// Should not return a session
			if session2 != nil {
				return false
			}

			// End first session
			err = sm.EndSession(session1.ID)
			if err != nil {
				return false
			}

			// Now should be able to create a new session
			session3, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			// New session should be created successfully
			if session3 == nil || !session3.IsActive {
				return false
			}

			// New session should have different ID
			return session3.ID != session1.ID
		},
		gen.Identifier(), // userID
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket
// Property 49: Automatic Session Name Generation
// **Validates: Requirements 15.5**
//
// For any new session, the Chat_System should automatically generate a
// descriptive session name based on the initial conversation content.
func TestProperty_AutomaticSessionNameGeneration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("session name is generated from first message", prop.ForAll(
		func(userID string, firstMessage string) bool {
			// Skip invalid inputs
			if userID == "" {
				return true
			}

			// The implementation uses a fixed maxLength of 50
			const maxLen = 50

			logger := getTestLogger()
			sm := NewSessionManager(15 * time.Minute, logger)

			// Create session
			session, err := sm.CreateSession(userID)
			if err != nil {
				return false
			}

			// Initially, session name should be empty
			if session.Name != "" {
				return false
			}

			// Set session name from first message
			err = sm.SetSessionNameFromMessage(session.ID, firstMessage)
			if err != nil {
				return false
			}

			// Retrieve session to check name
			retrieved, err := sm.GetSession(session.ID)
			if err != nil {
				return false
			}

			// Generate expected name using the same maxLength
			expectedName := GenerateSessionName(firstMessage, maxLen)

			// Verify name was set correctly
			if retrieved.Name != expectedName {
				return false
			}

			// Verify name is not longer than maxLength
			if len(retrieved.Name) > maxLen {
				return false
			}

			// If message is empty or whitespace, should get default name
			if len(trimWhitespace(firstMessage)) == 0 {
				if retrieved.Name != "New Chat" {
					return false
				}
			}

			// Try to set name again with different message
			secondMessage := "This should not change the name"
			err = sm.SetSessionNameFromMessage(session.ID, secondMessage)
			if err != nil {
				return false
			}

			// Retrieve again
			retrieved2, err := sm.GetSession(session.ID)
			if err != nil {
				return false
			}

			// Name should remain unchanged (only first message sets name)
			return retrieved2.Name == expectedName
		},
		gen.Identifier(),                    // userID
		gen.AnyString(),                     // firstMessage
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
