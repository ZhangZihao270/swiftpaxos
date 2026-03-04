---- MODULE RaftHT ----
\* TLA+ specification of Raft-HT: Raft with Hybrid Consistency (H+T)
\*
\* Raft-HT extends vanilla Raft with weak (causal) operations:
\*   - Strong ops: standard Raft consensus (linearizable)
\*   - Weak writes: early reply at leader, replicated in background
\*   - Weak reads: read committed state at any replica + client cache merge
\*
\* Properties to verify (safety only):
\*   1. Linearizability of strong operations (refinement to SeqKV)
\*   2. Causal consistency of all operations
\*   3. Hybrid compatibility (total order and causal order don't contradict)

EXTENDS Integers, Sequences, FiniteSets, TLC

\* ============================================================================
\* Constants
\* ============================================================================

CONSTANTS
    Replicas,       \* Set of replica IDs, e.g., {r1, r2, r3}
    Clients,        \* Set of client IDs, e.g., {c1, c2}
    Keys,           \* Set of keys, e.g., {k1, k2}
    Values,         \* Set of non-nil values, e.g., {v1, v2}
    MaxOps,         \* Max operations per client (bounds state space)
    Nil             \* Distinguished null value (not in Values)

\* Nil must not be in Values (enforced by using distinct model values)
\* MaxOps must be a positive natural number

\* ============================================================================
\* Symbolic Constants
\* ============================================================================

\* Operation types
Read  == "Read"
Write == "Write"

\* Consistency levels
Strong == "Strong"
Weak   == "Weak"

\* Replica roles (simplified: no Candidate — leader election is atomic)
Follower == "Follower"
Leader   == "Leader"

\* Client states
Idle    == "Idle"
Waiting == "Waiting"

\* ============================================================================
\* Type Definitions (as set expressions)
\* ============================================================================

\* All possible values including Nil
AllValues == Values \cup {Nil}

\* A command issued by a client
Command == [op: {Read, Write}, key: Keys, val: AllValues]

\* A log entry in a replica's log
LogEntryType == [
    cmd: Command,
    consistency: {Strong, Weak},
    term: Nat,
    client: Clients,
    seq: Nat               \* Client-side sequence number for reply routing
]

\* A client cache entry: value + version (log slot of last write)
CacheEntryType == [val: AllValues, ver: Nat]

\* ============================================================================
\* Variables
\* ============================================================================

VARIABLES
    \* --- Replica state (functions indexed by Replicas) ---
    role,           \* role[r] \in {Follower, Leader}
    currentTerm,    \* currentTerm[r] \in Nat
    log,            \* log[r] \in Seq(LogEntryType)
    commitIndex,    \* commitIndex[r] \in Nat (0 = nothing committed)
    lastApplied,    \* lastApplied[r] \in Nat (0 = nothing applied)
    kvStore,        \* kvStore[r] \in [Keys -> AllValues]
    keyVersion,     \* keyVersion[r] \in [Keys -> Nat]
    nextIndex,      \* nextIndex[r] \in [Replicas -> Nat] (leader only)
    matchIndex,     \* matchIndex[r] \in [Replicas -> Nat] (leader only)

    \* --- Client state (functions indexed by Clients) ---
    clientState,    \* clientState[c] \in {Idle, Waiting}
    clientOp,       \* clientOp[c] \in Command \cup {Nil} (current pending op)
    clientCon,      \* clientCon[c] \in {Strong, Weak} (consistency of pending op)
    clientSeq,      \* clientSeq[c] \in Nat (next sequence number)
    clientCache,    \* clientCache[c] \in [Keys -> CacheEntryType]
    clientInvEpoch, \* clientInvEpoch[c] \in Nat (epoch at invocation of pending op)
    opsCompleted,   \* opsCompleted[c] \in Nat

    \* --- Network ---
    messages,       \* Bag (set) of in-flight messages

    \* --- History (auxiliary variables for property checking) ---
    history,        \* Sequence of completed operation records
    epoch           \* Monotonic counter for real-time ordering

\* ============================================================================
\* Variable Tuples
\* ============================================================================

replicaVars == <<role, currentTerm, log, commitIndex, lastApplied,
                  kvStore, keyVersion, nextIndex, matchIndex>>

clientVars  == <<clientState, clientOp, clientCon, clientSeq,
                  clientCache, clientInvEpoch, opsCompleted>>

networkVars == <<messages>>

historyVars == <<history, epoch>>

vars == <<role, currentTerm, log, commitIndex, lastApplied,
          kvStore, keyVersion, nextIndex, matchIndex,
          clientState, clientOp, clientCon, clientSeq,
          clientCache, clientInvEpoch, opsCompleted,
          messages, history, epoch>>

\* ============================================================================
\* Helpers
\* ============================================================================

\* The set of all replicas currently serving as leader
Leaders == {r \in Replicas : role[r] = Leader}

\* Quorum size (strict majority)
Quorum == (Cardinality(Replicas) \div 2) + 1

\* Last log index for replica r (0 if empty)
LastLogIndex(r) == Len(log[r])

\* Last log term for replica r (0 if empty)
LastLogTerm(r) == IF Len(log[r]) = 0 THEN 0 ELSE log[r][Len(log[r])].term

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

\* Maximum of a non-empty set of naturals
SetMax(S) == CHOOSE x \in S : \A y \in S : y <= x

\* ============================================================================
\* Initial State
\* ============================================================================

Init ==
    \* One replica starts as leader; rest are followers
    /\ \E ldr \in Replicas :
         role = [r \in Replicas |-> IF r = ldr THEN Leader ELSE Follower]
    /\ currentTerm = [r \in Replicas |-> 1]
    /\ log         = [r \in Replicas |-> <<>>]
    /\ commitIndex = [r \in Replicas |-> 0]
    /\ lastApplied = [r \in Replicas |-> 0]
    /\ kvStore     = [r \in Replicas |-> [k \in Keys |-> Nil]]
    /\ keyVersion  = [r \in Replicas |-> [k \in Keys |-> 0]]
    /\ nextIndex   = [r \in Replicas |-> [f \in Replicas |-> 1]]
    /\ matchIndex  = [r \in Replicas |-> [f \in Replicas |-> 0]]
    /\ clientState = [c \in Clients |-> Idle]
    /\ clientOp    = [c \in Clients |-> Nil]
    /\ clientCon   = [c \in Clients |-> Strong]
    /\ clientSeq   = [c \in Clients |-> 1]
    /\ clientCache = [c \in Clients |-> [k \in Keys |-> [val |-> Nil, ver |-> 0]]]
    /\ clientInvEpoch = [c \in Clients |-> 0]
    /\ opsCompleted = [c \in Clients |-> 0]
    /\ messages    = {}
    /\ history     = <<>>
    /\ epoch       = 0

\* ============================================================================
\* Actions — Replica State Machine (Task 55.2)
\* ============================================================================

\* (55.2a) Leader sends AppendEntries to follower f
\* Guard: only send if no outstanding AE to f (prevents message bag explosion)
SendAppendEntries(r, f) ==
    /\ role[r] = Leader
    /\ r # f
    /\ ~\E m \in messages : m.type = "AE" /\ m.from = r /\ m.to = f
    /\ LET prevIdx == nextIndex[r][f] - 1
           prevTrm == IF prevIdx = 0 THEN 0 ELSE log[r][prevIdx].term
           ents    == IF nextIndex[r][f] <= Len(log[r])
                      THEN SubSeq(log[r], nextIndex[r][f], Len(log[r]))
                      ELSE <<>>
       IN messages' = messages \cup
          {[type         |-> "AE",
            from         |-> r,
            to           |-> f,
            term         |-> currentTerm[r],
            prevLogIndex |-> prevIdx,
            prevLogTerm  |-> prevTrm,
            entries      |-> ents,
            leaderCommit |-> commitIndex[r]]}
    /\ UNCHANGED <<replicaVars, clientVars, historyVars>>

\* (55.2b) Follower handles AppendEntries (log matches — success)
HandleAppendEntriesOk(r) ==
    \E m \in messages :
        /\ m.type = "AE"
        /\ m.to = r
        /\ m.term >= currentTerm[r]
        /\ IF m.prevLogIndex = 0
           THEN TRUE
           ELSE /\ m.prevLogIndex <= Len(log[r])
                /\ log[r][m.prevLogIndex].term = m.prevLogTerm
        /\ LET newLog == SubSeq(log[r], 1, m.prevLogIndex) \o m.entries
               newCI  == Max(commitIndex[r], Min(m.leaderCommit, Len(newLog)))
           IN
           \* Guard: never truncate below commitIndex (committed entries are durable)
           /\ Len(newLog) >= commitIndex[r]
           /\ log'         = [log EXCEPT ![r] = newLog]
           /\ commitIndex' = [commitIndex EXCEPT ![r] = newCI]
           /\ currentTerm' = [currentTerm EXCEPT ![r] = m.term]
           /\ role'        = [role EXCEPT ![r] = Follower]
           /\ messages' = (messages \ {m}) \cup
              {[type       |-> "AEReply",
                from       |-> r,
                to         |-> m.from,
                term       |-> m.term,
                success    |-> TRUE,
                matchIndex |-> Len(newLog)]}
        /\ UNCHANGED <<lastApplied, kvStore, keyVersion, nextIndex, matchIndex,
                       clientVars, historyVars>>

\* (55.2b) Follower handles AppendEntries (log mismatch — failure)
HandleAppendEntriesFail(r) ==
    \E m \in messages :
        /\ m.type = "AE"
        /\ m.to = r
        /\ m.term >= currentTerm[r]
        /\ m.prevLogIndex > 0
        /\ IF m.prevLogIndex > Len(log[r])
           THEN TRUE
           ELSE log[r][m.prevLogIndex].term # m.prevLogTerm
        /\ currentTerm' = [currentTerm EXCEPT ![r] = m.term]
        /\ role'        = [role EXCEPT ![r] = Follower]
        /\ messages' = (messages \ {m}) \cup
           {[type       |-> "AEReply",
             from       |-> r,
             to         |-> m.from,
             term       |-> m.term,
             success    |-> FALSE,
             matchIndex |-> 0]}
        /\ UNCHANGED <<log, commitIndex, lastApplied, kvStore, keyVersion,
                       nextIndex, matchIndex, clientVars, historyVars>>

\* (55.2c) Leader handles AppendEntriesReply — success: advance commit
HandleAEReplySuccess(r) ==
    \E m \in messages :
        /\ m.type = "AEReply"
        /\ m.to = r
        /\ role[r] = Leader
        /\ m.term = currentTerm[r]
        /\ m.success
        /\ LET newMI == [matchIndex[r] EXCEPT ![m.from] = Max(@, m.matchIndex)]
               \* Highest index committable with majority support in current term
               ci    == {i \in (commitIndex[r]+1)..Len(log[r]) :
                           /\ log[r][i].term = currentTerm[r]
                           /\ Cardinality({s \in Replicas : newMI[s] >= i}) >= Quorum}
               newCI == IF ci = {} THEN commitIndex[r] ELSE SetMax(ci)
           IN
           /\ matchIndex' = [matchIndex EXCEPT ![r] = newMI]
           /\ nextIndex'  = [nextIndex EXCEPT ![r][m.from] = Max(@, m.matchIndex + 1)]
           /\ commitIndex'= [commitIndex EXCEPT ![r] = newCI]
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<role, currentTerm, log, lastApplied, kvStore, keyVersion,
                       clientVars, historyVars>>

\* (55.2c) Leader handles AppendEntriesReply — failure: back off nextIndex
HandleAEReplyFailure(r) ==
    \E m \in messages :
        /\ m.type = "AEReply"
        /\ m.to = r
        /\ role[r] = Leader
        /\ m.term = currentTerm[r]
        /\ ~m.success
        /\ nextIndex' = [nextIndex EXCEPT ![r][m.from] = Max(1, @ - 1)]
        /\ messages'  = messages \ {m}
        /\ UNCHANGED <<role, currentTerm, log, commitIndex, lastApplied,
                       kvStore, keyVersion, matchIndex, clientVars, historyVars>>

\* (55.2d) Any replica applies next committed entry to state machine
ApplyEntry(r) ==
    /\ lastApplied[r] < commitIndex[r]
    /\ LET idx     == lastApplied[r] + 1
           entry   == log[r][idx]
           isWrite == entry.cmd.op = Write
       IN
       /\ lastApplied' = [lastApplied EXCEPT ![r] = idx]
       /\ IF isWrite
          THEN /\ kvStore'    = [kvStore EXCEPT ![r][entry.cmd.key] = entry.cmd.val]
               /\ keyVersion' = [keyVersion EXCEPT ![r][entry.cmd.key] = idx]
          ELSE UNCHANGED <<kvStore, keyVersion>>
       \* Leader sends reply for strong operations upon apply
       /\ IF role[r] = Leader /\ entry.consistency = Strong
          THEN LET result == IF isWrite THEN entry.cmd.val
                             ELSE kvStore[r][entry.cmd.key]
               IN messages' = messages \cup
                  {[type   |-> "StrongReply",
                    client |-> entry.client,
                    seq    |-> entry.seq,
                    val    |-> result,
                    slot   |-> idx]}
          ELSE UNCHANGED messages
    /\ UNCHANGED <<role, currentTerm, log, commitIndex, nextIndex, matchIndex,
                   clientVars, historyVars>>

\* (55.2e) Simplified leader election — TODO: add in later phase
\* For now, fixed leader. Leader election will be added to test C3
\* (uncommitted weak write loss on leader change).

\* Discard stale messages to bound state space
DiscardStaleMessage ==
    \E m \in messages :
        /\ \/ /\ m.type = "AE"
              /\ m.term < currentTerm[m.to]
           \/ /\ m.type = "AEReply"
              /\ m.term < currentTerm[m.to]
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, clientVars, historyVars>>

\* ============================================================================
\* Actions — Operation Handling (Task 55.3)
\* ============================================================================

\* (55.3a) Leader handles strong propose — append to log, reply after commit
HandleStrongPropose(r) ==
    \E m \in messages :
        /\ m.type = "StrongPropose"
        /\ role[r] = Leader
        /\ LET newLog == Append(log[r],
               [cmd         |-> m.cmd,
                consistency |-> Strong,
                term        |-> currentTerm[r],
                client      |-> m.client,
                seq         |-> m.seq])
           IN
           /\ log'        = [log EXCEPT ![r] = newLog]
           /\ matchIndex' = [matchIndex EXCEPT ![r][r] = Len(newLog)]
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, nextIndex, clientVars, historyVars>>

\* (55.3b) Leader handles weak write — append to log, reply IMMEDIATELY
HandleWeakPropose(r) ==
    \E m \in messages :
        /\ m.type = "WeakPropose"
        /\ role[r] = Leader
        /\ LET newLog == Append(log[r],
               [cmd         |-> m.cmd,
                consistency |-> Weak,
                term        |-> currentTerm[r],
                client      |-> m.client,
                seq         |-> m.seq])
               slot == Len(newLog)
           IN
           /\ log'        = [log EXCEPT ![r] = newLog]
           /\ matchIndex' = [matchIndex EXCEPT ![r][r] = Len(newLog)]
           \* Immediate reply — key Raft-HT behavior (1 WAN RTT)
           /\ messages' = (messages \ {m}) \cup
              {[type   |-> "WeakWriteReply",
                client |-> m.client,
                seq    |-> m.seq,
                slot   |-> slot]}
        /\ UNCHANGED <<role, currentTerm, commitIndex, lastApplied,
                       kvStore, keyVersion, nextIndex, clientVars, historyVars>>

\* (55.3c) Any replica handles weak read — read committed state, reply
HandleWeakRead(r) ==
    \E m \in messages :
        /\ m.type = "WeakRead"
        /\ m.dest = r
        /\ messages' = (messages \ {m}) \cup
           {[type    |-> "WeakReadReply",
             client  |-> m.client,
             seq     |-> m.seq,
             val     |-> kvStore[r][m.key],
             ver     |-> keyVersion[r][m.key],
             replica |-> r]}
        /\ UNCHANGED <<replicaVars, clientVars, historyVars>>

\* ============================================================================
\* Actions — Client Behavior (Task 55.4)
\* ============================================================================

\* (55.4a-d) Client issues an operation (strong read/write or weak read/write)
ClientIssueOp(c) ==
    /\ clientState[c] = Idle
    /\ opsCompleted[c] < MaxOps
    /\ \E k \in Keys, opType \in {Read, Write}, con \in {Strong, Weak} :
       \E v \in IF opType = Write THEN Values ELSE {Nil} :
         LET cmd == [op |-> opType, key |-> k, val |-> v]
         IN
         /\ epoch'          = epoch + 1
         /\ clientState'    = [clientState EXCEPT ![c] = Waiting]
         /\ clientOp'       = [clientOp EXCEPT ![c] = cmd]
         /\ clientCon'      = [clientCon EXCEPT ![c] = con]
         /\ clientInvEpoch' = [clientInvEpoch EXCEPT ![c] = epoch + 1]
         /\ clientSeq'      = [clientSeq EXCEPT ![c] = @ + 1]
         /\ UNCHANGED <<clientCache, opsCompleted>>
         \* Route message based on operation type
         /\ IF con = Strong
            THEN \* Strong ops go to leader
                 messages' = messages \cup
                   {[type   |-> "StrongPropose",
                     client |-> c,
                     seq    |-> clientSeq[c],
                     cmd    |-> cmd]}
            ELSE IF opType = Write
            THEN \* Weak writes go to leader
                 messages' = messages \cup
                   {[type   |-> "WeakPropose",
                     client |-> c,
                     seq    |-> clientSeq[c],
                     cmd    |-> cmd]}
            ELSE \* Weak reads go to any replica (nearest in impl)
                 \E dest \in Replicas :
                   messages' = messages \cup
                     {[type   |-> "WeakRead",
                       client |-> c,
                       seq    |-> clientSeq[c],
                       key    |-> k,
                       dest   |-> dest]}
         /\ UNCHANGED <<replicaVars, history>>

\* (55.4e) Client handles strong reply (after commit+apply)
ClientHandleStrongReply(c) ==
    \E m \in messages :
        /\ m.type = "StrongReply"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1    \* Match pending op's seq
        /\ clientState[c] = Waiting
        /\ clientCon[c] = Strong
        /\ LET k == clientOp[c].key
           IN
           /\ epoch'        = epoch + 1
           /\ clientState'  = [clientState EXCEPT ![c] = Idle]
           /\ clientOp'     = [clientOp EXCEPT ![c] = Nil]
           /\ opsCompleted' = [opsCompleted EXCEPT ![c] = @ + 1]
           \* Update cache: slot serves as version
           /\ clientCache'  = [clientCache EXCEPT ![c][k] =
                [val |-> m.val, ver |-> Max(@.ver, m.slot)]]
           \* Record in history
           /\ history' = Append(history,
                [client      |-> c,
                 op          |-> clientOp[c].op,
                 key         |-> k,
                 reqVal      |-> clientOp[c].val,
                 retVal      |-> m.val,
                 consistency |-> Strong,
                 invEpoch    |-> clientInvEpoch[c],
                 retEpoch    |-> epoch + 1,
                 slot        |-> m.slot])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, clientCon, clientSeq, clientInvEpoch>>

