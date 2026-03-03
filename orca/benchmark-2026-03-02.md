# Benchmark Results (2026-03-02)

## Configuration

| Parameter      | Value                                      |
|----------------|--------------------------------------------|
| Replicas       | 3 (130.245.173.101, .103, .104)            |
| Clients        | 3 (co-located with replicas)               |
| Network Delay  | 25ms one-way (50ms RTT), application-level |
| Requests/Client| 10,000 per thread                          |
| Pendings       | 15                                         |
| Pipeline       | true                                       |
| Weak Ratio     | 50%                                        |
| Weak Writes    | 10%                                        |
| Strong Writes  | 10%                                        |
| Command Size   | 100 bytes                                  |
| Batch Delay    | 150us                                      |

## CURP-HO Results

| Threads | Throughput | S-Avg  | S-Med  | S-P99  | W-Avg | W-Med | W-P99  | WW-P99 | WR-P99 |
|--------:|-----------:|-------:|-------:|-------:|------:|------:|-------:|-------:|-------:|
|       6 |      3,529 |  51.60 |  51.27 |  53.43 |  0.21 |  0.19 |   0.80 |   0.83 |   0.81 |
|      12 |      7,097 |  50.77 |  51.02 |  53.22 |  0.27 |  0.22 |   1.64 |   1.00 |   1.71 |
|      24 |     14,118 |  50.74 |  50.96 |  54.74 |  0.30 |  0.23 |   2.20 |   1.09 |   2.30 |
|      48 |     27,115 |  51.58 |  50.85 |  99.21 |  0.44 |  0.25 |   6.61 |   2.01 |   6.90 |
|      96 |     38,292 |  70.71 |  69.68 | 165.98 |  1.49 |  0.44 |  39.41 |  13.30 |  43.28 |
|     192 |     42,962 | 105.77 |  99.79 | 257.65 | 10.72 |  4.55 |  99.81 |   9.17 |  99.83 |
|     288 |     51,836 | 113.38 |  99.82 | 300.50 | 33.15 | 38.32 | 100.33 |  33.03 | 100.37 |

## CURP-HT Results

| Threads | Throughput | S-Avg  | S-Med  | S-P99  | W-Avg | W-Med | W-P99  | WW-P99 | WR-P99 |
|--------:|-----------:|-------:|-------:|-------:|------:|------:|-------:|-------:|-------:|
|       6 |      2,994 |  51.29 |  51.21 |  53.23 |  9.47 |  0.18 | 104.22 | 105.18 |   0.72 |
|      12 |      5,931 |  51.21 |  51.09 |  53.08 |  9.55 |  0.21 | 103.61 | 105.06 |   0.96 |
|      24 |     11,837 |  51.18 |  51.00 |  53.28 |  9.53 |  0.23 | 103.09 | 104.69 |   1.35 |
|      48 |     23,496 |  51.19 |  50.86 |  57.69 |  9.62 |  0.25 | 102.75 | 112.56 |   2.01 |
|      96 |     41,789 |  58.86 |  53.66 | 120.33 | 10.03 |  0.28 | 106.23 | 150.89 |   3.37 |
|     192 |     50,342 |  99.48 |  98.92 | 205.45 | 15.35 |  0.39 | 323.26 | 429.28 |   6.59 |
|     288 |     49,546 | 163.40 | 164.19 | 313.48 | 12.13 |  0.38 | 154.47 | 265.55 |   4.74 |

## Raft-HT Results (Phase 50 Post-Fix)

Phase 50.1-50.3 optimizations: RWMutex-based weak reads, batched weak writes, reduced event loop.

| Threads | Throughput | S-Avg  | S-Med  | S-P99   | W-Avg  | W-Med  | W-P99   | WW-P99  | WR-P99  |
|--------:|-----------:|-------:|-------:|--------:|-------:|-------:|--------:|--------:|--------:|
|       6 |      2,323 |  85.18 |  85.12 |  103.90 |   3.78 |   0.15 |   51.66 |   52.19 |    0.48 |
|      12 |      4,562 |  85.19 |  85.12 |  103.85 |   3.91 |   0.16 |   51.62 |   52.13 |    0.56 |
|      24 |      9,163 |  85.31 |  85.20 |  104.12 |   3.90 |   0.14 |   51.66 |   52.20 |    0.63 |
|      48 |     15,339 |  94.76 |  92.34 |  136.06 |   5.44 |   0.73 |   65.79 |   79.59 |   22.72 |
|      96 |     24,123 | 113.20 | 113.32 |  170.86 |   9.80 |   4.05 |   81.62 |   98.02 |   48.58 |
|     192 |     32,501 | 156.10 | 157.70 |  256.59 |  22.35 |  14.65 |  141.67 |  182.90 |  128.94 |
|     288 |     36,999 | 199.58 | 201.26 |  335.13 |  34.82 |  25.38 |  188.86 |  227.49 |  171.64 |

## Raft (Baseline) Results

Raft runs with 100% strong operations (no weak ops). Phase 50 re-run (consistent with Phase 49).

| Threads | Throughput | S-Avg  | S-Med  | S-P99  |
|--------:|-----------:|-------:|-------:|-------:|
|       6 |      1,361 |  68.48 |  68.40 |  78.77 |
|      12 |      2,716 |  68.61 |  68.52 |  78.81 |
|      24 |      5,418 |  68.75 |  68.63 |  78.96 |
|      48 |      9,976 |  74.17 |  71.39 | 112.05 |
|      96 |     17,781 |  82.34 |  79.73 | 130.06 |
|     192 |     22,341 | 129.22 | 131.22 | 212.07 |

