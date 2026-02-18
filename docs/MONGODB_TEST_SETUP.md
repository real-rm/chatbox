# MongoDB Test Configuration

## Overview

This document provides configuration details and setup instructions for running tests that require MongoDB integration. All integration tests in this project expect a MongoDB instance to be available with specific credentials and configuration.

## Connection Details

For local development and testing, use the following MongoDB configuration:

- **Host**: `localhost`
- **Port**: `27017`
- **Database**: `chatbox`
- **Authentication Database**: `admin`

### Credentials

- **Username**: `chatbox`
- **Password**: `ChatBox123`

### Connection String

```
mongodb://chatbox:ChatBox123@localhost:27017/chatbox?authSource=admin
```

## Local Development Setup

### Option 1: Docker (Recommended)

The easiest way to set up MongoDB for local testing is using Docker:

#### 1. Start MongoDB Container

```bash
docker run -d \
  --name chatbox-mongo \
  -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=admin \
  -e MONGO_INITDB_ROOT_PASSWORD=admin \
  mongo:latest
```

#### 2. Create Test User

Connect to the MongoDB container and create the test user:

```bash
docker exec -it chatbox-mongo mongosh -u admin -p admin --authenticationDatabase admin
```

In the MongoDB shell, run:

```javascript
use admin
db.createUser({
  user: "chatbox",
  pwd: "ChatBox123",
  roles: [
    { role: "readWrite", db: "chatbox" },
    { role: "dbAdmin", db: "chatbox" }
  ]
})
exit
```

#### 3. Verify Connection

Test the connection with the chatbox user:

```bash
docker exec -it chatbox-mongo mongosh \
  -u chatbox \
  -p ChatBox123 \
  --authenticationDatabase admin \
  chatbox
```

### Option 2: Docker Compose

If you're using docker-compose, add this service to your `docker-compose.yml`:

```yaml
services:
  mongodb:
    image: mongo:latest
    container_name: chatbox-mongo
    ports:
      - "27017:27017"
    environment:
      MONGO_INITDB_ROOT_USERNAME: admin
      MONGO_INITDB_ROOT_PASSWORD: admin
    volumes:
      - mongodb_data:/data/db
      - ./scripts/init-mongo.js:/docker-entrypoint-initdb.d/init-mongo.js:ro

volumes:
  mongodb_data:
```

Create `scripts/init-mongo.js` to automatically create the test user:

```javascript
db = db.getSiblingDB('admin');
db.createUser({
  user: "chatbox",
  pwd: "ChatBox123",
  roles: [
    { role: "readWrite", db: "chatbox" },
    { role: "dbAdmin", db: "chatbox" }
  ]
});
```

Then start the services:

```bash
docker-compose up -d mongodb
```

### Option 3: Local MongoDB Installation

If you have MongoDB installed locally:

1. Start MongoDB with authentication enabled
2. Connect as admin and create the test user using the commands from Option 1

## Running Tests

### Set Environment Variables

Before running tests, set the MongoDB connection string:

```bash
export MONGO_URI="mongodb://chatbox:ChatBox123@localhost:27017/chatbox?authSource=admin"
```

Or add it to your shell profile (`.bashrc`, `.zshrc`, etc.):

```bash
echo 'export MONGO_URI="mongodb://chatbox:ChatBox123@localhost:27017/chatbox?authSource=admin"' >> ~/.bashrc
source ~/.bashrc
```

### Run All Tests

```bash
go test -v ./...
```

### Run Tests with Coverage

```bash
# Coverage for specific packages
go test -cover ./cmd/server
go test -cover ./internal/storage
go test -cover -coverprofile=coverage.out .

# View coverage report
go tool cover -html=coverage.out
```

### Run Tests with Race Detection

```bash
go test -race ./...
```

### Run Integration Tests Only

```bash
go test -v -tags=integration ./...
```

## CI/CD Configuration

### GitLab CI

Add MongoDB as a service in your `.gitlab-ci.yml`:

```yaml
test:
  stage: test
  image: golang:1.21
  services:
    - name: mongo:latest
      alias: mongodb
      variables:
        MONGO_INITDB_ROOT_USERNAME: admin
        MONGO_INITDB_ROOT_PASSWORD: admin
  variables:
    MONGO_URI: "mongodb://chatbox:ChatBox123@mongodb:27017/chatbox?authSource=admin"
  before_script:
    # Wait for MongoDB to be ready
    - apt-get update && apt-get install -y netcat-openbsd
    - until nc -z mongodb 27017; do echo "Waiting for MongoDB..."; sleep 1; done
    # Create test user
    - |
      docker exec mongodb mongosh -u admin -p admin --authenticationDatabase admin --eval '
      use admin;
      db.createUser({
        user: "chatbox",
        pwd: "ChatBox123",
        roles: [
          { role: "readWrite", db: "chatbox" },
          { role: "dbAdmin", db: "chatbox" }
        ]
      });'
  script:
    - go test -v -cover ./...
    - go test -race ./...
```

