---- MODULE CurpHT ----
\* TLA+ specification of CURP-HT: CURP with Hybrid Consistency (Transparent)
\*
\* CURP-HT extends CURP with transparent weak (causal) operations:
\*   - Strong ops: witness-based fast path (3/4 quorum, no CausalDeps/ReadDep)
\*                 with slow path fallback (majority quorum)
\*   - Weak writes: leader only, 2 RTT (reply after commit+apply)
\*   - Weak reads: 1 RTT to bound replica + client cache merge
\*
\* Key differences from CURP-HO:
\*   - Witness pool (unsynced): only strong commands (transparency)
\*   - No CausalDeps/ReadDep tracking — non-leaders unaware of weak ops
\*   - Weak writes fully committed before reply → all ops have real slots
\*   - No clientWriteSet needed (weak writes committed before next op)
\*
\* Properties to verify (safety only, same as Raft-HT and CURP-HO):
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
CmdIdType == [client: Clients, seq: Nat]

\* A log entry in a replica's log
LogEntryType == [
    cmd: Command,
    consistency: {Strong, Weak},
    term: Nat,
    client: Clients,
    seq: Nat
]

\* A witness pool entry (unsynced) — ONLY strong commands in CURP-HT:
\*   - op, key, val: the command details
\*   - cmdId: unique command identifier
\* Weak commands never enter the witness pool (transparency).
UnsyncedEntryType == [
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

    \* --- Witness pool (per-replica, per-key) --- strong commands only
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

    \* --- CURP-HT specific client state ---
    \* boundReplica[c] \in Replicas
    \* The replica this client is bound to for weak read latency (closest).
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
                  boundReplica, fastPathResponses>>

networkVars == <<messages>>

historyVars == <<history, epoch>>

vars == <<role, currentTerm, log, commitIndex, lastApplied,
          kvStore, keyVersion, unsynced,
          clientState, clientOp, clientCon, clientSeq,
          clientCache, clientInvEpoch, opsCompleted,
          boundReplica, fastPathResponses,
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

\* Send a message (add to message bag)
Send(m) == messages' = messages \cup {m}

\* Send a set of messages
SendAll(ms) == messages' = messages \cup ms

\* Receive a message (remove from message bag)
Receive(m) == messages' = messages \ {m}

\* Send one message while receiving another
SendAndReceive(send, recv) == messages' = (messages \ {recv}) \cup {send}

\* Compute speculative read result for key k on replica r by scanning the log.
\* The leader's log contains all entries (including unapplied ones).
\* Returns the value of the latest write to k, or kvStore[r][k] if none.
SpeculativeVal(r, k) ==
    LET RECURSIVE ScanLog(_, _)
        ScanLog(idx, val) ==
            IF idx > Len(log[r]) THEN val
            ELSE IF /\ log[r][idx].cmd.op = Write
                    /\ log[r][idx].cmd.key = k
                 THEN ScanLog(idx + 1, log[r][idx].cmd.val)
                 ELSE ScanLog(idx + 1, val)
    IN ScanLog(1, Nil)

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
    \* Witness pool: empty (Nil per key per replica) — only strong commands
    /\ unsynced    = [r \in Replicas |-> [k \in Keys |-> Nil]]
    \* Client state
    /\ clientState = [c \in Clients |-> Idle]
    /\ clientOp    = [c \in Clients |-> Nil]
    /\ clientCon   = [c \in Clients |-> Strong]
    /\ clientSeq   = [c \in Clients |-> 1]
    /\ clientCache = [c \in Clients |-> [k \in Keys |-> [val |-> Nil, ver |-> 0]]]
    /\ clientInvEpoch = [c \in Clients |-> 0]
    /\ opsCompleted = [c \in Clients |-> 0]
    \* Bind each client to a non-leader replica for weak read latency.
    /\ boundReplica \in [Clients -> Replicas \ {InitLeader}]
    /\ fastPathResponses = [c \in Clients |-> {}]
    \* Network + history
    /\ messages    = {}
    /\ history     = <<>>
    /\ epoch       = 0

\* ============================================================================
\* Actions — Weak Write Path (leader only, 2 RTT)
\* ============================================================================

\* Client issues a weak (causal) write.
\* Sends WeakPropose to leader only (not broadcast — transparency).
ClientIssueWeakWrite(c) ==
    /\ clientState[c] = Idle
    /\ opsCompleted[c] < MaxOps
    /\ \E k \in Keys, v \in Values :
        LET cmd == [op |-> Write, key |-> k, val |-> v]
        IN
        /\ epoch'          = epoch + 1
        /\ clientState'    = [clientState EXCEPT ![c] = Waiting]
        /\ clientOp'       = [clientOp EXCEPT ![c] = cmd]
        /\ clientCon'      = [clientCon EXCEPT ![c] = Weak]
        /\ clientInvEpoch' = [clientInvEpoch EXCEPT ![c] = epoch + 1]
        /\ clientSeq'      = [clientSeq EXCEPT ![c] = @ + 1]
        \* Send WeakPropose to leader only
        /\ messages' = messages \cup
            {[type   |-> "WeakPropose",
              client |-> c,
              seq    |-> clientSeq[c],
              cmd    |-> cmd]}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCache, opsCompleted,
                       boundReplica, fastPathResponses, history>>

\* Leader handles WeakPropose (weak write).
\* Assigns slot, appends to log, sends Accept to followers.
\* Does NOT reply yet — waits for commit+apply (2 RTT).
\* Weak writes do NOT enter the witness pool (transparency).
HandleWeakPropose(r) ==
    \E m \in messages :
        /\ m.type = "WeakPropose"
        /\ role[r] = Leader
        /\ LET k      == m.cmd.key
               newLog  == Append(log[r],
                   [cmd         |-> m.cmd,
                    consistency |-> Weak,
                    term        |-> currentTerm[r],
                    client      |-> m.client,
                    seq         |-> m.seq])
               slot    == Len(newLog)
               accepts == {[type  |-> "Accept",
                            from  |-> r,
                            to    |-> f,
                            slot  |-> slot,
                            entry |-> newLog[slot]] : f \in Replicas \ {r}}
           IN
           /\ log' = [log EXCEPT ![r] = newLog]
           \* Send Accept to followers, consume propose (no reply yet)
           /\ messages' = (messages \ {m}) \cup accepts
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, unsyncedVars, clientVars,
                       historyVars>>

\* Client handles WeakWriteReply (sent by leader after commit+apply).
\* Completes the weak write with a real slot number.
ClientHandleWeakWriteReply(c) ==
    \E m \in messages :
        /\ m.type = "WeakWriteReply"
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
           \* Cache the written value with its real slot as version
           /\ clientCache'  = [clientCache EXCEPT ![c][k] =
                [val |-> v, ver |-> Max(@.ver, m.slot)]]
           \* Record in history with real slot (all versions are real in CurpHT)
           /\ history' = Append(history,
                [client      |-> c,
                 op          |-> Write,
                 key         |-> k,
                 reqVal      |-> v,
                 retVal      |-> v,
                 consistency |-> Weak,
                 invEpoch    |-> clientInvEpoch[c],
                 retEpoch    |-> epoch + 1,
                 slot        |-> m.slot,
                 retVer      |-> m.slot])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, boundReplica, fastPathResponses>>

