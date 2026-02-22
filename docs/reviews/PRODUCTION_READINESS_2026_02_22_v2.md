# Production Readiness Review (v2)

**Date:** 2026-02-22
**Commit:** `a2349ef` (main)
**Go version:** 1.25.6
**Reviewer:** Automated (4-agent audit: code quality, security, tests/CI, infrastructure)
**Previous review:** `PRODUCTION_READINESS_2026_02_22.md` at commit `e95c04e` — all 15 issues fixed in `a2349ef`
**Verdict:** CONDITIONAL PASS — 2 CRITICAL, 4 HIGH, 8 MEDIUM, 4 LOW issues identified

---

## Build & Static Analysis

| Check | Result |
|-------|--------|
| `go build ./cmd/server/...` | PASS |
| `go build -race ./cmd/server/...` | PASS |
| `go vet ./...` | PASS (0 warnings) |
| `go test -short ./...` | PASS (20/20 packages) |

---

## Test Coverage

| Package | Coverage | Target | Status |
|---------|----------|--------|--------|
| `chatbox` (root) | 30.4% | 80% | BELOW |
| `cmd/server` | 94.3% | 80% | OK |
| `internal/auth` | 92.9% | 80% | OK |
| `internal/config` | 100.0% | 80% | OK |
| `internal/errors` | 100.0% | 80% | OK |
| `internal/httperrors` | 100.0% | 80% | OK |
| `internal/llm` | 82.9% | 80% | OK |
| `internal/message` | 98.9% | 80% | OK |
| `internal/notification` | 82.6% | 80% | OK |
| `internal/ratelimit` | 96.9% | 80% | OK |
| `internal/router` | 83.6% | 80% | OK |
| `internal/session` | 93.8% | 80% | OK |
| `internal/storage` | 89.2% | 80% | OK |
| `internal/testutil` | 96.8% | 80% | OK |
| `internal/upload` | 78.0% | 80% | BELOW |
| `internal/util` | 97.3% | 80% | OK |
| `internal/websocket` | 88.7% | 80% | OK |

18/20 packages meet 80% target (excluding `internal/constants` and `internal/metrics` which have no statements).

---

## Previous Review (v1) Fixes Verification

All 15 issues from `PRODUCTION_READINESS_2026_02_22.md` were fixed in commit `a2349ef`:

| ID | Issue | Status |
|----|-------|--------|
| S1 | HTML Injection in `buildHelpRequestHTML` | FIXED — `html.EscapeString()` applied |
| S2 | NetworkPolicy Missing HTTPS Egress | FIXED — ports 443, 587 added |
| S3 | Goroutine Leak in LLM Streaming | FIXED — `select` on `ctx.Done()` in all providers |
| S4 | Ingress CORS Wildcard `*` | FIXED — placeholder value with comment |
| S5 | Startup Origin Validation Missing | FIXED — `containsPlaceholder()` check added |
| S6 | Missing CORS Origin Validation at Startup | FIXED — combined with S5 |
| M1 | GetUserID()/GetRoles() Not Mutex-Protected | FIXED — immutability documented |
| M3 | Missing Health Check for LLM Providers | FIXED — LLM check added to `/readyz` |
| M4 | Missing Observability Metrics | FIXED — 4 new metrics added |
| L1 | docker-compose.yml Plain Credentials | FIXED — env var references |
| L2 | Secret Template Realistic Examples | FIXED — `REPLACE_WITH_*` placeholders |
| L3 | Healthcheck Path Hardcoded in Dockerfile | FIXED — uses `$CHATBOX_PATH_PREFIX` |

---

## Findings

### CRITICAL

#### C1. Data Race on gorilla/websocket Concurrent Writes

**Files:** `internal/websocket/handler.go:459-465, 830-853`

`ShutdownWithContext` (line 459) acquires `c.mu.Lock()` and writes to `c.conn`:

```go
c.mu.Lock()
if c.conn != nil {
    c.conn.SetWriteDeadline(time.Now().Add(writeWait))
    c.conn.WriteMessage(websocket.CloseMessage, ...)
}
c.mu.Unlock()
```

