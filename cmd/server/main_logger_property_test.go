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

// Property 2: Logger Initialization Handles All Inputs
// **Validates: Requirements 1.3**
//
// For any logger configuration (directory path, log level, output settings),
// the logger initialization should either successfully create a logger or return
// an appropriate error without crashing.
func TestProperty_LoggerInitializationHandlesAllInputs(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 2.1: Logger initialization handles any directory path without crashing
	properties.Property("logger initialization handles any directory path without crashing", prop.ForAll(
		func(pathComponents []string) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Generate a directory path from components
			if len(pathComponents) == 0 {
				pathComponents = []string{"logs"}
			}

			// Filter out empty components and null bytes
			validComponents := make([]string, 0)
			for _, comp := range pathComponents {
				if comp != "" && !strings.Contains(comp, "\x00") {
					validComponents = append(validComponents, comp)
				}
			}

			if len(validComponents) == 0 {
				validComponents = []string{"logs"}
			}

			logDir := filepath.Join(validComponents...)

			// Create a config file with the generated log directory
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[log]
dir = "` + logDir + `"
level = "info"
standardOutput = true
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true // Skip this test case
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			cfg, err := loadConfiguration()
			if err != nil {
				// Config loading failed, skip
				return true
			}

			// Attempt to initialize logger - should not crash
			logger, err := initializeLogger(cfg)

			// Either succeeds or returns an error, but doesn't crash
			if err != nil {
				// Error is acceptable
				return logger == nil
			}

			// Success - clean up
			if logger != nil {
				logger.Close()
			}
			return logger != nil
		},
		gen.SliceOf(gen.AlphaString()),
	))

	// Property 2.2: Logger initialization handles all log levels gracefully
	properties.Property("logger initialization handles all log levels gracefully", prop.ForAll(
		func(logLevel string) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Create a temp directory for logs
			tmpDir := t.TempDir()
			logDir := filepath.Join(tmpDir, "logs")

			// Create config with the given log level
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[log]
dir = "` + logDir + `"
level = "` + logLevel + `"
standardOutput = true
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true // Skip this test case
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			cfg, err := loadConfiguration()
			if err != nil {
				return true // Skip if config loading fails
			}

			// Should handle any log level without crashing
			logger, err := initializeLogger(cfg)

			// Either returns error or succeeds
			if err != nil {
				return logger == nil
			}

			if logger != nil {
				logger.Close()
			}
			return logger != nil
		},
		gen.OneConstOf("debug", "info", "warn", "error", "DEBUG", "INFO", "WARN", "ERROR", "invalid", "123", "", "trace", "fatal"),
	))

	// Property 2.3: Logger initialization handles standardOutput flag variations
	properties.Property("logger initialization handles standardOutput flag variations", prop.ForAll(
		func(standardOutput bool) bool {
			clearEnvVars()
			defer clearEnvVars()

			tmpDir := t.TempDir()
			logDir := filepath.Join(tmpDir, "logs")

			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[log]
dir = "` + logDir + `"
level = "info"
standardOutput = ` + boolToString(standardOutput) + `
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			cfg, err := loadConfiguration()
			if err != nil {
				return true
			}

			logger, err := initializeLogger(cfg)

			if err != nil {
				return logger == nil
			}

			if logger != nil {
				logger.Close()
			}
			return logger != nil
		},
		gen.Bool(),
	))

	// Property 2.4: Logger initialization handles empty configuration values
	properties.Property("logger initialization handles empty configuration values", prop.ForAll(
		func(emptyDir bool, emptyLevel bool) bool {
			clearEnvVars()
			defer clearEnvVars()

			tmpDir := t.TempDir()

			var logDir, logLevel string
			if emptyDir {
				logDir = ""
			} else {
				logDir = filepath.Join(tmpDir, "logs")
			}

			if emptyLevel {
				logLevel = ""
			} else {
				logLevel = "info"
			}

			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[log]
dir = "` + logDir + `"
level = "` + logLevel + `"
standardOutput = true
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			cfg, err := loadConfiguration()
			if err != nil {
				return true
			}

			// Should handle empty values by using defaults
			logger, err := initializeLogger(cfg)

			if err != nil {
				return logger == nil
			}

			if logger != nil {
				logger.Close()
			}
			return logger != nil
		},
		gen.Bool(),
		gen.Bool(),
	))

	// Property 2.5: Logger initialization handles missing log section
	properties.Property("logger initialization handles missing log section", prop.ForAll(
		func() bool {
			clearEnvVars()
			defer clearEnvVars()

			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")

			// Config without log section
			configContent := `
[server]
port = 8080
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			cfg, err := loadConfiguration()
			if err != nil {
				return true
			}

			// Should use defaults when log section is missing
			logger, err := initializeLogger(cfg)

			if err != nil {
				return logger == nil
			}

			if logger != nil {
				logger.Close()
			}
			return logger != nil
		},
	))

	// Property 2.6: Logger initialization handles special characters in paths
	properties.Property("logger initialization handles special characters in paths", prop.ForAll(
		func(specialChar rune) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Skip null bytes and path separators
			if specialChar == 0 || specialChar == '/' || specialChar == '\\' {
				return true
			}

			tmpDir := t.TempDir()
			logDir := filepath.Join(tmpDir, "logs"+string(specialChar))

			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[log]
dir = "` + logDir + `"
level = "info"
standardOutput = true
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			cfg, err := loadConfiguration()
			if err != nil {
				return true
			}

			// Should handle special characters without crashing
			logger, err := initializeLogger(cfg)

			if err != nil {
				return logger == nil
			}

			if logger != nil {
				logger.Close()
			}
			return logger != nil
		},
		gen.Rune(),
	))

	// Property 2.7: Logger initialization handles absolute and relative paths
	properties.Property("logger initialization handles absolute and relative paths", prop.ForAll(
		func(useAbsolute bool, pathDepth int) bool {
			clearEnvVars()
			defer clearEnvVars()

			tmpDir := t.TempDir()

			// Constrain path depth to reasonable range
			if pathDepth < 0 {
				pathDepth = -pathDepth
			}
			if pathDepth > 10 {
				pathDepth = pathDepth % 10
			}

			var logDir string
			if useAbsolute {
				// Absolute path
				logDir = tmpDir
				for i := 0; i < pathDepth; i++ {
					logDir = filepath.Join(logDir, "level"+string(rune('0'+i)))
				}
			} else {
				// Relative path
				logDir = "logs"
				for i := 0; i < pathDepth; i++ {
					logDir = filepath.Join(logDir, "level"+string(rune('0'+i)))
				}
			}

			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[log]
dir = "` + logDir + `"
level = "info"
standardOutput = true
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			cfg, err := loadConfiguration()
			if err != nil {
				return true
			}

			// Should handle both absolute and relative paths
			logger, err := initializeLogger(cfg)

			if err != nil {
				return logger == nil
			}

			if logger != nil {
				logger.Close()
			}
			return logger != nil
		},
		gen.Bool(),
		gen.Int(),
	))

	// Property 2.8: Logger initialization with environment variable overrides
	properties.Property("logger initialization with environment variable overrides", prop.ForAll(
		func(logLevel string, useEnvOverride bool) bool {
			clearEnvVars()
			defer clearEnvVars()

			tmpDir := t.TempDir()
			logDir := filepath.Join(tmpDir, "logs")

			// Create base config
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[log]
dir = "` + logDir + `"
level = "info"
standardOutput = true
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				t.Logf("Failed to create config file: %v", err)
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Optionally override with environment variable
			if useEnvOverride && logLevel != "" {
				os.Setenv("LOG_LEVEL", logLevel)
				defer os.Unsetenv("LOG_LEVEL")
			}

			cfg, err := loadConfiguration()
			if err != nil {
				return true
			}

			// Should handle environment overrides
			logger, err := initializeLogger(cfg)

			if err != nil {
				return logger == nil
			}

			if logger != nil {
				logger.Close()
			}
			return logger != nil
		},
		gen.OneConstOf("debug", "info", "warn", "error", "invalid", ""),
		gen.Bool(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Helper function to convert bool to string for TOML
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
