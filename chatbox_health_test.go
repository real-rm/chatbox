package chatbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMongoDBHealthCheck_Integration tests the MongoDB health check with actual MongoDB
// Run with: go test -v -run TestMongoDBHealthCheck_Integration
// Requires: MongoDB running (docker-compose up -d mongodb)
func TestMongoDBHealthCheck_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if os.Getenv("SKIP_MONGO_TESTS") != "" {
		t.Skip("Skipping MongoDB integration test")
	}

	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Set config file path
	os.Setenv("RMBASE_FILE_CFG", "config.toml")
	t.Cleanup(func() { goconfig.ResetConfig() })

	// Load config (reset first to override any previously loaded config)
	goconfig.ResetConfig()
	err := goconfig.LoadConfig()
	require.NoError(t, err, "Failed to load config")

	// Create logger
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            "logs",
		Level:          "error",
		StandardOutput: false,
	})
	require.NoError(t, err, "Failed to initialize logger")
	defer logger.Close()

	// Get config accessor
	config, err := goconfig.Default()
	require.NoError(t, err, "Failed to get config accessor")

	t.Run("healthy MongoDB returns 200", func(t *testing.T) {
		// Initialize MongoDB
		mongo, err := gomongo.InitMongoDB(logger, config)
		if err != nil {
			t.Skipf("MongoDB not available: %v", err)
		}

		// Verify MongoDB is actually reachable by pinging it directly
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		testColl := mongo.Coll("admin", "system.version")
		err = testColl.Ping(ctx)
		if err != nil {
			t.Skipf("MongoDB not reachable: %v", err)
		}

		// Create router and register handler
		router := gin.New()
		router.GET("/readyz", handleReadyCheck(mongo, nil, logger))

		// Make request
		req, _ := http.NewRequest("GET", "/readyz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Verify response
		assert.Equal(t, 200, w.Code, "Expected 200 OK when MongoDB is healthy")
		assert.Contains(t, w.Body.String(), `"status":"ready"`)
		assert.Contains(t, w.Body.String(), "mongodb")

		t.Logf("Response: %s", w.Body.String())
	})
}

// TestMongoDBHealthCheck_Down tests the health check when MongoDB is down
// This test simulates MongoDB being unavailable
func TestMongoDBHealthCheck_Down(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	t.Run("nil MongoDB returns 503", func(t *testing.T) {
		// Create a test logger
		testLogger, _ := golog.InitLog(golog.LogConfig{
			Dir:            "logs",
			Level:          "error",
			StandardOutput: false,
		})
		defer testLogger.Close()

		// Create router with nil MongoDB
		router := gin.New()
		router.GET("/readyz", handleReadyCheck(nil, nil, testLogger))

		// Make request
		req, _ := http.NewRequest("GET", "/readyz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Verify response
		assert.Equal(t, 503, w.Code, "Expected 503 when MongoDB is nil")
		assert.Contains(t, w.Body.String(), `"status":"not ready"`)
		assert.Contains(t, w.Body.String(), "MongoDB not initialized")

		t.Logf("Response: %s", w.Body.String())
	})

	t.Run("unreachable MongoDB returns 503", func(t *testing.T) {
		if os.Getenv("SKIP_MONGO_TESTS") != "" {
			t.Skip("Skipping MongoDB test")
		}

		// Set config file path
		os.Setenv("RMBASE_FILE_CFG", "config.toml")
		t.Cleanup(func() { goconfig.ResetConfig() })

		// Load config (reset first to override any previously loaded config)
		goconfig.ResetConfig()
		err := goconfig.LoadConfig()
		if err != nil {
			t.Skipf("Failed to load config: %v", err)
		}

		// Create logger
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
			t.Skipf("Failed to get config accessor: %v", err)
		}

		// Override MongoDB URI to point to non-existent server
		// Use a port that's unlikely to be in use and set short timeouts
		os.Setenv("MONGO_URI", "mongodb://localhost:27099/test?connectTimeoutMS=500&serverSelectionTimeoutMS=500")

		// Try to initialize MongoDB
		mongo, err := gomongo.InitMongoDB(logger, config)
		if err != nil {
			// If initialization fails, test with nil
			t.Log("MongoDB initialization failed as expected:", err)

			router := gin.New()
			router.GET("/readyz", handleReadyCheck(nil, nil, logger))

			req, _ := http.NewRequest("GET", "/readyz", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, 503, w.Code)
			assert.Contains(t, w.Body.String(), "not ready")
			return
		}

		// If initialization succeeded (lazy connection), the Ping should fail
		router := gin.New()
		router.GET("/readyz", handleReadyCheck(mongo, nil, logger))

		req, _ := http.NewRequest("GET", "/readyz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should return 503 because MongoDB is unreachable
		assert.Equal(t, 503, w.Code, "Expected 503 when MongoDB is unreachable")
		assert.Contains(t, w.Body.String(), `"status":"not ready"`)
		assert.Contains(t, w.Body.String(), "Database connectivity check failed")

		t.Logf("Response: %s", w.Body.String())
	})
}

// TestMongoDBHealthCheck_Timeout tests that the health check respects the timeout
func TestMongoDBHealthCheck_Timeout(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	t.Run("health check completes within timeout", func(t *testing.T) {
		// Create a test logger
		testLogger, _ := golog.InitLog(golog.LogConfig{
			Dir:            "logs",
			Level:          "error",
			StandardOutput: false,
		})
		defer testLogger.Close()

		// Create router with nil MongoDB (fast path)
		router := gin.New()
		router.GET("/readyz", handleReadyCheck(nil, nil, testLogger))

		// Make request and measure time
		start := time.Now()
		req, _ := http.NewRequest("GET", "/readyz", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		elapsed := time.Since(start)

		// Should complete quickly (well under 2 seconds)
		assert.Less(t, elapsed, 100*time.Millisecond, "Health check should complete quickly when MongoDB is nil")
		assert.Equal(t, 503, w.Code)

		t.Logf("Health check completed in %v", elapsed)
	})
}
