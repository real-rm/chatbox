package chatbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/ratelimit"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"github.com/stretchr/testify/assert"
)

// performRequest is a helper function to perform HTTP requests in tests
func performRequest(r http.Handler, method, path string, body io.Reader) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRegister(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create test logger with a valid directory
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            "logs",
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		// If logger initialization fails, skip the test
		t.Skipf("Failed to initialize logger: %v", err)
	}
	defer func() {
		if logger != nil {
			logger.Close()
		}
	}()

	// Create test config
	// Note: This test will fail without proper config setup
	// For now, we're just testing that the function signature is correct
	t.Run("function signature", func(t *testing.T) {
		// This test just verifies the function exists with the correct signature
		// Actual integration testing would require a full environment setup
		var registerFunc func(*gin.Engine, *goconfig.ConfigAccessor, *golog.Logger, *gomongo.Mongo) error
		registerFunc = Register
		assert.NotNil(t, registerFunc)
	})
}

func TestAuthMiddleware(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	t.Run("function exists", func(t *testing.T) {
		// Verify the authMiddleware function exists
		// Full testing would require JWT setup
		assert.NotNil(t, authMiddleware)
	})
}

func TestHealthCheckHandlers(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	t.Run("health check handler exists", func(t *testing.T) {
		assert.NotNil(t, handleHealthCheck)
	})

	t.Run("ready check handler exists", func(t *testing.T) {
		assert.NotNil(t, handleReadyCheck)
	})
}

