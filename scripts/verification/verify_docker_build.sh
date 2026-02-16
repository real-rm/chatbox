#!/bin/bash
# Script to verify Docker build works with private GitHub modules
# This script is part of task 6.3 - Verify Docker build works

set -e

echo "=== Docker Build Verification Script ==="
echo ""

# Check if GitHub CLI is available
if ! command -v gh &> /dev/null; then
    echo "ERROR: GitHub CLI (gh) is not installed"
    echo "Install it from: https://cli.github.com/manual/installation"
    exit 1
fi

# Check if authenticated with GitHub
if ! gh auth status &> /dev/null; then
    echo "ERROR: Not authenticated with GitHub CLI"
    echo "Run: gh auth login"
    exit 1
fi

echo "✓ GitHub CLI is installed and authenticated"
echo ""

# Get GitHub token
echo "Getting GitHub token..."
GITHUB_TOKEN=$(gh auth token)
if [ -z "$GITHUB_TOKEN" ]; then
    echo "ERROR: Failed to get GitHub token"
    exit 1
fi
echo "✓ GitHub token obtained"
echo ""

# Build Docker image
echo "Building Docker image..."
echo "This may take a few minutes on first build..."
echo ""

if docker build \
    --build-arg GITHUB_TOKEN=$GITHUB_TOKEN \
    -t chatbox-server:verify \
    . ; then
    echo ""
    echo "✓ Docker build succeeded"
else
    echo ""
    echo "✗ Docker build failed"
    exit 1
fi

echo ""

# Check image was created
if docker images chatbox-server:verify --format "{{.Repository}}:{{.Tag}}" | grep -q "chatbox-server:verify"; then
    IMAGE_SIZE=$(docker images chatbox-server:verify --format "{{.Size}}")
    echo "✓ Docker image created successfully"
    echo "  Image: chatbox-server:verify"
    echo "  Size: $IMAGE_SIZE"
else
    echo "✗ Docker image not found"
    exit 1
fi

echo ""

# Test running the image
echo "Testing Docker image..."
if docker run --rm chatbox-server:verify --help &> /dev/null; then
    echo "✓ Docker image runs successfully"
else
    echo "✗ Docker image failed to run"
    exit 1
fi

echo ""
echo "=== Verification Complete ==="
echo ""
echo "The Docker build works correctly with private GitHub modules."
echo "To use this image:"
echo "  docker run -p 8080:8080 chatbox-server:verify"
echo ""
echo "To clean up the test image:"
echo "  docker rmi chatbox-server:verify"
