#!/bin/bash
#
# Phase 49 Vanilla Raft Baseline Benchmark
#
# Runs vanilla Raft (all-strong, weakRatio=0) as baseline for Raft-HT comparison.
# Uses same environment as the Raft-HT sweep.
#
# Usage: nohup bash scripts/run-phase49-raft-baseline.sh &
# Output: results/phase49-raft-baseline-*/ + evaluation/phase49-raft-baseline.md

# No set -e: grep -c returns exit 1 on no match, which would abort the script

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

HOSTS=("130.245.173.101" "130.245.173.103" "130.245.173.104")
SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=10"
SSH_USER="$(whoami)"
LOAD_THRESHOLD=2.0
LOAD_ABORT_THRESHOLD=3.0
POLL_INTERVAL=60
CONF="multi-client.conf"

THREAD_COUNTS=(2 4 8 16 32 64 96)

SWEEP_DIR="results/phase49-raft-baseline-$(date +%Y%m%d-%H%M%S)"
EVAL_FILE="evaluation/phase49-raft-baseline.md"
LOG_FILE="$SWEEP_DIR/sweep.log"

# Save original config values to restore later
ORIG_PROTOCOL=""
ORIG_WEAKRATIO=""

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

check_loads() {
    local threshold="$1"
    local all_clean=true
    for host in "${HOSTS[@]}"; do
        local load
        load=$(ssh $SSH_OPTS "$SSH_USER@$host" "cat /proc/loadavg 2>/dev/null | awk '{print \$1}'" 2>/dev/null || echo "999")
        local over
        over=$(echo "$load $threshold" | awk '{print ($1 >= $2) ? "1" : "0"}')
        if [ "$over" = "1" ]; then
            log "  $host: load=$load (>= $threshold)"
            all_clean=false
        fi
    done
    if $all_clean; then
        return 0
    else
        return 1
    fi
}

setup_raft_config() {
    # Save originals
    ORIG_PROTOCOL=$(grep "^protocol:" "$CONF" | awk '{print $2}')
    ORIG_WEAKRATIO=$(grep "^weakRatio:" "$CONF" | awk '{print $2}')

    # Set vanilla Raft with all-strong workload
    sed -i 's/^protocol:.*/protocol: raft/' "$CONF"
    sed -i 's/^weakRatio:.*/weakRatio: 0/' "$CONF"
    log "Config set to protocol: raft, weakRatio: 0 (all-strong baseline)"
}

restore_config() {
    local proto="${ORIG_PROTOCOL:-curpho}"
    local weak="${ORIG_WEAKRATIO:-50}"
    sed -i "s/^protocol:.*/protocol: $proto/" "$CONF"
    sed -i "s/^weakRatio:.*/weakRatio: $weak/" "$CONF"
    log "Config restored to protocol: $proto, weakRatio: $weak"
}

extract_results() {
    local dir="$1"
    local threads="$2"

    if [ ! -f "$dir/summary.txt" ]; then
        log "  WARNING: No summary.txt in $dir"
        echo "$threads|0|N/A|N/A|N/A"
        return
    fi

    python3 - "$dir" "$threads" << 'PYEOF'
import sys, os, re

results_dir = sys.argv[1]
threads = sys.argv[2]

summary_file = os.path.join(results_dir, "summary.txt")
with open(summary_file) as f:
    text = f.read()

tp_m = re.search(r'Aggregate throughput:\s*([\d.]+)', text)
throughput = tp_m.group(1) if tp_m else "0"

# Raft is all-strong, so only strong latencies matter
s_lat = re.search(r'Strong Operations.*?Avg:\s*([\d.]+)ms.*?Avg median:\s*([\d.]+)ms.*?Max P99:\s*([\d.]+)ms', text, re.DOTALL)
s_avg = s_lat.group(1) if s_lat else "N/A"
s_med = s_lat.group(2) if s_lat else "N/A"
s_p99 = s_lat.group(3) if s_lat else "N/A"

print("{}|{}|{}|{}|{}".format(threads, throughput, s_avg, s_med, s_p99))
PYEOF
}

# ========== MAIN ==========

