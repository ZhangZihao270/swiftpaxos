---- MODULE MC_CurpHO ----
\* Model checking configuration for CurpHO.

EXTENDS CurpHO

\* ============================================================================
\* Constant Overrides
\* ============================================================================

CONSTANTS
    r1, r2, r3,
    c1, c2,
    k1, k2,
    v1, v2,
    nil

\* Leader is always r1; r2/r3 are symmetric followers.
MCReplicas    == {r1, r2, r3}
MCClients     == {c1}
MCKeys        == {k1}
MCValues      == {v1, v2}
MCMaxOps      == 2
MCNil         == nil
MCInitLeader  == r1

\* ============================================================================
\* Symmetry Sets
\* ============================================================================

\* r2/r3 are interchangeable followers (leader is fixed to r1).
\* c1/c2 are interchangeable clients.
\* v1/v2 are interchangeable values.
MCSymmetry == Permutations({r2, r3})
          \cup Permutations({v1, v2})

\* ============================================================================
\* State Constraint
\* ============================================================================

MaxLogLen == MaxOps * Cardinality(Clients) + 2

MCStateConstraint ==
    /\ \A r \in Replicas : Len(log[r]) <= MaxLogLen
    /\ \A c \in Clients : opsCompleted[c] <= MaxOps
    /\ Cardinality(messages) <= 8
    /\ epoch <= MaxLogLen * 3

\* ============================================================================
\* Type Invariant
\* ============================================================================

MCTypeInv ==
    /\ \A r \in Replicas :
        /\ role[r] \in {Follower, Leader}
        /\ currentTerm[r] \in Nat
        /\ commitIndex[r] \in Nat
        /\ lastApplied[r] \in Nat
        /\ lastApplied[r] <= commitIndex[r]
        /\ commitIndex[r] <= Len(log[r])
    /\ \A c \in Clients :
        /\ clientState[c] \in {Idle, Waiting}
        /\ opsCompleted[c] \in 0..MaxOps
        /\ clientInvEpoch[c] \in Nat
    \* History entries have valid retVer (Nat)
    /\ \A i \in 1..Len(history) :
        /\ history[i].retVer \in Nat
        /\ history[i].slot \in Nat

\* Combined invariant for model checking: type + safety
MCSafetyInv == SafetyInv

====
