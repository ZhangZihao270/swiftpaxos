#!/bin/bash

# Exp 2.3 Phase 112: EPaxos-HO Failure Recovery — Kill Non-Client Replica
#
# Same as exp2.3-epaxosho.sh but kills replica3 (130.245.173.125) which has
# NO co-located client. Tests that throughput impact is minimal when only
# a non-client replica is lost.
#
# Contrast with Phase 111: kill replica0 (co-located with client0) → 34% drop.
# Expected here: <10% drop since all 3 clients keep their local replicas.
#
# Usage: bash scripts/exp2.3-epaxosho-kill-r3.sh [output-dir] [kill-delay-s]

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d_%H%M%S)
BASE_DIR="${1:-results/exp2.3-epaxosho-killr3-$DATE}"
KILL_DELAY="${2:-60}"
STARTUP_DELAY=25
THREADS=16

ALL_HOSTS=(130.245.173.101 130.245.173.103 130.245.173.104 130.245.173.125 130.245.173.126)
KILL_HOST="130.245.173.125"  # replica3 — NO client co-located
KILL_REPLICA="replica3"
BINARY="swiftpaxos-dist"
SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=10"

CONFIG_TEMPLATE="benchmark-5r-5m-3c.conf"
if [[ ! -f "$CONFIG_TEMPLATE" ]]; then
    echo "ERROR: Config template $CONFIG_TEMPLATE not found"
    exit 1
fi

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

ensure_clean() {
    for host in "${ALL_HOSTS[@]}"; do
        ssh $SSH_OPTS "$host" "pkill -9 -x $BINARY" 2>/dev/null || true
    done
    sleep 3
}

# Create experiment config
CONFIG="/tmp/exp2.3-epaxosho-killr3-$$.conf"
cp "$CONFIG_TEMPLATE" "$CONFIG"
sed -i -E "s/^protocol:.*$/protocol: epaxosho/" "$CONFIG"
sed -i -E "s/^reqs:.*$/reqs:        100000/" "$CONFIG"
sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$CONFIG"
sed -i -E "s/^writes:.*$/writes:      50/" "$CONFIG"
sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$CONFIG"
sed -i -E "s/^fast:.*$/fast:       true/" "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

mkdir -p "$BASE_DIR"

log "Exp 2.3 Phase 112: EPaxos-HO Failure Recovery — Kill Non-Client Replica"
log "Config: epaxosho, t=$THREADS, reqs=100000, writes=50%, weakRatio=50%"
log "Kill delay: ${KILL_DELAY}s after clients start"
log "Kill target: $KILL_REPLICA on $KILL_HOST (NO co-located client)"
log "Output: $BASE_DIR"
echo ""

# Build
log "Building $BINARY..."
go build -o "$BINARY" . 2>&1
if [[ $? -ne 0 ]]; then
    log "ERROR: Build failed"
    exit 1
fi

# Initial cleanup
ensure_clean

# Schedule replica kill in background
TOTAL_KILL_DELAY=$((STARTUP_DELAY + KILL_DELAY + 20))  # +20 for sync/setup overhead
log "Scheduling $KILL_REPLICA kill in ${TOTAL_KILL_DELAY}s (startup=${STARTUP_DELAY} + overhead=20 + kill_delay=${KILL_DELAY})"

(
    sleep "$TOTAL_KILL_DELAY"
    echo ""
    echo "[$(date '+%H:%M:%S')] *** KILLING $KILL_REPLICA on $KILL_HOST ***"
    ssh $SSH_OPTS "$KILL_HOST" "pkill -9 -f '$BINARY -run server'" 2>/dev/null || true
    echo "[$(date '+%H:%M:%S')] *** $KILL_REPLICA killed ***"
) &
KILL_PID=$!

# Run the benchmark (blocks until all clients finish or timeout)
log "Starting benchmark via run-multi-client.sh..."
timeout 600 ./run-multi-client.sh -d -c "$CONFIG" -t "$THREADS" -o "$BASE_DIR" \
    --startup-delay "$STARTUP_DELAY" \
    > "$BASE_DIR/run-output.txt" 2>&1 || true

# Wait for kill process to complete (if still running)
kill "$KILL_PID" 2>/dev/null || true
wait "$KILL_PID" 2>/dev/null || true

# Ensure cleanup
ensure_clean

# Extract TPUT lines from all client logs
log "Extracting TPUT data from client logs..."
TPUT_FILE="$BASE_DIR/tput-all.csv"
echo "timestamp,client,ops" > "$TPUT_FILE"

for client_log in "$BASE_DIR"/client*.log; do
    if [[ -f "$client_log" ]]; then
        client_name=$(basename "$client_log" .log)
        grep "^.*TPUT " "$client_log" | while read -r line; do
            prefix=$(echo "$line" | awk '{print $3}')
            if [[ "$prefix" == "TPUT" ]]; then
                client_id=$(echo "$line" | awk '{print $4}')
                ts=$(echo "$line" | awk '{print $5}')
                ops=$(echo "$line" | awk '{print $6}')
                echo "$ts,$client_id,$ops" >> "$TPUT_FILE"
            fi
        done
    fi
done

# Sort by timestamp
if [[ -f "$TPUT_FILE" ]]; then
    header=$(head -1 "$TPUT_FILE")
    tail -n +2 "$TPUT_FILE" | sort -t, -k1,1n > "$TPUT_FILE.tmp"
    echo "$header" > "$TPUT_FILE"
    cat "$TPUT_FILE.tmp" >> "$TPUT_FILE"
    rm -f "$TPUT_FILE.tmp"
fi

# Create aggregated per-second file (sum across all clients)
AGG_FILE="$BASE_DIR/tput-aggregated.csv"
echo "timestamp,total_ops" > "$AGG_FILE"
if [[ -f "$TPUT_FILE" ]]; then
    tail -n +2 "$TPUT_FILE" | awk -F, '{
        sum[$1] += $3
    } END {
        for (ts in sum) print ts "," sum[ts]
    }' | sort -t, -k1,1n >> "$AGG_FILE"
fi

# Print summary
log ""
log "=== Results Summary ==="
if [[ -f "$BASE_DIR/summary.txt" ]]; then
    cat "$BASE_DIR/summary.txt"
fi
echo ""

if [[ -f "$AGG_FILE" ]]; then
    total_lines=$(tail -n +2 "$AGG_FILE" | wc -l)
    log "TPUT data: $total_lines seconds recorded"
    log "TPUT file: $TPUT_FILE"
    log "Aggregated: $AGG_FILE"

    if [[ $total_lines -gt 0 ]]; then
        log ""
        log "First 5 seconds:"
        tail -n +2 "$AGG_FILE" | head -5
        log ""
        log "Around kill time (seconds 55-70):"
        tail -n +2 "$AGG_FILE" | awk -F, -v start_line=55 -v end_line=70 'NR>=start_line && NR<=end_line'
        log ""
        log "Last 5 seconds:"
        tail -n +2 "$AGG_FILE" | tail -5
    fi
fi

log ""
log "Exp 2.3 Phase 112 complete! Results in $BASE_DIR/"
