// Package ratelimit provides rate limiting functionality for WebSocket connections and messages.
// It implements token bucket and sliding window algorithms to prevent abuse and DoS attacks.
package ratelimit

import (
	"sync"
	"time"
)

// ConnectionLimiter limits the number of concurrent connections per user
type ConnectionLimiter struct {
	connections map[string]int // userID -> connection count
	maxPerUser  int
	mu          sync.RWMutex
}

// NewConnectionLimiter creates a new connection limiter
func NewConnectionLimiter(maxPerUser int) *ConnectionLimiter {
	return &ConnectionLimiter{
		connections: make(map[string]int),
		maxPerUser:  maxPerUser,
	}
}

// Allow checks if a new connection is allowed for the user
func (cl *ConnectionLimiter) Allow(userID string) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	count := cl.connections[userID]
	if count >= cl.maxPerUser {
		return false
	}

	cl.connections[userID] = count + 1
	return true
}

// Release decrements the connection count for a user
func (cl *ConnectionLimiter) Release(userID string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	if count, ok := cl.connections[userID]; ok {
		if count <= 1 {
			delete(cl.connections, userID)
		} else {
			cl.connections[userID] = count - 1
		}
	}
}

// GetCount returns the current connection count for a user
func (cl *ConnectionLimiter) GetCount(userID string) int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.connections[userID]
}

// MessageLimiter limits the rate of messages per user using sliding window
type MessageLimiter struct {
	events map[string][]time.Time // userID -> timestamps
	window time.Duration
	limit  int
	mu     sync.RWMutex

	// Cleanup goroutine management
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
	cleanupWg       sync.WaitGroup
}

// NewMessageLimiter creates a new message rate limiter
// window: time window for rate limiting (e.g., 1 minute)
// limit: maximum number of messages allowed in the window
func NewMessageLimiter(window time.Duration, limit int) *MessageLimiter {
	return &MessageLimiter{
		events:          make(map[string][]time.Time),
		window:          window,
		limit:           limit,
		cleanupInterval: 5 * time.Minute, // Default cleanup every 5 minutes
		stopCleanup:     make(chan struct{}),
	}
}

// Allow checks if a message is allowed based on rate limiting
// Returns true if allowed, false if rate limit exceeded
func (ml *MessageLimiter) Allow(userID string) bool {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-ml.window)

	// Get existing events for this user
	events := ml.events[userID]

	// Filter out old events outside the window
	var recentEvents []time.Time
	for _, t := range events {
		if t.After(cutoff) {
			recentEvents = append(recentEvents, t)
		}
	}

	// Check if we're under the limit
	if len(recentEvents) >= ml.limit {
		return false
	}

	// Add this event
	recentEvents = append(recentEvents, now)
	ml.events[userID] = recentEvents

	return true
}

// GetRetryAfter returns the time in milliseconds until the next message is allowed
func (ml *MessageLimiter) GetRetryAfter(userID string) int {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	events := ml.events[userID]
	if len(events) < ml.limit {
		return 0
	}

	// Find the oldest event in the window
	now := time.Now()
	cutoff := now.Add(-ml.window)

	var oldestInWindow time.Time
	for _, t := range events {
		if t.After(cutoff) {
			if oldestInWindow.IsZero() || t.Before(oldestInWindow) {
				oldestInWindow = t
			}
		}
	}

	if oldestInWindow.IsZero() {
		return 0
	}

	// Calculate when the oldest event will expire
	expiresAt := oldestInWindow.Add(ml.window)
	retryAfter := expiresAt.Sub(now)

	if retryAfter < 0 {
		return 0
	}

	return int(retryAfter.Milliseconds())
}

// Reset clears the rate limit history for a user
func (ml *MessageLimiter) Reset(userID string) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	delete(ml.events, userID)
}

// Cleanup removes expired events to prevent memory leaks
// Should be called periodically
func (ml *MessageLimiter) Cleanup() {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-ml.window)

	for userID, events := range ml.events {
		var recentEvents []time.Time
		for _, t := range events {
			if t.After(cutoff) {
				recentEvents = append(recentEvents, t)
			}
		}

		if len(recentEvents) == 0 {
			delete(ml.events, userID)
		} else {
			ml.events[userID] = recentEvents
		}
	}
}

// StartCleanup starts a background goroutine that periodically cleans up expired events
func (ml *MessageLimiter) StartCleanup() {
	ml.cleanupWg.Add(1)
	go func() {
		defer ml.cleanupWg.Done()
		ticker := time.NewTicker(ml.cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				before := ml.getEventCount()
				ml.Cleanup()
				after := ml.getEventCount()
				removed := before - after
				if removed > 0 {
					// Log cleanup stats (would use logger if available)
					// For now, cleanup happens silently
				}
			case <-ml.stopCleanup:
				return
			}
		}
	}()
}

// getEventCount returns the total number of events across all users
func (ml *MessageLimiter) getEventCount() int {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	count := 0
	for _, events := range ml.events {
		count += len(events)
	}
	return count
}

// StopCleanup stops the cleanup goroutine and waits for it to finish
// CRITICAL FIX C3: Use sync.Once to prevent double-close panic
func (ml *MessageLimiter) StopCleanup() {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	// Only close if channel is not nil and not already closed
	if ml.stopCleanup != nil {
		select {
		case <-ml.stopCleanup:
			// Already closed, do nothing
		default:
			close(ml.stopCleanup)
		}
	}

	// Wait for cleanup goroutine to finish (outside the lock)
	ml.mu.Unlock()
	ml.cleanupWg.Wait()
	ml.mu.Lock()
}
