package chatbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/constants"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/chatbox/internal/router"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/storage"
	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
)

// Test helper functions

// setupTestRouter creates a test Gin router with test mode enabled
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// setupTestConfig creates a test configuration accessor
func setupTestConfig(t *testing.T) *goconfig.ConfigAccessor {
	t.Helper()

	// Create a temporary config file
	configContent := `
[chatbox]
jwt_secret = "test-secret-that-is-at-least-32-characters-long"
reconnect_timeout = "30s"
encryption_key = ""
max_message_size = "1048576"
llm_stream_timeout = "30s"
admin_rate_limit = 10
admin_rate_window = "1m"
allowed_origins = "http://localhost:3000"
cors_allowed_origins = "http://localhost:3000"
path_prefix = "/chatbox"
`

	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
	})

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	tmpFile.Close()

	// Set config file path
	os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())
	t.Cleanup(func() {
		os.Unsetenv("RMBASE_FILE_CFG")
	})

	// Load config
	if err := goconfig.LoadConfig(); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Fatalf("Failed to get default config: %v", err)
	}

	return config
}

// setupTestLogger creates a test logger
func setupTestLogger(t *testing.T) *golog.Logger {
	t.Helper()

	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	return logger
}

// setupTestMongo creates a test MongoDB connection
func setupTestMongo(t *testing.T) *gomongo.Mongo {
	t.Helper()

	// Load config first
	if err := goconfig.LoadConfig(); err != nil {
		t.Skipf("MongoDB not available: failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Skipf("MongoDB not available: failed to get config: %v", err)
	}

	logger := setupTestLogger(t)

	mongo, err := gomongo.InitMongoDB(logger, config)
	if err != nil {
		t.Skipf("MongoDB not available: %v", err)
	}

	// Clean up test data
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mongo.Coll("chat", "sessions").Drop(ctx)
		mongo.Coll("chat", "file_stats").Drop(ctx)
	})

	return mongo
}

// createTestJWT creates a test JWT token
func createTestJWT(t *testing.T, secret string, userID string, roles []string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"exp":     time.Now().Add(1 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
		"user_id": userID,
		"name":    "Test User",
		"roles":   roles,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to generate test JWT: %v", err)
	}

	return tokenString
}

// makeAuthRequest creates an authenticated HTTP request
func makeAuthRequest(method, path, token string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// TestRegister_RouteSetup tests that Register function sets up routes correctly
func TestRegister_RouteSetup(t *testing.T) {
	router := setupTestRouter()
	config := setupTestConfig(t)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set JWT secret in environment
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
	defer os.Unsetenv("JWT_SECRET")

	err := Register(router, config, logger, mongo)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify routes are registered by checking the routes list
	routes := router.Routes()

	// Check for key endpoints
	expectedRoutes := []string{
		"/chatbox/ws",
		"/chatbox/sessions",
		"/chatbox/admin/sessions",
		"/chatbox/admin/metrics",
		"/chatbox/admin/takeover/:sessionID",
		"/chatbox/healthz",
		"/chatbox/readyz",
		"/metrics",
	}

	routeMap := make(map[string]bool)
	for _, route := range routes {
		routeMap[route.Path] = true
	}

	for _, expected := range expectedRoutes {
		if !routeMap[expected] {
			t.Errorf("Expected route %s not found", expected)
		}
	}
}

// TestRegister_MissingJWTSecret tests that Register fails when JWT secret is missing
func TestRegister_MissingJWTSecret(t *testing.T) {
	router := setupTestRouter()
	config := setupTestConfig(t)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Clear JWT secret from environment and config
	os.Unsetenv("JWT_SECRET")

	// Create config without JWT secret
	configContent := `
[chatbox]
reconnect_timeout = "30s"
`
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	tmpFile.Close()

	// Set config file path
	os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())
	defer os.Unsetenv("RMBASE_FILE_CFG")

	// Load config
	if err := goconfig.LoadConfig(); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	config, err = goconfig.Default()
	if err != nil {
		t.Fatalf("Failed to get default config: %v", err)
	}

	err = Register(router, config, logger, mongo)
	if err == nil {
		t.Fatal("Expected error when JWT secret is missing, got nil")
	}

	if !strings.Contains(err.Error(), "JWT secret") {
		t.Errorf("Expected error about JWT secret, got: %v", err)
	}
}

// TestRegister_WeakJWTSecret tests that Register fails with weak JWT secret
func TestRegister_WeakJWTSecret(t *testing.T) {
	router := setupTestRouter()
	config := setupTestConfig(t)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set weak JWT secret
	os.Setenv("JWT_SECRET", "password123")
	defer os.Unsetenv("JWT_SECRET")

	err := Register(router, config, logger, mongo)
	if err == nil {
		t.Fatal("Expected error with weak JWT secret, got nil")
	}

	if !strings.Contains(err.Error(), "weak") || !strings.Contains(err.Error(), "password") {
		t.Errorf("Expected error about weak secret, got: %v", err)
	}
}

// TestRegister_ShortJWTSecret tests that Register fails with short JWT secret
func TestRegister_ShortJWTSecret(t *testing.T) {
	router := setupTestRouter()
	config := setupTestConfig(t)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set short JWT secret
	os.Setenv("JWT_SECRET", "short")
	defer os.Unsetenv("JWT_SECRET")

	err := Register(router, config, logger, mongo)
	if err == nil {
		t.Fatal("Expected error with short JWT secret, got nil")
	}

	if !strings.Contains(err.Error(), "at least") {
		t.Errorf("Expected error about minimum length, got: %v", err)
	}
}

// TestRegister_InvalidEncryptionKey tests that Register fails with invalid encryption key
func TestRegister_InvalidEncryptionKey(t *testing.T) {
	router := setupTestRouter()
	config := setupTestConfig(t)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set valid JWT secret
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
	defer os.Unsetenv("JWT_SECRET")

	// Set invalid encryption key (not 32 bytes)
	os.Setenv("ENCRYPTION_KEY", "short-key")
	defer os.Unsetenv("ENCRYPTION_KEY")

	err := Register(router, config, logger, mongo)
	if err == nil {
		t.Fatal("Expected error with invalid encryption key, got nil")
	}

	if !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("Expected error about 32 bytes, got: %v", err)
	}
}

// TestRegister_ValidEncryptionKey tests that Register succeeds with valid 32-byte encryption key
func TestRegister_ValidEncryptionKey(t *testing.T) {
	router := setupTestRouter()
	config := setupTestConfig(t)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set valid JWT secret
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
	defer os.Unsetenv("JWT_SECRET")

	// Set valid 32-byte encryption key
	os.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012") // Exactly 32 bytes
	defer os.Unsetenv("ENCRYPTION_KEY")

	err := Register(router, config, logger, mongo)
	if err != nil {
		t.Fatalf("Register failed with valid encryption key: %v", err)
	}
}

// TestRegister_InvalidPathPrefix tests that Register fails with invalid path prefix
func TestRegister_InvalidPathPrefix(t *testing.T) {
	router := setupTestRouter()
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set valid JWT secret
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
	defer os.Unsetenv("JWT_SECRET")

	// Set invalid path prefix (doesn't start with /)
	os.Setenv("CHATBOX_PATH_PREFIX", "chatbox")
	defer os.Unsetenv("CHATBOX_PATH_PREFIX")

	// Create config with invalid path prefix
	configContent := `
[chatbox]
jwt_secret = "test-secret-that-is-at-least-32-characters-long"
reconnect_timeout = "30s"
path_prefix = "chatbox"
`
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	tmpFile.Close()

	// Set config file path
	os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())
	defer os.Unsetenv("RMBASE_FILE_CFG")

	// Load config
	if err := goconfig.LoadConfig(); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Fatalf("Failed to get default config: %v", err)
	}

	err = Register(router, config, logger, mongo)
	if err == nil {
		t.Fatal("Expected error with invalid path prefix, got nil")
	}

	if !strings.Contains(err.Error(), "must start with") {
		t.Errorf("Expected error about path prefix format, got: %v", err)
	}
}

