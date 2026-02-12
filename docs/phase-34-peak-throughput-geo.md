# Phase 34: Peak Throughput Under Geo-Setting Latency

## Overview

This document reports the peak throughput results for CURP-HO (Hybrid Optimal) and CURP-HT (Hybrid Two-phase) protocols under simulated geo-replication latency (50ms RTT).

## Experiment Setup

- **Cluster**: 3 replicas, 3 client servers
- **Machines**: 3 physical servers (130.245.173.{101, 102, 104})
- **Latency injection**: 25ms one-way application-level delay (50ms RTT)
- **Workload**: 50% weak/causal, 50% strong; 10% writes each; keySpace=1M, zipfSkew=0.99
- **Topology**:
  - replica0 (leader) on .101, co-located with client2
  - replica1 on .102, co-located with client0
  - replica2 on .104, co-located with client1

## Optimal Configurations

| Parameter | CURP-HO | CURP-HT |
|-----------|---------|---------|
| clientThreads | 32 | 32 |
| pendings | 15 | 20 |
| batchDelayUs | 150 | 50 |
| maxDescRoutines | 500 | 500 |

## Peak Throughput Validation (5 iterations)

### CURP-HO Raw Results

| Run | Throughput (ops/sec) | Strong Median (ms) | Strong P99 (ms) | Weak Median (ms) | Weak P99 (ms) |
|-----|---------------------|--------------------|-----------------|--------------------|----------------|
| 1 | 30,538 | 50.83 | 105.20 | 25.43 | 1,987.34 |
| 2 | 30,588 | 50.82 | 98.73 | 25.42 | 2,178.84 |
| 3 | 30,594 | 50.79 | 103.02 | 25.40 | 2,062.59 |
| 4 | 30,526 | 50.79 | 95.57 | 25.41 | 2,125.86 |
| 5 | 30,575 | 50.84 | 116.76 | 25.46 | 2,070.18 |

### CURP-HT Raw Results

| Run | Throughput (ops/sec) | Strong Median (ms) | Strong P99 (ms) | Weak Median (ms) | Weak P99 (ms) |
|-----|---------------------|--------------------|-----------------|--------------------|----------------|
| 1 | 38,419 | 58.09 | 129.32 | 26.36 | 127.07 |
| 2 | 40,552 | 55.57 | 157.10 | 25.72 | 103.77 |
| 3 | 37,934 | 60.50 | 177.20 | 25.52 | 80.29 |
| 4 | 39,944 | 59.99 | 127.19 | 25.40 | 86.62 |
| 5 | 36,291 | 66.16 | 172.96 | 25.86 | 108.26 |

## Final Comparison Table

| Metric | CURP-HO | CURP-HT |
|--------|---------|---------|
| **Peak throughput (avg)** | **30,564 ops/sec** | **38,628 ops/sec** |
| Peak throughput (min) | 30,526 | 36,291 |
| Peak throughput (max) | 30,594 | 40,552 |
| Throughput stddev | 31 | 1,690 |
| Throughput CV | 0.10% | 4.37% |
| Strong median latency | 50.81ms | 60.06ms |
| Weak median latency | 25.42ms | 25.77ms |
| Strong P99 latency | 103.86ms | 152.75ms |
| Weak P99 latency | 2,084.96ms | 101.20ms |
| Best clientThreads | 32 | 32 |
| Best pendings | 15 | 20 |
| Best batchDelayUs | 150 | 50 |

## Key Findings

### Throughput
- **CURP-HT achieves 1.26x higher peak throughput** than CURP-HO (38.6K vs 30.6K ops/sec).
- CURP-HO has remarkably stable throughput (CV=0.10%), while CURP-HT varies more (CV=4.37%).
- The throughput gap is driven by CURP-HO's asymmetric client distribution: the non-leader-colocated client (client1 on .104) achieves only ~6.3K ops/sec vs ~12K for the other two, because it must broadcast MCausalPropose to all replicas (including a round-trip to the leader on .101).

### Latency
- **CURP-HO has lower strong command latency** (50.81ms median vs 60.06ms for CURP-HT). This is because CURP-HO's strong commands can be committed in 1 RTT via the fast path, while CURP-HT requires 2 RTTs for strong commands.
- **Weak command median latency is similar** (~25.4-25.8ms), both achieving ~1 RTT for weak/causal commands.
- **CURP-HO has very high weak P99 latency** (~2,085ms) due to the broadcast pattern creating contention under high load. CURP-HT's weak P99 is much lower (~101ms).

### Scaling Behavior
- CURP-HT scales evenly across clients (each ~12-13K ops/sec), since all clients communicate only with the leader.
- CURP-HO creates an asymmetric load pattern where clients co-located with the leader (client2) or non-leader replicas that are close (client0) perform well, but distant clients (client1) bottleneck.

### Trade-offs
| Aspect | CURP-HO Advantage | CURP-HT Advantage |
|--------|-------------------|-------------------|
| Strong latency | 50.81ms (1 RTT fast path) | 60.06ms (2 RTT) |
| Weak P99 latency | - | 101ms vs 2,085ms |
| Peak throughput | - | 1.26x higher |
| Throughput stability | CV=0.10% | - |
| Client balance | - | Symmetric load |

## Experiment History (Phase 34.1-34.8)

| Phase | Description | Key Result |
|-------|-------------|------------|
| 34.1 | CURP-HO baseline (2×32, no latency tuning) | 17,845 ops/sec |
| 34.2 | CURP-HT baseline (2×32, no latency tuning) | 17,805 ops/sec |
| 34.3 | Thread scaling sweep (2×{2,4,8,10,16,24,32}) | Linear scaling up to 32 threads |
| 34.4 | Pipeline depth sweep (pendings 5-30) | HO optimal=15, HT optimal=20 |
| 34.5 | 3-client scaling | HO: 30.5K (+71%), HT: 39.2K (+120%) |
| 34.6 | Bug fix: leader Fatal on causal commands | Fixed unsyncCausal double-call on leader |
| 34.7 | Batch delay tuning under latency | Insensitive; HO=150μs, HT=50μs |
| 34.8 | Peak throughput validation (5 iterations) | HO: 30.6K, HT: 38.6K (1.26x) |
