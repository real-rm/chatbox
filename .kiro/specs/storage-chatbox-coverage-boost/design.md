# Storage and Chatbox Coverage Boost - Design

## 1. Overview

This design document outlines the approach to increase test coverage for `internal/storage/storage.go` and `chatbox.go` from ~33% and ~27% to at least 80% respectively. The strategy focuses on adding targeted unit tests that directly exercise uncovered code paths.

## 2. Architecture

### 2.1 Test Structure

```
internal/storage/
  ├── storage.go                    # Target: 80% coverage
  ├── storage_test.go               # Existing integration tests
  ├── storage_property_test.go      # Existing property tests
  ├── storage_unit_test.go          # NEW: Unit tests for CRUD operations
  └── storage_metrics_test.go       # NEW: Unit tests for metrics/aggregation

chatbox.go                          # Target: 80% coverage
chatbox_test.go                     # Existing tests
chatbox_register_test.go            # NEW: Tests for Register function
chatbox_handlers_test.go            # NEW: Tests for HTTP handlers
```

### 2.2 Testing Strategy

#### For internal/storage/storage.go:
1. **CRUD Operations**: Test with mock MongoDB or minimal test data
2. **List/Query Functions**: Test with various filter combinations
3. **Metrics Functions**: Test with sample session data
4. **Error Paths**: Test connection failures, invalid inputs

#### For chatbox.go:
1. **Register Function**: Test with mock dependencies
2. **HTTP Handlers**: Test with `httptest.ResponseRecorder`
3. **Middleware**: Test authentication and rate limiting
4. **Health Checks**: Test ready/health endpoints

## 3. Detailed Design

### 3.1 Storage Unit Tests (storage_unit_test.go)

**Target Functions** (0% coverage):
- `EnsureIndexes`
- `UpdateSession`
- `AddMessage`
- `EndSession`
- `ListUserSessions`
- `ListAllSessions`
- `ListAllSessionsWithOptions`

**Test Approach**:
```go
// Use real MongoDB connection with test database
// Each test creates minimal test data and cleans up after

func TestUpdateSession(t *testing.T) {
    // Setup: Create storage service with test DB
    // Create a test session
    // Update the session
    // Verify the update
    // Cleanup
}

func TestAddMessage(t *testing.T) {
    // Setup: Create storage service with test DB
    // Create a test session
    // Add a message
    // Verify the message was added
    // Cleanup
}

// Similar pattern for other functions
```

**Key Design Decisions**:
- Use real MongoDB for accurate behavior testing
- Use test database name from `test.md` configuration
- Clean up test data after each test
- Use table-driven tests for multiple scenarios

### 3.2 Storage Metrics Tests (storage_metrics_test.go)

**Target Functions** (0% coverage):
- `GetSessionMetrics`
- `GetTokenUsage`

**Test Approach**:
```go
func TestGetSessionMetrics(t *testing.T) {
    tests := []struct {
        name          string
        sessions      []SessionDocument  // Test data to insert
        startTime     time.Time
        endTime       time.Time
        expectedMetrics *Metrics
    }{
        {
            name: "single active session",
            sessions: []SessionDocument{...},
            expectedMetrics: &Metrics{
                TotalSessions: 1,
                ActiveSessions: 1,
                ...
            },
        },
        {
            name: "multiple sessions with admin assistance",
            sessions: []SessionDocument{...},
            expectedMetrics: &Metrics{...},
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup: Insert test sessions
            // Call GetSessionMetrics
            // Verify results match expected
            // Cleanup
        })
    }
}
```

### 3.3 Chatbox Register Tests (chatbox_register_test.go)

**Target Function**: `Register` (0% coverage)

**Test Approach**:
```go
func TestRegister(t *testing.T) {
    tests := []struct {
        name        string
        setupConfig func(*goconfig.ConfigAccessor)
        setupEnv    map[string]string
        expectError bool
        errorMsg    string
    }{
        {
            name: "successful registration with valid config",
            setupConfig: func(cfg *goconfig.ConfigAccessor) {
                // Set valid configuration
            },
            setupEnv: map[string]string{
                "JWT_SECRET": "test-secret-at-least-32-characters-long",
            },
            expectError: false,
        },
        {
            name: "missing JWT secret",
            setupConfig: func(cfg *goconfig.ConfigAccessor) {},
            setupEnv: map[string]string{},
            expectError: true,
            errorMsg: "JWT secret",
        },
        {
            name: "invalid encryption key length",
            setupConfig: func(cfg *goconfig.ConfigAccessor) {},
            setupEnv: map[string]string{
                "JWT_SECRET": "test-secret-at-least-32-characters-long",
                "ENCRYPTION_KEY": "short", // Invalid length
            },
            expectError: true,
            errorMsg: "encryption key",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup environment variables
            // Create mock dependencies
            // Call Register
            // Verify error or success
            // Cleanup
        })
    }
}
```

