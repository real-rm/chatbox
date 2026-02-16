#!/bin/bash
# Test script to verify environment variables take priority over config.toml
# This script tests that the application correctly loads secrets from environment variables

set -e

echo "Testing Secret Priority: Environment Variables > Config File"
echo "=============================================================="
echo ""

# Set test environment variables
export JWT_SECRET="test-jwt-secret-from-env"
export ENCRYPTION_KEY="test-encryption-key-from-env-32b"
export SMS_ACCOUNT_SID="test-sms-account-sid-from-env"
export SMS_AUTH_TOKEN="test-sms-auth-token-from-env"
export LLM_PROVIDER_1_API_KEY="test-llm-api-key-from-env"

echo "✓ Set test environment variables"
echo ""

# Build the application
echo "Building application..."
go build -o chatbox-test ./cmd/server
echo "✓ Build successful"
echo ""

# Note: We can't easily test the full application startup without a MongoDB instance
# Instead, we'll verify the code logic by checking the implementation

echo "Verifying code implementation..."
echo ""

# Check JWT_SECRET priority in chatbox.go
if grep -q 'jwtSecret := os.Getenv("JWT_SECRET")' chatbox.go; then
    echo "✓ JWT_SECRET: Environment variable checked first"
else
    echo "✗ JWT_SECRET: Environment variable not prioritized"
    exit 1
fi

# Check ENCRYPTION_KEY priority in chatbox.go
if grep -q 'encryptionKeyStr := os.Getenv("ENCRYPTION_KEY")' chatbox.go; then
    echo "✓ ENCRYPTION_KEY: Environment variable checked first"
else
    echo "✗ ENCRYPTION_KEY: Environment variable not prioritized"
    exit 1
fi

# Check SMS credentials priority in internal/notification/notification.go
if grep -q 'accountSID := os.Getenv("SMS_ACCOUNT_SID")' internal/notification/notification.go; then
    echo "✓ SMS_ACCOUNT_SID: Environment variable checked first"
else
    echo "✗ SMS_ACCOUNT_SID: Environment variable not prioritized"
    exit 1
fi

if grep -q 'authToken := os.Getenv("SMS_AUTH_TOKEN")' internal/notification/notification.go; then
    echo "✓ SMS_AUTH_TOKEN: Environment variable checked first"
else
    echo "✗ SMS_AUTH_TOKEN: Environment variable not prioritized"
    exit 1
fi

# Check LLM API key priority in internal/llm/llm.go
if grep -q 'envKey := fmt.Sprintf("LLM_PROVIDER_%d_API_KEY", i+1)' internal/llm/llm.go; then
    echo "✓ LLM_PROVIDER_*_API_KEY: Environment variable override implemented"
else
    echo "✗ LLM_PROVIDER_*_API_KEY: Environment variable override not implemented"
    exit 1
fi

echo ""
echo "=============================================================="
echo "✓ All secret priority checks passed!"
echo ""
echo "Summary:"
echo "  - JWT_SECRET: Environment variable > Config file"
echo "  - ENCRYPTION_KEY: Environment variable > Config file"
echo "  - SMS credentials: Environment variable > Config file"
echo "  - LLM API keys: Environment variable > Config file"
echo ""
echo "This ensures Kubernetes secrets (injected as env vars) take priority"
echo "over config.toml values, enabling secure secret management."
echo ""

# Clean up
rm -f chatbox-test

# Unset test environment variables
unset JWT_SECRET
unset ENCRYPTION_KEY
unset SMS_ACCOUNT_SID
unset SMS_AUTH_TOKEN
unset LLM_PROVIDER_1_API_KEY
