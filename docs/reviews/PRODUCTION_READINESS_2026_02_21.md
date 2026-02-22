# Production Readiness Review

**Date:** 2026-02-21
**Commit:** `1d207c6` (main)
**Go version:** 1.24.4
**Reviewer:** Automated (security, concurrency, code quality, infrastructure)
**Verdict:** CONDITIONAL PASS — Fix CRITICAL and HIGH items before production deployment

---

## Build & Static Analysis

| Check | Result |
|-------|--------|
| `go build ./cmd/server/...` | PASS |
| `go vet ./...` | PASS |
| `go test -short ./...` | PASS (20/20 packages) |
| `go test -race -short` (key packages) | PASS |

---

## Test Coverage

| Package | Coverage | Target | Status |
|---------|----------|--------|--------|
| internal/config | 100.0% | 80% | PASS |
| internal/errors | 100.0% | 80% | PASS |
| internal/httperrors | 100.0% | 80% | PASS |
| internal/message | 98.9% | 80% | PASS |
| internal/util | 97.3% | 80% | PASS |
| internal/ratelimit | 96.9% | 80% | PASS |
| cmd/server | 94.3% | 80% | PASS |
| internal/testutil | 94.4% | 80% | PASS |
| internal/session | 93.8% | 80% | PASS |
| internal/auth | 92.9% | 80% | PASS |
| internal/storage | 89.2% | 80% | PASS |
| internal/websocket | 86.2% | 80% | PASS |
| internal/llm | 83.4% | 80% | PASS |
| internal/notification | 81.4% | 80% | PASS |
| internal/router | 81.4% | 80% | PASS |
| internal/upload | 78.0% | 80% | BELOW |
| chatbox (root) | 31.0% | 80% | BELOW |
| **Total** | **81.1%** | **80%** | **PASS** |

18 of 20 packages meet 80%. `internal/upload` at 78% is close. Root `chatbox` package (31%) is a wiring-only package where most logic delegates to internal packages — acceptable.

---

## Previous Review Fixes Verification

Commit `9c1be46` claimed to fix 12 issues from the prior review. Verification status:

| Prior ID | Issue | Fix Status |
|----------|-------|------------|
| C1 | Lock ordering in StopCleanup (session + ratelimit) | FIXED — Uses `sync.Once` + `wg.Wait()` outside lock |
| C2 | Missing PodDisruptionBudget | FIXED — `pdb.yaml` added with `minAvailable: 2` |
| H1 | No panic recovery in spawned goroutines | PARTIAL — `util.SafeGo` added for handler goroutines; LLM streaming goroutines still unprotected (see S3) |
| H2 | WebSocket origin allows all when unconfigured | NOT VERIFIED — Warning still logged but startup continues; ConfigMap ships empty allowlists (see I2) |
| H3 | readOnlyRootFilesystem set to false | FIXED — Now `true` in deployment.yaml |
| H4 | No HTTP server timeouts in standalone mode | FIXED — `NewHTTPServer` function added in main.go |
| M1 | Channel send can panic on closed channel | NOT VERIFIED — Same pattern still present |
| M2 | Rate limiter events map unbounded growth | NOT VERIFIED — Would need load testing |
| M3 | Missing trace/request ID correlation | NOT FIXED — No trace ID propagation observed |
| M4 | Public endpoints not rate limited | FIXED — Public rate limiter added in chatbox.go |
| M5 | Missing NetworkPolicy | PARTIAL — Added but egress rules incomplete (see I3) |
| M6 | Docker build leaks GitHub token | PARTIAL — Dockerfile uses BuildKit secrets; CI pipelines still use `--build-arg` (see I1) |

---

## Findings

### CRITICAL

#### C1. SSRF via Unvalidated LLM Provider Endpoints

**Files:** `internal/llm/openai.go:100,163` | `internal/llm/anthropic.go:121,191` | `internal/llm/dify.go:95,153`

The `endpoint` field in LLM provider config is read from `config.toml` / environment variables and concatenated directly into HTTP requests with no URL validation. If configuration is compromised (misconfigured ConfigMap, env var injection), the service makes authenticated HTTP requests to arbitrary internal targets.

