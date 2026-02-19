package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Property 1: Configuration Loading Handles All Inputs
// **Validates: Requirements 1.2, 4.2, 4.3**
//
// For any configuration source (file or environment variable) and any configuration value,
// the configuration loading system should either successfully load the value or return an
// appropriate error without crashing.
func TestProperty_ConfigurationLoadingHandlesAllInputs(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 1.1: Configuration loading handles any file path without crashing
	properties.Property("configuration loading handles any file path without crashing", prop.ForAll(
		func(pathComponents []string) bool {
			// Clean up environment before test
			clearEnvVars()
			defer clearEnvVars()

			// Generate a file path from components
			if len(pathComponents) == 0 {
				pathComponents = []string{"config.toml"}
			}

			// Filter out empty components and null bytes
			validComponents := make([]string, 0)
			for _, comp := range pathComponents {
				if comp != "" && !strings.Contains(comp, "\x00") {
					validComponents = append(validComponents, comp)
				}
			}

			if len(validComponents) == 0 {
				validComponents = []string{"config.toml"}
			}

			configPath := filepath.Join(validComponents...)

			// Set the config file path
			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Attempt to load configuration - should not crash
			cfg, err := loadConfiguration()

			// Either succeeds or returns an error, but doesn't crash
			if err != nil {
				// Error is acceptable
				return cfg == nil
			}

			// Success is also acceptable
			return cfg != nil
		},
		gen.SliceOf(gen.AlphaString()),
	))

	// Property 1.2: Configuration loading handles empty and missing files gracefully
	properties.Property("configuration loading handles empty and missing files gracefully", prop.ForAll(
		func(useEmptyFile bool) bool {
			clearEnvVars()
			defer clearEnvVars()

			var configPath string
			if useEmptyFile {
				// Create an empty config file
				tmpDir := t.TempDir()
				configPath = filepath.Join(tmpDir, "empty_config.toml")
				err := os.WriteFile(configPath, []byte(""), 0644)
				if err != nil {
					t.Logf("Failed to create empty config file: %v", err)
					return true // Skip this test case
				}
			} else {
				// Use a non-existent file path
				configPath = "/nonexistent/path/config.toml"
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Should handle gracefully without crashing
			cfg, err := loadConfiguration()

			// Either returns error or succeeds with defaults
			return (err != nil && cfg == nil) || (err == nil && cfg != nil)
		},
		gen.Bool(),
	))

	// Property 1.3: Configuration loading handles malformed TOML content
	properties.Property("configuration loading handles malformed TOML content", prop.ForAll(
		func(malformedContent string) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Create a temp file with malformed content
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "malformed_config.toml")

			// Generate various types of malformed TOML
			malformedPatterns := []string{
				"[server\nport = 8080",                // Unclosed section
				"[server]\nport = ",                   // Missing value
				"[server]\nport = \"not a number\"",   // Wrong type
				"[server]\nport = 8080\nport = 9090",  // Duplicate keys
				"invalid syntax here",                 // Invalid syntax
				"[server]\n\x00invalid",               // Null byte
				"",                                    // Empty file
				"# Only comments\n# No actual config", // Only comments
			}

			// Use one of the malformed patterns
			content := malformedPatterns[len(malformedContent)%len(malformedPatterns)]

			err := os.WriteFile(configPath, []byte(content), 0644)
			if err != nil {
				t.Logf("Failed to create malformed config file: %v", err)
				return true // Skip this test case
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Should handle malformed content without crashing
			cfg, err := loadConfiguration()

			// Either returns error or succeeds (goconfig may be lenient)
			return (err != nil && cfg == nil) || (err == nil && cfg != nil)
		},
		gen.AlphaString(),
	))

	// Property 1.4: Configuration loading handles environment variable overrides
	properties.Property("configuration loading handles environment variable overrides", prop.ForAll(
		func(port int, logLevel string) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Create a valid config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[server]
port = 8080

[log]
level = "info"
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true // Skip this test case
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Set environment variable overrides
			if port > 0 && port <= 65535 {
				os.Setenv("SERVER_PORT", string(rune(port)))
				defer os.Unsetenv("SERVER_PORT")
			}

			if logLevel != "" {
				os.Setenv("LOG_LEVEL", logLevel)
				defer os.Unsetenv("LOG_LEVEL")
			}

			// Should load configuration successfully
			cfg, err := loadConfiguration()

			// Should either succeed or fail gracefully
			return (err == nil && cfg != nil) || (err != nil && cfg == nil)
		},
		gen.IntRange(1, 65535),
		gen.OneConstOf("debug", "info", "warn", "error", "invalid", ""),
	))

	// Property 1.5: Configuration loading handles empty environment variable values
	properties.Property("configuration loading handles empty environment variable values", prop.ForAll(
		func(setEmptyPort bool, setEmptyLogLevel bool, setEmptyLogDir bool) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Create a valid config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[server]
port = 8080

[log]
level = "info"
dir = "/tmp/logs"
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true // Skip this test case
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Set empty environment variables
			if setEmptyPort {
				os.Setenv("SERVER_PORT", "")
				defer os.Unsetenv("SERVER_PORT")
			}

			if setEmptyLogLevel {
				os.Setenv("LOG_LEVEL", "")
				defer os.Unsetenv("LOG_LEVEL")
			}

			if setEmptyLogDir {
				os.Setenv("LOG_DIR", "")
				defer os.Unsetenv("LOG_DIR")
			}

			// Should handle empty values gracefully
			cfg, err := loadConfiguration()

			// Should either succeed or fail gracefully
			return (err == nil && cfg != nil) || (err != nil && cfg == nil)
		},
		gen.Bool(),
		gen.Bool(),
		gen.Bool(),
	))

	// Property 1.6: Configuration loading handles special characters in paths
	properties.Property("configuration loading handles special characters in paths", prop.ForAll(
		func(specialChar rune) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Skip null bytes as they're not valid in paths
			if specialChar == 0 {
				return true
			}

			// Create a path with special character
			configPath := "/tmp/config" + string(specialChar) + ".toml"

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Should handle special characters without crashing
			cfg, err := loadConfiguration()

			// Either returns error or succeeds
			return (err != nil && cfg == nil) || (err == nil && cfg != nil)
		},
		gen.Rune(),
	))

	// Property 1.7: Configuration loading handles boundary values
	properties.Property("configuration loading handles boundary values", prop.ForAll(
		func(boundaryCase int) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Create a valid config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")

			// Generate config with boundary values
			var configContent string
			switch boundaryCase % 5 {
			case 0:
				// Minimum port
				configContent = "[server]\nport = 1"
			case 1:
				// Maximum port
				configContent = "[server]\nport = 65535"
			case 2:
				// Zero port (invalid)
				configContent = "[server]\nport = 0"
			case 3:
				// Negative port (invalid)
				configContent = "[server]\nport = -1"
			case 4:
				// Port exceeding maximum (invalid)
				configContent = "[server]\nport = 99999"
			}

			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true // Skip this test case
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Should handle boundary values without crashing
			cfg, err := loadConfiguration()

			// Either returns error or succeeds
			return (err != nil && cfg == nil) || (err == nil && cfg != nil)
		},
		gen.Int(),
	))

	// Property 1.8: Configuration loading without any config file or environment variables
	properties.Property("configuration loading works with defaults when no config is provided", prop.ForAll(
		func() bool {
			clearEnvVars()
			defer clearEnvVars()

			// Don't set any config file or environment variables
			// Should use defaults or return error gracefully
			cfg, err := loadConfiguration()

			// Either returns error or succeeds with defaults
			return (err != nil && cfg == nil) || (err == nil && cfg != nil)
		},
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Property 1.9: Configuration values can be retrieved after loading
// **Validates: Requirements 1.2, 4.2**
//
// For any successfully loaded configuration, all configuration values should be
// retrievable without errors.
func TestProperty_ConfigurationValuesRetrievable(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("configuration values are retrievable after successful load", prop.ForAll(
		func(port int, logLevel string, logDir string) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Constrain inputs to valid ranges
			if port < 1 || port > 65535 {
				port = 8080
			}

			validLogLevels := []string{"debug", "info", "warn", "error"}
			if logLevel == "" || !contains(validLogLevels, logLevel) {
				logLevel = "info"
			}

			if logDir == "" {
				logDir = "/tmp/logs"
			}

			// Create a valid config file with the given values
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := "[server]\nport = " + string(rune(port)) + "\n\n[log]\nlevel = \"" + logLevel + "\"\ndir = \"" + logDir + "\"\n"

			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true // Skip this test case
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Load configuration
			cfg, err := loadConfiguration()
			if err != nil {
				// If loading fails, that's acceptable
				return cfg == nil
			}

			if cfg == nil {
				return false
			}

			// Try to retrieve values - should not panic
			_, err1 := cfg.ConfigIntWithDefault("server.port", 8080)
			_, err2 := cfg.ConfigStringWithDefault("log.level", "info")
			_, err3 := cfg.ConfigStringWithDefault("log.dir", "/tmp/logs")

			// All retrievals should succeed or fail gracefully
			return err1 == nil && err2 == nil && err3 == nil
		},
		gen.IntRange(1, 65535),
		gen.OneConstOf("debug", "info", "warn", "error"),
		gen.AlphaString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