mkdir -p "$SWEEP_DIR"
mkdir -p "$(dirname "$EVAL_FILE")"

log "Phase 49 Vanilla Raft Baseline Benchmark"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Config: $CONF"

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos . 2>&1 | tee -a "$LOG_FILE"

# Setup Raft config
setup_raft_config

# ========== POLL LOOP ==========

while true; do
    log "Checking server loads..."
    if check_loads "$LOAD_THRESHOLD"; then
        log "All servers clean! Starting sweep..."
        break
    fi
    log "Servers loaded, waiting ${POLL_INTERVAL}s..."
    sleep "$POLL_INTERVAL"
done

# ========== BENCHMARK SWEEP ==========

ALL_RESULTS=()
run_idx=0

for threads in "${THREAD_COUNTS[@]}"; do
    run_idx=$((run_idx + 1))
    log ""
    log "===== Run $run_idx/${#THREAD_COUNTS[@]}: $threads threads ====="

    # Mid-sweep load check
    if ! check_loads "$LOAD_ABORT_THRESHOLD"; then
        log "Servers loaded, waiting 60s..."
        sleep 60
        if ! check_loads "$LOAD_ABORT_THRESHOLD"; then
            log "Servers still loaded (>$LOAD_ABORT_THRESHOLD), skipping run"
            ALL_RESULTS+=("$threads|SKIPPED|N/A|N/A|N/A")
            continue
        fi
    fi

    RUN_DIR="$SWEEP_DIR/run-${run_idx}-t${threads}"
    mkdir -p "$RUN_DIR"

    log "Running: ./run-multi-client.sh -c $CONF -d -t $threads"
    timeout 600 ./run-multi-client.sh -c "$CONF" -d -t "$threads" > "$RUN_DIR/benchmark-output.txt" 2>&1 || {
        log "WARNING: Benchmark run timed out or failed (exit=$?)"
        LATEST_RESULT=$(ls -dt results/benchmark-* 2>/dev/null | head -1)
        if [ -n "$LATEST_RESULT" ]; then
            cp -r "$LATEST_RESULT"/* "$RUN_DIR/" 2>/dev/null || true
        fi
    }

    LATEST_RESULT=$(ls -dt results/benchmark-* 2>/dev/null | head -1)
    if [ -n "$LATEST_RESULT" ] && [ -f "$LATEST_RESULT/summary.txt" ]; then
        cp -r "$LATEST_RESULT"/* "$RUN_DIR/" 2>/dev/null || true

        result=$(extract_results "$RUN_DIR" "$threads")
        ALL_RESULTS+=("$result")
        log "  Result: $result"
    else
        log "WARNING: No results found for run $run_idx"
        ALL_RESULTS+=("$threads|0|N/A|N/A|N/A")
    fi

    sleep 5
done

log ""
log "===== Sweep Complete ====="

# ========== GENERATE EVALUATION FILE ==========

log "Generating evaluation file: $EVAL_FILE"

cat > "$EVAL_FILE" << EOF
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
| Date             | $(date '+%Y-%m-%d')                        |

## Results

| Threads | Throughput | S-Avg  | S-Med  | S-P99  |
|--------:|-----------:|-------:|-------:|-------:|
EOF

for result in "${ALL_RESULTS[@]}"; do
    IFS='|' read -r threads tp s_avg s_med s_p99 <<< "$result"
    printf "| %-7s | %10s | %6s | %6s | %6s |\n" \
        "$threads" "$tp" "$s_avg" "$s_med" "$s_p99" >> "$EVAL_FILE"
done

cat >> "$EVAL_FILE" << 'EOF'

## Notes

- Vanilla Raft uses 2-RTT for all operations: leader appends, replicates, waits for majority, then replies.
- Expected S-Med ~100ms at low thread counts (2 x 50ms RTT).
- This baseline establishes the performance floor that Raft-HT should match for strong ops.
EOF

log "Evaluation file generated: $EVAL_FILE"
log ""
log "Phase 49 Raft baseline sweep complete!"
log "Results: $SWEEP_DIR"
log "Evaluation: $EVAL_FILE"

# Restore config
restore_config
