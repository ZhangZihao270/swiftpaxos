---- MODULE CurpHO ----
\* TLA+ specification of CURP-HO: CURP with Hybrid Consistency (Optimistic)
\*
\* CURP-HO extends CURP with optimistic weak (causal) operations:
\*   - Strong ops: witness-based fast path (3/4 quorum + CausalDeps + ReadDep)
\*                 with slow path fallback (majority quorum)
\*   - Weak writes: broadcast to ALL replicas, 1-RTT reply from bound replica,
\*                  leader async slot assignment + replication
\*   - Weak reads: 1-RTT to bound replica (speculative) + client cache merge
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
\*
\* Version tracking:
\*   A global writeId counter assigns each write a unique, monotonically increasing
\*   version. This enables comparable versions across strong and weak ops for
\*   session guarantee checking. The writeId travels through the protocol:
\*   client -> CausalPropose/StrongPropose -> log entry + unsynced -> kvWriteId.

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
    seq: Nat,
    writeId: Nat        \* globally unique write ID (0 for reads)
]

\* A witness pool entry (unsynced):
\*   - isStrong: whether this is a strong or weak command
\*   - op, key, val: the command details
\*   - cmdId: unique command identifier
\*   - writeId: globally unique write ID (0 for reads)
\* In the implementation, unsynced is per-key; in the model we use per-replica
\* per-key (at most one entry per key per replica for simplicity).
UnsyncedEntryType == [
    isStrong: BOOLEAN,
    op: {Read, Write},
    key: Keys,
    val: AllValues,
    cmdId: CmdIdType,
    writeId: Nat
]

\* A client cache entry: value + writeId of the source write
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
    kvWriteId,      \* kvWriteId[r][k] \in Nat — writeId of committed value

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

    \* --- Global write ID counter ---
    nextWriteId,    \* Nat, starts at 1, incremented on each write issue

    \* --- Network ---
    messages,       \* Set of in-flight messages

    \* --- History (auxiliary variables for property checking) ---
    history,        \* Sequence of completed operation records
    epoch           \* Monotonic counter for real-time ordering

\* ============================================================================
\* Variable Tuples
\* ============================================================================

replicaVars == <<role, currentTerm, log, commitIndex, lastApplied,
                  kvStore, keyVersion, kvWriteId>>

unsyncedVars == <<unsynced>>

clientVars  == <<clientState, clientOp, clientCon, clientSeq,
                  clientCache, clientInvEpoch, opsCompleted,
                  clientWriteSet, boundReplica, fastPathResponses>>

networkVars == <<messages>>

historyVars == <<history, epoch>>

vars == <<role, currentTerm, log, commitIndex, lastApplied,
          kvStore, keyVersion, kvWriteId, unsynced,
          clientState, clientOp, clientCon, clientSeq,
          clientCache, clientInvEpoch, opsCompleted,
          clientWriteSet, boundReplica, fastPathResponses,
          nextWriteId, messages, history, epoch>>

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
\* If there's a pending write to k in the witness pool, use its value.
\* Otherwise use the committed kvStore value.
SpeculativeVal(r, k) ==
    IF unsynced[r][k] # Nil /\ unsynced[r][k].op = Write
    THEN unsynced[r][k].val
    ELSE kvStore[r][k]

\* Speculative writeId for key k on replica r:
\* If there's a pending write in unsynced, use its writeId.
\* Otherwise use the committed kvWriteId.
SpeculativeWriteId(r, k) ==
    IF unsynced[r][k] # Nil /\ unsynced[r][k].op = Write
    THEN unsynced[r][k].writeId
    ELSE kvWriteId[r][k]

\* Max writeId of all writes to key k in logSeq[1..maxSlot].
\* Used for strong read retVer: ensures the version reflects ALL writes
\* to the key in the log prefix, not just the latest one (which may have
\* a lower writeId due to concurrent write reordering).
RECURSIVE MaxLogWriteId(_, _, _, _)
MaxLogWriteId(logSeq, k, maxSlot, acc) ==
    IF maxSlot = 0 THEN acc
    ELSE IF /\ logSeq[maxSlot].cmd.op = Write
            /\ logSeq[maxSlot].cmd.key = k
         THEN MaxLogWriteId(logSeq, k, maxSlot - 1, Max(acc, logSeq[maxSlot].writeId))
         ELSE MaxLogWriteId(logSeq, k, maxSlot - 1, acc)

