#!/bin/bash

# Local Multi-Client Benchmark Runner
#
# Starts master + N replicas + M clients on localhost, waits for all clients
# to finish, then merges results.
#
# Usage: ./run-local-multi.sh [options]
#
# Options:
#   -c, --config FILE       Config file (default: eval-local.conf)
#   -t, --threads N         Override clientThreads in config
#   -o, --output DIR        Output directory (default: auto-generated)
#   --startup-delay N       Seconds to wait for replicas (default: 5)
#   -h, --help              Show this help

set -e

# Default values
CONFIG="eval-local.conf"
THREADS=""
OUTPUT_DIR=""
STARTUP_DELAY=15
WORK_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$WORK_DIR"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -c|--config) CONFIG="$2"; shift 2 ;;
        -t|--threads) THREADS="$2"; shift 2 ;;
        -o|--output) OUTPUT_DIR="$2"; shift 2 ;;
        --startup-delay) STARTUP_DELAY="$2"; shift 2 ;;
        -h|--help) head -15 "$0" | tail -13; exit 0 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

if [[ ! -f "$CONFIG" ]]; then
    echo "ERROR: Config file $CONFIG not found"
    exit 1
fi

# Auto-generate output dir if not specified
if [[ -z "$OUTPUT_DIR" ]]; then
    OUTPUT_DIR="results/benchmark-$(date +%Y%m%d-%H%M%S)"
fi
mkdir -p "$OUTPUT_DIR"