\* ============================================================================
\* Actions — Replication (Accept/Commit/Apply)
\* ============================================================================

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
                       kvStore, keyVersion, unsyncedVars, clientVars,
                       historyVars>>

\* Leader handles AcceptAck — advance commit when majority reached.
HandleAcceptAck(r) ==
    \E m \in messages :
        /\ m.type = "AcceptAck"
        /\ m.to = r
        /\ role[r] = Leader
        /\ LET slot == m.slot
               \* All replicas that have acked up to this slot (including self)
               ackedReplicas == {r} \cup {r2 \in Replicas :
                   \E a \in messages : a.type = "AcceptAck" /\ a.to = r /\ a.slot >= slot}
           IN
           /\ IF Cardinality(ackedReplicas) >= Majority
              THEN commitIndex' = [commitIndex EXCEPT ![r] = Max(@, slot)]
              ELSE UNCHANGED commitIndex
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<role, currentTerm, log, lastApplied, kvStore, keyVersion,
                       unsyncedVars, clientVars, historyVars>>

\* Leader sends Commit to followers after advancing commitIndex.
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

\* Follower handles Commit — advance commitIndex.
HandleCommit(r) ==
    \E m \in messages :
        /\ m.type = "Commit"
        /\ m.to = r
        /\ commitIndex' = [commitIndex EXCEPT ![r] = Max(@, Min(m.commitIndex, Len(log[r])))]
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<role, currentTerm, log, lastApplied, kvStore, keyVersion,
                       unsyncedVars, clientVars, historyVars>>

