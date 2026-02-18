# Storage and Chatbox Coverage Boost - Tasks

## Phase 1: Storage Unit Tests (CRUD Operations)

- [ ] 1. Create storage_unit_test.go file
  - [ ] 1.1 Set up test file structure with imports and helper functions
  - [ ] 1.2 Create helper function for test storage service setup
  - [ ] 1.3 Create helper function for test session creation
  - [ ] 1.4 Create helper function for test data cleanup

- [ ] 2. Implement UpdateSession tests
  - [ ] 2.1 Test successful session update
  - [ ] 2.2 Test update with nil session (error case)
  - [ ] 2.3 Test update with empty session ID (error case)
  - [ ] 2.4 Test update for non-existent session (error case)
  - [ ] 2.5 Verify coverage improvement for UpdateSession function

- [ ] 3. Implement AddMessage tests
  - [ ] 3.1 Test adding message to existing session
  - [ ] 3.2 Test adding message with encryption enabled
  - [ ] 3.3 Test adding message with empty session ID (error case)
  - [ ] 3.4 Test adding nil message (error case)
  - [ ] 3.5 Test adding message to non-existent session (error case)
  - [ ] 3.6 Verify coverage improvement for AddMessage function

- [ ] 4. Implement EndSession tests
  - [ ] 4.1 Test ending active session successfully
  - [ ] 4.2 Test ending session with empty ID (error case)
  - [ ] 4.3 Test ending non-existent session (error case)
  - [ ] 4.4 Verify duration calculation is correct
  - [ ] 4.5 Verify coverage improvement for EndSession function

- [ ] 5. Implement EnsureIndexes tests
  - [ ] 5.1 Test successful index creation
  - [ ] 5.2 Test index creation with context timeout
  - [ ] 5.3 Verify all expected indexes are created
  - [ ] 5.4 Verify coverage improvement for EnsureIndexes function

## Phase 2: Storage List/Query Tests

- [ ] 6. Implement ListUserSessions tests
  - [ ] 6.1 Test listing sessions for user with multiple sessions
  - [ ] 6.2 Test listing sessions for user with no sessions
  - [ ] 6.3 Test listing with empty user ID (error case)
  - [ ] 6.4 Test listing with limit parameter
  - [ ] 6.5 Test sessions are sorted by timestamp (descending)
  - [ ] 6.6 Verify coverage improvement for ListUserSessions function

- [ ] 7. Implement ListAllSessions tests
  - [ ] 7.1 Test listing all sessions across multiple users
  - [ ] 7.2 Test listing with no sessions in database
  - [ ] 7.3 Test listing with limit parameter
  - [ ] 7.4 Test sessions are sorted by timestamp (descending)
  - [ ] 7.5 Verify coverage improvement for ListAllSessions function

- [ ] 8. Implement ListAllSessionsWithOptions tests
  - [ ] 8.1 Test filtering by user ID
  - [ ] 8.2 Test filtering by time range (start_time_from, start_time_to)
  - [ ] 8.3 Test filtering by admin_assisted status
  - [ ] 8.4 Test filtering by active status
  - [ ] 8.5 Test sorting by different fields (timestamp, tokens, user_id)
  - [ ] 8.6 Test sorting order (ascending vs descending)
  - [ ] 8.7 Test pagination (limit and offset)
  - [ ] 8.8 Test combined filters
  - [ ] 8.9 Verify coverage improvement for ListAllSessionsWithOptions function

## Phase 3: Storage Metrics Tests

- [ ] 9. Create storage_metrics_test.go file
  - [ ] 9.1 Set up test file structure with imports
  - [ ] 9.2 Create helper function for creating test sessions with metrics data