**Challenges**:
- `Register` has many dependencies (config, logger, mongo)
- Need to mock or create minimal implementations
- Need to verify route registration without starting server

**Solution**:
- Use `gin.New()` for clean router
- Use test MongoDB instance
- Use test configuration
- Verify routes are registered by checking router.Routes()

### 3.4 Chatbox Handler Tests (chatbox_handlers_test.go)

**Target Functions** (0% coverage):
- `handleUserSessions`
- `handleListSessions`
- `handleGetMetrics`
- `handleAdminTakeover`

**Test Approach**:
```go
func TestHandleUserSessions(t *testing.T) {
    tests := []struct {
        name           string
        userID         string
        setupStorage   func(*storage.StorageService)
        expectedStatus int
        expectedCount  int
    }{
        {
            name: "user with sessions",
            userID: "user123",
            setupStorage: func(s *storage.StorageService) {
                // Create test sessions for user123
            },
            expectedStatus: 200,
            expectedCount: 2,
        },
        {
            name: "user with no sessions",
            userID: "user456",
            setupStorage: func(s *storage.StorageService) {},
            expectedStatus: 200,
            expectedCount: 0,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup: Create storage service
            // Setup test data
            // Create HTTP request
            // Create response recorder
            // Call handler
            // Verify response
            // Cleanup
        })
    }
}
```

**Key Design Decisions**:
- Use `httptest.NewRecorder()` for response capture
- Use `httptest.NewRequest()` for request creation
- Mock JWT claims in Gin context
- Use real storage service with test database

### 3.5 Partial Coverage Improvements

**Target Functions** (partial coverage):
- `CreateSession` (80.0% → 100%)
- `GetSession` (73.3% → 100%)
- `encrypt` (81.2% → 100%)
- `decrypt` (89.5% → 100%)
- `handleReadyCheck` (57.1% → 100%)
- `Shutdown` (61.1% → 100%)

**Approach**:
- Identify uncovered branches using `go tool cover -html`
- Add test cases for uncovered error paths
- Add test cases for edge cases

## 4. Test Data Management

### 4.1 MongoDB Test Database
- Use database name: `chatbox` (from `test.md`)
- Use connection string from `MONGO_URI` environment variable
- Clean up test data after each test using `defer`

### 4.2 Test Session Data
```go
// Helper function to create test session
func createTestSession(t *testing.T, storage *storage.StorageService, userID string) *session.Session {
    sess := &session.Session{
        ID:        "test-" + userID + "-" + time.Now().Format("20060102150405"),
        UserID:    userID,
        Name:      "Test Session",
        ModelID:   "gpt-4",
        Messages:  []*session.Message{},
        StartTime: time.Now(),
        IsActive:  true,
    }
    
    err := storage.CreateSession(sess)
    require.NoError(t, err)
    
    return sess
}
```

### 4.3 Cleanup Strategy
```go
func cleanupTestSession(t *testing.T, storage *storage.StorageService, sessionID string) {
    // Delete test session from database
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    _, err := storage.collection.DeleteOne(ctx, bson.M{"_id": sessionID})
    if err != nil {
        t.Logf("Warning: Failed to cleanup test session %s: %v", sessionID, err)
    }
}
```

## 5. Correctness Properties

### 5.1 Storage CRUD Properties

**Property 1.1**: Session Creation Idempotency
- **Description**: Creating a session with the same ID twice should fail
- **Test**: Attempt to create duplicate session, verify error
- **Validates**: Requirements 3.1

**Property 1.2**: Session Update Consistency
- **Description**: Updating a session should preserve all unchanged fields
- **Test**: Update one field, verify others remain unchanged
- **Validates**: Requirements 3.1

**Property 1.3**: Message Ordering
- **Description**: Messages added to a session should maintain chronological order
- **Test**: Add multiple messages, verify order by timestamp
- **Validates**: Requirements 3.1

### 5.2 List/Query Properties

**Property 2.1**: Filter Correctness
- **Description**: List operations should only return sessions matching the filter
- **Test**: Create sessions with different attributes, verify filter results
- **Validates**: Requirements 3.1

**Property 2.2**: Pagination Consistency
- **Description**: Paginated results should be consistent across multiple requests
- **Test**: Query with different offsets, verify no duplicates or gaps
- **Validates**: Requirements 3.1

