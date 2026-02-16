# Private GitHub Registry Setup

## Overview
This document describes the configuration for accessing the company's private Go modules hosted on GitHub under the `github.com/real-rm` organization.

## Configuration

### Environment Variables
The `GOPRIVATE` environment variable has been configured to tell Go that modules under `github.com/real-rm/*` are private and should not be fetched through the public proxy.

**Location:** `~/.zshrc` and `~/.zprofile`

```bash
export GOPRIVATE=github.com/real-rm/*
```

### Git Authentication
Git authentication is configured to use SSH for GitHub access. The system is already authenticated with GitHub via SSH keys.

**Verification:**
```bash
ssh -T git@github.com
# Expected output: Hi [username]! You've successfully authenticated...
```

## Private Modules
The following private modules are available from the real-rm organization:

- `github.com/real-rm/goconfig` - Configuration management
- `github.com/real-rm/golog` - Logging utilities
- `github.com/real-rm/gohelper` - Helper functions
- `github.com/real-rm/gomail` - Email functionality
- `github.com/real-rm/gosms` - SMS functionality
- `github.com/real-rm/goupload` - File upload utilities
- `github.com/real-rm/gomongo` - MongoDB utilities
- `github.com/real-rm/golevelstore` - LevelDB storage
- `github.com/real-rm/go-toml` - TOML parsing

## Verification

### Check GOPRIVATE Configuration
```bash
go env GOPRIVATE
# Expected output: github.com/real-rm/*
```

### Test Module Access
```bash
go list -m github.com/real-rm/gomongo@latest
# Expected output: github.com/real-rm/gomongo v0.2.0
```

### Verify All Modules
```bash
for pkg in goconfig golog gohelper gomail gosms goupload gomongo golevelstore go-toml; do
  echo -n "$pkg: "
  go list -m github.com/real-rm/$pkg@latest 2>&1 | head -1
done
```

## Troubleshooting

### Module Not Found
If you get "module not found" errors:
1. Verify GOPRIVATE is set: `go env GOPRIVATE`
2. Check SSH authentication: `ssh -T git@github.com`
3. Verify you have access to the repository: `git ls-remote https://github.com/real-rm/[module].git`

### Authentication Failures
If you get authentication errors:
1. Ensure SSH keys are configured for GitHub
2. Check that your GitHub account has access to the real-rm organization
3. Verify Git is configured to use SSH: `git config --get url."git@github.com:".insteadof`

### New Shell Sessions
If GOPRIVATE is not set in new shell sessions:
1. Ensure the export is in both `~/.zshrc` and `~/.zprofile`
2. Restart your terminal or run: `source ~/.zshrc`

## Next Steps
With the private registry configured, you can now:
1. Remove local `replace` directives from `go.mod` (Task 6.2)
2. Run `go mod tidy` to fetch modules from GitHub
3. Verify Docker builds work without local dependencies
4. Test in CI/CD environments
