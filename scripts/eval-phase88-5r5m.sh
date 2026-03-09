#!/bin/bash

# Phase 88: 5-Replica on 5 Machines (No Shared Hosts) — CURP-HT
#
# 5 replicas on 5 separate machines (1 per machine):
#   .101 (64-core Xeon Silver 4216): replica0, client0
#   .103 (64-core Xeon E5-2683 v4): replica1, client1
#   .104 (64-core Xeon E5-2683 v4): replica2, client2
#   .125 (8-core Xeon L5420): replica3, client3
#   .126 (8-core Xeon L5420): replica4, client4
#
# Eliminates shared-host artifacts seen in Phase 86 (2-1-2 layout).
#
# Usage: bash scripts/eval-phase88-5r5m.sh [output-dir]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m-phase88-$DATE}"
EXP_DIR="$BASE_DIR/exp3.1"
THREAD_COUNTS=(1 4 16 32 64)
MAX_RETRIES=2

# Use the 5-replica 5-machine config
CONFIG="/tmp/eval-phase88-5r5m-$$.conf"
cp benchmark-5r-5m.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Only curpht with weakRatio=50
declare -a PROTOCOLS=(
    "curpht:curpht:50:5:5"
)

ALL_HOSTS=(130.245.173.101 130.245.173.103 130.245.173.104 130.245.173.125 130.245.173.126)

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
    for host in "${ALL_HOSTS[@]}"; do
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

log "Phase 88: 5-Replica on 5 Machines — CURP-HT"
log "Layout: .101 x1, .103 x1, .104 x1, .125 x1, .126 x1 (5 replicas, 5 clients)"
log "Note: .125/.126 are 8-core (vs 64-core on .101/.103/.104)"
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
log "Phase 88 complete! Results: $BASE_DIR/summary-exp3.1.csv"
