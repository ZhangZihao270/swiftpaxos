#!/bin/bash

# Phase 105: EPaxos-HO Throughput-Latency Benchmark
#
# Same setup as Phase 104 but using EPaxos-HO (hybrid, weakRatio=50, weakWrites=50).
# 5 replicas on 5 machines, 3 clients on .101/.103/.104
# reqs=3000, networkDelay=25, commandSize=100
#
# Usage: bash scripts/eval-phase105-epaxosho.sh [output-dir] [write-filter]
#   write-filter: all (default), w5, w50

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase105-$DATE}"
WRITE_FILTER="${2:-all}"
THREAD_COUNTS=(1 2 4 8 16 32 64 96)
MAX_RETRIES=3

ALL_HOSTS=(130.245.173.101 130.245.173.103 130.245.173.104 130.245.173.125 130.245.173.126)

CONFIG_TEMPLATE="benchmark-5r-5m-3c.conf"
if [[ ! -f "$CONFIG_TEMPLATE" ]]; then
    echo "ERROR: Config template $CONFIG_TEMPLATE not found"
    exit 1
fi

# Write ratio configs: label:writes
declare -a ALL_WRITE_CONFIGS=(
    "w5:5"
    "w50:50"
)

# Filter write configs
declare -a WRITE_CONFIGS=()
for wc in "${ALL_WRITE_CONFIGS[@]}"; do
    IFS=':' read -r label w <<< "$wc"
    if [[ "$WRITE_FILTER" == "all" || "$WRITE_FILTER" == "$label" ]]; then
        WRITE_CONFIGS+=("$wc")
    fi
done

if [[ ${#WRITE_CONFIGS[@]} -eq 0 ]]; then
    echo "ERROR: No write configs matched filter '$WRITE_FILTER' (valid: all, w5, w50)"
    exit 1
fi

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

make_config() {
    local out="$1" writes="$2"
    cp "$CONFIG_TEMPLATE" "$out"
    sed -i -E "s/^protocol:.*$/protocol: epaxosho/" "$out"
    sed -i -E "s/^reqs:.*$/reqs:        3000/" "$out"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$out"
    sed -i -E "s/^writes:.*$/writes:      $writes/" "$out"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$out"
    # EPaxos-HO is leaderless, uses fast path
    sed -i -E "s/^fast:.*$/fast:       true/" "$out"
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

log "Phase 105: EPaxos-HO Throughput-Latency Benchmark (5r/5m/3c)"
log "Write configs: $(printf '%s ' "${WRITE_CONFIGS[@]}" | sed 's/:[^ ]*//g')"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Output: $BASE_DIR"
echo ""

# Build
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1
if [[ $? -ne 0 ]]; then
    log "ERROR: Build failed"
    exit 1
fi

# Initial cleanup
ensure_clean

for wc_spec in "${WRITE_CONFIGS[@]}"; do
    IFS=':' read -r wc_label writes <<< "$wc_spec"
    EXP_DIR="$BASE_DIR/exp1.1-${wc_label}"
    mkdir -p "$EXP_DIR"

    log "======== Write config: $wc_label (writes=$writes) ========"

    total_runs=${#THREAD_COUNTS[@]}
    run_idx=0

    proto_config="/tmp/eval-phase105-epaxosho-${wc_label}-$$.conf"
    make_config "$proto_config" "$writes"

    log "=== Protocol: epaxosho (weakRatio=50, weakWrites=50, writes=$writes) ==="

    for threads in "${THREAD_COUNTS[@]}"; do
        run_idx=$((run_idx + 1))
        out_dir="$EXP_DIR/epaxosho/t${threads}"

        # Skip if results already exist (allows resuming)
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            if [[ "$tp" != "0.00" && -n "$tp" ]]; then
                log "  [$run_idx/$total_runs] threads=$threads -> SKIP (already done: ${tp} ops/sec)"
                continue
            fi
        fi

        log "  [$run_idx/$total_runs] threads=$threads -> $out_dir"

        # Run with retry
        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir" "$threads" "$proto_config"; then
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

    rm -f "$proto_config"

    # Collect results for this write config
    log "Collecting results for $wc_label..."
    bash scripts/collect-results.sh throughput "$EXP_DIR" "$BASE_DIR/summary-exp1.1-${wc_label}.csv" 2>/dev/null || true
done

log ""
log "Phase 105 complete! Results in $BASE_DIR/"
