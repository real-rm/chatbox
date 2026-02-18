# Storage and Chatbox Coverage Boost - Requirements

## 1. Overview

This spec aims to increase test coverage for `internal/storage/storage.go` and `chatbox.go` from their current levels (~33% and ~27% respectively) to at least 80% coverage. The focus is on adding targeted unit tests that will be properly measured by Go's coverage tools.

## 2. Current State

### 2.1 internal/storage/storage.go Coverage
- **Current Coverage**: 32.6%
- **Functions with 0% coverage**:
  - `EnsureIndexes` (0.0%)
  - `UpdateSession` (0.0%)
  - `AddMessage` (0.0%)
  - `EndSession` (0.0%)
  - `ListUserSessions` (0.0%)
  - `ListAllSessions` (0.0%)
  - `ListAllSessionsWithOptions` (0.0%)
  - `GetSessionMetrics` (0.0%)
  - `GetTokenUsage` (0.0%)

- **Functions with partial coverage**:
  - `CreateSession` (80.0%)
  - `GetSession` (73.3%)
  - `encrypt` (81.2%)
  - `decrypt` (89.5%)

### 2.2 chatbox.go Coverage
- **Current Coverage**: 26.8%
- **Functions with 0% coverage**:
  - `Register` (0.0%) - Main registration function
  - `handleUserSessions` (0.0%)
  - `handleListSessions` (0.0%)
  - `handleGetMetrics` (0.0%)
  - `handleAdminTakeover` (0.0%)

- **Functions with partial coverage**:
  - `handleReadyCheck` (57.1%)
  - `Shutdown` (61.1%)
  - `adminRateLimitMiddleware` (95.5%)

## 3. User Stories

### 3.1 Storage Service Testing
**As a developer**, I want comprehensive unit tests for all storage service functions so that I can confidently modify the storage layer without breaking existing functionality.

**Acceptance Criteria**:
- All MongoDB CRUD operations have unit tests with mock database
- All list/query functions have tests covering different filter combinations
- All metrics aggregation functions have tests with sample data
- Error paths are tested (connection failures, invalid inputs, etc.)
- Coverage for `internal/storage/storage.go` reaches at least 80%

### 3.2 Chatbox Registration Testing
**As a developer**, I want tests for the main `Register` function so that I can verify all service initialization and route registration works correctly.

**Acceptance Criteria**:
- `Register` function has tests covering successful initialization
- Configuration loading is tested with various scenarios
- Route registration is verified
- Error paths are tested (missing config, invalid values, etc.)
- Coverage for `chatbox.go` reaches at least 80%

### 3.3 HTTP Handler Testing
**As a developer**, I want unit tests for all HTTP handlers so that I can ensure API endpoints behave correctly under various conditions.

**Acceptance Criteria**:
- All handler functions have tests with mock dependencies
- Request parameter parsing is tested (valid and invalid inputs)
- Authentication and authorization are tested
- Response formats are verified
- Error handling is tested

## 4. Technical Requirements

### 4.1 Test Infrastructure
- Use `httptest` for HTTP handler testing
- Use mock MongoDB collections or in-memory test database
- Use table-driven tests for multiple scenarios
- Follow existing test patterns in the codebase

### 4.2 Coverage Measurement
- Tests must be measurable by `go test -cover`
- Focus on unit tests that directly exercise the target functions
- Avoid integration tests that don't contribute to coverage metrics

### 4.3 Test Quality
- Tests should be fast (< 1 second per test)
- Tests should be isolated (no shared state)
- Tests should be deterministic (no flaky tests)
- Tests should follow AAA pattern (Arrange, Act, Assert)

## 5. Non-Goals

- This spec does NOT aim to replace existing integration tests
- This spec does NOT aim to test WebSocket functionality (already covered)
- This spec does NOT aim to refactor existing code
- This spec does NOT aim to add new features

## 6. Success Metrics

- `internal/storage/storage.go` coverage: ≥ 80%
- `chatbox.go` coverage: ≥ 80%
- All new tests pass consistently
- No existing tests are broken
- Test execution time remains reasonable (< 60 seconds total)

## 7. Dependencies

- MongoDB test configuration (already documented in `test.md`)
- Existing test utilities in `internal/testutil`
- Go testing framework and `httptest` package

## 8. Risks and Mitigations

### Risk 1: MongoDB Dependency
**Risk**: Tests requiring real MongoDB may be slow or flaky
**Mitigation**: Use mock collections or minimal test data; skip tests if MongoDB unavailable

### Risk 2: Complex Handler Logic
**Risk**: HTTP handlers have many dependencies that are hard to mock
**Mitigation**: Use dependency injection and create minimal mock implementations

### Risk 3: Coverage Measurement Accuracy
**Risk**: Coverage tools may not count all executed lines
**Mitigation**: Focus on direct function calls in unit tests; verify coverage with `go tool cover -html`
