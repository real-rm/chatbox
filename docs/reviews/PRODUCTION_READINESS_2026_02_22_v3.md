# Production Readiness Review (v3)

**Date:** 2026-02-22
**Commit:** `8fd7fe5` (main)
**Go version:** 1.25.6
**Reviewer:** Automated (4-agent audit: Go code quality, security, architecture, test coverage/CI)
**Previous review:** `PRODUCTION_READINESS_2026_02_22_v2.md` at commit `a2349ef` — 17 of 18 issues fixed in `8fd7fe5` (M5 deferred)
**Verdict:** CONDITIONAL PASS — 3 CRITICAL, 7 HIGH, 12 MEDIUM, 7 LOW issues identified

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
| `chatbox` (root) | 30.0% | 80% | BELOW |
| `cmd/server` | 57.4% | 80% | BELOW |
| `internal/auth` | 92.9% | 80% | OK |
| `internal/config` | 100.0% | 80% | OK |
| `internal/errors` | 100.0% | 80% | OK |
| `internal/httperrors` | 100.0% | 80% | OK |
| `internal/llm` | 83.0% | 80% | OK |
| `internal/message` | 98.9% | 80% | OK |
| `internal/notification` | 82.9% | 80% | OK |
| `internal/ratelimit` | 96.9% | 80% | OK |
| `internal/router` | 83.6% | 80% | OK |
| `internal/session` | 93.8% | 80% | OK |
| `internal/storage` | 89.4% | 80% | OK |
| `internal/testutil` | 94.4% | 80% | OK |
| `internal/upload` | 78.0% | 80% | BELOW |
| `internal/util` | 97.3% | 80% | OK |
| `internal/websocket` | 87.5% | 80% | OK |

15/20 packages meet 80% target (excluding `internal/constants` [no test files], `internal/metrics` [no statements], `deployments/kubernetes` [no statements]).

---

## Previous Review (v2) Fixes Verification

17 of 18 issues from `PRODUCTION_READINESS_2026_02_22_v2.md` were fixed in commit `8fd7fe5`:

| ID | Issue | Status |
|----|-------|--------|
| C1 | WebSocket concurrent write race | FIXED — mutex in writePump |
| C2 | Duplicate HPA definition | FIXED — removed from deployment.yaml |
| H1 | HTML injection in SendCriticalError/SendSystemAlert | FIXED — html.EscapeString() applied |
| H2 | bufio.Scanner 64KB buffer truncation | FIXED — 1MB buffer in all LLM providers |
| H3 | Notification RateLimiter unbounded growth | FIXED — periodic cleanup added |
| H4 | NetworkPolicy OR→AND logic | FIXED — combined selectors |
| M1 | Standalone server non-functional | FIXED — Register/Shutdown/HTTP server added |
| M2 | createNewSession unused sessionID param | FIXED — parameter removed |
| M3 | Four Prometheus metrics never emitted | FIXED — instrumentation added |
| M4 | chatbox root coverage CI gate at 20% | FIXED — raised to 30% (incrementally) |
| M5 | Large files >800 lines | DEFERRED — requires major refactoring |
| M6 | XSS via onclick handlers in admin panel | FIXED — data attributes + addEventListener |
| M7 | postMessage without origin validation | FIXED — origin check added |
| M8 | No lint/vet in CI | FIXED — lint stage added to both pipelines |
| L1 | latest+IfNotPresent in K8s | FIXED — imagePullPolicy: Always |
| L2 | Session pointer escaped without copy | FIXED — documentation added |
| L3 | docker-compose hardcoded credentials | FIXED — env var substitution |
| L4 | CI mongo:latest | FIXED — pinned to mongo:7.0 |

---

## Findings

### CRITICAL

#### C1. Session Struct Fields Mutated Without Per-Field Lock

**Files:** `internal/session/session.go` — `RestoreSession`, `EndSession`, `AddMessage`, `GetSessionMetrics`

The `SessionManager` uses `sm.mu` (RWMutex) to protect the session map, but individual `Session` struct fields are mutated without holding `Session.mu`. The documented lock ordering is `SessionManager.mu → Session.mu`, but several methods skip `Session.mu`:

