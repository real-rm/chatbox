# Error Message Sanitization

## Overview

This document describes the error message sanitization implementation that prevents internal implementation details from being leaked to clients while maintaining detailed server-side logging for debugging.

## Security Requirement

**Requirement 2.2: Error Message Sanitization**
- Client-facing error messages must be generic and safe
- Detailed errors are logged server-side only
- JWT validation errors don't expose token details
- Database errors don't expose query details

## Implementation

### HTTP Endpoints

For HTTP endpoints, we use the `internal/httperrors` package which provides:

1. **Generic Error Messages**: Pre-defined safe messages that don't expose internal details
2. **Error Codes**: Machine-readable codes for client-side error handling
3. **Structured Responses**: Consistent JSON error response format

#### Error Response Format

```json
{
  "error": "Generic error message",
  "code": "ERROR_CODE",
  "details": "Optional additional context"
}
```

#### Available Error Responses

| Function | Status | Message | Use Case |
|----------|--------|---------|----------|
| `RespondUnauthorized` | 401 | "Authentication required" | Missing or invalid auth |
| `RespondInvalidToken` | 401 | "Invalid or expired authentication token" | Token validation failed |
| `RespondForbidden` | 403 | "Insufficient permissions" | Authorization failed |
| `RespondBadRequest` | 400 | Custom or "Bad request" | Invalid input |
| `RespondInternalError` | 500 | "An internal error occurred" | Server errors |
| `RespondServiceUnavailable` | 503 | "Service temporarily unavailable" | Service down |
| `RespondNotFound` | 404 | "Resource not found" | Resource missing |

#### Usage Example

**Before (Leaks Internal Details):**
```go
if err != nil {
    c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to list sessions: %v", err)})
    return
}
```

**After (Sanitized):**
```go
if err != nil {
    // Log detailed error server-side
    logger.Error("Failed to list sessions",
        "user_id", userID,
        "error", err,
        "component", "http")
    // Send generic error to client
    httperrors.RespondInternalError(c)
    return
}
```

### WebSocket Messages

For WebSocket messages, we use the `internal/errors` package which provides:

1. **ChatError Type**: Structured errors with categories and recoverability
2. **Error Categories**: Auth, Validation, Service, RateLimit
3. **Wire Protocol**: Conversion to `message.ErrorInfo` for transmission

#### Error Categories

- **CategoryAuth**: Authentication/authorization errors (fatal)
- **CategoryValidation**: Input validation errors (recoverable)
- **CategoryService**: Service-level errors like LLM, database, S3 (recoverable)
- **CategoryRateLimit**: Rate limiting errors (recoverable with retry)

#### Usage Example

```go
if err != nil {
    // Log detailed error server-side
    h.logger.Error("Failed to route message",
        "user_id", c.UserID,
        "session_id", c.SessionID,
        "error", err)
    
    // Send generic error to client
    errorMsg := &message.Message{
        Type:   message.TypeError,
        Sender: message.SenderAI,
        Error: &message.ErrorInfo{
            Code:        string(chaterrors.ErrCodeServiceError),
            Message:     "Failed to process message",
            Recoverable: true,
        },
        Timestamp: time.Now(),
    }
    c.send <- errorBytes
}
```

## Protected Endpoints

The following endpoints have been sanitized with server-side logging:

### Authentication Middleware
- `authMiddleware`: Admin authentication
  - Logs: Token validation failures with error details
  - Sends: Generic "Invalid or expired authentication token" message
- `userAuthMiddleware`: User authentication
  - Logs: Token validation failures with error details
  - Sends: Generic "Invalid or expired authentication token" message
- WebSocket `HandleWebSocket`: Token validation
  - Logs: JWT validation failures, connection limit exceeded, upgrade failures
  - Sends: Generic "Authentication failed" or "Connection limit exceeded" messages

### HTTP Handlers
- `handleUserSessions`: List user sessions
  - Logs: Database errors with user_id and full error details
  - Sends: Generic "An internal error occurred" message
- `handleListSessions`: List all sessions (admin)
  - Logs: Invalid time format parameters, database errors with full context
  - Sends: Generic "Invalid time format" or "An internal error occurred" messages
- `handleGetMetrics`: Get session metrics (admin)
  - Logs: Invalid time parameters, database query failures with full error details
  - Sends: Generic "Invalid time format" or "An internal error occurred" messages
- `handleAdminTakeover`: Admin session takeover
  - Logs: Takeover failures with session_id, admin_id, and error details
  - Sends: Generic "An internal error occurred" message

### WebSocket Handler
- `readPump`: Message parsing and routing errors
  - Logs: Unexpected close errors, message parsing failures, routing errors with full context
  - Sends: Generic error messages like "Invalid message format", "Failed to process message"
- Connection registration errors
  - Logs: Registration failures with user_id, session_id, connection_id, and error details
  - Sends: Generic "Failed to establish session connection" message
- Router unavailable errors
  - Logs: Warning when router is not configured
  - Sends: Generic "Service temporarily unavailable" message

### Message Router
- `HandleUserMessage`: LLM message processing
  - Logs: LLM service errors with session_id, model_id, and full error details
  - Sends: Generic error via ChatError system with appropriate error codes
