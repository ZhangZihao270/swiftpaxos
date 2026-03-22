#!/bin/bash

# Exp 3.1 (Final): CURP-HO vs CURP-HT vs Vanilla CURP — Throughput vs Latency
#
# 5 replicas on 5 separate machines, 3 clients co-located with replicas 0-2.
# Sweeps thread counts across 2 write groups (5% and 50%), 3 repetitions each.
#
# Protocols:
#   curpho       — protocol: curpho, weakRatio: 50
#   curpht       — protocol: curpht, weakRatio: 50
#   curp-baseline — protocol: curpht, weakRatio: 0 (all strong = vanilla CURP)
#
# Usage: bash scripts/eval-exp3.1-final.sh [output-dir]
# Output: <output-dir>/exp3.1/

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-exp3.1-$DATE}"
EXP_DIR="$BASE_DIR/exp3.1"
THREAD_COUNTS=(1 2 4 8 16 32 64 96)
WRITE_GROUPS=(5 50)
REPS=1
MAX_RETRIES=2

ALL_HOSTS=(13.58.3.12 100.26.140.158 52.35.107.158 3.250.222.202 15.222.250.161)

CONFIG="/tmp/eval-exp3.1-final-$$.conf"
cp configs/exp3.1-base.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocol configs: name, protocol-value, weakRatio
declare -a PROTOCOLS=(
    "curpho:curpho:50"
    "curpht:curpht:50"
    "curp-baseline:curpht:0"
)

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
    for host in "${ALL_HOSTS[@]}"; do
        ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "${SSH_USER:-$(whoami)}@$host" "pkill -9 -x swiftpaxos-dist" 2>/dev/null || true
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

cp "$SUMMARY_CSV" "$WORK_DIR/results/latest/exp3.1.csv" 2>/dev/null || true
log "Updated results/latest/exp3.1.csv"
log "Exp 3.1 (Final): CURP Throughput vs Latency"
log "Layout: 5 replicas on 5 machines, 3 clients"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Write groups: ${WRITE_GROUPS[*]}"
log "Repetitions: $REPS"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos-dist . 2>&1

# Initial cleanup
ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#THREAD_COUNTS[@]} * ${#WRITE_GROUPS[@]} * REPS ))
run_idx=0

for W in "${WRITE_GROUPS[@]}"; do
    log "====== Write Group: ${W}% ======"

    for proto_spec in "${PROTOCOLS[@]}"; do
        IFS=':' read -r name protocol weak_ratio <<< "$proto_spec"

        log "=== Protocol: $name (protocol=$protocol, weakRatio=$weak_ratio, writes=$W) ==="

        for threads in "${THREAD_COUNTS[@]}"; do
            for rep in $(seq 1 $REPS); do
                run_idx=$((run_idx + 1))
                out_dir="$EXP_DIR/w${W}/$name/t${threads}/run${rep}"

                log "  [$run_idx/$total_runs] w=$W $name t=$threads rep=$rep -> $out_dir"

                apply_config "$CONFIG" "$protocol" "$weak_ratio" "$W" "$W"

                # Run with retry
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
        done
        echo ""
    done
done

# Generate summary CSV with averaged results
log "Generating summary CSV..."
SUMMARY_CSV="$BASE_DIR/summary-exp3.1.csv"
echo "write_group,protocol,threads,avg_throughput,avg_s_p50,avg_s_p99,avg_w_p50,avg_w_p99" > "$SUMMARY_CSV"

for W in "${WRITE_GROUPS[@]}"; do
    for proto_spec in "${PROTOCOLS[@]}"; do
        IFS=':' read -r name protocol weak_ratio <<< "$proto_spec"
        for threads in "${THREAD_COUNTS[@]}"; do
            # Average across repetitions
            tp_sum=0; s_p50_sum=0; s_p99_sum=0; w_p50_sum=0; w_p99_sum=0; count=0
            for rep in $(seq 1 $REPS); do
                summary="$EXP_DIR/w${W}/$name/t${threads}/run${rep}/summary.txt"
                if [[ -f "$summary" ]]; then
                    tp=$(grep "Aggregate throughput" "$summary" | awk '{print $3}')
                    # Extract strong median and p99 (first "Avg median" line is strong)
                    s_p50=$(grep "Avg median" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
                    s_p99=$(grep "Avg p99" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
                    # Extract weak median and p99 (second "Avg median" line is weak)
                    w_p50=$(grep "Avg median" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
                    w_p99=$(grep "Avg p99" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
                    if [[ -n "$tp" && "$tp" != "0.00" ]]; then
                        tp_sum=$(echo "$tp_sum + $tp" | bc)
                        s_p50_sum=$(echo "$s_p50_sum + ${s_p50:-0}" | bc)
                        s_p99_sum=$(echo "$s_p99_sum + ${s_p99:-0}" | bc)
                        w_p50_sum=$(echo "$w_p50_sum + ${w_p50:-0}" | bc)
                        w_p99_sum=$(echo "$w_p99_sum + ${w_p99:-0}" | bc)
                        count=$((count + 1))
                    fi
                fi
            done
            if [[ $count -gt 0 ]]; then
                avg_tp=$(echo "scale=2; $tp_sum / $count" | bc)
                avg_s_p50=$(echo "scale=2; $s_p50_sum / $count" | bc)
                avg_s_p99=$(echo "scale=2; $s_p99_sum / $count" | bc)
                avg_w_p50=$(echo "scale=2; $w_p50_sum / $count" | bc)
                avg_w_p99=$(echo "scale=2; $w_p99_sum / $count" | bc)
                echo "$W,$name,$threads,$avg_tp,$avg_s_p50,$avg_s_p99,$avg_w_p50,$avg_w_p99" >> "$SUMMARY_CSV"
            fi
        done
    done
done

log ""
cp "$SUMMARY_CSV" "$WORK_DIR/results/latest/exp3.1.csv" 2>/dev/null || true
log "Updated results/latest/exp3.1.csv"
log "Exp 3.1 complete! Results: $SUMMARY_CSV"
