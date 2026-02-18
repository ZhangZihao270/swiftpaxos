# Protocol Spec vs. Implementation: Discrepancy Report

Comparison of `docs/protocol-overview.md` against `curp-ht/` and `curp-ho/` code.

---

## CURP-HT

### Matching Items

| Spec Requirement | Code Location | Status |
|---|---|---|
| Strong ops broadcast to all replicas | `client.go` → `SendProposal()` with `fast: true` | Match |
| Non-leader conflict check (strong-only unsynced) | `curp-ht.go:547` `ok()` + `unsync()` only in ProposeChan | Match |
| Leader speculative result via read-only `ComputeResult()` | `curp-ht.go:591` | Match |
| Fast path = super-majority Ok=TRUE; slow path = SyncReply | `client.go:193` ThreeQuarters quorum `(3N/4)+1` | Match |
| Leader Accept → majority AcceptAck → Commit → slot-order execute | `curp-ht.go:368,441,465,619` | Match |
| Weak writes sent to leader only | `client.go:378` `SendMsg(leader, ...)` | Match |
| Weak write reply includes slot number | `curp-ht.go:928` `rep.Slot = int32(slot)` | Match |
| Weak reads sent to nearest replica | `client.go:411` `SendMsg(ClosestId, ...)` | Match |
| Weak read merge: higher version wins | `client.go:339` `cached.version > replicaVer` | Match |
| Client cache key→(value, version), updated from 4 sources | `client.go:14` `cacheEntry` struct; 4 update sites | Match |
| Non-leader unsynced map contains only strong commands | `unsync()` called only in ProposeChan handler | Match |

### Discrepancies

#### 1. Commit timeout may reply before actual commit (Medium)

- **Spec**: "Reply to client ONLY AFTER commit"
- **Code**: `curp-ht.go:916` — 1-second timeout on commit wait; if it fires, reply is sent anyway
- **Impact**: Under high load or network delay, client could receive confirmation for an uncommitted weak write
- **Same issue at line 938**: execution-order wait also has 1-second timeout

#### 2. `sync()` called for weak commands on non-leaders (Low)

- **Spec**: Weak commands never interact with the unsynced map on non-leaders
- **Code**: `curp-ht.go:586-588` — when a weak command's Accept arrives at a non-leader and eventually commits, `deliver()` calls `sync()` which decrements the unsynced counter, even though `unsync()` was never called for it
- **Impact**: Could prematurely decrement a co-located strong command's counter on the same key. Mitigated by `v < 0 → v = 0` clamp, but logically incorrect under strong/weak same-key interleaving

#### 3. `fast: true` not enforced programmatically (Low)

- **Spec**: Strong ops require broadcast to all replicas
- **Code**: `main.go` curpht case does not set `c.Fast = true`; relies on config file. Other protocols (fastpaxos, n2paxos) set it in code
- **Impact**: If config omits `fast: true`, strong proposals silently degrade to leader-only, breaking the protocol

#### 4. Weak reads not excluded from leader (Low)

- **Spec**: "Client sends a weak read to the nearest replica (not the leader)"
- **Code**: `client.go:418` sends to `ClosestId` unconditionally, which could be the leader
- **Impact**: Functionally safe since `handleWeakRead` works on any replica; minor spec deviation

---

## CURP-HO

### Matching Items

| Spec Requirement | Code Location | Status |
|---|---|---|
| Causal writes broadcast to all replicas | `client.go:595` `sendMsgToAll()` | Match |
| Every replica tracks pending writes for read-your-writes | `curp-ho.go:1389` `addPendingWrite()` | Match |
| Every replica sends speculative CausalReply | `curp-ho.go:1397` `SendClientMsgFast()` | Match |
| Bound replica = closest; non-bound replies discarded | `client.go:110` + `client.go:626` | Match |
| Leader assigns slot, replicates in background | `curp-ho.go:1404` → `asyncReplicateCausal` | Match |
| Per-session causal deps collected from `unsyncedByClient` | `curp-ho.go:832-841` | Match |
| Per-key ReadDep for strong reads (weak PUT on same key) | `curp-ho.go:825-828` | Match |
| Leader speculative: strong read checks pending weak writes | `curp-ho.go:1682` `computeSpeculativeResultWithUnsynced()` | Match |
| Client fast path: causal dep check + ReadDep consistency | `client.go:406` `checkCausalDeps` + `checkReadDepConsistency` | Match |
| CausalDep = seqnum of previous weak write (0 if none) | `client.go:573` `lastWeakWriteSeqNum` | Match |
| `getPendingWrite()` returns only if `seqNum <= causalDep` | `curp-ho.go:1670-1680` | Match |

