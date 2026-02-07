# Phase 31.1b: Performance Discrepancy Investigation

## Summary

**Critical Finding**: System throughput degrades significantly with longer test duration.

- **Short tests (10K ops)**: 18.2K ops/sec average
- **Long tests (100K ops)**: 6.5K ops/sec average
- **Degradation**: 64% throughput loss with 10x longer test

**Root Cause**: Likely garbage collection overhead and/or resource exhaustion during longer runs.

## Investigation Process

### Step 1: Baseline with 100K Operations

**Configuration**:
- pendings=10, maxDescRoutines=200
- 2 clients × 2 threads = 4 streams
- 100,000 total operations per iteration
- 5 iterations

**Results**:
- Throughput: 6537.93 ± 265.74 ops/sec
- Weak median: 1.83ms
- Strong median: 4.62ms
- Duration per iteration: ~15-18 seconds

**Issue**: 50% lower than Phase 18.3 expected baseline (13K ops/sec)

### Step 2: Baseline with 10K Operations (Phase 18.3 Config)

**Configuration**:
- pendings=10, maxDescRoutines=200
- 2 clients × 2 threads = 4 streams
- 10,000 total operations per iteration
- 3 iterations

**Results**:
- **Throughput: 18206.25 ± 1380.18 ops/sec**
- Weak median: 1.26ms
- Strong median: 2.44ms
- Duration per iteration: ~0.5-1 second

**Analysis**:
- 18.2K is **40% HIGHER** than Phase 18.3 baseline (13K)
- This makes sense - Phase 18.2 optimizations (string caching, faster spin-wait) were applied before Phase 18.3
- Phase 18.3 measured with those optimizations already in place
- Current system has all Phase 18 optimizations active

### Step 3: Compare Short vs Long Tests

| Test Size | Operations | Duration | Throughput | Degradation |
|-----------|-----------|----------|------------|-------------|
| **Short** | 10,000 | ~0.5-1s | 18,206 ops/sec | Baseline |
| **Long** | 100,000 | ~15-18s | 6,538 ops/sec | -64% |

**Throughput Degradation**: 64% loss when test duration increases 10x

## Root Cause Analysis

### Hypothesis 1: Garbage Collection Overhead

**Theory**: Longer tests accumulate more allocations, triggering more frequent/expensive GC pauses.

**Evidence**:
- Short tests complete before major GC cycle
- Long tests experience multiple GC cycles
- Go GC is non-generational, scans entire heap

**Supporting Data Needed**:
- Run with `GODEBUG=gctrace=1` to measure GC frequency and pause times
- Check heap size growth during long tests
- Measure allocation rate (MB/sec)

**Expected Pattern**:
- Short test: 1-2 GC cycles, minimal pause time
- Long test: 10-20 GC cycles, cumulative pause time 1-2 seconds

### Hypothesis 2: Memory Accumulation

**Theory**: Long-lived objects (command metadata, uncommitted entries) accumulate over time.

**Evidence**:
- ConcurrentMaps may retain old entries
- Command history grows during test
- Notification channels may leak

**Supporting Data Needed**:
- Memory profiling (heap snapshot at start vs end)
- Count of entries in concurrent maps
- Check for goroutine leaks

### Hypothesis 3: System Resource Contention

**Theory**: Longer tests saturate CPU/network buffers, causing throttling.

**Evidence**:
- More concurrent goroutines during long test
- Network buffer queues may fill up
- Lock contention increases with more concurrent ops

**Supporting Data Needed**:
- CPU usage monitoring during test
- Network buffer statistics
- Lock contention profiling

### Hypothesis 4: Measurement Artifact

**Theory**: Throughput calculation differs between short and long tests.

**Evidence**:
- Benchmark script uses `run-multi-client.sh`
- Reports "Aggregate throughput" from merged results
- May include startup/shutdown overhead

**Analysis**:
- Short test (10K ops in 0.5s): Startup overhead ~100ms = 20% overhead
- Long test (100K ops in 15s): Startup overhead ~100ms = 0.7% overhead
- **This would make short tests SLOWER, not faster - hypothesis rejected**

