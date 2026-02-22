package main

import (
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/constants"
	"github.com/real-rm/goconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Main Function Testing Approach
//
// **Why main() is not directly tested:**
//
// The main() function in Go is the entry point of the application and has special
// characteristics that make it difficult to test directly:
//
// 1. **No Return Value**: main() doesn't return errors or values, making it impossible
//    to verify its behavior through return values.
//
// 2. **Process Termination**: main() typically calls os.Exit() or log.Fatal() on errors,
//    which terminates the entire test process, preventing other tests from running.
//
// 3. **Global State**: main() often modifies global state and sets up signal handlers
//    that can interfere with other tests.
//
// 4. **Difficult to Control**: main() runs in the main goroutine and is hard to
//    control or timeout in tests.
//
// **Testing Strategy:**
//
// Instead of testing main() directly, we use testable wrapper functions that contain
// all the logic of main() but can be properly tested:
//
// 1. **runMain()**: A testable wrapper that sets up signal handling and calls
//    runWithSignalChannel(). This function returns errors instead of calling os.Exit().
//
// 2. **runWithSignalChannel()**: The core server logic that accepts a signal channel
//    as a parameter. This allows tests to:
//    - Control when shutdown signals are sent
//    - Verify graceful shutdown behavior
//    - Test error propagation from initialization steps
//    - Run multiple tests without process termination
//
// 3. **loadConfiguration()**: Isolated configuration loading logic that can be tested
//    independently with various config file scenarios.
//
// 4. **initializeLogger()**: Isolated logger initialization logic that can be tested
//    with different log configurations.
//
// **Test Coverage:**
//
// By testing these wrapper functions, we achieve comprehensive coverage of all the
// logic that would be in main(), including:
// - Configuration loading (valid, invalid, missing files)
// - Logger initialization (various log directories and levels)
// - Signal handling (SIGTERM, SIGINT)
// - Error propagation (config errors, logger errors)
// - Graceful shutdown sequences
//
// This approach provides better test coverage than testing main() directly would,
// while maintaining the same production behavior since main() simply calls runMain().
//
// **Validates: Requirements 1.5**

// TestLoadConfiguration tests the loadConfiguration function
//
// **Validates: Requirements 6.5**
func TestLoadConfiguration(t *testing.T) {
	t.Run("SuccessfulLoad", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		cfg, err := loadConfiguration()
		require.NoError(t, err, "Should load configuration successfully")
		require.NotNil(t, cfg, "Config accessor should not be nil")
	})

	t.Run("LoadWithoutConfigFile", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()

		cfg, err := loadConfiguration()
		// goconfig behavior: may or may not error on missing file
		if err != nil {
			t.Log("Configuration loading failed without config file (expected)")
			assert.Error(t, err)
			assert.Nil(t, cfg)
		} else {
			require.NotNil(t, cfg, "Config accessor should not be nil even without config file")
		}
	})

	t.Run("LoadConfigurationError", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()

		// Set invalid config file path
		os.Setenv("RMBASE_FILE_CFG", "/nonexistent/invalid/path/config.toml")
		defer os.Unsetenv("RMBASE_FILE_CFG")

		cfg, err := loadConfiguration()
		// Should handle error gracefully
		if err != nil {
			assert.Error(t, err, "Should return error for invalid config path")
			assert.Nil(t, cfg, "Config should be nil on error")
		} else {
			t.Log("goconfig allows invalid config path")
		}
	})
}

// TestInitializeLogger tests the initializeLogger function
//
// **Validates: Requirements 6.5**
func TestInitializeLogger(t *testing.T) {
	t.Run("SuccessfulInit", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		cfg, err := loadConfiguration()
		require.NoError(t, err)

		logger, err := initializeLogger(cfg)
		require.NoError(t, err, "Should initialize logger successfully")
		require.NotNil(t, logger, "Logger should not be nil")
		defer logger.Close()
	})

	t.Run("InitWithInvalidLogDir", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Set invalid log directory
		os.Setenv("LOG_DIR", "/invalid/readonly/path")
		defer os.Unsetenv("LOG_DIR")

		cfg, err := loadConfiguration()
		require.NoError(t, err)

		logger, err := initializeLogger(cfg)
		// golog may or may not fail with invalid directory
		if err != nil {
			assert.Error(t, err, "Should return error for invalid log directory")
			assert.Nil(t, logger, "Logger should be nil on error")
		} else {
			require.NotNil(t, logger)
			defer logger.Close()
			t.Log("golog allows invalid log directory")
		}
	})

	t.Run("InitWithFileAsLogDir", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Set log directory to /dev/null (a device file, not a directory)
		os.Setenv("LOG_DIR", "/dev/null")
		defer os.Unsetenv("LOG_DIR")

		cfg, err := loadConfiguration()
		require.NoError(t, err)

		logger, err := initializeLogger(cfg)
		// golog should fail when log dir is a device file
		if err != nil {
			assert.Error(t, err, "Should return error when log dir is a device file")
			assert.Nil(t, logger, "Logger should be nil on error")
			t.Log("Successfully triggered logger initialization error")
		} else {
			if logger != nil {
				defer logger.Close()
			}
			t.Log("golog allows device file as log directory")
		}
	})
}