```go
// RestoreSession — mutates session fields under sm.mu.Lock() only
func (sm *SessionManager) RestoreSession(sess *Session) error {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    sess.IsActive = true          // no sess.mu.Lock()
    sess.LastActivity = time.Now() // no sess.mu.Lock()
    sm.sessions[sess.ID] = sess
}
```

Once the session is stored in the map and `sm.mu` is unlocked, other goroutines (via `RouteMessage`, cleanup goroutine, or concurrent WebSocket handlers) can access the same session pointer and race on `IsActive`, `LastActivity`, `Messages`, `TotalTokens`, and `ResponseTimes`.

Similarly, `EndSession` sets `IsActive = false` and `EndTime` without `sess.mu`, and `AddMessage` appends to `Messages` slice and updates `TotalTokens` without `sess.mu`.

**Impact:** Data race on Session struct fields under concurrent access; potential corrupt session state
**Fix:** Acquire `sess.mu.Lock()` in `RestoreSession`, `EndSession`, `AddMessage`, and any method that mutates session fields after the session is in the map

---

#### C2. In-Memory Session Store Cannot Scale Horizontally

**Files:** `internal/session/session.go`, `internal/router/router.go`

The `SessionManager` stores all active sessions in a Go `map[string]*Session` protected by a single `sync.RWMutex`. This is a single-process, single-node design:

- Running 2+ replicas means each instance has a different session map
- A user's WebSocket connection is pinned to one instance; if that instance restarts, the session is lost
- There is no session rehydration from MongoDB on startup
- The rate limiter (`internal/ratelimit/ratelimit.go`) has the same limitation — per-instance sliding window

The Kubernetes HPA is configured to scale to 10 replicas, but session affinity is not configured in any ingress or service manifest.

**Impact:** Session loss on pod restart; inconsistent session state across replicas; rate limiting ineffective at scale
**Fix:** Either:
- (a) Add Redis-backed session store (recommended for horizontal scaling)
- (b) Configure sticky sessions in ingress + add session rehydration from MongoDB on startup
- (c) Document single-replica limitation and disable HPA

---

#### C3. Placeholder Detection Gap for JWT Secret and Encryption Key

**Files:** `chatbox.go`, `internal/config/config.go`

The startup validation checks for `PLACEHOLDER_` prefix in JWT secret and encryption key. However, `goconfig` falls through to config.toml defaults when environment variables are not set. If `config.toml` contains a non-placeholder but weak value (e.g., `jwt_secret = "mysecretkey123456789012345678901234"`), it will pass the placeholder check and the weak-pattern check (which only looks for exact matches like "secret", "password", "12345678").

```go
// chatbox.go — only checks for PLACEHOLDER_ prefix
if strings.HasPrefix(jwtSecret, "PLACEHOLDER_") {
    return fmt.Errorf("JWT secret contains placeholder value")
}
```

A real config.toml value that is 32+ characters but not in the weak list will be accepted.

**Impact:** Weak production secrets if config.toml ships with non-placeholder default values
**Fix:** Add entropy check (e.g., reject secrets with Shannon entropy < 3.0 bits/char) or require secrets to be provided exclusively via environment variables

---

### HIGH

#### H1. `publicRateLimitMiddleware` Trusts X-Forwarded-For Without Validation

**File:** `chatbox.go` (rate limit middleware)

The rate limiter uses the client IP for per-IP throttling. If it reads from `X-Forwarded-For` or `X-Real-IP` headers (common in Gin), an attacker behind a proxy can spoof the header to bypass rate limiting entirely by rotating fake IPs.

```go
// Gin's c.ClientIP() trusts X-Forwarded-For by default
ip := c.ClientIP()
```

**Impact:** Rate limiting bypass via header spoofing; enables brute-force or DoS
**Fix:** Configure Gin to only trust known proxy IPs: `r.SetTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12"})` matching your infrastructure

---

#### H2. `handleAdminTakeover` Creates Connection with nil WebSocket Conn

**File:** `internal/websocket/handler.go` — `handleAdminTakeover`

When an admin takes over a session, a `websocket.Connection` is created with `Conn: nil` to represent the admin:

```go
adminConn := &websocket.Connection{
    UserID: claims.UserID,
    Conn:   nil,  // no actual WebSocket connection
}
```

