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

#### Optimization Candidates

- [ ] **18.1** Increase MaxDescRoutines (500 → 10000)
- [ ] **18.2** Remove weak command spin-wait overhead
- [ ] **18.3** Increase client pipeline depth (pendings: 5 → 10/20)
- [ ] **18.4** Reduce channel buffer contention

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

## Status: 📋 **PLANNED**

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

### Phase 20: Extend Unsynced Structure for Witness Pool [HIGH PRIORITY]

**Goal**: Extend CURP-HT's existing `unsynced` structure to support witness pool functionality, avoiding duplicate data structures.

**Background**: CURP-HT already has `unsynced cmap.ConcurrentMap` that tracks uncommitted commands by key for conflict detection. Currently it stores `int` (count or slot). We need to extend it to also store command metadata for CURP-HO.

- [ ] **20.1** Define UnsyncedEntry struct in curp-ho/defs.go
  ```go
  // Replaces simple int value in unsynced map
  type UnsyncedEntry struct {
      Slot      int             // Slot number (for leader) or count (for non-leader)
      IsStrong  bool            // true=strong, false=causal
      Op        state.Op        // GET/PUT
      Value     state.Value     // Value for PUT operations (needed for speculative reads)
      ClientId  int32           // Client that issued this command
      SeqNum    int32           // Sequence number
      CmdId     CommandId       // Full command ID
  }
  ```

- [ ] **20.2** Update unsynced usage in curp-ho/curp-ho.go
  - Keep the same `unsynced cmap.ConcurrentMap` (no new field needed!)
  - Change value type from `int` to `*UnsyncedEntry`
  - Update all unsynced operations: `unsync()`, `leaderUnsync()`, `sync()`, `ok()`

- [ ] **20.3** Implement enhanced conflict checking functions
  ```go
  // Check if there's a strong write conflict
  func (r *Replica) checkStrongWriteConflict(key state.Key) bool {
      keyStr := strconv.FormatInt(int64(key), 10)
      if entry, exists := r.unsynced.Get(keyStr); exists {
          e := entry.(*UnsyncedEntry)
          return e.IsStrong && e.Op == state.PUT
      }
      return false
  }

  // Get weak write dependency (returns nil if no weak write)
  func (r *Replica) getWeakWriteDep(key state.Key) *CommandId {
      keyStr := strconv.FormatInt(int64(key), 10)
      if entry, exists := r.unsynced.Get(keyStr); exists {
          e := entry.(*UnsyncedEntry)
          if !e.IsStrong && e.Op == state.PUT {
              return &e.CmdId
          }
      }
      return nil
  }

  // Get weak write value for speculative execution
  func (r *Replica) getWeakWriteValue(key state.Key) (state.Value, bool) {
      keyStr := strconv.FormatInt(int64(key), 10)
      if entry, exists := r.unsynced.Get(keyStr); exists {
          e := entry.(*UnsyncedEntry)
          if !e.IsStrong && e.Op == state.PUT {
              return e.Value, true
          }
      }
      return nil, false
  }
  ```

- [ ] **20.4** Add boundClients tracking to Replica struct
  ```go
  type Replica struct {
      // ... existing fields from CURP-HT
      boundClients  map[int32]bool  // clientId -> is bound to me?
  }
  ```

- [ ] **20.5** Update cleanup logic in deliver()
  - After Execute(), remove from unsynced (already happens in CURP-HT)
  - Prevent unbounded growth (already handled)
  - Plan: docs/dev/curp-ho/phase20-witness-pool-plan.md

**Advantages of reusing unsynced**:
- ✅ No duplicate data structures
- ✅ Reuse existing conflict detection logic
- ✅ Same cleanup mechanism
- ✅ Key-based indexing already optimal for lookups

---

### Phase 21: Client-Replica Binding [HIGH PRIORITY]

**Goal**: Implement client binding to closest replica.

- [ ] **21.1** Add boundReplica field to Client struct in curp-ho/client.go
  ```go
  type Client struct {
      boundReplica int32  // ID of bound (closest) replica
      // ... existing fields from CURP-HT
  }
  ```

- [ ] **21.2** Implement replica binding logic in NewClient()
  - Measure latency to all replicas (reuse existing ping logic)
  - Select replica with lowest latency
  - Store as boundReplica

- [ ] **21.3** Add boundClients tracking on replica side
  - Configuration file specifies which clients bind to which replicas
  - Or auto-detect from first message
  - Plan: docs/dev/curp-ho/phase21-binding-plan.md

---

### Phase 22: Causal Op Message Protocol [HIGH PRIORITY]

**Goal**: Define messages for causal ops (broadcast, reply from bound replica).

- [ ] **22.1** Define MCausalPropose message in curp-ho/defs.go
  ```go
  type MCausalPropose struct {
      CommandId int32
      ClientId  int32
      Command   state.Command
      CausalDep int32  // Previous causal command seqnum
      Timestamp int64
  }
  ```

