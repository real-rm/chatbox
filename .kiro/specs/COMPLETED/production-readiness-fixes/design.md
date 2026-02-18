# Design Document: Production Readiness Fixes

## Overview

This design addresses 10 confirmed production readiness issues identified through comprehensive testing. The issues range from critical memory leaks and data races to high-priority security and reliability concerns. Each fix is designed to be minimal, focused, and independently deployable with clear rollback strategies.

The fixes are prioritized by severity:
- **CRITICAL**: Issues #1 (session memory leak) and #13 (origin validation data race)
- **HIGH**: Issues #8, #11, #17, #18 (timeouts, cleanup, security)
- **MEDIUM**: Issues #12, #19, #9 (bounded growth, validation, retries)
- **LOW**: Issue #15 (shutdown timeout)

## Architecture

### Current System Architecture

The system consists of several key components:
- **Session Manager**: Manages user sessions in memory
- **Message Router**: Routes messages between clients and LLM providers
- **WebSocket Handler**: Handles WebSocket connections and protocol
- **Rate Limiter**: Enforces rate limits on messages and connections
- **Storage Layer**: Persists sessions to MongoDB
- **Config System**: Loads and validates configuration

### Design Principles

1. **Minimal Changes**: Each fix modifies only the necessary code
2. **Backward Compatibility**: No breaking changes to APIs or data formats
3. **Testability**: All fixes include property-based tests
4. **Observability**: Add logging and metrics for monitoring
5. **Graceful Degradation**: Failures don't cascade to other components

## Components and Interfaces

### Issue #1: Session Memory Leak (CRITICAL)

**Problem**: Sessions are marked inactive but never removed from memory, causing unbounded growth.

**Root Cause**: `EndSession()` sets `IsActive = false` but doesn't delete from `sessions` map.

**Solution**: Implement TTL-based cleanup with background goroutine.


**Implementation Approach**:

```go
// Add to SessionManager struct
type SessionManager struct {
    // ... existing fields ...
    cleanupInterval time.Duration
    sessionTTL      time.Duration
    stopCleanup     chan struct{}
    cleanupWg       sync.WaitGroup
}

// New method: StartCleanup
func (sm *SessionManager) StartCleanup() {
    sm.cleanupWg.Add(1)
    go func() {
        defer sm.cleanupWg.Done()
        ticker := time.NewTicker(sm.cleanupInterval)
        defer ticker.Stop()
        
        for {
            select {
            case <-ticker.C:
                sm.cleanupExpiredSessions()
            case <-sm.stopCleanup:
                return
            }
        }
    }()
}

// New method: cleanupExpiredSessions
func (sm *SessionManager) cleanupExpiredSessions() {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    now := time.Now()
    removed := 0
    
    for sessionID, session := range sm.sessions {
        if !session.IsActive && session.EndTime != nil {
            if now.Sub(*session.EndTime) > sm.sessionTTL {
                delete(sm.sessions, sessionID)
                removed++
            }
        }
    }
    
    if removed > 0 {
        sm.logger.Info("Cleaned up expired sessions", "count", removed)
    }
}

// New method: StopCleanup
func (sm *SessionManager) StopCleanup() {
    close(sm.stopCleanup)
    sm.cleanupWg.Wait()
}

// New method: GetMemoryStats
func (sm *SessionManager) GetMemoryStats() (active, inactive, total int) {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    
    for _, session := range sm.sessions {
        total++
        if session.IsActive {
            active++
        } else {
            inactive++
        }
    }
    return
}
```

**Configuration**:
- `cleanupInterval`: 5 minutes (configurable via env var)
- `sessionTTL`: 15 minutes after EndTime (matches reconnect timeout)

**Rollback Strategy**: If cleanup causes issues, set `cleanupInterval` to a very large value (24h) to effectively disable it.


### Issue #13: Origin Validation Data Race (CRITICAL)

**Problem**: `checkOrigin()` reads `allowedOrigins` map without lock while `SetAllowedOrigins()` writes with lock, causing data race that can crash the server.

**Root Cause**: Missing `RLock()` in `checkOrigin()` method.

