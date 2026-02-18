# Production Readiness Verification - Design

## 1. Overview

This design document outlines the unit tests needed to verify the production readiness review findings. The tests will validate current system behavior, identify true issues, and document false positives.

## 2. Test Organization

### 2.1 Test File Structure

Tests will be organized by component:

```
internal/session/session_production_test.go          # Session management tests
internal/router/router_production_test.go            # Router and connection tests
internal/websocket/handler_production_test.go        # WebSocket handler tests
internal/storage/storage_production_test.go          # Storage and serialization tests
internal/ratelimit/ratelimit_production_test.go      # Rate limiter tests
internal/config/config_production_test.go            # Configuration validation tests
cmd/server/main_production_test.go                   # Main server tests
chatbox_production_test.go                           # Integration-level tests
```

### 2.2 Test Naming Convention

Test names will follow the pattern:
```
TestProductionIssue<Number>_<ShortDescription>
```

Example: `TestProductionIssue01_SessionCleanup`

## 3. Test Specifications

### 3.1 Session Management Tests (Issue #1)

**File**: `internal/session/session_production_test.go`

#### Test: TestProductionIssue01_SessionCleanup
**Purpose**: Verify that EndSession marks sessions as inactive but does NOT remove them from memory

**Test Steps**:
1. Create a SessionManager
2. Create a new session
3. Verify session exists in sessions map
4. Call EndSession()
5. Verify session still exists in sessions map
6. Verify session.IsActive = false
7. Verify session.EndTime is set
8. Verify user mapping is removed from userSessions map

**Expected Result**: Session remains in memory but marked inactive (documents current behavior)

**Validates**: Requirements 3.1

---

#### Test: TestProductionIssue01_MemoryGrowth
**Purpose**: Demonstrate unbounded memory growth with many sessions

**Test Steps**:
1. Create a SessionManager
2. Create 1000 sessions for different users
3. End all sessions
4. Verify all 1000 sessions still exist in memory
5. Calculate approximate memory usage
6. Document that cleanup is not automatic

**Expected Result**: All sessions remain in memory after ending

**Validates**: Requirements 3.1

---

### 3.2 Session Creation Flow Tests (Issue #2)

**File**: `internal/router/router_production_test.go`

#### Test: TestProductionIssue02_SessionIDConsistency
**Purpose**: Verify session ID consistency between SessionManager and Router

**Test Steps**:
1. Create a MessageRouter with SessionManager
2. Create a mock connection with userID
3. Create a message with sessionID = ""
4. Call getOrCreateSession()
5. Verify SessionManager generates a session ID
6. Verify the returned session has the generated ID
7. Verify connection can be registered with the generated ID
8. Verify no session ID mismatch occurs

**Expected Result**: Session ID is consistent throughout the flow

**Validates**: Requirements 3.2

---

#### Test: TestProductionIssue02_CreateNewSessionFlow
**Purpose**: Test the complete session creation flow

**Test Steps**:
1. Create MessageRouter with mocked StorageService
2. Create a connection with userID
3. Call createNewSession()
4. Verify SessionManager.CreateSession() is called
5. Verify StorageService.CreateSession() is called with correct session
6. Verify session ID matches between memory and storage
7. Test rollback on storage failure

**Expected Result**: Session creation is atomic and consistent

**Validates**: Requirements 3.2

---

### 3.3 Connection Management Tests (Issue #3)

**File**: `internal/router/router_production_test.go`

#### Test: TestProductionIssue03_ConnectionReplacement
**Purpose**: Verify connection replacement behavior

**Test Steps**:
1. Create a MessageRouter
2. Register a connection for sessionID "test-session"
3. Verify connection is stored
4. Register a different connection for the same sessionID
5. Verify new connection replaces old connection
6. Document that old connection is not explicitly closed

**Expected Result**: Connection is replaced without cleanup (documents current behavior)

**Validates**: Requirements 3.3

---

#### Test: TestProductionIssue03_UnregisterConnection
**Purpose**: Verify connection cleanup on unregister

**Test Steps**:
1. Create a MessageRouter
2. Register a connection
3. Call UnregisterConnection()
4. Verify connection is removed from map
5. Verify no goroutines are leaked (use runtime.NumGoroutine())

**Expected Result**: Connection is properly cleaned up

**Validates**: Requirements 3.3

---

### 3.4 Concurrency Safety Tests (Issue #4)

**File**: `internal/websocket/handler_production_test.go`

#### Test: TestProductionIssue04_SessionIDDataRace
**Purpose**: Verify thread-safe access to Connection.SessionID

