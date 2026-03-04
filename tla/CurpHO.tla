---- MODULE CurpHO ----
\* TLA+ specification of CURP-HO: CURP with Hybrid Consistency (Optimistic)
\*
\* CURP-HO extends CURP with optimistic weak (causal) operations:
\*   - Strong ops: witness-based fast path (3/4 quorum + CausalDeps + ReadDep)
\*                 with slow path fallback (majority quorum)
\*   - Weak writes: broadcast to ALL replicas, 1-RTT reply from bound replica,
\*                  leader async slot assignment + replication
\*   - Weak reads: 1-RTT to bound replica + client cache merge
\*
\* Key differences from Raft-HT:
\*   - Witness pool (unsynced): both strong + weak commands, per-key per-replica
\*   - Fast path requires 3/4 quorum + causal dep coverage + ReadDep consistency
\*   - Weak writes complete before slot assignment (speculative 1-RTT)
\*   - Bound replica: each client bound to closest replica for weak op latency
\*
\* Properties to verify (safety only, same as Raft-HT):
\*   1. Linearizability of strong operations
\*   2. Causal consistency of all operations (full session guarantees)
\*   3. Hybrid compatibility (total order and causal order don't contradict)

EXTENDS Integers, Sequences, FiniteSets, TLC

\* ============================================================================
\* Constants
\* ============================================================================

CONSTANTS
    Replicas,       \* Set of replica IDs, e.g., {r1, r2, r3}
    Clients,        \* Set of client IDs, e.g., {c1, c2}
    Keys,           \* Set of keys, e.g., {k1}
    Values,         \* Set of non-nil values, e.g., {v1, v2}
    MaxOps,         \* Max operations per client (bounds state space)
    Nil,            \* Distinguished null value (not in Values)
    InitLeader      \* Fixed initial leader (enables follower symmetry)

\* ============================================================================
\* Symbolic Constants
\* ============================================================================

\* Operation types
Read  == "Read"
Write == "Write"

\* Consistency levels
Strong == "Strong"
Weak   == "Weak"

\* Replica roles (simplified: no election)
Follower == "Follower"
Leader   == "Leader"

\* Client states
Idle    == "Idle"
Waiting == "Waiting"

\* ============================================================================
\* Type Definitions
\* ============================================================================

\* All possible values including Nil
AllValues == Values \cup {Nil}

\* A command issued by a client
Command == [op: {Read, Write}, key: Keys, val: AllValues]

\* A command identifier: (clientId, seqNum)
\* Used for witness pool entries and dependency tracking.
CmdIdType == [client: Clients, seq: Nat]

\* A log entry in a replica's log
LogEntryType == [
    cmd: Command,
    consistency: {Strong, Weak},
    term: Nat,
    client: Clients,
    seq: Nat
]

\* A witness pool entry (unsynced):
\*   - isStrong: whether this is a strong or weak command
\*   - op, key, val: the command details
\*   - cmdId: unique command identifier
\* In the implementation, unsynced is per-key; in the model we use per-replica
\* per-key (at most one entry per key per replica for simplicity).
UnsyncedEntryType == [
    isStrong: BOOLEAN,
    op: {Read, Write},
    key: Keys,
    val: AllValues,
    cmdId: CmdIdType
]

\* A client cache entry: value + version (slot of source write)
CacheEntryType == [val: AllValues, ver: Nat]

\* ============================================================================
\* Variables
\* ============================================================================

VARIABLES
    \* --- Replica state (functions indexed by Replicas) ---
    role,           \* role[r] \in {Follower, Leader}
    currentTerm,    \* currentTerm[r] \in Nat
    log,            \* log[r] \in Seq(LogEntryType)
    commitIndex,    \* commitIndex[r] \in Nat
    lastApplied,    \* lastApplied[r] \in Nat
    kvStore,        \* kvStore[r] \in [Keys -> AllValues]
    keyVersion,     \* keyVersion[r] \in [Keys -> Nat]

    \* --- Witness pool (per-replica, per-key) ---
    \* unsynced[r][k] \in UnsyncedEntryType \cup {Nil}
    \* At most one unsynced entry per key per replica.
    \* Cleared when the command is committed via the log.
    unsynced,

    \* --- Client state (functions indexed by Clients) ---
    clientState,    \* clientState[c] \in {Idle, Waiting}
    clientOp,       \* clientOp[c] \in Command \cup {Nil}
    clientCon,      \* clientCon[c] \in {Strong, Weak}
    clientSeq,      \* clientSeq[c] \in Nat (next sequence number)
    clientCache,    \* clientCache[c] \in [Keys -> CacheEntryType]
    clientInvEpoch, \* clientInvEpoch[c] \in Nat
    opsCompleted,   \* opsCompleted[c] \in Nat

    \* --- CURP-HO specific client state ---
    \* clientWriteSet[c] \in SUBSET CmdIdType
    \* Tracks uncommitted weak writes by this client. Used for CausalDeps
    \* checking on strong op fast path. Cleared on SyncReply (leader commit),
    \* NOT on bound replica's 1-RTT reply.
    clientWriteSet,

    \* boundReplica[c] \in Replicas
    \* The replica this client is bound to for weak op latency (closest).
    \* Fixed at init for determinism.
    boundReplica,

    \* --- Strong op fast path state ---
    \* fastPathResponses[c] \in set of response records from witnesses
    \* Accumulates MRecordAck messages for the current pending strong op.
    \* Cleared when the op completes (fast or slow path).
    fastPathResponses,

    \* --- Network ---
    messages,       \* Set of in-flight messages

    \* --- History (auxiliary variables for property checking) ---
    history,        \* Sequence of completed operation records
    epoch           \* Monotonic counter for real-time ordering

\* ============================================================================
\* Variable Tuples
\* ============================================================================

replicaVars == <<role, currentTerm, log, commitIndex, lastApplied,
                  kvStore, keyVersion>>

unsyncedVars == <<unsynced>>

clientVars  == <<clientState, clientOp, clientCon, clientSeq,
                  clientCache, clientInvEpoch, opsCompleted,
                  clientWriteSet, boundReplica, fastPathResponses>>

networkVars == <<messages>>

historyVars == <<history, epoch>>

vars == <<role, currentTerm, log, commitIndex, lastApplied,
          kvStore, keyVersion, unsynced,
          clientState, clientOp, clientCon, clientSeq,
          clientCache, clientInvEpoch, opsCompleted,
          clientWriteSet, boundReplica, fastPathResponses,
          messages, history, epoch>>

\* ============================================================================
\* Helpers
\* ============================================================================

\* The set of all replicas currently serving as leader
Leaders == {r \in Replicas : role[r] = Leader}

\* Quorum sizes
\* Majority: N/2 + 1 (slow path)
Majority == (Cardinality(Replicas) \div 2) + 1

\* Three-quarters quorum: 3N/4 + 1 (fast path)
\* For N=3: (3*3)/4 + 1 = 3 (all replicas)
\* For N=5: (3*5)/4 + 1 = 4
ThreeQuarters == ((3 * Cardinality(Replicas)) \div 4) + 1

\* Last log index for replica r (0 if empty)
LastLogIndex(r) == Len(log[r])

\* Min of two naturals
Min(a, b) == IF a < b THEN a ELSE b

\* Max of two naturals
Max(a, b) == IF a > b THEN a ELSE b

\* Maximum of a non-empty set of naturals
SetMax(S) == CHOOSE x \in S : \A y \in S : y <= x

\* Send a message (add to message bag)
Send(m) == messages' = messages \cup {m}

\* Send a set of messages
SendAll(ms) == messages' = messages \cup ms

\* Receive a message (remove from message bag)
Receive(m) == messages' = messages \ {m}

\* Send one message while receiving another
SendAndReceive(send, recv) == messages' = (messages \ {recv}) \cup {send}

\* Get the value for a key from a witness pool entry (or Nil if no entry)
UnsyncedVal(r, k) ==
    IF unsynced[r][k] # Nil /\ unsynced[r][k].op = Write
    THEN unsynced[r][k].val
    ELSE Nil

\* Get the cmdId of a weak write in the witness pool for a key (or Nil)
\* Used for ReadDep reporting.
UnsyncedWeakWriteCmdId(r, k) ==
    IF /\ unsynced[r][k] # Nil
       /\ unsynced[r][k].op = Write
       /\ ~unsynced[r][k].isStrong
    THEN unsynced[r][k].cmdId
    ELSE Nil

\* Get all CausalDeps for a client from a replica's witness pool:
\* the set of cmdIds of weak writes by this client in the pool.
CausalDepsFor(r, c) ==
    {unsynced[r][k].cmdId : k \in {k2 \in Keys :
        /\ unsynced[r][k2] # Nil
        /\ ~unsynced[r][k2].isStrong
        /\ unsynced[r][k2].cmdId.client = c}}

\* Compute speculative read result for key k on replica r:
\* If there's a pending weak write to k in the witness pool, use its value.
\* Otherwise use the committed kvStore value.
SpeculativeVal(r, k) ==
    IF unsynced[r][k] # Nil /\ unsynced[r][k].op = Write
    THEN unsynced[r][k].val
    ELSE kvStore[r][k]

\* Speculative version for key k on replica r:
\* If there's a pending write in unsynced, we don't have a slot yet,
\* so use keyVersion + 1 as a placeholder. Otherwise use keyVersion.
\* (In practice, the client cache uses max-version merge.)
SpeculativeVer(r, k) ==
    IF unsynced[r][k] # Nil /\ unsynced[r][k].op = Write
    THEN keyVersion[r][k] + 1
    ELSE keyVersion[r][k]

\* Filter history to strong operations only (for LinearizabilityInv)
StrongOps ==
    LET RECURSIVE Filter(_, _)
        Filter(seq, i) ==
            IF i > Len(seq) THEN <<>>
            ELSE IF seq[i].consistency = Strong
                 THEN <<seq[i]>> \o Filter(seq, i + 1)
                 ELSE Filter(seq, i + 1)
    IN Filter(history, 1)

\* ============================================================================
\* Initial State
\* ============================================================================

Init ==
    \* Fixed leader (deterministic, enables follower symmetry reduction)
    /\ role        = [r \in Replicas |-> IF r = InitLeader THEN Leader ELSE Follower]
    /\ currentTerm = [r \in Replicas |-> 1]
    /\ log         = [r \in Replicas |-> <<>>]
    /\ commitIndex = [r \in Replicas |-> 0]
    /\ lastApplied = [r \in Replicas |-> 0]
    /\ kvStore     = [r \in Replicas |-> [k \in Keys |-> Nil]]
    /\ keyVersion  = [r \in Replicas |-> [k \in Keys |-> 0]]
    \* Witness pool: empty (Nil per key per replica)
    /\ unsynced    = [r \in Replicas |-> [k \in Keys |-> Nil]]
    \* Client state
    /\ clientState = [c \in Clients |-> Idle]
    /\ clientOp    = [c \in Clients |-> Nil]
    /\ clientCon   = [c \in Clients |-> Strong]
    /\ clientSeq   = [c \in Clients |-> 1]
    /\ clientCache = [c \in Clients |-> [k \in Keys |-> [val |-> Nil, ver |-> 0]]]
    /\ clientInvEpoch = [c \in Clients |-> 0]
    /\ opsCompleted = [c \in Clients |-> 0]
    \* CURP-HO specific: empty write set, deterministic bound replica
    /\ clientWriteSet = [c \in Clients |-> {}]
    \* Bind each client to a non-leader replica for weak op latency.
    \* In practice, this is the closest replica. For model checking, we assign
    \* deterministically to followers (non-leader replicas). This exercises the
    \* key case where bound replica != leader (speculative reply before slot
    \* assignment). Binding to leader would degenerate to Raft-HT-like behavior.
    \* MC_CurpHO overrides this with concrete bindings.
    /\ boundReplica \in [Clients -> Replicas \ {InitLeader}]
    /\ fastPathResponses = [c \in Clients |-> {}]
    \* Network + history
    /\ messages    = {}
    /\ history     = <<>>
    /\ epoch       = 0

\* ============================================================================
\* Placeholder: Actions will be added in subsequent phases (56.2-56.5)
\* ============================================================================

\* Next-state relation (stub — will be populated in phases 56.2-56.5)
Next == FALSE

Spec == Init /\ [][Next]_vars

\* ============================================================================
\* Safety Invariants (will be populated in phase 56.6)
\* ============================================================================

\* Placeholder — invariants ported from RaftHT in phase 56.6

====
