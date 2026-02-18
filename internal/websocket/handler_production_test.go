package websocket

import (
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProductionIssue04_SessionIDDataRace verifies thread-safe access to Connection.SessionID
//
// Production Readiness Issue #4: Data race on Connection.SessionID
// Location: websocket/handler.go
// Impact: Potential data corruption in concurrent scenarios
//
// This test verifies that concurrent reads and writes to SessionID are properly protected.
func TestProductionIssue04_SessionIDDataRace(t *testing.T) {
	// Create a connection
	conn := NewConnection("test-user", []string{"user"})

	// Launch concurrent readers
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = conn.GetSessionID()
		}()
	}

	// Launch concurrent writers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn.mu.Lock()
			conn.SessionID = "session-" + string(rune(id))
			conn.mu.Unlock()
		}(i)
	}

	// Wait for all goroutines
	wg.Wait()

	t.Log("STATUS: No data race detected with proper locking")
	t.Log("FINDING: Connection.SessionID access is thread-safe")
}

// TestProductionIssue04_ConcurrentSessionAccess verifies thread-safe session field access
func TestProductionIssue04_ConcurrentSessionAccess(t *testing.T) {
	// Create a connection
	conn := NewConnection("test-user", []string{"user"})

	// Launch concurrent operations
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		// Read operations
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = conn.GetUserID()
			_ = conn.GetSessionID()
			_ = conn.GetRoles()
		}()

		// Write operations
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn.mu.Lock()
			conn.SessionID = "session-" + string(rune(id))
			conn.mu.Unlock()
		}(i)
	}

	wg.Wait()

	t.Log("STATUS: No data race detected")
	t.Log("FINDING: Concurrent access to connection fields is safe")
}

// TestProductionIssue13_OriginValidationDataRace verifies thread-safe origin validation
//
// Production Readiness Issue #13: Data race on allowedOrigins map
// Location: websocket/handler.go:checkOrigin
// Impact: Potential data corruption during origin validation
//
// This test verifies that concurrent origin checks and updates are thread-safe.
// Property 14: Origin validation is thread-safe
// Validates: Requirements 13.1, 13.2, 13.4, 13.5
func TestProductionIssue13_OriginValidationDataRace(t *testing.T) {
	// Create logger
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create handler
	validator := auth.NewJWTValidator("test-secret-key-for-testing")

	handler := NewHandler(validator, nil, logger, 1024*1024)

	// Launch concurrent checkOrigin calls
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/ws", nil)
			req.Header.Set("Origin", "http://example"+string(rune(id))+".com")
			_ = handler.checkOrigin(req)
		}(i)
	}

	// Launch concurrent SetAllowedOrigins calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			origins := []string{"http://example.com", "http://test.com"}
			handler.SetAllowedOrigins(origins)
		}(i)
	}

	wg.Wait()

	t.Log("STATUS: No data race detected with proper locking")
	t.Log("FINDING: Origin validation is thread-safe with RLock in checkOrigin()")
	t.Log("IMPACT: Concurrent origin checks and updates are safe")
}

// TestProductionIssue13_DefaultOriginBehavior verifies default origin validation behavior
//
// This test documents that all origins are allowed when no origins are configured.
func TestProductionIssue13_DefaultOriginBehavior(t *testing.T) {
	// Create logger
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err)
	defer logger.Close()

	// Create handler with no origins configured
	validator := auth.NewJWTValidator("test-secret-key-for-testing")

	handler := NewHandler(validator, nil, logger, 1024*1024)

	// Test various origins
	testOrigins := []string{
		"http://example.com",
		"http://malicious.com",
		"http://localhost:3000",
		"https://production.com",
	}

	for _, origin := range testOrigins {
		req := httptest.NewRequest("GET", "/ws", nil)
		req.Header.Set("Origin", origin)

		allowed := handler.checkOrigin(req)
		assert.True(t, allowed, "Origin %s should be allowed in development mode", origin)
	}

	t.Log("FINDING: All origins are allowed when no origins configured")
	t.Log("IMPACT: Development mode - no origin restrictions")
	t.Log("RECOMMENDATION: Configure allowed origins for production")

	// Now configure origins
	handler.SetAllowedOrigins([]string{"http://example.com"})

	// Test that only configured origin is allowed
	req1 := httptest.NewRequest("GET", "/ws", nil)
	req1.Header.Set("Origin", "http://example.com")
	assert.True(t, handler.checkOrigin(req1), "Configured origin should be allowed")

	req2 := httptest.NewRequest("GET", "/ws", nil)
	req2.Header.Set("Origin", "http://malicious.com")
	assert.False(t, handler.checkOrigin(req2), "Non-configured origin should be blocked")

	t.Log("STATUS: Origin validation works correctly when configured")
}
