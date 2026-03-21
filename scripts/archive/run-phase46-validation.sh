#!/bin/bash
#
# Phase 46 Validation Benchmark Sweep
#
# Validates the Fast=false fix and thread-count propagation fix:
# 1. No "unknown client message" errors on any replica
# 2. Throughput scales with thread count
# 3. W-P99 < 5ms at 4-64 threads (async queue fix preserved)
# 4. S-P99 < 200ms at all thread counts
#
# Usage: bash scripts/run-phase46-validation.sh

set -e

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

HOSTS=("130.245.173.101" "130.245.173.103" "130.245.173.104")
SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=10"
SSH_USER="$(whoami)"
CONF="multi-client.conf"

THREAD_COUNTS=(2 4 8 16 32 64 96)

SWEEP_DIR="results/phase46-sweep-$(date +%Y%m%d-%H%M%S)"
EVAL_FILE="evaluation/phase46-results.md"
LOG_FILE="$SWEEP_DIR/sweep.log"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

# Check if all servers have load < threshold
check_loads() {
    local threshold="$1"
    for host in "${HOSTS[@]}"; do
        local load
        load=$(ssh $SSH_OPTS "$SSH_USER@$host" "cat /proc/loadavg 2>/dev/null | awk '{print \$1}'" 2>/dev/null || echo "999")
        local over
        over=$(echo "$load $threshold" | awk '{print ($1 >= $2) ? "1" : "0"}')
        if [ "$over" = "1" ]; then
            log "  $host: load=$load (>= $threshold)"
            return 1
        fi
    done
    return 0
}

# Extract results from a benchmark run directory
extract_results() {
    local dir="$1"
    local threads="$2"

    if [ ! -f "$dir/summary.txt" ]; then
        log "  WARNING: No summary.txt in $dir"
        echo "$threads|0|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A"
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

send_p99 = "N/A"
for i in range(10):
    log_file = os.path.join(results_dir, "client{}.log".format(i))
    if not os.path.exists(log_file):
        continue
    with open(log_file) as f:
        ctext = f.read()
    sm = re.search(r'sendMsgToAll.*?P99:\s*([\d.]+)ms', ctext)
    if sm:
        val = float(sm.group(1))
        if send_p99 == "N/A" or val > float(send_p99):
            send_p99 = "{:.2f}".format(val)

print("{}|{}|{}|{}|{}|{}|{}|{}|{}|{}|{}".format(
    threads, throughput, s_avg, s_med, s_p99, w_avg, w_med, w_p99, ww_p99, wr_p99, send_p99))
PYEOF
}

# Count "unknown client message" errors in replica logs
count_unknown_msgs() {
    local dir="$1"
    local total=0
    for logfile in "$dir"/replica*.log; do
        [ -f "$logfile" ] || continue
        local count
        count=$(grep -c "unknown client message" "$logfile" 2>/dev/null || true)
        count=${count:-0}
        # Ensure count is a valid number
        if [[ "$count" =~ ^[0-9]+$ ]]; then
            total=$((total + count))
        fi
    done
    echo "$total"
}

# ========== MAIN ==========

mkdir -p "$SWEEP_DIR"
mkdir -p "$(dirname "$EVAL_FILE")"

log "Phase 46 Validation Benchmark Sweep"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Config: $CONF"

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos . || { log "FATAL: Build failed"; exit 1; }

ALL_RESULTS=()
UNKNOWN_MSG_COUNTS=()
run_idx=0

