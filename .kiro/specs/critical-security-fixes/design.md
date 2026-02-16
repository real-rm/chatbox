# Design Document: Critical Security Fixes

## Overview

This design addresses four critical security and functional issues identified in the production readiness review. The fixes are designed to be minimal, focused, and non-breaking to existing functionality while significantly improving security and reliability.

The four issues addressed are:

1. **Encryption Key Validation** - Replace silent padding/truncation with fail-fast validation
2. **JWT Token Security Documentation** - Document security implications of tokens in URLs
3. **WebSocket Message Size Limit** - Prevent denial-of-service via oversized messages
4. **Session Creation Flow** - Implement missing session creation logic

## Architecture

### Current Architecture Context

The application follows a layered architecture:
- **WebSocket Handler Layer** (`internal/websocket/handler.go`) - Manages WebSocket connections
- **Message Router Layer** (`internal/router/router.go`) - Routes messages to appropriate handlers
- **Session Manager Layer** (`internal/session/session.go`) - Manages in-memory session state
- **Storage Layer** (`internal/storage/storage.go`) - Persists sessions to MongoDB
- **Main Application** (`chatbox.go`) - Initializes all components

### Changes Required

The fixes will be implemented across multiple layers:

1. **Encryption Key Validation**: Modify `chatbox.go` initialization
2. **JWT Token Documentation**: Add new documentation file
3. **Message Size Limit**: Modify `internal/websocket/handler.go`
4. **Session Creation**: Modify `internal/router/router.go` and potentially `internal/session/session.go`

## Components and Interfaces

### 1. Encryption Key Validator

**Location**: `chatbox.go` (Register function)

**Purpose**: Validate encryption key length at startup before any encryption operations.

**Interface**:
```go
// validateEncryptionKey checks if the encryption key is exactly 32 bytes
// Returns error if key is provided but not 32 bytes
// Returns nil if key is empty (encryption disabled) or exactly 32 bytes
func validateEncryptionKey(key []byte) error
```

**Behavior**:
- If key length is 0: Return nil (encryption disabled, warning already logged)
- If key length is 32: Return nil (valid)
- Otherwise: Return error with clear message including actual and required lengths

### 2. WebSocket Message Size Limiter

**Location**: `internal/websocket/handler.go`

**Purpose**: Set read limit on WebSocket connections to prevent memory exhaustion.

**Configuration**:
```go
// Configuration via environment variable or config file
// Default: 1MB (1048576 bytes)
maxMessageSize := getMaxMessageSize(config)
```

**Implementation Point**: In `HandleWebSocket` function, after upgrading connection but before starting read/write pumps:
```go
conn.SetReadLimit(maxMessageSize)
```

**Error Handling**: When limit exceeded, Gorilla WebSocket automatically closes connection with error. The `readPump` function will log this with user context.

### 3. Session Creation Handler

**Location**: `internal/router/router.go` (HandleUserMessage function)

**Purpose**: Create sessions automatically when they don't exist, instead of failing with "Session not found".

**Current Flow**:
```
1. Receive message with session ID
2. Call sessionManager.GetSession(sessionID)
3. If not found → ERROR (current behavior)
4. Process message
```

**New Flow**:
```
1. Receive message with session ID
2. Call sessionManager.GetSession(sessionID)
3. If not found:
   a. Create new session via sessionManager.CreateSession(userID)
   b. Store session in database via storageService.CreateSession(session)
   c. Register connection with new session ID
4. Process message
```

**Interface Changes**:

The MessageRouter will need access to StorageService:
```go
type MessageRouter struct {
    // ... existing fields ...
    storageService *storage.StorageService // NEW: for persisting sessions
}

func NewMessageRouter(
    sessionManager *session.SessionManager,
    llmService LLMService,
    uploadService *upload.UploadService,
    notificationService NotificationService,
    storageService *storage.StorageService, // NEW parameter
    logger *golog.Logger,
) *MessageRouter
```

**Session Creation Logic**:
```go
// In HandleUserMessage, replace the simple GetSession call with:
sess, err := mr.getOrCreateSession(conn, msg.SessionID)
if err != nil {
    return err
}

// New helper method:
func (mr *MessageRouter) getOrCreateSession(conn *Connection, sessionID string) (*session.Session, error) {
    // Try to get existing session
    sess, err := mr.sessionManager.GetSession(sessionID)
    if err == nil {
        return sess, nil // Session exists
    }
    
    // Session not found - create new one
    if errors.Is(err, session.ErrSessionNotFound) {
        return mr.createNewSession(conn, sessionID)
    }
    
    // Other error
    return nil, err
}

func (mr *MessageRouter) createNewSession(conn *Connection, sessionID string) (*session.Session, error) {
    // Create session in memory
    sess, err := mr.sessionManager.CreateSession(conn.UserID)
    if err != nil {
        return nil, chaterrors.ErrDatabaseError(err)
    }
    
    // Persist to database
    if err := mr.storageService.CreateSession(sess); err != nil {
        // Rollback in-memory session
        mr.sessionManager.EndSession(sess.ID)
        return nil, chaterrors.ErrDatabaseError(err)
    }
    
    return sess, nil
}
```

