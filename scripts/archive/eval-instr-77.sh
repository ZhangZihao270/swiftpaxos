#!/bin/bash

# Phase 77.1d: Run instrumented CURP-HT and CURP-HO at t=8 and t=128
#
# Collects: fast/slow path counters (client logs), SyncReply timing (replica logs),
# message drop counts (replica logs).
#
# Usage: bash scripts/eval-instr-77.sh [output-dir]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-instr-77-$DATE}"
THREAD_COUNTS=(8 128)
MAX_RETRIES=2

CONFIG="/tmp/eval-instr-77-$$.conf"
cp benchmark-5r.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

declare -a PROTOCOLS=(
    "curpht:curpht:50:5:5"
    "curpho:curpho:50:5:5"
)

mkdir -p "$BASE_DIR"

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

log "Phase 77.1d: Instrumented CURP-HT vs CURP-HO"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Output: $BASE_DIR"
echo ""

# Build
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1

ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#THREAD_COUNTS[@]} ))
run_idx=0

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio writes weak_writes <<< "$proto_spec"
    log "=== Protocol: $name ==="

    for threads in "${THREAD_COUNTS[@]}"; do
        run_idx=$((run_idx + 1))
        out_dir="$BASE_DIR/$name/t${threads}"
        log "  [$run_idx/$total_runs] threads=$threads -> $out_dir"

        apply_config "$CONFIG" "$protocol" "$weak_ratio" "$writes" "$weak_writes"

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
    echo ""
done

# Extract instrumentation data
log "Extracting instrumentation data..."

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio writes weak_writes <<< "$proto_spec"
    for threads in "${THREAD_COUNTS[@]}"; do
        out_dir="$BASE_DIR/$name/t${threads}"
        log "--- $name t=$threads ---"

        # Fast/slow path from client logs
        if ls "$out_dir"/client*.log >/dev/null 2>&1; then
            fast=0; slow=0
            for clog in "$out_dir"/client*.log; do
                last=$(grep "Fast/Slow Paths:" "$clog" 2>/dev/null | tail -1)
                if [[ -n "$last" ]]; then
                    f=$(echo "$last" | grep -oP 'Fast/Slow Paths: \K[0-9]+')
                    s=$(echo "$last" | grep -oP '/ \K[0-9]+')
                    fast=$((fast + f))
                    slow=$((slow + s))
                fi
            done
            total=$((fast + slow))
            if [[ $total -gt 0 ]]; then
                pct=$(echo "scale=1; $fast * 100 / $total" | bc)
                log "  Fast path: $fast ($pct%), Slow path: $slow"
            else
                log "  No fast/slow path data"
            fi
        fi

        # SyncReply timing from replica logs (only CURP-HT)
        if [[ "$name" == "curpht" ]]; then
            for rlog in "$out_dir"/replica*.log; do
                cnt=$(grep -c "SYNCREPLY-HT" "$rlog" 2>/dev/null || true)
                cnt=${cnt:-0}
                if [[ "$cnt" =~ ^[0-9]+$ ]] && [[ "$cnt" -gt 0 ]]; then
                    avg=$(grep "SYNCREPLY-HT" "$rlog" | grep -oP 'delay=\K[0-9.]+' | awk '{s+=$1; n++} END {if(n>0) printf "%.2f", s/n; else print "N/A"}')
                    max=$(grep "SYNCREPLY-HT" "$rlog" | grep -oP 'delay=\K[0-9.]+' | sort -rn | head -1)
                    log "  $(basename "$rlog"): SyncReply count=$cnt avg=${avg}ms max=${max}ms"
                fi
            done
        fi

        # Message drops from replica logs
        for rlog in "$out_dir"/replica*.log; do
            drops=$(grep -c "MSGDROP" "$rlog" 2>/dev/null || true)
            drops=${drops:-0}
            warn=$(grep -c "per-client channel full" "$rlog" 2>/dev/null || true)
            warn=${warn:-0}
            if [[ "$drops" =~ ^[0-9]+$ ]] && [[ "$warn" =~ ^[0-9]+$ ]] && [[ "$drops" -gt 0 || "$warn" -gt 0 ]]; then
                last_drop=$(grep "MSGDROP" "$rlog" 2>/dev/null | tail -1 | grep -oP 'total=\K[0-9]+' || echo "0")
                log "  $(basename "$rlog"): drops=$last_drop (warnings=$warn)"
            fi
        done
    done
done

log ""
log "Phase 77.1d complete! Results: $BASE_DIR"
