# Production Readiness Fixes - Migration Guide

## Overview

This guide covers migrating from the previous version to the production-ready version with critical fixes for memory leaks, data races, and security vulnerabilities.

**Version**: 1.0.0 → 1.1.0  
**Release Date**: 2024-02-16  
**Breaking Changes**: None  
**Recommended Downtime**: None (rolling update supported)

## What's Fixed

### Critical Issues (MUST UPGRADE)

1. **Session Memory Leak** - Sessions were never removed from memory, causing unbounded growth
2. **Origin Validation Data Race** - Concurrent access to origin map could crash the server
3. **Connection SessionID Data Race** - Concurrent access to SessionID field could cause crashes

### High Priority Issues (STRONGLY RECOMMENDED)

4. **LLM Streaming Timeout** - Streaming requests could hang indefinitely
5. **Rate Limiter Memory Growth** - Rate limiter events accumulated without cleanup
6. **JWT Secret Validation** - Weak secrets were accepted, compromising security
7. **Admin Endpoint Rate Limiting** - Admin endpoints had no rate limiting

### Medium Priority Issues (RECOMMENDED)

8. **ResponseTimes Unbounded Growth** - Response time arrays grew without limit
9. **Configuration Validation** - Invalid configurations could be loaded
10. **MongoDB Retry Logic** - Transient errors caused immediate failures

### Low Priority Issues (OPTIONAL)

11. **Shutdown Timeout** - Shutdown could hang with many connections

## Pre-Migration Checklist

- [ ] Review all changes in this guide
- [ ] Backup current configuration (ConfigMap and Secret)
- [ ] Backup MongoDB database
- [ ] Test new configuration in staging environment
- [ ] Verify JWT secret meets new requirements (32+ characters)
- [ ] Plan rollback procedure
- [ ] Schedule maintenance window (optional, but recommended)

## Migration Steps

### Step 1: Update Configuration

#### 1.1 Validate JWT Secret

The new version enforces JWT secret strength:

```bash
# Check current JWT secret length
echo -n "your-current-secret" | wc -c

# If less than 32 characters, generate a new one
openssl rand -base64 32
```

**IMPORTANT**: If you change the JWT secret, all existing tokens will be invalidated and users will need to re-authenticate.

#### 1.2 Add New Environment Variables (Optional)

Add these to your ConfigMap for custom configuration:

```yaml
# deployments/kubernetes/configmap.yaml
data:
  # Production Readiness Configuration
  LLM_STREAM_TIMEOUT: "120s"              # Default: 120s
  SESSION_CLEANUP_INTERVAL: "5m"          # Default: 5m
  SESSION_TTL: "15m"                      # Default: 15m (match RECONNECT_TIMEOUT)
  RATE_LIMIT_CLEANUP_INTERVAL: "5m"       # Default: 5m
  ADMIN_RATE_LIMIT: "20"                  # Default: 20 req/min
  ADMIN_RATE_WINDOW: "1m"                 # Default: 1m
  MONGO_RETRY_ATTEMPTS: "3"               # Default: 3
  MONGO_RETRY_DELAY: "100ms"              # Default: 100ms
```

**Note**: All these variables have sensible defaults. Only add them if you need custom values.

### Step 2: Update Application

#### 2.1 Pull New Image

```bash
# Pull the new image
docker pull your-registry.com/chatbox-websocket:v1.1.0

# Or build from source
GITHUB_TOKEN=$(gh auth token) docker build \
  --build-arg GITHUB_TOKEN=$GITHUB_TOKEN \
  -t your-registry.com/chatbox-websocket:v1.1.0 .

# Push to registry
docker push your-registry.com/chatbox-websocket:v1.1.0
```

#### 2.2 Update Deployment Manifest

```yaml
# deployments/kubernetes/deployment.yaml
spec:
  template:
    spec:
      containers:
      - name: chatbox
        image: your-registry.com/chatbox-websocket:v1.1.0  # Update version
```

#### 2.3 Apply Configuration Changes

```bash
# Apply updated ConfigMap (if you added new variables)
kubectl apply -f deployments/kubernetes/configmap.yaml -n chatbox

# Apply updated Secret (if you changed JWT secret)
kubectl apply -f deployments/kubernetes/secret.yaml -n chatbox
```

### Step 3: Deploy New Version

#### Option A: Rolling Update (Zero Downtime)

```bash
# Update deployment
kubectl apply -f deployments/kubernetes/deployment.yaml -n chatbox

# Watch rollout
kubectl rollout status deployment/chatbox-websocket -n chatbox

# Verify new pods are running
kubectl get pods -n chatbox -l app=chatbox
```

#### Option B: Blue-Green Deployment

