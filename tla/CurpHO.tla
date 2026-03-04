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
\* Actions — Weak Write Path (Phase 56.2)
\* ============================================================================

\* (56.2a) Client issues a weak (causal) write.
\* Broadcasts CausalPropose to ALL replicas and adds to client's write set.
ClientIssueCausalWrite(c) ==
    /\ clientState[c] = Idle
    /\ opsCompleted[c] < MaxOps
    /\ \E k \in Keys, v \in Values :
        LET cmd   == [op |-> Write, key |-> k, val |-> v]
            cmdId == [client |-> c, seq |-> clientSeq[c]]
        IN
        /\ epoch'          = epoch + 1
        /\ clientState'    = [clientState EXCEPT ![c] = Waiting]
        /\ clientOp'       = [clientOp EXCEPT ![c] = cmd]
        /\ clientCon'      = [clientCon EXCEPT ![c] = Weak]
        /\ clientInvEpoch' = [clientInvEpoch EXCEPT ![c] = epoch + 1]
        /\ clientSeq'      = [clientSeq EXCEPT ![c] = @ + 1]
        \* Add to write set (cleared only on commit, not on 1-RTT reply)
        /\ clientWriteSet' = [clientWriteSet EXCEPT ![c] = @ \cup {cmdId}]
        \* Broadcast CausalPropose to ALL replicas (one message per replica)
        /\ messages' = messages \cup
            {[type       |-> "CausalPropose",
              dest       |-> r2,
              client     |-> c,
              seq        |-> clientSeq[c],
              cmd        |-> cmd,
              boundRep   |-> boundReplica[c]] : r2 \in Replicas}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCache, opsCompleted,
                       boundReplica, fastPathResponses, history>>

\* (56.2b) Non-leader replica handles CausalPropose (weak write).
\* Adds to witness pool. If bound replica, sends speculative CausalReply.
HandleCausalProposeFollower(r) ==
    \E m \in messages :
        /\ m.type = "CausalPropose"
        /\ m.dest = r
        /\ role[r] = Follower
        /\ LET k     == m.cmd.key
               cmdId == [client |-> m.client, seq |-> m.seq]
               reply == [type    |-> "CausalReply",
                         client  |-> m.client,
                         seq     |-> m.seq,
                         val     |-> m.cmd.val,
                         replica |-> r]
           IN
           \* Add to witness pool
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> FALSE,
                 op       |-> m.cmd.op,
                 key      |-> k,
                 val      |-> m.cmd.val,
                 cmdId    |-> cmdId]]
           \* Bound replica sends CausalReply; others just consume
           /\ messages' = (messages \ {m}) \cup
                          (IF r = m.boundRep THEN {reply} ELSE {})
        /\ UNCHANGED <<replicaVars, clientVars, historyVars>>

\* (56.2b) Leader handles CausalPropose (weak write).
\* Adds to witness pool, assigns slot, appends to log, sends Accept to
\* followers. If also bound replica, sends CausalReply.
HandleCausalProposeLeader(r) ==
    \E m \in messages :
        /\ m.type = "CausalPropose"
        /\ m.dest = r
        /\ role[r] = Leader
        /\ LET k     == m.cmd.key
               cmdId == [client |-> m.client, seq |-> m.seq]
               newLog == Append(log[r],
                   [cmd         |-> m.cmd,
                    consistency |-> Weak,
                    term        |-> currentTerm[r],
                    client      |-> m.client,
                    seq         |-> m.seq])
               slot == Len(newLog)
               reply == [type    |-> "CausalReply",
                         client  |-> m.client,
                         seq     |-> m.seq,
                         val     |-> m.cmd.val,
                         replica |-> r]
               accepts == {[type  |-> "Accept",
                            from  |-> r,
                            to    |-> f,
                            slot  |-> slot,
                            entry |-> newLog[slot]] : f \in Replicas \ {r}}
           IN
           \* Add to witness pool
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> FALSE,
                 op       |-> m.cmd.op,
                 key      |-> k,
                 val      |-> m.cmd.val,
                 cmdId    |-> cmdId]]
           \* Assign slot and append to log
           /\ log' = [log EXCEPT ![r] = newLog]
           \* Send Accept to followers + CausalReply if bound
           /\ messages' = (messages \ {m}) \cup accepts \cup
                          (IF r = m.boundRep THEN {reply} ELSE {})
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, clientVars, historyVars>>

