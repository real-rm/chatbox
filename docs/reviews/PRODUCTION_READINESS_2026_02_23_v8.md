# Production Readiness Review v8

**Date:** 2026-02-23
**Base Commit:** `9408841` (main)
**Reviewers:** 3 parallel agents (Security & Concurrency, Architecture & Code Quality, Infrastructure & Testing)
**Previous:** v7 fixed 23 issues (0C, 7H, 10M, 6L)
**Status:** ALL 14 ISSUES FIXED

---

## Summary

| Severity | Count | Fixed | Description |
|----------|-------|-------|-------------|
| CRITICAL | 0 | 0 | None |
| HIGH | 4 | 4 | Session ID squatting via RegisterConnection, readPump blocks during LLM streaming, adminConns key scheme mismatch, stale production test |
| MEDIUM | 7 | 7 | MAX_MESSAGE_SIZE no positivity check, Metrics fields always zero, user sessions no pagination, pre-signed URL leaked to LLM, docker-compose healthcheck hardcoded path, CI MongoDB version mismatch, GitHub Actions no govulncheck |
| LOW | 3 | 3 | LLM config comment misleading, encryption key entropy docs, RegisterConnection overwrites without cleanup |
| **Total** | **14** | **14** | |

---

## Rejected Findings

**Agent 2 claimed `recoverStreamPanic` does not work in Go (CRITICAL):** REJECTED. The Go spec states `recover()` is effective when "called directly by a deferred function." `defer recoverStreamPanic(ch, "openai")` makes `recoverStreamPanic` the deferred function, and `recover()` is called directly inside it. This is valid Go. The existing test at `llm_panic_recovery_test.go:23` uses the identical pattern and passes.

---

## HIGH

### H1. Session ID squatting — `RegisterConnection` overwrites without ownership check -- FIXED

**Files:** `internal/websocket/handler.go:706-713`, `internal/router/router.go:134`

When a WebSocket connection sends its first message, `readPump` sets `c.SessionID = msg.SessionID` and calls `h.router.RegisterConnection(msg.SessionID, c)` — which blindly overwrites `mr.connections[sessionID] = conn`. This happens BEFORE `RouteMessage` calls `getOrCreateSession` (which checks ownership).

An authenticated attacker (valid JWT, different user) can send a message with a victim's session ID. The connections map entry for that session is overwritten with the attacker's connection.

**Fix:** Added session ownership check in `RegisterConnection`: if the session exists and the requesting user is not the owner, return an authorization error. Also closes old connection before overwriting (fixes L3).

---

### H2. `readPump` blocks during LLM streaming — pong timeout kills connection after 60s -- FIXED

**Files:** `internal/websocket/handler.go:748`, `internal/router/router.go:282`

`RouteMessage` is called synchronously from `readPump`. For `TypeUserMessage`, `HandleUserMessage` blocks for the entire LLM stream duration (potentially minutes). During this time, `readPump` cannot read WebSocket frames — including pong frames.

The `pongWait` deadline (60s) fires on the read side, causing `ReadMessage()` to return an error and close the connection.

**Fix:** Dispatched `RouteMessage` into a goroutine from `readPump` so the read loop stays free to handle pong frames. Message is copied to avoid data races with loop variable reuse.

---

### H3. `RegisterAdminConnection`/`UnregisterAdminConnection` key scheme conflicts with `HandleAdminTakeover` -- FIXED

**Files:** `internal/router/router.go:1284`, `internal/router/router.go:1165`

Two different key schemes in the same `adminConns` map. `RegisterAdminConnection` used bare `adminID`, while `HandleAdminTakeover`/`BroadcastToSession`/`HandleAdminLeave` used compound `adminID:sessionID`.

**Fix:** Changed `RegisterAdminConnection` and `UnregisterAdminConnection` to require `sessionID` parameter and use compound `adminID:sessionID` key. Updated all test call sites.

---

### H4. Stale `main_production_test.go` documents non-existent missing components -- FIXED

**File:** `cmd/server/main_production_test.go:74-106`

`DocumentMissingFunctionality` sub-test lists 7 "missing" components that all exist in `main.go`. Uses `t.Log` instead of `t.Fatal`, so false assertions never fail. Misleads developers.

**Fix:** Deleted the `DocumentMissingFunctionality` sub-test and updated the stale doc comment.

---

## MEDIUM

### M1. `MAX_MESSAGE_SIZE` accepts zero and negative values -- FIXED

**File:** `chatbox.go:193-194`

Zero or negative values assigned to `maxMessageSize` and passed to `conn.SetReadLimit()`. Zero causes every incoming message to fail with `ErrReadLimit`.

**Fix:** Added positivity check: `if parsedSize > 0 { ... } else { warn and use default }`.

---

### M2. `Metrics` struct `AvgConcurrent` and `MaxConcurrent` are always zero -- FIXED

