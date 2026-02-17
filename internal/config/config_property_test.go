package config

import (
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: production-readiness-fixes, Property 17: Weak secrets are rejected
// **Validates: Requirements 17.1, 17.2, 17.3**
//
// For any JWT secret shorter than 32 characters or containing common weak patterns,
// validation should fail.
func TestProperty_WeakSecretsAreRejected(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Test 1: Secrets shorter than 32 characters should be rejected
	properties.Property("secrets shorter than 32 characters are rejected", prop.ForAll(
		func(secretLength int) bool {
			if secretLength < 1 || secretLength >= 32 {
				return true // Skip invalid ranges
			}

			// Generate a secret of the specified length
			secret := strings.Repeat("a", secretLength)

			cfg := &Config{
				Server: ServerConfig{
					Port:      8080,
					JWTSecret: secret,
				},
			}

			// Validation should fail
			err := cfg.Validate()
			return err != nil && strings.Contains(err.Error(), "at least 32 characters")
		},
		gen.IntRange(1, 31),
	))

	// Test 2: Secrets containing weak patterns should be rejected
	properties.Property("secrets containing weak patterns are rejected", prop.ForAll(
		func(weakPattern string) bool {
			// Use one of the known weak patterns
			weakPatterns := []string{"secret", "test", "password", "admin", "changeme", "default", "example", "demo"}
			if len(weakPatterns) == 0 {
				return true
			}

			// Pick a weak pattern
			pattern := weakPatterns[len(weakPattern)%len(weakPatterns)]

			// Create a 32+ character secret that contains the weak pattern
			secret := pattern + strings.Repeat("x", 32)

			cfg := &Config{
				Server: ServerConfig{
					Port:      8080,
					JWTSecret: secret,
				},
			}

			// Validation should fail
			err := cfg.Validate()
			return err != nil && strings.Contains(err.Error(), "appears to be weak")
		},
		gen.AlphaString(),
	))

	// Test 3: Empty secret should be rejected
	properties.Property("empty secret is rejected", prop.ForAll(
		func() bool {
			cfg := &Config{
				Server: ServerConfig{
					Port:      8080,
					JWTSecret: "",
				},
			}

			// Validation should fail
			err := cfg.Validate()
			return err != nil && strings.Contains(err.Error(), "JWT secret is required")
		},
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: production-readiness-fixes, Property 18: Strong secrets are accepted
// **Validates: Requirements 17.1, 17.2, 17.3**
//
// For any JWT secret that is at least 32 characters and doesn't contain weak patterns,
// validation should succeed.
func TestProperty_StrongSecretsAreAccepted(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("strong random secrets are accepted", prop.ForAll(
		func(secretLength int, randomSuffix string) bool {
			if secretLength < 32 || secretLength > 100 {
				return true // Skip invalid ranges
			}

			// Generate a strong secret using random characters
			// Use a prefix that doesn't match weak patterns
			prefix := "strong_"
			
			// Calculate how many characters we need to add
			neededLength := secretLength - len(prefix) - len(randomSuffix)
			if neededLength < 0 {
				// If randomSuffix is too long, truncate it
				if len(randomSuffix) > secretLength-len(prefix) {
					randomSuffix = randomSuffix[:secretLength-len(prefix)]
					neededLength = 0
				} else {
					neededLength = 0
				}
			}
			
			secret := prefix + randomSuffix + strings.Repeat("x", neededLength)

			// Ensure it doesn't contain weak patterns
			lowerSecret := strings.ToLower(secret)
			weakPatterns := []string{"secret", "test", "password", "admin", "changeme", "default", "example", "demo"}
			for _, weak := range weakPatterns {
				if strings.Contains(lowerSecret, weak) {
					return true // Skip this test case
				}
			}

			// Ensure it's at least 32 characters
			if len(secret) < 32 {
				return true // Skip
			}

			cfg := &Config{
				Server: ServerConfig{
					Port:             8080,
					JWTSecret:        secret,
					ReconnectTimeout: 15 * 60 * 1000000000, // 15 minutes in nanoseconds
					MaxConnections:   1000,
					RateLimit:        100,
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

			// Validation should succeed (no JWT secret error)
			err := cfg.Validate()
			if err != nil {
				// Check that the error is not about JWT secret
				errStr := err.Error()
				if strings.Contains(errStr, "JWT secret") {
					return false
				}
			}

			return true
		},
		gen.IntRange(32, 100),
		gen.AlphaString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