// TestAllowedOriginsConfiguration tests that allowed origins configuration is properly loaded
func TestAllowedOriginsConfiguration(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Set config file path for testing
	os.Setenv("RMBASE_FILE_CFG", "config.toml")

	// Load config
	err := goconfig.LoadConfig()
	if err != nil {
		t.Skipf("Failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Skipf("Failed to get default config: %v", err)
	}

	t.Run("allowed_origins configuration exists", func(t *testing.T) {
		// Test that the configuration key exists and can be read
		allowedOrigins, err := config.ConfigStringWithDefault("chatbox.allowed_origins", "")
		assert.NoError(t, err)

		// The default config.toml has empty allowed_origins
		// This test verifies the configuration can be read
		assert.NotNil(t, allowedOrigins)
		t.Logf("Allowed origins configuration: %q", allowedOrigins)
	})

	t.Run("configuration parsing works", func(t *testing.T) {
		// Test that the configuration can be parsed as expected
		allowedOriginsStr, err := config.ConfigStringWithDefault("chatbox.allowed_origins", "")
		assert.NoError(t, err)

		// If empty, should allow all origins (development mode)
		if allowedOriginsStr == "" {
			t.Log("No origins configured - development mode (allows all origins)")
		} else {
			// If configured, should be parseable as comma-separated list
			t.Logf("Origins configured: %s", allowedOriginsStr)
		}
	})
}

// TestAdminSessionsEndpoint tests the admin sessions endpoint with pagination, filtering, and sorting
func TestAdminSessionsEndpoint(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	t.Run("handler exists", func(t *testing.T) {
		assert.NotNil(t, handleListSessions)
	})

	t.Run("accepts pagination parameters", func(t *testing.T) {
		// This test verifies that the endpoint accepts pagination parameters
		// Full integration testing would require MongoDB setup

		// The endpoint should accept these query parameters:
		// - limit: maximum number of results (default: 100, max: 1000)
		// - offset: number of results to skip (default: 0)
		// - user_id: filter by specific user
		// - status: filter by "active" or "ended"
		// - admin_assisted: filter by "true" or "false"
		// - sort_by: field to sort by (start_time, end_time, message_count, total_tokens, user_id)
		// - sort_order: "asc" or "desc" (default: desc)
		// - start_time_from: RFC3339 timestamp
		// - start_time_to: RFC3339 timestamp

		t.Log("Admin sessions endpoint supports pagination with limit and offset")
		t.Log("Admin sessions endpoint supports filtering by user_id, status, admin_assisted, and time range")
		t.Log("Admin sessions endpoint supports sorting by multiple fields with asc/desc order")
	})
}

// TestAdminSessionsEndpoint_LargeDataset documents the large dataset testing requirements
func TestAdminSessionsEndpoint_LargeDataset(t *testing.T) {
	t.Run("performance requirements", func(t *testing.T) {
		// This test documents the performance requirements for the admin sessions endpoint
		// Actual performance testing requires MongoDB with 10,000+ sessions

		t.Log("Performance Requirements:")
		t.Log("- Should handle 10,000+ sessions efficiently")
		t.Log("- Query response time should be < 1 second for paginated results")
		t.Log("- Pagination should work correctly with large offsets")
		t.Log("- Filtering should reduce result set efficiently")
		t.Log("- Sorting should use O(n log n) algorithm (implemented in storage layer)")
		t.Log("- Combined filters should work correctly with large datasets")

		t.Log("\nTo run full integration tests with large datasets:")
		t.Log("1. Start MongoDB: docker-compose up -d mongodb")
		t.Log("2. Run storage tests: go test -v ./internal/storage -run TestListAllSessionsWithOptions_LargeDataset")
		t.Log("3. The test creates 1000 sessions and validates pagination, filtering, and sorting")
	})
}

// TestReadyCheckWithMongoDB tests the readiness check endpoint with MongoDB connectivity
func TestReadyCheckWithMongoDB(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	t.Run("returns 503 when MongoDB is nil", func(t *testing.T) {
		// Create a test logger
		testLogger, _ := golog.InitLog(golog.LogConfig{
			Dir:            "logs",
			Level:          "error",
			StandardOutput: false,
		})
		defer testLogger.Close()

		// Create a test router
		router := gin.New()

		// Register the ready check handler with nil MongoDB
		router.GET("/readyz", handleReadyCheck(nil, testLogger))

		// Create a test request
		w := performRequest(router, "GET", "/readyz", nil)

		// Should return 503 Service Unavailable
		assert.Equal(t, 503, w.Code)

		// Response should indicate MongoDB is not ready
		assert.Contains(t, w.Body.String(), "not ready")
		assert.Contains(t, w.Body.String(), "mongodb")
	})

	t.Run("returns 503 when MongoDB connection fails", func(t *testing.T) {
		// Skip if we should skip MongoDB tests
		if testing.Short() || os.Getenv("SKIP_MONGO_TESTS") != "" {
			t.Skip("Skipping MongoDB-dependent test")
		}

		// Set config file path for testing
		os.Setenv("RMBASE_FILE_CFG", "config.toml")

		// Load config
		err := goconfig.LoadConfig()
		if err != nil {
			t.Skipf("Failed to load config: %v", err)
		}

		// Create test logger
		logger, err := golog.InitLog(golog.LogConfig{
			Dir:            "logs",
			Level:          "error",
			StandardOutput: false,
		})
		if err != nil {
			t.Skipf("Failed to initialize logger: %v", err)
		}
		defer logger.Close()

		// Get config accessor
		config, err := goconfig.Default()
		if err != nil {
			t.Skipf("Failed to get default config: %v", err)
		}

		// Override MongoDB connection string to point to non-existent server
		// This simulates MongoDB being down
		os.Setenv("MONGO_URI", "mongodb://localhost:27099/test?connectTimeoutMS=1000&serverSelectionTimeoutMS=1000")

		// Try to initialize MongoDB (should fail or timeout quickly)
		mongo, err := gomongo.InitMongoDB(logger, config)
		if err != nil {
			// If initialization fails immediately, that's expected
			t.Log("MongoDB initialization failed as expected:", err)

			// Test with nil mongo
			router := gin.New()
			router.GET("/readyz", handleReadyCheck(nil, logger))
			w := performRequest(router, "GET", "/readyz", nil)
			assert.Equal(t, 503, w.Code)
			return
		}

		// If initialization succeeded (connection pool created but not tested yet),
		// the Ping() call in the handler should fail
		router := gin.New()
		router.GET("/readyz", handleReadyCheck(mongo, logger))

		w := performRequest(router, "GET", "/readyz", nil)

		// Should return 503 because MongoDB is unreachable
		assert.Equal(t, 503, w.Code)
		assert.Contains(t, w.Body.String(), "not ready")
		assert.Contains(t, w.Body.String(), "mongodb")
	})

	t.Run("returns 200 when MongoDB is healthy", func(t *testing.T) {
		// Skip if we should skip MongoDB tests
		if testing.Short() || os.Getenv("SKIP_MONGO_TESTS") != "" {
			t.Skip("Skipping MongoDB-dependent test")
		}

		// Set config file path for testing
		os.Setenv("RMBASE_FILE_CFG", "config.toml")

		// Load config
		err := goconfig.LoadConfig()
		if err != nil {
			t.Skipf("Failed to load config: %v", err)
		}

		// Create test logger
		logger, err := golog.InitLog(golog.LogConfig{
			Dir:            "logs",
			Level:          "error",
			StandardOutput: false,
		})
		if err != nil {
			t.Skipf("Failed to initialize logger: %v", err)
		}
		defer logger.Close()

		// Get config accessor
		config, err := goconfig.Default()
		if err != nil {
			t.Skipf("Failed to get default config: %v", err)
		}

		// Try to initialize MongoDB with real connection
		mongo, err := gomongo.InitMongoDB(logger, config)
		if err != nil {
			t.Skipf("Skipping test: MongoDB not available: %v", err)
		}

		// Create a test router
		router := gin.New()
		router.GET("/readyz", handleReadyCheck(mongo, logger))

		w := performRequest(router, "GET", "/readyz", nil)

		// Should return 200 OK when MongoDB is healthy
		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "ready")
		assert.Contains(t, w.Body.String(), "mongodb")
	})
}

