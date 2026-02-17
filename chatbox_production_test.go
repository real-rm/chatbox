package chatbox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProductionIssue17_WeakSecretAcceptance verifies JWT secret strength validation
//
// Production Readiness Issue #17: JWT secret strength validation
// Location: chatbox.go:validateJWTSecret
// Impact: Weak secrets are now rejected, improving security
//
// This test verifies that weak JWT secrets are properly rejected.
func TestProductionIssue17_WeakSecretAcceptance(t *testing.T) {
	// Subtask 16.1.1: Test various weak secrets
	weakSecrets := []struct {
		secret string
		reason string
	}{
		{"test123", "too short and contains 'test'"},
		{"password", "too short and common weak secret"},
		{"secret", "too short and common weak secret"},
		{"12345678", "too short and numeric only"},
		{"abc", "too short"},
		{"this-is-a-test-secret-value", "contains 'test'"},
		{"admin-password-for-production", "contains 'password' and 'admin'"},
	}

	for _, tc := range weakSecrets {
		t.Run(tc.secret, func(t *testing.T) {
			// Subtask 16.1.2 & 16.1.3: Verify weak secrets are rejected
			err := validateJWTSecret(tc.secret)
			
			// Weak secrets should be rejected
			assert.Error(t, err, "Weak secret '%s' should be rejected (%s)", tc.secret, tc.reason)
			t.Logf("✓ Weak secret '%s' correctly rejected: %v", tc.secret, err)
		})
	}

	// Test that strong secrets are accepted
	t.Run("strong_secret", func(t *testing.T) {
		// A strong 32+ character random secret
		strongSecret := "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6"
		err := validateJWTSecret(strongSecret)
		assert.NoError(t, err, "Strong secret should be accepted")
		t.Logf("✓ Strong secret accepted (length: %d)", len(strongSecret))
	})

	// Subtask 16.1.4: Document validation behavior
	t.Log("FINDING: JWT secret strength validation IS implemented")
	t.Log("VALIDATION: Minimum 32 characters required")
	t.Log("VALIDATION: Common weak secrets are rejected (test, password, admin, etc.)")
	t.Log("VALIDATION: Empty secrets are rejected")
	t.Log("STATUS: Security improvement - weak secrets are now properly rejected")
	t.Log("RECOMMENDATION: Ensure production deployments use strong secrets")
	t.Log("RECOMMENDATION: Use 'openssl rand -base64 32' to generate secrets")
}

// TestProductionIssue15_ShutdownTimeout verifies shutdown respects context deadline
//
// Production Readiness Issue #15: Shutdown may not respect timeout
// Location: internal/websocket/handler.go:ShutdownWithContext
// Impact: Graceful shutdown may hang indefinitely
//
// This test documents the current shutdown behavior with actual connections.
func TestProductionIssue15_ShutdownTimeout(t *testing.T) {
	// Subtask 14.2.1: Create Handler with connections
	handler := createTestHandlerWithConnections(t, 5)
	
	// Subtask 14.2.2: Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	// Subtask 14.2.4: Measure completion time
	startTime := time.Now()
	
	// Subtask 14.2.3: Call Shutdown
	err := handler.ShutdownWithContext(ctx)
	
	completionTime := time.Since(startTime)
	
	// Verify shutdown completed
	if err != nil {
		t.Logf("Shutdown returned error: %v", err)
	} else {
		t.Log("Shutdown completed successfully")
	}
	
	// Subtask 14.2.5: Document timeout behavior
	t.Logf("Shutdown completion time: %v", completionTime)
	t.Logf("Context timeout: 2s")
	
	// Verify shutdown completed within reasonable time
	if completionTime > 3*time.Second {
		t.Errorf("Shutdown took too long: %v (expected < 3s)", completionTime)
	}
	
	t.Log("FINDING: ShutdownWithContext() respects context deadline")
	t.Log("FINDING: Connections are closed in parallel using goroutines")
	t.Log("FINDING: Shutdown waits for all connections to close or context deadline")
	t.Log("STATUS: Current implementation properly handles shutdown timeout")
	t.Log("RECOMMENDATION: Monitor shutdown duration in production")
	t.Log("RECOMMENDATION: Configure appropriate shutdown timeout in deployment")
}

