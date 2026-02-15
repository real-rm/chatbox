# Requirements Document

## Introduction

This document specifies the requirements for a real-time HTML chat application with a Golang backend. The system is embedded within an existing application (React Native mobile app and web iframe) and sits behind an nginx reverse proxy. Users access the chat with JWT tokens containing user ID and roles. The system enables AI-powered conversations, file upload/download, conversation persistence, administrative monitoring, and staff assistance capabilities. The backend integrates with existing API packages (gomongo, goupload, goconfig, gomain, golog, gohelper, gomail, gosms) and uses WebSocket protocol for bidirectional real-time communication.

## Glossary

- **Chat_System**: The complete application including frontend and backend components
- **WebSocket_Server**: The Golang backend server handling WebSocket connections
- **Chat_Client**: The HTML/JavaScript frontend interface embedded in webview or iframe
- **Session_List_Page**: The interface displaying all user sessions, sharing the same webview/iframe with Chat_Client
- **Storage_Service**: The gomongo package managing conversation persistence in MongoDB
- **Upload_Service**: The goupload package handling file storage on S3
- **Config_Service**: The goconfig package managing LLM/Dify backend configuration
- **Log_Service**: The golog package providing structured logging
- **Helper_Service**: The gohelper package providing common utility functions for DRY principles
- **Notification_Service**: The gomail and gosms packages for sending admin notifications
- **LLM_Backend**: The AI service (LLM or Dify) generating chat responses
- **Admin_UI**: The administrative interface for monitoring active sessions and metrics
- **Chat_Admin**: An administrator with elevated privileges to monitor and take over user sessions
- **Connection**: An active WebSocket connection between client and server
- **Session**: An active user connection with associated metadata and state
- **Message**: A single chat message with content, timestamp, and metadata
- **User**: An authenticated person using the chat application
- **JWT_Token**: JSON Web Token containing user ID and roles passed from parent application
- **Session_Takeover**: When a Chat_Admin joins an active user session to provide assistance
- **Help_Request**: A user-initiated request for staff assistance

## Requirements

### Requirement 1: JWT Authentication

**User Story:** As a user coming from the parent application, I want to access the chat seamlessly using my existing session, so that I don't need to log in again.

#### Acceptance Criteria

1. WHEN a Chat_Client initiates a WebSocket connection with a JWT_Token, THE WebSocket_Server SHALL validate the token signature and expiration
2. WHEN a JWT_Token is valid, THE WebSocket_Server SHALL extract the user ID and roles from the token claims
3. IF a JWT_Token is invalid or expired, THEN THE WebSocket_Server SHALL reject the connection with an unauthorized error
4. THE WebSocket_Server SHALL associate each Connection with the authenticated user ID from the JWT_Token
5. WHEN a JWT_Token is about to expire during an active session, THE Chat_Client SHALL request a token refresh from the parent application

### Requirement 2: WebSocket Connection Management

**User Story:** As a user, I want a stable real-time connection to the chat server with automatic reconnection, so that I can send and receive messages instantly without losing my session context.

#### Acceptance Criteria

1. WHEN a Chat_Client initiates a connection with valid authentication, THE WebSocket_Server SHALL establish a bidirectional WebSocket connection
2. WHILE a Connection is active, THE WebSocket_Server SHALL maintain the connection and handle ping/pong heartbeat messages
3. WHEN a Connection is lost unexpectedly, THE Chat_Client SHALL attempt to reconnect with exponential backoff
4. WHEN a Connection reconnects within the configured timeout period (default 15 minutes), THE WebSocket_Server SHALL restore the previous session context
5. IF a Connection reconnects after the timeout period, THEN THE WebSocket_Server SHALL create a new session
6. WHEN a Connection is closed gracefully, THE WebSocket_Server SHALL clean up associated resources and close the socket
7. THE WebSocket_Server SHALL support concurrent connections from multiple authenticated users
8. THE session continuity timeout SHALL be configurable through the Config_Service

### Requirement 3: Message Exchange