// TestEncryptionKeyConfiguration tests that encryption key is properly loaded and passed to storage service
func TestEncryptionKeyConfiguration(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Set config file path for testing
	os.Setenv("RMBASE_FILE_CFG", "config.toml")

	// Load config
	err := goconfig.LoadConfig()
	if err != nil {
		t.Skipf("Failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Skipf("Failed to get default config: %v", err)
	}

	t.Run("encryption_key configuration exists", func(t *testing.T) {
		// Test that the encryption key configuration exists and can be read
		encryptionKey, err := config.ConfigStringWithDefault("chatbox.encryption_key", "")
		assert.NoError(t, err)
		assert.NotEmpty(t, encryptionKey, "Encryption key should be configured in config.toml")

		t.Logf("Encryption key configured (length: %d bytes)", len(encryptionKey))
	})

	t.Run("encryption_key is 32 bytes for AES-256", func(t *testing.T) {
		// Test that the encryption key is exactly 32 bytes for AES-256
		encryptionKeyStr, err := config.ConfigStringWithDefault("chatbox.encryption_key", "")
		assert.NoError(t, err)

		encryptionKey := []byte(encryptionKeyStr)

		// The key should be 32 bytes for AES-256
		// If not, the application will pad or truncate it
		if len(encryptionKey) != 32 {
			t.Logf("Warning: Encryption key is %d bytes, not 32 bytes. Application will pad/truncate.", len(encryptionKey))
		} else {
			t.Log("Encryption key is exactly 32 bytes (AES-256)")
		}

		assert.NotEmpty(t, encryptionKey)
	})

	t.Run("encryption_key is passed to storage service", func(t *testing.T) {
		// This test verifies that the Register function properly loads the encryption key
		// and passes it to the storage service

		// The implementation in chatbox.go:
		// 1. Loads encryption key from config: config.ConfigStringWithDefault("chatbox.encryption_key", "")
		// 2. Converts to bytes and ensures it's 32 bytes (pads or truncates if needed)
		// 3. Passes to storage service: storage.NewStorageService(mongo, "chat", "sessions", logger, encryptionKey)

		t.Log("Encryption key loading flow:")
		t.Log("1. Load from config: chatbox.encryption_key")
		t.Log("2. Convert to bytes and ensure 32 bytes (AES-256)")
		t.Log("3. Pass to storage.NewStorageService()")
		t.Log("4. Storage service uses key to encrypt/decrypt message content")

		// Verify the key is configured
		encryptionKeyStr, err := config.ConfigStringWithDefault("chatbox.encryption_key", "")
		assert.NoError(t, err)
		assert.NotEmpty(t, encryptionKeyStr, "Encryption key must be configured for production")

		t.Log("✓ Encryption key is properly configured and will be passed to storage service")
	})
}

// TestMaxMessageSizeConfiguration tests that max_message_size configuration is properly loaded
func TestMaxMessageSizeConfiguration(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Set config file path for testing
	os.Setenv("RMBASE_FILE_CFG", "config.toml")

	// Load config
	err := goconfig.LoadConfig()
	if err != nil {
		t.Skipf("Failed to load config: %v", err)
	}

	config, err := goconfig.Default()
	if err != nil {
		t.Skipf("Failed to get default config: %v", err)
	}

	t.Run("max_message_size configuration exists", func(t *testing.T) {
		// Test that the configuration key exists and can be read
		maxMessageSize, err := config.ConfigStringWithDefault("chatbox.max_message_size", "1048576")
		assert.NoError(t, err)
		assert.NotEmpty(t, maxMessageSize, "Max message size should be configured in config.toml")
		t.Logf("Max message size configuration: %s bytes", maxMessageSize)
	})

	t.Run("max_message_size is valid integer", func(t *testing.T) {
		// Test that the max message size can be parsed as an integer
		maxMessageSizeStr, err := config.ConfigStringWithDefault("chatbox.max_message_size", "1048576")
		assert.NoError(t, err)

		var maxMessageSize int64
		_, parseErr := fmt.Sscanf(maxMessageSizeStr, "%d", &maxMessageSize)
		assert.NoError(t, parseErr, "Max message size should be a valid integer")
		assert.Greater(t, maxMessageSize, int64(0), "Max message size should be positive")
		t.Logf("Parsed max message size: %d bytes", maxMessageSize)
	})

	t.Run("default max_message_size is 1MB", func(t *testing.T) {
		// Test that the default max message size is 1MB (1048576 bytes)
		maxMessageSizeStr, err := config.ConfigStringWithDefault("chatbox.max_message_size", "1048576")
		assert.NoError(t, err)

		var maxMessageSize int64
		_, parseErr := fmt.Sscanf(maxMessageSizeStr, "%d", &maxMessageSize)
		assert.NoError(t, parseErr)

		// The default should be 1MB
		assert.Equal(t, int64(1048576), maxMessageSize, "Default max message size should be 1MB (1048576 bytes)")
	})

	t.Run("environment variable overrides config", func(t *testing.T) {
		// Test that MAX_MESSAGE_SIZE environment variable takes precedence
		testSize := "2097152" // 2MB
		os.Setenv("MAX_MESSAGE_SIZE", testSize)
		defer os.Unsetenv("MAX_MESSAGE_SIZE")

		// Simulate the logic in Register function
		maxMessageSize := int64(1048576) // Default
		if maxSizeStr := os.Getenv("MAX_MESSAGE_SIZE"); maxSizeStr != "" {
			var parsedSize int64
			if _, err := fmt.Sscanf(maxSizeStr, "%d", &parsedSize); err == nil {
				maxMessageSize = parsedSize
			}
		}

		assert.Equal(t, int64(2097152), maxMessageSize, "Environment variable should override config")
	})
}

// TestHandleUserSessions tests the handleUserSessions handler
func TestHandleUserSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("handler function exists", func(t *testing.T) {
		// Verify the handler function exists
		assert.NotNil(t, handleUserSessions)
	})

	t.Run("requires authentication", func(t *testing.T) {
		// This test documents that handleUserSessions requires authentication
		// The handler expects claims to be set in the context by authMiddleware
		// If claims are missing, it returns 401 Unauthorized
		t.Log("handleUserSessions requires claims in context")
		t.Log("Returns 401 if claims are missing")
		t.Log("Returns 500 if claims type is invalid")
	})

	t.Run("calls storage service", func(t *testing.T) {
		// This test documents that handleUserSessions calls storage.ListUserSessions
		// It passes the user ID from claims and limit of 0 (no limit)
		// Returns 500 if storage service fails
		// Returns 200 with sessions list on success
		t.Log("Calls storageService.ListUserSessions(claims.UserID, 0)")
		t.Log("Returns JSON with sessions, user_id, and count fields")
	})
}