for threads in "${THREAD_COUNTS[@]}"; do
    run_idx=$((run_idx + 1))

    log "===== Run $run_idx/${#THREAD_COUNTS[@]}: $threads threads ====="

    # Check loads before running
    if ! check_loads 2.0; then
        log "Servers loaded, waiting 60s..."
        sleep 60
        if ! check_loads 3.0; then
            log "Servers still loaded (>3.0), skipping run"
            ALL_RESULTS+=("$threads|0|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A")
            continue
        fi
    fi

    RUN_DIR="$SWEEP_DIR/run-${run_idx}-t${threads}"
    mkdir -p "$RUN_DIR"

    log "Running: ./run-multi-client.sh -c $CONF -d -t $threads"
    timeout 600 ./run-multi-client.sh -c "$CONF" -d -t "$threads" > "$RUN_DIR/benchmark-output.txt" 2>&1 || {
        log "WARNING: Benchmark run timed out or failed (exit=$?)"
    }

    # Copy results
    LATEST_RESULT=$(ls -dt results/benchmark-* 2>/dev/null | head -1)
    if [ -n "$LATEST_RESULT" ] && [ "$LATEST_RESULT" != "$RUN_DIR" ]; then
        cp -r "$LATEST_RESULT"/* "$RUN_DIR/" 2>/dev/null || true
    fi

    # Extract results
    if [ -f "$RUN_DIR/summary.txt" ]; then
        result=$(extract_results "$RUN_DIR" "$threads")
        ALL_RESULTS+=("$result")
        log "  Result: $result"
    else
        ALL_RESULTS+=("$threads|0|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A")
        log "  No summary.txt found"
    fi

    # Check for unknown client messages
    unknown_count=$(count_unknown_msgs "$RUN_DIR")
    UNKNOWN_MSG_COUNTS+=("$threads:$unknown_count")
    if [ "$unknown_count" -gt 0 ]; then
        log "  WARNING: $unknown_count 'unknown client message' errors found!"
    else
        log "  OK: No 'unknown client message' errors"
    fi

    # Brief pause between runs
    sleep 5
done

# ========== GENERATE EVALUATION ==========

log "Generating evaluation file: $EVAL_FILE"

cat > "$EVAL_FILE" << 'HEADER'
# Phase 46 Evaluation Results

## Changes Applied (since Phase 45)

### Phase 46.2 — Fix Writer Race:
1. **Set `c.Fast = false`** for CURP-HO and CURP-HT in `main.go` — prevents `SendProposal` from broadcasting to all replicas without mutex, eliminating the data race with `remoteSender` goroutines

### Phase 46.2.5 — Fix Benchmark Thread Count:
2. **Propagate `-t N`** to config — `run-multi-client.sh` now writes `clientThreads: N` to the temp config when `-t` is specified, so the client binary actually uses the requested thread count

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

## Results

| Threads | Throughput | S-Avg | S-Med | S-P99 | W-Avg | W-Med | W-P99 | WW-P99 | WR-P99 | SendAll-P99 |
|--------:|-----------:|------:|------:|------:|------:|------:|------:|-------:|-------:|------------:|
EOF

for result in "${ALL_RESULTS[@]}"; do
    IFS='|' read -r threads tp s_avg s_med s_p99 w_avg w_med w_p99 ww_p99 wr_p99 send_p99 <<< "$result"
    printf "| %-7s | %-10s | %-5s | %-5s | %-5s | %-5s | %-5s | %-5s | %-6s | %-6s | %-11s |\n" \
        "$threads" "$tp" "$s_avg" "$s_med" "$s_p99" "$w_avg" "$w_med" "$w_p99" "$ww_p99" "$wr_p99" "$send_p99" >> "$EVAL_FILE"
done

cat >> "$EVAL_FILE" << EOF

## Unknown Client Message Errors

| Threads | Errors |
|--------:|-------:|
EOF

for entry in "${UNKNOWN_MSG_COUNTS[@]}"; do
    IFS=':' read -r threads count <<< "$entry"
    printf "| %-7s | %-6s |\n" "$threads" "$count" >> "$EVAL_FILE"
done

cat >> "$EVAL_FILE" << 'EOF'

## Comparison with Phase 42 Reference (2026-02-19)

| Threads | Phase 42 Throughput | Phase 46 Throughput | Phase 42 W-P99 | Phase 46 W-P99 |
|--------:|--------------------:|--------------------:|---------------:|---------------:|
| 2       | 3,551               |                     | 0.86ms         |                |
| 4       | 4,109               |                     | 100.96ms       |                |
| 8       | 14,050              |                     | 2.62ms         |                |
| 16      | 8,771               |                     | 100.95ms       |                |
| 32      | 30,339              |                     | 100.38ms       |                |
| 64      | 34,797              |                     | 102.51ms       |                |
| 96      | 71,595              |                     | 119.61ms       |                |

(Fill in Phase 46 values from the results above)

## Validation Criteria

1. **Zero "unknown client message" errors** — confirms Fast=false fix works
2. **Throughput scaling** — confirms connections are no longer dropped
3. **W-P99 < 5ms at 4-64 threads** — confirms async queue optimization is preserved
4. **S-P99 < 200ms** — confirms no more timeout-level delays from dropped connections
EOF

log ""
log "Phase 46 validation sweep complete!"
log "Results: $SWEEP_DIR"
log "Evaluation: $EVAL_FILE"
