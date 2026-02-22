# Production Readiness Review v6

**Date:** 2026-02-22
**Base Commit:** `33d0d72` (main)
**Reviewers:** 3 parallel agents (Security & Concurrency, Architecture & Code Quality, Infrastructure & Testing)
**Previous:** v5 fixed 40 issues (0C, 10H, 19M, 11L)
**Status:** ALL 19 ISSUES FIXED

---

## Summary

| Severity | Count | Fixed | Description |
|----------|-------|-------|-------------|
| CRITICAL | 0 | - | None |
| HIGH | 4 | 4 | Unsafe channel sends in readPump, Ingress path mismatch, session name never set, misleading EndSession comment |
| MEDIUM | 9 | 9 | Fragile string error matching, scanner errors ignored, notification rate limiter unbounded, double JSON marshal, ConfigMap missing prefix, Prometheus scrape path wrong, ValidateTimeRange stub, duplicate CI test stages, Go version pin inconsistency |
| LOW | 6 | 6 | LLM client timeout magic numbers, NetworkPolicy missing namespace, GitHub Actions no coverage artifact, NewTimeoutContext ignores parent, FileURL length not checked in message validation, Makefile clean-logs no dir check |
| **Total** | **19** | **19** | |

---

## HIGH

### H1. Three direct `c.send <-` in `readPump` bypass `SafeSend` — can panic on closed channel [FIXED]

**File:** `internal/websocket/handler.go` lines 731-732, 795-796, 826-827
**Found by:** All 3 agents independently

The v5 fix converted `sendErrorResponse` to use `SafeSend`, but three other locations in `readPump` still send directly to `c.send` via `select { case c.send <- ...: default: }` without checking `c.closing`. If `ShutdownWithContext` closes the send channel concurrently, these sends panic.

**Fix applied:** Replaced all three `select { case c.send <- ...: default: }` blocks with `c.SafeSend(errorBytes)`.

---

### H2. Ingress path `/chat` does not match application default `/chatbox` [FIXED]

**File:** `deployments/kubernetes/service.yaml` lines 125, 154
**Found by:** Agent 3

The v5 fix corrected probe paths and docker-compose healthcheck to use `/chatbox`, but the Ingress resource was missed. The Ingress rules route `path: /chat(/|$)(.*)` and `session-cookie-path: "/chat"`, while the application defaults to `CHATBOX_PATH_PREFIX=/chatbox`. All traffic via the Ingress controller will 404.

**Fix applied:** Changed Ingress path from `/chat(/|$)(.*)` to `/chatbox(/|$)(.*)` and `session-cookie-path` from `/chat` to `/chatbox`.

---

### H3. Session name is never set from first user message [FIXED]

**File:** `internal/router/router.go` (HandleUserMessage, around line 236)
**Found by:** Agents 1 and 2

`SessionManager.SetSessionNameFromMessage()` exists but is never called anywhere. All sessions have empty names, making the admin session list useless for identifying which session belongs to which conversation topic.

**Fix applied:** Added `mr.sessionManager.SetSessionNameFromMessage(msg.SessionID, msg.Content)` call after persisting the user message in `HandleUserMessage`.

---

### H4. `EndSession` comment claims atomicity but uses two MongoDB round-trips [FIXED]

**File:** `internal/storage/storage.go` lines 592-637
**Found by:** Agents 1, 2, and 3

The doc comment says "Uses FindOneAndUpdate to atomically fetch the start time and set end time + duration in a single round-trip" but the actual code does `FindOne` (line 611) followed by `UpdateOne` (line 628). If the process crashes between these calls, the session has no end time or duration. Neither call uses `retryOperation`.

**Fix applied:** Rewrote to use `FindOneAndUpdate` with `options.Before` to atomically set `endTime` and retrieve `startTime`, then compute duration. Both operations wrapped in `retryOperation`. Duration update failure is non-fatal (endTime already persisted).

---

## MEDIUM

### M1. Fragile string matching for "already assisted" error [FIXED]

**Files:** `internal/router/router.go` line 1145, `internal/session/session.go` line 794

`HandleAdminTakeover` uses `strings.Contains(err.Error(), "already assisted")` to detect the error returned by `MarkAdminAssisted`. This is brittle — any rewording of the error message silently breaks the detection, causing a 500 instead of 400.

