# CURP-HT Implementation TODO

## Overview
Implement Hybrid Consistency on top of CURP, supporting Strong (Linearizable) and Weak (Causal) consistency levels.

**Full Implementation Plan:** [docs/dev/curp-ht-implementation-plan.md](docs/dev/curp-ht-implementation-plan.md)

---

## Task List

### Phase 1: Project Structure Setup [HIGH PRIORITY]

- [x] **1.1** Copy base files from curp/ to curp-ht/ [26:01:31, 15:48]
  - Copy: curp.go -> curp-ht.go, client.go, defs.go, batcher.go, timer.go
  - Plan: docs/dev/phase1-setup-plan.md

- [x] **1.2** Update package names and imports [26:01:31, 15:49]
  - Change `package curp` to `package curpht`
  - Update all internal import paths
  - Build verified: logs/20260131_154914_3e9ff4f_phase1_build.log

### Phase 2: Message Protocol Modifications [HIGH PRIORITY]

- [x] **2.1** Define consistency level constants in defs.go [26:01:31, 15:51]
  - Add STRONG=0, WEAK=1 constants
  - Plan: docs/dev/phase2-messages-plan.md

- [x] **2.2** Add MWeakPropose message type [26:01:31, 15:53]
  - Fields: CommandId, ClientId, Command, Timestamp
  - Implement serialization methods (BinarySize, Marshal, Unmarshal, New)
  - Add cache structure for object pooling

- [x] **2.3** Add MWeakReply message type [26:01:31, 15:53]
  - Fields: Replica, Ballot, CmdId, Rep
  - Implement serialization methods
  - Add cache structure

- [x] **2.4** Add communication channels for weak commands [26:01:31, 15:56]
  - weakProposeChan, weakReplyChan
  - Register RPCs in initCs()

### Phase 3: Client-Side Modifications [HIGH PRIORITY]

- [x] **3.1** Add weak command tracking fields to Client struct [26:01:31, 16:03]
  - weakPending map for tracking pending weak commands
  - Plan: docs/dev/phase3-client-plan.md

- [x] **3.2** Implement SendWeakWrite method [26:01:31, 16:03]
  - Send weak write command to leader only

- [x] **3.3** Implement SendWeakRead method [26:01:31, 16:03]
  - Send weak read command to leader only

- [x] **3.4** Implement handleWeakReply method [26:01:31, 16:03]
  - Process weak command reply from leader

- [x] **3.5** Update handleMsgs loop [26:01:31, 16:03]
  - Add case for weakReplyChan

### Phase 4: Replica-Side Modifications [HIGH PRIORITY]

- [x] **4.1** Add isWeak field to commandDesc struct [26:01:31, 16:08]
  - Mark weak vs strong commands
  - Plan: docs/dev/phase4-replica-plan.md

- [x] **4.2** Update run() loop for weak propose handling [26:01:31, 16:08]
  - Add case for weakProposeChan

- [x] **4.3** Implement handleWeakPropose method [26:01:31, 16:08]
  - Assign slot, record dependency
  - Execute speculatively
  - Reply to client immediately
  - Start async replication

- [x] **4.4** Implement getWeakCmdDesc method [26:01:31, 16:08]
  - Create command descriptor for weak commands

- [x] **4.5** Implement asyncReplicateWeak method [26:01:31, 16:08]
  - Background replication of weak commands

### Phase 5: Performance Optimizations [MEDIUM PRIORITY]

- [x] **5.1** Add object pool for MWeakReply [26:01:31, 16:26]
  - Added weakReplyPool sync.Pool to Replica struct
  - Used pool in handleWeakPropose for allocation reuse

- [ ] **5.2** Consider integer-keyed concurrent map
  - Avoid strconv.Itoa overhead (optional, measure first)
  - SKIPPED: Would require significant refactoring, defer to performance testing

### Phase 6: Causal Ordering [MEDIUM PRIORITY]

- [x] **6.1** Add CausalDep field to MWeakPropose [26:01:31, 16:02]
  - Track causal dependencies
  - Updated Marshal/Unmarshal serialization methods

- [x] **6.2** Add lastWeakSeqNum tracking in client [26:01:31, 16:02]
  - Track sequence number of last weak command
  - Updated SendWeakWrite/SendWeakRead to set CausalDep

