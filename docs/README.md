# Chatbox Documentation

This directory contains comprehensive documentation for the Chatbox WebSocket application.

## Quick Start

- [DEPLOYMENT.md](../DEPLOYMENT.md) - Deployment instructions and Docker setup
- [SECRET_SETUP_QUICKSTART.md](SECRET_SETUP_QUICKSTART.md) - Quick guide to setting up secrets

## Production Readiness

- [PRODUCTION_READINESS_PLAN.md](PRODUCTION_READINESS_PLAN.md) - Complete production readiness review
- [PRODUCTION_READINESS_STATUS.md](PRODUCTION_READINESS_STATUS.md) - Current status of production readiness tasks

## Configuration & Setup

- [CI_SETUP.md](CI_SETUP.md) - CI/CD configuration for GitHub Actions and GitLab CI
- [PRIVATE_REGISTRY_SETUP.md](PRIVATE_REGISTRY_SETUP.md) - Private Go module registry configuration
- [SECRET_MANAGEMENT.md](SECRET_MANAGEMENT.md) - Secret management best practices
- [KEY_MANAGEMENT.md](KEY_MANAGEMENT.md) - Encryption key management

## Features & Components

- [CORS_CONFIGURATION.md](CORS_CONFIGURATION.md) - CORS setup for admin endpoints
- [MONGODB_INDEXES.md](MONGODB_INDEXES.md) - Database index configuration
- [WEBSOCKET_ORIGIN_VALIDATION.md](WEBSOCKET_ORIGIN_VALIDATION.md) - WebSocket security configuration
- [ADMIN_NAME_DISPLAY.md](ADMIN_NAME_DISPLAY.md) - Admin takeover name display
- [GRACEFUL_SHUTDOWN.md](GRACEFUL_SHUTDOWN.md) - Graceful shutdown implementation

## Kubernetes Deployment

- [KUBERNETES_DEPLOYMENT_SUMMARY.md](KUBERNETES_DEPLOYMENT_SUMMARY.md) - Kubernetes deployment guide
- [deployments/kubernetes/](../deployments/kubernetes/) - Kubernetes manifests

## Testing

- [TESTING.md](TESTING.md) - Testing strategy and guidelines
- [verification/](verification/) - Test results and verification reports

## Registration & Integration

- [REGISTER.md](REGISTER.md) - Service registration documentation

## Verification Reports

The [verification/](verification/) directory contains detailed test results and verification reports:

- CI/CD build verification
- Docker build verification
- MongoDB field naming tests
- LLM provider integration tests
- Error handling verification
- Encryption verification
- Performance benchmarks
- And more...

## Directory Structure

```
docs/
├── README.md                          # This file
├── verification/                      # Test results and verification reports
├── testing/                          # Testing utilities and scripts
├── PRODUCTION_READINESS_PLAN.md      # Production readiness review
├── PRODUCTION_READINESS_STATUS.md    # Current status
├── CI_SETUP.md                       # CI/CD configuration
├── DEPLOYMENT.md                     # Deployment guide (in root)
└── [feature-specific docs]           # Individual feature documentation
```

## Contributing

When adding new documentation:
1. Place feature-specific docs in the `docs/` directory
2. Place verification/test results in `docs/verification/`
3. Update this README with links to new documentation
4. Follow the existing naming conventions (UPPERCASE_WITH_UNDERSCORES.md)
