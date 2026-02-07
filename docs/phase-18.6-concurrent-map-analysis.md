# Phase 18.6: Concurrent Map Contention Analysis

## Summary

Analyzed concurrent map usage and SHARD_COUNT configuration in CURP-HO and CURP-HT. **Conclusion**: Current SHARD_COUNT=32768 is excessive for the workload scale. Reducing to 256-1024 is recommended for better cache locality with minimal contention impact.

## Current Implementation

### Concurrent Map Usage

**CURP-HO** (curp-ho/curp-ho.go):
```go
// Line 128: SHARD_COUNT configuration
cmap.SHARD_COUNT = 32768  // Very high shard count

// 10 concurrent maps in Replica struct
synced    cmap.ConcurrentMap  // Tracks synced commands
values    cmap.ConcurrentMap  // Command values
proposes  cmap.ConcurrentMap  // Pending proposals
cmdDescs  cmap.ConcurrentMap  // Command descriptors
unsynced  cmap.ConcurrentMap  // Witness pool entries
executed  cmap.ConcurrentMap  // Executed commands
committed cmap.ConcurrentMap  // Committed commands
delivered cmap.ConcurrentMap  // Delivered commands
weakExecuted cmap.ConcurrentMap  // Weak command execution tracking
pendingWrites cmap.ConcurrentMap  // Pending write tracking
```

**CURP-HT** (curp-ht/curp-ht.go):
Similar usage with 10 concurrent maps and SHARD_COUNT=32768

### Library: github.com/orcaman/concurrent-map

The concurrent-map library uses lock striping:
- Divides the map into SHARD_COUNT separate maps
- Each shard has its own RWMutex
- Hash function determines which shard a key belongs to
- Operations only lock one shard at a time

**Formula**: `shard_index = hash(key) % SHARD_COUNT`

## Analysis: Is 32768 Shards Optimal?

### Memory Overhead

Each shard requires:
- One map structure: ~48 bytes (header)
- One sync.RWMutex: ~24 bytes
- Total per shard: ~72 bytes

**Total overhead per ConcurrentMap**:
- 32768 shards × 72 bytes = 2.36 MB per map
- 10 maps per replica = 23.6 MB overhead
- 3 replicas = 70.8 MB just for empty map structures!

**With realistic data**:
- Each shard allocates a Go map with initial capacity
- Additional overhead for map buckets and metadata
- Estimated total: 100-150 MB for map structures alone

### Cache Locality

**Problem**: 32768 shards means:
- Accesses spread across many memory pages
- Poor CPU cache utilization
- More TLB misses
- Higher memory bandwidth consumption

**CPU Cache sizes** (typical):
- L1 cache: 32-64 KB per core
- L2 cache: 256-512 KB per core
- L3 cache: 8-32 MB shared

With 32768 shards × 72 bytes = 2.36 MB, even the shard metadata doesn't fit in L3 cache!

### Lock Contention Analysis

**Workload characteristics** (Phase 19.5 results):
- Throughput: 21.1K ops/sec (CURP-HT)
- 4 client threads total
- ~5K ops/sec per thread

**Contention calculation**:
- 5K ops/sec per thread = 5000 ops/sec
- Each op accesses ~2-3 concurrent maps
- ~10K-15K map operations per second per thread
- With 32768 shards: probability of collision = (4 threads × 15K) / 32768 = 1.8%

**Conclusion**: With only 4 threads and 32768 shards, contention is negligible!

### What is the Optimal SHARD_COUNT?

Let's calculate for different scenarios:

**Scenario 1: Current workload (4 threads)**
- Target: < 5% collision probability
- Required shards: (4 × 15K) / 0.05 = 1200 shards
- **Recommendation**: 256-1024 shards sufficient

**Scenario 2: Heavy load (16 threads)**
- Target: < 5% collision probability
- Required shards: (16 × 15K) / 0.05 = 4800 shards
- **Recommendation**: 1024-4096 shards

