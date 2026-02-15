# Kubernetes Deployment Guide for Chatbox WebSocket Service

This directory contains Kubernetes manifests for deploying the chatbox WebSocket service in both K8s and K3s environments.

## Files Overview

- `deployment.yaml` - Deployment manifest with health probes, resource limits, and HPA
- `service.yaml` - Service manifest with session affinity for WebSocket connections
- `configmap.yaml` - ConfigMap for non-sensitive configuration
- `secret.yaml` - Secret for sensitive data (API keys, credentials)
- `README.md` - This file

## Prerequisites

### For Kubernetes (K8s)
- Kubernetes cluster v1.24+
- kubectl configured to access your cluster
- Ingress controller (nginx, traefik, etc.) for external access
- cert-manager (optional, for TLS certificates)
- Metrics server (optional, for HPA)

### For K3s
- K3s cluster v1.24+
- kubectl configured to access your cluster
- Traefik ingress controller (included with K3s)
- Metrics server (included with K3s)

### External Dependencies
- MongoDB instance (can be deployed in cluster or external)
- S3-compatible storage (AWS S3, MinIO, etc.)
- SMTP server for email notifications
- Twilio account for SMS notifications (optional)
- LLM API keys (OpenAI, Anthropic, Dify)

## Quick Start

### 1. Update Configuration

**Edit `secret.yaml`** - Replace all placeholder values with actual credentials:
```bash
# CRITICAL: Change these values before deploying!
JWT_SECRET: "your-strong-random-jwt-secret-at-least-32-characters"
S3_ACCESS_KEY_ID: "your-actual-s3-access-key"
S3_SECRET_ACCESS_KEY: "your-actual-s3-secret-key"
LLM_PROVIDER_1_API_KEY: "sk-your-actual-openai-api-key"
# ... etc
```

**Edit `configmap.yaml`** - Update configuration values:
```bash
# Update MongoDB URI to point to your MongoDB instance
MONGO_URI: "mongodb://your-mongo-host:27017/chat"

# Update S3 configuration
S3_BUCKET: "your-actual-s3-bucket-name"
S3_REGION: "your-aws-region"

# Update notification emails/phones
ADMIN_EMAILS: "admin@yourdomain.com,support@yourdomain.com"
ADMIN_PHONES: "+1234567890"

# Update SMTP settings
SMTP_HOST: "smtp.yourdomain.com"
EMAIL_FROM: "noreply@yourdomain.com"
```

**Edit `service.yaml`** - Update ingress host:
```yaml
# In the Ingress section, update the host
spec:
  tls:
  - hosts:
    - chat.yourdomain.com  # Change this
  rules:
  - host: chat.yourdomain.com  # Change this
```

### 2. Build and Push Docker Image

```bash
# Build the Docker image
docker build -t your-registry/chatbox-websocket:v1.0.0 .

# Push to your container registry
docker push your-registry/chatbox-websocket:v1.0.0

# Update deployment.yaml with your image
# Change: image: chatbox-websocket:latest
# To:     image: your-registry/chatbox-websocket:v1.0.0
```

### 3. Deploy to Kubernetes

```bash
# Create namespace (optional)
kubectl create namespace chatbox

# Apply manifests
kubectl apply -f deployments/kubernetes/configmap.yaml
kubectl apply -f deployments/kubernetes/secret.yaml
kubectl apply -f deployments/kubernetes/deployment.yaml
kubectl apply -f deployments/kubernetes/service.yaml

# Verify deployment
kubectl get pods -n default -l app=chatbox
kubectl get svc -n default -l app=chatbox
kubectl get ingress -n default -l app=chatbox
```

### 4. Verify Deployment

```bash
# Check pod status
kubectl get pods -n default -l app=chatbox

# Check logs
kubectl logs -n default -l app=chatbox --tail=100 -f

# Check health endpoints
kubectl port-forward -n default svc/chatbox-websocket 8080:80
curl http://localhost:8080/chat/healthz
curl http://localhost:8080/chat/readyz

# Test WebSocket connection (requires wscat)
wscat -c ws://localhost:8080/chat/ws?token=YOUR_JWT_TOKEN
```

