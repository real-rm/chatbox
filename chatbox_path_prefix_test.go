package chatbox

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/real-rm/chatbox/internal/constants"
	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPathPrefixRouteRegistration verifies that routes are registered with the configured path prefix
func TestPathPrefixRouteRegistration(t *testing.T) {
	// Skip when MongoDB is not available to avoid hanging on connection timeout
	if testing.Short() || os.Getenv("SKIP_MONGO_TESTS") != "" {
		t.Skip("Skipping MongoDB-dependent integration tests")
	}

	tests := []struct {
		name       string
		pathPrefix string
		testPath   string
		shouldWork bool
	}{
		{
			name:       "default prefix /chatbox",
			pathPrefix: constants.DefaultPathPrefix,
			testPath:   "/chatbox/healthz",
			shouldWork: true,
		},
		{
			name:       "custom prefix /api/v1",
			pathPrefix: "/api/v1",
			testPath:   "/api/v1/healthz",
			shouldWork: true,
		},
		{
			name:       "single slash prefix",
			pathPrefix: "/",
			testPath:   "/healthz",
			shouldWork: true,
		},
		{
			name:       "nested prefix",
			pathPrefix: "/api/v2/chatbox",
			testPath:   "/api/v2/chatbox/healthz",
			shouldWork: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test environment
			os.Clearenv()
			os.Setenv("CHATBOX_PATH_PREFIX", tt.pathPrefix)
			os.Setenv("JWT_SECRET", "V4l1d-JWT-K3y-F0r-T3st1ng-Purp0ses-1!")
			os.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012") // 32 bytes
			os.Setenv("RMBASE_FILE_CFG", "config.toml")

			// Load config (reset first to ensure clean state per subtest)
			goconfig.ResetConfig()
			err := goconfig.LoadConfig()
			if err != nil {
				t.Skipf("Failed to load config: %v", err)
			}
			t.Cleanup(func() { goconfig.ResetConfig() })

			config, err := goconfig.Default()
			if err != nil {
				t.Skipf("Failed to get default config: %v", err)
			}

			// Create test logger (use temp dir to avoid leaving stale log directories)
			logger, err := golog.InitLog(golog.LogConfig{
				Dir:            t.TempDir(),
				Level:          "error",
				StandardOutput: false,
			})
			if err != nil {
				t.Skipf("Failed to initialize logger: %v", err)
			}
			defer logger.Close()

			// Initialize MongoDB
			mongo, err := gomongo.InitMongoDB(logger, config)
			if err != nil {
				t.Skipf("MongoDB not available: %v", err)
			}

			// Set Gin to test mode
			gin.SetMode(gin.TestMode)

			// Create router and register routes
			router := gin.New()
			err = Register(router, config, logger, mongo)
			require.NoError(t, err, "Failed to register routes")

			// Test the health check endpoint with the configured prefix
			req := httptest.NewRequest(http.MethodGet, tt.testPath, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if tt.shouldWork {
				assert.Equal(t, http.StatusOK, w.Code, "Health check should return 200 OK")
				assert.Contains(t, w.Body.String(), "ok", "Health check should return 'ok'")
			}

			// Verify that the old path doesn't work (if using custom prefix)
			if tt.pathPrefix != constants.DefaultPathPrefix {
				oldReq := httptest.NewRequest(http.MethodGet, constants.DefaultPathPrefix+"/healthz", nil)
				oldW := httptest.NewRecorder()
				router.ServeHTTP(oldW, oldReq)
				assert.Equal(t, http.StatusNotFound, oldW.Code, "Old path should return 404")
			}
		})
	}
}

// TestPathPrefixValidation verifies that invalid path prefixes are rejected
func TestPathPrefixValidation(t *testing.T) {
	// Skip when MongoDB is not available to avoid hanging on connection timeout
	if testing.Short() || os.Getenv("SKIP_MONGO_TESTS") != "" {
		t.Skip("Skipping MongoDB-dependent integration tests")
	}

	tests := []struct {
		name        string
		pathPrefix  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty prefix",
			pathPrefix:  "",
			expectError: true,
			errorMsg:    "path prefix cannot be empty",
		},
		{
			name:        "missing leading slash",
			pathPrefix:  "chatbox",
			expectError: true,
			errorMsg:    "path prefix must start with '/'",
		},
		{
			name:        "valid prefix",
			pathPrefix:  "/chatbox",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test environment
			os.Clearenv()
			os.Setenv("CHATBOX_PATH_PREFIX", tt.pathPrefix)
			os.Setenv("JWT_SECRET", "V4l1d-JWT-K3y-F0r-T3st1ng-Purp0ses-1!")
			os.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012") // 32 bytes
			os.Setenv("RMBASE_FILE_CFG", "config.toml")

			// Load config (reset first to ensure clean state per subtest)
			goconfig.ResetConfig()
			err := goconfig.LoadConfig()
			if err != nil {
				t.Skipf("Failed to load config: %v", err)
			}
			t.Cleanup(func() { goconfig.ResetConfig() })

			config, err := goconfig.Default()
			if err != nil {
				t.Skipf("Failed to get default config: %v", err)
			}

			// Create test logger (use temp dir to avoid leaving stale log directories)
			logger, err := golog.InitLog(golog.LogConfig{
				Dir:            t.TempDir(),
				Level:          "error",
				StandardOutput: false,
			})
			if err != nil {
				t.Skipf("Failed to initialize logger: %v", err)
			}
			defer logger.Close()

			// Initialize MongoDB
			mongo, err := gomongo.InitMongoDB(logger, config)
			if err != nil {
				t.Skipf("MongoDB not available: %v", err)
			}

			// Set Gin to test mode
			gin.SetMode(gin.TestMode)

			// Create router and register routes
			router := gin.New()
			err = Register(router, config, logger, mongo)

			if tt.expectError {
				require.Error(t, err, "Expected error for invalid path prefix")
				assert.Contains(t, err.Error(), tt.errorMsg, "Error message should match")
			} else {
				require.NoError(t, err, "Should not error for valid path prefix")
			}
		})
	}
}
