# Storage Service - gomongo Integration

This storage service has been refactored to use the `gomongo` library for MongoDB operations.

## Initialization

The storage service now requires:
1. A `gomongo.Mongo` instance (initialized via `gomongo.InitMongoDB`)
2. A `golog.Logger` instance for logging
3. Database and collection names
4. Optional encryption key for sensitive data (32 bytes for AES-256)

**IMPORTANT**: For production deployments, see [KEY_MANAGEMENT.md](../../KEY_MANAGEMENT.md) for comprehensive guidance on encryption key generation, storage, rotation, and security best practices.

### Example Usage

```go
package main

import (
    "github.com/real-rm/goconfig"
    "github.com/real-rm/golog"
    "github.com/real-rm/gomongo"
    "github.com/real-rm/chatbox/internal/storage"
)

func main() {
    // 1. Load configuration
    if err := goconfig.LoadConfig(); err != nil {
        log.Fatal(err)
    }
    
    configAccessor, err := goconfig.Default()
    if err != nil {
        log.Fatal(err)
    }
    
    // 2. Initialize logger
    logger, err := golog.InitLog(golog.LogConfig{
        Level:          "info",
        StandardOutput: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer logger.Close()
    
    // 3. Initialize gomongo
    mongoClient, err := gomongo.InitMongoDB(logger, configAccessor)
    if err != nil {
        log.Fatal(err)
    }
    
    // 4. Create storage service
    // Optional: Create 32-byte encryption key for AES-256
    // PRODUCTION: Load from environment variable or Kubernetes secret
    // Generate with: openssl rand -base64 32
    encryptionKey := []byte("12345678901234567890123456789012")
    
    storageService := storage.NewStorageService(
        mongoClient,
        "chat",           // database name
        "sessions",       // collection name
        logger,
        encryptionKey,    // or nil for no encryption
    )
    
    // Now use the storage service
    // ...
}
```

## Configuration

The gomongo library requires configuration in TOML format:

```toml
[dbs]
verbose = 1
slowThreshold = 2

[dbs.chat]
uri = "mongodb://localhost:27017/chat"
```

## Key Changes from Previous Implementation

1. **Removed custom wrapper interfaces**: The service now uses `gomongo.Mongo` and `gomongo.MongoCollection` directly
2. **Automatic timestamp management**: gomongo automatically adds `_ts` (creation time) and `_mt` (modification time) to documents
3. **Logger integration**: The service now requires a `golog.Logger` for consistent logging
4. **Simplified initialization**: No need to manually create MongoDB client - gomongo handles connection pooling and configuration

## Benefits

- **Automatic timestamps**: `_ts` and `_mt` fields are managed automatically
- **Built-in logging**: All operations are logged with performance monitoring
- **Connection pooling**: gomongo manages connection pools efficiently
- **Consistent error handling**: Standardized error handling across all operations
- **Transaction support**: gomongo provides ACID transaction support when needed

## MongoDB Indexes

The storage service automatically creates MongoDB indexes during application initialization to ensure optimal query performance.

### Automatic Index Creation

Indexes are created by calling `EnsureIndexes(ctx)` during application startup. This operation:
- Creates all necessary indexes if they don't exist
- Is idempotent (safe to run multiple times)
- Has a 30-second timeout
- Logs success or failure (non-blocking - app continues if it fails)

### Indexes Created

1. **idx_user_id** - Index on `uid` field
   - Used for: User-specific session queries
   - Type: Single field, ascending

2. **idx_start_time** - Index on `ts` field
   - Used for: Time-based queries and sorting (most recent first)
   - Type: Single field, descending

3. **idx_admin_assisted** - Index on `adminAssisted` field
   - Used for: Filtering admin-assisted sessions
   - Type: Single field, ascending

4. **idx_user_start_time** - Compound index on `uid` and `ts`
   - Used for: Common query pattern (user's sessions sorted by time)
   - Type: Compound, `uid` ascending + `ts` descending

### Query Optimization

These indexes optimize the following operations:
- `ListSessions(userID)` - Uses `idx_user_id` or `idx_user_start_time`
- `ListAllSessions()` with sorting - Uses `idx_start_time`
- Admin dashboard filtering - Uses `idx_admin_assisted`
- Combined user + time queries - Uses `idx_user_start_time`

### Deployment Integration

Index creation is integrated into the deployment process:

1. **Application Startup**: Indexes are created automatically when the app starts
2. **Kubernetes Deployment**: No manual steps required
3. **Verification**: Check logs for "MongoDB indexes created successfully"
4. **Manual Creation**: If needed, indexes can be created manually via MongoDB shell

For deployment-specific documentation, see:
- [DEPLOYMENT.md](../../DEPLOYMENT.md) - General deployment guide
- [deployments/kubernetes/README.md](../../deployments/kubernetes/README.md) - Kubernetes-specific guide

### Manual Index Creation

If you need to create indexes manually (e.g., for troubleshooting):

```javascript
// Connect to MongoDB
use chat

// Create indexes
db.sessions.createIndex({ "uid": 1 }, { name: "idx_user_id" })
db.sessions.createIndex({ "ts": -1 }, { name: "idx_start_time" })
db.sessions.createIndex({ "adminAssisted": 1 }, { name: "idx_admin_assisted" })
db.sessions.createIndex({ "uid": 1, "ts": -1 }, { name: "idx_user_start_time" })

// Verify indexes
db.sessions.getIndexes()
```

### Performance Considerations

- Index creation on large collections may take time (seconds to minutes)
- Indexes consume disk space (typically 5-10% of collection size)
- Indexes improve read performance but slightly slow down writes
- The compound index `idx_user_start_time` can satisfy queries that only need `uid`

For more details on the implementation, see the `EnsureIndexes` function in `storage.go`.
