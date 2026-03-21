#!/bin/bash

# Quick re-run of Phase 76 curpht code to verify latency scaling behavior
# Only curpht, t=1,8,32,64

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

BASE_DIR="results/eval-5r-phase76-rerun-$(date +%Y%m%d)"
EXP_DIR="$BASE_DIR/exp3.1"
THREAD_COUNTS=(1 8 32 64)
MAX_RETRIES=2

CONFIG="/tmp/eval-phase76-rerun-$$.conf"
cp benchmark-5r.conf "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

# Only curpht
declare -a PROTOCOLS=(
    "curpht:curpht:50:5:5"
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
    for host in 130.245.173.101 130.245.173.103 130.245.173.104; do
        ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no "$host" "pkill -9 -x swiftpaxos-dist" 2>/dev/null || true
    done
    sleep 3
}

run_benchmark() {
    local out_dir="$1" threads="$2"
    mkdir -p "$out_dir"
    timeout 300 ./run-multi-client.sh -d -c "$CONFIG" -t "$threads" -o "$out_dir" \
        2>&1 | tee "$out_dir/run-output.txt"
    return ${PIPESTATUS[0]}
}

summarize() {
    local out_dir="$1"
    if [ -f "$out_dir/latencies.json" ]; then
        python3 scripts/summarize-latencies.py "$out_dir" > "$out_dir/summary.txt" 2>/dev/null
    fi
}

# Deploy binary
log "Building and deploying..."
go build -o swiftpaxos . || { log "Build failed"; exit 1; }
for host in 130.245.173.101 130.245.173.103 130.245.173.104; do
    scp -o StrictHostKeyChecking=no swiftpaxos "$host:~/swiftpaxos-dist" &
done
wait
log "Deploy complete"

for proto_spec in "${PROTOCOLS[@]}"; do
    IFS=: read -r name protocol weak_ratio writes weak_writes <<< "$proto_spec"
    apply_config "$CONFIG" "$protocol" "$weak_ratio" "$writes" "$weak_writes"

    for threads in "${THREAD_COUNTS[@]}"; do
        out_dir="$EXP_DIR/$name/t$threads"
        if [ -f "$out_dir/summary.txt" ]; then
            log "SKIP $name t=$threads (already done)"
            continue
        fi

        log "RUN $name t=$threads"
        ensure_clean

        success=false
        for attempt in $(seq 1 $MAX_RETRIES); do
            if run_benchmark "$out_dir" "$threads"; then
                summarize "$out_dir"
                if [ -f "$out_dir/summary.txt" ]; then
                    success=true
                    log "OK $name t=$threads"
                    break
                fi
            fi
            log "RETRY $name t=$threads (attempt $((attempt+1))/$MAX_RETRIES)"
            ensure_clean
            sleep 5
        done

        if ! $success; then
            log "FAIL $name t=$threads after $MAX_RETRIES attempts"
        fi
    done
done

log "Phase 76 re-run complete. Results in $BASE_DIR"
