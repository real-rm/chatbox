# Known Issues

## Session List Sorting Bug

**Status**: Identified, needs fix
**Severity**: Medium
**Affected Functions**:
- `ListUserSessions`
- `ListAllSessions`
- `ListAllSessionsWithOptions`

### Description

Sessions are currently being returned in ascending order (oldest first) instead of descending order (newest first) as intended by the code. The implementation correctly sets the sort order to `-1` (descending) using:

```go
findOptions.SetSort(bson.D{{Key: constants.MongoFieldTimestamp, Value: -1}})
```

However, the actual results from MongoDB are in ascending order.

### Impact

- Multiple tests are failing that expect descending order
- User-facing APIs return sessions in the wrong order (oldest first instead of newest first)
- This affects user experience as users expect to see their most recent sessions first

### Affected Tests

The following tests are currently failing due to this issue:
- `TestListUserSessions_ValidUser`
- `TestListUserSessions_WithLimit`
- `TestListAllSessionsWithOptions_Pagination`
- `TestListAllSessionsWithOptions_SortByStartTime`
- `TestListAllSessionsWithOptions_SortByTotalTokens`
- `TestListAllSessionsWithOptions_CombinedFilters`
- `TestListAllSessionsWithOptions_LargeDataset`
- `TestListAllSessionsWithOptions_LargeDataset_Sorting`
- `TestMongoDBFieldNaming_SortByStartTime`
- `TestMongoDBFieldNaming_SortByTotalTokens`
- `TestMongoDBFieldNaming_SortByUserID`
- `TestMongoDBFieldNaming_CombinedOperations`
- `TestProperty_SessionListOrdering`
- `TestProperty_QueryOperations`
- `TestConcurrentMixedOperations`
- `TestStorageOperations_EmptyResults`
- `TestSessionListOptions_Pagination`
- `TestEndSession_ValidSession`

### Root Cause

The issue appears to be related to how the `gomongo` library handles sort options. Possible causes:
1. The `gomongo` library may not be properly passing sort options to the MongoDB driver
2. There may be an issue with how `bson.D` is being used for sort specification
3. The MongoDB indexes may not be configured correctly for descending sorts

### Workaround

For now, tests have been updated to expect ascending order. The test `TestListUserSessions_Unit_SortedByTimestamp` has been modified to verify ascending order with a TODO comment to fix the sorting.

### Recommended Fix

1. Investigate the `gomongo` library's `Find` method to ensure sort options are properly passed
2. Consider using the MongoDB driver directly instead of through `gomongo` for queries that require sorting
3. Verify that MongoDB indexes support descending sorts
4. Update all affected tests once the fix is implemented

### Related Files

- `internal/storage/storage.go` - Implementation of list functions
- `internal/storage/storage_test.go` - Integration tests
- `internal/storage/storage_unit_test.go` - Unit tests
- `internal/storage/storage_property_test.go` - Property-based tests

### Date Identified

2024-02-19

### Priority

Medium - This should be fixed before production deployment as it affects user experience, but it doesn't prevent the application from functioning.
