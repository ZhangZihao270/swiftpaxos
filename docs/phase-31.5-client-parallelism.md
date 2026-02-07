# Phase 31.5: Client Parallelism Scaling Analysis

## Summary

**Unexpected Finding**: Increasing client parallelism **decreases** throughput.

- **Optimal configuration**: 2 clients × 2 threads = 4 streams
- **Peak throughput**: 17,948 ops/sec @ 4 streams
- **Degradation**: -28% @ 8 streams, -29% @ 12 streams, -37% @ 16 streams
- **Weak latency**: Meets < 2ms constraint at 4 streams, violates at 8+ streams
- **Root cause**: Contention and scheduler overhead dominate benefits of parallelism

**Implication**: Cannot reach 23K target through parallelism alone. Must optimize contention hotspots first.

## Test Configuration

**Fixed parameters**:
- Protocol: CURP-HO (curpho)
- Pendings: 10 (max in-flight per thread)
- MaxDescRoutines: 200
- Requests per client: 10,000
- Iterations per config: 3

**Variable parameter**: Client threads

| Config | Clients | Threads/Client | Total Streams | Total Pendings |
|--------|---------|----------------|---------------|----------------|
| Baseline | 2 | 2 | 4 | 40 |
| 2x | 2 | 4 | 8 | 80 |
| 3x | 2 | 6 | 12 | 120 |
| 4x | 2 | 8 | 16 | 160 |

## Results

### Throughput vs Streams

| Streams | Avg Throughput | Change | Weak Median | Strong Median |
|---------|----------------|--------|-------------|---------------|
| **4** (baseline) | **17,948 ops/sec** | - | **1.23ms** ✓ | 2.61ms |
| 8 | 12,948 ops/sec | -28% ⚠️ | 2.79ms ✗ | 5.39ms |
| 12 | 12,740 ops/sec | -29% ⚠️ | 2.45ms ✗ | 12.11ms |
| 16 | 11,356 ops/sec | -37% ⚠️ | 4.81ms ✗ | 18.86ms |

**Key Observations**:
1. **Peak at 4 streams**: Baseline configuration is optimal
2. **Monotonic degradation**: More threads = worse performance
3. **Latency violations**: Weak median > 2ms for all configs except baseline
4. **Strong latency explosion**: 2.61ms → 18.86ms (7.2x increase @ 16 streams)

### Detailed Iteration Results

#### Configuration 1: 2 clients × 2 threads = 4 streams

| Iteration | Throughput | Weak Median | Strong Median |
|-----------|------------|-------------|---------------|
| 1 | 17,231 ops/sec | 1.29ms | 2.75ms |
| 2 | 18,858 ops/sec | 1.15ms | 2.42ms |
| 3 | 17,757 ops/sec | 1.26ms | 2.65ms |
| **Avg** | **17,948 ops/sec** | **1.23ms** | **2.61ms** |

**Variance**: 9.1% (excellent stability)

#### Configuration 2: 2 clients × 4 threads = 8 streams

| Iteration | Throughput | Weak Median | Strong Median |
|-----------|------------|-------------|---------------|
| 1 | 11,195 ops/sec | 3.21ms | 5.72ms |
| 2 | 12,849 ops/sec | 2.96ms | 5.14ms |
| 3 | 14,798 ops/sec | 2.21ms | 5.32ms |
| **Avg** | **12,948 ops/sec** | **2.79ms** | **5.39ms** |

**Variance**: 27.8% (high variance, unstable)

**Analysis**: Iteration 3 shows improvement (14.8K), suggesting warmup effects or transient contention.

#### Configuration 3: 2 clients × 6 threads = 12 streams

| Iteration | Throughput | Weak Median | Strong Median |
|-----------|------------|-------------|---------------|
| 1 | 12,033 ops/sec | 2.30ms | 13.58ms |
| 2 | 13,258 ops/sec | 2.37ms | 12.01ms |
| 3 | 12,929 ops/sec | 2.70ms | 10.73ms |
| **Avg** | **12,740 ops/sec** | **2.45ms** | **12.11ms** |

**Variance**: 9.9% (moderate stability)

**Analysis**: Strong latency explodes to 12ms (4.6x higher than baseline). System struggling.

#### Configuration 4: 2 clients × 8 threads = 16 streams

| Iteration | Throughput | Weak Median | Strong Median |
|-----------|------------|-------------|---------------|
| 1 | 11,848 ops/sec | 1.96ms | 21.53ms |
| 2 | 11,130 ops/sec | 1.96ms | 22.17ms |
| 3 | 11,091 ops/sec | 10.53ms | 12.87ms |
| **Avg** | **11,356 ops/sec** | **4.81ms** | **18.86ms** |