\* (55.4f) Client handles weak write reply (immediate from leader)
ClientHandleWeakWriteReply(c) ==
    \E m \in messages :
        /\ m.type = "WeakWriteReply"
        /\ m.client = c
        /\ m.seq = clientSeq[c] - 1
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
           \* Cache the written value with its slot as version
           /\ clientCache'  = [clientCache EXCEPT ![c][k] =
                [val |-> v, ver |-> Max(@.ver, m.slot)]]
           \* Record in history
           /\ history' = Append(history,
                [client      |-> c,
                 op          |-> Write,
                 key         |-> k,
                 reqVal      |-> v,
                 retVal      |-> v,
                 consistency |-> Weak,
                 invEpoch    |-> clientInvEpoch[c],
                 retEpoch    |-> epoch + 1,
                 slot        |-> m.slot])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, clientCon, clientSeq, clientInvEpoch>>

\* (55.4g) Client handles weak read reply — cache merge (max version wins)
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
           \* Record in history — retVal is the merged result
           /\ history' = Append(history,
                [client      |-> c,
                 op          |-> Read,
                 key         |-> k,
                 reqVal      |-> Nil,
                 retVal      |-> finalVal,
                 consistency |-> Weak,
                 invEpoch    |-> clientInvEpoch[c],
                 retEpoch    |-> epoch + 1,
                 slot        |-> 0])
        /\ messages' = messages \ {m}
        /\ UNCHANGED <<replicaVars, clientCon, clientSeq, clientInvEpoch>>

