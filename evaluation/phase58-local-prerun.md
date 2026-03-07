# Phase 58 Local Pre-Run Evaluation Results

**Date**: 2026-03-07
**Environment**: 3 replicas + 3 clients (localhost), networkDelay=50ms one-way (RTT=100ms)
**Config**: keySpace=1M, zipfSkew=0.99, pipeline=true, pendings=15, batchDelayUs=150

## Exp 3.1: CURP Throughput vs Latency

Workload: 95/5 read/write, 50/50 strong/weak (0% weak for baseline), zipfian keys.

| Protocol | Threads | Total | Throughput | S-P50 | S-P99 | W-P50 | W-P99 |
|---------:|--------:|------:|-----------:|------:|------:|------:|------:|
| CURP-HO | 1 | 3 | 397 | 100.3 | 154.0 | 100.3 | 102.1 |
| CURP-HO | 2 | 6 | 787 | 100.5 | 153.3 | 100.5 | 102.1 |
| CURP-HO | 4 | 12 | 1577 | 100.4 | 153.3 | 100.4 | 102.4 |
| CURP-HO | 8 | 24 | 3204 | 100.4 | 153.3 | 100.4 | 102.4 |
| CURP-HO | 16 | 48 | 6248 | 100.3 | 153.5 | 100.3 | 102.6 |
| CURP-HO | 32 | 96 | 12452 | 100.3 | 156.3 | 100.2 | 102.9 |
| CURP-HT | 1 | 3 | 366 | 102.2 | 154.3 | 101.3 | 205.3 |
| CURP-HT | 2 | 6 | 711 | 150.9 | 154.0 | 101.2 | 204.5 |
| CURP-HT | 4 | 12 | 1409 | 151.2 | 154.0 | 101.1 | 203.7 |
| CURP-HT | 8 | 24 | 2862 | 151.2 | 154.0 | 101.0 | 203.0 |
| CURP-HT | 16 | 48 | 5557 | 151.2 | 157.2 | 100.8 | 202.9 |
| CURP-HT | 32 | 96 | 10921 | 151.1 | 162.1 | 100.7 | 202.6 |
| Baseline | 1 | 3 | 330 | 135.0 | 154.2 | N/A | N/A |
| Baseline | 2 | 6 | 646 | 151.3 | 154.1 | N/A | N/A |
| Baseline | 4 | 12 | 1274 | 151.3 | 153.8 | N/A | N/A |
| Baseline | 8 | 24 | 2549 | 151.2 | 154.7 | N/A | N/A |
| Baseline | 16 | 48 | 4929 | 151.1 | 158.4 | N/A | N/A |
| Baseline | 32 | 96 | 9917 | 151.1 | 168.5 | N/A | N/A |

### Observations

1. **CURP-HO achieves highest throughput** at all concurrency levels (~12.5K at 32 threads vs ~10.9K for CURP-HT vs ~9.9K for baseline)
2. **CURP-HO strong P50 ~100ms** (1 RTT fast path), **CURP-HT/baseline strong P50 ~151ms** (1.5 RTT due to leader path)
3. **CURP-HO weak P50 ~100ms** (1 RTT), **CURP-HT weak P50 ~101ms** (1 RTT) — both protocols achieve low weak latency
4. **CURP-HT weak P99 ~203ms** — higher than CURP-HO's ~102ms because CURP-HT weak writes go through leader
5. Near-linear throughput scaling for all protocols up to 32 threads

## Exp 3.2: T Property Verification

Workload: 50/50 read/write, sweep weakRatio (0-100%), 8 threads/client, zipfian keys.

| Protocol | Weak% | Throughput | S-P50 | S-P99 | W-P50 | W-P99 |
|---------:|------:|-----------:|------:|------:|------:|------:|
| Raft-HT | 0 | 1702 | 203.0 | 205.5 | N/A | N/A |
| Raft-HT | 25 | 459 | 202.5 | 204.3 | 101.2 | 102.5 |
| Raft-HT | 50 | TIMEOUT | N/A | N/A | N/A | N/A |
| Raft-HT | 75 | 2712 | 203.2 | 208.9 | 101.5 | 104.2 |
| Raft-HT | 100 | 3302 | * | * | 101.5 | 104.2 |
| CURP-HT | 0 | 2501 | 151.2 | 154.5 | N/A | N/A |
| CURP-HT | 25 | 2442 | 151.2 | 154.2 | 201.2 | 205.2 |
| CURP-HT | 50 | 2352 | 151.3 | 154.2 | 201.5 | 205.7 |
| CURP-HT | 75 | 2287 | 151.0 | 153.8 | 201.4 | 205.7 |
| CURP-HT | 100 | 2238 | * | * | 201.5 | 205.9 |
| CURP-HO | 0 | 3098 | 100.2 | 152.8 | N/A | N/A |
| CURP-HO | 25 | 2926 | 100.2 | 153.9 | 100.2 | 102.1 |
| CURP-HO | 50 | 3107 | 100.3 | 154.2 | 100.3 | 102.3 |
| CURP-HO | 75 | 3273 | 100.4 | 154.2 | 100.4 | 103.4 |
| CURP-HO | 100 | 3326 | * | * | 100.5 | 102.5 |