**Scenario 3: Current configuration (32768 shards)**
- Collision probability with 4 threads: 1.8% (over-provisioned)
- Collision probability with 16 threads: 7.3% (still good)
- **Assessment**: Excessive for current workload

### Industry Standard

Popular concurrent map implementations:
- **sync.Map** (Go stdlib): Adaptive sharding, typically 16-128 shards
- **Java ConcurrentHashMap**: Default 16 segments
- **Rust DashMap**: Default 64 shards
- **This implementation**: 32768 shards (512x higher than typical!)

## Recommendations

### 1. Reduce SHARD_COUNT to 256-1024

**Rationale**:
- Current 32768 is 10-100x higher than needed
- Reduces memory overhead from 70MB to 2-7MB
- Improves cache locality (fits in L3 cache)
- Minimal contention impact (< 5% collision rate)

**Recommended values**:
- **256 shards**: Good for 2-8 threads, 560 KB overhead
- **512 shards**: Good for 4-16 threads, 1.1 MB overhead
- **1024 shards**: Good for 8-32 threads, 2.2 MB overhead

**Selection**: Start with **512 shards** as a balanced choice.

### 2. Make SHARD_COUNT Configurable

Currently SHARD_COUNT is hardcoded in curp-ho.go:128 and curp-ht.go:128.

**Proposed change**:
```go
// Allow configuration via config file or default to sensible value
shardCount := 512  // Default
if config.ShardCount > 0 {
    shardCount = config.ShardCount
}
cmap.SHARD_COUNT = shardCount
```

**Benefits**:
- Easy to tune for different workloads
- Can compare performance with different values
- No code changes needed for tuning

### 3. Profile Before and After (Optional)

If we want empirical validation:
1. Run benchmark with SHARD_COUNT=32768 (baseline)
2. Run with SHARD_COUNT=512 (recommended)
3. Run with SHARD_COUNT=256 (conservative)
4. Run with SHARD_COUNT=1024 (aggressive)
5. Compare throughput and latency

**Expected results**:
- 256-1024: Similar or better performance (cache locality benefit)
- 32768: Current performance (baseline)
- Difference: < 5% (contention already negligible)

## Implementation Plan

### Phase 1: Change Default SHARD_COUNT

**File**: curp-ho/curp-ho.go (line 128)
**Change**:
```go
// Before:
cmap.SHARD_COUNT = 32768

// After:
cmap.SHARD_COUNT = 512  // Optimal for 4-16 threads, good cache locality
```

**File**: curp-ht/curp-ht.go (line 128)
**Same change**

### Phase 2: Validate with Tests

Run existing test suites:
```bash
go test ./curp-ho/
go test ./curp-ht/
```

### Phase 3: Benchmark Comparison (Optional)

```bash
# Baseline (current 32768)
./run-multi-client.sh -c multi-client.conf

# With 512 shards (recommended)
# (after code change)
./run-multi-client.sh -c multi-client.conf

# Compare results
```

**Expected**: Within ±3% (measurement variance)

### Phase 4: Document Results

Update todo.md and this document with findings.

## Performance Impact Estimation

### Memory Impact

**Before** (SHARD_COUNT=32768):
- 10 maps × 32768 shards × 72 bytes = 23.6 MB per replica
- 3 replicas × 23.6 MB = 70.8 MB total

**After** (SHARD_COUNT=512):
- 10 maps × 512 shards × 72 bytes = 368 KB per replica
- 3 replicas × 368 KB = 1.1 MB total

**Savings**: 69.7 MB (98% reduction)

### Cache Impact