\* Last write value for key k in logSeq[1..idx] (scanning backward).
\* Returns Nil if no write to key k exists in the prefix.
\* Used by the leader for strong read speculative result.
RECURSIVE LastLogWriteVal(_, _, _)
LastLogWriteVal(logSeq, k, idx) ==
    IF idx = 0 THEN Nil
    ELSE IF logSeq[idx].cmd.op = Write /\ logSeq[idx].cmd.key = k
         THEN logSeq[idx].cmd.val
         ELSE LastLogWriteVal(logSeq, k, idx - 1)

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
    /\ kvWriteId   = [r \in Replicas |-> [k \in Keys |-> 0]]
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
    /\ boundReplica \in [Clients -> Replicas \ {InitLeader}]
    /\ fastPathResponses = [c \in Clients |-> {}]
    \* Global write ID counter
    /\ nextWriteId = 1
    \* Network + history
    /\ messages    = {}
    /\ history     = <<>>
    /\ epoch       = 0

\* ============================================================================
\* Actions — Weak Write Path
\* ============================================================================

\* Client issues a weak (causal) write.
\* Broadcasts CausalPropose to ALL replicas and adds to client's write set.
\* Assigns a globally unique writeId for version tracking.
ClientIssueCausalWrite(c) ==
    /\ clientState[c] = Idle
    /\ opsCompleted[c] < MaxOps
    /\ \E k \in Keys, v \in Values :
        LET cmd   == [op |-> Write, key |-> k, val |-> v]
            cmdId == [client |-> c, seq |-> clientSeq[c]]
            wid   == nextWriteId
        IN
        /\ epoch'          = epoch + 1
        /\ nextWriteId'    = nextWriteId + 1
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
              writeId    |-> wid,
              boundRep   |-> boundReplica[c]] : r2 \in Replicas}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCache, opsCompleted,
                       boundReplica, fastPathResponses, history>>

\* Non-leader replica handles CausalPropose (weak write).
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
                         writeId |-> m.writeId,
                         replica |-> r]
           IN
           \* Add to witness pool
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> FALSE,
                 op       |-> m.cmd.op,
                 key      |-> k,
                 val      |-> m.cmd.val,
                 cmdId    |-> cmdId,
                 writeId  |-> m.writeId]]
           \* Bound replica sends CausalReply; others just consume
           /\ messages' = (messages \ {m}) \cup
                          (IF r = m.boundRep THEN {reply} ELSE {})
        /\ UNCHANGED <<replicaVars, clientVars, nextWriteId, historyVars>>

\* Leader handles CausalPropose (weak write).
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
                    seq         |-> m.seq,
                    writeId     |-> m.writeId])
               slot == Len(newLog)
               reply == [type    |-> "CausalReply",
                         client  |-> m.client,
                         seq     |-> m.seq,
                         val     |-> m.cmd.val,
                         writeId |-> m.writeId,
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
                 cmdId    |-> cmdId,
                 writeId  |-> m.writeId]]
           \* Assign slot and append to log
           /\ log' = [log EXCEPT ![r] = newLog]
           \* Send Accept to followers + CausalReply if bound
           /\ messages' = (messages \ {m}) \cup accepts \cup
                          (IF r = m.boundRep THEN {reply} ELSE {})
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, kvWriteId,
                       clientVars, nextWriteId, historyVars>>

\* Client handles CausalReply (bound replica's speculative reply).
\* Completes the weak write in 1 RTT. Does NOT remove from write set.
\* Caches the written value with its writeId for future cache merge.
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
           \* Cache the written value with its globally unique writeId
           /\ clientCache'  = [clientCache EXCEPT ![c][k] =
                [val |-> v, ver |-> Max(@.ver, m.writeId)]]
           \* Record in history with writeId as retVer
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
                 retVer      |-> m.writeId])
        /\ messages' = messages \ {m}
        \* Write set NOT cleared here — only on SyncReply
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, clientWriteSet, boundReplica,
                       fastPathResponses, nextWriteId>>

\* Follower handles Accept message — append entry to log at slot.
HandleAccept(r) ==
    \E m \in messages :
        /\ m.type = "Accept"
        /\ m.to = r
        /\ role[r] = Follower
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
                       kvStore, keyVersion, kvWriteId,
                       unsyncedVars, clientVars, nextWriteId, historyVars>>

