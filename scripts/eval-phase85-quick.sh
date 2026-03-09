#!/bin/bash

# Phase 85.2: Quick Verification — Fast path restored
#
# Runs t=1, t=32, t=64 for all 3 CURP protocols to verify
# fast path is restored after reverting Phase 83.1 slot ordering.
#
# Expected: S-P50 ≈ 50ms at low load (1 RTT fast path), not 100ms.
#
# Usage: bash scripts/eval-phase85-quick.sh [output-dir]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r-phase85-quick-$DATE}"
EXP_DIR="$BASE_DIR/exp3.1"
THREAD_COUNTS=(1 32 64)
MAX_RETRIES=2

# Use the 5-replica config
CONFIG="/tmp/eval-phase85-quick-$$.conf"
cp benchmark-5r.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocol configs: name, protocol-value, weakRatio, writes, weakWrites
declare -a PROTOCOLS=(
    "curp-baseline:curpht:0:5:5"
    "curpht:curpht:50:5:5"
    "curpho:curpho:50:5:5"
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
    local out_dir="$1" threads="$2"
    mkdir -p "$out_dir"
    timeout 300 ./run-multi-client.sh -d -c "$CONFIG" -t "$threads" -o "$out_dir" \
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

log "Phase 85.2: Quick Verification — Fast Path Restored"
log "Layout: .101 x2, .103 x1, .104 x2"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1

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

        apply_config "$CONFIG" "$protocol" "$weak_ratio" "$writes" "$weak_writes"

        # Run with retry
        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
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
log "Phase 85.2 quick verification complete! Results: $BASE_DIR/summary-exp3.1.csv"