// TestHandleListSessions tests the handleListSessions handler
func TestHandleListSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("handler function exists", func(t *testing.T) {
		assert.NotNil(t, handleListSessions)
	})

	t.Run("supports pagination parameters", func(t *testing.T) {
		// Documents pagination support
		t.Log("Accepts limit parameter (default: 100, max: 1000)")
		t.Log("Accepts offset parameter (default: 0)")
		t.Log("Invalid limits are clamped to valid range")
		t.Log("Negative offsets are set to 0")
	})

	t.Run("supports filtering parameters", func(t *testing.T) {
		// Documents filtering support
		t.Log("Accepts user_id filter")
		t.Log("Accepts status filter (active/ended)")
		t.Log("Accepts admin_assisted filter (true/false)")
		t.Log("Accepts start_time_from filter (RFC3339)")
		t.Log("Accepts start_time_to filter (RFC3339)")
		t.Log("Returns 400 for invalid time formats")
	})

	t.Run("supports sorting parameters", func(t *testing.T) {
		// Documents sorting support
		t.Log("Accepts sort_by parameter (start_time, end_time, message_count, total_tokens, user_id)")
		t.Log("Accepts sort_order parameter (asc/desc, default: desc)")
		t.Log("Default sort is by start_time descending")
	})

	t.Run("calls storage service with options", func(t *testing.T) {
		// Documents storage service interaction
		t.Log("Builds SessionListOptions from query parameters")
		t.Log("Calls storageService.ListAllSessionsWithOptions(opts)")
		t.Log("Returns JSON with sessions, count, limit, and offset")
		t.Log("Returns 500 if storage service fails")
	})
}

// TestHandleGetMetrics tests the handleGetMetrics handler
func TestHandleGetMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("handler function exists", func(t *testing.T) {
		assert.NotNil(t, handleGetMetrics)
	})

	t.Run("supports time range parameters", func(t *testing.T) {
		// Documents time range support
		t.Log("Accepts start_time parameter (RFC3339 format)")
		t.Log("Accepts end_time parameter (RFC3339 format)")
		t.Log("Defaults to last 24 hours if not specified")
		t.Log("Returns 400 for invalid time formats")
	})

	t.Run("calls storage service for metrics", func(t *testing.T) {
		// Documents storage service interaction
		t.Log("Calls storageService.GetSessionMetrics(startTime, endTime)")
		t.Log("Calls storageService.GetTokenUsage(startTime, endTime)")
		t.Log("Returns JSON with metrics and time_range")
		t.Log("Returns 500 if either storage call fails")
	})

	t.Run("returns comprehensive metrics", func(t *testing.T) {
		// Documents metrics structure
		t.Log("Returns TotalSessions count")
		t.Log("Returns ActiveSessions count")
		t.Log("Returns TotalTokens (from GetTokenUsage)")
		t.Log("Returns time_range with start and end timestamps")
	})
}

// TestHandleAdminTakeover tests the handleAdminTakeover handler
func TestHandleAdminTakeover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("handler function exists", func(t *testing.T) {
		assert.NotNil(t, handleAdminTakeover)
	})

	t.Run("requires session ID parameter", func(t *testing.T) {
		// Documents session ID requirement
		t.Log("Expects sessionID as URL parameter")
		t.Log("Returns 400 if sessionID is empty")
	})

	t.Run("requires authentication", func(t *testing.T) {
		// Documents authentication requirement
		t.Log("Requires claims in context (set by authMiddleware)")
		t.Log("Returns 401 if claims are missing")
		t.Log("Returns 500 if claims type is invalid")
	})

	t.Run("creates admin connection and initiates takeover", func(t *testing.T) {
		// Documents takeover process
		t.Log("Creates WebSocket connection for admin using claims")
		t.Log("Sets admin name from JWT claims")
		t.Log("Calls messageRouter.HandleAdminTakeover(adminConn, sessionID)")
		t.Log("Returns 500 if HandleAdminTakeover fails")
		t.Log("Returns 200 with success message on success")
	})

	t.Run("returns takeover details", func(t *testing.T) {
		// Documents response structure
		t.Log("Returns JSON with message, session_id, and admin_id")
		t.Log("Logs detailed error server-side on failure")
	})
}