// TestAuthMiddleware_MissingToken tests authentication middleware with missing token
func TestAuthMiddleware_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := setupTestLogger(t)

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Add middleware to test endpoint
	router.GET("/test", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Make request without token
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

// TestAuthMiddleware_InvalidToken tests authentication middleware with invalid token
func TestAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := setupTestLogger(t)

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Add middleware to test endpoint
	router.GET("/test", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Make request with invalid token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

// TestAuthMiddleware_ExpiredToken_Coverage tests authentication middleware with expired token
func TestAuthMiddleware_ExpiredToken_Coverage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := setupTestLogger(t)

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Add middleware to test endpoint
	router.GET("/test", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Create expired token
	claims := jwt.MapClaims{
		"exp":     time.Now().Add(-1 * time.Hour).Unix(), // Expired 1 hour ago
		"iat":     time.Now().Add(-2 * time.Hour).Unix(),
		"user_id": "test-user",
		"name":    "Test User",
		"roles":   []string{constants.RoleAdmin},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to generate expired token: %v", err)
	}

	// Make request with expired token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

// TestAuthMiddleware_ValidTokenNoAdminRole tests authentication middleware with valid token but no admin role
func TestAuthMiddleware_ValidTokenNoAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := setupTestLogger(t)

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Add middleware to test endpoint
	router.GET("/test", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Create token without admin role
	token := createTestJWT(t, secret, "test-user", []string{"user"})

	// Make request with non-admin token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", w.Code)
	}
}

// TestAuthMiddleware_ValidTokenWithAdminRole tests authentication middleware with valid admin token
func TestAuthMiddleware_ValidTokenWithAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := setupTestLogger(t)

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Add middleware to test endpoint
	router.GET("/test", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Create token with admin role
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	// Make request with admin token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestAuthMiddleware_ValidTokenWithChatAdminRole tests authentication middleware with chat_admin role
func TestAuthMiddleware_ValidTokenWithChatAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := setupTestLogger(t)

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Add middleware to test endpoint
	router.GET("/test", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Create token with chat_admin role
	token := createTestJWT(t, secret, "chat-admin-user", []string{constants.RoleChatAdmin})

	// Make request with chat_admin token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestUserAuthMiddleware_ValidToken_Coverage tests user authentication middleware with valid token
func TestUserAuthMiddleware_ValidToken_Coverage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := setupTestLogger(t)

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Add middleware to test endpoint
	router.GET("/test", userAuthMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Create token with regular user role
	token := createTestJWT(t, secret, "regular-user", []string{"user"})

	// Make request with user token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestUserAuthMiddleware_MissingToken tests user authentication middleware with missing token
func TestUserAuthMiddleware_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := setupTestLogger(t)

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Add middleware to test endpoint
	router.GET("/test", userAuthMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Make request without token
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

// TestProperty_AuthenticationMiddlewareEnforcesAccessControl tests Property 9
// **Validates: Requirements 3.4, 5.3, 6.3**
// Property: For any HTTP request, the authentication middleware should allow access
// only when a valid JWT token is present, and should return 401 Unauthorized for
// missing or invalid tokens.
func TestProperty_AuthenticationMiddlewareEnforcesAccessControl(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	testCases := []struct {
		name           string
		token          string
		expectedStatus int
		description    string
	}{
		{
			name:           "NoToken",
			token:          "",
			expectedStatus: http.StatusUnauthorized,
			description:    "Request without token should be rejected",
		},
		{
			name:           "InvalidToken",
			token:          "invalid-token-format",
			expectedStatus: http.StatusUnauthorized,
			description:    "Request with invalid token should be rejected",
		},
		{
			name:           "MalformedToken",
			token:          "Bearer.malformed.token",
			expectedStatus: http.StatusUnauthorized,
			description:    "Request with malformed token should be rejected",
		},
		{
			name:           "ValidAdminToken",
			token:          createTestJWT(t, secret, "admin", []string{constants.RoleAdmin}),
			expectedStatus: http.StatusOK,
			description:    "Request with valid admin token should be allowed",
		},
		{
			name:           "ValidChatAdminToken",
			token:          createTestJWT(t, secret, "chatadmin", []string{constants.RoleChatAdmin}),
			expectedStatus: http.StatusOK,
			description:    "Request with valid chat_admin token should be allowed",
		},
		{
			name:           "ValidUserToken",
			token:          createTestJWT(t, secret, "user", []string{"user"}),
			expectedStatus: http.StatusForbidden,
			description:    "Request with valid user token (no admin role) should be forbidden",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/test", authMiddleware(validator, logger), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "success"})
			})

			req := httptest.NewRequest("GET", "/test", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("%s: Expected status %d, got %d", tc.description, tc.expectedStatus, w.Code)
			}
		})
	}
}

// TestRateLimitMiddleware_Enforcement tests rate limit enforcement
func TestRateLimitMiddleware_Enforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)

	// Create rate limiter with low limit for testing
	limiter := ratelimit.NewMessageLimiter(1*time.Minute, 3)
	limiter.StartCleanup()
	defer limiter.StopCleanup()

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	router := gin.New()
	router.GET("/test",
		authMiddleware(validator, logger),
		adminRateLimitMiddleware(limiter, logger),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "success"})
		})

	// Create admin token
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	// Make requests up to the limit
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: Expected status 200, got %d", i+1, w.Code)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != constants.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", w.Code)
	}

	// Check for Retry-After header
	retryAfter := w.Header().Get(constants.HeaderRetryAfter)
	if retryAfter == "" {
		t.Error("Expected Retry-After header, got none")
	}
}

// TestRateLimitMiddleware_ResetAfterWindow tests rate limit reset after window expires
func TestRateLimitMiddleware_ResetAfterWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)

	// Create rate limiter with short window for testing
	limiter := ratelimit.NewMessageLimiter(100*time.Millisecond, 2)
	limiter.StartCleanup()
	defer limiter.StopCleanup()

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	router := gin.New()
	router.GET("/test",
		authMiddleware(validator, logger),
		adminRateLimitMiddleware(limiter, logger),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "success"})
		})

	// Create admin token
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	// Make requests up to the limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: Expected status 200, got %d", i+1, w.Code)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != constants.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", w.Code)
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Request should now succeed
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("After window reset: Expected status 200, got %d", w.Code)
	}
}

// TestRateLimitMiddleware_NoClaims tests rate limit middleware when claims are missing
func TestRateLimitMiddleware_NoClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)

	limiter := ratelimit.NewMessageLimiter(1*time.Minute, 10)
	limiter.StartCleanup()
	defer limiter.StopCleanup()

	router := gin.New()
	// Apply rate limit middleware without auth middleware
	router.GET("/test",
		adminRateLimitMiddleware(limiter, logger),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "success"})
		})

	// Make request without claims in context
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should pass through when no claims (let auth middleware handle it)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestRateLimitMiddleware_DifferentUsers tests that rate limits are per-user
func TestRateLimitMiddleware_DifferentUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)

	limiter := ratelimit.NewMessageLimiter(1*time.Minute, 2)
	limiter.StartCleanup()
	defer limiter.StopCleanup()

	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	router := gin.New()
	router.GET("/test",
		authMiddleware(validator, logger),
		adminRateLimitMiddleware(limiter, logger),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "success"})
		})

	// Create tokens for two different users
	token1 := createTestJWT(t, secret, "admin-user-1", []string{constants.RoleAdmin})
	token2 := createTestJWT(t, secret, "admin-user-2", []string{constants.RoleAdmin})

	// User 1 makes requests up to limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("User 1 request %d: Expected status 200, got %d", i+1, w.Code)
		}
	}

	// User 1 should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token1)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != constants.StatusTooManyRequests {
		t.Errorf("User 1: Expected status 429, got %d", w.Code)
	}

	// User 2 should still be able to make requests
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token2)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("User 2: Expected status 200, got %d", w.Code)
	}
}

