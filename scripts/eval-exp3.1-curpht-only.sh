#!/bin/bash
# Exp 3.1 — curpht only, 3 reps, both write groups

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-exp3.1-phase125-$DATE}"
EXP_DIR="$BASE_DIR/exp3.1"
THREAD_COUNTS=(1 2 4 8 16 32 64 96)
WRITE_GROUPS=(5 50)
REPS=3
MAX_RETRIES=2

ALL_HOSTS=(34.236.191.149 18.221.173.128 16.147.240.15 108.130.8.61 35.183.203.84)

CONFIG="/tmp/eval-exp3.1-curpht-$$.conf"
cp configs/exp3.1-base.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

log() { echo "[$(date '+%H:%M:%S')] $*"; }

apply_config() {
    local conf="$1" protocol="$2" weak_ratio="$3" writes="$4" weak_writes="$5"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$conf"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $weak_ratio/" "$conf"
    sed -i -E "s/^writes:.*$/writes:      $writes/" "$conf"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  $weak_writes/" "$conf"
}

ensure_clean() {
    for host in "${ALL_HOSTS[@]}"; do
        ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "${SSH_USER:-$(whoami)}@$host" \
            "pkill -9 -x swiftpaxos-dist" 2>/dev/null || true
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

log "Exp 3.1 Phase 125: curpht only, 3 reps"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Write groups: ${WRITE_GROUPS[*]}"
log "Output: $EXP_DIR"
echo ""

ensure_clean

total_runs=$(( ${#THREAD_COUNTS[@]} * ${#WRITE_GROUPS[@]} * REPS ))
run_idx=0

for W in "${WRITE_GROUPS[@]}"; do
    log "====== Write Group: ${W}% ======"
    for threads in "${THREAD_COUNTS[@]}"; do
        for rep in $(seq 1 $REPS); do
            run_idx=$((run_idx + 1))
            out_dir="$EXP_DIR/w${W}/curpht/t${threads}/run${rep}"

            if [[ -f "$out_dir/summary.txt" ]]; then
                log "  [$run_idx/$total_runs] SKIP (exists): w=$W curpht t=$threads rep=$rep"
                continue
            fi

            log "  [$run_idx/$total_runs] w=$W curpht t=$threads rep=$rep"
            apply_config "$CONFIG" "curpht" "50" "$W" "$W"

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
                log "    -> ${tp} ops/sec"
            else
                log "    -> FAILED"
            fi
        done
    done
done

log "Done! Results in $EXP_DIR"
