# Quick Fix: CI Authentication Error

## The Error
```
fatal: could not read Password for 'https://***@github.com': terminal prompts disabled
```

## The Fix (2 minutes)

### Step 1: Create Token
1. Go to https://github.com/settings/tokens
2. Click "Generate new token (classic)"
3. Name it: "CI Docker Build"
4. Select scope: ✅ `repo`
5. Click "Generate token"
6. **Copy the token immediately**

### Step 2: Add to Repository
1. Go to your repo → Settings → Secrets and variables → Actions
2. Click "New repository secret"
3. Name: `GO_MODULES_TOKEN`
4. Value: Paste your token
5. Click "Add secret"

### Step 3: Test
```bash
git commit --allow-empty -m "Test CI"
git push
```

Check the Actions tab - build should now succeed!

## Need More Details?
See [docs/CI_GITHUB_TOKEN_SETUP.md](../docs/CI_GITHUB_TOKEN_SETUP.md)