\* Any replica applies next committed entry to state machine.
\* On apply: update kvStore, keyVersion, clear matching unsynced entry.
\* Leader sends SyncReply for strong ops (slow path completion).
\* Leader sends WeakWriteReply for weak writes (2 RTT completion).
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
       \* Leader sends replies after apply
       /\ IF role[r] = Leader
          THEN LET result == IF isWrite THEN entry.cmd.val
                             ELSE kvStore[r][entry.cmd.key]
               IN
               IF entry.consistency = Strong
               THEN \* Strong ops: SyncReply (slow path completion)
                    messages' = messages \cup
                       {[type   |-> "SyncReply",
                         client |-> entry.client,
                         seq    |-> entry.seq,
                         val    |-> result,
                         slot   |-> idx]}
               ELSE \* Weak writes: WeakWriteReply (2 RTT completion)
                    messages' = messages \cup
                       {[type   |-> "WeakWriteReply",
                         client |-> entry.client,
                         seq    |-> entry.seq,
                         slot   |-> idx]}
          ELSE UNCHANGED messages
       /\ UNCHANGED <<role, currentTerm, log, commitIndex,
                      clientVars, historyVars>>

\* ============================================================================
\* Actions — Strong Write Path (fast path + slow path)
\* ============================================================================

\* Client issues a strong (linearizable) write.
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
                       boundReplica, history>>

\* Non-leader handles StrongPropose (strong write) — witness check.
\* Adds strong cmd to witness pool, checks key conflict (strong entries only).
\* No CausalDeps or ReadDep (transparency — non-leaders unaware of weak ops).
HandleStrongProposeFollower(r) ==
    \E m \in messages :
        /\ m.type = "StrongPropose"
        /\ m.dest = r
        /\ role[r] = Follower
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               \* Check for key conflict: another strong entry pending on same key
               conflict == unsynced[r][k] # Nil
               ok == ~conflict
           IN
           \* Add strong cmd to witness pool
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [op    |-> m.cmd.op,
                 key   |-> k,
                 val   |-> m.cmd.val,
                 cmdId |-> cmdId]]
           \* Reply with witness ack (no CausalDeps, no ReadDep)
           /\ messages' = (messages \ {m}) \cup
              {[type     |-> "MRecordAck",
                from     |-> r,
                client   |-> m.client,
                seq      |-> m.seq,
                ok       |-> ok,
                slot     |-> 0,
                val      |-> Nil,
                isLeader |-> FALSE]}
        /\ UNCHANGED <<replicaVars, clientVars, historyVars>>

\* Leader handles StrongPropose (strong write).
\* Appends to log, sends Accept for replication, replies with leader ack.
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
               leaderAck == [type     |-> "MRecordAck",
                             from     |-> r,
                             client   |-> m.client,
                             seq      |-> m.seq,
                             ok       |-> TRUE,
                             slot     |-> slot,
                             val      |-> Nil,
                             isLeader |-> TRUE]
           IN
           \* Add strong cmd to witness pool
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [op    |-> m.cmd.op,
                 key   |-> k,
                 val   |-> m.cmd.val,
                 cmdId |-> cmdId]]
           /\ log' = [log EXCEPT ![r] = newLog]
           /\ messages' = (messages \ {m}) \cup accepts \cup {leaderAck}
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, clientVars, historyVars>>

