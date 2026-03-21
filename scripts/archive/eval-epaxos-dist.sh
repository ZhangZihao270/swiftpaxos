#!/bin/bash

# Exp EPaxos: EPaxos Baseline — Throughput vs Latency (Distributed)
#
# Sweeps thread count for EPaxos on distributed machines.
# EPaxos is a strong-only leaderless protocol (all ops are linearizable).
# Workload: 95/5 read/write, weakRatio=0 (all strong), zipfian keys.
#
# Usage: bash scripts/eval-epaxos-dist.sh [output-dir]
# Output: results/eval-dist-YYYYMMDD/epaxos/

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-dist-$DATE}"
EXP_DIR="$BASE_DIR/epaxos"
THREAD_COUNTS=(1 2 4 8 16 32 64 96 128)
MAX_RETRIES=2

# Use a temp copy of the config to avoid file-watcher interference
CONFIG="/tmp/eval-epaxos-dist-$$.conf"
cp multi-client.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

apply_config() {
    local conf="$1" protocol="$2" weak_ratio="$3" writes="$4" weak_writes="$5" thrifty="$6" reqs="$7"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$conf"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $weak_ratio/" "$conf"
    sed -i -E "s/^writes:.*$/writes:      $writes/" "$conf"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  $weak_writes/" "$conf"
    sed -i -E "s/^thrifty:.*$/thrifty:    $thrifty/" "$conf"
    sed -i -E "s/^reqs:.*$/reqs:        $reqs/" "$conf"
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
    timeout 600 ./run-multi-client.sh -d -c "$CONFIG" -t "$threads" -o "$out_dir" \
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

log "EPaxos Baseline (Distributed): Throughput vs Latency"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Output: $EXP_DIR"
echo ""

# Build swiftpaxos-dist
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1

# Initial cleanup
ensure_clean

total_runs=${#THREAD_COUNTS[@]}
run_idx=0

log "=== Protocol: epaxos (leaderless, strong-only, thrifty=true) ==="

for threads in "${THREAD_COUNTS[@]}"; do
    run_idx=$((run_idx + 1))
    out_dir="$EXP_DIR/epaxos/t${threads}"

    # Use fewer reqs at low thread counts (EPaxos verbose logging is slow at t=1)
    # At t>=8, pipelining is effective and throughput scales
    reqs=10000
    if [[ $threads -le 4 ]]; then
        reqs=1000
    fi

    log "  [$run_idx/$total_runs] threads=$threads reqs=$reqs -> $out_dir"

    apply_config "$CONFIG" "epaxos" "0" "5" "5" "true" "$reqs"

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

# Restore reqs to default
sed -i -E "s/^reqs:.*$/reqs:        10000/" "$CONFIG"

# Collect results to CSV
log "Collecting results..."
bash scripts/collect-results.sh throughput "$EXP_DIR" "$BASE_DIR/summary-epaxos.csv"

log ""
log "EPaxos baseline experiment complete! Results: $BASE_DIR/summary-epaxos.csv"
