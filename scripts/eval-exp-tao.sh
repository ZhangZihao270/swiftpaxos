#!/bin/bash

# Exp TAO: TAO-like Benchmark — Throughput vs Latency
#
# 5 replicas on 5 separate machines, 3 clients co-located with replicas 0-2.
# TAO workload: 1% writes (all linear), 95% weak ratio, 44% of weak reads
# are SCAN ops with Zipf-distributed scan count [1, 1000].
#
# Protocols (10 total):
#   Baselines (weakRatio=0, all strong):
#     raft-baseline    — raftht:0
#     epaxos-baseline  — epaxosho:0
#     curp-baseline    — curpht:0
#   Hybrid (weakRatio=95):
#     raftht, epaxosho, curpht, curpho, mongotunable, pileus, pileusht
#
# Usage: bash scripts/eval-exp-tao.sh [output-dir]
# Output: <output-dir>/exp-tao/

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-exp-tao-$DATE}"
EXP_DIR="$BASE_DIR/exp-tao"
THREAD_COUNTS=(1 2 4 8 16 32 64 96)
REPS=3
MAX_RETRIES=2

ALL_HOSTS=(34.236.191.149 18.221.173.128 16.147.240.15 108.130.8.61 35.183.203.84)

CONFIG="/tmp/eval-exp-tao-$$.conf"
cp configs/exp-tao.conf "$CONFIG"
# Override reqs to 3000
sed -i -E "s/^reqs:.*$/reqs:        3000/" "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocol configs: name, protocol-value, weakRatio
declare -a PROTOCOLS=(
    "raft-baseline:raftht:0"
    "raftht:raftht:95"
    "epaxos-baseline:epaxosho:0"
    "epaxosho:epaxosho:95"
    "curp-baseline:curpht:0"
    "curpht:curpht:95"
    "curpho:curpho:95"
    "mongotunable:mongotunable:95"
    "pileus:pileus:95"
    "pileusht:pileusht:95"
)

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

