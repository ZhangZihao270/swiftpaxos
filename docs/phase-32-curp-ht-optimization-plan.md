# Phase 32: Apply Phase 31 Network Batching to CURP-HT

**Date**: February 7, 2026
**Goal**: Port the successful Phase 31.4 network batching optimization from CURP-HO to CURP-HT

---

## Background

### Phase 31 Achievement (CURP-HO)

**Optimization**: Added configurable batch delay to network batcher
**Result**: +18.6% peak throughput (16.0K → 22.8K @ batchDelayUs=150)
**Impact**: Reduced syscall overhead by ~75%, paradoxically improved latency by 23%

**Key Finding**: System is I/O bound (49% CPU, 38.76% in syscalls), not CPU bound

### Current CURP-HT Status

**Phase 18 Achievement**: 17.0K sustained, 18.96K peak
**Optimizations Applied**:
- ✅ String caching (Phase 18.2 / Phase 19.1)
- ✅ Pre-allocated closed channel (Phase 18.2 / Phase 19.2)
- ✅ Faster spin-wait (Phase 18.2 / Phase 19.3)
- ✅ Pipeline depth tuning (pendings=20, Phase 18.3)
- ✅ MaxDescRoutines tuning (200, Phase 18.4)

**Missing Optimization**:
- ❌ **Network batching delay** (Phase 31.4) ← NOT APPLIED TO CURP-HT

---

## Optimization Gap Analysis

### CURP-HO (Phase 31) vs CURP-HT (Phase 18)

| Component | CURP-HO | CURP-HT | Status |
|-----------|---------|---------|--------|
| String caching | ✅ Phase 31 | ✅ Phase 19.1 | Both have |
| Pre-allocated closed channel | ✅ Phase 31 | ✅ Phase 19.2 | Both have |
| Faster spin-wait | ✅ Phase 31 | ✅ Phase 19.3 | Both have |
| Pipeline depth (pendings) | 15 (Phase 31.5c) | 20 (Phase 18.3) | Both tuned |
| MaxDescRoutines | 500 (Phase 31.5b) | 200 (Phase 18.4) | Both tuned |
| **Network batch delay** | ✅ **150μs** (Phase 31.4) | ❌ **0μs** | **MISSING** |

**Impact of Missing Optimization**:
- CURP-HO gained +18.6% peak throughput from batching
- CURP-HT likely has same I/O bottleneck (syscall overhead)
- Expected CURP-HT improvement: +15-20% (similar to CURP-HO)

---

## Proposed Optimization: Phase 32

### Goal

Port Phase 31.4 network batching optimization to CURP-HT to reduce syscall overhead.

**Target**: Increase CURP-HT throughput from 18.96K peak → 22-23K peak

### Hypothesis

CURP-HT likely has the same I/O bottleneck as CURP-HO:
- Network syscalls dominate latency (30-40% of CPU time)
- Small batch sizes (1-2 messages per batch)
- Adding batch delay will reduce syscalls → increase throughput

### Implementation Plan

#### Phase 32.1: Baseline Measurement [PENDING]

**Goal**: Establish current CURP-HT performance baseline

**Tasks**:
- [ ] Run CURP-HT benchmark with current config (pendings=20, maxDescRoutines=200)
- [ ] Measure: throughput, latency (strong/weak), slow path rate
- [ ] Configuration: 2 clients × 2 threads, 10K ops per run, 5 iterations
- [ ] Document baseline in docs/phase-32-baseline.md

**Expected Baseline** (from Phase 18):
- Throughput: ~17-19K ops/sec
- Strong median: ~5-6ms
- Weak median: ~2-3ms

**Output**: docs/phase-32-baseline.md

---

#### Phase 32.2: CPU Profiling (Optional) [PENDING]

**Goal**: Verify that CURP-HT has the same I/O bottleneck as CURP-HO

**Tasks**:
- [ ] Enable pprof in CURP-HT replica
- [ ] Collect 30s CPU profile under load
- [ ] Analyze: % CPU in syscalls, network I/O, serialization
- [ ] Compare to CURP-HO profile (Phase 31.2)

**Expected Findings**:
- CPU utilization: ~50% (I/O bound)
- Network syscalls: 30-40% of CPU time (primary bottleneck)
- Similar profile to CURP-HO

**Decision Point**: If syscall % is high (>30%), proceed with batching. If low, investigate other bottlenecks.