**Solution**: Add read lock to `checkOrigin()` method.

**Implementation Approach**:

```go
// In internal/websocket/handler.go

// Current (broken) code:
func (h *Handler) checkOrigin(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        return true
    }
    
    // BUG: No lock here!
    for allowed := range h.allowedOrigins {
        if origin == allowed {
            return true
        }
    }
    return false
}

// Fixed code:
func (h *Handler) checkOrigin(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        return true
    }
    
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    for allowed := range h.allowedOrigins {
        if origin == allowed {
            return true
        }
    }
    return false
}
```

**Testing**: Run with `-race` flag to verify no data races.

**Rollback Strategy**: This is a one-line fix with no configuration. If issues arise, revert the commit.


### Issue #8: LLM Streaming Timeout (HIGH)

**Problem**: LLM streaming uses `context.Background()` with no timeout, allowing requests to hang indefinitely.

**Root Cause**: Line 211 in `internal/router/router.go` uses `context.Background()`.

**Solution**: Use `context.WithTimeout()` with configurable timeout.

**Implementation Approach**:

```go
// Add to Config struct
type ServerConfig struct {
    // ... existing fields ...
    LLMStreamTimeout time.Duration
}

// In router.go HandleUserMessage method:

// Current (broken) code:
ctx := context.Background()
chunkChan, err := mr.llmService.StreamMessage(ctx, modelID, llmMessages)

// Fixed code:
timeout := mr.config.Server.LLMStreamTimeout
if timeout == 0 {
    timeout = 120 * time.Second // Default 2 minutes
}

ctx, cancel := context.WithTimeout(context.Background(), timeout)
defer cancel()

startTime := time.Now()
chunkChan, err := mr.llmService.StreamMessage(ctx, modelID, llmMessages)
if err != nil {
    if ctx.Err() == context.DeadlineExceeded {
        mr.logger.Error("LLM streaming timeout",
            "session_id", msg.SessionID,
            "model_id", modelID,
            "timeout", timeout,
            "elapsed", time.Since(startTime))
    }
    // ... existing error handling ...
}
```

**Configuration**:
- Environment variable: `LLM_STREAM_TIMEOUT` (default: 120s)
- Recommended range: 60s - 300s depending on model complexity

**Rollback Strategy**: Set `LLM_STREAM_TIMEOUT` to a very large value (e.g., 1h) to effectively disable timeout.


### Issue #11: Rate Limiter Memory Growth (HIGH)

**Problem**: `Cleanup()` method exists but is never called, causing events to accumulate indefinitely.

**Root Cause**: No background goroutine calls `Cleanup()` periodically.

**Solution**: Add background cleanup goroutine to MessageLimiter.

**Implementation Approach**:

```go
// Add to MessageLimiter struct
type MessageLimiter struct {
    // ... existing fields ...
    cleanupInterval time.Duration
    stopCleanup     chan struct{}
    cleanupWg       sync.WaitGroup
}

// New method: StartCleanup
func (ml *MessageLimiter) StartCleanup() {
    ml.cleanupWg.Add(1)
    go func() {
        defer ml.cleanupWg.Done()
        ticker := time.NewTicker(ml.cleanupInterval)
        defer ticker.Stop()
        
        for {
            select {
            case <-ticker.C:
                before := ml.getEventCount()
                ml.Cleanup()
                after := ml.getEventCount()
                removed := before - after
                if removed > 0 {
                    // Log cleanup stats
                }
            case <-ml.stopCleanup:
                return
            }
        }
    }()
}

// New method: getEventCount
func (ml *MessageLimiter) getEventCount() int {
    ml.mu.RLock()
    defer ml.mu.RUnlock()
    
    count := 0
    for _, events := range ml.events {
        count += len(events)
    }
    return count
}

// New method: StopCleanup
func (ml *MessageLimiter) StopCleanup() {
    close(ml.stopCleanup)
    ml.cleanupWg.Wait()
}
```

**Configuration**:
- `cleanupInterval`: 5 minutes (configurable via env var)
- Cleanup runs independently of rate limit window