## Configuration Details

### Session Affinity for WebSocket

The service is configured with **ClientIP session affinity** which is CRITICAL for WebSocket connections:

```yaml
sessionAffinity: ClientIP
sessionAffinityConfig:
  clientIP:
    timeoutSeconds: 10800  # 3 hours
```

This ensures that all requests from the same client IP go to the same pod, maintaining WebSocket connection state.

### Health Probes

Three types of probes are configured:

1. **Liveness Probe** - Checks if container is alive
   - Endpoint: `/chat/healthz`
   - Failure action: Restart container

2. **Readiness Probe** - Checks if container is ready to serve traffic
   - Endpoint: `/chat/readyz`
   - Failure action: Remove from service endpoints

3. **Startup Probe** - Gives container time to start
   - Endpoint: `/chat/healthz`
   - Allows up to 60 seconds for startup

### Resource Limits

Default resource configuration:

```yaml
resources:
  requests:
    cpu: 250m      # 0.25 CPU cores
    memory: 256Mi  # 256 MB RAM
  limits:
    cpu: 1000m     # 1 CPU core
    memory: 1Gi    # 1 GB RAM
```

Adjust based on your load:
- For high traffic: Increase CPU/memory limits
- For low traffic: Decrease to save resources

### Horizontal Pod Autoscaler (HPA)

The HPA automatically scales pods based on CPU and memory usage:

```yaml
minReplicas: 3
maxReplicas: 10
metrics:
  - CPU: 70% utilization
  - Memory: 80% utilization
```

**Note**: Requires metrics-server to be installed in your cluster.

## Deployment Strategies

### Rolling Update (Default)

Zero-downtime deployments with rolling updates:

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 1        # Create 1 extra pod during update
    maxUnavailable: 0  # Keep all pods available
```

### Blue-Green Deployment

For critical updates, use blue-green deployment:

```bash
# Deploy new version with different label
kubectl apply -f deployment-v2.yaml

# Test new version
kubectl port-forward svc/chatbox-websocket-v2 8080:80

# Switch traffic by updating service selector
kubectl patch svc chatbox-websocket -p '{"spec":{"selector":{"version":"v2"}}}'

# Remove old version
kubectl delete deployment chatbox-websocket-v1
```

## Scaling

### Manual Scaling

```bash
# Scale to 5 replicas
kubectl scale deployment chatbox-websocket --replicas=5

# Check scaling status
kubectl get deployment chatbox-websocket
```

### Automatic Scaling (HPA)

HPA is configured by default. To modify:

```bash
# Edit HPA
kubectl edit hpa chatbox-websocket-hpa

# Check HPA status
kubectl get hpa chatbox-websocket-hpa
kubectl describe hpa chatbox-websocket-hpa
```

## Monitoring

### Logs

```bash
# View logs from all pods
kubectl logs -n default -l app=chatbox --tail=100 -f

# View logs from specific pod
kubectl logs -n default chatbox-websocket-xxxxx-yyyyy

# View logs from previous container (if crashed)
kubectl logs -n default chatbox-websocket-xxxxx-yyyyy --previous
```

### Metrics

```bash
# View pod metrics
kubectl top pods -n default -l app=chatbox

# View node metrics
kubectl top nodes
```

### Events

```bash
# View recent events
kubectl get events -n default --sort-by='.lastTimestamp'

# Watch events in real-time
kubectl get events -n default --watch
```

## Troubleshooting

### Pods Not Starting

```bash
# Check pod status
kubectl describe pod -n default chatbox-websocket-xxxxx-yyyyy

# Common issues:
# 1. Image pull errors - Check image name and registry credentials
# 2. ConfigMap/Secret not found - Ensure they are created first
# 3. Resource limits - Check if node has enough resources
```

### WebSocket Connections Failing

```bash
# Check service endpoints
kubectl get endpoints -n default chatbox-websocket

# Check ingress configuration
kubectl describe ingress -n default chatbox-websocket-ingress

