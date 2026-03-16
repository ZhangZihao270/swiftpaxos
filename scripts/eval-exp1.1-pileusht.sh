#!/bin/bash

# Exp 1.1 (Pileus-HT only): Throughput vs Latency
#
# 5 replicas on 5 separate machines, 3 clients co-located with replicas 0-2.
# Sweeps thread counts across 2 write groups (5% and 50%), 1 rep each.
# 1 protocol × 8 threads × 2 write groups × 1 rep = 16 runs
#
# Usage: bash scripts/eval-exp1.1-pileusht.sh [output-dir]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-exp1.1-pileusht-$DATE}"
EXP_DIR="$BASE_DIR/exp1.1"
THREAD_COUNTS=(1 2 4 8 16 32 64 96)
WRITE_GROUPS=(5 50)
REPS=1
MAX_RETRIES=2

ALL_HOSTS=(130.245.173.101 130.245.173.103 130.245.173.104 130.245.173.125 130.245.173.126)

CONFIG="/tmp/eval-exp1.1-pileusht-$$.conf"
cp configs/exp1.1-base.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

apply_config() {
    local conf="$1" writes="$2" weak_writes="$3"
    sed -i -E "s/^protocol:.*$/protocol: pileusht/" "$conf"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$conf"
    sed -i -E "s/^writes:.*$/writes:      $writes/" "$conf"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  $weak_writes/" "$conf"
    sed -i -E "s/^fast:.*$/fast:       false/" "$conf"
}

ensure_clean() {
    for host in "${ALL_HOSTS[@]}"; do
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

log "Exp 1.1 (Pileus-HT): Throughput vs Latency"
log "Layout: 5 replicas on 5 machines, 3 clients"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Write groups: ${WRITE_GROUPS[*]}"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos-dist . 2>&1
if [[ $? -ne 0 ]]; then
    log "ERROR: Build failed"
    exit 1
fi

ensure_clean

total_runs=$(( ${#THREAD_COUNTS[@]} * ${#WRITE_GROUPS[@]} * REPS ))
run_idx=0

for W in "${WRITE_GROUPS[@]}"; do
    log "====== Write Group: ${W}% ======"

    for threads in "${THREAD_COUNTS[@]}"; do
        for rep in $(seq 1 $REPS); do
            run_idx=$((run_idx + 1))
            out_dir="$EXP_DIR/w${W}/pileusht/t${threads}/run${rep}"

            log "  [$run_idx/$total_runs] w=$W pileusht t=$threads rep=$rep -> $out_dir"

            apply_config "$CONFIG" "$W" "$W"

            success=false
            for attempt in $(seq 1 $MAX_RETRIES); do
                if [[ $attempt -gt 1 ]]; then
                    log "  Retry $attempt/$MAX_RETRIES..."
                    rm -rf "$out_dir"
                    sleep 5
                fi
                if run_benchmark "$out_dir" "$threads"; then
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

            if ! $success; then
                log "  WARNING: Run failed after $MAX_RETRIES attempts"
            fi

            sleep 5
        done
    done
    echo ""
done

# Generate summary CSV
log "Generating summary CSV..."
SUMMARY_CSV="$BASE_DIR/summary-exp1.1-pileusht.csv"
echo "write_group,protocol,threads,avg_throughput,avg_s_p50,avg_s_p99,avg_w_p50,avg_w_p99" > "$SUMMARY_CSV"

for W in "${WRITE_GROUPS[@]}"; do
    for threads in "${THREAD_COUNTS[@]}"; do
        sum_tp=0; sum_s50=0; sum_s99=0; sum_w50=0; sum_w99=0; count=0
        for rep in $(seq 1 $REPS); do
            summary="$EXP_DIR/w${W}/pileusht/t${threads}/run${rep}/summary.txt"
            if [[ -f "$summary" ]]; then
                tp=$(grep "Aggregate throughput" "$summary" | awk '{print $3}')
                s50=$(grep "Avg median" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
                s99=$(grep "Avg p99" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
                w50=$(grep "Avg median" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
                w99=$(grep "Avg p99" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
                sum_tp=$(echo "$sum_tp + ${tp:-0}" | bc)
                sum_s50=$(echo "$sum_s50 + ${s50:-0}" | bc)
                sum_s99=$(echo "$sum_s99 + ${s99:-0}" | bc)
                sum_w50=$(echo "$sum_w50 + ${w50:-0}" | bc)
                sum_w99=$(echo "$sum_w99 + ${w99:-0}" | bc)
                count=$((count + 1))
            fi
        done
        if [[ $count -gt 0 ]]; then
            avg_tp=$(echo "scale=2; $sum_tp / $count" | bc)
            avg_s50=$(echo "scale=2; $sum_s50 / $count" | bc)
            avg_s99=$(echo "scale=2; $sum_s99 / $count" | bc)
            avg_w50=$(echo "scale=2; $sum_w50 / $count" | bc)
            avg_w99=$(echo "scale=2; $sum_w99 / $count" | bc)
            echo "$W,pileusht,$threads,$avg_tp,$avg_s50,$avg_s99,$avg_w50,$avg_w99" >> "$SUMMARY_CSV"
        fi
    done
done

log ""
log "Exp 1.1 (Pileus-HT) complete! Results: $SUMMARY_CSV"