# Parse config: extract replicas, clients, master
parse_section() {
    local section="$1"
    local prefix="$2"
    local -n aliases_ref="$3"
    local in_section=false
    while IFS= read -r line; do
        if [[ $line =~ ^--[[:space:]]+${section} ]]; then
            in_section=true; continue
        elif [[ $line =~ ^--[[:space:]]+ ]]; then
            in_section=false; continue
        fi
        if $in_section && [[ $line =~ ^${prefix}([0-9]+)[[:space:]]+([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
            aliases_ref+=("${prefix}${BASH_REMATCH[1]}")
        fi
    done < "$CONFIG"
}

REPLICA_ALIASES=()
CLIENT_ALIASES=()
parse_section "Replicas" "replica" REPLICA_ALIASES
parse_section "Clients" "client" CLIENT_ALIASES

NUM_REPLICAS=${#REPLICA_ALIASES[@]}
NUM_CLIENTS=${#CLIENT_ALIASES[@]}

# Determine thread count
if [[ -n "$THREADS" ]]; then
    THREAD_COUNT=$THREADS
else
    THREAD_COUNT=$(grep -iE "^clientThreads:" "$CONFIG" 2>/dev/null | awk '{print $2}' | head -1)
    THREAD_COUNT=${THREAD_COUNT:-1}
fi

TOTAL_THREADS=$((NUM_CLIENTS * THREAD_COUNT))

# Create temp config with thread override
TEMP_CONFIG="$OUTPUT_DIR/benchmark.conf"
cp "$CONFIG" "$TEMP_CONFIG"

if [[ -n "$THREADS" ]]; then
    if grep -qiE "^clientThreads:" "$TEMP_CONFIG"; then
        sed -i -E "s/^(clientThreads|clientthreads):.*$/clientThreads: $THREAD_COUNT/I" "$TEMP_CONFIG"
    else
        sed -i "/^commandSize:/a clientThreads: $THREAD_COUNT" "$TEMP_CONFIG"
    fi
fi

# Parse network delay
NETWORK_DELAY=$(grep -iE "^networkDelay:" "$CONFIG" 2>/dev/null | awk '{print $2}' | head -1)
NETWORK_DELAY=${NETWORK_DELAY:-0}

# Generate latency config
LATENCY_FLAG=""
if [[ "$NETWORK_DELAY" -gt 0 ]]; then
    RTT_MS=$((NETWORK_DELAY * 2))
    LATENCY_CONF="$WORK_DIR/benchmark-latency.conf"
    echo "uniform ${RTT_MS}ms" > "$LATENCY_CONF"
    LATENCY_FLAG="-latency benchmark-latency.conf"
fi

echo "============================================"
echo "  Local Multi-Client Benchmark"
echo "============================================"
echo "Config: $CONFIG"
echo "Replicas: $NUM_REPLICAS"
echo "Clients: $NUM_CLIENTS"
echo "Threads per client: $THREAD_COUNT"
echo "Total threads: $TOTAL_THREADS"
if [[ "$NETWORK_DELAY" -gt 0 ]]; then
    echo "Network delay: ${NETWORK_DELAY}ms one-way (${RTT_MS}ms RTT)"
fi
echo "Output: $OUTPUT_DIR"
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
    pkill -9 -x "swiftpaxos" 2>/dev/null || true
    # Wait until all swiftpaxos processes are actually gone
    for i in $(seq 1 30); do
        pgrep -x "swiftpaxos" >/dev/null 2>&1 || break
        sleep 0.2
    done
    # Clean up latency config
    rm -f "$WORK_DIR/benchmark-latency.conf"
    echo "Results saved in: $OUTPUT_DIR"
}
trap cleanup EXIT INT TERM

# Kill existing processes and wait for them to actually exit
pkill -9 -x "swiftpaxos" 2>/dev/null || true
for i in $(seq 1 30); do
    pgrep -x "swiftpaxos" >/dev/null 2>&1 || break
    sleep 0.2
done
sleep 1

# Start master
echo "Starting master..."
./swiftpaxos -run master -config "$TEMP_CONFIG" -alias master0 \
    > "$OUTPUT_DIR/master.log" 2>&1 &
sleep 1

# Start replicas
echo "Starting replicas..."
for alias in "${REPLICA_ALIASES[@]}"; do
    echo "  $alias..."
    ./swiftpaxos -run server -config "$TEMP_CONFIG" -alias "$alias" $LATENCY_FLAG \
        > "$OUTPUT_DIR/${alias}.log" 2>&1 &
done

echo "Waiting ${STARTUP_DELAY}s for replicas to connect..."
sleep "$STARTUP_DELAY"

# Start clients
echo ""
echo "========== Running Clients =========="
CLIENT_PIDS=()
for alias in "${CLIENT_ALIASES[@]}"; do
    echo "Starting $alias..."
    ./swiftpaxos -run client -config "$TEMP_CONFIG" -alias "$alias" \
        > "$OUTPUT_DIR/${alias}.log" 2>&1 &
    CLIENT_PIDS+=($!)
done

# Wait for all clients
echo ""
echo "Waiting for all clients to complete..."
ALL_OK=true
for pid in "${CLIENT_PIDS[@]}"; do
    if ! wait "$pid" 2>/dev/null; then
        ALL_OK=false
    fi
done

echo ""
echo "========== All Clients Finished =========="
echo ""

# Merge results using Python
echo "========== Merged Results =========="

python3 - "$OUTPUT_DIR" "${CLIENT_ALIASES[@]}" << 'PYTHON_SCRIPT' | tee "$OUTPUT_DIR/summary.txt"
import sys
import os
import re

results_dir = sys.argv[1]
client_aliases = sys.argv[2:]

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
    strong_detail = re.search(r'Strong Operations:.*?\n.*?Writes: (\d+).*?Reads: (\d+)', content, re.DOTALL)
    if strong_detail:
        sw = int(strong_detail.group(1))
        sr = int(strong_detail.group(2))
        strong_writes += sw
        strong_reads += sr
        result['strong_writes'] = sw
        result['strong_reads'] = sr

    strong_lat = re.search(r'Strong Operations.*?Avg: ([\d.]+)ms.*?Median: ([\d.]+)ms.*?P99: ([\d.]+)ms', content, re.DOTALL)
    if strong_lat:
        strong_latencies.append((float(strong_lat.group(1)), float(strong_lat.group(2)), float(strong_lat.group(3))))
        result['strong_avg'] = float(strong_lat.group(1))
        result['strong_median'] = float(strong_lat.group(2))
        result['strong_p99'] = float(strong_lat.group(3))

    # Weak operations
    weak_detail = re.search(r'Weak Operations:.*?\n.*?Writes: (\d+).*?Reads: (\d+)', content, re.DOTALL)
    if weak_detail:
        ww = int(weak_detail.group(1))
        wr = int(weak_detail.group(2))
        weak_writes += ww
        weak_reads += wr
        result['weak_writes'] = ww
        result['weak_reads'] = wr

    weak_lat = re.search(r'Weak Operations.*?Avg: ([\d.]+)ms.*?Median: ([\d.]+)ms.*?P99: ([\d.]+)ms', content, re.DOTALL)
    if weak_lat:
        weak_latencies.append((float(weak_lat.group(1)), float(weak_lat.group(2)), float(weak_lat.group(3))))
        result['weak_avg'] = float(weak_lat.group(1))
        result['weak_median'] = float(weak_lat.group(2))
        result['weak_p99'] = float(weak_lat.group(3))

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
        avg_avg = sum(l[0] for l in strong_latencies) / len(strong_latencies)
        avg_median = sum(l[1] for l in strong_latencies) / len(strong_latencies)
        max_p99 = max(l[2] for l in strong_latencies)
        print(f"  Avg: {avg_avg:.2f}ms | Avg median: {avg_median:.2f}ms | Max P99: {max_p99:.2f}ms")
    print()

if weak_ops > 0:
    weak_pct = weak_ops * 100 / total_ops if total_ops > 0 else 0
    print(f"Weak Operations: {weak_ops} ({weak_pct:.1f}%)")
    print(f"  Writes: {weak_writes} | Reads: {weak_reads}")
    if weak_latencies:
        avg_avg = sum(l[0] for l in weak_latencies) / len(weak_latencies)
        avg_median = sum(l[1] for l in weak_latencies) / len(weak_latencies)
        max_p99 = max(l[2] for l in weak_latencies)
        print(f"  Avg: {avg_avg:.2f}ms | Avg median: {avg_median:.2f}ms | Max P99: {max_p99:.2f}ms")
    print()

print("--- Per-Client Breakdown ---")
for r in client_results:
    tp = r.get('throughput', 0)
    ops = r.get('ops', 0)
    print(f"{r['alias']}: {tp:.2f} ops/sec ({ops} ops)")

PYTHON_SCRIPT

echo ""
echo "============================================"
echo "Individual logs: $OUTPUT_DIR/"
ls -la "$OUTPUT_DIR"/*.log 2>/dev/null | awk '{print "  " $NF}'