// TestProperty_RateLimitingMiddlewareEnforcesLimits tests Property 10
// **Validates: Requirements 3.4, 5.4**
// Property: For any sequence of HTTP requests from the same user, the rate limiting
// middleware should allow requests up to the configured limit and return 429 Too Many
// Requests for requests exceeding the limit.
func TestProperty_RateLimitingMiddlewareEnforcesLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	testCases := []struct {
		name        string
		limit       int
		window      time.Duration
		requests    int
		description string
	}{
		{
			name:        "LimitOf1",
			limit:       1,
			window:      1 * time.Minute,
			requests:    3,
			description: "With limit of 1, first request succeeds, rest are rate limited",
		},
		{
			name:        "LimitOf5",
			limit:       5,
			window:      1 * time.Minute,
			requests:    10,
			description: "With limit of 5, first 5 succeed, rest are rate limited",
		},
		{
			name:        "LimitOf10",
			limit:       10,
			window:      1 * time.Minute,
			requests:    15,
			description: "With limit of 10, first 10 succeed, rest are rate limited",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			limiter := ratelimit.NewMessageLimiter(tc.window, tc.limit)
			limiter.StartCleanup()
			defer limiter.StopCleanup()

			router := gin.New()
			router.GET("/test",
				authMiddleware(validator, logger),
				adminRateLimitMiddleware(limiter, logger),
				func(c *gin.Context) {
					c.JSON(http.StatusOK, gin.H{"message": "success"})
				})

			token := createTestJWT(t, secret, "test-user", []string{constants.RoleAdmin})

			successCount := 0
			rateLimitedCount := 0

			for i := 0; i < tc.requests; i++ {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				if w.Code == http.StatusOK {
					successCount++
				} else if w.Code == constants.StatusTooManyRequests {
					rateLimitedCount++

					// Verify Retry-After header is present
					retryAfter := w.Header().Get(constants.HeaderRetryAfter)
					if retryAfter == "" {
						t.Errorf("Request %d: Expected Retry-After header, got none", i+1)
					}
				} else {
					t.Errorf("Request %d: Unexpected status code %d", i+1, w.Code)
				}
			}

			// Verify exactly 'limit' requests succeeded
			if successCount != tc.limit {
				t.Errorf("%s: Expected %d successful requests, got %d", tc.description, tc.limit, successCount)
			}

			// Verify remaining requests were rate limited
			expectedRateLimited := tc.requests - tc.limit
			if rateLimitedCount != expectedRateLimited {
				t.Errorf("%s: Expected %d rate limited requests, got %d", tc.description, expectedRateLimited, rateLimitedCount)
			}
		})
	}
}

// TestAuthorizationMiddleware_AdminRole tests authorization with admin role
func TestAuthorizationMiddleware_AdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	router := gin.New()
	router.GET("/admin", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access granted"})
	})

	// Create token with admin role
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestAuthorizationMiddleware_ChatAdminRole tests authorization with chat_admin role
func TestAuthorizationMiddleware_ChatAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	router := gin.New()
	router.GET("/admin", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access granted"})
	})

	// Create token with chat_admin role
	token := createTestJWT(t, secret, "chat-admin-user", []string{constants.RoleChatAdmin})

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestAuthorizationMiddleware_NonAdminRole tests authorization rejection for non-admin
func TestAuthorizationMiddleware_NonAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	router := gin.New()
	router.GET("/admin", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access granted"})
	})

	// Create token with regular user role
	token := createTestJWT(t, secret, "regular-user", []string{"user", "viewer"})

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", w.Code)
	}
}

// TestAuthorizationMiddleware_EmptyRoles tests authorization with empty roles
func TestAuthorizationMiddleware_EmptyRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	router := gin.New()
	router.GET("/admin", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access granted"})
	})

	// Create token with no roles
	token := createTestJWT(t, secret, "no-role-user", []string{})

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", w.Code)
	}
}

// TestAuthorizationMiddleware_MultipleRolesWithAdmin tests authorization with multiple roles including admin
func TestAuthorizationMiddleware_MultipleRolesWithAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	router := gin.New()
	router.GET("/admin", authMiddleware(validator, logger), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access granted"})
	})

	// Create token with multiple roles including admin
	token := createTestJWT(t, secret, "multi-role-user", []string{"user", constants.RoleAdmin, "viewer"})

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestProperty_AuthorizationMiddlewareEnforcesRoles tests Property 11
// **Validates: Requirements 6.5**
// Property: For any HTTP request to an admin endpoint, the authorization middleware
// should allow access only when the JWT token contains the admin role, and should
// return 403 Forbidden otherwise.
func TestProperty_AuthorizationMiddlewareEnforcesRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	testCases := []struct {
		name           string
		roles          []string
		expectedStatus int
		description    string
	}{
		{
			name:           "AdminRole",
			roles:          []string{constants.RoleAdmin},
			expectedStatus: http.StatusOK,
			description:    "Token with admin role should be allowed",
		},
		{
			name:           "ChatAdminRole",
			roles:          []string{constants.RoleChatAdmin},
			expectedStatus: http.StatusOK,
			description:    "Token with chat_admin role should be allowed",
		},
		{
			name:           "UserRole",
			roles:          []string{"user"},
			expectedStatus: http.StatusForbidden,
			description:    "Token with only user role should be forbidden",
		},
		{
			name:           "ViewerRole",
			roles:          []string{"viewer"},
			expectedStatus: http.StatusForbidden,
			description:    "Token with only viewer role should be forbidden",
		},
		{
			name:           "EmptyRoles",
			roles:          []string{},
			expectedStatus: http.StatusForbidden,
			description:    "Token with no roles should be forbidden",
		},
		{
			name:           "MultipleRolesWithAdmin",
			roles:          []string{"user", constants.RoleAdmin, "viewer"},
			expectedStatus: http.StatusOK,
			description:    "Token with multiple roles including admin should be allowed",
		},
		{
			name:           "MultipleRolesWithChatAdmin",
			roles:          []string{"user", constants.RoleChatAdmin},
			expectedStatus: http.StatusOK,
			description:    "Token with multiple roles including chat_admin should be allowed",
		},
		{
			name:           "MultipleRolesWithoutAdmin",
			roles:          []string{"user", "viewer", "editor"},
			expectedStatus: http.StatusForbidden,
			description:    "Token with multiple roles but no admin should be forbidden",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/admin", authMiddleware(validator, logger), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "admin access granted"})
			})

			token := createTestJWT(t, secret, "test-user-"+tc.name, tc.roles)

			req := httptest.NewRequest("GET", "/admin", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("%s: Expected status %d, got %d", tc.description, tc.expectedStatus, w.Code)
			}
		})
	}
}

// TestHandleHealthCheck_Coverage tests the health check endpoint
func TestHandleHealthCheck_Coverage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/healthz", handleHealthCheck)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", response["status"])
	}

	if response["timestamp"] == nil {
		t.Error("Expected timestamp in response")
	}
}

// TestHandleReadyCheck_Success tests the readiness check endpoint with healthy MongoDB
func TestHandleReadyCheck_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	router := gin.New()
	router.GET("/readyz", handleReadyCheck(mongo, logger))

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "ready" {
		t.Errorf("Expected status 'ready', got %v", response["status"])
	}
}