\* (56.2c) Client handles CausalReply (bound replica's speculative reply).
\* Completes the weak write in 1 RTT. Does NOT remove from write set.
ClientHandleCausalReply(c) ==
    \E m \in messages :
        /\ m.type = "CausalReply"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1  \* Match pending op
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Weak
        /\ clientOp[c].op = Write
        /\ LET k == clientOp[c].key
               v == clientOp[c].val
           IN
           /\ epoch'        = epoch + 1
           /\ clientState'  = [clientState EXCEPT ![c] = Idle]
           /\ clientOp'     = [clientOp EXCEPT ![c] = Nil]
           /\ opsCompleted' = [opsCompleted EXCEPT ![c] = @ + 1]
           \* Cache the written value — use a provisional version.
           \* The real slot hasn't been assigned yet (leader does that async).
           \* We use clientSeq as a proxy version that increases monotonically.
           /\ clientCache'  = [clientCache EXCEPT ![c][k] =
                [val |-> v, ver |-> Max(@.ver, clientSeq[c] - 1)]]
           \* Record in history — slot=0 because leader hasn't assigned slot yet.
           \* The slot will be determined later by leader's async replication.
           \* For invariant checking, weak writes get slot 0 initially.
           \* HybridCompatibilityInv only checks pairs where both slots > 0.
           /\ history' = Append(history,
                [client      |-> c,
                 op          |-> Write,
                 key         |-> k,
                 reqVal      |-> v,
                 retVal      |-> v,
                 consistency |-> Weak,
                 invEpoch    |-> clientInvEpoch[c],
                 retEpoch    |-> epoch + 1,
                 slot        |-> 0,
                 retVer      |-> Max(clientCache[c][k].ver, clientSeq[c] - 1)])
        /\ messages' = messages \ {m}
        \* Write set NOT cleared here — only on SyncReply
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, clientWriteSet, boundReplica,
                       fastPathResponses>>

\* (56.2d) Follower handles Accept message — append entry to log at slot.
HandleAccept(r) ==
    \E m \in messages :
        /\ m.type = "Accept"
        /\ m.to = r
        /\ role[r] = Follower
        \* Append entry to log (may need to extend if slot > current length)
        \* In CURP-HO, Accept messages arrive in slot order from leader.
        \* Guard: only accept if slot = Len(log[r]) + 1 (next expected slot)
        /\ m.slot = Len(log[r]) + 1
        /\ log' = [log EXCEPT ![r] = Append(@, m.entry)]
        \* Send AcceptAck back to leader
        /\ messages' = (messages \ {m}) \cup
           {[type |-> "AcceptAck",
             from |-> r,
             to   |-> m.from,
             slot |-> m.slot]}
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, unsyncedVars, clientVars,
                       historyVars>>

\* (56.2d) Leader handles AcceptAck — advance commit when majority reached.
HandleAcceptAck(r) ==
    \E m \in messages :
        /\ m.type = "AcceptAck"
        /\ m.to = r
        /\ role[r] = Leader
        \* Count replicas that have this slot (including leader)
        /\ LET slot == m.slot
               \* All replicas that have acked up to this slot (including self)
               ackedReplicas == {r} \cup {r2 \in Replicas :
                   \E a \in messages : a.type = "AcceptAck" /\ a.to = r /\ a.slot >= slot}
           IN
           \* Advance commitIndex if majority have this slot
           /\ IF Cardinality(ackedReplicas) >= Majority
              THEN commitIndex' = [commitIndex EXCEPT ![r] = Max(@, slot)]
              ELSE UNCHANGED commitIndex
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<role, currentTerm, log, lastApplied, kvStore, keyVersion,
                       unsyncedVars, clientVars, historyVars>>