**Test Steps**:
1. Create a Connection
2. Launch 100 goroutines that read SessionID
3. Launch 100 goroutines that write SessionID (with lock)
4. Wait for all goroutines to complete
5. Run with `go test -race`

**Expected Result**: No data race detected

**Validates**: Requirements 3.4

---

#### Test: TestProductionIssue04_ConcurrentSessionAccess
**Purpose**: Verify thread-safe session field access

**Test Steps**:
1. Create a Session
2. Launch goroutines to read/write various fields
3. Verify proper locking is used
4. Run with `go test -race`

**Expected Result**: No data races on session fields

**Validates**: Requirements 3.4

---

### 3.5 Main Server Tests (Issue #5)

**File**: `cmd/server/main_production_test.go`

#### Test: TestProductionIssue05_MainServerStartup
**Purpose**: Verify main.go functionality

**Test Steps**:
1. Review main.go implementation
2. Document that it only waits for signals
3. Document that Register() is not called
4. Document that HTTP server is not started
5. Create test that verifies signal handling works

**Expected Result**: Test documents current non-functional state

**Validates**: Requirements 3.5

---

### 3.6 Secret Management Tests (Issue #6)

**File**: `deployments/kubernetes/secret_validation_test.go`

#### Test: TestProductionIssue06_PlaceholderSecrets
**Purpose**: Detect placeholder secrets in Kubernetes manifests

**Test Steps**:
1. Read deployments/kubernetes/secret.yaml
2. Parse YAML content
3. Check for placeholder patterns:
   - "your-*"
   - "CHANGE-ME"
   - "sk-your-*"
   - "ACXXXXXXXX"
4. List all placeholder secrets found
5. Fail test if placeholders are detected

**Expected Result**: Test fails, documenting placeholder secrets

**Validates**: Requirements 3.6

---

### 3.7 Message Validation Tests (Issue #7)

**File**: `internal/message/validation_production_test.go`

#### Test: TestProductionIssue07_ValidationCalled
**Purpose**: Verify message validation is called in WebSocket path

**Test Steps**:
1. Create a mock router that tracks validation calls
2. Send a message through the WebSocket handler
3. Verify Validate() is called (or not called)
4. Verify Sanitize() is called (or not called)
5. Document current behavior

**Expected Result**: Documents whether validation is active

**Validates**: Requirements 3.7

---

### 3.8 LLM Streaming Context Tests (Issue #8)

**File**: `internal/router/router_production_test.go`

#### Test: TestProductionIssue08_StreamingContext
**Purpose**: Verify LLM streaming uses proper context

**Test Steps**:
1. Create MessageRouter with mock LLM service
2. Call HandleUserMessage()
3. Verify StreamMessage() is called with context
4. Check if context has timeout set
5. Document context type (Background vs WithTimeout)

**Expected Result**: Documents current context usage

**Validates**: Requirements 3.8

---

#### Test: TestProductionIssue08_StreamingTimeout
**Purpose**: Test streaming timeout behavior

**Test Steps**:
1. Create mock LLM that hangs indefinitely
2. Call HandleUserMessage()
3. Verify request completes or times out
4. Measure time taken
5. Document timeout behavior

**Expected Result**: Documents whether timeouts are enforced

**Validates**: Requirements 3.8

---

### 3.9 MongoDB Retry Logic Tests (Issue #9)

**File**: `internal/storage/storage_production_test.go`

#### Test: TestProductionIssue09_MongoDBRetry
**Purpose**: Verify MongoDB retry behavior

**Test Steps**:
1. Create StorageService with mock MongoDB
2. Configure mock to return transient error
3. Call CreateSession()
4. Verify number of retry attempts (expect 1)
5. Verify timeout is 5 seconds
6. Document no retry logic exists

**Expected Result**: Single attempt with 5s timeout

**Validates**: Requirements 3.9

---

### 3.10 Session Serialization Tests (Issue #10)

**File**: `internal/storage/storage_production_test.go`

#### Test: TestProductionIssue10_SerializationDataRace
**Purpose**: Verify thread-safe session serialization

**Test Steps**:
1. Create a Session
2. Launch goroutine to call sessionToDocument()
3. Launch goroutine to modify session fields
4. Run with `go test -race`
5. Document if data race occurs

**Expected Result**: Documents current locking behavior

**Validates**: Requirements 3.10

---

#### Test: TestProductionIssue10_SerializationAccuracy
**Purpose**: Verify session data is correctly serialized

**Test Steps**:
1. Create a session with known data
2. Call sessionToDocument()
3. Verify all fields are correctly converted
4. Verify no data loss
5. Test with concurrent modifications

