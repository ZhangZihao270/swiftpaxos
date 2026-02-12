#!/bin/bash

# Distributed Multi-Client CURP-HT Benchmark Runner
#
# Architecture:
#   - Multiple client servers (can be different machines)
#   - Each server runs one client process with multiple clones (threads)
#   - Results from all servers are collected and merged
#
# Usage: ./run-multi-client.sh [options]
#
# Options:
#   -c, --config FILE       Config file (default: local.conf)
#   -k, --clones N          Clones per client process (deprecated, use -t)
#   -t, --threads N         Threads per client process (overrides config clientThreads)
#   -o, --output FILE       Output file for merged results
#   -d, --distributed       Run in distributed mode (SSH to remote servers)
#   --startup-delay N       Seconds to wait for replicas (default: 5)
#   -h, --help              Show this help
#
# Config file should define multiple clients:
#   -- Clients --
#   client0 192.168.1.10
#   client1 192.168.1.11
#   client2 192.168.1.12
#
# In distributed mode, the script will SSH to each client's IP and run the benchmark.
# In local mode, all clients run on localhost.

set -e

# Default values
CONFIG="local.conf"
CLONES=""  # Empty means use config value (deprecated)
THREADS=""  # Empty means use config value (preferred)
OUTPUT=""
DISTRIBUTED=false
STARTUP_DELAY=10
RESULTS_DIR="$(pwd)/results/benchmark-$(date +%Y%m%d-%H%M%S)"
SSH_USER="${SSH_USER:-$(whoami)}"
SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=10"
WORK_DIR="$(pwd)"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -c|--config) CONFIG="$2"; shift 2 ;;
        -k|--clones) CLONES="$2"; shift 2 ;;
        -t|--threads) THREADS="$2"; shift 2 ;;
        -o|--output) OUTPUT="$2"; shift 2 ;;
        -d|--distributed) DISTRIBUTED=true; shift ;;
        --startup-delay) STARTUP_DELAY="$2"; shift 2 ;;
        -h|--help)
            head -30 "$0" | tail -28
            exit 0 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Determine thread count (prefer --threads, then config clientThreads, then config clones)
if [[ -n "$THREADS" ]]; then
    # Explicitly specified via -t/--threads
    THREAD_COUNT=$THREADS
elif [[ -n "$CLONES" ]]; then
    # Legacy -k/--clones specified
    THREAD_COUNT=$((CLONES + 1))
else
    # Read from config (prefer clientThreads over clones)
    CLIENT_THREADS=$(grep -iE "^clientthreads:" "$CONFIG" 2>/dev/null | awk '{print $2}' | head -1)
    CLONES_CFG=$(grep -iE "^clones:" "$CONFIG" 2>/dev/null | awk '{print $2}' | head -1)

    if [[ -n "$CLIENT_THREADS" && "$CLIENT_THREADS" -gt 0 ]]; then
        THREAD_COUNT=$CLIENT_THREADS
    elif [[ -n "$CLONES_CFG" ]]; then
        THREAD_COUNT=$((CLONES_CFG + 1))
    else
        THREAD_COUNT=1  # Default: single thread
    fi
fi

mkdir -p "$RESULTS_DIR"
mkdir -p "$(pwd)/results"  # Ensure results parent dir exists

