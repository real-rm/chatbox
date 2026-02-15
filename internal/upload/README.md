# Upload Service - goupload Integration

This upload service has been refactored to use the `goupload` library from `github.com/real-rm/goupload`.

## Changes from AWS SDK v2

The upload service now uses goupload instead of direct AWS SDK v2 calls. This provides several benefits:

- **Transactional writes**: Automatic rollback on failure
- **Multi-storage support**: Can write to multiple storage targets (local + S3)
- **Statistics tracking**: Automatic file statistics and directory management
- **Chunked upload support**: For large files
- **Built-in logging**: Integrated with golog

## Configuration

The upload service requires configuration in `config.toml`:

```toml
# Connection sources for S3
[connection_sources]
  [[connection_sources.s3_providers]]
    name = "aws-chat-storage"
    endpoint = "s3.us-east-1.amazonaws.com"
    key = "YOUR_AWS_ACCESS_KEY"
    pass = "YOUR_AWS_SECRET_KEY"
    region = "us-east-1"

# Upload configuration
[userupload]
site = "CHAT"
  [[userupload.types]]
    entryName = "uploads"
    prefix = "/chat-files"
    tmpPath = "./temp/uploads"
    maxSize = "100MB"
    storage = [
      { type = "s3", target = "aws-chat-storage", bucket = "chat-files" }
    ]
```

## Initialization

Before using the upload service, you must initialize goupload:

```go
import (
    "github.com/real-rm/goupload"
    "github.com/real-rm/goconfig"
    "github.com/real-rm/golog"
    "github.com/real-rm/gomongo"
)

// 1. Initialize logger
logger, err := golog.InitLog(golog.LogConfig{
    Dir:   "logs",
    Level: "info",
})

// 2. Initialize config
config, err := goconfig.Default()

// 3. Initialize goupload (REQUIRED before using upload service)
if err := goupload.Init(goupload.InitOptions{
    Logger: logger,
    Config: config,
}); err != nil {
    log.Fatal(err)
}

// 4. Initialize MongoDB and create stats collection
mongo, err := gomongo.InitMongoDB(logger, config)
statsColl := mongo.Coll("chat", "file_stats")

// 5. Create upload service
uploadService, err := upload.NewUploadService("CHAT", "uploads", statsColl)
```

## Usage

### Upload a File

```go
file, _ := os.Open("document.pdf")
defer file.Close()

result, err := uploadService.UploadFile(ctx, file, "document.pdf", "user123")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("File uploaded: %s\n", result.FileURL)
fmt.Printf("File ID: %s\n", result.FileID)
fmt.Printf("Size: %d bytes\n", result.Size)
fmt.Printf("MIME type: %s\n", result.MimeType)
```

### Download a File

```go
content, filename, err := uploadService.DownloadFile(ctx, result.FileID)
if err != nil {
    log.Fatal(err)
}

// Save to local file
os.WriteFile(filename, content, 0644)
```

### Delete a File

```go
err := uploadService.DeleteFile(ctx, result.FileID)
if err != nil {
    log.Fatal(err)
}
```

### Generate Signed URL (Note)

The `GenerateSignedURL` method now returns the file path instead of a traditional signed URL. This is because goupload handles file access differently:

```go
filePath, err := uploadService.GenerateSignedURL(ctx, fileID, 1*time.Hour)
// filePath can be used with DownloadFile to retrieve the file
```

For actual file downloads, use the `DownloadFile` method or implement a download endpoint in your application that uses `goupload.Download()`.

## API Changes

### Constructor

**Old:**
```go
NewUploadService(endpoint, region, bucketName, accessKeyID, secretAccessKey string)
```

**New:**
```go
NewUploadService(site, entryName string, statsColl *gomongo.MongoCollection)
```

### UploadFile

**Old:**
```go
UploadFile(ctx context.Context, file io.Reader, filename string)
```

**New:**
```go
UploadFile(ctx context.Context, file io.Reader, filename string, userID string)
```

Note: Now requires `userID` parameter for tracking.

### New Methods

- `DownloadFile(ctx context.Context, filePath string) ([]byte, string, error)` - Downloads file content

## Migration Guide

1. Update initialization code to use goupload.Init()
2. Create MongoDB collection for file statistics
3. Update NewUploadService calls to use new signature
4. Add userID parameter to UploadFile calls
5. Replace signed URL downloads with DownloadFile calls
6. Update config.toml with goupload configuration

## Benefits

- **Automatic statistics**: File counts and directory statistics are tracked automatically
- **Multi-storage**: Can configure multiple storage targets with automatic failover
- **Better error handling**: Transactional writes with automatic rollback
- **Consistent with other services**: Uses the same company libraries (gomongo, goconfig, golog)
