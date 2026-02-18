// Package util provides common utility functions to eliminate code duplication.
package util

import (
	"context"
	"time"
)

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
