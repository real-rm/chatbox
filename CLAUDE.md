# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
make build                    # Produces ./bin/chatbox-server
go build -o ./bin/chatbox-server ./cmd/server/main.go

# Run
make run                      # go run ./cmd/server/main.go
make run-dev                  # Hot reload via air

# Test
make test                     # All tests, 2m timeout
make test-unit                # go test -short ./... (skips integration)
make test-integration         # -run TestIntegration
make test-property            # -run Property (gopter property-based tests)
make test-coverage            # Generates coverage.out + coverage.html
go test -v ./internal/storage # Single package
go test -v -run TestFoo ./internal/websocket  # Single test

# Lint / Format
make lint                     # gofmt -w -s . && go vet ./...
make vet                      # go vet ./...
make fmt                      # gofmt -w -s .

# Docker
make docker-compose-up        # Starts MongoDB + MinIO + MailHog + Chatbox
make docker-compose-down
```

## Architecture

This service is a **library consumed by a `gomain` host process** — it does not have its own HTTP server. The entry point is `chatbox.go:Register(r *gin.Engine, config, logger, mongo)`, which wires up all routes onto the provided Gin engine. `Shutdown(ctx)` handles graceful cleanup. `cmd/server/main.go` is a standalone server for direct execution.

### Request Flow

```
Client → WebSocket (gorilla/websocket)
  └─ Handler (internal/websocket/handler.go)  - JWT auth, upgrade, connection lifecycle
       └─ MessageRouter (internal/router/router.go) - routes by message type
            ├─ LLMService (internal/llm/) - OpenAI / Anthropic / Dify streaming
            ├─ UploadService (internal/upload/) - via goupload → S3
            ├─ SessionManager (internal/session/) - in-memory active sessions
            ├─ StorageService (internal/storage/) - MongoDB persistence + AES-256 encryption
            └─ NotificationService (internal/notification/) - email/SMS alerts
```

### Internal Packages

| Package | Role |
|---|---|
| `internal/auth` | JWT validation (`JWTValidator`, `Claims`) |
| `internal/constants` | All constants (no magic numbers/strings anywhere else) |
| `internal/errors` | Typed domain errors |
| `internal/httperrors` | Standardized HTTP error responses |
| `internal/llm` | Provider interface + OpenAI/Anthropic/Dify implementations |
| `internal/message` | Message types and validation |
| `internal/metrics` | Prometheus metrics collection |
| `internal/notification` | Email (gomail/SES/SMTP) + SMS (gosms/Twilio) |
| `internal/ratelimit` | Sliding-window rate limiter with background cleanup |
| `internal/router` | Core message routing logic |
| `internal/session` | In-memory session store with TTL cleanup goroutine |
| `internal/storage` | MongoDB CRUD + AES-256-GCM message encryption |
| `internal/upload` | File upload tracking on top of goupload |
| `internal/util` | Shared helpers: context timeouts, JWT extraction, JSON, logging, validation |
| `internal/websocket` | WebSocket upgrade, `Connection` struct, ping/pong, message framing |

### Private Modules (`github.com/real-rm/*`)

The `real-rm` packages are private Go modules:
- `goconfig` — TOML + environment config accessor
- `golog` — Structured logger (writes to `logs/info.log`, `warn.log`, `error.log`)
- `gomongo` — MongoDB wrapper (used via `mongo.Coll("db", "collection")`)
- `goupload` — S3-compatible file upload abstraction
- `gohelper`, `gomail`, `gosms` — Utility, mail, SMS

### Routes (all under configurable `CHATBOX_PATH_PREFIX`, default `/chatbox`)

```
GET  {prefix}/ws               WebSocket endpoint (JWT via query param or header)
GET  {prefix}/sessions         User's own sessions (JWT required)
GET  {prefix}/admin/sessions   List all sessions with filters (admin JWT)
GET  {prefix}/admin/metrics    Session metrics (admin JWT)
POST {prefix}/admin/takeover/:sessionID  Admin takeover (admin JWT)
GET  {prefix}/healthz          Liveness probe
GET  {prefix}/readyz            Readiness probe (pings MongoDB)
GET  /metrics                  Prometheus (public)
```

## Configuration

Configuration is read from `config.toml` (loaded by `goconfig.LoadConfig()`) with environment variable overrides. Secrets marked `PLACEHOLDER_*` in `config.toml` must be provided via environment or Kubernetes secrets.

Critical at startup:
- `JWT_SECRET` — min 32 chars, weak-pattern checked
- `ENCRYPTION_KEY` — must be exactly 32 bytes for AES-256 (or empty to disable)
- `CHATBOX_PATH_PREFIX` — must start with `/`

LLM providers configured as numbered env vars: `LLM_PROVIDER_1_ID`, `LLM_PROVIDER_1_TYPE`, `LLM_PROVIDER_1_API_KEY`, etc. Supported types: `openai`, `anthropic`, `dify`.

## Testing Patterns

- **Table-driven tests** are standard throughout
- **Property-based tests** use `github.com/leanovate/gopter` (files named `*_property_test.go`)
- **Integration tests** are tagged with `TestIntegration` prefix
- `internal/testutil/` and `internal/storage/test_setup.go` provide shared test helpers
- Tests that require MongoDB use real connections — need MongoDB running (use `make docker-compose-up`)
- The `-short` flag skips integration tests in unit test mode

## Key Invariants

- **All constants** live in `internal/constants/constants.go` — never hardcode values elsewhere
- **All HTTP errors** go through `internal/httperrors` — never use `c.JSON(4xx, ...)` directly
- **All shared utilities** live in `internal/util/` — context helpers, bearer token extraction, JSON, validation
- **Session cleanup** and **rate limiter cleanup** run as background goroutines; call `Stop*()` on shutdown
- **Message encryption** is AES-256-GCM applied per-field in `StorageService` before MongoDB writes
- The `websocket.Connection` struct requires mutex protection — use provided methods, not direct field access for `SessionID`

## Project Rules

### MongoDB Conventions
- **Field names**: `camelCase` in BSON documents (e.g., `userId`, `startTime`, `isActive`)
- **Collection names**: `snake_case` (e.g., `chat_sessions`, `file_stats`)

### DRY
Before writing new code, check `internal/util/` and the `github.com/real-rm/*` packages first. Extract any repeated logic into `internal/util/` rather than duplicating it across packages.

### TDD
Write the test first, run it to confirm it fails, then implement. Target 80%+ coverage. Use table-driven tests and `*_property_test.go` for invariant validation with gopter.

### Prefer `github.com/real-rm/*` packages
Use company packages over third-party or hand-rolled alternatives:
- `goconfig` — config loading (TOML + env)
- `golog` — structured logging
- `gomongo` — MongoDB access
- `goupload` — S3 file uploads
- `gomail` — email (SES + SMTP)
- `gosms` — SMS (Twilio)
- `gohelper` — general utilities

API docs for these packages are in the `goapidocs` directory.