**User Story:** As a user, I want to send messages and receive AI responses in real-time, so that I can have natural conversations.

#### Acceptance Criteria

1. WHEN a user sends a message through the Chat_Client, THE WebSocket_Server SHALL receive the message and validate its format
2. WHEN a valid message is received, THE WebSocket_Server SHALL forward it to the LLM_Backend for processing
3. WHEN the LLM_Backend generates a response, THE WebSocket_Server SHALL send the response to the Chat_Client through the WebSocket connection
4. WHEN a message fails validation, THE WebSocket_Server SHALL return an error message to the Chat_Client without forwarding to the LLM_Backend
5. THE WebSocket_Server SHALL preserve message order within a single session
6. WHEN a message is being processed by the LLM_Backend, THE Chat_Client SHALL display a loading animation to indicate work in progress
7. THE WebSocket_Server SHALL measure and log the response time for each LLM_Backend request

### Requirement 4: Conversation Persistence

**User Story:** As a user, I want my conversation history saved automatically in session records, so that I can review past conversations and maintain context across reconnections.

#### Acceptance Criteria

1. WHEN a new session begins, THE Storage_Service SHALL create a session record in MongoDB with a unique session identifier, user ID, and start timestamp
2. WHEN a message is sent or received, THE Storage_Service SHALL persist the message to the session record immediately
3. WHEN a user requests conversation history, THE Storage_Service SHALL retrieve all messages for the specified session ordered by timestamp
4. THE Storage_Service SHALL store each session as a single document containing all messages and metadata for that session
5. THE Storage_Service SHALL support multiple session records per user, each representing a distinct conversation
6. WHEN storing messages, THE Storage_Service SHALL include message content, timestamp, sender type (user or AI), and metadata
7. WHEN a session ends, THE Storage_Service SHALL update the session record with end timestamp and duration

### Requirement 5: File Upload and Download

**User Story:** As a user, I want to upload files, take photos, and download AI-generated files during conversations, so that I can share media and receive generated content.

#### Acceptance Criteria

1. WHEN a user uploads a file through the Chat_Client, THE Upload_Service SHALL store the file on S3 and return a unique file identifier and access URL
2. WHEN a file upload completes, THE WebSocket_Server SHALL send a message containing the file identifier, URL, and metadata to the session
3. WHEN a user requests to download a file, THE Upload_Service SHALL generate a signed URL with expiration time
4. THE Upload_Service SHALL validate file size and type before accepting uploads
5. IF a file upload fails, THEN THE WebSocket_Server SHALL notify the Chat_Client with an error message and reason
6. WHEN the LLM_Backend generates files (images, documents, etc.), THE WebSocket_Server SHALL store them on S3 or accept external URLs and include download links in the response message
7. THE Chat_Client SHALL provide access to device camera for taking photos on mobile devices
8. THE Chat_Client SHALL provide access to photo library for selecting existing images on mobile devices
9. WHEN a user captures a photo or selects from library, THE Chat_Client SHALL upload it using the same file upload flow

### Requirement 6: Voice Message Support

**User Story:** As a user, I want to send and receive voice messages, so that I can communicate more naturally and receive audio responses from the AI.

#### Acceptance Criteria

1. WHEN a user records a voice message through the Chat_Client, THE Chat_Client SHALL capture audio and encode it in a supported format (e.g., WebM, MP3, AAC)
2. WHEN a voice recording completes, THE Chat_Client SHALL upload the audio file using the Upload_Service
3. WHEN a voice message is uploaded, THE WebSocket_Server SHALL send the audio file reference to the LLM_Backend for transcription or processing
4. THE LLM_Backend SHALL support generating voice responses when configured for text-to-speech
5. WHEN the LLM_Backend generates a voice response, THE WebSocket_Server SHALL include the audio file URL in the response message
6. THE Chat_Client SHALL provide audio playback controls for voice messages from both user and AI
7. THE Chat_Client SHALL display visual indicators during voice recording (recording time, waveform, etc.)