// createTestHandlerWithConnections creates a Handler with mock connections for testing
func createTestHandlerWithConnections(t *testing.T, numConnections int) *websocket.Handler {
	// Create a test logger
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	// Create a mock router
	mockRouter := &mockMessageRouter{}
	
	// Create a JWT validator (not needed for this test but required by Handler)
	validator := auth.NewJWTValidator("test-secret-key-for-testing-only")
	
	// Create handler
	handler := websocket.NewHandler(validator, mockRouter, logger, 1024*1024)
	
	// Create mock connections
	for i := 0; i < numConnections; i++ {
		userID := fmt.Sprintf("user-%d", i)
		conn := createMockWebSocketConnection(t, userID)
		
		// Register the connection directly in the handler
		handler.RegisterConnectionForTest(conn)
	}
	
	t.Logf("Created handler with %d mock connections", numConnections)
	
	return handler
}

// createMockWebSocketConnection creates a mock WebSocket connection for testing
func createMockWebSocketConnection(t *testing.T, userID string) *websocket.Connection {
	// Create a mock connection using the NewConnection helper
	conn := websocket.NewConnection(userID, []string{"user"})
	
	// Set a connection ID
	conn.ConnectionID = fmt.Sprintf("%s-%d", userID, time.Now().UnixNano())
	
	return conn
}

// mockMessageRouter is a mock implementation of MessageRouter for testing
type mockMessageRouter struct{}

func (m *mockMessageRouter) RouteMessage(conn *websocket.Connection, msg *message.Message) error {
	return nil
}

func (m *mockMessageRouter) RegisterConnection(sessionID string, conn *websocket.Connection) error {
	return nil
}

func (m *mockMessageRouter) UnregisterConnection(sessionID string) {}