// TestHandleReadyCheck_MongoNil tests readiness check with nil MongoDB
func TestHandleReadyCheck_MongoNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)

	router := gin.New()
	router.GET("/readyz", handleReadyCheck(nil, logger))

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != constants.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "not ready" {
		t.Errorf("Expected status 'not ready', got %v", response["status"])
	}
}

// TestHandleUserSessions_Success tests user sessions endpoint with valid user
func TestHandleUserSessions_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)

	router := gin.New()
	router.GET("/sessions", userAuthMiddleware(validator, logger), handleUserSessions(storageService, logger))

	// Create test session in storage
	testSession := &session.Session{
		ID:        "test-session-1",
		UserID:    "test-user",
		StartTime: time.Now(),
		IsActive:  true,
		Messages:  []*session.Message{},
	}
	if err := storageService.CreateSession(testSession); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Create token for user
	token := createTestJWT(t, secret, "test-user", []string{"user"})

	req := httptest.NewRequest("GET", "/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["user_id"] != "test-user" {
		t.Errorf("Expected user_id 'test-user', got %v", response["user_id"])
	}
}

// TestHandleUserSessions_NoClaims tests user sessions endpoint without claims
func TestHandleUserSessions_NoClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)

	router := gin.New()
	// Skip auth middleware to test missing claims
	router.GET("/sessions", handleUserSessions(storageService, logger))

	req := httptest.NewRequest("GET", "/sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

// TestHandleListSessions_WithFilters tests session listing with query parameters
func TestHandleListSessions_WithFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)
	sessionManager := session.NewSessionManager(30*time.Second, logger)

	router := gin.New()
	router.GET("/admin/sessions", authMiddleware(validator, logger), handleListSessions(storageService, sessionManager, logger))

	// Create test sessions in storage
	testSession := &session.Session{
		ID:        "test-session-2",
		UserID:    "user-1",
		StartTime: time.Now(),
		IsActive:  true,
		Messages:  []*session.Message{},
	}
	if err := storageService.CreateSession(testSession); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Create admin token
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	// Test with user_id filter
	req := httptest.NewRequest("GET", "/admin/sessions?user_id=user-1&limit=10", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["limit"] != float64(10) {
		t.Errorf("Expected limit 10, got %v", response["limit"])
	}
}

// TestHandleListSessions_InvalidTimeFormat tests session listing with invalid time format
func TestHandleListSessions_InvalidTimeFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)
	sessionManager := session.NewSessionManager(30*time.Second, logger)

	router := gin.New()
	router.GET("/admin/sessions", authMiddleware(validator, logger), handleListSessions(storageService, sessionManager, logger))

	// Create admin token
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	// Test with invalid time format
	req := httptest.NewRequest("GET", "/admin/sessions?start_time_from=invalid-time", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleGetMetrics_Success tests metrics endpoint
func TestHandleGetMetrics_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)

	router := gin.New()
	router.GET("/admin/metrics", authMiddleware(validator, logger), handleGetMetrics(storageService, logger))

	// Create admin token
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	req := httptest.NewRequest("GET", "/admin/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["metrics"] == nil {
		t.Error("Expected metrics in response")
	}

	if response["time_range"] == nil {
		t.Error("Expected time_range in response")
	}
}

// TestHandleGetMetrics_InvalidTimeFormat tests metrics endpoint with invalid time
func TestHandleGetMetrics_InvalidTimeFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)

	router := gin.New()
	router.GET("/admin/metrics", authMiddleware(validator, logger), handleGetMetrics(storageService, logger))

	// Create admin token
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	// Test with invalid start_time
	req := httptest.NewRequest("GET", "/admin/metrics?start_time=invalid", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleAdminTakeover_Success tests admin takeover endpoint
func TestHandleAdminTakeover_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Create dependencies
	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)
	sessionManager := session.NewSessionManager(30*time.Second, logger)
	messageRouter := router.NewMessageRouter(sessionManager, nil, nil, nil, storageService, 30*time.Second, logger)

	router := gin.New()
	router.POST("/admin/takeover/:sessionID", authMiddleware(validator, logger), handleAdminTakeover(messageRouter, logger))

	// Create test session in storage and session manager
	testSession := &session.Session{
		ID:        "takeover-session",
		UserID:    "user-1",
		StartTime: time.Now(),
		IsActive:  true,
		Messages:  []*session.Message{},
	}
	if err := storageService.CreateSession(testSession); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Create session in session manager using CreateSession
	sess, err := sessionManager.CreateSession("user-1")
	if err != nil {
		t.Fatalf("Failed to create session in manager: %v", err)
	}
	// Update the session ID to match our test session
	testSession.ID = sess.ID

	// Create admin token
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	req := httptest.NewRequest("POST", "/admin/takeover/"+sess.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["session_id"] != sess.ID {
		t.Errorf("Expected session_id '%s', got %v", sess.ID, response["session_id"])
	}
}

// TestHandleAdminTakeover_MissingSessionID tests admin takeover without session ID
func TestHandleAdminTakeover_MissingSessionID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)
	sessionManager := session.NewSessionManager(30*time.Second, logger)
	messageRouter := router.NewMessageRouter(sessionManager, nil, nil, nil, storageService, 30*time.Second, logger)

	router := gin.New()
	router.POST("/admin/takeover/:sessionID", authMiddleware(validator, logger), handleAdminTakeover(messageRouter, logger))

	// Create admin token
	token := createTestJWT(t, secret, "admin-user", []string{constants.RoleAdmin})

	// Test with empty session ID
	req := httptest.NewRequest("POST", "/admin/takeover/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should get 404 because route doesn't match
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestProperty_HTTPHandlersProcessValidRequests tests Property 14
// **Validates: Requirements 3.3, 9.3, 9.4, 9.5**
// Property: For any valid HTTP request to a registered endpoint, the handler should
// process the request and return an appropriate HTTP response (2xx for success, 4xx
// for client errors, 5xx for server errors).
func TestProperty_HTTPHandlersProcessValidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)
	sessionManager := session.NewSessionManager(30*time.Second, logger)
	messageRouter := router.NewMessageRouter(sessionManager, nil, nil, nil, storageService, 30*time.Second, logger)

	testCases := []struct {
		name           string
		method         string
		path           string
		token          string
		expectedStatus int
		description    string
	}{
		{
			name:           "HealthCheck",
			method:         "GET",
			path:           "/healthz",
			token:          "",
			expectedStatus: http.StatusOK,
			description:    "Health check should return 200",
		},
		{
			name:           "ReadyCheck",
			method:         "GET",
			path:           "/readyz",
			token:          "",
			expectedStatus: http.StatusOK,
			description:    "Ready check should return 200 when MongoDB is available",
		},
		{
			name:           "UserSessionsWithAuth",
			method:         "GET",
			path:           "/sessions",
			token:          createTestJWT(t, secret, "user", []string{"user"}),
			expectedStatus: http.StatusOK,
			description:    "User sessions with valid token should return 200",
		},
		{
			name:           "UserSessionsWithoutAuth",
			method:         "GET",
			path:           "/sessions",
			token:          "",
			expectedStatus: http.StatusUnauthorized,
			description:    "User sessions without token should return 401",
		},
		{
			name:           "AdminSessionsWithAuth",
			method:         "GET",
			path:           "/admin/sessions",
			token:          createTestJWT(t, secret, "admin", []string{constants.RoleAdmin}),
			expectedStatus: http.StatusOK,
			description:    "Admin sessions with admin token should return 200",
		},
		{
			name:           "AdminSessionsWithoutAuth",
			method:         "GET",
			path:           "/admin/sessions",
			token:          "",
			expectedStatus: http.StatusUnauthorized,
			description:    "Admin sessions without token should return 401",
		},
		{
			name:           "AdminSessionsWithUserToken",
			method:         "GET",
			path:           "/admin/sessions",
			token:          createTestJWT(t, secret, "user", []string{"user"}),
			expectedStatus: http.StatusForbidden,
			description:    "Admin sessions with user token should return 403",
		},
		{
			name:           "MetricsWithAuth",
			method:         "GET",
			path:           "/admin/metrics",
			token:          createTestJWT(t, secret, "admin", []string{constants.RoleAdmin}),
			expectedStatus: http.StatusOK,
			description:    "Metrics with admin token should return 200",
		},
		{
			name:           "MetricsWithInvalidTime",
			method:         "GET",
			path:           "/admin/metrics?start_time=invalid",
			token:          createTestJWT(t, secret, "admin", []string{constants.RoleAdmin}),
			expectedStatus: http.StatusBadRequest,
			description:    "Metrics with invalid time should return 400",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()

			// Register endpoints
			router.GET("/healthz", handleHealthCheck)
			router.GET("/readyz", handleReadyCheck(mongo, logger))
			router.GET("/sessions", userAuthMiddleware(validator, logger), handleUserSessions(storageService, logger))

			adminGroup := router.Group("/admin")
			adminGroup.Use(authMiddleware(validator, logger))
			{
				adminGroup.GET("/sessions", handleListSessions(storageService, sessionManager, logger))
				adminGroup.GET("/metrics", handleGetMetrics(storageService, logger))
				adminGroup.POST("/takeover/:sessionID", handleAdminTakeover(messageRouter, logger))
			}

			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("%s: Expected status %d, got %d", tc.description, tc.expectedStatus, w.Code)
			}

			// Verify response is valid JSON for non-404 responses
			if w.Code != http.StatusNotFound {
				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("%s: Expected valid JSON response, got error: %v", tc.description, err)
				}
			}
		})
	}
}