### Requirement 7: LLM Backend Integration

**User Story:** As a user, I want the system to integrate with AI services and select different models, so that I receive intelligent responses tailored to my needs.

#### Acceptance Criteria

1. WHEN the WebSocket_Server starts, THE Config_Service SHALL load the LLM backend configuration (endpoint, API keys, model parameters)
2. WHEN forwarding a message to the LLM_Backend, THE WebSocket_Server SHALL include session context and user message
3. WHEN the LLM_Backend supports streaming responses, THE WebSocket_Server SHALL stream response chunks to the Chat_Client in real-time
4. IF the LLM_Backend is unavailable, THEN THE WebSocket_Server SHALL return an error message and retry with exponential backoff
5. THE Config_Service SHALL support configuration for multiple LLM providers (OpenAI, Anthropic, Dify, etc.)
6. WHERE multiple LLM models are configured, THE Chat_Client SHALL display a model selector field with unique names for each model
7. WHEN a user selects a different model, THE WebSocket_Server SHALL use that model for all subsequent requests in the session
8. THE model selector field SHALL only be visible when multiple models are configured in the Config_Service

### Requirement 8: Message Protocol

**User Story:** As a developer, I want a well-defined message protocol, so that the frontend and backend can communicate reliably.

#### Acceptance Criteria

1. THE Chat_System SHALL use JSON format for all WebSocket messages
2. WHEN sending any message, THE Chat_System SHALL include a message type field to identify the message purpose
3. THE WebSocket_Server SHALL define message types for: user messages, AI responses, file uploads, voice messages, errors, connection status, and typing indicators
4. WHEN parsing incoming messages, THE WebSocket_Server SHALL validate the message structure against the protocol specification
5. IF a message does not conform to the protocol, THEN THE WebSocket_Server SHALL return a protocol error message

### Requirement 9: Error Handling

**User Story:** As a user, I want clear error messages when something goes wrong, so that I understand what happened and can take appropriate action.

#### Acceptance Criteria

1. WHEN any error occurs, THE WebSocket_Server SHALL send an error message to the Chat_Client with error type and description
2. THE WebSocket_Server SHALL distinguish between recoverable errors (retry possible) and fatal errors (connection must close)
3. WHEN a fatal error occurs, THE WebSocket_Server SHALL close the connection gracefully after sending the error message
4. THE Log_Service SHALL log all errors with sufficient context for debugging including timestamp, user ID, session ID, and stack trace
5. THE Chat_Client SHALL display user-friendly error messages based on error types received from the server

### Requirement 10: Logging and Monitoring

**User Story:** As a developer and system administrator, I want comprehensive logging, so that I can debug issues and monitor system health.

#### Acceptance Criteria

1. THE WebSocket_Server SHALL use the Log_Service (golog) for all logging operations
2. THE Log_Service SHALL support multiple log levels (DEBUG, INFO, WARN, ERROR, FATAL)
3. WHEN logging events, THE Log_Service SHALL include structured fields (timestamp, level, user ID, session ID, component, message)
4. THE WebSocket_Server SHALL log all significant events including connections, disconnections, message exchanges, errors, and LLM backend interactions
5. THE Log_Service SHALL support configurable log output destinations (stdout, file, external logging service)

### Requirement 11: Admin Notifications

**User Story:** As an administrator, I want to be notified when unexpected situations arise, so that I can respond quickly to system issues.

#### Acceptance Criteria

1. WHEN a critical error occurs, THE Notification_Service SHALL send alerts to administrators via email (gomail) or SMS (gosms)
2. THE Config_Service SHALL define notification rules specifying which events trigger notifications and which channels to use
3. THE Notification_Service SHALL support notification types for: service crashes, LLM backend failures, database connection failures, and abnormal traffic patterns
4. THE Notification_Service SHALL implement rate limiting to prevent notification flooding
5. WHEN sending notifications, THE Notification_Service SHALL include error details, affected users count, and timestamp

### Requirement 12: Code Reusability

