#!/bin/bash
#
# Phase 44 Deadloop Benchmark Sweep
#
# Polls server loads until all machines are idle (load < 2.0),
# then runs the full CURP-HO benchmark sweep.
# If machines become loaded mid-sweep (load > 3.0), aborts and restarts polling.
#
# Usage: nohup bash scripts/run-phase44-sweep.sh &
# Output: evaluation/phase44-results.md + results/phase44-sweep-*/

set -e

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

HOSTS=("130.245.173.101" "130.245.173.102" "130.245.173.104")
SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=10"
SSH_USER="$(whoami)"
LOAD_THRESHOLD=2.0
LOAD_ABORT_THRESHOLD=3.0
POLL_INTERVAL=300  # 5 minutes
CONF="multi-client.conf"

# Thread counts: 2, 4, 4, 4, 8, 16, 32, 64, 96
# (4 threads repeated 3x for variance check)
THREAD_COUNTS=(2 4 4 4 8 16 32 64 96)

SWEEP_DIR="results/phase44-sweep-$(date +%Y%m%d-%H%M%S)"
EVAL_FILE="evaluation/phase44-results.md"
LOG_FILE="$SWEEP_DIR/sweep.log"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

# Check if all servers have load < threshold
# Returns 0 if all clean, 1 if any overloaded
check_loads() {
    local threshold="$1"
    local all_clean=true
    for host in "${HOSTS[@]}"; do
        local load
        load=$(ssh $SSH_OPTS "$SSH_USER@$host" "cat /proc/loadavg 2>/dev/null | awk '{print \$1}'" 2>/dev/null || echo "999")
        local over
        over=$(echo "$load $threshold" | awk '{print ($1 >= $2) ? "1" : "0"}')
        if [ "$over" = "1" ]; then
            log "  $host: load=$load (>= $threshold) — NOT CLEAN"
            all_clean=false
        else
            log "  $host: load=$load — OK"
        fi
    done
    if $all_clean; then
        return 0
    else
        return 1
    fi
}

# Ensure config is set to curpho
ensure_curpho() {
    if ! grep -q "^protocol: curpho" "$CONF"; then
        sed -i 's/^protocol:.*/protocol: curpho/' "$CONF"
        log "Config updated to protocol: curpho"
    fi
}

