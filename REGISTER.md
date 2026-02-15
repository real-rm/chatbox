# Chatbox Service Registration

This document describes the chatbox service registration with gomain.

## Overview

The chatbox service integrates with gomain by implementing a `Register` function that follows the gomain service interface. This function is called by gomain during service initialization to set up all HTTP and WebSocket endpoints for the chat application.

## Function Signature

```go
func Register(
    r *gin.Engine,
    config *goconfig.ConfigAccessor,
    logger *golog.Logger,
    mongo *gomongo.Mongo,
) error
```

### Parameters

- `r`: Gin router for registering HTTP and WebSocket endpoints
- `config`: Configuration accessor for loading service settings
- `logger`: Logger for structured logging
- `mongo`: MongoDB client for data persistence

### Returns

- `error`: Any error that occurred during registration

## Initialization Steps

The Register function performs the following initialization steps:

1. **Create chatbox-specific logger** - Creates a logger with "chatbox" group for all chatbox-related logs

2. **Load configuration** - Loads JWT secret and reconnect timeout from config

3. **Initialize goupload** - Initializes the file upload service with logger and config

4. **Create upload service** - Creates upload service with stats tracking

5. **Create storage service** - Creates storage service for MongoDB persistence

6. **Create session manager** - Creates session manager with reconnect timeout

7. **Create LLM service** - Creates LLM service for AI backend integration

8. **Create notification service** - Creates notification service for email/SMS alerts

9. **Create message router** - Creates message router for routing messages between clients and services

10. **Create JWT validator** - Creates JWT validator for authentication

11. **Create WebSocket handler** - Creates WebSocket handler with Gin adapter

12. **Register routes** - Registers all HTTP and WebSocket endpoints

## Registered Endpoints

### WebSocket Endpoint

- `GET /chat/ws` - WebSocket endpoint for real-time chat communication
  - Requires JWT token in query parameter or Authorization header
  - Upgrades HTTP connection to WebSocket
  - Handles bidirectional message exchange

### Admin HTTP Endpoints

All admin endpoints require JWT authentication with admin role:

- `GET /chat/admin/sessions` - List all sessions with filtering and sorting
- `GET /chat/admin/metrics` - Get session metrics and statistics
- `POST /chat/admin/takeover/:sessionID` - Initiate admin session takeover

### Health Check Endpoints

- `GET /chat/healthz` - Liveness probe for Kubernetes
- `GET /chat/readyz` - Readiness probe for Kubernetes (checks MongoDB connection)

## Configuration Requirements

The following configuration keys are required in `config.toml`:

```toml
[chatbox]
jwt_secret = "your-jwt-secret-key"
reconnect_timeout = "15m"

# LLM providers configuration
[[chatbox.llm.providers]]
id = "openai-gpt4"
name = "GPT-4"
type = "openai"
endpoint = "https://api.openai.com/v1"
apiKey = "sk-..."
model = "gpt-4"

# Upload configuration
[userupload]
site = "CHAT"
  [[userupload.types]]
    entryName = "uploads"
    prefix = "/chat-files"
    tmpPath = "./temp/uploads"
    maxSize = "100MB"
    storage = [
      { type = "s3", target = "aws-chat-storage", bucket = "chat-files" }
    ]

# S3 configuration
[connection_sources]
  [[connection_sources.s3_providers]]
    name = "aws-chat-storage"
    endpoint = "s3.us-east-1.amazonaws.com"
    key = "YOUR_AWS_ACCESS_KEY"
    pass = "YOUR_AWS_SECRET_KEY"
    region = "us-east-1"

# Mail configuration
[mail]
defaultFromName = "Chat Support"
replyToEmail = "support@example.com"
adminEmail = "admin@example.com"

[[mail.engines.ses]]
name = "ses-primary"
accessKeyId = "AKIAXXXXXXXX"
secretAccessKey = "xxxxxxxx"
region = "us-east-1"
from = "noreply@example.com"

# SMS configuration (optional)
[sms]
accountSID = "ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
authToken = "your_auth_token"
```

## Integration with gomain

To integrate the chatbox service with gomain:

1. Add the chatbox module to gomain's `go.mod`:
   ```
   require github.com/real-rm/chatbox v0.1.0
   ```

2. Import the chatbox package in gomain's service registry:
   ```go
   import "github.com/real-rm/chatbox"
   ```

3. Call the Register function during service initialization:
   ```go
   if err := chatbox.Register(router, config, logger, mongo); err != nil {
       log.Fatalf("Failed to register chatbox service: %v", err)
   }
   ```

## WebSocket Handler Gin Adapter

The WebSocket handler is adapted to work with Gin's context by extracting the `http.ResponseWriter` and `*http.Request` from the Gin context:

```go
chatGroup.GET("/ws", func(c *gin.Context) {
    wsHandler.HandleWebSocket(c.Writer, c.Request)
})
```

This allows the existing WebSocket handler (which uses `http.ResponseWriter` and `*http.Request`) to work seamlessly with Gin's routing system.

## Authentication Middleware

The `authMiddleware` function provides JWT authentication for admin endpoints:

1. Extracts JWT token from Authorization header
2. Validates token signature and expiration
3. Checks for admin role in token claims
4. Stores claims in Gin context for use by handlers
5. Returns 401 for invalid tokens or 403 for insufficient permissions

## Error Handling

The Register function returns errors for:

- Missing or invalid JWT secret configuration
- Invalid reconnect timeout format
- Failed goupload initialization
- Failed upload service creation
- Failed storage service creation
- Failed LLM service creation
- Failed notification service creation

All errors are wrapped with context using `fmt.Errorf` with `%w` verb for proper error chain handling.

## Logging

The Register function logs:

- Service initialization start
- Successful service registration with endpoint details

All logs use structured logging with key-value pairs for better observability.

## Testing

The chatbox package includes basic tests to verify:

- Register function signature is correct
- Authentication middleware exists
- Health check handlers exist

Full integration testing requires a complete environment setup with:
- MongoDB instance
- S3-compatible storage
- LLM backend configuration
- Email/SMS service configuration

## Next Steps

After implementing the Register function, the following tasks remain:

1. Implement admin HTTP endpoint handlers (session listing, metrics, takeover)
2. Integrate LLM service with message router
3. Implement frontend HTML/JavaScript chat client
4. Create Kubernetes deployment manifests
5. Write integration tests

See the tasks.md file in `.kiro/specs/chat-application-websocket/` for the complete implementation plan.