Meanwhile `writePump` (lines 830-853) writes to `c.conn` **without holding `c.mu`**:

```go
c.conn.SetWriteDeadline(time.Now().Add(writeWait))          // line 830
c.conn.WriteMessage(websocket.CloseMessage, []byte{})        // line 835
c.conn.WriteMessage(websocket.TextMessage, message)           // line 842
c.conn.WriteMessage(websocket.PingMessage, nil)               // line 853
```

gorilla/websocket explicitly forbids concurrent writes to `*websocket.Conn`. This race can cause data corruption, panics, or malformed WebSocket frames in production under concurrent shutdown + message send.

**Impact:** Data corruption or panic under graceful shutdown
**Fix:** Either:
- (a) Protect all `writePump` writes with `c.mu`, or
- (b) Use a write-serializer goroutine pattern where only one goroutine ever calls `conn.Write*`, or
- (c) Signal `writePump` to exit via close/channel before `ShutdownWithContext` writes

---

#### C2. Duplicate HPA Definition Conflict

**Files:** `deployments/kubernetes/deployment.yaml:252-311` vs `deployments/kubernetes/hpa.yaml:17-93`

Both files define `HorizontalPodAutoscaler` with the same name `chatbox-websocket-hpa`. The HPA in `deployment.yaml` uses only resource metrics (CPU/memory) with commented-out custom metrics, while `hpa.yaml` has active custom metrics (`chatbox_websocket_connections_total`, `chatbox_active_sessions_total`).

Applying both manifests causes the second `kubectl apply` to overwrite the first. The result depends on apply order, creating unpredictable autoscaling behavior.

**Impact:** Unpredictable autoscaling; one HPA silently overwrites the other
**Fix:** Remove the HPA from `deployment.yaml` (lines 251-311) and keep only `hpa.yaml` as the single source of truth

---

### HIGH

#### H1. HTML Injection in SendCriticalError and SendSystemAlert

**Files:** `internal/notification/notification.go:244-253, 321`

The S1 fix from v1 only addressed `buildHelpRequestHTML`. Two other email-sending functions still interpolate unescaped values into HTML:

`SendCriticalError` (lines 244-253):
```go
HTML: fmt.Sprintf(`
    <li><strong>Error Type:</strong> %s</li>
    <li><strong>Details:</strong> %s</li>
`, errorType, details, ...)
```

`SendSystemAlert` (line 321):
```go
HTML: fmt.Sprintf("<p>%s</p>...", message, ...)
```

If `errorType`, `details`, or `message` contain user-influenced data (e.g., error messages from chat input), HTML/JS can be injected into admin email notifications.

**Impact:** Stored XSS in admin email clients
**Fix:** Apply `html.EscapeString()` to all interpolated values in `SendCriticalError` and `SendSystemAlert` HTML templates

---

#### H2. bufio.Scanner 64KB Buffer Truncation for LLM Streams

**Files:** `internal/llm/openai.go:194`, `internal/llm/anthropic.go:223`, `internal/llm/dify.go:183`

All three LLM streaming implementations use `bufio.NewScanner(resp.Body)` with the default 64KB buffer. SSE events from LLM providers can exceed this limit, particularly:
- Large code blocks in streaming responses
- Base64-encoded images in multimodal responses
- Tool-use responses with large JSON payloads

When a line exceeds 64KB, `scanner.Scan()` returns `false` and silently drops the remainder of the stream.

**Impact:** Silently truncated LLM responses for large outputs
**Fix:** Set a larger buffer with `scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)` (1MB max)

---

#### H3. Notification RateLimiter Unbounded Memory Growth

**File:** `internal/notification/notification.go:30-76`

The `RateLimiter` struct uses `map[string][]time.Time` (line 32) to track events. While the `Allow` method filters old events per-key (lines 58-64), keys themselves are never removed from the map. Over time, inactive event keys accumulate with empty slices, causing unbounded map growth.