\* ============================================================================
\* Next-State Relation
\* ============================================================================

Next ==
    \* Replica actions
    \/ \E r, f \in Replicas : SendAppendEntries(r, f)
    \/ \E r \in Replicas : HandleAppendEntriesOk(r)
    \/ \E r \in Replicas : HandleAppendEntriesFail(r)
    \/ \E r \in Replicas : HandleAEReplySuccess(r)
    \/ \E r \in Replicas : HandleAEReplyFailure(r)
    \/ \E r \in Replicas : ApplyEntry(r)
    \/ DiscardStaleMessage
    \* Operation handling
    \/ \E r \in Replicas : HandleStrongPropose(r)
    \/ \E r \in Replicas : HandleWeakPropose(r)
    \/ \E r \in Replicas : HandleWeakRead(r)
    \* Client actions
    \/ \E c \in Clients : ClientIssueOp(c)
    \/ \E c \in Clients : ClientHandleStrongReply(c)
    \/ \E c \in Clients : ClientHandleWeakWriteReply(c)
    \/ \E c \in Clients : ClientHandleWeakReadReply(c)

\* ============================================================================
\* Invariants — Tasks 55.7-55.9
\* ============================================================================

\* --- Helpers for history analysis ---

\* Project history to strong operations only, in order
StrongOps == SelectSeq(history, LAMBDA e : e.consistency = Strong)