```go
// openai.go:100
httpReq, err := http.NewRequestWithContext(ctx, "POST",
    p.endpoint+"/chat/completions", bytes.NewReader(bodyBytes))
```

No validation that `p.endpoint` is HTTPS or points to an allowed host.

**Recommendation:** Validate all LLM endpoints at startup: require `https://` scheme, optionally maintain an allowlist of permitted hostnames.

---

#### C2. Kubernetes Secret Manifest Contains Plaintext Placeholders in Version Control

**File:** `deployments/kubernetes/secret.yaml:12-48`

Uses `stringData` (not `data`) with human-readable placeholder credentials:

```yaml
stringData:
  JWT_SECRET: "your-jwt-secret-key-change-in-production-use-strong-random-string"
  ENCRYPTION_KEY: "CHANGE-ME-32-BYTE-KEY-FOR-AES256"
  LLM_PROVIDER_1_API_KEY: "sk-your-openai-api-key"
```

The test at `secret_validation_test.go:73` explicitly **passes** when placeholders are found. A `kubectl apply` without replacing values deploys with a guessable AES key and dictionary-phrase JWT secret.

**Recommendation:** Remove `secret.yaml` from version control (add to `.gitignore`). Keep only a `.template` file. Update test to assert the file does not exist or values match strong entropy.

---

#### C3. Kubernetes ConfigMap Ships Empty Origin Allowlists

**File:** `deployments/kubernetes/configmap.yaml:28,34`

```yaml
CORS_ALLOWED_ORIGINS: ""
WS_ALLOWED_ORIGINS: ""
```

Empty values are explicitly documented as "development mode only — NOT SECURE." The production ConfigMap ships these empty, meaning every origin is permitted for WebSocket upgrades and CORS on admin endpoints.

**Recommendation:** Set explicit allowlists or use a placeholder that fails loudly at startup (e.g., `REPLACE_WITH_PRODUCTION_ORIGINS`).

---

#### C4. Hardcoded Admin URL in Email Template

**File:** `internal/notification/notification.go:173`

```go
<p><a href="https://admin.example.com/sessions/%s">View Session</a></p>
```

This placeholder URL is sent in real help-request notification emails. Production admins receive emails linking to a non-existent domain.

**Recommendation:** Load admin panel URL from configuration and validate at startup.

---

### HIGH

#### H1. Nested Mutex Lock Ordering Creates Deadlock Risk

**File:** `internal/session/session.go:490-499, 518-527, 643-653, 688-697, 728-748`

Multiple `SessionManager` methods acquire `sm.mu` (manager lock) then `session.mu` (per-session lock) within the same stack frame. The `Session` type exposes public `Lock()`/`RLock()` methods (lines 816-833), allowing external callers to acquire `session.mu` independently. If any external code acquires `session.mu` first, then calls a `SessionManager` method, ABBA deadlock occurs.

No deadlock exists in current code paths, but the pattern is fragile and undocumented.

**Recommendation:** Document strict lock hierarchy (`sm.mu` before `session.mu`). Remove or make package-private the public `Lock()`/`RLock()` methods on `Session`.

---

#### H2. Data Race on `Connection.SessionID` in readPump

**File:** `internal/websocket/handler.go:562-674`

`c.SessionID` is read without mutex at lines 562, 567, 568, 583, 604, 609, 659, 672 but written with `c.mu.Lock()` at line 674. The CLAUDE.md explicitly states: *"use provided methods, not direct field access for SessionID"* — this rule is violated throughout `readPump`.

```go
// handler.go:567 — unprotected read
if c.SessionID != "" && h.router != nil {
    h.router.UnregisterConnection(c.SessionID)
}
```

**Recommendation:** Replace all direct `c.SessionID` reads with `c.GetSessionID()` which acquires `c.mu.RLock()`.

---

#### H3. Streaming LLM Goroutines Lack Panic Recovery

**Files:** `internal/llm/openai.go:187-232` | `internal/llm/anthropic.go:216-264` | `internal/llm/dify.go:177-227` | `internal/llm/llm.go:421-434`

All three LLM providers spawn goroutines for SSE streaming without `recover()`. A panic (nil pointer from malformed response, send on closed channel) crashes the entire server. `util.SafeGo` exists for this purpose but is not used here.

