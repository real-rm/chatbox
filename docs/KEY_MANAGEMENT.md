# Encryption Key Management Guide

## Overview

This document provides comprehensive guidance on managing encryption keys for the chatbox WebSocket application. The application uses AES-256-GCM encryption to protect sensitive message content at rest in MongoDB.

## Table of Contents

1. [Encryption Overview](#encryption-overview)
2. [Key Generation](#key-generation)
3. [Development Environment](#development-environment)
4. [Production Environment](#production-environment)
5. [Key Rotation](#key-rotation)
6. [Backup and Recovery](#backup-and-recovery)
7. [Security Best Practices](#security-best-practices)
8. [Troubleshooting](#troubleshooting)

## Encryption Overview

### Algorithm Details

- **Algorithm**: AES-256-GCM (Advanced Encryption Standard with Galois/Counter Mode)
- **Key Size**: 32 bytes (256 bits)
- **Nonce**: 12 bytes, randomly generated for each encryption operation
- **Encoding**: Base64 encoding for storage in MongoDB
- **Authentication**: GCM mode provides both confidentiality and authenticity

### What Gets Encrypted

- **Message Content**: All message content stored in MongoDB is encrypted
- **Not Encrypted**: Message metadata (timestamps, sender, file IDs) remains unencrypted for query performance

### Implementation Location

- **Encryption/Decryption**: `internal/storage/storage.go`
- **Key Loading**: `chatbox.go` (lines 83-112)
- **Configuration**: `config.toml` or Kubernetes secrets

## Key Generation

### Generate a Secure Key

Use OpenSSL to generate a cryptographically secure 32-byte key:

```bash
# Generate base64-encoded 32-byte key
openssl rand -base64 32
```

Example output:
```
7xK9mP2nQ5vL8wR3tY6uI1oA4sD7fG9hJ2kL5mN8pQ0=
```

### Key Requirements

- **Length**: Exactly 32 bytes (256 bits) for AES-256
- **Randomness**: Must be cryptographically random (use `openssl rand`, not manual generation)
- **Uniqueness**: Each environment (dev, staging, production) should have a unique key
- **Secrecy**: Never commit keys to version control or share via insecure channels

### Alternative Key Sizes

The implementation also supports:
- **AES-128**: 16 bytes (128 bits)
- **AES-192**: 24 bytes (192 bits)

However, **AES-256 (32 bytes) is strongly recommended** for production use.

## Development Environment

### Local Development with config.toml

For local development, you can use config.toml:

```toml
[chatbox]
# Encryption key for message content at rest (must be 32 bytes for AES-256)
# IMPORTANT: Change this in production and store securely
# Generate with: openssl rand -base64 32
encryption_key = "CHANGE-ME-32-BYTE-KEY-FOR-AES256"
```

**Steps:**

1. Generate a development key:
   ```bash
   openssl rand -base64 32
   ```

2. Update `config.toml`:
   ```toml
   encryption_key = "your-generated-key-here"
   ```

3. Restart the application

### Docker Compose Development

For Docker Compose environments, use environment variables:

```yaml
# docker-compose.yml
services:
  chatbox:
    environment:
      - ENCRYPTION_KEY=${ENCRYPTION_KEY}
```

Create a `.env` file (add to `.gitignore`):
```bash
# .env
ENCRYPTION_KEY=your-generated-key-here
```

## Production Environment

### Kubernetes Secrets (Recommended)

**Never store production keys in config.toml or version control!**

#### Step 1: Create Kubernetes Secret

```bash
# Generate key
ENCRYPTION_KEY=$(openssl rand -base64 32)

# Create secret
kubectl create secret generic chat-secrets \
  --from-literal=ENCRYPTION_KEY="$ENCRYPTION_KEY" \
  --namespace=chatbox \
  --dry-run=client -o yaml | kubectl apply -f -
```

#### Step 2: Update Deployment

Add the encryption key to your deployment manifest:

```yaml
# deployments/kubernetes/deployment.yaml
spec:
  template:
    spec:
      containers:
      - name: chatbox
        env:
        # ... existing environment variables ...
        - name: ENCRYPTION_KEY
          valueFrom:
            secretKeyRef:
              name: chat-secrets
              key: ENCRYPTION_KEY
```

#### Step 3: Update Application Code

Modify `chatbox.go` to read from environment variable:

```go
// Load encryption key from environment or config
encryptionKeyStr := os.Getenv("ENCRYPTION_KEY")
if encryptionKeyStr == "" {
    encryptionKeyStr, err = config.ConfigStringWithDefault("chatbox.encryption_key", "")
    if err != nil {
        return fmt.Errorf("failed to get encryption key: %w", err)
    }
}
```

### AWS Secrets Manager

For AWS deployments, use AWS Secrets Manager with External Secrets Operator:

#### Step 1: Store Key in AWS Secrets Manager

```bash
# Create secret in AWS
aws secretsmanager create-secret \
  --name chatbox/encryption-key \
  --secret-string "$(openssl rand -base64 32)" \
  --region us-east-1
```

#### Step 2: Install External Secrets Operator

```bash
# Install External Secrets Operator
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets \
  external-secrets/external-secrets \
  -n external-secrets-system \
  --create-namespace
```

#### Step 3: Create SecretStore and ExternalSecret

```yaml
# secretstore.yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secrets-manager
  namespace: chatbox
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
      auth:
        jwt:
          serviceAccountRef:
            name: chatbox-sa
---
# externalsecret.yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: chatbox-encryption-key
  namespace: chatbox
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secrets-manager
    kind: SecretStore
  target:
    name: chat-secrets
    creationPolicy: Owner
  data:
  - secretKey: ENCRYPTION_KEY
    remoteRef:
      key: chatbox/encryption-key
```

### HashiCorp Vault

For HashiCorp Vault integration:

#### Step 1: Store Key in Vault

```bash
# Store key in Vault
vault kv put secret/chatbox/encryption-key \
  value="$(openssl rand -base64 32)"
```

#### Step 2: Configure Vault Agent Injector

```yaml
# deployment.yaml
metadata:
  annotations:
    vault.hashicorp.com/agent-inject: "true"
    vault.hashicorp.com/role: "chatbox"
    vault.hashicorp.com/agent-inject-secret-encryption-key: "secret/data/chatbox/encryption-key"
    vault.hashicorp.com/agent-inject-template-encryption-key: |
      {{- with secret "secret/data/chatbox/encryption-key" -}}
      export ENCRYPTION_KEY="{{ .Data.data.value }}"
      {{- end }}
```

## Key Rotation

### Why Rotate Keys?

- **Security Best Practice**: Regular rotation limits exposure if a key is compromised
- **Compliance**: Many regulations require periodic key rotation
- **Incident Response**: Rotate immediately if a key is suspected to be compromised

### Rotation Strategy

#### Option 1: Dual-Key Approach (Recommended)

Support both old and new keys during transition:

1. **Add New Key**: Deploy application with both old and new keys
2. **Re-encrypt Data**: Background job re-encrypts all messages with new key
3. **Remove Old Key**: After all data is re-encrypted, remove old key

**Implementation:**

```go
// storage.go - Support multiple keys
type StorageService struct {
    encryptionKeys [][]byte // Multiple keys, newest first
    // ...
}

// Try decryption with each key
func (s *StorageService) decrypt(ciphertext string) (string, error) {
    for _, key := range s.encryptionKeys {
        plaintext, err := s.decryptWithKey(ciphertext, key)
        if err == nil {
            return plaintext, nil
        }
    }
    return "", errors.New("decryption failed with all keys")
}
```

#### Option 2: Maintenance Window Approach

For smaller datasets, use a maintenance window:

1. **Schedule Downtime**: Announce maintenance window
2. **Export Data**: Export all messages, decrypt with old key
3. **Update Key**: Deploy new encryption key
4. **Import Data**: Re-encrypt and import messages with new key
5. **Resume Service**: Bring application back online

### Rotation Procedure

```bash
# 1. Generate new key
NEW_KEY=$(openssl rand -base64 32)

# 2. Update Kubernetes secret with both keys
kubectl create secret generic chat-secrets-new \
  --from-literal=ENCRYPTION_KEY="$NEW_KEY" \
  --from-literal=ENCRYPTION_KEY_OLD="$OLD_KEY" \
  --namespace=chatbox \
  --dry-run=client -o yaml | kubectl apply -f -

# 3. Deploy application with dual-key support
kubectl rollout restart deployment/chatbox-websocket -n chatbox

# 4. Run re-encryption job (implement as needed)
kubectl apply -f re-encryption-job.yaml

# 5. Monitor re-encryption progress
kubectl logs -f job/re-encryption-job -n chatbox

# 6. After completion, remove old key
kubectl create secret generic chat-secrets \
  --from-literal=ENCRYPTION_KEY="$NEW_KEY" \
  --namespace=chatbox \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Recommended Rotation Schedule

- **Production**: Every 90 days
- **Staging**: Every 180 days
- **Development**: Annually or as needed

## Backup and Recovery

### Backup Encryption Keys

**Critical**: Store encryption keys securely for disaster recovery.

#### Option 1: Encrypted Backup

```bash
# Export key from Kubernetes
kubectl get secret chat-secrets -n chatbox -o jsonpath='{.data.ENCRYPTION_KEY}' | base64 -d > encryption-key.txt

# Encrypt backup with GPG
gpg --symmetric --cipher-algo AES256 encryption-key.txt

# Store encrypted file in secure location
# - Offline storage (USB drive in safe)
# - Encrypted cloud storage (AWS S3 with encryption)
# - Password manager (1Password, LastPass)

# Delete plaintext file
shred -u encryption-key.txt
```

#### Option 2: Secret Sharing (Shamir's Secret Sharing)

Split key among multiple trusted parties:

```bash
# Install ssss (Shamir's Secret Sharing Scheme)
# Ubuntu/Debian: apt-get install ssss
# macOS: brew install ssss

# Split key into 5 shares, requiring 3 to reconstruct
kubectl get secret chat-secrets -n chatbox -o jsonpath='{.data.ENCRYPTION_KEY}' | \
  base64 -d | \
  ssss-split -t 3 -n 5

# Distribute shares to 5 different trusted parties
# Any 3 parties can reconstruct the key
```

### Recovery Procedure

#### Scenario 1: Key Lost, Backup Available

```bash
# Decrypt backup
gpg encryption-key.txt.gpg

# Restore to Kubernetes
kubectl create secret generic chat-secrets \
  --from-literal=ENCRYPTION_KEY="$(cat encryption-key.txt)" \
  --namespace=chatbox \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart application
kubectl rollout restart deployment/chatbox-websocket -n chatbox

# Clean up
shred -u encryption-key.txt
```

#### Scenario 2: Key Lost, No Backup (Data Loss)

**If the encryption key is permanently lost, encrypted data cannot be recovered.**

1. **Accept Data Loss**: Encrypted messages are unrecoverable
2. **Generate New Key**: Create new encryption key
3. **Clear Database**: Optionally clear old encrypted data
4. **Resume Service**: Deploy with new key

```bash
# Generate new key
NEW_KEY=$(openssl rand -base64 32)

# Update secret
kubectl create secret generic chat-secrets \
  --from-literal=ENCRYPTION_KEY="$NEW_KEY" \
  --namespace=chatbox \
  --dry-run=client -o yaml | kubectl apply -f -

# Optionally clear old data
kubectl exec -it mongodb-pod -n chatbox -- mongo chat --eval "db.sessions.deleteMany({})"

# Restart application
kubectl rollout restart deployment/chatbox-websocket -n chatbox
```

## Security Best Practices

### Key Storage

✅ **DO:**
- Store keys in Kubernetes secrets, AWS Secrets Manager, or HashiCorp Vault
- Use environment variables for key injection
- Encrypt key backups with strong passwords
- Use hardware security modules (HSM) for high-security environments
- Implement access controls (RBAC) for secret access

❌ **DON'T:**
- Commit keys to version control (Git)
- Store keys in config files in production
- Share keys via email, Slack, or other insecure channels
- Use weak or predictable keys
- Reuse keys across environments

### Access Control

```bash
# Create service account with minimal permissions
kubectl create serviceaccount chatbox-sa -n chatbox

# Create role for secret access
kubectl create role secret-reader \
  --verb=get \
  --resource=secrets \
  --resource-name=chat-secrets \
  -n chatbox

# Bind role to service account
kubectl create rolebinding chatbox-secret-reader \
  --role=secret-reader \
  --serviceaccount=chatbox:chatbox-sa \
  -n chatbox

# Update deployment to use service account
kubectl patch deployment chatbox-websocket \
  -n chatbox \
  -p '{"spec":{"template":{"spec":{"serviceAccountName":"chatbox-sa"}}}}'
```

### Audit Logging

Enable audit logging for secret access:

```yaml
# audit-policy.yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: RequestResponse
  verbs: ["get", "list", "watch"]
  resources:
  - group: ""
    resources: ["secrets"]
  namespaces: ["chatbox"]
```

### Key Validation

Implement key validation on startup:

```go
// chatbox.go
func validateEncryptionKey(key []byte) error {
    if len(key) == 0 {
        return errors.New("encryption key is empty")
    }
    if len(key) != 32 && len(key) != 24 && len(key) != 16 {
        return fmt.Errorf("invalid key length: %d (expected 16, 24, or 32)", len(key))
    }
    // Test encryption/decryption
    testData := "test-encryption-validation"
    encrypted, err := encrypt(testData, key)
    if err != nil {
        return fmt.Errorf("encryption test failed: %w", err)
    }
    decrypted, err := decrypt(encrypted, key)
    if err != nil {
        return fmt.Errorf("decryption test failed: %w", err)
    }
    if decrypted != testData {
        return errors.New("encryption round-trip validation failed")
    }
    return nil
}
```

## Troubleshooting

### Issue: Decryption Failures

**Symptoms:**
- Error logs: "failed to decrypt: ..."
- Messages appear as base64 gibberish

**Causes:**
1. Wrong encryption key configured
2. Key was rotated but old data not re-encrypted
3. Data corruption

**Solutions:**

```bash
# 1. Verify key is correct
kubectl get secret chat-secrets -n chatbox -o jsonpath='{.data.ENCRYPTION_KEY}' | base64 -d

# 2. Check application logs
kubectl logs -n chatbox -l app=chatbox | grep -i "encrypt"

# 3. Test encryption/decryption
kubectl exec -it chatbox-websocket-xxxxx -n chatbox -- sh
# Run test encryption in application

# 4. If key was rotated, restore old key temporarily
kubectl create secret generic chat-secrets \
  --from-literal=ENCRYPTION_KEY="$OLD_KEY" \
  --namespace=chatbox \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Issue: Key Not Loaded

**Symptoms:**
- Warning log: "No encryption key configured, messages will be stored unencrypted"
- Messages stored in plaintext

**Causes:**
1. Secret not created
2. Environment variable not configured
3. Application not reading from correct source

**Solutions:**

```bash
# 1. Verify secret exists
kubectl get secret chat-secrets -n chatbox

# 2. Verify secret has ENCRYPTION_KEY
kubectl describe secret chat-secrets -n chatbox

# 3. Verify deployment references secret
kubectl get deployment chatbox-websocket -n chatbox -o yaml | grep -A 10 "env:"

# 4. Check pod environment
kubectl exec -it chatbox-websocket-xxxxx -n chatbox -- env | grep ENCRYPTION
```

### Issue: Performance Degradation

**Symptoms:**
- Slow message retrieval
- High CPU usage

**Causes:**
- Encryption/decryption overhead on large datasets

**Solutions:**

```bash
# 1. Monitor CPU usage
kubectl top pods -n chatbox -l app=chatbox

# 2. Check message volume
kubectl exec -it mongodb-pod -n chatbox -- mongo chat --eval "db.sessions.count()"

# 3. Consider caching decrypted messages (with caution)
# 4. Optimize queries to reduce decryption operations
# 5. Scale horizontally to distribute load
kubectl scale deployment chatbox-websocket --replicas=5 -n chatbox
```

### Issue: Key Rotation Failed

**Symptoms:**
- Some messages decrypt, others don't
- Mixed encrypted/unencrypted data

**Solutions:**

```bash
# 1. Implement dual-key support (see Key Rotation section)
# 2. Run re-encryption job to completion
# 3. Verify all messages are re-encrypted before removing old key

# Check encryption status (implement in application)
kubectl exec -it chatbox-websocket-xxxxx -n chatbox -- \
  /app/chatbox-cli check-encryption-status
```

## Compliance and Regulations

### GDPR Considerations

- **Right to Erasure**: Ensure encrypted messages can be deleted
- **Data Portability**: Provide decrypted exports for users
- **Breach Notification**: If key is compromised, notify within 72 hours

### HIPAA Considerations

- **Encryption at Rest**: ✅ Implemented with AES-256-GCM
- **Access Controls**: Implement RBAC for key access
- **Audit Logging**: Enable audit logs for secret access
- **Key Management**: Document key lifecycle

### PCI DSS Considerations

- **Strong Cryptography**: ✅ AES-256 meets requirements
- **Key Rotation**: Rotate keys at least annually
- **Key Storage**: Use HSM or equivalent for key storage
- **Access Restrictions**: Limit key access to authorized personnel

## Additional Resources

- [NIST Key Management Guidelines](https://csrc.nist.gov/publications/detail/sp/800-57-part-1/rev-5/final)
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)
- [Kubernetes Secrets Management](https://kubernetes.io/docs/concepts/configuration/secret/)
- [AWS Secrets Manager Best Practices](https://docs.aws.amazon.com/secretsmanager/latest/userguide/best-practices.html)

## Support

For key management issues:

1. **Check Logs**: `kubectl logs -n chatbox -l app=chatbox | grep -i encrypt`
2. **Verify Configuration**: Review this document
3. **Test Encryption**: Run encryption round-trip tests
4. **Contact Security Team**: For key compromise or rotation assistance

---

**Document Version**: 1.0  
**Last Updated**: 2024  
**Owner**: DevOps/Security Team
