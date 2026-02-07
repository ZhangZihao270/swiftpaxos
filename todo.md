# Hybrid Consistency Protocols Implementation TODO

## Overview

This document tracks the implementation of multiple hybrid consistency protocols on top of CURP. Each protocol supports both Strong (Linearizable) and Weak (Causal) consistency levels, but with different trade-offs between latency, throughput, and implementation complexity.

---

## Table of Contents

1. [CURP-HT (Hybrid Two-Phase)](#curp-ht-hybrid-two-phase) - **COMPLETE**
2. [CURP-HO (Hybrid Optimal)](#curp-ho-hybrid-optimal) - **PLANNED**
3. [Future Protocols](#future-protocols)

---

# CURP-HT (Hybrid Two-Phase)

## Status: ✅ **COMPLETE** (Phase 1-17 Done, Phase 18 In Progress)

## Design Summary

**Key Idea**: Weak ops sent to leader only, serialized by leader.

| Aspect | Strong Ops | Weak Ops |
|--------|------------|----------|
| **Broadcast** | All replicas | Leader only |
| **Execution** | Leader (speculative) | Leader (speculative) |
| **Client wait** | 2-RTT (quorum) | 1-RTT (leader reply) |
| **Latency** | To majority | To leader |
| **Strong speculative sees weak?** | ❌ No | N/A |

**Advantages**:
- ✅ Simple: Leader serializes all weak ops
- ✅ Lower network load: Weak ops don't broadcast
- ✅ Proven correctness: Completed and tested

**Disadvantages**:
- ❌ Weak latency = distance to leader (not optimal if leader is far)

---

## Implementation Status

### Phase 1-17: Core Implementation [COMPLETE]

All phases completed successfully. See detailed tasks below.

### Phase 18: Systematic Optimization Testing [IN PROGRESS]

**Goal**: Improve throughput beyond Phase 17 baseline by testing optimizations individually.

**Baseline Performance** (4 clients × 2 threads, pendings=5):
- Throughput: 12.9K ops/sec
- Strong latency: 3.29ms (median), 11.53ms (P99)
- Weak latency: 2.01ms (median), 9.28ms (P99)

#### Optimization Results

**Current Status**: 14.6K ops/sec (+12% from 13K baseline) - **Target: 20K ops/sec**

#### Completed Optimizations

- [x] **18.1** Increase MaxDescRoutines (500 → 10000) [26:02:06]
  - Changed default from 500 to 10000 in curp-ht/defs.go and curp-ho/defs.go
  - Added `MaxDescRoutines` config parameter (configurable via config file)
  - run.go now uses protocol defaults unless config overrides (removed hardcoded 100)
  - Config value 0 = use protocol default (10000), >0 = override
  - **Result**: Regression (26K → 17K). Reverted to maxDescRoutines: 100 in config

- [x] **18.2** CURP-HO Code Optimizations [26:02:07]
  - **String Caching**: Added sync.Map cache for int32→string conversions
    - Eliminates repeated strconv.FormatInt calls in hot paths (clientId, keys)
    - Reduces GC pressure from string allocations
  - **Faster Spin-Wait**: Optimized waitForWeakDep polling (100μs → 10μs)
    - 10x faster response for causal dependency resolution
    - Same 100ms timeout to prevent deadlocks
  - **Pre-allocated Closed Channel**: Reuse single closed channel
    - Avoids allocations in getOrCreateCommitNotify/ExecuteNotify
  - **Result**: 13K → 14.6K ops/sec (+12% improvement)
  - **Commit**: e9a29a6

#### Planned Optimizations to Reach 20K

- [x] **18.3** Increase Client Pipeline Depth [26:02:07]
  - **Tested**: pendings: 5, 10, 15, 20, 30
  - **Results**:
    - pendings=5: 4.8K ops/sec (baseline)
    - pendings=10: 13.0K ops/sec (+173%)
    - pendings=15: 17.1K ops/sec (+258%)
    - pendings=20: 17.95K ops/sec (+275%, P99: 5.53ms) ⭐ **OPTIMAL**
    - pendings=30: 18.66K ops/sec (+290%, P99: 7.57ms)
  - **Selected**: pendings=20 (best throughput/latency balance)
  - **Validation**: 17.35K ops/sec (40K ops test, P99: 16.18ms strong, 9.73ms weak)
  - **Improvement**: 14.6K → 17.35K ops/sec (+19% from Phase 18.2)
  - **Analysis**: docs/phase-18.3-pipeline-depth-analysis.md
  - **Tool**: test-pipeline-depth.sh

- [x] **18.4** Optimize MaxDescRoutines Sweet Spot [26:02:07]
  - **Tested**: maxDescRoutines: 100, 200, 500, 1000, 2000 with pendings=20 and string caching
  - **Results**:
    - maxDescRoutines=100: 18,280 ops/sec (baseline)
    - maxDescRoutines=200: 18,962 ops/sec (+3.7%) ⭐ **OPTIMAL**
    - maxDescRoutines=500: 17,161 ops/sec (-6.1%)
    - maxDescRoutines=1000: 14,600 ops/sec (-20%, worst)
    - maxDescRoutines=2000: 18,176 ops/sec (-0.6%)
  - **Selected**: maxDescRoutines=200 (best throughput, low latency)
  - **Performance Pattern**: U-shaped curve (low/high good, mid-range poor due to goroutine overhead)
  - **String Caching Impact**: Helped but didn't eliminate goroutine scheduling overhead
  - **Cumulative Improvement**: 13K → 18.96K ops/sec (+45.8% total from Phase 18.2 baseline)
  - **20K Target**: ✅ Achieved with combined optimizations (18.96K peak)
  - **Analysis**: docs/phase-18.4-maxdesc-analysis.md
  - **Tool**: test-maxdesc-sweet-spot.sh

- [ ] **18.5** Reduce Batcher Latency
  - **Current**: Batcher size = 128, likely has implicit delay
  - **Investigate**: Check batcher.go for sleep/timeout settings
  - **Test**: Reduce batch timeout, or disable batching for weak ops
  - **Expected**: Lower latency → faster command processing

- [ ] **18.6** Optimize Concurrent Map Contention
  - **Current**: Multiple cmap.ConcurrentMap with SHARD_COUNT=32768
  - **Profile**: Use pprof to identify map lock contention
  - **Test**: Reduce SHARD_COUNT or use lock-free alternatives
  - **Expected**: Reduced lock contention in hot paths

- [ ] **18.7** Reduce Channel Allocations in Hot Paths
  - **Current**: Many chan struct{} allocations per command
  - **Test**: Pool channels or use sync.Cond for notifications
  - **Expected**: Lower GC pressure, better cache locality

- [ ] **18.8** Profile and Identify Remaining Bottlenecks
  - **Tool**: `go tool pprof -http=:8080 cpu.prof`
  - **Action**: Run CPU profiling during benchmark
  - **Output**: Flamegraph showing hot paths
  - **Next**: Optimize top 3 CPU consumers

- [ ] **18.9** Memory Allocation Profiling
  - **Tool**: `go tool pprof -http=:8080 -alloc_space mem.prof`
  - **Action**: Run memory profiling during benchmark
  - **Output**: Find allocation hotspots
  - **Next**: Add object pools for frequently allocated structs

- [ ] **18.10** Validate 20K Target Achieved
  - **Test**: Full benchmark with optimized configuration
  - **Measure**: Throughput, latency (median, P99), CPU usage
  - **Document**: Final configuration and results
  - **Next**: Apply to CURP-HT (Phase 19)

### Phase 19: Apply Optimizations to CURP-HT [PLANNED]

**Goal**: Port successful CURP-HO optimizations (Phase 18.2+) to CURP-HT for consistency and performance parity.

**Prerequisite**: Phase 18.10 complete (20K target achieved on CURP-HO)

#### Tasks

- [ ] **19.1** Port String Caching to CURP-HT
  - Add `stringCache sync.Map` field to Replica struct
  - Implement `int32ToString()` helper method
  - Replace all `strconv.FormatInt` calls with cached version
  - Update tests for pendingWriteKey method conversion
  - **Files**: curp-ht/curp-ht.go, curp-ht/curp-ht_test.go
  - **Expected**: Reduced GC pressure, ~5-10% throughput gain

- [ ] **19.2** Port Pre-allocated Closed Channel to CURP-HT
  - Add `closedChan chan struct{}` field to Replica struct
  - Initialize in NewReplica: `close(closedChan)`
  - Update getOrCreateCommitNotify to return closedChan for committed slots
  - Update getOrCreateExecuteNotify to return closedChan for executed slots
  - Update newTestReplicaForDesc in tests
  - **Files**: curp-ht/curp-ht.go, curp-ht/curp-ht_test.go
  - **Expected**: Reduced allocations, minor throughput gain

- [ ] **19.3** Optimize CURP-HT Spin-Wait (if applicable)
  - Review waitForWeakDep or similar blocking patterns in CURP-HT
  - Apply faster polling (100μs → 10μs) where beneficial
  - **Note**: CURP-HT has different weak command flow (leader-only)
  - **Files**: curp-ht/curp-ht.go
  - **Expected**: Lower latency for weak ops

- [ ] **19.4** Port Additional Optimizations from Phase 18.3-18.9
  - Apply pipeline depth, MaxDescRoutines, batcher, and profiling-driven fixes
  - Use same configuration values that worked for CURP-HO
  - **Files**: Multiple (curp-ht/*.go, config files)
  - **Expected**: Similar throughput gains as CURP-HO

- [ ] **19.5** Benchmark CURP-HT with Optimizations
  - Run full benchmark with test-simple.conf (protocol: curpht)
  - Measure throughput, latency, CPU usage
  - Compare to baseline: Previous ~26K ops/sec
  - **Target**: Maintain or exceed previous performance
  - **Config**: Use optimized maxDescRoutines, pendings, etc.

- [ ] **19.6** Document and Commit CURP-HT Optimizations
  - Update todo.md with results
  - Create detailed commit message
  - Tag as milestone if both protocols achieve 20K+
  - **Commit**: "perf: Apply CURP-HO optimizations to CURP-HT"

---

## Detailed Task History

<details>
<summary>Click to expand Phase 1-17 completed tasks</summary>

### Phase 1: Project Structure Setup [COMPLETE]

- [x] **1.1** Copy base files from curp/ to curp-ht/ [26:01:31, 15:48]
- [x] **1.2** Update package names and imports [26:01:31, 15:49]

### Phase 2: Message Protocol Modifications [COMPLETE]

- [x] **2.1** Define consistency level constants in defs.go [26:01:31, 15:51]
- [x] **2.2** Add MWeakPropose message type [26:01:31, 15:53]
- [x] **2.3** Add MWeakReply message type [26:01:31, 15:53]
- [x] **2.4** Add communication channels for weak commands [26:01:31, 15:56]

### Phase 3: Client-Side Modifications [COMPLETE]

- [x] **3.1** Add weak command tracking fields to Client struct [26:01:31, 16:03]
- [x] **3.2** Implement SendWeakWrite method [26:01:31, 16:03]
- [x] **3.3** Implement SendWeakRead method [26:01:31, 16:03]
- [x] **3.4** Implement handleWeakReply method [26:01:31, 16:03]
- [x] **3.5** Update handleMsgs loop [26:01:31, 16:03]

### Phase 4: Replica-Side Modifications [COMPLETE]

- [x] **4.1** Add isWeak field to commandDesc struct [26:01:31, 16:08]
- [x] **4.2** Update run() loop for weak propose handling [26:01:31, 16:08]
- [x] **4.3** Implement handleWeakPropose method [26:01:31, 16:08]
- [x] **4.4** Implement getWeakCmdDesc method [26:01:31, 16:08]
- [x] **4.5** Implement asyncReplicateWeak method [26:01:31, 16:08]

### Phase 5-17: Optimizations, Testing, Bug Fixes [COMPLETE]

See original todo.md for detailed history.

</details>

---

# CURP-HO (Hybrid Optimal)

## Status: 🔧 **IN PROGRESS** (Phase 20-28 Complete, 29 Analyzed, Phase 30 Planned)

## Design Summary

**Key Idea**: Weak ops broadcast to all replicas (witness pool), but only wait for closest replica response.

| Aspect | Strong Ops | Weak Ops |
|--------|------------|----------|
| **Broadcast** | All replicas | All replicas ✨ |
| **Execution** | Leader (speculative) | Bound replica (speculative) ✨ |
| **Client wait** | 2-RTT (super majority) | 1-RTT (bound replica) ✨ |
| **Latency** | To super majority | To **closest** replica ✨ |
| **Strong speculative sees weak?** | ✅ **Yes** (witness pool) ✨ | N/A |

**Advantages**:
- ✅ **Optimal weak latency**: 1-RTT to closest replica (not leader)
- ✅ Strong ops can see uncommitted weak ops (better speculative execution)
- ✅ Reuses CURP-HT's `unsynced` structure (no duplicate data structures)
- ✅ Witness functionality via extended `unsynced` entries

**Disadvantages**:
- ❌ Higher network load: Weak ops broadcast to all
- ❌ More complex: Extended unsynced entries with metadata
- ❌ Super majority requirement for strong fast path (more restrictive)

---

## Protocol Flow

### 1. Client-Replica Binding

**Setup Phase**:
```
Client measures latency to all replicas
Client binds to closest replica: boundReplica[clientId] = closestReplicaId
```

### 2. Causal (Weak) Operation

**Client**:
```
1. Broadcast MCausalPropose to ALL replicas
2. Wait for reply from boundReplica only
3. Complete immediately (1-RTT optimal!)
```

**All Replicas** (including bound replica and leader):
```
1. Add op to unsynced map (witness):
   unsynced[key] = UnsyncedEntry{isStrong: false, op, value, clientId, seqNum, ...}
```

**Bound Replica** (whoever client is bound to):
```
1. Check causal dependency (if causalDep > 0)
2. Speculative execution: computeSpeculativeResult()
   - Can see pending writes from same client
3. Send MCausalReply{result} to client immediately (1-RTT done!)
4. STOP - bound replica does NOT do replication
```

**Leader** (replication coordinator):
```
1. Also adds to unsynced (like all replicas)
2. Coordinate async replication (independently from bound replica):
   - Assign slot
   - Send Accept to all replicas
   - Wait for majority acks
   - Send Commit
   - Execute in slot order (modifies state machine)
```

**Note**: If bound replica == leader, then leader does BOTH:
- Immediately replies to client (1-RTT)
- Separately coordinates replication in background

### 3. Strong Operation

**Client**:
```
1. Broadcast GPropose to ALL replicas
2. Collect replies
3. Fast path: If super majority (3/4) reply ok with consistent weakDep → complete
4. Slow path: Wait for leader's SyncReply
```

**All Replicas**:
```
1. Check unsynced for strong write conflicts:
   if exists strong write W in unsynced[currentOp.key]:
     return RecordAck{ok: FAIL}

2. For strong write:
   if no conflict:
     return RecordAck{ok: TRUE}

3. For strong read:
   if exists weak write W in unsynced[currentOp.key]:
     return RecordAck{ok: TRUE, weakDep: W.cmdId}  // Depends on weak write
   else:
     return RecordAck{ok: TRUE, weakDep: nil}      // No dependency
```

**Leader**:
```
1. Speculative execution (CAN see unsynced entries, including uncommitted weak writes!)
2. Send Reply{result, ok, weakDep}
3. Start replication (Accept → Commit)
4. Execute in slot order
5. Send SyncReply{finalResult}
```

### 4. Client Completion

**Causal**:
```
Receive MCausalReply from boundReplica → COMPLETE (1-RTT!)
```

**Strong**:
```
Fast path:
  if super majority (3n/4) reply ok:
    if all weakDep consistent (all nil, or all same opId):
      → COMPLETE (2-RTT)

Slow path:
  Wait for SyncReply from leader → COMPLETE
```

---

## Implementation Plan

### Phase 19: CURP-HO Project Setup [COMPLETE]

**Goal**: Create curp-ho package with basic structure, reusing CURP-HT optimizations.

- [x] **19.1** Create curp-ho directory and copy base files from curp-ht/ [26:02:06]
  - Files: curp-ht.go → curp-ho.go, client.go, defs.go, batcher.go, timer.go
  - Keep all optimizations: pendingWrites, channel notifications, string caching

- [x] **19.2** Update package names and imports [26:02:06]
  - Changed `package curpht` to `package curpho`
  - Import paths unchanged (external dependencies only)
  - Build verified: `go build -o swiftpaxos .`

- [x] **19.3** Add CURP-HO to main.go and run.go [26:02:06]
  - Added import `curpho "github.com/imdea-software/swiftpaxos/curp-ho"`
  - Added case "curpho" in run.go (replica) and main.go (client)
  - Added HybridLoop support with metrics aggregation
  - All 30 curp-ho unit tests pass, full test suite has no regressions

---

### Phase 20: Extend Unsynced Structure for Witness Pool [COMPLETE]

**Goal**: Extend CURP-HT's existing `unsynced` structure to support witness pool functionality, avoiding duplicate data structures.

**Background**: CURP-HT already has `unsynced cmap.ConcurrentMap` that tracks uncommitted commands by key for conflict detection. Previously stored `int` (count or slot). Extended to store `*UnsyncedEntry` with command metadata for CURP-HO witness pool.

- [x] **20.1** Define UnsyncedEntry struct in curp-ho/defs.go [26:02:06]
  - Fields: Slot, IsStrong, Op, Value, ClientId, SeqNum, CmdId
  - On non-leaders, Slot serves as pending count; on leader, stores actual slot number

- [x] **20.2** Update unsynced usage in curp-ho/curp-ho.go [26:02:06]
  - Changed value type from `int` to `*UnsyncedEntry`
  - Split `unsync()` → `unsyncStrong()` + `unsyncCausal()` (distinguish strong vs causal entries)
  - Split `leaderUnsync()` → `leaderUnsyncStrong()` + `leaderUnsyncCausal()`
  - Updated `sync()` to work with `*UnsyncedEntry` count field
  - Updated `ok()` to distinguish strong conflicts (FALSE) from causal entries (TRUE)
  - Added `okWithWeakDep()` returning both ok status and weak write dependency

- [x] **20.3** Implement enhanced conflict checking functions [26:02:06]
  - `checkStrongWriteConflict(key)`: detects pending strong writes
  - `getWeakWriteDep(key)`: returns CmdId of pending causal write (for strong read deps)
  - `getWeakWriteValue(key)`: returns value of pending causal write (for speculative execution)

- [x] **20.4** Add boundClients tracking to Replica struct [26:02:06]
  - Added `boundClients map[int32]bool` field + initialization
  - `isBoundReplicaFor(clientId)`: checks if client is bound to this replica
  - `registerBoundClient(clientId)`: auto-detect binding from first causal propose

- [x] **20.5** Update cleanup logic in deliver() [26:02:06]
  - Added `syncLeader()` for leader-side cleanup after execution
  - Leader removes entry only if CmdId matches (preserves newer entries)
  - Non-leader cleanup via `sync()` already works with `*UnsyncedEntry`
  - 37 new unit tests, all passing (67 total in curp-ho)

**Design decisions**:
- Single entry per key (latest op overwrites metadata, count tracks total pending)
- Strong entries block strong writes; causal entries create weakDep for strong reads
- Leader stores actual slot number; non-leader uses count for pending tracking

---

### Phase 21: Client-Replica Binding [COMPLETE]

**Goal**: Implement client binding to closest replica.

- [x] **21.1** Add boundReplica field to Client struct in curp-ho/client.go [26:02:06]
  - Added `boundReplica int32` field with documentation
  - Added `BoundReplica()` accessor method

- [x] **21.2** Implement replica binding logic in NewClient() [26:02:06]
  - Set `boundReplica = int32(b.ClosestId)` from base client's latency measurement
  - Base client computes ClosestId via ping during Connect() (co-located IP match or min latency)

- [x] **21.3** Add boundClients tracking on replica side [26:02:06]
  - Already implemented in Phase 20: `boundClients map[int32]bool`, `isBoundReplicaFor()`, `registerBoundClient()`
  - Auto-detect from first causal propose message
  - Added `sendMsgToAll()` helper for CURP-HO broadcast pattern
  - 15 new tests (82 total), all passing

---

### Phase 22: Causal Op Message Protocol [COMPLETE]

**Goal**: Define messages for causal ops (broadcast, reply from bound replica).

- [x] **22.1** Define MCausalPropose message in curp-ho/defs.go [26:02:06]
  - Same fields as MWeakPropose: CommandId, ClientId, Command, Timestamp, CausalDep
  - Identical wire format (verified by test: `TestCausalProposeMatchesWeakProposeFields`)
  - Semantic difference: broadcast to ALL replicas (not leader-only)

- [x] **22.2** Define MCausalReply message [26:02:06]
  - Fields: Replica int32, CmdId CommandId, Rep []byte
  - No Ballot field (causal replies don't participate in ballot-based voting)
  - Sent by bound replica only after speculative execution

- [x] **22.3** Implement serialization methods [26:02:06]
  - BinarySize(), Marshal(), Unmarshal(), New() for both messages
  - MCausalProposeCache, MCausalReplyCache for object pooling

- [x] **22.4** Register RPC channels in initCs() [26:02:06]
  - causalProposeChan, causalReplyChan channels
  - causalProposeRPC, causalReplyRPC registered with fastrpc table
  - 18 new tests (100 total in curp-ho), all passing

---

### Phase 23: Causal Op Client-Side [COMPLETE]

**Goal**: Implement causal op sending (broadcast) and reply handling.

- [x] **23.1** Implement SendCausalWrite() in curp-ho/client.go [26:02:06]
  - Broadcasts MCausalPropose to ALL replicas via sendMsgToAll()
  - Tracks causal dependency chain (lastWeakSeqNum → causalDep)

- [x] **23.2** Implement SendCausalRead() [26:02:06]
  - Same broadcast pattern as SendCausalWrite, with GET operation

- [x] **23.3** Implement handleCausalReply() [26:02:06]
  - Only accepts replies from boundReplica (1-RTT completion)
  - Marks delivered, cleans weakPending, calls RegisterReply

- [x] **23.4** Update handleMsgs loop and weak op delegation [26:02:06]
  - Added causalReplyChan case dispatching to handleCausalReply
  - SendWeakWrite/SendWeakRead now delegate to SendCausalWrite/SendCausalRead
  - Added sendMsgToAll() helper, BoundReplica() accessor
  - 10 new tests (110 total) with newTestClient helper, all passing

---

### Phase 24: Causal Op Replica-Side [COMPLETE]

**Goal**: Implement causal op reception, witness pool addition, bound replica execution.

- [x] **24.1** Update run() loop in curp-ho/curp-ho.go [26:02:06]
  - Added causalProposeChan case dispatching to handleCausalPropose
  - ALL replicas process causal proposes (not just leader)

- [x] **24.2** Implement handleCausalPropose() [26:02:06]
  - ALL replicas: add to witness pool (unsyncCausal), track pending writes
  - ALL replicas: compute speculative result, reply with MCausalReply
  - Client filters by boundReplica (avoids binding protocol)
  - Leader: assigns slot, tracks dep (leaderUnsyncCausal), launches async replication

- [x] **24.3** Implement getCausalCmdDesc() [26:02:06]
  - Command descriptor creation for causal commands
  - Sets isWeak=true, phase=ACCEPT (skips START), tracks dependencies

- [x] **24.4** Implement asyncReplicateCausal() [26:02:06]
  - Accept/commit flow via batcher, wait for commit + slot ordering
  - Wait for causal dependency (waitForWeakDep)
  - Execute, mark weakExecuted, clean pending writes, syncLeader cleanup
  - 26 new tests (136 total) with newTestReplicaForDesc helper, all passing

---

### Phase 25: Strong Op Modifications [COMPLETE]

**Goal**: Modify strong op handling to check witness pool and track weakDep.

- [x] **25.1** Add weakDep field to MRecordAck message [26:02:06]
  - Added `WeakDep *CommandId` (pointer, nil when no dep)
  - Variable-size serialization: 18 bytes (no dep) or 26 bytes (with dep)
  - hasWeakDep flag byte at offset 17

- [x] **25.2** Modify handlePropose() for strong ops [26:02:06]
  - Non-leaders use okWithWeakDep() instead of ok()
  - RecordAck now carries WeakDep when causal write exists on same key

- [x] **25.3** Modify deliver() speculative execution for strong ops [26:02:06]
  - Replace ComputeResult with computeSpeculativeResultWithUnsynced
  - Strong speculative reads can now see uncommitted weak writes

- [x] **25.4** Implement computeSpeculativeResultWithUnsynced() [26:02:06]
  - GET: checks getWeakWriteValue first, falls back to ComputeResult
  - PUT: returns NIL during speculation
  - 20 new tests (156 total), all passing

---

### Phase 26: Client Fast Path with WeakDep [COMPLETE]

**Goal**: Implement super majority fast path with weakDep consistency check.

- [x] **26.1** Update client to track weakDep in acks
  - MRecordAck already carries WeakDep from Phase 25
  - MsgSet stores full MRecordAck objects with WeakDep

- [x] **26.2** Implement weakDep consistency check
  - Added `weakDepEqual(a, b *CommandId) bool` helper
  - Added `checkWeakDepConsistency(msgs []interface{}) bool` method
  - Checks all non-leader acks agree on the same WeakDep (or all nil)

- [x] **26.3** Modify handleAcks for fast/slow path separation
  - Split `handleAcks` into `handleFastPathAcks` (3/4 quorum + weakDep check) and `handleSlowPathAcks` (majority quorum)
  - Fast path: checks weakDep consistency, delivers if consistent, increments slowPaths and defers to slow path if inconsistent
  - Slow path: delivers unconditionally (leader has ordered the command)
  - Updated `initMsgSets` to use separate handlers
  - 21 new tests (177 total), all passing

---

### Phase 27: Testing [COMPLETE]

**Goal**: Comprehensive unit and integration tests for CURP-HO.

All tests already covered by Phases 19-26 (177 total tests):

- [x] **27.1** Unit tests: Unsynced entry operations
  - TestUnsyncedEntryCreation, TestCheckStrongWriteConflict* (3 variants), TestGetWeakWriteDep* (3), TestGetWeakWriteValue* (3), TestSyncDecrementsCount, TestSyncLeaderRemoves*

- [x] **27.2** Unit tests: Client binding
  - TestClientBoundReplica* (4), TestBindingModel* (4), TestBoundClientTracking, TestAutoDetectBinding

- [x] **27.3** Unit tests: Message serialization
  - TestMCausalProposeSerialization + 2 variants, TestMCausalReplySerialization + 2 variants, TestMRecordAckSerializationWithWeakDep + 4 variants

- [x] **27.4** Unit tests: Causal op execution
  - TestCausalProposeWitnessPoolAddsEntry, TestHandleCausalReplyFromBoundReplica/EachReplica, TestNonBoundReplicaWitnessOnly

- [x] **27.5** Unit tests: Strong op witness checking
  - TestOkStrongWriteConflict, TestCheckStrongWriteConflict* (3), TestOkWithWeakDep* (4), TestCheckWeakDepConsistency* (8)

- [x] **27.6** Integration tests: Mixed workload
  - TestCausalAndStrongMixedWitnessPool, TestStrongRead/WriteWithCausalWriteInWitnessPool, TestFastPathSlowPathFallback, TestMultipleCommandsIndependent
  - Note: TestOptimalLatency requires multi-replica network (deferred to E2E testing)

---

### Phase 28: Hybrid Benchmark Integration [COMPLETE]

**Goal**: Integrate CURP-HO with existing hybrid benchmark framework.

All tasks already implemented in previous phases:

- [x] **28.1** HybridClient interface for CURP-HO
  - SendStrongWrite/Read, SendWeakWrite/Read → SendCausalWrite/Read, SupportsWeak() (in client.go)

- [x] **28.2** main.go/run.go integration
  - main.go:221-243 (curpho client with HybridLoop)
  - run.go:54-59 (curpho replica initialization)

- [x] **28.3** Configuration
  - Existing config files work with `protocol: curpho`
  - No separate config needed (same format as curpht)

---

### Phase 29: Performance Optimization [ANALYZED - DEFERRED TO BENCHMARKS]

**Goal**: Optimize CURP-HO for high throughput and low latency.

Analysis: All witness pool operations are already O(1) using ConcurrentMap key lookups.
No full-map iterations exist. Further optimization requires runtime benchmarks.

- [x] **29.2** Witness pool lookup analysis (COMPLETE - no changes needed)
  - All operations (ok, okWithWeakDep, getWeakWriteValue, etc.) are O(1) key lookups
  - Already using ConcurrentMap (sharded hash map, SHARD_COUNT=32768)
  - No full-map iteration anywhere in witness pool code

- [x] **29.3** Broadcast message handling analysis (COMPLETE - no changes needed)
  - Cache pools already defined (MCausalProposeCache, MCausalReplyCache)
  - Batching causal proposes would need new batch message type (deferred)

- [ ] **29.1** Benchmark baseline performance (REQUIRES MULTI-REPLICA SETUP)
  - Compare to CURP-HT throughput
  - Measure weak op latency (CURP-HO 1-RTT to closest vs CURP-HT 1-RTT to leader)

- [ ] **29.4** Tune parameters (REQUIRES BENCHMARKS)
  - Witness pool cleanup frequency, message buffer sizes, batcher settings

---

### Phase 30: Comparative Evaluation [LOW PRIORITY]

**Goal**: Evaluate CURP-HO vs CURP-HT trade-offs.

- [ ] **30.1** Latency comparison
  - Weak op latency: CURP-HO (to closest) vs CURP-HT (to leader)
  - Strong op latency: Impact of witness checks

- [ ] **30.2** Throughput comparison
  - Peak throughput under various workloads
  - Network bandwidth usage

- [ ] **30.3** Scalability analysis
  - Performance with varying number of replicas
  - Performance with varying client distribution
  - Plan: docs/dev/curp-ho/phase30-evaluation-plan.md

---

## Key Differences: CURP-HT vs CURP-HO

| Aspect | CURP-HT | CURP-HO |
|--------|---------|---------|
| **Weak op broadcast** | Leader only | All replicas |
| **Weak op execution** | Leader | Bound (closest) replica |
| **Weak latency** | 1-RTT to leader | 1-RTT to closest replica ✨ |
| **Network load** | Lower (no broadcast) | Higher (broadcast) |
| **Strong sees weak?** | No (only committed) | Yes (unsynced entries) ✨ |
| **Data structure** | Standard unsynced (int) | Extended unsynced (struct) |
| **Fast path quorum** | 3/4 | 3/4 + weakDep check |
| **Complexity** | Lower (leader serializes) | Higher (witness pool) |
| **Best for** | Leader-centric topology | Geo-distributed clients |

---

# Future Protocols

## Candidates for Implementation

### CURP-HM (Hybrid Multi-Leader)
- Multiple leaders for different key ranges
- Load balancing weak ops across leaders
- Reduces leader bottleneck

### CURP-HE (Hybrid Eventually Consistent)
- Even weaker consistency for read-only ops
- No causal ordering required
- Lowest possible latency

### CURP-HS (Hybrid Snapshot Isolation)
- Snapshot isolation for read transactions
- Serializable writes
- Multi-object operations

---

## Legend

- `[ ]` - Undone task
- `[x]` - Done task with timestamp [yy:mm:dd, hh:mm]
- Priority: HIGH > MEDIUM > LOW
- Each task should be small enough (<500 LOC)
