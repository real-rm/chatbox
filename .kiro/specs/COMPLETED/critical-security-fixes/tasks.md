# Implementation Plan: Critical Security Fixes

## Overview

This implementation plan addresses four critical security and functional issues in a focused, incremental manner. Each task builds on previous work and includes validation through testing. The fixes are designed to be minimal and non-breaking while significantly improving security and reliability.

## Tasks

- [x] 1. Implement encryption key validation at startup
  - [x] 1.1 Create validateEncryptionKey function in chatbox.go
    - Add function that checks key length
    - Return error if key is not 0 or 32 bytes
    - Include actual and required lengths in error message
    - _Requirements: 1.1, 1.2, 1.3, 1.7_
  
  - [x] 1.2 Integrate validation into Register function
    - Call validateEncryptionKey before creating StorageService
    - Return error to terminate startup if validation fails
    - Keep existing warning log for empty key
    - Remove padding and truncation logic
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6_
  
  - [x] 1.3 Write unit tests for encryption key validation
    - Test valid 32-byte key
    - Test empty key (encryption disabled)
    - Test invalid lengths (16, 31, 33, 64 bytes)
    - Test error message format
    - _Requirements: 1.1, 1.2, 1.3, 1.7_
  
  - [x] 1.4 Write property test for error message completeness
    - **Property 1: Error message completeness for invalid keys**
    - **Validates: Requirements 1.7**
    - Generate random invalid key lengths
    - Verify error message contains both required and actual lengths
    - _Requirements: 1.7_

- [x] 2. Add WebSocket message size limit
  - [x] 2.1 Add configuration for message size limit
    - Add config key "chatbox.max_message_size" with default 1048576 (1MB)
    - Support environment variable MAX_MESSAGE_SIZE
    - Parse configuration value in Register function
    - _Requirements: 3.3, 3.4_
  
  - [x] 2.2 Set read limit on WebSocket connections
    - In HandleWebSocket, call conn.SetReadLimit after upgrade
    - Use configured max message size value
    - _Requirements: 3.1_
  
  - [x] 2.3 Add logging for message size limit exceeded
    - In readPump, detect message size errors
    - Log with user ID, connection ID, and limit
    - _Requirements: 3.2, 3.5, 3.6_
  
  - [x] 2.4 Write unit tests for message size limit
    - Test default limit is 1MB
    - Test custom limit from configuration
    - Test SetReadLimit is called
    - _Requirements: 3.1, 3.3, 3.4_
  
  - [x] 2.5 Write property tests for message size enforcement
    - **Property 2: Read limit enforcement**
    - **Validates: Requirements 3.1**
    - **Property 3: Oversized message rejection**
    - **Validates: Requirements 3.2**
    - **Property 4: Configuration value application**
    - **Validates: Requirements 3.3**
    - **Property 5: Oversized message logging**
    - **Validates: Requirements 3.5, 3.6**
    - _Requirements: 3.1, 3.2, 3.3, 3.5, 3.6_
  
  - [x] 2.6 Write integration test for oversized messages
    - Establish WebSocket connection
    - Send message at limit (should succeed)
    - Send message over limit (should fail)
    - Verify log entry created
    - _Requirements: 3.1, 3.2, 3.5, 3.6_

- [x] 3. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Implement session creation flow
  - [x] 4.1 Add StorageService to MessageRouter
    - Add storageService field to MessageRouter struct
    - Update NewMessageRouter to accept StorageService parameter
    - Update chatbox.go to pass StorageService when creating MessageRouter
    - _Requirements: 4.4_
  
  - [x] 4.2 Implement getOrCreateSession helper method
    - Create getOrCreateSession method in router.go
    - Try to get existing session from SessionManager
    - If not found, call createNewSession
    - Return session or error
    - _Requirements: 4.1, 4.2, 4.3_
  
  - [x] 4.3 Implement createNewSession helper method
    - Create createNewSession method in router.go
    - Call sessionManager.CreateSession with user ID
    - Call storageService.CreateSession to persist
    - Handle errors and rollback on failure
    - Return session or error
    - _Requirements: 4.1, 4.3, 4.4, 4.5, 4.6_
  
  - [x] 4.4 Update HandleUserMessage to use getOrCreateSession
    - Replace direct GetSession call with getOrCreateSession
    - Remove error handling for session not found
    - Keep other error handling
    - _Requirements: 4.1, 4.2, 4.3_
  
  - [x] 4.5 Write unit tests for session creation flow
    - Test automatic session creation for new user
    - Test existing session reuse
    - Test session creation with provided ID
    - Test dual storage (memory + database)
    - Test user ID association
    - Test error handling on database failure
    - Test rollback on partial failure
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6_
  
  - [x] 4.6 Write property tests for session creation
    - **Property 6: Automatic session creation for new users**
    - **Validates: Requirements 4.1**
    - **Property 7: Existing session reuse**
    - **Validates: Requirements 4.2**
    - **Property 8: Session creation with provided ID**
    - **Validates: Requirements 4.3**
    - **Property 9: Dual storage consistency**
    - **Validates: Requirements 4.4**
    - **Property 10: User association correctness**
    - **Validates: Requirements 4.5**
    - **Property 11: Session creation error handling**
    - **Validates: Requirements 4.6**
    - **Property 12: Concurrent session creation safety**
    - **Validates: Requirements 4.7**
    - **Property 13: Session restoration from database**
    - **Validates: Requirements 4.8**
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8_
  
  - [x] 4.7 Write integration test for session creation flow
    - Connect as new user, send message, verify session created
    - Reconnect with session ID, verify session restored
    - Send concurrent messages, verify single session
    - Simulate database failure, verify error handling
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.6, 4.7, 4.8_

- [x] 5. Create JWT token security documentation
  - [x] 5.1 Create docs/JWT_TOKEN_SECURITY.md
    - Document JWT token usage in the application
    - Document security implications of tokens in URL query parameters
    - Document that tokens appear in access logs, browser history, referrer headers, proxy logs
    - Recommend short-lived tokens (5-15 minutes) for query parameter usage
    - Document Authorization header as alternative
    - Document that WebSocket endpoint accepts both methods
    - Include monitoring and detection recommendations
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8_
  
  - [x] 5.2 Update main README.md to reference security documentation
    - Add link to JWT_TOKEN_SECURITY.md in security section
    - _Requirements: 2.8_

- [x] 6. Update configuration documentation
  - [x] 6.1 Update DEPLOYMENT.md with new configuration options
    - Document MAX_MESSAGE_SIZE environment variable
    - Document chatbox.max_message_size config key
    - Document default value (1MB)
    - Document encryption key validation requirements
    - _Requirements: 3.3, 3.4, 3.7_

- [x] 7. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties (minimum 100 iterations each)
- Unit tests validate specific examples and edge cases
- Integration tests validate end-to-end flows
- All fixes are designed to be minimal and non-breaking
- Session creation flow is the most complex change, requiring careful testing
