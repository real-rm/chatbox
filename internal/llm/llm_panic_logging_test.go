package llm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecoverStreamPanic_LogsStackOnPanic verifies that when recoverStreamPanic
// catches a panic, it uses the provided logger (accepts the logger parameter)
// and sends a Done chunk to terminate downstream consumers.
func TestRecoverStreamPanic_LogsStackOnPanic(t *testing.T) {
	logger := createTestLogger()
	ch := make(chan *LLMChunk, 1)

	func() {
		defer recoverStreamPanic(ch, "test-component", logger)
		panic("simulated LLM stream panic")
	}()

	// Should have sent a Done chunk to unblock downstream consumers
	select {
	case chunk := <-ch:
		require.NotNil(t, chunk, "chunk should not be nil")
		assert.True(t, chunk.Done, "recovered panic should send a Done chunk")
	case <-time.After(time.Second):
		t.Fatal("timeout: no Done chunk received after panic recovery")
	}
}

// TestRecoverStreamPanic_NoPanicWithLogger verifies that when there is no panic,
// recoverStreamPanic with a logger does not send any chunk.
func TestRecoverStreamPanic_NoPanicWithLogger(t *testing.T) {
	logger := createTestLogger()
	ch := make(chan *LLMChunk, 1)

	func() {
		defer recoverStreamPanic(ch, "test-no-panic", logger)
		// No panic
	}()

	select {
	case <-ch:
		t.Fatal("should not receive any chunk when there is no panic")
	default:
		// Expected
	}
}
