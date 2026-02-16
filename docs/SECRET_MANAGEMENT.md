# Secret Management Guide

## Overview

This document describes how secrets are managed in the chatbox application for production deployments. The application follows security best practices by:

1. **Never storing secrets in source control** - All secrets use placeholder values in config files
2. **Prioritizing environment variables** - Kubernetes secrets are injected as environment variables
3. **Supporting multiple deployment methods** - Works with Kubernetes secrets, external secret managers, or environment variables

## Architecture

### Secret Loading Priority

The application loads secrets in the following priority order:

1. **Environment Variables** (highest priority)
2. **Config File** (config.toml - fallback only)

This design allows Kubernetes secrets to override config.toml values without modifying the configuration file.

### Secrets vs Configuration

**Secrets** (sensitive data that must be protected):
- JWT signing keys
- Encryption keys for data at rest
- API keys (LLM providers, AWS, etc.)
- Database credentials
- SMTP/email credentials
- SMS API credentials

**Configuration** (non-sensitive settings):
- Server ports and timeouts
- Feature flags
- Database connection strings (without credentials)
- Logging levels
- Resource limits

## Kubernetes Secret Management

### Required Secrets

The following secrets must be configured before deploying to production:

| Secret Name | Environment Variable | Purpose | Generation Method |
|-------------|---------------------|---------|-------------------|
| JWT Secret | `JWT_SECRET` | Signs and verifies JWT tokens | `openssl rand -base64 32` |
| Encryption Key | `ENCRYPTION_KEY` | Encrypts message content at rest (AES-256) | `openssl rand -base64 32` |
| S3 Access Key | `S3_ACCESS_KEY_ID` | AWS S3 file storage access | AWS IAM Console |
| S3 Secret Key | `S3_SECRET_ACCESS_KEY` | AWS S3 file storage secret | AWS IAM Console |
| SMTP User | `SMTP_USER` | Email notification authentication | Email provider |
| SMTP Password | `SMTP_PASS` | Email notification authentication | Email provider |
| SMS Account SID | `SMS_ACCOUNT_SID` | Twilio SMS account identifier | Twilio Console |
| SMS Auth Token | `SMS_AUTH_TOKEN` | Twilio SMS authentication | Twilio Console |
| LLM API Keys | `LLM_PROVIDER_1_API_KEY`, `LLM_PROVIDER_2_API_KEY`, etc. | LLM provider authentication | Provider dashboards |

### Creating Kubernetes Secrets

#### Method 1: Using kubectl (Recommended for Development)

```bash
# Generate strong secrets
JWT_SECRET=$(openssl rand -base64 32)
ENCRYPTION_KEY=$(openssl rand -base64 32)

# Create the secret
kubectl create secret generic chat-secrets \
  --from-literal=JWT_SECRET="$JWT_SECRET" \
  --from-literal=ENCRYPTION_KEY="$ENCRYPTION_KEY" \
  --from-literal=S3_ACCESS_KEY_ID="your-s3-access-key" \
  --from-literal=S3_SECRET_ACCESS_KEY="your-s3-secret-key" \
  --from-literal=SMTP_USER="smtp-username" \
  --from-literal=SMTP_PASS="smtp-password" \
  --from-literal=SMS_ACCOUNT_SID="ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX" \
  --from-literal=SMS_AUTH_TOKEN="your-twilio-auth-token" \
  --from-literal=LLM_PROVIDER_1_API_KEY="sk-your-openai-api-key" \
  --from-literal=LLM_PROVIDER_2_API_KEY="sk-ant-your-anthropic-api-key" \
  --from-literal=LLM_PROVIDER_3_API_KEY="your-dify-api-key" \
  --namespace=default
```

#### Method 2: Using YAML Manifest (Not Recommended for Production)

**WARNING**: Never commit the secret.yaml file with real values to source control!

