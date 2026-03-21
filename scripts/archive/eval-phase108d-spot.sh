#!/bin/bash

# Phase 108d: Spot test — EPaxos-HO (post-optimization) vs EPaxos at t=64, w50%
#
# Runs two benchmarks:
# 1. EPaxos-HO with weakRatio=50, weakWrites=50, writes=50, t=64
# 2. EPaxos (vanilla) with weakRatio=0, weakWrites=0, writes=50, t=64 (control)
#
# Target: EPaxos-HO throughput >= EPaxos throughput (~42K ops/s)

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase108d-$DATE}"
THREADS=64
MAX_RETRIES=3

ALL_HOSTS=(130.245.173.101 130.245.173.103 130.245.173.104 130.245.173.125 130.245.173.126)

CONFIG_TEMPLATE="benchmark-5r-5m-3c.conf"
if [[ ! -f "$CONFIG_TEMPLATE" ]]; then
    echo "ERROR: Config template $CONFIG_TEMPLATE not found"
    exit 1
fi

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

ensure_clean() {
    for host in "${ALL_HOSTS[@]}"; do
        ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "zihao@$host" "pkill -9 -x swiftpaxos-dist" 2>/dev/null || true
    done
    sleep 3
}

run_benchmark() {
    local out_dir="$1" threads="$2" config="$3"
    mkdir -p "$out_dir"
    timeout 300 ./run-multi-client.sh -d -c "$config" -t "$threads" -o "$out_dir" \
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

log "Phase 108d: Spot test — EPaxos-HO vs EPaxos at t=$THREADS, w50%"
log "Output: $BASE_DIR"
echo ""

# Build
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1
if [[ $? -ne 0 ]]; then
    log "ERROR: Build failed"
    exit 1
fi

ensure_clean
mkdir -p "$BASE_DIR"

# --- Run 1: EPaxos-HO ---
log "=== EPaxos-HO (weakRatio=50, weakWrites=50, writes=50, t=$THREADS) ==="
config_ho="/tmp/eval-phase108d-epaxosho-$$.conf"
cp "$CONFIG_TEMPLATE" "$config_ho"
sed -i -E "s/^protocol:.*$/protocol: epaxosho/" "$config_ho"
sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config_ho"
sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$config_ho"
sed -i -E "s/^writes:.*$/writes:      50/" "$config_ho"
sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$config_ho"
sed -i -E "s/^fast:.*$/fast:       true/" "$config_ho"

out_ho="$BASE_DIR/epaxosho-t${THREADS}"
success=false
for attempt in $(seq 1 $MAX_RETRIES); do
    if [[ $attempt -gt 1 ]]; then
        log "  Retry $attempt/$MAX_RETRIES..."
        rm -rf "$out_ho"
        sleep 5
    fi
    if run_benchmark "$out_ho" "$THREADS" "$config_ho"; then
        success=true
        break
    fi
done

tp_ho="N/A"
if [[ -f "$out_ho/summary.txt" ]]; then
    tp_ho=$(grep "Aggregate throughput" "$out_ho/summary.txt" | awk '{print $3}')
    log "EPaxos-HO: throughput=${tp_ho} ops/sec"
else
    log "WARNING: EPaxos-HO run failed"
fi
rm -f "$config_ho"

sleep 5

# --- Run 2: EPaxos (vanilla, control) ---
log "=== EPaxos (vanilla, weakRatio=0, writes=50, t=$THREADS) ==="
config_ep="/tmp/eval-phase108d-epaxos-$$.conf"
cp "$CONFIG_TEMPLATE" "$config_ep"
sed -i -E "s/^protocol:.*$/protocol: epaxos/" "$config_ep"
sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config_ep"
sed -i -E "s/^weakRatio:.*$/weakRatio:   0/" "$config_ep"
sed -i -E "s/^writes:.*$/writes:      50/" "$config_ep"
sed -i -E "s/^weakWrites:.*$/weakWrites:  0/" "$config_ep"
sed -i -E "s/^fast:.*$/fast:       true/" "$config_ep"

out_ep="$BASE_DIR/epaxos-t${THREADS}"
success=false
for attempt in $(seq 1 $MAX_RETRIES); do
    if [[ $attempt -gt 1 ]]; then
        log "  Retry $attempt/$MAX_RETRIES..."
        rm -rf "$out_ep"
        sleep 5
    fi
    if run_benchmark "$out_ep" "$THREADS" "$config_ep"; then
        success=true
        break
    fi
done

tp_ep="N/A"
if [[ -f "$out_ep/summary.txt" ]]; then
    tp_ep=$(grep "Aggregate throughput" "$out_ep/summary.txt" | awk '{print $3}')
    log "EPaxos: throughput=${tp_ep} ops/sec"
else
    log "WARNING: EPaxos run failed"
fi
rm -f "$config_ep"

# --- Summary ---
echo ""
log "=========================================="
log "Phase 108d Spot Test Results (t=$THREADS, w50%)"
log "  EPaxos-HO: ${tp_ho} ops/sec"
log "  EPaxos:    ${tp_ep} ops/sec"
log "=========================================="

# Write machine-readable summary
cat > "$BASE_DIR/spot-summary.txt" << EOFSUM
Phase 108d Spot Test (t=$THREADS, w50%)
EPaxos-HO: ${tp_ho} ops/sec
EPaxos:    ${tp_ep} ops/sec
EOFSUM

log "Results in $BASE_DIR/"
