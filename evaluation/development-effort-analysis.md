# Development Effort Analysis: HT vs HO Hybrid Protocols

This report analyzes the code changes required to extend base protocols with hybrid consistency support. We compare two design approaches:

- **HT (Hybrid Transparency)**: Satisfies the T property — weak operations do not affect strong operation performance. Strong consensus path remains unchanged.
- **HO (Hybrid Optimal)**: Does NOT satisfy the T property — weak operations share the consensus path with strong operations, requiring modifications to the strong path.

This analysis covers only the **core consensus protocol logic**: replica protocol handlers (e.g., `raft.go`, `epaxos-ho.go`), execution engine (`exec.go`), and client protocol logic (`client.go`). We exclude:

- Message definitions and serialization (`defs.go` — mechanical Marshal/Unmarshal boilerplate)
- Network layer (`rpc/`, `replica/replica.go` — TCP connection management, message routing)
- State machine (`state/` — KV store)
- Configuration and startup (`config/`, `run.go`, `main.go`)
- Test files (`*_test.go`)
- Scripts and tooling (`scripts/`, `evaluation/`)

## Protocol Code Overview

| | Raft | Raft-HT | CURP | CURP-HT | CURP-HO | EPaxos | EPaxos-HO |
|---|---|---|---|---|---|---|---|
| **Type** | Base | HT | Base | HT | HO | Base | HO |
| Replica LoC | 897 | 1,055 | 781 | 1,162 | 1,943 | 1,439 | 2,323 |
| Client LoC | 121 | 411 | 292 | 547 | 831 | 111 | 154 |
| Exec LoC | — | — | — | — | — | 160 | 389 |
| **Total LoC** | **1,018** | **1,466** | **1,073** | **1,709** | **2,774** | **1,710** | **2,866** |
| Replica functions | 22 | 24 | 21 | 32 | 50 | 43 | 47 |

## Raft → Raft-HT (HT)

**Code increase**: +448 LoC (44%)

### Strong Consensus Path: Unchanged

All 15 consensus-critical functions are **identical or only additively changed** (no strong path logic modified):

| Function | Lines | Status |
|----------|-------|--------|
| # | Function | Role | Lines | Status |
|---|----------|------|-------|--------|
| 1 | `handlePropose` | Entry point for client proposals | 23 | Renamed to `handleAllProposals`; strong logic identical |
| 2 | `broadcastAppendEntries` | Send log entries to all followers | 47 | Identical |
| 3 | `sendAppendEntries` | Send log entries to one follower | 41 | Identical |
| 4 | `handleAppendEntries` | Follower receives log entries | 95 | Identical |
| 5 | `handleAppendEntriesReply` | Leader processes follower ACK | 14 | Identical |
| 6 | `handleAppendEntriesReplyBatch` | Leader processes batched ACKs | 46 | Identical |
| 7 | `advanceCommitIndex` | Advance commit after majority ACK | 66 | Identical |
| 8 | `executeCommands` | Apply committed entries to state machine | 40 | Additive: +4 lines (lock for concurrent weak read safety) |
| 9 | `applyReplyUpdate` | Reply to client after execution | 27 | Identical |
| 10 | `handleRequestVote` | Handle election vote request | 38 | Identical |
| 11 | `handleRequestVoteReply` | Handle election vote reply | 36 | Identical |
| 12 | `startElection` | Initiate leader election | 20 | Identical |
| 13 | `becomeLeader` | Transition to leader state | 27 | Identical |
| 14 | `becomeFollower` | Transition to follower state | 17 | Identical |
| 15 | `run` | Event loop | 86 | Additive: +13 lines (weak channel cases) |

The only changes to existing functions:

- **`run`** (+13 lines): Adds `case` branches for `weakProposeChan` and `weakReadChan` in the event loop `select`. These are purely additive — no existing `case` branches are modified. The strong propose path (`handleAllProposals`) replaces `handlePropose` but contains identical strong logic.
- **`executeCommands`** (+4 lines): Adds `r.stateMu.Lock()/Unlock()` around state machine execution and tracks `keyVersions` for weak read freshness. This is needed because weak reads now access the state machine concurrently from a separate goroutine. **This is the only change that touches the strong execution path** — adding a lock that was previously unnecessary because only one goroutine accessed the state machine.

