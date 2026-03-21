#!/bin/bash

# Phase 96: Exp 3.2 — T Property Verification (5r/5m/3c)
#
# Sweep weakRatio (0/25/50/75/100) at fixed t=8 to verify T property:
#   strong op P50 should remain stable as weak ratio increases.
# Protocols: curpht, curpho (no baseline since weakRatio=0 is already pure strong)
# 5 replicas on 5 machines, 3 clients on .101/.103/.104
# reqs=3000, networkDelay=25, commandSize=100, writes=50, weakWrites=50
#
# Usage: bash scripts/eval-phase96.sh [output-dir] [protocol-filter]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase96-$DATE}"
PROTO_FILTER="${2:-all}"
EXP_DIR="$BASE_DIR/exp3.2"
FIXED_THREADS=8
WEAK_RATIOS=(0 25 50 75 100)
MAX_RETRIES=3

ALL_HOSTS=(130.245.173.101 130.245.173.103 130.245.173.104 130.245.173.125 130.245.173.126)

PROTOCOLS_ALL=("curpht" "curpho")

# Filter protocols
declare -a PROTOCOLS=()
for p in "${PROTOCOLS_ALL[@]}"; do
    if [[ "$PROTO_FILTER" == "all" || "$PROTO_FILTER" == "$p" ]]; then
        PROTOCOLS+=("$p")
    fi
done

if [[ ${#PROTOCOLS[@]} -eq 0 ]]; then
    echo "ERROR: No protocols matched filter '$PROTO_FILTER'"
    echo "Valid: all, curpht, curpho"
    exit 1
fi

CONFIG_TEMPLATE="benchmark-5r-5m-3c.conf"
if [[ ! -f "$CONFIG_TEMPLATE" ]]; then
    echo "ERROR: Config template $CONFIG_TEMPLATE not found"
    exit 1
fi

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

make_config() {
    local out="$1" protocol="$2" weak_ratio="$3"
    cp "$CONFIG_TEMPLATE" "$out"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$out"
    sed -i -E "s/^reqs:.*$/reqs:        3000/" "$out"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $weak_ratio/" "$out"
    sed -i -E "s/^writes:.*$/writes:      50/" "$out"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$out"
    sed -i -E "s/^clientThreads:.*$/clientThreads: $FIXED_THREADS/" "$out"
}

ensure_clean() {
    for host in "${ALL_HOSTS[@]}"; do
        ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "$host" "pkill -9 -x swiftpaxos-dist" 2>/dev/null || true
    done
    sleep 3
}

run_benchmark() {
    local out_dir="$1" config="$2"
    mkdir -p "$out_dir"
    timeout 300 ./run-multi-client.sh -d -c "$config" -t "$FIXED_THREADS" -o "$out_dir" \
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

log "Phase 96: Exp 3.2 — T Property Verification (5r/5m/3c)"
log "Protocols: ${PROTOCOLS[*]}"
log "Weak ratios: ${WEAK_RATIOS[*]}"
log "Fixed threads: $FIXED_THREADS, writes=50"
log "Output: $EXP_DIR"
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

total_runs=$(( ${#PROTOCOLS[@]} * ${#WEAK_RATIOS[@]} ))
run_idx=0

for protocol in "${PROTOCOLS[@]}"; do
    log "=== Protocol: $protocol ==="

    for ratio in "${WEAK_RATIOS[@]}"; do
        run_idx=$((run_idx + 1))
        out_dir="$EXP_DIR/$protocol/w${ratio}"

        # Skip if results already exist (allows resuming)
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            if [[ "$tp" != "0.00" && -n "$tp" ]]; then
                log "  [$run_idx/$total_runs] weakRatio=$ratio -> SKIP (already done: ${tp} ops/sec)"
                continue
            fi
        fi

        log "  [$run_idx/$total_runs] weakRatio=$ratio -> $out_dir"

        # Create config for this run
        proto_config="/tmp/eval-phase96-${protocol}-w${ratio}-$$.conf"
        make_config "$proto_config" "$protocol" "$ratio"

        # Run with retry
        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir" "$proto_config"; then
                success=true
                break
            fi
        done

        rm -f "$proto_config"

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
bash scripts/collect-results.sh sweep "$EXP_DIR" "$BASE_DIR/summary-exp3.2.csv"

log ""
log "Phase 96 complete! Results: $BASE_DIR/summary-exp3.2.csv"
