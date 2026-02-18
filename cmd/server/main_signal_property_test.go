package main

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/real-rm/goconfig"
)

// Mutex to serialize property tests that use global goconfig state
var goconfigMutex sync.Mutex

// Property 3: Signal Handling Triggers Shutdown
// **Validates: Requirements 1.4**
//
// For any valid shutdown signal (SIGTERM or SIGINT), the server should complete
// its shutdown sequence and return without hanging.
func TestProperty_SignalHandlingTriggersShutdown(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	parameters.Workers = 1 // Force sequential execution to avoid goconfig state conflicts
	properties := gopter.NewProperties(parameters)

	// Property 3.1: Server shuts down gracefully on SIGTERM or SIGINT
	properties.Property("server shuts down gracefully on shutdown signals", prop.ForAll(
		func(useSIGTERM bool) bool {
			goconfig.ResetConfig()
			clearEnvVars()
			defer func() {
				clearEnvVars()
				goconfig.ResetConfig()
			}()

			// Create a valid config file
			tmpDir := t.TempDir()
			logDir := filepath.Join(tmpDir, "logs")
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[server]
port = 8080

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

			// Create a signal channel
			sigChan := make(chan os.Signal, 1)

			// Choose signal based on input
			var signal os.Signal
			if useSIGTERM {
				signal = syscall.SIGTERM
			} else {
				signal = syscall.SIGINT
			}

			// Run server in a goroutine
			errChan := make(chan error, 1)
			go func() {
				errChan <- runWithSignalChannel(sigChan)
			}()

			// Give server time to start
			time.Sleep(100 * time.Millisecond)

			// Send shutdown signal
			sigChan <- signal

			// Wait for shutdown with timeout
			select {
			case err := <-errChan:
				// Server should shut down without error
				return err == nil
			case <-time.After(5 * time.Second):
				// Server hung - this is a failure
				t.Logf("Server did not shut down within timeout after receiving %v", signal)
				return false
			}
		},
		gen.Bool(),
	))

	// Property 3.2: Server shuts down within reasonable time
	properties.Property("server shuts down within reasonable time", prop.ForAll(
		func(delayMs int) bool {
			goconfig.ResetConfig()
			clearEnvVars()
			defer func() {
				clearEnvVars()
				goconfig.ResetConfig()
			}()

			// Constrain delay to reasonable range (0-500ms)
			if delayMs < 0 {
				delayMs = -delayMs
			}
			delayMs = delayMs % 500

			// Create a valid config file
			tmpDir := t.TempDir()
			logDir := filepath.Join(tmpDir, "logs")
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[server]
port = 8080

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

			sigChan := make(chan os.Signal, 1)

			errChan := make(chan error, 1)
			go func() {
				errChan <- runWithSignalChannel(sigChan)
			}()

			// Wait for variable delay before sending signal
			time.Sleep(time.Duration(delayMs) * time.Millisecond)

			// Send SIGTERM
			sigChan <- syscall.SIGTERM

			// Server should shut down within 3 seconds
			select {
			case err := <-errChan:
				return err == nil
			case <-time.After(3 * time.Second):
				t.Logf("Server did not shut down within 3 seconds")
				return false
			}
		},
		gen.Int(),
	))

	// Property 3.3: Multiple signals don't cause issues
	properties.Property("multiple signals are handled gracefully", prop.ForAll(
		func(signalCount int) bool {
			goconfig.ResetConfig()
			clearEnvVars()
			defer func() {
				clearEnvVars()
				goconfig.ResetConfig()
			}()

			// Constrain signal count to 1-5
			if signalCount < 1 {
				signalCount = 1
			}
			signalCount = (signalCount % 5) + 1

			// Create a valid config file
			tmpDir := t.TempDir()
			logDir := filepath.Join(tmpDir, "logs")
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[server]
port = 8080

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

			sigChan := make(chan os.Signal, signalCount)

			errChan := make(chan error, 1)
			go func() {
				errChan <- runWithSignalChannel(sigChan)
			}()

			// Give server time to start
			time.Sleep(50 * time.Millisecond)

			// Send multiple signals
			for i := 0; i < signalCount; i++ {
				sigChan <- syscall.SIGTERM
			}

			// Server should shut down after first signal
			select {
			case err := <-errChan:
				return err == nil
			case <-time.After(3 * time.Second):
				t.Logf("Server did not shut down after %d signals", signalCount)
				return false
			}
		},
		gen.Int(),
	))

	// Property 3.4: Signal handling works with different configurations
	properties.Property("signal handling works with different configurations", prop.ForAll(
		func(port int, logLevel string) bool {
			// Serialize access to goconfig to avoid parallel test interference
			goconfigMutex.Lock()
			defer goconfigMutex.Unlock()
			
			goconfig.ResetConfig()
			clearEnvVars()
			defer func() {
				clearEnvVars()
				goconfig.ResetConfig()
			}()

			// Constrain port to valid range
			if port < 1024 || port > 65535 {
				port = 8080
			}

			// Use valid log level
			validLogLevels := []string{"debug", "info", "warn", "error"}
			if logLevel == "" || !contains(validLogLevels, logLevel) {
				logLevel = "info"
			}

			// Create config with custom values
			tmpDir := t.TempDir()
			logDir := filepath.Join(tmpDir, "logs")
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[server]
port = ` + strconv.Itoa(port) + `

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

			sigChan := make(chan os.Signal, 1)

			errChan := make(chan error, 1)
			go func() {
				errChan <- runWithSignalChannel(sigChan)
			}()

			time.Sleep(100 * time.Millisecond)

			sigChan <- syscall.SIGTERM

			select {
			case err := <-errChan:
				return err == nil
			case <-time.After(3 * time.Second):
				return false
			}
		},
		gen.IntRange(1024, 65535),
		gen.OneConstOf("debug", "info", "warn", "error"),
	))

	// Property 3.5: Immediate signal after start is handled
	properties.Property("immediate signal after start is handled", prop.ForAll(
		func() bool {
			goconfig.ResetConfig()
			clearEnvVars()
			defer func() {
				clearEnvVars()
				goconfig.ResetConfig()
			}()

			// Create a valid config file
			tmpDir := t.TempDir()
			logDir := filepath.Join(tmpDir, "logs")
			configPath := filepath.Join(tmpDir, "config.toml")
			configContent := `
[server]
port = 8080

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

			sigChan := make(chan os.Signal, 1)

			// Send signal immediately (before server fully starts)
			sigChan <- syscall.SIGTERM

			errChan := make(chan error, 1)
			go func() {
				errChan <- runWithSignalChannel(sigChan)
			}()

			// Server should still shut down gracefully
			select {
			case err := <-errChan:
				return err == nil
			case <-time.After(3 * time.Second):
				return false
			}
		},
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
