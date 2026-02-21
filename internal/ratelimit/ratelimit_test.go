package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionLimiter_Allow(t *testing.T) {
	cl := NewConnectionLimiter(3)

	// First 3 connections should be allowed
	assert.True(t, cl.Allow("user1"))
	assert.True(t, cl.Allow("user1"))
	assert.True(t, cl.Allow("user1"))

	// 4th connection should be denied
	assert.False(t, cl.Allow("user1"))

	// Different user should be allowed
	assert.True(t, cl.Allow("user2"))
}

func TestConnectionLimiter_Release(t *testing.T) {
	cl := NewConnectionLimiter(2)

	// Use up the limit
	cl.Allow("user1")
	cl.Allow("user1")
	assert.False(t, cl.Allow("user1"))

	// Release one connection
	cl.Release("user1")
	assert.True(t, cl.Allow("user1"))
}

func TestConnectionLimiter_GetCount(t *testing.T) {
	cl := NewConnectionLimiter(5)

	assert.Equal(t, 0, cl.GetCount("user1"))

	cl.Allow("user1")
	assert.Equal(t, 1, cl.GetCount("user1"))

	cl.Allow("user1")
	assert.Equal(t, 2, cl.GetCount("user1"))

	cl.Release("user1")
	assert.Equal(t, 1, cl.GetCount("user1"))
}

func TestMessageLimiter_Allow(t *testing.T) {
	ml := NewMessageLimiter(1*time.Second, 3)

	// First 3 messages should be allowed
	assert.True(t, ml.Allow("user1"))
	assert.True(t, ml.Allow("user1"))
	assert.True(t, ml.Allow("user1"))

	// 4th message should be denied
	assert.False(t, ml.Allow("user1"))

	// Different user should be allowed
	assert.True(t, ml.Allow("user2"))
}

func TestMessageLimiter_WindowExpiry(t *testing.T) {
	ml := NewMessageLimiter(100*time.Millisecond, 2)

	// Use up the limit
	assert.True(t, ml.Allow("user1"))
	assert.True(t, ml.Allow("user1"))
	assert.False(t, ml.Allow("user1"))

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	assert.True(t, ml.Allow("user1"))
}

func TestMessageLimiter_GetRetryAfter(t *testing.T) {
	ml := NewMessageLimiter(1*time.Second, 2)

	// Use up the limit
	ml.Allow("user1")
	ml.Allow("user1")

	// Should have retry after value
	retryAfter := ml.GetRetryAfter("user1")
	assert.Greater(t, retryAfter, 0)
	assert.LessOrEqual(t, retryAfter, 1000) // Should be within 1 second

	// User with no events should have 0 retry after
	assert.Equal(t, 0, ml.GetRetryAfter("user2"))
}

func TestMessageLimiter_Reset(t *testing.T) {
	ml := NewMessageLimiter(1*time.Second, 2)

	// Use up the limit
	ml.Allow("user1")
	ml.Allow("user1")
	assert.False(t, ml.Allow("user1"))

	// Reset
	ml.Reset("user1")

	// Should be allowed again
	assert.True(t, ml.Allow("user1"))
}

func TestMessageLimiter_Cleanup(t *testing.T) {
	ml := NewMessageLimiter(100*time.Millisecond, 2)

	// Add events for multiple users
	ml.Allow("user1")
	ml.Allow("user2")
	ml.Allow("user3")

	// Wait for events to expire
	time.Sleep(150 * time.Millisecond)

	// Cleanup should remove expired events
	ml.Cleanup()

	// Verify internal state is cleaned up
	ml.mu.RLock()
	assert.Equal(t, 0, len(ml.events))
	ml.mu.RUnlock()
}

func TestMessageLimiter_ConcurrentAccess(t *testing.T) {
	ml := NewMessageLimiter(1*time.Second, 100)

	// Test concurrent access from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				ml.Allow("user1")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have exactly 100 events (the limit)
	ml.mu.RLock()
	count := len(ml.events["user1"])
	ml.mu.RUnlock()
	assert.Equal(t, 100, count)
}

func TestConnectionLimiter_ConcurrentAccess(t *testing.T) {
	cl := NewConnectionLimiter(50)

	// Test concurrent access from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				cl.Allow("user1")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have exactly 50 connections (the limit)
	assert.Equal(t, 50, cl.GetCount("user1"))
}

// TestStopCleanup_ConcurrentSafety verifies that calling StopCleanup
// concurrently from multiple goroutines does not panic or race.
func TestStopCleanup_ConcurrentSafety(t *testing.T) {
	ml := NewMessageLimiter(100*time.Millisecond, 10)
	ml.cleanupInterval = 10 * time.Millisecond
	ml.StartCleanup()

	// Add some events while cleanup is running
	for i := 0; i < 5; i++ {
		ml.Allow(fmt.Sprintf("user-%d", i))
	}

	// Call StopCleanup concurrently from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ml.StopCleanup()
		}()
	}

	// Also call Allow concurrently with StopCleanup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ml.Allow(fmt.Sprintf("concurrent-user-%d", idx))
		}(i)
	}

	wg.Wait()
}

// TestStopCleanup_DoubleCallSafe verifies that calling StopCleanup
// twice does not panic.
func TestStopCleanup_DoubleCallSafe(t *testing.T) {
	ml := NewMessageLimiter(1*time.Second, 10)
	ml.StartCleanup()

	// Should not panic on double call
	ml.StopCleanup()
	ml.StopCleanup()
}

// TestBoundedEvents_MaxEventsPerUser verifies that the events map is bounded
// per user to prevent unbounded memory growth.
func TestBoundedEvents_MaxEventsPerUser(t *testing.T) {
	// Create limiter with high limit but we'll check event storage is bounded
	ml := NewMessageLimiter(1*time.Hour, 2000) // long window, high limit

	// Add more events than MaxEventsPerUser
	for i := 0; i < 1500; i++ {
		ml.Allow("flood-user")
	}

	ml.mu.RLock()
	eventCount := len(ml.events["flood-user"])
	ml.mu.RUnlock()

	require.LessOrEqual(t, eventCount, 1000,
		"events per user should be bounded to MaxEventsPerUser (1000)")
}

// TestBoundedEvents_MaxUsersTracked verifies that new users are rejected
// when the total tracked user count exceeds MaxUsersTracked.
func TestBoundedEvents_MaxUsersTracked(t *testing.T) {
	ml := NewMessageLimiter(1*time.Hour, 100)

	// Create fewer users for test speed but verify the mechanism works
	// The constant MaxUsersTracked is 100000, so we test the code path
	// by checking that Allow returns false for new users when map is at capacity

	// Manually fill the events map to exceed capacity
	ml.mu.Lock()
	for i := 0; i < 100001; i++ {
		ml.events[fmt.Sprintf("user-%d", i)] = []time.Time{time.Now()}
	}
	ml.mu.Unlock()

	// New user should be rate-limited (denied) when over MaxUsersTracked
	allowed := ml.Allow("brand-new-user")
	assert.False(t, allowed, "new user should be denied when MaxUsersTracked exceeded")
}
