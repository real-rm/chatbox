# Nginx Configuration Guide for Chatbox WebSocket Service

This guide provides comprehensive Nginx configuration examples for deploying the Chatbox WebSocket service in various scenarios.

## Table of Contents

1. [Overview](#overview)
2. [Basic Reverse Proxy Configuration](#basic-reverse-proxy-configuration)
3. [WebSocket Upgrade Configuration](#websocket-upgrade-configuration)
4. [SSL/TLS Configuration](#ssltls-configuration)
5. [Load Balancing Configuration](#load-balancing-configuration)
6. [Health Check Configuration](#health-check-configuration)
7. [Rate Limiting Configuration](#rate-limiting-configuration)
8. [Security Headers Configuration](#security-headers-configuration)
9. [Complete Production Example](#complete-production-example)
10. [Troubleshooting](#troubleshooting)

## Overview

The Chatbox WebSocket service requires special Nginx configuration to properly handle:
- WebSocket connections with upgrade headers
- Long-lived connections with appropriate timeouts
- SSL/TLS termination
- Load balancing across multiple backend instances
- Security headers and rate limiting

**Default Configuration:**
- Default path prefix: `/chatbox`
- WebSocket endpoint: `/chatbox/ws`
- Health check: `/chatbox/healthz`
- Readiness check: `/chatbox/readyz`
- Admin endpoints: `/chatbox/admin/*`

**Note:** The path prefix is configurable via `CHATBOX_PATH_PREFIX` environment variable. Adjust the Nginx configuration accordingly if using a custom prefix.

## Basic Reverse Proxy Configuration

Basic configuration for proxying HTTP requests to the Chatbox backend:

```nginx
upstream chatbox_backend {
    server localhost:8080;
}

server {
    listen 80;
    server_name chat.example.com;

    # Proxy all chatbox requests
    location /chatbox/ {
        proxy_pass http://chatbox_backend;
        
        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

**Key Points:**
- `proxy_pass` forwards requests to the backend service
- `proxy_set_header` preserves client information
- Timeouts prevent hung connections

## WebSocket Upgrade Configuration

WebSocket connections require special upgrade headers and longer timeouts:

```nginx
upstream chatbox_backend {
    server localhost:8080;
    
    # Enable keepalive connections to backend
    keepalive 32;
}

server {
    listen 80;
    server_name chat.example.com;

    # WebSocket endpoint
    location /chatbox/ws {
        proxy_pass http://chatbox_backend;
        
        # WebSocket upgrade headers (REQUIRED)
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket timeouts (long-lived connections)
        proxy_connect_timeout 7d;
        proxy_send_timeout 7d;
        proxy_read_timeout 7d;
        
        # Disable buffering for WebSocket
        proxy_buffering off;
    }
    
    # HTTP endpoints (health checks, admin API)
    location /chatbox/ {
        proxy_pass http://chatbox_backend;
        
        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Standard timeouts for HTTP
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

**Key Points:**
- `proxy_http_version 1.1` required for WebSocket
- `Upgrade` and `Connection` headers enable WebSocket protocol
- Long timeouts (7 days) for persistent WebSocket connections
- `proxy_buffering off` prevents buffering of WebSocket frames
- Separate location blocks for WebSocket and HTTP endpoints

## SSL/TLS Configuration

Production deployments should use SSL/TLS encryption:

### Self-Signed Certificate (Development)

```bash
# Generate self-signed certificate
sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout /etc/nginx/ssl/chatbox.key \
    -out /etc/nginx/ssl/chatbox.crt \
    -subj "/CN=chat.example.com"
```

### Let's Encrypt Certificate (Production)

```bash
# Install certbot
sudo apt-get install certbot python3-certbot-nginx

# Obtain certificate
sudo certbot --nginx -d chat.example.com
```

### Nginx SSL Configuration

```nginx
upstream chatbox_backend {
    server localhost:8080;
    keepalive 32;
}

server {
    listen 80;
    server_name chat.example.com;
    
    # Redirect HTTP to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name chat.example.com;
    
    # SSL certificate configuration
    ssl_certificate /etc/letsencrypt/live/chat.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/chat.example.com/privkey.pem;
    
    # SSL protocols and ciphers (Mozilla Intermediate configuration)
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384';
    ssl_prefer_server_ciphers off;
    
    # SSL session cache
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    
    # OCSP stapling
    ssl_stapling on;
    ssl_stapling_verify on;
    ssl_trusted_certificate /etc/letsencrypt/live/chat.example.com/chain.pem;
    
    # WebSocket endpoint
    location /chatbox/ws {
        proxy_pass http://chatbox_backend;
        
        # WebSocket upgrade headers
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket timeouts
        proxy_connect_timeout 7d;
        proxy_send_timeout 7d;
        proxy_read_timeout 7d;
        proxy_buffering off;
    }
    
    # HTTP endpoints
    location /chatbox/ {
        proxy_pass http://chatbox_backend;
        
        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Standard timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

**Key Points:**
- Redirect HTTP (port 80) to HTTPS (port 443)
- Use TLSv1.2 and TLSv1.3 protocols
- Enable OCSP stapling for better performance
- SSL session cache reduces handshake overhead

## Load Balancing Configuration

Distribute traffic across multiple backend instances:

### Round Robin (Default)

```nginx
upstream chatbox_backend {
    # Round-robin load balancing (default)
    server backend1.example.com:8080;
    server backend2.example.com:8080;
    server backend3.example.com:8080;
    
    # Keepalive connections
    keepalive 32;
}
```

### IP Hash (Session Affinity)

**RECOMMENDED for WebSocket connections:**

```nginx
upstream chatbox_backend {
    # IP hash ensures same client goes to same backend
    ip_hash;
    
    server backend1.example.com:8080;
    server backend2.example.com:8080;
    server backend3.example.com:8080;
    
    keepalive 32;
}
```

### Least Connections

```nginx
upstream chatbox_backend {
    # Route to backend with fewest active connections
    least_conn;
    
    server backend1.example.com:8080;
    server backend2.example.com:8080;
    server backend3.example.com:8080;
    
    keepalive 32;
}
```

### Weighted Load Balancing

```nginx
upstream chatbox_backend {
    ip_hash;
    
    # Weight determines proportion of traffic
    server backend1.example.com:8080 weight=3;  # 60% of traffic
    server backend2.example.com:8080 weight=1;  # 20% of traffic
    server backend3.example.com:8080 weight=1;  # 20% of traffic
    
    keepalive 32;
}
```

### Backup Servers

```nginx
upstream chatbox_backend {
    ip_hash;
    
    server backend1.example.com:8080;
    server backend2.example.com:8080;
    
    # Backup server only used when primary servers are down
    server backup.example.com:8080 backup;
    
    keepalive 32;
}
```

**Key Points:**
- Use `ip_hash` for WebSocket to maintain session affinity
- `keepalive` improves performance by reusing connections
- `weight` parameter controls traffic distribution
- `backup` servers provide failover capability

## Health Check Configuration

Monitor backend health and automatically remove unhealthy instances:

### Passive Health Checks (Open Source Nginx)

```nginx
upstream chatbox_backend {
    ip_hash;
    
    server backend1.example.com:8080 max_fails=3 fail_timeout=30s;
    server backend2.example.com:8080 max_fails=3 fail_timeout=30s;
    server backend3.example.com:8080 max_fails=3 fail_timeout=30s;
    
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name chat.example.com;
    
    # ... SSL configuration ...
    
    # Health check endpoint
    location /chatbox/healthz {
        proxy_pass http://chatbox_backend;
        
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        
        # Short timeout for health checks
        proxy_connect_timeout 5s;
        proxy_send_timeout 5s;
        proxy_read_timeout 5s;
        
        # Don't log health checks
        access_log off;
    }
    
    # Readiness check endpoint
    location /chatbox/readyz {
        proxy_pass http://chatbox_backend;
        
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        
        proxy_connect_timeout 5s;
        proxy_send_timeout 5s;
        proxy_read_timeout 5s;
        
        access_log off;
    }
    
    # ... other locations ...
}
```

**Parameters:**
- `max_fails=3`: Mark server as down after 3 failed attempts
- `fail_timeout=30s`: Keep server marked as down for 30 seconds

### Active Health Checks (Nginx Plus)

```nginx
upstream chatbox_backend {
    zone chatbox 64k;
    
    server backend1.example.com:8080;
    server backend2.example.com:8080;
    server backend3.example.com:8080;
}

server {
    listen 443 ssl http2;
    server_name chat.example.com;
    
    # ... SSL configuration ...
    
    location /chatbox/ {
        proxy_pass http://chatbox_backend;
        
        # Active health check
        health_check interval=10s fails=3 passes=2 uri=/chatbox/healthz;
        
        # ... proxy headers and settings ...
    }
}
```

**Parameters:**
- `interval=10s`: Check every 10 seconds
- `fails=3`: Mark unhealthy after 3 failures
- `passes=2`: Mark healthy after 2 successes
- `uri=/chatbox/healthz`: Health check endpoint

## Rate Limiting Configuration

Protect the service from abuse and DDoS attacks:

### Basic Rate Limiting

```nginx
# Define rate limit zones
limit_req_zone $binary_remote_addr zone=chatbox_general:10m rate=10r/s;
limit_req_zone $binary_remote_addr zone=chatbox_ws:10m rate=5r/s;
limit_req_zone $binary_remote_addr zone=chatbox_admin:10m rate=2r/s;

server {
    listen 443 ssl http2;
    server_name chat.example.com;
    
    # ... SSL configuration ...
    
    # WebSocket endpoint - stricter limit
    location /chatbox/ws {
        limit_req zone=chatbox_ws burst=10 nodelay;
        
        proxy_pass http://chatbox_backend;
        
        # ... WebSocket configuration ...
    }
    
    # Admin endpoints - very strict limit
    location /chatbox/admin/ {
        limit_req zone=chatbox_admin burst=5 nodelay;
        
        proxy_pass http://chatbox_backend;
        
        # ... proxy configuration ...
    }
    
    # General endpoints
    location /chatbox/ {
        limit_req zone=chatbox_general burst=20 nodelay;
        
        proxy_pass http://chatbox_backend;
        
        # ... proxy configuration ...
    }
}
```

**Parameters:**
- `rate=10r/s`: Allow 10 requests per second
- `burst=20`: Allow bursts up to 20 requests
- `nodelay`: Process burst requests immediately

### Connection Limiting

```nginx
# Limit concurrent connections per IP
limit_conn_zone $binary_remote_addr zone=chatbox_conn:10m;

server {
    listen 443 ssl http2;
    server_name chat.example.com;
    
    # Limit to 10 concurrent connections per IP
    limit_conn chatbox_conn 10;
    
    # ... rest of configuration ...
}
```

### Custom Error Pages for Rate Limiting

```nginx
server {
    listen 443 ssl http2;
    server_name chat.example.com;
    
    # Custom error page for rate limiting
    error_page 429 /429.html;
    location = /429.html {
        root /usr/share/nginx/html;
        internal;
    }
    
    # ... rest of configuration ...
}
```

**Key Points:**
- Different rate limits for different endpoints
- `$binary_remote_addr` uses less memory than `$remote_addr`
- `burst` allows temporary spikes in traffic
- Connection limiting prevents resource exhaustion

## Security Headers Configuration

Add security headers to protect against common attacks:

```nginx
server {
    listen 443 ssl http2;
    server_name chat.example.com;
    
    # ... SSL configuration ...
    
    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;
    
    # Content Security Policy
    add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' wss: https:; font-src 'self'; object-src 'none'; frame-ancestors 'self';" always;
    
    # HSTS (HTTP Strict Transport Security)
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;
    
    # ... location blocks ...
}
```

**Security Headers Explained:**
- `X-Frame-Options`: Prevents clickjacking attacks
- `X-Content-Type-Options`: Prevents MIME type sniffing
- `X-XSS-Protection`: Enables browser XSS protection
- `Referrer-Policy`: Controls referrer information
- `Permissions-Policy`: Controls browser features
- `Content-Security-Policy`: Restricts resource loading
- `Strict-Transport-Security`: Forces HTTPS connections

## Complete Production Example

Complete production-ready configuration with all features:

```nginx
# Rate limiting zones
limit_req_zone $binary_remote_addr zone=chatbox_general:10m rate=10r/s;
limit_req_zone $binary_remote_addr zone=chatbox_ws:10m rate=5r/s;
limit_req_zone $binary_remote_addr zone=chatbox_admin:10m rate=2r/s;
limit_conn_zone $binary_remote_addr zone=chatbox_conn:10m;

# Backend servers with health checks
upstream chatbox_backend {
    ip_hash;  # Session affinity for WebSocket
    
    server backend1.example.com:8080 max_fails=3 fail_timeout=30s;
    server backend2.example.com:8080 max_fails=3 fail_timeout=30s;
    server backend3.example.com:8080 max_fails=3 fail_timeout=30s;
    
    keepalive 32;
}

# HTTP to HTTPS redirect
server {
    listen 80;
    listen [::]:80;
    server_name chat.example.com;
    
    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }
    
    # Redirect all other traffic to HTTPS
    location / {
        return 301 https://$server_name$request_uri;
    }
}

# HTTPS server
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name chat.example.com;
    
    # SSL certificate configuration
    ssl_certificate /etc/letsencrypt/live/chat.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/chat.example.com/privkey.pem;
    
    # SSL protocols and ciphers
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384';
    ssl_prefer_server_ciphers off;
    
    # SSL session cache
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    
    # OCSP stapling
    ssl_stapling on;
    ssl_stapling_verify on;
    ssl_trusted_certificate /etc/letsencrypt/live/chat.example.com/chain.pem;
    resolver 8.8.8.8 8.8.4.4 valid=300s;
    resolver_timeout 5s;
    
    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;
    add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' wss: https:; font-src 'self'; object-src 'none'; frame-ancestors 'self';" always;
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;
    
    # Connection limiting
    limit_conn chatbox_conn 10;
    
    # Logging
    access_log /var/log/nginx/chatbox_access.log combined;
    error_log /var/log/nginx/chatbox_error.log warn;
    
    # WebSocket endpoint
    location /chatbox/ws {
        limit_req zone=chatbox_ws burst=10 nodelay;
        
        proxy_pass http://chatbox_backend;
        
        # WebSocket upgrade headers
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket timeouts
        proxy_connect_timeout 7d;
        proxy_send_timeout 7d;
        proxy_read_timeout 7d;
        proxy_buffering off;
    }
    
    # Admin endpoints
    location /chatbox/admin/ {
        limit_req zone=chatbox_admin burst=5 nodelay;
        
        proxy_pass http://chatbox_backend;
        
        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Standard timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
    
    # Health check endpoints (no rate limiting)
    location ~ ^/chatbox/(healthz|readyz)$ {
        proxy_pass http://chatbox_backend;
        
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        
        # Short timeout for health checks
        proxy_connect_timeout 5s;
        proxy_send_timeout 5s;
        proxy_read_timeout 5s;
        
        # Don't log health checks
        access_log off;
    }
    
    # Metrics endpoint (optional, restrict access)
    location /metrics {
        # Restrict to monitoring systems
        allow 10.0.0.0/8;
        deny all;
        
        proxy_pass http://chatbox_backend;
        
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        
        proxy_connect_timeout 10s;
        proxy_send_timeout 10s;
        proxy_read_timeout 10s;
        
        access_log off;
    }
    
    # General chatbox endpoints
    location /chatbox/ {
        limit_req zone=chatbox_general burst=20 nodelay;
        
        proxy_pass http://chatbox_backend;
        
        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Standard timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
    
    # Custom error pages
    error_page 429 /429.html;
    error_page 502 503 504 /50x.html;
    
    location = /429.html {
        root /usr/share/nginx/html;
        internal;
    }
    
    location = /50x.html {
        root /usr/share/nginx/html;
        internal;
    }
}
```

## Troubleshooting

### WebSocket Connection Fails

**Symptoms:**
- WebSocket connections immediately close
- Browser console shows "WebSocket connection failed"

**Solutions:**
1. Verify upgrade headers are configured:
   ```nginx
   proxy_http_version 1.1;
   proxy_set_header Upgrade $http_upgrade;
   proxy_set_header Connection "upgrade";
   ```

2. Check timeouts are long enough:
   ```nginx
   proxy_connect_timeout 7d;
   proxy_send_timeout 7d;
   proxy_read_timeout 7d;
   ```

3. Disable buffering:
   ```nginx
   proxy_buffering off;
   ```

### 502 Bad Gateway

**Symptoms:**
- HTTP 502 errors
- "upstream prematurely closed connection"

**Solutions:**
1. Verify backend is running:
   ```bash
   curl http://localhost:8080/chatbox/healthz
   ```

2. Check backend logs for errors

3. Increase timeouts:
   ```nginx
   proxy_connect_timeout 60s;
   ```

4. Verify upstream configuration:
   ```nginx
   upstream chatbox_backend {
       server localhost:8080;
   }
   ```

### Rate Limiting Too Aggressive

**Symptoms:**
- Legitimate users getting 429 errors
- "limiting requests, excess" in error log

**Solutions:**
1. Increase rate limits:
   ```nginx
   limit_req_zone $binary_remote_addr zone=chatbox_general:10m rate=20r/s;
   ```

2. Increase burst size:
   ```nginx
   limit_req zone=chatbox_general burst=50 nodelay;
   ```

3. Whitelist trusted IPs:
   ```nginx
   geo $limit {
       default 1;
       10.0.0.0/8 0;  # Internal network
       192.168.0.0/16 0;  # Private network
   }
   
   map $limit $limit_key {
       0 "";
       1 $binary_remote_addr;
   }
   
   limit_req_zone $limit_key zone=chatbox_general:10m rate=10r/s;
   ```

### SSL Certificate Issues

**Symptoms:**
- "SSL certificate problem"
- "certificate has expired"

**Solutions:**
1. Verify certificate files exist:
   ```bash
   ls -l /etc/letsencrypt/live/chat.example.com/
   ```

2. Check certificate expiration:
   ```bash
   openssl x509 -in /etc/letsencrypt/live/chat.example.com/fullchain.pem -noout -dates
   ```

3. Renew Let's Encrypt certificate:
   ```bash
   sudo certbot renew
   sudo nginx -s reload
   ```

### Session Affinity Not Working

**Symptoms:**
- WebSocket connections drop randomly
- Users reconnect to different backend servers

**Solutions:**
1. Use `ip_hash` in upstream:
   ```nginx
   upstream chatbox_backend {
       ip_hash;
       server backend1:8080;
       server backend2:8080;
   }
   ```

2. Verify no load balancer between Nginx and clients

3. Check for proxy/CDN that changes client IP

### High Memory Usage

**Symptoms:**
- Nginx consuming excessive memory
- Out of memory errors

**Solutions:**
1. Reduce zone sizes:
   ```nginx
   limit_req_zone $binary_remote_addr zone=chatbox_general:5m rate=10r/s;
   ```

2. Reduce SSL session cache:
   ```nginx
   ssl_session_cache shared:SSL:5m;
   ```

3. Reduce keepalive connections:
   ```nginx
   upstream chatbox_backend {
       server backend:8080;
       keepalive 16;  # Reduced from 32
   }
   ```

## Testing Configuration

### Test Configuration Syntax

```bash
# Test nginx configuration
sudo nginx -t

# Reload nginx if test passes
sudo nginx -s reload
```

### Test WebSocket Connection

```bash
# Install wscat
npm install -g wscat

# Test WebSocket connection
wscat -c wss://chat.example.com/chatbox/ws?token=YOUR_JWT_TOKEN
```

### Test Health Endpoints

```bash
# Test health check
curl https://chat.example.com/chatbox/healthz

# Test readiness check
curl https://chat.example.com/chatbox/readyz
```

### Test Rate Limiting

```bash
# Send multiple requests quickly
for i in {1..20}; do
    curl -s -o /dev/null -w "%{http_code}\n" https://chat.example.com/chatbox/healthz
done
```

### Test SSL Configuration

```bash
# Test SSL with OpenSSL
openssl s_client -connect chat.example.com:443 -servername chat.example.com

# Test SSL with SSL Labs
# Visit: https://www.ssllabs.com/ssltest/analyze.html?d=chat.example.com
```

## Additional Resources

- [Nginx Documentation](https://nginx.org/en/docs/)
- [Nginx WebSocket Proxying](https://nginx.org/en/docs/http/websocket.html)
- [Mozilla SSL Configuration Generator](https://ssl-config.mozilla.org/)
- [Let's Encrypt Documentation](https://letsencrypt.org/docs/)
- [Chatbox Deployment Guide](../DEPLOYMENT.md)

## Support

For issues specific to the Chatbox service:
1. Check application logs: `kubectl logs -n chatbox -l app=chatbox`
2. Verify backend health: `curl http://backend:8080/chatbox/healthz`
3. Review [DEPLOYMENT.md](../DEPLOYMENT.md)
4. Check Nginx error logs: `/var/log/nginx/chatbox_error.log`
