# Phase 52 CURP Evaluation Results

## Optimizations Applied

Phase 52.1-52.4 optimizations to bring vanilla CURP into benchmark pipeline:
- **52.1**: SHARD_COUNT 32768 → 512 (cache-friendly, proven in Phase 18.6)
- **52.2**: MaxDescRoutines 100 → 10000 (remove goroutine serialization ceiling)
- **52.3**: Configurable batch delay (150μs optimal for throughput)
- **52.4**: Wire into HybridBufferClient for metric collection (weakRatio=0)

## Environment

| Parameter        | Value                                      |
|------------------|--------------------------------------------|
| Replicas         | 3 (130.245.173.101, .103, .104)            |
| Clients          | 3 (co-located with replicas)               |
| Network Delay    | 25ms one-way (50ms RTT), application-level |
| Requests/Client  | 10,000                                     |
| Pendings         | 15                                         |
| Pipeline         | true                                       |
| Weak Ratio       | 0% (CURP strong-only)                      |
| Strong Writes    | 10%                                        |
| Command Size     | 100 bytes                                  |
| Batch Delay      | 150us                                      |
| Date             | 2026-03-03                        |

## CURP Results

| Threads | Throughput | S-Avg  | S-Med  | S-P99  |
|--------:|-----------:|-------:|-------:|-------:|
| 2       |    1747.64 |  51.42 |  51.37 |  53.29 |
| 4       |    3497.45 |  51.39 |  51.30 |  53.72 |
| 8       |    6989.82 |  51.42 |  51.24 |  55.07 |
| 16      |   13361.84 |  53.90 |  50.87 | 185.08 |
| 32      |   21217.10 |  76.89 |  51.04 | 1479.84 |
| 64      |   30324.51 | 125.79 |  51.60 | 4747.18 |
| 96      |   31365.61 | 188.91 |  69.26 | 5006.97 |

## Unknown Client Message Errors

| Threads | Errors |
|--------:|-------:|
| 2       |      0 |
| 4       |      0 |
| 8       |      0 |
| 16      |      0 |
| 32      |      0 |
| 64      |      0 |
| 96      |      0 |

## Validation Against Phase 52 Success Criteria

### 1. go test ./curp/ -v passes with all existing tests

(Verified before benchmark — 3 batcher tests + 6 client tests, all pass)

### 2. go test ./... -count=1 passes (no regressions in other protocols)

(Verified before benchmark — all packages pass)

### 3. CURP benchmark completes at all 7 thread counts without timeout

(Check results table above — no SKIPPED or 0 throughput entries)

### 4. CURP throughput scales monotonically (no collapse like pre-fix Raft)

(Check throughput column — should increase with thread count, no sudden drops)

### 5. CURP S-Med ≈ CURP-HO/HT S-Med (~51ms at low load)

(Check S-Med at 2-8 threads — all share 1-RTT fast path, should be ~51-53ms)

### 6. Results recorded in evaluation/phase52-curp-results.md and orca/benchmark-2026-03-02.md updated

(This file is the evaluation; orca table update is Phase 52.6a)

## CURP vs Other Protocols (for context)

Reference values from orca/benchmark-2026-03-02.md:

| Threads | CURP-HO  | CURP-HT  | Raft-HT  | Raft     | CURP     |
|--------:|---------:|---------:|---------:|---------:|---------:|
| 6       |   23,836 |   21,635 |    2,315 |    1,361 | (fill)   |
| 12      |   34,706 |   33,168 |    4,599 |    2,708 | (fill)   |
| 24      |   44,313 |   41,758 |    9,145 |    5,388 | (fill)   |
| 48      |   49,154 |   46,632 |   14,523 |    8,980 | (fill)   |
| 96      |   51,836 |   50,342 |   16,071 |   14,151 | (fill)   |
| 192     |   48,779 |   47,532 |   32,501 |   17,781 | (fill)   |
| 288     |   40,597 |   39,456 |   36,999 |      N/A | (fill)   |

Expected: CURP throughput between Raft and CURP-HT (no weak ops overhead, but also no hybrid optimizations)

S-Med reference (96 threads, from orca table):
- CURP-HO: 51.92ms
- CURP-HT: 51.50ms
- Raft-HT: 85.36ms
- Raft: 84.32ms
- CURP: (fill) — should be ~51-53ms (same fast path as CURP-HO/HT)