\* (56.2d) Leader sends Commit to followers after advancing commitIndex.
SendCommit(r, f) ==
    /\ role[r] = Leader
    /\ r # f
    /\ commitIndex[r] > 0
    \* Only send if follower doesn't already know about this commitIndex
    /\ ~\E m \in messages : m.type = "Commit" /\ m.from = r /\ m.to = f
                            /\ m.commitIndex >= commitIndex[r]
    /\ messages' = messages \cup
       {[type        |-> "Commit",
         from        |-> r,
         to          |-> f,
         commitIndex |-> commitIndex[r]]}
    /\ UNCHANGED <<replicaVars, unsyncedVars, clientVars, historyVars>>

\* (56.2d) Follower handles Commit — advance commitIndex.
HandleCommit(r) ==
    \E m \in messages :
        /\ m.type = "Commit"
        /\ m.to = r
        /\ commitIndex' = [commitIndex EXCEPT ![r] = Max(@, Min(m.commitIndex, Len(log[r])))]
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<role, currentTerm, log, lastApplied, kvStore, keyVersion,
                       unsyncedVars, clientVars, historyVars>>

\* (56.2d) Any replica applies next committed entry to state machine.
\* On apply: update kvStore, keyVersion, and clear unsynced entry for this key.
ApplyEntry(r) ==
    /\ lastApplied[r] < commitIndex[r]
    /\ LET idx     == lastApplied[r] + 1
           entry   == log[r][idx]
           k       == entry.cmd.key
           isWrite == entry.cmd.op = Write
       IN
       /\ lastApplied' = [lastApplied EXCEPT ![r] = idx]
       /\ IF isWrite
          THEN /\ kvStore'    = [kvStore EXCEPT ![r][k] = entry.cmd.val]
               /\ keyVersion' = [keyVersion EXCEPT ![r][k] = idx]
          ELSE UNCHANGED <<kvStore, keyVersion>>
       \* Clear unsynced entry for this key if it matches the committed command
       /\ LET cmdId == [client |-> entry.client, seq |-> entry.seq]
          IN IF unsynced[r][k] # Nil /\ unsynced[r][k].cmdId = cmdId
             THEN unsynced' = [unsynced EXCEPT ![r][k] = Nil]
             ELSE UNCHANGED unsyncedVars
       \* Leader sends SyncReply for strong ops (slow path completion)
       /\ IF role[r] = Leader /\ entry.consistency = Strong
          THEN LET result == IF isWrite THEN entry.cmd.val
                             ELSE kvStore[r][entry.cmd.key]
               IN messages' = messages \cup
                  {[type   |-> "SyncReply",
                    client |-> entry.client,
                    seq    |-> entry.seq,
                    val    |-> result,
                    slot   |-> idx]}
          ELSE UNCHANGED messages
       /\ UNCHANGED <<role, currentTerm, log, commitIndex,
                      clientVars, historyVars>>

\* ============================================================================
\* Actions — Strong Write Path (Phase 56.3)
\* ============================================================================

\* (56.3a) Client issues a strong (linearizable) write.
\* Broadcasts StrongPropose to ALL replicas (for witness fast path).
ClientIssueStrongWrite(c) ==
    /\ clientState[c] = Idle
    /\ opsCompleted[c] < MaxOps
    /\ \E k \in Keys, v \in Values :
        LET cmd == [op |-> Write, key |-> k, val |-> v]
        IN
        /\ epoch'          = epoch + 1
        /\ clientState'    = [clientState EXCEPT ![c] = Waiting]
        /\ clientOp'       = [clientOp EXCEPT ![c] = cmd]
        /\ clientCon'      = [clientCon EXCEPT ![c] = Strong]
        /\ clientInvEpoch' = [clientInvEpoch EXCEPT ![c] = epoch + 1]
        /\ clientSeq'      = [clientSeq EXCEPT ![c] = @ + 1]
        \* Clear fast path responses for this new op
        /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = {}]
        \* Broadcast StrongPropose to ALL replicas
        /\ messages' = messages \cup
            {[type   |-> "StrongPropose",
              dest   |-> r2,
              client |-> c,
              seq    |-> clientSeq[c],
              cmd    |-> cmd] : r2 \in Replicas}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCache, opsCompleted,
                       clientWriteSet, boundReplica, history>>

