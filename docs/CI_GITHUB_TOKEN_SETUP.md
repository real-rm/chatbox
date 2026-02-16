# GitHub CI Token Setup for Private Dependencies

## Problem

The Docker build in GitHub Actions fails when trying to download private Go modules from `github.com/real-rm/*` because the default `GITHUB_TOKEN` doesn't have access to private repositories.

## Solution

You need to create a Personal Access Token (PAT) with appropriate permissions and add it as a repository secret.

## Setup Steps

### 1. Create a Personal Access Token

1. Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Click "Generate new token (classic)"
3. Give it a descriptive name like "CI Docker Build - Private Modules"
4. Set expiration (recommend 90 days or 1 year)
5. Select the following scopes:
   - `repo` (Full control of private repositories)
   - Or at minimum: `repo:status`, `repo_deployment`, `public_repo`, `repo:invite`
6. Click "Generate token"
7. **Copy the token immediately** (you won't be able to see it again)

### 2. Add Token as Repository Secret

1. Go to your repository → Settings → Secrets and variables → Actions
2. Click "New repository secret"
3. Name: `GO_MODULES_TOKEN`
4. Value: Paste the PAT you created
5. Click "Add secret"

### 3. Verify the Workflow

The workflow file `.github/workflows/docker-build.yml` is already configured to use this token:

```yaml
- name: Build Docker image
  run: |
    docker build \
      --build-arg GITHUB_TOKEN=${{ secrets.GO_MODULES_TOKEN }} \
      -t chatbox-websocket:ci-test \
      .
```

## Alternative: Use Fine-Grained PAT (Recommended)

For better security, use a fine-grained personal access token:

1. Go to GitHub Settings → Developer settings → Personal access tokens → Fine-grained tokens
2. Click "Generate new token"
3. Configure:
   - Token name: "CI Docker Build - Private Modules"
   - Expiration: 90 days or as needed
   - Repository access: Select "Only select repositories" and choose the private module repos
   - Permissions:
     - Repository permissions → Contents: Read-only
     - Repository permissions → Metadata: Read-only
4. Generate and copy the token
5. Add as `GO_MODULES_TOKEN` secret in your repository

## Testing

After adding the secret, trigger the workflow:

```bash
git commit --allow-empty -m "Test CI with new token"
git push
```

Check the Actions tab to verify the build succeeds.

## Troubleshooting

### Build still fails with authentication error

1. Verify the secret name matches: `GO_MODULES_TOKEN`
2. Check token hasn't expired
3. Ensure token has `repo` scope or read access to private repos
4. Try regenerating the token

### Token works locally but not in CI

The Dockerfile is configured to use the token via build args. Make sure:
- The secret is added to the repository (not your personal settings)
- The workflow file references the correct secret name
- The repository has access to the private modules

## Security Notes

- Never commit tokens to the repository
- Use fine-grained tokens with minimal permissions when possible
- Rotate tokens regularly
- Consider using GitHub Apps for organization-wide access
