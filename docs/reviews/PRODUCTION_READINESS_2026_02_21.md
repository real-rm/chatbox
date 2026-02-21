# Production Readiness Review

**Date:** 2026-02-21
**Branch:** main (`7482a0d`)
**Go version:** 1.24.4
**Verdict:** CONDITIONAL PASS - Fix CRITICAL and HIGH items before production deployment

---

## Build & Static Analysis

| Check | Result |
|-------|--------|
| `go build ./cmd/server/...` | PASS |
| `go vet ./...` | PASS |
| `go test -short ./...` | PASS (20/20 packages) |

---

## Test Coverage

| Package | Coverage | Target |
|---------|----------|--------|
| internal/config | 100.0% | 80% |
| internal/errors | 100.0% | 80% |
| internal/httperrors | 100.0% | 80% |
| internal/message | 98.9% | 80% |
| internal/util | 98.3% | 80% |
| internal/ratelimit | 97.8% | 80% |
| cmd/server | 94.1% | 80% |
| internal/testutil | 94.4% | 80% |
| internal/session | 93.9% | 80% |
| internal/auth | 92.9% | 80% |
| internal/storage | 89.2% | 80% |
| internal/websocket | 85.5% | 80% |
| internal/llm | 83.4% | 80% |
| internal/notification | 81.4% | 80% |
| internal/router | 80.5% | 80% |
| internal/upload | 78.0% | 80% |
| chatbox (root) | 28.9% | 80% |

17 of 18 packages meet the 80% target. `internal/upload` is close at 78%.
The root `chatbox` package (registration + route wiring) is low at 28.9% because most logic delegates to internal packages; this is acceptable for a wiring-only package.

---

## Findings

### CRITICAL

#### C1. Lock Ordering Violation in StopCleanup (Session + RateLimiter)

Both `SessionManager.StopCleanup()` and `MessageLimiter.StopCleanup()` use an unsafe unlock-wait-relock pattern inside a deferred unlock:

```go
// session/session.go:296-314, ratelimit/ratelimit.go:228-246
func (sm *SessionManager) StopCleanup() {
    sm.mu.Lock()
    defer sm.mu.Unlock()  // deferred unlock at end
    // ...
    sm.mu.Unlock()        // manual unlock mid-function
    sm.cleanupWg.Wait()   // wait outside lock
    sm.mu.Lock()          // re-lock so defer doesn't double-unlock
}
```

**Risk:** Between `Unlock()` and `Lock()`, another goroutine can modify shared state. The cleanup goroutine itself acquires `sm.mu` during cleanup ticks, creating a potential deadlock: if the cleanup goroutine is blocked waiting for the lock while `Wait()` expects it to finish, the system hangs.

**Recommendation:** Extract cleanup lifecycle into a separate mechanism that doesn't share the data mutex. Use a dedicated `cleanupMu` or `sync.Once` for shutdown coordination:

```go
func (sm *SessionManager) StopCleanup() {
    sm.stopOnce.Do(func() {
        close(sm.stopCleanup)
    })
    sm.cleanupWg.Wait()
}
```

#### C2. Missing PodDisruptionBudget

No PDB exists in `deployments/kubernetes/`. During node drains or cluster maintenance, Kubernetes can evict all pods simultaneously, causing full service outage for an application serving persistent WebSocket connections.

**Recommendation:** Add `deployments/kubernetes/pdb.yaml`:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: chatbox-websocket-pdb
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: chatbox
      component: websocket
```

---

### HIGH

#### H1. No Panic Recovery in Spawned Goroutines

The codebase uses `gin.New()` (no built-in recovery). Since chatbox is a library consumed by gomain, HTTP-level recovery depends on the host process. However, goroutines spawned internally (`readPump`, `writePump`, LLM processing, voice message processing) are never covered by HTTP middleware recovery:

- `handler.go:254` - `go connection.readPump(h)`
- `handler.go:255` - `go connection.writePump()`
- `router.go:390` - `go func() { notificationService.SendHelpRequestAlert(...) }()`
- `router.go:652` - `go mr.processVoiceMessageWithLLM(...)`

A panic in any of these kills the goroutine silently. If `readPump` panics, `writePump` hangs forever on a dead connection.

**Recommendation:** Wrap goroutine launches with deferred recovery:

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            logger.Error("Panic in readPump", "error", r)
            metrics.MessageErrors.Inc()
        }
    }()
    connection.readPump(h)
}()
```

