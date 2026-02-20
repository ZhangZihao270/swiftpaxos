# CURP-HT & CURP-HO Protocol Flows

This document describes the detailed message flows for both CURP-HT and CURP-HO protocols as implemented in SwiftPaxos.

## Table of Contents

- [1. Overview](#1-overview)
- [2. CURP-HT Protocol](#2-curp-ht-protocol)
  - [2.1 Message Types](#21-message-types)
  - [2.2 Strong Write Flow](#22-strong-write-flow)
  - [2.3 Strong Read Flow](#23-strong-read-flow)
  - [2.4 Weak Write Flow](#24-weak-write-flow)
  - [2.5 Weak Read Flow](#25-weak-read-flow)
  - [2.6 Fast Path vs Slow Path](#26-fast-path-vs-slow-path)
- [3. CURP-HO Protocol](#3-curp-ho-protocol)
  - [3.1 Message Types](#31-message-types)
  - [3.2 Strong Write Flow](#32-strong-write-flow)
  - [3.3 Strong Read Flow](#33-strong-read-flow)
  - [3.4 Weak/Causal Write Flow](#34-weakcausal-write-flow)
  - [3.5 Weak/Causal Read Flow](#35-weakcausal-read-flow)
  - [3.6 Witness Pool & Conflict Detection](#36-witness-pool--conflict-detection)
  - [3.7 Fast Path vs Slow Path](#37-fast-path-vs-slow-path)
- [4. CURP-HT vs CURP-HO Comparison](#4-curp-ht-vs-curp-ho-comparison)
- [5. Execution Ordering](#5-execution-ordering)

---

## 1. Overview

Both protocols are hybrid-consistency variants of CURP (Consistent Unordered Replication Protocol). They support two consistency levels:

- **Strong (Linearizable)**: Commands that require a quorum to confirm.
- **Weak (Causal)**: Commands that can be speculatively replied to without waiting for consensus.

The key difference:

| | CURP-HT | CURP-HO |
|---|---|---|
| Weak commands sent to | **Leader only** | **All replicas** (broadcast) |
| Weak completion | Leader replies (1-RTT to leader) | Bound replica replies (1-RTT to closest) |
| Strong-weak interaction | No cross-visibility | Strong reads see uncommitted weak writes via **WeakDep** |
| Witness pool | Strong-only `unsynced` map | Strong + weak entries in `unsynced` map |

---

## 2. CURP-HT Protocol

Source: `curp-ht/curp-ht.go`, `curp-ht/client.go`, `curp-ht/defs.go`

### 2.1 Message Types

| Message | Direction | Purpose |
|---------|-----------|---------|
| `GPropose` | Client -> Leader | Generic propose (strong commands) |
| `MReply` | Leader -> Client | Speculative reply (strong, fast path) |
| `MRecordAck` | Non-leader -> Client | Recording ack with Ok field |
| `MAccept` | Leader -> All | Replication request |
| `MAcceptAck` | All -> Leader | Replication ack |
| `MCommit` | Leader -> All | Commit notification |
| `MSyncReply` | Leader -> Client | Final value after commit (slow path) |
| `MWeakPropose` | Client -> Leader | Weak command proposal |
| `MWeakReply` | Leader -> Client | Immediate weak reply |

### 2.2 Strong Write Flow

```
Client                    Leader                   Non-leaders (R1, R2)
  |                         |                         |
  |--- GPropose(PUT) ------>|                         |
  |--- GPropose(PUT) -------|------------------------>|
  |                         |                         |
  |                    1. leaderUnsync()               |
  |                       assign slot                  |
  |                    2. computeResult()              |
  |                       (speculative, no             |
  |                        state modification)         |
  |<-- MReply(Ok=T/F) -----|                    3. ok() -> TRUE/FALSE
  |                         |                    4. unsync() on key
  |<-- MRecordAck(Ok) ------|-------------------------|
  |                         |                         |
  | [Fast path: if enough   |                         |
  |  Ok=TRUE acks + leader] |                         |
  |                         |                         |
  |                    5. Send MAccept to all          |
  |                         |--- MAccept ------------>|
  |                         |                         |
  |                         |<-- MAcceptAck ----------|
  |                         |                         |
  |                    6. Majority acked -> commit     |
  |                         |--- MCommit ------------>|
  |                         |                         |
  |                    7. Execute(r.State)             |
  |                       (actual state modification)  |
  |<-- MSyncReply ----------|                         |
  |    (slow path only)     |                         |
```

**Key points:**
- Client sends `GPropose` to **all replicas** (leader + non-leaders).
- Leader assigns a slot, computes speculative result, replies immediately with `MReply`.
- Non-leaders check `ok()`: TRUE if no pending unsynced commands on same key.
- After commit, leader calls `Execute()` to actually modify state.
- `MSyncReply` is sent only on the slow path (when fast path fails).

### 2.3 Strong Read Flow

Identical to Strong Write flow, except:
- `GPropose` carries `Op=GET` instead of `Op=PUT`.
- Speculative result is computed via `cmd.ComputeResult(r.State)` (reads committed state).
- After commit, `Execute()` reads from state (idempotent for reads).

### 2.4 Weak Write Flow

```
Client                    Leader                   Non-leaders
  |                         |                         |
  |--- MWeakPropose(PUT) -->|                         |
  |    (leader only!)       |                         |
  |                    1. Assign slot                  |
  |                    2. leaderUnsync()               |
  |                    3. addPendingWrite()            |
  |                    4. computeSpeculativeResult()   |
  |                       -> returns NIL for PUT       |
  |<-- MWeakReply(NIL) -----|                         |
  |    (immediate, 1-RTT)   |                         |
  |                         |                         |
  |                    [async, background goroutine]   |
  |                    5. Send MAccept to all          |
  |                         |--- MAccept ------------>|
  |                         |<-- MAcceptAck ----------|
  |                    6. Wait for commit              |
  |                    7. Wait for slot ordering       |
  |                    8. Wait for causalDep           |
  |                    9. Execute(r.State)             |
  |                   10. removePendingWrite()         |
  |                   11. markWeakExecuted()           |
```

**Key points:**
- Client sends `MWeakPropose` to **leader only** (not broadcast).
- Leader replies **immediately** with `MWeakReply` - no quorum needed.
- Replication happens **asynchronously** in a background goroutine.
- `pendingWrites` map tracks uncommitted writes so later weak reads can see them.
- Execution waits for: (a) commit, (b) slot order, (c) causal dependency.

### 2.5 Weak Read Flow

```
Client                    Leader
  |                         |
  |--- MWeakPropose(GET) -->|
  |                    1. Assign slot
  |                    2. leaderUnsync()
  |                    3. computeSpeculativeResult():
  |                       - Check pendingWrites for
  |                         same client + same key
  |                         where seqNum <= causalDep
  |                       - If found: return pending value
  |                       - Else: read committed state
  |<-- MWeakReply(val) -----|
  |    (immediate, 1-RTT)   |
  |                         |
  |                    [async replication + execution]
```

**Key points:**
- Same as weak write, except speculative result computation differs.
- `computeSpeculativeResult()` checks `pendingWrites` map for the same client's uncommitted writes on the same key, enabling **read-your-writes** semantics.
- The `causalDep` field acts as a visibility fence: only pending writes with `seqNum <= causalDep` are visible.

### 2.6 Fast Path vs Slow Path

**Fast Path** (strong commands only):
- Client collects `MRecordAck` from non-leaders.
- If majority (N/2) have `Ok=TRUE`, **plus** leader's `MReply` received -> deliver immediately.
- Alternatively: 3/4 quorum of `Ok=TRUE` can complete fast path.

**Slow Path**:
- Not enough `Ok=TRUE` responses (key conflict on some replicas).
- Client waits for `MSyncReply` after the commit phase completes.

**Weak commands**: Never have a slow path. Client always completes on `MWeakReply`.

---

## 3. CURP-HO Protocol

Source: `curp-ho/curp-ho.go`, `curp-ho/client.go`, `curp-ho/defs.go`

### 3.1 Message Types

All CURP-HT messages, plus:

| Message | Direction | Purpose |
|---------|-----------|---------|
| `MRecordAck.WeakDep` | Non-leader -> Client | **New field**: optional `*CommandId` pointing to uncommitted weak write |
| `MCausalPropose` | Client -> **All replicas** | Causal command (replaces leader-only MWeakPropose for causal path) |
| `MCausalReply` | **All replicas** -> Client | Speculative reply from all; client filters by bound replica |
| `MWeakPropose` | Client -> Leader | Still exists for leader-only weak path |
| `MWeakReply` | Leader -> Client | Leader's immediate reply |

### 3.2 Strong Write Flow

```
Client                    Leader                   Non-leaders (R1, R2)
  |                         |                         |
  |--- GPropose(PUT) ------>|                         |
  |--- GPropose(PUT) -------|------------------------>|
  |                         |                         |
  |                    1. leaderUnsyncStrong()    3. okWithWeakDep():
  |                       assign slot                 - Strong write pending?
  |                    2. computeSpeculative-            -> (FALSE, nil)
  |                       ResultWithUnsynced()        - Any strong pending?
  |                       -> NIL for PUT                -> (FALSE, nil)
  |                                                   - Weak write pending?
  |                                                     -> (TRUE, nil)
  |                                                     [no weakDep for writes]
  |                                                4. unsyncStrong() on key
  |<-- MReply(Ok=T/F) -----|                         |
  |<-- MRecordAck(Ok,nil) --|-------------------------|
  |                         |                         |
  |                    [Accept -> Commit -> Execute same as CURP-HT]
```

**Key difference from CURP-HT**: `okWithWeakDep()` replaces `ok()`. For strong **writes**, even if there's a pending weak write on the same key, it returns `(TRUE, nil)` - no WeakDep needed because writes don't read.

### 3.3 Strong Read Flow

```
Client                    Leader                   Non-leaders (R1, R2)
  |                         |                         |
  |--- GPropose(GET) ------>|                         |
  |--- GPropose(GET) -------|------------------------>|
  |                         |                         |
  |                    1. leaderUnsyncStrong()    3. okWithWeakDep():
  |                       assign slot                 - Strong write pending?
  |                    2. computeSpeculative-            -> (FALSE, nil)
  |                       ResultWithUnsynced():       - Any strong pending?
  |                       - Check unsynced for           -> (FALSE, nil)
  |                         weak write on key         - Weak WRITE pending +
  |                       - If found: return            incoming is GET?
  |                         weak write value            -> (TRUE, &weakDep)
  |                       - Else: committed state       [weakDep = CmdId of
  |                                                      pending weak write]
  |                                                   - Otherwise:
  |                                                     -> (TRUE, nil)
  |                                                4. unsyncStrong()
  |<-- MReply(specVal) -----|                         |
  |<-- MRecordAck(Ok=T, ----|-------------------------|
  |    WeakDep=&cmdId)      |                         |
  |                         |                         |
  | [Client fast path:                                |
  |  check WeakDep consistency                        |
  |  across all non-leaders]                          |
```

**This is the core CURP-HO innovation:**

1. Non-leaders return `WeakDep` when a strong READ arrives and there's a pending weak write on the same key.
2. The client checks whether **all non-leaders report the same WeakDep** (`checkWeakDepConsistency()`).
3. If consistent: fast path succeeds - the strong read correctly accounts for the weak write dependency.
4. If inconsistent: fall back to slow path, because different replicas have different views of uncommitted weak writes.

### 3.4 Weak/Causal Write Flow

```
Client                    Leader                   Non-leaders (R1, R2)
  |                         |                         |
  |--- MCausalPropose(PUT)-->|                        |
  |--- MCausalPropose(PUT)---|----------------------->|
  |    (broadcast to ALL!)   |                        |
  |                         |                         |
  |                    ALL REPLICAS:                   |
  |                    1. unsyncCausal() - add to witness pool
  |                    2. addPendingWrite() - track for speculative reads
  |                    3. computeSpeculativeResult() -> NIL for PUT
  |                    4. Send MCausalReply to client
  |                         |                         |
  |<-- MCausalReply --------|                         |
  |<-- MCausalReply ---------|------------------------|
  |                         |                         |
  | [Client: accept ONLY    |                         |
  |  boundReplica's reply   |                         |
  |  = 1-RTT to closest]    |                         |
  |                         |                         |
  |                    LEADER ONLY:                    |
  |                    5. Assign slot                  |
  |                    6. leaderUnsyncCausal()         |
  |                    7. asyncReplicateCausal():      |
  |                       - MAccept -> all             |
  |                       - Wait commit                |
  |                       - Wait slot order            |
  |                       - Wait causalDep             |
  |                       - Execute(r.State)           |
  |                       - removePendingWrite()       |
  |                       - markWeakExecuted()         |
```

**Key differences from CURP-HT weak write:**
- Broadcast to **all replicas**, not just leader.
- **All replicas** add to witness pool (`unsyncCausal`) and reply with `MCausalReply`.
- Client only uses **bound replica's** reply (closest replica, set during `Connect()`).
- Latency = 1-RTT to closest replica (can be lower than 1-RTT to leader).

### 3.5 Weak/Causal Read Flow

```
Client                    Leader                   Non-leaders (R1, R2)
  |                         |                         |
  |--- MCausalPropose(GET)->|                         |
  |--- MCausalPropose(GET)--|------------------------>|
  |    (broadcast to ALL!)  |                         |
  |                         |                         |
  |                    ALL REPLICAS:                   |
  |                    1. unsyncCausal() - add to witness pool
  |                    2. [skip addPendingWrite - it's a read]
  |                    3. computeSpeculativeResult():
  |                       - getPendingWrite(clientId, key, causalDep)
  |                         checks: same client, same key,
  |                         pending.seqNum <= causalDep
  |                       - If found: return pending write value
  |                         (read-your-writes)
  |                       - Else: read committed state
  |                    4. Send MCausalReply(val) to client
  |                         |                         |
  |<-- MCausalReply(val) ---|                         |
  |    [from bound replica] |                         |
  |                         |                         |
  |                    LEADER ONLY:                    |
  |                    [async replication + execution] |
```

**Speculative read-your-writes:**
- `pendingWrites` map is keyed by `"clientId:key"`.
- Per (client, key) pair, only the **latest** pending write is stored (last-writer-wins).
- `causalDep` acts as a visibility fence: `getPendingWrite()` only returns a pending write if `pending.seqNum <= causalDep`.
- This ensures a causal read sees exactly the writes that causally precede it in the client's causal chain.

### 3.6 Witness Pool & Conflict Detection

The `unsynced` concurrent map is the witness pool. Each entry is an `UnsyncedEntry`:

```go
type UnsyncedEntry struct {
    Slot     int          // Slot (leader) or pending count (non-leader)
    IsStrong bool         // true=strong, false=causal/weak
    Op       Operation    // GET / PUT / SCAN
    Value    Value        // For PUT: the written value
    ClientId int32
    SeqNum   int32
    CmdId    CommandId
}
```

**How entries are added:**

| Operation | Function | Who calls it |
|-----------|----------|-------------|
| Strong command on non-leader | `unsyncStrong()` | Non-leaders on GPropose |
| Strong command on leader | `leaderUnsyncStrong()` | Leader on GPropose |
| Causal/weak command on all | `unsyncCausal()` | All replicas on MCausalPropose |
| Causal/weak command on leader | `leaderUnsyncCausal()` | Leader for slot assignment |

**Conflict detection: `okWithWeakDep(cmd)`**

Called by non-leaders when processing a strong command. Checks the `unsynced` map for the same key:

```
Pending entry          Incoming cmd     Result
--------------------------------------------------------------
Strong write           any              (FALSE, nil)
Any strong op          any              (FALSE, nil)
Weak write             strong READ      (TRUE, &weakDep)
Weak write             strong WRITE     (TRUE, nil)
Nothing pending        any              (TRUE, nil)
```

**Why strong reads get WeakDep but strong writes don't:**
- A strong read needs to know about uncommitted weak writes to ensure consistency: if the weak write commits, the read should have seen it.
- A strong write doesn't read, so it doesn't matter whether a pending weak write exists - the write just overwrites.

### 3.7 Fast Path vs Slow Path

**Fast path** for strong commands:
1. Client collects `MRecordAck` from non-leaders.
2. When 3/4 quorum reached, call `checkWeakDepConsistency()`:
   - All non-leader `WeakDep` fields must be **identical** (all nil, or all pointing to same CmdId).
   - If consistent: deliver immediately.
   - If inconsistent: fall back to slow path.
3. When majority (1/2) reached (slow path quorum): deliver via `handleSlowPathAcks()`.

**Weak/causal commands**: No fast/slow path distinction. Always complete on bound replica's reply.

---

## 4. CURP-HT vs CURP-HO Comparison

| Aspect | CURP-HT | CURP-HO |
|--------|---------|---------|
| **Weak command destination** | Leader only | Broadcast to all replicas |
| **Weak reply source** | Leader (`MWeakReply`) | Bound replica (`MCausalReply`) |
| **Weak completion latency** | 1-RTT to leader | 1-RTT to closest replica |
| **Non-leader ack for strong** | `Ok` only | `Ok` + optional `WeakDep` |
| **Strong read sees weak writes?** | No | Yes (via `getWeakWriteValue()` on leader, `WeakDep` on non-leaders) |
| **Witness pool entries** | Strong only | Strong + weak/causal |
| **Fast path extra check** | None | `WeakDep` consistency across non-leaders |
| **Causal dependency tracking** | `CausalDep` (same) | `CausalDep` (same) |
| **`pendingWrites` populated by** | Leader only | All replicas |

### Why CURP-HO?

CURP-HT's weakness: weak commands go through the leader only. If the leader is far from the client, weak command latency is high (1-RTT to leader). Also, strong reads cannot see uncommitted weak writes, which can cause inconsistency when mixing strong and weak commands.

CURP-HO fixes both:
1. **Lower weak latency**: Client binds to closest replica; weak commands complete in 1-RTT to that replica.
2. **Cross-consistency**: Strong reads can see uncommitted weak writes via the witness pool, and the WeakDep mechanism ensures fast-path correctness.

---

## 5. Execution Ordering

Both protocols enforce the same execution ordering:

### 5.1 Global Slot Ordering
- All commands (strong and weak) share the same slot space on the leader.
- Slot N can only execute after slot N-1 has executed.
- Enforced via `r.executed` map and `getOrCreateExecuteNotify()` channels.

### 5.2 Causal Ordering (Weak Commands)
- Within a single client, weak commands form a causal chain via `CausalDep`.
- Each weak command records `CausalDep = lastWeakSeqNum` (the previous weak command's SeqNum).
- Server's `asyncReplicateWeak/Causal()` waits for `CausalDep` to be executed before executing current command.
- Enforced via `waitForWeakDep()` and `markWeakExecuted()`.

### 5.3 Speculative vs Committed Execution

```
Phase         What happens                 State modified?
-----------------------------------------------------------------
Speculation   computeSpeculativeResult()   No  (read-only)
Commit        cmd.Execute(r.State)         Yes (actual state change)
```

- Speculative results are returned to clients immediately.
- Committed execution happens later, in slot order, after consensus.
- For reads, the speculative result and committed result should match (assuming no conflicts).
- For writes, speculation returns NIL; the actual write happens at commit time.

### 5.4 Pending Write Lifecycle

```
addPendingWrite()                    removePendingWrite()
      |                                      |
      v                                      v
  On propose arrival                  After committed execution
  (before speculative reply)          (in asyncReplicate goroutine)

  pendingWrites["clientId:key"] = {seqNum, value}
      |
      | used by computeSpeculativeResult()
      | for read-your-writes semantics
      v
  getPendingWrite(clientId, key, causalDep)
      -> returns value if pending.seqNum <= causalDep
```

Per (client, key) pair, only the **latest** write is stored (highest seqNum wins). This is sufficient because the causal chain is linear per client.