- [ ] **22.2** Define MCausalReply message
  ```go
  type MCausalReply struct {
      Replica int32
      CmdId   CommandId
      Rep     []byte  // Speculative result
  }
  ```

- [ ] **22.3** Implement serialization methods
  - BinarySize(), Marshal(), Unmarshal(), New()
  - Add cache structures for object pooling

- [ ] **22.4** Register RPC channels in initCs()
  - causalProposeChan, causalReplyChan
  - Register RPCs with fastrpc table
  - Plan: docs/dev/curp-ho/phase22-messages-plan.md

---

### Phase 23: Causal Op Client-Side [HIGH PRIORITY]

**Goal**: Implement causal op sending (broadcast) and reply handling.

- [ ] **23.1** Implement SendCausalWrite() in curp-ho/client.go
  ```go
  func (c *Client) SendCausalWrite(key int64, value []byte) int32 {
      seqnum := c.getNextSeqnum()
      causalDep := c.lastWeakSeqNum
      c.lastWeakSeqNum = seqnum

      p := &MCausalPropose{...}
      c.SendMsgToAll(p, c.cs.causalProposeRPC)  // Broadcast!
      return seqnum
  }
  ```

- [ ] **23.2** Implement SendCausalRead()
  - Similar to SendCausalWrite, but with GET op
  - Also broadcasts to all replicas

- [ ] **23.3** Implement handleCausalReply()
  ```go
  func (c *Client) handleCausalReply(rep *MCausalReply) {
      // Only process if from boundReplica
      if rep.Replica != c.boundReplica {
          return
      }
      // Mark as delivered and complete
      c.delivered[rep.CmdId.SeqNum] = struct{}{}
      c.RegisterReply(rep.Rep, rep.CmdId.SeqNum)
  }
  ```

- [ ] **23.4** Update handleMsgs loop
  - Add case for causalReplyChan
  - Plan: docs/dev/curp-ho/phase23-client-causal-plan.md

---

### Phase 24: Causal Op Replica-Side [HIGH PRIORITY]

**Goal**: Implement causal op reception, witness pool addition, bound replica execution.

- [ ] **24.1** Update run() loop in curp-ho/curp-ho.go
  ```go
  case m := <-r.cs.causalProposeChan:
      propose := m.(*MCausalPropose)
      r.handleCausalPropose(propose)
  ```

- [ ] **24.2** Implement handleCausalPropose()
  ```go
  func (r *Replica) handleCausalPropose(propose *MCausalPropose) {
      // 1. Add to unsynced (ALL replicas do this - witness for conflict detection)
      keyStr := strconv.FormatInt(int64(propose.Command.K), 10)
      entry := &UnsyncedEntry{
          Slot:     -1,  // Not assigned yet (leader will assign)
          IsStrong: false,
          Op:       propose.Command.Op,
          Value:    propose.Command.V,
          ClientId: propose.ClientId,
          SeqNum:   propose.CommandId,
          CmdId:    CommandId{ClientId: propose.ClientId, SeqNum: propose.CommandId},
      }
      r.unsynced.Set(keyStr, entry)

      // 2. If bound replica, execute and reply (but DON'T replicate)
      if r.isBoundReplicaFor(propose.ClientId) {
          r.executeCausalAndReply(propose)
      }

      // 3. If leader, coordinate replication (separate responsibility)
      if r.isLeader {
          go r.asyncReplicateCausal(propose)
      }
      // Note: If bound replica == leader, it does BOTH (2) and (3)
  }
  ```

- [ ] **24.3** Implement executeCausalAndReply() (bound replica only)
  - Check causal dependency (waitForWeakDep if needed)
  - Speculative execution with pendingWrites (reuse from CURP-HT!)
  - Send MCausalReply to client immediately
  - **NO replication** - this is purely for fast 1-RTT response

- [ ] **24.4** Implement asyncReplicateCausal() (leader only)
  - Assign slot number
  - Send Accept{slot, cmd} to all replicas
  - Wait for majority acks
  - Send Commit{slot}
  - Execute in slot order after commit
  - **Important**: This happens independently of step 24.3
  - Plan: docs/dev/curp-ho/phase24-replica-causal-plan.md

---

### Phase 25: Strong Op Modifications [HIGH PRIORITY]

**Goal**: Modify strong op handling to check witness pool and track weakDep.

- [ ] **25.1** Add weakDep field to MRecordAck message
  ```go
  type MRecordAck struct {
      Replica int32
      CmdId   CommandId
      Ok      uint8
      WeakDep *CommandId  // NEW: weak write dependency (if any)
  }
  ```
  - Update serialization methods

- [ ] **25.2** Modify handlePropose() for strong ops
  - Check unsynced for strong write conflicts using `checkStrongWriteConflict()`
  - For strong read, check weak write conflicts using `getWeakWriteDep()`
  - Return weakDep in RecordAck if applicable

