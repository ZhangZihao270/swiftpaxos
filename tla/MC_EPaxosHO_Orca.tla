---- MODULE MC_EPaxosHO_Orca ----
\* Model checking configuration for EPaxos-HO (Orca spec).
\* Ported from Orca/HybridProtocol_TLA.toolbox/Model_Strong_Causal/

EXTENDS EPaxosHO_Orca_Properties, TLC

\* ============================================================================
\* Constant Overrides
\* ============================================================================

MCMaxBallot == 3
MCReplicas == {1, 2, 3}
MCConsistencyLevel == {"strong", "causal"}
MCCtxId == {1}
MCKeys == {"x", "y"}
MCCommands == {
    [op |-> [key |-> "x", type |-> "w"]],
    [op |-> [key |-> "y", type |-> "r"]]
}

====
