# Test Configuration

## MongoDB Test Settings

**IMPORTANT**: Use these exact settings for all MongoDB tests.

### Connection Details
- **Host**: `localhost`
- **Port**: `27017`
- **Database**: `chatbox`
- **Auth Database**: `admin`
- **Username**: `chatbox`
- **Password**: `ChatBox123`

### Connection String
```
mongodb://chatbox:ChatBox123@127.0.0.1:27017/chatbox?authSource=admin
```

### Environment Variable
```bash
export MONGO_URI="mongodb://chatbox:ChatBox123@127.0.0.1:27017/chatbox?authSource=admin"
```

### Go Test Configuration
All tests automatically use the `MONGO_URI` environment variable. If not set, they default to the connection string above.

```bash
# Run tests with MongoDB
export MONGO_URI="mongodb://chatbox:ChatBox123@127.0.0.1:27017/chatbox?authSource=admin"
go test ./...

# Or set it inline
MONGO_URI="mongodb://chatbox:ChatBox123@127.0.0.1:27017/chatbox?authSource=admin" go test ./...
```

### Test Database Name
All tests use the database name: `chatbox`

This is configured in the test setup and should NOT be hardcoded in individual tests.

## Notes
- MongoDB must be running on localhost:27017 before running tests
- The user `chatbox` must exist with readWrite and dbAdmin roles on the `chatbox` database
- Authentication database is `admin`, NOT `chatbox`
- See `docs/MONGODB_TEST_SETUP.md` for detailed setup instructions
- Tests will automatically skip if MongoDB is not available (set `SKIP_MONGO_TESTS=1` to force skip)