## 4-Protocol Comparison: Throughput (ops/sec)

Raft-HT numbers reflect Phase 50 post-fix results.

| Threads |   Raft | Raft-HT | CURP-HO | CURP-HT |
|--------:|-------:|--------:|--------:|--------:|
|       6 |  1,361 |   2,323 |   3,529 |   2,994 |
|      12 |  2,716 |   4,562 |   7,097 |   5,931 |
|      24 |  5,418 |   9,163 |  14,118 |  11,837 |
|      48 |  9,976 |  15,339 |  27,115 |  23,496 |
|      96 | 17,781 |  24,123 |  38,292 |  41,789 |
|     192 | 22,341 |  32,501 |  42,962 |  50,342 |
|     288 |    N/A |  36,999 |  51,836 |  49,546 |

## 4-Protocol Comparison: Strong Latency S-Med (ms)

| Threads |  Raft | Raft-HT | CURP-HO | CURP-HT |
|--------:|------:|--------:|--------:|--------:|
|       6 | 68.40 |   85.12 |   51.27 |   51.21 |
|      12 | 68.52 |   85.12 |   51.02 |   51.09 |
|      24 | 68.63 |   85.20 |   50.96 |   51.00 |
|      48 | 71.39 |   92.34 |   50.85 |   50.86 |
|      96 | 79.73 |  113.32 |   69.68 |   53.66 |
|     192 |131.22 |  157.70 |   99.79 |   98.92 |

## 3-Protocol Comparison: Weak Write WW-P99 (ms)

| Threads | Raft-HT | CURP-HO | CURP-HT |
|--------:|--------:|--------:|--------:|
|       6 |   52.19 |    0.83 |  105.18 |
|      12 |   52.13 |    1.00 |  105.06 |
|      24 |   52.20 |    1.09 |  104.69 |
|      48 |   79.59 |    2.01 |  112.56 |
|      96 |   98.02 |   13.30 |  150.89 |
|     192 |  182.90 |    9.17 |  429.28 |

## 3-Protocol Comparison: Weak Read WR-P99 (ms)

| Threads | Raft-HT | CURP-HO | CURP-HT |
|--------:|--------:|--------:|--------:|
|       6 |    0.48 |    0.81 |    0.72 |
|      12 |    0.56 |    1.71 |    0.96 |
|      24 |    0.63 |    2.30 |    1.35 |
|      48 |   22.72 |    6.90 |    2.01 |
|      96 |   48.58 |   43.28 |    3.37 |
|     192 |  128.94 |   99.83 |    6.59 |

All latencies in milliseconds. Throughput in ops/sec.

- **S-Avg/Med/P99**: Strong operation latency (linearizable)
- **W-Avg/Med/P99**: Weak operation latency (all weak ops combined)
- **WW-P99**: Weak Write P99 latency
- **WR-P99**: Weak Read P99 latency
- Raft baseline runs 100% strong ops (no weak consistency support)
- Raft-HT/CURP-HO/CURP-HT run 50% weak ratio, 10% weak writes, 10% strong writes

## Analysis

### Raft-HT vs Raft Baseline

Raft-HT achieves 1.36-1.71x throughput over vanilla Raft at all concurrency levels. The throughput advantage comes from weak operations (50% of workload) bypassing the 2-RTT strong path:

- **Weak reads**: local state read with `stateMu.RLock()`, sub-ms at low concurrency
- **Weak writes**: 1-RTT leader-only apply with early reply, ~52ms WW-P99

Raft-HT S-Med (~85ms) is higher than Raft S-Med (~68ms) due to event loop contention between strong and weak paths.

### Raft-HT vs CURP-HO / CURP-HT

Raft-HT throughput is approximately 0.6-0.7x of CURP protocols at all concurrency levels. This is a fundamental protocol-level difference:

| Metric             | Raft-HT          | CURP-HO           | CURP-HT           |
|--------------------|-------------------|--------------------|--------------------|
| Strong path        | 2-RTT (Raft)      | 1-RTT (fast path)  | 1-RTT (fast path)  |
| Weak write         | 1-RTT leader-only | Leader-local apply | 2-RTT sync repl.   |
| Weak read          | Local RLock        | Local read         | Versioned snapshot  |
| Peak throughput    | 37K               | 52K                | 50K                |

- **Strong latency**: CURP S-Med ~51ms (1-RTT fast path) vs Raft-HT ~85ms (2-RTT). This is the primary throughput gap.
- **WW-P99**: CURP-HO < 1ms (local apply), Raft-HT ~52ms (1-RTT), CURP-HT ~105ms (2-RTT sync replication)
- **WR-P99**: Raft-HT best at low concurrency (0.48ms vs CURP-HO 0.81ms), but degrades at high concurrency due to RWMutex contention. CURP-HT maintains <7ms WR-P99 at all levels via versioned snapshots.

### Key Takeaways

1. Hybrid transparency improves throughput over baseline for both Raft and CURP
2. The throughput ceiling is primarily determined by the strong path RTT count
3. Weak read implementation matters at high concurrency: lock-based (Raft-HT) degrades, version-based (CURP-HT) does not
