# Design: Code Quality Improvements

## Overview
This document outlines the design for comprehensive code quality improvements across the chatbox codebase.

## 1. Magic Numbers and Strings Elimination

### 1.1 Constants Package Structure
Create `internal/constants/constants.go` to centralize all constants:

```go
package constants

// HTTP Status Codes
const (
    StatusOK                  = 200
    StatusTooManyRequests     = 429
    StatusServiceUnavailable  = 503
)

// Timeouts
const (
    DefaultContextTimeout     = 10 * time.Second
    LongContextTimeout        = 30 * time.Second
    DefaultLLMStreamTimeout   = 120 * time.Second
    MongoIndexTimeout         = 30 * time.Second
    ShortTimeout              = 2 * time.Second
    MessageAddTimeout         = 5 * time.Second
    SessionEndTimeout         = 5 * time.Second
    HealthCheckTimeout        = 2 * time.Second
    MetricsTimeout            = 30 * time.Second
)

// Sizes and Limits
const (
    DefaultMaxMessageSize     = 1048576  // 1MB in bytes
    EncryptionKeyLength       = 32       // AES-256 requires 32 bytes
    DefaultSessionLimit       = 100
    MaxSessionLimit           = 1000
    DefaultRateLimit          = 100
    DefaultAdminRateLimit     = 20
    MaxRetryAttempts          = 3
)

// Durations
const (
    DefaultReconnectTimeout   = 15 * time.Minute
    DefaultRateWindow         = 1 * time.Minute
    DefaultCleanupInterval    = 5 * time.Minute
    DefaultSessionTTL         = 15 * time.Minute
    InitialRetryDelay         = 100 * time.Millisecond
    MaxRetryDelay             = 2 * time.Second
    RetryMultiplier           = 2.0
)

// Role Names
const (
    RoleAdmin                 = "admin"
    RoleChatAdmin             = "chat_admin"
)

// Sender Types
const (
    SenderUser                = "user"
    SenderAI                  = "ai"
    SenderAdmin               = "admin"
)

// Default Values
const (
    DefaultMongoURI           = "mongodb://localhost:27017"
    DefaultDatabase           = "chat"
    DefaultCollection         = "sessions"
    DefaultModel              = "gpt-4"
    DefaultPort               = 8080
    DefaultLogLevel           = "info"
    DefaultLogDir             = "logs"
)

// HTTP Headers
const (
    HeaderAuthorization       = "Authorization"
    HeaderRetryAfter          = "Retry-After"
    BearerPrefix              = "Bearer "
    BearerPrefixLength        = 7
)

// Error Messages
const (
    ErrMsgInvalidAuthHeader   = "Invalid or missing Authorization header"
    ErrMsgInvalidToken        = "Invalid or expired token"
    ErrMsgForbidden           = "Insufficient permissions"
    ErrMsgInternalError       = "Internal server error"
    ErrMsgRateLimitExceeded   = "Too many requests. Please try again later."
    ErrMsgInvalidTimeFormat   = "Invalid time format. Use RFC3339 format."
    ErrMsgSessionIDRequired   = "Session ID is required"
)

// MongoDB Field Names
const (
    MongoFieldID              = "_id"
    MongoFieldUserID          = "uid"
    MongoFieldTimestamp       = "ts"
    MongoFieldEndTime         = "endTs"
    MongoFieldAdminAssisted   = "adminAssisted"
)

// Index Names
const (
    IndexUserID               = "idx_user_id"
    IndexStartTime            = "idx_start_time"
    IndexAdminAssisted        = "idx_admin_assisted"
    IndexUserStartTime        = "idx_user_start_time"
)

// Token Estimation
const (
    CharsPerToken             = 4  // Rough estimate: 4 characters per token
)

// Weak Secrets (for validation)
var WeakSecrets = []string{
    "secret", "test", "test123", "password", "admin",
    "changeme", "default", "example", "demo", "12345",
}

// Minimum Security Requirements
const (
    MinJWTSecretLength        = 32
    MinPasswordLength         = 8
)
```

### 1.2 Migration Strategy
1. Create constants package
2. Update imports in all files
3. Replace magic values with constants
4. Run tests to ensure no breakage

## 2. If-Without-Else Analysis

### 2.1 Review Categories
- **Early returns**: Document that else is not needed (guard clauses)
- **Optional operations**: Document that else is not needed (fire-and-forget)
- **Potential bugs**: Add else clause or fix logic

### 2.2 Documentation Format
```go
// No else needed: early return pattern (guard clause)
if err != nil {
    return err
}

// No else needed: optional operation (fire-and-forget)
if condition {
    doOptionalThing()
}
// Continue with main logic
```

## 3. HTTP Path Prefix Configuration

### 3.1 Configuration Structure
Add to config:
```go
type ServerConfig struct {
    // ... existing fields
    PathPrefix string // Default: "/chatbox"
}
```

