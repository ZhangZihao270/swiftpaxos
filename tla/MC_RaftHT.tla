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
\* Config A (exhaustive, ~2 min): 1 client, 1 key, 1 value, MaxOps=2
\*   → 49.8M states, 7.58M distinct, depth 36
\* Config B (partial, ~10 min): 2 clients, 1 key, 1 value, MaxOps=1
\*   → 148M+ states explored (does not terminate quickly)
MCReplicas == {r1, r2, r3}
MCClients  == {c1}
MCKeys     == {k1}
MCValues   == {v1}
MCMaxOps   == 2
MCNil      == nil

\* ============================================================================
\* Symmetry Sets (for state space reduction)
\* ============================================================================

\* No symmetry — leader is chosen nondeterministically, so replicas aren't symmetric
\* MCSymmetry == {}

\* ============================================================================
\* State Constraint (bound state space)
\* ============================================================================

\* Limit log length to prevent unbounded growth
MaxLogLen == MaxOps * Cardinality(Clients) + 2

MCStateConstraint ==
    /\ \A r \in Replicas : Len(log[r]) <= MaxLogLen
    /\ \A c \in Clients : opsCompleted[c] <= MaxOps
    /\ Cardinality(messages) <= 10
    /\ epoch <= MaxLogLen * 4

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