\* (56.3b) Non-leader handles StrongPropose (strong write) — witness check.
\* Adds strong cmd to witness pool, checks key conflict.
\* Replies with MRecordAck: Ok/Nok + CausalDeps (same-client weak writes).
\* ReadDep is only relevant for strong READS (not writes), so Nil here.
HandleStrongProposeFollower(r) ==
    \E m \in messages :
        /\ m.type = "StrongPropose"
        /\ m.dest = r
        /\ role[r] = Follower
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               \* Check for key conflict: another STRONG write pending on same key
               conflict == /\ unsynced[r][k] # Nil
                           /\ unsynced[r][k].isStrong
               \* Causal deps: same-client weak writes currently in witness pool
               causalDeps == CausalDepsFor(r, m.client)
               ok == ~conflict
           IN
           \* Add strong cmd to witness pool (overwrites any existing weak entry)
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> TRUE,
                 op       |-> m.cmd.op,
                 key      |-> k,
                 val      |-> m.cmd.val,
                 cmdId    |-> cmdId]]
           \* Reply with witness ack (slot=0 for non-leaders, val=Nil for writes)
           /\ messages' = (messages \ {m}) \cup
              {[type       |-> "MRecordAck",
                from       |-> r,
                client     |-> m.client,
                seq        |-> m.seq,
                ok         |-> ok,
                causalDeps |-> causalDeps,
                readDep    |-> Nil,
                slot       |-> 0,
                val        |-> Nil,
                isLeader   |-> FALSE]}
        /\ UNCHANGED <<replicaVars, clientVars, historyVars>>

\* (56.3b) Leader handles StrongPropose (strong write).
\* Appends to log, sends Accept for replication, replies with leader ack.
\* Leader always accepts (it assigns the slot, no conflict concept).
HandleStrongProposeLeader(r) ==
    \E m \in messages :
        /\ m.type = "StrongPropose"
        /\ m.dest = r
        /\ role[r] = Leader
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               newLog == Append(log[r],
                   [cmd         |-> m.cmd,
                    consistency |-> Strong,
                    term        |-> currentTerm[r],
                    client      |-> m.client,
                    seq         |-> m.seq])
               slot == Len(newLog)
               \* Leader sends Accept to followers for replication
               accepts == {[type  |-> "Accept",
                            from  |-> r,
                            to    |-> f,
                            slot  |-> slot,
                            entry |-> newLog[slot]] : f \in Replicas \ {r}}
               \* Leader ack: always ok (leader determines order)
               \* Include slot so client can record it
               leaderAck == [type       |-> "MRecordAck",
                             from       |-> r,
                             client     |-> m.client,
                             seq        |-> m.seq,
                             ok         |-> TRUE,
                             causalDeps |-> {},
                             readDep    |-> Nil,
                             slot       |-> slot,
                             val        |-> Nil,
                             isLeader   |-> TRUE]
           IN
           \* Add strong cmd to witness pool
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> TRUE,
                 op       |-> m.cmd.op,
                 key      |-> k,
                 val      |-> m.cmd.val,
                 cmdId    |-> cmdId]]
           /\ log' = [log EXCEPT ![r] = newLog]
           /\ messages' = (messages \ {m}) \cup accepts \cup {leaderAck}
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, clientVars, historyVars>>

