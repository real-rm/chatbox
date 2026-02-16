# Production Readiness Tasks

## Overview
This task list addresses all blocking and high-priority issues identified in the production readiness review to make the chatbox application production-ready.

## BLOCKING Issues (Must Fix)

- [x] 1. Wire WebSocket message processing to router
  - [x] 1.1 Connect readPump to message router in handler.go
  - [x] 1.2 Add proper error handling for routing failures
  - [x] 1.3 Test end-to-end message flow
  - _Issue: Messages received but never processed_

- [x] 2. Connect LLM service to message router
  - [x] 2.1 Replace echo placeholder with actual LLM service call
  - [x] 2.2 Implement streaming response forwarding
  - [x] 2.3 Add error handling for LLM failures
  - [x] 2.4 Test with all three LLM providers
  - _Issue: LLM integration is stubbed_

- [x] 3. Fix WebSocket CheckOrigin security
  - [x] 3.1 Implement proper origin validation
  - [x] 3.2 Add configuration for allowed origins
  - [x] 3.3 Add tests for origin validation
  - _Issue: CSRF/WebSocket hijacking vulnerability_

- [x] 4. Fix single connection per user bug
  - [x] 4.1 Change connections map to support multiple connections per user
  - [x] 4.2 Implement connection ID generation
  - [x] 4.3 Add graceful handling of duplicate connections
  - [x] 4.4 Notify user when connection is replaced
  - [x] 4.5 Test multi-device scenarios
  - _Issue: Second connection overwrites first, causing data loss_

- [x] 5. Fix LLM property test timeout
  - [x] 5.1 Reduce test iteration count or timeout values
  - [x] 5.2 Optimize retry logic test parameters
  - [x] 5.3 Verify test passes consistently
  - _Issue: Test suite fails with timeout_

- [x] 6. Remove local replace directives from go.mod
  - [x] 6.1 Use company private github registry from system shell
  - [x] 6.2 Update go.mod to use latest versions
  - [x] 6.3 Verify Docker build works
  - [x] 6.4 Test in CI environment
  - _Issue: Project cannot be built by others_

## HIGH Priority Issues

- [x] 7. Implement admin sessions endpoint
  - [x] 7.1 Add storage method to list all sessions
  - [x] 7.2 Implement pagination for large result sets
  - [x] 7.3 Add filtering and sorting
  - [x] 7.4 Test with large datasets
  - _Issue: Admin dashboard core feature unimplemented_

- [x] 8. Implement proper MongoDB health check
  - [x] 8.1 Add Ping() call to readiness probe
  - [x] 8.2 Add timeout for health check
  - [x] 8.3 Test with MongoDB down scenario
  - _Issue: Readiness probe doesn't check database connectivity_

- [x] 9. Sanitize error messages
  - [x] 9.1 Create generic error messages for clients
  - [x] 9.2 Log detailed errors server-side only
  - [x] 9.3 Review all error responses
  - _Issue: Internal details leaked to clients_

- [x] 10. Enable message encryption
  - [x] 10.1 Generate and configure encryption key
  - [x] 10.2 Pass encryption key to storage service
  - [x] 10.3 Test encryption/decryption round-trip
  - [x] 10.4 Document key management
  - _Issue: Encryption code exists but not activated_

- [x] 11. Replace bubble sort with efficient sorting
  - [x] 11.1 Use Go's sort.Slice with proper comparators
  - [x] 11.2 Benchmark performance improvement
  - [x] 11.3 Test with large datasets
  - _Issue: O(n²) algorithm in production path_

- [x] 12. Fix WebSocket message batching
  - [x] 12.1 Send each message as separate WebSocket frame
  - [x] 12.2 Remove newline concatenation
  - [x] 12.3 Test client-side parsing
  - _Issue: Multiple JSON messages concatenated incorrectly_

## MEDIUM Priority Issues

- [x] 13. Add Prometheus metrics endpoint
  - [x] 13.1 Add /metrics endpoint
  - [x] 13.2 Expose application-level metrics
  - [x] 13.3 Update HPA configuration
  - _Issue: No application metrics for monitoring_

- [x] 14. Implement proper secret management
  - [x] 14.1 Remove secrets from config.toml
  - [x] 14.2 Use Kubernetes secrets or external secret manager
  - [x] 14.3 Document secret setup process
  - _Issue: Secrets in source control_

- [x] 15. Add MongoDB indexes
  - [x] 15.1 Create indexes for uid, ts, adminAssisted
  - [x] 15.2 Add index creation to deployment process
  - [x] 15.3 Document index strategy
  - _Issue: Queries will do collection scans_

- [x] 16. Add CORS configuration
  - [x] 16.1 Add CORS middleware to Gin router
  - [x] 16.2 Configure allowed origins
  - [x] 16.3 Test cross-origin requests
  - _Issue: Frontend can't call admin endpoints from different origin_

- [x] 17. Extract admin name from JWT
  - [x] 17.1 Add admin name extraction in takeover handler
  - [x] 17.2 Pass admin name to session
  - [x] 17.3 Display admin name in UI
  - _Issue: Admin takeover uses empty admin name_

- [x] 18. Update MongoDB field naming conventions
  - [x] 18.1 Update BSON tags to use camelCase and abbreviations
  - [x] 18.2 Update query filters and sort field mappings
  - [x] 18.3 Test MongoDB operations with new field names
  - _Issue: MongoDB fields use snake_case instead of camelCase convention_

## Notes
- All blocking issues must be resolved before production deployment
- High priority issues should be addressed for production readiness
- Medium priority issues can be addressed post-launch but should be prioritized