\* Client handles strong write fast path.
\* Collects 3/4 quorum of Ok acks + leader slot.
\* No CausalDeps or ReadDep check needed (transparency).
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
               \* Leader ack (has slot assignment)
               leaderAcks == {a \in newResponses : a.isLeader}
               haveLeader == Cardinality(leaderAcks) > 0
               \* Total acks with ok=TRUE
               allOkAcks == {a \in newResponses : a.ok = TRUE}
               \* Check if we have super-majority (3/4) of ok acks
               haveQuorum == Cardinality(allOkAcks) >= ThreeQuarters
               \* Fast path succeeds if: quorum + have leader slot
               fastPathOk == haveQuorum /\ haveLeader
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
                     retVer      |-> slot])
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                              clientInvEpoch, boundReplica>>
           ELSE
               \* Not enough acks yet — just accumulate
               /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = newResponses]
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientState, clientOp,
                              clientCon, clientSeq, clientCache, clientInvEpoch,
                              opsCompleted, boundReplica, historyVars>>

\* Client handles strong write slow path.
\* Leader sends SyncReply after commit+apply.
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
                 retVer      |-> slot])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, boundReplica>>

\* ============================================================================
\* Actions — Strong Read Path (fast path + slow path)
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
                       boundReplica, history>>

\* Non-leader handles StrongReadPropose — witness check.
\* Checks key conflict (strong entries only). No ReadDep (transparency).
HandleStrongReadProposeFollower(r) ==
    \E m \in messages :
        /\ m.type = "StrongReadPropose"
        /\ m.dest = r
        /\ role[r] = Follower
        /\ LET k      == m.cmd.key
               cmdId  == [client |-> m.client, seq |-> m.seq]
               \* Check for key conflict: another strong entry pending on same key
               conflict == unsynced[r][k] # Nil
               ok == ~conflict
           IN
           \* Add strong read to witness pool
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [op    |-> Read,
                 key   |-> k,
                 val   |-> Nil,
                 cmdId |-> cmdId]]
           \* Reply with witness ack (no ReadDep — transparency)
           /\ messages' = (messages \ {m}) \cup
              {[type     |-> "MRecordAck",
                from     |-> r,
                client   |-> m.client,
                seq      |-> m.seq,
                ok       |-> ok,
                slot     |-> 0,
                val      |-> Nil,
                isLeader |-> FALSE]}
        /\ UNCHANGED <<replicaVars, clientVars, historyVars>>

\* Leader handles StrongReadPropose.
\* Appends read to log, computes speculative result by scanning leader's log.
\* The log may contain entries not yet applied to kvStore (e.g., strong writes
\* on the fast path that are in the log but not yet committed/applied).
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
               \* Speculative result: scan leader's log for latest write to k.
               \* The log may contain unapplied entries (writes that haven't
               \* been committed yet), so kvStore alone is insufficient.
               specVal == SpeculativeVal(r, k)
               \* Leader sends Accept to followers for replication
               accepts == {[type  |-> "Accept",
                            from  |-> r,
                            to    |-> f,
                            slot  |-> slot,
                            entry |-> newLog[slot]] : f \in Replicas \ {r}}
               \* Leader ack: always ok, includes slot + speculative value
               leaderAck == [type     |-> "MRecordAck",
                             from     |-> r,
                             client   |-> m.client,
                             seq      |-> m.seq,
                             ok       |-> TRUE,
                             slot     |-> slot,
                             val      |-> specVal,
                             isLeader |-> TRUE]
           IN
           \* Add strong read to witness pool
           /\ unsynced' = [unsynced EXCEPT ![r][k] =
                [op    |-> Read,
                 key   |-> k,
                 val   |-> Nil,
                 cmdId |-> cmdId]]
           /\ log' = [log EXCEPT ![r] = newLog]
           /\ messages' = (messages \ {m}) \cup accepts \cup {leaderAck}
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, clientVars, historyVars>>