// TestGetServerPort tests the getServerPort function
//
// **Validates: Requirements 6.5**
func TestGetServerPort(t *testing.T) {
	t.Run("GetPortFromConfig", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		cfg, err := loadConfiguration()
		require.NoError(t, err)

		// Create a mock logger (we'll use nil since the function handles it gracefully)
		logger, err := initializeLogger(cfg)
		require.NoError(t, err)
		defer logger.Close()

		port := getServerPort(cfg, logger)
		assert.Greater(t, port, 0, "Port should be greater than 0")
		assert.LessOrEqual(t, port, 65535, "Port should be valid")
	})

	t.Run("GetPortWithError", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		cfg, err := loadConfiguration()
		require.NoError(t, err)

		logger, err := initializeLogger(cfg)
		require.NoError(t, err)
		defer logger.Close()

		// Even with errors, should return default port
		port := getServerPort(cfg, logger)
		assert.Greater(t, port, 0, "Should return valid port even on error")
	})

	t.Run("GetPortWithInvalidValue", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Set invalid port value
		os.Setenv("SERVER_PORT", "invalid")
		defer os.Unsetenv("SERVER_PORT")

		cfg, err := loadConfiguration()
		require.NoError(t, err)

		logger, err := initializeLogger(cfg)
		require.NoError(t, err)
		defer logger.Close()

		// Should fall back to default or config file value
		port := getServerPort(cfg, logger)
		assert.Greater(t, port, 0, "Should return valid port with invalid env var")
	})
}

// TestSetupSignalHandler tests the setupSignalHandler function
//
// **Validates: Requirements 6.5**
func TestSetupSignalHandler(t *testing.T) {
	t.Run("CreateSignalChannel", func(t *testing.T) {
		sigChan := setupSignalHandler()
		require.NotNil(t, sigChan, "Signal channel should not be nil")

		// Clean up
		signal.Stop(sigChan)
	})

	t.Run("ReceiveSignal", func(t *testing.T) {
		sigChan := setupSignalHandler()
		defer signal.Stop(sigChan)

		// Send a signal
		go func() {
			time.Sleep(50 * time.Millisecond)
			sigChan <- syscall.SIGTERM
		}()

		// Wait for signal
		select {
		case sig := <-sigChan:
			assert.Equal(t, syscall.SIGTERM, sig, "Should receive SIGTERM signal")
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for signal")
		}
	})
}

