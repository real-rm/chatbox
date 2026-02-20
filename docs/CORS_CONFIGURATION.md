# CORS Configuration Guide

This guide explains how to configure Cross-Origin Resource Sharing (CORS) and WebSocket origin validation for the chatbox application.

## Overview

The application provides two types of origin validation:

1. **CORS for HTTP Endpoints** - Controls access to REST APIs (admin endpoints, metrics)
2. **WebSocket Origin Validation** - Controls WebSocket connection establishment

Both are critical for production security.

## Configuration Options

### 1. CORS for HTTP Endpoints

**Configuration Key**: `cors_allowed_origins`

**Purpose**: Allows web applications from different origins to access HTTP endpoints like:
- Admin API (`/chat/admin/*`)
- Metrics endpoint (`/metrics`)
- Health checks (`/chat/healthz`, `/chat/readyz`)

**Format**: Comma-separated list of allowed origins

**Examples**:

```toml
# config.toml
[chatbox]
# Allow admin dashboard and monitoring tools
cors_allowed_origins = "https://admin.example.com,https://dashboard.example.com,https://grafana.example.com"
```

```yaml
# Kubernetes ConfigMap
CORS_ALLOWED_ORIGINS: "https://admin.example.com,https://dashboard.example.com"
```

```bash
# Environment variable
export CORS_ALLOWED_ORIGINS="https://admin.example.com,https://dashboard.example.com"
```

**Behavior**:
- **Empty value**: CORS middleware is NOT applied. Endpoints only accessible from same origin.
- **Configured**: Allows cross-origin requests from listed domains
- Automatically handles preflight OPTIONS requests
- Allows credentials (cookies, authorization headers)
- Preflight responses cached for 12 hours

**Allowed Methods**: GET, POST, PUT, PATCH, DELETE, OPTIONS

**Allowed Headers**: Origin, Content-Type, Accept, Authorization

### 2. WebSocket Origin Validation

**Configuration Key**: `allowed_origins`

**Purpose**: Validates the origin of WebSocket connection requests to prevent:
- Cross-Site WebSocket Hijacking (CSWSH)
- CSRF attacks via WebSocket
- Unauthorized access from malicious websites

**Format**: Comma-separated list of allowed origins

**Examples**:

```toml
# config.toml
[chatbox]
# Allow chat frontend from multiple domains
allowed_origins = "https://chat.example.com,https://app.example.com,https://mobile.example.com"
```

```yaml
# Kubernetes ConfigMap
WS_ALLOWED_ORIGINS: "https://chat.example.com,https://app.example.com"
```

```bash
# Environment variable
export WS_ALLOWED_ORIGINS="https://chat.example.com,https://app.example.com"
```

**Behavior**:
- **Empty value**: ALL origins allowed (INSECURE - development only)
- **Configured**: Only listed origins can establish WebSocket connections
- Invalid origins receive 403 Forbidden response
- Validation happens during WebSocket upgrade handshake

**Security Warning**: Never leave `allowed_origins` empty in production. This creates a serious security vulnerability allowing any website to connect to your WebSocket server.

## Environment-Specific Configuration

### Development Environment

```toml
[chatbox]
# Allow localhost for local development
allowed_origins = "http://localhost:3000,http://localhost:8080,http://127.0.0.1:3000"
cors_allowed_origins = "http://localhost:3000,http://localhost:8080"
```

### Staging Environment

```toml
[chatbox]
# Allow staging domains
allowed_origins = "https://chat-staging.example.com,https://app-staging.example.com"
cors_allowed_origins = "https://admin-staging.example.com,https://dashboard-staging.example.com"
```

### Production Environment

```toml
[chatbox]
# Only allow production domains
allowed_origins = "https://chat.example.com,https://app.example.com"
cors_allowed_origins = "https://admin.example.com,https://dashboard.example.com"
```

### Multi-Environment Setup

```toml
[chatbox]
# Support both staging and production
allowed_origins = "https://chat.example.com,https://chat-staging.example.com,https://app.example.com"
cors_allowed_origins = "https://admin.example.com,https://admin-staging.example.com"
```

## Testing CORS Configuration

### Test CORS Preflight Request

```bash
# Test OPTIONS preflight request
curl -X OPTIONS http://your-service:8080/chat/admin/sessions \
  -H "Origin: https://admin.example.com" \
  -H "Access-Control-Request-Method: GET" \
  -H "Access-Control-Request-Headers: Authorization" \
  -v

# Expected response headers:
# HTTP/1.1 204 No Content
# Access-Control-Allow-Origin: https://admin.example.com
# Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
# Access-Control-Allow-Headers: Origin, Content-Type, Accept, Authorization
# Access-Control-Allow-Credentials: true
# Access-Control-Max-Age: 43200
```

### Test CORS Actual Request

```bash
# Test actual GET request with CORS
curl -X GET http://your-service:8080/chat/admin/sessions \
  -H "Origin: https://admin.example.com" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -v

# Expected response headers:
# Access-Control-Allow-Origin: https://admin.example.com
# Access-Control-Allow-Credentials: true
```

### Test WebSocket Origin Validation