**User Story:** As a developer, I want to follow DRY principles using shared utilities, so that the codebase is maintainable and consistent.

#### Acceptance Criteria

1. THE WebSocket_Server SHALL use the Helper_Service (gohelper) for common utility functions
2. THE Helper_Service SHALL provide reusable functions for: string manipulation, data validation, error handling, and data transformation
3. WHEN implementing new features, THE development team SHALL extract common patterns into the Helper_Service
4. THE Chat_System SHALL avoid code duplication by referencing shared functions from the Helper_Service
5. THE Helper_Service functions SHALL have comprehensive unit tests to ensure reliability

### Requirement 13: Security

**User Story:** As a system administrator, I want the chat system to be secure, so that user data and conversations are protected from unauthorized access.

#### Acceptance Criteria

1. THE WebSocket_Server SHALL use WSS (WebSocket Secure) protocol with TLS encryption for all connections
2. THE WebSocket_Server SHALL validate and sanitize all user input before processing
3. THE WebSocket_Server SHALL implement rate limiting to prevent abuse and denial-of-service attacks
4. THE Storage_Service SHALL encrypt sensitive data at rest in MongoDB
5. THE Upload_Service SHALL validate file content and scan for malicious files before storing on S3

### Requirement 14: Frontend Chat Interface

**User Story:** As a user, I want an intuitive chat interface that works in both mobile webview and web iframe, so that I can easily send messages, view responses, and manage files.

#### Acceptance Criteria

1. THE Chat_Client SHALL display a message input field and send button for composing messages
2. WHEN messages are sent or received, THE Chat_Client SHALL display them in chronological order with timestamps
3. THE Chat_Client SHALL visually distinguish between user messages and AI responses
4. THE Chat_Client SHALL provide a file upload button and display upload progress
5. WHEN the WebSocket connection status changes, THE Chat_Client SHALL display connection status indicators (connected, disconnecting, reconnecting)
6. THE Chat_Client SHALL be responsive and function correctly in both React Native webview and web iframe contexts
7. THE Chat_Client SHALL communicate with the parent application for JWT token refresh when needed

### Requirement 15: Session List Management

**User Story:** As a user, I want to view and manage my chat sessions, so that I can navigate between different conversations.

#### Acceptance Criteria

1. THE Session_List_Page SHALL display all sessions for the authenticated user ordered by most recent activity
2. THE Session_List_Page SHALL share the same webview or iframe with the Chat_Client and navigate between them
3. WHEN a user selects a session from the list, THE Chat_System SHALL navigate to the Chat_Client and load that session's conversation history
4. THE Chat_System SHALL NOT support concurrent active sessions for the same user
5. WHEN a new session is created, THE Chat_System SHALL automatically generate a descriptive session name based on the initial conversation content
6. THE Session_List_Page SHALL display session metadata including name, last message timestamp, and message count
7. WHEN a session has been assisted by a Chat_Admin, THE Session_List_Page SHALL display a special flag or indicator for that session

### Requirement 16: Help Request and Admin Notification

**User Story:** As a user, I want to request help from staff when needed, so that I can get human assistance during my conversation.

#### Acceptance Criteria

1. THE Chat_Client SHALL provide a help request button for users to request staff assistance
2. WHEN a user initiates a Help_Request, THE WebSocket_Server SHALL mark the session as requiring assistance
3. WHEN a Help_Request is created, THE Notification_Service SHALL send email and SMS notifications to all Chat_Admin users
4. THE notification SHALL include user ID, session ID, and a link to access the session
5. THE WebSocket_Server SHALL update the session record to indicate a Help_Request is pending

### Requirement 17: Admin Session Takeover

**User Story:** As a Chat_Admin, I want to view user sessions and take over conversations, so that I can provide direct assistance to users.

#### Acceptance Criteria

