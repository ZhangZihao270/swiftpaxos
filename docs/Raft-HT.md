# Raft-HT Implementation Plan

## 1. Protocol Overview

Raft-HT extends vanilla Raft with weak (causal) operations to achieve **H + T** (Hybrid Consistency + Transparency). It is the simplest HOT-optimal protocol because the leader's sequential log implicitly satisfies conditions C1–C3 without modifying the strong operation path.

### Key Properties
- **H (Hybrid Consistency)**: Strong ops are linearizable; weak ops are causally consistent; linearizability holds even when strong ops observe weak effects.
- **T (Transparency)**: Strong operations are completely unmodified from vanilla Raft — no dependency tracking, no waiting for weak ops.
- **Not O**: Weak writes must go through the leader (1 WAN RTT), not local (1 LAN RTT).

### Latency Summary

| Operation | Latency | Path |
|-----------|---------|------|
| Strong read/write | 2 WAN RTT | Client → Leader → Majority replication → Leader → Client |
| Weak write | 1 WAN RTT | Client → Leader (assign log slot, reply immediately, replicate in background) |
| Weak read | 1 LAN RTT | Client → Nearest replica (read committed state) |

---

## 2. Protocol Description

### 2.1 Strong Operations (Unchanged Raft)

Completely standard Raft. No modifications whatsoever.

1. Client sends command to leader.
2. Leader appends to log, assigns slot number.
3. Leader replicates to majority via AppendEntries.
4. Once majority acknowledges, leader commits (advances commitIndex).
5. Leader executes command in log order, replies to client.

### 2.2 Weak Writes (Early Reply at Leader)

1. Client sends weak write to leader (tagged with `consistency = weak`).
2. Leader assigns the write a slot in the log (same log as strong ops).
3. Leader **immediately replies to client** without waiting for replication.
4. The write is replicated to followers through Raft's normal AppendEntries mechanism in the background.

**Why this satisfies C1–C3:**
- The weak write occupies a log slot, so it is ordered before all subsequent entries.
- When a later strong operation at slot `n` commits (majority-replicated), all preceding slots including the weak write at slot `m < n` are also committed by Raft's commit rule.
- C1 (same-session read-from): The log serializes the weak write before any subsequent strong read in the same session.
- C2 (same-session causal delivery): The log serializes the weak write before any subsequent strong write in the same session.
- C3 (cross-session fault-tolerance): A weak write becomes visible to other replicas only after it is replicated through the log. At that point, it is committed (majority-replicated) because the follower applies entries only up to commitIndex.

**Why T is satisfied:** Strong operations follow standard Raft — they don't know or care whether preceding log entries are weak or strong. The leader's log provides implicit ordering.

### 2.3 Weak Reads (Local at Nearest Replica)

