#!/bin/bash

# CDF Data Collection — Run all protocols at t=32 (moderate load) for latency distribution data.
#
# This is a focused run that only collects data at one thread count for CDF plots.
# It runs both Exp 1.1 protocols (raft, raftht) and Exp 3.1 protocols (curpho, curpht, curp-baseline).
#
# Usage: bash scripts/eval-cdf-dist.sh [output-dir]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

BASE_DIR="${1:-results/eval-dist-cdf}"
THREADS=32
MAX_RETRIES=2

CONFIG="/tmp/eval-cdf-dist-$$.conf"
cp multi-client.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# All protocols: name, protocol-value, weakRatio, writes, weakWrites, experiment
declare -a PROTOCOLS=(
    "curpho:curpho:50:5:5:exp3.1"
    "curpht:curpht:50:5:5:exp3.1"
    "curp-baseline:curpht:0:5:5:exp3.1"
    "raftht:raftht:50:5:5:exp1.1"
    "raft:raft:0:5:5:exp1.1"
)

mkdir -p "$BASE_DIR"

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

log "CDF Data Collection (Distributed): All protocols at t=$THREADS"
log "Output: $BASE_DIR"
echo ""

# Initial cleanup
ensure_clean

total=${#PROTOCOLS[@]}
run_idx=0

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio writes weak_writes exp <<< "$proto_spec"
    run_idx=$((run_idx + 1))

    out_dir="$BASE_DIR/$exp/$name/t${THREADS}"
    log "[$run_idx/$total] $name (protocol=$protocol, weakRatio=$weak_ratio) -> $out_dir"

    apply_config "$CONFIG" "$protocol" "$weak_ratio" "$writes" "$weak_writes"

    success=false
    for attempt in $(seq 1 $MAX_RETRIES); do
        if [[ $attempt -gt 1 ]]; then
            log "  Retry $attempt/$MAX_RETRIES..."
            rm -rf "$out_dir"
            sleep 5
        fi
        if run_benchmark "$out_dir" "$THREADS"; then
            success=true
            break
        fi
    done

    if [[ -f "$out_dir/summary.txt" ]]; then
        tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
        log "  Result: throughput=${tp} ops/sec"
    fi

    if [[ -f "$out_dir/latencies.json" ]]; then
        size=$(wc -c < "$out_dir/latencies.json")
        log "  Latencies: ${size} bytes"
    else
        log "  WARNING: No latencies.json generated"
    fi

    if ! $success; then
        log "  WARNING: Run failed after $MAX_RETRIES attempts"
    fi

    sleep 5
done

log ""
log "CDF data collection complete! Results: $BASE_DIR/"
log "Run: .venv/bin/python scripts/plot-cdf.py to generate CDF figures"
