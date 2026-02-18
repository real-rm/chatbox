package util

import (
	"fmt"

	"github.com/real-rm/golog"
)

// LogError logs an error with component and operation context.
// This helper standardizes error logging across the codebase.
//
// Parameters:
//   - logger: The logger instance to use
//   - component: The component where the error occurred (e.g., "http", "websocket", "router")
//   - operation: The operation that failed (e.g., "list sessions", "register connection")
//   - err: The error that occurred
//   - fields: Additional key-value pairs to include in the log
//
// Example:
//
//	LogError(logger, "http", "list sessions", err, "user_id", userID)
func LogError(logger *golog.Logger, component, operation string, err error, fields ...interface{}) {
	allFields := []interface{}{"error", err, "component", component}
	allFields = append(allFields, fields...)
	logger.Error(fmt.Sprintf("Failed to %s", operation), allFields...)
}
