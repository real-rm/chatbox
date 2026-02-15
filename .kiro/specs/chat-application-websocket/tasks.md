# Implementation Plan: Chat Application WebSocket

## Overview

This implementation plan breaks down the chat application into discrete coding tasks. The approach follows TDD principles, building incrementally from core infrastructure to complete features. The backend is implemented in Go, leveraging existing API packages (gomongo, goupload, goconfig, golog, gohelper, gomail, gosms). The frontend is a lightweight HTML/JavaScript interface.

## Tasks

- [x] 1. Set up project structure and configuration
  - Create Go module and directory structure (cmd, internal, pkg)
  - Set up configuration loading using goconfig package
  - Create configuration structs for server, LLM, database, storage, notifications
  - Implement configuration validation
  - Create Kubernetes ConfigMap and Secret templates
  - _Requirements: 7.1, 19.1, 19.3_

- [x] 1.1 Write unit tests for configuration loading and validation
  - Test configuration loading from environment variables
  - Test configuration loading from Kubernetes ConfigMaps
  - Test configuration validation for required fields
  - _Requirements: 7.1_

- [ ] 2. Implement JWT authentication middleware
  - [x] 2.1 Create JWT token validation function
    - Implement token signature verification
    - Implement token expiration checking
    - Extract user ID and roles from claims
    - _Requirements: 1.1, 1.2, 1.3_
  
  - [x] 2.2 Write property test for JWT validation
    - **Property 1: JWT Token Validation**
    - **Validates: Requirements 1.1, 1.3**
  
  - [x] 2.3 Write property test for JWT claims extraction
    - **Property 2: JWT Claims Extraction Round Trip**
    - **Validates: Requirements 1.2**

- [ ] 3. Implement WebSocket connection handling
  - [x] 3.1 Create WebSocket server and connection upgrade handler
    - Implement HTTP to WebSocket upgrade
    - Create Connection struct with user context
    - Implement connection authentication using JWT middleware
    - _Requirements: 1.4, 2.1_
  
  - [x] 3.2 Implement connection lifecycle management
    - Implement readPump for receiving messages
    - Implement writePump for sending messages
    - Implement ping/pong heartbeat handling
    - Implement graceful connection closure and resource cleanup
    - _Requirements: 2.2, 2.6_
  
  - [x] 3.3 Write property tests for connection management
    - **Property 3: Connection User Association**
    - **Property 4: WebSocket Connection Establishment**
    - **Property 5: Heartbeat Response**
    - **Property 7: Connection Resource Cleanup**
    - **Validates: Requirements 1.4, 2.1, 2.2, 2.6**

- [ ] 4. Implement session management
  - [x] 4.1 Create SessionManager with session tracking
    - Implement Session struct with all required fields
    - Implement CreateSession method
    - Implement GetSession and RestoreSession methods
    - Implement session timeout logic (configurable, default 15 min)
    - Implement user-to-session mapping (single active session per user)
    - _Requirements: 2.4, 2.5, 2.8, 4.1, 15.4_
  
  - [x] 4.2 Implement session name generation
    - Create function to generate descriptive names from first message
    - Use gohelper for text processing
    - _Requirements: 15.5_
  
  - [x] 4.3 Implement session metrics tracking
    - Track start time, end time, duration
    - Track response times (max, average)
    - Track token usage
    - _Requirements: 3.7, 18.3, 18.10, 18.13_
  
  - [x] 4.4 Write property tests for session management
    - **Property 6: Session Continuity Timeout**
    - **Property 17: Multiple Sessions Per User**
    - **Property 48: Single Active Session Constraint**
    - **Property 49: Automatic Session Name Generation**
    - **Validates: Requirements 2.4, 2.5, 4.5, 15.4, 15.5**

- [x] 5. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 6. Implement message protocol and routing
  - [x] 6.1 Define message types and structures
    - Create Message struct with all message types
    - Create MessageType and SenderType enums
    - Implement JSON marshaling/unmarshaling
    - _Requirements: 8.1, 8.2, 8.3_
  
  - [x] 6.2 Implement message validation
    - Validate message format against protocol specification
    - Validate required fields based on message type
    - Implement input sanitization using gohelper
    - _Requirements: 3.1, 8.4, 13.2_
  
  - [x] 6.3 Create MessageRouter for message routing
    - Implement RouteMessage method
    - Implement HandleUserMessage for user messages
    - Implement message order preservation
    - Track connections by session ID
    - _Requirements: 3.2, 3.5_
  
  - [x] 6.4 Write property tests for message protocol
    - **Property 9: Message Format Validation**
    - **Property 31: JSON Message Format**
    - **Property 32: Message Type Field Presence**
    - **Property 12: Message Order Preservation**
    - **Property 42: Input Sanitization**
    - **Validates: Requirements 3.1, 3.5, 8.1, 8.2, 8.4, 8.5, 13.2**