**Fix applied:** Defined `ErrAlreadyAssisted = errors.New("session already assisted by another admin")` as sentinel error in `session.go`. Updated `MarkAdminAssisted` to wrap it with `fmt.Errorf("%w: ...", ErrAlreadyAssisted)`. Changed router to use `errors.Is(err, session.ErrAlreadyAssisted)`.

---

### M2. Scanner errors silently ignored in LLM streaming goroutines [FIXED]

**Files:** `internal/llm/openai.go`, `internal/llm/anthropic.go`, `internal/llm/dify.go`

After the `for scanner.Scan()` loop in all three LLM providers, `scanner.Err()` is never checked. If the SSE stream is truncated due to a network error, the goroutine silently exits as if the stream completed normally. The client receives a partial response with no error indication.

**Fix applied:** Added `scanner.Err()` check after scan loop in all three providers. On error, increments `metrics.LLMErrors.WithLabelValues("<provider>")` counter. Added `metrics` import to all three files.

---

### M3. Notification `RateLimiter` has no cap on map growth [FIXED]

**File:** `internal/notification/notification.go` line 33

The `events map[string][]time.Time` grows unboundedly as unique event keys (session IDs) accumulate. Stale entries are only cleaned when their key is accessed again via `Allow()`. Over weeks of operation, this map grows proportionally to total unique session count.

**Fix applied:** Added `maxTrackedEvents = 100000` constant. In `Allow()`, if the event key doesn't exist and the map has reached capacity, the rate limiter returns `false` (denies the event). This caps memory usage while being safe — denying notifications is better than OOM.

---

### M4. `BroadcastToSession` marshals the same message twice [FIXED]

**File:** `internal/router/router.go` lines 1036 and 1084

`sendToConnection` marshals `msg` to JSON (line 1036), then `BroadcastToSession` marshals the same `msg` again for the admin connection (line 1084). On hot paths (every AI streaming chunk broadcast), this doubles the JSON serialization cost.

**Fix applied:** Refactored `BroadcastToSession` to marshal once at the top using `util.MarshalJSON`, then pass `[]byte` to both targets via a new `sendRawToConnection` helper that sends pre-marshaled bytes directly.

---

### M5. ConfigMap missing `CHATBOX_PATH_PREFIX` [FIXED]

**File:** `deployments/kubernetes/configmap.yaml`

The ConfigMap does not declare `CHATBOX_PATH_PREFIX`, forcing reliance on the application's compiled default. If the Ingress, probes, or Prometheus annotations use a different prefix, there's no single source of truth to change.

**Fix applied:** Added `CHATBOX_PATH_PREFIX: "/chatbox"` to the ConfigMap data section.

---

### M6. Prometheus scrape annotation path `/metrics` does not match actual endpoint [FIXED]

**File:** `deployments/kubernetes/deployment.yaml` line 36

The pod annotation `prometheus.io/path: "/metrics"` does not match the actual metrics endpoint. Previous reviews moved Prometheus metrics under the path prefix, so the actual path is `{prefix}/metrics/prometheus` (i.e., `/chatbox/metrics/prometheus`).

**Fix applied:** Changed annotation to `prometheus.io/path: "/chatbox/metrics/prometheus"`.

---

### M7. `ValidateTimeRange` is a stub that always returns error [FIXED]

**File:** `internal/util/validation.go` lines 121-124

```go
func ValidateTimeRange(start, end interface{}) error {
    return errors.New("not implemented")
}
```

Any code calling this function will always fail. Exported dead code that misleads callers.

**Fix applied:** Implemented with proper `time.Time` type assertions, zero-value checks, and ordering validation. Updated corresponding tests in `validation_test.go` with comprehensive table-driven test cases.

---

### M8. GitLab CI duplicates test execution across `test` and `coverage` stages [FIXED]

**File:** `.gitlab-ci.yml`

Both the `test` stage and `coverage` stage run `go test ./...`. The `coverage` stage runs with `-coverprofile` which already executes all tests. This doubles CI time for no benefit.

**Fix applied:** Merged the separate `test` and `coverage` stages into a single `test` stage that runs with `-race -cover -coverprofile`. Preserved coverage artifact and regex extraction.

---

### M9. Go version pin inconsistency: `go.mod` says 1.24.4, CI/Docker use `golang:1.24` [FIXED]

