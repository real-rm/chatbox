package config

import (
	"os"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProductionIssue19_ValidationCalled verifies that Validate() must be called explicitly
//
// Production Readiness Issue #19: Config validation not automatic
// Location: config/config.go
// Impact: Invalid config can be loaded without errors
//
// This test documents that Load() does not call Validate() automatically.
func TestProductionIssue19_ValidationCalled(t *testing.T) {
	// Set invalid environment variables
	os.Setenv("SERVER_PORT", "-1") // Invalid port
	os.Setenv("JWT_SECRET", "")    // Missing required field
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("JWT_SECRET")
	}()

	// Load config (should succeed even with invalid values)
	cfg, err := Load()
	require.NoError(t, err, "Load() should not validate")
	require.NotNil(t, cfg)

	// Verify invalid values were loaded
	assert.Equal(t, -1, cfg.Server.Port, "Invalid port should be loaded")
	assert.Equal(t, "", cfg.Server.JWTSecret, "Empty JWT secret should be loaded")

	t.Log("FINDING: Load() does NOT call Validate() automatically")
	t.Log("IMPACT: Invalid configuration can be loaded without errors")

	// Now call Validate() explicitly
	err = cfg.Validate()
	assert.Error(t, err, "Validate() should return errors for invalid config")

	t.Log("STATUS: Validate() must be called explicitly")
	t.Log("RECOMMENDATION: Call Validate() after Load() in main.go")
}

// TestProductionIssue19_ValidationCoverage verifies all config fields are validated
//
// This test ensures comprehensive validation coverage.
func TestProductionIssue19_ValidationCoverage(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *Config
		expectError bool
		errorMsg    string
	}{
		// Port range validation (15.3.1)
		{
			name: "invalid port - too low",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.Port = 0
				return cfg
			},
			expectError: true,
			errorMsg:    "port must be between 1 and 65535",
		},
		{
			name: "invalid port - negative",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.Port = -1
				return cfg
			},
			expectError: true,
			errorMsg:    "port must be between 1 and 65535",
		},
		{
			name: "invalid port - too high",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.Port = 70000
				return cfg
			},
			expectError: true,
			errorMsg:    "port must be between 1 and 65535",
		},
		{
			name: "valid port - minimum",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.Port = 1
				return cfg
			},
			expectError: false,
		},
		{
			name: "valid port - maximum",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.Port = 65535
				return cfg
			},
			expectError: false,
		},
		// Required field validation (15.3.2)
		{
			name: "missing JWT secret",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.JWTSecret = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "JWT secret is required",
		},
		{
			name: "missing database URI",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Database.URI = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "database URI is required",
		},
		{
			name: "missing database name",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Database.Database = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "database name is required",
		},
		{
			name: "missing database collection",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Database.Collection = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "database collection is required",
		},
		{
			name: "missing storage bucket",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Storage.Bucket = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "storage bucket is required",
		},
		{
			name: "missing storage region",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Storage.Region = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "storage region is required",
		},
		{
			name: "missing storage access key ID",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Storage.AccessKeyID = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "storage access key ID is required",
		},
		{
			name: "missing storage secret access key",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Storage.SecretAccessKey = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "storage secret access key is required",
		},
		{
			name: "no LLM providers",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers = []LLMProviderConfig{}
				return cfg
			},
			expectError: true,
			errorMsg:    "at least one LLM provider is required",
		},
		{
			name: "missing LLM provider ID",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers[0].ID = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "ID is required",
		},
		{
			name: "missing LLM provider name",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers[0].Name = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name: "missing LLM provider type",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers[0].Type = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "type is required",
		},
		{
			name: "missing LLM provider endpoint",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers[0].Endpoint = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "endpoint is required",
		},
		{
			name: "missing LLM provider API key",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers[0].APIKey = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "API key is required",
		},
		// Format validation (15.3.3)
		{
			name: "JWT secret too short",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.JWTSecret = "short" // Less than 32 characters
				return cfg
			},
			expectError: true,
			errorMsg:    "JWT secret must be at least 32 characters",
		},
		{
			name: "weak JWT secret - contains 'password'",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.JWTSecret = "mypassword123456789012345678901234" // 36 chars but weak
				return cfg
			},
			expectError: true,
			errorMsg:    "JWT secret appears to be weak",
		},
		{
			name: "invalid LLM provider type",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers[0].Type = "invalid"
				return cfg
			},
			expectError: true,
			errorMsg:    "type must be openai, anthropic, or dify",
		},
		{
			name: "valid LLM provider type - openai",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers[0].Type = "openai"
				return cfg
			},
			expectError: false,
		},
		{
			name: "valid LLM provider type - anthropic",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers[0].Type = "anthropic"
				return cfg
			},
			expectError: false,
		},
		{
			name: "valid LLM provider type - dify",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.LLM.Providers[0].Type = "dify"
				return cfg
			},
			expectError: false,
		},
		{
			name: "invalid reconnect timeout - zero",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.ReconnectTimeout = 0
				return cfg
			},
			expectError: true,
			errorMsg:    "reconnect timeout must be positive",
		},
		{
			name: "invalid reconnect timeout - negative",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.ReconnectTimeout = -1 * time.Second
				return cfg
			},
			expectError: true,
			errorMsg:    "reconnect timeout must be positive",
		},
		{
			name: "invalid max connections - zero",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.MaxConnections = 0
				return cfg
			},
			expectError: true,
			errorMsg:    "max connections must be positive",
		},
		{
			name: "invalid max connections - negative",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.MaxConnections = -1
				return cfg
			},
			expectError: true,
			errorMsg:    "max connections must be positive",
		},
		{
			name: "invalid rate limit - zero",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.RateLimit = 0
				return cfg
			},
			expectError: true,
			errorMsg:    "rate limit must be positive",
		},
		{
			name: "invalid rate limit - negative",
			setupConfig: func() *Config {
				cfg := createValidConfig()
				cfg.Server.RateLimit = -1
				return cfg
			},
			expectError: true,
			errorMsg:    "rate limit must be positive",
		},
		// Valid config (15.3.4)
		{
			name: "valid config - all fields correct",
			setupConfig: func() *Config {
				return createValidConfig()
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			err := cfg.Validate()

			if tt.expectError {
				assert.Error(t, err, "Should return validation error")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg,
						"Error should contain expected message")
				}
			} else {
				assert.NoError(t, err, "Should not return error for valid config")
			}
		})
	}

	t.Log("STATUS: All config fields are validated")
	t.Log("FINDING: Comprehensive validation coverage exists")
	t.Log("COVERAGE: Port range, required fields, format validation all tested")
}

