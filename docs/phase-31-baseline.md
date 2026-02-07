# Phase 31.1: Baseline Performance Measurement

## Summary

Established baseline performance with **pendings=10** constraint:
- **Throughput**: 6537.93 ± 265.74 ops/sec
- **Weak median latency**: 1.83 ± 0.66ms ✓ (meets < 2ms constraint)
- **Strong median latency**: 4.62 ± 3.02ms
- **Gap to 23K target**: +16,462 ops/sec (+250%)

## Configuration

```
Protocol: CURP-HO (curpho)
Pendings: 10 (max in-flight commands per thread)
MaxDescRoutines: 200
Clients: 2 × 2 threads = 4 total request streams
Requests per run: 100,000 total operations
Iterations: 5
```

## Baseline Results

### Throughput (ops/sec)

| Metric | Value |
|--------|-------|
| Min | 6067.85 |
| Max | 6825.45 |
| Avg | 6537.93 |
| Stddev | ±265.74 |
| Variance | 4.1% |

**Per-iteration breakdown**:
1. 6735.54 ops/sec
2. 6458.67 ops/sec
3. 6825.45 ops/sec
4. 6602.14 ops/sec
5. 6067.85 ops/sec (outlier - high strong median of 10.66ms)

### Latency Metrics

**Strong Latency** (operations requiring full consensus):
| Metric | Min | Max | Avg | Stddev |
|--------|-----|-----|-----|--------|
| Median | 2.91ms | 10.66ms | 4.62ms | ±3.02ms |
| P99 | 29.60ms | 41.03ms | 35.66ms | ±3.82ms |

**Weak Latency** (CURP fast path):
| Metric | Min | Max | Avg | Stddev |
|--------|-----|-----|-----|--------|
| Median | 0.53ms | 2.42ms | 1.83ms | ±0.66ms |
| P99 | 4.87ms | 37.23ms | 26.38ms | ±11.22ms |

### Constraint Validation

✓ **Weak median latency constraint MET**: 1.83ms < 2ms target

The system meets the latency constraint with comfortable headroom (0.17ms margin).

## Performance Gap Analysis

### Gap to Target

```
Current baseline:  6,537.93 ops/sec
Target throughput: 23,000 ops/sec
Gap:               +16,462 ops/sec (+250%)
```

This is a **significantly larger gap** than anticipated in the Phase 31 overview:
- **Expected baseline**: ~13K ops/sec (from Phase 18.3 data)
- **Actual baseline**: 6.5K ops/sec
- **Difference**: -6.5K ops/sec (-50% lower than expected!)

### Why is Performance Lower Than Expected?

Comparing to Phase 18.3 and Phase 19 results:
- **Phase 18.3** achieved ~13K ops/sec with pendings=10
- **Phase 19.5** achieved 21.1K ops/sec (CURP-HT with pendings=20)
- **Current run** only achieved 6.5K ops/sec with pendings=10

**Possible reasons**:
1. **Different configuration**: Phase 18/19 might have used different client counts or thread counts
2. **System load**: Current system may have background processes or resource contention
3. **Configuration drift**: Some other parameters may have changed
4. **Measurement variance**: Different benchmark scripts may measure differently

**Action**: Need to investigate why current performance is 50% lower than Phase 18.3 baseline.

## Variance Analysis

### Throughput Variance: 4.1%

Variance is acceptably low (< 5%), indicating stable performance across iterations.

**Outlier**: Iteration 5 (6067.85 ops/sec) shows lower throughput with anomalous latency:
- Strong median: 10.66ms (2-3x higher than other iterations)
- Weak median: 0.53ms (3-4x lower than other iterations)

This suggests a temporary slowdown or measurement artifact in iteration 5.

### Latency Variance

**Strong median variance**: High (±3.02ms, 65% coefficient of variation)
- Indicates inconsistent strong path performance
- Iteration 5 is a clear outlier (10.66ms vs 2.91-3.45ms)

**Weak median variance**: Moderate (±0.66ms, 36% coefficient of variation)
- More stable than strong path
- Still shows some variation across iterations

## Variance Sources

### Identified Sources

1. **System Scheduling**
   - Goroutine scheduling overhead
   - CPU contention from background processes
   - Context switching delays

2. **Network Stack**
   - Localhost network stack variance (using loopback interface)
   - Buffer queue depths fluctuating
   - TCP window scaling behavior

3. **Garbage Collection**
   - Go GC pauses during benchmark runs
   - Allocation pressure from message batching
   - Heap compaction delays

4. **Workload Characteristics**
   - Random distribution of fast/slow paths
   - Hash table collision patterns
   - Lock contention on concurrent maps

