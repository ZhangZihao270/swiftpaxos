---- MODULE MC_RaftHT ----
\* Model checking configuration for RaftHT.
\*
\* Instantiates the RaftHT specification with concrete values for
\* model checking by TLC.

EXTENDS RaftHT

\* ============================================================================
\* Constant Overrides
\* ============================================================================

\* Model values (declared in .cfg file)
CONSTANTS
    r1, r2, r3,             \* Replica IDs
    c1, c2,                 \* Client IDs
    k1, k2,                 \* Keys
    v1, v2,                 \* Values
    nil                     \* Nil model value

\* Concrete sets for model checking
\* Leader is always r1; r2/r3 are symmetric followers.
MCReplicas    == {r1, r2, r3}
MCClients     == {c1, c2}
MCKeys        == {k1}
MCValues      == {v1, v2}
MCMaxOps      == 2
MCNil         == nil
MCInitLeader  == r1

\* ============================================================================
\* Symmetry Sets (for state space reduction)
\* ============================================================================

\* r2/r3 are interchangeable followers (leader is fixed to r1).
\* c1/c2 are interchangeable clients.
\* v1/v2 are interchangeable values.
MCSymmetry == Permutations({r2, r3})
          \cup Permutations({c1, c2})
          \cup Permutations({v1, v2})

\* ============================================================================
\* State Constraint (bound state space)
\* ============================================================================

\* Limit log length to prevent unbounded growth
MaxLogLen == MaxOps * Cardinality(Clients) + 2

MCStateConstraint ==
    /\ \A r \in Replicas : Len(log[r]) <= MaxLogLen
    /\ \A c \in Clients : opsCompleted[c] <= MaxOps
    /\ Cardinality(messages) <= 8
    /\ epoch <= MaxLogLen * 3

\* ============================================================================
\* Invariants to Check
\* ============================================================================

\* Type invariant (useful for debugging)
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

====