# Extract results from a benchmark run directory
extract_results() {
    local dir="$1"
    local threads="$2"

    if [ ! -f "$dir/summary.txt" ]; then
        log "  WARNING: No summary.txt in $dir"
        echo "$threads|0|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A"
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

# Strong latency (from summary)
s_lat = re.search(r'Strong Operations.*?Avg:\s*([\d.]+)ms.*?Avg median:\s*([\d.]+)ms.*?Max P99:\s*([\d.]+)ms', text, re.DOTALL)
s_avg = s_lat.group(1) if s_lat else "N/A"
s_med = s_lat.group(2) if s_lat else "N/A"
s_p99 = s_lat.group(3) if s_lat else "N/A"

# Weak latency (from summary — combined)
w_lat = re.search(r'Weak Operations.*?Avg:\s*([\d.]+)ms.*?Avg median:\s*([\d.]+)ms.*?Max P99:\s*([\d.]+)ms', text, re.DOTALL)
w_avg = w_lat.group(1) if w_lat else "N/A"
w_med = w_lat.group(2) if w_lat else "N/A"
w_p99 = w_lat.group(3) if w_lat else "N/A"

# Extract separate Weak Write / Weak Read P99 from client logs
ww_p99_vals = []
wr_p99_vals = []
for i in range(10):
    log_file = os.path.join(results_dir, "client{}.log".format(i))
    if not os.path.exists(log_file):
        continue
    with open(log_file) as f:
        ctext = f.read()
    # Look for "Weak Write:  Avg: ...ms | Median: ...ms | P99: ...ms"
    ww = re.search(r'Weak Write:\s+Avg:\s*([\d.]+)ms.*?P99:\s*([\d.]+)ms', ctext)
    if ww:
        ww_p99_vals.append(float(ww.group(2)))
    wr = re.search(r'Weak Read:\s+Avg:\s*([\d.]+)ms.*?P99:\s*([\d.]+)ms', ctext)
    if wr:
        wr_p99_vals.append(float(wr.group(2)))

ww_p99 = "{:.2f}".format(max(ww_p99_vals)) if ww_p99_vals else "N/A"
wr_p99 = "{:.2f}".format(max(wr_p99_vals)) if wr_p99_vals else "N/A"

# Also extract sendMsgToAll instrumentation if present
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

# ========== MAIN ==========

mkdir -p "$SWEEP_DIR"
mkdir -p "$(dirname "$EVAL_FILE")"

log "Phase 44 Deadloop Benchmark Sweep"
log "Working directory: $WORK_DIR"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Sweep results: $SWEEP_DIR"
log ""

# Build first
log "Building swiftpaxos..."
go build -o swiftpaxos . 2>&1 | tee -a "$LOG_FILE"
log "Build complete"
log ""

# Ensure config is curpho
ensure_curpho

# ========== POLL LOOP ==========

while true; do
    log "Checking server loads (threshold: $LOAD_THRESHOLD)..."
    if check_loads "$LOAD_THRESHOLD"; then
        log "All servers clean! Starting benchmark sweep..."
        break
    fi
    log "Servers not clean. Waiting ${POLL_INTERVAL}s..."
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
    log "Mid-sweep load check (threshold: $LOAD_ABORT_THRESHOLD)..."
    if ! check_loads "$LOAD_ABORT_THRESHOLD"; then
        log "SERVER LOAD TOO HIGH mid-sweep! Aborting sweep, restarting poll..."
        log "Completed $((run_idx - 1))/${#THREAD_COUNTS[@]} runs before abort"

        # Kill any running processes
        for host in "${HOSTS[@]}"; do
            ssh $SSH_OPTS "$SSH_USER@$host" "pkill -x swiftpaxos" 2>/dev/null || true
        done
        sleep 10

        # Restart polling
        while true; do
            log "Re-checking server loads (threshold: $LOAD_THRESHOLD)..."
            if check_loads "$LOAD_THRESHOLD"; then
                log "All servers clean! Restarting sweep from run $run_idx..."
                break
            fi
            log "Servers not clean. Waiting ${POLL_INTERVAL}s..."
            sleep "$POLL_INTERVAL"
        done
    fi

    # Run benchmark
    RUN_DIR="$SWEEP_DIR/run-${run_idx}-t${threads}"
    mkdir -p "$RUN_DIR"

    log "Running: ./run-multi-client.sh -c $CONF -d -t $threads"
    timeout 600 ./run-multi-client.sh -c "$CONF" -d -t "$threads" > "$RUN_DIR/benchmark-output.txt" 2>&1 || {
        log "WARNING: Benchmark run timed out or failed (exit=$?)"
        # Copy any results that were generated
        LATEST_RESULT=$(ls -dt results/benchmark-* 2>/dev/null | head -1)
        if [ -n "$LATEST_RESULT" ]; then
            cp -r "$LATEST_RESULT"/* "$RUN_DIR/" 2>/dev/null || true
        fi
    }

    # Copy results from latest benchmark dir
    LATEST_RESULT=$(ls -dt results/benchmark-* 2>/dev/null | head -1)
    if [ -n "$LATEST_RESULT" ] && [ -f "$LATEST_RESULT/summary.txt" ]; then
        cp -r "$LATEST_RESULT"/* "$RUN_DIR/" 2>/dev/null || true
        log "Results copied from $LATEST_RESULT"

        # Extract and log result
        result=$(extract_results "$RUN_DIR" "$threads")
        ALL_RESULTS+=("$result")
        log "Result: $result"
    else
        log "WARNING: No results found for run $run_idx"
        ALL_RESULTS+=("$threads|0|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A")
    fi

    # Brief pause between runs
    sleep 5
done

log ""
log "===== Sweep Complete ====="
log ""

# ========== GENERATE EVALUATION FILE ==========

log "Generating evaluation file: $EVAL_FILE"

cat > "$EVAL_FILE" << 'HEADER'
# Phase 44 Evaluation Results

## Changes Applied (since Phase 42 baseline)

### Phase 43 (retained):
1. **Split handleMsgs** (43.2c): Separate goroutines for strong/weak reply processing
2. **BoundReplica filtering** (43.3): Non-bound replicas skip MCausalReply

### Phase 44 (new):
3. **sendMsgToAll writer race fix** (44.3): Use `sendMsgSafe` with per-replica mutex
4. **Priority fast-path removed** (44.4): Eliminated run loop starvation risk

HEADER

# Add environment info
cat >> "$EVAL_FILE" << EOF
## Environment

| Parameter        | Value                                      |
|------------------|--------------------------------------------|
| Replicas         | 3 (130.245.173.101, .102, .104)            |
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

# Add comparison table
cat >> "$EVAL_FILE" << 'EOF'

## Comparison with Phase 42 Reference (2026-02-19)

| Threads | Phase 42 Throughput | Phase 44 Throughput | Phase 42 W-P99 | Phase 44 W-P99 |
|--------:|--------------------:|--------------------:|---------------:|---------------:|
| 2       | 3,551               |                     | 0.86ms         |                |
| 4       | 4,109               |                     | 100.96ms       |                |
| 8       | 14,050              |                     | 2.62ms         |                |
| 16      | 8,771               |                     | 100.95ms       |                |
| 32      | 30,339              |                     | 100.38ms       |                |
| 64      | 34,797              |                     | 102.51ms       |                |
| 96      | 71,595              |                     | 119.61ms       |                |

(Fill in Phase 44 values from the results above)

## Analysis

WW-P99 = Weak Write P99, WR-P99 = Weak Read P99, SendAll-P99 = sendMsgToAll duration P99
All latencies in ms.

### Key Questions Answered:
1. **Throughput scaling**: Does throughput scale with thread count? (Compare with Phase 42 reference)
2. **W-P99 at 4 threads**: Is it still ~100ms? If so, is it Weak Write or Weak Read?
3. **W-P99 at 16 threads**: Does Phase 43 improvement (1.08ms) hold?
4. **sendMsgToAll blocking**: Is send duration > 10ms at any thread count?
EOF

log "Evaluation file generated: $EVAL_FILE"
log ""
log "Phase 44 deadloop sweep complete!"
log "Results: $SWEEP_DIR"
log "Evaluation: $EVAL_FILE"
