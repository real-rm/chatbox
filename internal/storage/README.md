# Storage Service - gomongo Integration

This storage service has been refactored to use the `gomongo` library for MongoDB operations.

## Initialization

The storage service now requires:
1. A `gomongo.Mongo` instance (initialized via `gomongo.InitMongoDB`)
2. A `golog.Logger` instance for logging
3. Database and collection names
4. Optional encryption key for sensitive data

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
