# Phase 31: Achieving 23K Throughput with Pendings=10

## Goal

Achieve **23,000 ops/sec** throughput for CURP-HO with the following constraints:
- **Pendings: 10** (max in-flight commands per thread)
- **Weak median latency: < 2ms**
- **Configuration**: 2 clients, adjustable threads

## Performance Gap

**Current Baseline** (from Phase 18.3 data):
- Throughput: ~13K ops/sec with pendings=10
- Weak median latency: ~2.0-2.5ms
- Configuration: 2 clients × 2 threads = 4 request streams

**Target**:
- Throughput: 23K ops/sec
- **Gap**: +10K ops/sec (+77% improvement needed)

**Comparison to Phase 18 Achievement**:
- Phase 18 achieved 17.35K sustained with pendings=20
- We need 23K with pendings=10 → much more challenging
- Cannot simply increase pendings (constraint), must optimize other dimensions

## Strategy

### Multi-Dimensional Optimization Approach

Since we cannot increase `pendings` beyond 10, we must optimize across multiple dimensions:

1. **Client Parallelism** (Highest Impact: +5-6K ops/sec)
   - Increase from 4 → 12 request streams (3x)
   - Options: More clients (2→4) and/or more threads per client (2→3)
   - Rationale: More parallel request streams WITHOUT increasing per-thread pendings

2. **CPU Optimization** (+3-5K ops/sec)
   - Profile-driven: identify and optimize hot paths
   - Focus areas: serialization, map operations, state machine
   - Expected: 10-20% CPU improvement → 1.5-2.5K ops/sec gain

3. **Network Batching** (+1-2K ops/sec)
   - Improve message batching efficiency
   - Larger batch sizes (2-5 → 5-10 messages per batch)
   - Trade-off: +50-100μs latency for +10-15% throughput

4. **Lock Contention Reduction** (+1-2K ops/sec)
   - Tune SHARD_COUNT for 12 threads
   - Reduce critical section sizes
   - Profile mutex contention with pprof

5. **State Machine Optimization** (+1-2K ops/sec)
   - Faster GET/PUT operations
   - Read-write lock optimization
   - Key generation caching

**Combined Expected Improvement**: 13K → 23K+ ops/sec (+10K total)

## Latency Constraint

**Challenge**: Increasing throughput often increases latency due to:
- Higher contention on shared resources
- More goroutine scheduling overhead
- Deeper queues (more in-flight operations)

**Mitigation Strategies**:
1. Monitor weak median latency at each optimization step
2. Reject optimizations that push latency > 2ms
3. Prioritize low-contention optimizations (client parallelism, batching)
4. Avoid increasing pendings (keeps per-thread queue depth low)

**Expected Latency Impact**:
- Client parallelism: +0.2-0.3ms (acceptable, still < 2ms)
- Network batching: +0.05-0.1ms (minimal)
- CPU optimization: -0.1-0.2ms (improvement!)
- Lock contention: -0.1-0.2ms (improvement!)
- **Net effect**: ~1.8-2.0ms weak median (within constraint)

## Implementation Phases

### Phase 31.1: Baseline Measurement
**Script**: `scripts/phase-31-baseline.sh`

Establishes accurate baseline with pendings=10:
- Run 5+ iterations, measure throughput and latency
- Document variance and reproducibility
- Identify current performance characteristics

**Output**: `docs/phase-31-profiles/baseline-results-*.txt`

### Phase 31.2: CPU Profiling
**Script**: `scripts/phase-31-profile.sh cpu 30`

Identify CPU bottlenecks:
- Collect CPU profiles (replica + client)
- Analyze top 10 functions by CPU%
- Categorize: serialization, consensus, state machine, GC
- Identify optimization targets (> 5% CPU each)

**Output**: `docs/phase-31.2-cpu-profile.md`, `*.prof` files

### Phase 31.3: Memory Profiling
**Script**: `scripts/phase-31-profile.sh mem 30`

Identify allocation hotspots:
- Collect allocation profiles
- Measure allocation rate (target: < 10 MB/sec)
- Identify candidates for object pooling
- Analyze GC impact

**Output**: `docs/phase-31.3-memory-profile.md`

### Phase 31.4: Network Optimization
Improve message batching:
- Measure current batch sizes
- Test adaptive batching (50-100μs window)
- Trade latency for throughput (+0.05-0.1ms for +1-2K ops/sec)

**Expected**: +1-2K ops/sec

### Phase 31.5: Client Parallelism (Highest ROI)
**Key optimization**: More request streams, same per-thread pendings

Test configurations:
```
Current: 2 clients × 2 threads = 4 streams → ~13K ops/sec
Test 1:  2 clients × 4 threads = 8 streams → ~18-20K ops/sec (expected)
Test 2:  2 clients × 6 threads = 12 streams → ~23-25K ops/sec (expected)
Test 3:  4 clients × 3 threads = 12 streams → ~23-25K ops/sec (expected)
```

**Rationale**:
- Each thread has pendings=10 (constraint satisfied)
- Total in-flight ops: threads × pendings (e.g., 12 threads × 10 = 120 ops)
- More parallelism = more throughput WITHOUT violating per-thread constraint

**Expected**: +5-6K ops/sec (largest single gain)

### Phase 31.6: State Machine Optimization
Optimize GET/PUT operations:
- Profile Execute() time
- Read-write lock tuning
- Key generation caching

