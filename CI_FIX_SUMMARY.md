# GitHub CI Docker Build Fix

## Problem

GitHub Actions CI was failing during Docker build with the error:
```
fatal: could not read Password for 'https://***@github.com': terminal prompts disabled
```

This occurred when trying to download private Go modules from `github.com/real-rm/*`.

## Root Cause

The default `secrets.GITHUB_TOKEN` in GitHub Actions doesn't have access to private repositories, even within the same organization. Go's module system needs proper Git authentication to download private dependencies.

## Solution Implemented

### 1. Updated Dockerfile
- Added `git config --global credential.helper store` to properly configure Git credentials
- Moved `GOPRIVATE` environment variable before Git configuration
- Added clearer comments about token requirements

**File**: `Dockerfile`

### 2. Updated GitHub Actions Workflow
- Added explicit permissions for `contents: read` and `packages: read`
- Changed to use `GO_MODULES_TOKEN` secret with fallback to `GITHUB_TOKEN`
- Uses environment variable to avoid exposing token in logs

**File**: `.github/workflows/docker-build.yml`

### 3. Created Comprehensive Documentation
- New guide: `docs/CI_GITHUB_TOKEN_SETUP.md` with step-by-step token setup
- Updated: `docs/CI_SETUP.md` to reference the new guide
- Updated: `README.md` with troubleshooting section

## Required Action

To fix the CI build, you need to:

1. **Create a Personal Access Token (PAT)**:
   - Go to GitHub Settings → Developer settings → Personal access tokens
   - Generate new token (classic) with `repo` scope
   - Copy the token

2. **Add as Repository Secret**:
   - Go to Repository Settings → Secrets and variables → Actions
   - Create new secret named `GO_MODULES_TOKEN`
   - Paste the PAT as the value

3. **Trigger CI**:
   ```bash
   git commit --allow-empty -m "Test CI with new token"
   git push
   ```

## Files Changed

- `Dockerfile` - Improved Git credential configuration
- `.github/workflows/docker-build.yml` - Updated to use GO_MODULES_TOKEN
- `docs/CI_GITHUB_TOKEN_SETUP.md` - New comprehensive setup guide
- `docs/CI_SETUP.md` - Updated with reference to new guide
- `README.md` - Added troubleshooting section

## Testing

After adding the `GO_MODULES_TOKEN` secret, the CI build should:
1. Successfully authenticate to GitHub
2. Download all private Go modules
3. Complete the Docker build
4. Create the chatbox-websocket image

## Security Notes

- Token is only used during build (not in final image)
- Token is passed as build argument (not hardcoded)
- Multi-stage build ensures minimal runtime image
- Token should be rotated regularly (every 90 days recommended)

## Alternative Solutions Considered

1. **Using GitHub App**: More complex setup, better for organizations
2. **BuildKit Secrets**: More secure but requires Docker BuildKit
3. **SSH Keys**: Requires more configuration in Dockerfile

The PAT approach was chosen for simplicity and immediate resolution.

## References

- [GitHub Actions: Automatic token authentication](https://docs.github.com/en/actions/security-guides/automatic-token-authentication)
- [Go Modules: Private modules](https://go.dev/ref/mod#private-modules)
- [Docker Build: Build arguments](https://docs.docker.com/engine/reference/commandline/build/#build-arg)