#### H2. WebSocket Origin Allows All When Unconfigured

`handler.go:162-165` - When `allowed_origins` is empty, `checkOrigin()` returns `true` for all origins. The same applies to CORS middleware (`chatbox.go:330-331`). A warning is logged but startup continues.

If deployed without configuring origins, the service accepts cross-origin WebSocket connections from any domain.

**Recommendation:** At minimum, log a highly visible warning. Ideally, require explicit opt-in for "allow all" mode (e.g., `allowed_origins = "*"`) rather than defaulting to open.

#### H3. Kubernetes readOnlyRootFilesystem Set to False

`deployments/kubernetes/deployment.yaml:188` sets `readOnlyRootFilesystem: false`. The application already mounts `emptyDir` volumes for `/app/logs` and `/app/temp`, so the root filesystem can be made read-only.

**Recommendation:** Set `readOnlyRootFilesystem: true`. This limits damage from container escape or code injection.

#### H4. No HTTP Server Timeouts in Standalone Mode

`cmd/server/main.go` does not set `ReadTimeout`, `WriteTimeout`, or `IdleTimeout` on the HTTP server. Slowloris-style attacks can exhaust connections.

**Note:** When consumed as a library by gomain, the host process controls server timeouts. This only affects standalone mode.

**Recommendation:** If standalone mode is used in production, wrap with explicit timeouts:

```go
srv := &http.Server{
    Handler:      engine,
    ReadTimeout:  15 * time.Second,
    WriteTimeout: 15 * time.Second,
    IdleTimeout:  60 * time.Second,
}
```

---

### MEDIUM

#### M1. Channel Send After Snapshot Can Panic on Closed Channel

Multiple locations take a snapshot of connections under lock, then send to channels outside the lock:

- `handler.go:386-397` (`notifyConnectionLimit`) - uses non-blocking `select/default`
- `router.go:946-950` (`BroadcastToSession`) - uses non-blocking `select/default`
- `router.go:897-902` (`sendToConnection`) - uses non-blocking `select/default`

If a connection is unregistered and its `send` channel is closed between the snapshot and the send, `send <- data` panics. The non-blocking `select/default` prevents blocking but does not prevent sends to closed channels.

**Recommendation:** Either protect the channel with a `closing` atomic flag checked before send, or use a helper that recovers from send-on-closed-channel panics.

#### M2. Rate Limiter Events Map Unbounded Growth

`ratelimit/ratelimit.go:88-116` - The `events` map grows per-user. Cleanup runs periodically, but if `StartCleanup()` is never called or the cleanup goroutine crashes, memory grows without limit.

**Recommendation:** Add a safety cap in `Allow()`:

```go
if len(recentEvents) > maxEventsPerUser {
    recentEvents = recentEvents[len(recentEvents)-maxEventsPerUser:]
}
```

#### M3. Missing Trace/Request ID Correlation

Logs across goroutines handling the same request cannot be correlated. When a WebSocket message flows through handler -> router -> LLM -> storage, each component logs independently without a shared request ID.

**Recommendation:** Generate a request ID per incoming WebSocket message and thread it through context:

```go
ctx = context.WithValue(ctx, "trace_id", uuid.New().String())
```

#### M4. Public Endpoints Not Rate Limited

`/healthz`, `/readyz`, and `/metrics` have no rate limiting. While health probes are typically safe, the `/metrics` endpoint returns detailed system state and could be abused for reconnaissance or resource exhaustion if exposed to untrusted networks.

**Recommendation:** Either add rate limiting to these endpoints or document that they must be behind a reverse proxy that handles this.

#### M5. Missing NetworkPolicy

No Kubernetes NetworkPolicy restricts ingress/egress traffic. By default, all pods can communicate with all other pods.

**Recommendation:** Add a NetworkPolicy that restricts ingress to the ingress controller and egress to MongoDB and external LLM APIs only.

