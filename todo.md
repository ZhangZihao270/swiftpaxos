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

---

## Repeated Tasks

(None currently)

---

## Legend

- `[ ]` - Undone task
- `[x]` - Done task with timestamp [yy:mm:dd, hh:mm]
- Priority: HIGH > MEDIUM > LOW
- Each task should be small enough (<500 LOC)