# Parse config to get clients and replicas
parse_config() {
    # Extract client aliases and addresses (only from -- Clients -- section)
    CLIENT_ALIASES=()
    CLIENT_ADDRS=()
    in_clients_section=false
    while IFS= read -r line; do
        # Check for section markers
        if [[ $line =~ ^--[[:space:]]+Clients ]]; then
            in_clients_section=true
            continue
        elif [[ $line =~ ^--[[:space:]]+ ]]; then
            in_clients_section=false
            continue
        fi

        # Only parse client lines when in Clients section
        if $in_clients_section && [[ $line =~ ^client([0-9]+)[[:space:]]+([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
            CLIENT_ALIASES+=("client${BASH_REMATCH[1]}")
            CLIENT_ADDRS+=("${BASH_REMATCH[2]}")
        fi
    done < "$CONFIG"

    # Extract replica aliases and addresses (only from -- Replicas -- section)
    REPLICA_ALIASES=()
    REPLICA_ADDRS=()
    in_replicas_section=false
    while IFS= read -r line; do
        if [[ $line =~ ^--[[:space:]]+Replicas ]]; then
            in_replicas_section=true
            continue
        elif [[ $line =~ ^--[[:space:]]+ ]]; then
            in_replicas_section=false
            continue
        fi

        if $in_replicas_section && [[ $line =~ ^replica([0-9]+)[[:space:]]+([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
            REPLICA_ALIASES+=("replica${BASH_REMATCH[1]}")
            REPLICA_ADDRS+=("${BASH_REMATCH[2]}")
        fi
    done < "$CONFIG"

    # Extract master address
    MASTER_ADDR=$(grep -E "^master0[[:space:]]+" "$CONFIG" | awk '{print $2}')

    NUM_CLIENTS=${#CLIENT_ALIASES[@]}
    NUM_REPLICAS=${#REPLICA_ALIASES[@]}
}

parse_config

TOTAL_THREADS=$((NUM_CLIENTS * THREAD_COUNT))

# Parse network delay from config (one-way delay in ms, 0 = disabled)
# Uses the built-in application-level delay injection (no root/tc needed)
NETWORK_DELAY=$(grep -iE "^networkDelay:" "$CONFIG" 2>/dev/null | awk '{print $2}' | head -1)
NETWORK_DELAY=${NETWORK_DELAY:-0}

# Compute unique host list (used for sync, cleanup)
ALL_HOSTS=($(printf '%s\n' "$MASTER_ADDR" "${REPLICA_ADDRS[@]}" "${CLIENT_ADDRS[@]}" | sort -u))

# Generate latency config file if network delay is enabled
LATENCY_FLAG=""
if [[ "$NETWORK_DELAY" -gt 0 ]]; then
    RTT_MS=$((NETWORK_DELAY * 2))
    LATENCY_CONF="$RESULTS_DIR/benchmark-latency.conf"
    echo "uniform ${RTT_MS}ms" > "$LATENCY_CONF"
    LATENCY_FLAG="-latency benchmark-latency.conf"
fi

echo "============================================"
echo "  Distributed Multi-Client CURP-HT Benchmark"
echo "============================================"
echo "Config: $CONFIG"
echo "Mode: $(if $DISTRIBUTED; then echo 'Distributed (SSH)'; else echo 'Local'; fi)"
echo "Client servers: $NUM_CLIENTS"
echo "Threads per server: $THREAD_COUNT"
echo "Total concurrent threads: $TOTAL_THREADS"
echo "Replicas: $NUM_REPLICAS"
if [[ "$NETWORK_DELAY" -gt 0 ]]; then
    echo "Network delay: ${NETWORK_DELAY}ms one-way (${RTT_MS}ms RTT)"
fi
echo "Results dir: $RESULTS_DIR"
echo ""

for i in "${!CLIENT_ALIASES[@]}"; do
    echo "  ${CLIENT_ALIASES[$i]} -> ${CLIENT_ADDRS[$i]}"
done
echo ""

# Build if needed
if [[ ! -f ./swiftpaxos ]]; then
    echo "Building swiftpaxos..."
    go build -o swiftpaxos . || exit 1
fi

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."

    if $DISTRIBUTED; then
        # Kill on master
        ssh $SSH_OPTS "$SSH_USER@$MASTER_ADDR" "pkill -x swiftpaxos" 2>/dev/null || true

        # Kill on replicas
        for addr in "${REPLICA_ADDRS[@]}"; do
            ssh $SSH_OPTS "$SSH_USER@$addr" "pkill -x swiftpaxos" 2>/dev/null || true
        done

        # Kill on clients
        for addr in "${CLIENT_ADDRS[@]}"; do
            ssh $SSH_OPTS "$SSH_USER@$addr" "pkill -x swiftpaxos" 2>/dev/null || true
        done

    else
        pkill -x "swiftpaxos" 2>/dev/null || true
    fi

    echo "Results saved in: $RESULTS_DIR"
}
trap cleanup EXIT INT TERM

# Create config copy (don't modify clones - use value from config)
TEMP_CONFIG="$RESULTS_DIR/benchmark.conf"
cp "$CONFIG" "$TEMP_CONFIG"

# Sync files to remote servers (distributed mode)
sync_to_remote() {
    local host="$1"
    echo "  Syncing to $host..."
    # Build list of files to sync
    local files=(./swiftpaxos "$TEMP_CONFIG")
    if [[ -n "$LATENCY_CONF" ]]; then
        files+=("$LATENCY_CONF")
    fi
    rsync -az --delete \
        "${files[@]}" \
        "$SSH_USER@$host:$WORK_DIR/" 2>/dev/null || {
        echo "Warning: rsync failed to $host, trying scp..."
        scp $SSH_OPTS "${files[@]}" "$SSH_USER@$host:$WORK_DIR/"
    }
}

run_remote() {
    local host="$1"
    shift
    ssh $SSH_OPTS "$SSH_USER@$host" "cd $WORK_DIR && $*"
}

run_remote_bg() {
    local host="$1"
    local logfile="$2"
    shift 2
    # -f: fork SSH to background after auth; -n: redirect stdin from /dev/null
    # Without these, SSH hangs waiting for the backgrounded process to exit
    ssh -f -n $SSH_OPTS "$SSH_USER@$host" "cd $WORK_DIR && nohup $* </dev/null > $logfile 2>&1 &"
}

# ========== START SERVERS ==========

if $DISTRIBUTED; then
    # Sync to all servers first
    echo "Syncing files to remote servers..."
    for host in "${ALL_HOSTS[@]}"; do
        sync_to_remote "$host"
    done
    echo ""

    # Kill existing processes (use pkill -x to match exact binary name, not args)
    echo "Stopping existing processes..."
    for host in "${ALL_HOSTS[@]}"; do
        ssh $SSH_OPTS "$SSH_USER@$host" "pkill -x swiftpaxos" 2>/dev/null || true
    done
    sleep 2

    # Start master
    echo "Starting master on $MASTER_ADDR..."
    run_remote_bg "$MASTER_ADDR" "$RESULTS_DIR/master.log" "./swiftpaxos -run master -config $(basename $TEMP_CONFIG) -alias master0"
    sleep 2

    # Start replicas
    echo "Starting replicas..."
    for i in "${!REPLICA_ALIASES[@]}"; do
        echo "  ${REPLICA_ALIASES[$i]} on ${REPLICA_ADDRS[$i]}..."
        run_remote_bg "${REPLICA_ADDRS[$i]}" "$RESULTS_DIR/${REPLICA_ALIASES[$i]}.log" "./swiftpaxos -run server -config $(basename $TEMP_CONFIG) -alias ${REPLICA_ALIASES[$i]} $LATENCY_FLAG"
    done

else
    # Local mode
    echo "Stopping existing processes..."
    pkill -x "swiftpaxos" 2>/dev/null || true
    sleep 1

    echo "Starting master..."
    ./swiftpaxos -run master -config "$TEMP_CONFIG" -alias master0 > "$RESULTS_DIR/master.log" 2>&1 &
    sleep 1

    echo "Starting replicas..."
    for i in "${!REPLICA_ALIASES[@]}"; do
        echo "  ${REPLICA_ALIASES[$i]}..."
        ./swiftpaxos -run server -config "$TEMP_CONFIG" -alias "${REPLICA_ALIASES[$i]}" $LATENCY_FLAG \
            > "$RESULTS_DIR/${REPLICA_ALIASES[$i]}.log" 2>&1 &
    done
fi

echo "Waiting ${STARTUP_DELAY}s for replicas to connect..."
sleep "$STARTUP_DELAY"

# ========== RUN CLIENTS ==========

echo ""
echo "========== Running Clients =========="
CLIENT_PIDS=()

for i in "${!CLIENT_ALIASES[@]}"; do
    alias="${CLIENT_ALIASES[$i]}"
    addr="${CLIENT_ADDRS[$i]}"
    log_file="$RESULTS_DIR/${alias}.log"

    if $DISTRIBUTED; then
        echo "Starting $alias on $addr (remote)..."
        # No -latency flag for clients: clients are co-located with replicas
        ssh $SSH_OPTS "$SSH_USER@$addr" \
            "cd $WORK_DIR && ./swiftpaxos -run client -config $(basename $TEMP_CONFIG) -alias $alias" \
            > "$log_file" 2>&1 &
        CLIENT_PIDS+=($!)
    else
        echo "Starting $alias (local)..."
        ./swiftpaxos -run client -config "$TEMP_CONFIG" -alias "$alias" \
            > "$log_file" 2>&1 &
        CLIENT_PIDS+=($!)
    fi
done

# Wait for all clients
echo ""
echo "Waiting for all clients to complete..."
for pid in "${CLIENT_PIDS[@]}"; do
    wait "$pid" 2>/dev/null || true
done

echo ""
echo "========== All Clients Finished =========="
echo ""

# ========== MERGE RESULTS ==========

echo "========== Merged Results =========="

# Python script for merging results - output to both terminal and file
python3 - "$RESULTS_DIR" "${CLIENT_ALIASES[@]}" << 'PYTHON_SCRIPT' | tee "$RESULTS_DIR/summary.txt"
import sys
import os
import re

results_dir = sys.argv[1]
client_aliases = sys.argv[2:]

# Aggregate metrics
total_ops = 0
durations = []
strong_writes = 0
strong_reads = 0
weak_writes = 0
weak_reads = 0
strong_latencies = []
weak_latencies = []
throughputs = []
client_results = []

for alias in client_aliases:
    log_file = os.path.join(results_dir, f"{alias}.log")
    if not os.path.exists(log_file):
        print(f"Warning: {log_file} not found")
        continue

    with open(log_file, 'r') as f:
        content = f.read()

    result = {'alias': alias}

    # Extract metrics
    ops_match = re.search(r'Total operations: (\d+)', content)
    if ops_match:
        ops = int(ops_match.group(1))
        total_ops += ops
        result['ops'] = ops

    duration_match = re.search(r'Duration: ([\d.]+)s', content)
    if duration_match:
        d = float(duration_match.group(1))
        durations.append(d)
        result['duration'] = d

    throughput_match = re.search(r'Throughput: ([\d.]+) ops/sec', content)
    if throughput_match:
        tp = float(throughput_match.group(1))
        throughputs.append(tp)
        result['throughput'] = tp

    # Strong operations
    strong_match = re.search(r'Strong Operations: (\d+)', content)
    if strong_match:
        # Find the line with Writes and Reads for Strong
        strong_detail = re.search(r'Strong Operations:.*?\n.*?Writes: (\d+).*?Reads: (\d+)', content, re.DOTALL)
        if strong_detail:
            sw = int(strong_detail.group(1))
            sr = int(strong_detail.group(2))
            strong_writes += sw
            strong_reads += sr
            result['strong_writes'] = sw
            result['strong_reads'] = sr

        strong_lat = re.search(r'Strong Operations.*?Median latency: ([\d.]+)ms.*?P99: ([\d.]+)ms', content, re.DOTALL)
        if strong_lat:
            strong_latencies.append((float(strong_lat.group(1)), float(strong_lat.group(2))))
            result['strong_median'] = float(strong_lat.group(1))
            result['strong_p99'] = float(strong_lat.group(2))

    # Weak operations
    weak_match = re.search(r'Weak Operations: (\d+)', content)
    if weak_match:
        weak_detail = re.search(r'Weak Operations:.*?\n.*?Writes: (\d+).*?Reads: (\d+)', content, re.DOTALL)
        if weak_detail:
            ww = int(weak_detail.group(1))
            wr = int(weak_detail.group(2))
            weak_writes += ww
            weak_reads += wr
            result['weak_writes'] = ww
            result['weak_reads'] = wr

        weak_lat = re.search(r'Weak Operations.*?Median latency: ([\d.]+)ms.*?P99: ([\d.]+)ms', content, re.DOTALL)
        if weak_lat:
            weak_latencies.append((float(weak_lat.group(1)), float(weak_lat.group(2))))
            result['weak_median'] = float(weak_lat.group(1))
            result['weak_p99'] = float(weak_lat.group(2))

    client_results.append(result)

# Print aggregated results
total_throughput = sum(throughputs)
max_duration = max(durations) if durations else 0
strong_ops = strong_writes + strong_reads
weak_ops = weak_writes + weak_reads

print(f"Client servers: {len(client_aliases)}")
print(f"Total operations: {total_ops}")
print(f"Max duration: {max_duration:.2f}s")
print(f"Aggregate throughput: {total_throughput:.2f} ops/sec")
print()

if strong_ops > 0:
    strong_pct = strong_ops * 100 / total_ops if total_ops > 0 else 0
    print(f"Strong Operations: {strong_ops} ({strong_pct:.1f}%)")
    print(f"  Writes: {strong_writes} | Reads: {strong_reads}")
    if strong_latencies:
        avg_median = sum(l[0] for l in strong_latencies) / len(strong_latencies)
        max_p99 = max(l[1] for l in strong_latencies)
        print(f"  Avg median: {avg_median:.2f}ms | Max P99: {max_p99:.2f}ms")
    print()

if weak_ops > 0:
    weak_pct = weak_ops * 100 / total_ops if total_ops > 0 else 0
    print(f"Weak Operations: {weak_ops} ({weak_pct:.1f}%)")
    print(f"  Writes: {weak_writes} | Reads: {weak_reads}")
    if weak_latencies:
        avg_median = sum(l[0] for l in weak_latencies) / len(weak_latencies)
        max_p99 = max(l[1] for l in weak_latencies)
        print(f"  Avg median: {avg_median:.2f}ms | Max P99: {max_p99:.2f}ms")
    print()

print("--- Per-Client Breakdown ---")
for r in client_results:
    tp = r.get('throughput', 0)
    ops = r.get('ops', 0)
    print(f"{r['alias']}: {tp:.2f} ops/sec ({ops} ops)")

PYTHON_SCRIPT

echo ""
echo "============================================"
echo "Individual logs: $RESULTS_DIR/"
ls -la "$RESULTS_DIR"/*.log 2>/dev/null | awk '{print "  " $NF}'