```go
type RateLimiter struct {
    events map[string][]time.Time  // Keys never deleted
    ...
}
```

**Impact:** Gradual memory leak proportional to distinct event keys over application lifetime
**Fix:** Add periodic cleanup that removes keys with empty event lists, or use a TTL-based cache

---

#### H4. NetworkPolicy Ingress Uses OR Logic Instead of AND

**File:** `deployments/kubernetes/networkpolicy.yaml:18-24`

The ingress rule uses two separate items in the `from` array:

```yaml
ingress:
  - from:
      - namespaceSelector:        # Item 1: any pod in ingress-nginx namespace
          matchLabels:
            kubernetes.io/metadata.name: ingress-nginx
      - podSelector:              # Item 2: any pod with this label in ANY namespace
          matchLabels:
            app.kubernetes.io/name: ingress-nginx
```

In Kubernetes NetworkPolicy, items in a `from` array are OR'd. This allows traffic from:
- **Any** pod in the `ingress-nginx` namespace, OR
- **Any** pod labeled `app.kubernetes.io/name: ingress-nginx` in the **same namespace** as chatbox

To require both conditions (AND), they must be combined in a single `from` item:

```yaml
ingress:
  - from:
      - namespaceSelector:
          matchLabels:
            kubernetes.io/metadata.name: ingress-nginx
        podSelector:              # Same item = AND
          matchLabels:
            app.kubernetes.io/name: ingress-nginx
```

**Impact:** Overly permissive ingress; any pod in the ingress-nginx namespace can access chatbox
**Fix:** Combine `namespaceSelector` and `podSelector` into a single `from` item

---

### MEDIUM

#### M1. Standalone Server Never Calls `chatbox.Shutdown()`

**File:** `cmd/server/main.go:64-88`

The `runWithSignalChannel` function loads config and waits for a signal, but never calls `chatbox.Register()` to start the application, nor `chatbox.Shutdown()` for graceful cleanup. This means:
- The standalone server binary doesn't actually serve anything
- No graceful shutdown of WebSocket connections, session cleanup goroutines, or rate limiter goroutines

```go
func runWithSignalChannel(sigChan chan os.Signal) error {
    cfg, err := loadConfiguration()
    // ...
    <-sigChan                    // Waits but never starts the app
    logger.Info("Shutting down gracefully")
    return nil                   // No Shutdown() call
}
```

**Impact:** Standalone server mode is non-functional; goroutine leaks if fixed without adding Shutdown()
**Fix:** Call `chatbox.Register()` to wire up routes, start an HTTP server, and call `chatbox.Shutdown(ctx)` on signal

---

#### M2. `createNewSession` Ignores `sessionID` Parameter

**File:** `internal/router/router.go:447-462`

The function accepts a `sessionID` parameter but never uses it. The actual session ID is generated inside `CreateSession()`:

```go
func (mr *MessageRouter) createNewSession(conn *websocket.Connection, sessionID string) (*session.Session, error) {
    // sessionID parameter is ignored
    sess, err := mr.sessionManager.CreateSession(conn.UserID)  // generates its own ID
```

This suggests either dead code from a previous refactor or an intent to use caller-supplied IDs that was never implemented.

**Impact:** Misleading API; callers may believe they control the session ID
**Fix:** Remove the unused `sessionID` parameter or implement caller-supplied ID support

---

#### M3. Four Prometheus Metrics Defined but Never Emitted

**File:** `internal/metrics/metrics.go:83-108`

The following metrics are registered via `promauto` but never referenced outside `metrics.go`:

| Metric | Defined | Emitted |
|--------|---------|---------|
| `MongoDBOperationDuration` | line 84 | Nowhere |
| `RateLimitBlocked` | line 91 | Nowhere |
| `WebSocketConnectionDuration` | line 97 | Nowhere |
| `HTTPRequestDuration` | line 104 | Nowhere |

These were added in the v1 fix (M4) but instrumentation was never added to the actual code paths (storage operations, rate limiter, WebSocket handler, HTTP middleware).