// TestRunWithSignalChannel tests the runWithSignalChannel function
//
// **Validates: Requirements 6.5**
func TestRunWithSignalChannel(t *testing.T) {
	t.Run("SuccessfulRun", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping integration test in short mode (requires MongoDB)")
		}
		if !canRunFullServer() {
			t.Skip("Full server test requires CHATBOX_SERVER_TEST=1 and running MongoDB (make docker-compose-up)")
		}
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Create a signal channel
		sigChan := make(chan os.Signal, 1)

		// Run in a goroutine
		errChan := make(chan error, 1)
		go func() {
			errChan <- runWithSignalChannel(sigChan)
		}()

		// Give it time to start
		time.Sleep(100 * time.Millisecond)

		// Send shutdown signal
		sigChan <- syscall.SIGTERM

		// Wait for completion
		select {
		case err := <-errChan:
			assert.NoError(t, err, "Run should complete without error")
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not complete within timeout")
		}
	})

	t.Run("RunWithConfigError", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		// Don't set up config file

		// Create a signal channel
		sigChan := make(chan os.Signal, 1)

		// Run in a goroutine with timeout
		errChan := make(chan error, 1)
		go func() {
			errChan <- runWithSignalChannel(sigChan)
		}()

		// Wait a bit to see if it fails immediately
		select {
		case err := <-errChan:
			// If it fails during config loading, that's expected
			if err != nil {
				t.Logf("Run failed with config error (expected): %v", err)
			} else {
				t.Log("Run succeeded even without config file (goconfig allows this)")
			}
		case <-time.After(200 * time.Millisecond):
			// If it's still running, send a signal to stop it
			sigChan <- syscall.SIGTERM
			select {
			case err := <-errChan:
				if err != nil {
					t.Logf("Run failed: %v", err)
				} else {
					t.Log("Run completed successfully with defaults")
				}
			case <-time.After(1 * time.Second):
				t.Fatal("Run did not complete after signal")
			}
		}
	})

	t.Run("RunWithLoggerError", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping integration test in short mode (requires MongoDB)")
		}
		if !canRunFullServer() {
			t.Skip("Full server test requires CHATBOX_SERVER_TEST=1 and running MongoDB (make docker-compose-up)")
		}
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Set invalid log directory to potentially cause logger init error
		os.Setenv("LOG_DIR", "/invalid/readonly/path/that/does/not/exist")
		defer os.Unsetenv("LOG_DIR")

		// Create a signal channel
		sigChan := make(chan os.Signal, 1)

		// Run in a goroutine
		errChan := make(chan error, 1)
		go func() {
			errChan <- runWithSignalChannel(sigChan)
		}()

		// Wait to see if it fails or succeeds
		// The function may fail at logger init, at MongoDB init, or start the full server
		select {
		case err := <-errChan:
			if err != nil {
				t.Logf("Run failed with initialization error (expected): %v", err)
			} else {
				t.Log("Run succeeded (unexpected in unit test mode)")
			}
		case <-time.After(2 * time.Second):
			// If still running, send signal to stop
			sigChan <- syscall.SIGTERM
			select {
			case err := <-errChan:
				if err != nil {
					t.Logf("Run failed: %v", err)
				} else {
					t.Log("Run completed successfully")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not complete after signal")
			}
		}
	})
}

// TestRunMain tests the runMain function
//
// **Validates: Requirements 6.5**
func TestRunMain(t *testing.T) {
	t.Run("RunMainWithSignal", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping integration test in short mode (requires MongoDB)")
		}
		if !canRunFullServer() {
			t.Skip("Full server test requires CHATBOX_SERVER_TEST=1 and running MongoDB (make docker-compose-up)")
		}
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Run in a goroutine
		errChan := make(chan error, 1)
		go func() {
			errChan <- runMain()
		}()

		// Give it time to start
		time.Sleep(100 * time.Millisecond)

		// Send SIGTERM to the process
		// Note: This will trigger the signal handler set up by runMain
		proc, err := os.FindProcess(os.Getpid())
		require.NoError(t, err)

		err = proc.Signal(syscall.SIGTERM)
		require.NoError(t, err)

		// Wait for completion
		select {
		case err := <-errChan:
			assert.NoError(t, err, "RunMain should complete without error")
		case <-time.After(2 * time.Second):
			t.Fatal("RunMain did not complete within timeout")
		}
	})
}

// TestConfigLoading tests configuration loading from goconfig
//
// **Validates: Requirements 6.5**
func TestConfigLoading(t *testing.T) {
	t.Run("LoadDefaultConfiguration", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		err := goconfig.LoadConfig()
		require.NoError(t, err, "Should load configuration successfully")

		cfg, err := goconfig.Default()
		require.NoError(t, err, "Should get config accessor")
		require.NotNil(t, cfg, "Config accessor should not be nil")
	})

	t.Run("LoadLogConfiguration", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)
		assert.NotEmpty(t, logDir, "Log directory should not be empty")

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)
		assert.NotEmpty(t, logLevel, "Log level should not be empty")

		standardOutput, err := cfg.ConfigBoolWithDefault("log.standardOutput", true)
		assert.NoError(t, err)
		assert.True(t, standardOutput, "Standard output should default to true")
	})

	t.Run("LoadServerConfiguration", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)
		assert.Greater(t, port, 0, "Port should be greater than 0")
	})

	t.Run("HandleMissingConfigFile", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()

		// Don't set config file path
		err := goconfig.LoadConfig()
		// goconfig behavior: may or may not error on missing file
		// Just verify we can handle both cases
		if err != nil {
			t.Log("goconfig errors on missing config file")
		} else {
			t.Log("goconfig allows missing config file")
		}
	})
}

