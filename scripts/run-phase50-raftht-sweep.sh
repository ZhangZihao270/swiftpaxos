#!/bin/bash
#
# Phase 50 Raft-HT Benchmark Sweep (Post-Fix)
#
# Runs Raft-HT benchmarks after Phase 50.1-50.3 fixes:
# - 50.1: RWMutex-based weak reads (decoupled from executeCommands)
# - 50.2: Batched weak write replication (single broadcastAppendEntries per batch)
# - 50.3: Event loop reduced from 10 to 9 select cases
#
# Validates:
# - Peak throughput > 30K ops/sec
# - Throughput >= vanilla Raft at all thread counts
# - WR-P99 < 5ms at 96 threads (was 134ms before fix)
# - WW-P99 unchanged (~52ms at low load)
# - S-Med unchanged (~85ms at low load)
#
# Usage: nohup bash scripts/run-phase50-raftht-sweep.sh &
# Output: results/phase50-raftht-sweep-*/ + evaluation/phase50-raftht-results.md

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

SWEEP_DIR="results/phase50-raftht-sweep-$(date +%Y%m%d-%H%M%S)"
EVAL_FILE="evaluation/phase50-raftht-results.md"
LOG_FILE="$SWEEP_DIR/sweep.log"

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

ensure_raftht() {
    if ! grep -q "^protocol: raftht" "$CONF"; then
        sed -i 's/^protocol:.*/protocol: raftht/' "$CONF"
        log "Config updated to protocol: raftht"
    fi
    # Ensure weak ratio is 50%
    sed -i 's/^weakRatio:.*/weakRatio: 50/' "$CONF"
    log "Config set: protocol=raftht, weakRatio=50"
}

restore_config() {
    sed -i 's/^protocol:.*/protocol: curpho/' "$CONF"
    sed -i 's/^weakRatio:.*/weakRatio: 50/' "$CONF"
    log "Config restored to protocol: curpho, weakRatio: 50"
}

extract_results() {
    local dir="$1"
    local threads="$2"

    if [ ! -f "$dir/summary.txt" ]; then
        log "  WARNING: No summary.txt in $dir"
        echo "$threads|0|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A"
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

s_lat = re.search(r'Strong Operations.*?Avg:\s*([\d.]+)ms.*?Avg median:\s*([\d.]+)ms.*?Max P99:\s*([\d.]+)ms', text, re.DOTALL)
s_avg = s_lat.group(1) if s_lat else "N/A"
s_med = s_lat.group(2) if s_lat else "N/A"
s_p99 = s_lat.group(3) if s_lat else "N/A"

w_lat = re.search(r'Weak Operations.*?Avg:\s*([\d.]+)ms.*?Avg median:\s*([\d.]+)ms.*?Max P99:\s*([\d.]+)ms', text, re.DOTALL)
w_avg = w_lat.group(1) if w_lat else "N/A"
w_med = w_lat.group(2) if w_lat else "N/A"
w_p99 = w_lat.group(3) if w_lat else "N/A"

ww_p99_vals = []
wr_p99_vals = []
for i in range(10):
    log_file = os.path.join(results_dir, "client{}.log".format(i))
    if not os.path.exists(log_file):
        continue
    with open(log_file) as f:
        ctext = f.read()
    ww = re.search(r'Weak Write:\s+Avg:\s*([\d.]+)ms.*?P99:\s*([\d.]+)ms', ctext)
    if ww:
        ww_p99_vals.append(float(ww.group(2)))
    wr = re.search(r'Weak Read:\s+Avg:\s*([\d.]+)ms.*?P99:\s*([\d.]+)ms', ctext)
    if wr:
        wr_p99_vals.append(float(wr.group(2)))

ww_p99 = "{:.2f}".format(max(ww_p99_vals)) if ww_p99_vals else "N/A"
wr_p99 = "{:.2f}".format(max(wr_p99_vals)) if wr_p99_vals else "N/A"

print("{}|{}|{}|{}|{}|{}|{}|{}|{}|{}".format(
    threads, throughput, s_avg, s_med, s_p99, w_avg, w_med, w_p99, ww_p99, wr_p99))
PYEOF
}

# ========== MAIN ==========

mkdir -p "$SWEEP_DIR"
mkdir -p "$(dirname "$EVAL_FILE")"

log "Phase 50 Raft-HT Benchmark Sweep (Post-Fix)"
log "Fixes: 50.1 (RWMutex weak reads), 50.2 (batched weak writes), 50.3 (event loop -1 case)"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Config: $CONF"

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos . 2>&1 | tee -a "$LOG_FILE"

