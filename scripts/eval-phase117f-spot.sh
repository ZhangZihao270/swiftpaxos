#!/bin/bash

# Phase 117f: Spot test — MongoDB-Tunable and Pileus at t=8, w50%, weakRatio=50%
#
# Runs three benchmarks:
# 1. mongotunable with weakRatio=50, weakWrites=50, writes=50, t=8
# 2. pileus with weakRatio=50, weakWrites=50, writes=50, t=8
# 3. raftht (control) with weakRatio=50, weakWrites=50, writes=50, t=8
#
# Target: all three protocols complete without hangs/crashes

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase117f-$DATE}"
THREADS=8
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

log "Phase 117f: Spot test — MongoDB-Tunable, Pileus, Raft-HT at t=$THREADS, w50%, weakRatio=50%"
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

# --- Run 1: MongoDB-Tunable ---
log "=== mongotunable (weakRatio=50, weakWrites=50, writes=50, t=$THREADS) ==="
config_mt="/tmp/eval-phase117f-mongotunable-$$.conf"
cp "$CONFIG_TEMPLATE" "$config_mt"
sed -i -E "s/^protocol:.*$/protocol: mongotunable/" "$config_mt"
sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config_mt"
sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$config_mt"
sed -i -E "s/^writes:.*$/writes:      50/" "$config_mt"
sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$config_mt"
sed -i -E "s/^fast:.*$/fast:       false/" "$config_mt"

out_mt="$BASE_DIR/mongotunable-t${THREADS}"
success=false
for attempt in $(seq 1 $MAX_RETRIES); do
    if [[ $attempt -gt 1 ]]; then
        log "  Retry $attempt/$MAX_RETRIES..."
        rm -rf "$out_mt"
        sleep 5
    fi
    if run_benchmark "$out_mt" "$THREADS" "$config_mt"; then
        success=true
        break
    fi
done

tp_mt="N/A"
if [[ -f "$out_mt/summary.txt" ]]; then
    tp_mt=$(grep "Aggregate throughput" "$out_mt/summary.txt" | awk '{print $3}')
    log "mongotunable: throughput=${tp_mt} ops/sec"
else
    log "WARNING: mongotunable run failed"
fi
rm -f "$config_mt"

sleep 5

# --- Run 2: Pileus ---
log "=== pileus (weakRatio=50, weakWrites=50, writes=50, t=$THREADS) ==="
config_pi="/tmp/eval-phase117f-pileus-$$.conf"
cp "$CONFIG_TEMPLATE" "$config_pi"
sed -i -E "s/^protocol:.*$/protocol: pileus/" "$config_pi"
sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config_pi"
sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$config_pi"
sed -i -E "s/^writes:.*$/writes:      50/" "$config_pi"
sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$config_pi"
sed -i -E "s/^fast:.*$/fast:       false/" "$config_pi"

out_pi="$BASE_DIR/pileus-t${THREADS}"
success=false
for attempt in $(seq 1 $MAX_RETRIES); do
    if [[ $attempt -gt 1 ]]; then
        log "  Retry $attempt/$MAX_RETRIES..."
        rm -rf "$out_pi"
        sleep 5
    fi
    if run_benchmark "$out_pi" "$THREADS" "$config_pi"; then
        success=true
        break
    fi
done

tp_pi="N/A"
if [[ -f "$out_pi/summary.txt" ]]; then
    tp_pi=$(grep "Aggregate throughput" "$out_pi/summary.txt" | awk '{print $3}')
    log "pileus: throughput=${tp_pi} ops/sec"
else
    log "WARNING: pileus run failed"
fi
rm -f "$config_pi"

sleep 5

# --- Run 3: Raft-HT (control) ---
log "=== raftht (weakRatio=50, weakWrites=50, writes=50, t=$THREADS, control) ==="
config_rht="/tmp/eval-phase117f-raftht-$$.conf"
cp "$CONFIG_TEMPLATE" "$config_rht"
sed -i -E "s/^protocol:.*$/protocol: raftht/" "$config_rht"
sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config_rht"
sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$config_rht"
sed -i -E "s/^writes:.*$/writes:      50/" "$config_rht"
sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$config_rht"
sed -i -E "s/^fast:.*$/fast:       false/" "$config_rht"

out_rht="$BASE_DIR/raftht-t${THREADS}"
success=false
for attempt in $(seq 1 $MAX_RETRIES); do
    if [[ $attempt -gt 1 ]]; then
        log "  Retry $attempt/$MAX_RETRIES..."
        rm -rf "$out_rht"
        sleep 5
    fi
    if run_benchmark "$out_rht" "$THREADS" "$config_rht"; then
        success=true
        break
    fi
done

tp_rht="N/A"
if [[ -f "$out_rht/summary.txt" ]]; then
    tp_rht=$(grep "Aggregate throughput" "$out_rht/summary.txt" | awk '{print $3}')
    log "raftht: throughput=${tp_rht} ops/sec"
else
    log "WARNING: raftht run failed"
fi
rm -f "$config_rht"

# --- Summary ---
echo ""
log "=========================================="
log "Phase 117f Spot Test Results (t=$THREADS, w50%, weakRatio=50%)"
log "  mongotunable: ${tp_mt} ops/sec"
log "  pileus:       ${tp_pi} ops/sec"
log "  raftht:       ${tp_rht} ops/sec (control)"
log "=========================================="

# Write machine-readable summary
cat > "$BASE_DIR/spot-summary.txt" << EOFSUM
Phase 117f Spot Test (t=$THREADS, w50%, weakRatio=50%)
mongotunable: ${tp_mt} ops/sec
pileus:       ${tp_pi} ops/sec
raftht:       ${tp_rht} ops/sec (control)
EOFSUM

log "Results in $BASE_DIR/"