### 3.2 Environment Variable
- `CHATBOX_PATH_PREFIX` - overrides config file
- Default: `/chatbox`
- Validation: must start with `/`

### 3.3 Implementation
```go
// In chatbox.go Register function
pathPrefix := getPathPrefix(config)
chatGroup := r.Group(pathPrefix)
```

### 3.4 Backward Compatibility
- Document migration path from `/chat` to `/chatbox`
- Provide configuration example for both

## 4. Nginx Configuration Documentation

### 4.1 Document Structure
Create `docs/NGINX_SETUP.md`:
- Basic reverse proxy configuration
- WebSocket upgrade configuration
- SSL/TLS termination
- Load balancing
- Health check configuration
- Rate limiting
- Security headers

### 4.2 Configuration Templates
Provide ready-to-use nginx.conf templates for:
- Single server deployment
- Multi-server load balanced deployment
- SSL/TLS with Let's Encrypt
- Development vs Production

## 5. DRY Principle Violations

### 5.1 Identified Duplications

#### 5.1.1 Context with Timeout Pattern
**Current**: Repeated in multiple files
```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
```

**Solution**: Create utility function
```go
// internal/util/context.go
func NewTimeoutContext(timeout time.Duration) (context.Context, context.CancelFunc) {
    return context.WithTimeout(context.Background(), timeout)
}
```

#### 5.1.2 Error Logging Pattern
**Current**: Repeated error logging
```go
logger.Error("Failed to X", "error", err, "component", "Y")
```

**Solution**: Create logging helpers
```go
// internal/util/logging.go
func LogError(logger *golog.Logger, component, operation string, err error, fields ...interface{}) {
    allFields := append([]interface{}{"error", err, "component", component, "operation", operation}, fields...)
    logger.Error(fmt.Sprintf("Failed to %s", operation), allFields...)
}
```

#### 5.1.3 JWT Token Extraction
**Current**: Duplicated in authMiddleware and userAuthMiddleware
**Solution**: Extract to shared function

#### 5.1.4 Message Marshaling
**Current**: Repeated JSON marshaling with error handling
**Solution**: Create utility function

### 5.2 New Utility Package
Create `internal/util/` with:
- `context.go` - Context creation helpers
- `logging.go` - Logging helpers
- `json.go` - JSON marshaling/unmarshaling helpers
- `validation.go` - Common validation functions

## 6. Test Coverage Improvements

### 6.1 Coverage Targets
- internal/router: 80%
- internal/errors: 80%
- internal/storage: 80%
- chatbox.go: 80%
- cmd/server: 80%

### 6.2 Test Strategy

#### 6.2.1 internal/router
Add tests for:
- Error handling paths
- Edge cases in message routing
- Admin takeover scenarios
- Rate limiting
- Concurrent operations

#### 6.2.2 internal/errors
Add tests for:
- All error types
- Error serialization
- Error code validation
- ToErrorInfo conversion

#### 6.2.3 internal/storage
Add tests for:
- Encryption/decryption edge cases
- Retry logic with various error types
- Index creation
- Concurrent operations
- Large dataset handling

#### 6.2.4 chatbox.go
Add tests for:
- All HTTP handlers
- Middleware functions
- Configuration validation
- Graceful shutdown
- Health checks

#### 6.2.5 cmd/server
Add tests for:
- Configuration loading
- Startup sequence
- Signal handling
- Integration scenarios

### 6.3 Test Utilities
Create test helpers in `internal/testutil/`:
- Mock implementations
- Test data generators
- Assertion helpers
- Setup/teardown utilities

## 7. Implementation Order

1. **Phase 1: Foundation**
   - Create constants package
   - Create util package
   - Update imports

2. **Phase 2: Magic Numbers/Strings**
   - Replace all magic values with constants
   - Run tests after each file

3. **Phase 3: If-Without-Else**
   - Review and document all cases
   - Fix any bugs found

4. **Phase 4: Path Prefix**
   - Add configuration
   - Update route registration
   - Update documentation

5. **Phase 5: DRY Violations**
   - Extract common functions
   - Update all call sites
   - Run tests

6. **Phase 6: Nginx Documentation**
   - Create documentation
   - Add configuration examples

7. **Phase 7: Test Coverage**
   - Add missing tests
   - Fix failing tests
   - Verify coverage targets

## 8. Rollback Strategy

Each phase is independent and can be rolled back:
- Constants: Revert to magic values
- Util functions: Inline the code
- Path prefix: Revert to hardcoded `/chat`
- Tests: Can be added incrementally

## 9. Validation

After each phase:
1. Run full test suite: `make test`
2. Run with race detector: `make test-race`
3. Check test coverage: `make coverage`
4. Run linter: `make lint`
5. Build and run integration tests

## 10. Documentation Updates

Update the following documents:
- README.md - Configuration examples
- DEPLOYMENT.md - Path prefix configuration
- docs/NGINX_SETUP.md - New document
- config.toml - Add path_prefix example
