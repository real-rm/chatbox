# Storage and Chatbox Coverage Boost - Tasks

## Phase 1: Storage Unit Tests (CRUD Operations)

- [x] 1. Create storage_unit_test.go file
  - [x] 1.1 Set up test file structure with imports and helper functions
  - [x] 1.2 Create helper function for test storage service setup
  - [x] 1.3 Create helper function for test session creation
  - [x] 1.4 Create helper function for test data cleanup

- [x] 2. Implement UpdateSession tests
  - [x] 2.1 Test successful session update
  - [x] 2.2 Test update with nil session (error case)
  - [x] 2.3 Test update with empty session ID (error case)
  - [x] 2.4 Test update for non-existent session (error case)
  - [x] 2.5 Verify coverage improvement for UpdateSession function

- [x] 3. Implement AddMessage tests
  - [x] 3.1 Test adding message to existing session
  - [x] 3.2 Test adding message with encryption enabled
  - [x] 3.3 Test adding message with empty session ID (error case)
  - [x] 3.4 Test adding nil message (error case)
  - [x] 3.5 Test adding message to non-existent session (error case)
  - [x] 3.6 Verify coverage improvement for AddMessage function

- [x] 4. Implement EndSession tests
  - [x] 4.1 Test ending active session successfully
  - [x] 4.2 Test ending session with empty ID (error case)
  - [x] 4.3 Test ending non-existent session (error case)
  - [x] 4.4 Verify duration calculation is correct
  - [x] 4.5 Verify coverage improvement for EndSession function

- [x] 5. Implement EnsureIndexes tests
  - [x] 5.1 Test successful index creation
  - [x] 5.2 Test index creation with context timeout
  - [x] 5.3 Verify all expected indexes are created
  - [x] 5.4 Verify coverage improvement for EnsureIndexes function

## Phase 2: Storage List/Query Tests

- [x] 6. Implement ListUserSessions tests
  - [x] 6.1 Test listing sessions for user with multiple sessions
  - [x] 6.2 Test listing sessions for user with no sessions
  - [x] 6.3 Test listing with empty user ID (error case)
  - [x] 6.4 Test listing with limit parameter
  - [x] 6.5 Test sessions are sorted by timestamp (descending)
  - [x] 6.6 Verify coverage improvement for ListUserSessions function

- [x] 7. Implement ListAllSessions tests
  - [x] 7.1 Test listing all sessions across multiple users
  - [x] 7.2 Test listing with no sessions in database
  - [x] 7.3 Test listing with limit parameter
  - [x] 7.4 Test sessions are sorted by timestamp (descending)
  - [x] 7.5 Verify coverage improvement for ListAllSessions function

- [x] 8. Implement ListAllSessionsWithOptions tests
  - [x] 8.1 Test filtering by user ID
  - [x] 8.2 Test filtering by time range (start_time_from, start_time_to)
  - [x] 8.3 Test filtering by admin_assisted status
  - [x] 8.4 Test filtering by active status
  - [x] 8.5 Test sorting by different fields (timestamp, tokens, user_id)
  - [x] 8.6 Test sorting order (ascending vs descending)
  - [x] 8.7 Test pagination (limit and offset)
  - [x] 8.8 Test combined filters
  - [x] 8.9 Verify coverage improvement for ListAllSessionsWithOptions function

## Phase 3: Storage Metrics Tests

- [x] 9. Create storage_metrics_test.go file
  - [x] 9.1 Set up test file structure with imports
  - [x] 9.2 Create helper function for creating test sessions with metrics data

- [x] 10. Implement GetSessionMetrics tests
  - [x] 10.1 Test metrics for single active session
  - [x] 10.2 Test metrics for multiple sessions with different states
  - [x] 10.3 Test metrics with admin-assisted sessions
  - [x] 10.4 Test metrics calculation (avg response time, max response time)
  - [x] 10.5 Test concurrent session tracking
  - [x] 10.6 Test with invalid time range (end before start)
  - [x] 10.7 Test with empty time range (no sessions)
  - [x] 10.8 Verify coverage improvement for GetSessionMetrics function

- [x] 11. Implement GetTokenUsage tests
  - [x] 11.1 Test token usage calculation for multiple sessions
  - [x] 11.2 Test token usage with time range filter
  - [x] 11.3 Test with no sessions in time range (zero tokens)
  - [x] 11.4 Test with invalid time range (error case)
  - [x] 11.5 Verify coverage improvement for GetTokenUsage function

- [x] 12. Run storage coverage verification
  - [x] 12.1 Run `go test -cover ./internal/storage`
  - [x] 12.2 Generate HTML coverage report
  - [x] 12.3 Verify coverage is ≥ 80%
  - [x] 12.4 Document any remaining uncovered lines

## Phase 4: Chatbox Handler Tests

- [x] 13. Create chatbox_handlers_test.go file
  - [x] 13.1 Set up test file structure with imports
  - [x] 13.2 Create helper function for creating test HTTP request
  - [x] 13.3 Create helper function for creating mock JWT claims
  - [x] 13.4 Create helper function for setting up test storage with data

- [x] 14. Implement handleUserSessions tests
  - [x] 14.1 Test successful session listing for authenticated user
  - [x] 14.2 Test with user having no sessions
  - [x] 14.3 Test without authentication (error case)
  - [x] 14.4 Test with invalid claims in context (error case)
  - [x] 14.5 Test with storage error (error case)
  - [x] 14.6 Verify response format and status codes
  - [x] 14.7 Verify coverage improvement for handleUserSessions function