```bash
# Create new deployment with different name
kubectl apply -f deployments/kubernetes/deployment-v1.1.0.yaml -n chatbox

# Wait for new pods to be ready
kubectl wait --for=condition=ready pod -l app=chatbox,version=v1.1.0 -n chatbox

# Switch service to new deployment
kubectl patch service chatbox-websocket -n chatbox \
  -p '{"spec":{"selector":{"version":"v1.1.0"}}}'

# Delete old deployment after verification
kubectl delete deployment chatbox-websocket-v1.0.0 -n chatbox
```

### Step 4: Verify Deployment

#### 4.1 Check Pod Status

```bash
# Check pods are running
kubectl get pods -n chatbox -l app=chatbox

# Check logs for startup messages
kubectl logs -n chatbox -l app=chatbox --tail=50 | grep -E "INFO|ERROR"
```

Expected log messages:
```
INFO Starting session cleanup goroutine interval=5m0s ttl=15m0s
INFO Starting rate limiter cleanup goroutine interval=5m0s
INFO Configuration validated successfully
INFO Server listening on :8080
```

#### 4.2 Verify Health Endpoints

```bash
# Port forward to test locally
kubectl port-forward -n chatbox svc/chatbox-websocket 8080:80

# Test health endpoint
curl http://localhost:8080/chat/healthz
# Expected: {"status":"ok"}

# Test readiness endpoint
curl http://localhost:8080/chat/readyz
# Expected: {"status":"ready"}
```

#### 4.3 Test WebSocket Connection

```bash
# Get a valid JWT token (from your auth system)
TOKEN="your-jwt-token"

# Test WebSocket connection
wscat -c ws://localhost:8080/chat/ws?token=$TOKEN

# Send a test message
> {"type":"user","content":"Hello"}

# Verify response
< {"type":"assistant","content":"..."}
```

#### 4.4 Verify Memory Management

```bash
# Check memory usage over time
kubectl top pods -n chatbox -l app=chatbox

# Check cleanup logs
kubectl logs -n chatbox -l app=chatbox | grep -i cleanup
```

Expected cleanup logs:
```
INFO Cleaned up expired sessions count=5
INFO Rate limiter cleanup completed before=1234 after=234 removed=1000
```

### Step 5: Monitor for Issues

#### 5.1 Watch Metrics

```bash
# Monitor pod metrics
watch kubectl top pods -n chatbox -l app=chatbox

# Check Prometheus metrics (if configured)
curl http://localhost:8080/metrics | grep -E "websocket|session|rate"
```

#### 5.2 Monitor Logs

```bash
# Watch for errors
kubectl logs -n chatbox -l app=chatbox -f | grep ERROR

# Watch for warnings
kubectl logs -n chatbox -l app=chatbox -f | grep WARN
```

#### 5.3 Check for Data Races

If you have the race detector enabled:

```bash
# Check logs for race detector warnings
kubectl logs -n chatbox -l app=chatbox | grep "WARNING: DATA RACE"

# Should see no race warnings
```

## Rollback Procedure

If you encounter issues, rollback to the previous version:

### Quick Rollback

```bash
# Rollback deployment
kubectl rollout undo deployment/chatbox-websocket -n chatbox

# Verify rollback
kubectl rollout status deployment/chatbox-websocket -n chatbox
```

### Manual Rollback

```bash
# Update deployment to previous image
kubectl set image deployment/chatbox-websocket \
  chatbox=your-registry.com/chatbox-websocket:v1.0.0 \
  -n chatbox

# Restore previous ConfigMap (if changed)
kubectl apply -f backup/configmap-v1.0.0.yaml -n chatbox

# Restore previous Secret (if changed)
kubectl apply -f backup/secret-v1.0.0.yaml -n chatbox
```

## Troubleshooting

### Issue: Pods Fail to Start

**Symptom**: Pods in CrashLoopBackOff state

**Possible Causes**:
1. JWT secret too short (< 32 characters)
2. JWT secret contains weak patterns
3. Invalid configuration values

**Solution**:
```bash
# Check pod logs
kubectl logs -n chatbox chatbox-websocket-xxxxx-yyyyy

# Look for validation errors
kubectl logs -n chatbox chatbox-websocket-xxxxx-yyyyy | grep "validation failed"

# Fix configuration and reapply
kubectl apply -f deployments/kubernetes/configmap.yaml -n chatbox
kubectl apply -f deployments/kubernetes/secret.yaml -n chatbox
```

### Issue: High Memory Usage

**Symptom**: Memory usage continues to grow over time

**Possible Causes**:
1. Cleanup intervals too long
2. Session TTL too long
3. High session creation rate

**Solution**:
```bash
# Reduce cleanup interval
kubectl set env deployment/chatbox-websocket \
  SESSION_CLEANUP_INTERVAL=2m \
  -n chatbox

# Reduce session TTL
kubectl set env deployment/chatbox-websocket \
  SESSION_TTL=10m \
  -n chatbox

# Check cleanup logs
kubectl logs -n chatbox -l app=chatbox | grep "Cleaned up"
```

