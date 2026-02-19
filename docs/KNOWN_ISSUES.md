# Known Issues

## Session List Sorting Bug

**Status**: Fixed
**Severity**: Medium (was)
**Fixed**: 2026-02-19
**Affected Functions** (now working correctly):
- `ListUserSessions`
- `ListAllSessions`
- `ListAllSessionsWithOptions`

### Description

Sessions were being returned in ascending order (oldest first) instead of descending order
(newest first) as intended. The implementation used `*options.FindOptions` from the standard
MongoDB driver, but the `gomongo` wrapper library's `Find()` method expects `gomongo.QueryOptions`.
Since the type didn't match, sort/limit/skip options were silently ignored.

### Fix Applied

Replaced all three uses of `options.Find()` with `gomongo.QueryOptions` in `storage.go`.
Additionally:
- Fixed `UpdateSession` to exclude the `msgs` field from `$set` updates, preventing
  it from overwriting messages added concurrently via `AddMessage` (`$push`).
- Fixed nil vs empty slice returns when no sessions match a query.
- Fixed tests that were written to expect ascending order as a workaround.

### Related Files

- `internal/storage/storage.go` — Implementation fixed
- `internal/storage/storage_unit_test.go` — Test updated to expect correct descending order
- `internal/storage/storage_test.go` — Test bugs fixed (wrong SortBy values, time precision)
- `internal/storage/storage_concurrent_test.go` — Concurrent test assertion corrected