#### M6. Docker Build Leaks GitHub Token in Layer Cache

`Dockerfile:12-22` uses `ARG GITHUB_TOKEN` and writes it to git config. This token can be extracted from the build layer cache.

**Recommendation:** Use BuildKit secrets:

```dockerfile
RUN --mount=type=secret,id=github_token \
    GITHUB_TOKEN=$(cat /run/secrets/github_token) && \
    git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"
```

---

### LOW

#### L1. Fire-and-Forget Goroutines Not Tracked for Shutdown

`router.go:390-397` (help request notification) and `router.go:652` (voice message processing) spawn goroutines that are not tracked by any WaitGroup. During shutdown, these goroutines are killed mid-flight.

**Impact:** Notifications may be lost during graceful shutdown. Acceptable for best-effort delivery but worth documenting.

#### L2. Debug Logging May Expose Message Content

`handler.go:619-624` logs message details at DEBUG level. If DEBUG logging is enabled in production, message content could appear in logs.

**Recommendation:** Ensure production log level is set to INFO or higher. Consider replacing content logging with content length only.

#### L3. Shutdown Does Not Pass Context to Sub-Component Shutdowns

`chatbox.go:916-930` - `StopCleanup()` calls for session manager, message router, and admin limiter do not accept or respect the context deadline. If any component hangs, the entire shutdown blocks.

**Recommendation:** Add `StopCleanupWithContext(ctx)` variants or add internal timeouts.

---

## Security Summary

| Area | Status |
|------|--------|
| Hardcoded secrets | PASS - None found |
| NoSQL injection | PASS - All queries use typed BSON filters with constants |
| Input validation | PASS - Comprehensive validation in `message/validation.go` |
| JWT security | PASS - Algorithm pinned to HMAC, expiry validated, claims checked |
| Encryption at rest | PASS - AES-256-GCM, unique nonce per message, proper key validation |
| File upload security | PASS - Multi-layered: MIME whitelist, malicious pattern scanning, path traversal prevention |
| Connection limits | PASS - 10 concurrent connections per user, enforced at upgrade |
| Rate limiting | PARTIAL - WebSocket messages and admin endpoints covered; public HTTP endpoints not covered |
| CORS/Origin validation | WARNING - Open by default when unconfigured (see H2) |
| Error sanitization | PASS - Generic client messages, detailed server logs |
| HTTP timeouts | PASS - LLM clients 60s, WebSocket ping/pong/write properly configured |

---

## Infrastructure Summary

| Area | Status |
|------|--------|
| Docker multi-stage build | PASS - Alpine-based, non-root user, health check |
| Kubernetes deployment | PASS - Resource limits, HPA, security context, init container |
| Health probes | PASS - Liveness (healthz), readiness (readyz + MongoDB ping), startup probe |
| Prometheus metrics | PASS - Connections, messages, errors, LLM latency, sessions, tokens |
| Structured logging | PASS - Component/operation context, key-value pairs, log level separation |
| Graceful shutdown | PARTIAL - WebSocket connections drained with context; sub-components lack timeout |
| Dependencies | PASS - All major dependencies current (gin 1.11, jwt v5.3, mongo-driver 1.17) |

---

## Action Items by Priority

### Must Fix (Before Production)

1. **C1** - Refactor `StopCleanup()` lock ordering in session and ratelimit packages
2. **C2** - Add PodDisruptionBudget to Kubernetes manifests
3. **H1** - Add panic recovery to spawned goroutines (readPump, writePump, LLM goroutines)
4. **H2** - Require explicit origin configuration or fail loudly in production

### Should Fix (First Production Sprint)

5. **H3** - Set `readOnlyRootFilesystem: true`
6. **M1** - Protect channel sends from closed-channel panics
7. **M2** - Cap rate limiter events map growth
8. **M3** - Add trace ID for log correlation
9. **M6** - Use BuildKit secrets for Docker builds

### Nice to Have

10. **M4** - Rate limit public endpoints
11. **M5** - Add NetworkPolicy
12. **L1** - Track fire-and-forget goroutines
13. **L3** - Add timeouts to sub-component shutdowns
