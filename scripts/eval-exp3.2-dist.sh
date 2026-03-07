#!/bin/bash

# Exp 3.2: T Property Verification — Weak Ratio Sweep (Distributed)
#
# Sweeps weak proportion for 3 protocols at fixed concurrency on distributed machines,
# measuring strong op throughput and latency stability.
# Workload: 50/50 read/write, sweep weakRatio (0-100%), zipfian keys.
#
# Expected: Raft-HT and CURP-HT show flat strong throughput (T satisfied).
#           CURP-HO shows declining strong throughput (T violated).
#
# Usage: bash scripts/eval-exp3.2-dist.sh [output-dir]
# Output: results/eval-dist-YYYYMMDD/exp3.2/

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-dist-$DATE}"
EXP_DIR="$BASE_DIR/exp3.2"
FIXED_THREADS=8
WEAK_RATIOS=(0 25 50 75 100)
MAX_RETRIES=2

# Use a temp copy of the config to avoid file-watcher interference
CONFIG="/tmp/eval-exp3.2-dist-$$.conf"
cp multi-client.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocols to test
PROTOCOLS=("raftht" "curpht" "curpho")

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
    local out_dir="$1"
    mkdir -p "$out_dir"
    timeout 300 ./run-multi-client.sh -d -c "$CONFIG" -t "$FIXED_THREADS" -o "$out_dir" \
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

log "Exp 3.2 (Distributed): T Property Verification (Weak Ratio Sweep)"
log "Protocols: ${PROTOCOLS[*]}"
log "Weak ratios: ${WEAK_RATIOS[*]}"
log "Fixed threads: $FIXED_THREADS"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1

# Initial cleanup
ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#WEAK_RATIOS[@]} ))
run_idx=0

for protocol in "${PROTOCOLS[@]}"; do
    log "=== Protocol: $protocol ==="

    for ratio in "${WEAK_RATIOS[@]}"; do
        run_idx=$((run_idx + 1))
        out_dir="$EXP_DIR/$protocol/w${ratio}"

        log "  [$run_idx/$total_runs] weakRatio=$ratio -> $out_dir"

        apply_config "$CONFIG" "$protocol" "$ratio" "50" "50"

        # Run with retry
        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir"; then
                success=true
                break
            fi
        done

        # Brief summary
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            s_med=$(grep "Avg median" "$out_dir/summary.txt" | head -1 | grep -oP '[\d.]+ms' | head -2 | tail -1)
            log "  Result: throughput=${tp} ops/sec, strong_median=${s_med}"
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
log "Exp 3.2 complete! Results: $BASE_DIR/summary-exp3.2.csv"