\* (56.3c) Client handles strong write fast path.
\* Collects 3/4 quorum of Ok acks where:
\*   - All Ok (no key conflicts)
\*   - CausalDeps from all witnesses cover client's writeSet
\* On success: complete immediately with leader's slot.
\* On failure: fall through to slow path (ClientHandleStrongWriteSlowPath).
ClientHandleStrongWriteFastPath(c) ==
    \E m \in messages :
        /\ m.type = "MRecordAck"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Strong
        /\ clientOp[c].op = Write
        \* Accumulate this ack
        /\ LET newResponses == fastPathResponses[c] \cup {m}
               \* Follower acks (witnesses, not leader)
               followerOkAcks == {a \in newResponses : a.ok = TRUE /\ ~a.isLeader}
               \* Leader ack (has slot assignment)
               leaderAcks == {a \in newResponses : a.isLeader}
               haveLeader == Cardinality(leaderAcks) > 0
               \* Total acks with ok=TRUE (leader always ok)
               allOkAcks == {a \in newResponses : a.ok = TRUE}
               \* Check if we have super-majority (3/4) of ok acks
               haveQuorum == Cardinality(allOkAcks) >= ThreeQuarters
               \* Check CausalDeps: every entry in client's writeSet must appear
               \* in EVERY follower witness's causalDeps
               causalDepsOk ==
                   \A ws \in clientWriteSet[c] :
                       \A a \in followerOkAcks :
                           ws \in a.causalDeps
               \* Fast path succeeds if: quorum + causal deps covered + have leader slot
               fastPathOk == haveQuorum /\ causalDepsOk /\ haveLeader
           IN
           IF fastPathOk
           THEN
               \* Complete on fast path
               LET leaderAck == CHOOSE a \in leaderAcks : TRUE
                   slot == leaderAck.slot
                   k    == clientOp[c].key
                   v    == clientOp[c].val
               IN
               /\ epoch'          = epoch + 1
               /\ clientState'    = [clientState EXCEPT ![c] = Idle]
               /\ clientOp'       = [clientOp EXCEPT ![c] = Nil]
               /\ opsCompleted'   = [opsCompleted EXCEPT ![c] = @ + 1]
               /\ clientCache'    = [clientCache EXCEPT ![c][k] =
                    [val |-> v, ver |-> Max(@.ver, slot)]]
               \* Clear writeSet entries (strong op commits all prior weak writes)
               /\ clientWriteSet' = [clientWriteSet EXCEPT ![c] = {}]
               /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = {}]
               /\ history' = Append(history,
                    [client      |-> c,
                     op          |-> Write,
                     key         |-> k,
                     reqVal      |-> v,
                     retVal      |-> v,
                     consistency |-> Strong,
                     invEpoch    |-> clientInvEpoch[c],
                     retEpoch    |-> epoch + 1,
                     slot        |-> slot,
                     retVer      |-> Max(clientCache[c][k].ver, slot)])
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                              clientInvEpoch, boundReplica>>
           ELSE
               \* Not enough acks yet or conditions not met — just accumulate
               /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = newResponses]
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientState, clientOp,
                              clientCon, clientSeq, clientCache, clientInvEpoch,
                              opsCompleted, clientWriteSet, boundReplica,
                              historyVars>>

\* (56.3d) Client handles strong write slow path.
\* Falls back when fast path cannot succeed: leader has committed + applied.
\* Leader sends SyncReply after apply (in ApplyEntry).
\* The client completes with the committed result.
ClientHandleStrongWriteSlowPath(c) ==
    \E m \in messages :
        /\ m.type = "SyncReply"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Strong
        /\ clientOp[c].op = Write
        /\ LET k    == clientOp[c].key
               v    == m.val
               slot == m.slot
           IN
           /\ epoch'          = epoch + 1
           /\ clientState'    = [clientState EXCEPT ![c] = Idle]
           /\ clientOp'       = [clientOp EXCEPT ![c] = Nil]
           /\ opsCompleted'   = [opsCompleted EXCEPT ![c] = @ + 1]
           /\ clientCache'    = [clientCache EXCEPT ![c][k] =
                [val |-> v, ver |-> Max(@.ver, slot)]]
           \* Clear writeSet on commit
           /\ clientWriteSet' = [clientWriteSet EXCEPT ![c] = {}]
           /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = {}]
           /\ history' = Append(history,
                [client      |-> c,
                 op          |-> Write,
                 key         |-> k,
                 reqVal      |-> clientOp[c].val,
                 retVal      |-> v,
                 consistency |-> Strong,
                 invEpoch    |-> clientInvEpoch[c],
                 retEpoch    |-> epoch + 1,
                 slot        |-> slot,
                 retVer      |-> Max(clientCache[c][k].ver, slot)])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, boundReplica>>

