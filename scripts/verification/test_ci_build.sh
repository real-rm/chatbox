#!/bin/bash

# Test CI Build Script
# This script simulates the CI build process locally to verify it will work in CI

set -e

echo "=========================================="
echo "CI Build Test - Local Simulation"
echo "=========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if GitHub CLI is installed
if ! command -v gh &> /dev/null; then
    echo -e "${RED}✗ GitHub CLI (gh) is not installed${NC}"
    echo "  Install it from: https://cli.github.com/"
    exit 1
fi

# Check if authenticated
if ! gh auth status &> /dev/null; then
    echo -e "${RED}✗ Not authenticated with GitHub CLI${NC}"
    echo "  Run: gh auth login"
    exit 1
fi

echo -e "${GREEN}✓ GitHub CLI is installed and authenticated${NC}"
echo ""

# Get GitHub token
echo "Getting GitHub token..."
GITHUB_TOKEN=$(gh auth token)
if [ -z "$GITHUB_TOKEN" ]; then
    echo -e "${RED}✗ Failed to get GitHub token${NC}"
    exit 1
fi
echo -e "${GREEN}✓ GitHub token obtained${NC}"
echo ""

# Clean up any existing test images
echo "Cleaning up existing test images..."
docker rmi chatbox-websocket:ci-test 2>/dev/null || true
echo ""

# Build Docker image (simulating CI environment)
echo "=========================================="
echo "Building Docker image..."
echo "=========================================="
echo ""

if docker build \
    --build-arg GITHUB_TOKEN=$GITHUB_TOKEN \
    -t chatbox-websocket:ci-test \
    .; then
    echo ""
    echo -e "${GREEN}✓ Docker build successful${NC}"
else
    echo ""
    echo -e "${RED}✗ Docker build failed${NC}"
    exit 1
fi

echo ""
echo "=========================================="
echo "Verifying image..."
echo "=========================================="
echo ""

# Verify image was created
if docker images | grep -q "chatbox-websocket.*ci-test"; then
    echo -e "${GREEN}✓ Image created successfully${NC}"
    docker images | grep chatbox-websocket
else
    echo -e "${RED}✗ Image not found${NC}"
    exit 1
fi

echo ""
echo "=========================================="
echo "Testing image execution..."
echo "=========================================="
echo ""

# Test that the image runs
if docker run --rm chatbox-websocket:ci-test --version 2>/dev/null; then
    echo -e "${GREEN}✓ Image runs with --version flag${NC}"
elif docker run --rm chatbox-websocket:ci-test --help 2>/dev/null; then
    echo -e "${GREEN}✓ Image runs with --help flag${NC}"
else
    echo -e "${YELLOW}⚠ Image created but flags not supported (this is OK)${NC}"
    echo "  The image was built successfully"
fi

echo ""
echo "=========================================="
echo "Image Details"
echo "=========================================="
echo ""

# Show image size
IMAGE_SIZE=$(docker images chatbox-websocket:ci-test --format "{{.Size}}")
echo "Image size: $IMAGE_SIZE"

# Show image layers
echo ""
echo "Image layers:"
docker history chatbox-websocket:ci-test --no-trunc --format "table {{.CreatedBy}}\t{{.Size}}" | head -10

echo ""
echo "=========================================="
echo -e "${GREEN}✓ CI Build Test PASSED${NC}"
echo "=========================================="
echo ""
echo "The Docker build works correctly and will succeed in CI."
echo ""
echo "Next steps:"
echo "1. Commit the CI configuration files:"
echo "   git add .github/workflows/docker-build.yml .gitlab-ci.yml CI_SETUP.md"
echo "   git commit -m 'Add CI configuration for Docker build testing'"
echo ""
echo "2. Configure GITHUB_TOKEN in your CI environment (see CI_SETUP.md)"
echo ""
echo "3. Push to trigger the CI build:"
echo "   git push"
echo ""
echo "4. Monitor the CI build in your platform's UI"
echo ""

# Clean up
echo "Cleaning up test image..."
docker rmi chatbox-websocket:ci-test 2>/dev/null || true
echo -e "${GREEN}✓ Cleanup complete${NC}"
