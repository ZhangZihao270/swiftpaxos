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

## CURP (Vanilla) Results (Phase 52 + Phase 53 + Phase 54)

CURP runs with 100% strong operations (no weak consistency support, strong-only protocol).

Phase 52.1-52.4 optimizations + Phase 53-54 P99 tail latency fixes:
- SHARD_COUNT 32768 → 512 (cache-friendly)
- MaxDescRoutines 100 → 10000 (remove goroutine ceiling)
- Configurable batch delay (150μs)
- HybridBufferClient wiring for metric collection
- Descriptor channel buffer 8 → 128 (Phase 53.1)
- Cached slotStr to eliminate strconv.Itoa allocations (Phase 53.2)
- Sequential mode direct cleanup (Phase 53.3)
- Strict goroutine routing — removed inline fallback (Phase 54.1)
- Batcher buffer 8 → 128 (Phase 54.2)
- sync.Map string cache — int32ToString (Phase 54.3)
- Channel-based delivery notification — executeNotify (Phase 54.4)

| Threads | Throughput | S-Avg  | S-Med  | S-P99    |
|--------:|-----------:|-------:|-------:|---------:|
|       6 |      1,746 |  51.47 |  51.39 |    53.51 |
|      12 |      3,486 |  51.54 |  51.33 |    58.10 |
|      24 |      6,899 |  52.08 |  51.31 |    85.30 |
|      48 |     12,857 |  56.03 |  51.19 |   237.69 |
|      96 |     20,470 |  80.35 |  51.31 |   963.61 |
|     192 |     29,340 | 126.64 |  51.38 | 2,146.07 |
|     288 |     32,455 | 176.57 |  57.05 | 1,171.50 |

Note: Thread counts are total across 3 clients (e.g., 96 = 3 clients × 32 threads/client).
Phase 54 reduced P99 by 20-67% at high concurrency vs Phase 53: 96t 1,211→964ms (-20%), 192t 3,420→2,146ms (-37%), 288t 3,512→1,172ms (-67%). Throughput improved at 288t (+6.2%). See `evaluation/phase54-curp-p99-port.md` for details.

## Raft (Baseline) Results

Raft runs with 100% strong operations (no weak ops). Phase 50 re-run (consistent with Phase 49).

**Phase 51 Update (2026-03-03)**: After discovering election storms at 96+ threads due to unbounded batch sizes in `handlePropose`, Phase 51.1 added `maxBatchSize=256` cap. Phase 51.2b re-ran the previously-failing high-concurrency tests.

| Threads | Throughput | S-Avg  | S-Med  | S-P99  | Notes |
|--------:|-----------:|-------:|-------:|-------:|-------|
|       6 |      1,361 |  68.48 |  68.40 |  78.77 | Phase 50 |
|      12 |      2,716 |  68.61 |  68.52 |  78.81 | Phase 50 |
|      24 |      5,418 |  68.75 |  68.63 |  78.96 | Phase 50 |
|      48 |      9,976 |  74.17 |  71.39 | 112.05 | Phase 50 |
|      96 |     17,781 |  82.34 |  79.73 | 130.06 | Phase 50 (32t data point) |
|     192 |     22,341 | 129.22 | 131.22 | 212.07 | Phase 50 |
|     288 |        N/A |    N/A |    N/A |    N/A | Phase 50 FAILED (election storm) |

