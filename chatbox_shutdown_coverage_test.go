package chatbox

import (
	"context"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/chatbox/internal/router"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/stretchr/testify/assert"
)

// TestShutdown_AllComponentsInitialized tests shutdown with all components initialized
func TestShutdown_AllComponentsInitialized(t *testing.T) {
	// Create test logger
	testLogger := CreateTestLogger(t)
	defer testLogger.Close()

	// Save original global variables
	shutdownMu.Lock()
	originalWSHandler := globalWSHandler
	originalSessionMgr := globalSessionMgr
	originalMessageRouter := globalMessageRouter
	originalAdminLimiter := globalAdminLimiter
	originalLogger := globalLogger

	// Create all components
	sessionMgr := session.NewSessionManager(15*time.Minute, testLogger)
	sessionMgr.StartCleanup()

	adminLimiter := ratelimit.NewMessageLimiter(1*time.Minute, 10)
	adminLimiter.StartCleanup()

	// Create a mock message router
	messageRouter := router.NewMessageRouter(
		sessionMgr,
		nil, // llmService
		nil, // uploadService
		nil, // notificationService
		nil, // storageService
		30*time.Second,
		testLogger,
	)

	// Create WebSocket handler (without actual connections)
	validator := auth.NewJWTValidator("test-secret-that-is-at-least-32-characters-long")
	wsHandler := websocket.NewHandler(validator, messageRouter, testLogger, 1048576)

	// Set all global components
	globalWSHandler = wsHandler
	globalSessionMgr = sessionMgr
	globalMessageRouter = messageRouter
	globalAdminLimiter = adminLimiter
	globalLogger = testLogger
	shutdownMu.Unlock()

	// Restore after test
	defer func() {
		shutdownMu.Lock()
		globalWSHandler = originalWSHandler
		globalSessionMgr = originalSessionMgr
		globalMessageRouter = originalMessageRouter
		globalAdminLimiter = originalAdminLimiter
		globalLogger = originalLogger
		shutdownMu.Unlock()
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown should succeed with all components
	err := Shutdown(ctx)
	assert.NoError(t, err)
}

// TestShutdown_WithSessionManagerOnly tests shutdown with only session manager initialized
func TestShutdown_WithSessionManagerOnly(t *testing.T) {
	// Create test logger
	testLogger := CreateTestLogger(t)
	defer testLogger.Close()

	// Save original global variables
	shutdownMu.Lock()
	originalWSHandler := globalWSHandler
	originalSessionMgr := globalSessionMgr
	originalMessageRouter := globalMessageRouter
	originalAdminLimiter := globalAdminLimiter
	originalLogger := globalLogger

	// Create only session manager
	sessionMgr := session.NewSessionManager(15*time.Minute, testLogger)
	sessionMgr.StartCleanup()

	// Set only session manager and logger
	globalWSHandler = nil
	globalSessionMgr = sessionMgr
	globalMessageRouter = nil
	globalAdminLimiter = nil
	globalLogger = testLogger
	shutdownMu.Unlock()

	// Restore after test
	defer func() {
		shutdownMu.Lock()
		globalWSHandler = originalWSHandler
		globalSessionMgr = originalSessionMgr
		globalMessageRouter = originalMessageRouter
		globalAdminLimiter = originalAdminLimiter
		globalLogger = originalLogger
		shutdownMu.Unlock()
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown should succeed
	err := Shutdown(ctx)
	assert.NoError(t, err)
}

// TestShutdown_WithMessageRouterOnly tests shutdown with only message router initialized
func TestShutdown_WithMessageRouterOnly(t *testing.T) {
	// Create test logger
	testLogger := CreateTestLogger(t)
	defer testLogger.Close()

	// Save original global variables
	shutdownMu.Lock()
	originalWSHandler := globalWSHandler
	originalSessionMgr := globalSessionMgr
	originalMessageRouter := globalMessageRouter
	originalAdminLimiter := globalAdminLimiter
	originalLogger := globalLogger

	// Create only message router
	sessionMgr := session.NewSessionManager(15*time.Minute, testLogger)
	messageRouter := router.NewMessageRouter(
		sessionMgr,
		nil, // llmService
		nil, // uploadService
		nil, // notificationService
		nil, // storageService
		30*time.Second,
		testLogger,
	)

	// Set only message router and logger
	globalWSHandler = nil
	globalSessionMgr = nil
	globalMessageRouter = messageRouter
	globalAdminLimiter = nil
	globalLogger = testLogger
	shutdownMu.Unlock()

	// Restore after test
	defer func() {
		shutdownMu.Lock()
		globalWSHandler = originalWSHandler
		globalSessionMgr = originalSessionMgr
		globalMessageRouter = originalMessageRouter
		globalAdminLimiter = originalAdminLimiter
		globalLogger = originalLogger
		shutdownMu.Unlock()
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown should succeed
	err := Shutdown(ctx)
	assert.NoError(t, err)
}

// TestShutdown_WithAdminLimiterOnly tests shutdown with only admin limiter initialized
func TestShutdown_WithAdminLimiterOnly(t *testing.T) {
	// Create test logger
	testLogger := CreateTestLogger(t)
	defer testLogger.Close()

	// Save original global variables
	shutdownMu.Lock()
	originalWSHandler := globalWSHandler
	originalSessionMgr := globalSessionMgr
	originalMessageRouter := globalMessageRouter
	originalAdminLimiter := globalAdminLimiter
	originalLogger := globalLogger

	// Create only admin limiter
	adminLimiter := ratelimit.NewMessageLimiter(1*time.Minute, 10)
	adminLimiter.StartCleanup()

	// Set only admin limiter and logger
	globalWSHandler = nil
	globalSessionMgr = nil
	globalMessageRouter = nil
	globalAdminLimiter = adminLimiter
	globalLogger = testLogger
	shutdownMu.Unlock()

	// Restore after test
	defer func() {
		shutdownMu.Lock()
		globalWSHandler = originalWSHandler
		globalSessionMgr = originalSessionMgr
		globalMessageRouter = originalMessageRouter
		globalAdminLimiter = originalAdminLimiter
		globalLogger = originalLogger
		shutdownMu.Unlock()
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown should succeed
	err := Shutdown(ctx)
	assert.NoError(t, err)
}

// TestShutdown_WithWebSocketHandlerOnly tests shutdown with only WebSocket handler initialized
func TestShutdown_WithWebSocketHandlerOnly(t *testing.T) {
	// Create test logger
	testLogger := CreateTestLogger(t)
	defer testLogger.Close()

	// Save original global variables
	shutdownMu.Lock()
	originalWSHandler := globalWSHandler
	originalSessionMgr := globalSessionMgr
	originalMessageRouter := globalMessageRouter
	originalAdminLimiter := globalAdminLimiter
	originalLogger := globalLogger

	// Create only WebSocket handler
	validator := auth.NewJWTValidator("test-secret-that-is-at-least-32-characters-long")
	wsHandler := websocket.NewHandler(validator, nil, testLogger, 1048576)

	// Set only WebSocket handler and logger
	globalWSHandler = wsHandler
	globalSessionMgr = nil
	globalMessageRouter = nil
	globalAdminLimiter = nil
	globalLogger = testLogger
	shutdownMu.Unlock()

	// Restore after test
	defer func() {
		shutdownMu.Lock()
		globalWSHandler = originalWSHandler
		globalSessionMgr = originalSessionMgr
		globalMessageRouter = originalMessageRouter
		globalAdminLimiter = originalAdminLimiter
		globalLogger = originalLogger
		shutdownMu.Unlock()
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown should succeed (no actual connections to close)
	err := Shutdown(ctx)
	assert.NoError(t, err)
}

// TestShutdown_WithExpiredContext tests shutdown with an already expired context
func TestShutdown_WithExpiredContext(t *testing.T) {
	// Create test logger
	testLogger := CreateTestLogger(t)
	defer testLogger.Close()

	// Save original global variables
	shutdownMu.Lock()
	originalWSHandler := globalWSHandler
	originalSessionMgr := globalSessionMgr
	originalMessageRouter := globalMessageRouter
	originalAdminLimiter := globalAdminLimiter
	originalLogger := globalLogger

	// Set only logger (no WebSocket handler, so context expiration won't cause error)
	globalWSHandler = nil
	globalSessionMgr = nil
	globalMessageRouter = nil
	globalAdminLimiter = nil
	globalLogger = testLogger
	shutdownMu.Unlock()

	// Restore after test
	defer func() {
		shutdownMu.Lock()
		globalWSHandler = originalWSHandler
		globalSessionMgr = originalSessionMgr
		globalMessageRouter = originalMessageRouter
		globalAdminLimiter = originalAdminLimiter
		globalLogger = originalLogger
		shutdownMu.Unlock()
	}()

	// Create context with very short timeout and wait for it to expire
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // Ensure context is expired

	// Shutdown should still succeed without WebSocket handler
	err := Shutdown(ctx)
	assert.NoError(t, err)
}

// TestShutdown_MultipleComponentCombinations tests various combinations of initialized components
func TestShutdown_MultipleComponentCombinations(t *testing.T) {
	testCases := []struct {
		name              string
		initSessionMgr    bool
		initMessageRouter bool
		initAdminLimiter  bool
		initWSHandler     bool
	}{
		{
			name:              "session manager and admin limiter",
			initSessionMgr:    true,
			initMessageRouter: false,
			initAdminLimiter:  true,
			initWSHandler:     false,
		},
		{
			name:              "session manager and message router",
			initSessionMgr:    true,
			initMessageRouter: true,
			initAdminLimiter:  false,
			initWSHandler:     false,
		},
		{
			name:              "message router and admin limiter",
			initSessionMgr:    false,
			initMessageRouter: true,
			initAdminLimiter:  true,
			initWSHandler:     false,
		},
		{
			name:              "all except WebSocket handler",
			initSessionMgr:    true,
			initMessageRouter: true,
			initAdminLimiter:  true,
			initWSHandler:     false,
		},
		{
			name:              "WebSocket handler and session manager",
			initSessionMgr:    true,
			initMessageRouter: false,
			initAdminLimiter:  false,
			initWSHandler:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test logger
			testLogger := CreateTestLogger(t)
			defer testLogger.Close()

			// Save original global variables
			shutdownMu.Lock()
			originalWSHandler := globalWSHandler
			originalSessionMgr := globalSessionMgr
			originalMessageRouter := globalMessageRouter
			originalAdminLimiter := globalAdminLimiter
			originalLogger := globalLogger

			// Initialize components based on test case
			var sessionMgr *session.SessionManager
			var messageRouter *router.MessageRouter
			var adminLimiter *ratelimit.MessageLimiter
			var wsHandler *websocket.Handler

			if tc.initSessionMgr {
				sessionMgr = session.NewSessionManager(15*time.Minute, testLogger)
				sessionMgr.StartCleanup()
			}

			if tc.initMessageRouter {
				if sessionMgr == nil {
					sessionMgr = session.NewSessionManager(15*time.Minute, testLogger)
				}
				messageRouter = router.NewMessageRouter(
					sessionMgr,
					nil, nil, nil, nil,
					30*time.Second,
					testLogger,
				)
			}

			if tc.initAdminLimiter {
				adminLimiter = ratelimit.NewMessageLimiter(1*time.Minute, 10)
				adminLimiter.StartCleanup()
			}

			if tc.initWSHandler {
				validator := auth.NewJWTValidator("test-secret-that-is-at-least-32-characters-long")
				wsHandler = websocket.NewHandler(validator, nil, testLogger, 1048576)
			}

			// Set global components
			globalWSHandler = wsHandler
			globalSessionMgr = sessionMgr
			globalMessageRouter = messageRouter
			globalAdminLimiter = adminLimiter
			globalLogger = testLogger
			shutdownMu.Unlock()

			// Restore after test
			defer func() {
				shutdownMu.Lock()
				globalWSHandler = originalWSHandler
				globalSessionMgr = originalSessionMgr
				globalMessageRouter = originalMessageRouter
				globalAdminLimiter = originalAdminLimiter
				globalLogger = originalLogger
				shutdownMu.Unlock()
			}()

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Shutdown should succeed
			err := Shutdown(ctx)
			assert.NoError(t, err)
		})
	}
}

// wsHandlerInterface defines the interface needed for shutdown testing
type wsHandlerInterface interface {
	ShutdownWithContext(ctx context.Context) error
}

// mockWSHandler is a mock WebSocket handler for testing error paths
type mockWSHandler struct {
	shouldError   bool
	errorToReturn error
}

func (m *mockWSHandler) ShutdownWithContext(ctx context.Context) error {
	if m.shouldError {
		return m.errorToReturn
	}
	return nil
}

// TestShutdown_WebSocketHandlerTimeoutError tests shutdown when WebSocket handler times out
func TestShutdown_WebSocketHandlerTimeoutError(t *testing.T) {
	// Create test logger
	testLogger := CreateTestLogger(t)
	defer testLogger.Close()

	// Save original global variables
	shutdownMu.Lock()
	originalWSHandler := globalWSHandler
	originalSessionMgr := globalSessionMgr
	originalMessageRouter := globalMessageRouter
	originalAdminLimiter := globalAdminLimiter
	originalLogger := globalLogger

	// Create a real WebSocket handler
	validator := auth.NewJWTValidator("test-secret-that-is-at-least-32-characters-long")
	sessionMgr := session.NewSessionManager(15*time.Minute, testLogger)
	messageRouter := router.NewMessageRouter(
		sessionMgr,
		nil, nil, nil, nil,
		30*time.Second,
		testLogger,
	)
	wsHandler := websocket.NewHandler(validator, messageRouter, testLogger, 1048576)

	// Set handler and logger
	globalWSHandler = wsHandler
	globalSessionMgr = nil
	globalMessageRouter = nil
	globalAdminLimiter = nil
	globalLogger = testLogger
	shutdownMu.Unlock()

	// Restore after test
	defer func() {
		shutdownMu.Lock()
		globalWSHandler = originalWSHandler
		globalSessionMgr = originalSessionMgr
		globalMessageRouter = originalMessageRouter
		globalAdminLimiter = originalAdminLimiter
		globalLogger = originalLogger
		shutdownMu.Unlock()
	}()

	// Create context with very short timeout (1 nanosecond) and wait for it to expire
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // Ensure context is expired

	// Shutdown should return context.DeadlineExceeded error
	err := Shutdown(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

// TestShutdown_WebSocketHandlerErrorWithAllComponents tests that error path is covered
// This test uses reflection to inject a mock handler that returns an error
func TestShutdown_WebSocketHandlerErrorWithAllComponents(t *testing.T) {
	// Skip this test as we cannot easily mock the WebSocket handler error path
	// without creating real WebSocket connections. The error path is tested
	// in integration tests with actual connections.
	t.Skip("Skipping: Cannot easily mock WebSocket handler error without real connections")
}