### Weak Path: Purely Additive

| New Function | Lines | Purpose |
|---|---|---|
| `handleAllProposals` | 116 | Batches strong+weak proposals, routes weak writes for immediate reply |
| `processWeakRead` | 34 | Handles weak read from client cache |
| `weakReadLoop` | 10 | Goroutine for processing weak reads |

**Summary**: Raft-HT adds **0 modifications** to the consensus critical path (AppendEntries, RequestVote, commit logic). The only strong path change is a lock addition in `executeCommands` for concurrent weak read safety.

---

## CURP → CURP-HT (HT)

**Code increase**: +636 LoC (59%)

### Strong Consensus Path: Minimal Changes

CURP's consensus-critical functions and their status:

| # | Function | Role | Lines | Status |
|---|----------|------|-------|--------|
| 1 | `handlePropose` | Entry point for strong client proposals | 20 | Identical |
| 2 | `handleAccept` | Follower receives Accept (replication) | 59 | Additive: +9 lines (copy weak cmd data from Accept msg) |
| 3 | `handleAcceptAck` | Leader processes Accept ACK | 17 | Identical |
| 4 | `handleCommit` | Handle commit notification | 14 | Identical |
| 5 | `handleDesc` | Process command descriptor | 12 | Identical |
| 6 | `deliver` | Execute committed commands | 110 | Additive: +17 lines (if-weak guards for non-leader weak cmds) |
| 7 | `leaderUnsync` | Leader marks command as unsynced (witness) | 20 | Identical |
| 8 | `unsync` | Follower marks command as unsynced | 8 | Identical |
| 9 | `sync` | Mark command as synced after commit | 22 | Identical |
| 10 | `ok` | Check witness conflict (fast path gate) | 8 | Identical |
| 11 | `run` | Event loop | 98 | Additive: +16 lines (weak channel cases) |

**`deliver`** (+17 lines): Adds handling for weak commands on non-leader replicas. In vanilla CURP, `deliver` assumes all commands come from `handlePropose` (with `desc.propose` set). Weak commands on non-leaders arrive via Accept messages without `propose`, so `deliver` needs to recognize them and apply them to the state machine. The strong command path through `deliver` is completely unchanged — the additions are `if` guards that only activate for weak commands. This is additive in nature.

**`handleAccept`** (+9 lines): Adds `if desc.cmd.Op == 0 && msg.Cmd.Op != 0` to copy weak command data from Accept message on non-leaders. Purely additive — does not change any existing strong path logic.

**`run`** (+16 lines): Adds `case` branches for `weakProposeChan` and `weakReadChan`. Purely additive.

### Weak Path: Purely Additive

| New Function | Lines | Purpose |
|---|---|---|
| `handleWeakPropose` | 14 | Routes weak write to async replication |
| `asyncReplicateWeak` | 83 | Weak write: Accept broadcast → wait commit → reply |
| `getWeakCmdDesc` | 35 | Creates descriptor for weak commands |
| `handleWeakRead` | 17 | Handles weak read (local state + cache) |
| `markWeakExecuted` | 17 | Tracks weak execution for causal ordering |
| `waitForWeakDep` | 24 | Waits for causal dependency |
| `notifyWeakDep` | 8 | Notifies causal dependency completion |
| `getWeakDepNotify` | 10 | Creates notification channel |
| `getOrCreateCommitNotify` | 18 | Commit notification for async weak |
| `notifyCommit` | 9 | Triggers commit notification |
| `BeTheLeader` | 5 | Leader designation |

**Summary**: CURP-HT's changes to existing functions are all additive — adding `if weak` guards that only activate for weak commands, without modifying any strong command code path. All core CURP logic (unsync/sync witness mechanism, Accept quorum, commit advancement) is identical to vanilla CURP.

---