\* ============================================================================
\* Actions — Strong Read Path (Phase 56.4)
\* ============================================================================

\* (56.4a) Client issues a strong (linearizable) read.
\* Broadcasts StrongReadPropose to ALL replicas (for witness fast path).
ClientIssueStrongRead(c) ==
    /\ clientState[c] = Idle
    /\ opsCompleted[c] < MaxOps
    /\ \E k \in Keys :
        LET cmd == [op |-> Read, key |-> k, val |-> Nil]
        IN
        /\ epoch'          = epoch + 1
        /\ clientState'    = [clientState EXCEPT ![c] = Waiting]
        /\ clientOp'       = [clientOp EXCEPT ![c] = cmd]
        /\ clientCon'      = [clientCon EXCEPT ![c] = Strong]
        /\ clientInvEpoch' = [clientInvEpoch EXCEPT ![c] = epoch + 1]
        /\ clientSeq'      = [clientSeq EXCEPT ![c] = @ + 1]
        \* Clear fast path responses for this new op
        /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = {}]
        \* Broadcast StrongReadPropose to ALL replicas
        /\ messages' = messages \cup
            {[type   |-> "StrongReadPropose",
              dest   |-> r,
              client |-> c,
              seq    |-> clientSeq[c],
              cmd    |-> cmd] : r \in Replicas}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCache, opsCompleted,
                       clientWriteSet, boundReplica, history>>

\* (56.4b) Non-leader handles StrongReadPropose — witness check.
\* Reports Ok/Nok (conflict with strong write on same key) +
\* CausalDeps (same-client weak writes) +
\* ReadDep (weak write on same key, if any — for read consistency check).
HandleStrongReadProposeFollower(r) ==
    \E m \in messages :
        /\ m.type = "StrongReadPropose"
        /\ m.dest = r
        /\ role[r] = Follower
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               \* Check for key conflict: another STRONG entry pending on same key
               conflict == /\ unsynced[r][k] # Nil
                           /\ unsynced[r][k].isStrong
               \* Causal deps: same-client weak writes currently in witness pool
               causalDeps == CausalDepsFor(r, m.client)
               \* ReadDep: weak write on same key (not strong — those cause conflict)
               readDep == UnsyncedWeakWriteCmdId(r, k)
               ok == ~conflict
           IN
           \* Add strong read to witness pool (mark as strong, op=Read)
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> TRUE,
                 op       |-> Read,
                 key      |-> k,
                 val      |-> Nil,
                 cmdId    |-> cmdId]]
           \* Reply with witness ack including ReadDep
           /\ messages' = (messages \ {m}) \cup
              {[type       |-> "MRecordAck",
                from       |-> r,
                client     |-> m.client,
                seq        |-> m.seq,
                ok         |-> ok,
                causalDeps |-> causalDeps,
                readDep    |-> readDep,
                slot       |-> 0,
                val        |-> Nil,
                isLeader   |-> FALSE]}
        /\ UNCHANGED <<replicaVars, clientVars, historyVars>>

