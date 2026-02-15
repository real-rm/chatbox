# Testing Guide

This document provides information about running tests for the chatbox service.

## Test Environment Setup

### MongoDB Local Test Service

For running integration tests that require MongoDB, use the following credentials:

```json
{
  "user": "chatbox",
  "pwd": "ChatBox123",
  "db": "chatbox",
  "authDb": "admin",
  "host": "localhost",
  "port": 27017
}
```

**Connection String:**
```
mongodb://chatbox:ChatBox123@localhost:27017/chatbox?authSource=admin
```

### Setting Up MongoDB for Tests

#### Using Docker

```bash
# Start MongoDB with authentication
docker run -d \
  --name mongodb-test \
  -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=admin \
  -e MONGO_INITDB_ROOT_PASSWORD=admin123 \
  mongo:5.0

# Create test user
docker exec -it mongodb-test mongosh -u admin -p admin123 --authenticationDatabase admin --eval '
db.getSiblingDB("admin").createUser({
  user: "chatbox",
  pwd: "ChatBox123",
  roles: [
    { role: "readWrite", db: "chatbox" },
    { role: "dbAdmin", db: "chatbox" }
  ]
})
'
```

#### Using Docker Compose

```bash
# Start services defined in docker-compose.yml
docker-compose up -d mongodb

# Wait for MongoDB to be ready
sleep 5

# Create test user
docker-compose exec mongodb mongosh -u admin -p admin123 --authenticationDatabase admin --eval '
db.getSiblingDB("admin").createUser({
  user: "chatbox",
  pwd: "ChatBox123",
  roles: [
    { role: "readWrite", db: "chatbox" },
    { role: "dbAdmin", db: "chatbox" }
  ]
})
'
```

### Configuration for Tests

Create a test configuration file `config.test.toml`:

```toml
[dbs]
verbose = 1
slowThreshold = 2

[dbs.chat]
uri = "mongodb://chatbox:ChatBox123@localhost:27017/chatbox?authSource=admin"
database = "chatbox"
collection = "sessions"
connectTimeout = "10s"
```

## Running Tests

### All Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run with coverage
go test ./... -cover

# Generate coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Unit Tests Only

```bash
# Run tests without MongoDB integration
go test ./... -short

# Run specific package tests
go test ./internal/auth -v
go test ./internal/message -v
go test ./internal/session -v
```

### Integration Tests with MongoDB

```bash
# Set config file for tests
export RMBASE_FILE_CFG=config.test.toml

# Run storage tests
go test ./internal/storage -v

# Run all integration tests
go test ./... -v
```

### Property-Based Tests

```bash
# Run property tests with more iterations
go test ./internal/auth -v -args -quickchecks=1000

# Run specific property test
go test ./internal/auth -v -run TestProperty_JWTTokenValidation
```

## Test Categories

### Unit Tests
- **Auth**: JWT token validation and claims extraction
- **Config**: Configuration loading and validation
- **Message**: Message protocol and validation
- **Session**: Session management and lifecycle
- **Upload**: File upload validation and security
- **WebSocket**: Connection handling and lifecycle

### Integration Tests (Require MongoDB)
- **Storage**: Session and message persistence
- **Storage Property Tests**: Data integrity and encryption

### Property-Based Tests
- JWT token validation (Property 1, 2)
- WebSocket connection management (Property 3, 4, 5, 7)
- Session management (Property 6, 17, 48, 49)
- Message protocol (Property 9, 12, 31, 32, 42)
- Storage operations (Property 14, 15, 16, 18, 44, 46)
- LLM service (Property 10, 11, 13, 27, 28, 29, 30, 62)
- File upload (Property 19, 20, 21, 22, 23, 24, 45)

## Test Data Cleanup

After running integration tests, clean up test data:

```bash
# Connect to MongoDB
mongosh mongodb://chatbox:ChatBox123@localhost:27017/chatbox?authSource=admin

# Drop test collections
db.sessions.drop()
db.file_stats.drop()

# Or drop entire test database
use chatbox
db.dropDatabase()
```

## Continuous Integration

For CI/CD pipelines, use the following approach:

```yaml
# Example GitHub Actions workflow
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    
    services:
      mongodb:
        image: mongo:5.0
        env:
          MONGO_INITDB_ROOT_USERNAME: admin
          MONGO_INITDB_ROOT_PASSWORD: admin123
        ports:
          - 27017:27017
        options: >-
          --health-cmd "mongosh --eval 'db.adminCommand(\"ping\")'"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Create MongoDB test user
        run: |
          mongosh mongodb://admin:admin123@localhost:27017/admin --eval '
          db.createUser({
            user: "chatbox",
            pwd: "ChatBox123",
            roles: [
              { role: "readWrite", db: "chatbox" },
              { role: "dbAdmin", db: "chatbox" }
            ]
          })
          '
      
      - name: Run tests
        env:
          RMBASE_FILE_CFG: config.test.toml
        run: |
          go test ./... -v -cover
```

## Troubleshooting

### MongoDB Connection Issues

If tests fail with MongoDB connection errors:

1. Verify MongoDB is running:
   ```bash
   docker ps | grep mongodb
   ```

2. Test connection manually:
   ```bash
   mongosh mongodb://chatbox:ChatBox123@localhost:27017/chatbox?authSource=admin
   ```

3. Check MongoDB logs:
   ```bash
   docker logs mongodb-test
   ```

4. Verify user exists:
   ```bash
   mongosh mongodb://admin:admin123@localhost:27017/admin --eval 'db.getUsers()'
   ```

### Test Timeout Issues

For tests that timeout (especially property-based tests):

```bash
# Increase test timeout
go test ./... -timeout 30m

# Run with race detector (slower but catches concurrency issues)
go test ./... -race
```

### Skipped Tests

Some tests are skipped when MongoDB is not available. To run all tests:

1. Start MongoDB with test credentials
2. Set `RMBASE_FILE_CFG` environment variable
3. Run tests with `-v` flag to see which tests run

## Test Coverage Goals

- **Unit Tests**: Minimum 80% code coverage
- **Integration Tests**: All CRUD operations covered
- **Property Tests**: Minimum 100 iterations per property
- **Edge Cases**: All error paths tested

## Writing New Tests

When adding new features, follow these guidelines:

1. **Write tests first** (TDD approach)
2. **Use table-driven tests** for multiple scenarios
3. **Add property tests** for universal correctness properties
4. **Mock external dependencies** (LLM APIs, S3, etc.)
5. **Clean up test data** in defer statements
6. **Use descriptive test names** that explain what is being tested

Example test structure:

```go
func TestFeature_Scenario(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {
            name:    "valid input",
            input:   "test",
            want:    "expected",
            wantErr: false,
        },
        {
            name:    "invalid input",
            input:   "",
            want:    "",
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Feature(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Feature() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Feature() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## References

- [Go Testing Package](https://pkg.go.dev/testing)
- [Testify Assertions](https://github.com/stretchr/testify)
- [Gopter Property Testing](https://github.com/leanovate/gopter)
- [MongoDB Go Driver Testing](https://www.mongodb.com/docs/drivers/go/current/fundamentals/testing/)