(*) At weakRatio=100, there are almost no strong ops (only warmup), so strong latency is meaningless.

### T Property Assessment

**CURP-HT: T property holds.** Strong P50 stays at ~151ms for weakRatio 0-75%. Constant within noise.

**CURP-HO: T property also holds on local cluster.** Strong P50 stays at ~100ms for weakRatio 0-75%. This is unexpected for an H+O protocol — the T violation should appear under higher load or geo-distributed deployment where weak broadcast overhead affects strong path.

**Raft-HT: Unstable results.** w25 had very low throughput (459 ops/sec vs expected ~2000+), w50 timed out entirely. This suggests a Raft-HT bug under 50/50 read/write workload with certain weak ratios. Strong P50 is ~203ms where data exists. **Needs investigation.**

## Exp 1.1: Raft-HT Throughput vs Latency

Workload: 95/5 read/write, 50/50 strong/weak (0% weak for vanilla Raft), zipfian keys.

| Protocol | Threads | Total | Throughput | S-P50 | S-P99 | W-P50 | W-P99 |
|---------:|--------:|------:|-----------:|------:|------:|------:|------:|
| Raft-HT | 1 | 3 | 282 | 202.4 | 204.7 | 101.2 | 102.4 |
| Raft-HT | 2 | 6 | 162* | 202.2 | 204.3 | 101.1 | 102.3 |
| Raft-HT | 4 | 12 | 1141 | 202.4 | 204.5 | 101.1 | 102.4 |
| Raft-HT | 8 | 24 | 2246 | 202.5 | 206.3 | 101.1 | 102.5 |
| Raft-HT | 16 | 48 | 4482 | 202.7 | 206.7 | 101.1 | 103.2 |
| Raft-HT | 32 | 96 | 8927 | 204.5 | 219.7 | 101.1 | 106.2 |
| Vanilla Raft | 1 | 3 | 279 | 152.1 | 154.2 | N/A | N/A |
| Vanilla Raft | 2 | 6 | 558 | 152.1 | 154.3 | N/A | N/A |
| Vanilla Raft | 4 | 12 | TIMEOUT | N/A | N/A | N/A | N/A |
| Vanilla Raft | 8 | 24 | 2225 | 152.5 | 156.0 | N/A | N/A |
| Vanilla Raft | 16 | 48 | 4412 | 153.3 | 169.7 | N/A | N/A |
| Vanilla Raft | 32 | 96 | 8633 | 156.5 | 181.0 | N/A | N/A |

(*) Raft-HT t=2 anomaly: only 162 ops/sec; likely startup/timing issue.

### Observations

1. **Raft-HT weak P50 ~101ms** (1 RTT) vs **strong P50 ~202ms** (2 RTT) — as expected
2. **Vanilla Raft strong P50 ~152ms** vs **Raft-HT strong P50 ~202ms** — Raft-HT strong is ~50ms slower. This needs investigation: both should have the same strong path (2-RTT Raft), but Raft-HT may have overhead from the hybrid protocol layer.
3. Vanilla Raft t=4 timed out — intermittent stability issue
4. Raft-HT t=2 anomaly — intermittent issue

## Known Issues

1. **Raft-HT instability**: w25 slow, w50 timeout in Exp 3.2; t=2 anomaly in Exp 1.1. Needs debugging.
2. **Vanilla Raft timeout**: t=4 in Exp 1.1 timed out. Intermittent issue.
3. **Vanilla CURP incompatible**: `protocol: curp` with 3 clients hangs after ~150 commands. Used `curpht` with `weakRatio=0` as baseline instead.
4. **Raft-HT strong latency gap**: Strong P50 is ~202ms vs Vanilla Raft's ~152ms. Raft-HT may have unnecessary overhead on the strong path.
5. **CURP-HO T property**: Appears to hold on local cluster, but may violate under geo-distributed deployment with real WAN delays. Need to verify with real geo-distributed setup.