**Property 2.3**: Sort Order Correctness
- **Description**: Results should be sorted according to the specified field and order
- **Test**: Create sessions with different values, verify sort order
- **Validates**: Requirements 3.1

### 5.3 Metrics Properties

**Property 3.1**: Token Sum Accuracy
- **Description**: Total token usage should equal the sum of all session tokens
- **Test**: Create sessions with known token counts, verify sum
- **Validates**: Requirements 3.1

**Property 3.2**: Active Session Count
- **Description**: Active session count should match sessions without end time
- **Test**: Create mix of active/ended sessions, verify count
- **Validates**: Requirements 3.1

**Property 3.3**: Time Range Filtering
- **Description**: Metrics should only include sessions within the specified time range
- **Test**: Create sessions at different times, verify time range filter
- **Validates**: Requirements 3.1

### 5.4 HTTP Handler Properties

**Property 4.1**: Authentication Enforcement
- **Description**: Protected endpoints should reject requests without valid JWT
- **Test**: Call handlers without auth header, verify 401 response
- **Validates**: Requirements 3.3

**Property 4.2**: Authorization Enforcement
- **Description**: Admin endpoints should reject requests without admin role
- **Test**: Call admin handlers with non-admin JWT, verify 403 response
- **Validates**: Requirements 3.3

**Property 4.3**: Response Format Consistency
- **Description**: All handlers should return JSON with consistent structure
- **Test**: Call handlers, verify response content-type and structure
- **Validates**: Requirements 3.3

### 5.5 Registration Properties

**Property 5.1**: Configuration Validation
- **Description**: Register should fail with clear error for invalid configuration
- **Test**: Call Register with missing/invalid config, verify error message
- **Validates**: Requirements 3.2

**Property 5.2**: Route Registration Completeness
- **Description**: Register should register all expected routes
- **Test**: Call Register, verify all routes exist in router
- **Validates**: Requirements 3.2

## 6. Implementation Plan

### Phase 1: Storage Unit Tests
1. Create `storage_unit_test.go`
2. Implement tests for CRUD operations (UpdateSession, AddMessage, EndSession)
3. Implement tests for list operations (ListUserSessions, ListAllSessions, ListAllSessionsWithOptions)
4. Implement test for EnsureIndexes
5. Run coverage and verify improvement

### Phase 2: Storage Metrics Tests
1. Create `storage_metrics_test.go`
2. Implement tests for GetSessionMetrics
3. Implement tests for GetTokenUsage
4. Run coverage and verify improvement

### Phase 3: Chatbox Handler Tests
1. Create `chatbox_handlers_test.go`
2. Implement tests for handleUserSessions
3. Implement tests for handleListSessions
4. Implement tests for handleGetMetrics
5. Implement tests for handleAdminTakeover
6. Run coverage and verify improvement

### Phase 4: Chatbox Register Tests
1. Create `chatbox_register_test.go`
2. Implement tests for Register function
3. Test configuration loading and validation
4. Test route registration
5. Run coverage and verify improvement

### Phase 5: Coverage Completion
1. Identify remaining uncovered lines using `go tool cover -html`
2. Add tests for uncovered branches and error paths
3. Improve partial coverage functions to 100%
4. Final coverage verification

## 7. Testing Guidelines

### 7.1 Test Naming Convention
- Test functions: `Test<FunctionName>_<Scenario>`
- Example: `TestUpdateSession_Success`, `TestUpdateSession_NotFound`

### 7.2 Test Structure (AAA Pattern)
```go
func TestFunctionName_Scenario(t *testing.T) {
    // Arrange: Setup test data and dependencies
    
    // Act: Call the function under test
    
    // Assert: Verify the results
    
    // Cleanup: Remove test data
}
```

### 7.3 Error Testing
- Always test error paths
- Verify error messages are meaningful
- Test both expected and unexpected errors

### 7.4 Table-Driven Tests
- Use for multiple scenarios of the same function
- Keep test cases readable and maintainable
- Use descriptive test case names

## 8. Success Criteria

- [ ] `internal/storage/storage.go` coverage ≥ 80%
- [ ] `chatbox.go` coverage ≥ 80%
- [ ] All new tests pass consistently
- [ ] No existing tests broken
- [ ] Test execution time < 60 seconds
- [ ] All correctness properties validated

## 9. Risks and Mitigations

### Risk 1: MongoDB Test Flakiness
**Mitigation**: Use proper cleanup, unique test IDs, and skip tests if MongoDB unavailable

### Risk 2: Complex Mock Setup
**Mitigation**: Create helper functions for common mock setups

### Risk 3: Coverage Tool Limitations
**Mitigation**: Verify coverage manually with `go tool cover -html`, focus on direct function calls
