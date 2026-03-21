#!/bin/bash

# Exp 3.2 CDF Data Collection — T Property verification via latency distributions.
#
# Collects CDF data at 3 weak ratios (0%, 50%, 100%) for 3 protocols.
# Shows how strong latency distribution remains stable as weak ratio increases (T property).
#
# Usage: bash scripts/eval-exp3.2-cdf-dist.sh [output-dir]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

BASE_DIR="${1:-results/eval-dist-cdf}"
EXP_DIR="$BASE_DIR/exp3.2"
FIXED_THREADS=8
WEAK_RATIOS=(0 50 100)
MAX_RETRIES=2

CONFIG="/tmp/eval-exp3.2-cdf-$$.conf"
cp multi-client.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

PROTOCOLS=("curpho" "curpht" "raftht")

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

apply_config() {
    local conf="$1" protocol="$2" weak_ratio="$3" writes="$4" weak_writes="$5"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$conf"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $weak_ratio/" "$conf"
    sed -i -E "s/^writes:.*$/writes:      $writes/" "$conf"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  $weak_writes/" "$conf"
}

ensure_clean() {
    for host in 130.245.173.101 130.245.173.103 130.245.173.104; do
        ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "$host" "pkill -9 -x swiftpaxos-dist" 2>/dev/null || true
    done
    sleep 3
}

run_benchmark() {
    local out_dir="$1" threads="$2"
    mkdir -p "$out_dir"
    timeout 300 ./run-multi-client.sh -d -c "$CONFIG" -t "$threads" -o "$out_dir" \
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

log "Exp 3.2 CDF Collection (Distributed): T Property at w={${WEAK_RATIOS[*]}}, t=$FIXED_THREADS"
log "Output: $EXP_DIR"
echo ""

ensure_clean

total=$(( ${#PROTOCOLS[@]} * ${#WEAK_RATIOS[@]} ))
run_idx=0

for protocol in "${PROTOCOLS[@]}"; do
    log "=== Protocol: $protocol ==="

    for ratio in "${WEAK_RATIOS[@]}"; do
        run_idx=$((run_idx + 1))
        out_dir="$EXP_DIR/$protocol/w${ratio}"

        log "  [$run_idx/$total] weakRatio=$ratio -> $out_dir"

        # 5% writes / 95% reads (matches Phase 65 Exp 3.2 config)
        apply_config "$CONFIG" "$protocol" "$ratio" "5" "5"

        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir" "$FIXED_THREADS"; then
                success=true
                break
            fi
        done

        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            log "  Result: throughput=${tp} ops/sec"
        fi

        if [[ -f "$out_dir/latencies.json" ]]; then
            size=$(wc -c < "$out_dir/latencies.json")
            log "  Latencies: ${size} bytes"
        else
            log "  WARNING: No latencies.json generated"
        fi

        if ! $success; then
            log "  WARNING: Run failed after $MAX_RETRIES attempts"
        fi

        sleep 5
    done
    echo ""
done

log ""
log "Exp 3.2 CDF collection complete! Results: $EXP_DIR/"
