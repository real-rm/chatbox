# WebSocket Origin Validation

## Overview

The chatbox application implements proper origin validation for WebSocket connections to prevent Cross-Site Request Forgery (CSRF) and WebSocket hijacking attacks.

## Security Threat

Without origin validation, malicious websites could establish WebSocket connections to your server on behalf of authenticated users, potentially:
- Intercepting sensitive data
- Sending unauthorized messages
- Hijacking user sessions

## Implementation

### Configuration

Origin validation is configured via the `chatbox.allowed_origins` setting in `config.toml`:

```toml
[chatbox]
allowed_origins = "https://example.com,https://app.example.com"
```

**Important Notes:**
- Multiple origins are separated by commas
- Each origin must include the full protocol and domain (e.g., `https://example.com`)
- Subdomains must be explicitly listed (e.g., `https://sub.example.com`)
- Port numbers must match exactly if specified
- If `allowed_origins` is empty or not set, all origins are allowed (development mode only)

### Production Configuration

For production deployments, you **must** configure allowed origins:

```toml
[chatbox]
allowed_origins = "https://yourdomain.com,https://app.yourdomain.com"
```

### Development Configuration

For local development, you can allow localhost:

```toml
[chatbox]
allowed_origins = "http://localhost:3000,http://localhost:8080"
```

Or leave it empty to allow all origins (not recommended for production):

```toml
[chatbox]
allowed_origins = ""
```

## Behavior

### Allowed Origins
When a WebSocket connection request comes from an allowed origin:
1. The origin header is checked against the configured list
2. If matched, the connection is upgraded successfully
3. The client can establish a WebSocket connection

### Disallowed Origins
When a WebSocket connection request comes from a disallowed origin:
1. The origin header is checked against the configured list
2. If not matched, the upgrade is rejected
3. The client receives a `403 Forbidden` response
4. The attempt is logged with a warning

### No Configuration (Development Mode)
When no origins are configured:
1. All origins are allowed
2. A warning is logged on startup
3. This mode should **never** be used in production

## Testing

The origin validation feature includes comprehensive tests:

```bash
# Run origin validation tests
go test -v ./internal/websocket -run "TestHandler_CheckOrigin|TestHandler_SetAllowedOrigins|TestHandler_OriginValidationIntegration"
```

Test coverage includes:
- Exact origin matching
- Multiple allowed origins
- Subdomain handling
- Protocol (http vs https) validation
- Port number validation
- Empty origin handling
- Development mode (no restrictions)
- Integration testing with actual WebSocket upgrades

## Environment Variables

You can also configure allowed origins via environment variables (if using environment-based configuration):

```bash
export CHATBOX_ALLOWED_ORIGINS="https://example.com,https://app.example.com"
```

## Kubernetes Deployment

For Kubernetes deployments, configure origins in your ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: chat-config
data:
  config.toml: |
    [chatbox]
    allowed_origins = "https://yourdomain.com"
```

## Monitoring

Origin validation failures are logged with the following information:
- Origin that was rejected
- List of allowed origins
- Timestamp of the attempt

Monitor these logs to detect potential attack attempts:

```
level=WARN module=websocket msg=Origin not allowed, origin=https://evil.com, allowed_origins=[https://example.com]
```

## Best Practices

1. **Always configure origins in production** - Never deploy with empty `allowed_origins`
2. **Use HTTPS origins** - Ensure all production origins use HTTPS
3. **Be specific** - Only list origins that need access
4. **Include all subdomains** - If you have multiple subdomains, list them all
5. **Match ports exactly** - If using non-standard ports, include them in the origin
6. **Monitor logs** - Watch for rejected origin attempts
7. **Update on changes** - When adding new frontends, update the configuration

## Related Security Features

This origin validation works in conjunction with:
- JWT authentication (validates user identity)
- Rate limiting (prevents abuse)
- TLS/SSL termination (encrypts traffic)
- Connection limits (prevents resource exhaustion)

## References

- [OWASP WebSocket Security](https://owasp.org/www-community/vulnerabilities/WebSocket_Security)
- [RFC 6455 - The WebSocket Protocol](https://tools.ietf.org/html/rfc6455)
- [MDN - Origin Header](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Origin)