\* Project history to operations with slots (strong + weak writes)
SlottedOps == SelectSeq(history, LAMBDA e : e.slot > 0)

\* Compute the KV state after applying a prefix of ops in slot order.
\* Given a sequence of ops sorted by slot, apply writes to build the store.
\* Returns [Keys -> AllValues].
ApplyOpsToStore(ops) ==
    LET RECURSIVE Apply(_, _)
        Apply(store, i) ==
            IF i > Len(ops) THEN store
            ELSE IF ops[i].op = Write
                 THEN Apply([store EXCEPT ![ops[i].key] = ops[i].reqVal], i + 1)
                 ELSE Apply(store, i + 1)
    IN Apply([k \in Keys |-> Nil], 1)

\* Sort slotted ops by slot (using insertion sort for TLC tractability)
\* Returns a sequence of history entries sorted by slot
SortBySlot(ops) ==
    LET RECURSIVE Insert(_, _)
        Insert(sorted, e) ==
            IF sorted = <<>> THEN <<e>>
            ELSE IF e.slot <= Head(sorted).slot
                 THEN <<e>> \o sorted
                 ELSE <<Head(sorted)>> \o Insert(Tail(sorted), e)
        RECURSIVE DoSort(_)
        DoSort(remaining) ==
            IF remaining = <<>> THEN <<>>
            ELSE Insert(DoSort(Tail(remaining)), Head(remaining))
    IN DoSort(ops)

