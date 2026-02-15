package ratelimit

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: chat-application-websocket
// Property 43: Rate Limiting Enforcement
// **Validates: Requirements 13.3**
//
// For any user making excessive requests, the WebSocket_Server should throttle
// or reject requests to prevent abuse.
func TestProperty_RateLimitingEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("message limiter enforces rate limits", prop.ForAll(
		func(userID string, limit int, numRequests int) bool {
			// Skip invalid inputs
			if userID == "" || limit <= 0 || limit > 1000 || numRequests <= 0 || numRequests > 2000 {
				return true
			}

			// Create message limiter with short window for testing
			ml := NewMessageLimiter(100*time.Millisecond, limit)

			// Track allowed and denied requests
			allowed := 0
			denied := 0

			// Make requests
			for i := 0; i < numRequests; i++ {
				if ml.Allow(userID) {
					allowed++
				} else {
					denied++
				}
			}

			// If numRequests <= limit, all should be allowed
			if numRequests <= limit {
				return allowed == numRequests && denied == 0
			}

			// If numRequests > limit, exactly 'limit' requests should be allowed
			if allowed != limit {
				return false
			}

			// Verify that remaining requests were denied
			if denied != numRequests-limit {
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.IntRange(1, 100),
		gen.IntRange(1, 200),
	))

	properties.Property("connection limiter enforces connection limits", prop.ForAll(
		func(userID string, maxConnections int, numAttempts int) bool {
			// Skip invalid inputs
			if userID == "" || maxConnections <= 0 || maxConnections > 100 || numAttempts <= 0 || numAttempts > 200 {
				return true
			}

			// Create connection limiter
			cl := NewConnectionLimiter(maxConnections)

			// Track allowed and denied connections
			allowed := 0
			denied := 0

			// Attempt connections
			for i := 0; i < numAttempts; i++ {
				if cl.Allow(userID) {
					allowed++
				} else {
					denied++
				}
			}

			// If numAttempts <= maxConnections, all should be allowed
			if numAttempts <= maxConnections {
				return allowed == numAttempts && denied == 0
			}

			// If numAttempts > maxConnections, exactly 'maxConnections' should be allowed
			if allowed != maxConnections {
				return false
			}

			// Verify that remaining attempts were denied
			if denied != numAttempts-maxConnections {
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.IntRange(1, 50),
		gen.IntRange(1, 100),
	))

	properties.Property("rate limiter isolates users", prop.ForAll(
		func(user1 string, user2 string, limit int) bool {
			// Skip invalid inputs
			if user1 == "" || user2 == "" || user1 == user2 || limit <= 0 || limit > 100 {
				return true
			}

			// Create message limiter
			ml := NewMessageLimiter(1*time.Second, limit)

			// User 1 uses up their limit
			for i := 0; i < limit; i++ {
				if !ml.Allow(user1) {
					return false // Should all be allowed
				}
			}

			// User 1 should be rate limited
			if ml.Allow(user1) {
				return false
			}

			// User 2 should still be able to make requests
			for i := 0; i < limit; i++ {
				if !ml.Allow(user2) {
					return false // Should all be allowed
				}
			}

			return true
		},
		gen.AlphaString(),
		gen.AlphaString(),
		gen.IntRange(1, 50),
	))

	properties.Property("rate limiter resets after window expires", prop.ForAll(
		func(userID string, limit int) bool {
			// Skip invalid inputs
			if userID == "" || limit <= 0 || limit > 50 {
				return true
			}

			// Create message limiter with very short window
			ml := NewMessageLimiter(50*time.Millisecond, limit)

			// Use up the limit
			for i := 0; i < limit; i++ {
				if !ml.Allow(userID) {
					return false
				}
			}

			// Should be rate limited
			if ml.Allow(userID) {
				return false
			}

			// Wait for window to expire
			time.Sleep(100 * time.Millisecond)

			// Should be allowed again
			if !ml.Allow(userID) {
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.IntRange(1, 20),
	))

	properties.Property("connection limiter releases connections correctly", prop.ForAll(
		func(userID string, maxConnections int, numReleases int) bool {
			// Skip invalid inputs
			if userID == "" || maxConnections <= 0 || maxConnections > 50 || numReleases < 0 || numReleases > maxConnections {
				return true
			}

			// Create connection limiter
			cl := NewConnectionLimiter(maxConnections)

			// Use up all connections
			for i := 0; i < maxConnections; i++ {
				if !cl.Allow(userID) {
					return false
				}
			}

			// Should be at limit
			if cl.Allow(userID) {
				return false
			}

			// Release some connections
			for i := 0; i < numReleases; i++ {
				cl.Release(userID)
			}

			// Should be able to create exactly numReleases new connections
			for i := 0; i < numReleases; i++ {
				if !cl.Allow(userID) {
					return false
				}
			}

			// Should be at limit again
			if cl.Allow(userID) {
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.IntRange(1, 30),
		gen.IntRange(0, 30),
	))

	properties.Property("retry after value is reasonable", prop.ForAll(
		func(userID string, limit int) bool {
			// Skip invalid inputs
			if userID == "" || limit <= 0 || limit > 50 {
				return true
			}

			// Create message limiter with 1 second window
			ml := NewMessageLimiter(1*time.Second, limit)

			// Use up the limit
			for i := 0; i < limit; i++ {
				ml.Allow(userID)
			}

			// Get retry after value
			retryAfter := ml.GetRetryAfter(userID)

			// Should be between 0 and 1000ms (1 second window)
			if retryAfter < 0 || retryAfter > 1000 {
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.IntRange(1, 20),
	))

	properties.Property("cleanup removes expired events", prop.ForAll(
		func(userID string, limit int) bool {
			// Skip invalid inputs
			if userID == "" || limit <= 0 || limit > 50 {
				return true
			}

			// Create message limiter with very short window
			ml := NewMessageLimiter(50*time.Millisecond, limit)

			// Add some events
			for i := 0; i < limit; i++ {
				ml.Allow(userID)
			}

			// Wait for events to expire
			time.Sleep(100 * time.Millisecond)

			// Cleanup
			ml.Cleanup()

			// Should be able to make requests again (events were cleaned up)
			if !ml.Allow(userID) {
				return false
			}

			return true
		},
		gen.AlphaString(),
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket
// Property 43: Rate Limiting Enforcement (Concurrent Access)
// **Validates: Requirements 13.3**
//
// For any concurrent requests from multiple users, the rate limiter should
// correctly enforce limits without race conditions.
func TestProperty_RateLimitingConcurrency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("message limiter handles concurrent access safely", prop.ForAll(
		func(numUsers int, limit int) bool {
			// Skip invalid inputs
			if numUsers <= 0 || numUsers > 20 || limit <= 0 || limit > 50 {
				return true
			}

			// Create message limiter
			ml := NewMessageLimiter(1*time.Second, limit)

			// Create channels for synchronization
			done := make(chan bool, numUsers)

			// Launch goroutines for each user
			for i := 0; i < numUsers; i++ {
				go func(userID int) {
					// Each user makes limit+5 requests
					for j := 0; j < limit+5; j++ {
						ml.Allow(string(rune('A' + userID)))
					}
					done <- true
				}(i)
			}

			// Wait for all goroutines to complete
			for i := 0; i < numUsers; i++ {
				<-done
			}

			// Verify each user has exactly 'limit' events
			for i := 0; i < numUsers; i++ {
				userID := string(rune('A' + i))
				ml.mu.RLock()
				events := ml.events[userID]
				ml.mu.RUnlock()

				if len(events) != limit {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
		gen.IntRange(1, 30),
	))

	properties.Property("connection limiter handles concurrent access safely", prop.ForAll(
		func(numUsers int, maxConnections int) bool {
			// Skip invalid inputs
			if numUsers <= 0 || numUsers > 20 || maxConnections <= 0 || maxConnections > 30 {
				return true
			}

			// Create connection limiter
			cl := NewConnectionLimiter(maxConnections)

			// Create channels for synchronization
			done := make(chan bool, numUsers)

			// Launch goroutines for each user
			for i := 0; i < numUsers; i++ {
				go func(userID int) {
					// Each user attempts maxConnections+5 connections
					for j := 0; j < maxConnections+5; j++ {
						cl.Allow(string(rune('A' + userID)))
					}
					done <- true
				}(i)
			}

			// Wait for all goroutines to complete
			for i := 0; i < numUsers; i++ {
				<-done
			}

			// Verify each user has exactly 'maxConnections' connections
			for i := 0; i < numUsers; i++ {
				userID := string(rune('A' + i))
				count := cl.GetCount(userID)

				if count != maxConnections {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