- [ ] 7. Integrate with Storage Service (gomongo)
  - [x] 7.1 Create StorageService wrapper
    - Initialize MongoDB client using gomongo
    - Create SessionDocument and MessageDocument structs
    - Implement CreateSession method
    - Implement UpdateSession method
    - Implement GetSession method
    - _Requirements: 4.1, 4.2_
  
  - [x] 7.2 Implement session and message persistence
    - Implement message persistence on send/receive
    - Implement session end timestamp and duration updates
    - Implement data encryption for sensitive fields
    - _Requirements: 4.2, 4.6, 4.7, 13.4_
  
  - [x] 7.3 Implement session listing and retrieval
    - Implement ListUserSessions with ordering by last activity
    - Implement GetSessionMetrics for admin monitoring
    - Implement GetTokenUsage for token aggregation
    - _Requirements: 4.3, 15.1, 18.2, 18.6_
  
  - [x] 7.4 Write property tests for storage operations
    - **Property 14: Session Creation and Persistence**
    - **Property 15: Message Persistence**
    - **Property 16: Conversation History Retrieval**
    - **Property 18: Session Lifecycle Tracking**
    - **Property 44: Data Encryption at Rest**
    - **Property 46: Session List Ordering**
    - **Validates: Requirements 4.1, 4.2, 4.3, 4.6, 4.7, 13.4, 15.1**

- [ ] 8. Implement LLM Service integration
  - [x] 8.1 Create LLMService with provider interface
    - Define LLMProvider interface
    - Create LLMRequest and LLMResponse structs
    - Implement provider registry for multiple LLM backends
    - Load LLM configurations from Config_Service
    - _Requirements: 7.1, 7.5_
  
  - [x] 8.2 Implement LLM provider implementations
    - Implement OpenAI provider
    - Implement Anthropic provider
    - Implement Dify provider
    - Implement streaming support for each provider
    - _Requirements: 7.3_
  
  - [x] 8.3 Implement LLM request handling
    - Implement SendMessage with context inclusion
    - Implement StreamMessage with chunk forwarding
    - Implement token counting and tracking
    - Implement retry logic with exponential backoff
    - Measure and log response times
    - _Requirements: 3.7, 7.2, 7.3, 7.4, 18.13_
  
  - [x] 8.4 Implement model selection
    - Store selected model in session
    - Use selected model for subsequent requests
    - _Requirements: 7.7_
  
  - [~] 8.5 Write property tests for LLM service
    - **Property 10: Valid Message Routing to LLM**
    - **Property 11: LLM Response Delivery**
    - **Property 13: Response Time Tracking**
    - **Property 27: LLM Request Context Inclusion**
    - **Property 28: Streaming Response Forwarding**
    - **Property 29: LLM Backend Retry Logic**
    - **Property 30: Model Selection Persistence**
    - **Property 62: Token Usage Tracking and Storage**
    - **Validates: Requirements 3.2, 3.3, 3.7, 7.2, 7.3, 7.4, 7.7, 18.13**

- [~] 9. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 10. Implement file upload service (goupload)
  - [~] 10.1 Create UploadService wrapper
    - Initialize S3 client using goupload
    - Implement UploadFile method
    - Implement GenerateSignedURL method
    - Implement DeleteFile method
    - _Requirements: 5.1, 5.3_
  
  - [~] 10.2 Implement file validation
    - Validate file size limits
    - Validate file types (whitelist)
    - Implement malicious file scanning
    - _Requirements: 5.4, 13.5_
  
  - [~] 10.3 Integrate file uploads with message flow
    - Handle file upload messages
    - Send file upload completion messages
    - Handle file upload errors
    - Handle AI-generated files from LLM
    - _Requirements: 5.2, 5.5, 5.6_
  
  - [~] 10.4 Write property tests for file upload
    - **Property 19: File Upload and Identifier Generation**
    - **Property 20: File Upload Notification**
    - **Property 21: Signed URL Generation**
    - **Property 22: File Validation**
    - **Property 23: File Upload Error Handling**
    - **Property 24: AI-Generated File Handling**
    - **Property 45: Malicious File Detection**
    - **Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 13.5**

- [ ] 11. Implement voice message support
  - [~] 11.1 Handle voice message uploads
    - Process voice message upload via UploadService
    - Forward audio file reference to LLM for transcription
    - _Requirements: 6.3_
  
  - [~] 11.2 Handle voice responses from LLM
    - Include audio file URL in response messages
    - _Requirements: 6.5_
  
  - [~] 11.3 Write property tests for voice messages
    - **Property 25: Voice Message Routing**
    - **Property 26: Voice Response Formatting**
    - **Validates: Requirements 6.3, 6.5**