1. THE Admin_UI SHALL allow a Chat_Admin to view any user's session list by clicking on the user
2. WHEN a Chat_Admin selects a session from a user's session list, THE Admin_UI SHALL allow the Chat_Admin to take over that session
3. WHEN a Session_Takeover occurs, THE WebSocket_Server SHALL establish a connection for the Chat_Admin to the user's session
4. WHILE a Session_Takeover is active, THE WebSocket_Server SHALL route messages from both the user and Chat_Admin to each other
5. THE Chat_Client SHALL visually indicate when a Chat_Admin has joined the session
6. WHEN a Session_Takeover ends, THE WebSocket_Server SHALL mark the session with an admin-assisted flag
7. THE WebSocket_Server SHALL log all Session_Takeover events including Chat_Admin ID, start time, and end time
8. WHEN a Chat_Admin joins a session, THE WebSocket_Server SHALL mark the session as "assisted by [Admin Name]" to prevent other Chat_Admin users from joining accidentally
9. WHEN a Chat_Admin is assisting a session, THE Chat_Client SHALL display the Chat_Admin's name in place of the model selector field

### Requirement 18: Administrative Monitoring

**User Story:** As an administrator, I want to monitor active chat sessions and view usage metrics with filtering and sorting capabilities, so that I can understand system load and user engagement.

#### Acceptance Criteria

1. THE Admin_UI SHALL display a list of currently active sessions with user ID, connection time, status, and admin-assisted flag
2. WHEN an administrator requests session metrics, THE WebSocket_Server SHALL return total session count, average concurrent sessions, and maximum concurrent sessions for a specified time period
3. THE WebSocket_Server SHALL track session start time, end time, and duration for all connections
4. THE Admin_UI SHALL provide filter options for: user ID, date range, session status (active/ended), and admin-assisted flag
5. THE Admin_UI SHALL provide sort options for: connection time, duration, user ID, and last activity timestamp
6. THE WebSocket_Server SHALL persist session metrics to the Storage_Service for historical analysis
7. THE Admin_UI SHALL refresh active session data automatically at regular intervals
8. THE Admin_UI SHALL require administrator role verification from JWT_Token before displaying monitoring data
9. THE Admin_UI SHALL display maximum and average response time for each session
10. THE WebSocket_Server SHALL calculate and store maximum and average LLM_Backend response times per session
11. THE Admin_UI SHALL display total tokens used for each session
12. THE Admin_UI SHALL display total tokens used across all sessions for a specified time period
13. THE WebSocket_Server SHALL track and store token usage for each LLM_Backend request
14. WHEN storing session records, THE Storage_Service SHALL include total token count as part of the session metadata

### Requirement 19: Kubernetes Deployment

**User Story:** As a DevOps engineer, I want the backend service to run in Kubernetes or K3s, so that it can scale and be managed in a cloud-native environment.

#### Acceptance Criteria

1. THE Chat_System SHALL provide Kubernetes deployment manifests (Deployment, Service, ConfigMap, Secret)
2. THE WebSocket_Server SHALL support horizontal scaling with multiple pod replicas
3. THE Config_Service SHALL load configuration from Kubernetes ConfigMaps and Secrets
4. THE WebSocket_Server SHALL implement health check endpoints for Kubernetes liveness and readiness probes
5. THE deployment manifests SHALL support both Kubernetes (K8s) and K3s environments
6. THE WebSocket_Server SHALL handle graceful shutdown when receiving termination signals from Kubernetes
7. WHERE session affinity is required, THE Kubernetes Service SHALL be configured with appropriate session affinity settings

### Requirement 20: Testing and Code Quality

**User Story:** As a developer, I want comprehensive test coverage following TDD principles, so that the system is reliable and maintainable.

#### Acceptance Criteria

1. THE Chat_System SHALL have unit tests for all core functions and methods with minimum 80% code coverage
2. THE WebSocket_Server SHALL have integration tests validating the complete message flow from client to LLM and back
3. THE Chat_System SHALL follow DRY principles with shared code extracted into reusable functions
4. WHEN writing new features, THE development process SHALL follow TDD by writing tests before implementation
5. THE Chat_System SHALL have property-based tests for message protocol parsing and validation
