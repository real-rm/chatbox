package config

import (
	"os"
	"testing"

	"github.com/real-rm/chatbox/internal/constants"
	"github.com/stretchr/testify/assert"
)

func TestPathPrefix_DefaultValue(t *testing.T) {
	// Clear environment
	os.Clearenv()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.Equal(t, constants.DefaultPathPrefix, cfg.Server.PathPrefix)
}

func TestPathPrefix_EnvironmentVariable(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "custom prefix",
			envValue: "/api/chat",
			expected: "/api/chat",
		},
		{
			name:     "single slash",
			envValue: "/",
			expected: "/",
		},
		{
			name:     "nested path",
			envValue: "/v1/api/chatbox",
			expected: "/v1/api/chatbox",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set environment
			os.Clearenv()
			os.Setenv("CHATBOX_PATH_PREFIX", tt.envValue)

			cfg, err := Load()
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.Server.PathPrefix)
		})
	}
}

func TestPathPrefix_ConfigFile(t *testing.T) {
	// This test verifies that path prefix can be loaded from config file
	// when environment variable is not set
	tests := []struct {
		name     string
		envValue string // Empty means not set
		expected string
	}{
		{
			name:     "from config file when env not set",
			envValue: "",
			expected: "/chatbox", // Value from config.toml
		},
		{
			name:     "env overrides config file",
			envValue: "/custom",
			expected: "/custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set environment variable only if specified
			if tt.envValue != "" {
				os.Setenv("CHATBOX_PATH_PREFIX", tt.envValue)
			}

			// Load config (will read from config.toml if available)
			cfg, err := Load()
			assert.NoError(t, err)

			// Verify the path prefix matches expected value
			assert.Equal(t, tt.expected, cfg.Server.PathPrefix)
		})
	}
}

func TestPathPrefix_Validation(t *testing.T) {
	tests := []struct {
		name        string
		pathPrefix  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid prefix",
			pathPrefix:  "/chatbox",
			expectError: false,
		},
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
			name:        "single slash",
			pathPrefix:  "/",
			expectError: false,
		},
		{
			name:        "nested path",
			pathPrefix:  "/api/v1/chat",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Server.PathPrefix = tt.pathPrefix

			err := cfg.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