Any code path in `MessageRouter` that tries to send a message to this connection will nil-dereference `Conn`, causing a panic. While the current `RouteMessage` path may not write directly to the admin connection, future changes or error-path writes are unprotected.

**Impact:** Potential nil pointer panic on admin takeover message routing
**Fix:** Either create a real WebSocket connection for admin takeover, or add a nil check in all `Connection.Send*` methods, or use a no-op writer implementation

---

#### H3. `GetSessionMetrics` Loads All Sessions Into Memory

**File:** `internal/session/session.go` — `GetSessionMetrics`

```go
func (sm *SessionManager) GetSessionMetrics() SessionMetrics {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    for _, sess := range sm.sessions { ... }
}
```

This iterates over all sessions under a read lock. With thousands of concurrent sessions, this:
1. Holds `sm.mu.RLock()` for the entire iteration, blocking all writes
2. Computes metrics over all sessions in a single goroutine
3. Is exposed via the `/admin/metrics` HTTP endpoint, making it callable externally

An attacker or misconfigured monitoring system hitting `/admin/metrics` frequently could cause write starvation on the session manager.

**Impact:** Write starvation under high session count; potential DoS via metrics endpoint
**Fix:** Pre-compute metrics incrementally (update counters on session create/end/message) instead of recomputing from scratch

---

#### H4. `/metrics` Prometheus Endpoint Publicly Accessible

**File:** `chatbox.go` — route registration

The `/metrics` endpoint is registered without any authentication middleware:

```go
r.GET("/metrics", gin.WrapH(promhttp.Handler()))
```

This exposes internal application metrics (connection counts, request durations, error rates, Go runtime stats) to anyone. Attackers can use this data for reconnaissance (understanding load patterns, identifying vulnerable endpoints, timing attacks).

**Impact:** Information disclosure; reconnaissance vector
**Fix:** Either protect `/metrics` with authentication, restrict to internal network via NetworkPolicy, or bind to a separate internal port

---

#### H5. `sort_by` and `sort_order` Query Parameters Not Validated

**Files:** `chatbox.go` — admin session listing endpoint, `internal/storage/storage.go`

The admin session list endpoint accepts `sort_by` and `sort_order` as query parameters that are passed to MongoDB queries without validation:

```go
sortBy := c.DefaultQuery("sort_by", "start_time")
sortOrder := c.DefaultQuery("sort_order", "desc")
```

If these values are passed directly into a MongoDB `$orderBy` or `sort()` call, an attacker could:
- Sort by non-indexed fields causing full collection scans
- Inject unexpected field names

**Impact:** Potential NoSQL injection or performance degradation via unvalidated sort parameters
**Fix:** Whitelist allowed sort fields (e.g., `start_time`, `user_id`, `is_active`) and validate sort order is exactly `"asc"` or `"desc"`

---

#### H6. FileURL Not Validated — SSRF Vector

**Files:** `internal/message/message.go`, `internal/upload/upload.go`

File upload responses include a `FileURL` field that is stored and potentially served to users. If the URL is not validated against an allowlist of known storage origins (e.g., the MinIO/S3 bucket URL), a compromised or malicious LLM provider could inject URLs pointing to internal services.

**Impact:** Server-side request forgery if FileURL is fetched server-side; open redirect if served to clients
**Fix:** Validate FileURL against allowed origins (S3 bucket domain) before storing

---

#### H7. Cobertura XML CI Artifact Declared But Never Generated

**File:** `.gitlab-ci.yml:196-199`

```yaml
reports:
  coverage_report:
    coverage_format: cobertura
    path: coverage.xml   # This file is never generated
```

The `coverage` CI job generates `.out` and `.html` files but never produces `coverage.xml` in Cobertura format. GitLab's MR diff coverage annotations silently fail because the declared artifact does not exist. Developers receive no per-line coverage feedback on merge requests.

**Impact:** Broken MR coverage annotations; developers unaware of uncovered lines
**Fix:** Add conversion step: `go install github.com/boumenot/gocover-cobertura@latest && gocover-cobertura < coverage.out > coverage.xml`

---

### MEDIUM

#### M1. LLM Retry Configuration Magic Numbers Duplicated

**Files:** `internal/llm/openai.go`, `internal/llm/anthropic.go`, `internal/llm/dify.go`

