---- MODULE MC_CurpHT_5r ----
\* Model checking configuration for CurpHT with 5 replicas.

EXTENDS CurpHT

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

MaxLogLen == MaxOps * Cardinality(Clients) + 2

MCStateConstraint ==
    /\ \A r \in Replicas : Len(log[r]) <= MaxLogLen
    /\ \A c \in Clients : opsCompleted[c] <= MaxOps
    /\ Cardinality(messages) <= 8
    /\ epoch <= MaxLogLen * 3

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
    /\ \A i \in 1..Len(history) :
        /\ history[i].retVer \in Nat
        /\ history[i].slot \in Nat

MCSafetyInv == SafetyInv

====
