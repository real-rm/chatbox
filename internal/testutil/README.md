# Test Utilities Package

This package provides common test helpers and mock implementations for production readiness verification tests.

## Overview

The `testutil` package centralizes mock implementations and assertion helpers used across the test suite. This ensures consistency and reduces code duplication in tests.

## Mock Implementations

### MockStorageService

A thread-safe mock implementation of the StorageService interface.

**Features:**
- Tracks all method calls (CreateSession, UpdateSession, GetSession)
- Allows custom behavior via function fields
- Supports error injection for testing failure scenarios
- Thread-safe for concurrent testing
- Includes Reset() method to clear tracking data

**Example Usage:**
```go
mock := &testutil.MockStorageService{
    CreateSessionError: errors.New("database error"),
}

err := mock.CreateSession(session)
assert.Error(t, err)
assert.True(t, mock.CreateSessionCalled)
```

### MockLLMService

A thread-safe mock implementation of the LLMService interface.

**Features:**
- Tracks StreamMessage and SendMessage calls
- Captures context, model ID, and messages for verification
- Allows custom behavior via function fields
- Supports error injection
- Thread-safe for concurrent testing
- Includes Reset() method to clear tracking data

**Example Usage:**
```go
mock := &testutil.MockLLMService{
    StreamMessageFunc: func(ctx context.Context, modelID string, messages []llm.ChatMessage) (<-chan *llm.LLMChunk, error) {
        ch := make(chan *llm.LLMChunk, 1)
        ch <- &llm.LLMChunk{Content: "test", Done: true}
        close(ch)
        return ch, nil
    },
}

ch, err := mock.StreamMessage(ctx, "gpt-4", messages)
assert.NoError(t, err)
assert.True(t, mock.StreamMessageCalled)
```

## Helper Functions

### MockConnection

Creates a mock WebSocket connection for testing.

```go
conn := testutil.MockConnection("user-1", "session-1", []string{"user"})
```

### CreateTestSession

Creates a session with test data.

```go
sess := testutil.CreateTestSession("user-1", "session-1")
```

### CreateTestSessionWithMessages

Creates a session with predefined messages.

```go
sess := testutil.CreateTestSessionWithMessages("user-1", "session-1", 10)
assert.Len(t, sess.Messages, 10)
```

### CreateTestLogger

Creates a logger for testing that writes to a temporary directory.

```go
logger := testutil.CreateTestLogger(t)
defer logger.Close()
```

## Assertion Helpers

### AssertNoDataRace

Documents the need to run tests with the `-race` flag. This is a documentation helper that reminds developers to check for data races.

```go
testutil.AssertNoDataRace(t, "concurrent session access")
// Run with: go test -race
```

### AssertMemoryGrowth

Measures and reports memory growth between two points.

```go
before := testutil.MeasureMemory()
// ... perform operations ...
after := testutil.MeasureMemory()
testutil.AssertMemoryGrowth(t, before, after, "session creation")
```

### AssertGoroutineCount

Measures and reports goroutine count changes, with tolerance for test framework overhead.

```go
before := testutil.MeasureGoroutines()
// ... perform operations ...
testutil.WaitForGoroutines()
after := testutil.MeasureGoroutines()
testutil.AssertGoroutineCount(t, before, after, "connection cleanup")
```

## Utility Functions

### MeasureMemory

Captures current memory statistics after forcing GC.

```go
mem := testutil.MeasureMemory()
fmt.Printf("Allocated: %d bytes\n", mem.Alloc)
```

### MeasureGoroutines

Returns the current goroutine count.

```go
count := testutil.MeasureGoroutines()
```

### WaitForGoroutines

Waits for goroutines to stabilize by forcing GC and sleeping briefly.

```go
testutil.WaitForGoroutines()
```

## Best Practices

1. **Use mocks for external dependencies**: Always mock StorageService and LLMService in unit tests
2. **Reset mocks between tests**: Call `Reset()` on mocks when reusing them
3. **Run with -race flag**: Always run tests with `-race` to detect data races
4. **Measure before and after**: Use `MeasureMemory()` and `MeasureGoroutines()` before and after operations
5. **Wait for cleanup**: Use `WaitForGoroutines()` before measuring goroutine counts

## Thread Safety

All mock implementations are thread-safe and can be used in concurrent tests. They use mutexes to protect internal state.

## Testing the Helpers

The helpers themselves are tested in `helpers_test.go`. Run:

```bash
go test ./internal/testutil/
go test -race ./internal/testutil/
```

## Related Documentation

- [Production Readiness Verification Spec](.kiro/specs/production-readiness-verification/)
- [Testing Guidelines](../../docs/TESTING.md)
