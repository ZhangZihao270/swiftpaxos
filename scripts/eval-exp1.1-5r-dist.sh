#!/bin/bash

# Exp 1.1 (5-Replica): Raft-HT vs Vanilla Raft — Throughput vs Latency
#
# Same as eval-exp1.1-dist.sh but with 5 replicas on 3 machines (2-1-2 layout):
#   .101: replica0, replica1
#   .103: replica2
#   .104: replica3, replica4
#
# Workload: 95/5 read/write, 50/50 strong/weak (0% weak for vanilla Raft), zipfian keys.
#
# Usage: bash scripts/eval-exp1.1-5r-dist.sh [output-dir]
# Output: results/eval-5r-YYYYMMDD/exp1.1/

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r-$DATE}"
EXP_DIR="$BASE_DIR/exp1.1"
THREAD_COUNTS=(1 2 4 8 16 32 64 96 128)
MAX_RETRIES=2

# Use the 5-replica config
CONFIG="/tmp/eval-exp1.1-5r-dist-$$.conf"
cp benchmark-5r.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocol configs: name, protocol-value, weakRatio, writes, weakWrites
declare -a PROTOCOLS=(
    "raftht:raftht:50:5:5"
    "raft:raft:0:5:5"
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

log "Exp 1.1 (5-Replica Distributed): Raft-HT Throughput vs Latency"
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
bash scripts/collect-results.sh throughput "$EXP_DIR" "$BASE_DIR/summary-exp1.1.csv"

log ""
log "Exp 1.1 (5-Replica) complete! Results: $BASE_DIR/summary-exp1.1.csv"