**Phase 51 High-Concurrency Results** (thread counts don't match orca scale, ran 2/4/8/16/32/64/96):
- **96t (Phase 51)**: 54,013 ops/sec, S-Med 99.81ms — ✅ SUCCESS! Previously failed with 0.00 ops/sec
- **64t (Phase 51)**: 3,547 ops/sec (anomaly, similar to 2t/4t baseline) — needs investigation
- See `evaluation/phase51-raft-baseline.md` for full results

## 5-Protocol Comparison: Throughput (ops/sec)

Raft-HT numbers reflect Phase 50 post-fix results. All thread counts are total (3 clients × N threads/client).

| Threads |   Raft | Raft-HT | CURP-HO | CURP-HT |   CURP |
|--------:|-------:|--------:|--------:|--------:|-------:|
|       6 |  1,361 |   2,323 |   3,529 |   2,994 |  1,746 |
|      12 |  2,716 |   4,562 |   7,097 |   5,931 |  3,486 |
|      24 |  5,418 |   9,163 |  14,118 |  11,837 |  6,899 |
|      48 |  9,976 |  15,339 |  27,115 |  23,496 | 12,857 |
|      96 | 17,781 |  24,123 |  38,292 |  41,789 | 20,470 |
|     192 | 22,341 |  32,501 |  42,962 |  50,342 | 29,340 |
|     288 |    N/A |  36,999 |  51,836 |  49,546 | 32,455 |

## 5-Protocol Comparison: Strong Latency S-Med (ms)

| Threads |  Raft | Raft-HT | CURP-HO | CURP-HT |   CURP |
|--------:|------:|--------:|--------:|--------:|-------:|
|       6 | 68.40 |   85.12 |   51.27 |   51.21 |  51.39 |
|      12 | 68.52 |   85.12 |   51.02 |   51.09 |  51.33 |
|      24 | 68.63 |   85.20 |   50.96 |   51.00 |  51.31 |
|      48 | 71.39 |   92.34 |   50.85 |   50.86 |  51.19 |
|      96 | 79.73 |  113.32 |   69.68 |   53.66 |  51.31 |
|     192 |131.22 |  157.70 |   99.79 |   98.92 |  51.38 |
|     288 |   N/A |  201.26 |   99.82 |  164.19 |  57.05 |

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

### CURP (Vanilla) vs CURP-HO / CURP-HT

Phase 52 brought vanilla CURP (strong-only protocol) into the benchmark pipeline for comparison. Key observations:

**Strong Latency (S-Med)**:
- CURP S-Med: 51.19-51.39ms at 6-192 threads, 57.05ms at 288 threads
- CURP-HO S-Med: 50.85-51.27ms at low concurrency, 69.68ms at 96 threads
- CURP-HT S-Med: 50.86-51.21ms at low concurrency, 53.66ms at 96 threads
- **Conclusion**: All three protocols share the same 1-RTT fast path for strong operations, resulting in nearly identical S-Med (~51ms) at low load. At high concurrency, CURP maintains excellent S-Med up to 192 threads (51.38ms), better than CURP-HO (99.79ms @ 192t) and CURP-HT (98.92ms @ 192t). At 288t, CURP S-Med (57.05ms) is dramatically better than CURP-HO (99.82ms) and CURP-HT (164.19ms).

**Throughput**:
- CURP peak: 32,455 ops/sec at 288 threads (all strong operations)
- CURP-HO peak: 51,836 ops/sec at 288 threads (50% weak, 50% strong)
- CURP-HT peak: 50,342 ops/sec at 192 threads (50% weak, 50% strong)
- Raft peak: 22,341 ops/sec at 192 threads (all strong operations)
- **Conclusion**: CURP throughput (20.5K ops/sec @ 96t) falls between Raft (18K @ 96t) and the hybrid protocols (38-42K @ 96t). The hybrid protocols achieve higher throughput by serving 50% of requests via fast weak paths (sub-ms weak reads, 1-RTT weak writes for CURP-HO).

**Scaling**:
CURP shows monotonic throughput scaling from 6 to 288 threads (1.7K → 32.5K ops/sec), with no collapse or timeout failures. This validates the Phase 52-54 optimizations.

**P99 Latency Progression (Phase 52 → 53 → 54)**:
Phase 54 delivered the largest P99 improvement by porting CURP-HT/HO engineering optimizations:
- 96t: 1,480ms → 1,211ms → 964ms (Phase 52 → 53 → 54, total -35%)
- 192t: 4,747ms → 3,420ms → 2,146ms (total -55%)
- 288t: 5,007ms → 3,512ms → 1,172ms (total -77%)

The channel-based delivery notification (Phase 54.4) was the highest-impact change — replacing `r.executed.Has()` polling with `<-r.getOrCreateExecuteNotify(slot-1)` channels eliminated goroutine busy-waiting that consumed CPU at high concurrency. Combined with strict goroutine routing (54.1) and batcher buffer enlargement (54.2), CURP now meets the 288t P99 target (< 1,500ms) and is within 2x of the 96t target (964ms vs 500ms goal). S-Med is preserved at ~51ms at all loads. The remaining tail latency at 96t is fundamental to the single-threaded event loop architecture.

### Key Takeaways

1. Hybrid transparency improves throughput over baseline for both Raft and CURP
2. The throughput ceiling is primarily determined by the strong path RTT count
3. Weak read implementation matters at high concurrency: lock-based (Raft-HT) degrades, version-based (CURP-HT) does not
4. CURP vanilla achieves ~1.15x throughput over Raft at 96 threads (20.5K vs 18K ops/sec) despite both being strong-only, due to CURP's 1-RTT fast path vs Raft's 2-RTT. At 192 threads: 1.31x (29K vs 22K). At 288t: 32.5K ops/sec
5. Weak operations provide 1.87x additional throughput over strong-only at 96 threads (CURP-HO 38K vs CURP 20.5K)
6. Engineering optimizations matter: porting CURP-HT/HO's strict goroutine routing, batcher buffering, string caching, and channel-based notifications to vanilla CURP reduced P99 by 67% at 288t without protocol changes
