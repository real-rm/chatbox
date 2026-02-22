# Multi-stage build for chatbox service
# NOTE: For production, pin images by digest (e.g., golang:1.24-alpine@sha256:<digest>)
# to ensure reproducible builds. Run: docker pull golang:1.24-alpine && docker inspect --format='{{index .RepoDigests 0}}'
# Stage 1: Build the Go application
FROM golang:1.24.4-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Private Go modules configuration
ENV GOPRIVATE=github.com/real-rm/*

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies using BuildKit secret mount (preferred).
# The token is mounted at /run/secrets/github_token and never stored in a layer.
# Build with: docker build --secret id=github_token,env=GITHUB_TOKEN .
# Or:         docker build --secret id=github_token,src=~/.github_token .
# Fallback:   docker build --build-arg GITHUB_TOKEN=<token> .  (LESS SECURE - token may persist in layer cache)
ARG GITHUB_TOKEN
RUN --mount=type=secret,id=github_token \
    if [ -s /run/secrets/github_token ]; then \
        GITHUB_TOKEN=$(cat /run/secrets/github_token) && \
        git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"; \
    elif [ -n "$GITHUB_TOKEN" ]; then \
        git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"; \
    fi && \
    go mod download && \
    git config --global --unset-all url."https://${GITHUB_TOKEN}@github.com/".insteadOf 2>/dev/null || true

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=0 for static binary
# -ldflags="-w -s" to reduce binary size
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s" \
    -o chatbox-server \
    ./cmd/server

# Stage 2: Create minimal runtime image
FROM alpine:3.21

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for security
RUN addgroup -g 1000 chatbox && \
    adduser -D -u 1000 -G chatbox chatbox

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/chatbox-server /app/chatbox-server

# Copy configuration template (will be overridden by ConfigMap)
COPY config.toml /app/config.toml

# Create directories for logs and temp files
RUN mkdir -p /app/logs /app/temp/uploads && \
    chown -R chatbox:chatbox /app

# Switch to non-root user
USER chatbox

# Expose WebSocket port
EXPOSE 8080

# Health check
# CHATBOX_PATH_PREFIX is set by config.toml (default: /chatbox).
# Do NOT bake a default here — config.toml is the single source of truth.
# Override at runtime: docker run -e CHATBOX_PATH_PREFIX=/chat ...
ENV CHATBOX_PATH_PREFIX=/chatbox
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080${CHATBOX_PATH_PREFIX}/healthz || exit 1

# Run the application
ENTRYPOINT ["/app/chatbox-server"]
