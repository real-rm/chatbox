# Code Quality Standards

This document describes the code quality standards and improvements implemented in the chatbox application.

## Overview

The chatbox codebase follows strict code quality standards to ensure maintainability, reliability, and ease of understanding. All code adheres to clean code principles including elimination of magic numbers/strings, DRY (Don't Repeat Yourself) principle, and comprehensive testing.

## Magic Numbers and Strings Elimination

### Problem

Magic numbers and strings scattered throughout code make it:
- Hard to maintain (changes require finding all occurrences)
- Error-prone (typos and inconsistencies)
- Difficult to understand (unclear meaning)

### Solution

All constants are centralized in `internal/constants/constants.go`:

```go
// Timeouts for various operations
const (
    DefaultContextTimeout   = 10 * time.Second  // Standard database operations
    LongContextTimeout      = 30 * time.Second  // Complex queries
    DefaultLLMStreamTimeout = 120 * time.Second // LLM streaming requests
    // ... more timeouts
)

// Sizes and Limits
const (
    DefaultMaxMessageSize = 1048576 // 1MB in bytes
    EncryptionKeyLength   = 32      // AES-256 requires 32 bytes
    DefaultRateLimit      = 100     // Messages per minute per user
    // ... more limits
)

// MongoDB Field Names
const (
    MongoFieldID            = "_id"
    MongoFieldUserID        = "uid"
    MongoFieldTimestamp     = "ts"
    // ... more field names
)
```

### Benefits

- **Single source of truth**: Change once, applies everywhere
- **Self-documenting**: Constant names explain their purpose
- **Type-safe**: Compiler catches errors
- **Easy to find**: All constants in one place

### Usage Example

**Before**:
```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
```

**After**:
```go
ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultContextTimeout)
```

## DRY Principle (Don't Repeat Yourself)

### Problem

Code duplication leads to:
- Maintenance burden (fix bugs in multiple places)
- Inconsistencies (different implementations of same logic)
- Increased code size
- Higher chance of errors

### Solution

Common functionality extracted to `internal/util/` package:

#### Context Helpers (`util/context.go`)

**Before** (repeated everywhere):
```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
```

**After** (centralized):
```go
ctx, cancel := util.NewTimeoutContext(constants.DefaultContextTimeout)
defer cancel()
```

#### Auth Helpers (`util/auth.go`)

**Before** (duplicated in multiple middleware):
```go
authHeader := c.GetHeader("Authorization")
if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
    return "", errors.New("invalid auth header")
}
token := authHeader[7:]
```

**After** (single implementation):
```go
token, err := util.ExtractBearerToken(c)
if err != nil {
    return "", err
}
```

#### JSON Helpers (`util/json.go`)

**Before** (repeated marshaling logic):
```go
data, err := json.Marshal(message)
if err != nil {
    logger.Error("Failed to marshal message", "error", err)
    return err
}
```

**After** (centralized with consistent error handling):
```go
data, err := util.MarshalJSON(message)
if err != nil {
    logger.Error("Failed to marshal message", "error", err)
    return err
}
```

#### Validation Helpers (`util/validation.go`)

Common validation functions:
- `IsValidEmail(email string) bool`
- `IsValidPhoneNumber(phone string) bool`
- `IsWeakSecret(secret string) bool`
- `ValidateJWTSecret(secret string) error`

#### Logging Helpers (`util/logging.go`)

Consistent error logging patterns:
```go
util.LogError(logger, "storage", "add message", err, "sessionID", sessionID)
```

### Benefits

- **Consistency**: Same logic everywhere
- **Maintainability**: Fix once, applies everywhere
- **Testability**: Test once, covers all uses
- **Readability**: Less code to read and understand

## Documented Code Patterns

### If-Without-Else Documentation

All "if without else" patterns are documented with clear reasoning:

#### Early Return Pattern (Guard Clauses)

```go
// No else needed: early return pattern (guard clause)
if err != nil {
    return err
}
// Continue with main logic
```

#### Optional Operations (Fire-and-Forget)

```go
// No else needed: optional operation (fire-and-forget)
if shouldNotify {
    go sendNotification()
}
// Continue regardless of notification
```

#### State Checks

```go
// No else needed: state check with early return
if session.IsEnded() {
    return ErrSessionEnded
}
// Continue with active session logic
```

### Benefits

- **Clarity**: Readers understand why there's no else
- **Maintainability**: Future developers know the intent
- **Code review**: Easier to verify correctness

## Test Coverage

### Coverage Targets

All major packages maintain 80%+ test coverage:

- `internal/router`: 80%+
- `internal/errors`: 80%+
- `internal/storage`: 80%+
- `internal/session`: 80%+
- `internal/websocket`: 80%+
- `chatbox.go`: 80%+
- `cmd/server`: 80%+

### Test Types

#### Unit Tests

Test specific examples and edge cases:
```go
func TestExtractBearerToken_Valid(t *testing.T) {
    // Test valid token extraction
}

func TestExtractBearerToken_Missing(t *testing.T) {
    // Test missing header
}

func TestExtractBearerToken_InvalidFormat(t *testing.T) {
    // Test invalid format
}
```

#### Property-Based Tests

Validate universal properties across all inputs:
```go
func TestRateLimiter_Properties(t *testing.T) {
    properties := gopter.NewProperties(nil)
    
    properties.Property("rate limiter never allows more than limit", prop.ForAll(
        func(events int) bool {
            // Test property holds for all event counts
        },
        gen.IntRange(0, 1000),
    ))
    
    properties.TestingRun(t)
}
```

#### Integration Tests

Test end-to-end flows:
```go
func TestMessageFlow_Integration(t *testing.T) {
    // Setup: Create session, connect WebSocket
    // Action: Send message, route to LLM, receive response
    // Verify: Message stored, response delivered
}
```

### Benefits

- **Confidence**: High coverage catches regressions
- **Documentation**: Tests show how code should be used
- **Refactoring**: Safe to refactor with good tests
- **Quality**: Forces thinking about edge cases

## HTTP Path Prefix Configuration

### Feature

Configurable HTTP path prefix for all routes:

```bash
# Default
CHATBOX_PATH_PREFIX="/chatbox"
# Routes: /chatbox/ws, /chatbox/healthz, etc.

# Custom
CHATBOX_PATH_PREFIX="/api/v1/chat"
# Routes: /api/v1/chat/ws, /api/v1/chat/healthz, etc.
```

### Benefits

- **Flexibility**: Run multiple services on same domain
- **API Versioning**: Easy to version your API
- **Integration**: Fits into existing API structures
- **Namespace Separation**: Multi-tenant environments

### Implementation

Uses constants and configuration:
```go
// Default in constants
const DefaultPathPrefix = "/chatbox"

// Configurable via environment or config file
pathPrefix := config.GetPathPrefix() // Falls back to DefaultPathPrefix

// Applied to all routes
chatGroup := r.Group(pathPrefix)
```

## Nginx Configuration

### Documentation

Comprehensive nginx setup documentation in [docs/NGINX_SETUP.md](NGINX_SETUP.md):

- Basic reverse proxy configuration
- WebSocket upgrade handling
- SSL/TLS termination with Let's Encrypt
- Load balancing across multiple instances
- Health check configuration
- Rate limiting and security headers
- Ready-to-use configuration templates

### Configuration Templates

Pre-built templates in `deployments/nginx/`:
- `basic.conf` - Single server deployment
- `load-balanced.conf` - Multi-server with load balancing
- `ssl.conf` - SSL/TLS configuration
- `development.conf` - Development environment

### Benefits

- **Production-ready**: Tested configurations
- **Security**: Best practices included
- **Performance**: Optimized settings
- **Flexibility**: Easy to customize

## Code Organization

### Package Structure

```
internal/
├── constants/          # All constants (no magic numbers/strings)
│   └── constants.go
├── util/              # Shared utilities (DRY principle)
│   ├── context.go     # Context helpers
│   ├── auth.go        # Auth helpers
│   ├── json.go        # JSON helpers
│   ├── validation.go  # Validation helpers
│   └── logging.go     # Logging helpers
├── auth/              # JWT authentication
├── config/            # Configuration management
├── router/            # Message routing
├── storage/           # MongoDB storage
├── websocket/         # WebSocket handling
└── ...                # Other domain packages
```

### Benefits

- **Clear separation**: Each package has single responsibility
- **Easy to find**: Logical organization
- **Reusability**: Util package used everywhere
- **Maintainability**: Changes localized to packages

## Validation and Error Handling

### Startup Validation

All configuration validated at startup:
```go
// Validate JWT secret
if err := util.ValidateJWTSecret(config.JWTSecret); err != nil {
    log.Fatal("Invalid JWT secret", "error", err)
}

// Validate encryption key
if err := validateEncryptionKey(config.EncryptionKey); err != nil {
    log.Fatal("Invalid encryption key", "error", err)
}

// Validate path prefix
if err := validatePathPrefix(config.PathPrefix); err != nil {
    log.Fatal("Invalid path prefix", "error", err)
}
```

### Benefits

- **Fail fast**: Catch errors before serving traffic
- **Clear messages**: Know exactly what's wrong
- **Security**: Prevent weak configurations
- **Reliability**: No runtime surprises

## Best Practices

### Adding New Constants

1. Add to appropriate section in `internal/constants/constants.go`
2. Add documentation comment
3. Use descriptive name
4. Group with related constants

### Adding New Utilities

1. Identify repeated code pattern
2. Extract to appropriate file in `internal/util/`
3. Write comprehensive tests
4. Update all call sites
5. Document usage

### Writing Tests

1. Test happy path
2. Test error cases
3. Test edge cases
4. Add property-based tests for universal properties
5. Aim for 80%+ coverage

### Documenting Code

1. Document all exported functions
2. Explain "if without else" patterns
3. Add examples for complex logic
4. Keep comments up to date

## Maintenance

### Updating Constants

1. Change value in `internal/constants/constants.go`
2. Run tests: `go test ./...`
3. Update documentation if needed
4. Deploy

### Refactoring

1. Ensure good test coverage first
2. Make changes incrementally
3. Run tests after each change
4. Update documentation
5. Review with team

### Adding Features

1. Follow existing patterns
2. Use constants instead of magic values
3. Extract common logic to util package
4. Write tests (unit + property-based + integration)
5. Document new functionality

## Metrics and Monitoring

### Code Quality Metrics

Track these metrics:
- Test coverage (target: 80%+)
- Code duplication (target: <5%)
- Cyclomatic complexity (target: <10 per function)
- Documentation coverage (target: 100% of exported symbols)

### Tools

- `go test -cover ./...` - Test coverage
- `go vet ./...` - Static analysis
- `golangci-lint run` - Comprehensive linting
- `gocyclo .` - Complexity analysis

## References

- [Clean Code by Robert C. Martin](https://www.amazon.com/Clean-Code-Handbook-Software-Craftsmanship/dp/0132350882)
- [The Pragmatic Programmer](https://www.amazon.com/Pragmatic-Programmer-journey-mastery-Anniversary/dp/0135957052)
- [Effective Go](https://golang.org/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

## Summary

The chatbox application maintains high code quality through:

1. **No magic numbers/strings** - All constants centralized
2. **DRY principle** - Common functionality in util package
3. **Documented patterns** - Clear reasoning for all code patterns
4. **High test coverage** - 80%+ with multiple test types
5. **Clean architecture** - Clear package organization
6. **Validation** - Fail fast with clear error messages
7. **Best practices** - Following Go and clean code standards

This results in code that is:
- Easy to understand
- Easy to maintain
- Easy to test
- Easy to deploy
- Reliable in production