apply_config() {
    local conf="$1" protocol="$2" weak_ratio="$3"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$conf"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $weak_ratio/" "$conf"
    # For baseline (weakRatio=0), disable scan since all ops are strong
    if [[ "$weak_ratio" == "0" ]]; then
        sed -i -E "s/^scanRatio:.*$/scanRatio:   0/" "$conf"
    else
        sed -i -E "s/^scanRatio:.*$/scanRatio:   44/" "$conf"
    fi
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

log "Exp TAO: TAO-like Benchmark — Throughput vs Latency"
log "Layout: 5 replicas on 5 machines, 3 clients"
log "Workload: 1% writes (20% of strong), 95% weak, scanRatio=44, scanCount=1000, zipf=0.8"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Repetitions: $REPS, reqs: 3000"
log "Protocols: ${#PROTOCOLS[@]}"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos..."
go build -o swiftpaxos-dist . 2>&1

# Initial cleanup
ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#THREAD_COUNTS[@]} * REPS ))
run_idx=0

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio <<< "$proto_spec"

    log "=== Protocol: $name (protocol=$protocol, weakRatio=$weak_ratio) ==="

    for threads in "${THREAD_COUNTS[@]}"; do
        for rep in $(seq 1 $REPS); do
            run_idx=$((run_idx + 1))
            out_dir="$EXP_DIR/$name/t${threads}/run${rep}"

            if [[ -f "$out_dir/summary.txt" ]]; then
                log "  [$run_idx/$total_runs] SKIP (exists): $name t=$threads rep=$rep"
                continue
            fi

            log "  [$run_idx/$total_runs] $name t=$threads rep=$rep -> $out_dir"

            apply_config "$CONFIG" "$protocol" "$weak_ratio"

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

# Generate summary CSV with averaged results
log "Generating summary CSV..."
SUMMARY_CSV="$BASE_DIR/summary-exp-tao.csv"
echo "protocol,threads,avg_throughput,avg_s_avg,avg_s_p50,avg_s_p99,avg_w_avg,avg_w_p50,avg_w_p99,avg_lat" > "$SUMMARY_CSV"

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio <<< "$proto_spec"
    for threads in "${THREAD_COUNTS[@]}"; do
        tp_sum=0; s_avg_sum=0; s_p50_sum=0; s_p99_sum=0
        w_avg_sum=0; w_p50_sum=0; w_p99_sum=0; count=0
        for rep in $(seq 1 $REPS); do
            summary="$EXP_DIR/$name/t${threads}/run${rep}/summary.txt"
            if [[ -f "$summary" ]]; then
                tp=$(grep "Aggregate throughput" "$summary" | awk '{print $3}')
                # Strong
                s_avg=$(sed -n '/Strong/,/Weak\|Per-Client/{/Avg:/p}' "$summary" | head -1 | grep -oP 'Avg:\s*\K[\d.]+')
                s_p50=$(grep "Avg median" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
                s_p99=$(grep "Max P99" "$summary" | head -1 | grep -oP '[\d.]+' | head -1)
                # Strong %
                s_pct=$(grep "Strong Operations" "$summary" | grep -oP '\(([\d.]+)%\)' | grep -oP '[\d.]+')
                # Weak
                w_avg=$(sed -n '/Weak/,/Per-Client/{/Avg:/p}' "$summary" | head -1 | grep -oP 'Avg:\s*\K[\d.]+')
                w_p50=$(grep "Avg median" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
                w_p99=$(grep "Max P99" "$summary" | tail -1 | grep -oP '[\d.]+' | head -1)
                if [[ -n "$tp" && "$tp" != "0.00" ]]; then
                    tp_sum=$(echo "$tp_sum + $tp" | bc)
                    s_avg_sum=$(echo "$s_avg_sum + ${s_avg:-0}" | bc)
                    s_p50_sum=$(echo "$s_p50_sum + ${s_p50:-0}" | bc)
                    s_p99_sum=$(echo "$s_p99_sum + ${s_p99:-0}" | bc)
                    w_avg_sum=$(echo "$w_avg_sum + ${w_avg:-0}" | bc)
                    w_p50_sum=$(echo "$w_p50_sum + ${w_p50:-0}" | bc)
                    w_p99_sum=$(echo "$w_p99_sum + ${w_p99:-0}" | bc)
                    count=$((count + 1))
                fi
            fi
        done
        if [[ $count -gt 0 ]]; then
            avg_tp=$(echo "scale=2; $tp_sum / $count" | bc)
            avg_s_avg=$(echo "scale=2; $s_avg_sum / $count" | bc)
            avg_s_p50=$(echo "scale=2; $s_p50_sum / $count" | bc)
            avg_s_p99=$(echo "scale=2; $s_p99_sum / $count" | bc)
            avg_w_avg=$(echo "scale=2; $w_avg_sum / $count" | bc)
            avg_w_p50=$(echo "scale=2; $w_p50_sum / $count" | bc)
            avg_w_p99=$(echo "scale=2; $w_p99_sum / $count" | bc)
            # Weighted avg latency (use s_pct from last valid run, or 100% for baselines)
            if [[ "$weak_ratio" == "0" ]]; then
                avg_lat="$avg_s_avg"
            else
                avg_lat=$(echo "scale=2; $avg_s_avg * 0.05 + $avg_w_avg * 0.95" | bc)
            fi
            echo "$name,$threads,$avg_tp,$avg_s_avg,$avg_s_p50,$avg_s_p99,$avg_w_avg,$avg_w_p50,$avg_w_p99,$avg_lat" >> "$SUMMARY_CSV"
        fi
    done
done

log ""
mkdir -p "$WORK_DIR/results/latest"
cp "$SUMMARY_CSV" "$WORK_DIR/results/latest/exp-tao.csv" 2>/dev/null || true
log "Updated results/latest/exp-tao.csv"
log "Exp TAO complete! Results: $SUMMARY_CSV"