```bash
# Edit the secret.yaml file with real values
vi deployments/kubernetes/secret.yaml

# Apply the secret
kubectl apply -f deployments/kubernetes/secret.yaml
```

#### Method 3: Using External Secret Manager (Recommended for Production)

For production environments, use an external secret manager like:

- **AWS Secrets Manager** with [External Secrets Operator](https://external-secrets.io/)
- **HashiCorp Vault** with [Vault Agent Injector](https://www.vaultproject.io/docs/platform/k8s/injector)
- **Google Secret Manager** with [Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)
- **Azure Key Vault** with [Secrets Store CSI Driver](https://secrets-store-csi-driver.sigs.k8s.io/)

Example using External Secrets Operator with AWS Secrets Manager:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: chat-secrets
  namespace: default
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secrets-manager
    kind: SecretStore
  target:
    name: chat-secrets
    creationPolicy: Owner
  data:
  - secretKey: JWT_SECRET
    remoteRef:
      key: chatbox/production
      property: jwt_secret
  - secretKey: ENCRYPTION_KEY
    remoteRef:
      key: chatbox/production
      property: encryption_key
  - secretKey: S3_ACCESS_KEY_ID
    remoteRef:
      key: chatbox/production
      property: s3_access_key_id
  - secretKey: S3_SECRET_ACCESS_KEY
    remoteRef:
      key: chatbox/production
      property: s3_secret_access_key
  # ... add all other secrets
```

### Verifying Secret Configuration

```bash
# Check if secret exists
kubectl get secret chat-secrets -n default

# View secret keys (not values)
kubectl describe secret chat-secrets -n default

# Verify secret is mounted in pod
kubectl exec -it <pod-name> -n default -- env | grep -E "JWT_SECRET|ENCRYPTION_KEY"
```

## Application Code Changes

The following code changes ensure environment variables take priority over config.toml:

### 1. JWT Secret (chatbox.go)

```go
// Priority: Environment variable > Config file
var err error
jwtSecret := os.Getenv("JWT_SECRET")
if jwtSecret == "" {
    jwtSecret, err = config.ConfigString("chatbox.jwt_secret")
    if err != nil {
        return fmt.Errorf("failed to get JWT secret: %w", err)
    }
}
```

### 2. Encryption Key (chatbox.go)

```go
// Priority: Environment variable > Config file
encryptionKeyStr := os.Getenv("ENCRYPTION_KEY")
if encryptionKeyStr == "" {
    encryptionKeyStr, err = config.ConfigStringWithDefault("chatbox.encryption_key", "")
    if err != nil {
        return fmt.Errorf("failed to get encryption key: %w", err)
    }
}
```

### 3. LLM API Keys (internal/llm/llm.go)

```go
// Override API key from environment variable if available
// Format: LLM_PROVIDER_<INDEX>_API_KEY (e.g., LLM_PROVIDER_1_API_KEY)
envKey := fmt.Sprintf("LLM_PROVIDER_%d_API_KEY", i+1)
if envAPIKey := os.Getenv(envKey); envAPIKey != "" {
    provider.APIKey = envAPIKey
}
```

### 4. SMS Credentials (internal/notification/notification.go)

```go
// Priority: Environment variables > Config file
accountSID := os.Getenv("SMS_ACCOUNT_SID")
if accountSID == "" {
    accountSID, err = config.ConfigString("sms.accountSID")
}

authToken := os.Getenv("SMS_AUTH_TOKEN")
if authToken == "" {
    authToken, err = config.ConfigString("sms.authToken")
}
```

## Security Best Practices

### 1. Secret Generation

Always use cryptographically secure random generation:

```bash
# Generate 32-byte keys for JWT and encryption
openssl rand -base64 32

# Or use /dev/urandom
head -c 32 /dev/urandom | base64
```

### 2. Secret Rotation

Implement a secret rotation strategy:

1. **Generate new secret** in secret manager
2. **Update Kubernetes secret** with new value
3. **Rolling restart** pods to pick up new secret
4. **Verify** all pods are using new secret
5. **Revoke old secret** after grace period

```bash
# Update secret
kubectl create secret generic chat-secrets \
  --from-literal=JWT_SECRET="new-secret-value" \
  --dry-run=client -o yaml | kubectl apply -f -

# Rolling restart
kubectl rollout restart deployment chatbox-websocket -n default

# Monitor rollout
kubectl rollout status deployment chatbox-websocket -n default
```

### 3. Access Control

Restrict access to secrets using Kubernetes RBAC:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: secret-reader
  namespace: default
rules:
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["chat-secrets"]
  verbs: ["get"]
```

### 4. Encryption at Rest

Enable encryption at rest for Kubernetes secrets:

```yaml
# /etc/kubernetes/encryption-config.yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
    - secrets
    providers:
    - aescbc:
        keys:
        - name: key1
          secret: <base64-encoded-secret>
    - identity: {}
```

### 5. Audit Logging

Enable audit logging for secret access:

```yaml
# audit-policy.yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: RequestResponse
  resources:
  - group: ""
    resources: ["secrets"]
```

## Troubleshooting

### Secret Not Found

```bash
# Check if secret exists
kubectl get secret chat-secrets -n default

# If not found, create it
kubectl create secret generic chat-secrets --from-literal=JWT_SECRET="temp-value"
```

### Pod Can't Read Secret

```bash
# Check pod events
kubectl describe pod <pod-name> -n default

# Check if secret is mounted
kubectl exec -it <pod-name> -n default -- env | grep JWT_SECRET

# Verify secret reference in deployment
kubectl get deployment chatbox-websocket -o yaml | grep -A 10 secretKeyRef
```

### Application Not Using Environment Variable

```bash
# Check if environment variable is set in pod
kubectl exec -it <pod-name> -n default -- printenv | grep -E "JWT_SECRET|ENCRYPTION_KEY"

# Check application logs for configuration loading
kubectl logs <pod-name> -n default | grep -i "secret\|encryption"
```

### Secret Value Incorrect

```bash
# Decode secret value (be careful - this exposes the secret!)
kubectl get secret chat-secrets -n default -o jsonpath='{.data.JWT_SECRET}' | base64 -d

# Update secret value
kubectl create secret generic chat-secrets \
  --from-literal=JWT_SECRET="correct-value" \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart pods to pick up new value
kubectl rollout restart deployment chatbox-websocket -n default
```

## Migration from Config File to Kubernetes Secrets

If you're migrating from config.toml to Kubernetes secrets:

1. **Extract secrets from config.toml**:
   ```bash
   grep -E "jwt_secret|encryption_key|apiKey|pass|token" config.toml
   ```

2. **Create Kubernetes secret** with extracted values

3. **Update config.toml** with placeholder values:
   ```toml
   jwt_secret = "PLACEHOLDER_JWT_SECRET"
   encryption_key = "PLACEHOLDER_ENCRYPTION_KEY"
   ```

4. **Deploy application** - it will use Kubernetes secrets

5. **Verify** secrets are loaded from environment variables:
   ```bash
   kubectl logs <pod-name> | grep -i "loaded.*environment"
   ```

## References

- [Kubernetes Secrets Documentation](https://kubernetes.io/docs/concepts/configuration/secret/)
- [External Secrets Operator](https://external-secrets.io/)
- [KEY_MANAGEMENT.md](./KEY_MANAGEMENT.md) - Encryption key management guide
- [DEPLOYMENT.md](./DEPLOYMENT.md) - General deployment guide
- [deployments/kubernetes/README.md](./deployments/kubernetes/README.md) - Kubernetes-specific deployment guide

## Support

For questions or issues with secret management:

1. Check this documentation
2. Review application logs: `kubectl logs -l app=chatbox`
3. Verify secret configuration: `kubectl describe secret chat-secrets`
4. Contact DevOps team
