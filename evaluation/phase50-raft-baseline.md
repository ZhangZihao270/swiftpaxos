# Phase 50 Vanilla Raft Baseline Results

## Purpose

Baseline measurement for vanilla Raft (all-strong, weakRatio=0).
Used to validate Phase 50 success criterion: Raft-HT throughput >= Raft at all thread counts.

## Environment

| Parameter        | Value                                      |
|------------------|--------------------------------------------|
| Replicas         | 3 (130.245.173.101, .103, .104)            |
| Clients          | 3 (co-located with replicas)               |
| Network Delay    | 25ms one-way (50ms RTT), application-level |
| Requests/Client  | 10,000                                     |
| Pendings         | 15                                         |
| Pipeline         | true                                       |
| Weak Ratio       | 0% (all strong)                            |
| Strong Writes    | 10%                                        |
| Command Size     | 100 bytes                                  |
| Batch Delay      | 150us                                      |
| Date             | 2026-03-02                        |

## Results

| Threads | Throughput | S-Avg  | S-Med  | S-P99  |
|--------:|-----------:|-------:|-------:|-------:|
| 2       |    1360.77 |  68.48 |  68.40 |  78.77 |
| 4       |    2715.68 |  68.61 |  68.52 |  78.81 |
| 8       |    5418.43 |  68.75 |  68.63 |  78.96 |
| 16      |    9975.72 |  74.17 |  71.39 | 112.05 |
| 32      |   17780.60 |  82.34 |  79.73 | 130.06 |
| 64      |    SKIPPED |    N/A |    N/A |    N/A |
| 96      |       0.00 | 173.50 | 174.70 | 255.38 |

## Notes

- Vanilla Raft uses 2-RTT for all operations: leader appends, replicates, waits for majority, then replies.
- Expected S-Med ~68ms at low thread counts (matching Phase 49 baseline).
- This baseline establishes the performance floor that Raft-HT (post-fix) should exceed.
- **64-thread run SKIPPED** due to high cluster load (.104 at load=5.75).
- **96-thread run FAILED** with 0.00 throughput due to election storms (root cause: unbounded batch size in handlePropose).

## Phase 51 Update (2026-03-03)

After applying the `maxBatchSize=256` fix in Phase 51.1, the previously-failing high-concurrency runs were re-attempted:

| Threads | Throughput | S-Avg  | S-Med  | S-P99  | Status |
|--------:|-----------:|-------:|-------:|-------:|--------|
| 64      |    3546.94 |  51.23 |  51.28 |  53.48 | ⚠️ Anomaly (unexpectedly low, similar to 2t/4t) |
| 96      |   54012.64 | 112.37 |  99.81 | 297.56 | ✅ SUCCESS! Fix validated |

**Key Results**:
- 96-thread run completed successfully with 54K ops/sec (Phase 50 had 0.00 ops/sec timeout)
- Election storm issue resolved by capping batch size
- 64-thread result appears to be a measurement anomaly (needs investigation)
- See `evaluation/phase51-raft-baseline.md` for full Phase 51 results
