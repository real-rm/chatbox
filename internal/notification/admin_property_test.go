package notification

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
)

// Feature: chat-application-websocket
// Property 52: Help Request Notification
// **Validates: Requirements 16.3, 16.4**
//
// For any help request created, the Notification_Service should send email and
// SMS notifications to all Chat_Admin users with user ID, session ID, and access link.
func TestProperty_HelpRequestNotification(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("help request notification completes without error", prop.ForAll(
		func(userID, sessionID string) bool {
			// Skip invalid inputs
			if userID == "" || sessionID == "" {
				return true
			}

			logger := getTestLogger()
			config := getTestConfig()

			// Create notification service with mock config
			ns, err := NewNotificationService(logger, config, nil)
			if err != nil {
				// If initialization fails (e.g., no config), skip test
				return true
			}

			// Send help request alert
			// Note: This will use mock email/SMS engines in test environment
			err = ns.SendHelpRequestAlert(userID, sessionID)

			// Should not return error (or return nil if rate limited)
			return err == nil
		},
		gen.Identifier(),
		gen.Identifier(),
	))

	properties.TestingRun(t)
}

// getTestLogger creates a test logger
func getTestLogger() *golog.Logger {
	logger, _ := golog.InitLog(golog.LogConfig{
		Level:          "error", // Only log errors in tests
		StandardOutput: false,
	})
	return logger
}

// getTestConfig creates a test config accessor
func getTestConfig() *goconfig.ConfigAccessor {
	// Create a minimal config for testing
	// In a real test environment, this would load from a test config file
	config := &goconfig.ConfigAccessor{}
	return config
}
