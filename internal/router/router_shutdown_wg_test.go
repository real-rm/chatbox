package router

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/assert"
)

// TestShutdown_WaitsForInFlightSafeGoGoroutines verifies that Shutdown() blocks
// until all goroutines launched via the router's safeGo method have finished.
// Without a WaitGroup, Shutdown() would return immediately while goroutines are
// still running, causing use-after-shutdown access to router dependencies.
func TestShutdown_WaitsForInFlightSafeGoGoroutines(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mr := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	var goroutineCompleted atomic.Bool
	goroutineStarted := make(chan struct{})
	allowFinish := make(chan struct{})

	// Launch a goroutine via the router's tracked safeGo method.
	// It signals when started and blocks until allowed to finish.
	mr.safeGo("test-shutdown-wait", func() {
		close(goroutineStarted)
		<-allowFinish
		goroutineCompleted.Store(true)
	})

	// Wait for goroutine to start so we know it's in-flight during Shutdown.
	select {
	case <-goroutineStarted:
	case <-time.After(time.Second):
		t.Fatal("goroutine did not start in time")
	}

	// Start Shutdown in a goroutine; it should block until our goroutine finishes.
	shutdownDone := make(chan struct{})
	go func() {
		mr.Shutdown()
		close(shutdownDone)
	}()

	// Shutdown should NOT have returned yet (goroutine is still blocked).
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned before in-flight goroutine finished — missing WaitGroup")
	case <-time.After(100 * time.Millisecond):
		// Expected: Shutdown is still waiting.
	}

	// Now unblock the goroutine.
	close(allowFinish)

	// Shutdown should complete shortly after the goroutine finishes.
	select {
	case <-shutdownDone:
		assert.True(t, goroutineCompleted.Load(),
			"goroutine should have completed before Shutdown returned")
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not complete after goroutine finished")
	}
}
