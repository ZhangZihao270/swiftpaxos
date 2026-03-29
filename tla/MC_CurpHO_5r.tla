---- MODULE MC_CurpHO_5r ----
\* Model checking configuration for CurpHO with 5 replicas.

EXTENDS CurpHO

CONSTANTS
    r1, r2, r3, r4, r5,
    c1, c2,
    k1, k2,
    v1, v2,
    nil

MCReplicas    == {r1, r2, r3, r4, r5}
MCClients     == {c1, c2}
MCKeys        == {k1}
MCValues      == {v1, v2}
MCMaxOps      == 2
MCNil         == nil
MCInitLeader  == r1

MCSymmetry == Permutations({r2, r3, r4, r5})
          \cup Permutations({c1, c2})
          \cup Permutations({v1, v2})

MaxLogLen == MCMaxOps * Cardinality(MCClients) + 2

MCStateConstraint ==
    /\ \A r \in Replicas : Len(log[r]) <= MaxLogLen
    /\ \A c \in Clients : opsCompleted[c] <= MCMaxOps
    /\ Cardinality(messages) <= 12
    /\ epoch <= MaxLogLen * 4
    /\ nextWriteId <= MCMaxOps * Cardinality(MCClients) + 1

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
        /\ opsCompleted[c] \in 0..MCMaxOps
        /\ clientInvEpoch[c] \in Nat
    /\ \A i \in 1..Len(history) :
        /\ history[i].retVer \in Nat
        /\ history[i].slot \in Nat
    /\ nextWriteId \in Nat

MCSafetyInv == SafetyInv

====
