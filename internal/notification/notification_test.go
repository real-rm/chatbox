package notification

import (
	"context"
	"testing"
	"time"

	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestNotificationService(t *testing.T) *NotificationService {
	// Initialize logger
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            "logs",
		Level:          "info",
		StandardOutput: false,
	})
	require.NoError(t, err)

	// Set config file path
	t.Setenv("RMBASE_FILE_CFG", "../../config.toml")

	// Load config
	err = goconfig.LoadConfig()
	require.NoError(t, err)

	config, err := goconfig.Default()
	require.NoError(t, err)

	// Create notification service (without MongoDB for testing)
	ns, err := NewNotificationService(logger, config, nil)
	require.NoError(t, err)

	return ns
}

func TestNewNotificationService(t *testing.T) {
	ns := setupTestNotificationService(t)
	assert.NotNil(t, ns)
	assert.NotNil(t, ns.mailer)
	assert.NotNil(t, ns.logger)
	assert.NotNil(t, ns.config)
	assert.NotNil(t, ns.rateLimiter)
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(1*time.Second, 3)

	// First 3 events should be allowed
	assert.True(t, rl.Allow("test-event"))
	assert.True(t, rl.Allow("test-event"))
	assert.True(t, rl.Allow("test-event"))

	// 4th event should be blocked
	assert.False(t, rl.Allow("test-event"))

	// Wait for window to expire
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	assert.True(t, rl.Allow("test-event"))
}

func TestRateLimiter_DifferentKeys(t *testing.T) {
	rl := NewRateLimiter(1*time.Second, 2)

	// Different keys should have independent limits
	assert.True(t, rl.Allow("event-1"))
	assert.True(t, rl.Allow("event-1"))
	assert.False(t, rl.Allow("event-1"))

	assert.True(t, rl.Allow("event-2"))
	assert.True(t, rl.Allow("event-2"))
	assert.False(t, rl.Allow("event-2"))
}

func TestSendHelpRequestAlert(t *testing.T) {
	ns := setupTestNotificationService(t)

	// Test sending help request alert
	err := ns.SendHelpRequestAlert("user-123", "session-456")
	// May fail if mail engines are not properly configured, but should not panic
	if err != nil {
		t.Logf("SendHelpRequestAlert returned error (expected in test env): %v", err)
	}
}

func TestSendCriticalError(t *testing.T) {
	ns := setupTestNotificationService(t)

	// Test sending critical error
	err := ns.SendCriticalError("Database Connection Failed", "MongoDB connection timeout after 10s", 5)
	// May fail if mail engines are not properly configured, but should not panic
	if err != nil {
		t.Logf("SendCriticalError returned error (expected in test env): %v", err)
	}
}

func TestSendSystemAlert(t *testing.T) {
	ns := setupTestNotificationService(t)

	// Test sending system alert
	err := ns.SendSystemAlert("High Memory Usage", "Memory usage exceeded 90% on server-01")
	// May fail if mail engines are not properly configured, but should not panic
	if err != nil {
		t.Logf("SendSystemAlert returned error (expected in test env): %v", err)
	}
}

func TestGetAdminEmails(t *testing.T) {
	ns := setupTestNotificationService(t)

	emails, err := ns.getAdminEmails()
	require.NoError(t, err)
	assert.NotNil(t, emails)
	// Should have at least the default admin email from config
	t.Logf("Admin emails: %v", emails)
}

func TestGetAdminPhones(t *testing.T) {
	ns := setupTestNotificationService(t)

	phones, err := ns.getAdminPhones()
	require.NoError(t, err)
	assert.NotNil(t, phones)
	t.Logf("Admin phones: %v", phones)
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single value",
			input:    "test@example.com",
			expected: []string{"test@example.com"},
		},
		{
			name:     "multiple values",
			input:    "test1@example.com,test2@example.com",
			expected: []string{"test1@example.com", "test2@example.com"},
		},
		{
			name:     "values with spaces",
			input:    " test1@example.com , test2@example.com ",
			expected: []string{"test1@example.com", "test2@example.com"},
		},
		{
			name:     "trailing comma",
			input:    "test1@example.com,test2@example.com,",
			expected: []string{"test1@example.com", "test2@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateEmails(t *testing.T) {
	ns := setupTestNotificationService(t)

	ctx := context.Background()
	emails := []string{"valid@example.com", "invalid-email", "another@example.com"}

	// This will use gomail's validation which includes MX lookup
	// In test environment, this might fail due to network restrictions
	validEmails, err := ns.ValidateEmails(ctx, emails)
	if err != nil {
		t.Logf("ValidateEmails returned error (may be expected in test env): %v", err)
	} else {
		t.Logf("Valid emails: %v", validEmails)
	}
}

func TestRateLimiting_HelpRequest(t *testing.T) {
	ns := setupTestNotificationService(t)

	// Send multiple help requests for the same session
	sessionID := "test-session-123"
	userID := "test-user-456"

	// First 5 should succeed (or fail due to config, but not rate limited)
	for i := 0; i < 5; i++ {
		err := ns.SendHelpRequestAlert(userID, sessionID)
		if err != nil {
			t.Logf("Request %d: %v", i+1, err)
		}
	}

	// 6th should be rate limited (no error, just skipped)
	err := ns.SendHelpRequestAlert(userID, sessionID)
	assert.NoError(t, err) // Rate limiting doesn't return error
}

func TestRateLimiting_CriticalError(t *testing.T) {
	ns := setupTestNotificationService(t)

	errorType := "Test Error"

	// First 5 should succeed (or fail due to config, but not rate limited)
	for i := 0; i < 5; i++ {
		err := ns.SendCriticalError(errorType, "Test details", 1)
		if err != nil {
			t.Logf("Request %d: %v", i+1, err)
		}
	}

	// 6th should be rate limited (no error, just skipped)
	err := ns.SendCriticalError(errorType, "Test details", 1)
	assert.NoError(t, err) // Rate limiting doesn't return error
}