// TestValidateJWTSecret_EmptySecret tests JWT secret validation with empty secret
func TestValidateJWTSecret_EmptySecret(t *testing.T) {
	err := validateJWTSecret("")
	if err == nil {
		t.Fatal("Expected error with empty secret, got nil")
	}

	if !strings.Contains(err.Error(), "required") {
		t.Errorf("Expected error about required secret, got: %v", err)
	}
}

// TestValidateJWTSecret_ShortSecret tests JWT secret validation with short secret
func TestValidateJWTSecret_ShortSecret(t *testing.T) {
	err := validateJWTSecret("short")
	if err == nil {
		t.Fatal("Expected error with short secret, got nil")
	}

	if !strings.Contains(err.Error(), "at least") {
		t.Errorf("Expected error about minimum length, got: %v", err)
	}
}

// TestValidateJWTSecret_WeakSecrets tests JWT secret validation with weak patterns
func TestValidateJWTSecret_WeakSecrets(t *testing.T) {
	weakSecrets := []string{
		"password123456789012345678901234567890",
		"secret123456789012345678901234567890",
		"admin123456789012345678901234567890",
		"test123456789012345678901234567890",
		"default123456789012345678901234567890",
	}

	for _, weak := range weakSecrets {
		t.Run(weak, func(t *testing.T) {
			err := validateJWTSecret(weak)
			if err == nil {
				t.Fatalf("Expected error with weak secret '%s', got nil", weak)
			}

			if !strings.Contains(err.Error(), "weak") {
				t.Errorf("Expected error about weak secret, got: %v", err)
			}
		})
	}
}

// TestValidateJWTSecret_ValidSecret tests JWT secret validation with valid secret
func TestValidateJWTSecret_ValidSecret(t *testing.T) {
	validSecret := "xK9mP2nQ7wR4vL8zT6yU3bN5cM1aS0dF"
	err := validateJWTSecret(validSecret)
	if err != nil {
		t.Errorf("Expected no error with valid secret, got: %v", err)
	}
}

// TestValidateEncryptionKey_EmptyKey tests encryption key validation with empty key
func TestValidateEncryptionKey_EmptyKey(t *testing.T) {
	err := validateEncryptionKey([]byte{})
	if err != nil {
		t.Errorf("Expected no error with empty key (encryption disabled), got: %v", err)
	}
}

// TestValidateEncryptionKey_ValidKey tests encryption key validation with 32-byte key
func TestValidateEncryptionKey_ValidKey(t *testing.T) {
	validKey := []byte("12345678901234567890123456789012") // Exactly 32 bytes
	err := validateEncryptionKey(validKey)
	if err != nil {
		t.Errorf("Expected no error with 32-byte key, got: %v", err)
	}
}

// TestValidateEncryptionKey_InvalidLength tests encryption key validation with wrong length
func TestValidateEncryptionKey_InvalidLength(t *testing.T) {
	testCases := []struct {
		name   string
		keyLen int
	}{
		{"16bytes", 16},
		{"24bytes", 24},
		{"31bytes", 31},
		{"33bytes", 33},
		{"64bytes", 64},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := make([]byte, tc.keyLen)
			err := validateEncryptionKey(key)
			if err == nil {
				t.Fatalf("Expected error with %d-byte key, got nil", tc.keyLen)
			}

			if !strings.Contains(err.Error(), "32 bytes") {
				t.Errorf("Expected error about 32 bytes, got: %v", err)
			}
		})
	}
}

// TestValidateEncryptionKey_NilKey tests encryption key validation with nil key
func TestValidateEncryptionKey_NilKey(t *testing.T) {
	err := validateEncryptionKey(nil)
	if err != nil {
		t.Errorf("Expected no error with nil key (encryption disabled), got: %v", err)
	}
}

// TestProperty_WeakJWTSecretsAreRejected tests Property 13
// **Validates: Requirements 6.4**
// Property: For any JWT secret that is shorter than 32 characters or matches a known
// weak secret, the validation function should return an error indicating the secret
// is insecure.
func TestProperty_WeakJWTSecretsAreRejected(t *testing.T) {
	testCases := []struct {
		name        string
		secret      string
		shouldFail  bool
		description string
	}{
		{
			name:        "EmptySecret",
			secret:      "",
			shouldFail:  true,
			description: "Empty secret should be rejected",
		},
		{
			name:        "ShortSecret5Chars",
			secret:      "short",
			shouldFail:  true,
			description: "5-character secret should be rejected",
		},
		{
			name:        "ShortSecret15Chars",
			secret:      "short-secret-15",
			shouldFail:  true,
			description: "15-character secret should be rejected",
		},
		{
			name:        "ShortSecret31Chars",
			secret:      "1234567890123456789012345678901",
			shouldFail:  true,
			description: "31-character secret should be rejected",
		},
		{
			name:        "WeakPassword",
			secret:      "password123456789012345678901234567890",
			shouldFail:  true,
			description: "Secret containing 'password' should be rejected",
		},
		{
			name:        "WeakSecret",
			secret:      "secret123456789012345678901234567890",
			shouldFail:  true,
			description: "Secret containing 'secret' should be rejected",
		},
		{
			name:        "WeakAdmin",
			secret:      "admin123456789012345678901234567890",
			shouldFail:  true,
			description: "Secret containing 'admin' should be rejected",
		},
		{
			name:        "WeakTest",
			secret:      "test123456789012345678901234567890",
			shouldFail:  true,
			description: "Secret containing 'test' should be rejected",
		},
		{
			name:        "WeakDefault",
			secret:      "default123456789012345678901234567890",
			shouldFail:  true,
			description: "Secret containing 'default' should be rejected",
		},
		{
			name:        "ValidSecret32Chars",
			secret:      "xK9mP2nQ7wR4vL8zT6yU3bN5cM1aS0dF",
			shouldFail:  false,
			description: "32-character strong secret should be accepted",
		},
		{
			name:        "ValidSecretLong",
			secret:      "xK9mP2nQ7wR4vL8zT6yU3bN5cM1aS0dF-extra-chars",
			shouldFail:  false,
			description: "Long strong secret should be accepted",
		},
		{
			name:        "ValidSecretBase64",
			secret:      "dGhpcyBpcyBhIHZlcnkgc3Ryb25nIGFuZCByYW5kb20gc2VjcmV0IGtleQ==",
			shouldFail:  false,
			description: "Base64-encoded secret should be accepted",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateJWTSecret(tc.secret)

			if tc.shouldFail {
				if err == nil {
					t.Errorf("%s: Expected error, got nil", tc.description)
				}
			} else {
				if err != nil {
					t.Errorf("%s: Expected no error, got: %v", tc.description, err)
				}
			}
		})
	}
}

