package util

import (
	"context"
	"testing"
	"time"
)

func TestNewTimeoutContext(t *testing.T) {
	timeout := 5 * time.Second
	ctx, cancel := NewTimeoutContext(timeout)
	defer cancel()

	if ctx == nil {
		t.Fatal("Expected non-nil context")
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("Expected context to have deadline")
	}

	// Check that deadline is approximately timeout from now
	expectedDeadline := time.Now().Add(timeout)
	diff := deadline.Sub(expectedDeadline)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("Deadline differs from expected by %v", diff)
	}
}

func TestNewDefaultTimeoutContext(t *testing.T) {
	ctx, cancel := NewDefaultTimeoutContext()
	defer cancel()

	if ctx == nil {
		t.Fatal("Expected non-nil context")
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("Expected context to have deadline")
	}

	// Check that deadline is approximately 10 seconds from now
	expectedDeadline := time.Now().Add(10 * time.Second)
	diff := deadline.Sub(expectedDeadline)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("Deadline differs from expected by %v", diff)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := NewTimeoutContext(100 * time.Millisecond)

	// Cancel immediately
	cancel()

	// Context should be cancelled
	select {
	case <-ctx.Done():
		// Expected
		if ctx.Err() != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("Context was not cancelled")
	}
}

func TestContextTimeout(t *testing.T) {
	ctx, cancel := NewTimeoutContext(50 * time.Millisecond)
	defer cancel()

	// Wait for timeout
	<-ctx.Done()

	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", ctx.Err())
	}
}