**Output**: docs/phase-32.2-cpu-profile.md (optional, skip if confident based on CURP-HO results)

---

#### Phase 32.3: Port Network Batching to CURP-HT [PENDING]

**Goal**: Implement configurable batch delay in CURP-HT batcher

**Tasks**:
- [ ] Add `batchDelay time.Duration` field to CURP-HT Batcher struct
- [ ] Add `SetBatchDelay(ns int64)` method
- [ ] Modify batcher run loop to implement delay before sending
- [ ] Apply batch delay from config in New() (curp-ht/curp-ht.go)
- [ ] Add batch statistics (optional, for debugging)

**Implementation Details**:

**File**: `curp-ht/batcher.go`

```go
type Batcher struct {
    acks chan rpc.Serializable
    accs chan rpc.Serializable
    batchDelay time.Duration  // NEW: configurable delay
}

// NEW: SetBatchDelay sets the batch delay in nanoseconds
func (b *Batcher) SetBatchDelay(ns int64) {
    b.batchDelay = time.Duration(ns)
}

// MODIFIED: Add delay before sending (in run loop)
func NewBatcher(r *Replica, size int) *Batcher {
    b := &Batcher{
        acks: make(chan rpc.Serializable, size),
        accs: make(chan rpc.Serializable, size),
    }

    go func() {
        for !r.Shutdown {
            select {
            case op := <-b.acks:
                // NEW: Wait for batchDelay to accumulate more messages
                if b.batchDelay > 0 {
                    time.Sleep(b.batchDelay)
                }

                l1 := len(b.acks) + 1
                l2 := len(b.accs)
                aacks := &MAAcks{
                    Acks:    make([]MAcceptAck, l1),
                    Accepts: make([]MAccept, l2),
                }
                // ... rest of batching logic unchanged ...
                r.sender.SendToAll(aacks, r.cs.aacksRPC)

            case op := <-b.accs:
                // NEW: Same delay for accepts
                if b.batchDelay > 0 {
                    time.Sleep(b.batchDelay)
                }

                l1 := len(b.acks)
                l2 := len(b.accs) + 1
                // ... rest of batching logic unchanged ...
                r.sender.SendToAll(aacks, r.cs.aacksRPC)
            }
        }
    }()

    return b
}
```

**File**: `curp-ht/curp-ht.go` (initialization)

```go
func New(...) *Replica {
    // ... existing initialization ...

    r.batcher = NewBatcher(r, 128)

    // NEW: Apply batch delay from config (Phase 32.3)
    if conf.BatchDelayUs > 0 {
        r.batcher.SetBatchDelay(int64(conf.BatchDelayUs * 1000)) // Convert μs to ns
    }

    // ... rest of initialization ...
}
```

**Verification**:
- [ ] Code compiles: `go build -o swiftpaxos .`
- [ ] Unit tests pass: `go test ./curp-ht/`
- [ ] No regressions with batchDelayUs=0 (backward compatible)

**Output**: Code changes in curp-ht/batcher.go and curp-ht/curp-ht.go

---

#### Phase 32.4: Test Network Batching [PENDING]

**Goal**: Find optimal batch delay for CURP-HT

**Tasks**:
- [ ] Test batch delays: 0, 50, 75, 100, 125, 150, 200μs
- [ ] Measure throughput and latency at each delay
- [ ] Find sweet spot (max throughput, acceptable latency)
- [ ] Compare to CURP-HO optimal (150μs)

**Test Configuration**:
```yaml
protocol: curpht
maxDescRoutines: 200
batchDelayUs: [0, 50, 75, 100, 125, 150, 200]  # Test each
pendings: 20
clientThreads: 2
clients: 2
```

**Expected Results** (based on CURP-HO Phase 31.4):
- batchDelayUs=0: ~17-19K ops/sec (baseline)
- batchDelayUs=50: ~20-21K ops/sec
- batchDelayUs=75: ~21-22K ops/sec
- batchDelayUs=100: ~21-22K ops/sec
- **batchDelayUs=150**: ~22-23K ops/sec (expected optimal)
- batchDelayUs=200: ~22-23K ops/sec (diminishing returns)

**Metrics to Track**:
- Throughput (ops/sec)
- Strong median latency
- Weak median latency
- Batch size (messages per batch)