All three LLM providers have hardcoded retry parameters (max retries, backoff multiplier, base delay) that are duplicated across files instead of using shared constants from `internal/constants/`:

```go
// Duplicated in each provider
maxRetries := 3
baseDelay := time.Second
```

**Impact:** Inconsistent retry behavior if one provider is updated without the others
**Fix:** Move retry constants to `internal/constants/constants.go` and reference from all providers

---

#### M2. Anthropic `MaxTokens` Hardcoded to 4096

**File:** `internal/llm/anthropic.go`

```go
MaxTokens: 4096,  // hardcoded
```

The OpenAI provider reads `max_tokens` from configuration, but the Anthropic provider hardcodes 4096. Anthropic's Claude models support up to 8192 or more tokens. This limits response length unnecessarily and creates inconsistency between providers.

**Impact:** Truncated Anthropic responses; provider behavior inconsistency
**Fix:** Read `MaxTokens` from provider configuration, falling back to a constant in `internal/constants/`

---

#### M3. `chatbox` Root Package CI Coverage Gate Set to 30%

**File:** `.gitlab-ci.yml:176-178`

The CI gate for the root `chatbox` package was raised from 20% to 30% in v2 fixes, but the project requirement is 80%. The `Register` function — the most business-critical code path — is guarded by a threshold 50 percentage points below the target.

```yaml
if (( $(echo "$CHATBOX_COV < 30" | bc -l) )); then
  echo "NOTE: Target threshold is 80%, incrementally raising from 20% → 30%"
```

**Impact:** Core HTTP handlers (sessions, admin, readiness probe) largely untested in CI
**Fix:** Write integration-style tests with mocked dependencies to drive coverage to 80%; raise gate accordingly

---

#### M4. `cmd/server` Coverage at 57.4% — Below 80% Target

**File:** `cmd/server/main.go`

The standalone server package is below the 80% threshold. Key uncovered paths:

| Function | Coverage |
|----------|----------|
| `runWithSignalChannel` | 38.7% |
| `main` | 0.0% |
| `runMain` | 0.0% |

The MongoDB initialization failure path, `chatbox.Register` failure path, and post-signal shutdown paths are untested.

**Impact:** Regressions in server startup/shutdown not caught by CI
**Fix:** Add integration tests covering failure paths with `canRunFullServer()` guard

---

#### M5. `internal/upload` at 78% — `DownloadFile` 33%, `DeleteFile` 25%

**File:** `internal/upload/upload.go:326-359`

Tests only cover the empty-string guard clause. The success path, `goupload.Download` error path, `goupload.Delete` partial-delete path, and normal delete path are untested.

**Impact:** File I/O error paths not validated
**Fix:** Add tests with mocked `goupload` calls covering all branches

---

#### M6. GitHub Actions Has No Test Stage — Only Lint + Docker Build

**File:** `.github/workflows/docker-build.yml`

The GitHub Actions workflow performs only lint and Docker build. There is no `go test` step, no race detector, no coverage measurement. If the repository uses GitHub for code review, PRs can merge without test validation.

**Impact:** Test failures don't block GitHub PRs
**Fix:** Add `go test -race -short ./...` step to GitHub Actions workflow

---

#### M7. Health Probe Paths May Not Match Configurable Prefix

**Files:** `deployments/kubernetes/deployment.yaml`, `chatbox.go`

The `CHATBOX_PATH_PREFIX` is configurable, but Kubernetes liveness/readiness probes use paths that must match the registered routes. If the prefix changes and the K8s manifest is not updated, health checks fail and pods restart continuously.

**Impact:** Pod crash loop if path prefix changes without manifest update
**Fix:** Use environment variable substitution in K8s manifests for probe paths, or document the coupling explicitly

---

#### M8. Session State Divergence Between Memory and MongoDB

**Files:** `internal/session/session.go`, `internal/storage/storage.go`, `internal/router/router.go`

During an active session, messages are stored in MongoDB individually (via `StorageService.StoreMessage`) but the in-memory `Session` struct also accumulates messages in its `Messages` slice. If the in-memory session is lost (pod restart), the MongoDB session record does not contain the full message history needed to reconstruct the session — individual messages must be queried separately. There is no reconciliation mechanism.

