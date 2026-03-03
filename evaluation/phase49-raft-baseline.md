# Phase 49 Vanilla Raft Baseline Results

## Purpose

Baseline measurement for vanilla Raft (all-strong, weakRatio=0).
Used to validate Raft-HT Transparency: strong ops should have identical latency.

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
| 2       |    1361.60 |  68.45 |  68.38 |  78.68 |
| 4       |    2715.68 |  68.61 |  68.52 |  78.88 |
| 8       |    5420.19 |  68.74 |  68.64 |  78.86 |
| 16      |    9949.86 |  74.33 |  71.33 | 111.71 |
| 32      |   17647.93 |  82.85 |  81.00 | 127.87 |
| 64      |   22340.84 | 129.22 | 131.22 | 212.07 |
| 96      |       0.00 | 190.90 | 189.87 | 291.79 |

## Notes

- Vanilla Raft uses 2-RTT for all operations: leader appends, replicates, waits for majority, then replies.
- Expected S-Med ~100ms at low thread counts (2 x 50ms RTT).
- This baseline establishes the performance floor that Raft-HT should match for strong ops.