- [ ] **25.3** Modify deliver() speculative execution for strong ops
  ```go
  // CURP-HO: Strong speculative CAN see unsynced (including uncommitted weak writes)!
  if desc.val == nil && desc.phase != COMMIT {
      desc.val = r.computeSpeculativeResultWithUnsynced(desc.cmd)
  }
  ```

- [ ] **25.4** Implement computeSpeculativeResultWithUnsynced()
  ```go
  func (r *Replica) computeSpeculativeResultWithUnsynced(cmd state.Command) state.Value {
      switch cmd.Op {
      case state.GET:
          // Check unsynced for weak write first
          if val, found := r.getWeakWriteValue(cmd.K); found {
              return val
          }
          // Otherwise check pendingWrites (for committed but not in state yet)
          // Then fall back to ComputeResult(r.State)
          return r.computeSpeculativeResult(...) // Reuse CURP-HT logic
      // ...
      }
  }
  ```
  - Plan: docs/dev/curp-ho/phase25-strong-witness-plan.md

---

### Phase 26: Client Fast Path with WeakDep [HIGH PRIORITY]

**Goal**: Implement super majority fast path with weakDep consistency check.

- [ ] **26.1** Update client to track weakDep in acks
  - Modify acks MsgSet to include weakDep field
  - Each RecordAck carries weakDep

- [ ] **26.2** Implement weakDep consistency check
  ```go
  func (c *Client) checkWeakDepConsistency(acks []MRecordAck) bool {
      var firstWeakDep *CommandId
      for _, ack := range acks {
          if firstWeakDep == nil {
              firstWeakDep = ack.WeakDep
          } else if !weakDepEqual(firstWeakDep, ack.WeakDep) {
              return false  // Inconsistent!
          }
      }
      return true
  }
  ```

- [ ] **26.3** Modify handleAcks for super majority
  - Change quorum from 3/4 to super majority
  - Add weakDep consistency check
  - If consistent, complete fast path
  - Plan: docs/dev/curp-ho/phase26-fast-path-plan.md

---

### Phase 27: Testing [HIGH PRIORITY]

**Goal**: Comprehensive unit and integration tests for CURP-HO.

- [ ] **27.1** Unit tests: Unsynced entry operations
  - TestUnsyncedEntryCreation
  - TestCheckStrongWriteConflict
  - TestGetWeakWriteDep
  - TestGetWeakWriteValue
  - TestUnsyncedCleanup

- [ ] **27.2** Unit tests: Client binding
  - TestClientReplicaBinding
  - TestBoundReplicaSelection
  - TestBoundClientTracking

- [ ] **27.3** Unit tests: Message serialization
  - TestMCausalProposeSerialization
  - TestMCausalReplySerialization
  - TestMRecordAckWithWeakDep

- [ ] **27.4** Unit tests: Causal op execution
  - TestCausalOpBroadcast
  - TestBoundReplicaExecutes
  - TestNonBoundReplicaWitness

- [ ] **27.5** Unit tests: Strong op witness checking
  - TestStrongConflictDetection
  - TestWeakDepTracking
  - TestWeakDepConsistency

- [ ] **27.6** Integration tests: Mixed workload
  - TestCausalStrongMixed
  - TestOptimalLatency
  - TestSuperMajorityFastPath
  - Plan: docs/dev/curp-ho/phase27-testing-plan.md

---

### Phase 28: Hybrid Benchmark Integration [MEDIUM PRIORITY]

**Goal**: Integrate CURP-HO with existing hybrid benchmark framework.

- [ ] **28.1** Implement HybridClient interface for CURP-HO
  - SendStrongWrite(), SendStrongRead()
  - SendWeakWrite() → SendCausalWrite()
  - SendWeakRead() → SendCausalRead()
  - SupportsWeak() returns true

- [ ] **28.2** Update main.go for curpho protocol
  - Add case "curpho" in runClient()
  - Initialize CURP-HO client
  - Use HybridLoop() for benchmarking

- [ ] **28.3** Add sample configuration
  - Update README.md with CURP-HO description
  - Add curpho.conf example
  - Plan: docs/dev/curp-ho/phase28-benchmark-plan.md

---

### Phase 29: Performance Optimization [MEDIUM PRIORITY]

**Goal**: Optimize CURP-HO for high throughput and low latency.

- [ ] **29.1** Benchmark baseline performance
  - Compare to CURP-HT throughput
  - Measure weak op latency improvement
  - Measure strong op latency (may be higher due to witness checks)

- [ ] **29.2** Optimize witness pool lookups
  - Consider indexing by key for faster conflict detection
  - Use sync.Map or concurrent hash map

- [ ] **29.3** Optimize broadcast message handling
  - Batch causal propose messages
  - Reuse message objects (object pooling)

- [ ] **29.4** Tune parameters
  - Witness pool cleanup frequency
  - Message buffer sizes
  - Batcher settings
  - Plan: docs/dev/curp-ho/phase29-optimization-plan.md

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