// TestProperty_ValidationFunctionsRejectInvalidInputs tests Property 16
// **Validates: Requirements 3.5, 4.2**
// Property: For any validation function and any invalid input, the validation function
// should return an error describing why the input is invalid.
func TestProperty_ValidationFunctionsRejectInvalidInputs(t *testing.T) {
	testCases := []struct {
		name        string
		validator   func() error
		shouldFail  bool
		errorCheck  func(error) bool
		description string
	}{
		{
			name:        "EmptyJWTSecret",
			validator:   func() error { return validateJWTSecret("") },
			shouldFail:  true,
			errorCheck:  func(err error) bool { return strings.Contains(err.Error(), "required") },
			description: "Empty JWT secret should return error with 'required'",
		},
		{
			name:        "ShortJWTSecret",
			validator:   func() error { return validateJWTSecret("short") },
			shouldFail:  true,
			errorCheck:  func(err error) bool { return strings.Contains(err.Error(), "at least") },
			description: "Short JWT secret should return error with 'at least'",
		},
		{
			name:        "WeakJWTSecret",
			validator:   func() error { return validateJWTSecret("password123456789012345678901234567890") },
			shouldFail:  true,
			errorCheck:  func(err error) bool { return strings.Contains(err.Error(), "weak") },
			description: "Weak JWT secret should return error with 'weak'",
		},
		{
			name:        "ValidJWTSecret",
			validator:   func() error { return validateJWTSecret("xK9mP2nQ7wR4vL8zT6yU3bN5cM1aS0dF") },
			shouldFail:  false,
			errorCheck:  nil,
			description: "Valid JWT secret should not return error",
		},
		{
			name:        "InvalidEncryptionKey16Bytes",
			validator:   func() error { return validateEncryptionKey(make([]byte, 16)) },
			shouldFail:  true,
			errorCheck:  func(err error) bool { return strings.Contains(err.Error(), "32 bytes") },
			description: "16-byte encryption key should return error with '32 bytes'",
		},
		{
			name:        "InvalidEncryptionKey24Bytes",
			validator:   func() error { return validateEncryptionKey(make([]byte, 24)) },
			shouldFail:  true,
			errorCheck:  func(err error) bool { return strings.Contains(err.Error(), "32 bytes") },
			description: "24-byte encryption key should return error with '32 bytes'",
		},
		{
			name:        "InvalidEncryptionKey31Bytes",
			validator:   func() error { return validateEncryptionKey(make([]byte, 31)) },
			shouldFail:  true,
			errorCheck:  func(err error) bool { return strings.Contains(err.Error(), "32 bytes") },
			description: "31-byte encryption key should return error with '32 bytes'",
		},
		{
			name:        "InvalidEncryptionKey33Bytes",
			validator:   func() error { return validateEncryptionKey(make([]byte, 33)) },
			shouldFail:  true,
			errorCheck:  func(err error) bool { return strings.Contains(err.Error(), "32 bytes") },
			description: "33-byte encryption key should return error with '32 bytes'",
		},
		{
			name:        "ValidEncryptionKey32Bytes",
			validator:   func() error { return validateEncryptionKey(make([]byte, 32)) },
			shouldFail:  false,
			errorCheck:  nil,
			description: "32-byte encryption key should not return error",
		},
		{
			name:        "EmptyEncryptionKey",
			validator:   func() error { return validateEncryptionKey([]byte{}) },
			shouldFail:  false,
			errorCheck:  nil,
			description: "Empty encryption key (disabled) should not return error",
		},
		{
			name:        "NilEncryptionKey",
			validator:   func() error { return validateEncryptionKey(nil) },
			shouldFail:  false,
			errorCheck:  nil,
			description: "Nil encryption key (disabled) should not return error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.validator()

			if tc.shouldFail {
				if err == nil {
					t.Errorf("%s: Expected error, got nil", tc.description)
				} else if tc.errorCheck != nil && !tc.errorCheck(err) {
					t.Errorf("%s: Error message doesn't match expected pattern: %v", tc.description, err)
				}
			} else {
				if err != nil {
					t.Errorf("%s: Expected no error, got: %v", tc.description, err)
				}
			}
		})
	}
}

// TestConcurrentHTTPRequests_SameEndpoint tests concurrent requests to the same endpoint
func TestConcurrentHTTPRequests_SameEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)

	router := gin.New()
	router.GET("/sessions", userAuthMiddleware(validator, logger), handleUserSessions(storageService, logger))

	// Create test token
	token := createTestJWT(t, secret, "concurrent-user", []string{"user"})

	// Run concurrent requests
	concurrency := 10
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			req := httptest.NewRequest("GET", "/sessions", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Concurrent request %d: Expected status 200, got %d", id, w.Code)
			}

			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < concurrency; i++ {
		<-done
	}
}

// TestConcurrentHTTPRequests_DifferentEndpoints tests concurrent requests to different endpoints
func TestConcurrentHTTPRequests_DifferentEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)
	sessionManager := session.NewSessionManager(30*time.Second, logger)

	router := gin.New()
	router.GET("/healthz", handleHealthCheck)
	router.GET("/readyz", handleReadyCheck(mongo, logger))
	router.GET("/sessions", userAuthMiddleware(validator, logger), handleUserSessions(storageService, logger))

	adminGroup := router.Group("/admin")
	adminGroup.Use(authMiddleware(validator, logger))
	{
		adminGroup.GET("/sessions", handleListSessions(storageService, sessionManager, logger))
		adminGroup.GET("/metrics", handleGetMetrics(storageService, logger))
	}

	// Create tokens
	userToken := createTestJWT(t, secret, "user", []string{"user"})
	adminToken := createTestJWT(t, secret, "admin", []string{constants.RoleAdmin})

	endpoints := []struct {
		path  string
		token string
	}{
		{"/healthz", ""},
		{"/readyz", ""},
		{"/sessions", userToken},
		{"/admin/sessions", adminToken},
		{"/admin/metrics", adminToken},
	}

	// Run concurrent requests to different endpoints
	concurrency := len(endpoints) * 3
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		endpoint := endpoints[i%len(endpoints)]
		go func(id int, ep struct {
			path  string
			token string
		}) {
			req := httptest.NewRequest("GET", ep.path, nil)
			if ep.token != "" {
				req.Header.Set("Authorization", "Bearer "+ep.token)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Concurrent request %d to %s: Expected status 200, got %d", id, ep.path, w.Code)
			}

			done <- true
		}(i, endpoint)
	}

	// Wait for all requests to complete
	for i := 0; i < concurrency; i++ {
		<-done
	}
}

