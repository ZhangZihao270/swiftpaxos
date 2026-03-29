#!/bin/bash

# Exp 2.1 (Final): EPaxos-HO vs EPaxos — Throughput vs Latency
#
# 5 replicas on 5 separate machines, 3 clients co-located with replicas 0-2.
# Sweeps thread counts across 2 write groups (5% and 50%), 1 rep each.
#
# Protocols:
#   epaxosho    — protocol: epaxosho, weakRatio: 50
#   epaxos      — protocol: epaxos, weakRatio: 0 (all strong = vanilla EPaxos)
#
# Usage: bash scripts/eval-exp2.1-final.sh [output-dir]
# Output: <output-dir>/exp2.1/

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-exp2.1-$DATE}"
EXP_DIR="$BASE_DIR/exp2.1"
THREAD_COUNTS=(1 2 4 8 16 32 64 96)
WRITE_GROUPS=(5 50)
REPS=1
MAX_RETRIES=2

ALL_HOSTS=(34.236.191.149 18.221.173.128 16.147.240.15 108.130.8.61 35.183.203.84)

CONFIG="/tmp/eval-exp2.1-final-$$.conf"
cp configs/exp2.1-base.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocol configs: name, protocol-value, weakRatio
declare -a PROTOCOLS=(
    "epaxosho:epaxosho:50"
    "epaxos:epaxosho:0"
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

cp "$SUMMARY_CSV" "$WORK_DIR/results/latest/exp2.1.csv" 2>/dev/null || true
log "Updated results/latest/exp2.1.csv"
log "Exp 2.1 (Final): EPaxos-HO vs EPaxos — Throughput vs Latency"
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

        # For epaxos baseline (weakRatio=0), weakWrites doesn't matter
        local_weak_writes="$W"
        if [[ "$weak_ratio" == "0" ]]; then
            local_weak_writes="0"
        fi

        log "=== Protocol: $name (protocol=$protocol, weakRatio=$weak_ratio, writes=$W) ==="

        for threads in "${THREAD_COUNTS[@]}"; do
            for rep in $(seq 1 $REPS); do
                run_idx=$((run_idx + 1))
                out_dir="$EXP_DIR/w${W}/$name/t${threads}/run${rep}"

                log "  [$run_idx/$total_runs] w=$W $name t=$threads rep=$rep -> $out_dir"

                apply_config "$CONFIG" "$protocol" "$weak_ratio" "$W" "$local_weak_writes"

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
SUMMARY_CSV="$BASE_DIR/summary-exp2.1.csv"
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
                    s_p50=$(grep "Avg median" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
                    s_p99=$(grep "Avg p99" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
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
cp "$SUMMARY_CSV" "$WORK_DIR/results/latest/exp2.1.csv" 2>/dev/null || true
log "Updated results/latest/exp2.1.csv"
log "Exp 2.1 complete! Results: $SUMMARY_CSV"