**Decision Criteria**:
- Maximize throughput
- Weak median latency < 3ms (acceptable for CURP-HT)
- Strong median latency < 7ms

**Output**: docs/phase-32.4-network-batching-results.md

---

#### Phase 32.5: Validation [PENDING]

**Goal**: Validate that network batching improves CURP-HT performance

**Tasks**:
- [ ] Run 10 iterations with optimal batchDelayUs (likely 150μs)
- [ ] Calculate min/max/avg/stddev for throughput and latency
- [ ] Compare to baseline (Phase 32.1)
- [ ] Document improvement

**Success Criteria**:
- Throughput: ≥ 21K ops/sec sustained, 22-23K peak
- Strong median: < 7ms
- Weak median: < 3ms
- Improvement: +10-20% over baseline

**Validation Test**:
```bash
# Configuration
protocol: curpht
maxDescRoutines: 200
batchDelayUs: 150  # Optimal from Phase 32.4
pendings: 20
clientThreads: 2
clients: 2
reqs: 10000

# Run 10 iterations
for i in {1..10}; do
    ./run-multi-client.sh -c multi-client.conf
done
```

**Output**: docs/phase-32.5-validation-results.md

---

#### Phase 32.6: Final Documentation [PENDING]

**Goal**: Document Phase 32 optimization and results

**Tasks**:
- [ ] Update todo.md with Phase 32 completion
- [ ] Document optimal configuration
- [ ] Create summary document
- [ ] Update CURP-HT status to reflect new performance

**Deliverables**:
- docs/phase-32-summary.md
- docs/phase-32-final-config.md
- Updated todo.md (CURP-HT section)

**Final Configuration** (expected):
```yaml
Protocol: curpht
MaxDescRoutines: 200
BatchDelayUs: 150         # NEW - Phase 32 optimization
Pendings: 20
ClientThreads: 2
Clients: 2

Expected Performance:
  Throughput: 21-23K ops/sec (sustained-peak)
  Strong Median Latency: 5-6ms
  Weak Median Latency: 2-3ms
```

---

## Expected Impact

### Throughput Improvement

| Metric | Baseline (Phase 18) | Expected (Phase 32) | Improvement |
|--------|---------------------|---------------------|-------------|
| **Peak Throughput** | 18.96K ops/sec | 22-23K ops/sec | +15-21% |
| **Sustained Throughput** | 17.0K ops/sec | 20-21K ops/sec | +18-24% |

### Latency Impact

**Expected** (based on CURP-HO results):
- Weak latency: Same or slightly better (batching reduces queueing)
- Strong latency: +0.1-0.2ms (acceptable trade-off)

### Why This Will Work

1. **Proven Approach**: Same optimization worked for CURP-HO (+18.6% peak)
2. **Same Bottleneck**: Both protocols likely I/O bound (syscall overhead)
3. **Low Risk**: Backward compatible (batchDelayUs=0 = current behavior)
4. **Easy to Implement**: ~50 lines of code, reuses CURP-HO pattern

---

## Implementation Effort

| Phase | Description | Effort | Lines of Code |
|-------|-------------|--------|---------------|
| 32.1 | Baseline measurement | 1-2 hours | 0 (testing only) |
| 32.2 | CPU profiling (optional) | 1-2 hours | 0 (testing only) |
| 32.3 | Port batching code | 2-3 hours | ~50 LOC |
| 32.4 | Test batch delays | 2-3 hours | 0 (testing only) |
| 32.5 | Validation | 1-2 hours | 0 (testing only) |
| 32.6 | Documentation | 1-2 hours | 0 (docs only) |

**Total Effort**: 8-14 hours (1-2 days)

---

## Risk Assessment

### Low Risk

**Reasons**:
- Proven optimization (worked for CURP-HO)
- Backward compatible (batchDelayUs=0 = no change)
- Isolated code changes (batcher only)
- Can be reverted easily if issues arise

### Validation Plan

1. **Unit tests**: Ensure no regressions
2. **Backward compatibility**: Test with batchDelayUs=0
3. **Incremental testing**: Test each delay value separately
4. **Comparison**: Compare to Phase 18 baseline

---

## Alternative Approaches (Lower Priority)

### 1. Different Batch Delay Values

If 150μs doesn't work well for CURP-HT, try:
- Lower: 75-100μs (lower latency, less batching)
- Higher: 200-300μs (more batching, higher latency)

