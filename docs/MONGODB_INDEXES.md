# MongoDB Index Strategy

## Overview

The chatbox application uses MongoDB indexes to optimize query performance for session retrieval and filtering operations. Indexes are automatically created during application startup to ensure optimal performance without manual intervention.

## Automatic Index Creation

### Integration with Deployment

Index creation is fully integrated into the application deployment process:

1. **Application Startup**: When the application starts, it automatically calls `EnsureIndexes()` during initialization
2. **Timeout**: Index creation has a 30-second timeout to prevent startup delays
3. **Non-Blocking**: If index creation fails, the application logs a warning but continues to start
4. **Idempotent**: Running `EnsureIndexes()` multiple times is safe - it won't recreate existing indexes

### Implementation Location

The index creation logic is implemented in:
- **Function**: `EnsureIndexes()` in `internal/storage/storage.go`
- **Called from**: `Register()` function in `chatbox.go` (lines 132-136)

```go
// Ensure MongoDB indexes are created for optimal query performance
indexCtx, indexCancel := context.WithTimeout(context.Background(), 30*time.Second)
defer indexCancel()
if err := storageService.EnsureIndexes(indexCtx); err != nil {
    chatboxLogger.Warn("Failed to create MongoDB indexes", "error", err)
    // Don't fail startup - indexes can be created manually if needed
}
```

## Index Definitions

### 1. User ID Index (`idx_user_id`)

**Field**: `uid` (ascending)

**Purpose**: Optimizes queries that retrieve sessions for a specific user

**Used by**:
- `ListSessions(userID)` - Get all sessions for a user
- User-specific session queries

**Query Pattern**:
```javascript
db.sessions.find({ "uid": "user123" })
```

### 2. Start Time Index (`idx_start_time`)

**Field**: `ts` (descending)

**Purpose**: Optimizes time-based queries and sorting, especially for "most recent first" queries

**Used by**:
- `ListAllSessions()` with time-based sorting
- Admin dashboard session listing
- Recent session queries

**Query Pattern**:
```javascript
db.sessions.find({}).sort({ "ts": -1 })
```

### 3. Admin Assisted Index (`idx_admin_assisted`)

**Field**: `adminAssisted` (ascending)

**Purpose**: Optimizes filtering for admin-assisted sessions

**Used by**:
- Admin dashboard filtering
- Queries that filter by admin assistance status

**Query Pattern**:
```javascript
db.sessions.find({ "adminAssisted": true })
```

### 4. Compound User-Time Index (`idx_user_start_time`)

**Fields**: `uid` (ascending) + `ts` (descending)

**Purpose**: Optimizes the most common query pattern - getting a user's sessions sorted by time

**Used by**:
- `ListSessions(userID)` with time sorting
- User session history views

**Query Pattern**:
```javascript
db.sessions.find({ "uid": "user123" }).sort({ "ts": -1 })
```

**Note**: This compound index can also satisfy queries that only need the `uid` field, making the single `idx_user_id` index somewhat redundant. However, we keep both for flexibility and explicit query optimization.

## Deployment Verification

### Verify Index Creation in Kubernetes

After deploying to Kubernetes, verify that indexes were created successfully:

```bash
# Check application logs for index creation
kubectl logs -n chatbox -l app=chatbox | grep "MongoDB indexes"

# Expected output:
# INFO MongoDB indexes created successfully indexes=[idx_user_id, idx_start_time, idx_admin_assisted, idx_user_start_time]
```

### Verify Indexes in MongoDB

If you have direct access to MongoDB, you can verify the indexes:

```bash
# Connect to MongoDB pod
kubectl exec -it mongodb-pod -n chatbox -- mongosh chat

# List all indexes on the sessions collection
db.sessions.getIndexes()
```

Expected output:
```javascript
[
  { v: 2, key: { _id: 1 }, name: '_id_' },
  { v: 2, key: { uid: 1 }, name: 'idx_user_id' },
  { v: 2, key: { ts: -1 }, name: 'idx_start_time' },
  { v: 2, key: { adminAssisted: 1 }, name: 'idx_admin_assisted' },
  { v: 2, key: { uid: 1, ts: -1 }, name: 'idx_user_start_time' }
]
```

## Manual Index Creation

If automatic index creation fails or you need to create indexes manually (e.g., during database migration), use the following commands:

### Using MongoDB Shell

```javascript
// Connect to the chat database
use chat

// Create all indexes
db.sessions.createIndex({ "uid": 1 }, { name: "idx_user_id" })
db.sessions.createIndex({ "ts": -1 }, { name: "idx_start_time" })
db.sessions.createIndex({ "adminAssisted": 1 }, { name: "idx_admin_assisted" })
db.sessions.createIndex({ "uid": 1, "ts": -1 }, { name: "idx_user_start_time" })

// Verify indexes were created
db.sessions.getIndexes()
```

