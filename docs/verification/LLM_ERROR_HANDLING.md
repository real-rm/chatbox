# LLM Error Handling Implementation

## Overview
This document describes the error handling implementation for LLM failures in the chatbox WebSocket application, addressing task 2.3 of the production-readiness spec.

## Requirements
From section 1.2 LLM Integration:
- LLM errors are handled and communicated to the user
- Error messages are user-friendly and don't leak internal details
- Errors are properly logged for debugging

## Implementation

### 1. Error Types
All LLM errors are wrapped using the `ErrLLMUnavailable` error type from `internal/errors`:
- **Error Code**: `LLM_UNAVAILABLE`
- **Message**: "AI service is temporarily unavailable"
- **Category**: Service error
- **Recoverable**: Yes (users can retry)

### 2. Error Handling Flow

#### LLM Service Layer (`internal/llm/llm.go`)
- Implements retry logic with exponential backoff (max 3 attempts)
- Distinguishes between retryable and non-retryable errors:
  - **Retryable**: Network timeouts, 5xx errors, rate limits (429), service unavailable
  - **Non-retryable**: 4xx client errors (except 429), authentication failures
- Logs all errors with full context (model ID, attempt number, error details)
- Returns wrapped errors with context

#### Router Layer (`internal/router/router.go`)
- Catches LLM service errors during streaming
- Wraps errors using `chaterrors.ErrLLMUnavailable(err)`
- Sends user-friendly error messages to clients
- Logs detailed error information server-side only
- Error messages sent to clients include:
  - Generic error message (no internal details)
  - Error code for client-side handling
  - Recoverable flag (always true for LLM errors)

### 3. Error Message Sanitization
- Client-facing messages are generic: "AI service is temporarily unavailable"
- Internal details (IPs, credentials, stack traces) are logged server-side only
- Error wrapping preserves the cause chain for debugging without exposing it to clients

### 4. User Experience
When an LLM failure occurs:
1. User sends a message
2. Loading indicator is displayed
3. If LLM fails, user receives:
   - Error type: `TypeError`
   - Error code: `LLM_UNAVAILABLE`
   - Message: "AI service is temporarily unavailable"
   - Recoverable: `true` (user can retry)
4. Connection remains open (error is recoverable)

## Testing

### Unit Tests (`internal/router/streaming_test.go`)
1. **TestStreamingErrorHandling**: Verifies basic error handling during streaming
2. **TestLLMErrorScenarios**: Tests various failure scenarios (network timeout, service unavailable, rate limits)
3. **TestStreamingChunkError**: Tests mid-stream failures
4. **TestLLMErrorDoesNotLeakInternalDetails**: Verifies error messages don't expose internal details

### Property Tests (`internal/llm/llm_property_test.go`)
- **TestProperty_LLMBackendRetryLogic**: Verifies retry logic with exponential backoff across many scenarios

### Integration Tests
- **TestReadPump_RoutingErrorHandling**: End-to-end test of error handling from WebSocket to router

## Error Scenarios Covered

| Scenario | Retry | User Message | Connection |
|----------|-------|--------------|------------|
| Network timeout | Yes (3x) | "AI service is temporarily unavailable" | Stays open |
| Service unavailable (5xx) | Yes (3x) | "AI service is temporarily unavailable" | Stays open |
| Rate limit (429) | Yes (3x) | "AI service is temporarily unavailable" | Stays open |
| Authentication error (401) | No | "AI service is temporarily unavailable" | Stays open |
| Invalid request (4xx) | No | "AI service is temporarily unavailable" | Stays open |
| Stream establishment failure | Yes (3x) | "AI service is temporarily unavailable" | Stays open |
| Mid-stream connection drop | No | Partial content delivered | Stays open |

## Logging
All LLM errors are logged with appropriate context:
- **Error level**: For failures after all retries
- **Warn level**: For individual retry attempts
- **Info level**: For successful requests and retry attempts
- **Context included**: session_id, model_id, attempt number, error details

## Security Considerations
- Error messages never expose:
  - Internal IP addresses or hostnames
  - Database connection strings
  - API keys or credentials
  - Stack traces
  - Internal service names or versions
- All sensitive details are logged server-side only
- Error codes are generic and safe to expose

## Future Enhancements
Potential improvements for future iterations:
1. Different error messages for different failure types (rate limit vs. service down)
2. Estimated retry time in error messages
3. Circuit breaker pattern for repeated failures
4. Fallback to alternative LLM providers
5. Graceful degradation (cached responses, simplified mode)
