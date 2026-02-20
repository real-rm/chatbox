# Test Execution Guide

## Overview

This guide provides instructions for running the production readiness verification tests. These tests validate critical runtime behaviors, security controls, and production readiness concerns identified in the production readiness review.

## Prerequisites

- Go 1.21 or later
- MongoDB running locally or via Docker (for integration tests)
- All project dependencies installed: `go mod download`

## Running All Tests

### Basic Test Execution

Run all tests in the project:

```bash
go test ./...
```

Run with verbose output:

```bash
go test -v ./...
```

### Race Detection

Run all tests with the race detector (recommended for production readiness verification):

```bash
go test -race ./...
```

Run specific package with race detection:

```bash
go test -race ./internal/session
```

### Coverage Analysis

Generate coverage report for all packages:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

View coverage in terminal:

```bash
go test -cover ./...
```

Generate coverage for specific package:

```bash
go test -coverprofile=coverage.out ./internal/session
go tool cover -func=coverage.out
```

## Running Production Readiness Tests

### All Production Tests

Run all production readiness verification tests:

```bash
go test -v -race ./... -run TestProductionIssue
```

### Specific Issue Tests

Run tests for a specific production issue:

```bash
# Session management tests (Issue #1)
go test -v -race ./internal/session -run TestProductionIssue01

# Session creation flow tests (Issue #2)
go test -v -race ./internal/router -run TestProductionIssue02

# Connection management tests (Issue #3)
go test -v -race ./internal/router -run TestProductionIssue03

# Concurrency safety tests (Issue #4)
go test -v -race ./internal/websocket -run TestProductionIssue04

# Main server tests (Issue #5)
go test -v -race ./cmd/server -run TestProductionIssue05

# Secret management tests (Issue #6)
go test -v ./deployments/kubernetes -run TestProductionIssue06

# Message validation tests (Issue #7)
go test -v ./internal/message -run TestProductionIssue07

# LLM streaming context tests (Issue #8)
go test -v -race ./internal/router -run TestProductionIssue08

# MongoDB retry logic tests (Issue #9)
go test -v ./internal/storage -run TestProductionIssue09

# Session serialization tests (Issue #10)
go test -v -race ./internal/storage -run TestProductionIssue10

# Rate limiter cleanup tests (Issue #11)
go test -v ./internal/ratelimit -run TestProductionIssue11

# Response times tracking tests (Issue #12)
go test -v ./internal/session -run TestProductionIssue12

# Origin validation tests (Issue #13)
go test -v -race ./internal/websocket -run TestProductionIssue13

# Shutdown behavior tests (Issue #15)
go test -v -race . -run TestProductionIssue15

# Configuration validation tests (Issue #19)
go test -v ./internal/config -run TestProductionIssue19

# JWT secret validation tests (Issue #17)
go test -v . -run TestProductionIssue17

# Admin endpoint security tests (Issue #18)
go test -v . -run TestProductionIssue18
```

## Running Tests by Category

### Concurrency Tests

Run all tests that check for race conditions:

```bash
go test -race -v ./internal/session ./internal/router ./internal/websocket
```

### Memory Management Tests

Run tests that verify memory usage and cleanup:

```bash
go test -v ./internal/session -run "MemoryGrowth|Cleanup"
go test -v ./internal/ratelimit -run "MemoryGrowth|Cleanup"
```

### Security Tests

Run tests that verify security controls:

```bash
go test -v ./deployments/kubernetes -run TestProductionIssue06
go test -v . -run "TestProductionIssue17|TestProductionIssue18"
go test -v ./internal/websocket -run TestProductionIssue13
```

### Configuration Tests

Run tests that verify configuration validation:

```bash
go test -v ./internal/config -run TestProductionIssue19
```

## Test Output Interpretation

### Successful Test

```
=== RUN   TestProductionIssue01_SessionCleanup
--- PASS: TestProductionIssue01_SessionCleanup (0.00s)
```

### Failed Test

```
=== RUN   TestProductionIssue06_PlaceholderSecrets
    secret_validation_test.go:45: Found placeholder secrets that must be replaced
--- FAIL: TestProductionIssue06_PlaceholderSecrets (0.01s)
```

### Race Condition Detected

```
==================
WARNING: DATA RACE
Read at 0x00c000120080 by goroutine 8:
  ...
Previous write at 0x00c000120080 by goroutine 7:
  ...
==================
```

## Continuous Integration

### GitLab CI

The `.gitlab-ci.yml` file includes test stages:

```bash
# Run tests locally as CI would
go test -race -coverprofile=coverage.out ./...
```

### Pre-commit Testing

Run before committing changes:

```bash
# Quick test
go test ./...

# Full verification
go test -race -cover ./...
```

## Troubleshooting

### Race Detector Issues

If race detector reports false positives or issues:

1. Verify the code actually has proper locking
2. Check if the race is in test code vs production code
3. Review the stack traces to identify the conflicting accesses

### Timeout Issues

If tests timeout:

```bash
# Increase timeout
go test -timeout 30s ./...

# Run specific slow test
go test -v -timeout 60s ./internal/storage -run TestProductionIssue09
```

### MongoDB Connection Issues

If storage tests fail due to MongoDB:

1. Ensure MongoDB is running: `docker-compose up -d mongodb`
2. Check connection string in test setup
3. Verify network connectivity

### Coverage Not Generated

If coverage report is empty:

```bash
# Ensure tests are actually running
go test -v -coverprofile=coverage.out ./...

# Check if coverage file was created
ls -lh coverage.out
```

## Best Practices

1. **Always run with race detector** for concurrency tests
2. **Run full test suite** before pushing changes
3. **Check coverage** to ensure new code is tested
4. **Review test output** for warnings and documented behaviors
5. **Update tests** when fixing production issues

## Performance Considerations

- Unit tests should complete in <5 seconds total
- Use `-short` flag to skip slow tests: `go test -short ./...`
- Run integration tests separately if needed
- Parallel execution is enabled by default: `go test -parallel 4 ./...`

## Reporting Issues

When tests fail:

1. Capture full output: `go test -v ./... > test_output.txt 2>&1`
2. Include race detector output if applicable
3. Note which specific test failed
4. Provide steps to reproduce
5. Check if issue is documented in production readiness review

## Additional Resources

- [Production Readiness Review](PRODUCTION_READINESS_REVIEW.md)
- [Production Readiness Test Results](../PRODUCTION_READINESS_TEST_RESULTS.md)
- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Go Race Detector](https://golang.org/doc/articles/race_detector.html)