\* Leader handles AcceptAck — advance commit when majority reached.
HandleAcceptAck(r) ==
    \E m \in messages :
        /\ m.type = "AcceptAck"
        /\ m.to = r
        /\ role[r] = Leader
        \* Count replicas that have this slot (including leader)
        /\ LET slot == m.slot
               ackedReplicas == {r} \cup {r2 \in Replicas :
                   \E a \in messages : a.type = "AcceptAck" /\ a.to = r /\ a.slot >= slot}
           IN
           /\ IF Cardinality(ackedReplicas) >= Majority
              THEN commitIndex' = [commitIndex EXCEPT ![r] = Max(@, slot)]
              ELSE UNCHANGED commitIndex
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<role, currentTerm, log, lastApplied, kvStore, keyVersion,
                       kvWriteId, unsyncedVars, clientVars, nextWriteId, historyVars>>

\* Leader sends Commit to followers after advancing commitIndex.
SendCommit(r, f) ==
    /\ role[r] = Leader
    /\ r # f
    /\ commitIndex[r] > 0
    /\ ~\E m \in messages : m.type = "Commit" /\ m.from = r /\ m.to = f
                            /\ m.commitIndex >= commitIndex[r]
    /\ messages' = messages \cup
       {[type        |-> "Commit",
         from        |-> r,
         to          |-> f,
         commitIndex |-> commitIndex[r]]}
    /\ UNCHANGED <<replicaVars, unsyncedVars, clientVars, nextWriteId, historyVars>>

\* Follower handles Commit — advance commitIndex.
HandleCommit(r) ==
    \E m \in messages :
        /\ m.type = "Commit"
        /\ m.to = r
        /\ commitIndex' = [commitIndex EXCEPT ![r] = Max(@, Min(m.commitIndex, Len(log[r])))]
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<role, currentTerm, log, lastApplied, kvStore, keyVersion,
                       kvWriteId, unsyncedVars, clientVars, nextWriteId, historyVars>>

\* Any replica applies next committed entry to state machine.
\* On apply: update kvStore, keyVersion, kvWriteId, and clear unsynced.
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
               /\ kvWriteId'  = [kvWriteId EXCEPT ![r][k] = entry.writeId]
          ELSE UNCHANGED <<kvStore, keyVersion, kvWriteId>>
       \* Clear unsynced entry for this key if it matches the committed command
       /\ LET cmdId == [client |-> entry.client, seq |-> entry.seq]
          IN IF unsynced[r][k] # Nil /\ unsynced[r][k].cmdId = cmdId
             THEN unsynced' = [unsynced EXCEPT ![r][k] = Nil]
             ELSE UNCHANGED unsyncedVars
       \* Leader sends SyncReply for strong ops (slow path completion)
       /\ IF role[r] = Leader /\ entry.consistency = Strong
          THEN LET result == IF isWrite THEN entry.cmd.val
                             ELSE kvStore[r][entry.cmd.key]
                   resultWid == IF isWrite THEN entry.writeId
                                ELSE MaxLogWriteId(log[r], k, idx - 1, 0)
               IN messages' = messages \cup
                  {[type    |-> "SyncReply",
                    client  |-> entry.client,
                    seq     |-> entry.seq,
                    val     |-> result,
                    slot    |-> idx,
                    writeId |-> resultWid]}
          ELSE UNCHANGED messages
       /\ UNCHANGED <<role, currentTerm, log, commitIndex,
                      clientVars, nextWriteId, historyVars>>

\* ============================================================================
\* Actions — Strong Write Path
\* ============================================================================

\* Client issues a strong (linearizable) write.
\* Broadcasts StrongPropose to ALL replicas (for witness fast path).
\* Assigns a globally unique writeId.
ClientIssueStrongWrite(c) ==
    /\ clientState[c] = Idle
    /\ opsCompleted[c] < MaxOps
    /\ \E k \in Keys, v \in Values :
        LET cmd == [op |-> Write, key |-> k, val |-> v]
            wid == nextWriteId
        IN
        /\ epoch'          = epoch + 1
        /\ nextWriteId'    = nextWriteId + 1
        /\ clientState'    = [clientState EXCEPT ![c] = Waiting]
        /\ clientOp'       = [clientOp EXCEPT ![c] = cmd]
        /\ clientCon'      = [clientCon EXCEPT ![c] = Strong]
        /\ clientInvEpoch' = [clientInvEpoch EXCEPT ![c] = epoch + 1]
        /\ clientSeq'      = [clientSeq EXCEPT ![c] = @ + 1]
        \* Clear fast path responses for this new op
        /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = {}]
        \* Broadcast StrongPropose to ALL replicas
        /\ messages' = messages \cup
            {[type    |-> "StrongPropose",
              dest    |-> r2,
              client  |-> c,
              seq     |-> clientSeq[c],
              cmd     |-> cmd,
              writeId |-> wid] : r2 \in Replicas}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCache, opsCompleted,
                       clientWriteSet, boundReplica, history>>