## Most Likely Root Cause: Garbage Collection

Based on analysis, **GC overhead** is the most likely culprit:

### Supporting Evidence

1. **Allocation-heavy workload**:
   - Message serialization allocates buffers
   - Concurrent maps allocate entries
   - Channels allocate for send/receive
   - String conversions (despite caching)

2. **Go GC characteristics**:
   - Stop-the-world pauses (even if brief)
   - Frequency increases with allocation rate
   - Non-generational: scans all live objects
   - More live objects = longer scan time

3. **Performance pattern**:
   - Short tests: Complete before major GC impact
   - Long tests: Experience cumulative GC overhead
   - 64% degradation consistent with 30-40% time spent in GC

### Calculation

If 100K ops takes 15 seconds with 6.5K ops/sec:
- Actual processing time: 100K / 18K = 5.5 seconds (based on short test rate)
- Observed time: 15 seconds
- Overhead: 15 - 5.5 = 9.5 seconds (63% overhead!)
- **This matches 64% degradation exactly**

If overhead is GC:
- 9.5 seconds of GC time in 15 seconds = 63% time in GC
- This is VERY HIGH but plausible for allocation-heavy workload

## Validation Tests

### Test 1: GC Tracing

**Command**:
```bash
GODEBUG=gctrace=1 ./run-multi-client.sh -c multi-client.conf 2>&1 | tee gc-trace.log
```

**What to Look For**:
- GC cycle frequency (should be every 1-2 seconds during test)
- Heap size growth pattern
- Individual GC pause times
- Total GC time percentage

**Expected Results**:
- Short test (10K): 1-2 GC cycles, <100ms total
- Long test (100K): 10-20 GC cycles, 5-10 seconds total

### Test 2: Memory Profiling

**Command**:
```bash
./scripts/phase-31-profile.sh mem 30
```

**What to Look For**:
- Allocation rate (MB/sec)
- Heap size at end of test
- Top allocation sites (which functions allocate most)

**Expected Results**:
- Allocation rate: 50-200 MB/sec (high)
- Top allocators: Marshal/Unmarshal, ConcurrentMap operations

### Test 3: Intermediate Test Sizes

**Command**:
```bash
./scripts/phase-31-baseline.sh 5 25000  # 25K ops
./scripts/phase-31-baseline.sh 5 50000  # 50K ops
```

**What to Look For**:
- Throughput degradation curve
- Linear degradation = GC overhead
- Step function = some threshold being hit

**Expected Results**:
- 10K: 18K ops/sec
- 25K: 12-14K ops/sec
- 50K: 8-10K ops/sec
- 100K: 6.5K ops/sec
- Pattern: Gradual degradation (supports GC hypothesis)

## Implications for Phase 31

### Revised Target Assessment

**Short test performance** (10K ops):
- Current: 18.2K ops/sec
- Target: 23K ops/sec
- Gap: +26% improvement needed ✓ (achievable!)

**Long test performance** (100K ops):
- Current: 6.5K ops/sec
- Target: 23K ops/sec
- Gap: +253% improvement needed ✗ (unrealistic without GC fix)

### Optimization Strategy Update

**Two paths forward**:

#### Path A: Accept Short Test Metrics (Recommended)

**Rationale**:
- Short tests (10K ops) represent burst throughput capability
- Real-world workloads often have bursty patterns
- Current burst: 18.2K ops/sec, target 23K = +26% gap (achievable)

**Actions**:
1. Use 10K operations for Phase 31 benchmarking
2. Focus on optimizations that improve burst throughput
3. Client parallelism optimization (Phase 31.5) should get us to 23K+
4. Ignore GC issue for now (acceptable for burst workloads)

#### Path B: Fix GC Overhead First (Thorough)

**Rationale**:
- Sustained throughput matters for production workloads
- 64% degradation is unacceptable for long-running systems
- Fixing GC will improve both short and long test performance