- [x] 15. Implement handleListSessions tests
  - [x] 15.1 Test listing all sessions with default parameters
  - [x] 15.2 Test with user_id filter
  - [x] 15.3 Test with status filter (active/ended)
  - [x] 15.4 Test with admin_assisted filter
  - [x] 15.5 Test with time range filters
  - [x] 15.6 Test with sorting parameters
  - [x] 15.7 Test with pagination (limit/offset)
  - [x] 15.8 Test with invalid time format (error case)
  - [x] 15.9 Test with storage error (error case)
  - [x] 15.10 Verify coverage improvement for handleListSessions function

- [x] 16. Implement handleGetMetrics tests
  - [x] 16.1 Test metrics retrieval with default time range
  - [x] 16.2 Test with custom time range
  - [x] 16.3 Test with invalid start_time format (error case)
  - [x] 16.4 Test with invalid end_time format (error case)
  - [x] 16.5 Test with storage error (error case)
  - [x] 16.6 Verify response includes metrics and time range
  - [x] 16.7 Verify coverage improvement for handleGetMetrics function

- [x] 17. Implement handleAdminTakeover tests
  - [x] 17.1 Test successful admin takeover
  - [x] 17.2 Test with empty session ID (error case)
  - [x] 17.3 Test without authentication (error case)
  - [x] 17.4 Test with invalid claims (error case)
  - [x] 17.5 Test with router error (error case)
  - [x] 17.6 Verify response format
  - [x] 17.7 Verify coverage improvement for handleAdminTakeover function

## Phase 5: Chatbox Register Tests

- [x] 18. Create chatbox_register_test.go file
  - [x] 18.1 Set up test file structure with imports
  - [x] 18.2 Create helper function for creating test config
  - [x] 18.3 Create helper function for setting up test environment variables
  - [x] 18.4 Create helper function for creating test MongoDB instance

- [x] 19. Implement Register function tests
  - [x] 19.1 Test successful registration with valid configuration
  - [x] 19.2 Test with missing JWT secret (error case)
  - [x] 19.3 Test with weak JWT secret (error case)
  - [x] 19.4 Test with invalid encryption key length (error case)
  - [x] 19.5 Test with missing MongoDB connection (error case)
  - [x] 19.6 Test with invalid reconnect timeout format (error case)
  - [x] 19.7 Test with invalid max message size (error case)
  - [x] 19.8 Test route registration (verify all routes exist)
  - [x] 19.9 Test CORS configuration
  - [x] 19.10 Test path prefix configuration
  - [x] 19.11 Verify coverage improvement for Register function

## Phase 6: Partial Coverage Improvements

- [x] 20. Improve CreateSession coverage (80% → 100%)
  - [x] 20.1 Identify uncovered lines using coverage report
  - [x] 20.2 Add tests for uncovered error paths
  - [x] 20.3 Verify 100% coverage for CreateSession

- [x] 21. Improve GetSession coverage (73.3% → 100%)
  - [x] 21.1 Identify uncovered lines using coverage report
  - [x] 21.2 Add tests for uncovered error paths
  - [x] 21.3 Verify 100% coverage for GetSession

- [x] 22. Improve encrypt coverage (81.2% → 100%)
  - [x] 22.1 Identify uncovered lines using coverage report
  - [x] 22.2 Add tests for uncovered error paths
  - [x] 22.3 Verify 100% coverage for encrypt

- [x] 23. Improve decrypt coverage (89.5% → 100%)
  - [x] 23.1 Identify uncovered lines using coverage report
  - [x] 23.2 Add tests for uncovered error paths
  - [x] 23.3 Verify 100% coverage for decrypt

- [x] 24. Improve handleReadyCheck coverage (57.1% → 100%)
  - [x] 24.1 Identify uncovered lines using coverage report
  - [x] 24.2 Add tests for MongoDB connection failure case
  - [x] 24.3 Add tests for nil MongoDB case
  - [x] 24.4 Verify 100% coverage for handleReadyCheck

- [x] 25. Improve Shutdown coverage (61.1% → 100%)
  - [x] 25.1 Identify uncovered lines using coverage report
  - [x] 25.2 Add tests for shutdown with all components initialized
  - [x] 25.3 Add tests for shutdown with nil components
  - [x] 25.4 Add tests for shutdown with context deadline
  - [x] 25.5 Verify 100% coverage for Shutdown

## Phase 7: Final Verification

- [x] 26. Run comprehensive coverage tests
  - [x] 26.1 Run `go test -cover ./internal/storage` and verify ≥ 80%
  - [x] 26.2 Run `go test -cover .` and verify chatbox.go ≥ 80%
  - [x] 26.3 Generate HTML coverage reports for both files
  - [x] 26.4 Review coverage reports for any remaining gaps

- [x] 27. Verify all tests pass
  - [x] 27.1 Run `go test ./internal/storage` without cache
  - [x] 27.2 Run `go test .` without cache
  - [x] 27.3 Verify no test failures or flaky tests
  - [x] 27.4 Verify test execution time is reasonable (< 60 seconds)

- [x] 28. Update CI/CD pipeline
  - [x] 28.1 Verify coverage checks pass in CI
  - [x] 28.2 Update coverage thresholds if needed
  - [x] 28.3 Document any CI-specific configuration

- [x] 29. Documentation
  - [x] 29.1 Update test documentation with new test files
  - [x] 29.2 Document any test helpers or utilities created
  - [x] 29.3 Create summary report of coverage improvements

- [-] 30. Final cleanup
  - [x] 30.1 Remove any temporary test files or debug code
  - [x] 30.2 Verify code formatting and linting
  - [-] 30.3 Commit all changes with descriptive message