\* Non-leader handles StrongPropose (strong write) — witness check.
HandleStrongProposeFollower(r) ==
    \E m \in messages :
        /\ m.type = "StrongPropose"
        /\ m.dest = r
        /\ role[r] = Follower
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               conflict == /\ unsynced[r][k] # Nil
                           /\ unsynced[r][k].isStrong
               causalDeps == CausalDepsFor(r, m.client)
               ok == ~conflict
           IN
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> TRUE,
                 op       |-> m.cmd.op,
                 key      |-> k,
                 val      |-> m.cmd.val,
                 cmdId    |-> cmdId,
                 writeId  |-> m.writeId]]
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
                writeId    |-> m.writeId,
                isLeader   |-> FALSE]}
        /\ UNCHANGED <<replicaVars, clientVars, nextWriteId, historyVars>>

\* Leader handles StrongPropose (strong write).
\* Causal barrier: must process all pending CausalPropose from this client first.
HandleStrongProposeLeader(r) ==
    \E m \in messages :
        /\ m.type = "StrongPropose"
        /\ m.dest = r
        /\ role[r] = Leader
        \* Barrier: no pending weak writes from this client
        /\ ~\E cp \in messages : cp.type = "CausalPropose" /\ cp.dest = r /\ cp.client = m.client
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               newLog == Append(log[r],
                   [cmd         |-> m.cmd,
                    consistency |-> Strong,
                    term        |-> currentTerm[r],
                    client      |-> m.client,
                    seq         |-> m.seq,
                    writeId     |-> m.writeId])
               slot == Len(newLog)
               accepts == {[type  |-> "Accept",
                            from  |-> r,
                            to    |-> f,
                            slot  |-> slot,
                            entry |-> newLog[slot]] : f \in Replicas \ {r}}
               leaderAck == [type       |-> "MRecordAck",
                             from       |-> r,
                             client     |-> m.client,
                             seq        |-> m.seq,
                             ok         |-> TRUE,
                             causalDeps |-> {},
                             readDep    |-> Nil,
                             slot       |-> slot,
                             val        |-> Nil,
                             writeId    |-> m.writeId,
                             isLeader   |-> TRUE]
           IN
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> TRUE,
                 op       |-> m.cmd.op,
                 key      |-> k,
                 val      |-> m.cmd.val,
                 cmdId    |-> cmdId,
                 writeId  |-> m.writeId]]
           /\ log' = [log EXCEPT ![r] = newLog]
           /\ messages' = (messages \ {m}) \cup accepts \cup {leaderAck}
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, kvWriteId,
                       clientVars, nextWriteId, historyVars>>

\* Client handles strong write fast path.
ClientHandleStrongWriteFastPath(c) ==
    \E m \in messages :
        /\ m.type = "MRecordAck"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Strong
        /\ clientOp[c].op = Write
        /\ LET newResponses == fastPathResponses[c] \cup {m}
               followerOkAcks == {a \in newResponses : a.ok = TRUE /\ ~a.isLeader}
               leaderAcks == {a \in newResponses : a.isLeader}
               haveLeader == Cardinality(leaderAcks) > 0
               allOkAcks == {a \in newResponses : a.ok = TRUE}
               haveQuorum == Cardinality(allOkAcks) >= ThreeQuarters
               causalDepsOk ==
                   \A ws \in clientWriteSet[c] :
                       \A a \in followerOkAcks :
                           ws \in a.causalDeps
               fastPathOk == haveQuorum /\ causalDepsOk /\ haveLeader
           IN
           IF fastPathOk
           THEN
               LET leaderAck == CHOOSE a \in leaderAcks : TRUE
                   slot == leaderAck.slot
                   k    == clientOp[c].key
                   v    == clientOp[c].val
                   wid  == leaderAck.writeId
               IN
               /\ epoch'          = epoch + 1
               /\ clientState'    = [clientState EXCEPT ![c] = Idle]
               /\ clientOp'       = [clientOp EXCEPT ![c] = Nil]
               /\ opsCompleted'   = [opsCompleted EXCEPT ![c] = @ + 1]
               /\ clientCache'    = [clientCache EXCEPT ![c][k] =
                    [val |-> v, ver |-> Max(@.ver, wid)]]
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
                     retVer      |-> wid])
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                              clientInvEpoch, boundReplica, nextWriteId>>
           ELSE
               /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = newResponses]
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientState, clientOp,
                              clientCon, clientSeq, clientCache, clientInvEpoch,
                              opsCompleted, clientWriteSet, boundReplica,
                              nextWriteId, historyVars>>

