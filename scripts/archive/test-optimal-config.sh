#!/bin/bash
# Phase 31.5d: Test Optimal Configuration Combination
# Combine best pendings (15) with best thread count (8 streams)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/docs/phase-31-profiles"
ITERATIONS=5  # More iterations for final validation
REQUESTS=10000

mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULT_FILE="$OUTPUT_DIR/optimal-config-results-$TIMESTAMP.txt"

echo "=== Phase 31.5d: Optimal Configuration Test ===" | tee "$RESULT_FILE"
echo "Goal: Reach 23K ops/sec by combining optimal pendings + threads" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

cd "$PROJECT_ROOT"

# Function to run test
run_test() {
    local pendings=$1
    local threads=$2
    local streams=$((2 * threads))  # 2 clients

    echo "=== Configuration: pendings=$pendings, threads=$threads, streams=$streams ===" | tee -a "$RESULT_FILE"

    cp multi-client.conf multi-client.conf.backup
    sed -i "s/^reqs:.*/reqs:        $REQUESTS/" multi-client.conf
    sed -i "s/^clientThreads:.*/clientThreads: $threads/" multi-client.conf
    sed -i "s/^pendings:.*/pendings:    $pendings/" multi-client.conf

    declare -a throughputs
    declare -a weak_medians

    for i in $(seq 1 $ITERATIONS); do
        echo "  Iteration $i/$ITERATIONS..." | tee -a "$RESULT_FILE"

        OUTPUT=$(timeout 120 ./run-multi-client.sh -c multi-client.conf 2>&1)

        THROUGHPUT=$(echo "$OUTPUT" | grep -oP 'Aggregate throughput:\s+\K[0-9.]+' | tail -1)
        WEAK_MED=$(echo "$OUTPUT" | grep "Weak Operations:" -A 2 | grep -oP 'Avg median:\s+\K[0-9.]+' | head -1)

        throughputs+=("$THROUGHPUT")
        weak_medians+=("$WEAK_MED")

        echo "    Throughput: $THROUGHPUT ops/sec, Weak median: ${WEAK_MED}ms" | tee -a "$RESULT_FILE"
        sleep 2
    done

    # Calculate statistics
    local sum=0
    local min=999999
    local max=0
    for val in "${throughputs[@]}"; do
        sum=$(echo "$sum + $val" | bc -l)
        if (( $(echo "$val < $min" | bc -l) )); then min=$val; fi
        if (( $(echo "$val > $max" | bc -l) )); then max=$val; fi
    done
    local avg=$(echo "scale=2; $sum / $ITERATIONS" | bc -l)

    local weak_sum=0
    for val in "${weak_medians[@]}"; do
        weak_sum=$(echo "$weak_sum + $val" | bc -l)
    done
    local weak_avg=$(echo "scale=2; $weak_sum / $ITERATIONS" | bc -l)

    # Check target
    local target_met=""
    if (( $(echo "$avg >= 23000" | bc -l) )) && (( $(echo "$weak_avg < 2.0" | bc -l) )); then
        target_met=" ✓✓✓ TARGET MET! ✓✓✓"
    elif (( $(echo "$weak_avg >= 2.0" | bc -l) )); then
        target_met=" ✗ Latency constraint violated"
    fi

    echo "  Statistics:" | tee -a "$RESULT_FILE"
    echo "    Min: $min ops/sec" | tee -a "$RESULT_FILE"
    echo "    Max: $max ops/sec" | tee -a "$RESULT_FILE"
    echo "    Avg: $avg ops/sec" | tee -a "$RESULT_FILE"
    echo "    Weak median avg: ${weak_avg}ms" | tee -a "$RESULT_FILE"
    echo "  $target_met" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"

    mv multi-client.conf.backup multi-client.conf
}

echo "Testing optimal configurations..." | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Test 1: pendings=15, 4 streams (best from pendings sweep)
run_test 15 2

# Test 2: pendings=15, 8 streams (combining best of both)
run_test 15 4

# Test 3: pendings=12, 8 streams (more conservative)
run_test 12 4

# Test 4: pendings=18, 4 streams (higher pendings, fewer threads)
run_test 18 2

echo "=== Final Summary ===" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Target: 23,000 ops/sec with weak median < 2.0ms" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Results saved to: $RESULT_FILE" | tee -a "$RESULT_FILE"
