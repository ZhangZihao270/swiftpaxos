---- MODULE MC_EPaxosHO_Orca_5r ----
\* Model checking configuration for EPaxos-HO (Orca spec, 5 replicas).

EXTENDS EPaxosHO_Orca_5r_Properties, TLC

MCMaxBallot == 3
MCReplicas == {1, 2, 3, 4, 5}
MCConsistencyLevel == {"strong", "causal"}
MCCtxId == {1}
MCKeys == {"x", "y"}
MCCommands == {
    [op |-> [key |-> "x", type |-> "w"]],
    [op |-> [key |-> "y", type |-> "r"]]
}

====