### Using MongoDB Compass

1. Connect to your MongoDB instance
2. Navigate to the `chat` database
3. Select the `sessions` collection
4. Go to the "Indexes" tab
5. Click "Create Index" for each index:
   - `{ "uid": 1 }` with name `idx_user_id`
   - `{ "ts": -1 }` with name `idx_start_time`
   - `{ "adminAssisted": 1 }` with name `idx_admin_assisted`
   - `{ "uid": 1, "ts": -1 }` with name `idx_user_start_time`

## Performance Considerations

### Index Creation Time

- **Small collections** (< 1,000 documents): Indexes create in milliseconds
- **Medium collections** (1,000 - 100,000 documents): Indexes create in seconds
- **Large collections** (> 100,000 documents): Indexes may take minutes

The 30-second timeout should be sufficient for most deployments. If you have a very large existing collection, consider creating indexes manually before deploying the application.

### Index Storage

Indexes consume disk space:
- Typical overhead: 5-10% of collection size
- Compound indexes are larger than single-field indexes
- Monitor disk usage in production

### Query Performance Impact

**Read Performance** (Improved):
- User session queries: 10-100x faster with indexes
- Time-based sorting: 50-500x faster with indexes
- Admin filtering: 10-50x faster with indexes

**Write Performance** (Slight decrease):
- Each insert/update must update 4 indexes
- Typical overhead: 5-15% slower writes
- Trade-off is worth it for read-heavy workloads

### Index Usage Monitoring

Monitor which indexes are being used:

```javascript
// Get index usage statistics
db.sessions.aggregate([
  { $indexStats: {} }
])
```

This shows:
- How many times each index was used
- When it was last used
- Whether indexes are being utilized effectively

## Troubleshooting

### Index Creation Failed

If you see "Failed to create MongoDB indexes" in the logs:

1. **Check MongoDB connectivity**:
   ```bash
   kubectl exec -it chatbox-pod -- nc -zv mongodb-service 27017
   ```

2. **Check MongoDB permissions**:
   - Ensure the MongoDB user has `createIndex` permission
   - Required role: `readWrite` or higher on the `chat` database

3. **Check MongoDB version**:
   - Minimum version: MongoDB 5.0+
   - Index syntax may differ in older versions

4. **Create indexes manually**:
   - Use the manual creation commands above
   - Restart the application to verify it works with existing indexes

### Slow Queries Despite Indexes

If queries are still slow:

1. **Verify indexes are being used**:
   ```javascript
   db.sessions.find({ "uid": "user123" }).explain("executionStats")
   ```
   Look for `"stage": "IXSCAN"` (index scan) not `"stage": "COLLSCAN"` (collection scan)

2. **Check index selectivity**:
   - Indexes work best when they filter out most documents
   - If most documents match the query, indexes may not help

3. **Consider additional indexes**:
   - Analyze your query patterns
   - Add indexes for frequently used query combinations

## Migration and Rollback

### Adding New Indexes

To add new indexes in the future:

1. Update the `EnsureIndexes()` function in `internal/storage/storage.go`
2. Add the new index definition
3. Deploy the updated application
4. Indexes will be created automatically on startup

### Removing Indexes

To remove unused indexes:

```javascript
// Drop a specific index
db.sessions.dropIndex("idx_name")

// Drop all indexes except _id
db.sessions.dropIndexes()
```

**Warning**: Removing indexes will impact query performance. Only remove indexes that are confirmed to be unused.

## Best Practices

1. **Monitor index usage**: Regularly check which indexes are being used
2. **Avoid over-indexing**: Too many indexes slow down writes
3. **Test with production data**: Index performance varies with data size and distribution
4. **Plan for growth**: Consider future data volume when designing indexes
5. **Document changes**: Update this document when adding or removing indexes

## Related Documentation

- [DEPLOYMENT.md](../DEPLOYMENT.md) - General deployment guide with index verification steps
- [deployments/kubernetes/README.md](../deployments/kubernetes/README.md) - Kubernetes-specific deployment
- [internal/storage/README.md](../internal/storage/README.md) - Storage service documentation
- [internal/storage/storage.go](../internal/storage/storage.go) - Index implementation code

## Summary

MongoDB indexes are automatically created during application startup, requiring no manual intervention during deployment. The index strategy is designed to optimize the most common query patterns while maintaining reasonable write performance. Indexes are idempotent and safe to create multiple times, making the deployment process robust and reliable.