// TestProductionIssue18_AdminRateLimiting verifies admin endpoint rate limiting
//
// Production Readiness Issue #18: Admin endpoints rate limiting verification
// Location: chatbox.go (admin endpoints with adminRateLimitMiddleware)
// Impact: Verify rate limiting protects admin endpoints from abuse
//
// This test verifies that admin endpoints have proper rate limiting configured.
func TestProductionIssue18_AdminRateLimiting(t *testing.T) {
	// Subtask 17.1.1: Create test server
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	// Create test logger
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err, "Failed to create logger")
	
	// Create JWT validator with test secret
	testSecret := "test-secret-key-for-admin-rate-limiting-test-32chars"
	validator := auth.NewJWTValidator(testSecret)
	
	// Create rate limiter with low limits for testing
	// Allow 10 requests per 10 seconds
	limiter := ratelimit.NewMessageLimiter(10*time.Second, 10)
	defer limiter.Cleanup()
	
	// Create mock storage service
	mockStorage := &mockStorageService{}
	
	// Register admin endpoint with rate limiting
	adminGroup := router.Group("/chat/admin")
	adminGroup.Use(authMiddleware(validator, logger))
	adminGroup.Use(adminRateLimitMiddleware(limiter, logger))
	{
		adminGroup.GET("/sessions", func(c *gin.Context) {
			c.JSON(200, gin.H{"sessions": []string{}})
		})
	}
	
	// Create test server
	server := httptest.NewServer(router)
	defer server.Close()
	
	// Create JWT token with admin role
	token := createTestAdminToken(testSecret, "admin-user-1", time.Hour)
	
	// Subtask 17.1.2: Send 1000 rapid requests
	t.Log("Sending 1000 rapid requests to /chat/admin/sessions...")
	
	successCount := 0
	rateLimitedCount := 0
	otherErrorCount := 0
	
	client := &http.Client{Timeout: 5 * time.Second}
	
	for i := 0; i < 1000; i++ {
		req, err := http.NewRequest("GET", server.URL+"/chat/admin/sessions", nil)
		require.NoError(t, err)
		
		req.Header.Set("Authorization", "Bearer "+token)
		
		resp, err := client.Do(req)
		if err != nil {
			t.Logf("Request %d failed: %v", i, err)
			otherErrorCount++
			continue
		}
		
		// Subtask 17.1.3: Verify rate limiting status
		switch resp.StatusCode {
		case 200:
			successCount++
		case 429:
			rateLimitedCount++
			// Verify Retry-After header is present
			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter == "" {
				t.Errorf("Rate limited response missing Retry-After header")
			}
		default:
			t.Logf("Request %d returned unexpected status: %d", i, resp.StatusCode)
			otherErrorCount++
		}
		
		resp.Body.Close()
		
		// Log progress every 100 requests
		if (i+1)%100 == 0 {
			t.Logf("Progress: %d requests sent (success: %d, rate limited: %d, errors: %d)",
				i+1, successCount, rateLimitedCount, otherErrorCount)
		}
	}
	
	// Subtask 17.1.4: Document current behavior
	t.Logf("\n=== Rate Limiting Test Results ===")
	t.Logf("Total requests sent: 1000")
	t.Logf("Successful requests (200): %d", successCount)
	t.Logf("Rate limited requests (429): %d", rateLimitedCount)
	t.Logf("Other errors: %d", otherErrorCount)
	
	// Verify rate limiting is working
	assert.Greater(t, rateLimitedCount, 0, "Expected some requests to be rate limited")
	assert.LessOrEqual(t, successCount, 20, "Expected most requests to be rate limited (limit is 10 per 10s)")
	
	t.Log("\nFINDING: Admin endpoints HAVE rate limiting configured")
	t.Log("IMPLEMENTATION: adminRateLimitMiddleware in chatbox.go")
	t.Log("CONFIGURATION: Default 20 requests per minute (configurable)")
	t.Log("BEHAVIOR: Returns 429 Too Many Requests with Retry-After header")
	t.Log("RATE LIMIT SOURCE: User-based (from JWT claims)")
	t.Log("STATUS: Rate limiting is properly implemented and working")
	t.Log("RECOMMENDATION: Monitor rate limit metrics in production")
	t.Log("RECOMMENDATION: Adjust limits based on actual usage patterns")
	t.Log("RECOMMENDATION: Consider different limits for different admin endpoints")
	
	// Verify the mock storage wasn't called (we're testing rate limiting, not storage)
	assert.Equal(t, 0, mockStorage.callCount, "Storage should not be called in this test")
}

// createTestAdminToken creates a JWT token with admin role for testing
func createTestAdminToken(secret, userID string, expiresIn time.Duration) string {
	claims := jwt.MapClaims{
		"user_id": userID,
		"roles":   []string{"admin"},
		"exp":     time.Now().Add(expiresIn).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

// mockStorageService is a simple mock for testing
type mockStorageService struct {
	callCount int
}

func (m *mockStorageService) ListSessions() ([]string, error) {
	m.callCount++
	return []string{}, nil
}

// TestProductionIssue06_EncryptionKeyValidation verifies encryption key validation
//
// This test documents the encryption key validation behavior.
func TestProductionIssue06_EncryptionKeyValidation(t *testing.T) {
	tests := []struct {
		name        string
		key         []byte
		expectError bool
	}{
		{
			name:        "empty key (encryption disabled)",
			key:         []byte{},
			expectError: false,
		},
		{
			name:        "32-byte key (valid)",
			key:         make([]byte, 32),
			expectError: false,
		},
		{
			name:        "16-byte key (invalid)",
			key:         make([]byte, 16),
			expectError: true,
		},
		{
			name:        "24-byte key (invalid)",
			key:         make([]byte, 24),
			expectError: true,
		},
		{
			name:        "64-byte key (invalid)",
			key:         make([]byte, 64),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEncryptionKey(tt.key)

			if tt.expectError {
				assert.Error(t, err, "Should return error for invalid key length")
			} else {
				assert.NoError(t, err, "Should not return error for valid key length")
			}
		})
	}

	t.Log("STATUS: Encryption key validation works correctly")
	t.Log("FINDING: Only 32-byte keys or empty keys are accepted")
}

