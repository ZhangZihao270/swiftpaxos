# TAO-Like Benchmark Design

## Motivation

Our primary evaluation uses a YCSB-like benchmark with simple GET/PUT operations.
To demonstrate the applicability of hybrid consistency to real-world workloads, we
design a benchmark inspired by Facebook's TAO social graph store [Bronson et al.,
ATC'13], with workload parameters cross-referenced against NCC [Lu et al., OSDI'23].

TAO is a natural fit for hybrid consistency evaluation because:
- It is overwhelmingly read-dominated (99.8% reads, 0.2% writes)
- Reads are served from cache (eventual/causal consistency) in production
- Writes are routed through the leader (linearizable)
- A small fraction of reads ("critical reads") require linearizable freshness
- Workload includes both point reads and range reads (association queries)

## TAO Operation Breakdown

### Write Operations (0.2% of total requests) -- All Linear

| TAO Operation  | % of writes | Our System | Consistency |
|----------------|-------------|------------|-------------|
| assoc_add      | 52.5%       | PUT        | Linear      |
| obj_update     | 20.7%       | PUT        | Linear      |
| obj_add        | 16.5%       | PUT        | Linear      |
| assoc_del      | 8.3%        | PUT        | Linear      |
| obj_delete     | 2.0%        | PUT        | Linear      |
| assoc_change   | 0.9%        | PUT        | Linear      |

All writes are linear because TAO routes writes through the leader
for synchronous persistence in MySQL.

### Read Operations (99.8% of total requests)

| TAO Operation    | % of reads | Our System     | Consistency | Rationale                                   |
|------------------|------------|----------------|-------------|---------------------------------------------|
| assoc_range      | 40.9%      | SCAN (1-1K keys) | Causal   | Browsing timelines, comment lists           |
| obj_get          | 28.9%      | GET            | Mostly causal, ~5% linear | Viewing profiles (causal); checking own privacy settings or credentials (linear) |
| assoc_get        | 15.7%      | GET            | Causal      | Checking relationship between two objects   |
| assoc_count      | 11.7%      | GET            | Causal      | Friend count, like count -- approximate OK  |
| assoc_time_range | 2.8%       | SCAN           | Causal      | Time-range edge queries, similar to assoc_range |

**Critical reads (linear):** TAO's paper mentions critical reads are proxied to the
master region for freshness, used for "authentication processes so that replication lag
doesn't allow use of stale credentials." Critical reads are not tied to a specific
operation type; any point read may occasionally require linear consistency. We model
this as ~5% of all point reads being linear.

## Configuration Parameters

```
# Write ratio
writes:      1          # ~1% (TAO: 0.2%, rounded to integer minimum)

# Consistency split
weakRatio:   95         # 95% of operations are causal
weakWrites:  0          # All causal operations are reads (no causal writes)

# Within causal reads: SCAN vs GET
scanRatio:   44         # 43.7% of reads are SCAN
                        #   assoc_range (40.9%) + assoc_time_range (2.8%)
                        # 56.3% of reads are GET
                        #   obj_get (28.9%) + assoc_get (15.7%) + assoc_count (11.7%)

# SCAN length
scanCount:   1-1000     # Range of keys per SCAN, following NCC [OSDI'23]
                        # Distribution: Zipfian (most scans are short, few are long)

# Access distribution
zipfSkew:    0.8        # Following NCC [OSDI'23] and LinkBench

# Key space
keySpace:    1000000    # 1M keys, following NCC [OSDI'23]
```

### Derived Operation Mix (per 1000 requests)

| Category       | Op   | Consistency | Count | % of total |
|----------------|------|-------------|-------|------------|
| Write          | PUT  | Linear      | 10    | 1.0%       |
| Critical read  | GET  | Linear      | 40    | 4.0%       |
| Point read     | GET  | Causal      | 527   | 52.7%      |
| Range read     | SCAN | Causal      | 423   | 42.3%      |
| **Total**      |      |             | 1000  | 100%       |

## Implementation Plan

### Code Changes Required

1. **config/config.go**: Add two new parameters
   - `ScanRatio int` -- percentage of causal reads that are SCAN (default 0)
   - `ScanCount int` -- max number of keys per SCAN (default 20)

2. **client/hybrid.go**: Modify the WeakRead branch in the benchmark loop
   - With probability `scanRatio/100`, send `SendScan(key, count)` instead of `SendRead(key)`
   - `count` drawn from Zipf distribution over [1, scanCount]

3. **No protocol changes needed** -- SCAN as causal read goes through the existing
   local-replica read path, no consensus required.

4. **New config file**: `configs/exp-tao.conf` with the parameters above.

5. **New experiment script**: `scripts/eval-exp-tao.sh` to run all protocols.

6. **New plot script**: `scripts/plot-exp-tao.py` for the TAO workload figure.

## Paper Text

### For the Evaluation Section

> **TAO-inspired workload.**
> To evaluate hybrid consistency under a realistic workload, we design a benchmark
> inspired by Facebook's TAO [Bronson et al., ATC'13], a social graph store that
> serves over 99.8% reads in production. We adopt the workload parameters from
> Lu et al. [NCC, OSDI'23]: 0.2% writes, Zipfian access distribution with
> alpha = 0.8, and read transactions accessing 1 to 1K keys.
>
> We map TAO's operations to our hybrid consistency model as follows.
> **Writes** (0.2% of requests) are all linear, as TAO routes writes through the
> leader for synchronous persistence. **Reads** are partitioned into linear and
> causal: TAO serves the vast majority of reads from its follower cache tier
> (eventual consistency), which we model as causal reads. A small fraction of
> reads (~5%) are linear, modeling TAO's critical reads that are proxied to the
> master region for freshness-sensitive operations such as authentication
> [Bronson et al., ATC'13]. Among causal reads, 44% are range reads (SCAN)
> corresponding to TAO's `assoc_range` and `assoc_time_range` operations, and
> 56% are point reads (GET) corresponding to `obj_get`, `assoc_get`, and
> `assoc_count`. Table X summarizes the workload parameters.
>
> This workload highlights the strengths of hybrid consistency: the dominant
> causal reads (95% of operations) benefit from low-latency local-replica
> serving, while the small fraction of linear operations (writes and critical
> reads) maintain strong consistency guarantees without impacting causal read
> performance (the T property).

### For a Table in the Paper

> | Parameter | Value | Source |
> |-----------|-------|--------|
> | Write fraction | 0.2% | TAO [ATC'13] |
> | Linear read fraction | ~5% | TAO critical reads [ATC'13] |
> | Causal read fraction | ~95% | TAO cache reads [ATC'13] |
> | SCAN ratio (of reads) | 44% | TAO op breakdown [ATC'13] |
> | Keys per SCAN | 1--1K | NCC [OSDI'23] |
> | Key access distribution | Zipf, alpha=0.8 | NCC [OSDI'23] |
> | Key space | 1M | NCC [OSDI'23] |
> | Value size | 100B | -- |

## References

- Bronson et al., "TAO: Facebook's Distributed Data Store for the Social Graph," USENIX ATC 2013.
- Lu et al., "NCC: Natural Concurrency Control for Strictly Serializable Datastores by Avoiding the Timestamp-Inversion Pitfall," USENIX OSDI 2023.
- Armstrong et al., "LinkBench: A Database Benchmark Based on the Facebook Social Graph," SIGMOD 2013.
