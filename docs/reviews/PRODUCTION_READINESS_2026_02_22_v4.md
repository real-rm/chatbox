# Production Readiness Review v4

**Date:** 2026-02-22
**Scope:** Full codebase audit — security, code quality, Go patterns, infrastructure
**Status:** BLOCK — Critical issues must be resolved before production

---

## Summary

| Severity | Count | Action |
|----------|-------|--------|
| CRITICAL | 4     | Must fix immediately |
| HIGH     | 10    | Must fix before production |
| MEDIUM   | 12    | Should fix |
| LOW      | 8     | Nice to have |

---

## CRITICAL

### C1. `recoverStreamPanic` never catches panics — all streaming goroutines unprotected

**Files:** `internal/llm/llm.go:428,549-558`, `internal/llm/openai.go:191`, `internal/llm/anthropic.go:220`

`recoverStreamPanic` calls `recover()` inside a regular function. When used as `defer recoverStreamPanic(ch, name)`, `recover()` only returns non-nil when called directly from a deferred function — not from a function called by a deferred function. This means **every streaming goroutine has zero panic protection**. A panic in any LLM provider's stream processing will crash the entire process.

**Fix:** Replace all `defer recoverStreamPanic(...)` with inline `defer func() { if r := recover(); r != nil { ... } }()` closures.

---

### C2. Session fields read without lock across router.go — data races

**Files:** `internal/router/router.go:192,216,385,390,422,552,630,667,670,934-965`

After `GetSession()` returns a shared `*Session` pointer, the router reads `sess.ModelID`, `sess.UserID`, `sess.AssistingAdminID`, `sess.AssistingAdminName` with no lock held. Meanwhile, `SetModelID`, `MarkAdminAssisted`, `ClearAdminAssistance` write these fields under `session.mu`. This is a textbook data race detectable by `-race`.

Also affects `cleanupExpiredSessions` (session.go:334) and `GetMemoryStats` (session.go:364) which read `session.IsActive` without `session.mu`.

**Fix:** Add thread-safe accessor methods to `Session` (e.g., `GetModelID()`, `GetUserID()`) that acquire `session.mu.RLock()`, and use them everywhere in the router.

---

### C3. `Register()` overwrites package-level globals without stopping previous goroutines

**Files:** `chatbox.go:47-56,322-329`

If `Register()` is called twice (tests, hot-reload), it overwrites `globalWSHandler`, `globalSessionMgr`, etc. without calling `Shutdown()` on the previous instances. This permanently leaks background goroutines from `StartCleanup()`, and can panic on duplicate Prometheus metrics registration.

**Fix:** Guard with `sync.Once`, or call `Shutdown()` on existing globals before overwriting.

---

### C4. Double-close of `conn.send` channel in concurrent pump teardown

**Files:** `internal/websocket/handler.go:568,829,356,441-477`

`readPump` defer calls `unregisterConnection(c)` which closes `conn.send`. `writePump` defer calls `c.Close()`. During `ShutdownWithContext`, connections are closed without unregistering first, then `readPump` teardown re-enters and may close the already-closed `send` channel, causing a panic.

**Fix:** Protect `conn.send` close with a `sync.Once`, and ensure only one goroutine owns teardown.

---

## HIGH

### H1. User and AI messages never persisted to MongoDB from the router

**Files:** `internal/router/router.go:51-53,170-351`

The router's `StorageService` interface only exposes `CreateSession`. There is no `AddMessage` method, so `HandleUserMessage` adds messages to in-memory `SessionManager` only. All chat history is lost on pod restart, even though `RehydrateFromStorage` reloads sessions.

**Fix:** Add `AddMessage(sessionID string, msg *session.Message) error` to the router's `StorageService` interface, and call it from `HandleUserMessage` after each user/AI message.

---

### H2. HTML-escaped content sent verbatim to LLM

**Files:** `internal/message/validation.go:181-209`, `internal/router/router.go:210`

`Sanitize()` calls `html.EscapeString()` on `msg.Content`, turning `<` into `&lt;`. This garbled text is then passed directly to the LLM. Users typing `<`, `>`, `&`, `"` get degraded AI responses.

**Fix:** Remove `html.EscapeString` from `Sanitize()`. For WebSocket JSON, null-byte stripping and length limiting are appropriate; HTML escaping belongs at render time only.

---

### H3. No HTTP security headers

**Files:** `chatbox.go` (no middleware), `cmd/server/main.go:95`

No security headers set: `X-Content-Type-Options`, `X-Frame-Options`, `Strict-Transport-Security`, `Referrer-Policy`. Also `gin.Default()` runs in debug mode (leaks stack traces in 500 responses).

**Fix:** Add security headers middleware in `Register()`. Set `gin.SetMode(gin.ReleaseMode)` in production.

---

