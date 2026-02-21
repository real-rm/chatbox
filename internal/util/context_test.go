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

func TestNewContextWithTraceID(t *testing.T) {
	ctx := NewContextWithTraceID(context.Background())

	traceID := TraceIDFromContext(ctx)
	if traceID == "" {
		t.Fatal("expected non-empty trace ID")
	}
	if len(traceID) != 32 {
		t.Errorf("expected 32-char hex trace ID, got %d chars: %s", len(traceID), traceID)
	}
}

func TestTraceIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	traceID := TraceIDFromContext(ctx)
	if traceID != "" {
		t.Errorf("expected empty trace ID from bare context, got %q", traceID)
	}
}

func TestContextWithTraceID_Custom(t *testing.T) {
	customID := "abc123def456"
	ctx := ContextWithTraceID(context.Background(), customID)

	got := TraceIDFromContext(ctx)
	if got != customID {
		t.Errorf("expected %q, got %q", customID, got)
	}
}

func TestTraceID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		ctx := NewContextWithTraceID(context.Background())
		id := TraceIDFromContext(ctx)
		if ids[id] {
			t.Fatalf("duplicate trace ID generated: %s", id)
		}
		ids[id] = true
	}
}
