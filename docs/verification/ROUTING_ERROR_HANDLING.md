# WebSocket Routing Error Handling

## Overview
This document describes the error handling improvements made to the WebSocket message routing system to ensure routing failures are properly logged and handled gracefully.

## Implementation Details

### Error Handling for Message Routing

The `readPump` function in `internal/websocket/handler.go` now implements comprehensive error handling for routing failures:

1. **Connection Registration Errors**
   - When a connection fails to register with the router, a detailed error is logged server-side
   - An error message is sent to the client with code `SERVICE_ERROR` and message "Failed to establish session connection"
   - The connection continues to operate, allowing retry on subsequent messages
   - Non-blocking send to prevent channel deadlock

2. **Message Routing Errors**
   - All routing errors are logged with full context (user_id, session_id, connection_id, message_type)
   - Errors are checked to see if they are `ChatError` instances
   - If a `ChatError`, the error info is extracted and sent to the client with proper error codes
   - If not a `ChatError`, a generic `SERVICE_ERROR` is sent to the client
   - Detailed error information is logged server-side only (not exposed to clients)
   - Non-blocking send to prevent channel deadlock

### Error Types

The system leverages the existing `ChatError` framework from `internal/errors`:

- **Authentication Errors** (`CategoryAuth`): Fatal errors that close the connection
- **Validation Errors** (`CategoryValidation`): Recoverable errors for invalid input
- **Service Errors** (`CategoryService`): Recoverable errors for backend failures
- **Rate Limit Errors** (`CategoryRateLimit`): Recoverable errors with retry-after information

### Client Error Messages

Error messages sent to clients include:
- `Code`: Specific error code (e.g., `LLM_UNAVAILABLE`, `DATABASE_ERROR`, `SERVICE_ERROR`)
- `Message`: User-friendly error message
- `Recoverable`: Boolean indicating if the client can retry
- `RetryAfter`: Milliseconds to wait before retrying (for rate limit errors)

### Server-Side Logging

All routing errors are logged with:
- Error level: `ERROR` for routing failures
- Context fields: `user_id`, `session_id`, `connection_id`, `message_type`
- Full error details including error code, category, and cause
- Additional debug logging for `ChatError` instances with error category and recoverability

## Testing

Two new integration tests verify the error handling:

1. **TestReadPump_RoutingErrorHandling**
   - Tests that routing errors (e.g., LLM unavailable) are properly handled
   - Verifies error messages are sent to clients with correct error codes
   - Confirms detailed errors are logged server-side

2. **TestReadPump_RegistrationErrorHandling**
   - Tests that connection registration errors are properly handled
   - Verifies error messages are sent to clients
   - Confirms the connection continues to operate after registration failure

## Security Considerations

- Internal error details (stack traces, database queries, etc.) are never exposed to clients
- Only sanitized, user-friendly error messages are sent over WebSocket
- Detailed error information is logged server-side for debugging
- Error codes are standardized and don't leak implementation details

## Requirements Satisfied

This implementation satisfies the following acceptance criteria from the production readiness requirements:

**Section 1.1 WebSocket Message Routing:**
- ✅ Messages received via WebSocket are parsed and validated
- ✅ Valid messages are forwarded to the message router
- ✅ Invalid messages result in error responses to the client
- ✅ Message routing errors are logged and handled gracefully

## Future Improvements

Potential enhancements:
- Add metrics for routing error rates
- Implement circuit breaker pattern for repeated routing failures
- Add error aggregation for monitoring dashboards
- Consider implementing exponential backoff hints in error responses
