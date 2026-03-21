#!/bin/bash

# Test CURP leader saturation point
# Runs CURP baseline (weakRatio=0) with increasing thread counts
# to find when the leader gets overloaded.
#
# Usage: bash scripts/test-curp-saturation.sh [output-dir]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/curp-saturation-$DATE}"
THREAD_COUNTS=(64 128 192 256 384 512)

CONFIG="/tmp/test-curp-saturation-$$.conf"
cp benchmark-5r.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Set to CURP baseline (curpht with weakRatio=0)
sed -i -E "s/^protocol:.*$/protocol: curpht/" "$CONFIG"
sed -i -E "s/^weakRatio:.*$/weakRatio:   0/" "$CONFIG"
sed -i -E "s/^writes:.*$/writes:      5/" "$CONFIG"
sed -i -E "s/^weakWrites:.*$/weakWrites:  5/" "$CONFIG"

mkdir -p "$BASE_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

ensure_clean() {
    for host in 130.245.173.101 130.245.173.103 130.245.173.104; do
        ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "$host" "pkill -9 -x swiftpaxos-dist" 2>/dev/null || true
    done
    sleep 3
}

log "=== CURP Leader Saturation Test ==="
log "Thread counts: ${THREAD_COUNTS[*]}"
log "5 clients × threads × 15 pendings = effective concurrency"
log "Output: $BASE_DIR"
echo ""

# Build
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1

ensure_clean

for threads in "${THREAD_COUNTS[@]}"; do
    eff=$((5 * threads * 15))
    out_dir="$BASE_DIR/t${threads}"
    total=$((threads * 5))
    log "threads=$threads (total=$total, effective=$eff) -> $out_dir"

    mkdir -p "$out_dir"
    timeout 300 ./run-multi-client.sh -d -c "$CONFIG" -t "$threads" -o "$out_dir" \
        > "$out_dir/run-output.txt" 2>&1 || true
    ensure_clean

    if [[ -f "$out_dir/summary.txt" ]]; then
        tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
        avg=$(grep -A3 "Strong Operations" "$out_dir/summary.txt" | grep "Average" | awk '{print $2}')
        p99=$(grep -A5 "Strong Operations" "$out_dir/summary.txt" | grep "99th" | awk '{print $2}')
        log "  => throughput=${tp} ops/s, s_avg=${avg}ms, s_p99=${p99}ms"
    else
        log "  => FAILED (no summary.txt)"
    fi

    sleep 5
done

log ""
log "=== Summary ==="
printf "%-10s %-12s %-10s %-12s %-12s %-12s\n" "threads" "total" "effective" "throughput" "s_avg" "s_p99"
for threads in "${THREAD_COUNTS[@]}"; do
    out_dir="$BASE_DIR/t${threads}"
    if [[ -f "$out_dir/summary.txt" ]]; then
        tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
        avg=$(grep -A3 "Strong Operations" "$out_dir/summary.txt" | grep "Average" | awk '{print $2}')
        p99=$(grep -A5 "Strong Operations" "$out_dir/summary.txt" | grep "99th" | awk '{print $2}')
        printf "%-10s %-12s %-10s %-12s %-12s %-12s\n" "$threads" "$((threads*5))" "$((threads*5*15))" "$tp" "$avg" "$p99"
    else
        printf "%-10s %-12s %-10s %-12s %-12s %-12s\n" "$threads" "$((threads*5))" "$((threads*5*15))" "FAILED" "-" "-"
    fi
done
log "Done."
