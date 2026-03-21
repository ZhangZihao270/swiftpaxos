#!/bin/bash

# Phase 118e: Spot test — verify s_p50 parity after fair strong path fix
#
# Runs 4 benchmarks at t=8, w50%, weakRatio=50%:
# 1. mongotunable
# 2. pileus
# 3. raftht (control)
# 4. raft (control — all-strong baseline)
#
# Expected after fix: mongotunable/pileus s_p50 ≈ 85ms (same as Raft)
# Causal ops: w_p50 should be much lower (fast reply at commit time)

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase118e-$DATE}"
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

extract_latency() {
    local summary="$1"
    if [[ -f "$summary" ]]; then
        local s_p50 w_p50
        s_p50=$(grep "Avg median" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
        w_p50=$(grep "Avg median" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
        echo "${s_p50:-N/A} ${w_p50:-N/A}"
    else
        echo "N/A N/A"
    fi
}

log "Phase 118e: Spot test — s_p50 parity verification (t=$THREADS, w50%, weakRatio=50%)"
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

# Protocols to test
declare -A PROTOS
PROTOS[mongotunable]="mongotunable"
PROTOS[pileus]="pileus"
PROTOS[raftht]="raftht"
PROTOS[raft]="raft"

declare -A WEAK_RATIOS
WEAK_RATIOS[mongotunable]=50
WEAK_RATIOS[pileus]=50
WEAK_RATIOS[raftht]=50
WEAK_RATIOS[raft]=0

ORDER=(mongotunable pileus raftht raft)

for proto in "${ORDER[@]}"; do
    wr=${WEAK_RATIOS[$proto]}
    log "=== $proto (weakRatio=$wr, writes=50, t=$THREADS) ==="
    config="/tmp/eval-phase118e-${proto}-$$.conf"
    cp "$CONFIG_TEMPLATE" "$config"
    sed -i -E "s/^protocol:.*$/protocol: $proto/" "$config"
    sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $wr/" "$config"
    sed -i -E "s/^writes:.*$/writes:      50/" "$config"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$config"
    sed -i -E "s/^fast:.*$/fast:       false/" "$config"

    out="$BASE_DIR/${proto}-t${THREADS}"
    success=false
    for attempt in $(seq 1 $MAX_RETRIES); do
        if [[ $attempt -gt 1 ]]; then
            log "  Retry $attempt/$MAX_RETRIES..."
            rm -rf "$out"
            sleep 5
        fi
        if run_benchmark "$out" "$THREADS" "$config"; then
            success=true
            break
        fi
    done

    if [[ -f "$out/summary.txt" ]]; then
        tp=$(grep "Aggregate throughput" "$out/summary.txt" | awk '{print $3}')
        read s_p50 w_p50 <<< $(extract_latency "$out/summary.txt")
        log "  $proto: throughput=${tp} ops/sec, s_p50=${s_p50}ms, w_p50=${w_p50}ms"
        eval "tp_${proto}=$tp; sp50_${proto}=$s_p50; wp50_${proto}=$w_p50"
    else
        log "  WARNING: $proto run failed"
        eval "tp_${proto}=N/A; sp50_${proto}=N/A; wp50_${proto}=N/A"
    fi
    rm -f "$config"
    sleep 5
done

# --- Summary ---
echo ""
log "=========================================="
log "Phase 118e Spot Test Results (t=$THREADS, w50%, weakRatio=50%)"
log "  Protocol        Throughput    s_p50    w_p50"
log "  mongotunable    ${tp_mongotunable}    ${sp50_mongotunable}ms    ${wp50_mongotunable}ms"
log "  pileus          ${tp_pileus}    ${sp50_pileus}ms    ${wp50_pileus}ms"
log "  raftht          ${tp_raftht}    ${sp50_raftht}ms    ${wp50_raftht}ms"
log "  raft            ${tp_raft}    ${sp50_raft}ms    ${wp50_raft}ms"
log ""
log "Expected: mongotunable/pileus s_p50 ≈ raft s_p50 ≈ 85ms"
log "=========================================="

cat > "$BASE_DIR/spot-summary.txt" << EOFSUM
Phase 118e Spot Test (t=$THREADS, w50%, weakRatio=50%)
Protocol        Throughput    s_p50    w_p50
mongotunable    ${tp_mongotunable}    ${sp50_mongotunable}ms    ${wp50_mongotunable}ms
pileus          ${tp_pileus}    ${sp50_pileus}ms    ${wp50_pileus}ms
raftht          ${tp_raftht}    ${sp50_raftht}ms    ${wp50_raftht}ms
raft            ${tp_raft}    ${sp50_raft}ms    ${wp50_raft}ms
EOFSUM

log "Results in $BASE_DIR/"