**Impact:** Incomplete session state after pod restart; potential message ordering issues
**Fix:** Either store messages only in MongoDB (not in memory), or add session rehydration from MongoDB on startup

---

#### M9. CI `mongosh` User Creation Not Idempotent

**File:** `.gitlab-ci.yml:95-101`

```yaml
mongosh "...admin" --eval 'db.createUser({user: "chatbox", ...})'
```

If the CI job is retried (common GitLab CI operation), `createUser` fails with "user already exists", causing the test job to never run.

**Impact:** CI job fails on retry
**Fix:** Use `try-catch` in mongosh eval or add `|| true` suffix

---

#### M10. CI Uses Deprecated `apt-key add`

**File:** `.gitlab-ci.yml:97-100`

```yaml
wget -qO - https://www.mongodb.org/static/pgp/server-6.0.asc | apt-key add -
```

`apt-key add` is deprecated since Debian Bullseye/Ubuntu 22.04 and will break when the CI base image updates.

**Impact:** CI breakage on base image update
**Fix:** Use `/usr/share/keyrings/` pattern instead of `apt-key add`

---

#### M11. `docker-compose.yml` Uses Unpinned `minio/minio:latest` and `mailhog/mailhog:latest`

**File:** `docker-compose.yml:31, 73`

Unpinned images cause irreproducible development environments. MinIO has had breaking configuration changes between releases.

**Impact:** Inconsistent local dev environments
**Fix:** Pin to specific version tags

---

#### M12. 110+ `time.Sleep` Calls in Tests — Flaky Test Risk

**Files:** Widespread across test files

The test suite uses `time.Sleep` over 110 times to wait for async goroutines. On loaded CI runners, these sleeps may not be long enough, causing intermittent failures.

**Impact:** Flaky tests in CI; excessively long test suite
**Fix:** Use channels, `sync.WaitGroup`, or testable tick hooks instead of sleeps

---

### LOW

#### L1. Deprecated `Shutdown()` Uses `context.Background()` — Unbounded Timeout

**File:** `chatbox.go` — `Shutdown()`

The deprecated `Shutdown()` function (kept for backward compatibility) calls `ShutdownWithContext(context.Background())`, providing no timeout bound. A stuck MongoDB connection or WebSocket drain could hang the process indefinitely.

**Impact:** Process hang during shutdown if dependencies are unresponsive
**Fix:** Use `context.WithTimeout(context.Background(), 30*time.Second)` in the deprecated wrapper

---

#### L2. `internal/constants` Has No Test File

**File:** `internal/constants/` — no test files

The constants package defines security-related values (`MinJWTSecretLength`, `WeakSecrets`) with no validation tests. A typo or accidental change could weaken security checks.

**Impact:** Silent regression in security constants
**Fix:** Add invariant tests (timeouts > 0, key lengths > 0, weak secret list non-empty)

---

#### L3. Kubernetes Secret Template Contains Weak Non-Placeholder Values

**File:** `deployments/kubernetes/secret.yaml`

Some values in the secret template may use realistic-looking but weak defaults that don't trigger the `PLACEHOLDER_` check.

**Impact:** Weak secrets if template is applied without customization
**Fix:** Ensure all secret template values use `PLACEHOLDER_*` prefix or `REPLACE_WITH_*` prefix consistently

---

#### L4. MongoDB URI in config.toml Has No Authentication

**File:** `config.toml`

The default MongoDB URI uses `mongodb://localhost:27017/chat` without authentication credentials.

**Impact:** Insecure default for local development; potential misconfiguration in staging
**Fix:** Document that production must override via environment variable; add startup warning if no auth

---

#### L5. `docker-compose.yml` Sets Public Anonymous Access on MinIO Bucket

**File:** `docker-compose.yml:65`

```yaml
mc anonymous set download myminio/chat-files;
```

All uploaded chat files become unauthenticated public downloads. While this is the development compose file, copying it to staging would expose user files.

**Impact:** Public file access in development; risk of copy to non-dev environments
**Fix:** Remove `mc anonymous` line; use signed URLs via `goupload`

---

#### L6. CI Test Job Has No Explicit Timeout Flag

**File:** `.gitlab-ci.yml:106`

```yaml
- go test -race -v ./...
```