**Expected Result**: Data is accurately serialized

**Validates**: Requirements 3.10

---

### 3.11 Rate Limiter Cleanup Tests (Issue #11)

**File**: `internal/ratelimit/ratelimit_production_test.go`

#### Test: TestProductionIssue11_CleanupMethod
**Purpose**: Verify Cleanup() method exists and works

**Test Steps**:
1. Create MessageLimiter
2. Generate events for 100 users
3. Wait for events to expire
4. Call Cleanup()
5. Verify expired events are removed
6. Verify memory is freed

**Expected Result**: Cleanup() removes expired events

**Validates**: Requirements 3.11

---

#### Test: TestProductionIssue11_MemoryGrowth
**Purpose**: Demonstrate memory growth without cleanup

**Test Steps**:
1. Create MessageLimiter
2. Generate 10,000 events over time
3. Never call Cleanup()
4. Measure memory usage
5. Document unbounded growth

**Expected Result**: Memory grows without cleanup

**Validates**: Requirements 3.11

---

### 3.12 Response Times Tracking Tests (Issue #12)

**File**: `internal/session/session_production_test.go`

#### Test: TestProductionIssue12_ResponseTimesGrowth
**Purpose**: Verify ResponseTimes slice growth

**Test Steps**:
1. Create a Session
2. Record 10,000 response times
3. Verify all are stored in slice
4. Measure memory usage
5. Document unbounded growth

**Expected Result**: Slice grows without limit

**Validates**: Requirements 3.12

---

### 3.13 Origin Validation Tests (Issue #13)

**File**: `internal/websocket/handler_production_test.go`

#### Test: TestProductionIssue13_OriginValidationDataRace
**Purpose**: Verify thread-safe origin validation

**Test Steps**:
1. Create Handler
2. Launch goroutines calling checkOrigin()
3. Launch goroutine calling SetAllowedOrigins()
4. Run with `go test -race`

**Expected Result**: Documents if data race exists

**Validates**: Requirements 3.13

---

#### Test: TestProductionIssue13_DefaultOriginBehavior
**Purpose**: Verify default origin validation behavior

**Test Steps**:
1. Create Handler with no origins configured
2. Call checkOrigin() with various origins
3. Verify all origins are allowed
4. Document development mode behavior

**Expected Result**: All origins allowed when unconfigured

**Validates**: Requirements 3.13

---

### 3.14 Shutdown Behavior Tests (Issue #15)

**File**: `chatbox_production_test.go`

#### Test: TestProductionIssue15_ShutdownTimeout
**Purpose**: Verify shutdown respects context deadline

**Test Steps**:
1. Create Handler with active connections
2. Create context with 1-second timeout
3. Call Shutdown(ctx)
4. Measure time taken
5. Verify shutdown completes within timeout

**Expected Result**: Documents current timeout behavior

**Validates**: Requirements 3.14

---

### 3.15 Configuration Validation Tests (Issue #19)

**File**: `internal/config/config_production_test.go`

#### Test: TestProductionIssue19_ValidationCalled
**Purpose**: Verify Config.Validate() must be called explicitly

**Test Steps**:
1. Create Config with invalid values
2. Verify Load() does not call Validate()
3. Call Validate() explicitly
4. Verify errors are returned
5. Document that validation is not automatic

**Expected Result**: Validation is manual, not automatic

**Validates**: Requirements 3.15

---

#### Test: TestProductionIssue19_ValidationCoverage
**Purpose**: Verify all config fields are validated

**Test Steps**:
1. Test each validation rule in Validate()
2. Verify port range validation
3. Verify required field validation
4. Verify format validation
5. Ensure comprehensive coverage

**Expected Result**: All fields are validated

**Validates**: Requirements 3.15

---

### 3.16 JWT Secret Validation Tests (Issue #17)

**File**: `chatbox_production_test.go`

#### Test: TestProductionIssue17_WeakSecretAcceptance
**Purpose**: Verify weak secrets are accepted

**Test Steps**:
1. Create config with weak JWT secret ("test123")
2. Initialize chatbox service
3. Verify service starts successfully
4. Document no strength validation exists

**Expected Result**: Weak secrets are accepted

**Validates**: Requirements 3.16

---

### 3.17 Admin Endpoint Security Tests (Issue #18)

**File**: `chatbox_production_test.go`

#### Test: TestProductionIssue18_AdminRateLimiting
**Purpose**: Verify admin endpoint rate limiting

**Test Steps**:
1. Create test server with admin endpoints
2. Send 1000 rapid requests to /chat/admin/sessions
3. Verify if rate limiting is applied
4. Document current behavior