\* Client handles strong write slow path.
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
               wid  == m.writeId
           IN
           /\ epoch'          = epoch + 1
           /\ clientState'    = [clientState EXCEPT ![c] = Idle]
           /\ clientOp'       = [clientOp EXCEPT ![c] = Nil]
           /\ opsCompleted'   = [opsCompleted EXCEPT ![c] = @ + 1]
           /\ clientCache'    = [clientCache EXCEPT ![c][k] =
                [val |-> v, ver |-> Max(@.ver, wid)]]
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
                 retVer      |-> wid])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, boundReplica, nextWriteId>>

\* ============================================================================
\* Actions — Strong Read Path
\* ============================================================================

\* Client issues a strong (linearizable) read.
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
        /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = {}]
        /\ messages' = messages \cup
            {[type   |-> "StrongReadPropose",
              dest   |-> r,
              client |-> c,
              seq    |-> clientSeq[c],
              cmd    |-> cmd] : r \in Replicas}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCache, opsCompleted,
                       clientWriteSet, boundReplica, nextWriteId, history>>

\* Non-leader handles StrongReadPropose — witness check.
HandleStrongReadProposeFollower(r) ==
    \E m \in messages :
        /\ m.type = "StrongReadPropose"
        /\ m.dest = r
        /\ role[r] = Follower
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               conflict == /\ unsynced[r][k] # Nil
                           /\ unsynced[r][k].isStrong
               causalDeps == CausalDepsFor(r, m.client)
               readDep == UnsyncedWeakWriteCmdId(r, k)
               ok == ~conflict
           IN
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> TRUE,
                 op       |-> Read,
                 key      |-> k,
                 val      |-> Nil,
                 cmdId    |-> cmdId,
                 writeId  |-> 0]]
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
                writeId    |-> 0,
                isLeader   |-> FALSE]}
        /\ UNCHANGED <<replicaVars, clientVars, nextWriteId, historyVars>>

\* Leader handles StrongReadPropose.
\* Appends read to log, computes speculative result including writeId.
\* Causal barrier: must process all pending CausalPropose from this client first.
HandleStrongReadProposeLeader(r) ==
    \E m \in messages :
        /\ m.type = "StrongReadPropose"
        /\ m.dest = r
        /\ role[r] = Leader
        \* Barrier: no pending weak writes from this client
        /\ ~\E cp \in messages : cp.type = "CausalPropose" /\ cp.dest = r /\ cp.client = m.client
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               newLog == Append(log[r],
                   [cmd         |-> m.cmd,
                    consistency |-> Strong,
                    term        |-> currentTerm[r],
                    client      |-> m.client,
                    seq         |-> m.seq,
                    writeId     |-> 0])
               slot == Len(newLog)
               \* Compute speculative value by replaying the log, NOT from
               \* unsynced (which may have been overwritten by a later strong op).
               \* The leader has the full log so this is always correct.
               specVal == LastLogWriteVal(newLog, k, slot - 1)
               \* Use max writeId of all writes to key k in the log prefix.
               specWid == MaxLogWriteId(newLog, k, slot - 1, 0)
               accepts == {[type  |-> "Accept",
                            from  |-> r,
                            to    |-> f,
                            slot  |-> slot,
                            entry |-> newLog[slot]] : f \in Replicas \ {r}}
               leaderAck == [type       |-> "MRecordAck",
                             from       |-> r,
                             client     |-> m.client,
                             seq        |-> m.seq,
                             ok         |-> TRUE,
                             causalDeps |-> {},
                             readDep    |-> Nil,
                             slot       |-> slot,
                             val        |-> specVal,
                             writeId    |-> specWid,
                             isLeader   |-> TRUE]
           IN
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [isStrong |-> TRUE,
                 op       |-> Read,
                 key      |-> k,
                 val      |-> Nil,
                 cmdId    |-> cmdId,
                 writeId  |-> 0]]
           /\ log' = [log EXCEPT ![r] = newLog]
           /\ messages' = (messages \ {m}) \cup accepts \cup {leaderAck}
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, kvWriteId,
                       clientVars, nextWriteId, historyVars>>