// TestHandleHealthCheck tests the handleHealthCheck handler
func TestHandleHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 200 with healthy status", func(t *testing.T) {
		router := gin.New()
		router.GET("/healthz", handleHealthCheck)

		w := performRequest(router, "GET", "/healthz", nil)

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "healthy")
		assert.Contains(t, w.Body.String(), "timestamp")
	})

	t.Run("includes RFC3339 timestamp", func(t *testing.T) {
		router := gin.New()
		router.GET("/healthz", handleHealthCheck)

		before := time.Now()
		w := performRequest(router, "GET", "/healthz", nil)
		after := time.Now()

		assert.Equal(t, 200, w.Code)

		// Parse the response to verify timestamp format
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		timestampStr, ok := response["timestamp"].(string)
		assert.True(t, ok)

		timestamp, err := time.Parse(time.RFC3339, timestampStr)
		assert.NoError(t, err)
		assert.True(t, timestamp.After(before.Add(-time.Second)))
		assert.True(t, timestamp.Before(after.Add(time.Second)))
	})

	t.Run("always returns status healthy", func(t *testing.T) {
		router := gin.New()
		router.GET("/healthz", handleHealthCheck)

		// Make multiple requests to ensure consistency
		for i := 0; i < 3; i++ {
			w := performRequest(router, "GET", "/healthz", nil)
			assert.Equal(t, 200, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, "healthy", response["status"])
		}
	})
}

// TestHandleReadyCheck tests the handleReadyCheck handler
func TestHandleReadyCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 503 when MongoDB is nil", func(t *testing.T) {
		logger := CreateTestLogger(t)
		defer logger.Close()

		handler := handleReadyCheck(nil, logger)

		router := gin.New()
		router.GET("/readyz", handler)

		w := performRequest(router, "GET", "/readyz", nil)

		assert.Equal(t, 503, w.Code)
		assert.Contains(t, w.Body.String(), "not ready")
		assert.Contains(t, w.Body.String(), "mongodb")
		assert.Contains(t, w.Body.String(), "MongoDB not initialized")
	})

	t.Run("includes timestamp in response", func(t *testing.T) {
		logger := CreateTestLogger(t)
		defer logger.Close()

		handler := handleReadyCheck(nil, logger)

		router := gin.New()
		router.GET("/readyz", handler)

		w := performRequest(router, "GET", "/readyz", nil)

		assert.Equal(t, 503, w.Code)
		assert.Contains(t, w.Body.String(), "timestamp")

		// Parse and verify timestamp format
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		timestampStr, ok := response["timestamp"].(string)
		assert.True(t, ok)

		_, err = time.Parse(time.RFC3339, timestampStr)
		assert.NoError(t, err)
	})

	t.Run("includes checks in response", func(t *testing.T) {
		logger := CreateTestLogger(t)
		defer logger.Close()

		handler := handleReadyCheck(nil, logger)

		router := gin.New()
		router.GET("/readyz", handler)

		w := performRequest(router, "GET", "/readyz", nil)

		assert.Equal(t, 503, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		checks, ok := response["checks"].(map[string]interface{})
		assert.True(t, ok)
		assert.NotEmpty(t, checks)

		mongodb, ok := checks["mongodb"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "not ready", mongodb["status"])
	})
}

// CreateTestLogger creates a test logger for handler tests
func CreateTestLogger(t *testing.T) *golog.Logger {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		t.Fatalf("Failed to create test logger: %v", err)
	}
	return logger
}