**Rollback Strategy**: If cleanup causes performance issues, increase `cleanupInterval` to reduce frequency.


### Issue #17: JWT Secret Validation (HIGH - Security)

**Problem**: No validation of JWT secret strength, allowing weak secrets like "test123".

**Root Cause**: `Config.Validate()` doesn't check JWT secret strength.

**Solution**: Add JWT secret validation to config validation.

**Implementation Approach**:

```go
// Add to config.go Validate() method

// Common weak secrets to reject
var weakSecrets = []string{
    "secret", "test", "test123", "password", "admin",
    "changeme", "default", "example", "demo",
}

func (c *Config) Validate() error {
    var errs []error
    
    // ... existing validation ...
    
    // Validate JWT secret strength
    if c.Server.JWTSecret == "" {
        errs = append(errs, errors.New("JWT secret is required"))
    } else {
        // Check minimum length
        if len(c.Server.JWTSecret) < 32 {
            errs = append(errs, fmt.Errorf(
                "JWT secret must be at least 32 characters (got %d). "+
                "Generate a strong secret with: openssl rand -base64 32",
                len(c.Server.JWTSecret)))
        }
        
        // Check for common weak secrets
        lowerSecret := strings.ToLower(c.Server.JWTSecret)
        for _, weak := range weakSecrets {
            if strings.Contains(lowerSecret, weak) {
                errs = append(errs, fmt.Errorf(
                    "JWT secret appears to be weak (contains '%s'). "+
                    "Use a cryptographically random secret", weak))
                break
            }
        }
    }
    
    // ... rest of validation ...
    
    if len(errs) > 0 {
        return fmt.Errorf("configuration validation failed: %v", errs)
    }
    return nil
}
```

**Configuration**: No new configuration needed. Validation runs at startup.

**Rollback Strategy**: If validation blocks legitimate secrets, adjust the weak secret list or minimum length requirement.


### Issue #18: Admin Endpoint Rate Limiting (HIGH - Security)

**Problem**: Admin endpoints have no rate limiting, vulnerable to abuse and DoS.

**Root Cause**: Admin endpoints don't use rate limiting middleware.

**Solution**: Add separate rate limiter for admin endpoints.

**Implementation Approach**:

```go
// Add to chatbox.go

// Create admin rate limiter
adminLimiter := ratelimit.NewMessageLimiter(1*time.Minute, 20) // 20 req/min
adminLimiter.StartCleanup()

// Admin rate limiting middleware
adminRateLimit := func(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Extract user ID from JWT token
        userID := extractUserIDFromRequest(r)
        
        if !adminLimiter.Allow(userID) {
            retryAfter := adminLimiter.GetRetryAfter(userID)
            w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter/1000))
            w.WriteHeader(http.StatusTooManyRequests)
            json.NewEncoder(w).Encode(map[string]interface{}{
                "error": "rate_limit_exceeded",
                "message": "Too many admin requests",
                "retry_after_ms": retryAfter,
            })
            
            logger.Warn("Admin rate limit exceeded",
                "user_id", userID,
                "endpoint", r.URL.Path,
                "retry_after_ms", retryAfter)
            return
        }
        
        next(w, r)
    }
}

// Apply to admin endpoints
http.HandleFunc("/admin/sessions", adminRateLimit(handleAdminSessions))
http.HandleFunc("/admin/metrics", adminRateLimit(handleAdminMetrics))
http.HandleFunc("/admin/users", adminRateLimit(handleAdminUsers))
// ... other admin endpoints ...
```

**Configuration**:
- `ADMIN_RATE_LIMIT`: 20 requests per minute (default)
- `ADMIN_RATE_WINDOW`: 1 minute (default)
- Stricter than user rate limits (100 req/min)

**Rollback Strategy**: Set `ADMIN_RATE_LIMIT` to a very high value (e.g., 10000) to effectively disable.


### Issue #12: ResponseTimes Unbounded Growth (MEDIUM)

**Problem**: `ResponseTimes` slice grows without limit, consuming memory proportional to session lifetime.

**Root Cause**: No cap on slice size in `RecordResponseTime()`.

**Solution**: Implement rolling window with fixed maximum size.

