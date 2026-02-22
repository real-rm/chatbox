# Production Readiness Review v5

**Date:** 2026-02-22
**Base Commit:** `f583f73` (main)
**Reviewers:** 3 parallel agents (Security & Concurrency, Architecture & Code Quality, Infrastructure & Testing)
**Previous:** v4 fixed 34 issues (4C, 10H, 12M, 8L)

---

## Summary

| Severity | Count | Description |
|----------|-------|-------------|
| ~~CRITICAL~~ | ~~2~~ 0 | ~~C1 FALSE POSITIVE (defer ordering is correct per LIFO); C2 is documentation-only~~ |
| HIGH | 10 | Missing AI response persistence, unsafe channel send, health check path mismatches, CI private module auth, k8s manifest gaps, storage error handling |
| MEDIUM | 19 | TOCTOU races, missing locks, unbounded queries, stdlib reinvention, CI improvements, Dockerfile/Makefile fixes |
| LOW | 11 | Minor lock safety, deprecated commands, stale images, missing health checks |
| **Total** | **40** (excluding 2 false positives) | |

---

## CRITICAL

### ~~C1. Defer ordering causes `recoverStreamPanic` to send to closed channel~~ FALSE POSITIVE

**Status:** FALSE POSITIVE -- All 3 agents independently got the defer LIFO analysis backwards.

In Go, defers execute in LIFO (last-in, first-out) order. `defer close(ch)` at line 190 is deferred FIRST, `defer recoverStreamPanic(ch, "x")` at line 191 is deferred SECOND. LIFO means the second-deferred runs first. So `recoverStreamPanic` runs FIRST (channel still open), then `close` runs SECOND. The existing test `TestStreamPanicRecovery` confirms this behavior works correctly. No fix needed.

### C2. `sess.UserID` read without session lock across router (latent race)

**File:** `internal/router/router.go` (lines 424, 449, 486, 556, 633, 723, 1157, 1224)

After `GetSession()` returns a `*Session` pointer, the code reads `sess.UserID` without `sess.mu.RLock()`. While `UserID` is set at creation and never mutated in current code, Go `string` is a multi-word struct, making this a latent data race if any future code mutates it.

**Fix:** Document `UserID` and `ID` as immutable-after-construction in the `Session` struct with a comment, making the contract explicit.

---

## HIGH

### H1. AI streaming response never stored in session history

**Files:** `internal/router/router.go:324-398`
**Found by:** 2 agents independently

`HandleUserMessage` persists the user message but after streaming the full LLM response, `fullContent` is never stored via `AddMessage` or `persistMessage`. Compare with `processVoiceMessageWithLLM` which correctly stores the AI response. Session history in MongoDB will be missing all AI streaming replies.

**Fix:** After the streaming loop, create a `session.Message` for the AI response and call `persistMessage`.

### H2. `sendErrorResponse` bypasses `SafeSend` -- can panic on closed channel

**File:** `internal/websocket/handler.go:563-580`

`sendErrorResponse` sends directly to `c.send` with a `select/default` guard but does not check `c.closing` first. If the connection is being torn down concurrently, this panics on send-to-closed-channel. The existing `SafeSend` method properly checks `c.closing.Load()`.

**Fix:** Replace the direct send in `sendErrorResponse` with `c.SafeSend(errorBytes)`.

### H3. docker-compose health check path `/chat/healthz` does not match default prefix `/chatbox`

**File:** `docker-compose.yml:149`

The health check polls `/chat/healthz` but the application uses `CHATBOX_PATH_PREFIX=/chatbox`. Docker will report the container as unhealthy.

**Fix:** Change health check path to `/chatbox/healthz`.

### H4. Kubernetes probe paths `/chat/healthz` and `/chat/readyz` do not match default prefix

**File:** `deployments/kubernetes/deployment.yaml:162,174,186`

All three K8s probes use `/chat/` prefix while the default is `/chatbox/`. Pods will enter crash loops.

**Fix:** Change all probe paths to use `/chatbox/healthz` and `/chatbox/readyz`.

### H5. GitLab CI and GitHub Actions lack GOPRIVATE and Git credentials for private modules

**Files:** `.gitlab-ci.yml:78-114`, `.github/workflows/docker-build.yml:14-43`
**Confirmed by:** GitHub CI failure on push (all `github.com/real-rm/*` modules fail to resolve)

Both CI pipelines run `go vet` and `go test` without configuring `GOPRIVATE=github.com/real-rm/*` or Git authentication.

**Fix:** Add `GOPRIVATE` env var and `git config --global url` setup for private module access.

### H6. `k8s-deploy` target omits `hpa.yaml`, `pdb.yaml`, and `networkpolicy.yaml`

**File:** `Makefile:179-183`

The deploy target only applies 4 of 7 manifests, skipping autoscaler, disruption budget, and network policy.

**Fix:** Add `kubectl apply` for the missing manifests; add corresponding deletes to `k8s-delete`.

### H7. `NewStorageService` silently swallows AES-GCM initialization errors

**File:** `internal/storage/storage.go:155-163`

If `aes.NewCipher` or `NewGCM` fails, the error is silently ignored and `svc.gcm` remains nil. The constructor returns successfully, giving the impression encryption is active.

**Fix:** Log a warning if AES-GCM initialization fails with a non-empty key, making the silent fallback visible.

### H8. `AddMessage` in Storage does not use retry logic for transient errors

**File:** `internal/storage/storage.go:564-583`