- `handleHelpRequest`: Help request processing
  - Logs: Database errors, notification failures with session_id and user_id
  - Sends: Generic error messages via ChatError system
- `handleModelSelection`: Model selection
  - Logs: Database errors (via ChatError system)
  - Sends: Generic error messages via ChatError system
- `handleFileUpload`: File upload processing
  - Logs: Database errors with session_id and full error details
  - Sends: Generic error messages via ChatError system
- `handleChatError`: Centralized error handling
  - Logs: All errors with full context (session_id, error_code, category, cause)
  - Sends: Generic error messages based on error category

### LLM Service
- `SendMessage`: LLM API calls
  - Logs: Request failures with model_id, attempt number, and error details
  - Returns: Wrapped errors to calling code for further handling
- `StreamMessage`: LLM streaming
  - Logs: Stream failures with model_id, attempt number, and error details
  - Returns: Wrapped errors to calling code for further handling

### Notification Service
- `SendHelpRequestAlert`: Help request notifications
  - Logs: Rate limiting, email failures, SMS failures with full error details
  - Returns: Errors to calling code (logged at router level)
- `SendCriticalErrorAlert`: Critical error notifications
  - Logs: Rate limiting, email failures, SMS failures with full error details
  - Returns: Errors to calling code
- `SendSystemAlert`: System alerts
  - Logs: Rate limiting, missing configuration, email failures with full error details
  - Returns: Errors to calling code

## What Gets Logged vs. Sent

### Server-Side Logs (Detailed)
All error locations log comprehensive details including:
- **Full error messages** with stack traces and wrapped error chains
- **Database query details** (via error wrapping, not raw queries)
- **JWT token validation failures** (error type, not token contents)
- **User context**: User IDs, session IDs, connection IDs
- **Request parameters**: Query parameters, path parameters
- **Internal service errors**: LLM failures, S3 errors, notification failures
- **Component tags**: "http", "websocket", "auth" for filtering
- **Attempt counts**: For retry logic (LLM service)
- **Rate limiting**: When requests are rate limited

### Client Responses (Generic)
Clients receive only safe, generic information:
- **Generic error messages**: "An internal error occurred", "Authentication failed"
- **Error codes**: Machine-readable codes for programmatic handling (e.g., "INTERNAL_ERROR", "INVALID_TOKEN")
- **Recoverability information**: Whether the error is recoverable
- **Retry-after hints**: For rate limits (via ChatError system)
- **No internal details**: No file paths, database names, query structures, or stack traces

### Logging Patterns by Layer

#### HTTP Layer (chatbox.go)
```go
// Pattern: Log with context, then send generic error
logger.Error("Failed to list sessions",
    "user_id", userID,
    "error", err,
    "component", "http")
httperrors.RespondInternalError(c)
```

#### WebSocket Layer (handler.go)
```go
// Pattern: Log with connection context, then send generic error message
h.logger.Error("Failed to route message",
    "user_id", c.UserID,
    "session_id", c.SessionID,
    "connection_id", c.ConnectionID,
    "message_type", msg.Type,
    "error", err)
// Send generic error via WebSocket
errorMsg := &message.Message{
    Type: message.TypeError,
    Error: &message.ErrorInfo{
        Code: "SERVICE_ERROR",
        Message: "Failed to process message",
        Recoverable: true,
    },
}
```

#### Router Layer (router.go)
```go
// Pattern: Log with session context, use ChatError system
mr.logger.Error("LLM service error",
    "session_id", msg.SessionID,
    "model_id", modelID,
    "error", err)
// Send structured error via ChatError
llmErr := chaterrors.ErrLLMUnavailable(err)
errorMsg := &message.Message{
    Type: message.TypeError,
    Error: llmErr.ToErrorInfo(),
}
```

#### Service Layer (llm.go, notification.go)
```go
// Pattern: Log failures, return wrapped errors
s.logger.Error("LLM request failed after all retries",
    "model_id", modelID,
    "max_retries", maxRetries,
    "error", lastErr)
return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
```

## Testing

The `internal/httperrors/httperrors_test.go` file includes:

1. **Unit Tests**: Verify each error response function
2. **Sanitization Tests**: Ensure messages don't contain internal details
3. **Format Tests**: Verify JSON response structure

Run tests:
```bash
go test ./internal/httperrors/... -v
```

## Security Benefits

1. **No Information Disclosure**: Attackers can't learn about internal architecture
2. **No Token Leakage**: JWT validation errors don't expose token contents
3. **No Query Leakage**: Database errors don't expose query structure
4. **No Path Leakage**: File system errors don't expose internal paths
5. **Consistent Responses**: All errors follow the same safe pattern

## Monitoring and Debugging

While clients receive generic errors, operators have full visibility through:

1. **Structured Logging**: All errors logged with context
2. **Error Tracking**: Correlation IDs for tracing
3. **Metrics**: Error rates by type and endpoint
4. **Audit Logs**: Security-relevant events

## Future Enhancements

1. **Error IDs**: Add unique error IDs for support correlation
2. **Localization**: Support multiple languages for error messages
3. **Rate Limit Headers**: Add standard rate limit headers
4. **Error Analytics**: Track error patterns for proactive fixes
