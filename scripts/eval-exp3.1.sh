#!/bin/bash

# Exp 3.1: CURP-HO vs CURP-HT vs Vanilla CURP — Throughput vs Latency
#
# Sweeps thread count for 3 protocols, measuring throughput and latency.
# Workload: 95/5 read/write, 50/50 strong/weak (0% weak for vanilla CURP), zipfian keys.
#
# Usage: bash scripts/eval-exp3.1.sh [output-dir]
# Output: results/eval-local-YYYYMMDD/exp3.1/

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-local-$DATE}"
EXP_DIR="$BASE_DIR/exp3.1"
THREAD_COUNTS=(1 2 4 8 16 32 64 96 128)
MAX_RETRIES=2

# Use a temp copy of the config to avoid file-watcher interference
CONFIG="/tmp/eval-exp3.1-$$.conf"
cp eval-local.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocol configs: name, protocol-value, weakRatio, writes, weakWrites
# Note: Vanilla CURP baseline uses curpht with weakRatio=0 (all ops strong,
# same behavior as curp but uses the more robust HybridBufferClient path).
declare -a PROTOCOLS=(
    "curpho:curpho:50:5:5"
    "curpht:curpht:50:5:5"
    "curp-baseline:curpht:0:5:5"
)

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

apply_config() {
    local conf="$1" protocol="$2" weak_ratio="$3" writes="$4" weak_writes="$5"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$conf"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $weak_ratio/" "$conf"
    sed -i -E "s/^writes:.*$/writes:      $writes/" "$conf"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  $weak_writes/" "$conf"
}

ensure_clean() {
    pkill -9 -x swiftpaxos 2>/dev/null || true
    for i in $(seq 1 30); do
        pgrep -x swiftpaxos >/dev/null 2>&1 || break
        sleep 0.2
    done
    sleep 1
}

run_benchmark() {
    local out_dir="$1" threads="$2"
    mkdir -p "$out_dir"
    timeout 300 ./run-local-multi.sh -c "$CONFIG" -t "$threads" -o "$out_dir" \
        > "$out_dir/run-output.txt" 2>&1 || true
    ensure_clean
    if [[ -f "$out_dir/summary.txt" ]]; then
        local tp
        tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
        if [[ "$tp" != "0.00" && -n "$tp" ]]; then
            return 0
        fi
    fi
    return 1
}

log "Exp 3.1: CURP Throughput vs Latency"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos . 2>&1

# Initial cleanup
ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#THREAD_COUNTS[@]} ))
run_idx=0

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio writes weak_writes <<< "$proto_spec"

    log "=== Protocol: $name (protocol=$protocol, weakRatio=$weak_ratio) ==="

    for threads in "${THREAD_COUNTS[@]}"; do
        run_idx=$((run_idx + 1))
        out_dir="$EXP_DIR/$name/t${threads}"

        log "  [$run_idx/$total_runs] threads=$threads -> $out_dir"

        # Apply config
        apply_config "$CONFIG" "$protocol" "$weak_ratio" "$writes" "$weak_writes"

        # Run with retry
        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                mkdir -p "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir" "$threads"; then
                success=true
                break
            fi
        done

        # Brief summary
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            log "  Result: throughput=${tp} ops/sec"
        else
            log "  WARNING: No summary.txt generated"
        fi

        if ! $success; then
            log "  WARNING: Run failed after $MAX_RETRIES attempts"
        fi

        sleep 5
    done
    echo ""
done

# Collect results to CSV
log "Collecting results..."
bash scripts/collect-results.sh throughput "$EXP_DIR" "$BASE_DIR/summary-exp3.1.csv"

log ""
log "Exp 3.1 complete! Results: $BASE_DIR/summary-exp3.1.csv"
