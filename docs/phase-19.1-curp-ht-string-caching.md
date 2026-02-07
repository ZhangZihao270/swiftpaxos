# Phase 19.1: Port String Caching to CURP-HT

## Summary

Successfully ported string caching optimization from CURP-HO to CURP-HT. This optimization reduces GC pressure by caching int32→string conversions, eliminating repeated `strconv.FormatInt` calls in hot paths.

## Changes Made

### 1. Added String Cache Field to Replica Struct

```go
// String conversion cache to avoid repeated strconv.FormatInt calls
// Key: int32, Value: string representation
stringCache sync.Map
```

### 2. Implemented int32ToString Helper Method

```go
// int32ToString converts an int32 to string using a cache to avoid repeated conversions
func (r *Replica) int32ToString(val int32) string {
	// Try to load from cache first
	if cached, ok := r.stringCache.Load(val); ok {
		return cached.(string)
	}
	// Not in cache, convert and store
	str := strconv.FormatInt(int64(val), 10)
	r.stringCache.Store(val, str)
	return str
}
```

### 3. Updated pendingWriteKey to Use Caching

Changed from standalone function to method that uses string cache:

```go
// Before (standalone function)
func pendingWriteKey(clientId int32, key state.Key) string {
	return strconv.FormatInt(int64(clientId), 10) + ":" + strconv.FormatInt(int64(key), 10)
}

// After (method with caching)
func (r *Replica) pendingWriteKey(clientId int32, key state.Key) string {
	return r.int32ToString(clientId) + ":" + r.int32ToString(int32(key))
}
```

### 4. Replaced All strconv.FormatInt Calls

**Locations updated:**
- `sync()` - Key conversion for unsynced map (line ~475)
- `unsync()` - Key conversion for unsynced map (line ~495)
- `leaderUnsync()` - Key conversion for unsynced map (line ~507)
- `ok()` - Key conversion for unsynced map lookup (line ~523)
- `waitForWeakDep()` - Client ID to string (line ~935)
- `markWeakExecuted()` - Client ID to string (line ~954)
- `pendingWriteKey()` - Client ID and key conversions (line ~980)

**Pattern:**
- `strconv.FormatInt(int64(cmd.K), 10)` → `r.int32ToString(int32(cmd.K))`
- `strconv.FormatInt(int64(clientId), 10)` → `r.int32ToString(clientId)`

### 5. Updated Tests

Modified tests to use method syntax:
- `TestPendingWriteKey` - Create minimal replica and call `r.pendingWriteKey()`
- `TestCrossClientIsolation` - Create minimal replica and call `r.pendingWriteKey()`

## Benefits

### Performance Impact

**Expected Improvements:**
- **GC Pressure**: Reduced allocations for frequently used string conversions
- **CPU Usage**: Fewer strconv calls (O(1) map lookup vs O(log n) conversion)
- **Cache Efficiency**: Hot values cached (common client IDs, frequently accessed keys)

**Hot Paths Optimized:**
1. Command processing (sync/unsync operations) - Every command
2. Weak command causal dependency tracking - Per weak command
3. Pending write tracking - Per write operation

### Consistency with CURP-HO

This change brings CURP-HT in line with CURP-HO optimizations (Phase 18.2), ensuring:
- Code consistency across protocols
- Similar performance characteristics
- Shared optimization strategy

## Testing

### Test Results

All tests pass successfully:
```
go test ./curp-ht/
ok      github.com/imdea-software/swiftpaxos/curp-ht    0.008s
```

### Test Coverage

**Existing tests validated:**
- String conversion correctness (TestPendingWriteKey)
- Cross-client isolation (TestCrossClientIsolation)
- All protocol functionality tests (serialization, weak commands, etc.)

**No new tests needed** - String caching is transparent optimization, existing tests validate correctness.

## Technical Notes

### Why sync.Map?

- Thread-safe without explicit locking
- Optimized for read-heavy workloads (string cache is read-mostly)
- No contention overhead for reads
- Suitable for hot path usage

### Cache Growth

Cache grows as new int32 values are encountered:
- **Client IDs**: Limited by number of clients (typically < 100)
- **Keys**: Limited by key space (could be large, but Zipf distribution means hot keys dominate)
- **Memory**: ~50 bytes per cached entry (int32 + string + map overhead)

**Memory impact**: Minimal (< 1MB even with 10K cached values)

### Cache Invalidation

No invalidation needed:
- int32→string conversion is deterministic
- Values never change
- Cache can grow indefinitely (bounded by int32 domain)

## Comparison to CURP-HO Implementation

**Similarities:**
- Same string cache mechanism (sync.Map)
- Same int32ToString helper implementation
- Same conversion patterns replaced

**Differences:**
- CURP-HT has different weak command flow (leader-only)
- Fewer strconv calls in CURP-HT (7 locations vs 11 in CURP-HO)
- Different data structures (no weakExecNotify in CURP-HT)

## Next Steps

1. ✅ Phase 19.1 complete - String caching ported
2. Next: Phase 19.2 - Port pre-allocated closed channel
3. Next: Phase 19.3 - Optimize spin-wait patterns
4. Next: Phase 19.4 - Port pipeline depth and MaxDescRoutines
5. Benchmark CURP-HT with all optimizations (Phase 19.5)

## Expected Performance

Based on CURP-HO results (Phase 18.2: +12% throughput), expect:
- **Baseline**: CURP-HT previously achieved ~26K ops/sec
- **With string caching**: Expect 5-10% improvement
- **Target**: Maintain or exceed 26K baseline

Actual impact to be measured in Phase 19.5 comprehensive benchmark.
