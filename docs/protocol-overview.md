# CURP-HT & CURP-HO Protocol Overview

A natural-language description of both hybrid consistency protocols.

---

## CURP-HT (Hybrid + Transparency)

**Key Idea**: Weak ops go to leader only; strong ops broadcast to all. The leader's log serialization satisfies hybrid consistency (C1–C3) without modifying the strong operation code path.

### Strong Operations (Linearizable)

Client broadcasts a propose to all replicas. Each non-leader checks whether it has any pending (unsynced) strong commands on the same key. If no conflict, it replies `Ok=TRUE` to the client; otherwise `Ok=FALSE`. The leader speculatively computes the result from committed state (without modifying state) and replies immediately.

The client collects these replies. If a super-majority reply `Ok=TRUE`, the client completes on the **fast path** without waiting for consensus — the speculative result is the final answer. If not enough `Ok=TRUE`, the client falls back to the **slow path**: it waits for the leader to complete the full Accept-Commit cycle, at which point the leader sends a `SyncReply` with the committed result.

Meanwhile, the leader coordinates consensus in the background: it sends an Accept message to all replicas, waits for majority acknowledgment, then commits. After commit, commands execute in strict slot order, which is when state is actually modified.

Non-leaders are entirely unaware of weak operations. Their witness pools contain only strong commands, and their conflict detection logic is unchanged from the original CURP protocol. This is how CURP-HT achieves transparency.

### Weak Operations (Causal)

Client sends a propose to the **leader only**. The leader assigns a slot, then **immediately** replies with a speculative result — no quorum, no waiting. For a weak write, the reply value is NIL; for a weak read, the leader checks its `pendingWrites` map for the same client's uncommitted writes on the same key (read-your-writes), and if found, returns that pending value; otherwise it reads committed state.

