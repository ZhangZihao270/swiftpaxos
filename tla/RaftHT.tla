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
                  clientCache, opsCompleted>>

networkVars == <<messages>>

historyVars == <<history, epoch>>

vars == <<role, currentTerm, log, commitIndex, lastApplied,
          kvStore, keyVersion, nextIndex, matchIndex,
          clientState, clientOp, clientCon, clientSeq,
          clientCache, opsCompleted,
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
    /\ opsCompleted = [c \in Clients |-> 0]
    /\ messages    = {}
    /\ history     = <<>>
    /\ epoch       = 0

\* ============================================================================
\* Actions — Task 55.2: Replica State Machine
\* ============================================================================

\* TODO(55.2a): SendAppendEntries(leader, follower)
\* TODO(55.2b): HandleAppendEntries(follower, msg)
\* TODO(55.2c): HandleAppendEntriesReply(leader, msg)
\* TODO(55.2d): AdvanceCommitIndex(leader)
\* TODO(55.2e): ApplyEntry(replica)
\* TODO(55.2f): ElectLeader(replica)

\* ============================================================================
\* Actions — Task 55.3: Operation Handling
\* ============================================================================

\* TODO(55.3a): HandleStrongPropose(leader, msg)  — append to log, wait for commit
\* TODO(55.3b): HandleWeakPropose(leader, msg)    — append to log, reply immediately
\* TODO(55.3c): HandleWeakRead(replica, msg)      — read committed state, reply

\* ============================================================================
\* Actions — Task 55.4: Client Behavior
\* ============================================================================

\* TODO(55.4a): IssueStrongWrite(client, key, val)
\* TODO(55.4b): IssueStrongRead(client, key)
\* TODO(55.4c): IssueWeakWrite(client, key, val)
\* TODO(55.4d): IssueWeakRead(client, key)
\* TODO(55.4e): HandleStrongReply(client, msg)
\* TODO(55.4f): HandleWeakWriteReply(client, msg)
\* TODO(55.4g): HandleWeakReadReply(client, msg)  — includes cache merge

\* ============================================================================
\* Next-State Relation (placeholder until actions are defined)
\* ============================================================================

Next == UNCHANGED vars

\* ============================================================================
\* Invariants — Task 55.7-55.9 (placeholders)
\* ============================================================================

\* Task 55.7: Linearizability — strong ops form a valid linearization
LinearizabilityInv == TRUE

\* Task 55.8: Causal consistency — all ops respect causal order
CausalConsistencyInv == TRUE

\* Task 55.9: Hybrid compatibility — total and causal orders don't contradict
HybridCompatibilityInv == TRUE

\* Combined safety invariant
SafetyInv == LinearizabilityInv /\ CausalConsistencyInv /\ HybridCompatibilityInv

\* ============================================================================
\* Specification
\* ============================================================================

Spec == Init /\ [][Next]_vars

====