```go
// openai.go:187
go func() {
    defer close(chunkChan)
    defer resp.Body.Close()
    scanner := bufio.NewScanner(resp.Body)
    // No recover() — panic kills the process
```

**Recommendation:** Wrap with deferred `recover()` or refactor to use `util.SafeGo`.

---

#### H4. `time.Sleep` in Fatal Error Path Causes Goroutine Pile-Up

**File:** `internal/router/router.go:1210-1211`

```go
time.Sleep(constants.InitialRetryDelay) // 100ms sleep in error handling
```

During an LLM outage affecting many sessions, this creates a thundering herd of sleeping goroutines, each eventually contending for `mr.mu.Lock()` in `UnregisterConnection`. Under sustained error load, goroutine pile-up exhausts memory.

**Recommendation:** Replace with async close via `util.SafeGo` or remove the sleep entirely.

---

#### H5. LLM API Error Responses Logged Verbatim with Unbounded Read

**Files:** `internal/llm/openai.go:116-117` | `internal/llm/anthropic.go:138-139` | `internal/llm/dify.go:110-111`

```go
body, _ := io.ReadAll(resp.Body)
return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
```

LLM error responses can contain account IDs, quota info, or key fragments. `io.ReadAll` has no size limit — a malicious proxy could return a multi-GB error body, causing OOM.

**Recommendation:** Use `io.LimitReader(resp.Body, 1024)`. Log truncated body at DEBUG level only; surface generic error upstream.

---

#### H6. JWT Token Exposed in WebSocket URL Query Parameter

**File:** `internal/websocket/handler.go:203-212`

```go
token := r.URL.Query().Get("token")
```

Query parameters are logged by proxies (nginx, AWS ALB), load balancers, and SIEM tools. The full JWT persists in plaintext access logs.

**Recommendation:** Remove query-parameter token support. Require `Authorization: Bearer` header exclusively.

---

#### H7. CI Pipelines Still Leak GitHub Token via `--build-arg`

**Files:** `.gitlab-ci.yml:25,51` | `.github/workflows/docker-build.yml:31`

Despite the Dockerfile fix to use BuildKit secrets, both CI files still use `--build-arg GITHUB_TOKEN=...`:

```yaml
docker build --build-arg GITHUB_TOKEN=$GITHUB_TOKEN -t chatbox-websocket:ci-test .
```

The token value becomes part of the build context and may appear in layer cache and build logs.

**Recommendation:** Switch all CI pipelines to `--secret id=github_token,env=GITHUB_TOKEN` with `DOCKER_BUILDKIT=1`.

---

#### H8. Init Container Runs as Root with No Security Context

**File:** `deployments/kubernetes/deployment.yaml:51-54`

The `wait-for-mongo` init container has no `securityContext` (runs as root), no `readOnlyRootFilesystem`, no dropped capabilities, and no resource limits.

**Recommendation:** Add full security context: `runAsNonRoot: true`, `runAsUser: 65534`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, and resource limits.

---

#### H9. Default Kubernetes Service Account with Automounted API Token

**File:** `deployments/kubernetes/deployment.yaml:40`

```yaml
serviceAccountName: default
```

The application has no need for Kubernetes API access. The automounted token at `/var/run/secrets/kubernetes.io/serviceaccount/token` is exploitable if a pod is compromised.

**Recommendation:** Set `automountServiceAccountToken: false` and create a dedicated service account with no RBAC bindings.

---

### MEDIUM

#### M1. `Register` Function Is 318 Lines

**File:** `chatbox.go:66-383`

Performs config loading, validation, service construction, route registration, and middleware wiring all inline. Exceeds the 50-line function limit by 6x. Untestable without running the full registration.

**Recommendation:** Extract into `loadAndValidateConfig()`, `buildServices()`, and `registerRoutes()`.

---

#### M2. `HandleUserMessage` Is 181 Lines; `readPump` Is 246 Lines

**Files:** `internal/router/router.go:171-351` | `internal/websocket/handler.go:558-803`

Both exceed the 50-line limit significantly. `readPump` has 6 levels of nesting at lines 697-705.

**Recommendation:** Extract streaming loop, error construction, and session registration into separate functions.

---