### H4. Model ID not validated against configured provider whitelist

**Files:** `internal/router/router.go:494`, `internal/llm/llm.go:477`

`handleModelSelection` stores any arbitrary model ID string without calling `ValidateModel()` (which exists but is never called). Invalid IDs are persisted and break subsequent messages.

**Fix:** Call `llmService.ValidateModel(msg.ModelID)` before storing in `handleModelSelection`.

---

### H5. `readPump`/`writePump` goroutines not awaited on shutdown

**Files:** `internal/websocket/handler.go:279-280,437-494`

`ShutdownWithContext` closes connections but does not wait for `readPump` goroutines to exit. `h.connections` may not be fully drained when shutdown returns.

**Fix:** Add a `sync.WaitGroup` to `Handler` that both pumps increment/decrement, waited on in `ShutdownWithContext`.

---

### H6. `handleGetMetrics` calls two separate aggregations — inconsistent and redundant

**Files:** `chatbox.go:854-876`

`GetSessionMetrics` and `GetTokenUsage` run sequentially. Between them data can change, producing inconsistent results. `GetSessionMetrics` already computes `TotalTokens` but the result is overwritten.

**Fix:** Remove the `GetTokenUsage` call; use `TotalTokens` from `GetSessionMetrics`.

---

### H7. AES cipher allocated per encrypt/decrypt call

**Files:** `internal/storage/storage.go:651-657,691-697`

`aes.NewCipher` and `cipher.NewGCM` are called on every message encrypt/decrypt. These perform key schedule computation each time.

**Fix:** Pre-compute `cipher.AEAD` once in `NewStorageService` and store on the struct.

---

### H8. SSRF: file URLs validated by lexical parsing only, not DNS resolution

**Files:** `internal/util/validation.go:150-188`

`ValidateFileURL` rejects private IP literals but not hostnames that resolve to private IPs (DNS rebinding). Also, AI-generated file URLs (`router.go:777-897`) skip validation entirely.

**Fix:** Add DNS pre-flight lookup in `ValidateFileURL`. Apply validation to `HandleAIGeneratedFile` and `HandleAIVoiceResponse`.

---

### H9. Ingress rewrite-target strips path prefix, breaking all routes

**Files:** `deployments/kubernetes/service.yaml:110`

`rewrite-target: /` with `path: /chat` forwards `/chat/ws` as `/`, returning 404 on all routes.

**Fix:** Remove `rewrite-target` annotation or use capture group: `/$2` with `path: /chat(/|$)(.*)`.

---

### H10. CHATBOX_PATH_PREFIX mismatch between Dockerfile and config.toml

**Files:** `Dockerfile:75`, `config.toml:29`

Dockerfile sets `CHATBOX_PATH_PREFIX=/chat`, config.toml sets `path_prefix = "/chatbox"`. Docker HEALTHCHECK will poll the wrong path, causing container restart loops.

**Fix:** Remove baked-in `ENV` from Dockerfile; let config.toml be the single source of truth.

---

## MEDIUM

### M1. Goroutine leak — notification/voice goroutines have no cancellation on shutdown

**Files:** `internal/router/router.go:392-398,671-674`

`util.SafeGo` goroutines use `context.Background()` or fresh timeout contexts, not connected to shutdown. In-flight goroutines are killed on SIGTERM.

**Fix:** Pass the router's lifecycle context to these goroutines.

---

### M2. Goroutine leak — `Register()` error path leaks already-started cleanup goroutines

**Files:** `chatbox.go:285-286,299-300`

If `Register()` fails after `adminLimiter.StartCleanup()`, the goroutine leaks because globals are never set and `Shutdown()` never stops them.

**Fix:** Call `StopCleanup()` in error paths, or start cleanup only after `Register()` succeeds.

---

### M3. Session ownership not verified in `handleModelSelection`, `handleHelpRequest`, `handleFileUpload`

**Files:** `internal/router/router.go:354-411,464-509,511-588`

These handlers call `GetSession(msg.SessionID)` without checking `sess.UserID == conn.UserID`. A user with valid JWT but wrong session ID could modify another user's session.

**Fix:** Add `sess.UserID == conn.UserID` check after `GetSession` in each handler.

---

### M4. `sortByMessageCount` applied after DB pagination — returns wrong results

**Files:** `internal/storage/storage.go:932-1005`

When `SortBy == "message_count"`, docs are fetched with a limit, then re-sorted in memory. This gives the first N by timestamp sorted by message count, not the top N by message count.

**Fix:** Use MongoDB aggregation: `$addFields` + `$sort` + `$limit`.

---

### M5. Duplicate Service definitions in K8s manifests

**Files:** `deployments/kubernetes/deployment.yaml:256-282`, `deployments/kubernetes/service.yaml`

