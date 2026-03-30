#!/bin/bash

# Exp 2.3 (Local): CURP-HT Leader Failure Recovery
#
# Runs 5 replicas + 1 client on localhost with simulated network delay.
# Kills the leader at t=KILL_DELAY, observes throughput drop and recovery.
#
# Usage: bash scripts/exp2.3-curpht-local.sh [output-dir] [kill-delay-s]
#   kill-delay-s: seconds after clients start before killing leader (default: 30)

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$WORK_DIR"

DATE=$(date +%Y%m%d_%H%M%S)
BASE_DIR="${1:-results/exp2.3-curpht-local-$DATE}"
KILL_DELAY="${2:-30}"
THREADS=4
BINARY="swiftpaxos-dist"

mkdir -p "$BASE_DIR"

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

# Create local config: 5 replicas on 127.0.0.x, 1 client
CONFIG="$BASE_DIR/benchmark.conf"
cat > "$CONFIG" << 'CONF'
-- Replicas --
replica0 127.0.0.1
replica1 127.0.0.2
replica2 127.0.0.3
replica3 127.0.0.4
replica4 127.0.0.5

-- Clients --
client0 127.0.0.6

-- Master --
master0 127.0.0.1

masterPort: 7087

protocol: curpht
weakRatio:   50
writes:      50
weakWrites:  50

maxDescRoutines: 200
batchDelayUs: 0

noop:       false
thrifty:    false
fast:       true

reqs:        50000
commandSize: 100
clientThreads: 4
pipeline:    true
pendings:    15

keySpace:    1000000
zipfSkew:    0

-- Proxy --
server_alias replica0
client0 (local)
---
CONF

# Create latency config: uniform 25ms RTT (12.5ms one-way)
LATENCY_CONF="$BASE_DIR/latency.conf"
echo "uniform 25ms" > "$LATENCY_CONF"

log "Exp 2.3 (Local): CURP-HT Leader Failure Recovery"
log "Config: curpht, t=$THREADS, reqs=50000, weakRatio=50%"
log "Network delay: 25ms RTT (simulated)"
log "Kill delay: ${KILL_DELAY}s after client starts"
log "Output: $BASE_DIR"
echo ""

# Build
log "Building $BINARY..."
go build -o "$BINARY" . 2>&1
if [[ $? -ne 0 ]]; then
    log "ERROR: Build failed"
    exit 1
fi

# Kill any existing instances
pkill -9 -x "$BINARY" 2>/dev/null || true
sleep 2

# Start master
log "Starting master..."
./"$BINARY" -run master -config "$CONFIG" -alias master0 \
    > "$BASE_DIR/master.log" 2>&1 &
MASTER_PID=$!
sleep 2

# Start 5 replicas
REPLICA_PIDS=()
for i in 0 1 2 3 4; do
    log "Starting replica$i..."
    ./"$BINARY" -run server -config "$CONFIG" -alias "replica$i" -latency "$LATENCY_CONF" \
        > "$BASE_DIR/replica${i}.log" 2>&1 &
    REPLICA_PIDS+=($!)
done
sleep 5

# Schedule leader kill in background
log "Scheduling leader kill in ${KILL_DELAY}s..."
(
    sleep "$KILL_DELAY"
    echo ""
    echo "[$(date '+%H:%M:%S')] *** KILLING LEADER replica0 (PID ${REPLICA_PIDS[0]}) ***"
    kill -9 "${REPLICA_PIDS[0]}" 2>/dev/null || true
    echo "[$(date '+%H:%M:%S')] *** Leader killed ***"
) &
KILL_PID=$!

# Start client
log "Starting client (t=$THREADS)..."
./"$BINARY" -run client -config "$CONFIG" -alias client0 -latency "$LATENCY_CONF" \
    > "$BASE_DIR/client0.log" 2>&1 &
CLIENT_PID=$!

# Wait for client to finish (or timeout)
log "Waiting for client to finish (timeout 300s)..."
timeout 300 tail --pid=$CLIENT_PID -f /dev/null 2>/dev/null || true

# Cleanup
kill "$KILL_PID" 2>/dev/null || true
wait "$KILL_PID" 2>/dev/null || true
kill "$CLIENT_PID" 2>/dev/null || true
for pid in "${REPLICA_PIDS[@]}"; do
    kill -9 "$pid" 2>/dev/null || true
done
kill -9 "$MASTER_PID" 2>/dev/null || true
sleep 1

# Extract TPUT data
log "Extracting TPUT data..."
TPUT_FILE="$BASE_DIR/tput-all.csv"
echo "timestamp,client,ops" > "$TPUT_FILE"

grep "TPUT " "$BASE_DIR/client0.log" | while read -r line; do
    # Format: 2026/03/30 12:00:00 TPUT client_id unix_ts ops
    client_id=$(echo "$line" | awk '{print $4}')
    ts=$(echo "$line" | awk '{print $5}')
    ops=$(echo "$line" | awk '{print $6}')
    if [[ -n "$ts" && -n "$ops" ]]; then
        echo "$ts,$client_id,$ops" >> "$TPUT_FILE"
    fi
done

# Aggregate per second
AGG_FILE="$BASE_DIR/tput-aggregated.csv"
echo "second,total_ops" > "$AGG_FILE"
if [[ -f "$TPUT_FILE" ]]; then
    # Get the first timestamp as t=0 reference
    FIRST_TS=$(tail -n +2 "$TPUT_FILE" | head -1 | cut -d, -f1)
    if [[ -n "$FIRST_TS" ]]; then
        tail -n +2 "$TPUT_FILE" | awk -F, -v first="$FIRST_TS" '{
            sec = $1 - first
            sum[sec] += $3
        } END {
            for (s in sum) print s "," sum[s]
        }' | sort -t, -k1,1n >> "$AGG_FILE"
    fi
fi

# Print summary
log ""
log "=== Results ==="
total_lines=$(tail -n +2 "$AGG_FILE" | wc -l)
log "TPUT data: $total_lines seconds recorded"

if [[ -f "$AGG_FILE" && $total_lines -gt 0 ]]; then
    log ""
    log "Per-second throughput (second, ops):"
    cat "$AGG_FILE"

    # Show pre/post kill stats
    log ""
    PRE_KILL=$(tail -n +2 "$AGG_FILE" | awk -F, -v kd="$KILL_DELAY" '$1 < kd && $1 > 5 {sum+=$2; n++} END {if(n>0) printf "%.0f", sum/n; else print "N/A"}')
    POST_KILL=$(tail -n +2 "$AGG_FILE" | awk -F, -v kd="$KILL_DELAY" '$1 > kd+5 {sum+=$2; n++} END {if(n>0) printf "%.0f", sum/n; else print "N/A"}')
    log "Pre-kill avg throughput (t=5 to t=$((KILL_DELAY-1))): $PRE_KILL ops/sec"
    log "Post-kill avg throughput (t=$((KILL_DELAY+5))+): $POST_KILL ops/sec"
fi

log ""
log "Done! Results in $BASE_DIR"