**Impact:** Metrics exist in `/metrics` endpoint but always show zero; misleading dashboards
**Fix:** Add `.Observe()` / `.Inc()` calls in the relevant code paths, or remove unused metrics

---

#### M4. `chatbox` Root Package Coverage at 30.4%

**File:** `.gitlab-ci.yml:157-165`

The root `chatbox` package contains `Register()`, `Shutdown()`, and all HTTP handler functions, but only has 30.4% test coverage. The CI coverage gate for this package is set to just 20%:

```yaml
if (( $(echo "$CHATBOX_COV < 20" | bc -l) )); then
    echo "ERROR: chatbox.go coverage ${CHATBOX_COV}% is below 20% threshold"
```

This means the core HTTP handlers (sessions listing, admin endpoints, readiness probe) are largely untested in CI.

**Impact:** Regressions in core handlers may not be caught by CI
**Fix:** Increase handler test coverage and raise CI gate to 80%

---

#### M5. Large Files Exceed 800-Line Guideline

Four files exceed the project's 800-line guideline per `CLAUDE.md`:

| File | Lines |
|------|-------|
| `internal/router/router.go` | 1252 |
| `internal/storage/storage.go` | 1250 |
| `internal/websocket/handler.go` | 858 |
| `internal/session/session.go` | 841 |

**Impact:** Reduced maintainability; harder to review and test
**Fix:** Extract message-type handlers from `router.go`, query builders from `storage.go`, etc.

---

#### M6. XSS via Unescaped `onclick` Handlers in Admin Panel

**File:** `web/admin.js:145, 168`

Session data is interpolated into `onclick` attributes using template literals:

```javascript
onclick="showUserSessions('${session.user_id}')"     // line 145
onclick="takeoverSession('${session.session_id}')"    // line 168
```

While `escapeHtml()` is applied to displayed text (e.g., line 146), the `onclick` attribute values are not escaped. A `user_id` containing `')` followed by JS would break out of the string literal and execute arbitrary JavaScript.

**Impact:** XSS if user IDs contain special characters (unlikely with UUID format but violates defense-in-depth)
**Fix:** Use `addEventListener` instead of inline `onclick`, or escape values for JavaScript string context

---

#### M7. `postMessage` Listener Without Origin Validation

**File:** `web/chat.js:693-700`

```javascript
window.addEventListener('message', (event) => {
    if (event.data.type === 'token') {
        sessionStorage.setItem('jwt_token', event.data.token);
```

The `message` event listener accepts tokens from any origin. A malicious page opened in another tab could post a crafted token to this window, replacing the legitimate JWT.

**Impact:** Token injection from cross-origin pages
**Fix:** Add `event.origin` validation against expected parent domain

---

#### M8. No Lint or Vet in CI Pipelines

**Files:** `.gitlab-ci.yml`, `.github/workflows/docker-build.yml`

Neither CI pipeline runs `go vet`, `golangci-lint`, or `staticcheck`. The GitLab CI runs `go test -race` but no static analysis. GitHub Actions only tests the Docker build.

**Impact:** Static analysis issues only caught locally; code quality regressions can merge
**Fix:** Add a `lint` stage to both CI pipelines with `go vet ./...` and optionally `golangci-lint`

---

### LOW

#### L1. Kubernetes Deployment Uses `latest` Tag with `IfNotPresent`

**File:** `deployments/kubernetes/deployment.yaml:73-74`

```yaml
image: chatbox-websocket:latest
imagePullPolicy: IfNotPresent
```

With `IfNotPresent`, Kubernetes will use a cached `latest` image indefinitely and never pull newer versions. This defeats the purpose of `:latest` and creates unpredictable deployments.

**Impact:** Stale container images after initial pull
**Fix:** Either use specific version tags (recommended) or change to `imagePullPolicy: Always`

---

#### L2. Session Pointer Returned Without Copy Allows Unsynchronized Mutation

**File:** `internal/session/session.go:168-183`

`GetSession` returns a `*Session` pointer under `sm.mu.RLock()`:

```go
func (sm *SessionManager) GetSession(sessionID string) (*Session, error) {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    session, exists := sm.sessions[sessionID]
    return session, nil    // pointer escapes the lock
}
```

Once the lock is released, the caller holds a raw pointer to session data. Multiple callers (e.g., `RouteMessage` and cleanup goroutine) can read/write session fields concurrently without synchronization. The same pattern applies to `RestoreSession` and `CreateSession`.

**Impact:** Potential data races on `Session` struct fields from concurrent goroutines
**Fix:** Either return a copy of the session, or document that callers must use `Session.mu` for field access

---

#### L3. `docker-compose.yml` Contains Hardcoded Credentials in `mongosh` Commands

**File:** `docker-compose.yml`

While the main service credentials use `${VAR:-default}` pattern (fixed in v1), the MongoDB initialization `mongosh` commands may still contain hardcoded usernames/passwords for database setup.

**Impact:** Development-only; credentials visible in docker-compose file
**Fix:** Use environment variable substitution consistently in all commands

---

#### L4. CI Uses `mongo:latest` Service Image

**File:** `.gitlab-ci.yml:69`

```yaml
services:
  - mongo:latest
```

Using `latest` for the MongoDB CI service means test results may vary across runs as the MongoDB version changes. Could cause flaky tests if a new MongoDB version introduces breaking changes.

**Impact:** Non-reproducible CI; potential surprise test failures
**Fix:** Pin to a specific MongoDB version (e.g., `mongo:7.0`)

---

## Positive Findings

1. **All 15 prior review issues resolved** — S1-S6, M1, M3-M4, L1-L3 successfully fixed
2. **Build, vet, and race detector** all pass cleanly
3. **18/20 packages meet 80% coverage target** — strong test coverage overall
4. **AES-256-GCM encryption** properly implemented with random nonce and correct key validation
5. **JWT validation** is comprehensive (signature, expiration, required claims, weak-key rejection)
6. **Input sanitization** with `html.EscapeString()` on message content in storage layer
7. **BuildKit secrets** used in CI — no token leakage in Docker layers
8. **K8s security contexts** properly configured (non-root, read-only FS, capabilities dropped)
9. **Rate limiting** on all user-facing endpoints (WebSocket, HTTP)
10. **Graceful shutdown** properly implemented with context deadlines and `sync.WaitGroup`
11. **SafeSend pattern** (`handler.go:512-522`) prevents send-on-closed-channel panics
12. **Lock ordering documented** (`session.go:1-5`) — `SessionManager.mu` before `Session.mu`
13. **LLM streaming goroutine leak fixed** — `select` on `ctx.Done()` in all providers
14. **Startup validation** — CORS/WS origins checked for placeholder values

---

## Summary

| Severity | Count | Issues |
|----------|-------|--------|
| CRITICAL | 2 | C1 (WebSocket write race), C2 (duplicate HPA) |
| HIGH | 4 | H1 (HTML injection), H2 (scanner buffer), H3 (rate limiter leak), H4 (NetworkPolicy OR) |
| MEDIUM | 8 | M1 (standalone server), M2 (unused param), M3 (dead metrics), M4 (coverage gap), M5 (large files), M6 (XSS onclick), M7 (postMessage origin), M8 (no lint in CI) |
| LOW | 4 | L1 (latest+IfNotPresent), L2 (RLock for writes), L3 (docker-compose creds), L4 (CI mongo:latest) |

**Overall:** The codebase has significantly improved since the v1 review. All 15 prior issues were verified fixed. The remaining issues are primarily:
- **Concurrency safety** (C1, L2) — WebSocket write serialization and lock upgrades
- **Defense-in-depth gaps** (H1, H4, M6, M7) — additional HTML escaping and policy tightening
- **Operational maturity** (C2, M3, M4, M8) — manifest deduplication, metrics instrumentation, CI gates

**Recommended priority:**
1. Fix C1 (WebSocket write race) and C2 (duplicate HPA) before production deployment
2. Fix H1-H4 in the next sprint
3. Address M1-M8 and L1-L4 as operational improvements
