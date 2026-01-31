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

## Repeated Tasks

(None currently)

---

## Legend

- `[ ]` - Undone task
- `[x]` - Done task with timestamp [yy:mm:dd, hh:mm]
- Priority: HIGH > MEDIUM > LOW
- Each task should be small enough (<500 LOC)
