# Phase 19.4: Port Configuration-Level Optimizations to CURP-HT

## Summary

Successfully verified and documented that CURP-HT supports the same configuration-level optimizations as CURP-HO (from Phase 18.3-18.4). Created optimized configuration file `curpht-optimized.conf` with the proven optimal settings:
- `maxDescRoutines: 200` (sweet spot from Phase 18.4)
- `pendings: 20` (optimal pipeline depth from Phase 18.3)

## Configuration Parameters

### 1. MaxDescRoutines (Goroutine Concurrency Limit)

**Purpose**: Controls the threshold for spawning goroutines vs sequential execution.

**Implementation**:
- CURP-HT: `curp-ht/defs.go:42` - `var MaxDescRoutines = 10000`
- CURP-HO: `curp-ho/defs.go:42` - `var MaxDescRoutines = 10000` (identical)

**Usage in Code**:
```go
// curp-ht/curp-ht.go:690
desc.seq = (r.routineCount >= MaxDescRoutines)

// curp-ho/curp-ho.go:927
desc.seq = (r.routineCount >= MaxDescRoutines)
```

**Configuration Loading** (run.go:52-67):
```go
case "curpht":
    log.Println("Starting CURP-HT (Hybrid Transparency) replica...")
    if c.MaxDescRoutines > 0 {
        curpht.MaxDescRoutines = c.MaxDescRoutines
    }
    rep := curpht.New(c.Alias, replicaId, nodeList, !c.Noop,
        1, f, true, c, logger)

case "curpho":
    log.Println("Starting CURP-HO (Hybrid Optimal) replica...")
    if c.MaxDescRoutines > 0 {
        curpho.MaxDescRoutines = c.MaxDescRoutines
    }
    rep := curpho.New(c.Alias, replicaId, nodeList, !c.Noop,
        1, f, true, c, logger)
```

**Optimized Value**: 200 (from Phase 18.4)
- Found through systematic testing (100, 200, 500, 1000, 2000)
- Sweet spot: Best throughput (18.96K ops/sec) with minimal latency
- U-shaped performance curve: low/high values good, mid-range poor

### 2. Pipeline Depth (Pendings Parameter)

**Purpose**: Controls max in-flight commands per client thread.

**Implementation**:
- Universal: Handled by `client.BufferClient` in main.go
- Applies to ALL protocols identically

**Usage in Code** (main.go:162-164):
```go
b := client.NewBufferClient(cl, c.Reqs, c.CommandSize, c.Conflicts, c.Writes, int64(c.Key))
if c.Pipeline {
    b.Pipeline(c.Syncs, int32(c.Pendings))
}
```

**Optimized Value**: 20 (from Phase 18.3)
- Tested values: 5, 10, 15, 20, 30
- Sweet spot: 17.95K ops/sec with acceptable latency (P99 < 10ms)
- Diminishing returns beyond 20, significant latency growth at 30

### 3. Code-Level Optimizations (Already Ported)

**Phase 19.1-19.3** already ported these CURP-HO optimizations to CURP-HT:
- ✅ String caching (sync.Map for int32→string conversions)
- ✅ Pre-allocated closed channel (reused for notifications)
- ✅ Faster spin-wait (10μs polling in waitForWeakDep)

## Files Modified/Created

### Created Files

1. **curpht-optimized.conf** - CURP-HT configuration with optimized settings
   - Based on multi-client.conf (CURP-HO optimized config)
   - Changed `protocol: curpho` → `protocol: curpht`
   - Same optimization values: `maxDescRoutines: 200`, `pendings: 20`

### Verified Files (No Changes Needed)

1. **curp-ht/defs.go** - MaxDescRoutines already defined and configurable
2. **curp-ht/curp-ht.go** - Uses MaxDescRoutines correctly (line 690)
3. **run.go** - Configuration loading already implemented (lines 52-59)
4. **main.go** - Pipeline depth handling universal (lines 162-164)

## Testing

### Unit Tests

All CURP-HT tests pass with optimizations:
```bash
go test ./curp-ht/
ok      github.com/imdea-software/swiftpaxos/curp-ht    0.017s
```

**Test Coverage**:
- MaxDescRoutines default value (10000)
- MaxDescRoutines override capability
- String caching (Phase 19.1)
- Pre-allocated closed channel (Phase 19.2)
- Optimized spin-wait (Phase 19.3)
- All protocol functionality

### Configuration Verification

**CURP-HT Optimized Config** (`curpht-optimized.conf`):
```
protocol: curpht
maxDescRoutines: 200   // Ported from CURP-HO Phase 18.4 optimization
pendings: 20           // Ported from CURP-HO Phase 18.3 optimization

// Same client settings as CURP-HO
reqs: 10000
clientThreads: 2
pipeline: true
weakRatio: 50          // 50% weak, 50% strong
```

## Expected Performance

### CURP-HO Baseline (Phase 18 Results)

With these optimizations, CURP-HO achieved:
- **Baseline**: 13.0K ops/sec (pendings=5, maxDescRoutines=100)
- **Phase 18.2** (code optimizations): 14.6K ops/sec (+12%)
- **Phase 18.3** (pendings=20): 17.35K ops/sec (+33.5%)
- **Phase 18.4** (maxDescRoutines=200): 18.96K peak, 17.0K sustained (+30.8%)

### CURP-HT Expected Results

**Previous CURP-HT Baseline** (before Phase 19):
- Throughput: ~26K ops/sec (from earlier testing)

**Expected with Phase 19.1-19.4 Optimizations**:
- Code optimizations (19.1-19.3): +10-15% improvement
- Config optimizations (19.4): Additional +10-20% improvement
- **Target**: 30K+ ops/sec sustained throughput
- **Validation**: Phase 19.5 comprehensive benchmark