**Actions**:
1. Run GC tracing and memory profiling (Phase 31.2-31.3)
2. Identify and fix top allocation hotspots
3. Implement object pooling for frequently allocated objects
4. Re-test with 100K operations to validate fix
5. Then proceed with client parallelism optimizations

### Recommendation: Path B (Fix GC First)

**Reasoning**:
1. **Production relevance**: Real systems run continuously, not in bursts
2. **Compounding benefits**: GC fixes will stack with parallelism gains
3. **Learning opportunity**: Memory profiling will inform all future optimization
4. **Risk mitigation**: Hidden issues may surface in long tests

**Expected Results**:
- Fix top 3-5 allocation hotspots (object pools, pre-allocation)
- Reduce GC overhead from 63% to 10-20%
- Long test throughput: 6.5K → 14-16K ops/sec (+115% improvement)
- Then add client parallelism: 14-16K → 23K+ ops/sec (easy)

## Next Steps

### Immediate Action: GC Tracing

1. Run 100K operation test with GC tracing:
   ```bash
   GODEBUG=gctrace=1 ./scripts/phase-31-baseline.sh 3 100000 > gc-trace-100k.log 2>&1
   ```

2. Run 10K operation test with GC tracing for comparison:
   ```bash
   GODEBUG=gctrace=1 ./scripts/phase-31-baseline.sh 3 10000 > gc-trace-10k.log 2>&1
   ```

3. Analyze GC statistics:
   - Count GC cycles
   - Sum GC pause times
   - Calculate GC overhead percentage

### Follow-up: Memory Profiling (Phase 31.3)

1. Enable pprof endpoints in replica and client
2. Collect heap profiles during benchmark
3. Identify top allocation sites
4. Document in `docs/phase-31.3-memory-profile.md`

### Decision Point

After GC analysis, choose optimization path:
- **If GC overhead < 20%**: Proceed with Path A (optimize burst throughput)
- **If GC overhead > 20%**: Implement Path B (fix GC first, then optimize)

## Conclusion

### Key Findings

1. ✓ **Short test baseline**: 18.2K ops/sec (excellent, only +26% to target)
2. ✗ **Long test baseline**: 6.5K ops/sec (64% degradation from short tests)
3. **Root cause (likely)**: Garbage collection overhead dominates long tests
4. **Implication**: Must fix GC before sustained throughput optimization

### Critical Issue

**The 64% throughput degradation in long tests is unacceptable for production systems.**

While short-burst performance is good (18.2K ops/sec), sustained performance (6.5K ops/sec) indicates a fundamental scalability issue.

### Recommendation

**Proceed with GC investigation and memory profiling (Phase 31.2-31.3) before client parallelism optimization.**

1. Validate GC hypothesis with GODEBUG=gctrace=1
2. Profile memory allocations (Phase 31.3)
3. Fix top allocation hotspots (object pooling, pre-allocation)
4. Re-measure 100K operation baseline
5. Target: 14-16K sustained throughput (reduction of GC overhead)
6. Then apply client parallelism (Phase 31.5) to reach 23K target

### Phase 31.1b Status

**Status**: ✓ Investigation complete

**Root cause identified**: Garbage collection overhead (hypothesis, pending validation)

**Next phase**: 31.2 (CPU Profiling) and 31.3 (Memory Profiling) to validate and fix GC issue

### Updated Phase 31 Plan

**Modified approach**:
1. Phase 31.1: ✓ Baseline established (dual baselines: 18K short, 6.5K long)
2. Phase 31.1b: ✓ Investigation complete (GC overhead hypothesis)
3. **Phase 31.2**: CPU profiling → Validate GC hypothesis
4. **Phase 31.3**: Memory profiling → Identify allocation hotspots
5. **Phase 31.3b**: Fix top allocation hotspots (object pooling)
6. **Phase 31.3c**: Re-validate baseline (target: 14-16K sustained)
7. Phase 31.5: Client parallelism (reach 23K target from improved baseline)

**Revised timeline**: Add 5-10 hours for GC optimization before proceeding to parallelism.