- [ ] 12. Implement admin features
  - [~] 12.1 Implement help request handling
    - Handle help request messages from users
    - Mark session as requiring assistance
    - Persist help request state
    - _Requirements: 16.2, 16.5_
  
  - [~] 12.2 Implement admin session takeover
    - Create HandleAdminTakeover method
    - Establish admin connection to user session
    - Implement bidirectional message routing
    - Implement session locking (one admin per session)
    - Mark session with admin-assisted flag on completion
    - Log takeover events
    - _Requirements: 17.3, 17.4, 17.6, 17.7, 17.8_
  
  - [~] 12.3 Implement admin monitoring endpoints
    - Create HTTP endpoints for admin UI
    - Implement session list with filtering and sorting
    - Implement session metrics calculation
    - Implement token usage aggregation
    - Verify admin role from JWT before serving data
    - _Requirements: 18.1, 18.2, 18.8_
  
  - [~] 12.4 Write property tests for admin features
    - **Property 51: Help Request State Update**
    - **Property 53: Admin Takeover Connection**
    - **Property 54: Bidirectional Message Routing During Takeover**
    - **Property 55: Admin-Assisted Session Marking**
    - **Property 56: Session Takeover Event Logging**
    - **Property 57: Admin Session Locking**
    - **Property 58: Session Metrics Calculation**
    - **Property 61: Admin Authorization Check**
    - **Validates: Requirements 16.2, 16.5, 17.3, 17.4, 17.6, 17.7, 17.8, 18.2, 18.8**

- [ ] 13. Implement notification service (gomail, gosms)
  - [~] 13.1 Create NotificationService wrapper
    - Initialize email client using gomail
    - Initialize SMS client using gosms
    - Load notification configuration
    - Implement rate limiting for notifications
    - _Requirements: 11.4_
  
  - [~] 13.2 Implement notification methods
    - Implement SendHelpRequestAlert
    - Implement SendCriticalError
    - Implement SendSystemAlert
    - Include required information in notifications (user ID, session ID, error details, timestamp)
    - _Requirements: 11.1, 11.3, 11.5, 16.3, 16.4_
  
  - [~] 13.3 Write property tests for notifications
    - **Property 38: Critical Error Notification**
    - **Property 39: Notification Type Support**
    - **Property 40: Notification Rate Limiting**
    - **Property 41: Notification Content Completeness**
    - **Property 52: Help Request Notification**
    - **Validates: Requirements 11.1, 11.3, 11.4, 11.5, 16.3, 16.4**

- [~] 14. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 15. Implement logging service (golog)
  - [~] 15.1 Create LogService wrapper
    - Initialize golog logger
    - Configure log levels and output destinations
    - _Requirements: 10.1, 10.2, 10.5_
  
  - [~] 15.2 Integrate logging throughout application
    - Log all significant events (connections, disconnections, messages, errors, LLM interactions)
    - Include structured fields in all logs
    - Log errors with full context
    - _Requirements: 9.4, 10.3, 10.4_
  
  - [~] 15.3 Write property tests for logging
    - **Property 35: Error Logging Completeness**
    - **Property 36: Structured Log Field Inclusion**
    - **Property 37: Significant Event Logging**
    - **Validates: Requirements 9.4, 10.3, 10.4**

- [ ] 16. Implement error handling
  - [~] 16.1 Create error types and error response formatting
    - Define error categories (auth, validation, service, rate limit)
    - Create ErrorInfo struct
    - Implement error message generation with type and recoverability
    - _Requirements: 9.1, 9.2_
  
  - [~] 16.2 Implement error handling throughout application
    - Handle fatal errors with connection closure
    - Handle recoverable errors with error messages
    - _Requirements: 9.3_
  
  - [~] 16.3 Write property tests for error handling
    - **Property 33: Error Message Generation**
    - **Property 34: Fatal Error Connection Closure**
    - **Validates: Requirements 9.1, 9.2, 9.3**

- [ ] 17. Implement rate limiting and security
  - [~] 17.1 Implement rate limiting
    - Create rate limiter for connections
    - Create rate limiter for messages
    - Implement rate limit error responses
    - _Requirements: 13.3_
  
  - [~] 17.2 Implement security measures
    - Ensure WSS protocol usage
    - Implement input sanitization (already in message validation)
    - _Requirements: 13.1_
  
  - [~] 17.3 Write property tests for rate limiting
    - **Property 43: Rate Limiting Enforcement**
    - **Validates: Requirements 13.3**

