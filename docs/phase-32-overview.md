# Phase 32 Overview: Port Network Batching to CURP-HT

**Status**: PENDING - Ready to implement
**Goal**: Apply Phase 31.4 network batching optimization to CURP-HT
**Expected Impact**: +15-21% throughput (18.96K → 22-23K peak)

---

## Quick Summary

### What's Missing?

CURP-HT is missing the **network batching optimization** from Phase 31.4:

| Optimization | CURP-HO | CURP-HT | Status |
|--------------|---------|---------|--------|
| String caching | ✅ Phase 31 | ✅ Phase 19.1 | Both have |
| Pre-allocated closed channel | ✅ Phase 31 | ✅ Phase 19.2 | Both have |
| Faster spin-wait | ✅ Phase 31 | ✅ Phase 19.3 | Both have |
| **Network batch delay** | ✅ **150μs** | ❌ **0μs** | **MISSING** |

### Why Port This Optimization?

**Phase 31.4 Results (CURP-HO)**:
- Added configurable `batchDelayUs` parameter to batcher
- Tested 0, 50, 75, 100, 150, 200μs delays
- **Optimal**: 150μs → 22.8K ops/sec (+18.6% peak!)
- **Why it worked**: Reduced syscalls by ~75% (18K → 5K syscalls/sec)
- **Surprise**: Latency improved despite added delay (fewer syscalls = less queueing)

**Key Finding**: System is **I/O bound** (49% CPU, 38.76% in syscalls), not CPU bound

**Hypothesis**: CURP-HT likely has the same bottleneck → same optimization should work

---

## Current Performance Gap

| Protocol | Throughput | Weak Latency | Strong Latency | Optimization |
|----------|------------|--------------|----------------|--------------|
| **CURP-HO** (Phase 31) | 23.0K peak | 1.41ms | 2.76ms | Has batching ✅ |
| **CURP-HT** (Phase 19) | 18.96K peak | 2.01ms | 3.29ms | No batching ❌ |
| **Gap** | -4.04K (-17.5%) | +0.6ms | +0.53ms | |

**After Phase 32 (Expected)**:
- CURP-HT: 22-23K peak (throughput parity!)
- Network batching closes the performance gap

---

## Implementation Plan (6 Phases)

### Phase 32.1: Baseline Measurement (1-2 hours)
Run CURP-HT benchmark, establish baseline (~17-19K ops/sec expected)

### Phase 32.2: CPU Profiling - Optional (1-2 hours)
Verify syscall overhead is the bottleneck (expected: 30-40% CPU time)

### Phase 32.3: Port Batching Code (2-3 hours)
Add batch delay to CURP-HT batcher (~50 lines of code)

**Files to Modify**:
- `curp-ht/batcher.go`: Add `batchDelay` field and `SetBatchDelay()` method
- `curp-ht/curp-ht.go`: Apply batch delay from config in `New()`

**Implementation Pattern** (same as CURP-HO):
```go
// In batcher run loop
if b.batchDelay > 0 {
    time.Sleep(b.batchDelay)
}
// ... rest of batching logic ...
```

### Phase 32.4: Test Batch Delays (2-3 hours)
Test delays: 0, 50, 75, 100, 125, 150, 200μs
Find optimal (expected: 150μs like CURP-HO)

### Phase 32.5: Validation (1-2 hours)
Run 10 iterations with optimal delay, validate improvement

### Phase 32.6: Documentation (1-2 hours)
Update todo.md, create summary documents

**Total Effort**: 8-14 hours (1-2 days)

---

## Why This Will Work

### 1. Proven Optimization
- CURP-HO: +18.6% peak throughput
- Same code pattern, same optimization

### 2. Same Bottleneck (Likely)
- Both protocols use similar batching code
- Both do network I/O via syscalls
- Same RPC framework (fastrpc)

### 3. Low Risk
- Backward compatible (batchDelayUs=0 = current behavior)
- Isolated code changes (batcher only)
- Can be reverted easily

### 4. Easy to Implement
- ~50 lines of code
- Reuses CURP-HO pattern
- 1-2 days total effort

---

## Expected Results

### Throughput

| Metric | Before (Phase 19) | After (Phase 32) | Improvement |
|--------|-------------------|------------------|-------------|
| **Peak** | 18.96K | 22-23K | +15-21% |
| **Sustained** | 17.0K | 20-21K | +18-24% |

### Latency

**Expected** (based on CURP-HO):
- Weak: Same or slightly better
- Strong: +0.1-0.2ms (acceptable)

### Protocol Comparison (After Phase 32)

| Protocol | Throughput | Use Case |
|----------|------------|----------|
| **CURP-HO** | 23.0K peak | Geo-distributed, optimal weak latency |
| **CURP-HT** | 22-23K peak | Leader-centric, simpler protocol |

**Throughput parity achieved!** Choose based on deployment topology.

---

## Quick Start

### 1. Read the Plan
```bash
cat docs/phase-32-curp-ht-optimization-plan.md
```

### 2. Run Baseline (Phase 32.1)
```bash
# Set config to CURP-HT
# protocol: curpht
# pendings: 20
# maxDescRoutines: 200

./run-multi-client.sh -c multi-client.conf
# Measure baseline throughput
```

### 3. Implement Batching (Phase 32.3)
```bash
# Edit curp-ht/batcher.go
# Add batchDelay field and SetBatchDelay method
# Add delay in run loop

# Edit curp-ht/curp-ht.go
# Apply batch delay from config in New()

go build -o swiftpaxos .
go test ./curp-ht/
```

### 4. Test Delays (Phase 32.4)
```bash
# Test batchDelayUs: 0, 50, 75, 100, 125, 150, 200
# Find optimal delay
```

### 5. Validate (Phase 32.5)
```bash
# Run 10 iterations with optimal delay
# Verify improvement
```

---

## Files to Reference

### Phase 31 Implementation (CURP-HO)
- `curp-ho/batcher.go`: Network batching with delay
- `curp-ho/curp-ho.go`: Config initialization (lines 176-178)
- `docs/phase-31.4-network-batching.md`: Detailed analysis

### Phase 32 Plan (CURP-HT)
- `docs/phase-32-curp-ht-optimization-plan.md`: Complete plan
- `todo.md`: Phase 32 task breakdown

### Code Pattern to Copy
```go
// From curp-ho/batcher.go (SetBatchDelay method)
func (b *Batcher) SetBatchDelay(ns int64) {
    b.batchDelay = time.Duration(ns)
}

// From curp-ho/curp-ho.go (initialization)
if conf.BatchDelayUs > 0 {
    r.batcher.SetBatchDelay(int64(conf.BatchDelayUs * 1000))
}

// From curp-ho/batcher.go (run loop)
if b.batchDelay > 0 {
    time.Sleep(b.batchDelay)
}
```

---

## Success Criteria

### Primary Goals
- ✅ Throughput: ≥ 21K sustained, 22-23K peak
- ✅ Latency: Weak < 3ms, Strong < 7ms
- ✅ Improvement: +15-20% over baseline

### Validation
- ✅ 10 validation runs, <10% variance
- ✅ All tests pass
- ✅ Backward compatible (batchDelayUs=0)

---

## Next Actions

1. **Read the full plan**: `docs/phase-32-curp-ht-optimization-plan.md`
2. **Run baseline**: Establish current CURP-HT performance
3. **Implement batching**: Port code from CURP-HO
4. **Test and validate**: Find optimal delay, verify improvement

**Phase 32 is ready to go!** 🚀