**Concurrency Handling**: The SessionManager already has mutex protection. If two messages arrive simultaneously for the same user, the second CreateSession call will fail with `ErrActiveSessionExists`, which is the correct behavior.

### 4. JWT Token Security Documentation

**Location**: New file `docs/JWT_TOKEN_SECURITY.md`

**Purpose**: Document security implications and best practices for JWT token handling.

**Content Structure**:
1. Overview of JWT token usage in the application
2. Security implications of tokens in URL query parameters
3. Recommended mitigations (short-lived tokens, token rotation)
4. Alternative authentication methods (Authorization header)
5. Monitoring and detection recommendations

## Data Models

No changes to existing data models are required. All fixes work with existing structures.


## Correctness Properties

A property is a characteristic or behavior that should hold true across all valid executions of a system—essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.

### Encryption Key Validation Properties

Property 1: Error message completeness for invalid keys
*For any* encryption key that is not exactly 32 bytes in length, the error message returned during validation should contain both the required length (32) and the actual length of the provided key
**Validates: Requirements 1.7**

### WebSocket Message Size Limit Properties

Property 2: Read limit enforcement
*For any* WebSocket connection established by the system, the connection should have a read limit configured that prevents reading messages larger than the configured maximum
**Validates: Requirements 3.1**

Property 3: Oversized message rejection
*For any* message sent to a WebSocket connection that exceeds the configured read limit, the connection should be closed with an error
**Validates: Requirements 3.2**

Property 4: Configuration value application
*For any* valid configuration value for the message size limit, when the system starts with that configuration, the read limit should be set to that value
**Validates: Requirements 3.3**

Property 5: Oversized message logging
*For any* connection that is closed due to an oversized message, a log entry should be created that contains both the user ID and connection ID
**Validates: Requirements 3.5, 3.6**

### Session Creation Flow Properties

Property 6: Automatic session creation for new users
*For any* authenticated user who sends a message without providing a session ID, a new session should be created and associated with that user
**Validates: Requirements 4.1**

Property 7: Existing session reuse
*For any* message that includes a session ID for an existing session, the system should use that existing session rather than creating a new one
**Validates: Requirements 4.2**

Property 8: Session creation with provided ID
*For any* message that includes a session ID that does not exist, the system should create a new session
**Validates: Requirements 4.3**

Property 9: Dual storage consistency
*For any* session that is created, the session should exist in both the SessionManager (in-memory) and the StorageService (database) with consistent data
**Validates: Requirements 4.4**

Property 10: User association correctness
*For any* session that is created, the session's user ID should match the user ID from the JWT token of the connection that triggered the creation
**Validates: Requirements 4.5**

Property 11: Session creation error handling
*For any* session creation attempt that fails (e.g., due to database error), an error message should be sent to the client
**Validates: Requirements 4.6**

Property 12: Concurrent session creation safety
*For any* set of concurrent session creation requests from the same user, only one session should be successfully created
**Validates: Requirements 4.7**

Property 13: Session restoration from database
*For any* user who reconnects with a session ID that exists in the database, the session should be restored with all its previous data intact
**Validates: Requirements 4.8**

## Error Handling

### Encryption Key Validation Errors

**Error Type**: Startup Fatal Error
**Trigger**: Encryption key length is not 32 bytes
**Behavior**: 
- Log error with clear message including actual and required lengths
- Terminate application with non-zero exit code
- Do not start accepting connections

**Error Message Format**:
```
"Encryption key must be exactly 32 bytes for AES-256, got %d bytes. Please provide a valid 32-byte key or remove the key to disable encryption."
```

### WebSocket Message Size Errors

**Error Type**: Connection Error
**Trigger**: Client sends message exceeding read limit
**Behavior**:
- Gorilla WebSocket automatically closes connection
- Log error with user ID, connection ID, and limit exceeded
- Client receives WebSocket close frame

**Log Message Format**:
```
"WebSocket message size limit exceeded"
  "user_id": <user_id>
  "connection_id": <connection_id>
  "limit": <max_size_bytes>
```

### Session Creation Errors

**Error Type**: Recoverable Error
**Trigger**: Session creation fails (database error, validation error)
**Behavior**:
- Log detailed error server-side
- Send generic error message to client
- Rollback any partial state (in-memory session)
- Client can retry

