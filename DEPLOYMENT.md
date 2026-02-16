# Chatbox WebSocket Service - Deployment Guide

This guide covers deployment of the chatbox WebSocket service to Kubernetes (K8s) and K3s environments.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Local Development](#local-development)
3. [Building the Docker Image](#building-the-docker-image)
4. [Kubernetes Deployment](#kubernetes-deployment)
5. [K3s Deployment](#k3s-deployment)
6. [Configuration](#configuration)
7. [Monitoring and Troubleshooting](#monitoring-and-troubleshooting)
8. [Scaling](#scaling)
9. [Security](#security)
10. [Backup and Recovery](#backup-and-recovery)

## Prerequisites

### Required Tools

- Docker 20.10+
- kubectl 1.24+
- Go 1.21+ (for building from source)
- make (optional, for using Makefile)

### Required Services

- Kubernetes cluster (K8s 1.24+ or K3s 1.24+)
- MongoDB 5.0+ (for session storage)
- S3-compatible storage (AWS S3, MinIO, etc.)
- SMTP server (for email notifications)
- Twilio account (optional, for SMS notifications)

### Required API Keys

- OpenAI API key (for GPT models)
- Anthropic API key (for Claude models)
- Dify API key (optional, for Dify integration)

## Local Development

### Using Docker Compose

The easiest way to run the service locally is using Docker Compose:

```bash
# Start all services (MongoDB, MinIO, MailHog, Chatbox)
docker-compose up -d

# View logs
docker-compose logs -f chatbox

# Stop all services
docker-compose down

# Stop and remove volumes
docker-compose down -v
```

Access points:
- Chatbox WebSocket: `ws://localhost:8080/chat/ws`
- Health check: `http://localhost:8080/chat/healthz`
- MinIO Console: `http://localhost:9001` (minioadmin/minioadmin)
- MailHog UI: `http://localhost:8025`
- MongoDB: `mongodb://admin:password@localhost:27017/chat`

### Running from Source

```bash
# Install dependencies
go mod download

# Copy and edit configuration
cp config.toml config.local.toml
# Edit config.local.toml with your settings

# Run the service
go run cmd/server/main.go -config config.local.toml

# Or build and run
go build -o chatbox-server cmd/server/main.go
./chatbox-server -config config.local.toml
```

## Building the Docker Image

### Prerequisites for Building

The chatbox application uses private Go modules from the `github.com/real-rm` organization. To build the Docker image, you need:

1. **GitHub Personal Access Token (PAT)** with `repo` scope
   - Create at: https://github.com/settings/tokens
   - Required scopes: `repo` (for private repository access)

2. **GitHub CLI (gh)** - Alternative to manual token management
   ```bash
   # Install GitHub CLI
   brew install gh  # macOS
   # or follow: https://cli.github.com/manual/installation
   
   # Authenticate
   gh auth login
   ```

### Build with Private Modules

The Dockerfile is configured to authenticate with GitHub for private module access:

```bash
# Option 1: Using GitHub CLI (Recommended)
GITHUB_TOKEN=$(gh auth token) docker build \
  --build-arg GITHUB_TOKEN=$GITHUB_TOKEN \
  -t chatbox-websocket:v1.0.0 .

# Option 2: Using Personal Access Token
docker build \
  --build-arg GITHUB_TOKEN=ghp_your_token_here \
  -t chatbox-websocket:v1.0.0 .

# Option 3: Using environment variable
export GITHUB_TOKEN=$(gh auth token)
docker build \
  --build-arg GITHUB_TOKEN=$GITHUB_TOKEN \
  -t chatbox-websocket:v1.0.0 .
```

**Security Note**: The `GITHUB_TOKEN` is only used during the build process and is not stored in the final image. However, Docker will show a warning about using secrets in build arguments - this is expected.

### CI/CD Environment Setup

For automated builds in CI/CD pipelines, see the comprehensive **[CI_SETUP.md](CI_SETUP.md)** documentation.

**Quick Start**:
- GitHub Actions: Use `.github/workflows/docker-build.yml` (included)
- GitLab CI: Use `.gitlab-ci.yml` (included)
- Test locally: Run `./test_ci_build.sh`

For automated builds in CI/CD pipelines (GitHub Actions, GitLab CI, Jenkins, etc.):

1. **Store GitHub Token as Secret**:
   - GitHub Actions: Add as repository secret `GITHUB_TOKEN`
   - GitLab CI: Add as CI/CD variable `GITHUB_TOKEN`
   - Jenkins: Add as credential

2. **Example GitHub Actions Workflow**:
   ```yaml
   - name: Build Docker image
     run: |
       docker build \
         --build-arg GITHUB_TOKEN=${{ secrets.GITHUB_TOKEN }} \
         -t chatbox-websocket:${{ github.sha }} .
   ```

3. **Example GitLab CI**:
   ```yaml
   build:
     script:
       - docker build --build-arg GITHUB_TOKEN=$GITHUB_TOKEN -t chatbox-websocket:$CI_COMMIT_SHA .
   ```

### Build Locally (Without Private Modules)

If you have the private modules already cached locally:

```bash
# Build the image
docker build -t chatbox-websocket:v1.0.0 .

# Test the image
docker run -p 8080:8080 \
  -e JWT_SECRET="test-secret" \
  -e MONGO_URI="mongodb://localhost:27017/chat" \
  chatbox-websocket:v1.0.0
```

### Build and Push to Registry

```bash
# Tag for your registry
docker tag chatbox-websocket:v1.0.0 your-registry.com/chatbox-websocket:v1.0.0

# Push to registry
docker push your-registry.com/chatbox-websocket:v1.0.0
```

### Using Makefile

```bash
# Build, tag, and push in one command
cd deployments/kubernetes
make build-push REGISTRY=your-registry.com IMAGE_TAG=v1.0.0
```

## Kubernetes Deployment

### Step 1: Prepare Configuration

**IMPORTANT: Secret Management**

See [SECRET_MANAGEMENT.md](./SECRET_MANAGEMENT.md) for comprehensive guidance on secret management, including:
- Kubernetes secrets setup
- External secret managers (AWS Secrets Manager, HashiCorp Vault)
- Secret rotation procedures
- Security best practices

1. **Edit Secret** (`deployments/kubernetes/secret.yaml`):
   ```bash
   # Generate strong JWT secret
   openssl rand -base64 32
   
   # Generate encryption key for message content (32 bytes for AES-256)
   openssl rand -base64 32
   
   # Update secret.yaml with:
   # - JWT_SECRET (generated above)
   # - ENCRYPTION_KEY (generated above) - CRITICAL for message encryption
   # - S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY
   # - LLM_PROVIDER_*_API_KEY (OpenAI, Anthropic, etc.)
   # - SMTP credentials
   # - SMS credentials (if using)
   ```
   
   **IMPORTANT**: See [KEY_MANAGEMENT.md](../KEY_MANAGEMENT.md) for comprehensive guidance on encryption key generation, storage, rotation, and security best practices.

2. **Edit ConfigMap** (`deployments/kubernetes/configmap.yaml`):
   ```yaml
   # Update these values:
   MONGO_URI: "mongodb://your-mongo-host:27017/chat"
   S3_BUCKET: "your-s3-bucket-name"
   S3_REGION: "your-aws-region"
   ADMIN_EMAILS: "admin@yourdomain.com"
   SMTP_HOST: "smtp.yourdomain.com"
   ```

3. **Edit Service** (`deployments/kubernetes/service.yaml`):
   ```yaml
   # Update ingress host:
   spec:
     tls:
     - hosts:
       - chat.yourdomain.com
     rules:
     - host: chat.yourdomain.com
   ```

4. **Edit Deployment** (`deployments/kubernetes/deployment.yaml`):
   ```yaml
   # Update image:
   spec:
     template:
       spec:
         containers:
         - name: chatbox
           image: your-registry.com/chatbox-websocket:v1.0.0
   ```

### Step 2: Deploy to Kubernetes

```bash
# Using kubectl
cd deployments/kubernetes

# Create namespace (optional)
kubectl create namespace chatbox

# Deploy configuration
kubectl apply -f configmap.yaml -n chatbox
kubectl apply -f secret.yaml -n chatbox

# Deploy application
kubectl apply -f deployment.yaml -n chatbox
kubectl apply -f service.yaml -n chatbox

# Verify deployment
kubectl get pods -n chatbox -l app=chatbox
kubectl get svc -n chatbox
kubectl get ingress -n chatbox
```

### Step 3: Using Makefile (Recommended)

```bash
cd deployments/kubernetes

# Check current context
make check-context

# Validate manifests
make validate

# Deploy everything
make deploy NAMESPACE=chatbox

# Check status
make status NAMESPACE=chatbox

# View logs
make logs NAMESPACE=chatbox
```

### Step 4: Verify Deployment

```bash
# Check pod status
kubectl get pods -n chatbox -l app=chatbox

# Check logs
kubectl logs -n chatbox -l app=chatbox --tail=100

# Test health endpoints
kubectl port-forward -n chatbox svc/chatbox-websocket 8080:80
curl http://localhost:8080/chat/healthz
curl http://localhost:8080/chat/readyz

# Test WebSocket connection (requires wscat)
wscat -c ws://localhost:8080/chat/ws?token=YOUR_JWT_TOKEN
```

## K3s Deployment

K3s deployment is similar to K8s, with a few differences:

### Differences from K8s

1. **Ingress Controller**: K3s uses Traefik by default (instead of nginx)
2. **Metrics Server**: Included by default
3. **Storage**: Local path provisioner included

### Deploy to K3s

```bash
# Update ingress for Traefik
# Edit deployments/kubernetes/service.yaml:
# Change ingressClassName from "nginx" to "traefik"

# Deploy using same commands as K8s
cd deployments/kubernetes
make deploy NAMESPACE=chatbox

# Or use k3s-specific target
make k3s-deploy NAMESPACE=chatbox
```

### K3s-Specific Configuration

Update `service.yaml` for Traefik:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: chatbox-websocket-ingress
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
spec:
  ingressClassName: traefik
  # ... rest of configuration
```

## Configuration

### Environment Variables

The service can be configured using environment variables or config.toml file:

| Variable | Description | Default |
|----------|-------------|---------|
| `SERVER_PORT` | HTTP server port | `8080` |
| `JWT_SECRET` | JWT signing secret | **Required** |
| `MONGO_URI` | MongoDB connection URI | **Required** |
| `S3_BUCKET` | S3 bucket name | **Required** |
| `RECONNECT_TIMEOUT` | Session reconnect timeout | `15m` |
| `MAX_CONNECTIONS` | Max concurrent connections | `10000` |
| `RATE_LIMIT` | Requests per second limit | `100` |

See `deployments/kubernetes/configmap.yaml` for full list.

### CORS and Origin Validation

The application provides two layers of origin validation for security:

#### CORS for HTTP Endpoints

Configure CORS to allow admin dashboards and monitoring tools to access HTTP endpoints:

| Variable | Description | Example |
|----------|-------------|---------|
| `CORS_ALLOWED_ORIGINS` | Comma-separated list of allowed origins for HTTP endpoints | `https://admin.example.com,https://dashboard.example.com` |

**Behavior**:
- Empty value = CORS middleware disabled (same-origin only)
- Configured = Allows cross-origin requests from listed domains
- Handles preflight OPTIONS requests automatically
- Allows credentials (cookies, auth headers)

**Example Configuration**:
```yaml
# In configmap.yaml
CORS_ALLOWED_ORIGINS: "https://admin.example.com,https://dashboard.example.com"

# Or via environment variable
export CORS_ALLOWED_ORIGINS="https://admin.example.com,https://dashboard.example.com"
```

#### WebSocket Origin Validation

Configure allowed origins for WebSocket connections to prevent CSRF attacks:

| Variable | Description | Example |
|----------|-------------|---------|
| `WS_ALLOWED_ORIGINS` | Comma-separated list of allowed origins for WebSocket connections | `https://chat.example.com,https://app.example.com` |

**Security Warning**: Empty value allows ALL origins (development only). Always configure this in production.

**Example Configuration**:
```yaml
# In configmap.yaml
WS_ALLOWED_ORIGINS: "https://chat.example.com,https://app.example.com"

# Or via environment variable
export WS_ALLOWED_ORIGINS="https://chat.example.com,https://app.example.com"
```

**Testing CORS**:
```bash
# Test CORS preflight
curl -X OPTIONS http://your-service/chat/admin/sessions \
  -H "Origin: https://admin.example.com" \
  -H "Access-Control-Request-Method: GET" \
  -v

# Test WebSocket origin
wscat -c ws://your-service/chat/ws?token=TOKEN \
  --origin https://chat.example.com
```

For detailed CORS configuration, see [deployments/kubernetes/README.md](deployments/kubernetes/README.md#cors-and-origin-validation).

### Session Affinity

**CRITICAL**: Session affinity is required for WebSocket connections.

The service is configured with ClientIP session affinity:

```yaml
sessionAffinity: ClientIP
sessionAffinityConfig:
  clientIP:
    timeoutSeconds: 10800  # 3 hours
```

This ensures all requests from the same client go to the same pod.

### Health Probes

Three types of health probes are configured:

1. **Liveness Probe** (`/chat/healthz`): Checks if container is alive
2. **Readiness Probe** (`/chat/readyz`): Checks if ready to serve traffic
3. **Startup Probe** (`/chat/healthz`): Gives time for slow startup

## Monitoring and Troubleshooting

### View Logs

```bash
# All pods
kubectl logs -n chatbox -l app=chatbox --tail=100 -f

# Specific pod
kubectl logs -n chatbox chatbox-websocket-xxxxx-yyyyy -f

# Previous container (if crashed)
kubectl logs -n chatbox chatbox-websocket-xxxxx-yyyyy --previous
```

### Check Status

```bash
# Deployment status
kubectl get deployment chatbox-websocket -n chatbox

# Pod status
kubectl get pods -n chatbox -l app=chatbox

# Service status
kubectl get svc chatbox-websocket -n chatbox

# Ingress status
kubectl get ingress -n chatbox
```

### Common Issues

#### Pods Not Starting

```bash
# Check pod events
kubectl describe pod -n chatbox chatbox-websocket-xxxxx-yyyyy

# Common causes:
# 1. Image pull errors - Check image name and registry credentials
# 2. ConfigMap/Secret not found - Ensure they exist
# 3. Resource limits - Check node resources
```

#### WebSocket Connections Failing

```bash
# Check service endpoints
kubectl get endpoints -n chatbox chatbox-websocket

# Verify session affinity
kubectl get svc chatbox-websocket -n chatbox -o yaml | grep -A 5 sessionAffinity

# Common causes:
# 1. Session affinity not configured
# 2. Ingress timeout too short
# 3. Load balancer not supporting WebSocket
```

#### Database Connection Issues

```bash
# Test MongoDB from pod
kubectl exec -it -n chatbox chatbox-websocket-xxxxx-yyyyy -- sh
nc -zv mongo-service 27017

# Check MongoDB credentials
kubectl get secret chat-secrets -n chatbox -o yaml
```

### Using Makefile for Troubleshooting

```bash
cd deployments/kubernetes

# Check status
make status NAMESPACE=chatbox

# View logs
make logs NAMESPACE=chatbox

# View events
make events NAMESPACE=chatbox

# Check health
make health NAMESPACE=chatbox

# Open shell in pod
make shell NAMESPACE=chatbox
```

## Scaling

### Manual Scaling

```bash
# Scale to 5 replicas
kubectl scale deployment chatbox-websocket -n chatbox --replicas=5

# Using Makefile
make scale REPLICAS=5 NAMESPACE=chatbox
```

### Horizontal Pod Autoscaler (HPA)

HPA is configured by default in `deployment.yaml`:

```yaml
minReplicas: 3
maxReplicas: 10
metrics:
  - CPU: 70% utilization
  - Memory: 80% utilization
```

Check HPA status:

```bash
# View HPA
kubectl get hpa chatbox-websocket-hpa -n chatbox

# Describe HPA
kubectl describe hpa chatbox-websocket-hpa -n chatbox

# Using Makefile
make hpa-status NAMESPACE=chatbox
```

### Resource Limits

Adjust resource limits in `deployment.yaml`:

```yaml
resources:
  requests:
    cpu: 250m      # Minimum CPU
    memory: 256Mi  # Minimum memory
  limits:
    cpu: 1000m     # Maximum CPU
    memory: 1Gi    # Maximum memory
```

## Security

### Best Practices

1. **Use Strong Secrets**:
   ```bash
   # Generate strong JWT secret
   openssl rand -base64 32
   ```

2. **Enable RBAC**:
   ```bash
   # Create service account
   kubectl create serviceaccount chatbox-sa -n chatbox
   
   # Create role binding
   kubectl create rolebinding chatbox-rb \
     --clusterrole=view \
     --serviceaccount=chatbox:chatbox-sa \
     -n chatbox
   ```

3. **Use Network Policies**:
   ```bash
   # Restrict traffic to/from pods
   kubectl apply -f network-policy.yaml -n chatbox
   ```

4. **Enable Pod Security Standards**:
   ```bash
   kubectl label namespace chatbox \
     pod-security.kubernetes.io/enforce=restricted
   ```

5. **Use TLS/SSL**:
   - Configure ingress with TLS certificates
   - Use cert-manager for automatic certificate management

### Secrets Management

**Never commit secrets to version control!**

For comprehensive secret management documentation, see [SECRET_MANAGEMENT.md](./SECRET_MANAGEMENT.md).

Use one of these approaches:

1. **Kubernetes Secrets** (default):
   ```bash
   kubectl create secret generic chat-secrets \
     --from-literal=JWT_SECRET=$(openssl rand -base64 32) \
     -n chatbox
   ```

2. **External Secrets Operator**:
   - Sync secrets from AWS Secrets Manager, HashiCorp Vault, etc.

3. **Sealed Secrets**:
   - Encrypt secrets for safe storage in Git

## Database Indexes

### Automatic Index Creation

MongoDB indexes are **automatically created** when the application starts. The application includes an `EnsureIndexes` function that creates all necessary indexes during initialization.

**For comprehensive index documentation, see [docs/MONGODB_INDEXES.md](docs/MONGODB_INDEXES.md)**

**Quick Summary:**
- Indexes are created automatically during application startup
- 30-second timeout for index creation
- Non-blocking (app continues if creation fails)
- Idempotent (safe to run multiple times)

**Indexes Created:**
- `idx_user_id` - User-specific session queries
- `idx_start_time` - Time-based sorting (descending)
- `idx_admin_assisted` - Admin session filtering
- `idx_user_start_time` - Compound index for common patterns

**Verification:**

```bash
# Check application logs for index creation
kubectl logs -n chatbox -l app=chatbox | grep "MongoDB indexes"

# Expected output:
# INFO MongoDB indexes created successfully indexes=[idx_user_id, idx_start_time, idx_admin_assisted, idx_user_start_time]
```

For detailed information including:
- Index definitions and query patterns
- Manual creation procedures
- Performance considerations
- Troubleshooting guide

See the complete documentation: [docs/MONGODB_INDEXES.md](docs/MONGODB_INDEXES.md)

## Backup and Recovery

### Backup Configuration

```bash
# Backup all manifests
cd deployments/kubernetes
make backup NAMESPACE=chatbox

# Manual backup
kubectl get configmap chat-config -n chatbox -o yaml > backup/configmap.yaml
kubectl get secret chat-secrets -n chatbox -o yaml > backup/secret.yaml
kubectl get deployment chatbox-websocket -n chatbox -o yaml > backup/deployment.yaml
kubectl get service chatbox-websocket -n chatbox -o yaml > backup/service.yaml
```

### Restore Configuration

```bash
# Restore from backup
kubectl apply -f backup/ -n chatbox
```

### Database Backup

```bash
# Backup MongoDB
kubectl exec -n chatbox mongodb-pod -- mongodump \
  --uri="mongodb://admin:password@localhost:27017/chat" \
  --out=/backup

# Copy backup from pod
kubectl cp chatbox/mongodb-pod:/backup ./mongodb-backup
```

## Rolling Updates

### Update Application

```bash
# Update image in deployment.yaml
# Then apply:
kubectl apply -f deployment.yaml -n chatbox

# Or use kubectl set image:
kubectl set image deployment/chatbox-websocket \
  chatbox=your-registry.com/chatbox-websocket:v1.1.0 \
  -n chatbox

# Watch rollout
kubectl rollout status deployment/chatbox-websocket -n chatbox
```

### Rollback

```bash
# Rollback to previous version
kubectl rollout undo deployment/chatbox-websocket -n chatbox

# Rollback to specific revision
kubectl rollout undo deployment/chatbox-websocket --to-revision=2 -n chatbox

# Using Makefile
make rollback NAMESPACE=chatbox
```

## Cleanup

```bash
# Delete all resources
kubectl delete -f deployments/kubernetes/ -n chatbox

# Delete namespace
kubectl delete namespace chatbox

# Using Makefile
make delete NAMESPACE=chatbox
```

## Additional Resources

- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [K3s Documentation](https://docs.k3s.io/)
- [Docker Documentation](https://docs.docker.com/)
- [WebSocket on Kubernetes](https://kubernetes.io/blog/2019/04/23/running-websocket-servers-on-kubernetes/)

## Support

For issues or questions:
1. Check logs: `kubectl logs -n chatbox -l app=chatbox`
2. Check events: `kubectl get events -n chatbox`
3. Review this guide
4. Check `deployments/kubernetes/README.md`
5. Contact DevOps team