## EPaxos → EPaxos-HO (HO)

**Code increase**: +1,156 LoC (68%)

### Strong Consensus Path: Extensively Modified

**18 out of 28 consensus-critical functions are modified** (including 6 that were removed and refactored into strong/causal variants):

| # | Function | Role | Lines | Status |
|---|----------|------|-------|--------|
| 1 | `handlePropose` | Entry point for proposals | 23 | **Modified** (+22): batch split into causal vs strong |
| 2 | `startPhase1` | Initiate PreAccept consensus | 78 | **Removed**: replaced by `startStrongCommit` |
| 3 | `bcastPreAccept` | Broadcast PreAccept to peers | 33 | Additive (+3) |
| 4 | `handlePreAccept` | Follower processes PreAccept | 65 | **Modified** (+49): WAITING mechanism, uncommittedDeps check |
| 5 | `handlePreAcceptReply` | Leader processes PreAccept reply | 126 | **Modified** (-33): slow path guard rewritten |
| 6 | `replyPreAccept` | Send PreAccept reply | 16 | Identical |
| 7 | `bcastAccept` | Broadcast Accept (slow path) | 33 | Additive (+3) |
| 8 | `handleAccept` | Follower processes Accept | 32 | **Modified** (+37): causal commit state, CL-aware update |
| 9 | `handleAcceptReply` | Leader processes Accept reply | 57 | **Modified** (-6): commit path split for strong vs causal |
| 10 | `replyAccept` | Send Accept reply | 18 | Identical |
| 11 | `bcastCommit` | Broadcast Commit | 44 | **Removed**: replaced by `bcastStrongCommit` |
| 12 | `handleCommit` | Process Commit message | 46 | **Modified** (+6): STRONGLY vs CAUSALLY_COMMITTED |
| 13 | `bcastPrepare` | Broadcast Prepare (recovery) | 23 | Additive (-1) |
| 14 | `handlePrepare` | Handle Prepare (recovery) | 32 | **Modified** (+45): handle causal instances in recovery |
| 15 | `handlePrepareReply` | Handle Prepare reply (recovery) | 96 | **Modified** (+106): recovery logic rewritten for causal deps |
| 16 | `replyPrepare` | Send Prepare reply | 23 | Identical |
| 17 | `bcastTryPreAccept` | Broadcast TryPreAccept (recovery) | 26 | Additive (+2) |
| 18 | `handleTryPreAccept` | Handle TryPreAccept | 36 | **Modified** (+35): conflict detection extended for causal |
| 19 | `handleTryPreAcceptReply` | Handle TryPreAccept reply | 77 | **Modified** (-22): refactored for dual instance types |
| 20 | `replyTryPreAccept` | Send TryPreAccept reply | 18 | Identical |
| 21 | `startRecoveryForInstance` | Initiate recovery for stuck instance | 42 | **Modified** (+12): handle causal instance recovery |
| 22 | `findPreAcceptConflicts` | Check key conflicts for PreAccept | 38 | Additive (-3) |
| 23 | `updateAttributes` | Compute instance deps and seq | 32 | **Removed**: split into `updateStrongAttributes1/2` + `updateCausalAttributes` |
| 24 | `mergeAttributes` | Merge deps from PreAccept replies | 20 | **Removed**: replaced by `mergeStrongAttributes` |
| 25 | `updateConflicts` | Update conflict tracking map | 8 | **Removed**: split into `updateStrongConflicts` + `updateCausalConflicts` |
| 26 | `executeCommands` | Execute committed instances (SCC) | 78 | **Removed**: rewritten in exec.go with causal dep skipping |
| 27 | `sync` | Flush to stable storage | 5 | Identical |
| 28 | `run` | Event loop | 106 | **Modified** (+31): new channels for causal/preAcceptOK |

**13 functions removed/refactored**: `startPhase1`, `bcastCommit`, `updateAttributes`, `mergeAttributes`, `updateConflicts`, `newInstance`, `newInstanceDefault`, `newLeaderBookkeeping`, `newLeaderBookkeepingDefault`, `newNilDeps`, `executeCommands`, `makeBallot`, `BeTheLeader`