**Implementation Approach**:

```go
// Add to Config
const MaxResponseTimes = 100 // Keep last 100 response times

// Modify RecordResponseTime in session.go
func (sm *SessionManager) RecordResponseTime(sessionID string, duration time.Duration) error {
    if sessionID == "" {
        return ErrInvalidSessionID
    }
    if duration < 0 {
        return ErrNegativeDuration
    }

    sm.mu.Lock()
    defer sm.mu.Unlock()

    session, exists := sm.sessions[sessionID]
    if !exists {
        return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
    }

    session.mu.Lock()
    defer session.mu.Unlock()

    // Implement rolling window
    if len(session.ResponseTimes) >= MaxResponseTimes {
        // Remove oldest entry (shift left)
        copy(session.ResponseTimes, session.ResponseTimes[1:])
        session.ResponseTimes[MaxResponseTimes-1] = duration
    } else {
        session.ResponseTimes = append(session.ResponseTimes, duration)
    }

    return nil
}
```

**Alternative Approach** (more efficient for large windows):

```go
// Use circular buffer instead of slice shifting
type Session struct {
    // ... existing fields ...
    ResponseTimes []time.Duration
    responseTimeIndex int // Current write position
}

func (sm *SessionManager) RecordResponseTime(sessionID string, duration time.Duration) error {
    // ... validation ...
    
    session.mu.Lock()
    defer session.mu.Unlock()
    
    if len(session.ResponseTimes) < MaxResponseTimes {
        session.ResponseTimes = append(session.ResponseTimes, duration)
    } else {
        session.ResponseTimes[session.responseTimeIndex] = duration
        session.responseTimeIndex = (session.responseTimeIndex + 1) % MaxResponseTimes
    }
    
    return nil
}
```

**Configuration**: `MaxResponseTimes` = 100 (hardcoded constant, can be made configurable if needed)

**Rollback Strategy**: Increase `MaxResponseTimes` if 100 is insufficient for analytics.


### Issue #19: Configuration Validation (MEDIUM)

**Problem**: `Load()` doesn't call `Validate()` automatically, allowing invalid config to be loaded.

**Root Cause**: Manual validation required in main.go.

**Solution**: Call `Validate()` automatically in `Load()` or enforce in main.go.

**Implementation Approach (Option 1 - Automatic)**:

```go
// Modify Load() in config.go
func Load() (*Config, error) {
    cfg := &Config{
        // ... load configuration ...
    }
    
    // Automatically validate
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("configuration validation failed: %w", err)
    }
    
    return cfg, nil
}
```

**Implementation Approach (Option 2 - Explicit in main.go)**:

```go
// In chatbox.go main()
func main() {
    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        logger.Fatal("Failed to load configuration", "error", err)
    }
    
    // Validate configuration
    if err := cfg.Validate(); err != nil {
        logger.Fatal("Configuration validation failed", "error", err)
    }
    
    // ... rest of startup ...
}
```

**Recommendation**: Use Option 2 (explicit validation in main.go) for better separation of concerns and clearer error messages.

**Configuration**: No new configuration needed.

**Rollback Strategy**: If validation blocks startup, temporarily comment out specific validation rules while fixing config.


### Issue #9: MongoDB Retry Logic (MEDIUM)

**Problem**: No retry logic for transient MongoDB errors, reducing reliability.

**Root Cause**: Storage operations use fixed timeouts without retry.

**Solution**: Implement retry with exponential backoff for transient errors.

**Implementation Approach**:

