#!/bin/bash

# Phase 122f: Spot test — Pileus-HT at w5%, weakRatio=50%, t={1,8,32,96}
#
# Expected: Pileus-HT tput ≈ Raft-HT (same fast weak write path)
# Expected: Pileus-HT w_p50 ≈ Raft-HT for writes (immediate reply)
# Expected: Pileus-HT w_p50 slightly higher for reads (causal MinIndex wait)

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase122f-$DATE}"
MAX_RETRIES=3
THREAD_COUNTS=(1 8 32 96)

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

log "Phase 122f: Spot test — Pileus-HT v2 (w5%, weakRatio=50%)"
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

declare -a results

total=${#THREAD_COUNTS[@]}
run=0

for threads in "${THREAD_COUNTS[@]}"; do
    run=$((run + 1))
    log "[$run/$total] pileusht t=$threads (w5%, weakRatio=50)"
    config="/tmp/eval-phase122f-pileusht-${threads}-$$.conf"
    cp "$CONFIG_TEMPLATE" "$config"
    sed -i -E "s/^protocol:.*$/protocol: pileusht/" "$config"
    sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$config"
    sed -i -E "s/^writes:.*$/writes:      5/" "$config"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  5/" "$config"
    sed -i -E "s/^fast:.*$/fast:       false/" "$config"

    out="$BASE_DIR/pileusht/t${threads}"
    success=false
    for attempt in $(seq 1 $MAX_RETRIES); do
        if [[ $attempt -gt 1 ]]; then
            log "  Retry $attempt/$MAX_RETRIES..."
            rm -rf "$out"
            sleep 5
        fi
        if run_benchmark "$out" "$threads" "$config"; then
            success=true
            break
        fi
    done

    if [[ -f "$out/summary.txt" ]]; then
        tp=$(grep "Aggregate throughput" "$out/summary.txt" | awk '{print $3}')
        read s_p50 w_p50 <<< $(extract_latency "$out/summary.txt")
        log "  Result: throughput=$tp, s_p50=${s_p50}ms, w_p50=${w_p50}ms"
        results+=("pileusht $threads $tp $s_p50 $w_p50")
    else
        log "  WARNING: pileusht t=$threads failed"
        results+=("pileusht $threads 0.00 N/A N/A")
    fi
    rm -f "$config"
    sleep 5
done

# --- Summary ---
echo ""
log "=========================================="
log "Phase 122f Results (w5%, weakRatio=50%)"
log ""
printf "  %-16s %-8s %-12s %-10s %-10s\n" "Protocol" "Threads" "Throughput" "s_p50" "w_p50"
for r in "${results[@]}"; do
    read proto threads tp s_p50 w_p50 <<< "$r"
    printf "  %-16s %-8s %-12s %-10s %-10s\n" "$proto" "$threads" "$tp" "${s_p50}ms" "${w_p50}ms"
done
log ""
log "Phase 121e reference (w5%): Raft-HT t=96=33.5K, Pileus t=96=35.7K, Mongo t=96=32.6K"
log "Expected: Pileus-HT v2 ≈ Raft-HT (same engine, same fast weak write path)"
log "=========================================="

cat > "$BASE_DIR/spot-summary.csv" << EOFSUM
protocol,threads,throughput,s_p50,w_p50
$(for r in "${results[@]}"; do
    read proto threads tp s_p50 w_p50 <<< "$r"
    echo "$proto,$threads,$tp,$s_p50,$w_p50"
done)
EOFSUM

log "Results in $BASE_DIR/"