\* (56.4b) Leader handles StrongReadPropose.
\* Appends read to log, computes speculative result (including unsynced weak writes),
\* sends Accept for replication, replies with leader ack including result.
HandleStrongReadProposeLeader(r) ==
    \E m \in messages :
        /\ m.type = "StrongReadPropose"
        /\ m.dest = r
        /\ role[r] = Leader
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               newLog == Append(log[r],
                   [cmd         |-> m.cmd,
                    consistency |-> Strong,
                    term        |-> currentTerm[r],
                    client      |-> m.client,
                    seq         |-> m.seq])
               slot == Len(newLog)
               \* Speculative result: sees unsynced weak writes on this key
               specVal == SpeculativeVal(r, k)
               \* Leader sends Accept to followers for replication
               accepts == {[type  |-> "Accept",
                            from  |-> r,
                            to    |-> f,
                            slot  |-> slot,
                            entry |-> newLog[slot]] : f \in Replicas \ {r}}
               \* Leader ack: always ok, includes slot + speculative value
               leaderAck == [type       |-> "MRecordAck",
                             from       |-> r,
                             client     |-> m.client,
                             seq        |-> m.seq,
                             ok         |-> TRUE,
                             causalDeps |-> {},
                             readDep    |-> Nil,
                             slot       |-> slot,
                             val        |-> specVal,
                             isLeader   |-> TRUE]
           IN
           \* Add strong read to witness pool
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> TRUE,
                 op       |-> Read,
                 key      |-> k,
                 val      |-> Nil,
                 cmdId    |-> cmdId]]
           /\ log' = [log EXCEPT ![r] = newLog]
           /\ messages' = (messages \ {m}) \cup accepts \cup {leaderAck}
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, clientVars, historyVars>>

\* (56.4c) Client handles strong read fast path.
\* Requires: 3/4 quorum ok + CausalDeps cover writeSet + ReadDep consistent
\* (all followers report same ReadDep — all nil or all same cmdId).
\* On success: complete with leader's speculative value.
ClientHandleStrongReadFastPath(c) ==
    \E m \in messages :
        /\ m.type = "MRecordAck"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Strong
        /\ clientOp[c].op = Read
        \* Accumulate this ack
        /\ LET newResponses == fastPathResponses[c] \cup {m}
               \* Follower acks (witnesses, not leader)
               followerOkAcks == {a \in newResponses : a.ok = TRUE /\ ~a.isLeader}
               \* Leader ack (has slot + speculative value)
               leaderAcks == {a \in newResponses : a.isLeader}
               haveLeader == Cardinality(leaderAcks) > 0
               \* Total acks with ok=TRUE
               allOkAcks == {a \in newResponses : a.ok = TRUE}
               \* Check super-majority quorum
               haveQuorum == Cardinality(allOkAcks) >= ThreeQuarters
               \* Check CausalDeps coverage
               causalDepsOk ==
                   \A ws \in clientWriteSet[c] :
                       \A a \in followerOkAcks :
                           ws \in a.causalDeps
               \* Check ReadDep consistency: all follower acks must agree
               \* (all Nil or all the same cmdId)
               followerReadDeps == {a.readDep : a \in followerOkAcks}
               readDepOk == Cardinality(followerReadDeps) <= 1
               \* Fast path succeeds if all conditions met
               fastPathOk == haveQuorum /\ causalDepsOk /\ readDepOk /\ haveLeader
           IN
           IF fastPathOk
           THEN
               \* Complete on fast path with leader's speculative value
               LET leaderAck == CHOOSE a \in leaderAcks : TRUE
                   slot == leaderAck.slot
                   k    == clientOp[c].key
                   retVal == leaderAck.val
               IN
               /\ epoch'          = epoch + 1
               /\ clientState'    = [clientState EXCEPT ![c] = Idle]
               /\ clientOp'       = [clientOp EXCEPT ![c] = Nil]
               /\ opsCompleted'   = [opsCompleted EXCEPT ![c] = @ + 1]
               /\ clientCache'    = [clientCache EXCEPT ![c][k] =
                    [val |-> retVal, ver |-> Max(@.ver, slot)]]
               \* Clear writeSet (strong op commits all prior weak writes)
               /\ clientWriteSet' = [clientWriteSet EXCEPT ![c] = {}]
               /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = {}]
               /\ history' = Append(history,
                    [client      |-> c,
                     op          |-> Read,
                     key         |-> k,
                     reqVal      |-> Nil,
                     retVal      |-> retVal,
                     consistency |-> Strong,
                     invEpoch    |-> clientInvEpoch[c],
                     retEpoch    |-> epoch + 1,
                     slot        |-> slot,
                     retVer      |-> Max(clientCache[c][k].ver, slot)])
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                              clientInvEpoch, boundReplica>>
           ELSE
               \* Not enough acks yet — just accumulate
               /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = newResponses]
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientState, clientOp,
                              clientCon, clientSeq, clientCache, clientInvEpoch,
                              opsCompleted, clientWriteSet, boundReplica,
                              historyVars>>