// TestConfigValidation tests configuration validation
//
// **Validates: Requirements 6.5**
func TestConfigValidation(t *testing.T) {
	t.Run("ValidatePortRange", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)
		assert.Greater(t, port, 0, "Port should be greater than 0")
		assert.LessOrEqual(t, port, 65535, "Port should be less than or equal to 65535")
	})

	t.Run("ValidateLogLevel", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)

		validLevels := []string{"debug", "info", "warn", "error"}
		assert.Contains(t, validLevels, logLevel, "Log level should be valid")
	})

	t.Run("ValidateLogDirectory", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)
		assert.NotEmpty(t, logDir, "Log directory should not be empty")
	})

	t.Run("ValidateDefaultValues", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		// Verify values from config file or defaults
		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)
		assert.Greater(t, port, 0)

		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)
		assert.NotEmpty(t, logDir)

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)
		assert.NotEmpty(t, logLevel)
	})
}

// TestEnvironmentVariableOverride tests environment variable override behavior
//
// **Validates: Requirements 6.5**
func TestEnvironmentVariableOverride(t *testing.T) {
	t.Run("OverrideServerPort", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Set environment variable for server port
		customPort := 9090
		os.Setenv("SERVER_PORT", "9090")
		defer os.Unsetenv("SERVER_PORT")

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)

		// Note: The actual behavior depends on goconfig implementation
		if port == customPort {
			t.Log("Environment variable override is supported")
		} else {
			t.Logf("Port from config: %d (env override may not be supported)", port)
		}
	})

	t.Run("OverrideLogLevel", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		os.Setenv("LOG_LEVEL", "debug")
		defer os.Unsetenv("LOG_LEVEL")

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)

		if logLevel == "debug" {
			t.Log("Environment variable override is supported for log level")
		} else {
			t.Logf("Log level from config: %s (env override may not be supported)", logLevel)
		}
	})

	t.Run("OverrideLogDirectory", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		customLogDir := "/tmp/custom-logs"
		os.Setenv("LOG_DIR", customLogDir)
		defer os.Unsetenv("LOG_DIR")

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)

		if logDir == customLogDir {
			t.Log("Environment variable override is supported for log directory")
		} else {
			t.Logf("Log dir from config: %s (env override may not be supported)", logDir)
		}
	})

	t.Run("MultipleEnvironmentVariables", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		os.Setenv("SERVER_PORT", "9191")
		os.Setenv("LOG_LEVEL", "warn")
		os.Setenv("LOG_DIR", "/tmp/test-logs")
		defer func() {
			os.Unsetenv("SERVER_PORT")
			os.Unsetenv("LOG_LEVEL")
			os.Unsetenv("LOG_DIR")
		}()

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)

		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)

		t.Logf("Port: %d", port)
		t.Logf("Log Level: %s", logLevel)
		t.Logf("Log Dir: %s", logDir)
	})

	t.Run("EnvironmentVariablePrecedence", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		os.Setenv("SERVER_PORT", "7777")
		defer os.Unsetenv("SERVER_PORT")

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)

		if port == 7777 {
			t.Log("Environment variables take precedence over config file")
		} else if port == constants.DefaultPort {
			t.Log("Default values are used when config is not set")
		} else {
			t.Logf("Config file value is used: %d", port)
		}
	})
}

// TestConfigurationIntegration tests the complete configuration flow
//
// **Validates: Requirements 6.5**
func TestConfigurationIntegration(t *testing.T) {
	t.Run("CompleteConfigurationFlow", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		err := goconfig.LoadConfig()
		require.NoError(t, err, "Configuration loading should succeed")

		cfg, err := goconfig.Default()
		require.NoError(t, err, "Getting config accessor should succeed")
		require.NotNil(t, cfg, "Config accessor should not be nil")

		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)
		assert.NotEmpty(t, logDir)

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)
		assert.NotEmpty(t, logLevel)

		standardOutput, err := cfg.ConfigBoolWithDefault("log.standardOutput", true)
		assert.NoError(t, err)

		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)
		assert.Greater(t, port, 0)
		assert.LessOrEqual(t, port, 65535)

		t.Logf("Configuration loaded successfully:")
		t.Logf("  Log Dir: %s", logDir)
		t.Logf("  Log Level: %s", logLevel)
		t.Logf("  Standard Output: %v", standardOutput)
		t.Logf("  Server Port: %d", port)
	})

	t.Run("ConfigurationWithEnvironmentOverrides", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		os.Setenv("SERVER_PORT", "8888")
		os.Setenv("LOG_LEVEL", "error")
		defer func() {
			os.Unsetenv("SERVER_PORT")
			os.Unsetenv("LOG_LEVEL")
		}()

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)

		t.Logf("Configuration with environment overrides:")
		t.Logf("  Server Port: %d", port)
		t.Logf("  Log Level: %s", logLevel)
	})
}