\* Client handles strong read fast path.
\* Requires: 3/4 quorum ok + leader slot.
\* No CausalDeps or ReadDep check (transparency).
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
               \* Leader ack (has slot + speculative value)
               leaderAcks == {a \in newResponses : a.isLeader}
               haveLeader == Cardinality(leaderAcks) > 0
               \* Total acks with ok=TRUE
               allOkAcks == {a \in newResponses : a.ok = TRUE}
               \* Check super-majority quorum
               haveQuorum == Cardinality(allOkAcks) >= ThreeQuarters
               \* Fast path succeeds if: quorum + have leader slot
               fastPathOk == haveQuorum /\ haveLeader
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
                     retVer      |-> slot])
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                              clientInvEpoch, boundReplica>>
           ELSE
               \* Not enough acks yet — just accumulate
               /\ fastPathResponses' = [fastPathResponses EXCEPT ![c] = newResponses]
               /\ messages' = messages \ {m}
               /\ UNCHANGED <<replicaVars, unsyncedVars, clientState, clientOp,
                              clientCon, clientSeq, clientCache, clientInvEpoch,
                              opsCompleted, boundReplica, historyVars>>

\* Client handles strong read slow path.
\* Completes on SyncReply from leader (after commit+apply).
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
           IN
           /\ epoch'          = epoch + 1
           /\ clientState'    = [clientState EXCEPT ![c] = Idle]
           /\ clientOp'       = [clientOp EXCEPT ![c] = Nil]
           /\ opsCompleted'   = [opsCompleted EXCEPT ![c] = @ + 1]
           /\ clientCache'    = [clientCache EXCEPT ![c][k] =
                [val |-> retVal, ver |-> Max(@.ver, slot)]]
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
                 retVer      |-> slot])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, boundReplica>>

\* ============================================================================
\* Actions — Weak Read Path (1 RTT to bound replica)
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
        \* Send WeakRead to bound replica only
        /\ messages' = messages \cup
            {[type   |-> "WeakRead",
              client |-> c,
              seq    |-> clientSeq[c],
              key    |-> k,
              dest   |-> boundReplica[c]]}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCache, opsCompleted,
                       boundReplica, fastPathResponses, history>>

\* Bound replica handles WeakRead.
\* Returns committed value + version (keyVersion) from kvStore.
HandleWeakRead(r) ==
    \E m \in messages :
        /\ m.type = "WeakRead"
        /\ m.dest = r
        /\ LET k == m.key
           IN
           /\ messages' = (messages \ {m}) \cup
              {[type    |-> "WeakReadReply",
                client  |-> m.client,
                seq     |-> m.seq,
                val     |-> kvStore[r][k],
                ver     |-> keyVersion[r][k]]}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientVars, historyVars>>

\* Client handles WeakReadReply — cache merge (max version wins).
\* All versions are real slots in CurpHT (no proxy versions).
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
               \* Cache merge: higher version wins
               useCache == cached.ver > m.ver
               finalVal == IF useCache THEN cached.val ELSE m.val
               finalVer == IF useCache THEN cached.ver ELSE m.ver
           IN
           /\ epoch'        = epoch + 1
           /\ clientState'  = [clientState EXCEPT ![c] = Idle]
           /\ clientOp'     = [clientOp EXCEPT ![c] = Nil]
           /\ opsCompleted' = [opsCompleted EXCEPT ![c] = @ + 1]
           /\ clientCache'  = [clientCache EXCEPT ![c][k] =
                [val |-> finalVal, ver |-> finalVer]]
           \* Record in history — weak reads have no log slot (slot=0).
           \* retVer is the merged version from cache/replica (real slot-based).
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
                 retVer      |-> finalVer])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, unsyncedVars, clientCon, clientSeq,
                       clientInvEpoch, boundReplica, fastPathResponses>>

\* ============================================================================
\* Next-State Relation
\* ============================================================================

