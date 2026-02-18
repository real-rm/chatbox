# Nginx Configuration Templates

This directory contains ready-to-use Nginx configuration templates for deploying the Chatbox WebSocket service in various scenarios.

## Available Templates

### 1. single-server.conf
**Use Case:** Development, testing, or small production deployments with a single backend instance.

**Features:**
- Basic reverse proxy configuration
- WebSocket upgrade support
- Health check endpoints
- Simple logging

**Best For:**
- Development environments
- Small deployments
- Testing and staging
- Single-server production (low traffic)

### 2. load-balanced.conf
**Use Case:** Production deployments with multiple backend instances requiring load balancing.

**Features:**
- IP hash load balancing for WebSocket session affinity
- Multiple backend servers with health checks
- Rate limiting (general, WebSocket, admin endpoints)
- Connection limiting
- Custom error pages
- Passive health checks

**Best For:**
- Medium to large production deployments
- High availability setups
- Horizontal scaling scenarios
- Traffic distribution across multiple servers

### 3. ssl-tls.conf
**Use Case:** Production deployments requiring HTTPS/TLS encryption.

**Features:**
- HTTP to HTTPS redirect
- Modern TLS protocols (TLSv1.2, TLSv1.3)
- Security headers (HSTS, CSP, X-Frame-Options, etc.)
- OCSP stapling
- Let's Encrypt support
- Load balancing with IP hash
- Rate limiting
- Metrics endpoint with access control

**Best For:**
- Production deployments
- Public-facing services
- Security-sensitive applications
- Compliance requirements (PCI-DSS, HIPAA, etc.)

### 4. development.conf
**Use Case:** Local development and testing environments.

**Features:**
- Minimal configuration
- Verbose logging (debug level)
- CORS headers for local development
- No rate limiting
- Shorter timeouts for faster debugging
- Static file serving
- Unrestricted metrics access

**Best For:**
- Local development machines
- CI/CD testing
- Quick prototyping
- Debugging and troubleshooting

## Quick Start

### 1. Choose the Right Template

Select the template that matches your deployment scenario:
- **Development/Testing:** `development.conf`
- **Single Server:** `single-server.conf`
- **Multiple Servers:** `load-balanced.conf`
- **Production with HTTPS:** `ssl-tls.conf`

### 2. Copy and Customize

```bash
# Copy template to Nginx sites-available
sudo cp deployments/nginx/ssl-tls.conf /etc/nginx/sites-available/chatbox

# Edit the configuration
sudo nano /etc/nginx/sites-available/chatbox
```

### 3. Update Configuration

At minimum, update these values:
- `server_name`: Your domain name (e.g., `chat.example.com`)
- `server` addresses in `upstream`: Your backend server addresses
- SSL certificate paths (for SSL/TLS template)

### 4. Enable and Test

```bash
# Create symlink to enable the site
sudo ln -s /etc/nginx/sites-available/chatbox /etc/nginx/sites-enabled/

# Test configuration
sudo nginx -t

# Reload Nginx
sudo nginx -s reload
```

## Configuration Checklist

### All Templates
- [ ] Update `server_name` with your domain
- [ ] Update backend server addresses in `upstream` block
- [ ] Verify path prefix matches your application configuration (default: `/chatbox`)
- [ ] Adjust timeouts if needed
- [ ] Configure logging paths