```go
// Add retry helper function to storage.go

type retryConfig struct {
    maxAttempts int
    initialDelay time.Duration
    maxDelay time.Duration
    multiplier float64
}

var defaultRetryConfig = retryConfig{
    maxAttempts: 3,
    initialDelay: 100 * time.Millisecond,
    maxDelay: 2 * time.Second,
    multiplier: 2.0,
}

func (s *Storage) retryOperation(ctx context.Context, operation string, fn func() error) error {
    var lastErr error
    delay := defaultRetryConfig.initialDelay
    
    for attempt := 1; attempt <= defaultRetryConfig.maxAttempts; attempt++ {
        err := fn()
        if err == nil {
            return nil
        }
        
        // Check if error is retryable
        if !isRetryableError(err) {
            return err
        }
        
        lastErr = err
        
        if attempt < defaultRetryConfig.maxAttempts {
            s.logger.Warn("MongoDB operation failed, retrying",
                "operation", operation,
                "attempt", attempt,
                "max_attempts", defaultRetryConfig.maxAttempts,
                "delay", delay,
                "error", err)
            
            time.Sleep(delay)
            
            // Exponential backoff
            delay = time.Duration(float64(delay) * defaultRetryConfig.multiplier)
            if delay > defaultRetryConfig.maxDelay {
                delay = defaultRetryConfig.maxDelay
            }
        }
    }
    
    return fmt.Errorf("operation failed after %d attempts: %w", 
        defaultRetryConfig.maxAttempts, lastErr)
}

func isRetryableError(err error) bool {
    if err == nil {
        return false
    }
    
    errStr := err.Error()
    
    // Network errors
    if strings.Contains(errStr, "connection refused") ||
       strings.Contains(errStr, "connection reset") ||
       strings.Contains(errStr, "timeout") ||
       strings.Contains(errStr, "temporary failure") {
        return true
    }
    
    // MongoDB specific transient errors
    if strings.Contains(errStr, "server selection timeout") ||
       strings.Contains(errStr, "no reachable servers") {
        return true
    }
    
    return false
}

// Example usage in CreateSession
func (s *Storage) CreateSession(sess *session.Session) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    return s.retryOperation(ctx, "CreateSession", func() error {
        doc := sessionToDocument(sess)
        _, err := s.collection.InsertOne(ctx, doc)
        return err
    })
}
```

**Configuration**:
- `MONGO_RETRY_MAX_ATTEMPTS`: 3 (default)
- `MONGO_RETRY_INITIAL_DELAY`: 100ms (default)
- `MONGO_RETRY_MAX_DELAY`: 2s (default)

**Rollback Strategy**: Set `MONGO_RETRY_MAX_ATTEMPTS=1` to disable retries.


### Issue #15: Shutdown Timeout (LOW)

**Problem**: Shutdown doesn't respect context deadline, can hang with many connections.

**Root Cause**: `Shutdown()` iterates connections synchronously without checking deadline.

**Solution**: Respect context deadline and use parallel closure with timeout.

**Implementation Approach**:

```go
// Modify Shutdown() in chatbox.go
func (c *Chatbox) Shutdown(ctx context.Context) error {
    c.logger.Info("Starting graceful shutdown")
    
    // Close HTTP server first
    if err := c.httpServer.Shutdown(ctx); err != nil {
        c.logger.Error("HTTP server shutdown error", "error", err)
    }
    
    // Get all connections
    c.handler.mu.RLock()
    connections := make([]*websocket.Connection, 0, len(c.handler.connections))
    for _, conn := range c.handler.connections {
        connections = append(connections, conn)
    }
    c.handler.mu.RUnlock()
    
    // Close connections in parallel with timeout
    var wg sync.WaitGroup
    errChan := make(chan error, len(connections))
    
    for _, conn := range connections {
        wg.Add(1)
        go func(c *websocket.Connection) {
            defer wg.Done()
            if err := c.Close(); err != nil {
                errChan <- err
            }
        }(conn)
    }
    
    // Wait for all closures or context deadline
    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        c.logger.Info("All connections closed gracefully")
        return nil
    case <-ctx.Done():
        c.logger.Warn("Shutdown deadline exceeded, forcing closure",
            "remaining_connections", len(connections))
        return ctx.Err()
    }
}
```

**Configuration**: Shutdown timeout controlled by caller (typically 30s in main.go).

**Rollback Strategy**: Increase shutdown timeout in main.go if 30s is insufficient.


## Data Models

### Configuration Changes