\* Client handles strong read fast path.
ClientHandleStrongReadFastPath(c) ==
    \E m \in messages :
        /\ m.type = "MRecordAck"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Strong
        /\ clientOp[c].op = Read
        /\ LET newResponses == fastPathResponses[c] \cup {m}
               followerOkAcks == {a \in newResponses : a.ok = TRUE /\ ~a.isLeader}
               leaderAcks == {a \in newResponses : a.isLeader}
               haveLeader == Cardinality(leaderAcks) > 0
               allOkAcks == {a \in newResponses : a.ok = TRUE}
               haveQuorum == Cardinality(allOkAcks) >= ThreeQuarters
               causalDepsOk ==
                   \A ws \in clientWriteSet[c] :
                       \A a \in followerOkAcks :
                           ws \in a.causalDeps
               followerReadDeps == {a.readDep : a \in followerOkAcks}
               readDepOk == Cardinality(followerReadDeps) <= 1
               fastPathOk == haveQuorum /\ causalDepsOk /\ readDepOk /\ haveLeader
           IN
           IF fastPathOk
           THEN
               LET leaderAck == CHOOSE a \in leaderAcks : TRUE
                   slot   == leaderAck.slot
                   k      == clientOp[c].key
                   retVal == leaderAck.val
                   srcWid == leaderAck.writeId
               IN
               /\ epoch'          = epoch + 1
               /\ clientState'    = [clientState EXCEPT ![c] = Idle]
               /\ clientOp'       = [clientOp EXCEPT ![c] = Nil]
               /\ opsCompleted'   = [opsCompleted EXCEPT ![c] = @ + 1]
               /\ clientCache'    = [clientCache EXCEPT ![c][k] =
                    [val |-> retVal, ver |-> Max(@.ver, srcWid)]]
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
                     retVer      |-> srcWid])
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                              clientInvEpoch, boundReplica, nextWriteId>>
           ELSE
               /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = newResponses]
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientState, clientOp,
                              clientCon, clientSeq, clientCache, clientInvEpoch,
                              opsCompleted, clientWriteSet, boundReplica,
                              nextWriteId, historyVars>>

\* Client handles strong read slow path.
ClientHandleStrongReadSlowPath(c) ==
    \E m \in messages :
        /\ m.type = "SyncReply"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Strong
        /\ clientOp[c].op = Read
        /\ LET k      == clientOp[c].key
               retVal == m.val
               slot   == m.slot
               srcWid == m.writeId
           IN
           /\ epoch'          = epoch + 1
           /\ clientState'    = [clientState EXCEPT ![c] = Idle]
           /\ clientOp'       = [clientOp EXCEPT ![c] = Nil]
           /\ opsCompleted'   = [opsCompleted EXCEPT ![c] = @ + 1]
           /\ clientCache'    = [clientCache EXCEPT ![c][k] =
                [val |-> retVal, ver |-> Max(@.ver, srcWid)]]
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
                 retVer      |-> srcWid])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, boundReplica, nextWriteId>>

\* ============================================================================
\* Actions — Weak Read Path
\* ============================================================================

\* Client issues a weak (causal) read.
\* Sends WeakRead to bound replica only (1 RTT to nearest replica).
ClientIssueWeakRead(c) ==
    /\ clientState[c] = Idle
    /\ opsCompleted[c] < MaxOps
    /\ \E k \in Keys :
        LET cmd == [op |-> Read, key |-> k, val |-> Nil]
        IN
        /\ epoch'          = epoch + 1
        /\ clientState'    = [clientState EXCEPT ![c] = Waiting]
        /\ clientOp'       = [clientOp EXCEPT ![c] = cmd]
        /\ clientCon'      = [clientCon EXCEPT ![c] = Weak]
        /\ clientInvEpoch' = [clientInvEpoch EXCEPT ![c] = epoch + 1]
        /\ clientSeq'      = [clientSeq EXCEPT ![c] = @ + 1]
        /\ messages' = messages \cup
            {[type   |-> "WeakRead",
              client |-> c,
              seq    |-> clientSeq[c],
              key    |-> k,
              dest   |-> boundReplica[c]]}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCache, opsCompleted,
                       clientWriteSet, boundReplica, fastPathResponses,
                       nextWriteId, history>>

