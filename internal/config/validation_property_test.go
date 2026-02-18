package config

import (
	"strings"
	"testing"
	"testing/quick"

	"github.com/real-rm/chatbox/internal/constants"
)

// Feature: production-readiness-fixes, Property 21: Invalid config prevents startup
// **Validates: Requirements 19.2, 19.4, 19.5**
//
// Property: For any configuration with missing required fields or invalid values,
// validation should fail
func TestProperty_InvalidConfigRejection(t *testing.T) {
	property := func(secretLength uint8) bool {
		// Test with secrets that are too short (0-31 characters)
		length := int(secretLength % 32)

		cfg := &Config{
			Server: ServerConfig{
				Port:             8080,
				ReconnectTimeout: 15 * 60 * 1000000000, // 15 minutes in nanoseconds
				MaxConnections:   10000,
				RateLimit:        100,
				JWTSecret:        strings.Repeat("a", length), // Too short
				PathPrefix:       constants.DefaultPathPrefix,
			},
			Database: DatabaseConfig{
				URI:        "mongodb://localhost:27017",
				Database:   "test",
				Collection: "sessions",
			},
			Storage: StorageConfig{
				Bucket:          "test-bucket",
				Region:          "us-east-1",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
			},
			LLM: LLMConfig{
				Providers: []LLMProviderConfig{
					{
						ID:       "test",
						Name:     "Test Provider",
						Type:     "openai",
						Endpoint: "https://api.openai.com",
						APIKey:   "test-key",
					},
				},
			},
		}

		// Validation should fail for short secrets
		err := cfg.Validate()
		if err == nil {
			t.Logf("Validation passed for secret length %d, but should have failed", length)
			return false
		}

		// Error message should mention JWT secret
		if !strings.Contains(err.Error(), "JWT secret") {
			t.Logf("Error message doesn't mention JWT secret: %v", err)
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}

// Feature: production-readiness-fixes, Property 21: Invalid config prevents startup
// **Validates: Requirements 19.2, 19.4, 19.5**
//
// Property: Configuration with weak JWT secrets should be rejected
func TestProperty_WeakSecretRejection(t *testing.T) {
	weakPatterns := []string{"test", "password", "admin", "secret", "changeme", "default"}

	property := func(patternIndex uint8) bool {
		// Select a weak pattern
		pattern := weakPatterns[int(patternIndex)%len(weakPatterns)]

		// Create a 32+ character secret that contains a weak pattern
		weakSecret := strings.Repeat("x", 16) + pattern + strings.Repeat("y", 16)

		cfg := &Config{
			Server: ServerConfig{
				Port:             8080,
				ReconnectTimeout: 15 * 60 * 1000000000,
				MaxConnections:   10000,
				RateLimit:        100,
				JWTSecret:        weakSecret,
				PathPrefix:       constants.DefaultPathPrefix,
			},
			Database: DatabaseConfig{
				URI:        "mongodb://localhost:27017",
				Database:   "test",
				Collection: "sessions",
			},
			Storage: StorageConfig{
				Bucket:          "test-bucket",
				Region:          "us-east-1",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
			},
			LLM: LLMConfig{
				Providers: []LLMProviderConfig{
					{
						ID:       "test",
						Name:     "Test Provider",
						Type:     "openai",
						Endpoint: "https://api.openai.com",
						APIKey:   "test-key",
					},
				},
			},
		}

		// Validation should fail for weak secrets
		err := cfg.Validate()
		if err == nil {
			t.Logf("Validation passed for weak secret containing '%s', but should have failed", pattern)
			return false
		}

		// Error message should mention weak secret
		if !strings.Contains(err.Error(), "weak") {
			t.Logf("Error message doesn't mention weak secret: %v", err)
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}

// Feature: production-readiness-fixes, Property 22: Valid config passes validation
// **Validates: Requirements 19.4, 19.5**
//
// Property: For any configuration with all required fields and valid values,
// validation should succeed
func TestProperty_ValidConfigAcceptance(t *testing.T) {
	property := func(portOffset uint8, maxConns uint16) bool {
		// Generate valid values
		port := 8000 + int(portOffset%100)          // 8000-8099
		maxConnections := 1000 + int(maxConns%9000) // 1000-10000

		// Generate a strong random-looking secret (32+ characters, no weak patterns)
		strongSecret := "AbCdEfGhIjKlMnOpQrStUvWxYz6789!@#$%^&*()"

		cfg := &Config{
			Server: ServerConfig{
				Port:             port,
				ReconnectTimeout: 15 * 60 * 1000000000,
				MaxConnections:   maxConnections,
				RateLimit:        100,
				JWTSecret:        strongSecret,
				PathPrefix:       constants.DefaultPathPrefix,
			},
			Database: DatabaseConfig{
				URI:        "mongodb://localhost:27017",
				Database:   "test",
				Collection: "sessions",
			},
			Storage: StorageConfig{
				Bucket:          "test-bucket",
				Region:          "us-east-1",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
			},
			LLM: LLMConfig{
				Providers: []LLMProviderConfig{
					{
						ID:       "test",
						Name:     "Test Provider",
						Type:     "openai",
						Endpoint: "https://api.openai.com",
						APIKey:   "test-key",
					},
				},
			},
		}

		// Validation should succeed
		err := cfg.Validate()
		if err != nil {
			t.Logf("Validation failed for valid config: %v", err)
			return false
		}

		return true
	}

	config := &quick.Config{
		MaxCount: 100,
	}

	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property violated: %v", err)
	}
}