\* ============================================================================
\* Task 55.7: Linearizability of strong operations
\* ============================================================================
\* Strong ops are linearized by their slot order (log position).
\* Requirements:
\*   (a) Real-time respect: if op1 completes before op2 invokes, op1.slot < op2.slot
\*   (b) Read consistency: each strong read returns the value consistent with
\*       replaying all strong writes in slot order up to the read's slot

\* (a) Real-time respect among strong ops
RealTimeRespect ==
    LET sOps == StrongOps
    IN \A i, j \in 1..Len(sOps) :
        (sOps[i].retEpoch < sOps[j].invEpoch) => (sOps[i].slot < sOps[j].slot)

\* (b) Read consistency: strong reads return correct values in log slot order.
\* The leader's log IS the linearization. For each strong read at slot s of
\* key k, replay all writes in the leader's log with slot < s.
\* Note: the leader's log contains both strong and weak ops in order.
StrongReadConsistency ==
    \A i \in 1..Len(history) :
        /\ history[i].consistency = Strong
        /\ history[i].op = Read
        =>
        LET k == history[i].key
            s == history[i].slot
            \* Find the leader that has this slot committed
            \* (any replica with the entry at slot s works since logs agree on committed entries)
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
\* Task 55.8: Causal consistency of all operations
\* ============================================================================
\* Causal consistency requires:
\*   (a) Session order: same-client ops are ordered (earlier ops are "visible")
\*   (b) Reads return values from some write (or Nil for unwritten keys)
\*   (c) Monotonic reads: within a session, if a read returns version v,
\*       a later read of the same key returns version >= v
\*
\* In our model, clients are sequential (one op at a time), so session order
\* is trivially maintained by the history sequence. We check:
\*   - Every read returns a valid value
\*   - Monotonic reads per client per key