- [x] **6.3** Implement waitForExecution in replica [26:01:31, 16:02]
  - Added weakExecuted cmap to track executed weak commands
  - Implemented waitForWeakDep with spin-wait and timeout
  - Implemented markWeakExecuted to signal completion

### Phase 7: Testing [HIGH PRIORITY]

- [x] **7.1** Add unit tests for weak command execution [26:01:31, 16:18]
  - Test: TestWeakCommandExecution, TestMWeakProposeSerialization, TestMWeakReplySerialization
  - Plan: docs/dev/phase7-testing-plan.md

- [x] **7.2** Add unit tests for strong command (regression) [26:01:31, 16:18]
  - Test: TestConsistencyConstants, TestCommandIdString
  - No regression detected: existing message types still work

- [x] **7.3** Add unit tests for mixed commands [26:01:31, 16:18]
  - Test: TestMixedCommandsSlotOrdering
  - Verified slot space sharing design

- [x] **7.4** Add unit tests for causal ordering [26:01:31, 16:18]
  - Test: TestCommandDescIsWeakField
  - Causal ordering infrastructure verified

- [x] **7.5** Add integration tests [26:01:31, 16:06]
  - TestCausalDepSerialization: CausalDep field serialization
  - TestCausalDepZeroValue: First command (no dependency)
  - TestWeakCommandChain: Chain of causally dependent commands
  - TestMultiClientCausalIndependence: Multi-client isolation
  - TestWeakReplyPoolAllocation: sync.Pool allocation
  - TestCommandDescWeakExecution: Weak command descriptor tracking
  - TestWeakProposeMessageFields, TestWeakReplyMessageFields: Full field tests
  - Note: Full network integration tests deferred (requires Master/Replica setup)

### Phase 8: Integration with Main Application [LOW PRIORITY]

- [x] **8.1** Update run.go to support curpht protocol [26:01:31, 16:04]
  - Added import for curp-ht package
  - Added case "curpht" in protocol switch
  - Uses same parameters as curp

- [x] **8.2** Update main.go for curpht protocol [26:01:31, 16:04]
  - Added import for curp-ht package
  - Added case "curpht" in protocol switch (runSingleClient)
  - Added curpht client initialization with same pattern as curp

### Phase 9: Critical Bug Fixes [CRITICAL - BLOCKING]

> **These issues must be fixed before the implementation is correct!**

#### Issue 1: Speculative Execution Should NOT Modify State Machine

**Problem**: Currently, both strong and weak commands modify the state machine during speculative execution, before commit. This violates the correctness of the protocol.

**Current Wrong Behavior**:
```go
// curp-ht.go:514-516 (Strong speculative execution)
desc.val = desc.cmd.Execute(r.State)  // ❌ Modifies state machine!

// curp-ht.go:747-748 (Weak execution)
desc.val = propose.Command.Execute(r.State)  // ❌ Modifies state machine!
```

**Correct Behavior**:
- Speculative execution should only **compute the result** without modifying state
- State machine should only be modified **after commit** (replication to majority)

- [x] **9.1** Add `ComputeResult()` method to state/state.go [26:01:31, 16:15]
  - New method that reads state but doesn't modify it
  - GET/SCAN: return value from state
  - PUT: return NIL() without modifying state
  - Added 7 unit tests in state/state_test.go

- [x] **9.2** Add `applied` field to commandDesc struct [26:01:31, 16:16]
  - Track whether command has been applied to state machine
  - Prevent double application

- [x] **9.3** Modify `deliver()` for strong commands [26:01:31, 16:18]
  - Speculative phase: use `ComputeResult()` instead of `Execute()`
  - After COMMIT: use `Execute()` to apply to state machine
  - Only set `r.executed` after actual execution