### Discrepancies

#### 1. Causal reads NOT broadcast — uses MWeakRead instead (High)

- **Spec**: "Client broadcasts a causal propose to all replicas" — reads use `MCausalPropose` same as writes
- **Code**: `client.go:597-619` `SendCausalRead()` sends `MWeakRead` to nearest replica only, NOT `MCausalPropose` broadcast
- **Consequences**:
  - Reads bypass the witness pool on all replicas
  - No `CausalDep` carried in `MWeakRead` message
  - Uses version-based cache merge (CURP-HT style) instead of pending-write speculative reply
  - Non-closest replicas never learn about the read

#### 2. Weak reads skip pending writes — breaks read-your-writes for uncommitted writes (High)

- **Spec**: "For reads, result comes from either same client's pending write (if causalDep covers it) or committed state"
- **Code**: `curp-ho.go:1736-1738` `handleWeakRead()` calls `ComputeResult(r.State)` only — never checks `pendingWrites`
- **Impact**: A weak read issued immediately after a weak write to the same key may return stale data if the write hasn't committed yet. The version-based cache merge partially compensates (causal write reply updates cache) but the replica-side behavior differs from spec

#### 3. Write set cleared on fast path, not just on commit confirmation (Medium)

- **Spec**: "Entries removed ONLY upon receiving the leader's commit confirmation — not upon fast-path or bound-replica completion"
- **Code**: `client.go:442-447` in `handleFastPathAcks()` clears write set entries with `SeqNum < cmdId.SeqNum`
- **Impact**: Premature write set cleanup could cause subsequent causal dep checks to trivially pass (empty write set = no entries to verify), weakening the causal dependency guarantee

#### 4. `witnessCheck` returns Ok=FALSE for strong reads, not just writes (Medium)

- **Spec**: "Per-key conflict detection: if a pending strong write exists on the same key → conflict → Ok=FALSE"
- **Code**: `curp-ho.go:823` checks `entry.IsStrong` without checking `entry.Op == state.PUT`; a pending strong READ on the same key also triggers Ok=FALSE
- **Note**: A correct `checkStrongWriteConflict()` function exists at line 848 (`entry.IsStrong && entry.Op == state.PUT`) but is **unused**
- **Impact**: Unnecessary slow-path fallbacks when strong reads are pending on the same key

#### 5. Strong writes not added to client write set (Low)

- **Spec**: "Client write set tracks uncommitted writes (weak and strong)"
- **Code**: `client.go:531-536` `SendStrongWrite()` does not add to `writeSet`; only `SendCausalWrite()` does
- **Impact**: Causal dep checks on fast path only verify weak writes are witnessed, not strong writes. Acceptable if strong writes' visibility is guaranteed by the Accept-Commit cycle

#### 6. Single entry per key in unsynced map (Low)

- **Spec**: "Witness pool stores both strong and weak commands" (implies multiple can coexist per key)
- **Code**: `curp-ho.go:686-723` — unsynced map uses data key as map key, one entry per key (latest wins)
- **Impact**: If a weak write and strong write target the same key, the later overwrites the earlier, losing ReadDep metadata for the overwritten entry

#### 7. Leader adds to witness pool after speculative reply (Low)

- **Spec**: "Every replica adds command to unsynced witness pool" (before reply)
- **Code**: `curp-ho.go:1379` non-leaders call `unsyncCausal` before reply; leader calls `leaderUnsyncCausal` at line 1404, after reply at line 1397
- **Impact**: Narrow race window where a strong read arriving at the leader between reply and unsync would miss the causal write in the witness pool

---

## Summary

| Protocol | Matches | Discrepancies | High | Medium | Low |
|---|---|---|---|---|---|
| CURP-HT | 11 | 4 | 0 | 1 | 3 |
| CURP-HO | 11 | 7 | 2 | 2 | 3 |

**CURP-HT** is largely faithful to the spec. The main concern is the commit timeout safety net that could violate durability guarantees under extreme load.

**CURP-HO** has two high-severity deviations: weak reads use a completely different mechanism (nearest-replica-only with version merge) instead of the spec's broadcast-with-pending-write-lookup approach. This is a deliberate design tradeoff — the current implementation achieves the same 1-RTT latency with lower network cost, but relies on client-side cache merge rather than replica-side pending write lookup for read-your-writes.