1. Client sends weak read to the **nearest replica** (not necessarily the leader).
2. The replica reads the key's value from its committed state (entries up to commitIndex that have been applied to the state machine).
3. The replica returns the value along with the key's **version** (the log index of the last committed write to that key).
4. The client maintains a local **causal set** — a cache of `(key, version, value)` tuples from its own writes and prior reads.
5. Upon receiving the replica's response, the client merges: the entry with the higher version wins.
   - If the replica is stale (its version < client's cached version), the client returns the cached value.
   - If the replica is fresher (its version >= client's cached version), the client uses the replica's value and updates the causal set.

---

## 3. Implementation Plan on Vanilla Raft

### 3.1 Prerequisites

- **Language**: Go 1.20.2 (matching existing codebase)
- **Framework**: Shared framework with common RPC layer and key-value data store (from Orca project)
- **RPC**: Copilot RPC stub generator (https://github.com/princeton-sns/copilot)
- **Base**: Start from the existing vanilla Raft implementation (needs to be implemented first or use an existing Go Raft library)

### 3.2 Step 0: Implement Vanilla Raft Baseline

If vanilla Raft is not yet implemented, implement it first within the shared framework:

1. **Log replication**: Leader appends entries, replicates via AppendEntries RPC.
2. **Commit rule**: commitIndex advances when entry is replicated to majority.
3. **State machine**: Apply committed entries in order to a key-value store.
4. **Leader election**: Standard Raft election with term-based voting.
5. **Client interface**: `Request(command, key, value)` → result.

This serves as both the vanilla Raft baseline for evaluation and the foundation for Raft-HT.

### 3.3 Step 1: Extend Log Entry with Consistency Tag

**File changes**: Log entry struct

```go
type LogEntry struct {
    Term        int
    Index       int
    Command     Command
    Consistency ConsistencyLevel  // NEW: "strong" or "weak"
}

type ConsistencyLevel int
const (
    Strong ConsistencyLevel = iota
    Weak
)
```

This is the only structural change to the log. The rest of Raft's log replication is unchanged.

### 3.4 Step 2: Extend Client Request Interface

**File changes**: Client request struct, client RPC handler

```go
type ClientRequest struct {
    Command     Command
    Consistency ConsistencyLevel  // NEW: client specifies per-operation
    SessionID   string            // For causal set tracking
}
```

The leader's request handler branches on consistency level:
- `Strong` → standard Raft path (wait for majority commit, then reply)
- `Weak write` → assign log slot, reply immediately (Step 3)
- `Weak read` → handled at any replica (Step 5)

### 3.5 Step 3: Implement Weak Write Path (Leader Side)

**File changes**: Leader request handler

When the leader receives a weak write:

```
func (r *Raft) handleClientRequest(req ClientRequest) Response {
    if req.Consistency == Weak && req.Command.IsWrite() {
        // Assign log slot (same as strong — append to log)
        entry := LogEntry{
            Term:        r.currentTerm,
            Index:       r.nextIndex(),
            Command:     req.Command,
            Consistency: Weak,
        }
        r.log.Append(entry)

        // Reply IMMEDIATELY — do not wait for replication
        // The entry will be replicated in the background via
        // normal AppendEntries heartbeats/batches.
        return Response{
            Success: true,
            Value:   nil,
            Version: entry.Index,  // Return log index as version
        }
    }

    // Strong path: unchanged Raft
    // ... standard Raft commit-then-reply ...
}
```

**Critical**: The background replication is already handled by Raft's existing AppendEntries mechanism. No new replication code is needed. The weak write just sits in the log and gets replicated with the next batch.

### 3.6 Step 4: Extend State Machine with Version Tracking

**File changes**: Key-value store / state machine

```go
type KVStore struct {
    data     map[string]string   // key → value
    versions map[string]int      // key → log index of last write (version)
}

func (kv *KVStore) Apply(entry LogEntry) interface{} {
    if entry.Command.IsWrite() {
        kv.data[entry.Command.Key] = entry.Command.Value
        kv.versions[entry.Command.Key] = entry.Index  // Track version
        return nil
    }
    // Read: return value + version
    return ReadResult{
        Value:   kv.data[entry.Command.Key],
        Version: kv.versions[entry.Command.Key],
    }
}
```

This is needed for weak reads (Step 5) to return the key's version.

### 3.7 Step 5: Implement Weak Read Path (Any Replica)

**File changes**: New RPC handler on all replicas, client-side causal set

**Server side** (any replica, including followers):

```
func (r *Raft) handleWeakRead(req WeakReadRequest) WeakReadResponse {
    // Read from committed state (applied entries up to commitIndex)
    value := r.kvStore.data[req.Key]
    version := r.kvStore.versions[req.Key]

    return WeakReadResponse{
        Value:   value,
        Version: version,
    }
}
```

**Client side** (causal set merge):

```
type CausalSet struct {
    cache map[string]CausalEntry  // key → (value, version)
}

type CausalEntry struct {
    Value   string
    Version int
}

func (cs *CausalSet) Merge(key string, replicaValue string, replicaVersion int) (string, int) {
    if cached, ok := cs.cache[key]; ok && cached.Version > replicaVersion {
        // Client has fresher data (from own writes or prior reads)
        return cached.Value, cached.Version
    }
    // Replica has fresher or equal data
    cs.cache[key] = CausalEntry{Value: replicaValue, Version: replicaVersion}
    return replicaValue, replicaVersion
}
```

**Causal set is updated on**:
- Every weak write reply (client caches key, value, version=logIndex)
- Every weak read reply (merged as above)
- Every strong read reply (same merge logic)

### 3.8 Step 6: Weak Read Routing (Client Side)

**File changes**: Client connection logic

The client must be able to send weak reads to the nearest replica, not just the leader:

```
func (c *Client) Read(key string, consistency ConsistencyLevel) ReadResult {
    if consistency == Weak {
        // Send to nearest replica
        resp := c.nearestReplica.WeakRead(key)
        value, version := c.causalSet.Merge(key, resp.Value, resp.Version)
        return ReadResult{Value: value, Version: version}
    }
    // Strong read: send to leader (standard Raft)
    return c.leader.StrongRead(key)
}
```

### 3.9 Step 7: Failure Recovery (Leader Change)

**No special handling needed.** This is a key advantage of Raft-HT:

- **Uncommitted weak writes after leader crash**: The new leader may or may not have the uncommitted weak write in its log. If it doesn't, the weak write is lost — this is acceptable because weak writes have no durability guarantee before being committed.
- **Committed weak writes**: Already majority-replicated. Standard Raft recovery preserves them.
- **Log consistency**: Standard Raft leader election and log repair (leader overwrites inconsistent follower entries) works unchanged. The consistency tag on log entries doesn't affect Raft's election or log matching.

The only consideration: a client's causal set may contain a version from a lost weak write. After leader change, a weak read from a replica may return an older version. The causal set merge handles this correctly — the client keeps the higher version (its cached one), which is harmless since the value will eventually be overwritten or the client will restart.

---

## 4. Summary of Changes vs Vanilla Raft

| Component | Change | Complexity |
|-----------|--------|------------|
| LogEntry struct | Add `Consistency` field | Trivial |
| Client request | Add `Consistency` field | Trivial |
| Leader request handler | Branch: weak write → early reply | Small |
| KV store | Add version tracking per key | Small |
| All replicas | New WeakRead RPC handler | Small |
| Client | Causal set (local cache + merge) | Medium |
| Client | Weak read routing to nearest replica | Small |
| Log replication | **No change** | None |
| Commit rule | **No change** | None |
| Leader election | **No change** | None |
| AppendEntries | **No change** | None |

**Total new code**: ~200–300 lines of Go (excluding tests).

**Key insight**: The strong operation path is literally zero lines of change. This is what T (Transparency) means — strong ops are completely unaware of weak ops.

---

## 5. Testing Plan

### 5.1 Correctness Tests

1. **Strong-only workload**: Verify Raft-HT behaves identically to vanilla Raft when no weak ops are issued.
2. **Weak write durability**: Issue weak write → wait for replication → kill leader → verify new leader has the write.
3. **Weak write loss**: Issue weak write → immediately kill leader before replication → verify system continues correctly (write may be lost, no inconsistency).
4. **C1 verification**: Same session: weak write(x=1) → strong read(x) must return 1 (or later value).
5. **C2 verification**: Same session: weak write(x=1) → strong write(y=2) → after commit, verify x=1 is replicated.
6. **C3 verification**: Session A: weak write(x=1). Session B at same replica: read(x) → strong write(y=x). After commit, verify x=1 is majority-replicated.
7. **Causal set merge**: Verify client correctly returns fresher value between cached and replica responses.

### 5.2 Performance Tests

See evaluation-plan.md Case Study 1:
- Exp 1.1: Throughput vs Latency (varying strong/weak ratio)
- Exp 1.2: Failure Recovery (kill leader, measure throughput dip + recovery)

---

## 6. Implementation Priority and Dependencies

```
Step 0: Vanilla Raft baseline
  ↓
Step 1: LogEntry + Consistency tag
  ↓
Step 2: Client request interface
  ↓
Step 3: Weak write path (leader early reply)  ← core change
  ↓
Step 4: Version tracking in KV store
  ↓
Step 5: Weak read handler (all replicas)
  ↓
Step 6: Client-side causal set + routing
  ↓
Step 7: Testing
```

Steps 1–6 can be implemented in ~1–2 days after vanilla Raft is ready.
The vanilla Raft baseline (Step 0) is the main dependency.
