#!/bin/bash

# Phase 75.3: Instrumented experiment — CURP-HT vs CURP-HO at t=8 and t=64
#
# Runs both protocols at low (t=8) and high (t=64) load to capture:
# - Leader-side event loop stats: [INSTR-HT] / [INSTR-HO]
# - Client-side strong op timing: [CINSTR-HT] / [CINSTR-HO]
# - Slow path pipeline timing
# - MSync retry counts
#
# Usage: bash scripts/eval-instr-75.sh [output-dir]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-instr-75-$DATE}"
THREAD_COUNTS=(8 64)
MAX_RETRIES=2

CONFIG="/tmp/eval-instr-75-$$.conf"
cp benchmark-5r.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Protocol configs: name, protocol-value, weakRatio, writes, weakWrites
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

log "Phase 75.3: Instrumented CURP-HT vs CURP-HO"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Output: $BASE_DIR"
echo ""

# Build
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1

# Initial cleanup
ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#THREAD_COUNTS[@]} ))
run_idx=0

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio writes weak_writes <<< "$proto_spec"

    log "=== Protocol: $name (protocol=$protocol, weakRatio=$weak_ratio) ==="

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

# Extract instrumentation lines from logs
log "Extracting instrumentation data..."
for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio writes weak_writes <<< "$proto_spec"
    for threads in "${THREAD_COUNTS[@]}"; do
        out_dir="$BASE_DIR/$name/t${threads}"
        if [[ -d "$out_dir" ]]; then
            # Extract replica instrumentation (leader = replica0)
            grep -h "\[INSTR-" "$out_dir/replica0.log" 2>/dev/null > "$out_dir/instr-leader.txt" || true
            # Extract client instrumentation from all client logs
            grep -h "\[CINSTR-" "$out_dir"/client*.log 2>/dev/null > "$out_dir/instr-clients.txt" || true
            log "  $name/t$threads: $(wc -l < "$out_dir/instr-leader.txt" 2>/dev/null || echo 0) leader lines, $(wc -l < "$out_dir/instr-clients.txt" 2>/dev/null || echo 0) client lines"
        fi
    done
done

log ""
log "Phase 75.3 complete! Results: $BASE_DIR"
log "Instrumentation logs extracted to instr-leader.txt and instr-clients.txt per run"
