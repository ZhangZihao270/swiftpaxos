# TLA+ Model Checking Results (5 Replicas)

All models: 5 replicas, 2 clients, 2 keys, 2 values, BFS mode, 64 cores, TLC 2026.03.16

## Summary Table

| Protocol | Spec | Workers | Heap | Depth | States Generated | Distinct States | Queue Remaining | Runtime | Invariants Checked | Violations | Status |
|----------|------|---------|------|-------|-----------------|-----------------|-----------------|---------|-------------------|------------|--------|
| Raft-HT | RaftHT.tla | 20 | 19 GB | 24 | 6.49 B | 1.75 B | 997 M | ~26h 52m | TypeInv, SafetyInv | 0 | Killed (no error) |
| CURP-HT | CurpHT.tla | 15 | 13 GB | 26 | 7.86 B | 1.06 B | 469 M | ~26h 47m | TypeInv, SafetyInv | 0 | Killed (no error) |
| CURP-HO | CurpHO.tla | 15 | 13 GB | 20 | 2.60 B | 439 M | 270 M | ~16h 1m | TypeInv, SafetyInv | 0 | Disk full |
| EPaxos-HO | EPaxosHO_Orca_5r.tla | 20 | 19 GB | 10 | 5.36 B | 1.81 B | 1.58 B | ~23h 21m | TypeOK, Nontriviality, Consistency, Stability, SameSessionCausality, GetFromCausality, GlobalOrderingOfWrite, GlobalOrderingOfRead, RealTimeOrderingOfStrong, OrcaLinearizabilityInv, OrcaCausalConsistencyInv, OrcaHybridCompatibilityInv, OrcaConvergence | 0 | Killed (no error) |

## Key Observations

- **No property violations found in any protocol.**
- Raft-HT and CURP-HT ran for ~27 hours each (started 2026-03-26 14:48–14:53, killed 2026-03-27 17:40).
- CURP-HO ran ~16 hours before hitting disk space limit (started 2026-03-27 18:12, errored 2026-03-28 10:13).
- EPaxos-HO Orca ran ~23 hours (started 2026-03-27 18:11, killed 2026-03-28 17:32); two TLC instances ran concurrently writing to the same log — the larger instance explored 5.36B states.
- EPaxos-HO checks 13 invariants (including hybrid consistency properties); the other three check 2 each.
- None of the runs completed exhaustively (all had states remaining in the queue when stopped).

## Run Details

### Raft-HT (5r)
- **Start**: 2026-03-26 14:48:41
- **End**: 2026-03-27 17:40:47 (killed)
- **Config**: MC_RaftHT_5r.cfg — INVARIANT TypeInv, SafetyInv

### CURP-HT (5r)
- **Start**: 2026-03-26 14:53:45
- **End**: 2026-03-27 17:40:53 (killed)
- **Config**: MC_CurpHT_5r.cfg — INVARIANT TypeInv, SafetyInv

### CURP-HO (5r)
- **Start**: 2026-03-27 18:12:25
- **End**: 2026-03-28 10:13:33 (disk full: `No space left on device`)
- **Config**: MC_CurpHO_5r.cfg — INVARIANT TypeInv, SafetyInv

### EPaxos-HO Orca (5r)
- **Start**: 2026-03-27 18:11:41
- **End**: 2026-03-28 17:32:55 (killed)
- **Config**: MC_EPaxosHO_Orca_5r.cfg — 13 invariants (see table above)
- **Note**: Two TLC processes (pid 135359, 135863) ran concurrently on the same model, both writing to the same log file.