# Verify session affinity
kubectl get svc chatbox-websocket -o yaml | grep -A 5 sessionAffinity

# Common issues:
# 1. Session affinity not configured - WebSocket connections drop
# 2. Ingress timeout too short - Increase proxy timeouts
# 3. Load balancer not supporting WebSocket - Check LB configuration
```

### Health Check Failures

```bash
# Check health endpoints directly
kubectl port-forward -n default svc/chatbox-websocket 8080:80
curl http://localhost:8080/chat/healthz
curl http://localhost:8080/chat/readyz

# Check pod logs for errors
kubectl logs -n default -l app=chatbox --tail=100

# Common issues:
# 1. MongoDB connection failure - Check MONGO_URI in ConfigMap
# 2. Slow startup - Increase startupProbe failureThreshold
# 3. Resource exhaustion - Check pod metrics
```

### Database Connection Issues

```bash
# Test MongoDB connectivity from pod
kubectl exec -it -n default chatbox-websocket-xxxxx-yyyyy -- sh
nc -zv mongo-service 27017

# Check MongoDB credentials in Secret
kubectl get secret chat-secrets -o yaml

# Common issues:
# 1. Wrong MongoDB URI - Update MONGO_URI in ConfigMap
# 2. Network policy blocking - Check network policies
# 3. MongoDB not ready - Check MongoDB pod status
```

## Security Best Practices

### 1. Use Strong Secrets

```bash
# Generate strong JWT secret
openssl rand -base64 32

# Update secret
kubectl create secret generic chat-secrets \
  --from-literal=JWT_SECRET=$(openssl rand -base64 32) \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 2. Enable RBAC

```bash
# Create service account with minimal permissions
kubectl create serviceaccount chatbox-sa
kubectl create rolebinding chatbox-rb \
  --clusterrole=view \
  --serviceaccount=default:chatbox-sa

# Update deployment to use service account
kubectl patch deployment chatbox-websocket \
  -p '{"spec":{"template":{"spec":{"serviceAccountName":"chatbox-sa"}}}}'
```

### 3. Network Policies

```bash
# Create network policy to restrict traffic
kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: chatbox-netpol
spec:
  podSelector:
    matchLabels:
      app: chatbox
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: nginx-ingress
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: mongodb
    ports:
    - protocol: TCP
      port: 27017
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 443  # For external API calls
EOF
```

### 4. Pod Security Standards

```bash
# Apply pod security standards
kubectl label namespace default \
  pod-security.kubernetes.io/enforce=restricted \
  pod-security.kubernetes.io/audit=restricted \
  pod-security.kubernetes.io/warn=restricted
```

## K3s Specific Configuration

K3s includes Traefik ingress controller by default. Update ingress configuration:

```yaml
# Change ingressClassName from nginx to traefik
spec:
  ingressClassName: traefik
  
# Update annotations for Traefik
metadata:
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
```

## Backup and Restore

### Backup Configuration

```bash
# Backup all manifests
kubectl get configmap chat-config -o yaml > backup/configmap.yaml
kubectl get secret chat-secrets -o yaml > backup/secret.yaml
kubectl get deployment chatbox-websocket -o yaml > backup/deployment.yaml
kubectl get service chatbox-websocket -o yaml > backup/service.yaml
```

### Restore Configuration

```bash
# Restore from backup
kubectl apply -f backup/
```

## Cleanup

```bash
# Delete all resources
kubectl delete -f deployments/kubernetes/

# Or delete by label
kubectl delete all -l app=chatbox

# Delete namespace (if created)
kubectl delete namespace chatbox
```

## Additional Resources

- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [K3s Documentation](https://docs.k3s.io/)
- [WebSocket on Kubernetes](https://kubernetes.io/blog/2019/04/23/running-websocket-servers-on-kubernetes/)
- [Horizontal Pod Autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
- [Ingress Controllers](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/)

## Support

For issues or questions:
1. Check logs: `kubectl logs -n default -l app=chatbox`
2. Check events: `kubectl get events -n default`
3. Review this README
4. Contact DevOps team