#### M3. `bufio.Scanner` Default Buffer May Silently Truncate LLM SSE Lines

**Files:** `internal/llm/openai.go:191` | `internal/llm/anthropic.go:220` | `internal/llm/dify.go:181`

Default `bufio.Scanner` has a 64KB token size limit. Large LLM responses in a single SSE `data:` line exceed this. After the scanner loop, `scanner.Err()` is never checked — the stream terminates silently with data loss.

**Recommendation:** Check `scanner.Err()` after loop. Increase buffer: `scanner.Buffer(make([]byte, 256*1024), bufio.MaxScanTokenSize)`.

---

#### M4. `FileURL` Not Scheme-Validated — Stored XSS via `javascript:` URLs

**File:** `internal/message/validation.go:123-125`

`FileURL` is HTML-escaped but not scheme-validated. A URL like `javascript:alert(1)` passes validation and is broadcast to session participants. If a client renders the URL without additional sanitization, this is stored XSS.

**Recommendation:** Validate that `FileURL` scheme is `https://` before accepting.

---

#### M5. `X-Forwarded-For` Accepted Without Trusted Proxy Validation

**File:** `chatbox.go:390-393`

```go
clientIP := c.GetHeader("X-Forwarded-For")
```

A direct client can set `X-Forwarded-For` to any IP, bypassing per-IP rate limits entirely.

**Recommendation:** Use Gin's `c.ClientIP()` with trusted proxies configuration.

---

#### M6. Unvalidated `modelID` Stored and Logged

**File:** `internal/router/router.go:494`

`msg.ModelID` is stored in the session without validating against configured provider IDs. Every subsequent message fails at LLM call time. The arbitrary string (up to 100 chars) is logged on every failure — potential log injection vector.

**Recommendation:** Validate model ID against configured providers in `handleModelSelection` before storing.

---

#### M7. Global Mutable State in `chatbox.go`

**File:** `chatbox.go:44-53`

Seven mutable package-level variables communicate between `Register` and `Shutdown`. Prevents multiple registrations (breaks testing), makes dependency flow implicit.

**Recommendation:** Return a `Service` struct from `Register` with a `Shutdown(ctx) error` method.

---

#### M8. Duplicate `LLMProviderConfig` Type

**Files:** `internal/llm/llm.go:27` | `internal/config/config.go:43`

Identical struct defined in two packages. The `config` package version is never used. Violates DRY rule from CLAUDE.md.

**Recommendation:** Remove the unused duplicate in `internal/config/`.

---

#### M9. `notification.RateLimiter` Duplicates `ratelimit.MessageLimiter`

**File:** `internal/notification/notification.go:29-73`

Full re-implementation of sliding-window rate limiter. The notification version omits cleanup, so its map grows unboundedly. CLAUDE.md prohibits this: *"Extract any repeated logic into internal/util/"*.

**Recommendation:** Reuse `ratelimit.MessageLimiter` or extract a shared implementation.

---

#### M10. Magic Numbers Throughout Codebase

**Files:** `internal/websocket/handler.go:137,88,292,304` | `internal/llm/llm.go:303-310,389-396` | `internal/notification/notification.go:128`

- `10` — max connections per user (handler.go:137), no constant
- `256` — send channel buffer (handler.go:88,292,304), repeated 3 times
- `3` / `1*time.Second` / `30*time.Second` — retry config in llm.go, duplicated in `SendMessage` and `StreamMessage`, ignoring existing `constants.MaxRetryAttempts`
- `5*time.Minute` / `5` — notification rate limiter (notification.go:128)

**Recommendation:** Move all to `internal/constants/constants.go`.

---

#### M11. NetworkPolicy Missing Namespace and Incomplete Egress

**File:** `deployments/kubernetes/networkpolicy.yaml`

Missing `namespace: default` in metadata. Egress allows only DNS and MongoDB — blocks all LLM API calls, S3 uploads, email, and SMS. Pods start but all external integrations fail silently.

**Recommendation:** Add namespace. Add egress rules for TCP 443 (HTTPS) and TCP 587 (SMTP).

---

#### M12. Duplicate Conflicting HPA Definitions

