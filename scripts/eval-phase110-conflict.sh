#!/bin/bash

# Phase 110: EPaxos vs EPaxos-HO under Varying Conflict Rates
#
# Runs both EPaxos and EPaxos-HO at t=32, w50%, across conflict rates 0,2,10,25,50,100.
# EPaxos-HO uses weakRatio=50, weakWrites=50.
#
# Usage: bash scripts/eval-phase110-conflict.sh [output-dir] [protocol-filter]
#   protocol-filter: all (default), epaxos, epaxosho

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase110-$DATE}"
PROTO_FILTER="${2:-all}"
THREADS=32
CONFLICT_RATES=(0 2 10 25 50 100)
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

log "Phase 110: EPaxos vs EPaxos-HO under Varying Conflict Rates"
log "Threads: $THREADS, Writes: 50%"
log "Conflict rates: ${CONFLICT_RATES[*]}"
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

# --- EPaxos runs (110a) ---
if [[ "$PROTO_FILTER" == "all" || "$PROTO_FILTER" == "epaxos" ]]; then
    log "=== EPaxos (vanilla, weakRatio=0, writes=50%) ==="
    for conflict in "${CONFLICT_RATES[@]}"; do
        out_dir="$BASE_DIR/epaxos/c${conflict}"

        # Skip if already done
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            if [[ "$tp" != "0.00" && -n "$tp" ]]; then
                log "  conflict=$conflict -> SKIP (already done: ${tp} ops/sec)"
                continue
            fi
        fi

        config="/tmp/eval-phase110-epaxos-c${conflict}-$$.conf"
        cp "$CONFIG_TEMPLATE" "$config"
        sed -i -E "s/^protocol:.*$/protocol: epaxos/" "$config"
        sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config"
        sed -i -E "s/^weakRatio:.*$/weakRatio:   0/" "$config"
        sed -i -E "s/^writes:.*$/writes:      50/" "$config"
        sed -i -E "s/^weakWrites:.*$/weakWrites:  0/" "$config"
        sed -i -E "s/^conflicts:.*$/conflicts:   $conflict/" "$config"
        sed -i -E "s/^fast:.*$/fast:       true/" "$config"

        log "  conflict=$conflict -> $out_dir"

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

# --- EPaxos-HO runs (110b) ---
if [[ "$PROTO_FILTER" == "all" || "$PROTO_FILTER" == "epaxosho" ]]; then
    log "=== EPaxos-HO (weakRatio=50, weakWrites=50, writes=50%) ==="
    for conflict in "${CONFLICT_RATES[@]}"; do
        out_dir="$BASE_DIR/epaxosho/c${conflict}"

        # Skip if already done
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            if [[ "$tp" != "0.00" && -n "$tp" ]]; then
                log "  conflict=$conflict -> SKIP (already done: ${tp} ops/sec)"
                continue
            fi
        fi

        config="/tmp/eval-phase110-epaxosho-c${conflict}-$$.conf"
        cp "$CONFIG_TEMPLATE" "$config"
        sed -i -E "s/^protocol:.*$/protocol: epaxosho/" "$config"
        sed -i -E "s/^reqs:.*$/reqs:        3000/" "$config"
        sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$config"
        sed -i -E "s/^writes:.*$/writes:      50/" "$config"
        sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$config"
        sed -i -E "s/^conflicts:.*$/conflicts:   $conflict/" "$config"
        sed -i -E "s/^fast:.*$/fast:       true/" "$config"

        log "  conflict=$conflict -> $out_dir"

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
log "=========================================="
log "Phase 110 Results Summary (t=$THREADS, w50%)"
log "=========================================="

echo ""
echo "conflict  epaxos_tput  epaxosho_tput"
echo "--------  -----------  -------------"
for conflict in "${CONFLICT_RATES[@]}"; do
    tp_ep="N/A"
    tp_ho="N/A"
    if [[ -f "$BASE_DIR/epaxos/c${conflict}/summary.txt" ]]; then
        tp_ep=$(grep "Aggregate throughput" "$BASE_DIR/epaxos/c${conflict}/summary.txt" | awk '{print $3}')
    fi
    if [[ -f "$BASE_DIR/epaxosho/c${conflict}/summary.txt" ]]; then
        tp_ho=$(grep "Aggregate throughput" "$BASE_DIR/epaxosho/c${conflict}/summary.txt" | awk '{print $3}')
    fi
    printf "%-8s  %-11s  %-13s\n" "$conflict%" "$tp_ep" "$tp_ho"
done

echo ""
log "Results in $BASE_DIR/"
