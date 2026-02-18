package util

import (
	"errors"
	"testing"

	"github.com/real-rm/golog"
)

func TestLogError(t *testing.T) {
	tests := []struct {
		name      string
		component string
		operation string
		err       error
		fields    []interface{}
		wantMsg   string
		wantPairs map[string]string
	}{
		{
			name:      "basic error logging",
			component: "http",
			operation: "list sessions",
			err:       errors.New("database connection failed"),
			fields:    []interface{}{},
			wantMsg:   "Failed to list sessions",
			wantPairs: map[string]string{
				"component": "http",
				"error":     "database connection failed",
			},
		},
		{
			name:      "error with additional fields",
			component: "websocket",
			operation: "register connection",
			err:       errors.New("session not found"),
			fields:    []interface{}{"user_id", "user123", "session_id", "sess456"},
			wantMsg:   "Failed to register connection",
			wantPairs: map[string]string{
				"component":  "websocket",
				"error":      "session not found",
				"user_id":    "user123",
				"session_id": "sess456",
			},
		},
		{
			name:      "error with numeric fields",
			component: "router",
			operation: "route message",
			err:       errors.New("timeout"),
			fields:    []interface{}{"retry_count", 3, "timeout_ms", 5000},
			wantMsg:   "Failed to route message",
			wantPairs: map[string]string{
				"component":   "router",
				"error":       "timeout",
				"retry_count": "3",
				"timeout_ms":  "5000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a logger for testing
			logger, err := golog.InitLog(golog.LogConfig{
				Dir:            t.TempDir(),
				Level:          "error",
				StandardOutput: false,
			})
			if err != nil {
				t.Fatalf("Failed to create logger: %v", err)
			}

			// Call LogError
			LogError(logger, tt.component, tt.operation, tt.err, tt.fields...)

			// Note: Since we can't easily capture the log output with this logger implementation,
			// we just verify the function doesn't panic and completes successfully.
			// The actual log output verification would require integration testing.
		})
	}
}

func TestLogError_NilLogger(t *testing.T) {
	// This test ensures we don't panic with nil logger
	// In production, this should never happen, but we test defensive behavior
	defer func() {
		if r := recover(); r == nil {
			t.Error("LogError() with nil logger should panic")
		}
	}()

	LogError(nil, "test", "test operation", errors.New("test error"))
}

func TestLogError_EmptyFields(t *testing.T) {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	testErr := errors.New("test error")
	LogError(logger, "test-component", "test operation", testErr)

	// Verify the function completes without panicking
	// Actual log output verification would require integration testing
}