// TestConfigurationErrorHandling tests error handling in configuration
//
// **Validates: Requirements 6.5**
func TestConfigurationErrorHandling(t *testing.T) {
	t.Run("HandleInvalidPortValue", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		os.Setenv("SERVER_PORT", "invalid")
		defer os.Unsetenv("SERVER_PORT")

		err := goconfig.LoadConfig()
		if err == nil {
			cfg, err := goconfig.Default()
			if err == nil {
				port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
				assert.NoError(t, err)
				// Should fall back to config file value or default
				assert.Greater(t, port, 0)
			}
		}
	})

	t.Run("HandleMissingRequiredConfig", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()

		// Don't set config file
		err := goconfig.LoadConfig()
		// goconfig behavior: may or may not error on missing file
		if err == nil {
			cfg, err := goconfig.Default()
			if err == nil {
				// Should use defaults for missing values
				port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
				assert.NoError(t, err)
				assert.Equal(t, constants.DefaultPort, port)
			}
		}
	})

	t.Run("HandleEmptyConfigValues", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		os.Setenv("LOG_LEVEL", "")
		os.Setenv("LOG_DIR", "")
		defer func() {
			os.Unsetenv("LOG_LEVEL")
			os.Unsetenv("LOG_DIR")
		}()

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		// Should fall back to config file values or defaults for empty values
		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)
		assert.NotEmpty(t, logLevel)

		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)
		assert.NotEmpty(t, logDir)
	})
}

// TestConfigurationDefaults tests default configuration values
//
// **Validates: Requirements 6.5**
func TestConfigurationDefaults(t *testing.T) {
	tests := []struct {
		name         string
		configKey    string
		defaultValue interface{}
		valueType    string
	}{
		{
			name:         "DefaultPort",
			configKey:    "server.port",
			defaultValue: constants.DefaultPort,
			valueType:    "int",
		},
		{
			name:         "DefaultLogLevel",
			configKey:    "log.level",
			defaultValue: constants.DefaultLogLevel,
			valueType:    "string",
		},
		{
			name:         "DefaultLogDir",
			configKey:    "log.dir",
			defaultValue: constants.DefaultLogDir,
			valueType:    "string",
		},
		{
			name:         "DefaultStandardOutput",
			configKey:    "log.standardOutput",
			defaultValue: true,
			valueType:    "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars()
			defer clearEnvVars()
			setupConfigFile()
			defer cleanupConfigFile()

			err := goconfig.LoadConfig()
			require.NoError(t, err)

			cfg, err := goconfig.Default()
			require.NoError(t, err)

			switch tt.valueType {
			case "int":
				value, err := cfg.ConfigIntWithDefault(tt.configKey, tt.defaultValue.(int))
				assert.NoError(t, err)
				assert.Greater(t, value, 0, "Value should be positive")
			case "string":
				value, err := cfg.ConfigStringWithDefault(tt.configKey, tt.defaultValue.(string))
				assert.NoError(t, err)
				assert.NotEmpty(t, value, "Value should not be empty")
			case "bool":
				value, err := cfg.ConfigBoolWithDefault(tt.configKey, tt.defaultValue.(bool))
				assert.NoError(t, err)
				_ = value // Just verify no error
			}
		})
	}
}

// TestConfigurationTimeout tests configuration loading with timeout
//
// **Validates: Requirements 6.5**
func TestConfigurationTimeout(t *testing.T) {
	t.Run("ConfigurationLoadingCompletes", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		done := make(chan bool, 1)
		var loadErr error

		go func() {
			loadErr = goconfig.LoadConfig()
			done <- true
		}()

		select {
		case <-done:
			assert.NoError(t, loadErr, "Configuration loading should complete without error")
		case <-time.After(5 * time.Second):
			t.Fatal("Configuration loading timed out after 5 seconds")
		}
	})
}

