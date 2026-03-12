#!/bin/bash

# Exp 2.3: Raft-HT Leader Failure Recovery (5r/5m/3c)
#
# Measures per-second throughput with a leader kill at t=60s.
# The experiment runs long enough (100K reqs) to observe:
#   1. Steady-state throughput (~60s)
#   2. Leader kill → throughput drop to 0
#   3. New leader election → throughput recovery
#
# Output: TPUT lines in client logs for time-series analysis.
#
# Usage: bash scripts/exp2.3-raftht.sh [output-dir] [kill-delay-s]
#   kill-delay-s: seconds after clients start before killing leader (default: 60)

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d_%H%M%S)
BASE_DIR="${1:-results/exp2.3-raftht-$DATE}"
KILL_DELAY="${2:-60}"
STARTUP_DELAY=25
THREADS=16

ALL_HOSTS=(130.245.173.101 130.245.173.103 130.245.173.104 130.245.173.125 130.245.173.126)
LEADER_HOST="130.245.173.101"  # replica0
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
CONFIG="/tmp/exp2.3-raftht-$$.conf"
cp "$CONFIG_TEMPLATE" "$CONFIG"
sed -i -E "s/^protocol:.*$/protocol: raftht/" "$CONFIG"
sed -i -E "s/^reqs:.*$/reqs:        100000/" "$CONFIG"
sed -i -E "s/^weakRatio:.*$/weakRatio:   50/" "$CONFIG"
sed -i -E "s/^writes:.*$/writes:      50/" "$CONFIG"
sed -i -E "s/^weakWrites:.*$/weakWrites:  50/" "$CONFIG"
trap 'rm -f "$CONFIG"' EXIT

mkdir -p "$BASE_DIR"

log "Exp 2.3: Raft-HT Leader Failure Recovery"
log "Config: raftht, t=$THREADS, reqs=100000, writes=50%, weakWrites=50%"
log "Kill delay: ${KILL_DELAY}s after clients start"
log "Leader: replica0 on $LEADER_HOST"
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

# Schedule leader kill in background
# Total delay = startup_delay + kill_delay (from script start)
TOTAL_KILL_DELAY=$((STARTUP_DELAY + KILL_DELAY + 20))  # +20 for sync/setup overhead
log "Scheduling leader kill in ${TOTAL_KILL_DELAY}s (startup=${STARTUP_DELAY} + overhead=20 + kill_delay=${KILL_DELAY})"

(
    sleep "$TOTAL_KILL_DELAY"
    echo ""
    echo "[$(date '+%H:%M:%S')] *** KILLING LEADER replica0 on $LEADER_HOST ***"
    ssh $SSH_OPTS "$LEADER_HOST" "pkill -9 -x $BINARY" 2>/dev/null || true
    echo "[$(date '+%H:%M:%S')] *** Leader killed ***"
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
            # Format: 2026/03/11 17:28:46 TPUT client0 1773282526 50
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

# Also create an aggregated per-second file (sum across all clients)
AGG_FILE="$BASE_DIR/tput-aggregated.csv"
echo "timestamp,total_ops" > "$AGG_FILE"
if [[ -f "$TPUT_FILE" ]]; then
    tail -n +2 "$TPUT_FILE" | awk -F, '{
        sum[$1] += $3
    } END {
        n = asorti(sum, sorted)
        for (i = 1; i <= n; i++) {
            print sorted[i] "," sum[sorted[i]]
        }
    }' >> "$AGG_FILE"
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

    # Show a few lines around the kill time
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
log "Exp 2.3 complete! Results in $BASE_DIR/"