\* (56.4d) Client handles strong read slow path.
\* Completes on SyncReply from leader (after commit+apply).
\* The leader sends the committed result after applying the read.
ClientHandleStrongReadSlowPath(c) ==
    \E m \in messages :
        /\ m.type = "SyncReply"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Strong
        /\ clientOp[c].op = Read
        /\ LET k    == clientOp[c].key
               retVal == m.val
               slot == m.slot
           IN
           /\ epoch'          = epoch + 1
           /\ clientState'    = [clientState EXCEPT ![c] = Idle]
           /\ clientOp'       = [clientOp EXCEPT ![c] = Nil]
           /\ opsCompleted'   = [opsCompleted EXCEPT ![c] = @ + 1]
           /\ clientCache'    = [clientCache EXCEPT ![c][k] =
                [val |-> retVal, ver |-> Max(@.ver, slot)]]
           \* Clear writeSet on commit
           /\ clientWriteSet' = [clientWriteSet EXCEPT ![c] = {}]
           /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = {}]
           /\ history' = Append(history,
                [client      |-> c,
                 op          |-> Read,
                 key         |-> k,
                 reqVal      |-> Nil,
                 retVal      |-> retVal,
                 consistency |-> Strong,
                 invEpoch    |-> clientInvEpoch[c],
                 retEpoch    |-> epoch + 1,
                 slot        |-> slot,
                 retVer      |-> Max(clientCache[c][k].ver, slot)])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, boundReplica>>

\* ============================================================================
\* Placeholder: Weak reads (56.5) to be added
\* ============================================================================

\* ============================================================================
\* Next-State Relation
\* ============================================================================

Next ==
    \* Client actions — weak write
    \/ \E c \in Clients : ClientIssueCausalWrite(c)
    \/ \E c \in Clients : ClientHandleCausalReply(c)
    \* Client actions — strong write
    \/ \E c \in Clients : ClientIssueStrongWrite(c)
    \/ \E c \in Clients : ClientHandleStrongWriteFastPath(c)
    \/ \E c \in Clients : ClientHandleStrongWriteSlowPath(c)
    \* Client actions — strong read
    \/ \E c \in Clients : ClientIssueStrongRead(c)
    \/ \E c \in Clients : ClientHandleStrongReadFastPath(c)
    \/ \E c \in Clients : ClientHandleStrongReadSlowPath(c)
    \* Replica actions — weak write
    \/ \E r \in Replicas : HandleCausalProposeFollower(r)
    \/ \E r \in Replicas : HandleCausalProposeLeader(r)
    \* Replica actions — strong write
    \/ \E r \in Replicas : HandleStrongProposeFollower(r)
    \/ \E r \in Replicas : HandleStrongProposeLeader(r)
    \* Replica actions — strong read
    \/ \E r \in Replicas : HandleStrongReadProposeFollower(r)
    \/ \E r \in Replicas : HandleStrongReadProposeLeader(r)
    \* Replica actions — replication
    \/ \E r \in Replicas : HandleAccept(r)
    \/ \E r \in Replicas : HandleAcceptAck(r)
    \/ \E r, f \in Replicas : SendCommit(r, f)
    \/ \E r \in Replicas : HandleCommit(r)
    \/ \E r \in Replicas : ApplyEntry(r)

Spec == Init /\ [][Next]_vars

\* ============================================================================
\* Safety Invariants (will be populated in phase 56.6)
\* ============================================================================

\* Placeholder — invariants ported from RaftHT in phase 56.6

====