Next ==
    \* Client actions — weak write
    \/ \E c \in Clients : ClientIssueWeakWrite(c)
    \/ \E c \in Clients : ClientHandleWeakWriteReply(c)
    \* Client actions — strong write
    \/ \E c \in Clients : ClientIssueStrongWrite(c)
    \/ \E c \in Clients : ClientHandleStrongWriteFastPath(c)
    \/ \E c \in Clients : ClientHandleStrongWriteSlowPath(c)
    \* Client actions — strong read
    \/ \E c \in Clients : ClientIssueStrongRead(c)
    \/ \E c \in Clients : ClientHandleStrongReadFastPath(c)
    \/ \E c \in Clients : ClientHandleStrongReadSlowPath(c)
    \* Client actions — weak read
    \/ \E c \in Clients : ClientIssueWeakRead(c)
    \/ \E c \in Clients : ClientHandleWeakReadReply(c)
    \* Replica actions — weak write
    \/ \E r \in Replicas : HandleWeakPropose(r)
    \* Replica actions — strong write
    \/ \E r \in Replicas : HandleStrongProposeFollower(r)
    \/ \E r \in Replicas : HandleStrongProposeLeader(r)
    \* Replica actions — strong read
    \/ \E r \in Replicas : HandleStrongReadProposeFollower(r)
    \/ \E r \in Replicas : HandleStrongReadProposeLeader(r)
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
\* In CURP-HT, all ops have real slots/versions:
\*   - Strong ops: slot assigned by leader before completion
\*   - Weak writes: slot assigned by leader, reply after commit+apply (2 RTT)
\*   - Weak reads: slot=0 (no log entry), retVer = merged version (real slots)
\*
\* Unlike CURP-HO, weak writes have real slots, so causal invariants involving
\* writes don't need > 0 guards. Weak reads still have slot=0 but their retVer
\* is based on real slots from committed state or client cache.

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
            /\ LET \* Replay all writes to key k in slots 1..s-1
                   RECURSIVE ComputeVal(_, _)
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
\* In CURP-HT, weak writes have real slots (committed before reply), so most
\* causal invariants apply to all ops without guards. Only weak reads have
\* slot=0 (they don't enter the log), but their retVer is real.

\* Every read returns either Nil or a value that was written.
\* In CURP-HT, all writes are committed before reply, so values always exist
\* in some replica's log. We also check history writes as a fallback.
ReadsReturnValidValues ==
    \A i \in 1..Len(history) :
        history[i].op = Read =>
        \/ history[i].retVal = Nil
        \/ \E r \in Replicas :
            \E idx \in 1..Len(log[r]) :
                /\ log[r][idx].cmd.op = Write
                /\ log[r][idx].cmd.key = history[i].key
                /\ log[r][idx].cmd.val = history[i].retVal
        \/ \E j \in 1..i :
            /\ history[j].op = Write
            /\ history[j].key = history[i].key
            /\ history[j].reqVal = history[i].retVal

\* Monotonic reads: within a session, reads of the same key never go backwards.
\* retVer is always real (slot-based) in CURP-HT. Weak reads with retVer=0
\* (reading initial state) are trivially satisfied.
MonotonicReads ==
    \A i, j \in 1..Len(history) :
        /\ history[i].op = Read
        /\ history[j].op = Read
        /\ history[i].client = history[j].client
        /\ history[i].key = history[j].key
        /\ i < j
        /\ history[i].retVer > 0
        => history[j].retVer >= history[i].retVer

\* Read-your-writes: after client c writes key k at slot s, any
\* subsequent read by c of key k must see >= s.
\* Weak writes have real slots in CURP-HT, so this covers all writes.
\* Weak reads may have retVer=0 if reading initial state — guard with > 0.
ReadYourWrites ==
    \A i, j \in 1..Len(history) :
        /\ history[i].op = Write
        /\ history[j].op = Read
        /\ history[i].client = history[j].client
        /\ history[i].key = history[j].key
        /\ i < j
        /\ history[j].retVer > 0
        => history[j].retVer >= history[i].slot

\* Monotonic writes: same client's writes must have increasing slots.
\* All writes (strong and weak) have real slots in CURP-HT.
MonotonicWrites ==
    \A i, j \in 1..Len(history) :
        /\ history[i].op = Write
        /\ history[j].op = Write
        /\ history[i].client = history[j].client
        /\ i < j
        => history[i].slot < history[j].slot

\* Writes-follow-reads: if client c reads and gets retVer v,
\* then c's next write must have slot > v.
\* Guard: retVer > 0 on read (skip reads of initial state).
WritesFollowReads ==
    \A i, j \in 1..Len(history) :
        /\ history[i].op = Read
        /\ history[j].op = Write
        /\ history[i].client = history[j].client
        /\ i < j
        /\ history[i].retVer > 0
        => history[j].slot > history[i].retVer

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
\* Weak reads have slot=0, so they are excluded from this check.

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