**Error Response Format**:
```json
{
  "type": "error",
  "error": {
    "code": "database_error",
    "message": "Failed to create session. Please try again.",
    "recoverable": true
  }
}
```

## Testing Strategy

### Unit Tests

Unit tests will focus on specific examples and edge cases:

1. **Encryption Key Validation**
   - Test with 32-byte key (valid)
   - Test with empty key (valid, encryption disabled)
   - Test with 16-byte key (invalid)
   - Test with 31-byte key (invalid)
   - Test with 33-byte key (invalid)
   - Test with 64-byte key (invalid)
   - Verify error message format

2. **WebSocket Message Size Limit**
   - Test default limit is 1MB
   - Test custom limit from configuration
   - Test connection with limit set
   - Test message at limit (should succeed)
   - Test message over limit (should fail)
   - Verify logging on limit exceeded

3. **Session Creation Flow**
   - Test session creation for new user
   - Test session reuse for existing session
   - Test session creation with provided ID
   - Test dual storage (memory + database)
   - Test user ID association
   - Test error handling on database failure
   - Test concurrent creation attempts
   - Test session restoration

### Property-Based Tests

Property tests will verify universal properties across all inputs (minimum 100 iterations per test):

1. **Property 1: Error message completeness**
   - Generate random invalid key lengths (0-31, 33-100)
   - Verify error message contains both required (32) and actual length
   - **Tag**: Feature: critical-security-fixes, Property 1: Error message completeness for invalid keys

2. **Property 2: Read limit enforcement**
   - Generate random valid configuration values
   - Verify all connections have read limit set
   - **Tag**: Feature: critical-security-fixes, Property 2: Read limit enforcement

3. **Property 3: Oversized message rejection**
   - Generate random message sizes above limit
   - Verify all are rejected
   - **Tag**: Feature: critical-security-fixes, Property 3: Oversized message rejection

4. **Property 4: Configuration value application**
   - Generate random valid configuration values
   - Verify read limit matches configuration
   - **Tag**: Feature: critical-security-fixes, Property 4: Configuration value application

5. **Property 5: Oversized message logging**
   - Generate random oversized messages
   - Verify all log entries contain user ID and connection ID
   - **Tag**: Feature: critical-security-fixes, Property 5: Oversized message logging

6. **Property 6: Automatic session creation**
   - Generate random user IDs
   - Verify session created for each
   - **Tag**: Feature: critical-security-fixes, Property 6: Automatic session creation for new users

7. **Property 7: Existing session reuse**
   - Generate random existing sessions
   - Verify no new session created
   - **Tag**: Feature: critical-security-fixes, Property 7: Existing session reuse

8. **Property 8: Session creation with provided ID**
   - Generate random non-existent session IDs
   - Verify session created for each
   - **Tag**: Feature: critical-security-fixes, Property 8: Session creation with provided ID

9. **Property 9: Dual storage consistency**
   - Generate random session data
   - Verify consistency between memory and database
   - **Tag**: Feature: critical-security-fixes, Property 9: Dual storage consistency

10. **Property 10: User association correctness**
    - Generate random user IDs in JWT tokens
    - Verify session user ID matches JWT
    - **Tag**: Feature: critical-security-fixes, Property 10: User association correctness

11. **Property 11: Session creation error handling**
    - Generate random database failure scenarios
    - Verify error message sent to client
    - **Tag**: Feature: critical-security-fixes, Property 11: Session creation error handling

12. **Property 12: Concurrent session creation safety**
    - Generate random concurrent request counts
    - Verify only one session created per user
    - **Tag**: Feature: critical-security-fixes, Property 12: Concurrent session creation safety

13. **Property 13: Session restoration**
    - Generate random session data
    - Verify restoration preserves all data
    - **Tag**: Feature: critical-security-fixes, Property 13: Session restoration from database

### Integration Tests

Integration tests will verify end-to-end flows:

1. **Encryption Key Validation Integration**
   - Start application with invalid key, verify startup fails
   - Start application with valid key, verify startup succeeds
   - Verify no encryption operations occur with invalid key

2. **WebSocket Message Size Integration**
   - Establish WebSocket connection
   - Send message at limit, verify success
   - Send message over limit, verify connection closes
   - Verify log entry created

3. **Session Creation Flow Integration**
   - Connect as new user, send message, verify session created
   - Reconnect with session ID, verify session restored
   - Send concurrent messages, verify single session
   - Simulate database failure, verify error handling

### Test Configuration

All property-based tests will be configured to run a minimum of 100 iterations to ensure comprehensive coverage through randomization. This is necessary because property-based testing relies on generating many random inputs to find edge cases and verify universal properties.