Both define the same Service with different `sessionAffinity` timeouts (3600 vs 10800). Last apply wins silently.

**Fix:** Remove Service from `deployment.yaml`; use `service.yaml` as single source of truth.

---

### M6. `EndSession` in storage makes two MongoDB round-trips (FindOne + UpdateOne)

**Files:** `internal/storage/storage.go:582-636`

**Fix:** Use `FindOneAndUpdate` or accept start time from caller.

---

### M7. Three files exceed 800-line project limit

**Files:** `internal/router/router.go` (1266), `internal/storage/storage.go` (1231), `chatbox.go` (1177)

**Fix:** Split router into `handler_user.go`, `handler_admin.go`, `handler_file.go`. Split storage into `storage_crud.go`, `storage_encryption.go`, `storage_metrics.go`. Extract chatbox handlers.

---

### M8. JWT via query parameter — token value in Gin access logs

**Files:** `internal/websocket/handler.go:214-219`

`gin.Default()` logs full URLs including `?token=...` query params. JWT appears in logs.

**Fix:** Remove query-param token path (already deprecated per code comment), or redact from logs.

---

### M9. `containsAny` in storage.go reimplements `strings.Contains`

**Files:** `internal/storage/storage.go:189-207`

Hand-rolled O(n*m) substring search instead of stdlib.

**Fix:** Replace with `strings.Contains`.

---

### M10. Coverage gate only measures 3 packages — misses majority of codebase

**Files:** `.gitlab-ci.yml:139-151`

Only `./cmd/server`, `./internal/storage`, and `.` are measured. 10+ packages have no coverage gate.

**Fix:** Change to `go test -cover -coverprofile=coverage.out ./...` for full module coverage.

---

### M11. No security scanning in either CI pipeline

**Files:** `.gitlab-ci.yml`, `.github/workflows/docker-build.yml`

No `govulncheck`, `gosec`, or container image scanning.

**Fix:** Add `govulncheck ./...` to test job. Add Trivy scan on built Docker image.

---

### M12. GitLab CI `before_script` duplicated across `test` and `coverage` jobs

**Files:** `.gitlab-ci.yml:91-101,126-136`

11 lines of identical MongoDB setup in two jobs.

**Fix:** Extract to YAML anchor `.mongo-setup`.

---

## LOW

### L1. `RecordResponseTime`/`UpdateTokenUsage` errors silently dropped

**Files:** `internal/router/router.go:341,347`

**Fix:** Log errors with `logger.Warn`.

---

### L2. Deprecated `Shutdown()` uses `context.Background()` — no deadline

**Files:** `internal/websocket/handler.go:430-433`

**Fix:** Remove deprecated method or add context parameter.

---

### L3. `ListUserSessions` accepts `limit=0` for unbounded query

**Files:** `internal/storage/storage.go:724-789`

**Fix:** Default to `constants.DefaultSessionLimit` when 0.

---

### L4. Pre-signed URLs logged in full (expose signing keys)

**Files:** `internal/router/router.go:693-696,551,626-630`

**Fix:** Log only URL path, not query params.

---

### L5. `user_id` query parameter has no length bound

**Files:** `chatbox.go:682`

**Fix:** Reject `len(userID) > 255`.

---

### L6. `make k8s-deploy` references `secret.yaml` but only template exists

**Files:** `Makefile:171`

**Fix:** Add guard or generation step from template.

---

### L7. Container images not digest-pinned in Dockerfile

**Files:** `Dockerfile:3,45`

**Fix:** Pin with `@sha256:<digest>`.

---

### L8. `make docker-build` doesn't pass `--secret` for private modules

**Files:** `Makefile:138`

**Fix:** Add `--secret id=github_token,env=GITHUB_TOKEN` or document requirement.

---

## Fix Priority Order

### Phase 1 — Critical (must fix)
1. C1: Fix `recoverStreamPanic` → inline defer closures
2. C2: Add thread-safe session accessors, fix all unguarded reads
3. C3: Guard `Register()` with sync.Once or prior Shutdown
4. C4: Protect `conn.send` close with sync.Once

### Phase 2 — High (must fix before production)
5. H1: Widen router StorageService interface, persist messages
6. H2: Remove html.EscapeString from Sanitize()
7. H3: Add HTTP security headers middleware
8. H4: Validate model ID against provider whitelist
9. H5: Add WaitGroup for pump goroutines in shutdown
10. H6: Remove redundant GetTokenUsage call
11. H7: Pre-compute AES cipher in NewStorageService
12. H8: Add DNS resolution to SSRF check + validate AI file URLs
13. H9: Fix ingress rewrite-target
14. H10: Fix Dockerfile path prefix mismatch

### Phase 3 — Medium
15-26: M1-M12 in order listed

### Phase 4 — Low
27-34: L1-L8 in order listed
