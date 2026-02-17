package ratelimit

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProductionIssue11_CleanupMethod verifies that Cleanup() removes expired events
//
// Production Readiness Issue #11: No automatic cleanup of rate limiter events
// Location: ratelimit/ratelimit.go
// Impact: Unbounded memory growth
//
// This test verifies that the Cleanup() method properly removes expired events.
func TestProductionIssue11_CleanupMethod(t *testing.T) {
	// Create message limiter with short window for testing
	window := 100 * time.Millisecond
	limit := 10
	ml := NewMessageLimiter(window, limit)

	// Generate events for multiple users
	numUsers := 100
	for i := 0; i < numUsers; i++ {
		userID := fmt.Sprintf("user-%d", i)
		for j := 0; j < 5; j++ {
			allowed := ml.Allow(userID)
			require.True(t, allowed, "Should allow messages under limit")
		}
	}

	// Verify events are stored
	ml.mu.RLock()
	eventsBefore := len(ml.events)
	ml.mu.RUnlock()

	assert.Equal(t, numUsers, eventsBefore, "Should have events for all users")

	// Wait for events to expire
	time.Sleep(window + 50*time.Millisecond)

	// Call Cleanup()
	ml.Cleanup()

	// Verify events are removed
	ml.mu.RLock()
	eventsAfter := len(ml.events)
	ml.mu.RUnlock()

	assert.Equal(t, 0, eventsAfter, "All expired events should be removed")

	t.Log("STATUS: Cleanup() method works correctly")
	t.Log("FINDING: Expired events are properly removed")
	t.Log("RECOMMENDATION: Call Cleanup() periodically to prevent memory growth")
}

// TestProductionIssue11_MemoryGrowth demonstrates unbounded memory growth without cleanup
//
// Production Readiness Issue #11: No automatic cleanup of rate limiter events
// Location: ratelimit/ratelimit.go
// Impact: Unbounded memory growth
//
// This test documents that without calling Cleanup(), memory grows unbounded.
// Generates 10,000 events and measures memory usage.
func TestProductionIssue11_MemoryGrowth(t *testing.T) {
	// Create message limiter with long window to prevent expiration
	window := 1 * time.Hour
	limit := 100
	ml := NewMessageLimiter(window, limit)

	// Generate 10,000 events as specified in requirements
	numEvents := 10000
	numUsers := 100
	eventsPerUser := numEvents / numUsers

	// Record initial state
	ml.mu.RLock()
	initialUsers := len(ml.events)
	ml.mu.RUnlock()

	assert.Equal(t, 0, initialUsers, "Should start with no events")

	// Generate events
	for i := 0; i < numUsers; i++ {
		userID := fmt.Sprintf("user-%d", i)
		for j := 0; j < eventsPerUser; j++ {
			ml.Allow(userID)
		}
	}

	// Verify all events are stored
	ml.mu.RLock()
	totalUsers := len(ml.events)
	totalEvents := 0
	for _, events := range ml.events {
		totalEvents += len(events)
	}
	ml.mu.RUnlock()

	assert.Equal(t, numUsers, totalUsers, "Should have events for all users")
	assert.Equal(t, numEvents, totalEvents, "Should have exactly 10,000 events")
	
	t.Logf("Total users tracked: %d", totalUsers)
	t.Logf("Total events stored: %d", totalEvents)

	// Calculate approximate memory usage
	// Each time.Time is 24 bytes (3 int64 fields: wall, ext, loc pointer)
	// Plus map overhead and slice overhead
	bytesPerEvent := 24
	mapOverheadPerUser := 48 // approximate overhead for map entry
	expectedMinMemory := (totalEvents * bytesPerEvent) + (totalUsers * mapOverheadPerUser)
	
	t.Logf("Estimated memory usage: ~%d bytes (%.2f KB)", expectedMinMemory, float64(expectedMinMemory)/1024)

	// Document unbounded growth
	t.Log("")
	t.Log("=== UNBOUNDED GROWTH DOCUMENTATION ===")
	t.Log("FINDING: Events accumulate in memory without automatic cleanup")
	t.Log("IMPACT: Memory grows unbounded with user activity")
	t.Log("DETAILS:")
	t.Logf("  - %d events stored indefinitely", totalEvents)
	t.Logf("  - %d users tracked in memory", totalUsers)
	t.Logf("  - Estimated minimum memory: %.2f KB", float64(expectedMinMemory)/1024)
	t.Log("  - No automatic cleanup mechanism")
	t.Log("  - Events remain until Cleanup() is manually called")
	t.Log("RECOMMENDATION: Implement periodic cleanup goroutine or automatic expiration")
	t.Log("WORKAROUND: Call Cleanup() periodically in production")
}

// TestProductionIssue11_CleanupEffectiveness tests cleanup with mixed expired/active events
func TestProductionIssue11_CleanupEffectiveness(t *testing.T) {
	// Create message limiter
	window := 200 * time.Millisecond
	limit := 10
	ml := NewMessageLimiter(window, limit)

	// Generate old events
	for i := 0; i < 50; i++ {
		userID := fmt.Sprintf("old-user-%d", i)
		ml.Allow(userID)
	}

	// Wait for old events to expire
	time.Sleep(window + 50*time.Millisecond)

	// Generate new events
	for i := 0; i < 50; i++ {
		userID := fmt.Sprintf("new-user-%d", i)
		ml.Allow(userID)
	}

	// Before cleanup
	ml.mu.RLock()
	usersBefore := len(ml.events)
	ml.mu.RUnlock()

	assert.Equal(t, 100, usersBefore, "Should have 100 users before cleanup")

	// Call cleanup
	ml.Cleanup()

	// After cleanup
	ml.mu.RLock()
	usersAfter := len(ml.events)
	ml.mu.RUnlock()

	assert.Equal(t, 50, usersAfter, "Should have 50 users after cleanup (only new ones)")

	t.Log("STATUS: Cleanup correctly removes only expired events")
	t.Log("FINDING: Active events are preserved during cleanup")
}