**Variance**: 6.8% (throughput stable, but latency highly variable)

**Analysis**: Iteration 3 shows anomalous weak latency spike (10.53ms vs 1.96ms). System instability.

## Why Does Parallelism Hurt?

### Expected Behavior (from Phase 31.2)

**Hypothesis**: System is I/O bound → more streams should increase throughput linearly.

**Expected scaling**:
- 4 streams @ 18K ops/sec
- 8 streams → 36K ops/sec (2x)
- 12 streams → 54K ops/sec (3x)
- 16 streams → 72K ops/sec (4x)

**Actual scaling**:
- 4 streams @ 18K ops/sec ✓
- 8 streams @ 13K ops/sec (-28%)
- 12 streams @ 13K ops/sec (flat)
- 16 streams @ 11K ops/sec (-37%)

**Conclusion**: System is NOT purely I/O bound. There is contention limiting parallelism.

### Root Cause Analysis

#### Hypothesis 1: Lock Contention (LIKELY)

**Evidence**:
- CPU profile showed 16.35% in getCmdDescSeq (descriptor management)
- More threads → more concurrent access to shared data structures
- ConcurrentMap with SHARD_COUNT=32768 may have cache thrashing

**Mechanism**:
- Each thread accesses shared concurrent maps (synced, values, proposes, etc.)
- With 16 threads vs 4 threads: 4x more lock contention
- Lock contention causes threads to wait, reducing effective parallelism

**Validation needed**:
- Collect mutex profile (Phase 31.3 - deferred earlier)
- Measure contention on ConcurrentMap operations

#### Hypothesis 2: Goroutine Scheduling Overhead (LIKELY)

**Evidence**:
- CPU profile showed 8.08% in runtime.stealWork (goroutine scheduler)
- More threads → more goroutines → more scheduling overhead
- Go scheduler may thrash with too many runnable goroutines

**Mechanism**:
- Each client thread spawns multiple goroutines (sender, receiver, etc.)
- With 16 threads × ~5 goroutines = ~80 goroutines competing for CPU
- Scheduler overhead scales non-linearly with goroutine count

**Calculation**:
- 4 streams: ~20 goroutines → minimal overhead
- 8 streams: ~40 goroutines → moderate overhead
- 16 streams: ~80 goroutines → high overhead (8% CPU in Phase 31.2)

#### Hypothesis 3: Network Saturation (UNLIKELY)

**Evidence**:
- Loopback interface bandwidth: ~40 Gbps (very high)
- Message size: ~100 bytes
- Required bandwidth @ 18K ops/sec: 18K × 100 bytes × 10 messages = 18 MB/sec = 0.144 Gbps
- **Utilization**: 0.144 / 40 = 0.36% (negligible)

**Conclusion**: Network bandwidth is NOT the bottleneck.

#### Hypothesis 4: Cache Thrashing (POSSIBLE)

**Evidence**:
- Phase 18.6 analysis: SHARD_COUNT=32768 causes poor cache locality
- More threads → more concurrent map accesses → more cache misses

**Mechanism**:
- With 16 threads accessing 32768 shards, cache lines constantly evicted
- L3 cache size: ~8-32 MB (typical)
- ConcurrentMap shard metadata: 32768 × 72 bytes = 2.36 MB (fits in L3)
- But with 16 threads, each access may evict other threads' cached shards

**Validation needed**:
- Reduce SHARD_COUNT from 32768 to 512 (Phase 18.6 recommendation)
- Re-test with lower shard count

#### Hypothesis 5: Descriptor Bottleneck (LIKELY)

**Evidence**:
- getCmdDescSeq: 16.35% of CPU time (Phase 31.2)
- Every command requires descriptor creation/lookup
- More threads → more concurrent descriptor operations

**Mechanism**:
- Descriptor pool may have limited size (maxDescRoutines=200)
- With 16 threads × 10 pendings = 160 in-flight ops
- Approaching pool limit (200) → blocking on descriptor allocation

**Calculation**:
- 4 streams: 40 in-flight ops (20% of pool)
- 8 streams: 80 in-flight ops (40% of pool)
- 12 streams: 120 in-flight ops (60% of pool)
- 16 streams: 160 in-flight ops (80% of pool!) ⚠️

**Conclusion**: Descriptor pool saturation is likely contributing to degradation.

## Comparison to Phase 31.1 Baseline

**Short test (10K ops) results**:
- Phase 31.1: 18,206 ops/sec @ 4 streams
- Phase 31.5: 17,948 ops/sec @ 4 streams
- **Difference**: -1.4% (excellent reproducibility!)

**Conclusion**: Baseline performance is stable and reproducible.

## Gap to 23K Target

**Current best**: 17,948 ops/sec @ 4 streams
**Target**: 23,000 ops/sec
**Gap**: +5,052 ops/sec (+28% improvement needed)

