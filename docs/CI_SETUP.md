# CI/CD Setup for Docker Build Testing

## Overview
This document describes the CI/CD configuration for testing Docker builds in automated environments. This addresses Task 6.4 of the production readiness plan, ensuring the project can be built without local dependencies.

## Purpose
The CI configuration verifies that:
1. Docker builds succeed in a clean environment
2. Private GitHub modules are accessible with proper authentication
3. No local replace directives are required
4. The build process is reproducible across different environments

## CI Platforms Supported

### GitHub Actions
**File**: `.github/workflows/docker-build.yml`

**Features**:
- Triggers on push to main/master/develop branches
- Triggers on pull requests
- Manual workflow dispatch available
- Uses Docker Buildx for optimized builds
- Verifies image creation and basic functionality

**Configuration Required**:
The workflow uses `secrets.GITHUB_TOKEN` which is automatically provided by GitHub Actions. This token has read access to repositories in the same organization.

**For Private Modules in Different Organizations**:
If your private modules are in a different GitHub organization, you need to:
1. Create a Personal Access Token (PAT) with `repo` scope
2. Add it as a repository secret named `GH_PRIVATE_TOKEN`
3. Update the workflow to use `${{ secrets.GH_PRIVATE_TOKEN }}`

### GitLab CI/CD
**File**: `.gitlab-ci.yml`

**Features**:
- Two-stage pipeline: build and test
- Docker-in-Docker (dind) support
- Automatic build on main/master/develop branches
- Manual push to container registry
- Merge request pipeline support

**Configuration Required**:
1. Add `GITHUB_TOKEN` as a CI/CD variable in GitLab:
   - Go to Settings > CI/CD > Variables
   - Add variable: `GITHUB_TOKEN`
   - Value: Your GitHub Personal Access Token
   - Type: Masked (recommended)
   - Protected: Yes (for production branches)

2. Ensure GitLab Runner has Docker executor configured

## GitHub Token Setup

### Quick Start
**See [CI_GITHUB_TOKEN_SETUP.md](CI_GITHUB_TOKEN_SETUP.md) for detailed token setup instructions.**

### For GitHub Actions
The default `GITHUB_TOKEN` may not have access to private repositories. You need to:

1. Create a Personal Access Token (PAT) with `repo` scope
2. Add it as a repository secret named `GO_MODULES_TOKEN`
3. The workflow will automatically use it (already configured)

See [CI_GITHUB_TOKEN_SETUP.md](CI_GITHUB_TOKEN_SETUP.md) for step-by-step instructions.

### For GitLab CI
1. Create a GitHub Personal Access Token:
   - Go to GitHub Settings > Developer settings > Personal access tokens
   - Generate new token (classic)
   - Select scope: `repo` (Full control of private repositories)
   - Copy the token

2. Add to GitLab:
   - Project Settings > CI/CD > Variables
   - Key: `GITHUB_TOKEN`
   - Value: [paste token]
   - Flags: Masked, Protected

### For Other CI Systems
The same approach applies to other CI systems:
1. Create a GitHub PAT with `repo` scope
2. Store it as a secret/variable in your CI system
3. Pass it as a build argument: `--build-arg GITHUB_TOKEN=$GITHUB_TOKEN`

## Testing the CI Configuration

### GitHub Actions
1. Push the workflow file to your repository:
   ```bash
   git add .github/workflows/docker-build.yml
   git commit -m "Add CI workflow for Docker build testing"
   git push
   ```

2. Check the Actions tab in your GitHub repository
3. The workflow should trigger automatically
4. Monitor the build progress and logs

### GitLab CI
1. Push the configuration file:
   ```bash
   git add .gitlab-ci.yml
   git commit -m "Add GitLab CI configuration"
   git push
   ```

2. Go to CI/CD > Pipelines in your GitLab project
3. The pipeline should start automatically
4. Monitor the job logs

### Manual Testing
You can also test the CI build locally using the same commands:

```bash
# Simulate GitHub Actions build
docker build \
  --build-arg GITHUB_TOKEN=$(gh auth token) \
  -t chatbox-websocket:ci-test \
  .

# Verify image
docker images | grep chatbox-websocket

# Test image runs
docker run --rm chatbox-websocket:ci-test --help
```

## Troubleshooting

### Build Fails with "Module Not Found"
**Problem**: CI can't access private GitHub modules

**Solutions**:
1. Verify GITHUB_TOKEN is set in CI environment
2. Check token has `repo` scope
3. Verify token has access to the private repositories
4. Ensure GOPRIVATE is set in Dockerfile (already configured)

### Authentication Errors
**Problem**: Git authentication fails during module download

**Solutions**:
1. Verify the token is correctly passed as build argument
2. Check the Dockerfile git config is correct:
   ```dockerfile
   RUN if [ -n "$GITHUB_TOKEN" ]; then \
           git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"; \
       fi
   ```
3. Ensure the token hasn't expired

### Docker Build Timeout
**Problem**: Build takes too long and times out

**Solutions**:
1. Enable Docker layer caching in CI
2. Use Docker Buildx with cache backends
3. Increase timeout in CI configuration

### Image Runs But Fails Tests
**Problem**: Image builds successfully but doesn't run correctly

**Solutions**:
1. Check the binary is correctly copied in Dockerfile
2. Verify runtime dependencies are included
3. Test locally with same Docker commands
4. Check application configuration requirements

## Security Best Practices

### Token Management
1. ✅ Never commit tokens to version control
2. ✅ Use CI/CD secrets management
3. ✅ Rotate tokens regularly (every 90 days recommended)
4. ✅ Use minimal required scopes (repo access only)
5. ✅ Use masked/protected variables in CI

### Build Security
1. ✅ Token is only used during build (not in final image)
2. ✅ Token is passed as build argument (not hardcoded)
3. ⚠️ Consider using Docker BuildKit secrets for enhanced security
4. ✅ Multi-stage build ensures minimal runtime image

### Enhanced Security with BuildKit Secrets
For production environments, consider using Docker BuildKit secrets:

```yaml
# GitHub Actions example
- name: Build with BuildKit secrets
  run: |
    echo "${{ secrets.GH_PRIVATE_TOKEN }}" > /tmp/github_token
    DOCKER_BUILDKIT=1 docker build \
      --secret id=github_token,src=/tmp/github_token \
      -t chatbox-websocket:ci-test \
      .
    rm /tmp/github_token
```

Update Dockerfile to use secrets:
```dockerfile
# Mount secret instead of build arg
RUN --mount=type=secret,id=github_token \
    GITHUB_TOKEN=$(cat /run/secrets/github_token) && \
    git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"
```

## Verification Checklist

Before marking Task 6.4 complete, verify:

- [ ] CI workflow file is committed to repository
- [ ] GITHUB_TOKEN is configured in CI environment
- [ ] CI build triggers on push/PR
- [ ] Docker build completes successfully in CI
- [ ] Image is created and verified
- [ ] No local dependencies are required
- [ ] Build logs show successful module downloads
- [ ] Documentation is complete and accurate

## Next Steps

After CI is working:
1. Set up automated builds for all branches
2. Configure container registry integration
3. Add image scanning for security vulnerabilities
4. Set up deployment pipelines
5. Configure notifications for build failures

## Related Documentation

- `DOCKER_BUILD_VERIFICATION.md` - Local Docker build verification (Task 6.3)
- `DEPLOYMENT.md` - Deployment instructions and prerequisites
- `docs/PRIVATE_REGISTRY_SETUP.md` - Private module configuration
- `.github/workflows/docker-build.yml` - GitHub Actions workflow
- `.gitlab-ci.yml` - GitLab CI configuration

## Conclusion

The CI configuration ensures that:
✅ Docker builds work in clean environments
✅ Private modules are accessible with proper authentication
✅ No local replace directives are needed
✅ Build process is reproducible and documented

This completes the requirement that "the project can be built by others" and addresses the blocking issue from the production readiness review.