// TestConcurrentHTTPRequests_WithRateLimiting tests concurrent requests with rate limiting
func TestConcurrentHTTPRequests_WithRateLimiting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	// Create rate limiter with low limit
	limiter := ratelimit.NewMessageLimiter(1*time.Minute, 5)
	limiter.StartCleanup()
	defer limiter.StopCleanup()

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)
	sessionManager := session.NewSessionManager(30*time.Second, logger)

	router := gin.New()
	adminGroup := router.Group("/admin")
	adminGroup.Use(authMiddleware(validator, logger))
	adminGroup.Use(adminRateLimitMiddleware(limiter, logger))
	{
		adminGroup.GET("/sessions", handleListSessions(storageService, sessionManager, logger))
	}

	// Create admin token
	token := createTestJWT(t, secret, "admin", []string{constants.RoleAdmin})

	// Run concurrent requests
	concurrency := 10
	done := make(chan int, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			req := httptest.NewRequest("GET", "/admin/sessions", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			done <- w.Code
		}(i)
	}

	// Collect results
	successCount := 0
	rateLimitedCount := 0

	for i := 0; i < concurrency; i++ {
		code := <-done
		if code == http.StatusOK {
			successCount++
		} else if code == constants.StatusTooManyRequests {
			rateLimitedCount++
		}
	}

	// Verify rate limiting worked (should have exactly 5 successes)
	if successCount != 5 {
		t.Errorf("Expected 5 successful requests, got %d", successCount)
	}

	if rateLimitedCount != 5 {
		t.Errorf("Expected 5 rate limited requests, got %d", rateLimitedCount)
	}
}

// TestConcurrentHTTPRequests_MultipleUsers tests concurrent requests from multiple users
func TestConcurrentHTTPRequests_MultipleUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)

	router := gin.New()
	router.GET("/sessions", userAuthMiddleware(validator, logger), handleUserSessions(storageService, logger))

	// Create tokens for multiple users
	numUsers := 5
	tokens := make([]string, numUsers)
	for i := 0; i < numUsers; i++ {
		tokens[i] = createTestJWT(t, secret, fmt.Sprintf("user-%d", i), []string{"user"})
	}

	// Run concurrent requests from different users
	concurrency := numUsers * 3
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		userIdx := i % numUsers
		go func(id int, token string) {
			req := httptest.NewRequest("GET", "/sessions", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Concurrent request %d: Expected status 200, got %d", id, w.Code)
			}

			done <- true
		}(i, tokens[userIdx])
	}

	// Wait for all requests to complete
	for i := 0; i < concurrency; i++ {
		<-done
	}
}

// TestProperty_ConcurrentHTTPRequestsAreThreadSafe tests Property 18
// **Validates: Requirements 7.4**
// Property: For any set of concurrent HTTP requests, the middleware and handlers
// should process all requests without data races or shared state corruption.
func TestProperty_ConcurrentHTTPRequestsAreThreadSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)
	secret := "test-secret-that-is-at-least-32-characters-long"
	validator := auth.NewJWTValidator(secret)

	storageService := storage.NewStorageService(mongo, "chat", "sessions", logger, nil)
	sessionManager := session.NewSessionManager(30*time.Second, logger)
	messageRouter := router.NewMessageRouter(sessionManager, nil, nil, nil, storageService, 30*time.Second, logger)

	// Create rate limiter
	limiter := ratelimit.NewMessageLimiter(1*time.Minute, 100)
	limiter.StartCleanup()
	defer limiter.StopCleanup()

	// Set up router with all endpoints
	r := gin.New()
	r.GET("/healthz", handleHealthCheck)
	r.GET("/readyz", handleReadyCheck(mongo, logger))
	r.GET("/sessions", userAuthMiddleware(validator, logger), handleUserSessions(storageService, logger))

	adminGroup := r.Group("/admin")
	adminGroup.Use(authMiddleware(validator, logger))
	adminGroup.Use(adminRateLimitMiddleware(limiter, logger))
	{
		adminGroup.GET("/sessions", handleListSessions(storageService, sessionManager, logger))
		adminGroup.GET("/metrics", handleGetMetrics(storageService, logger))
		adminGroup.POST("/takeover/:sessionID", handleAdminTakeover(messageRouter, logger))
	}

	// Create test sessions in storage
	for i := 0; i < 5; i++ {
		testSession := &session.Session{
			ID:        fmt.Sprintf("concurrent-session-%d", i),
			UserID:    fmt.Sprintf("user-%d", i),
			StartTime: time.Now(),
			IsActive:  true,
			Messages:  []*session.Message{},
		}
		if err := storageService.CreateSession(testSession); err != nil {
			t.Fatalf("Failed to create test session: %v", err)
		}
	}

	// Create tokens
	userToken := createTestJWT(t, secret, "user", []string{"user"})
	adminToken := createTestJWT(t, secret, "admin", []string{constants.RoleAdmin})

	// Define test scenarios
	scenarios := []struct {
		method string
		path   string
		token  string
	}{
		{"GET", "/healthz", ""},
		{"GET", "/readyz", ""},
		{"GET", "/sessions", userToken},
		{"GET", "/admin/sessions", adminToken},
		{"GET", "/admin/sessions?user_id=user-1", adminToken},
		{"GET", "/admin/sessions?status=active", adminToken},
		{"GET", "/admin/metrics", adminToken},
	}

	// Run concurrent requests with different scenarios
	concurrency := 50
	done := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		scenario := scenarios[i%len(scenarios)]
		go func(id int, s struct {
			method string
			path   string
			token  string
		}) {
			req := httptest.NewRequest(s.method, s.path, nil)
			if s.token != "" {
				req.Header.Set("Authorization", "Bearer "+s.token)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Verify response is valid
			if w.Code < 200 || w.Code >= 600 {
				done <- fmt.Errorf("request %d to %s: invalid status code %d", id, s.path, w.Code)
				return
			}

			// Verify response body is valid JSON (for non-404 responses)
			if w.Code != http.StatusNotFound {
				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					done <- fmt.Errorf("request %d to %s: invalid JSON response: %v", id, s.path, err)
					return
				}
			}

			done <- nil
		}(i, scenario)
	}

	// Collect results and check for errors
	errorCount := 0
	for i := 0; i < concurrency; i++ {
		if err := <-done; err != nil {
			t.Error(err)
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Property violation: %d out of %d concurrent requests failed", errorCount, concurrency)
	}
}

// TestRegister_InvalidReconnectTimeout tests that Register fails with invalid reconnect timeout format
// Subtask 19.6
func TestRegister_InvalidReconnectTimeout(t *testing.T) {
	router := setupTestRouter()
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set valid JWT secret
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
	defer os.Unsetenv("JWT_SECRET")

	// Create config with invalid reconnect timeout
	configContent := `
[chatbox]
jwt_secret = "test-secret-that-is-at-least-32-characters-long"
reconnect_timeout = "invalid-duration"
`
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	tmpFile.Close()

	// Set config file path
	os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())
	defer os.Unsetenv("RMBASE_FILE_CFG")

	// Load config
	if err := goconfig.LoadConfig(); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Fatalf("Failed to get default config: %v", err)
	}

	err = Register(router, config, logger, mongo)
	if err == nil {
		t.Fatal("Expected error with invalid reconnect timeout, got nil")
	}

	if !strings.Contains(err.Error(), "reconnect timeout") {
		t.Errorf("Expected error about reconnect timeout, got: %v", err)
	}
}

// TestRegister_InvalidMaxMessageSize tests that Register handles invalid max message size gracefully
// Subtask 19.7
func TestRegister_InvalidMaxMessageSize(t *testing.T) {
	router := setupTestRouter()
	config := setupTestConfig(t)
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set valid JWT secret
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
	defer os.Unsetenv("JWT_SECRET")

	// Set invalid max message size (should be handled gracefully with default)
	os.Setenv("MAX_MESSAGE_SIZE", "invalid-number")
	defer os.Unsetenv("MAX_MESSAGE_SIZE")

	// Register should succeed and use default value
	err := Register(router, config, logger, mongo)
	if err != nil {
		t.Fatalf("Register should succeed with invalid max message size (using default): %v", err)
	}
}

