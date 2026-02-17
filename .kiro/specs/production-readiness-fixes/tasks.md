

# Implementation Plan: Production Readiness Fixes

## Overview

This plan implements 10 confirmed production readiness fixes, prioritized by severity. Each task is focused on a specific issue with clear implementation steps. The fixes are designed to be independently deployable with minimal risk.

Priority order:
1. CRITICAL issues (#1, #13) - Memory leak and data race
2. HIGH priority (#8, #11, #17, #18) - Timeouts, cleanup, security
3. MEDIUM priority (#12, #19, #9) - Bounded growth, validation, retries
4. LOW priority (#15) - Shutdown improvements

## Tasks

- [x] 1. Fix Issue #13: Origin Validation Data Race (CRITICAL)
  - Add RLock/RUnlock to checkOrigin() method in internal/websocket/handler.go
  - This is a one-line fix that prevents server crashes
  - _Requirements: 13.1, 13.2, 13.4, 13.5_

- [x] 1.1 Write property test for origin validation thread safety
  - **Property 14: Origin validation is thread-safe**
  - **Validates: Requirements 13.1, 13.2, 13.4, 13.5**
  - Test concurrent calls to checkOrigin() and SetAllowedOrigins() with -race flag

- [ ] 2. Fix Issue #1: Session Memory Leak (CRITICAL)
  - [x] 2.1 Add cleanup fields to SessionManager struct
    - Add cleanupInterval, sessionTTL, stopCleanup channel, cleanupWg
    - _Requirements: 1.1, 1.2_
  
  - [x] 2.2 Implement StartCleanup() method
    - Create background goroutine with ticker
    - Call cleanupExpiredSessions() periodically
    - _Requirements: 1.1, 1.2_
  
  - [x] 2.3 Implement cleanupExpiredSessions() method
    - Remove sessions where IsActive=false and EndTime > TTL
    - Log cleanup statistics
    - _Requirements: 1.1, 1.2, 1.5_
  
  - [x] 2.4 Implement StopCleanup() method
    - Close stopCleanup channel and wait for goroutine
    - _Requirements: 1.1_
  
  - [x] 2.5 Implement GetMemoryStats() method
    - Return active, inactive, and total session counts
    - _Requirements: 1.4_
  
  - [x] 2.6 Update NewSessionManager to initialize cleanup fields
    - Set default cleanupInterval (5 minutes) and sessionTTL (15 minutes)
    - _Requirements: 1.2_
  
  - [x] 2.7 Call StartCleanup() in main.go after creating SessionManager
    - Add defer StopCleanup() for graceful shutdown
    - _Requirements: 1.1_

- [x] 2.8 Write property test for session cleanup
  - **Property 1: Session cleanup removes expired sessions**
  - **Validates: Requirements 1.1, 1.2, 1.5**
  - Test that expired sessions are removed after TTL

- [x] 2.9 Write property test for active session preservation
  - **Property 2: Active sessions are never cleaned up**
  - **Validates: Requirements 1.1**
  - Test that active sessions remain in memory during cleanup

- [x] 2.10 Write unit test for GetMemoryStats accuracy
  - **Property 3: Memory stats are accurate**
  - **Validates: Requirements 1.4**
  - Test that counts sum to total sessions


- [ ] 3. Fix Issue #8: LLM Streaming Timeout (HIGH)
  - [x] 3.1 Add LLMStreamTimeout to ServerConfig struct
    - Add field to config.go with default value (120s)
    - Add environment variable parsing
    - _Requirements: 8.3_
  
  - [x] 3.2 Update HandleUserMessage to use context with timeout
    - Replace context.Background() with context.WithTimeout()
    - Add defer cancel()
    - _Requirements: 8.1, 8.2_
  
  - [x] 3.3 Add timeout error handling and logging
    - Check for context.DeadlineExceeded
    - Log timeout events with session ID and elapsed time
    - Return appropriate error to client
    - _Requirements: 8.4, 8.5_

- [x] 3.4 Write property test for streaming timeout
  - **Property 4: Streaming requests have timeout**
  - **Validates: Requirements 8.1, 8.3**
  - Test that all streaming requests have context deadline

- [x] 3.5 Write property test for timeout cancellation
  - **Property 5: Timeout cancels streaming**
  - **Validates: Requirements 8.2, 8.5**
  - Test with mock LLM that hangs, verify cancellation

- [ ] 4. Fix Issue #11: Rate Limiter Memory Growth (HIGH)
  - [x] 4.1 Add cleanup fields to MessageLimiter struct
    - Add cleanupInterval, stopCleanup channel, cleanupWg
    - _Requirements: 11.1, 11.2_
  
  - [x] 4.2 Implement StartCleanup() method for MessageLimiter
    - Create background goroutine with ticker
    - Call existing Cleanup() method periodically
    - Log cleanup statistics
    - _Requirements: 11.1, 11.3, 11.4_
  
  - [x] 4.3 Implement getEventCount() helper method
    - Count total events across all users
    - Used for cleanup statistics
    - _Requirements: 11.4_
  
  - [x] 4.4 Implement StopCleanup() method for MessageLimiter
    - Close stopCleanup channel and wait for goroutine
    - _Requirements: 11.5_
  
  - [x] 4.5 Update NewMessageLimiter to initialize cleanup fields
    - Set default cleanupInterval (5 minutes)
    - _Requirements: 11.2_
  
  - [x] 4.6 Call StartCleanup() in main.go after creating MessageLimiter
    - Add defer StopCleanup() for graceful shutdown
    - _Requirements: 11.1_

- [x] 4.7 Write property test for cleanup removes old events
  - **Property 9: Cleanup removes old events**
  - **Validates: Requirements 11.3**
  - Test that events older than window are removed

- [x] 4.8 Write property test for periodic cleanup
  - **Property 10: Cleanup runs periodically**
  - **Validates: Requirements 11.1, 11.2**
  - Test that cleanup is called at configured interval

- [x] 4.9 Write property test for cleanup goroutine termination
  - **Property 11: Cleanup goroutine terminates**
  - **Validates: Requirements 11.5**
  - Test that StopCleanup() terminates goroutine


- [ ] 5. Fix Issue #17: JWT Secret Validation (HIGH - Security)
  - [x] 5.1 Add weak secret list to config.go
    - Define common weak secrets array
    - _Requirements: 17.3_
  
  - [x] 5.2 Add JWT secret validation to Config.Validate()
    - Check minimum length (32 characters)
    - Check for weak patterns
    - Provide helpful error messages with generation guidance
    - _Requirements: 17.1, 17.2, 17.3, 17.5_

- [x] 5.3 Write property test for weak secret rejection
  - **Property 17: Weak secrets are rejected**
  - **Validates: Requirements 17.1, 17.2, 17.3**
  - Test secrets shorter than 32 chars and with weak patterns

- [x] 5.4 Write property test for strong secret acceptance
  - **Property 18: Strong secrets are accepted**
  - **Validates: Requirements 17.1, 17.2, 17.3**
  - Test valid secrets pass validation

- [ ] 6. Fix Issue #18: Admin Endpoint Rate Limiting (HIGH - Security)
  - [x] 6.1 Add admin rate limit config to ServerConfig
    - Add AdminRateLimit and AdminRateWindow fields
    - Add environment variable parsing with defaults (20 req/min)
    - _Requirements: 18.2_
  
  - [x] 6.2 Create admin rate limiter in main.go
    - Instantiate separate MessageLimiter for admin endpoints
    - Call StartCleanup() on admin limiter
    - _Requirements: 18.1, 18.5_
  
  - [x] 6.3 Implement adminRateLimit middleware function
    - Extract user ID from JWT token
    - Check admin rate limiter
    - Return HTTP 429 with Retry-After header if exceeded
    - Log rate limit violations
    - _Requirements: 18.1, 18.3, 18.4_
  
  - [x] 6.4 Apply middleware to all admin endpoints
    - Wrap /admin/sessions, /admin/metrics, /admin/users, etc.
    - _Requirements: 18.1_

- [x] 6.5 Write property test for admin rate limit enforcement
  - **Property 19: Admin endpoints enforce rate limits**
  - **Validates: Requirements 18.1, 18.3**
  - Test rapid requests return HTTP 429

- [x] 6.6 Write property test for independent rate limits
  - **Property 20: Admin and user limits are independent**
  - **Validates: Requirements 18.5**
  - Test that admin and user limits don't interfere

- [x] 7. Checkpoint - Ensure all tests pass
  - Run all tests with -race flag
  - Verify no data races or memory leaks
  - Ensure all critical and high priority fixes are working


- [ ] 8. Fix Issue #12: ResponseTimes Unbounded Growth (MEDIUM)
  - [x] 8.1 Define MaxResponseTimes constant
    - Add constant to session.go (value: 100)
    - _Requirements: 12.3_
  
  - [x] 8.2 Update RecordResponseTime to implement rolling window
    - Check if slice is at max size
    - Remove oldest entry before adding new one
    - Ensure slice never exceeds max size
    - _Requirements: 12.1, 12.2, 12.4, 12.5_

- [x] 8.3 Write property test for bounded slice
  - **Property 12: ResponseTimes slice is bounded**
  - **Validates: Requirements 12.1, 12.5**
  - Test that length never exceeds MaxResponseTimes

- [x] 8.4 Write property test for rolling window
  - **Property 13: Rolling window maintains recent times**
  - **Validates: Requirements 12.2, 12.4**
  - Test that oldest time is removed when adding to full slice

- [ ] 9. Fix Issue #19: Configuration Validation (MEDIUM)
  - [x] 9.1 Add explicit validation call in main.go
    - Call cfg.Validate() after config.Load()
    - Log fatal error if validation fails
    - _Requirements: 19.1, 19.2, 19.3_

- [x] 9.2 Write property test for invalid config rejection
  - **Property 21: Invalid config prevents startup**
  - **Validates: Requirements 19.2, 19.4, 19.5**
  - Test various invalid configurations fail validation

- [x] 9.3 Write property test for valid config acceptance
  - **Property 22: Valid config passes validation**
  - **Validates: Requirements 19.4, 19.5**
  - Test valid configurations pass validation

- [ ] 10. Fix Issue #9: MongoDB Retry Logic (MEDIUM)
  - [x] 10.1 Add retry configuration to config.go
    - Add MongoRetryAttempts, MongoRetryDelay fields
    - Add environment variable parsing with defaults
    - _Requirements: 9.3_
  
  - [x] 10.2 Implement retryConfig struct and defaultRetryConfig
    - Define retry parameters (maxAttempts, delays, multiplier)
    - _Requirements: 9.2, 9.3_
  
  - [x] 10.3 Implement isRetryableError() helper function
    - Check for network errors and transient MongoDB errors
    - Return false for permanent errors
    - _Requirements: 9.1_
  
  - [x] 10.4 Implement retryOperation() method
    - Implement retry loop with exponential backoff
    - Log retry attempts with error details
    - Return error after max attempts exhausted
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5_
  
  - [x] 10.5 Update CreateSession to use retryOperation
    - Wrap InsertOne call with retry logic
    - _Requirements: 9.1_
  
  - [x] 10.6 Update other storage methods to use retryOperation
    - Apply to UpdateSession, GetSession, etc.
    - _Requirements: 9.1_

- [x] 10.7 Write property test for transient error retry
  - **Property 6: Transient errors are retried**
  - **Validates: Requirements 9.1, 9.3**
  - Test with mock MongoDB that fails transiently

- [x] 10.8 Write property test for exponential backoff
  - **Property 7: Retry uses exponential backoff**
  - **Validates: Requirements 9.2**
  - Test delay increases exponentially between attempts

- [x] 10.9 Write property test for non-transient error handling
  - **Property 8: Non-transient errors fail immediately**
  - **Validates: Requirements 9.1**
  - Test permanent errors don't trigger retries


- [ ] 11. Fix Issue #15: Shutdown Timeout (LOW)
  - [x] 11.1 Update Shutdown() method in chatbox.go
    - Collect all connections into slice
    - Close connections in parallel with goroutines
    - Wait for completion or context deadline
    - Log warning if deadline exceeded
    - _Requirements: 15.1, 15.2, 15.3, 15.4, 15.5_

- [x] 11.2 Write property test for shutdown deadline respect
  - **Property 15: Shutdown respects deadline**
  - **Validates: Requirements 15.1, 15.3**
  - Test shutdown completes or errors before deadline

- [x] 11.3 Write property test for connection closure
  - **Property 16: Shutdown closes all connections**
  - **Validates: Requirements 15.2, 15.5**
  - Test all connections are closed on successful shutdown

- [ ] 12. Integration Testing
  - [x] 12.1 Run all tests with race detector
    - Execute: go test -race ./...
    - Verify no data races detected
    - _Requirements: All_
  
  - [x] 12.2 Run memory leak tests
    - Test session cleanup over extended period
    - Test rate limiter cleanup over extended period
    - Verify memory usage stabilizes
    - _Requirements: 1.1, 11.1_
  
  - [x] 12.3 Run load tests for rate limiting
    - Test admin endpoint rate limiting under load
    - Test user endpoint rate limiting under load
    - Verify limits are enforced correctly
    - _Requirements: 18.1_
  
  - [x] 12.4 Test configuration validation
    - Test startup with invalid configurations
    - Verify server refuses to start
    - Test error messages are clear
    - _Requirements: 19.1, 19.2_

- [ ] 13. Documentation Updates
  - [x] 13.1 Update README with new configuration options
    - Document all new environment variables
    - Provide recommended values for production
    - _Requirements: All_
  
  - [x] 13.2 Update deployment documentation
    - Document cleanup goroutine behavior
    - Document rate limiting configuration
    - Document JWT secret requirements
    - _Requirements: 1.1, 11.1, 17.1, 18.1_
  
  - [x] 13.3 Create migration guide
    - Document changes from previous version
    - Provide rollback procedures
    - List breaking changes (none expected)
    - _Requirements: All_

- [x] 14. Final Checkpoint - Production Readiness Verification
  - Run complete test suite including property tests
  - Verify all 10 issues are resolved
  - Review logs for any warnings or errors
  - Confirm all configuration validation works
  - Ensure all tests pass, ask the user if questions arise

## Notes

- Tasks marked with `*` are optional property-based tests that can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties
- Unit tests validate specific examples and edge cases
- All fixes are designed to be independently deployable
- Rollback strategies are documented in the design document
- Configuration changes are backward compatible with sensible defaults