Replication happens asynchronously in the background. The leader sends Accept to other replicas, waits for commit, then executes the command in slot order. Execution also respects the per-client causal chain: each weak command carries a `CausalDep` (the SeqNum of this client's previous weak command), and the leader waits for that dependency to be executed before executing the current one.

Because weak commands only go to the leader, they have low network overhead but their latency equals the round-trip time to the leader.

### Satisfying Hybrid Consistency (C1–C3)

All operations (weak and strong) are assigned slots in the leader's log:

- **C1 & C2**: A weak write from a given session occupies an earlier log slot than any subsequent strong operation from the same session. Log-ordered replication ensures the weak write is committed before the strong operation. Execution in slot order guarantees the strong operation observes the correct state.
- **C3**: Weak writes are processed only at the leader and remain invisible to other replicas until committed. Cross-session causal chains at non-leader replicas cannot include unreplicated weak writes.

### Conflict Detection

Non-leaders maintain an `unsynced` map keyed by the operation's key. Each entry is a counter of how many unconfirmed strong commands are pending on that key. When a new strong command arrives, the non-leader checks:

- Counter > 0 → conflict → `Ok=FALSE`
- Counter = 0 → no conflict → `Ok=TRUE`

Weak commands do not appear in the non-leader's unsynced map (they only go to the leader), so they never cause conflicts for strong commands at non-leaders.

---

## CURP-HO (Hybrid + Optimal latency)

**Key Idea**: Weak ops broadcast to all replicas (creating a witness pool), and the client completes by waiting only for the closest replica's reply. Strong operations track causal dependencies on same-session weak writes and read dependencies on same-key weak writes, ensuring hybrid consistency while achieving optimal latency.

### Weak/Causal Operations

Client **broadcasts** a causal propose to **all replicas** (not just leader). Every replica that receives it does three things:

1. **Witness pool**: Add the command to the `unsynced` map so that future strong commands can detect it during conflict checking and dependency tracking.
2. **Pending writes**: If it's a write, record it in `pendingWrites` so that later reads from the same client can see it (read-your-writes).
3. **Speculative reply**: Compute the result and send a `CausalReply` back to the client. For reads, the result comes from either the same client's pending write (if `causalDep` covers it) or the committed state.

The client is **bound to its closest replica** (determined during connection setup by measuring latency). It only waits for the bound replica's reply and ignores all others. This means weak command latency = 1-RTT to the closest replica, which can be much faster than 1-RTT to the leader.

Separately, the leader (which also received the broadcast) coordinates replication in the background: assign a slot, send Accept, wait for commit, then execute in slot order respecting the causal chain.

### Strong Operations (Linearizable)

Client broadcasts to all replicas, same as CURP-HT. The key difference is in what the non-leader witnesses do and what the client checks on the fast path.

#### Witness Checks

When a non-leader receives a strong command, it performs three checks:

1. **Per-key conflict detection**: If a pending **strong write** exists on the same key → conflict, return `Ok=FALSE`. (Same as CURP-HT.)

2. **Per-session causal dependency**: For any strong command (read or write), the witness collects all weak writes from the **same session** present in its witness pool and reports their identifiers as **causal dependencies** in its response. This allows the client to verify that its preceding weak writes have been witnessed.

3. **Per-key read dependency (strong reads only)**: If a pending weak write exists on the same key — from **any session** — the witness reports the weak write's identifier as a **read dependency** (`ReadDep`). This tells the client which weak write the speculative result may depend on.

#### Leader Speculative Execution

For strong reads, the leader checks its witness pool for pending weak writes on the same key (from any session). If found, the leader returns the weak write's value as the speculative result. For strong writes, the leader returns NIL as usual.

#### Client Write Set

The client maintains a **write set** of its uncommitted writes (both weak and strong). Entries are added when a write is issued and removed **only upon receiving the leader's commit confirmation** — not upon fast-path or bound-replica completion. This write set is used for the causal dependency check on the fast path.

#### Client Fast Path

The client's fast path checks **two conditions**:

1. **Causal dependency check** (all strong commands): Every weak write in the client's write set must appear in the causal dependencies reported by a **super-majority** of witnesses. This ensures same-session weak writes are fault-tolerant before the strong command completes.

2. **ReadDep consistency check** (strong reads only): A super-majority of witnesses must report the **same ReadDep** — either all nil or all pointing to the same weak write. This ensures a consistent view of the same-key weak state across replicas. If witnesses disagree on which weak write they saw on the key, the fast path fails.

If either check fails, the client falls back to the **slow path** and waits for the leader's `SyncReply` after the Accept-Commit cycle.

#### Example Scenarios

**Scenario 1: Same-session weak write → strong read (same key)**
```
Client issues: WeakWrite(x=1) → StrongRead(x)

1. WeakWrite(x=1) broadcasts to all replicas, bound replica replies immediately
2. StrongRead(x) broadcasts to all replicas
3. Each witness:
   - Per-session causal dep: finds WeakWrite(x=1) from same session → reports it
   - Per-key ReadDep: finds WeakWrite(x=1) on same key → reports it
4. Leader: finds WeakWrite(x=1) on key x → speculative result = 1
5. Client fast path:
   - Causal dep check: WeakWrite(x=1) in write set, appears in super-majority causal deps ✓
   - ReadDep check: all witnesses report same ReadDep (WeakWrite(x=1)) ✓
   → Fast path succeeds, returns 1
```

**Scenario 2: Same-session weak write → strong write (different key)**
```
Client issues: WeakWrite(x=1) → StrongWrite(y=2)

1. WeakWrite(x=1) broadcasts to all replicas
2. StrongWrite(y=2) broadcasts to all replicas
3. Each witness:
   - Per-key conflict: no strong write on key y → Ok=TRUE
   - Per-session causal dep: finds WeakWrite(x=1) from same session → reports it
   - No ReadDep (strong writes don't read)
4. Client fast path:
   - Causal dep check: WeakWrite(x=1) in write set, appears in super-majority ✓
   → Fast path succeeds; WeakWrite(x=1) is fault-tolerant before StrongWrite(y=2) becomes visible
```

**Scenario 3: Cross-session weak write → strong read (same key)**
```
Session A issues: WeakWrite(x=1)
Session B issues: StrongRead(x)

1. Session A's WeakWrite(x=1) broadcasts to all replicas
2. Session B's StrongRead(x) broadcasts to all replicas
3. Each witness:
   - Per-session causal dep: no same-session (B) weak writes → empty
   - Per-key ReadDep: finds WeakWrite(x=1) from session A on same key → reports it
4. Leader: finds WeakWrite(x=1) on key x → speculative result = 1
5. Client B fast path:
   - Causal dep check: B's write set may be empty → trivially passes ✓
   - ReadDep check: all witnesses report same ReadDep → consistent ✓
   → Fast path succeeds, returns 1; the cross-session weak write is observed safely
```

**Scenario 4: Witnesses disagree on ReadDep**
```
Session A issues: WeakWrite(x=1)
Session B issues: StrongRead(x)
But WeakWrite(x=1) has only reached some witnesses, not all.

3. Witness R1: sees WeakWrite(x=1) → ReadDep = WeakWrite(x=1)
   Witness R2: hasn't received it yet → ReadDep = nil
   Witness R3: sees WeakWrite(x=1) → ReadDep = WeakWrite(x=1)
5. Client B fast path:
   - ReadDep check: witnesses disagree (some nil, some point to WeakWrite) ✗
   → Fast path fails, fall back to slow path
   → Slow path returns committed result after leader's consensus
```

### Satisfying Hybrid Consistency (C1–C3)

- **C1 (same-session read-from) & C2 (same-session causal delivery)**: Both handled by the causal dependency mechanism. Each witness reports same-session weak writes, and the client verifies its write set entries appear in a super-majority of reports. This ensures all same-session weak writes — same key (C1) or different keys (C2) — are fault-tolerant before the strong command completes. For strong reads, the leader's speculative execution returns the weak write's value.

- **C3 (cross-session visibility)**: For strong writes, the causal dependency mechanism tracks only same-session weak writes, so cross-session weak writes remain invisible until committed (visibility barrier). For strong reads, the ReadDep mechanism allows observing a cross-session weak write on the same key, but only when a super-majority of witnesses agree on which write is observed, guaranteeing fault-tolerance.

### Witness Pool

The `unsynced` concurrent map serves as the witness pool. Each entry stores:

- `IsStrong`: whether the pending command is strong or weak/causal.
- `Op`: GET or PUT.
- `Value`: the written value (for PUT commands).
- `CmdId`: the command's identity (ClientId + SeqNum).
- `Slot`: on the leader, the actual slot number; on non-leaders, a counter of pending commands for this key.

Both strong and weak commands create entries in the witness pool. This is the fundamental difference from CURP-HT, where only strong commands appear in the non-leaders' unsynced map. By including weak writes in the witness pool, witnesses can track causal dependencies and read dependencies.

### Causal Dependency Chain

Each weak/causal command carries a `CausalDep` field — the SeqNum of this client's previous weak command (0 if none). This forms a per-client linear chain:

```
weak_1 (dep=0) → weak_2 (dep=1) → weak_3 (dep=2) → ...
```

This chain serves two purposes:
1. **Read-your-writes**: When computing a speculative result for a weak read, `getPendingWrite()` only returns a pending write if `pending.seqNum <= causalDep`. This ensures the read only sees writes that causally precede it.
2. **Write set management**: The client's write set tracks all uncommitted writes. Entries are cleared only upon the leader's commit confirmation, not on fast-path or bound-replica completion.

### Bound Replica

During connection setup, the client measures latency to all replicas and binds to the closest one. The binding is stored client-side (`boundReplica = closestId`). When a `CausalReply` arrives, the client checks `if rep.Replica != c.boundReplica { return }` — replies from non-bound replicas are silently discarded.

This is why CURP-HO achieves optimal weak latency: the bound replica is physically closest, so the round trip is minimized. If the bound replica happens to be the leader, it also coordinates replication — otherwise, replication is handled independently by the leader.

---

## Comparison Summary

|  | CURP-HT | CURP-HO |
|--|---------|---------|
| **Weak command destination** | Leader only | All replicas (broadcast) |
| **Weak completion source** | Leader | Bound replica (closest) |
| **Weak latency** | 1-RTT to leader | 1-RTT to closest replica |
| **Non-leader witness pool** | Strong commands only | Strong + weak commands |
| **Network cost for weak ops** | Low (leader only) | Higher (broadcast to all) |
| **Strong reads see weak writes?** | No (committed state only) | Yes (via ReadDep + leader speculative execution) |
| **Witness checks for strong ops** | Per-key conflict (strong only) | Per-key conflict + per-session causal deps + per-key ReadDep |
| **Client write set** | Not needed | Maintained; cleared on leader commit confirmation |
| **Fast-path extra checks** | None | Causal dep check (all) + ReadDep consistency (reads) |
| **Speculative execution on leader** | Reads committed state only | Reads committed state + uncommitted weak writes (same key) |
| **HOT properties** | H + T | H + O |

### When to use which?

- **CURP-HT**: When weak commands are infrequent or the leader is already close to clients. Simpler design, lower network overhead for weak ops, and strong operations are completely unchanged from the original CURP protocol (transparency).
- **CURP-HO**: When weak command latency matters and clients are geographically distributed. The bound-replica mechanism ensures optimal latency regardless of leader placement. Causal dependency tracking ensures same-session weak writes are fault-tolerant before strong operations complete. ReadDep consistency ensures strong reads correctly observe same-key weak writes across sessions.