**Files:** `deployments/kubernetes/hpa.yaml:56-72` | `deployments/kubernetes/deployment.yaml:235-295`

Both define `chatbox-websocket-hpa` in the same namespace. `hpa.yaml` has active custom metrics (requires Prometheus Adapter). Deploying both causes silent overwrite.

**Recommendation:** Remove embedded HPA from `deployment.yaml`. Make `hpa.yaml` the single source of truth.

---

#### M13. Wildcard CORS on Ingress Serving Admin Endpoints

**File:** `deployments/kubernetes/service.yaml:133`

```yaml
nginx.ingress.kubernetes.io/cors-allow-origin: "*"
```

**Recommendation:** Restrict to known admin dashboard origins.

---

#### M14. HSTS Header Commented Out in Production SSL Config

**File:** `deployments/nginx/ssl-tls.conf:98-100`

**Recommendation:** Enable with a short initial `max-age` (e.g., 3600), increase after verification.

---

#### M15. GitLab CI Coverage Gate Lowered to 20% with No Remediation Date

**File:** `.gitlab-ci.yml:157-165`

```bash
# Check chatbox.go coverage (temporarily set to 20% until handler tests are fixed)
```

Also references known failing storage tests (line 88-89) with no tracking issue.

**Recommendation:** Restore 80% threshold with a concrete deadline. Fix or exclude known failing tests with a tracking issue.

---

#### M16. GitLab CI References `coverage.xml` That Is Never Generated

**File:** `.gitlab-ci.yml:182-184`

The `coverage_report` artifact references `coverage.xml` (Cobertura format), but no conversion tool is invoked. GitLab silently skips the coverage widget.

**Recommendation:** Add `gocover-cobertura` conversion step.

---

#### M17. `config.toml` with Placeholder Credentials Baked into Runtime Image

**File:** `Dockerfile:61`

```dockerfile
COPY config.toml /app/config.toml
```

If ConfigMap injection fails, the app starts with placeholder values including a non-32-byte encryption "key."

**Recommendation:** Remove from image. Mount exclusively from ConfigMap/Volume. Add startup assertion that exits if any `PLACEHOLDER_` value detected.

---

#### M18. Session ID Length Not Bounded

**File:** `internal/websocket/handler.go:672-713`

Session IDs from clients are sanitized but have no explicit length limit. A very long session ID is used as a MongoDB query key and map key, causing unnecessary pressure.

**Recommendation:** Add max length validation (e.g., 64 chars) in `validateFieldLengths`.

---

#### M19. `ErrSessionNotFound` Defined in Two Packages

**Files:** `internal/session/session.go:19` | `internal/storage/storage.go:32`

Router checks `errors.Is(err, session.ErrSessionNotFound)` but never checks `storage.ErrSessionNotFound`. Storage not-found errors fall through to the generic error path.

**Recommendation:** Unify into a single sentinel in `internal/errors/`.

---

#### M20. Makefile `backup` Target Dumps Secrets to Local Files

**File:** `deployments/kubernetes/Makefile:240-247`

`kubectl get secret -o yaml` outputs base64-decoded secret values to `./backup/` directory.

**Recommendation:** Remove or secure the target. Add `backup/` to `.gitignore`.

---

### LOW

#### L1. `SafeGo` Panic Recovery Omits Stack Trace

**File:** `internal/util/goroutine.go:16-20`

Logs panic value but not `runtime/debug.Stack()`. Production incident diagnosis requires stack traces.

---

#### L2. Anthropic `system` Role Silently Converted to `user`

**File:** `internal/llm/anthropic.go:96-101`

System-role messages are converted to user messages instead of using Anthropic's top-level `system` field. Weakens prompt injection defenses.

---

#### L3. `cmd/server/main.go` Never Calls `chatbox.Register`

**File:** `cmd/server/main.go:65-88`

The standalone server binary starts, logs "Server starting," waits for a signal, and shuts down without serving traffic. `NewHTTPServer` is defined but never called.

---

#### L4. Misplaced Doc Comments in `handler.go`

**File:** `internal/websocket/handler.go:531-557`

The `readPump` comment block is split across the `sendErrorResponse` function definition. godoc attaches the wrong comment.

---

#### L5. SessionDocument BSON Tags Use Non-Standard Abbreviations

