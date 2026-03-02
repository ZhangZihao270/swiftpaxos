# Phase 44 Evaluation Results

## Changes Applied (since Phase 42 baseline)

### Phase 43 (retained):
1. **Split handleMsgs** (43.2c): Separate goroutines for strong/weak reply processing
2. **BoundReplica filtering** (43.3): Non-bound replicas skip MCausalReply

### Phase 44 (new):
3. **sendMsgToAll writer race fix** (44.3): Use `sendMsgSafe` with per-replica mutex
4. **Priority fast-path removed** (44.4): Eliminated run loop starvation risk

## Environment

| Parameter        | Value                                      |
|------------------|--------------------------------------------|
| Replicas         | 3 (130.245.173.101, .103, .104)            |
| Clients          | 3 (co-located with replicas)               |
| Network Delay    | 25ms one-way (50ms RTT), application-level |
| Requests/Client  | 10,000                                     |
| Pendings         | 15                                         |
| Pipeline         | true                                       |
| Weak Ratio       | 50%                                        |
| Weak Writes      | 10%                                        |
| Strong Writes    | 10%                                        |
| Command Size     | 100 bytes                                  |
| Batch Delay      | 150us                                      |
| Date             | 2026-03-02                        |

## Results

| Threads | Throughput | S-Avg | S-Med | S-P99 | W-Avg | W-Med | W-P99 | WW-P99 | WR-P99 | SendAll-P99 |
|--------:|-----------:|------:|------:|------:|------:|------:|------:|-------:|-------:|------------:|
| 2       | 1304.51    | 108.26 | 68.36 | 1631.37 | 14.38 | 0.30  | 101.02 | 101.02 | 101.02 | N/A         |
| 4       | 1949.92    | 97.61 | 67.78 | 1579.88 | 0.29  | 0.26  | 0.81  | 0.76   | 0.81   | N/A         |
| 4       | 2003.97    | 95.70 | 67.73 | 1415.17 | 0.29  | 0.26  | 0.89  | 0.79   | 0.91   | N/A         |
| 4       | 2086.53    | 91.97 | 67.95 | 1052.30 | 0.29  | 0.26  | 0.87  | 0.73   | 0.88   | N/A         |
| 8       | 1711.44    | 68.76 | 51.68 | 1062.16 | 0.35  | 0.31  | 0.87  | 0.77   | 0.88   | N/A         |
| 16      | 2165.97    | 92.24 | 67.98 | 1052.33 | 0.27  | 0.25  | 0.88  | 0.77   | 0.90   | N/A         |
| 32      | 2098.08    | 91.49 | 67.88 | 1061.53 | 0.28  | 0.25  | 0.87  | 0.69   | 0.87   | N/A         |
| 64      | 1938.61    | 96.60 | 67.65 | 1684.42 | 0.28  | 0.26  | 0.79  | 0.67   | 0.80   | N/A         |
| 96      | 1773.38    | 108.35 | 84.27 | 101.09 | 33.07 | 33.64 | 101.09 | 101.06 | 101.10 | N/A         |

## Comparison with Phase 42 Reference (2026-02-19)

| Threads | Phase 42 Throughput | Phase 44 Throughput | Phase 42 W-P99 | Phase 44 W-P99 |
|--------:|--------------------:|--------------------:|---------------:|---------------:|
| 2       | 3,551               |                     | 0.86ms         |                |
| 4       | 4,109               |                     | 100.96ms       |                |
| 8       | 14,050              |                     | 2.62ms         |                |
| 16      | 8,771               |                     | 100.95ms       |                |
| 32      | 30,339              |                     | 100.38ms       |                |
| 64      | 34,797              |                     | 102.51ms       |                |
| 96      | 71,595              |                     | 119.61ms       |                |

(Fill in Phase 44 values from the results above)

## Analysis

WW-P99 = Weak Write P99, WR-P99 = Weak Read P99, SendAll-P99 = sendMsgToAll duration P99
All latencies in ms.

### Key Questions Answered:
1. **Throughput scaling**: Does throughput scale with thread count? (Compare with Phase 42 reference)
2. **W-P99 at 4 threads**: Is it still ~100ms? If so, is it Weak Write or Weak Read?
3. **W-P99 at 16 threads**: Does Phase 43 improvement (1.08ms) hold?
4. **sendMsgToAll blocking**: Is send duration > 10ms at any thread count?