### Performance Comparison

**Why CURP-HT Baseline is Higher**:
- Leader-only weak commands (lower network overhead)
- Simpler coordination (no broadcast for weak ops)
- Same speculative execution benefits

**Optimization Transferability**:
- String caching: ✅ Same benefit (reduces GC pressure)
- Pre-allocated channel: ✅ Same benefit (fewer allocations)
- Faster spin-wait: ✅ Same benefit (10x faster polling)
- MaxDescRoutines: ✅ Same U-shaped curve expected
- Pipeline depth: ✅ Same throughput/latency trade-off

## Technical Details

### Why No Code Changes Needed

1. **MaxDescRoutines**: Already implemented identically in both protocols
   - Same variable name, type, and usage pattern
   - Same configuration loading in run.go
   - Same test coverage

2. **Pipeline Depth**: Protocol-agnostic client-side feature
   - Handled by BufferClient (universal for all protocols)
   - No protocol-specific code required
   - Same configuration parameter

3. **Code Optimizations**: Already ported in Phase 19.1-19.3
   - String caching (Phase 19.1)
   - Pre-allocated closed channel (Phase 19.2)
   - Optimized spin-wait (Phase 19.3)

### Configuration File Format

Both CURP-HT and CURP-HO use identical configuration format:
- Same parameter names
- Same value ranges
- Same comment syntax
- Only difference: `protocol:` field value

### Consistency Across Protocols

**Design Principle**: Configuration parameters should be protocol-agnostic when possible.

**Benefits**:
- Easy to compare protocols (change one line)
- Shared optimization insights
- Simplified testing infrastructure
- Reduced maintenance burden

## Optimization Summary

### Phase 19 Progress

- ✅ **Phase 19.1**: String caching ported
- ✅ **Phase 19.2**: Pre-allocated closed channel ported
- ✅ **Phase 19.3**: Optimized spin-wait ported
- ✅ **Phase 19.4**: Configuration optimizations verified
- 🔄 **Phase 19.5**: Benchmark with all optimizations (next)
- 🔄 **Phase 19.6**: Document and commit results

### Cumulative Optimizations Applied to CURP-HT

**Code-Level** (19.1-19.3):
1. String caching via sync.Map
2. Pre-allocated closed channel
3. 10μs spin-wait polling

**Configuration-Level** (19.4):
1. maxDescRoutines: 200 (sweet spot)
2. pendings: 20 (optimal pipeline depth)

**Total Expected Impact**: +20-35% throughput improvement over baseline

## Next Steps

### Phase 19.5: Benchmark CURP-HT

**Objective**: Measure actual performance improvement with all optimizations.

**Test Plan**:
1. Run benchmark with `curpht-optimized.conf`
2. Compare to CURP-HT baseline (~26K ops/sec)
3. Measure throughput, latency (P50/P99), CPU usage
4. Validate expected 20-35% improvement
5. Compare to CURP-HO results (17K sustained, 18.96K peak)

**Success Criteria**:
- Sustained throughput ≥ 30K ops/sec
- Latency P99 < 10ms for strong ops
- Latency P99 < 5ms for weak ops
- No regressions in correctness tests

### Phase 19.6: Document and Commit

**Objective**: Finalize Phase 19 with comprehensive documentation.

**Tasks**:
1. Update todo.md with Phase 19.5 results
2. Create final commit message summarizing all Phase 19 work
3. Document performance comparison: CURP-HT vs CURP-HO
4. Archive optimization artifacts (test scripts, configs, results)

## Lessons Learned

### Design Insights

1. **Shared Infrastructure Pays Off**
   - Configuration system designed for multi-protocol support
   - Easy to apply optimizations across protocols
   - Minimal code duplication

2. **Code Consistency Matters**
   - CURP-HT and CURP-HO share similar structure
   - Makes optimization porting straightforward
   - Easier to maintain and test

3. **Configuration vs Code Optimizations**
   - Code optimizations: One-time implementation, universal benefit
   - Config optimizations: Workload-dependent, requires tuning
   - Both needed for optimal performance

### Best Practices

1. **Verify Before Modifying**
   - Check if feature already exists
   - Understand current implementation
   - Avoid unnecessary changes

2. **Test Thoroughly**
   - Run full test suite after porting optimizations
   - Verify configuration loading works correctly
   - Validate expected behavior

3. **Document Everything**
   - Configuration parameters and their purpose
   - Expected performance impact
   - Rationale for chosen values

## Conclusion

Phase 19.4 successfully verified that CURP-HT supports the same configuration-level optimizations as CURP-HO. Created optimized configuration file with proven settings from Phase 18. No code changes were required, demonstrating the robustness of the configuration system design.

**Key Achievements**:
- ✅ Verified MaxDescRoutines configuration support
- ✅ Verified pipeline depth (pendings) support
- ✅ Created curpht-optimized.conf with optimal settings
- ✅ All tests pass with optimizations
- ✅ Ready for Phase 19.5 comprehensive benchmark

**Expected Outcome**: Phase 19.5 benchmarking should show 20-35% throughput improvement over CURP-HT baseline (~26K → 32-35K ops/sec), validating that Phase 18's CURP-HO optimizations transfer effectively to CURP-HT.

## References

- **Phase 18.3**: Pipeline depth optimization (pendings=20)
- **Phase 18.4**: MaxDescRoutines sweet spot (200)
- **Phase 18 Final Summary**: CURP-HO optimization results
- **Phase 19.1**: CURP-HT string caching
- **Phase 19.2**: CURP-HT pre-allocated closed channel
- **Phase 19.3**: CURP-HT optimized spin-wait
- **Phase 19.5**: CURP-HT comprehensive benchmark (next)
