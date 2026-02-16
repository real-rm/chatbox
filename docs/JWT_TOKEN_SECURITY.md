# JWT Token Security

## Overview

This document describes the security implications of JWT token authentication in the Chatbox WebSocket Service, particularly when tokens are passed via URL query parameters. Understanding these risks is essential for secure deployment and operation.

## JWT Token Usage in the Application

The Chatbox WebSocket Service uses JWT tokens to authenticate WebSocket connections. The application supports two methods for providing authentication tokens:

1. **Query Parameter**: `wss://example.com/ws?token=<jwt_token>`
2. **Authorization Header**: `Authorization: Bearer <jwt_token>`

Both methods are functionally equivalent and provide the same level of authentication. However, they have significantly different security characteristics.

## Security Implications of Tokens in URL Query Parameters

When JWT tokens are passed as URL query parameters, they are exposed in multiple locations where they can be logged, leaked, or intercepted:

### 1. Web Server Access Logs

**Risk**: JWT tokens in URLs are logged by web servers (nginx, Apache, etc.) in their access logs.

**Impact**: Anyone with access to server logs can extract valid authentication tokens. This includes:
- System administrators
- Log aggregation systems
- Security monitoring tools
- Backup systems

**Example log entry**:
```
192.168.1.100 - - [01/Jan/2024:12:00:00 +0000] "GET /ws?token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9... HTTP/1.1" 101 0
```

### 2. Browser History

**Risk**: URLs with query parameters are stored in browser history.

**Impact**: Tokens remain accessible after the session ends:
- Anyone with physical access to the device can view tokens
- Browser sync features may replicate tokens across devices
- Tokens persist even after logout
- Browser history is often backed up and difficult to fully erase

### 3. HTTP Referer Headers

**Risk**: When users navigate from the WebSocket page to external sites, the full URL (including token) may be sent in the Referer header.

**Impact**: Third-party websites receive valid authentication tokens:
- External analytics services
- Advertising networks
- Any linked external resource
- Malicious sites linked from user-generated content

### 4. Proxy Server Logs

**Risk**: Corporate proxies, CDNs, and reverse proxies log full URLs including query parameters.

**Impact**: Tokens are exposed to infrastructure outside your direct control:
- Corporate IT departments
- CDN providers (Cloudflare, Akamai, etc.)
- ISP proxies
- VPN providers

### 5. Browser Developer Tools

**Risk**: URLs are visible in browser developer tools, network tabs, and debugging interfaces.

**Impact**: Tokens can be extracted during development, debugging, or by malicious browser extensions.

## Recommended Mitigations

### 1. Use Short-Lived Tokens for Query Parameters

If you must use query parameter authentication, issue tokens with very short expiration times:

**Recommended TTL**: 5-15 minutes

**Rationale**: Even if a token is logged or leaked, it will quickly become invalid, limiting the window of opportunity for misuse.

**Implementation**:
```go
// When generating tokens for WebSocket connections
token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
    "user_id": userID,
    "exp":     time.Now().Add(10 * time.Minute).Unix(), // 10-minute expiration
    "iat":     time.Now().Unix(),
})
```

### 2. Prefer Authorization Header Authentication

**Recommended**: Use the Authorization header method whenever possible.

**Advantages**:
- Headers are not logged in standard web server access logs
- Headers are not stored in browser history
- Headers are not sent in Referer headers
- Headers are not visible in URLs

**Client Implementation**:
```javascript
const ws = new WebSocket('wss://example.com/ws');
ws.addEventListener('open', () => {
    // Send token in first message or use subprotocol
    ws.send(JSON.stringify({
        type: 'auth',
        token: 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...'
    }));
});
```

**Note**: The WebSocket API does not support custom headers directly. Consider using:
- WebSocket subprotocols to pass the token
- Sending the token in the first message after connection
- Using a separate HTTP endpoint to exchange a short-lived token for query parameter use

### 3. Implement Token Rotation

Issue new tokens periodically and invalidate old ones:
- Reduces the impact of token leakage
- Limits the lifetime of compromised tokens
- Provides audit trail of token usage

### 4. Monitor for Token Misuse

Implement monitoring to detect suspicious token usage:
- Multiple connections from different IP addresses with the same token
- Connections from unexpected geographic locations
- High-frequency connection attempts
- Token usage after user logout

## WebSocket Endpoint Authentication Methods

The Chatbox WebSocket Service accepts authentication via both methods:

### Query Parameter Method
```
wss://example.com/ws?token=<jwt_token>
```

### Authorization Header Method
```
GET /ws HTTP/1.1
Host: example.com
Upgrade: websocket
Connection: Upgrade
Authorization: Bearer <jwt_token>
```

Both methods validate the token and extract the user ID for session management. Choose the method that best fits your security requirements and client capabilities.

## Monitoring and Detection Recommendations

### 1. Log Analysis

Monitor access logs for patterns indicating token leakage:
- Tokens used from multiple IP addresses
- Tokens used after expiration
- High volume of failed authentication attempts
- Unusual geographic distribution of connections

### 2. Token Lifecycle Tracking

Implement server-side tracking of token usage:
- Record token issuance time and user
- Log each token usage with IP address and timestamp
- Alert on anomalous usage patterns
- Maintain audit trail for security investigations

### 3. Rate Limiting

Apply rate limiting to WebSocket connection attempts:
- Limit connections per IP address
- Limit connections per user account
- Implement exponential backoff for failed attempts
- Block IP addresses with suspicious patterns

### 4. Alerting

Configure alerts for security events:
- Token reuse from different IP addresses within short time window
- Connections from blacklisted IP ranges
- Spike in connection attempts
- Token usage patterns inconsistent with normal user behavior

## Best Practices Summary

1. **Use Authorization headers** instead of query parameters when possible
2. **Issue short-lived tokens** (5-15 minutes) for query parameter authentication
3. **Implement token rotation** to limit exposure window
4. **Monitor token usage** for suspicious patterns
5. **Apply rate limiting** to prevent brute force attacks
6. **Educate users** about not sharing URLs containing tokens
7. **Sanitize logs** to remove tokens before long-term storage or sharing
8. **Use HTTPS/WSS** exclusively to prevent token interception in transit

## Additional Resources

- [RFC 6750: OAuth 2.0 Bearer Token Usage](https://tools.ietf.org/html/rfc6750)
- [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [JWT Best Practices](https://tools.ietf.org/html/rfc8725)

## Conclusion

While query parameter authentication is convenient and widely supported, it introduces significant security risks through logging and leakage. For production deployments, prefer Authorization header authentication or implement short-lived tokens with robust monitoring to mitigate these risks.