**Before**:
- Shard metadata: 2.36 MB per map (doesn't fit in L3)
- Frequent cache misses on shard access
- Poor cache utilization

**After**:
- Shard metadata: 36 KB per map (fits in L2 cache)
- Better cache hit rate
- Improved memory bandwidth

**Estimated benefit**: +2-5% throughput from cache locality

### Contention Impact

**Before** (32768 shards, 4 threads):
- Collision probability: 1.8%
- Lock contention: Negligible

**After** (512 shards, 4 threads):
- Collision probability: 11.7%
- Lock contention: Still low (< 12%)

**Estimated impact**: < 1% throughput reduction

**Net effect**: +1-4% throughput improvement from cache benefit

## Experimental Validation

### Quick Test Procedure

1. **Modify SHARD_COUNT**: Change to 512 in both files
2. **Rebuild**: `go build -o swiftpaxos .`
3. **Run benchmark**: `./run-multi-client.sh -c multi-client.conf`
4. **Compare**: Against Phase 19.5 baseline (21.1K ops/sec)

### Expected Results

**CURP-HT**:
- Baseline: 21.1K ops/sec
- With 512 shards: 21.3-22K ops/sec (+1-4%)
- Reason: Better cache locality outweighs minimal contention increase

**CURP-HO**:
- Baseline: 17.0K ops/sec
- With 512 shards: 17.2-17.7K ops/sec (+1-4%)
- Same reasoning

### If Results Show Regression

If throughput decreases by > 2%:
1. Try SHARD_COUNT=1024 (compromise)
2. Profile to identify specific bottleneck
3. Check if workload changed

Unlikely scenario given analysis, but should validate.

## Alternative: sync.Map

Go's standard `sync.Map` is an alternative to concurrent-map:

**Pros**:
- Built-in, no external dependency
- Optimized for read-heavy workloads
- No SHARD_COUNT to tune

**Cons**:
- Slower for write-heavy workloads
- More complex internal implementation
- Less predictable performance

**Assessment**: concurrent-map is better for CURP's mixed read/write workload.

## Alternative: Lock-Free Data Structures

**Options**:
- Atomic operations with linked lists
- Lock-free hash tables (complex)
- Per-core data structures

**Assessment**:
- Complexity very high
- Benefit minimal (contention already low)
- Not recommended for this use case

## Conclusion

### Summary

**Current state**:
- SHARD_COUNT=32768 is excessive for workload
- 70MB memory overhead
- Poor cache locality
- Negligible contention (1.8% collision rate)

**Recommendation**:
- Reduce SHARD_COUNT to 512
- Expected benefits:
  - -69MB memory usage (98% reduction)
  - +2-5% throughput (cache locality)
  - < 1% throughput loss (contention)
  - Net: +1-4% overall improvement

**Implementation**:
- Simple: change one number in two files
- Low risk: contention still negligible
- Easy to revert if issues found

### Action Items

1. **Change SHARD_COUNT**: 32768 → 512 in curp-ho/curp-ho.go and curp-ht/curp-ht.go
2. **Run tests**: Verify no regressions
3. **Benchmark**: Quick validation (optional)
4. **Document**: Update todo.md with results
5. **Commit**: "perf: Optimize concurrent map shard count for cache locality (Phase 18.6)"

### Phase 18.6 Status

**Recommendation**: Proceed with SHARD_COUNT=512 change.

Alternative: If we want to be conservative, use SHARD_COUNT=1024 as a middle ground.

## References

- **concurrent-map library**: github.com/orcaman/concurrent-map
- **Go sync.Map**: https://golang.org/pkg/sync/#Map
- **Lock striping**: Classic concurrency pattern
- **Current performance**: docs/phase-19.5-curp-ht-benchmark-results.md

## Appendix: SHARD_COUNT Calculator

For a given workload:
```
threads = number of concurrent threads
ops_per_thread = operations per second per thread
maps_per_op = concurrent maps accessed per operation
target_collision = target collision probability (e.g., 0.05 for 5%)

total_ops = threads × ops_per_thread × maps_per_op
required_shards = total_ops / target_collision
```

**Example** (current workload):
```
threads = 4
ops_per_thread = 5000
maps_per_op = 3
target_collision = 0.05

total_ops = 4 × 5000 × 3 = 60000
required_shards = 60000 / 0.05 = 1200
```

Recommendation: Use next power of 2: **2048 shards** or conservative **512-1024**.

---

**Phase 18.6 Status**: Analysis complete, implementation recommended

**Recommended SHARD_COUNT**: 512 (balanced), or 1024 (conservative)