\* Every read returns either Nil (initial value) or a valid value.
\* The value must have been written to this key by some operation — we check
\* that the value appears as a write in the log of some replica.
ReadsReturnValidValues ==
    \A i \in 1..Len(history) :
        history[i].op = Read =>
        \/ history[i].retVal = Nil
        \/ \E r \in Replicas :
            \E idx \in 1..Len(log[r]) :
                /\ log[r][idx].cmd.op = Write
                /\ log[r][idx].cmd.key = history[i].key
                /\ log[r][idx].cmd.val = history[i].retVal

\* Monotonic reads: within a session, reads of the same key never go backwards.
\* If client c reads key k and gets a non-Nil value, a later read by c of k
\* must not return Nil (cannot "unsee" a write). This is guaranteed by the
\* client cache merge (max version wins).
MonotonicReads ==
    \A i, j \in 1..Len(history) :
        /\ history[i].op = Read
        /\ history[j].op = Read
        /\ history[i].client = history[j].client
        /\ history[i].key = history[j].key
        /\ i < j
        /\ history[i].retVal # Nil
        => history[j].retVal # Nil

CausalConsistencyInv == ReadsReturnValidValues /\ MonotonicReads

\* ============================================================================
\* Task 55.9: Hybrid compatibility
\* ============================================================================
\* ≺_T (total order) is slot order for all slotted ops (strong + weak writes).
\* ≺_P (partial order) includes session order.
\* Hybrid compatibility: ¬(o1 ≺_T o2 ∧ o2 ≺_P o1)
\*
\* Since clients are sequential, session order means: for same-client ops,
\* earlier in history ≺_P later. For ≺_T: lower slot ≺_T higher slot.
\*
\* Violation would be: o1.slot < o2.slot but o2 completed before o1 started
\* in the same session. This can't happen since clients wait for each op.
\*
\* Cross-client: ≺_P only has session order and read-from.
\* Read-from: if op j reads value written by op i, then i ≺_P j.
\*
\* Check: for all pairs (i,j) with slots: if slot[i] < slot[j] (i ≺_T j),
\* then ¬(j ≺_P i). Since ≺_P includes read-from, check that j is NOT
\* read-from by i (i.e., i doesn't read a value written by j when i ≺_T j would
\* mean j should come after i).

HybridCompatibilityInv ==
    \* For all pairs of slotted operations:
    \* if o1.slot < o2.slot (o1 ≺_T o2), then o2 must not causally precede o1
    \A i, j \in 1..Len(history) :
        /\ history[i].slot > 0
        /\ history[j].slot > 0
        /\ history[i].slot < history[j].slot
        =>
        \* o2 (j by slot) must not be in causal past of o1 (i by slot)
        \* Session order check: j not before i in same session
        /\ ~(history[j].client = history[i].client /\ j < i)
        \* Read-from check: i must not read a value written by j
        \* (since i ≺_T j means j comes after i in total order)
        /\ ~(history[i].op = Read /\ history[j].op = Write
              /\ history[i].key = history[j].key
              /\ history[i].retVal = history[j].retVal
              /\ j < i)

\* Combined safety invariant
SafetyInv == LinearizabilityInv /\ CausalConsistencyInv /\ HybridCompatibilityInv

\* ============================================================================
\* Specification
\* ============================================================================

Spec == Init /\ [][Next]_vars

====
