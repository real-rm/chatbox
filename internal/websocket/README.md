# WebSocket Package

This package provides WebSocket connection handling with JWT authentication for the chat application.

## Features

- HTTP to WebSocket protocol upgrade
- JWT token authentication and validation
- Connection management with user context
- Thread-safe connection tracking

## Usage

```go
import (
    "github.com/real-rm/chatbox/internal/auth"
    "github.com/real-rm/chatbox/internal/websocket"
)

// Create JWT validator
validator := auth.NewJWTValidator("your-secret-key")

// Create WebSocket handler
handler := websocket.NewHandler(validator)

// Register HTTP endpoint
http.HandleFunc("/ws", handler.HandleWebSocket)
```

## Authentication

The handler supports two methods for passing JWT tokens:

1. **Query Parameter**: `ws://localhost:8080/ws?token=<jwt-token>`
2. **Authorization Header**: `Authorization: Bearer <jwt-token>`

## Connection Struct

Each WebSocket connection is represented by a `Connection` struct containing:

- `UserID`: Authenticated user's ID from JWT
- `Roles`: User's roles from JWT claims
- `SessionID`: Current session identifier (set later in session management)
- `send`: Buffered channel for outbound messages
- `conn`: Underlying WebSocket connection

## Testing

The package includes:

- Unit tests for authentication and connection creation
- Property-based tests validating:
  - Property 3: Connection User Association (Requirement 1.4)
  - Property 4: WebSocket Connection Establishment (Requirement 2.1)

Run tests with:
```bash
go test -v ./internal/websocket/
```

## Requirements Satisfied

- **Requirement 1.4**: Connection associated with authenticated user ID
- **Requirement 2.1**: Bidirectional WebSocket connection establishment