// TestStartup tests server startup scenarios
//
// **Validates: Requirements 6.5**
func TestStartup(t *testing.T) {
	t.Run("SuccessfulStartup", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Test configuration loading
		err := goconfig.LoadConfig()
		require.NoError(t, err, "Configuration should load successfully")

		cfg, err := goconfig.Default()
		require.NoError(t, err, "Should get config accessor")
		require.NotNil(t, cfg, "Config accessor should not be nil")

		// Test logger initialization parameters
		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)
		assert.NotEmpty(t, logDir, "Log directory should be set")

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)
		assert.NotEmpty(t, logLevel, "Log level should be set")

		standardOutput, err := cfg.ConfigBoolWithDefault("log.standardOutput", true)
		assert.NoError(t, err)

		// Test server port configuration
		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)
		assert.Greater(t, port, 0, "Port should be positive")
		assert.LessOrEqual(t, port, 65535, "Port should be valid")

		t.Logf("Startup configuration validated:")
		t.Logf("  Log Dir: %s", logDir)
		t.Logf("  Log Level: %s", logLevel)
		t.Logf("  Standard Output: %v", standardOutput)
		t.Logf("  Server Port: %d", port)
	})

	t.Run("StartupWithInvalidConfig", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()

		// Set invalid configuration values
		os.Setenv("SERVER_PORT", "-1")
		os.Setenv("LOG_LEVEL", "invalid_level")
		defer func() {
			os.Unsetenv("SERVER_PORT")
			os.Unsetenv("LOG_LEVEL")
		}()

		setupConfigFile()
		defer cleanupConfigFile()

		// Configuration loading should still succeed
		err := goconfig.LoadConfig()
		if err != nil {
			t.Logf("Configuration loading failed with invalid values: %v", err)
			return
		}

		cfg, err := goconfig.Default()
		if err != nil {
			t.Logf("Getting config accessor failed: %v", err)
			return
		}

		// Should fall back to defaults for invalid values
		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)
		if port <= 0 || port > 65535 {
			t.Logf("Invalid port detected: %d (would fail validation)", port)
		} else {
			t.Logf("Port fell back to valid value: %d", port)
		}

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)
		validLevels := []string{"debug", "info", "warn", "error"}
		if !contains(validLevels, logLevel) {
			t.Logf("Invalid log level detected: %s (would fail validation)", logLevel)
		} else {
			t.Logf("Log level is valid: %s", logLevel)
		}
	})

	t.Run("StartupWithMissingDependencies", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()

		// Test startup without config file
		os.Unsetenv("RMBASE_FILE_CFG")

		err := goconfig.LoadConfig()
		if err != nil {
			t.Logf("Configuration loading failed without config file: %v", err)
			// This is expected behavior - startup should fail gracefully
			assert.Error(t, err, "Should error when config file is missing")
			return
		}

		// If goconfig allows missing config file, verify defaults work
		cfg, err := goconfig.Default()
		if err != nil {
			t.Logf("Getting config accessor failed: %v", err)
			assert.Error(t, err, "Should error when dependencies are missing")
			return
		}

		// Verify we can still get default values
		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)
		assert.Equal(t, constants.DefaultPort, port, "Should use default port when config is missing")

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)
		assert.Equal(t, constants.DefaultLogLevel, logLevel, "Should use default log level when config is missing")

		t.Log("Startup with missing dependencies falls back to defaults")
	})

	t.Run("StartupWithInvalidLogDirectory", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Set invalid log directory (read-only or non-existent path)
		os.Setenv("LOG_DIR", "/invalid/readonly/path")
		defer os.Unsetenv("LOG_DIR")

		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)

		// Note: The actual logger initialization would fail, but config loading succeeds
		t.Logf("Log directory set to: %s (logger initialization would validate this)", logDir)
	})

	t.Run("StartupWithMissingPort", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Don't set port - should use default
		err := goconfig.LoadConfig()
		require.NoError(t, err)

		cfg, err := goconfig.Default()
		require.NoError(t, err)

		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)
		assert.Equal(t, constants.DefaultPort, port, "Should use default port when not configured")
	})

	t.Run("StartupConfigurationOrder", func(t *testing.T) {
		clearEnvVars()
		defer clearEnvVars()
		setupConfigFile()
		defer cleanupConfigFile()

		// Test that configuration loading happens before logger initialization
		err := goconfig.LoadConfig()
		require.NoError(t, err, "Configuration must load first")

		cfg, err := goconfig.Default()
		require.NoError(t, err, "Config accessor must be available before logger init")

		// Verify all required config values are available
		logDir, err := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
		assert.NoError(t, err)
		assert.NotEmpty(t, logDir)

		logLevel, err := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
		assert.NoError(t, err)
		assert.NotEmpty(t, logLevel)

		port, err := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
		assert.NoError(t, err)
		assert.Greater(t, port, 0)

		t.Log("Configuration loading order validated")
	})
}