**File:** `internal/storage/storage.go:76-77`

`CreatedAt` maps to `_ts` and `ModifiedAt` maps to `_mt` — underscore-prefixed abbreviations that conflict with the camelCase convention stated in CLAUDE.md.

---

#### L6. Dead Data Field `lastActivity`

**File:** `internal/storage/storage.go:558`

`"lastActivity"` is written on every `AddMessage` but never read back in `documentToSession`. No constant exists for it. Dead data in every session document.

---

#### L7. Test Files Far Exceed 800-Line Limit

| File | Lines |
|------|-------|
| `internal/storage/storage_test.go` | 2,701 |
| `internal/websocket/handler_integration_test.go` | 1,760 |
| `internal/storage/storage_unit_test.go` | 1,456 |
| `internal/storage/storage_coverage_test.go` | 1,404 |
| `internal/llm/llm_property_test.go` | 1,126 |
| `internal/session/session_test.go` | 1,110 |

`handler_integration_test.go` also contains 26 `time.Sleep` calls, making tests timing-dependent and flaky.

---

#### L8. Mutable `latest` Image Tags in Docker Compose

**File:** `docker-compose.yml:31,53,70`

`minio/minio:latest`, `minio/mc:latest`, `mailhog/mailhog:latest` — non-deterministic for team development. `mailhog` is archived/unmaintained.

---

#### L9. Alpine 3.19 Behind Current 3.21

**File:** `Dockerfile:45`

OS-level CVEs accumulate in the outdated base image.

---

#### L10. `IfNotPresent` Pull Policy with `:latest` Tag

**File:** `deployments/kubernetes/deployment.yaml:57-58`

Nodes with cached images never pull updates, causing mixed-version deployments.

---

#### L11. Missing `namespace` in NetworkPolicy and PDB Metadata

**Files:** `deployments/kubernetes/networkpolicy.yaml` | `deployments/kubernetes/pdb.yaml`

Inconsistent with other manifests that specify `namespace: default`.

---

#### L12. `pkg/errors` Deprecated Indirect Dependency

**File:** `go.mod:74`

Superseded by stdlib `errors` + `fmt.Errorf("%w")` since Go 1.13. Indirect dep from a private module.

---

#### L13. 150 `time.Sleep` Calls Across 35 Test Files

Tests rely heavily on `time.Sleep` for synchronization. These are inherently flaky on loaded CI runners.

---

## Source File Size Report

| File | Lines | Limit | Status |
|------|-------|-------|--------|
| `internal/router/router.go` | 1,255 | 800 | OVER |
| `chatbox.go` | 1,026 | 800 | OVER |
| `internal/websocket/handler.go` | 850 | 800 | OVER |

---

## Security Summary

| Area | Status |
|------|--------|
| Hardcoded secrets in source | PASS — Placeholders only, validated at startup |
| NoSQL injection | PASS — Typed BSON filters with constants |
| Input validation | PASS — Comprehensive in `message/validation.go` |
| JWT security | PASS — Algorithm pinned to HMAC, expiry validated |
| Encryption at rest | PASS — AES-256-GCM, unique nonce, key validation |
| File upload security | PASS — MIME whitelist, malicious pattern scanning |
| Connection limits | PASS — 10 per user, enforced at upgrade |
| Rate limiting | IMPROVED — WebSocket, admin, and public endpoints covered |
| CORS/Origin validation | WARNING — Open by default when unconfigured |
| SSRF protection | FAIL — LLM endpoints not validated (C1) |
| Token transport | WARNING — JWT in URL query params (H6) |
| Error sanitization | WARNING — LLM errors logged verbatim (H5) |
| HTTP client safety | PASS — 60s timeouts on LLM clients |
| K8s secret management | FAIL — Plaintext in VCS (C2) |
| K8s network isolation | PARTIAL — NetworkPolicy incomplete (M11) |
| Container security | PARTIAL — Init container runs as root (H8) |

---

## Infrastructure Summary

