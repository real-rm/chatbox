# Production Readiness Review v8

**Date:** 2026-02-22  
**Base Commit:** `9408841` (`main`)  
**Reviewer:** Codex (single-agent review)  
**Status:** NOT READY for production rollout

---

## Verification Performed

```bash
go test ./...   # PASS
go vet ./...    # PASS
```

---

## Summary

| Severity | Count | Description |
|----------|-------|-------------|
| CRITICAL | 0 | None found |
| HIGH | 3 | Frontend/API contract drift, mutable production image tag, deployment docs contain invalid operational instructions |
| MEDIUM | 2 | GitHub Actions coverage gate missing, GitLab CI uses unpinned coverage converter |
| LOW | 0 | None listed |
| **Total** | **5** | |

---

## HIGH

### H1. Frontend clients are incompatible with current backend routes and payload contracts

**Files:** `web/chat.js:124`, `web/sessions.js:37`, `web/admin.js:79`, `web/admin.js:118`, `web/admin.js:130`, `web/admin.js:157`, `web/admin.js:169`, `web/admin.js:223`, `chatbox.go:115`, `chatbox.go:430`, `chatbox.go:437`, `chatbox.go:739`, `chatbox.go:852`, `chatbox.go:920`, `internal/storage/storage.go:95`, `internal/storage/storage.go:128`

The frontend still hardcodes `/chat/...` endpoints, while backend routes are registered under configurable `CHATBOX_PATH_PREFIX` with default `/chatbox`.  
Additional contract mismatches:
- `web/admin.js` calls `/chat/admin/users/:userID/sessions`, but backend only exposes `/admin/sessions` with query filters.
- Admin session filters send `start_time`/`end_time`, but backend expects `start_time_from`/`start_time_to`.
- Admin sessions API response shape is `{ "sessions": [...] }`, but frontend treats root as an array.
- Backend session/metrics structs (`SessionMetadata`, `Metrics`) have no JSON tags and serialize as `ID`, `StartTime`, `TotalSessions`, etc., while frontend expects snake_case fields (`session_id`, `start_time`, `total_sessions`).

**Impact:** chat/admin UI flows break in default deployment; admin dashboard rendering and filtering are unreliable or empty.

**Recommendation:** introduce a single versioned API contract (OpenAPI or typed DTOs), align JSON tags with frontend expectations, and add end-to-end tests for `web/*.js` against real handlers.

---

### H2. Kubernetes deployment uses mutable runtime image (`:latest`) with forced pull

**File:** `deployments/kubernetes/deployment.yaml:73`
**File:** `deployments/kubernetes/deployment.yaml:74`

`image: chatbox-websocket:latest` with `imagePullPolicy: Always` creates non-deterministic runtime behavior. Any pod restart can pull a different artifact under the same tag.

**Impact:** rollback predictability and incident forensics degrade; production behavior can change without manifest change.

**Recommendation:** deploy immutable image references (tagged release + digest), and pin pull behavior to immutable artifacts.

---

### H3. Kubernetes and secret-management docs still prescribe invalid key generation and stale endpoints

**Files:** `deployments/kubernetes/README.md:59`, `deployments/kubernetes/README.md:60`, `deployments/kubernetes/README.md:165`, `deployments/kubernetes/README.md:169`, `deployments/kubernetes/README.md:598`, `deployments/kubernetes/README.md:601`, `docs/SECRET_SETUP_QUICKSTART.md:23`, `docs/SECRET_SETUP_QUICKSTART.md:26`, `docs/SECRET_SETUP_QUICKSTART.md:52`, `docs/SECRET_SETUP_QUICKSTART.md:53`, `docs/SECRET_MANAGEMENT.md:48`, `docs/SECRET_MANAGEMENT.md:64`, `docs/KEY_MANAGEMENT.md:47`, `README.md:36`, `README.md:37`, `chatbox.go:537`, `chatbox.go:542`

Current docs still instruct `openssl rand -base64 32` for `ENCRYPTION_KEY`, but runtime strictly requires exactly 32 bytes. They also still reference `/chat/...` endpoints in several operational steps, while default routing is `/chatbox/...`.

**Impact:** following official docs can cause startup failures (invalid key length) and false-negative deployment checks (health/WebSocket commands against wrong endpoints).

**Recommendation:** perform a documentation sweep and block merges on doc lint checks for known-invalid patterns (`/chat/healthz`, `/chat/ws`, `ENCRYPTION_KEY=.*base64 32`).

---

## MEDIUM

### M1. GitHub Actions test workflow has no enforceable coverage threshold

**File:** `.github/workflows/docker-build.yml:58`
**File:** `.github/workflows/docker-build.yml:62`
**File:** `.github/workflows/docker-build.yml:64`

The workflow computes and uploads coverage but never fails if coverage regresses below project policy.

**Impact:** coverage can drop silently in GitHub-based PR flow.

**Recommendation:** add a threshold gate (consistent with policy) before artifact upload.

---

### M2. GitLab CI installs coverage converter from mutable `@latest`

**File:** `.gitlab-ci.yml:124`

`go install github.com/boumenot/gocover-cobertura@latest` is non-reproducible and can break CI unexpectedly on upstream releases.

**Impact:** flaky or drifting CI behavior without repository changes.

**Recommendation:** pin an explicit version tag for `gocover-cobertura`.

---

## Residual Risk / Test Gaps

- Current Go test suite is strong for backend behavior, but there is no automated UI/API contract validation for `web/admin.js`, `web/chat.js`, and `web/sessions.js`.
- No deployment smoke test currently asserts doc commands against the configured path prefix.

---

## Recommended Order of Fixes

1. Fix H1 (frontend/backend contract and route parity) to restore operational admin/chat functionality.
2. Fix H2 (immutable image references) before the next production release.
3. Fix H3 docs drift to prevent operator-induced outages.
4. Add CI hardening for M1 and M2 to prevent regression.
