#!/bin/bash

# Phase 110.1d: Re-run Exp 2.2 with Fixed Zipf Sampler
#
# Same as Phase 110 eval but with the CDF-based Zipf sampler fix applied.
# Now s ≤ 1 values (0.25, 0.5, 0.75, 0.9, 0.99) use proper Zipf distributions
# instead of being silently clamped to 1.01.
#
# Runs both EPaxos and EPaxos-HO at t=32, w50%, across zipfSkew values.
# Higher zipfSkew = more key concentration = more conflicts.
# EPaxos-HO uses weakRatio=50, weakWrites=50.
#
# Usage: bash scripts/eval-phase110.1d-zipf-rerun.sh [output-dir] [protocol-filter]
#   protocol-filter: all (default), epaxos, epaxosho

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase110.1d-zipf-$DATE}"
PROTO_FILTER="${2:-all}"
THREADS=32
ZIPF_SKEWS=(0 0.25 0.5 0.75 0.99 1.2 1.5 2.0)
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

log "Phase 110.1d: Re-run Exp 2.2 — EPaxos vs EPaxos-HO under Zipf Skew (fixed sampler)"
log "Threads: $THREADS, Writes: 50%, KeySpace: 1000000"
log "Zipf skews: ${ZIPF_SKEWS[*]}"
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

# --- EPaxos runs ---
if [[ "$PROTO_FILTER" == "all" || "$PROTO_FILTER" == "epaxos" ]]; then
    log "=== EPaxos (vanilla, weakRatio=0, writes=50%) ==="
    for skew in "${ZIPF_SKEWS[@]}"; do
        skew_label=$(echo "$skew" | tr '.' '_')
        out_dir="$BASE_DIR/epaxos/z${skew_label}"

        # Skip if already done
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            if [[ "$tp" != "0.00" && -n "$tp" ]]; then
                log "  zipfSkew=$skew -> SKIP (already done: ${tp} ops/sec)"
                continue
            fi
        fi

        config="/tmp/eval-phase110.1d-epaxos-z${skew_label}-$$.conf"
        cp "$CONFIG_TEMPLATE" "$config"
        sed -i -E "s/^protocol:.*$/protocol: epaxos/" "$config"
        sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config"
        sed -i -E "s/^weakRatio:.*$/weakRatio:   0/" "$config"
        sed -i -E "s/^writes:.*$/writes:      50/" "$config"
        sed -i -E "s/^weakWrites:.*$/weakWrites:  0/" "$config"
        sed -i -E "s/^conflicts:.*$/conflicts:   0/" "$config"
        sed -i -E "s/^zipfSkew:.*$/zipfSkew:    $skew/" "$config"
        sed -i -E "s/^fast:.*$/fast:       true/" "$config"

        log "  zipfSkew=$skew -> $out_dir"

        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir" "$THREADS" "$config"; then
                success=true
                break
            fi
        done

        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            log "  Result: throughput=${tp} ops/sec"
        else
            log "  WARNING: No summary.txt generated"
        fi

        rm -f "$config"
        sleep 5
    done
    echo ""
fi

# --- EPaxos-HO runs ---
if [[ "$PROTO_FILTER" == "all" || "$PROTO_FILTER" == "epaxosho" ]]; then
    log "=== EPaxos-HO (weakRatio=50, weakWrites=50, writes=50%) ==="
    for skew in "${ZIPF_SKEWS[@]}"; do
        skew_label=$(echo "$skew" | tr '.' '_')
        out_dir="$BASE_DIR/epaxosho/z${skew_label}"

        # Skip if already done
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            if [[ "$tp" != "0.00" && -n "$tp" ]]; then
                log "  zipfSkew=$skew -> SKIP (already done: ${tp} ops/sec)"
                continue
            fi
        fi

        config="/tmp/eval-phase110.1d-epaxosho-z${skew_label}-$$.conf"
        cp "$CONFIG_TEMPLATE" "$config"
        sed -i -E "s/^protocol:.*$/protocol: epaxosho/" "$config"
        sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config"
        sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$config"
        sed -i -E "s/^writes:.*$/writes:      50/" "$config"
        sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$config"
        sed -i -E "s/^conflicts:.*$/conflicts:   0/" "$config"
        sed -i -E "s/^zipfSkew:.*$/zipfSkew:    $skew/" "$config"
        sed -i -E "s/^fast:.*$/fast:       true/" "$config"

        log "  zipfSkew=$skew -> $out_dir"

        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir" "$THREADS" "$config"; then
                success=true
                break
            fi
        done

        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            log "  Result: throughput=${tp} ops/sec"
        else
            log "  WARNING: No summary.txt generated"
        fi

        rm -f "$config"
        sleep 5
    done
    echo ""
fi

# --- Summary ---
log ""
log "Phase 110.1d complete! Results in $BASE_DIR/"
log ""
log "=== Summary ==="
for proto in epaxos epaxosho; do
    if [[ -d "$BASE_DIR/$proto" ]]; then
        log "--- $proto ---"
        for skew in "${ZIPF_SKEWS[@]}"; do
            skew_label=$(echo "$skew" | tr '.' '_')
            summary="$BASE_DIR/$proto/z${skew_label}/summary.txt"
            if [[ -f "$summary" ]]; then
                tp=$(grep "Aggregate throughput" "$summary" | awk '{print $3}')
                p50=$(grep "Aggregate s_p50" "$summary" | awk '{print $3}')
                log "  zipfSkew=$skew: tp=${tp} ops/sec, s_p50=${p50}ms"
            else
                log "  zipfSkew=$skew: MISSING"
            fi
        done
    fi
done