### SSL/TLS Template
- [ ] Obtain SSL certificate (Let's Encrypt recommended)
- [ ] Update certificate paths
- [ ] Test HTTPS redirect
- [ ] Enable HSTS header (after testing)
- [ ] Configure OCSP stapling
- [ ] Adjust Content Security Policy for your frontend

### Load Balanced Template
- [ ] Add all backend server addresses
- [ ] Configure health check parameters (`max_fails`, `fail_timeout`)
- [ ] Adjust rate limiting values
- [ ] Configure connection limits
- [ ] Set up custom error pages
- [ ] Consider weighted load balancing if servers have different capacities

### Development Template
- [ ] Update backend server address (usually `localhost:8080`)
- [ ] Adjust CORS headers if needed
- [ ] Configure static file paths (if using)

## Path Prefix Configuration

The Chatbox service uses a configurable path prefix (default: `/chatbox`). If you change the path prefix in your application configuration, update the Nginx configuration accordingly:

**Application Configuration:**
```bash
# Environment variable
export CHATBOX_PATH_PREFIX="/api"

# Or in config.toml
[server]
path_prefix = "/api"
```

**Nginx Configuration:**
```nginx
# Update all location blocks
location /api/ws {
    # WebSocket endpoint
}

location /api/ {
    # HTTP endpoints
}
```

## Testing Your Configuration

### 1. Test Configuration Syntax
```bash
sudo nginx -t
```

### 2. Test Health Endpoints
```bash
# HTTP
curl http://chat.example.com/chatbox/healthz
curl http://chat.example.com/chatbox/readyz

# HTTPS
curl https://chat.example.com/chatbox/healthz
curl https://chat.example.com/chatbox/readyz
```

### 3. Test WebSocket Connection
```bash
# Install wscat
npm install -g wscat

# Test WebSocket (replace with your JWT token)
wscat -c ws://chat.example.com/chatbox/ws?token=YOUR_JWT_TOKEN

# Test WebSocket with SSL
wscat -c wss://chat.example.com/chatbox/ws?token=YOUR_JWT_TOKEN
```

### 4. Test Rate Limiting
```bash
# Send multiple requests quickly
for i in {1..20}; do
    curl -s -o /dev/null -w "%{http_code}\n" https://chat.example.com/chatbox/healthz
done

# Should see some 429 (Too Many Requests) responses
```

### 5. Test SSL Configuration
```bash
# Test SSL with OpenSSL
openssl s_client -connect chat.example.com:443 -servername chat.example.com

# Check SSL Labs rating (production only)
# Visit: https://www.ssllabs.com/ssltest/analyze.html?d=chat.example.com
```

## Common Customizations

### Adjust Rate Limits

```nginx
# Increase rate limits for higher traffic
limit_req_zone $binary_remote_addr zone=chatbox_general:10m rate=20r/s;
limit_req_zone $binary_remote_addr zone=chatbox_ws:10m rate=10r/s;

# Increase burst size
location /chatbox/ {
    limit_req zone=chatbox_general burst=50 nodelay;
}
```

### Whitelist Internal IPs

```nginx
# Exclude internal IPs from rate limiting
geo $limit {
    default 1;
    10.0.0.0/8 0;
    192.168.0.0/16 0;
}

map $limit $limit_key {
    0 "";
    1 $binary_remote_addr;
}

limit_req_zone $limit_key zone=chatbox_general:10m rate=10r/s;
```

### Add Custom Headers

```nginx
location /chatbox/ {
    # Add custom headers
    add_header X-Custom-Header "value" always;
    add_header X-Backend-Server $upstream_addr always;
    
    proxy_pass http://chatbox_backend;
}
```

### Configure Client Body Size

```nginx
server {
    # Increase max upload size (default: 1MB)
    client_max_body_size 10M;
    
    # For file uploads
    location /chatbox/upload {
        client_max_body_size 50M;
        proxy_pass http://chatbox_backend;
    }
}
```

## Troubleshooting

### WebSocket Connection Fails

**Problem:** WebSocket connections immediately close or fail to upgrade.

**Solution:**
1. Verify upgrade headers are present:
   ```nginx
   proxy_http_version 1.1;
   proxy_set_header Upgrade $http_upgrade;
   proxy_set_header Connection "upgrade";
   ```

2. Check timeouts are sufficient:
   ```nginx
   proxy_connect_timeout 7d;
   proxy_send_timeout 7d;
   proxy_read_timeout 7d;
   ```

3. Ensure buffering is disabled:
   ```nginx
   proxy_buffering off;
   ```

### 502 Bad Gateway

**Problem:** Nginx returns 502 errors.

**Solution:**
1. Verify backend is running:
   ```bash
   curl http://localhost:8080/chatbox/healthz
   ```

2. Check backend logs for errors

3. Verify upstream configuration:
   ```nginx
   upstream chatbox_backend {
       server localhost:8080;  # Correct address?
   }
   ```

### Rate Limiting Too Aggressive

**Problem:** Legitimate users getting 429 errors.

**Solution:**
1. Increase rate limits:
   ```nginx
   limit_req_zone $binary_remote_addr zone=chatbox_general:10m rate=20r/s;
   ```

2. Increase burst size:
   ```nginx
   limit_req zone=chatbox_general burst=50 nodelay;
   ```

3. Whitelist trusted IPs (see Common Customizations)

### SSL Certificate Errors

**Problem:** SSL certificate warnings or errors.

**Solution:**
1. Verify certificate files exist:
   ```bash
   ls -l /etc/letsencrypt/live/chat.example.com/
   ```

2. Check certificate expiration:
   ```bash
   openssl x509 -in /etc/letsencrypt/live/chat.example.com/fullchain.pem -noout -dates
   ```

3. Renew certificate:
   ```bash
   sudo certbot renew
   sudo nginx -s reload
   ```

### Session Affinity Not Working

**Problem:** WebSocket connections drop when load balancing.

**Solution:**
1. Ensure `ip_hash` is enabled:
   ```nginx
   upstream chatbox_backend {
       ip_hash;
       server backend1:8080;
       server backend2:8080;
   }
   ```

2. Verify no proxy/CDN between Nginx and clients that changes IP

## Performance Tuning

### Worker Processes and Connections

```nginx
# In nginx.conf (main configuration)
worker_processes auto;  # One per CPU core

events {
    worker_connections 4096;  # Increase for high traffic
    use epoll;  # Linux
}
```

### Keepalive Connections

```nginx
upstream chatbox_backend {
    server backend:8080;
    
    # Increase keepalive connections
    keepalive 64;
    keepalive_requests 100;
    keepalive_timeout 60s;
}
```

### Buffer Sizes

```nginx
server {
    # Adjust buffer sizes for large requests
    client_body_buffer_size 128k;
    client_header_buffer_size 1k;
    large_client_header_buffers 4 16k;
}
```

## Security Best Practices

1. **Always use HTTPS in production** - Use the `ssl-tls.conf` template
2. **Enable HSTS** - After testing HTTPS works correctly
3. **Configure rate limiting** - Protect against abuse and DDoS
4. **Restrict admin endpoints** - Use IP whitelisting for admin API
5. **Keep Nginx updated** - Apply security patches regularly
6. **Monitor logs** - Watch for suspicious activity
7. **Use strong SSL ciphers** - Follow Mozilla SSL Configuration Generator
8. **Enable OCSP stapling** - Improve SSL performance and privacy
9. **Set security headers** - Protect against common web attacks
10. **Limit connection counts** - Prevent resource exhaustion

## Additional Resources

- [Nginx Documentation](https://nginx.org/en/docs/)
- [Nginx WebSocket Proxying](https://nginx.org/en/docs/http/websocket.html)
- [Mozilla SSL Configuration Generator](https://ssl-config.mozilla.org/)
- [Let's Encrypt Documentation](https://letsencrypt.org/docs/)
- [Chatbox NGINX Setup Guide](../../docs/NGINX_SETUP.md)
- [Chatbox Deployment Guide](../../DEPLOYMENT.md)

## Support

For issues specific to the Chatbox service:
1. Check application logs
2. Verify backend health endpoints
3. Review [NGINX_SETUP.md](../../docs/NGINX_SETUP.md)
4. Check Nginx error logs: `/var/log/nginx/chatbox_error.log`