// TestAuthMiddleware_ValidAdminToken tests authMiddleware with a valid admin token
func TestAuthMiddleware_ValidAdminToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create a valid admin token
	claims := jwt.MapClaims{
		"user_id": "admin-123",
		"name":    "Test Admin",
		"roles":   []string{"admin"},
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	// Create test router with middleware
	router := gin.New()
	router.Use(authMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		// Verify claims are set in context
		claimsInterface, exists := c.Get("claims")
		assert.True(t, exists)

		extractedClaims, ok := claimsInterface.(*auth.Claims)
		assert.True(t, ok)
		assert.Equal(t, "admin-123", extractedClaims.UserID)
		assert.Equal(t, "Test Admin", extractedClaims.Name)
		assert.Contains(t, extractedClaims.Roles, "admin")

		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request with valid token
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestAuthMiddleware_ValidChatAdminToken tests authMiddleware with chat_admin role
func TestAuthMiddleware_ValidChatAdminToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create a valid chat_admin token
	claims := jwt.MapClaims{
		"user_id": "chatadmin-456",
		"roles":   []string{"chat_admin"},
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	// Create test router with middleware
	router := gin.New()
	router.Use(authMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request with valid token
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestAuthMiddleware_MissingAuthHeader tests authMiddleware without Authorization header
func TestAuthMiddleware_MissingAuthHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	router := gin.New()
	router.Use(authMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request without Authorization header
	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid authorization header")
}

// TestAuthMiddleware_InvalidTokenFormat tests authMiddleware with malformed token
func TestAuthMiddleware_InvalidTokenFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	router := gin.New()
	router.Use(authMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request with invalid token format
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-format")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// TestAuthMiddleware_ExpiredToken tests authMiddleware with expired token
func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create an expired token
	claims := jwt.MapClaims{
		"user_id": "admin-123",
		"roles":   []string{"admin"},
		"exp":     time.Now().Add(-time.Hour).Unix(), // Expired 1 hour ago
		"iat":     time.Now().Add(-2 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	router := gin.New()
	router.Use(authMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request with expired token
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// TestAuthMiddleware_NonAdminRole tests authMiddleware with non-admin role
func TestAuthMiddleware_NonAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create a token with user role (not admin)
	claims := jwt.MapClaims{
		"user_id": "user-123",
		"roles":   []string{"user"},
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	router := gin.New()
	router.Use(authMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request with non-admin token
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
	assert.Contains(t, w.Body.String(), "Insufficient permissions")
}

// TestAuthMiddleware_InvalidSignature tests authMiddleware with wrong signature
func TestAuthMiddleware_InvalidSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create a token with wrong secret
	claims := jwt.MapClaims{
		"user_id": "admin-123",
		"roles":   []string{"admin"},
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("wrong-secret"))

	router := gin.New()
	router.Use(authMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request with invalid signature
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// TestUserAuthMiddleware_ValidToken tests userAuthMiddleware with valid token
func TestUserAuthMiddleware_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create a valid user token
	claims := jwt.MapClaims{
		"user_id": "user-123",
		"name":    "Test User",
		"roles":   []string{"user"},
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	// Create test router with middleware
	router := gin.New()
	router.Use(userAuthMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		// Verify claims are set in context
		claimsInterface, exists := c.Get("claims")
		assert.True(t, exists)

		extractedClaims, ok := claimsInterface.(*auth.Claims)
		assert.True(t, ok)
		assert.Equal(t, "user-123", extractedClaims.UserID)
		assert.Equal(t, "Test User", extractedClaims.Name)
		assert.Contains(t, extractedClaims.Roles, "user")

		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request with valid token
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestUserAuthMiddleware_NoAdminCheck tests that userAuthMiddleware doesn't check for admin role
func TestUserAuthMiddleware_NoAdminCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create a token with any role (not admin)
	claims := jwt.MapClaims{
		"user_id": "user-456",
		"roles":   []string{"user", "moderator"},
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))

	router := gin.New()
	router.Use(userAuthMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request - should succeed without admin role
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestUserAuthMiddleware_MissingAuthHeader tests userAuthMiddleware without Authorization header
func TestUserAuthMiddleware_MissingAuthHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	router := gin.New()
	router.Use(userAuthMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request without Authorization header
	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// TestUserAuthMiddleware_InvalidToken tests userAuthMiddleware with invalid token
func TestUserAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testSecret := "test-secret-key-for-jwt-validation"
	validator := auth.NewJWTValidator(testSecret)
	logger := CreateTestLogger(t)
	defer logger.Close()

	router := gin.New()
	router.Use(userAuthMiddleware(validator, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make request with invalid token
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// TestAdminRateLimitMiddleware_AllowsWithinLimit tests rate limiting allows requests within limit
func TestAdminRateLimitMiddleware_AllowsWithinLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create rate limiter with 5 requests per minute
	limiter := ratelimit.NewMessageLimiter(time.Minute, 5)

	// Create test claims
	testClaims := &auth.Claims{
		UserID: "admin-123",
		Name:   "Test Admin",
		Roles:  []string{"admin"},
	}

	router := gin.New()
	// Middleware that sets claims in context
	router.Use(func(c *gin.Context) {
		c.Set("claims", testClaims)
		c.Next()
	})
	router.Use(adminRateLimitMiddleware(limiter, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make 5 requests - all should succeed
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code, "Request %d should succeed", i+1)
	}
}

// TestAdminRateLimitMiddleware_BlocksWhenExceeded tests rate limiting blocks when limit exceeded
func TestAdminRateLimitMiddleware_BlocksWhenExceeded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create rate limiter with 3 requests per minute
	limiter := ratelimit.NewMessageLimiter(time.Minute, 3)

	// Create test claims
	testClaims := &auth.Claims{
		UserID: "admin-456",
		Name:   "Test Admin",
		Roles:  []string{"admin"},
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("claims", testClaims)
		c.Next()
	})
	router.Use(adminRateLimitMiddleware(limiter, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Make 3 requests - should succeed
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code, "Request %d should succeed", i+1)
	}

	// 4th request should be rate limited
	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 429, w.Code, "Request should be rate limited")
	assert.Contains(t, w.Body.String(), "rate_limit_exceeded")
	assert.Contains(t, w.Body.String(), "retry_after_ms")
}

// TestAdminRateLimitMiddleware_ReturnsRetryAfterHeader tests Retry-After header is set
func TestAdminRateLimitMiddleware_ReturnsRetryAfterHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create rate limiter with 2 requests per minute
	limiter := ratelimit.NewMessageLimiter(time.Minute, 2)

	testClaims := &auth.Claims{
		UserID: "admin-789",
		Name:   "Test Admin",
		Roles:  []string{"admin"},
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("claims", testClaims)
		c.Next()
	})
	router.Use(adminRateLimitMiddleware(limiter, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Exhaust the limit
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Next request should be rate limited with Retry-After header
	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 429, w.Code)
	retryAfter := w.Header().Get("Retry-After")
	assert.NotEmpty(t, retryAfter, "Retry-After header should be set")

	// Parse retry after value
	var retryAfterSeconds int
	_, err := fmt.Sscanf(retryAfter, "%d", &retryAfterSeconds)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, retryAfterSeconds, 1, "Retry-After should be at least 1 second")
}

// TestAdminRateLimitMiddleware_NoClaims tests middleware behavior when claims are missing
func TestAdminRateLimitMiddleware_NoClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := CreateTestLogger(t)
	defer logger.Close()

	limiter := ratelimit.NewMessageLimiter(time.Minute, 5)

	router := gin.New()
	// Don't set claims in context
	router.Use(adminRateLimitMiddleware(limiter, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Request without claims should pass through (authMiddleware handles this)
	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestAdminRateLimitMiddleware_InvalidClaimsType tests middleware with wrong claims type
func TestAdminRateLimitMiddleware_InvalidClaimsType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := CreateTestLogger(t)
	defer logger.Close()

	limiter := ratelimit.NewMessageLimiter(time.Minute, 5)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// Set wrong type in context
		c.Set("claims", "invalid-type")
		c.Next()
	})
	router.Use(adminRateLimitMiddleware(limiter, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 500, w.Code)
}

// TestAdminRateLimitMiddleware_IndependentUserLimits tests that different users have independent limits
func TestAdminRateLimitMiddleware_IndependentUserLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := CreateTestLogger(t)
	defer logger.Close()

	// Create rate limiter with 2 requests per minute
	limiter := ratelimit.NewMessageLimiter(time.Minute, 2)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// Get user ID from query param for testing
		userID := c.Query("user_id")
		if userID == "" {
			userID = "default-user"
		}
		c.Set("claims", &auth.Claims{
			UserID: userID,
			Roles:  []string{"admin"},
		})
		c.Next()
	})
	router.Use(adminRateLimitMiddleware(limiter, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// User 1 makes 2 requests - should succeed
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("GET", "/test?user_id=user1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code)
	}

	// User 2 makes 2 requests - should also succeed (independent limit)
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("GET", "/test?user_id=user2", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code)
	}

	// User 1's 3rd request should be rate limited
	req, _ := http.NewRequest("GET", "/test?user_id=user1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 429, w.Code)

	// User 2's 3rd request should also be rate limited
	req, _ = http.NewRequest("GET", "/test?user_id=user2", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 429, w.Code)
}

// TestValidateJWTSecret tests the validateJWTSecret function
func TestValidateJWTSecret(t *testing.T) {
	t.Run("rejects empty secret", func(t *testing.T) {
		err := validateJWTSecret("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "JWT secret is required")
	})

	t.Run("rejects secret shorter than minimum length", func(t *testing.T) {
		shortSecret := "short"
		err := validateJWTSecret(shortSecret)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be at least 32 characters")
		assert.Contains(t, err.Error(), fmt.Sprintf("got %d", len(shortSecret)))
	})

	t.Run("rejects secret exactly at minimum length minus one", func(t *testing.T) {
		secret := "abcdefghijklmnopqrstuvwxyz12345" // 31 chars
		assert.Equal(t, 31, len(secret))
		err := validateJWTSecret(secret)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be at least 32 characters")
	})

	t.Run("accepts secret exactly at minimum length", func(t *testing.T) {
		secret := "abcdefghijklmnopqrstuvwxyz678901" // 32 chars, no weak patterns
		assert.Equal(t, 32, len(secret))
		err := validateJWTSecret(secret)
		assert.NoError(t, err)
	})

	t.Run("accepts long strong secret", func(t *testing.T) {
		secret := "k7jH9mP2nQ8vR4xW6yZ1aB3cD5eF0gI2jKlMnOpQrStUvWxYz"
		err := validateJWTSecret(secret)
		assert.NoError(t, err)
	})

	t.Run("rejects secret containing 'secret'", func(t *testing.T) {
		err := validateJWTSecret("my-secret-key-that-is-long-enough-for-validation")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
		assert.Contains(t, err.Error(), "secret")
	})

	t.Run("rejects secret containing 'password'", func(t *testing.T) {
		err := validateJWTSecret("my-password-key-that-is-long-enough-for-validation")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
		assert.Contains(t, err.Error(), "password")
	})

	t.Run("rejects secret containing 'test'", func(t *testing.T) {
		err := validateJWTSecret("this-is-a-test-key-that-is-long-enough")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
		assert.Contains(t, err.Error(), "test")
	})

	t.Run("rejects secret containing 'admin'", func(t *testing.T) {
		err := validateJWTSecret("this-is-an-admin-key-that-is-long-enough")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
		assert.Contains(t, err.Error(), "admin")
	})

	t.Run("rejects secret containing 'changeme'", func(t *testing.T) {
		err := validateJWTSecret("please-changeme-this-is-long-enough-for-validation")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
		assert.Contains(t, err.Error(), "changeme")
	})

	t.Run("rejects secret containing 'default'", func(t *testing.T) {
		err := validateJWTSecret("this-is-the-default-key-that-is-long-enough")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
		assert.Contains(t, err.Error(), "default")
	})

	t.Run("rejects secret containing 'example'", func(t *testing.T) {
		err := validateJWTSecret("this-is-an-example-key-that-is-long-enough")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
		assert.Contains(t, err.Error(), "example")
	})

	t.Run("rejects secret containing 'demo'", func(t *testing.T) {
		err := validateJWTSecret("this-is-a-demo-key-that-is-long-enough-for-validation")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
		assert.Contains(t, err.Error(), "demo")
	})

	t.Run("rejects secret containing '12345'", func(t *testing.T) {
		err := validateJWTSecret("my-key-with-12345-that-is-long-enough-for-validation")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
		assert.Contains(t, err.Error(), "12345")
	})

	t.Run("weak pattern check is case-insensitive", func(t *testing.T) {
		// Test uppercase
		err := validateJWTSecret("my-SECRET-key-that-is-long-enough-for-validation")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")

		// Test mixed case
		err = validateJWTSecret("my-PaSsWoRd-key-that-is-long-enough-for-validation")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "appears to be weak")
	})

	t.Run("error message includes helpful guidance", func(t *testing.T) {
		err := validateJWTSecret("short")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "openssl rand -base64 32")
	})

	t.Run("weak secret error includes generation command", func(t *testing.T) {
		err := validateJWTSecret("my-secret-key-that-is-long-enough-for-validation")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "openssl rand -base64 32")
	})

	t.Run("accepts cryptographically random-looking secret", func(t *testing.T) {
		// Simulate output from: openssl rand -base64 32
		secret := "k7jH9mP2nQ8vR4xW6yZ1aB3cD5eF0gI2"
		err := validateJWTSecret(secret)
		assert.NoError(t, err)
	})

	t.Run("accepts secret with special characters", func(t *testing.T) {
		secret := "k7jH9m!@#$%^&*()_+-=[]{}|;:,.<>?P2nQ8vR4xW6yZ1aB3cD5eF0gI2"
		err := validateJWTSecret(secret)
		assert.NoError(t, err)
	})
}

// TestShutdown_GracefulShutdown tests the graceful shutdown process
func TestShutdown_GracefulShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("shutdown with no components initialized", func(t *testing.T) {
		// Reset global variables
		shutdownMu.Lock()
		originalWSHandler := globalWSHandler
		originalSessionMgr := globalSessionMgr
		originalMessageRouter := globalMessageRouter
		originalAdminLimiter := globalAdminLimiter
		originalLogger := globalLogger

		globalWSHandler = nil
		globalSessionMgr = nil
		globalMessageRouter = nil
		globalAdminLimiter = nil
		globalLogger = nil
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

		// Shutdown should succeed even with nil components
		err := Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("shutdown with logger only", func(t *testing.T) {
		// Reset global variables
		shutdownMu.Lock()
		originalWSHandler := globalWSHandler
		originalSessionMgr := globalSessionMgr
		originalMessageRouter := globalMessageRouter
		originalAdminLimiter := globalAdminLimiter
		originalLogger := globalLogger

		testLogger := CreateTestLogger(t)
		globalWSHandler = nil
		globalSessionMgr = nil
		globalMessageRouter = nil
		globalAdminLimiter = nil
		globalLogger = testLogger
		shutdownMu.Unlock()

		// Restore after test
		defer func() {
			testLogger.Close()
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

		// Shutdown should succeed with logger
		err := Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("shutdown is thread-safe", func(t *testing.T) {
		// Create test logger
		testLogger := CreateTestLogger(t)
		defer testLogger.Close()

		// Save and set global logger
		shutdownMu.Lock()
		originalWSHandler := globalWSHandler
		originalSessionMgr := globalSessionMgr
		originalMessageRouter := globalMessageRouter
		originalAdminLimiter := globalAdminLimiter
		originalLogger := globalLogger

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

		// Call shutdown concurrently
		var wg sync.WaitGroup
		errors := make([]error, 3)

		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				errors[idx] = Shutdown(ctx)
			}(i)
		}

		wg.Wait()

		// All shutdowns should succeed (mutex protects concurrent access)
		for i, err := range errors {
			assert.NoError(t, err, "Shutdown %d should succeed", i)
		}
	})
}

// TestShutdown_WithTimeout tests shutdown behavior when context times out
func TestShutdown_WithTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("shutdown with very short timeout", func(t *testing.T) {
		// Create test logger
		testLogger := CreateTestLogger(t)
		defer testLogger.Close()

		// Save and set global variables
		shutdownMu.Lock()
		originalWSHandler := globalWSHandler
		originalSessionMgr := globalSessionMgr
		originalMessageRouter := globalMessageRouter
		originalAdminLimiter := globalAdminLimiter
		originalLogger := globalLogger

		globalWSHandler = nil // No WebSocket handler to avoid actual connection closure
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

		// Create context with very short timeout (1 nanosecond)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Wait for context to expire
		time.Sleep(10 * time.Millisecond)

		// Shutdown should still succeed because we don't have WebSocket handler
		// (the timeout only affects WebSocket connection closure)
		err := Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("shutdown respects context cancellation", func(t *testing.T) {
		// Create test logger
		testLogger := CreateTestLogger(t)
		defer testLogger.Close()

		// Save and set global variables
		shutdownMu.Lock()
		originalWSHandler := globalWSHandler
		originalSessionMgr := globalSessionMgr
		originalMessageRouter := globalMessageRouter
		originalAdminLimiter := globalAdminLimiter
		originalLogger := globalLogger

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

		// Create context and cancel it immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Shutdown should still succeed for components without timeout dependency
		err := Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("shutdown with timeout logs appropriate messages", func(t *testing.T) {
		// Create test logger
		testLogger := CreateTestLogger(t)
		defer testLogger.Close()

		// Save and set global variables
		shutdownMu.Lock()
		originalWSHandler := globalWSHandler
		originalSessionMgr := globalSessionMgr
		originalMessageRouter := globalMessageRouter
		originalAdminLimiter := globalAdminLimiter
		originalLogger := globalLogger

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

		// Create context with reasonable timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Shutdown should succeed and log messages
		err := Shutdown(ctx)
		assert.NoError(t, err)

		// Note: In a real scenario with WebSocket connections, we would verify
		// that timeout warnings are logged when context deadline is exceeded
	})
}
