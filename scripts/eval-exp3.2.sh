#!/bin/bash

# Exp 3.2: T Property Verification — Weak Ratio Sweep
#
# Sweeps weak proportion for 3 protocols at fixed concurrency, measuring
# strong op throughput and latency stability.
# Workload: 50/50 read/write, sweep weakRatio (0-100%), zipfian keys.
#
# Expected: Raft-HT and CURP-HT show flat strong throughput (T satisfied).
#           CURP-HO shows declining strong throughput (T violated).
#
# Usage: bash scripts/eval-exp3.2.sh [output-dir]
# Output: results/eval-local-YYYYMMDD/exp3.2/

set -e

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-local-$DATE}"
EXP_DIR="$BASE_DIR/exp3.2"
CONFIG="eval-local.conf"
FIXED_THREADS=8
WEAK_RATIOS=(0 25 50 75 100)

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

log "Exp 3.2: T Property Verification (Weak Ratio Sweep)"
log "Protocols: ${PROTOCOLS[*]}"
log "Weak ratios: ${WEAK_RATIOS[*]}"
log "Fixed threads: $FIXED_THREADS"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos . 2>&1

total_runs=$(( ${#PROTOCOLS[@]} * ${#WEAK_RATIOS[@]} ))
run_idx=0

for protocol in "${PROTOCOLS[@]}"; do
    log "=== Protocol: $protocol ==="

    for ratio in "${WEAK_RATIOS[@]}"; do
        run_idx=$((run_idx + 1))
        out_dir="$EXP_DIR/$protocol/w${ratio}"
        mkdir -p "$out_dir"

        log "  [$run_idx/$total_runs] weakRatio=$ratio -> $out_dir"

        # Exp 3.2 uses 50/50 read/write (writes=50, weakWrites=50)
        apply_config "$CONFIG" "$protocol" "$ratio" "50" "50"

        # Run benchmark
        timeout 300 ./run-local-multi.sh -c "$CONFIG" -t "$FIXED_THREADS" -o "$out_dir" \
            > "$out_dir/run-output.txt" 2>&1 || {
            log "  WARNING: Run failed or timed out (exit=$?)"
        }

        # Brief summary
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            s_med=$(grep "Avg median" "$out_dir/summary.txt" | head -1 | grep -oP '[\d.]+ms' | head -2 | tail -1)
            log "  Result: throughput=${tp} ops/sec, strong_median=${s_med}"
        else
            log "  WARNING: No summary.txt generated"
        fi

        sleep 2
    done
    echo ""
done

# Restore config defaults
apply_config "$CONFIG" "curpht" "50" "5" "5"

# Collect results to CSV
log "Collecting results..."
bash scripts/collect-results.sh sweep "$EXP_DIR" "$BASE_DIR/summary-exp3.2.csv"

log ""
log "Exp 3.2 complete! Results: $BASE_DIR/summary-exp3.2.csv"
