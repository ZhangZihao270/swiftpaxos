#!/bin/bash

# Exp 2.2 (Final): EPaxos-HO vs EPaxos — Conflict Rate Sweep
#
# 5 replicas on 5 separate machines, 3 clients co-located with replicas 0-2.
# Sweeps zipfSkew at fixed thread count (t=16), 1 rep each.
# Why t=16: at t=32, EPaxos is already slow-path dominated (~105ms p50),
# masking the effect of key conflicts. t=16 keeps EPaxos in fast-path range.
# writes=50, weakWrites=50.
#
# Protocols:
#   epaxosho    — protocol: epaxosho, weakRatio: 50
#   epaxos      — protocol: epaxos, weakRatio: 0
#
# Usage: bash scripts/eval-exp2.2-final.sh [output-dir]
# Output: <output-dir>/exp2.2/

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-exp2.2-$DATE}"
EXP_DIR="$BASE_DIR/exp2.2"
FIXED_THREADS=16
ZIPF_SKEWS=(0 0.25 0.5 0.75 0.99 1.2 1.5 2.0)
REPS=1
MAX_RETRIES=2

ALL_HOSTS=(130.245.173.101 130.245.173.103 130.245.173.104 130.245.173.125 130.245.173.126)

CONFIG="/tmp/eval-exp2.2-final-$$.conf"
cp configs/exp2.2-base.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocol configs: name, protocol-value, weakRatio
declare -a PROTOCOLS=(
    "epaxosho:epaxosho:50"
    "epaxos:epaxos:0"
)

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

apply_config() {
    local conf="$1" protocol="$2" weak_ratio="$3" zipf_skew="$4"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$conf"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $weak_ratio/" "$conf"
    sed -i -E "s/^zipfSkew:.*$/zipfSkew:    $zipf_skew/" "$conf"
    # For epaxos baseline (weakRatio=0), set weakWrites to 0
    if [[ "$weak_ratio" == "0" ]]; then
        sed -i -E "s/^weakWrites:.*$/weakWrites:  0/" "$conf"
    else
        sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$conf"
    fi
}

ensure_clean() {
    for host in "${ALL_HOSTS[@]}"; do
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

cp "$SUMMARY_CSV" "$WORK_DIR/results/latest/exp2.2.csv" 2>/dev/null || true
log "Updated results/latest/exp2.2.csv"
log "Exp 2.2 (Final): EPaxos-HO vs EPaxos — Conflict Rate Sweep"
log "Layout: 5 replicas on 5 machines, 3 clients"
log "Protocols: epaxosho, epaxos"
log "Zipf skews: ${ZIPF_SKEWS[*]}"
log "Fixed threads: $FIXED_THREADS"
log "Repetitions: $REPS"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos-dist . 2>&1

# Initial cleanup
ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#ZIPF_SKEWS[@]} * REPS ))
run_idx=0

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio <<< "$proto_spec"
    log "=== Protocol: $name (protocol=$protocol, weakRatio=$weak_ratio) ==="

    for skew in "${ZIPF_SKEWS[@]}"; do
        for rep in $(seq 1 $REPS); do
            run_idx=$((run_idx + 1))
            out_dir="$EXP_DIR/$name/z${skew}/run${rep}"

            log "  [$run_idx/$total_runs] $name zipfSkew=$skew rep=$rep -> $out_dir"

            apply_config "$CONFIG" "$protocol" "$weak_ratio" "$skew"

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
                s_med=$(grep "Avg median" "$out_dir/summary.txt" | head -1 | grep -oP '[\d.]+' | head -1)
                log "  Result: throughput=${tp} ops/sec, median=${s_med}ms"
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

# Generate summary CSV with averaged results
log "Generating summary CSV..."
SUMMARY_CSV="$BASE_DIR/summary-exp2.2.csv"
echo "protocol,zipf_skew,avg_throughput,avg_s_tput,avg_s_p50,avg_s_p99,avg_w_tput,avg_w_p50,avg_w_p99" > "$SUMMARY_CSV"

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio <<< "$proto_spec"
    for skew in "${ZIPF_SKEWS[@]}"; do
        tp_sum=0; s_tput_sum=0; s_p50_sum=0; s_p99_sum=0
        w_tput_sum=0; w_p50_sum=0; w_p99_sum=0; count=0
        for rep in $(seq 1 $REPS); do
            summary="$EXP_DIR/$name/z${skew}/run${rep}/summary.txt"
            if [[ -f "$summary" ]]; then
                tp=$(grep "Aggregate throughput" "$summary" | awk '{print $3}')
                s_tput=$(grep -A1 "Strong" "$summary" | grep "throughput" | grep -oP '[\d.]+' | head -1)
                s_p50=$(grep "Avg median" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
                s_p99=$(grep "Avg p99" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
                w_tput=$(grep -A1 "Weak" "$summary" | grep "throughput" | grep -oP '[\d.]+' | head -1)
                w_p50=$(grep "Avg median" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
                w_p99=$(grep "Avg p99" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
                if [[ -n "$tp" && "$tp" != "0.00" ]]; then
                    tp_sum=$(echo "$tp_sum + $tp" | bc)
                    s_tput_sum=$(echo "$s_tput_sum + ${s_tput:-0}" | bc)
                    s_p50_sum=$(echo "$s_p50_sum + ${s_p50:-0}" | bc)
                    s_p99_sum=$(echo "$s_p99_sum + ${s_p99:-0}" | bc)
                    w_tput_sum=$(echo "$w_tput_sum + ${w_tput:-0}" | bc)
                    w_p50_sum=$(echo "$w_p50_sum + ${w_p50:-0}" | bc)
                    w_p99_sum=$(echo "$w_p99_sum + ${w_p99:-0}" | bc)
                    count=$((count + 1))
                fi
            fi
        done
        if [[ $count -gt 0 ]]; then
            avg_tp=$(echo "scale=2; $tp_sum / $count" | bc)
            avg_s_tput=$(echo "scale=2; $s_tput_sum / $count" | bc)
            avg_s_p50=$(echo "scale=2; $s_p50_sum / $count" | bc)
            avg_s_p99=$(echo "scale=2; $s_p99_sum / $count" | bc)
            avg_w_tput=$(echo "scale=2; $w_tput_sum / $count" | bc)
            avg_w_p50=$(echo "scale=2; $w_p50_sum / $count" | bc)
            avg_w_p99=$(echo "scale=2; $w_p99_sum / $count" | bc)
            echo "$name,$skew,$avg_tp,$avg_s_tput,$avg_s_p50,$avg_s_p99,$avg_w_tput,$avg_w_p50,$avg_w_p99" >> "$SUMMARY_CSV"
        fi
    done
done

log ""
cp "$SUMMARY_CSV" "$WORK_DIR/results/latest/exp2.2.csv" 2>/dev/null || true
log "Updated results/latest/exp2.2.csv"
log "Exp 2.2 complete! Results: $SUMMARY_CSV"
