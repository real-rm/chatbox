package session

import (
	"testing"
	"testing/quick"
	"time"

	"github.com/real-rm/golog"
)

// Feature: production-readiness-fixes, Property 12: ResponseTimes slice is bounded
// **Validates: Requirements 12.1, 12.5**
//
// Property: For any session, the length of ResponseTimes should never exceed MaxResponseTimes
func TestProperty_ResponseTimesBounded(t *testing.T) {
	property := func(numRecordings uint8) bool {
		// Create session manager with a simple logger
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		if err != nil {
			t.Logf("Failed to create logger: %v", err)
			return false
		}
		defer logger.Close()

		sm := NewSessionManager(15*time.Minute, logger)

		// Create a session
		sess, err := sm.CreateSession("test-user")
		if err != nil {
			t.Logf("Failed to create session: %v", err)
			return false
		}

		// Record response times (at least MaxResponseTimes + 1, up to MaxResponseTimes + 50)
		count := int(numRecordings%50) + MaxResponseTimes + 1

		for i := 0; i < count; i++ {
			duration := time.Duration(i+1) * time.Millisecond
			if err := sm.RecordResponseTime(sess.ID, duration); err != nil {
				t.Logf("Failed to record response time %d: %v", i, err)
				return false
			}
		}

		// Get the session and check ResponseTimes length
		sess, err = sm.GetSession(sess.ID)
		if err != nil {
			t.Logf("Failed to get session: %v", err)
			return false
		}

		// Length should never exceed MaxResponseTimes
		if len(sess.ResponseTimes) > MaxResponseTimes {
			t.Logf("ResponseTimes length %d exceeds MaxResponseTimes %d", len(sess.ResponseTimes), MaxResponseTimes)
			return false
		}

		// Length should be exactly MaxResponseTimes since we recorded more than that
		if len(sess.ResponseTimes) != MaxResponseTimes {
			t.Logf("ResponseTimes length %d, expected %d", len(sess.ResponseTimes), MaxResponseTimes)
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}

// Feature: production-readiness-fixes, Property 13: Rolling window maintains recent times
// **Validates: Requirements 12.2, 12.4**
//
// Property: For any session with full ResponseTimes, adding a new time should remove the oldest time
func TestProperty_RollingWindowMaintainsRecent(t *testing.T) {
	property := func(extraRecordings uint8) bool {
		// Create session manager with a simple logger
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		if err != nil {
			t.Logf("Failed to create logger: %v", err)
			return false
		}
		defer logger.Close()

		sm := NewSessionManager(15*time.Minute, logger)

		// Create a session
		sess, err := sm.CreateSession("test-user")
		if err != nil {
			t.Logf("Failed to create session: %v", err)
			return false
		}

		// Fill up to MaxResponseTimes
		for i := 0; i < MaxResponseTimes; i++ {
			duration := time.Duration(i+1) * time.Millisecond
			if err := sm.RecordResponseTime(sess.ID, duration); err != nil {
				t.Logf("Failed to record response time %d: %v", i, err)
				return false
			}
		}

		// Get the session and verify it's full
		sess, err = sm.GetSession(sess.ID)
		if err != nil {
			t.Logf("Failed to get session: %v", err)
			return false
		}

		if len(sess.ResponseTimes) != MaxResponseTimes {
			t.Logf("ResponseTimes length %d, expected %d", len(sess.ResponseTimes), MaxResponseTimes)
			return false
		}

		// Remember the first value (should be 1ms)
		firstValue := sess.ResponseTimes[0]
		if firstValue != 1*time.Millisecond {
			t.Logf("First value is %v, expected 1ms", firstValue)
			return false
		}

		// Add more recordings (1-10 additional)
		numExtra := int(extraRecordings%10) + 1
		for i := 0; i < numExtra; i++ {
			duration := time.Duration(MaxResponseTimes+i+1) * time.Millisecond
			if err := sm.RecordResponseTime(sess.ID, duration); err != nil {
				t.Logf("Failed to record extra response time %d: %v", i, err)
				return false
			}
		}

		// Get the session again
		sess, err = sm.GetSession(sess.ID)
		if err != nil {
			t.Logf("Failed to get session: %v", err)
			return false
		}

		// Length should still be MaxResponseTimes
		if len(sess.ResponseTimes) != MaxResponseTimes {
			t.Logf("ResponseTimes length %d, expected %d", len(sess.ResponseTimes), MaxResponseTimes)
			return false
		}

		// The first value should have changed (oldest was removed)
		newFirstValue := sess.ResponseTimes[0]
		if newFirstValue == firstValue {
			t.Logf("First value is still %v, but oldest should have been removed", firstValue)
			return false
		}

		// The new first value should be the second value from before (2ms + numExtra - 1)
		expectedFirstValue := time.Duration(numExtra+1) * time.Millisecond
		if newFirstValue != expectedFirstValue {
			t.Logf("First value is %v, expected %v", newFirstValue, expectedFirstValue)
			return false
		}

		// The last value should be the most recent one we added
		lastValue := sess.ResponseTimes[MaxResponseTimes-1]
		expectedLastValue := time.Duration(MaxResponseTimes+numExtra) * time.Millisecond
		if lastValue != expectedLastValue {
			t.Logf("Last value is %v, expected %v", lastValue, expectedLastValue)
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}