No `-timeout` flag; defaults to Go's 10-minute timeout. Session and ratelimit packages take 43-60 seconds each in short mode; with race detection, this could exceed 10 minutes on a slow runner.

**Impact:** Tests hang instead of failing on slow runners
**Fix:** Add explicit `-timeout 5m` flag

---

#### L7. Property Tests Use Low `MinSuccessfulTests` (5-20)

**Files:** `internal/llm/llm_property_test.go:778`, `internal/session/session_cleanup_property_test.go:20`

```go
parameters.MinSuccessfulTests = 5   // gopter default is 100
parameters.MinSuccessfulTests = 20  // Reduced from 100
```

Low sample counts drastically reduce input space exploration for property-based tests.

**Impact:** Corner cases missed by property-based tests
**Fix:** Run with at least 50 cases; make tests faster instead of reducing samples

---

## Positive Findings

1. **All 17 v2 issues resolved** — C1-C2, H1-H4, M1-M4, M6-M8, L1-L4 verified fixed
2. **Build, vet, and race detector** all pass cleanly
3. **15/20 packages meet 80% coverage** — strong test coverage overall
4. **AES-256-GCM encryption** properly implemented with random nonce and key validation
5. **JWT validation** comprehensive (signature, expiration, claims, weak-key rejection)
6. **Input sanitization** with `html.EscapeString()` applied broadly
7. **BuildKit secrets** used in CI — no token leakage in Docker layers
8. **K8s security contexts** properly configured (non-root, read-only FS, capabilities dropped)
9. **Rate limiting** on all user-facing endpoints (WebSocket, HTTP)
10. **Graceful shutdown** implemented with context deadlines and `sync.WaitGroup`
11. **SafeSend pattern** prevents send-on-closed-channel panics
12. **Lock ordering documented** — `SessionManager.mu` before `Session.mu`
13. **LLM streaming goroutine leak fixed** — `select` on `ctx.Done()` in all providers
14. **Startup validation** — CORS/WS origins checked for placeholder values
15. **Scanner buffer increased** to 1MB for LLM streaming
16. **CI lint stages added** to both GitLab CI and GitHub Actions
17. **NetworkPolicy AND logic** correctly combined

---

## Summary

| Severity | Count | Issues |
|----------|-------|--------|
| CRITICAL | 3 | C1 (session field races), C2 (in-memory scaling), C3 (placeholder detection gap) |
| HIGH | 7 | H1 (X-Forwarded-For spoofing), H2 (admin takeover nil conn), H3 (metrics OOM), H4 (/metrics public), H5 (sort param injection), H6 (FileURL SSRF), H7 (Cobertura artifact missing) |
| MEDIUM | 12 | M1 (retry magic numbers), M2 (Anthropic MaxTokens), M3 (root coverage 30%), M4 (cmd/server 57%), M5 (upload 78%), M6 (GitHub Actions no tests), M7 (health probe paths), M8 (session state divergence), M9 (CI mongosh idempotency), M10 (apt-key deprecated), M11 (unpinned images), M12 (time.Sleep flakiness) |
| LOW | 7 | L1 (unbounded Shutdown), L2 (constants untested), L3 (secret template), L4 (MongoDB no auth), L5 (MinIO public), L6 (CI no timeout), L7 (low property test counts) |

**Overall:** The codebase has improved significantly through two review cycles. The v2 issues were comprehensively addressed. The remaining findings fall into three categories:

1. **Scalability** (C2, M8) — The in-memory session store is the primary architectural limitation. This is acceptable for single-instance deployment but blocks horizontal scaling.
2. **Concurrency safety** (C1) — Session struct field mutations need per-field locking to match the documented lock ordering.
3. **Security hardening** (C3, H1, H4, H5, H6) — Defense-in-depth gaps around input validation, secret strength, and endpoint exposure.

**Recommended priority:**
1. **Before production deployment:** Fix C1 (session field races), C3 (placeholder detection), H1 (trusted proxies), H4 (/metrics auth), H5 (sort validation)
2. **First sprint after launch:** Fix H2, H3, H6, H7, and address coverage gaps M3-M5
3. **Operational improvements:** C2 (scaling architecture), M1-M12, L1-L7
4. **Architecture decision needed:** C2 requires a product decision — single-instance vs. horizontal scaling determines whether Redis session store is needed
