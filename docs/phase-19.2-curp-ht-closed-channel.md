# Phase 19.2: Port Pre-allocated Closed Channel to CURP-HT

## Summary

Successfully ported pre-allocated closed channel optimization from CURP-HO to CURP-HT. This optimization eliminates repeated channel allocations for already-committed/executed slots, reducing memory churn and GC pressure.

## Changes Made

### 1. Added Pre-allocated Closed Channel Field

```go
// Pre-allocated closed channel for immediate notifications (avoids repeated allocations)
closedChan chan struct{}
```

### 2. Initialized Closed Channel in New()

Added initialization after replica creation:

```go
// Initialize pre-allocated closed channel for immediate notifications
r.closedChan = make(chan struct{})
close(r.closedChan)
```

### 3. Updated getOrCreateCommitNotify()

**Before:**
```go
// Check if already committed
if r.committed.Has(strconv.Itoa(slot)) {
    // Return a closed channel
    ch := make(chan struct{})
    close(ch)
    return ch
}
```

**After:**
```go
// Check if already committed
if r.committed.Has(strconv.Itoa(slot)) {
    // Return pre-allocated closed channel (avoids allocation)
    return r.closedChan
}
```

### 4. Updated getOrCreateExecuteNotify()

**Before:**
```go
// Check if already executed
if r.executed.Has(strconv.Itoa(slot)) {
    // Return a closed channel
    ch := make(chan struct{})
    close(ch)
    return ch
}
```

**After:**
```go
// Check if already executed
if r.executed.Has(strconv.Itoa(slot)) {
    // Return pre-allocated closed channel (avoids allocation)
    return r.closedChan
}
```

## Benefits

### Performance Impact

**Memory Allocations Eliminated:**
- Every call to getOrCreateCommitNotify() for already-committed slots
- Every call to getOrCreateExecuteNotify() for already-executed slots
- These are called in hot paths when commands check slot status

**Typical Scenario:**
- Commands frequently check if dependencies are committed/executed
- With sequential execution, many slots are already committed/executed
- Previously: Allocate new channel + close it (2 operations per check)
- Now: Return single pre-allocated closed channel (1 operation, no allocation)

**Expected Improvements:**
- Reduced GC pressure from eliminated allocations
- Better memory locality (reuse same channel)
- Minor throughput gain (1-2%)
- More consistent performance (fewer GC pauses)

### Code Quality

**Consistency:**
- Matches CURP-HO implementation (Phase 18.2)
- Same optimization pattern across protocols
- Cleaner code (less repetition)

**Correctness:**
- Closed channel behavior identical
- All select/receive operations work the same
- No semantic changes, pure optimization

## Technical Notes

### Why This Works

**Closed Channel Semantics:**
- Reading from closed channel returns immediately with zero value
- Multiple goroutines can safely read from same closed channel
- Channel can be reused indefinitely once closed
- No need to track readers (unlike open channels)

**Safety:**
- closedChan is read-only after creation
- Never written to or re-closed
- Thread-safe for concurrent access
- No synchronization needed

### Memory Impact

**Before:**
- Per-call allocation: ~16 bytes (chan struct{}) + GC overhead
- Frequent allocations in hot path
- GC pressure increases with throughput

**After:**
- One-time allocation: ~16 bytes total
- Zero per-call allocations
- Negligible memory footprint
- GC pressure reduced

**Savings Example:**
- 20K ops/sec × 2 checks per op = 40K calls/sec
- Before: 40K allocations/sec = 640 KB/sec + GC overhead
- After: 0 allocations/sec in this path

## Testing

### Test Results

All tests pass successfully:
```
go test ./curp-ht/
ok      github.com/imdea-software/swiftpaxos/curp-ht    0.012s

go test ./config/ ./curp-ho/
ok      github.com/imdea-software/swiftpaxos/config     (cached)
ok      github.com/imdea-software/swiftpaxos/curp-ho    (cached)
```

### Test Coverage

**Existing tests validated:**
- All protocol functionality tests pass
- Notification mechanism tests (implicit)
- No regressions detected

**No new tests needed:**
- Transparent optimization (no semantic changes)
- Closed channel behavior unchanged
- Existing tests cover notification correctness

## Comparison to CURP-HO Implementation

**Similarities:**
- Identical closedChan field
- Same initialization pattern
- Same usage in notification functions
- Same performance benefits

**Differences:**
- CURP-HT has simpler notification flow (leader-only weak commands)
- Fewer notification call sites than CURP-HO
- Same optimization effectiveness

## Implementation Details

### Files Modified

**curp-ht/curp-ht.go:**
- Added closedChan field to Replica struct (line ~75)
- Initialized closedChan in New() function (line ~177)
- Updated getOrCreateCommitNotify() to use closedChan (line ~1068)
- Updated getOrCreateExecuteNotify() to use closedChan (line ~1101)

**Total Changes:**
- 4 locations modified
- ~10 lines of code
- No test changes needed

### Code Locations

1. **Struct Definition** (~line 75)
2. **Initialization** (~line 177)
3. **Commit Notification** (~line 1068)
4. **Execute Notification** (~line 1101)

## Performance Expectations

Based on CURP-HO experience (Phase 18.2):
- **Minor impact** individually (1-2%)
- **Cumulative impact** with other optimizations (significant)
- **GC improvement** more visible under high load
- **Latency reduction** through fewer allocations

**Validation:**
- Will be measured in Phase 19.5 comprehensive benchmark
- Expected to contribute to overall optimization gains
- Synergizes with string caching (Phase 19.1)

## Next Steps

1. ✅ Phase 19.2 complete - Pre-allocated closed channel ported
2. Next: Phase 19.3 - Optimize CURP-HT spin-wait patterns
3. Next: Phase 19.4 - Port pipeline depth and MaxDescRoutines optimizations
4. Benchmark all optimizations together (Phase 19.5)

## Lessons Learned

### Optimization Principles

1. **Small changes compound**: 1-2% improvements add up
2. **Allocation matters**: Even small allocations impact GC
3. **Hot path focus**: Optimize frequently-called code
4. **Reuse resources**: Pre-allocate when possible

### Best Practices

1. **Profile first**: Know where allocations happen
2. **Measure impact**: Benchmark before and after
3. **Test thoroughly**: Ensure correctness preserved
4. **Document clearly**: Explain why and how

## Conclusion

Pre-allocated closed channel optimization successfully ported to CURP-HT, achieving:
- ✅ Eliminated repeated channel allocations
- ✅ Reduced GC pressure
- ✅ Maintained code correctness
- ✅ Achieved protocol consistency with CURP-HO

This optimization, while minor individually, contributes to the cumulative performance improvements that will be validated in Phase 19.5 benchmarking.
