#!/bin/bash

# Exp 3.2 (5-Replica, Phase 78): Weak Ratio Sweep — CURP-HO vs CURP-HT vs Baseline
#
# 5 replicas on 3 machines (2-1-2 layout):
#   .101: replica0, replica1
#   .103: replica2
#   .104: replica3, replica4
#
# Sweeps weak proportion at fixed concurrency t=32,
# measuring throughput and latency for each protocol.
# Workload: 5% write / 95% read, sweep weakRatio (0-100%), zipfian keys.
#
# Usage: bash scripts/eval-exp3.2-5r-phase78.sh [output-dir]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r-exp3.2-phase78-$DATE}"
EXP_DIR="$BASE_DIR/exp3.2"
FIXED_THREADS=32
WEAK_RATIOS=(0 10 25 50 75 100)
MAX_RETRIES=2

# Use the 5-replica config
CONFIG="/tmp/eval-exp3.2-5r-phase78-$$.conf"
cp benchmark-5r.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocol configs: name, protocol-value, weakWrites
# curp-baseline: uses curpht with weakRatio=0 (forced override below)
declare -a PROTOCOLS=(
    "curpho:curpho:5"
    "curpht:curpht:5"
    "curp-baseline:curpht:5"
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
    for host in 130.245.173.101 130.245.173.103 130.245.173.104; do
        ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "$host" "pkill -9 -x swiftpaxos-dist" 2>/dev/null || true
    done
    sleep 3
}

run_benchmark() {
    local out_dir="$1"
    mkdir -p "$out_dir"
    timeout 300 ./run-multi-client.sh -d -c "$CONFIG" -t "$FIXED_THREADS" -o "$out_dir" \
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

log "Exp 3.2 (5-Replica Phase 78): Weak Ratio Sweep"
log "Layout: .101 x2, .103 x1, .104 x2"
log "Weak ratios: ${WEAK_RATIOS[*]}"
log "Fixed threads: $FIXED_THREADS"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1

# Initial cleanup
ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#WEAK_RATIOS[@]} ))
run_idx=0

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_writes <<< "$proto_spec"

    log "=== Protocol: $name (protocol=$protocol) ==="

    for ratio in "${WEAK_RATIOS[@]}"; do
        run_idx=$((run_idx + 1))

        # curp-baseline always uses weakRatio=0 regardless of sweep value
        if [[ "$name" == "curp-baseline" ]]; then
            actual_ratio=0
        else
            actual_ratio=$ratio
        fi

        out_dir="$EXP_DIR/$name/w${ratio}"
        log "  [$run_idx/$total_runs] weakRatio=$actual_ratio (sweep=$ratio) -> $out_dir"

        apply_config "$CONFIG" "$protocol" "$actual_ratio" "5" "$weak_writes"

        # Run with retry
        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir"; then
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
bash scripts/collect-results.sh sweep "$EXP_DIR" "$BASE_DIR/summary-exp3.2.csv"

log ""
log "Exp 3.2 (5-Replica Phase 78) complete! Results: $BASE_DIR/summary-exp3.2.csv"