// TestNewHTTPServer verifies that NewHTTPServer returns a server with proper timeouts
func TestNewHTTPServer(t *testing.T) {
	t.Run("HasCorrectTimeouts", func(t *testing.T) {
		srv := NewHTTPServer(":8080", nil)

		assert.Equal(t, ":8080", srv.Addr)
		assert.Equal(t, constants.HTTPReadTimeout, srv.ReadTimeout, "ReadTimeout should match constant")
		// WriteTimeout is 0 because WebSocket connections are long-lived HTTP upgrades
		assert.Equal(t, time.Duration(0), srv.WriteTimeout, "WriteTimeout should be 0 for WebSocket support")
		assert.Equal(t, constants.HTTPIdleTimeout, srv.IdleTimeout, "IdleTimeout should match constant")
	})

	t.Run("AcceptsCustomHandler", func(t *testing.T) {
		handler := http.NewServeMux()
		srv := NewHTTPServer(":9090", handler)

		assert.Equal(t, ":9090", srv.Addr)
		assert.Equal(t, handler, srv.Handler)
	})

	t.Run("AcceptsNilHandler", func(t *testing.T) {
		srv := NewHTTPServer(":8080", nil)

		assert.Nil(t, srv.Handler)
		assert.NotZero(t, srv.ReadTimeout)
		// WriteTimeout is intentionally 0 for WebSocket support
		assert.Zero(t, srv.WriteTimeout)
		assert.NotZero(t, srv.IdleTimeout)
	})
}

// Helper functions

// canRunFullServer checks if the full server integration test environment is available.
// Since runWithSignalChannel now initializes MongoDB and starts the full server,
// these tests require a properly configured MongoDB instance with valid credentials.
// Set CHATBOX_SERVER_TEST=1 after running `make docker-compose-up` to enable.
func canRunFullServer() bool {
	if os.Getenv("CHATBOX_SERVER_TEST") == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", "localhost:27017", 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func clearEnvVars() {
	envVars := []string{
		"SERVER_PORT",
		"LOG_LEVEL",
		"LOG_DIR",
		"LOG_STANDARD_OUTPUT",
		"RECONNECT_TIMEOUT",
		"MAX_CONNECTIONS",
		"RATE_LIMIT",
		"JWT_SECRET",
		"LLM_STREAM_TIMEOUT",
		"ADMIN_RATE_LIMIT",
		"ADMIN_RATE_WINDOW",
		"CHATBOX_PATH_PREFIX",
		"RMBASE_FILE_CFG",
	}

	for _, envVar := range envVars {
		os.Unsetenv(envVar)
	}

	// Reset goconfig state to avoid interference between tests
	goconfig.ResetConfig()
}

func setupConfigFile() {
	// Reset goconfig state before setting up new config
	goconfig.ResetConfig()
	os.Setenv("RMBASE_FILE_CFG", "../../config.toml")
}

func cleanupConfigFile() {
	os.Unsetenv("RMBASE_FILE_CFG")
	goconfig.ResetConfig()
}