```bash
# Install wscat if not already installed
npm install -g wscat

# Test with allowed origin
wscat -c ws://your-service:8080/chat/ws?token=YOUR_JWT_TOKEN \
  --origin https://chat.example.com

# Expected: Connection established successfully

# Test with disallowed origin
wscat -c ws://your-service:8080/chat/ws?token=YOUR_JWT_TOKEN \
  --origin https://malicious-site.com

# Expected: HTTP 403 Forbidden
```

### Check Application Logs

```bash
# Check if CORS is configured
kubectl logs -l app=chatbox | grep CORS

# Expected output when configured:
# INFO CORS middleware configured allowed_origins=[https://admin.example.com https://dashboard.example.com] allow_credentials=true

# Expected output when not configured:
# WARN No CORS origins configured, CORS middleware not enabled
```

## Common Use Cases

### Use Case 1: Admin Dashboard on Different Domain

**Scenario**: Admin dashboard hosted at `https://admin.example.com` needs to access API at `https://api.example.com`

**Configuration**:
```toml
[chatbox]
cors_allowed_origins = "https://admin.example.com"
```

### Use Case 2: Multiple Frontend Applications

**Scenario**: Chat available on web, mobile web, and embedded widget

**Configuration**:
```toml
[chatbox]
allowed_origins = "https://chat.example.com,https://m.example.com,https://widget.example.com"
```

### Use Case 3: Monitoring Tools

**Scenario**: Grafana and Prometheus UI need to access metrics endpoint

**Configuration**:
```toml
[chatbox]
cors_allowed_origins = "https://grafana.example.com,https://prometheus.example.com"
```

### Use Case 4: Development with Multiple Ports

**Scenario**: Frontend dev server on port 3000, backend on port 8080

**Configuration**:
```toml
[chatbox]
allowed_origins = "http://localhost:3000,http://127.0.0.1:3000"
cors_allowed_origins = "http://localhost:3000,http://127.0.0.1:3000"
```

## Security Best Practices

### 1. Always Configure in Production

Never deploy to production with empty `allowed_origins`. This allows any website to connect to your WebSocket server.

❌ **Bad**:
```toml
allowed_origins = ""  # Allows all origins - INSECURE
```

✅ **Good**:
```toml
allowed_origins = "https://chat.example.com,https://app.example.com"
```

### 2. Use HTTPS in Production

Always use HTTPS origins in production. HTTP origins are vulnerable to man-in-the-middle attacks.

❌ **Bad**:
```toml
allowed_origins = "http://chat.example.com"  # Insecure HTTP
```

✅ **Good**:
```toml
allowed_origins = "https://chat.example.com"  # Secure HTTPS
```

### 3. Be Specific with Origins

Don't use wildcards or overly broad patterns. List specific origins.

❌ **Bad**:
```toml
allowed_origins = "*.example.com"  # Not supported, won't work
```

✅ **Good**:
```toml
allowed_origins = "https://chat.example.com,https://app.example.com"
```

### 4. Separate Development and Production

Use different configurations for development and production environments.

```toml
# Development
[chatbox]
allowed_origins = "http://localhost:3000"

# Production
[chatbox]
allowed_origins = "https://chat.example.com"
```

### 5. Review Regularly

Periodically review and update allowed origins as your infrastructure changes.

## Troubleshooting

### CORS Preflight Fails

**Symptom**: Browser shows CORS error in console

**Check**:
1. Verify `cors_allowed_origins` is configured
2. Ensure origin matches exactly (including protocol and port)
3. Check application logs for CORS configuration

**Solution**:
```bash
# Add the origin to configuration
CORS_ALLOWED_ORIGINS: "https://your-frontend-domain.com"
```

### WebSocket Connection Refused

**Symptom**: WebSocket connection fails with 403 Forbidden

**Check**:
1. Verify `allowed_origins` is configured
2. Ensure origin matches exactly
3. Check WebSocket upgrade request headers

**Solution**:
```bash
# Add the origin to configuration
WS_ALLOWED_ORIGINS: "https://your-chat-domain.com"
```

### CORS Headers Not Present

**Symptom**: No CORS headers in response

**Check**:
1. Verify `cors_allowed_origins` is not empty
2. Check application logs for "CORS middleware configured"

**Solution**:
```bash
# Ensure CORS is configured
CORS_ALLOWED_ORIGINS: "https://your-domain.com"

# Restart application
kubectl rollout restart deployment chatbox-websocket
```

### Origin Mismatch

**Symptom**: CORS works for some requests but not others

**Check**:
1. Verify origin includes protocol (http:// or https://)
2. Check if port is included when non-standard
3. Ensure no trailing slashes

**Examples**:
- ✅ `https://example.com`
- ✅ `https://example.com:8443`
- ❌ `example.com` (missing protocol)
- ❌ `https://example.com/` (trailing slash)

## Related Documentation

- [Kubernetes Deployment Guide](../deployments/kubernetes/README.md)
- [Security Best Practices](DEPLOYMENT.md#security)
- [WebSocket Configuration](../README.md#websocket-configuration)

## References

- [CORS Specification](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS)
- [WebSocket Security](https://owasp.org/www-community/attacks/WebSocket_hijacking)
- [gin-contrib/cors Documentation](https://github.com/gin-contrib/cors)