```go
// Add to ServerConfig
type ServerConfig struct {
    Port             int
    ReconnectTimeout time.Duration
    MaxConnections   int
    RateLimit        int
    JWTSecret        string
    
    // NEW: Production readiness config
    LLMStreamTimeout    time.Duration // Issue #8
    SessionCleanupInterval time.Duration // Issue #1
    SessionTTL          time.Duration // Issue #1
    RateLimitCleanupInterval time.Duration // Issue #11
    AdminRateLimit      int // Issue #18
    AdminRateWindow     time.Duration // Issue #18
    MongoRetryAttempts  int // Issue #9
    MongoRetryDelay     time.Duration // Issue #9
}
```

### Session Manager Changes

```go
type SessionManager struct {
    sessions         map[string]*Session
    userSessions     map[string]string
    mu               sync.RWMutex
    reconnectTimeout time.Duration
    logger           *golog.Logger
    
    // NEW: Cleanup goroutine management
    cleanupInterval time.Duration
    sessionTTL      time.Duration
    stopCleanup     chan struct{}
    cleanupWg       sync.WaitGroup
}
```

### Rate Limiter Changes

```go
type MessageLimiter struct {
    events map[string][]time.Time
    window time.Duration
    limit  int
    mu     sync.RWMutex
    
    // NEW: Cleanup goroutine management
    cleanupInterval time.Duration
    stopCleanup     chan struct{}
    cleanupWg       sync.WaitGroup
}
```

### Session Changes

```go
type Session struct {
    // ... existing fields ...
    ResponseTimes []time.Duration
    
    // NEW: For circular buffer implementation (optional)
    responseTimeIndex int
}
```


## Correctness Properties

A property is a characteristic or behavior that should hold true across all valid executions of a system—essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.

### Issue #1: Session Memory Leak

Property 1: Session cleanup removes expired sessions
*For any* session that has been inactive for longer than the TTL, the cleanup process should remove it from the sessions map
**Validates: Requirements 1.1, 1.2, 1.5**

Property 2: Active sessions are never cleaned up
*For any* active session, the cleanup process should never remove it from the sessions map
**Validates: Requirements 1.1**

Property 3: Memory stats are accurate
*For any* point in time, GetMemoryStats() should return counts that sum to the total number of sessions in the map
**Validates: Requirements 1.4**

### Issue #8: LLM Streaming Timeout

Property 4: Streaming requests have timeout
*For any* LLM streaming request, the context should have a deadline set
**Validates: Requirements 8.1, 8.3**

Property 5: Timeout cancels streaming
*For any* LLM streaming request that exceeds the timeout, the context should be cancelled and an error returned
**Validates: Requirements 8.2, 8.5**

### Issue #9: MongoDB Retry Logic

Property 6: Transient errors are retried
*For any* MongoDB operation that fails with a transient error, the operation should be retried up to the maximum attempts
**Validates: Requirements 9.1, 9.3**

Property 7: Retry uses exponential backoff
*For any* sequence of retry attempts, the delay between attempts should increase exponentially up to the maximum delay
**Validates: Requirements 9.2**

Property 8: Non-transient errors fail immediately
*For any* MongoDB operation that fails with a non-transient error, the operation should fail immediately without retries
**Validates: Requirements 9.1**

### Issue #11: Rate Limiter Cleanup

Property 9: Cleanup removes old events
*For any* rate limiter state, running Cleanup() should remove all events older than the time window
**Validates: Requirements 11.3**

Property 10: Cleanup runs periodically
*For any* rate limiter with cleanup enabled, Cleanup() should be called at the configured interval
**Validates: Requirements 11.1, 11.2**

Property 11: Cleanup goroutine terminates
*For any* rate limiter, calling StopCleanup() should cause the cleanup goroutine to terminate within a reasonable time
**Validates: Requirements 11.5**


### Issue #12: ResponseTimes Unbounded Growth

Property 12: ResponseTimes slice is bounded
*For any* session, the length of ResponseTimes should never exceed MaxResponseTimes
**Validates: Requirements 12.1, 12.5**

Property 13: Rolling window maintains recent times
*For any* session with full ResponseTimes, adding a new time should remove the oldest time
**Validates: Requirements 12.2, 12.4**

### Issue #13: Origin Validation Data Race