- [ ] 18. Implement Kubernetes deployment configuration
  - [~] 18.1 Create Kubernetes manifests
    - Create Deployment manifest with health check endpoints
    - Create Service manifest with session affinity
    - Create ConfigMap for configuration
    - Create Secret for sensitive data
    - Support both K8s and K3s
    - _Requirements: 19.1, 19.2, 19.5, 19.7_
  
  - [~] 18.2 Implement health check endpoints
    - Implement /healthz for liveness probe
    - Implement /readyz for readiness probe
    - _Requirements: 19.4_
  
  - [~] 18.3 Implement graceful shutdown
    - Handle SIGTERM signal
    - Close connections gracefully
    - Flush logs and metrics
    - _Requirements: 19.6_

- [ ] 19. Implement frontend HTML/JavaScript chat client
  - [~] 19.1 Create HTML structure
    - Create chat interface with message list and input field
    - Create file upload button
    - Create voice recording button
    - Create model selector (conditionally visible)
    - Create connection status indicator
    - Create loading animation
    - _Requirements: 14.1, 14.3, 14.4, 14.5_
  
  - [~] 19.2 Implement WebSocket client
    - Implement WebSocket connection with JWT token
    - Implement message sending and receiving
    - Implement reconnection logic with exponential backoff
    - Implement heartbeat ping/pong
    - _Requirements: 1.1, 2.1, 2.3_
  
  - [~] 19.3 Implement message rendering
    - Render messages in chronological order
    - Distinguish user vs AI messages visually
    - Display timestamps
    - Display admin name when admin joins
    - _Requirements: 14.2, 14.3_
  
  - [~] 19.4 Implement file upload UI
    - Handle file selection
    - Handle camera access (mobile)
    - Handle photo library access (mobile)
    - Display upload progress
    - _Requirements: 14.4_
  
  - [~] 19.5 Implement voice message UI
    - Implement audio recording
    - Display recording indicators
    - Implement audio playback controls
    - _Requirements: 6.1, 6.6, 6.7_
  
  - [~] 19.6 Implement model selection UI
    - Display model selector when multiple models configured
    - Send model selection message to server
    - _Requirements: 7.6, 7.7_
  
  - [~] 19.7 Implement loading animation
    - Show loading indicator when message is being processed
    - Hide when response received
    - _Requirements: 3.6_

- [ ] 20. Implement session list page
  - [~] 20.1 Create session list HTML structure
    - Display list of user sessions
    - Show session metadata (name, timestamp, message count, admin flag)
    - Implement navigation between session list and chat
    - _Requirements: 15.1, 15.2, 15.6, 15.7_
  
  - [~] 20.2 Implement session list functionality
    - Fetch user sessions from server
    - Handle session selection and loading
    - Display admin-assisted flag
    - _Requirements: 15.3_

- [ ] 21. Implement admin UI
  - [~] 21.1 Create admin UI HTML structure
    - Display active sessions list
    - Display session metrics
    - Implement filtering and sorting controls
    - _Requirements: 18.1, 18.4, 18.5_
  
  - [~] 21.2 Implement admin UI functionality
    - Fetch session list and metrics
    - Implement filtering by user ID, date range, status, admin flag
    - Implement sorting by connection time, duration, user ID, last activity
    - Display response times and token usage
    - Auto-refresh data
    - _Requirements: 18.4, 18.5, 18.7, 18.9, 18.11, 18.12_
  
  - [~] 21.3 Implement admin takeover UI
    - Display user session list when clicking user
    - Implement session takeover button
    - Display admin name in user's chat when joined
    - _Requirements: 17.1, 17.2_

- [~] 22. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 23. Integration testing
  - [~] 23.1 Write end-to-end integration tests
    - Test complete message flow: connect → send → LLM → receive
    - Test file upload flow
    - Test voice message flow
    - Test admin takeover flow
    - Test reconnection flow
    - Test multi-model selection flow
    - _Requirements: All_

- [ ] 24. Wire everything together
  - [~] 24.1 Create main server entry point
    - Initialize all services (config, storage, upload, LLM, notification, log)
    - Initialize session manager and message router
    - Start WebSocket server
    - Start HTTP server for admin UI
    - _Requirements: All_
  
  - [~] 24.2 Create deployment scripts
    - Create Docker build script
    - Create Kubernetes deployment script
    - Create environment configuration examples
    - _Requirements: 19.1_

- [~] 25. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties (minimum 100 iterations each)
- Unit tests validate specific examples and edge cases
- Integration tests validate end-to-end flows
- Follow TDD: write tests before implementation
- Use gohelper for common utility functions to maintain DRY principles
- Use golog for all logging operations
- All WebSocket connections must use WSS protocol
- All sensitive data must be encrypted at rest