\* Bound replica handles WeakRead.
\* Returns speculative value ONLY if the unsynced entry is the requesting
\* client's own write (for read-your-writes). Other clients' uncommitted
\* writes in the witness pool are not visible — use committed kvStore instead.
HandleWeakRead(r) ==
    \E m \in messages :
        /\ m.type = "WeakRead"
        /\ m.dest = r
        /\ LET k == m.key
               \* Only speculate on the requesting client's own unsynced write
               isOwnWrite == /\ unsynced[r][k] # Nil
                             /\ unsynced[r][k].op = Write
                             /\ unsynced[r][k].cmdId.client = m.client
               retVal == IF isOwnWrite THEN unsynced[r][k].val  ELSE kvStore[r][k]
               retWid == IF isOwnWrite THEN unsynced[r][k].writeId ELSE kvWriteId[r][k]
           IN
           /\ messages' = (messages \ {m}) \cup
              {[type    |-> "WeakReadReply",
                client  |-> m.client,
                seq     |-> m.seq,
                val     |-> retVal,
                writeId |-> retWid]}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientVars,
                       nextWriteId, historyVars>>

\* Client handles WeakReadReply — cache merge (max writeId wins).
\* The final value comes from whichever source has a higher writeId.
\* Use >= to prefer cache on equal writeId (read-your-writes safety).
ClientHandleWeakReadReply(c) ==
    \E m \in messages :
        /\ m.type = "WeakReadReply"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Weak
        /\ clientOp[c].op = Read
        /\ LET k       == clientOp[c].key
               cached  == clientCache[c][k]
               \* Cache merge: higher writeId wins; prefer cache on tie
               useCache == cached.ver >= m.writeId
               finalVal == IF useCache THEN cached.val ELSE m.val
               finalWid == IF useCache THEN cached.ver ELSE m.writeId
           IN
           /\ epoch'        = epoch + 1
           /\ clientState'  = [clientState EXCEPT ![c] = Idle]
           /\ clientOp'     = [clientOp EXCEPT ![c] = Nil]
           /\ opsCompleted' = [opsCompleted EXCEPT ![c] = @ + 1]
           /\ clientCache'  = [clientCache EXCEPT ![c][k] =
                [val |-> finalVal, ver |-> finalWid]]
           /\ history' = Append(history,
                [client      |-> c,
                 op          |-> Read,
                 key         |-> k,
                 reqVal      |-> Nil,
                 retVal      |-> finalVal,
                 consistency |-> Weak,
                 invEpoch    |-> clientInvEpoch[c],
                 retEpoch    |-> epoch + 1,
                 slot        |-> 0,
                 retVer      |-> finalWid])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, clientWriteSet, boundReplica,
                       fastPathResponses, nextWriteId>>

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
    \* Client actions — weak read
    \/ \E c \in Clients : ClientIssueWeakRead(c)
    \/ \E c \in Clients : ClientHandleWeakReadReply(c)
    \* Replica actions — weak read
    \/ \E r \in Replicas : HandleWeakRead(r)
    \* Replica actions — replication
    \/ \E r \in Replicas : HandleAccept(r)
    \/ \E r \in Replicas : HandleAcceptAck(r)
    \/ \E r, f \in Replicas : SendCommit(r, f)
    \/ \E r \in Replicas : HandleCommit(r)
    \/ \E r \in Replicas : ApplyEntry(r)

Spec == Init /\ [][Next]_vars

\* ============================================================================
\* Safety Invariants
\* ============================================================================
\*
\* All invariants use writeId-based retVer for version comparison.
\* This enables checking session guarantees for BOTH strong and weak ops.
\* The writeId namespace is global and monotonically increasing.
\*
\* Weak writes have slot=0 (no log position at completion time) but retVer > 0
\* (they have a globally unique writeId). Strong ops have both slot > 0 and
\* retVer > 0. Reads have retVer = the writeId of the source value (0 if Nil).

\* ============================================================================
\* Linearizability of strong operations
\* ============================================================================

\* (a) Real-time respect among strong ops
RealTimeRespect ==
    LET sOps == StrongOps
    IN \A i, j \in 1..Len(sOps) :
        (sOps[i].retEpoch < sOps[j].invEpoch) => (sOps[i].slot < sOps[j].slot)