Property 14: Origin validation is thread-safe
*For any* concurrent access to checkOrigin() and SetAllowedOrigins(), no data races should occur
**Validates: Requirements 13.1, 13.2, 13.4, 13.5**

### Issue #15: Shutdown Timeout

Property 15: Shutdown respects deadline
*For any* shutdown operation with a context deadline, the operation should complete or return an error before the deadline
**Validates: Requirements 15.1, 15.3**

Property 16: Shutdown closes all connections
*For any* shutdown operation that completes successfully, all connections should be closed
**Validates: Requirements 15.2, 15.5**

### Issue #17: JWT Secret Validation

Property 17: Weak secrets are rejected
*For any* JWT secret shorter than 32 characters or containing common weak patterns, validation should fail
**Validates: Requirements 17.1, 17.2, 17.3**

Property 18: Strong secrets are accepted
*For any* JWT secret that is at least 32 characters and doesn't contain weak patterns, validation should succeed
**Validates: Requirements 17.1, 17.2, 17.3**

### Issue #18: Admin Endpoint Rate Limiting

Property 19: Admin endpoints enforce rate limits
*For any* sequence of admin requests exceeding the rate limit, requests should be rejected with HTTP 429
**Validates: Requirements 18.1, 18.3**

Property 20: Admin and user limits are independent
*For any* user making both admin and user requests, the rate limits should be tracked separately
**Validates: Requirements 18.5**

### Issue #19: Configuration Validation

Property 21: Invalid config prevents startup
*For any* configuration with missing required fields or invalid values, validation should fail
**Validates: Requirements 19.2, 19.4, 19.5**

Property 22: Valid config passes validation
*For any* configuration with all required fields and valid values, validation should succeed
**Validates: Requirements 19.4, 19.5**


## Error Handling

### General Error Handling Strategy

1. **Graceful Degradation**: Failures in non-critical components (cleanup, logging) should not affect core functionality
2. **Clear Error Messages**: All errors include context (session ID, user ID, operation) for debugging
3. **Logging**: All errors are logged with appropriate severity levels
4. **Metrics**: Error rates are tracked for monitoring and alerting

### Component-Specific Error Handling

#### Session Cleanup (Issue #1)
- **Cleanup Failure**: Log error but continue with next cleanup cycle
- **Lock Contention**: Use RWMutex to minimize blocking
- **Panic Recovery**: Cleanup goroutine has defer/recover to prevent crashes

#### Origin Validation (Issue #13)
- **Lock Failure**: Should never happen with proper mutex usage
- **Invalid Origin**: Reject connection with clear error message

#### LLM Streaming (Issue #8)
- **Timeout**: Return timeout error to client with retry guidance
- **Context Cancellation**: Clean up resources and return appropriate error
- **Network Errors**: Distinguish between timeout and other errors

#### MongoDB Retry (Issue #9)
- **Transient Errors**: Retry with exponential backoff
- **Permanent Errors**: Fail immediately with clear error
- **Retry Exhaustion**: Return error indicating all retries failed

#### Rate Limiting (Issue #11, #18)
- **Cleanup Failure**: Log error but continue operation
- **Rate Limit Exceeded**: Return HTTP 429 with Retry-After header
- **Lock Contention**: Use RWMutex for read-heavy workload

#### Configuration Validation (Issue #17, #19)
- **Validation Failure**: Refuse to start with detailed error messages
- **Missing Fields**: List all missing fields in error
- **Invalid Values**: Explain what values are acceptable

#### Shutdown (Issue #15)
- **Deadline Exceeded**: Log warning and force close remaining connections
- **Connection Close Failure**: Log error but continue with other connections
- **Panic During Shutdown**: Recover and log, but complete shutdown


## Testing Strategy

### Dual Testing Approach

We use both unit tests and property-based tests for comprehensive coverage:

- **Unit tests**: Verify specific examples, edge cases, and error conditions
- **Property tests**: Verify universal properties across all inputs
- Together they provide comprehensive coverage: unit tests catch concrete bugs, property tests verify general correctness

### Property-Based Testing

