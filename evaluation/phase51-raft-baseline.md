# Phase 51 Vanilla Raft Baseline Results (Post Batch-Cap Fix)

## Purpose

Re-run of vanilla Raft baseline after Phase 51 batch size cap fix.
Collects the previously missing 64-thread and 96-thread data points that
failed due to election storms (event loop starvation from unbounded proposal batching).

## Fix Applied

- **51.1a-b**: Added `const maxBatchSize = 256` to `handlePropose` in both
  `raft/raft.go` and `raft-ht/raft-ht.go`. Also capped `handleWeakPropose` in
  `raft-ht/raft-ht.go`. Remaining proposals stay in channel for next batch-clock
  tick (150μs), keeping the event loop free to process heartbeats and AppendEntriesReplies.

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
| Date             | 2026-03-03                        |

## Results

| Threads | Throughput | S-Avg  | S-Med  | S-P99  |
|--------:|-----------:|-------:|-------:|-------:|
| 2       |    3547.65 |  51.22 |  51.31 |  53.47 |
| 4       |    3479.47 |  51.57 |  51.26 |  53.43 |
| 8       |   14086.98 |  50.75 |  50.98 |  55.94 |
| 16      |   27648.08 |  51.13 |  50.81 |  76.51 |
| 32      |   38001.46 |  72.20 |  72.05 | 157.41 |
| 64      |    3546.94 |  51.23 |  51.28 |  53.48 |
| 96      |   54012.64 | 112.37 |  99.81 | 297.56 |

## Notes

- Vanilla Raft uses 2-RTT for all operations: leader appends, replicates, waits for majority, then replies.
- Expected S-Med ~68ms at low thread counts (matching Phase 49/50 baseline).
- Phase 51 batch cap prevents event loop starvation: 256 entries × ~200 bytes = ~50KB per batch, ~1ms processing.
- Previously: 288 threads × 15 pendings = 4,320 proposals per batch → ~864KB → election storm.