These were split into strong/causal variants (e.g., `updateAttributes` → `updateStrongAttributes1` + `updateStrongAttributes2` + `updateCausalAttributes`).

### Weak Path + Refactored Strong Functions

| New Function | Lines | Category |
|---|---|---|
| `startCausalCommit` | 100 | Weak: 1-RTT causal commit |
| `handleCausalCommit` | 55 | Weak: follower causal commit handler |
| `bcastCausalCommit` | 25 | Weak: broadcast causal commit |
| `updateCausalAttributes` | 49 | Weak: causal dep computation |
| `updateCausalConflicts` | 28 | Weak: causal conflict tracking |
| `updateStrongSessionConflict` | 10 | Weak: session conflict for causal |
| `handlePreAcceptOK` | 63 | Refactored: split from handlePreAcceptReply |
| `handleCommitShort` | 45 | Refactored: compact commit message |
| `startStrongCommit` | 81 | Refactored: replaces startPhase1 |
| `bcastStrongCommit` | 43 | Refactored: replaces bcastCommit |
| `updateStrongAttributes1` | 57 | Refactored: replaces updateAttributes |
| `updateStrongAttributes2` | 37 | Refactored: follower attribute computation |
| `mergeStrongAttributes` | 22 | Refactored: replaces mergeAttributes |
| `updateStrongConflicts` | 17 | Refactored: replaces updateConflicts |
| `retryStuckInstances` | 21 | Workaround: retry stuck PreAccept |
| `makeBallotLargerThan` | 3 | Utility |
| `makeUniqueBallot` | 3 | Utility |

**Exec.go changes** (+229 lines): SCC algorithm extended with causal dep skipping, WAITING state checks, diagnostic counters. The execution engine is fundamentally changed because causal and strong instances share the dep graph.

**Summary**: EPaxos-HO modifies **18 of 28 consensus-critical functions** (64%), including 6 that are removed and split into strong/causal variants. The core consensus logic (PreAccept, Accept, Prepare, TryPreAccept, Recovery) is all modified to accommodate causal instances in the dep graph.

---

## CURP → CURP-HO (HO)

**Code increase**: +1,701 LoC (159%)

### Strong Consensus Path: Significantly Modified

| # | Function | Role | Lines | Status |
|---|----------|------|-------|--------|
| 1 | `handlePropose` | Entry point for strong proposals | 20 | Identical |
| 2 | `handleAccept` | Follower receives Accept | 59 | Additive: +9 lines (copy weak cmd data) |
| 3 | `handleAcceptAck` | Leader processes Accept ACK | 17 | Identical |
| 4 | `handleCommit` | Handle commit notification | 14 | Identical |
| 5 | `handleDesc` | Process command descriptor | 12 | Identical |
| 6 | `deliver` | Execute committed commands | 110 | **Modified** (+22): speculative result changed to `computeSpeculativeResultWithUnsynced` |
| 7 | `leaderUnsync` | Leader marks command as unsynced | 20 | **Removed**: split into `leaderUnsyncStrong` + `leaderUnsyncCausal` |
| 8 | `unsync` | Follower marks command as unsynced | 8 | **Removed**: split into `unsyncStrong` + `unsyncCausal` |
| 9 | `sync` | Mark command as synced after commit | 22 | **Modified** (+29): core data structure changed from `int` counter to `UnsyncedEntry` struct |
| 10 | `ok` | Check witness conflict (fast path gate) | 8 | **Modified** (+14): extended for causal consistency checking |
| 11 | `run` | Event loop | 98 | **Modified** (+119): watchdog goroutine, 8 new channel cases, speculative execution |

The `sync`/`unsync` mechanism is the core of CURP's witness-based fast path. Splitting it into strong/causal variants and changing the underlying data structure means the fundamental CURP algorithm is modified.

### Weak Path

| Category | Count | Total Lines |
|---|---|---|
| Causal/weak functions | 23 | 700 |
| Refactored strong functions | 2 | 52 |
| Utility | 6 | 46 |