We use Go's `testing/quick` package for property-based testing. Each property test:
- Runs minimum 100 iterations with random inputs
- References the design document property it validates
- Uses tag format: `Feature: production-readiness-fixes, Property N: [property text]`

### Test Organization

Tests are organized by component and issue:

```
internal/session/
  session_production_fix_test.go          # Issue #1, #12
internal/router/
  router_production_fix_test.go           # Issue #8
internal/websocket/
  handler_production_fix_test.go          # Issue #13
internal/ratelimit/
  ratelimit_production_fix_test.go        # Issue #11, #18
internal/storage/
  storage_production_fix_test.go          # Issue #9
internal/config/
  config_production_fix_test.go           # Issue #17, #19
chatbox_production_fix_test.go            # Issue #15
```

### Test Coverage by Issue

#### Issue #1: Session Memory Leak
- **Property Test**: Verify cleanup removes expired sessions
- **Property Test**: Verify active sessions are never cleaned
- **Unit Test**: Test cleanup with various TTL values
- **Unit Test**: Test GetMemoryStats accuracy
- **Integration Test**: Run cleanup for extended period, verify no leaks

#### Issue #8: LLM Streaming Timeout
- **Property Test**: Verify all streaming requests have timeout
- **Property Test**: Verify timeout cancels request
- **Unit Test**: Test with mock LLM that hangs
- **Unit Test**: Test timeout configuration
- **Integration Test**: Test with real LLM provider

#### Issue #9: MongoDB Retry Logic
- **Property Test**: Verify transient errors are retried
- **Property Test**: Verify exponential backoff timing
- **Property Test**: Verify non-transient errors fail immediately
- **Unit Test**: Test retry count limits
- **Unit Test**: Test various error types
- **Integration Test**: Test with real MongoDB (network issues)

#### Issue #11: Rate Limiter Cleanup
- **Property Test**: Verify cleanup removes old events
- **Property Test**: Verify cleanup runs periodically
- **Property Test**: Verify cleanup goroutine terminates
- **Unit Test**: Test cleanup with various time windows
- **Unit Test**: Test cleanup statistics logging

#### Issue #12: ResponseTimes Bounded
- **Property Test**: Verify slice never exceeds max size
- **Property Test**: Verify rolling window behavior
- **Unit Test**: Test with exactly max size
- **Unit Test**: Test with circular buffer implementation

#### Issue #13: Origin Validation Data Race
- **Property Test**: Verify no data races with concurrent access
- **Unit Test**: Test with -race flag
- **Unit Test**: Test with many concurrent goroutines
- **Stress Test**: Run for extended period with high concurrency

#### Issue #15: Shutdown Timeout
- **Property Test**: Verify shutdown respects deadline
- **Property Test**: Verify all connections closed
- **Unit Test**: Test with various deadline values
- **Unit Test**: Test with hanging connections
- **Integration Test**: Test with real connections

#### Issue #17: JWT Secret Validation
- **Property Test**: Verify weak secrets rejected
- **Property Test**: Verify strong secrets accepted
- **Unit Test**: Test each weak pattern
- **Unit Test**: Test minimum length boundary
- **Unit Test**: Test error messages

#### Issue #18: Admin Rate Limiting
- **Property Test**: Verify rate limits enforced
- **Property Test**: Verify admin/user limits independent
- **Unit Test**: Test HTTP 429 response
- **Unit Test**: Test Retry-After header
- **Integration Test**: Test with real HTTP requests

#### Issue #19: Configuration Validation
- **Property Test**: Verify invalid config fails
- **Property Test**: Verify valid config passes
- **Unit Test**: Test each validation rule
- **Unit Test**: Test error message clarity
- **Unit Test**: Test validation in main.go

### Race Detection

All tests must pass with `-race` flag:
```bash
go test -race ./...
```

### Performance Testing

For cleanup and retry logic:
- Benchmark cleanup performance with various data sizes
- Measure memory usage before/after cleanup
- Verify retry backoff doesn't cause excessive delays

### Integration Testing

End-to-end tests for:
- Session lifecycle with cleanup
- LLM streaming with timeout
- MongoDB operations with retry
- Rate limiting under load
- Shutdown with active connections