### Issue: LLM Requests Timing Out

**Symptom**: LLM requests fail with timeout errors

**Possible Causes**:
1. LLM_STREAM_TIMEOUT too short for your models
2. Network latency to LLM provider
3. LLM provider slow response

**Solution**:
```bash
# Increase timeout
kubectl set env deployment/chatbox-websocket \
  LLM_STREAM_TIMEOUT=300s \
  -n chatbox

# Check timeout logs
kubectl logs -n chatbox -l app=chatbox | grep "timeout"
```

### Issue: Admin Endpoints Rate Limited

**Symptom**: Admin requests return HTTP 429

**Possible Causes**:
1. ADMIN_RATE_LIMIT too low for your usage
2. Multiple admins sharing same user ID
3. Automated monitoring tools hitting limits

**Solution**:
```bash
# Increase admin rate limit
kubectl set env deployment/chatbox-websocket \
  ADMIN_RATE_LIMIT=50 \
  -n chatbox

# Check rate limit logs
kubectl logs -n chatbox -l app=chatbox | grep "rate limit exceeded"
```

### Issue: MongoDB Connection Failures

**Symptom**: Frequent MongoDB connection errors

**Possible Causes**:
1. Transient network issues
2. MongoDB overloaded
3. Retry attempts exhausted

**Solution**:
```bash
# Increase retry attempts
kubectl set env deployment/chatbox-websocket \
  MONGO_RETRY_ATTEMPTS=5 \
  MONGO_RETRY_DELAY=200ms \
  -n chatbox

# Check retry logs
kubectl logs -n chatbox -l app=chatbox | grep "retry"
```

## Performance Tuning

### Memory Optimization

For high-traffic deployments:

```yaml
# Aggressive cleanup
SESSION_CLEANUP_INTERVAL: "2m"
SESSION_TTL: "10m"
RATE_LIMIT_CLEANUP_INTERVAL: "2m"
```

For low-traffic deployments:

```yaml
# Conservative cleanup
SESSION_CLEANUP_INTERVAL: "15m"
SESSION_TTL: "30m"
RATE_LIMIT_CLEANUP_INTERVAL: "10m"
```

### Timeout Optimization

For fast LLM providers (OpenAI GPT-3.5):

```yaml
LLM_STREAM_TIMEOUT: "60s"
```

For slow LLM providers (Claude Opus, GPT-4):

```yaml
LLM_STREAM_TIMEOUT: "300s"
```

### Rate Limiting Optimization

For high-traffic admin dashboards:

```yaml
ADMIN_RATE_LIMIT: "100"
ADMIN_RATE_WINDOW: "1m"
```

For low-traffic admin access:

```yaml
ADMIN_RATE_LIMIT: "10"
ADMIN_RATE_WINDOW: "1m"
```

## Testing Recommendations

### Load Testing

Test the new version under load:

```bash
# Install k6 or similar load testing tool
brew install k6

# Run load test
k6 run load-test.js

# Monitor during test
watch kubectl top pods -n chatbox -l app=chatbox
```

### Chaos Testing

Test resilience:

```bash
# Kill random pods
kubectl delete pod -n chatbox -l app=chatbox --force --grace-period=0

# Verify recovery
kubectl get pods -n chatbox -l app=chatbox -w
```

### Memory Leak Testing

Run for extended period:

```bash
# Monitor memory over 24 hours
while true; do
  kubectl top pods -n chatbox -l app=chatbox
  sleep 300  # 5 minutes
done
```

## Post-Migration Checklist

- [ ] All pods running successfully
- [ ] Health endpoints responding
- [ ] WebSocket connections working
- [ ] No error logs
- [ ] Memory usage stable
- [ ] Cleanup logs appearing
- [ ] Admin endpoints accessible
- [ ] LLM requests completing
- [ ] MongoDB operations succeeding
- [ ] Metrics being collected

## Support

If you encounter issues not covered in this guide:

1. Check application logs: `kubectl logs -n chatbox -l app=chatbox`
2. Check pod events: `kubectl describe pod -n chatbox <pod-name>`
3. Review [DEPLOYMENT.md](DEPLOYMENT.md) for general deployment issues
4. Review [PRODUCTION_READINESS_REVIEW.md](PRODUCTION_READINESS_REVIEW.md) for issue details
5. Contact DevOps team

## Additional Resources

- [Production Readiness Review](PRODUCTION_READINESS_REVIEW.md) - Detailed issue analysis
- [Deployment Guide](DEPLOYMENT.md) - General deployment documentation
- [Configuration Guide](../README.md#configuration) - Configuration options
- [Kubernetes Deployment](../deployments/kubernetes/README.md) - K8s-specific docs