**Expected**: +1-2K ops/sec

### Phase 31.7: Serialization Optimization
Reduce Marshal/Unmarshal overhead:
- Optimize hot message types (MAccept, MCausalPropose)
- Zero-copy deserialization (if feasible)
- Message size caching

**Expected**: +1.5-2.5K ops/sec

### Phase 31.8: Lock Contention Analysis
Reduce lock contention:
- Profile mutex contention
- Tune SHARD_COUNT for 12 threads
- Reduce critical section sizes

**Expected**: +1-2K ops/sec

### Phase 31.9: Combined Testing
Apply best optimizations:
- Implement top 3-5 optimizations
- Measure combined impact
- Validate latency constraint (< 2ms)

**Target**: 23K ops/sec sustained

### Phase 31.10: Validation
Extended testing:
- 10+ iterations, 200K ops each
- Measure stability (variance < 5%)
- Stress test (1M ops continuous)
- Document final configuration

## Key Tools

1. **Baseline Measurement**
   ```bash
   ./scripts/phase-31-baseline.sh 5 100000
   ```

2. **CPU Profiling**
   ```bash
   # Start replica with pprof
   # Run benchmark in separate terminal
   ./scripts/phase-31-profile.sh cpu 30
   ```

3. **Memory Profiling**
   ```bash
   ./scripts/phase-31-profile.sh mem
   ```

4. **Interactive Analysis**
   ```bash
   go tool pprof -http=:8080 docs/phase-31-profiles/replica-cpu.prof
   ```

5. **GC Tracing**
   ```bash
   GODEBUG=gctrace=1 make run-hybrid
   ```

## Expected Timeline

- **Phase 31.1** (Baseline): 1-2 hours
- **Phase 31.2-31.3** (Profiling): 2-3 hours
- **Phase 31.4** (Network): 2-3 hours (implementation + testing)
- **Phase 31.5** (Parallelism): 1-2 hours (testing only, no code changes)
- **Phase 31.6-31.8** (Optimizations): 3-5 hours each (6-15 hours total)
- **Phase 31.9** (Combined): 2-3 hours
- **Phase 31.10** (Validation): 2-3 hours

**Total**: 20-35 hours (depends on optimization complexity)

## Risk Factors

1. **System-Dependent Performance**
   - Network stack variance
   - Go scheduler behavior
   - System load interference
   - **Mitigation**: Multiple iterations, document system specs

2. **Latency-Throughput Trade-off**
   - Higher throughput → higher latency
   - Risk: exceed 2ms weak median constraint
   - **Mitigation**: Monitor at each step, reject bad optimizations

3. **Diminishing Returns**
   - Early optimizations: high ROI
   - Later optimizations: low ROI, high complexity
   - **Mitigation**: Profile-first, measure each optimization individually

4. **Go Runtime Limitations**
   - GC overhead at high allocation rates
   - Goroutine scheduling overhead with many threads
   - **Mitigation**: Memory profiling, allocation reduction, thread count tuning

## Success Criteria

**Primary Goals**:
- ✅ Throughput: ≥ 23K ops/sec (sustained over 10 runs)
- ✅ Weak median latency: < 2ms (all runs)
- ✅ Configuration: pendings=10 (constraint)
- ✅ Variance: < 5% between runs (stability)

**Secondary Goals**:
- Strong median latency: < 6ms (acceptable)
- P99 latencies: < 10ms weak, < 20ms strong
- Slow path rate: < 5%
- No memory leaks (stable heap usage)

## Documentation Outputs

- `docs/phase-31-baseline.md`: Baseline performance analysis
- `docs/phase-31.2-cpu-profile.md`: CPU profiling results
- `docs/phase-31.3-memory-profile.md`: Memory profiling results
- `docs/phase-31.4-network-batching.md`: Network optimization analysis
- `docs/phase-31.5-client-parallelism.md`: Parallelism scaling results
- `docs/phase-31.6-state-machine.md`: State machine optimization
- `docs/phase-31.7-serialization.md`: Serialization optimization
- `docs/phase-31.8-lock-contention.md`: Lock contention analysis
- `docs/phase-31.9-combined-results.md`: Combined optimization results
- `docs/phase-31-final-config.md`: Final configuration and settings
- `docs/phase-31-summary.md`: Phase 31 summary and lessons learned

## Next Steps

1. **Run baseline measurement**:
   ```bash
   ./scripts/phase-31-baseline.sh 5 100000
   ```

2. **Review baseline results**:
   - Check current throughput vs 13K expected
   - Verify weak median latency
   - Calculate gap to 23K target

3. **Run profiling** (if replicas running with pprof):
   ```bash
   ./scripts/phase-31-profile.sh all 30
   ```

4. **Prioritize optimizations**:
   - Start with Phase 31.5 (client parallelism) - highest ROI, no code changes
   - Then profile-driven optimizations (31.2-31.3 → 31.6-31.8)

5. **Iterate and measure**:
   - Apply one optimization at a time
   - Measure impact on throughput and latency
   - Document results before proceeding

## Notes

- **Profile before optimizing**: Avoid premature optimization
- **Test incrementally**: One optimization at a time
- **Monitor latency**: Reject optimizations that violate < 2ms constraint
- **Document everything**: Reproducibility is critical for validation