**Files:** `internal/storage/storage.go:131-132`, `GetSessionMetrics`

Never populated by the aggregation pipeline. Always returns `0`.

**Fix:** Removed unused fields from `Metrics` struct. Added comment recommending external time-series tools for concurrency metrics. Updated all test assertions.

---

### M3. `handleUserSessions` passes 0 as limit — silently truncated to 100 -- FIXED

**File:** `chatbox.go:706`

Users with >100 sessions got truncated results with no indication.

**Fix:** Pass `constants.DefaultSessionLimit` explicitly. Added `limit` and `truncated` fields to the JSON response.

---

### M4. Pre-signed S3 URL leaked to external LLM provider -- FIXED

**File:** `internal/router/router.go:810-812`

`audioFileURL` with pre-signed S3 credentials sent to external LLM. The `redactURLQuery` function existed but was only used for logging.

**Fix:** Applied `redactURLQuery(audioFileURL)` in the LLM message content.

---

### M5. `docker-compose.yml` healthcheck path hardcoded -- FIXED

**File:** `docker-compose.yml:152`

Hardcoded `/chatbox/healthz` breaks if `CHATBOX_PATH_PREFIX` is overridden.

**Fix:** Changed to CMD-SHELL form with `$${CHATBOX_PATH_PREFIX:-/chatbox}` variable expansion.

---

### M6. GitLab CI installs MongoDB 6.0 tooling for 7.0 service -- FIXED

**File:** `.gitlab-ci.yml:20-23,103`

6.0 GPG key and apt repository used with 7.0 service.

**Fix:** Updated to 7.0 GPG key and repository.

---

### M7. GitHub Actions workflow missing `govulncheck` -- FIXED

**File:** `.github/workflows/docker-build.yml`

No vulnerability scanning in GitHub Actions workflow.

**Fix:** Added `govulncheck@v1.1.3` step to the test job.

---

## LOW

### L1. LLM provider config comment misleading -- FIXED

**File:** `internal/config/config.go:348`

Comment says `// No more providers` but behavior is skip-and-continue.

**Fix:** Changed to `// provider slot is unconfigured, scan remaining slots`.

---

### L2. Encryption key generation docs inaccurate about entropy -- FIXED

**Files:** `deployments/kubernetes/secret.yaml.template:15`, `deployments/kubernetes/create-secrets.sh:57`, `config.toml:44`

`openssl rand -hex 16` produces 128-bit entropy (not 256-bit as implied). Still secure but docs were misleading.

**Fix:** Updated comments to accurately state "128-bit entropy" instead of "for AES-256".

---

### L3. `RegisterConnection` overwrites existing connection without closing old one -- FIXED

**File:** `internal/router/router.go:134`

Old connection's `send` channel orphaned on reconnect.

**Fix:** Added `oldConn.SetClosing()` before overwriting in `RegisterConnection` (implemented as part of H1 fix).

---

## Verification Checks

```
gofmt -l .            — PASS
go vet ./...          — PASS
go build ./cmd/server — PASS
go test -short -race ./... — PASS (all 20 packages)
```

---

## Files Modified

| File | Issues Fixed |
|------|-------------|
| `internal/router/router.go` | H1 (ownership check + old conn cleanup), H3 (compound admin keys), M4 (redact URL) |
| `internal/websocket/handler.go` | H2 (goroutine dispatch) |
| `cmd/server/main_production_test.go` | H4 (delete stale test) |
| `chatbox.go` | M1 (MAX_MESSAGE_SIZE check), M3 (pagination metadata) |
| `internal/storage/storage.go` | M2 (remove unused fields) |
| `internal/storage/storage_metrics_test.go` | M2 (test updates) |
| `internal/storage/storage_test.go` | M2 (test updates) |
| `internal/storage/admin_property_test.go` | M2 (test updates) |
| `docker-compose.yml` | M5 (variable healthcheck path) |
| `.gitlab-ci.yml` | M6 (MongoDB 7.0 tooling) |
| `.github/workflows/docker-build.yml` | M7 (govulncheck) |
| `internal/config/config.go` | L1 (comment fix) |
| `config.toml` | L2 (entropy docs) |
| `deployments/kubernetes/secret.yaml.template` | L2 (entropy docs) |
| `deployments/kubernetes/create-secrets.sh` | L2 (entropy docs) |
| `internal/router/router_coverage_test.go` | H3 (test updates) |
| `internal/router/admin_property_test.go` | H3 (test updates) |
| `internal/router/edge_case_test.go` | H3 (test updates) |

---

## Cross-Reference with v7

All v7 fixes verified as correctly applied. The v7 H6 fix (compound admin key in HandleAdminTakeover) correctly uses `adminID:sessionID`; H3 above unified the remaining methods to match.
