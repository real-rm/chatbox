# Secret Setup Quick Start Guide

This guide provides a quick reference for setting up secrets in the chatbox application. For comprehensive documentation, see [SECRET_MANAGEMENT.md](../SECRET_MANAGEMENT.md).

## Overview

The chatbox application requires several secrets to operate:
- JWT signing secret
- Encryption key for message content
- LLM API keys (OpenAI, Anthropic, Dify)
- AWS S3 credentials
- SMTP credentials
- SMS credentials (Twilio)

**IMPORTANT**: Never commit secrets to version control!

## Quick Setup (Development)

### 1. Generate Secrets

```bash
# Generate JWT secret (32 bytes)
openssl rand -base64 32

# Generate encryption key (32 bytes for AES-256)
openssl rand -base64 32
```

### 2. Set Environment Variables

```bash
export JWT_SECRET="your-generated-jwt-secret"
export ENCRYPTION_KEY="your-generated-encryption-key"
export LLM_PROVIDER_1_API_KEY="sk-your-openai-api-key"
export S3_ACCESS_KEY_ID="your-s3-access-key"
export S3_SECRET_ACCESS_KEY="your-s3-secret-key"
export SMTP_USER="your-smtp-username"
export SMTP_PASS="your-smtp-password"
```

### 3. Run Application

```bash
go run cmd/server/main.go
```

## Quick Setup (Kubernetes)

### 1. Generate Secrets

```bash
JWT_SECRET=$(openssl rand -base64 32)
ENCRYPTION_KEY=$(openssl rand -base64 32)
```

### 2. Create Kubernetes Secret

```bash
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

### 3. Deploy Application

```bash
kubectl apply -f deployments/kubernetes/configmap.yaml
kubectl apply -f deployments/kubernetes/deployment.yaml
kubectl apply -f deployments/kubernetes/service.yaml
```

### 4. Verify Secrets

```bash
# Check secret exists
kubectl get secret chat-secrets -n default

# Verify secrets are loaded in pods
kubectl exec -it $(kubectl get pod -l app=chatbox -o jsonpath='{.items[0].metadata.name}') \
  -n default -- env | grep -E "JWT_SECRET|ENCRYPTION_KEY"
```

## Secret Priority

The application loads secrets in this priority order:

1. **Environment Variables** (highest priority) ← Kubernetes secrets inject here
2. **Config File** (config.toml) ← Only for development, uses placeholders

This means Kubernetes secrets automatically override config.toml values.

## Required Secrets

| Secret | Environment Variable | Required | Purpose |
|--------|---------------------|----------|---------|
| JWT Secret | `JWT_SECRET` | Yes | Signs and verifies JWT tokens |
| Encryption Key | `ENCRYPTION_KEY` | Yes | Encrypts message content at rest |
| S3 Access Key | `S3_ACCESS_KEY_ID` | Yes | AWS S3 file storage |
| S3 Secret Key | `S3_SECRET_ACCESS_KEY` | Yes | AWS S3 file storage |
| SMTP User | `SMTP_USER` | Yes | Email notifications |
| SMTP Password | `SMTP_PASS` | Yes | Email notifications |
| SMS Account SID | `SMS_ACCOUNT_SID` | Optional | SMS notifications |
| SMS Auth Token | `SMS_AUTH_TOKEN` | Optional | SMS notifications |
| LLM API Keys | `LLM_PROVIDER_*_API_KEY` | Yes | LLM provider access |

## Troubleshooting

### Secret Not Found

```bash
# Check if secret exists
kubectl get secret chat-secrets -n default

# If not found, create it
kubectl create secret generic chat-secrets --from-literal=JWT_SECRET="temp-value"
```

### Application Not Using Secret

```bash
# Check if environment variable is set in pod
kubectl exec -it <pod-name> -n default -- env | grep JWT_SECRET

# Check deployment configuration
kubectl get deployment chatbox-websocket -o yaml | grep -A 10 "env:"
```

### Decryption Failures

```bash
# Verify encryption key is correct
kubectl get secret chat-secrets -n default -o jsonpath='{.data.ENCRYPTION_KEY}' | base64 -d

# Check application logs
kubectl logs -l app=chatbox -n default | grep -i "encrypt"
```

## Production Best Practices

1. **Use External Secret Managers**:
   - AWS Secrets Manager
   - HashiCorp Vault
   - Google Secret Manager
   - Azure Key Vault

2. **Rotate Secrets Regularly**:
   - JWT Secret: Every 90 days
   - Encryption Key: Every 90 days (requires re-encryption)
   - API Keys: Per provider recommendations

3. **Backup Encryption Keys**:
   - Store encrypted backups in secure location
   - Use Shamir's Secret Sharing for critical keys
   - Document recovery procedures

4. **Enable Access Controls**:
   - Use Kubernetes RBAC for secret access
   - Enable audit logging for secret operations
   - Restrict secret access to service accounts

## Additional Documentation

- **[SECRET_MANAGEMENT.md](../SECRET_MANAGEMENT.md)** - Comprehensive secret management guide
- **[KEY_MANAGEMENT.md](../KEY_MANAGEMENT.md)** - Encryption key management and rotation
- **[DEPLOYMENT.md](../DEPLOYMENT.md)** - General deployment guide
- **[deployments/kubernetes/README.md](../deployments/kubernetes/README.md)** - Kubernetes deployment guide

## Support

For secret management issues:

1. Check application logs: `kubectl logs -l app=chatbox`
2. Verify secret configuration: `kubectl describe secret chat-secrets`
3. Review [SECRET_MANAGEMENT.md](../SECRET_MANAGEMENT.md)
4. Contact DevOps team
