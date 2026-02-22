#!/bin/bash
# Script to create Kubernetes secrets for chatbox application
# Usage: ./create-secrets.sh [namespace]

set -e

NAMESPACE="${1:-default}"

echo "Creating Kubernetes secrets for chatbox application in namespace: $NAMESPACE"
echo "=========================================================================="
echo ""

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "Error: kubectl is not installed or not in PATH"
    exit 1
fi

# Check if openssl is available
if ! command -v openssl &> /dev/null; then
    echo "Error: openssl is not installed or not in PATH"
    exit 1
fi

# Function to prompt for secret value
prompt_secret() {
    local var_name=$1
    local description=$2
    local generate_cmd=$3
    local value=""
    
    echo "---"
    echo "$description"
    if [ -n "$generate_cmd" ]; then
        echo "Generate with: $generate_cmd"
        read -p "Generate automatically? (y/n): " auto_generate
        if [ "$auto_generate" = "y" ] || [ "$auto_generate" = "Y" ]; then
            value=$(eval "$generate_cmd")
            echo "Generated: ${value:0:10}... (truncated for security)"
        else
            read -sp "Enter value: " value
            echo ""
        fi
    else
        read -sp "Enter value: " value
        echo ""
    fi
    
    echo "$value"
}

# Generate or prompt for secrets
echo "Generating/collecting secrets..."
echo ""

JWT_SECRET=$(prompt_secret "JWT_SECRET" "JWT Secret (32+ characters)" "openssl rand -base64 32")
ENCRYPTION_KEY=$(prompt_secret "ENCRYPTION_KEY" "Encryption Key (exactly 32 bytes for AES-256)" "openssl rand -hex 16")

echo ""
echo "AWS S3 Configuration:"
S3_ACCESS_KEY_ID=$(prompt_secret "S3_ACCESS_KEY_ID" "S3 Access Key ID" "")
S3_SECRET_ACCESS_KEY=$(prompt_secret "S3_SECRET_ACCESS_KEY" "S3 Secret Access Key" "")

echo ""
echo "SMTP Configuration:"
SMTP_USER=$(prompt_secret "SMTP_USER" "SMTP Username" "")
SMTP_PASS=$(prompt_secret "SMTP_PASS" "SMTP Password" "")

echo ""
echo "SMS Configuration (Twilio):"
SMS_ACCOUNT_SID=$(prompt_secret "SMS_ACCOUNT_SID" "Twilio Account SID" "")
SMS_AUTH_TOKEN=$(prompt_secret "SMS_AUTH_TOKEN" "Twilio Auth Token" "")

echo ""
echo "LLM Provider API Keys:"
LLM_PROVIDER_1_API_KEY=$(prompt_secret "LLM_PROVIDER_1_API_KEY" "OpenAI API Key" "")
LLM_PROVIDER_2_API_KEY=$(prompt_secret "LLM_PROVIDER_2_API_KEY" "Anthropic API Key" "")
LLM_PROVIDER_3_API_KEY=$(prompt_secret "LLM_PROVIDER_3_API_KEY" "Dify API Key" "")

echo ""
echo "Creating Kubernetes secret..."

# Create the secret
kubectl create secret generic chat-secrets \
  --from-literal=JWT_SECRET="$JWT_SECRET" \
  --from-literal=ENCRYPTION_KEY="$ENCRYPTION_KEY" \
  --from-literal=S3_ACCESS_KEY_ID="$S3_ACCESS_KEY_ID" \
  --from-literal=S3_SECRET_ACCESS_KEY="$S3_SECRET_ACCESS_KEY" \
  --from-literal=SMTP_USER="$SMTP_USER" \
  --from-literal=SMTP_PASS="$SMTP_PASS" \
  --from-literal=SMS_ACCOUNT_SID="$SMS_ACCOUNT_SID" \
  --from-literal=SMS_AUTH_TOKEN="$SMS_AUTH_TOKEN" \
  --from-literal=LLM_PROVIDER_1_API_KEY="$LLM_PROVIDER_1_API_KEY" \
  --from-literal=LLM_PROVIDER_2_API_KEY="$LLM_PROVIDER_2_API_KEY" \
  --from-literal=LLM_PROVIDER_3_API_KEY="$LLM_PROVIDER_3_API_KEY" \
  --namespace="$NAMESPACE" \
  --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "✓ Secret 'chat-secrets' created successfully in namespace '$NAMESPACE'"
echo ""
echo "Verify with:"
echo "  kubectl get secret chat-secrets -n $NAMESPACE"
echo "  kubectl describe secret chat-secrets -n $NAMESPACE"
echo ""
echo "IMPORTANT: Store these secrets securely in a password manager!"
echo "           Never commit them to source control!"
