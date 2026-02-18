# Race Detector Results - Task 8.3

## Summary

Ran all tests with the race detector using `go test -race ./...` and found **multiple data races** in the websocket handler tests.

## Data Races Found

### 1. mockRouter.RouteMessage() - Test Data Race
**Location**: `internal/websocket/handler_integration_test.go:36`
**Affected Tests**:
- `TestReadPump_NilRouterHandling`
- `TestOversizedMessages_Integration`
- `TestOversizedMessages_ExactLimit`
- `TestOversizedMessages_MultipleConnections`

**Issue**: The test's `mockRouter` struct has fields that are being written to by the websocket goroutine (in `RouteMessage()`) and read by the test goroutine without synchronization.

**Root Cause**: The `mockRouter` type at line 36 in `handler_integration_test.go` has fields that are accessed concurrently:
- `called` (bool)
- `lastMessage` (string)
- `lastMessageType` (int)

These fields are written in the `RouteMessage()` method (called from the websocket goroutine) and read in the test assertions without proper synchronization.

### 2. Handler.unregisterConnection() - Map Access Race
**Location**: `internal/websocket/handler.go:327`
**Affected Tests**:
- `TestHandler_MultipleConnectionsPerUser`
- `TestHandler_ConnectionLimitGracefulHandling`
- `TestHandler_ConnectionLimitPerUser`
- `TestHandler_NotifyConnectionLimit`
- `TestMultiDevice_ConnectionFailureIsolation`
- `TestMultiDevice_ReconnectionScenario`
- `TestMultiDevice_MaxConnectionsEnforcement`

**Issue**: The `Handler.connections` map is being accessed concurrently:
- **Write**: In `unregisterConnection()` when a connection closes (goroutine running `readPump`)
- **Read**: In test code checking connection counts or in `notifyConnectionLimit()` iterating over connections

**Root Cause**: The `connections` map in the `Handler` struct is accessed from multiple goroutines without proper mutex protection:
1. The websocket goroutine writes to it when unregistering
2. The test goroutine reads from it to check connection counts
3. The `notifyConnectionLimit()` method iterates over it

## Recommendations

### Fix 1: Synchronize mockRouter Access
Add a mutex to the `mockRouter` struct in test files:

```go
type mockRouter struct {
    mu              sync.Mutex
    called          bool
    lastMessage     string
    lastMessageType int
}

func (m *mockRouter) RouteMessage(...) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.called = true
    m.lastMessage = message
    m.lastMessageType = messageType
}
```

And protect reads in tests:
```go
m.mu.Lock()
called := m.called
m.mu.Unlock()
assert.True(t, called)
```

### Fix 2: Protect Handler.connections Map
The `Handler` struct already has a `mu sync.RWMutex`, but it needs to be used consistently:

1. Use `RLock/RUnlock` when reading the connections map (e.g., in tests or `notifyConnectionLimit`)
2. Use `Lock/Unlock` when writing to the connections map
3. Ensure all map access goes through the mutex

Example for test code:
```go
h.mu.RLock()
connCount := len(h.connections[userID])
h.mu.RUnlock()
```

## Test Results

- **Total Tests Run**: All tests in the repository
- **Tests with Data Races**: 10 tests failed due to race conditions
- **Packages Affected**: `internal/websocket`
- **Exit Code**: 1 (failure due to detected races)

## Next Steps

1. Fix the `mockRouter` synchronization issues in test files
2. Review and fix all `Handler.connections` map access to use proper locking
3. Re-run tests with `-race` flag to verify fixes
4. Consider adding more comprehensive concurrent access tests

## Notes

- The race detector successfully identified real concurrency issues that could cause problems in production
- These races are primarily in test code (`mockRouter`) and in the websocket handler's connection management
- The issues are fixable with proper mutex usage
- No races were detected in other packages (cmd/server, internal/storage, etc.)