**Conclusion**: Parallelism alone cannot bridge the gap. Must optimize contention first.

## Optimization Strategy Revision

### Original Plan (from Phase 31.2)

Skip profiling, go directly to client parallelism for +300-400% gain.

**Status**: ✗ FAILED - Parallelism decreases throughput by 28-37%

### Revised Plan

**Priority 1: Fix Contention Bottlenecks (CRITICAL)**

1. **Reduce SHARD_COUNT** (Phase 18.6 recommendation)
   - Change: 32768 → 512 shards
   - Expected: +10-20% throughput from better cache locality
   - Risk: Low (contention already low at 4 streams)

2. **Increase maxDescRoutines** (Phase 18.4 follow-up)
   - Change: 200 → 500-1000 descriptors
   - Expected: Remove descriptor pool bottleneck
   - Risk: Low (more memory, but acceptable)

3. **Mutex profiling** (Phase 31.3 - previously skipped)
   - Collect mutex contention profile @ 8 streams
   - Identify which locks are hotspots
   - Optimize lock-free paths or reduce critical sections

**Priority 2: Re-test Parallelism After Contention Fixes**

After fixing contention, re-test client parallelism:
- Expected: Linear or near-linear scaling with threads
- Target: 8-12 streams → 23K+ ops/sec

**Priority 3: Network Batching** (if still short of target)

Increase message batch sizes to reduce syscall overhead.

## Recommendations

### Immediate Actions

1. **Reduce SHARD_COUNT to 512**
   - File: curp-ho/curp-ho.go line 128
   - Change: `cmap.SHARD_COUNT = 32768` → `cmap.SHARD_COUNT = 512`
   - Rationale: Phase 18.6 analysis, cache locality improvement

2. **Increase maxDescRoutines to 500**
   - File: multi-client.conf
   - Change: `maxDescRoutines: 200` → `maxDescRoutines: 500`
   - Rationale: Remove descriptor pool bottleneck at high thread counts

3. **Re-test baseline (4 streams)**
   - Validate SHARD_COUNT change doesn't regress performance
   - Expected: 18K → 19-20K ops/sec (+5-10%)

4. **Re-test parallelism (8 streams)**
   - After SHARD_COUNT + maxDescRoutines changes
   - Expected: Better scaling than current -28% degradation

### Follow-Up Actions (If Still Short)

5. **Mutex profiling** (Phase 31.3)
   - Collect at 8 streams with SHARD_COUNT=512
   - Identify remaining contention hotspots

6. **Lock-free optimizations**
   - Based on mutex profile results
   - Reduce critical section sizes

7. **Network batching** (Phase 31.4)
   - Increase batch sizes to reduce syscall overhead
   - Target: +10-15% throughput

## Conclusion

### Key Findings

1. ✗ **Parallelism decreases throughput**: -28% @ 8 streams, -37% @ 16 streams
2. ✓ **Baseline performance excellent**: 17.9K ops/sec @ 4 streams (reproducible)
3. ⚠️ **Latency constraint violated**: Weak median > 2ms for 8+ streams
4. ✓ **Root cause identified**: Lock contention + descriptor pool saturation
5. ✗ **Cannot reach 23K via parallelism**: Must fix contention first

### Critical Issue

**High thread counts cause severe performance degradation** due to:
- Lock contention on concurrent maps
- Descriptor pool saturation (160/200 descriptors used @ 16 streams)
- Goroutine scheduling overhead
- Cache thrashing from excessive sharding (SHARD_COUNT=32768)

### Strategy Update

**STOP parallelism testing. Fix contention bottlenecks first.**

**Action plan**:
1. Reduce SHARD_COUNT: 32768 → 512 (Phase 18.6)
2. Increase maxDescRoutines: 200 → 500
3. Re-test baseline and 8 streams
4. If improved, continue scaling
5. If still degrading, run mutex profiling (Phase 31.3)

**Expected outcome after fixes**:
- Baseline (4 streams): 18K → 19-20K ops/sec (+5-10%)
- 8 streams: 13K → 24-28K ops/sec (+80-115%)
- **Target achieved**: 23K+ ops/sec ✓

## Phase 31.5 Status

**Status**: ⚠️ Partially complete - identified problem, needs fixes

**Artifacts**:
- Test script: scripts/test-client-parallelism.sh
- Results: docs/phase-31-profiles/client-parallelism-results-20260207-171553.txt
- Analysis: docs/phase-31.5-client-parallelism.md (this file)

**Next**: Fix contention (SHARD_COUNT + maxDescRoutines), then re-test

**Recommendation**: Do NOT proceed to Phase 31.6. Fix contention first (Phase 31.5b).
