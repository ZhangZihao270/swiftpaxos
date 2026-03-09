#!/bin/bash

# Exp Conflict (Write-Heavy): Conflict Sensitivity at 50% Writes
#
# Same as eval-conflict-dist.sh but with writes=50 to stress conflicts.
# EPaxos should show larger degradation under high contention with many writes.
#
# Usage: bash scripts/eval-conflict-w50-dist.sh [output-dir]
# Output: results/eval-dist-YYYYMMDD/conflict-w50/

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-dist-$DATE}"
EXP_DIR="$BASE_DIR/conflict-w50"
THREADS=32
REQS=10000
MAX_RETRIES=2

CONFLICT_LEVELS=(
    "uniform 0 1000000"
    "mild 0.5 1000000"
    "moderate 0.99 1000000"
    "high 1.5 1000000"
    "hotspot 0.99 1000"
)

PROTOCOLS=("epaxos" "curp" "raft")

CONFIG="/tmp/eval-conflict-w50-dist-$$.conf"
cp multi-client.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

apply_config() {
    local conf="$1" protocol="$2" zipf_skew="$3" key_space="$4" thrifty="$5"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$conf"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   0/" "$conf"
    sed -i -E "s/^writes:.*$/writes:      50/" "$conf"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$conf"
    sed -i -E "s/^thrifty:.*$/thrifty:    $thrifty/" "$conf"
    sed -i -E "s/^reqs:.*$/reqs:        $REQS/" "$conf"
    sed -i -E "s/^zipfSkew:.*$/zipfSkew:    $zipf_skew/" "$conf"
    sed -i -E "s/^keySpace:.*$/keySpace:    $key_space/" "$conf"
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
    timeout 600 ./run-multi-client.sh -d -c "$CONFIG" -t "$threads" -o "$out_dir" \
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

log "Conflict Rate Sensitivity — Write-Heavy (50% writes)"
log "Thread count: $THREADS (fixed)"
log "Conflict levels: ${#CONFLICT_LEVELS[@]}"
log "Protocols: ${PROTOCOLS[*]}"
log "Output: $EXP_DIR"
echo ""

log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1

ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#CONFLICT_LEVELS[@]} ))
run_idx=0

for protocol in "${PROTOCOLS[@]}"; do
    thrifty="false"
    if [[ "$protocol" == "epaxos" ]]; then
        thrifty="true"
    fi

    log "=== Protocol: $protocol (thrifty=$thrifty) ==="

    for level_cfg in "${CONFLICT_LEVELS[@]}"; do
        read -r label zipf_skew key_space <<< "$level_cfg"
        run_idx=$((run_idx + 1))
        out_dir="$EXP_DIR/$protocol/$label"

        log "  [$run_idx/$total_runs] $protocol/$label (zipfSkew=$zipf_skew, keySpace=$key_space)"

        apply_config "$CONFIG" "$protocol" "$zipf_skew" "$key_space" "$thrifty"

        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir" "$THREADS"; then
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

# Collect results
log "Collecting results..."
SUMMARY_CSV="$BASE_DIR/summary-conflict-w50.csv"

echo "protocol,conflict_level,zipf_skew,key_space,throughput,s_avg,s_p50,s_p99" > "$SUMMARY_CSV"

for protocol in "${PROTOCOLS[@]}"; do
    for level_cfg in "${CONFLICT_LEVELS[@]}"; do
        read -r label zipf_skew key_space <<< "$level_cfg"
        summary="$EXP_DIR/$protocol/$label/summary.txt"
        if [[ -f "$summary" ]]; then
            tp=$(python3 -c "
import re, sys
with open('$summary') as f: text = f.read()
m = re.search(r'Aggregate throughput:\s*([\d.]+)', text)
tp = m.group(1) if m else 'N/A'
s = re.search(r'Strong Operations.*?Avg: ([\d.]+)ms.*?Avg median: ([\d.]+)ms.*?Max P99: ([\d.]+)ms', text, re.DOTALL)
s_avg = s.group(1) if s else 'N/A'
s_p50 = s.group(2) if s else 'N/A'
s_p99 = s.group(3) if s else 'N/A'
print(f'{tp},{s_avg},{s_p50},{s_p99}')
")
            echo "$protocol,$label,$zipf_skew,$key_space,$tp" >> "$SUMMARY_CSV"
        else
            echo "$protocol,$label,$zipf_skew,$key_space,N/A,N/A,N/A,N/A" >> "$SUMMARY_CSV"
        fi
    done
done

log ""
log "Write-heavy conflict experiment complete!"
log "Results: $SUMMARY_CSV"
echo ""
column -t -s, "$SUMMARY_CSV"