### GitHub Actions

Add MongoDB as a service in your workflow:

```yaml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    
    services:
      mongodb:
        image: mongo:latest
        ports:
          - 27017:27017
        env:
          MONGO_INITDB_ROOT_USERNAME: admin
          MONGO_INITDB_ROOT_PASSWORD: admin
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
          docker exec ${{ job.services.mongodb.id }} mongosh \
            -u admin -p admin --authenticationDatabase admin \
            --eval 'use admin; db.createUser({user: "chatbox", pwd: "ChatBox123", roles: [{role: "readWrite", db: "chatbox"}, {role: "dbAdmin", db: "chatbox"}]});'
      
      - name: Run tests
        env:
          MONGO_URI: mongodb://chatbox:ChatBox123@localhost:27017/chatbox?authSource=admin
        run: |
          go test -v -cover ./...
          go test -race ./...
```

## Test Data Management

### Cleanup Between Tests

Tests should clean up their data after execution to avoid interference:

```go
func cleanupTestData(t *testing.T, mongo *gomongo.Mongo) {
    t.Helper()
    ctx := context.Background()
    
    // Drop test collections
    _ = mongo.Coll("chatbox", "sessions").Drop(ctx)
    _ = mongo.Coll("chatbox", "messages").Drop(ctx)
}

func TestExample(t *testing.T) {
    mongo := setupTestMongo(t)
    defer cleanupTestData(t, mongo)
    
    // Test code here
}
```

### Using Test-Specific Databases

For parallel test execution, consider using test-specific database names:

```go
func setupTestMongo(t *testing.T) *gomongo.Mongo {
    dbName := fmt.Sprintf("chatbox_test_%d", time.Now().UnixNano())
    uri := fmt.Sprintf("mongodb://chatbox:ChatBox123@localhost:27017/%s?authSource=admin", dbName)
    
    mongo, err := gomongo.New(uri)
    require.NoError(t, err)
    
    t.Cleanup(func() {
        ctx := context.Background()
        mongo.Database(dbName).Drop(ctx)
    })
    
    return mongo
}
```

## Troubleshooting

### Connection Refused

If you get "connection refused" errors:

1. Verify MongoDB is running: `docker ps | grep mongo`
2. Check port binding: `netstat -an | grep 27017`
3. Verify firewall rules allow connections to port 27017

### Authentication Failed

If you get authentication errors:

1. Verify the user was created: Connect as admin and run `db.getUsers()`
2. Check the authentication database is set to "admin"
3. Verify credentials match exactly (case-sensitive)

### Tests Hang or Timeout

If tests hang when connecting to MongoDB:

1. Increase connection timeout in test setup
2. Verify MongoDB is healthy: `docker logs chatbox-mongo`
3. Check for network issues between test and MongoDB

### Permission Denied

If you get permission errors:

1. Verify the chatbox user has the correct roles
2. Check the database name matches "chatbox"
3. Ensure authSource is set to "admin"

## Security Notes

### Production vs Test Credentials

**IMPORTANT**: The credentials documented here are for **local testing only**. Never use these credentials in production environments.

For production:
- Use strong, randomly generated passwords
- Store credentials in secure secret management systems
- Use environment variables or secret files, never hardcode
- Enable TLS/SSL for MongoDB connections
- Restrict network access to MongoDB

### Credential Rotation

For local development, these test credentials can be shared among the team. However:
- Rotate credentials if they are accidentally committed to version control
- Use different credentials for staging/production environments
- Follow your organization's security policies

## Additional Resources

- [MongoDB Docker Hub](https://hub.docker.com/_/mongo)
- [MongoDB Connection String Documentation](https://docs.mongodb.com/manual/reference/connection-string/)
- [Go MongoDB Driver Documentation](https://pkg.go.dev/go.mongodb.org/mongo-driver/mongo)
- [Test Execution Guide](./TEST_EXECUTION_GUIDE.md)

## Summary

This configuration provides a consistent MongoDB setup for all developers and CI/CD pipelines. By following these instructions, you can:

- Run all integration tests locally
- Ensure tests pass in CI/CD pipelines
- Maintain test data isolation
- Troubleshoot common MongoDB connection issues

For questions or issues with MongoDB test setup, please refer to the troubleshooting section or contact the development team.
