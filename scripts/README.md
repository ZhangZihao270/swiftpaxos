# Evaluation Scripts

## Experiment Run Scripts

| Script | Experiment | Protocols | Key Config |
|--------|-----------|-----------|------------|
| `eval-exp1.1.sh` | Exp 1.1: Throughput vs Latency | Raft, Raft-HT, MongoDB, Pileus, Pileus-HT | w=5%/50%, t=1..96, 1 rep |
| `eval-exp2.1-final.sh` | Exp 2.1: Throughput vs Latency | EPaxos, EPaxos-HO | w=5%/50%, t=1..96, 1 rep |
| `eval-exp2.2-final.sh` | Exp 2.2: Conflict Sweep | EPaxos, EPaxos-HO | t=16, zipf=0..2.0, 1 rep |
| `eval-exp3.1-final.sh` | Exp 3.1: Throughput vs Latency | CURP, CURP-HO, CURP-HT | w=5%/50%, t=1..96, 3 reps |
| `eval-exp3.2-final.sh` | Exp 3.2: T Property | CURP-HO, CURP-HT | t=32, wr=0..99%, 1 rep |

### Exp 2.3 (Failure Recovery)

| Script | Description |
|--------|------------|
| `exp2.3-raftht.sh` | Raft-HT failure recovery (kill leader at t=60s) |
| `exp2.3-epaxosho.sh` | EPaxos-HO failure recovery |
| `exp2.3-epaxosho-kill-r0.sh` | EPaxos-HO kill co-located replica |
| `exp2.3-epaxosho-kill-r3.sh` | EPaxos-HO kill non-co-located replica |

### Utility

| Script | Description |
|--------|------------|
| `collect-results.sh` | Collect results from remote servers |

## Plot Scripts

All plot scripts read from `results/latest/` (symlink-free, copied by eval scripts).

| Script | Figure | Layout |
|--------|--------|--------|
| `plot-exp1.1.py` | Exp 1.1 | 2×2: tput-lat + CDF (w=5%, w=50%) |
| `plot-exp2.1.py` | Exp 2.1 | 2×2: tput-lat + CDF (w=5%, w=50%) |
| `plot-exp2.2.py` | Exp 2.2 | 1×2: tput vs skew + broken-axis lat vs skew |
| `plot-exp3.1.py` | Exp 3.1 | 2×2: tput-lat + CDF (w=5%, w=50%) |
| `plot-exp3.2.py` | Exp 3.2 | 1×2: strong lat (p50+p99) vs wr + tput vs wr |
| `plot_style.py` | Shared | Colors, markers, labels, helpers |

### Generate all plots

```bash
python3 scripts/plot-exp1.1.py
python3 scripts/plot-exp2.1.py
python3 scripts/plot-exp2.2.py
python3 scripts/plot-exp3.1.py
python3 scripts/plot-exp3.2.py
```

Output: `evaluation/plots/`

## AWS Deployment

### Machine Setup

| Role | Count | Instance | Notes |
|------|-------|----------|-------|
| Replica | 5 | c5.xlarge (4 vCPU, 8GB) | 5 different regions/AZs for geo-distribution |
| Client | 3 | c5.2xlarge (8 vCPU, 16GB) | Co-located with replica 0/1/2 |

Total: 8 machines, ~$1.87/h. Budget ~$120-150 for full eval (~48h + debugging).

### Step 1: Configure IPs

After launching instances, update all configs and eval scripts in one command:

```bash
bash scripts/setup-aws-ips.sh <r0> <r1> <r2> <r3> <r4> <c0> <c1> <c2>
```

If clients are co-located with replicas (same machine), omit `<c0> <c1> <c2>`:

```bash
bash scripts/setup-aws-ips.sh <r0> <r1> <r2> <r3> <r4>
```

### Step 2: SSH Setup

Eval scripts use SSH to launch remote processes. Ensure passwordless SSH:

```bash
for host in <r0> <r1> <r2> <r3> <r4> <c0> <c1> <c2>; do
    ssh-copy-id $host
done
```

### Step 3: Build

```bash
go build -o swiftpaxos .
# Binary is synced to remote machines automatically by run-multi-client.sh
```

### Step 4: Network Delay

Configs use `networkDelay: 25` (application-level 25ms one-way = 50ms RTT simulation).

- **Same region**: keep `networkDelay: 25` to simulate geo-distribution
- **Cross-region (real geo)**: set `networkDelay: 0` in all `configs/exp*.conf`

### Step 5: Run Experiments

```bash
# Exp 1.1: ~3h (5 protocols × 2 write groups × 8 thread counts)
bash scripts/eval-exp1.1.sh

# Exp 2.1: ~1.5h (2 protocols × 2 write groups × 8 thread counts)
bash scripts/eval-exp2.1-final.sh

# Exp 2.2: ~1h (2 protocols × 8 zipf skews, t=16 fixed)
bash scripts/eval-exp2.2-final.sh

# Exp 3.1: ~4h (3 protocols × 2 write groups × 8 thread counts × 3 reps)
bash scripts/eval-exp3.1-final.sh

# Exp 3.2: ~1h (2 protocols × 5 weak ratios, t=32 fixed)
bash scripts/eval-exp3.2-final.sh
```

Results auto-copied to `results/latest/` after each experiment.

### Step 6: Plot

```bash
python3 scripts/plot-exp1.1.py
python3 scripts/plot-exp2.1.py
python3 scripts/plot-exp2.2.py
python3 scripts/plot-exp3.1.py
python3 scripts/plot-exp3.2.py
```

Output: `evaluation/plots/`

## Archived

Old phase-specific, debugging, and superseded scripts are in `scripts/archive/`.
