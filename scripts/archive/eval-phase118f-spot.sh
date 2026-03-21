#!/bin/bash

# Phase 118f: Spot test — mongotunable + pileus at w5%, multiple thread counts
#
# Runs 2 protocols × 5 thread counts = 10 benchmarks
# Expected: s_p50 ≈ 85ms at low threads, throughput comparable to Raft-HT

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase118f-$DATE}"
THREAD_COUNTS=(1 8 32 64 96)
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

extract_stats() {
    local summary="$1"
    if [[ -f "$summary" ]]; then
        local tp s_p50 w_p50
        tp=$(grep "Aggregate throughput" "$summary" | awk '{print $3}')
        s_p50=$(grep "Avg median" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
        w_p50=$(grep "Avg median" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
        echo "${tp:-N/A} ${s_p50:-N/A} ${w_p50:-N/A}"
    else
        echo "N/A N/A N/A"
    fi
}

log "Phase 118f: Spot test — mongotunable + pileus at w5%, t={1,8,32,64,96}"
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

echo "protocol,threads,throughput,s_p50,w_p50" > "$BASE_DIR/summary.csv"

PROTOS=(mongotunable pileus)
run_idx=0
total_runs=$(( ${#PROTOS[@]} * ${#THREAD_COUNTS[@]} ))

for proto in "${PROTOS[@]}"; do
    for threads in "${THREAD_COUNTS[@]}"; do
        run_idx=$((run_idx + 1))
        log "[$run_idx/$total_runs] $proto t=$threads (w5%, weakRatio=50)"

        config="/tmp/eval-phase118f-${proto}-$$.conf"
        cp "$CONFIG_TEMPLATE" "$config"
        sed -i -E "s/^protocol:.*$/protocol: $proto/" "$config"
        sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config"
        sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$config"
        sed -i -E "s/^writes:.*$/writes:      5/" "$config"
        sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$config"
        sed -i -E "s/^fast:.*$/fast:       false/" "$config"

        out="$BASE_DIR/${proto}-t${threads}"
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
            read tp s_p50 w_p50 <<< $(extract_stats "$out/summary.txt")
            log "  Result: throughput=${tp}, s_p50=${s_p50}ms, w_p50=${w_p50}ms"
            echo "$proto,$threads,$tp,$s_p50,$w_p50" >> "$BASE_DIR/summary.csv"
        else
            log "  WARNING: $proto t=$threads failed"
            echo "$proto,$threads,N/A,N/A,N/A" >> "$BASE_DIR/summary.csv"
        fi
        rm -f "$config"
        sleep 5
    done
done

# --- Summary ---
echo ""
log "=========================================="
log "Phase 118f Results (w5%, weakRatio=50%)"
log ""
cat "$BASE_DIR/summary.csv" | column -t -s,
log ""
log "Expected: s_p50 ≈ 85ms at low threads"
log "=========================================="
log "Results in $BASE_DIR/"