- [ ] 10. Implement GetSessionMetrics tests
  - [ ] 10.1 Test metrics for single active session
  - [ ] 10.2 Test metrics for multiple sessions with different states
  - [ ] 10.3 Test metrics with admin-assisted sessions
  - [ ] 10.4 Test metrics calculation (avg response time, max response time)
  - [ ] 10.5 Test concurrent session tracking
  - [ ] 10.6 Test with invalid time range (end before start)
  - [ ] 10.7 Test with empty time range (no sessions)
  - [ ] 10.8 Verify coverage improvement for GetSessionMetrics function

- [ ] 11. Implement GetTokenUsage tests
  - [ ] 11.1 Test token usage calculation for multiple sessions
  - [ ] 11.2 Test token usage with time range filter
  - [ ] 11.3 Test with no sessions in time range (zero tokens)
  - [ ] 11.4 Test with invalid time range (error case)
  - [ ] 11.5 Verify coverage improvement for GetTokenUsage function

- [ ] 12. Run storage coverage verification
  - [ ] 12.1 Run `go test -cover ./internal/storage`
  - [ ] 12.2 Generate HTML coverage report
  - [ ] 12.3 Verify coverage is ≥ 80%
  - [ ] 12.4 Document any remaining uncovered lines

## Phase 4: Chatbox Handler Tests

- [ ] 13. Create chatbox_handlers_test.go file
  - [ ] 13.1 Set up test file structure with imports
  - [ ] 13.2 Create helper function for creating test HTTP request
  - [ ] 13.3 Create helper function for creating mock JWT claims
  - [ ] 13.4 Create helper function for setting up test storage with data

- [ ] 14. Implement handleUserSessions tests
  - [ ] 14.1 Test successful session listing for authenticated user
  - [ ] 14.2 Test with user having no sessions
  - [ ] 14.3 Test without authentication (error case)
  - [ ] 14.4 Test with invalid claims in context (error case)
  - [ ] 14.5 Test with storage error (error case)
  - [ ] 14.6 Verify response format and status codes
  - [ ] 14.7 Verify coverage improvement for handleUserSessions function

- [ ] 15. Implement handleListSessions tests
  - [ ] 15.1 Test listing all sessions with default parameters
  - [ ] 15.2 Test with user_id filter
  - [ ] 15.3 Test with status filter (active/ended)
  - [ ] 15.4 Test with admin_assisted filter
  - [ ] 15.5 Test with time range filters
  - [ ] 15.6 Test with sorting parameters
  - [ ] 15.7 Test with pagination (limit/offset)
  - [ ] 15.8 Test with invalid time format (error case)
  - [ ] 15.9 Test with storage error (error case)
  - [ ] 15.10 Verify coverage improvement for handleListSessions function

- [ ] 16. Implement handleGetMetrics tests
  - [ ] 16.1 Test metrics retrieval with default time range
  - [ ] 16.2 Test with custom time range
  - [ ] 16.3 Test with invalid start_time format (error case)
  - [ ] 16.4 Test with invalid end_time format (error case)
  - [ ] 16.5 Test with storage error (error case)
  - [ ] 16.6 Verify response includes metrics and time range
  - [ ] 16.7 Verify coverage improvement for handleGetMetrics function

- [ ] 17. Implement handleAdminTakeover tests
  - [ ] 17.1 Test successful admin takeover
  - [ ] 17.2 Test with empty session ID (error case)
  - [ ] 17.3 Test without authentication (error case)
  - [ ] 17.4 Test with invalid claims (error case)
  - [ ] 17.5 Test with router error (error case)
  - [ ] 17.6 Verify response format
  - [ ] 17.7 Verify coverage improvement for handleAdminTakeover function

## Phase 5: Chatbox Register Tests

- [ ] 18. Create chatbox_register_test.go file
  - [ ] 18.1 Set up test file structure with imports
  - [ ] 18.2 Create helper function for creating test config
  - [ ] 18.3 Create helper function for setting up test environment variables
  - [ ] 18.4 Create helper function for creating test MongoDB instance