# Ensure config is raftht
ensure_raftht

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
ALL_ERRORS=()
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
            ALL_RESULTS+=("$threads|SKIPPED|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A")
            ALL_ERRORS+=("$threads|SKIPPED")
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

        # Check for errors
        errors=0
        for rl in "$RUN_DIR"/replica*.log; do
            [ -f "$rl" ] || continue
            e=$(grep -c "unknown client message" "$rl" 2>/dev/null || true)
            e=${e:-0}
            e=$(echo "$e" | tr -d '[:space:]')
            errors=$((errors + e))
        done
        ALL_ERRORS+=("$threads|$errors")
        if [ "$errors" -eq 0 ]; then
            log "  OK: No 'unknown client message' errors"
        else
            log "  WARNING: $errors 'unknown client message' errors"
        fi
    else
        log "WARNING: No results found for run $run_idx"
        ALL_RESULTS+=("$threads|0|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A")
        ALL_ERRORS+=("$threads|N/A")
    fi

    sleep 5
done

log ""
log "===== Sweep Complete ====="

# ========== GENERATE EVALUATION FILE ==========

log "Generating evaluation file: $EVAL_FILE"

cat > "$EVAL_FILE" << 'HEADER'
# Phase 50 Raft-HT Evaluation Results (Post-Fix)

## Fixes Applied

Phase 50.1-50.3 optimizations to fix high-concurrency throughput regression:
- **50.1**: RWMutex-based weak reads — dedicated `weakReadLoop` goroutine with `stateMu.RLock()`, decoupled from executeCommands
- **50.2**: Batched weak write replication — drain `weakProposeChan` per select case, single `broadcastAppendEntries` per batch
- **50.3**: Event loop reduced from 10 to 9 select cases (weakReadChan removed)

HEADER

cat >> "$EVAL_FILE" << EOF
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
| Date             | $(date '+%Y-%m-%d')                        |

## Raft-HT Results (Post-Fix)

| Threads | Throughput | S-Avg  | S-Med  | S-P99  | W-Avg | W-Med | W-P99  | WW-P99 | WR-P99 |
|--------:|-----------:|-------:|-------:|-------:|------:|------:|-------:|-------:|-------:|
EOF

for result in "${ALL_RESULTS[@]}"; do
    IFS='|' read -r threads tp s_avg s_med s_p99 w_avg w_med w_p99 ww_p99 wr_p99 <<< "$result"
    printf "| %-7s | %10s | %6s | %6s | %6s | %5s | %5s | %6s | %6s | %6s |\n" \
        "$threads" "$tp" "$s_avg" "$s_med" "$s_p99" "$w_avg" "$w_med" "$w_p99" "$ww_p99" "$wr_p99" >> "$EVAL_FILE"
done

# Add error table
cat >> "$EVAL_FILE" << 'EOF'

## Unknown Client Message Errors

| Threads | Errors |
|--------:|-------:|
EOF

for err in "${ALL_ERRORS[@]}"; do
    IFS='|' read -r threads errors <<< "$err"
    printf "| %-7s | %6s |\n" "$threads" "$errors" >> "$EVAL_FILE"
done

# Add validation section
cat >> "$EVAL_FILE" << 'EOF'

## Validation Against Phase 50 Success Criteria

### 1. Peak Raft-HT throughput > 30K ops/sec

(Check max throughput across all thread counts)

### 2. Raft-HT throughput >= Raft baseline at all thread counts

(Compare with Phase 49 Raft baseline — Raft-HT should never be slower)

### 3. WR-P99 < 5ms at 96 threads (was 134ms before fix)

(Check WR-P99 column at 32 threads / 96 total — should be dramatically improved)

### 4. WW-P99 unchanged (~52ms at low load)

(Check WW-P99 at 2-8 threads — should match pre-fix values)

### 5. S-Med unchanged (~85ms at low load)

(Check S-Med at 2-8 threads — should match pre-fix values)

### 6. All tests pass, zero errors

(Check error table above; all unit tests verified before benchmark)

## Pre-Fix vs Post-Fix Comparison

| Threads | Pre-Fix TP | Post-Fix TP | Pre-Fix WR-P99 | Post-Fix WR-P99 |
|--------:|-----------:|------------:|---------------:|----------------:|
| 6       |      2,315 |             |           0.44 |                 |
| 12      |      4,599 |             |           0.60 |                 |
| 24      |      9,145 |             |           0.68 |                 |
| 48      |     14,523 |             |          31.52 |                 |
| 96      |     14,699 |             |         134.83 |                 |
| 192     |      7,584 |             |        1029.38 |                 |

(Fill in Post-Fix columns after benchmark completes)
EOF

log "Evaluation file generated: $EVAL_FILE"
log ""
log "Phase 50 Raft-HT sweep complete!"
log "Results: $SWEEP_DIR"
log "Evaluation: $EVAL_FILE"

# Restore config
restore_config
