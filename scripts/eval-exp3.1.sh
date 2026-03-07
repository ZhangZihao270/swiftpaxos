#!/bin/bash

# Exp 3.1: CURP-HO vs CURP-HT vs Vanilla CURP — Throughput vs Latency
#
# Sweeps thread count for 3 protocols, measuring throughput and latency.
# Workload: 95/5 read/write, 50/50 strong/weak (0% weak for vanilla CURP), zipfian keys.
#
# Usage: bash scripts/eval-exp3.1.sh [output-dir]
# Output: results/eval-local-YYYYMMDD/exp3.1/

set -e

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-local-$DATE}"
EXP_DIR="$BASE_DIR/exp3.1"
CONFIG="eval-local.conf"
THREAD_COUNTS=(1 2 4 8 16 32)

# Protocol configs: name, protocol-value, weakRatio, writes, weakWrites
declare -a PROTOCOLS=(
    "curpho:curpho:50:5:5"
    "curpht:curpht:50:5:5"
    "curp:curp:0:5:5"
)

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

# Apply config overrides via sed
apply_config() {
    local conf="$1" protocol="$2" weak_ratio="$3" writes="$4" weak_writes="$5"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$conf"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $weak_ratio/" "$conf"
    sed -i -E "s/^writes:.*$/writes:      $writes/" "$conf"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  $weak_writes/" "$conf"
}

log "Exp 3.1: CURP Throughput vs Latency"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos . 2>&1

total_runs=$(( ${#PROTOCOLS[@]} * ${#THREAD_COUNTS[@]} ))
run_idx=0

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio writes weak_writes <<< "$proto_spec"

    log "=== Protocol: $name (protocol=$protocol, weakRatio=$weak_ratio) ==="

    for threads in "${THREAD_COUNTS[@]}"; do
        run_idx=$((run_idx + 1))
        out_dir="$EXP_DIR/$name/t${threads}"
        mkdir -p "$out_dir"

        log "  [$run_idx/$total_runs] threads=$threads -> $out_dir"

        # Apply config
        apply_config "$CONFIG" "$protocol" "$weak_ratio" "$writes" "$weak_writes"

        # Run benchmark
        timeout 300 ./run-local-multi.sh -c "$CONFIG" -t "$threads" -o "$out_dir" \
            > "$out_dir/run-output.txt" 2>&1 || {
            log "  WARNING: Run failed or timed out (exit=$?)"
        }

        # Brief summary
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            log "  Result: throughput=${tp} ops/sec"
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
bash scripts/collect-results.sh throughput "$EXP_DIR" "$BASE_DIR/summary-exp3.1.csv"

log ""
log "Exp 3.1 complete! Results: $BASE_DIR/summary-exp3.1.csv"
