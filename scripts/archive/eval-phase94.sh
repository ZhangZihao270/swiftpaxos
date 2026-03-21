#!/bin/bash

# Phase 94: Re-run Exp 3.1 — Validate CURP-HO s_p50 anomaly
#
# Same setup as Phase 93, but without t=128 (only 8 thread counts).
# Protocols: curp-baseline, curpht, curpho
# Thread counts: 1, 2, 4, 8, 16, 32, 64, 96
# 5 replicas on 5 machines, 3 clients on .101/.103/.104
# reqs=3000, networkDelay=25, commandSize=100
#
# Usage: bash scripts/eval-phase94.sh [output-dir] [protocol-filter]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d)
BASE_DIR="${1:-results/eval-5r5m3c-phase94-$DATE}"
PROTO_FILTER="${2:-all}"
EXP_DIR="$BASE_DIR/exp3.1"
THREAD_COUNTS=(1 2 4 8 16 32 64 96)
MAX_RETRIES=3

ALL_HOSTS=(130.245.173.101 130.245.173.103 130.245.173.104 130.245.173.125 130.245.173.126)

# Protocol specs: name:protocol:weakRatio:writes:weakWrites
declare -a ALL_PROTOCOLS=(
    "curp-baseline:curp:0:5:5"
    "curpht:curpht:50:5:5"
    "curpho:curpho:50:5:5"
)

# Filter protocols based on user selection
declare -a PROTOCOLS=()
for spec in "${ALL_PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol wr w ww <<< "$spec"
    if [[ "$PROTO_FILTER" == "all" || "$PROTO_FILTER" == "$name" || "$PROTO_FILTER" == "$protocol" ]]; then
        PROTOCOLS+=("$spec")
    fi
done

if [[ ${#PROTOCOLS[@]} -eq 0 ]]; then
    echo "ERROR: No protocols matched filter '$PROTO_FILTER'"
    echo "Valid: all, curp, curpht, curpho, curp-baseline"
    exit 1
fi

CONFIG_TEMPLATE="benchmark-5r-5m-3c.conf"
if [[ ! -f "$CONFIG_TEMPLATE" ]]; then
    echo "ERROR: Config template $CONFIG_TEMPLATE not found"
    exit 1
fi

mkdir -p "$EXP_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

make_config() {
    local out="$1" protocol="$2" weak_ratio="$3" writes="$4" weak_writes="$5"
    cp "$CONFIG_TEMPLATE" "$out"
    sed -i -E "s/^protocol:.*$/protocol: $protocol/" "$out"
    sed -i -E "s/^reqs:.*$/reqs:        3000/" "$out"
    sed -i -E "s/^weakRatio:.*$/weakRatio:   $weak_ratio/" "$out"
    sed -i -E "s/^writes:.*$/writes:      $writes/" "$out"
    sed -i -E "s/^weakWrites:.*$/weakWrites:  $weak_writes/" "$out"
}

ensure_clean() {
    for host in "${ALL_HOSTS[@]}"; do
        ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "$host" "pkill -9 -x swiftpaxos-dist" 2>/dev/null || true
    done
    sleep 3
}

run_benchmark() {
    local out_dir="$1" threads="$2" config="$3"
    mkdir -p "$out_dir"
    timeout 300 ./run-multi-client.sh -d -c "$config" -t "$threads" -o "$out_dir" \
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

log "Phase 94: Re-run Exp 3.1 — Validate CURP-HO s_p50 (5r/5m/3c)"
log "Protocols: $(printf '%s ' "${PROTOCOLS[@]}" | sed 's/:[^ ]*//g')"
log "Thread counts: ${THREAD_COUNTS[*]}"
log "Output: $EXP_DIR"
echo ""

# Build
log "Building swiftpaxos-dist..."
go build -o swiftpaxos-dist . 2>&1
if [[ $? -ne 0 ]]; then
    log "ERROR: Build failed"
    exit 1
fi

# Initial cleanup
ensure_clean

total_runs=$(( ${#PROTOCOLS[@]} * ${#THREAD_COUNTS[@]} ))
run_idx=0

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=':' read -r name protocol weak_ratio writes weak_writes <<< "$proto_spec"

    # Create per-protocol config
    proto_config="/tmp/eval-phase94-${name}-$$.conf"
    make_config "$proto_config" "$protocol" "$weak_ratio" "$writes" "$weak_writes"

    log "=== Protocol: $name (protocol=$protocol, weakRatio=$weak_ratio) ==="

    for threads in "${THREAD_COUNTS[@]}"; do
        run_idx=$((run_idx + 1))
        out_dir="$EXP_DIR/$name/t${threads}"

        # Skip if results already exist (allows resuming)
        if [[ -f "$out_dir/summary.txt" ]]; then
            tp=$(grep "Aggregate throughput" "$out_dir/summary.txt" | awk '{print $3}')
            if [[ "$tp" != "0.00" && -n "$tp" ]]; then
                log "  [$run_idx/$total_runs] threads=$threads -> SKIP (already done: ${tp} ops/sec)"
                continue
            fi
        fi

        log "  [$run_idx/$total_runs] threads=$threads -> $out_dir"

        # Run with retry
        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if [[ $attempt -gt 1 ]]; then
                log "  Retry $attempt/$MAX_RETRIES..."
                rm -rf "$out_dir"
                sleep 5
            fi
            if run_benchmark "$out_dir" "$threads" "$proto_config"; then
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
    echo ""

    # Cleanup per-protocol config
    rm -f "$proto_config"
done

# Collect results to CSV
log "Collecting results..."
bash scripts/collect-results.sh throughput "$EXP_DIR" "$BASE_DIR/summary-exp3.1.csv"

log ""
log "Phase 94 complete! Results: $BASE_DIR/summary-exp3.1.csv"