// TestRegister_CORSConfiguration tests CORS configuration
// Subtask 19.9
func TestRegister_CORSConfiguration(t *testing.T) {
	t.Run("with CORS origins configured", func(t *testing.T) {
		router := setupTestRouter()
		logger := setupTestLogger(t)
		mongo := setupTestMongo(t)

		// Set valid JWT secret
		os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
		defer os.Unsetenv("JWT_SECRET")

		// Create config with CORS configuration
		configContent := `
[chatbox]
jwt_secret = "test-secret-that-is-at-least-32-characters-long"
reconnect_timeout = "30s"
cors_allowed_origins = "http://localhost:3000,https://example.com"
path_prefix = "/chatbox"
`
		tmpFile, err := os.CreateTemp("", "config-*.toml")
		if err != nil {
			t.Fatalf("Failed to create temp config file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(configContent); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}
		tmpFile.Close()

		// Set config file path
		os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())
		defer os.Unsetenv("RMBASE_FILE_CFG")

		// Load config
		if err := goconfig.LoadConfig(); err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		config, err := goconfig.Default()
		if err != nil {
			t.Fatalf("Failed to get default config: %v", err)
		}

		err = Register(router, config, logger, mongo)
		if err != nil {
			t.Fatalf("Register should succeed with CORS configuration: %v", err)
		}

		// CORS middleware is applied to the router
		// We can't easily verify it without making actual HTTP requests,
		// but we verify that Register doesn't fail
	})

	t.Run("without CORS origins configured", func(t *testing.T) {
		router := setupTestRouter()
		config := setupTestConfig(t)
		logger := setupTestLogger(t)
		mongo := setupTestMongo(t)

		// Set valid JWT secret
		os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
		defer os.Unsetenv("JWT_SECRET")

		err := Register(router, config, logger, mongo)
		if err != nil {
			t.Fatalf("Register should succeed without CORS configuration: %v", err)
		}
	})
}

// TestRegister_CustomPathPrefix tests custom path prefix configuration
// Subtask 19.10 (additional test)
func TestRegister_CustomPathPrefix(t *testing.T) {
	router := setupTestRouter()
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set valid JWT secret
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
	defer os.Unsetenv("JWT_SECRET")

	// Set custom path prefix
	os.Setenv("CHATBOX_PATH_PREFIX", "/api/v1/chat")
	defer os.Unsetenv("CHATBOX_PATH_PREFIX")

	// Create config with custom path prefix
	configContent := `
[chatbox]
jwt_secret = "test-secret-that-is-at-least-32-characters-long"
reconnect_timeout = "30s"
path_prefix = "/api/v1/chat"
`
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	tmpFile.Close()

	// Set config file path
	os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())
	defer os.Unsetenv("RMBASE_FILE_CFG")

	// Load config
	if err := goconfig.LoadConfig(); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Fatalf("Failed to get default config: %v", err)
	}

	err = Register(router, config, logger, mongo)
	if err != nil {
		t.Fatalf("Register should succeed with custom path prefix: %v", err)
	}

	// Verify routes use custom prefix
	routes := router.Routes()
	routeMap := make(map[string]bool)
	for _, route := range routes {
		routeMap[route.Path] = true
	}

	// Check that routes use custom prefix
	if !routeMap["/api/v1/chat/ws"] {
		t.Error("WebSocket route should use custom prefix /api/v1/chat")
	}
	if !routeMap["/api/v1/chat/healthz"] {
		t.Error("Health route should use custom prefix /api/v1/chat")
	}
	if !routeMap["/api/v1/chat/readyz"] {
		t.Error("Ready route should use custom prefix /api/v1/chat")
	}
}

// TestRegister_EmptyPathPrefix tests that Register fails with empty path prefix
// Subtask 19.10 (additional test)
func TestRegister_EmptyPathPrefix(t *testing.T) {
	router := setupTestRouter()
	logger := setupTestLogger(t)
	mongo := setupTestMongo(t)

	// Set valid JWT secret
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-characters-long")
	defer os.Unsetenv("JWT_SECRET")

	// Set empty path prefix
	os.Setenv("CHATBOX_PATH_PREFIX", "")
	defer os.Unsetenv("CHATBOX_PATH_PREFIX")

	// Create config with empty path prefix
	configContent := `
[chatbox]
jwt_secret = "test-secret-that-is-at-least-32-characters-long"
reconnect_timeout = "30s"
path_prefix = ""
`
	tmpFile, err := os.CreateTemp("", "config-*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	tmpFile.Close()

	// Set config file path
	os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())
	defer os.Unsetenv("RMBASE_FILE_CFG")

	// Load config
	if err := goconfig.LoadConfig(); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Fatalf("Failed to get default config: %v", err)
	}

	err = Register(router, config, logger, mongo)
	if err == nil {
		t.Fatal("Expected error with empty path prefix, got nil")
	}

	if !strings.Contains(err.Error(), "path prefix") {
		t.Errorf("Expected error about path prefix, got: %v", err)
	}
}

// TestHandleReadyCheck_MongoConnectionFailure tests readiness check when MongoDB ping fails
// This test attempts to create a scenario where MongoDB initialization succeeds but ping fails
func TestHandleReadyCheck_MongoPingFailure(t *testing.T) {
	if os.Getenv("SKIP_MONGO_TESTS") != "" {
		t.Skip("Skipping MongoDB test")
	}

	gin.SetMode(gin.TestMode)
	logger := setupTestLogger(t)

	// Strategy: Connect to a non-existent port to simulate MongoDB being down
	// Use a very short timeout so the test doesn't hang
	configContent := `
[dbs]
verbose = 1
slowThreshold = 2

[dbs.chat]
uri = "mongodb://localhost:27099/test?connectTimeoutMS=100&serverSelectionTimeoutMS=100&socketTimeoutMS=100"
database = "test"
collection = "test"
connectTimeout = "100ms"
`
	tmpFile, err := os.CreateTemp("", "config-mongo-fail-*.toml")
	if err != nil {
		t.Skipf("Failed to create temp config file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Skipf("Failed to write config file: %v", err)
	}
	tmpFile.Close()

	// Set config file path
	os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())
	defer os.Unsetenv("RMBASE_FILE_CFG")

	// Load config
	if err := goconfig.LoadConfig(); err != nil {
		t.Skipf("Failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Skipf("Failed to get config accessor: %v", err)
	}

	// Try to initialize MongoDB
	mongo, err := gomongo.InitMongoDB(logger, config)
	if err != nil {
		// If initialization fails, we still test the nil path
		t.Logf("MongoDB initialization failed: %v", err)
		router := gin.New()
		router.GET("/readyz", handleReadyCheck(nil, logger))

		req := httptest.NewRequest("GET", "/readyz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != constants.StatusServiceUnavailable {
			t.Errorf("Expected status 503, got %d", w.Code)
		}
		return
	}

	// If initialization succeeded (lazy connection), test the ping failure
	t.Log("MongoDB initialized, testing ping failure")
	router := gin.New()
	router.GET("/readyz", handleReadyCheck(mongo, logger))

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 503 because ping will fail
	if w.Code != constants.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "not ready" {
		t.Errorf("Expected status 'not ready', got %v", response["status"])
	}

	// Verify MongoDB check shows failure
	checks, ok := response["checks"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected checks to be a map")
	}

	mongoCheck, ok := checks["mongodb"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected mongodb check to be a map")
	}

	if mongoCheck["status"] != "not ready" {
		t.Errorf("Expected mongodb status 'not ready', got %v", mongoCheck["status"])
	}

	t.Log("Successfully tested MongoDB ping failure path")
}