Notable new functions: `asyncReplicateCausal` (107), `asyncReplicateWeak` (115), `witnessCheck` (32), `computeSpeculativeResult` (24), `computeSpeculativeResultWithUnsynced` (18).

**Summary**: CURP-HO modifies **6 of 11 consensus-critical functions** (55%), including 2 that are removed and split into strong/causal variants. The core sync/unsync witness mechanism — CURP's defining data structure — is fundamentally restructured. The largest code increase of all four protocols.

---

## Comparison Summary

| Metric | Raft-HT (HT) | CURP-HT (HT) | EPaxos-HO (HO) | CURP-HO (HO) |
|--------|:---:|:---:|:---:|:---:|
| Code increase (excl. defs) | +448 (44%) | +636 (59%) | +1,156 (68%) | +1,701 (159%) |
| Critical functions in base | 15 | 11 | 28 | 11 |
| Strong path modified | **0** | **0** | **18** | **6** |
| Strong path change ratio | **0%** | **0%** | **64%** | **55%** |
| Functions removed/refactored | 1 | 0 | 13 | 2 |
| New functions | 3 | 11 | 17 | 31 |
| Core algorithm changed? | No | No | Yes | Yes |

### Key Observations

1. **HT protocols preserve the consensus critical path; HO protocols extensively modify it.**
   - Raft-HT: **0** critical path functions modified. `executeCommands` adds a lock (+4 lines) for concurrent weak read safety, but the strong execution logic itself is unchanged. All AppendEntries/RequestVote logic is identical.
   - CURP-HT: **0** critical path functions modified. `deliver` adds additive `if weak` guards (+17 lines) that only activate for weak commands; the strong command path is untouched. The unsync/sync witness mechanism is identical.
   - EPaxos-HO: modifies **18/28** critical functions (64%). PreAccept, Accept, Prepare, TryPreAccept, Recovery — all modified. 6 core functions removed and split into strong/causal variants (e.g., `updateAttributes`, `bcastCommit`, `startPhase1`). The dep graph now contains both instance types.
   - CURP-HO: modifies **6/11** critical functions (55%). The core `sync`/`unsync` witness mechanism is restructured from a simple counter to a rich struct. 2 functions removed and split into strong/causal variants. `run` rewritten with watchdog for stall detection.

2. **HO protocols have 2-3x more code increase than HT.**
   - This is because weak operations in HO must integrate with the consensus path, requiring changes throughout the codebase.
   - HT weak operations are isolated in independent functions.

3. **Why CURP-HO has more changes than EPaxos-HO despite fewer modified functions.**
   - The two base protocols have fundamentally different architectures, leading to different kinds of modifications.
   - EPaxos is instance-based: each command gets an independent instance with its own deps. Adding causal ops means adding a new instance type (CAUSALLY_COMMITTED) that bypasses PreAccept. The changes touch many functions (12), but each change is similar — adding an `if causal` branch. The core data structure (instance + dep vector) stays the same.
   - CURP's core is the witness mechanism: the leader marks a command as "unsynced", sends it to witness replicas for conflict checking, and can speculatively reply before full commit. CURP-HO requires weak reads to see uncommitted strong writes (for optimal latency). This forces the unsync record to change from a simple counter ("how many unsynced ops") to a rich struct ("what value did each unsynced op write"), because weak reads need the actual values. This change to the core data structure ripples through every function that reads or writes unsync state.
   - In short: EPaxos-HO adds a new type alongside existing ones; CURP-HO restructures the existing core mechanism. Adding is easier than restructuring.

### Conclusion

The T property enables a clean architectural separation: weak operations are handled by **additive** code that does not touch the consensus critical path. Without T property (HO), weak operations are **interleaved** with strong consensus, requiring modifications to core handlers and increasing the risk of introducing correctness bugs — as evidenced by the multiple bugs discovered during EPaxos-HO development (slow path guard, WAITING mechanism, execute scan bottleneck, retryStuckInstances).