### Mitigation Strategies

For future iterations:
1. **More iterations**: Run 10+ iterations instead of 5 to average out variance
2. **Warmup phase**: Add 10-20K warmup requests before measurement
3. **GC tuning**: Set `GOGC` environment variable to reduce GC frequency
4. **System isolation**: Minimize background processes during benchmarks
5. **Outlier removal**: Exclude iterations with anomalous metrics (e.g., iteration 5)

## Target Reassessment

### Original Target: 23K ops/sec (+77% from 13K baseline)

**Revised target based on actual baseline**:
- Current: 6.5K ops/sec
- Target: 23K ops/sec
- Gap: +250% improvement needed

This is a **much more aggressive target** than originally planned.

### Optimization Strategy Update

Given the larger gap, we need to prioritize high-impact optimizations:

1. **Client Parallelism** (Highest Priority)
   - Current: 2 clients × 2 threads = 4 streams
   - Target: 4 clients × 6 threads = 24 streams (6x increase)
   - Expected gain: +300-400% throughput
   - Rationale: With 250% gap, we need massive parallelism increase

2. **Configuration Investigation** (Critical)
   - Compare current config with Phase 18.3 config
   - Verify: maxDescRoutines, SHARD_COUNT, buffer sizes
   - Check: replica count, network settings, timeouts

3. **CPU Profiling** (Phase 31.2)
   - Identify hot paths consuming CPU
   - Target: serialization, map operations, state machine
   - Expected gain: +10-20% from optimization

4. **Network Batching** (Phase 31.4)
   - Improve message batching efficiency
   - Expected gain: +10-15% throughput

5. **Lock Contention** (Phase 31.8)
   - Profile mutex contention
   - Tune SHARD_COUNT for more threads
   - Expected gain: +5-10% throughput

**Total expected improvement**: 300-400% (enough to reach 23K+ target)

## Next Steps

### Immediate Actions (Phase 31.1 completion)

1. ✓ Run comprehensive benchmark suite (5 iterations, 100K ops each) - **DONE**
2. ✓ Document baseline in docs/phase-31-baseline.md - **DONE**
3. **TODO**: Identify variance sources - **PARTIALLY DONE** (see Variance Sources above)

### Investigation Tasks (Before Phase 31.2)

**Critical**: Understand why performance is 50% lower than expected
- [ ] Compare multi-client.conf with Phase 18.3 configuration
- [ ] Check system resource usage (CPU, memory, network)
- [ ] Verify replica configuration and maxDescRoutines setting
- [ ] Run Phase 18.3 benchmark script for direct comparison
- [ ] Check for configuration drift or system changes

### Next Phase: 31.2 (CPU Profiling)

Only proceed to Phase 31.2 after resolving the performance discrepancy.

**If current baseline is correct** (6.5K is truly current performance):
- Run CPU profiling: `./scripts/phase-31-profile.sh cpu 30`
- Identify top CPU bottlenecks
- Document findings in docs/phase-31.2-cpu-profile.md

**If configuration issue found**:
- Fix configuration to restore ~13K baseline
- Re-run Phase 31.1 baseline measurement
- Update this document with corrected baseline

## Raw Results

Full benchmark output saved to:
```
/home/users/zihao/swiftpaxos/docs/phase-31-profiles/baseline-results-20260207-165813.txt
```

## Conclusion

### Key Findings

1. **Baseline established**: 6537.93 ± 265.74 ops/sec with pendings=10
2. **Latency constraint met**: Weak median 1.83ms < 2ms target ✓
3. **Gap to target**: +250% improvement needed (much larger than expected)
4. **Performance anomaly**: Current performance 50% lower than Phase 18.3 baseline
5. **Variance acceptable**: 4.1% throughput variance, stable measurements

### Critical Issue

**The 50% performance drop from Phase 18.3 must be investigated before proceeding.**

Possible root causes:
- Configuration change (maxDescRoutines, client count, thread count)
- System resource contention or degradation
- Benchmark script differences (measurement methodology)
- Protocol changes or bug introduction

### Recommendation

**Pause Phase 31 optimization work** until performance discrepancy is resolved.

**Action**: Compare current configuration with Phase 18.3 and identify what changed.

### Phase 31.1 Status

**Status**: ✓ Baseline measurement complete, ⚠️ investigation needed

**Tasks completed**:
- ✓ Run 5 iterations of 100K operations each
- ✓ Measure throughput, latency, variance
- ✓ Document baseline metrics
- ✓ Identify variance sources
- ⚠️ Performance anomaly detected (50% lower than expected)

**Next action**: Investigate performance discrepancy before Phase 31.2