`CreateSession`, `UpdateSession`, and `GetSession` all use `retryOperation()` but `AddMessage` -- the most frequently called write -- performs a single `UpdateOne` without retry.

**Fix:** Wrap the `UpdateOne` in `retryOperation`.

### H9. Admin takeover creates leaking mock connection with no consumer

**File:** `chatbox.go:953-955`

`handleAdminTakeover` creates a `Connection` with a 256-capacity `send` channel but no `writePump` goroutine consuming it. Messages accumulate silently and the connection leaks.

**Fix:** Add a comment documenting this limitation. The admin takeover is HTTP-based (not WebSocket), so the connection serves only as a session marker. Add cleanup on session end.

### H10. `cleanupExpiredSessions` reads session fields without session lock

**File:** `internal/session/session.go:333-336`

Reads `session.IsActive` and `session.EndTime` while holding only `sm.mu`, not `session.mu`. Safe today because both paths hold `sm.mu.Lock()`, but fragile if cleanup is ever changed to use `RLock`.

**Fix:** Add `session.mu.RLock()` / `session.mu.RUnlock()` before reading session fields in cleanup loop.

---

## MEDIUM

### M1. TOCTOU race on admin assistance check in HandleAdminTakeover
**File:** `internal/router/router.go:1112-1148`
Between `GetAdminAssistance()` and `MarkAdminAssisted()`, another admin could race in.

### M2. `SetSessionNameFromMessage` reads/writes `session.Name` without session lock
**File:** `internal/session/session.go:375-394`

### M3. `HandleAIGeneratedFile`/`HandleAIVoiceResponse` lack session ownership checks
**File:** `internal/router/router.go:890-1010`

### M4. `ListAllSessions(limit=0)` performs unbounded MongoDB query
**File:** `internal/storage/storage.go:788-849`

### M5. `EndSession` performs two MongoDB round-trips that can diverge
**File:** `internal/storage/storage.go:588-635`
First `FindOneAndUpdate` sets `endTs`, second `UpdateOne` sets `dur`. If second fails, session has no duration.

### M6. `handleAdminTakeover` returns HTTP 500 for all errors including client errors
**File:** `chatbox.go:926-975`

### M7. `splitAndTrim` in notification.go reinvents `strings.Split` + `strings.TrimSpace`
**File:** `internal/notification/notification.go:423-456`

### M8. `trimWhitespace` in session.go reinvents `strings.TrimSpace`
**File:** `internal/session/session.go:455-500`

### M9. Dockerfile hardcodes `GOARCH=amd64` -- cannot build for ARM64
**File:** `Dockerfile:41`

### M10. `docker-compose.yml` uses deprecated `version: '3.8'` field
**File:** `docker-compose.yml:4`

### M11. PodDisruptionBudget missing `namespace` field
**File:** `deployments/kubernetes/pdb.yaml`

### M12. `make test` and `make test-unit` do not enable race detector
**File:** `Makefile:91-97`

### M13. GitLab CI `docker-build` uses `docker:24-dind` as both image and service
**File:** `.gitlab-ci.yml:28-30`

### M14. GitLab CI uses deprecated `only` keyword instead of `rules`
**File:** `.gitlab-ci.yml` (multiple jobs)

### M15. No Go module cache in either CI pipeline
**Files:** `.gitlab-ci.yml`, `.github/workflows/docker-build.yml`

### M16. Makefile references `./test_integration.sh` but script is at `./scripts/testing/test_integration.sh`
**File:** `Makefile:165`

### M17. `k8s-describe` and `k8s-restart` use wrong deployment name `chatbox` instead of `chatbox-websocket`
**File:** `Makefile:206,212-213`

### M18. GitHub Actions workflow has no coverage measurement
**File:** `.github/workflows/docker-build.yml`

### M19. No container image scanning in either CI pipeline
**Files:** `.gitlab-ci.yml`, `.github/workflows/docker-build.yml`

---

## LOW

### L1. `GetMemoryStats` reads `session.IsActive` without session lock
**File:** `internal/session/session.go:358-371`

### L2. Admin takeover can proceed on session ended between check and mark
**File:** `internal/router/router.go:1112-1129`

### L3. LLM wrapper goroutine has same defer-ordering issue as C1
**File:** `internal/llm/llm.go:427-428` (already covered by C1 fix)

### L4. `session.EndTime` pointer read without lock in cleanup (safe due to shared sm.mu)
**File:** `internal/session/session.go:334-335`

### L5. `mailhog` service in docker-compose has no health check
**File:** `docker-compose.yml:71-79`

### L6. MongoDB health check uses unauthenticated `mongosh`
**File:** `docker-compose.yml:24`

### L7. Makefile uses deprecated `docker-compose` (v1) command
**File:** `Makefile:149,155,159`

### L8. `docker-compose-test` silently swallows test failures
**File:** `Makefile:165`

### L9. `alpine:3.19` in Dockerfile runtime stage is past end-of-life
**File:** `Dockerfile:47`

### L10. `k8s-deploy` does not wait for rollout to complete
**File:** `Makefile:179-183`

### L11. GitLab CI coverage regex fragility
**File:** `.gitlab-ci.yml:147`

---

## Verification Plan

1. `go build ./cmd/server/...` -- must compile
2. `go vet ./...` -- must pass
3. `go test -short -race ./...` -- all packages must pass
4. Health check paths verified against `CHATBOX_PATH_PREFIX` default
5. K8s manifest names verified against deployment metadata
6. CI pipeline succeeds with GOPRIVATE configured