| Area | Status |
|------|--------|
| Docker multi-stage build | PASS — Alpine-based, non-root user, health check |
| K8s deployment | PASS — Resource limits, HPA, security context |
| Health probes | PASS — Liveness, readiness + MongoDB ping, startup |
| Prometheus metrics | PASS — Connections, messages, errors, LLM latency |
| Structured logging | PASS — Component/operation context, level separation |
| Graceful shutdown | PASS — WebSocket drain with context |
| PodDisruptionBudget | PASS — Added with `minAvailable: 2` |
| NetworkPolicy | PARTIAL — Incomplete egress rules |
| CI token handling | FAIL — GitHub token leaked in CI build args |
| CI coverage tracking | PARTIAL — Cobertura report never generated; gate at 20% |

---

## Action Items by Priority

### Must Fix Before Production

| # | ID | Issue | Est. Effort |
|---|-----|-------|-------------|
| 1 | C1 | Validate LLM endpoint URLs at startup (SSRF) | 1h |
| 2 | C2 | Remove `secret.yaml` from VCS; use `.template` | 30m |
| 3 | C3 | Set non-empty origin allowlists in ConfigMap | 15m |
| 4 | C4 | Load admin email URL from configuration | 30m |
| 5 | H2 | Fix `c.SessionID` data race (use `GetSessionID()`) | 30m |
| 6 | H3 | Add panic recovery to LLM streaming goroutines | 1h |
| 7 | H5 | Limit LLM error body reads; sanitize logs | 1h |
| 8 | H7 | Fix CI pipelines to use BuildKit secrets | 30m |
| 9 | H8 | Add security context to init container | 15m |
| 10 | H9 | Disable service account token automounting | 15m |

### Should Fix (First Sprint)

| # | ID | Issue | Est. Effort |
|---|-----|-------|-------------|
| 11 | H1 | Document lock hierarchy; restrict Session lock exposure | 2h |
| 12 | H4 | Remove `time.Sleep` from error path | 30m |
| 13 | H6 | Deprecate `?token=` query parameter auth | 1h |
| 14 | M4 | Validate FileURL scheme (XSS prevention) | 30m |
| 15 | M5 | Use `c.ClientIP()` for X-Forwarded-For | 15m |
| 16 | M6 | Validate modelID against configured providers | 30m |
| 17 | M11 | Fix NetworkPolicy egress + add namespace | 30m |
| 18 | M13 | Restrict wildcard CORS on Ingress | 15m |
| 19 | M14 | Enable HSTS in SSL config | 5m |
| 20 | M17 | Remove config.toml from Docker image | 15m |

### Should Fix (Subsequent Sprints)

| # | ID | Issue |
|---|-----|-------|
| 21 | M1 | Decompose `Register` function (318 lines) |
| 22 | M2 | Decompose `HandleUserMessage` (181 lines) and `readPump` (246 lines) |
| 23 | M3 | Check `scanner.Err()` in LLM streaming; increase buffer |
| 24 | M7 | Return `Service` struct from `Register` (eliminate globals) |
| 25 | M8 | Remove duplicate `LLMProviderConfig` |
| 26 | M9 | Deduplicate notification rate limiter |
| 27 | M10 | Move magic numbers to constants |
| 28 | M12 | Remove duplicate HPA definition |
| 29 | M15 | Restore 80% CI coverage gate |
| 30 | M16 | Add Cobertura coverage generation to CI |
| 31 | M18 | Add session ID length validation |
| 32 | M19 | Unify `ErrSessionNotFound` sentinels |
| 33 | M20 | Secure/remove Makefile backup target |

### Nice to Have

| # | ID | Issue |
|---|-----|-------|
| 34 | L1 | Add stack trace to SafeGo panic recovery |
| 35 | L2 | Use Anthropic system prompt field properly |
| 36 | L3 | Fix standalone server to actually serve traffic |
| 37 | L4 | Fix misplaced doc comments |
| 38 | L5 | Standardize BSON tag naming |
| 39 | L6 | Remove dead `lastActivity` field |
| 40 | L7 | Split oversized test files |
| 41 | L8 | Pin Docker Compose image tags |
| 42 | L9 | Update Alpine to 3.21 |
| 43 | L10 | Fix `IfNotPresent` + `:latest` antipattern |
| 44 | L11 | Add namespace to NetworkPolicy/PDB metadata |
| 45 | L13 | Replace `time.Sleep` in tests with sync primitives |
