# Multi-stage build for chatbox service
# Stage 1: Build the Go application
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=0 for static binary
# -ldflags="-w -s" to reduce binary size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o chatbox-server \
    ./cmd/server

# Stage 2: Create minimal runtime image
FROM alpine:3.19

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
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/chat/healthz || exit 1

# Run the application
ENTRYPOINT ["/app/chatbox-server"]