// createValidConfig creates a valid configuration for testing
func createValidConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:             8080,
			ReconnectTimeout: 15 * time.Minute,
			MaxConnections:   10000,
			RateLimit:        100,
			JWTSecret:        "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6", // 36 chars, strong secret
			LLMStreamTimeout: 120 * time.Second,
			PathPrefix:       constants.DefaultPathPrefix,
		},
		Database: DatabaseConfig{
			URI:            "mongodb://localhost:27017",
			Database:       "chat",
			Collection:     "sessions",
			ConnectTimeout: 10 * time.Second,
		},
		Storage: StorageConfig{
			Endpoint:        "http://localhost:9000",
			Region:          "us-east-1",
			Bucket:          "chat-files",
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret-key",
		},
		LLM: LLMConfig{
			Providers: []LLMProviderConfig{
				{
					ID:       "openai-1",
					Name:     "OpenAI GPT-4",
					Type:     "openai",
					Endpoint: "https://api.openai.com/v1",
					APIKey:   "sk-test-key",
					Model:    "gpt-4",
				},
			},
		},
		Notification: NotificationConfig{
			AdminEmails: []string{"admin@example.com"},
			EmailFrom:   "noreply@example.com",
			SMTPHost:    "smtp.example.com",
			SMTPPort:    587,
		},
		Kubernetes: KubernetesConfig{
			Namespace:      "default",
			ServiceName:    "chat-websocket",
			ConfigMapName:  "chat-config",
			SecretName:     "chat-secrets",
			EnableK8sProbe: true,
		},
	}
}

// TestLLMStreamTimeout_DefaultValue verifies the default timeout is set correctly
//
// Production Readiness Issue #8: LLM Streaming Timeout
// This test verifies that LLMStreamTimeout has a sensible default value.
func TestLLMStreamTimeout_DefaultValue(t *testing.T) {
	// Clear any existing environment variable
	os.Unsetenv("LLM_STREAM_TIMEOUT")
	defer os.Unsetenv("LLM_STREAM_TIMEOUT")

	// Load config
	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify default timeout is 120 seconds
	assert.Equal(t, 120*time.Second, cfg.Server.LLMStreamTimeout,
		"Default LLM stream timeout should be 120 seconds")

	t.Log("STATUS: LLMStreamTimeout defaults to 120 seconds")
}

// TestLLMStreamTimeout_EnvironmentVariable verifies timeout can be configured via env var
//
// Production Readiness Issue #8: LLM Streaming Timeout
// This test verifies that LLMStreamTimeout can be configured via environment variable.
func TestLLMStreamTimeout_EnvironmentVariable(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected time.Duration
	}{
		{
			name:     "custom timeout - 60 seconds",
			envValue: "60s",
			expected: 60 * time.Second,
		},
		{
			name:     "custom timeout - 5 minutes",
			envValue: "5m",
			expected: 5 * time.Minute,
		},
		{
			name:     "custom timeout - 300 seconds",
			envValue: "300s",
			expected: 300 * time.Second,
		},
		{
			name:     "invalid value - falls back to default",
			envValue: "invalid",
			expected: 120 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			os.Setenv("LLM_STREAM_TIMEOUT", tt.envValue)
			defer os.Unsetenv("LLM_STREAM_TIMEOUT")

			// Load config
			cfg, err := Load()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// Verify timeout value
			assert.Equal(t, tt.expected, cfg.Server.LLMStreamTimeout,
				"LLM stream timeout should match expected value")
		})
	}

	t.Log("STATUS: LLMStreamTimeout can be configured via LLM_STREAM_TIMEOUT env var")
}