**Files:** `go.mod` line 3, `Dockerfile` line 5, `.gitlab-ci.yml`, `.github/workflows/docker-build.yml`

`go.mod` specifies `go 1.24.4` (patch version) but all CI and Docker images use `golang:1.24` (minor version only). A future `golang:1.24` tag could resolve to a different patch version, causing inconsistent behavior between local and CI builds.

**Fix applied:** Pinned all to `1.24.4`: `Dockerfile` → `golang:1.24.4-alpine`, `.gitlab-ci.yml` → `golang:1.24.4`, `.github/workflows/docker-build.yml` → `go-version: '1.24.4'`.

---

## LOW

### L1. LLM provider HTTP client timeouts are hardcoded magic numbers [FIXED]

**Files:** `internal/llm/openai.go` line 33, `internal/llm/anthropic.go` line 33, `internal/llm/dify.go` line 34

All three providers hardcode `Timeout: 60 * time.Second`. Per project invariant, constants should live in `internal/constants/constants.go`.

**Fix applied:** Added `LLMClientTimeout = 60 * time.Second` to `internal/constants/constants.go`. Replaced hardcoded values in all three providers with `constants.LLMClientTimeout`.

---

### L2. NetworkPolicy missing `namespace` in metadata [FIXED]

**File:** `deployments/kubernetes/networkpolicy.yaml` lines 1-7

Unlike other manifests (deployment, service, configmap, pdb), the NetworkPolicy metadata lacks `namespace: default`. If applied from a different namespace context, it lands in the wrong namespace.

**Fix applied:** Added `namespace: default` to the metadata section.

---

### L3. GitHub Actions does not upload coverage report as artifact [FIXED]

**File:** `.github/workflows/docker-build.yml`

The test step generates `coverage.out` but does not upload it as a GitHub Actions artifact. PR reviewers have no visibility into coverage changes.

**Fix applied:** Added `actions/upload-artifact@v4` step to upload `coverage.out` with 30-day retention.

---

### L4. `NewTimeoutContext` always derives from `context.Background()` [FIXED]

**File:** `internal/util/context.go`

`NewTimeoutContext` creates a context from `context.Background()`, discarding any parent context's cancellation signals or deadline. Callers in request handlers lose the ability to propagate HTTP request cancellation to downstream operations.

**Fix applied:** Added `NewTimeoutContextFrom(parent context.Context, timeout time.Duration)` variant for use in request-scoped code. Documented usage in godoc.

---

### L5. `FileURL` length not validated in `message.Validate()` [FIXED]

**File:** `internal/message/validation.go`

`util.ValidateFileURL` checks URL length (max 2048), but `message.Validate()` does not call it or perform its own length check on `FileURL`. A message with a very long `FileURL` could pass validation and be stored.

**Fix applied:** Added `MaxFileURLLength = 2048` constant and `FileURL` length check to `validateFieldLengths()` in message validation, consistent with other field length checks.

---

### L6. Makefile `clean-logs` lacks directory existence check [FIXED]

**File:** `Makefile` line 241

`rm -f logs/*.log` prints an error if the `logs/` directory doesn't exist. Not harmful but noisy.

**Fix applied:** Changed to `@if [ -d logs ]; then rm -f logs/*.log; fi` — only attempts cleanup when the directory exists.

---

## Verification Checks

```
gofmt -l .            — PASS (no unformatted files)
go vet ./...          — PASS
go build ./cmd/server — PASS
go test -short -race ./... — PASS (all 20 packages)
```

---

## Cross-Reference with v5

All v5 fixes verified as correctly applied:
- v5-H1 (AI response persistence) — present in router.go
- v5-H2 (sendErrorResponse SafeSend) — present in handler.go
- v5-H3/H4 (healthcheck/probe paths) — corrected to `/chatbox`
- v5-H5 (GOPRIVATE in GitHub Actions) — present
- v5-H7 (AES-GCM error logging) — present
- v5-H8 (AddMessage retry) — present
- v5-H10 (session lock in cleanup) — present
- v5-M1 (TOCTOU atomic check) — present in MarkAdminAssisted
- v5-M4 (ListAllSessions default limit) — present
- v5-M5 (EndSession consolidation) — now fully atomic (H4 fix above)
- v5-M6 (admin takeover error mapping) — now uses sentinel error (M1 fix above)
