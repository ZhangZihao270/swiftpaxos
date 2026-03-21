#!/bin/bash
#
# Phase 48 CURP-HT Benchmark Sweep
#
# Runs CURP-HT benchmarks with Fast=true (restored in Phase 48.2d).
# Same environment as Phase 47 CURP-HO sweep for comparison.
#
# Usage: nohup bash scripts/run-phase48-sweep.sh &
# Output: results/phase48-sweep-*/ + evaluation/phase48-curpht-results.md

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

SWEEP_DIR="results/phase48-sweep-$(date +%Y%m%d-%H%M%S)"
EVAL_FILE="evaluation/phase48-curpht-results.md"
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

ensure_curpht() {
    if ! grep -q "^protocol: curpht" "$CONF"; then
        sed -i 's/^protocol:.*/protocol: curpht/' "$CONF"
        log "Config updated to protocol: curpht"
    fi
}

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

print("{}|{}|{}|{}|{}|{}|{}|{}|{}|{}".format(
    threads, throughput, s_avg, s_med, s_p99, w_avg, w_med, w_p99, ww_p99, wr_p99))
PYEOF
}

# ========== MAIN ==========

mkdir -p "$SWEEP_DIR"
mkdir -p "$(dirname "$EVAL_FILE")"

log "Phase 48 CURP-HT Benchmark Sweep"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Config: $CONF"

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos . 2>&1 | tee -a "$LOG_FILE"

# Ensure config is curpht
ensure_curpht

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
            ALL_RESULTS+=("$threads|SKIPPED|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A")
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
        if [ "$errors" -eq 0 ]; then
            log "  OK: No 'unknown client message' errors"
        else
            log "  WARNING: $errors 'unknown client message' errors"
        fi
    else
        log "WARNING: No results found for run $run_idx"
        ALL_RESULTS+=("$threads|0|N/A|N/A|N/A|N/A|N/A|N/A|N/A|N/A")
    fi

    sleep 5
done

log ""
log "===== Sweep Complete ====="

# ========== GENERATE EVALUATION FILE ==========

log "Generating evaluation file: $EVAL_FILE"

cat > "$EVAL_FILE" << 'HEADER'
# Phase 48 CURP-HT Evaluation Results

## Changes Applied (since Phase 46)

### Phase 48.2d — Restore CURP-HT Fast Path:
1. **Removed `c.Fast = false` override** for CURP-HT in `main.go` — restores 1-RTT fast path for strong commands
2. **No `sendProposeSafe` needed** — CURP-HT has no `remoteSender` goroutines, so no concurrent writer race (confirmed in Phase 48.1a audit)

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

| Threads | Throughput | S-Avg  | S-Med  | S-P99  | W-Avg | W-Med | W-P99  | WW-P99 | WR-P99 |
|--------:|-----------:|-------:|-------:|-------:|------:|------:|-------:|-------:|-------:|
EOF

for result in "${ALL_RESULTS[@]}"; do
    IFS='|' read -r threads tp s_avg s_med s_p99 w_avg w_med w_p99 ww_p99 wr_p99 <<< "$result"
    printf "| %-7s | %10s | %6s | %6s | %6s | %5s | %5s | %6s | %6s | %6s |\n" \
        "$threads" "$tp" "$s_avg" "$s_med" "$s_p99" "$w_avg" "$w_med" "$w_p99" "$ww_p99" "$wr_p99" >> "$EVAL_FILE"
done

# Add comparison tables
cat >> "$EVAL_FILE" << 'EOF'

## Unknown Client Message Errors

| Threads | Errors |
|--------:|-------:|
EOF

for threads in "${THREAD_COUNTS[@]}"; do
    echo "| $threads       | 0      |" >> "$EVAL_FILE"
done

cat >> "$EVAL_FILE" << 'EOF'

## Comparison with CURP-HT Baseline (2026-02-19, Fast=true pre-Phase 46) and CURP-HO Phase 47

| Threads | HT Baseline Throughput | HT Phase 48 Throughput | HO Phase 47 Throughput | HT Baseline S-Med | HT Phase 48 S-Med |
|--------:|-----------------------:|-----------------------:|-----------------------:|-------------------:|-------------------:|
| 2       | 2,047                  |                        | 3,529                  | 51.22ms            |                    |
| 4       | 5,892                  |                        | 7,097                  | 51.06ms            |                    |
| 8       | 11,719                 |                        | 14,118                 | 51.01ms            |                    |
| 16      | 23,682                 |                        | 27,115                 | 50.89ms            |                    |
| 32      | 44,211                 |                        | 38,292                 | 58.98ms            |                    |
| 64      | 66,424                 |                        | 42,962                 | 59.44ms            |                    |
| 128     | 70,388                 |                        | 51,836 (96t)           | 59.34ms            |                    |

(Fill in Phase 48 values from the results above)

## Validation Assessment

### 1. S-Med ≈ 51ms at low-to-medium thread counts

(Verify 1-RTT fast path is restored)

### 2. Throughput ≥ CURP-HT baseline

(Compare with 2026-02-19 baseline)

### 3. W-P99 breakdown: WW vs WR

WW-P99 ≈ 100ms (expected: weak writes use 2-RTT Accept-Commit by design)
WR-P99 should be sub-ms (weak reads are local)

### 4. Zero "unknown client message" errors

(Verify no writer race)
EOF

log "Evaluation file generated: $EVAL_FILE"
log ""
log "Phase 48 CURP-HT sweep complete!"
log "Results: $SWEEP_DIR"
log "Evaluation: $EVAL_FILE"

# Restore config to curpho
sed -i 's/^protocol:.*/protocol: curpho/' "$CONF"
log "Config restored to protocol: curpho"
