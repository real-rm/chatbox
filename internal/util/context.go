// Package util provides common utility functions to eliminate code duplication.
package util

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// traceIDKey is the context key for trace/request IDs.
const traceIDKey contextKey = "trace_id"

// NewTimeoutContext creates a new context with the specified timeout.
// This eliminates the repeated pattern of context.WithTimeout(context.Background(), timeout).
//
// Example:
//
//	ctx, cancel := util.NewTimeoutContext(10 * time.Second)
//	defer cancel()
func NewTimeoutContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// NewDefaultTimeoutContext creates a new context with a default 10-second timeout.
// Use this for standard database operations.
func NewDefaultTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// NewContextWithTraceID creates a child context with a generated trace ID.
// The trace ID is a 16-byte random hex string (32 characters).
func NewContextWithTraceID(parent context.Context) context.Context {
	return context.WithValue(parent, traceIDKey, generateTraceID())
}

// ContextWithTraceID creates a child context with the provided trace ID.
func ContextWithTraceID(parent context.Context, traceID string) context.Context {
	return context.WithValue(parent, traceIDKey, traceID)
}

// TraceIDFromContext extracts the trace ID from the context.
// Returns empty string if no trace ID is set.
func TraceIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey).(string); ok {
		return id
	}
	return ""
}

// generateTraceID creates a cryptographically random 16-byte hex trace ID.
func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: should never happen in practice
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}