**Expected Result**: Documents rate limiting status

**Validates**: Requirements 3.17

---

## 4. Test Helpers and Utilities

### 4.1 Mock Implementations

```go
// MockStorageService for testing
type MockStorageService struct {
    CreateSessionFunc func(*session.Session) error
    UpdateSessionFunc func(*session.Session) error
    GetSessionFunc    func(string) (*session.Session, error)
}

// MockLLMService for testing
type MockLLMService struct {
    StreamMessageFunc func(context.Context, string, []llm.ChatMessage) (<-chan *llm.LLMChunk, error)
}

// MockConnection for testing
type MockConnection struct {
    UserID    string
    SessionID string
    SendChan  chan []byte
}
```

### 4.2 Test Fixtures

```go
// CreateTestSession creates a session for testing
func CreateTestSession(userID string) *session.Session {
    return &session.Session{
        ID:           "test-session-" + userID,
        UserID:       userID,
        Messages:     []*session.Message{},
        StartTime:    time.Now(),
        IsActive:     true,
        TotalTokens:  0,
        ResponseTimes: []time.Duration{},
    }
}

// CreateTestConnection creates a connection for testing
func CreateTestConnection(userID, sessionID string) *websocket.Connection {
    return &websocket.Connection{
        UserID:    userID,
        SessionID: sessionID,
        Roles:     []string{"user"},
    }
}
```

### 4.3 Assertion Helpers

```go
// AssertNoDataRace runs a function and checks for data races
func AssertNoDataRace(t *testing.T, fn func()) {
    // Run with -race flag
    fn()
    // If data race occurs, test will fail automatically
}

// AssertMemoryGrowth measures memory growth
func AssertMemoryGrowth(t *testing.T, before, after runtime.MemStats) {
    growth := after.Alloc - before.Alloc
    t.Logf("Memory growth: %d bytes", growth)
}
```

## 5. Correctness Properties

### Property 1: Session Lifecycle Consistency
**Statement**: For any session, if EndSession() is called, the session must be marked inactive but remain in memory.

**Test**: TestProductionIssue01_SessionCleanup

**Validation**: 
- Create session → Verify IsActive = true
- End session → Verify IsActive = false
- Check memory → Verify session still exists

---

### Property 2: Session ID Uniqueness
**Statement**: All sessions created by SessionManager must have unique IDs.

**Test**: TestProductionIssue02_SessionIDConsistency

**Validation**:
- Create 1000 sessions
- Verify all IDs are unique
- Verify no collisions

---

### Property 3: Connection Registration Idempotency
**Statement**: Registering a connection multiple times for the same session should replace the previous connection.

**Test**: TestProductionIssue03_ConnectionReplacement

**Validation**:
- Register connection A for session X
- Register connection B for session X
- Verify only connection B is registered

---

### Property 4: Thread-Safe Field Access
**Statement**: All concurrent reads and writes to shared fields must be protected by locks.

**Test**: TestProductionIssue04_SessionIDDataRace

**Validation**:
- Run with -race flag
- Perform concurrent reads/writes
- Verify no data races detected

---

### Property 5: Rate Limiter Cleanup Effectiveness
**Statement**: Calling Cleanup() must remove all expired events from the rate limiter.

**Test**: TestProductionIssue11_CleanupMethod

**Validation**:
- Add events
- Wait for expiration
- Call Cleanup()
- Verify events are removed

---

## 6. Implementation Notes

### 6.1 Test Execution

Run all production readiness tests:
```bash
go test -v -race ./... -run TestProductionIssue
```

Run specific issue test:
```bash
go test -v -race ./internal/session -run TestProductionIssue01
```

### 6.2 Coverage Analysis

Generate coverage report:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### 6.3 Race Detection

All tests must pass with race detector:
```bash
go test -race ./...
```

### 6.4 Documentation

Each test must include:
- Comment header with issue number and description
- Link to production readiness review
- Expected vs actual behavior
- Recommendations for fixes (if applicable)

## 7. Success Criteria

- All 19 production readiness issues have corresponding tests
- Tests accurately document current behavior
- Tests identify true issues vs false positives
- All tests pass (documenting current state)
- No data races detected in concurrent tests
- Test coverage >80% for tested components
- Clear documentation of findings

## 8. Future Enhancements

After tests are implemented:
1. Create follow-up specs to fix identified issues
2. Add integration tests for end-to-end flows
3. Implement property-based tests for complex behaviors
4. Add performance benchmarks
5. Create chaos testing scenarios
