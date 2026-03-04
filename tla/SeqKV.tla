---- MODULE SeqKV ----
\* Sequential Key-Value Store specification.
\*
\* This module defines a linearizable (sequential) KV store that serves as
\* the refinement target for verifying linearizability of strong operations
\* in RaftHT.
\*
\* The sequential spec processes one operation at a time in a total order.
\* RaftHT refines this spec when projected to strong operations only,
\* with the log slot order serving as the linearization order.

EXTENDS Integers, Sequences, FiniteSets

CONSTANTS
    Keys,           \* Set of keys
    Values,         \* Set of non-nil values
    Nil             \* Null value

\* Nil must not be in Values (enforced by using distinct model values)

\* ============================================================================
\* State Variables
\* ============================================================================

VARIABLES
    store,          \* store \in [Keys -> Values \cup {Nil}]
    seqHistory      \* Sequence of [op, key, val, retVal] records

vars == <<store, seqHistory>>

\* ============================================================================
\* Type Definitions
\* ============================================================================

AllValues == Values \cup {Nil}

Read  == "Read"
Write == "Write"

SeqEntry == [op: {Read, Write}, key: Keys, val: AllValues, retVal: AllValues]

\* ============================================================================
\* Initial State
\* ============================================================================

Init ==
    /\ store = [k \in Keys |-> Nil]
    /\ seqHistory = <<>>

\* ============================================================================
\* Actions
\* ============================================================================

\* Execute a write: update store, record in history
DoWrite(k, v) ==
    /\ v \in Values
    /\ store' = [store EXCEPT ![k] = v]
    /\ seqHistory' = Append(seqHistory,
                        [op |-> Write, key |-> k, val |-> v, retVal |-> v])

\* Execute a read: return current value, record in history
DoRead(k) ==
    /\ store' = store
    /\ seqHistory' = Append(seqHistory,
                        [op |-> Read, key |-> k, val |-> Nil, retVal |-> store[k]])

\* ============================================================================
\* Next-State Relation
\* ============================================================================

Next ==
    \E k \in Keys :
        \/ DoRead(k)
        \/ \E v \in Values : DoWrite(k, v)

\* ============================================================================
\* Specification
\* ============================================================================

Spec == Init /\ [][Next]_vars

====