- [ ] 19. Implement Register function tests
  - [ ] 19.1 Test successful registration with valid configuration
  - [ ] 19.2 Test with missing JWT secret (error case)
  - [ ] 19.3 Test with weak JWT secret (error case)
  - [ ] 19.4 Test with invalid encryption key length (error case)
  - [ ] 19.5 Test with missing MongoDB connection (error case)
  - [ ] 19.6 Test with invalid reconnect timeout format (error case)
  - [ ] 19.7 Test with invalid max message size (error case)
  - [ ] 19.8 Test route registration (verify all routes exist)
  - [ ] 19.9 Test CORS configuration
  - [ ] 19.10 Test path prefix configuration
  - [ ] 19.11 Verify coverage improvement for Register function

## Phase 6: Partial Coverage Improvements

- [ ] 20. Improve CreateSession coverage (80% → 100%)
  - [ ] 20.1 Identify uncovered lines using coverage report
  - [ ] 20.2 Add tests for uncovered error paths
  - [ ] 20.3 Verify 100% coverage for CreateSession

- [ ] 21. Improve GetSession coverage (73.3% → 100%)
  - [ ] 21.1 Identify uncovered lines using coverage report
  - [ ] 21.2 Add tests for uncovered error paths
  - [ ] 21.3 Verify 100% coverage for GetSession

- [ ] 22. Improve encrypt coverage (81.2% → 100%)
  - [ ] 22.1 Identify uncovered lines using coverage report
  - [ ] 22.2 Add tests for uncovered error paths
  - [ ] 22.3 Verify 100% coverage for encrypt

- [ ] 23. Improve decrypt coverage (89.5% → 100%)
  - [ ] 23.1 Identify uncovered lines using coverage report
  - [ ] 23.2 Add tests for uncovered error paths
  - [ ] 23.3 Verify 100% coverage for decrypt

- [ ] 24. Improve handleReadyCheck coverage (57.1% → 100%)
  - [ ] 24.1 Identify uncovered lines using coverage report
  - [ ] 24.2 Add tests for MongoDB connection failure case
  - [ ] 24.3 Add tests for nil MongoDB case
  - [ ] 24.4 Verify 100% coverage for handleReadyCheck

- [ ] 25. Improve Shutdown coverage (61.1% → 100%)
  - [ ] 25.1 Identify uncovered lines using coverage report
  - [ ] 25.2 Add tests for shutdown with all components initialized
  - [ ] 25.3 Add tests for shutdown with nil components
  - [ ] 25.4 Add tests for shutdown with context deadline
  - [ ] 25.5 Verify 100% coverage for Shutdown

## Phase 7: Final Verification

- [ ] 26. Run comprehensive coverage tests
  - [ ] 26.1 Run `go test -cover ./internal/storage` and verify ≥ 80%
  - [ ] 26.2 Run `go test -cover .` and verify chatbox.go ≥ 80%
  - [ ] 26.3 Generate HTML coverage reports for both files
  - [ ] 26.4 Review coverage reports for any remaining gaps

- [ ] 27. Verify all tests pass
  - [ ] 27.1 Run `go test ./internal/storage` without cache
  - [ ] 27.2 Run `go test .` without cache
  - [ ] 27.3 Verify no test failures or flaky tests
  - [ ] 27.4 Verify test execution time is reasonable (< 60 seconds)

- [ ] 28. Update CI/CD pipeline
  - [ ] 28.1 Verify coverage checks pass in CI
  - [ ] 28.2 Update coverage thresholds if needed
  - [ ] 28.3 Document any CI-specific configuration

- [ ] 29. Documentation
  - [ ] 29.1 Update test documentation with new test files
  - [ ] 29.2 Document any test helpers or utilities created
  - [ ] 29.3 Create summary report of coverage improvements

- [ ] 30. Final cleanup
  - [ ] 30.1 Remove any temporary test files or debug code
  - [ ] 30.2 Verify code formatting and linting
  - [ ] 30.3 Commit all changes with descriptive message
