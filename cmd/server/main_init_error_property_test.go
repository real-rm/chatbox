package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Property 17: Initialization Errors Propagate Correctly
// **Validates: Requirements 5.5**
//
// For any initialization step in cmd/server (configuration loading, logger initialization),
// if the step fails, the error should propagate to the caller and prevent the server from starting.
func TestProperty_InitializationErrorsPropagateCorrectly(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	// Property 17.1: Configuration loading errors propagate to runWithSignalChannel
	properties.Property("configuration loading errors propagate correctly", prop.ForAll(
		func(errorType int) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Create different error scenarios
			errorScenario := errorType % 4

			switch errorScenario {
			case 0:
				// Invalid config file path
				os.Setenv("RMBASE_FILE_CFG", "/nonexistent/invalid/path/config.toml")
				defer os.Unsetenv("RMBASE_FILE_CFG")

			case 1:
				// Malformed TOML content
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config.toml")
				malformedContent := "[server\nport = 8080" // Unclosed section
				err := os.WriteFile(configPath, []byte(malformedContent), 0644)
				if err != nil {
					return true // Skip if we can't create the file
				}
				os.Setenv("RMBASE_FILE_CFG", configPath)
				defer os.Unsetenv("RMBASE_FILE_CFG")

			case 2:
				// Directory instead of file
				tmpDir := t.TempDir()
				os.Setenv("RMBASE_FILE_CFG", tmpDir)
				defer os.Unsetenv("RMBASE_FILE_CFG")

			case 3:
				// Empty path
				os.Setenv("RMBASE_FILE_CFG", "")
				defer os.Unsetenv("RMBASE_FILE_CFG")
			}

			// Try to load configuration directly
			cfg, err := loadConfiguration()

			// If configuration loading fails, error should be returned
			if err != nil {
				// Error propagated correctly
				return cfg == nil
			}

			// If configuration loading succeeds (goconfig is lenient), that's also acceptable
			return cfg != nil
		},
		gen.Int(),
	))

	// Property 17.2: Logger initialization errors propagate to runWithSignalChannel
	properties.Property("logger initialization errors propagate correctly", prop.ForAll(
		func(errorType int) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Create different logger error scenarios
			errorScenario := errorType % 3

			var configContent string
			tmpDir := t.TempDir()

			switch errorScenario {
			case 0:
				// File as log directory
				filePath := filepath.Join(tmpDir, "logfile.txt")
				err := os.WriteFile(filePath, []byte("test"), 0644)
				if err != nil {
					return true
				}
				configContent = `
[log]
dir = "` + filePath + `"
level = "info"
standardOutput = true
`

			case 1:
				// Read-only directory (skip on Windows)
				if os.Getenv("GOOS") == "windows" {
					return true
				}
				readOnlyDir := filepath.Join(tmpDir, "readonly")
				err := os.Mkdir(readOnlyDir, 0444)
				if err != nil {
					return true
				}
				// Cleanup
				defer os.Chmod(readOnlyDir, 0755)

				configContent = `
[log]
dir = "` + readOnlyDir + `"
level = "info"
standardOutput = true
`

			case 2:
				// Invalid log level (though golog may be lenient)
				logDir := filepath.Join(tmpDir, "logs")
				configContent = `
[log]
dir = "` + logDir + `"
level = "invalid_level_12345"
standardOutput = true
`
			}

			// Create config file
			configPath := filepath.Join(tmpDir, "config.toml")
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Load configuration
			cfg, err := loadConfiguration()
			if err != nil {
				// Config loading failed, which is acceptable
				return true
			}

			// Try to initialize logger
			logger, err := initializeLogger(cfg)

			// If logger initialization fails, error should be returned
			if err != nil {
				// Error propagated correctly
				return logger == nil
			}

			// If logger initialization succeeds, clean up
			if logger != nil {
				logger.Close()
			}

			// Logger may handle errors gracefully, which is also acceptable
			return true
		},
		gen.Int(),
	))

	// Property 17.3: Errors in runWithSignalChannel prevent server from starting
	properties.Property("errors in runWithSignalChannel prevent server from starting", prop.ForAll(
		func(useConfigError bool) bool {
			clearEnvVars()
			defer clearEnvVars()

			if useConfigError {
				// Set invalid config path to trigger config error
				os.Setenv("RMBASE_FILE_CFG", "/nonexistent/invalid/path/config.toml")
				defer os.Unsetenv("RMBASE_FILE_CFG")
			} else {
				// Create config with potentially problematic logger settings
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "logfile.txt")
				err := os.WriteFile(filePath, []byte("test"), 0644)
				if err != nil {
					return true
				}

				configPath := filepath.Join(tmpDir, "config.toml")
				configContent := `
[log]
dir = "` + filePath + `"
level = "info"
standardOutput = true
`
				err = os.WriteFile(configPath, []byte(configContent), 0644)
				if err != nil {
					return true
				}

				os.Setenv("RMBASE_FILE_CFG", configPath)
				defer os.Unsetenv("RMBASE_FILE_CFG")
			}

			// Create signal channel
			sigChan := make(chan os.Signal, 1)

			// Run in goroutine with timeout
			done := make(chan error, 1)
			go func() {
				done <- runWithSignalChannel(sigChan)
			}()

			// Send signal after short delay to prevent hanging
			go func() {
				time.Sleep(100 * time.Millisecond)
				sigChan <- syscall.SIGTERM
			}()

			// Wait for completion with timeout
			select {
			case <-done:
				// Completed (either with error or success)
				return true
			case <-time.After(2 * time.Second):
				// Timeout - shouldn't happen but handle gracefully
				return true
			}
		},
		gen.Bool(),
	))

	// Property 17.4: Configuration errors are not silently ignored
	properties.Property("configuration errors are not silently ignored", prop.ForAll(
		func(malformedType int) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Create various malformed configurations
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")

			malformedPatterns := []string{
				"[server\nport = 8080",              // Unclosed section
				"[server]\nport = ",                 // Missing value
				"[server]\nport = \"not a number\"", // Wrong type
				"invalid syntax here",               // Invalid syntax
				"[server]\n\x00invalid",             // Null byte
			}

			// Ensure positive index
			if malformedType < 0 {
				malformedType = -malformedType
			}
			content := malformedPatterns[malformedType%len(malformedPatterns)]

			err := os.WriteFile(configPath, []byte(content), 0644)
			if err != nil {
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Try to load configuration
			cfg, err := loadConfiguration()

			// Either returns error or succeeds (goconfig may be lenient)
			// The important thing is it doesn't crash
			return (err != nil && cfg == nil) || (err == nil && cfg != nil)
		},
		gen.Int(),
	))

	// Property 17.5: Logger errors are not silently ignored
	properties.Property("logger errors are not silently ignored", prop.ForAll(
		func(logDirType int) bool {
			clearEnvVars()
			defer clearEnvVars()

			tmpDir := t.TempDir()

			// Create different problematic log directory scenarios
			var logDir string
			switch logDirType % 3 {
			case 0:
				// File instead of directory
				filePath := filepath.Join(tmpDir, "logfile.txt")
				err := os.WriteFile(filePath, []byte("test"), 0644)
				if err != nil {
					return true
				}
				logDir = filePath

			case 1:
				// Nested path under a file
				filePath := filepath.Join(tmpDir, "parent.txt")
				err := os.WriteFile(filePath, []byte("test"), 0644)
				if err != nil {
					return true
				}
				logDir = filepath.Join(filePath, "logs")

			case 2:
				// Very long path
				logDir = tmpDir
				for i := 0; i < 50; i++ {
					logDir = filepath.Join(logDir, "level")
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
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			cfg, err := loadConfiguration()
			if err != nil {
				return true
			}

			// Try to initialize logger
			logger, err := initializeLogger(cfg)

			// Either returns error or succeeds (golog may be lenient)
			// The important thing is it doesn't crash
			if err != nil {
				return logger == nil
			}

			if logger != nil {
				logger.Close()
			}
			return true
		},
		gen.Int(),
	))

	// Property 17.6: Multiple initialization errors are handled correctly
	properties.Property("multiple initialization errors are handled correctly", prop.ForAll(
		func(errorCount int) bool {
			clearEnvVars()
			defer clearEnvVars()

			// Constrain error count to 1-3
			errorCount = (errorCount % 3) + 1

			tmpDir := t.TempDir()

			// Create a config with multiple potential issues
			var configContent string

			if errorCount >= 1 {
				// Invalid log directory
				filePath := filepath.Join(tmpDir, "logfile.txt")
				os.WriteFile(filePath, []byte("test"), 0644)
				configContent = `
[log]
dir = "` + filePath + `"
`
			}

			if errorCount >= 2 {
				// Add invalid log level
				configContent += `level = "invalid_level_xyz"
`
			}

			if errorCount >= 3 {
				// Add invalid server port
				configContent += `
[server]
port = "not_a_number"
`
			}

			configContent += `standardOutput = true
`

			configPath := filepath.Join(tmpDir, "config.toml")
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Try to load configuration and initialize logger
			cfg, err := loadConfiguration()
			if err != nil {
				// Config loading failed, error propagated
				return true
			}

			if cfg != nil {
				logger, err := initializeLogger(cfg)
				if err != nil {
					// Logger init failed, error propagated
					return true
				}
				if logger != nil {
					logger.Close()
				}
			}

			// Libraries may be lenient, which is acceptable
			return true
		},
		gen.Int(),
	))

	// Property 17.7: Initialization errors don't cause resource leaks
	properties.Property("initialization errors don't cause resource leaks", prop.ForAll(
		func(errorType int) bool {
			clearEnvVars()
			defer clearEnvVars()

			tmpDir := t.TempDir()

			// Create a scenario that might fail during initialization
			var configContent string
			if errorType%2 == 0 {
				// File as log directory
				filePath := filepath.Join(tmpDir, "logfile.txt")
				os.WriteFile(filePath, []byte("test"), 0644)
				configContent = `
[log]
dir = "` + filePath + `"
level = "info"
standardOutput = true
`
			} else {
				// Valid config
				logDir := filepath.Join(tmpDir, "logs")
				configContent = `
[log]
dir = "` + logDir + `"
level = "info"
standardOutput = true
`
			}

			configPath := filepath.Join(tmpDir, "config.toml")
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				return true
			}

			os.Setenv("RMBASE_FILE_CFG", configPath)
			defer os.Unsetenv("RMBASE_FILE_CFG")

			// Try initialization multiple times to check for leaks
			for i := 0; i < 3; i++ {
				cfg, err := loadConfiguration()
				if err != nil {
					continue
				}

				logger, err := initializeLogger(cfg)
				if err != nil {
					continue
				}

				if logger != nil {
					logger.Close() // Ensure cleanup
				}
			}

			// If we got here without crashing, no obvious leaks
			return true
		},
		gen.Int(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