### 2. Adaptive Batching

Adjust batch delay based on load:
- High load: Increase delay (more batching)
- Low load: Decrease delay (lower latency)

**Complexity**: High (requires load detection heuristic)
**Expected gain**: +3-5% over fixed delay
**Recommendation**: Defer to future work

### 3. Profile-Driven Optimization

If batching doesn't help (unlikely):
- CPU profile CURP-HT to find actual bottleneck
- Optimize based on profiling results

---

## Comparison: CURP-HO vs CURP-HT

### Current Performance Gap

| Protocol | Throughput | Weak Latency | Strong Latency | Status |
|----------|------------|--------------|----------------|--------|
| **CURP-HO** | 23.0K peak | 1.41ms | 2.76ms | Phase 31 ✅ |
| **CURP-HT** | 18.96K peak | 2.01ms | 3.29ms | Phase 18 ✅ |
| **Gap** | -4.04K (-17.5%) | +0.6ms | +0.53ms | Phase 32 target |

### After Phase 32 (Expected)

| Protocol | Throughput | Weak Latency | Strong Latency | Status |
|----------|------------|--------------|----------------|--------|
| **CURP-HO** | 23.0K peak | 1.41ms | 2.76ms | Phase 31 ✅ |
| **CURP-HT** | 22-23K peak | ~2.0ms | ~5-6ms | Phase 32 target |
| **Gap** | 0-1K (0-4%) | +0.6ms | +2-3ms | Throughput parity |

**Note**: CURP-HO has better latency due to 1-RTT causal ops (bound replica), while CURP-HT requires 1-RTT to leader. This is expected and not a gap to close.

---

## Success Criteria

### Primary Goals

- ✅ **Throughput**: ≥ 21K ops/sec sustained, 22-23K peak
- ✅ **Latency**: Weak < 3ms, Strong < 7ms
- ✅ **Improvement**: +15-20% over Phase 18 baseline

### Secondary Goals

- ✅ **Backward compatible**: Works with batchDelayUs=0
- ✅ **No regressions**: All tests pass
- ✅ **Reproducible**: 10 validation runs, <10% variance

### Stretch Goals

- ✅ **Match CURP-HO throughput**: 23K peak
- ✅ **Weak latency**: < 2.5ms (better than Phase 18)

---

## Next Steps

### Immediate Actions

1. **Read this plan**: Understand the optimization and expected impact
2. **Run Phase 32.1**: Establish CURP-HT baseline performance
3. **Implement Phase 32.3**: Port batching code from CURP-HO
4. **Test Phase 32.4**: Find optimal batch delay

### Decision Points

**After Phase 32.1 (Baseline)**:
- If baseline is already >21K: Success! No optimization needed.
- If baseline is 17-19K: Proceed with batching (likely scenario)
- If baseline is <15K: Investigate regression from Phase 18

**After Phase 32.2 (CPU Profile) - Optional**:
- If syscall % > 30%: Proceed with batching (high confidence)
- If syscall % < 20%: Investigate other bottlenecks first

**After Phase 32.4 (Testing)**:
- If optimal delay found: Proceed to validation
- If no improvement: Investigate alternative approaches

---

## Summary

**Goal**: Port Phase 31.4 network batching optimization to CURP-HT

**Expected Impact**: +15-20% throughput (18.96K → 22-23K peak)

**Effort**: 8-14 hours (1-2 days)

**Risk**: Low (proven optimization, backward compatible)

**Implementation**: Add batch delay to CURP-HT batcher (same as CURP-HO)

**Rationale**: CURP-HT likely has same I/O bottleneck as CURP-HO (syscall overhead)

**Status**: Ready to implement

---

## References

- **Phase 31.4**: CURP-HO network batching optimization
  - docs/phase-31.4-network-batching.md
  - Achieved +18.6% peak throughput

- **Phase 31.2**: CURP-HO CPU profiling
  - docs/phase-31.2-cpu-profile.md
  - Found 38.76% CPU time in syscalls

- **Phase 18**: CURP-HT optimization baseline
  - Achieved 18.96K peak with configuration tuning
  - Did not include network batching

- **Phase 19**: Port Phase 18 optimizations to CURP-HT
  - String caching, closed channel, faster spin-wait
  - No network batching

**Phase 32 fills the gap: Apply the missing network batching optimization to CURP-HT.**
