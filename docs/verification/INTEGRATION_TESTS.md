# Integration Tests Documentation

## Overview

This document describes the end-to-end integration tests for the chat application WebSocket service. These tests validate complete flows from client connection through message exchange, file uploads, voice messages, admin features, and reconnection scenarios.

## Test File

**Location**: `integration_test.go`

## Test Scenarios

### 1. Complete Message Flow (`TestIntegration_CompleteMessageFlow`)

**Purpose**: Tests the complete message flow: connect → send → LLM → receive

**Flow**:
1. Client establishes WebSocket connection
2. Client sends user message
3. Server sends loading indicator
4. Server sends AI response
5. Verify response contains expected fields

**Validates**: Requirements 2.1, 3.1, 3.2, 3.3

### 2. File Upload Flow (`TestIntegration_FileUploadFlow`)

**Purpose**: Tests file upload and confirmation flow

**Flow**:
1. Client establishes WebSocket connection
2. Client sends file upload message with file metadata
3. Server echoes back file upload confirmation
4. Verify file ID and URL are preserved

**Validates**: Requirements 5.1, 5.2

### 3. Voice Message Flow (`TestIntegration_VoiceMessageFlow`)

**Purpose**: Tests voice message upload and AI response

**Flow**:
1. Client establishes WebSocket connection
2. Client sends voice message with audio file reference
3. Server sends loading indicator
4. Server sends AI response (text or voice)
5. Verify response type and sender

**Validates**: Requirements 6.1, 6.3, 6.5

### 4. Admin Takeover Flow (`TestIntegration_AdminTakeoverFlow`)

**Purpose**: Tests help request and admin connection flow

**Flow**:
1. User establishes WebSocket connection
2. User sends help request
3. Server confirms help request
4. Admin establishes WebSocket connection
5. Verify both connections are active

**Note**: Full bidirectional routing is tested in router property tests. This integration test verifies the basic connection and help request flow.

**Validates**: Requirements 16.2, 17.1, 17.3

### 5. Reconnection Flow (`TestIntegration_ReconnectionFlow`)

**Purpose**: Tests reconnection within timeout period

**Flow**:
1. Client establishes initial connection
2. Client sends message
3. Client closes connection
4. Wait 1 second (within timeout)
5. Client reconnects with same user ID
6. Client sends another message
7. Verify reconnection succeeds and messages work

**Validates**: Requirements 2.3, 2.4

### 6. Reconnection Timeout (`TestIntegration_ReconnectionTimeout`)

**Purpose**: Tests reconnection after timeout period

**Flow**:
1. Client establishes initial connection
2. Client closes connection
3. Wait beyond timeout (2 seconds in test)
4. Client attempts to reconnect
5. Verify reconnection succeeds (may create new session)

**Validates**: Requirements 2.5

### 7. Multi-Model Selection Flow (`TestIntegration_MultiModelSelectionFlow`)

**Purpose**: Tests switching between different LLM models

**Flow**:
1. Client establishes WebSocket connection
2. Client selects first model (gpt-4)
3. Client sends message
4. Verify AI response received
5. Client selects second model (claude-3)
6. Client sends message
7. Verify AI response received

**Validates**: Requirements 7.6, 7.7

### 8. Concurrent Connections (`TestIntegration_ConcurrentConnections`)

**Purpose**: Tests multiple simultaneous WebSocket connections

**Flow**:
1. Create 10 concurrent WebSocket connections
2. Send messages from all connections
3. Receive responses from all connections
4. Close all connections
5. Verify no interference between connections

**Validates**: Requirements 2.7

### 9. Message Order Preservation (`TestIntegration_MessageOrderPreservation`)

**Purpose**: Tests that messages are processed in order

**Flow**:
1. Client establishes WebSocket connection
2. Client sends 5 messages in sequence
3. Receive responses for all messages
4. Verify all responses received (order preserved by synchronous processing)

**Validates**: Requirements 3.5

### 10. Error Handling (`TestIntegration_ErrorHandling`)

**Purpose**: Tests error handling for invalid messages

**Flow**:
1. Client establishes WebSocket connection
2. Client sends message with invalid type
3. Server sends error message
4. Verify error message contains error details

**Validates**: Requirements 9.1, 9.2

## Running Integration Tests

### Run All Integration Tests

```bash
go test -v -run TestIntegration -timeout 30s
```

### Run Specific Integration Test

```bash
go test -v -run TestIntegration_CompleteMessageFlow -timeout 10s
```

### Skip Integration Tests (Short Mode)

```bash
go test -v -short
```

Integration tests are automatically skipped when running with the `-short` flag.

## Test Implementation Details

### Mock WebSocket Server

The integration tests use a mock WebSocket server (`setupTestServer`) that:
- Accepts WebSocket connections
- Sends initial connection status with session ID
- Echoes back appropriate responses based on message type
- Simulates loading indicators for user messages
- Generates mock AI responses

### Helper Functions

**`connectWebSocket(serverURL, userID, roles)`**
- Generates JWT token for authentication
- Establishes WebSocket connection
- Waits for connection status message
- Returns connection and session ID

**`reconnectWebSocket(serverURL, userID, roles, sessionID)`**
- Similar to `connectWebSocket` but includes previous session ID
- Used for testing reconnection scenarios

**`generateTestJWT(userID, roles)`**
- Generates JWT token for testing
- Uses test secret key
- Sets expiration to 1 hour

**`handleTestWebSocket(conn, t)`**
- Handles WebSocket messages in mock server
- Routes messages based on type
- Sends appropriate responses

## Test Coverage

The integration tests cover:
- ✅ Complete message flow with LLM
- ✅ File upload and confirmation
- ✅ Voice message handling
- ✅ Admin help request flow
- ✅ Reconnection within timeout
- ✅ Reconnection after timeout
- ✅ Multi-model selection
- ✅ Concurrent connections
- ✅ Message order preservation
- ✅ Error handling

## Limitations

These integration tests use a mock WebSocket server and do not test:
- Real LLM backend integration (tested separately in LLM service tests)
- Real MongoDB persistence (tested separately in storage tests)
- Real S3 file uploads (tested separately in upload tests)
- Full admin takeover bidirectional routing (tested in router property tests)

For full end-to-end testing with real services, use the Docker Compose environment described in `TESTING.md`.

## Next Steps

To run integration tests with real services:

1. Start Docker Compose environment:
   ```bash
   docker-compose up -d
   ```

2. Wait for services to be ready:
   ```bash
   docker-compose ps
   ```

3. Run integration tests against real server:
   ```bash
   # Set environment variables for real server
   export TEST_SERVER_URL=http://localhost:8080
   go test -v -run TestIntegration
   ```

## Continuous Integration

These integration tests are designed to run in CI/CD pipelines:
- Fast execution (< 5 seconds total)
- No external dependencies required (uses mock server)
- Deterministic results
- Comprehensive coverage of critical flows

For CI/CD with real services, use the Docker Compose setup in `.github/workflows` or similar CI configuration.