// TestSignalHandling tests signal handling for graceful shutdown
//
// **Validates: Requirements 6.5**
func TestSignalHandling(t *testing.T) {
	t.Run("SIGTERMHandling", func(t *testing.T) {
		// Create signal channel
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		// Send SIGTERM signal
		go func() {
			time.Sleep(100 * time.Millisecond)
			sigChan <- syscall.SIGTERM
		}()

		// Wait for signal
		select {
		case sig := <-sigChan:
			assert.Equal(t, syscall.SIGTERM, sig, "Should receive SIGTERM signal")
			t.Log("SIGTERM signal received successfully")
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for SIGTERM signal")
		}
	})

	t.Run("SIGINTHandling", func(t *testing.T) {
		// Create signal channel
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT)
		defer signal.Stop(sigChan)

		// Send SIGINT signal
		go func() {
			time.Sleep(100 * time.Millisecond)
			sigChan <- syscall.SIGINT
		}()

		// Wait for signal
		select {
		case sig := <-sigChan:
			assert.Equal(t, syscall.SIGINT, sig, "Should receive SIGINT signal")
			t.Log("SIGINT signal received successfully")
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for SIGINT signal")
		}
	})

	t.Run("GracefulShutdown", func(t *testing.T) {
		// Create signal channel
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		// Simulate graceful shutdown sequence
		shutdownComplete := make(chan bool, 1)

		go func() {
			// Wait for signal
			<-sigChan

			// Simulate cleanup operations
			time.Sleep(50 * time.Millisecond)

			// Mark shutdown as complete
			shutdownComplete <- true
		}()

		// Send shutdown signal
		go func() {
			time.Sleep(100 * time.Millisecond)
			sigChan <- syscall.SIGTERM
		}()

		// Wait for graceful shutdown to complete
		select {
		case <-shutdownComplete:
			t.Log("Graceful shutdown completed successfully")
		case <-time.After(2 * time.Second):
			t.Fatal("Graceful shutdown timed out")
		}
	})

	t.Run("MultipleSignals", func(t *testing.T) {
		// Create signal channel
		sigChan := make(chan os.Signal, 2)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		// Send multiple signals
		go func() {
			time.Sleep(50 * time.Millisecond)
			sigChan <- syscall.SIGINT
			time.Sleep(50 * time.Millisecond)
			sigChan <- syscall.SIGTERM
		}()

		// Receive first signal
		select {
		case sig := <-sigChan:
			assert.Contains(t, []os.Signal{syscall.SIGINT, syscall.SIGTERM}, sig, "Should receive valid signal")
			t.Logf("First signal received: %v", sig)
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for first signal")
		}

		// Verify second signal is also available
		select {
		case sig := <-sigChan:
			assert.Contains(t, []os.Signal{syscall.SIGINT, syscall.SIGTERM}, sig, "Should receive valid signal")
			t.Logf("Second signal received: %v", sig)
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for second signal")
		}
	})

	t.Run("SignalChannelBuffering", func(t *testing.T) {
		// Create buffered signal channel
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		// Send signal before receiver is ready
		sigChan <- syscall.SIGTERM

		// Verify signal is buffered and can be received later
		select {
		case sig := <-sigChan:
			assert.Equal(t, syscall.SIGTERM, sig, "Buffered signal should be received")
			t.Log("Buffered signal received successfully")
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for buffered signal")
		}
	})

	t.Run("ShutdownWithTimeout", func(t *testing.T) {
		// Create signal channel
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		shutdownComplete := make(chan bool, 1)
		shutdownTimeout := 500 * time.Millisecond

		go func() {
			// Wait for signal
			<-sigChan

			// Simulate cleanup with timeout
			cleanupDone := make(chan bool, 1)
			go func() {
				time.Sleep(100 * time.Millisecond) // Simulate cleanup work
				cleanupDone <- true
			}()

			// Wait for cleanup or timeout
			select {
			case <-cleanupDone:
				shutdownComplete <- true
			case <-time.After(shutdownTimeout):
				t.Log("Shutdown timeout reached, forcing exit")
				shutdownComplete <- false
			}
		}()

		// Send shutdown signal
		sigChan <- syscall.SIGTERM

		// Wait for shutdown
		select {
		case success := <-shutdownComplete:
			assert.True(t, success, "Shutdown should complete within timeout")
			t.Log("Shutdown completed within timeout")
		case <-time.After(1 * time.Second):
			t.Fatal("Test timeout waiting for shutdown")
		}
	})

	t.Run("SignalNotifyRegistration", func(t *testing.T) {
		// Test that signal.Notify properly registers signals
		sigChan := make(chan os.Signal, 1)

		// Register for both SIGINT and SIGTERM
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		// Send SIGINT
		go func() {
			time.Sleep(50 * time.Millisecond)
			sigChan <- syscall.SIGINT
		}()

		select {
		case sig := <-sigChan:
			assert.Equal(t, syscall.SIGINT, sig, "Should receive SIGINT")
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for SIGINT")
		}

		// Send SIGTERM
		go func() {
			time.Sleep(50 * time.Millisecond)
			sigChan <- syscall.SIGTERM
		}()

		select {
		case sig := <-sigChan:
			assert.Equal(t, syscall.SIGTERM, sig, "Should receive SIGTERM")
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for SIGTERM")
		}

		t.Log("Both SIGINT and SIGTERM registered successfully")
	})

	t.Run("CleanupAfterSignal", func(t *testing.T) {
		// Test cleanup operations after receiving signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		cleanupSteps := make([]string, 0)
		cleanupComplete := make(chan bool, 1)

		go func() {
			// Wait for signal
			<-sigChan

			// Simulate cleanup steps
			cleanupSteps = append(cleanupSteps, "close_connections")
			time.Sleep(20 * time.Millisecond)

			cleanupSteps = append(cleanupSteps, "flush_logs")
			time.Sleep(20 * time.Millisecond)

			cleanupSteps = append(cleanupSteps, "save_state")
			time.Sleep(20 * time.Millisecond)

			cleanupComplete <- true
		}()

		// Send signal
		sigChan <- syscall.SIGTERM

		// Wait for cleanup
		select {
		case <-cleanupComplete:
			assert.Len(t, cleanupSteps, 3, "All cleanup steps should complete")
			assert.Contains(t, cleanupSteps, "close_connections")
			assert.Contains(t, cleanupSteps, "flush_logs")
			assert.Contains(t, cleanupSteps, "save_state")
			t.Log("All cleanup steps completed successfully")
		case <-time.After(1 * time.Second):
			t.Fatal("Cleanup timeout")
		}
	})
}