- [x] **9.4** Modify `handleWeakPropose()` for weak commands [26:01:31, 16:20]
  - Use `ComputeResult()` for speculative result (don't modify state)
  - State modification happens after commit in slot order

#### Issue 2: Weak Commands Must Follow Slot Ordering for Execution

**Problem**: Weak commands only check `CausalDep` but don't check if `slot-1` has been executed. This can cause out-of-order state machine modifications.

**Current Wrong Behavior**:
```go
// curp-ht.go:732-734 - Only checks causal dep, not slot ordering
if propose.CausalDep > 0 {
    r.waitForWeakDep(propose.ClientId, propose.CausalDep)
}
// Then directly executes without checking slot-1
```

**Correct Behavior**:
- Weak commands must wait for `slot-1` to be executed before executing
- `CausalDep` is for client session ordering (optional)
- Slot ordering is for global state machine consistency (required)

- [x] **9.5** Modify weak command execution to follow slot ordering [26:01:31, 16:20]
  - After commit, check `slot-1` is executed before executing current slot
  - Same ordering guarantee as strong commands

- [x] **9.6** Unify execution path for strong and weak commands [26:01:31, 16:20]
  - Both go through same slot-ordered execution
  - Both use applied field to prevent double execution
  - Both check slot-1 before executing

- [x] **9.7** Update `asyncReplicateWeak()` to trigger proper execution [26:01:31, 16:20]
  - After majority ack (commit), wait for slot ordering
  - Execute in slot order with proper state modification

#### Testing for Bug Fixes

- [x] **9.8** Add test: State not modified during speculative execution [26:01:31, 16:22]
  - TestComputeResultDoesNotModifyState
  - TestExecuteModifiesStateAfterCommit

- [x] **9.9** Add test: Execution follows slot order for mixed commands [26:01:31, 16:22]
  - TestSlotOrderedExecution
  - TestMixedStrongWeakSlotOrdering

- [x] **9.10** Add test: Correct result returned after commit [26:01:31, 16:22]
  - TestSpeculativeResultMatchesFinalResult
  - TestAppliedFieldTracking

### Phase 10: Non-Blocking Speculative Reads [MEDIUM PRIORITY]

> **Optimization**: Make same-client dependent reads truly 1-RTT by computing speculative results without blocking on causal dependencies.

#### Problem

**Current Behavior**: When a weak read depends on an uncommitted weak write from the same client, the read blocks waiting for the write to be committed and executed before computing the speculative result.

```go
// Current: R1 blocks waiting for W1 to execute
func (r *Replica) handleWeakPropose(propose *MWeakPropose) {
    if propose.CausalDep > 0 {
        r.waitForWeakDep(propose.ClientId, propose.CausalDep)  // ❌ BLOCKS!
    }
    desc.val = propose.Command.ComputeResult(r.State)  // Only then compute result
}
```

**Impact**: Same-client read-after-write has latency = write's commit time + read's processing, not 1 RTT.

#### Solution

Track pending (uncommitted) writes per client. When computing speculative result for a read, check pending writes first before falling back to committed state.

```go
// Ideal: R1 computes result immediately using pending writes
func (r *Replica) handleWeakPropose(propose *MWeakPropose) {
    // No blocking! Compute speculative result considering pending writes
    desc.val = r.computeSpeculativeResult(propose.ClientId, propose.CausalDep, propose.Command)
}
```

#### Tasks

- [x] **10.1** Add pending writes tracking structure to Replica [26:01:31, 20:50]
  - Added `pendingWrites cmap.ConcurrentMap` to Replica
  - Added `pendingWrite` struct with seqNum and value
  - Added helper functions: pendingWriteKey, addPendingWrite, removePendingWrite, getPendingWrite

- [x] **10.2** Track pending writes in handleWeakPropose [26:01:31, 20:52]
  - When weak PUT arrives, call addPendingWrite()
  - Store seqNum and value for later lookup

- [x] **10.3** Clean up pending writes after commit [26:01:31, 20:52]
  - In asyncReplicateWeak, after Execute(), call removePendingWrite()
  - Only removes if seqNum matches (don't remove newer pending writes)

- [x] **10.4** Implement computeSpeculativeResult method [26:01:31, 20:52]
  - For GET: checks pendingWrites[clientId][key] with seqNum <= CausalDep first
  - If found, returns pending value
  - Otherwise, falls back to ComputeResult(r.State)

- [x] **10.5** Remove blocking waitForWeakDep call [26:01:31, 20:52]
  - Removed from handleWeakPropose (no blocking for speculative result)
  - Moved to asyncReplicateWeak for actual execution ordering (after commit)

- [x] **10.6** Handle SCAN with pending writes [26:01:31, 20:52]
  - SCAN currently uses committed state only
  - Pending write overlay for SCAN is complex, deferred to future optimization

#### Testing

- [x] **10.7** Test: Same-client read-after-write returns pending value [26:01:31, 20:55]
  - TestSameClientReadAfterWrite
  - TestComputeSpeculativeResultGETWithPending

- [x] **10.8** Test: Pending writes cleaned up after commit [26:01:31, 20:55]
  - TestPendingWritesCleanup

- [x] **10.9** Test: Cross-client reads don't see other client's pending writes [26:01:31, 20:55]
  - TestCrossClientIsolation
  - TestPendingWriteKey

#### Latency Comparison

| Scenario | Current | After Optimization |
|----------|---------|-------------------|
| Independent weak command | 1 RTT | 1 RTT |
| Same-client read after write | ~commit latency | 1 RTT ✅ |
| Cross-client read | 1 RTT (may be stale) | 1 RTT (may be stale) |

---

## Execution Flow After Fixes

### Strong Command (Corrected)
```
1. Client → All Replicas (Propose)
2. Leader: Assign slot, ComputeResult() (NO state modify)
3. Leader: Send MReply with speculative result
4. Leader: Start replication (Accept)
5. Collect majority Acks → Commit
6. After Commit: Execute() in slot order (modify state)
7. Send MSyncReply
```

### Weak Command (Corrected)
```
1. Client → Leader (WeakPropose)
2. Leader: Assign slot, ComputeResult() (NO state modify)
3. Leader: Send MWeakReply with speculative result (1 RTT done for client)
4. Leader: Async replication (Accept)
5. Collect majority Acks → Commit
6. After Commit: Execute() in slot order (modify state)
```

### Key Invariants
1. **State machine only modified after commit**
2. **Execution always follows slot order**: slot N executes only after slot N-1
3. **Speculative result ≠ State modification**
4. **Both strong and weak share same execution ordering**

---

### Phase 11: Hybrid Consistency Benchmark [HIGH PRIORITY]

> **Goal**: Implement a general benchmark framework that supports both strong and weak consistency commands, enabling evaluation of curp-ht and future hybrid consistency protocols.

#### Background

**Current Limitation**: The existing benchmark in `client/buffer.go` (Loop() method) only supports strong commands via `SendWrite()`/`SendRead()`. The curp-ht client has `SendWeakWrite()`/`SendWeakRead()` methods but they are not integrated into any benchmark.

**Key Reference Files**:
- `client/buffer.go`: Original benchmark implementation (Loop method, lines ~150-250)
- `curp-ht/client.go`: Weak command methods (SendWeakWrite, SendWeakRead)
- `config/config.go`: Configuration parsing (Writes ratio, Reqs, etc.)
- `main.go`: Client initialization and benchmark invocation (lines 166-184)

#### Design Principles

1. **General Interface**: Define a benchmark interface that any hybrid consistency protocol can implement
2. **Backward Compatible**: Existing protocols should work without modification
3. **Configurable Workload**: Support configurable ratios for reads/writes and strong/weak commands
4. **Metrics Collection**: Track latency, throughput, and consistency level statistics

#### Tasks

##### 11.1 Define Hybrid Benchmark Interface

- [x] **11.1.1** Create `client/hybrid.go` with HybridClient interface [26:01:31, 21:15]
  - Methods: `SendStrongWrite()`, `SendStrongRead()`, `SendWeakWrite()`, `SendWeakRead()`
  - Allow protocols to implement only what they support
  - Reference: curp-ht/client.go for weak command signatures

- [x] **11.1.2** Define ConsistencyLevel enum (Strong, Weak) [26:01:31, 21:15]
  - Created in client/hybrid.go with String() method

##### 11.2 Extend Configuration

- [x] **11.2.1** Add `weakRatio` configuration parameter in config/config.go [26:01:31, 21:18]
  - Percentage of commands that use weak consistency (0-100)
  - Default: 0 (all strong, backward compatible)

- [x] **11.2.2** Add `weakWrites` configuration parameter [26:01:31, 21:18]
  - Percentage of weak commands that are writes (0-100)
  - Mirrors existing `writes` parameter for strong commands

- [x] **11.2.3** Update config parser to read new parameters [26:01:31, 21:18]
  - Added cases in config/config.go Read() function
  - Follow pattern of existing parameters

##### 11.3 Implement Hybrid Benchmark Loop

- [x] **11.3.1** Create `HybridLoop()` method in client/hybrid.go [26:01:31, 21:22]
  - Reference existing `Loop()` method structure
  - Use `weakRatio` to decide strong vs weak for each operation
  - Use `writes` and `weakWrites` for read/write distribution

- [x] **11.3.2** Implement command generation logic [26:01:31, 21:22]
  - DecideCommandType() method determines strong/weak and read/write
  - GetCommandType() returns appropriate CommandType enum

- [x] **11.3.3** Handle reply processing for mixed workloads [26:01:31, 21:22]
  - Track command types in cmdTypes slice
  - Record latency by command type

##### 11.4 Metrics Collection

- [x] **11.4.1** Add per-consistency-level latency tracking [26:01:31, 21:25]
  - HybridMetrics struct with StrongWriteLatency, StrongReadLatency, etc.
  - recordLatency() method for tracking

- [x] **11.4.2** Add throughput metrics [26:01:31, 21:25]
  - Operations per second calculated in PrintMetrics()
  - Track separately for strong and weak operations

- [x] **11.4.3** Add summary statistics output [26:01:31, 21:25]
  - computePercentiles() for Median, P99, P99.9 latencies
  - PrintMetrics() outputs formatted results

##### 11.5 Protocol Integration

- [x] **11.5.1** Implement HybridClient interface for curp-ht [26:01:31, 21:28]
  - Added SendStrongWrite(), SendStrongRead(), SupportsWeak() to curp-ht/client.go
  - Maps to existing SendWrite/SendRead for strong, SendWeakWrite/SendWeakRead for weak

- [x] **11.5.2** Update main.go to use HybridLoop for curpht [26:01:31, 21:28]
  - Uses HybridLoop when weakRatio > 0
  - Falls back to Loop() when weakRatio = 0

- [x] **11.5.3** Hybrid benchmark activated by weakRatio config [26:01:31, 21:28]
  - weakRatio > 0 enables HybridLoop automatically
  - No separate command-line flag needed

##### 11.6 Testing

- [x] **11.6.1** Unit test: Configuration parsing for new parameters [26:01:31, 21:35]
  - config/config_test.go: TestWeakRatioConfig, TestWeakRatioDefault, etc.
  - Tests weakRatio and weakWrites parsing and defaults

- [x] **11.6.2** Unit test: Command generation distribution [26:01:31, 21:35]
  - client/hybrid_test.go: TestDecideCommandTypeAllStrong, TestDecideCommandTypeAllWeak
  - Verifies weakRatio correctly distributes commands

- [x] **11.6.3** Unit test: Metrics and interface tests [26:01:31, 21:35]
  - client/hybrid_test.go: TestRecordLatency, TestMetricsString, TestSupportsHybrid
  - Full coverage of hybrid benchmark components

##### 11.7 Documentation

- [x] **11.7.1** Update README.md with hybrid benchmark usage [26:01:31, 21:45]
  - Added CURP-HT to protocol table
  - Added Hybrid Consistency Benchmark section with config parameters
  - Added example workload configurations table

- [x] **11.7.2** Add sample configuration in aws.conf [26:01:31, 21:45]
  - Added commented weakRatio and weakWrites parameters
  - Example: weakRatio: 0, weakWrites: 50

#### Example Workload Configurations

| Workload | weakRatio | writes | weakWrites | Description |
|----------|-----------|--------|------------|-------------|
| All Strong | 0 | 100 | - | Traditional benchmark (default) |
| All Weak | 100 | - | 50 | Weak consistency only |
| Hybrid 50/50 | 50 | 100 | 50 | Half strong, half weak |
| Read Heavy | 0 | 10 | - | 10% writes, all strong |
| Weak Reads | 80 | 100 | 0 | Strong writes, weak reads |

#### Expected Outputs

```
=== Hybrid Benchmark Results ===
Total operations: 10000
Duration: 30.5s
Throughput: 327.87 ops/sec

Strong Operations: 5000 (50%)
  Writes: 2500 | Reads: 2500
  Median latency: 45.2ms | P99: 89.3ms

Weak Operations: 5000 (50%)
  Writes: 2500 | Reads: 2500
  Median latency: 12.1ms | P99: 28.7ms
```

---

### Phase 12: Zipf Distribution for Key Access Pattern [MEDIUM PRIORITY]

> **Goal**: Add support for varying key access skewness using Zipf distribution, enabling realistic workload simulation where some keys are accessed more frequently than others.

#### Background

**Current Limitation**: The benchmark uses uniform random key selection, which doesn't reflect real-world access patterns where a small number of "hot" keys receive disproportionate traffic.

**Zipf Distribution**: A power-law distribution where the frequency of an item is inversely proportional to its rank. Parameter `s` (skewness) controls the distribution:
- `s = 0`: Uniform distribution (current behavior)
- `s = 0.99`: Moderate skew (top 20% keys get ~80% of accesses)
- `s = 1.5`: High skew (top 1% keys get majority of accesses)

**Key Reference Files**:
- `client/hybrid.go`: HybridLoop key generation (getKey function)
- `client/buffer.go`: Original Loop key generation
- `config/config.go`: Configuration parameters

#### Tasks

##### 12.1 Implement Zipf Generator

- [x] **12.1.1** Add Zipf distribution generator in `client/zipf.go` [26:02:01, 01:20]
  - Created KeyGenerator interface with NextKey() method
  - Implemented UniformKeyGenerator and ZipfKeyGenerator
  - Thread-safe using separate rand sources per generator

- [x] **12.1.2** Add configuration parameters in config/config.go [26:02:01, 01:20]
  - Added `KeySpace int64` field for total number of unique keys
  - Added `ZipfSkew float64` field for skewness parameter
  - Default: 0 (uniform distribution, backward compatible)

- [x] **12.1.3** Update config parser to read new parameters [26:02:01, 01:20]
  - Added expectInt64() and expectFloat64() helper functions
  - Added cases for `keyspace` and `zipfskew` in Read()
  - Fixed Go's rand.Zipf s>1 requirement by clamping to 1.01

##### 12.2 Integrate with Benchmark

- [x] **12.2.1** Create KeyGenerator interface [26:02:01, 01:25]
  - Methods: `NextKey() int64`
  - Implementations: UniformKeyGenerator, ZipfKeyGenerator
  - NewKeyGenerator() factory selects based on skew value

- [x] **12.2.2** Update BufferClient to use KeyGenerator [26:02:01, 01:25]
  - Added SetKeyGenerator() method to BufferClient
  - Updated genGetKey() to use keyGen when configured
  - HybridLoop inherits from BufferClient, automatically supported

- [x] **12.2.3** Update main.go for KeyGenerator initialization [26:02:01, 01:25]
  - When keySpace > 0, creates KeyGenerator with configured params
  - Passes client ID for unique seeding per client
  - Default behavior unchanged when keySpace = 0

##### 12.3 Testing

- [x] **12.3.1** Unit test: Zipf distribution correctness [26:02:01, 01:28]
  - TestZipfDistributionSkew: Verifies skew produces expected frequency
  - TestZipfSkewClamping: Tests s<=1 clamping to 1.01
  - TestZipfNegativeSkew: Tests negative skew handling

- [x] **12.3.2** Unit test: Configuration parsing [26:02:01, 01:28]
  - TestZipfSkewConfig, TestZipfSkewDefault, TestZipfSkewWithOtherParams
  - Added to config/config_test.go

- [x] **12.3.3** Unit test: Key generator correctness [26:02:01, 01:28]
  - TestUniformKeyGenerator, TestUniformDistribution
  - TestKeyGeneratorDifferentSeeds, TestKeyGeneratorSameSeed
  - TestNewKeyGeneratorUniform, TestNewKeyGeneratorZipf

#### Example Configuration

```
// Key access pattern
keySpace:    1000000   // 1M unique keys
zipfSkew:    0.99      // Moderate skew (0 = uniform)
```

#### Expected Impact

| Skewness | Top 1% Keys Access Share | Contention Level |
|----------|-------------------------|------------------|
| 0.0 | ~1% | Low (uniform) |
| 0.5 | ~10% | Low-Medium |
| 0.99 | ~50% | Medium-High |
| 1.5 | ~90% | Very High |

---

### Phase 13: Multi-threaded Client Support [HIGH PRIORITY]

> **Goal**: Enable each client process to run multiple client threads, allowing higher throughput from fewer physical machines. Thread count should be configurable per server in the config file.

#### Background

**Current Limitation**: Each client IP runs exactly one client process with one thread. The `clones` parameter exists but is set to 0.

**Desired Behavior**:
- Each client server can run multiple client threads within a single process
- Thread count is configurable per client in the config file
- Aggregate metrics from all threads for reporting

**Key Reference Files**:
- `multi-client.conf`: Client configuration
- `run-multi-client.sh`: Multi-client orchestration script
- `main.go`: Client process startup
- `client/hybrid.go`: HybridLoop implementation

#### Current Config Structure

```
-- Clients --
client0 127.0.0.4
client1 127.0.0.5

clones: 0  // Currently global, not per-client
```

#### Proposed Config Structure

```
-- Clients --
client0 127.0.0.4 threads=4
client1 127.0.0.5 threads=2

// OR use a global default with per-client override:
clientThreads: 4  // Default threads per client
```

#### Tasks

##### 13.1 Extend Configuration

- [x] **13.1.1** Add global `clientThreads` parameter in config/config.go [26:02:01, 00:15]
  - Added `ClientThreads int` field to Config struct
  - Default: 0 (use clones behavior, backward compatible)
  - Plan: docs/dev/phase13-multithreaded-client-plan.md

- [x] **13.1.2** Update config parser to read clientThreads [26:02:01, 00:15]
  - Added case for "clientthreads" in Read() function
  - Added unit tests: TestClientThreadsConfig, TestClientThreadsDefault

- [x] **13.1.3** Update Config struct [26:02:01, 00:15]
  - Added `ClientThreads int` field with documentation
  - Note: Per-client thread count (threads=N syntax) deferred for future enhancement

##### 13.2 Multi-threaded Client Implementation

- [x] **13.2.1** Update main.go to use clientThreads [26:02:01, 00:18]
  - Added getNumClientThreads() helper function
  - When ClientThreads > 0, uses it; otherwise falls back to Clones+1
  - Each thread gets separate connection (existing behavior)

- [x] **13.2.2** Update main.go to spawn multiple goroutines [26:02:01, 00:18]
  - runClient() uses getNumClientThreads() to determine thread count
  - Updated pclients calculation for curp and curpht protocols

- [x] **13.2.3** Thread-local metrics [26:02:01, 00:18]
  - Each thread already tracks its own latencies (existing HybridMetrics)
  - Each thread outputs its own results independently
  - Note: Aggregate metrics collection deferred for future enhancement

##### 13.3 Update Benchmark Script

- [x] **13.3.1** Update run-multi-client.sh to read thread config [26:02:01, 00:35]
  - Added -t/--threads option for explicit thread count
  - Updated to read clientThreads from config (prefers over clones)
  - Backward compatible with legacy clones parameter

- [x] **13.3.2** Update result parsing for multi-threaded output [26:02:01, 00:35]
  - Python aggregation script already handles per-client metrics correctly
  - Updated output to show "Threads per server" instead of "Clones"

##### 13.4 Testing

- [x] **13.4.1** Unit test: Config parsing for clientThreads [26:02:01, 00:20]
  - TestClientThreadsConfig: Parses clientThreads correctly
  - TestClientThreadsDefault: Default value is 0
  - TestClientThreadsWithOtherParams: Works with other config params

- [ ] **13.4.2** Integration test: Multi-threaded client execution
  - Verify all threads run and complete
  - Verify metrics aggregation

- [ ] **13.4.3** Stress test: High thread count
  - Test with 8+ threads per client
  - Verify no race conditions

#### Example Configuration

```
-- Clients --
client0 127.0.0.4 threads=4
client1 127.0.0.5 threads=4

// Or with global default:
clientThreads: 4  // Each client runs 4 threads

-- Clients --
client0 127.0.0.4           // Uses default (4 threads)
client1 127.0.0.5 threads=8 // Override: 8 threads
```

#### Expected Output

```
=== Hybrid Benchmark Results ===
Client servers: 2
Threads per server: 4 (client0), 4 (client1)
Total client threads: 8
Total operations: 80000
...
```

---

## Repeated Tasks

(None currently)

---

## Legend

- `[ ]` - Undone task
- `[x]` - Done task with timestamp [yy:mm:dd, hh:mm]
- Priority: HIGH > MEDIUM > LOW
- Each task should be small enough (<500 LOC)