\* (b) Read consistency: strong reads return correct values in log slot order.
StrongReadConsistency ==
    \A i \in 1..Len(history) :
        /\ history[i].consistency = Strong
        /\ history[i].op = Read
        =>
        LET k == history[i].key
            s == history[i].slot
        IN \E r \in Replicas :
            /\ Len(log[r]) >= s
            /\ LET RECURSIVE ComputeVal(_, _)
                   ComputeVal(store, idx) ==
                       IF idx >= s THEN store
                       ELSE IF /\ log[r][idx].cmd.op = Write
                               /\ log[r][idx].cmd.key = k
                            THEN ComputeVal(log[r][idx].cmd.val, idx + 1)
                            ELSE ComputeVal(store, idx + 1)
                   expectedVal == ComputeVal(Nil, 1)
               IN history[i].retVal = expectedVal

LinearizabilityInv == RealTimeRespect /\ StrongReadConsistency

\* ============================================================================
\* Causal consistency of all operations
\* ============================================================================
\*
\* With writeId-based retVer, ALL ops (strong and weak) get proper version
\* tracking. The only exception is reads of initial state (retVer=0),
\* which are guarded by retVer > 0 checks.

\* Every read returns either Nil (initial value) or a value that was written.
ReadsReturnValidValues ==
    \A i \in 1..Len(history) :
        history[i].op = Read =>
        \/ history[i].retVal = Nil
        \/ \E r \in Replicas :
            \E idx \in 1..Len(log[r]) :
                /\ log[r][idx].cmd.op = Write
                /\ log[r][idx].cmd.key = history[i].key
                /\ log[r][idx].cmd.val = history[i].retVal
        \* Value was written by some completed write (covers uncommitted weak writes)
        \/ \E j \in 1..i :
            /\ history[j].op = Write
            /\ history[j].key = history[i].key
            /\ history[j].reqVal = history[i].retVal

\* Monotonic reads: within a session, reads of the same key never go backwards.
\* Both reads must have retVer > 0 (i.e., not reading initial state).
MonotonicReads ==
    \A i, j \in 1..Len(history) :
        /\ history[i].op = Read
        /\ history[j].op = Read
        /\ history[i].client = history[j].client
        /\ history[i].key = history[j].key
        /\ i < j
        /\ history[i].retVer > 0
        => history[j].retVer >= history[i].retVer

\* Read-your-writes: after client c writes key k, any subsequent read
\* by c of key k must see a value at least as recent as that write.
\* Uses retVer (writeId) for both writes and reads — comparable namespace.
ReadYourWrites ==
    \A i, j \in 1..Len(history) :
        /\ history[i].op = Write
        /\ history[j].op = Read
        /\ history[i].client = history[j].client
        /\ history[i].key = history[j].key
        /\ i < j
        /\ history[i].retVer > 0
        => history[j].retVer >= history[i].retVer

\* Monotonic writes: same client's writes must get increasing writeIds.
\* Since writeIds are assigned in issue order and clients are sequential,
\* this should always hold.
MonotonicWrites ==
    \A i, j \in 1..Len(history) :
        /\ history[i].op = Write
        /\ history[j].op = Write
        /\ history[i].client = history[j].client
        /\ i < j
        => history[i].retVer < history[j].retVer

\* Writes-follow-reads: if client c reads and gets retVer v (v > 0),
\* then c's next write must have retVer > v.
WritesFollowReads ==
    \A i, j \in 1..Len(history) :
        /\ history[i].op = Read
        /\ history[j].op = Write
        /\ history[i].client = history[j].client
        /\ i < j
        /\ history[i].retVer > 0
        => history[j].retVer > history[i].retVer

CausalConsistencyInv ==
    /\ ReadsReturnValidValues
    /\ MonotonicReads
    /\ ReadYourWrites
    /\ MonotonicWrites
    /\ WritesFollowReads

\* ============================================================================
\* Hybrid compatibility
\* ============================================================================
\* For same-client slotted ops: if slot[i] < slot[j] then i must have been
\* issued before j (i.e., i appears earlier in history).
\* Only checks ops with slot > 0 (weak writes have slot=0 and are excluded).

HybridCompatibilityInv ==
    \A i, j \in 1..Len(history) :
        /\ history[i].slot > 0
        /\ history[j].slot > 0
        /\ history[i].slot < history[j].slot
        /\ history[i].client = history[j].client
        =>
        i < j

\* Combined safety invariant
SafetyInv == LinearizabilityInv /\ CausalConsistencyInv /\ HybridCompatibilityInv

====
