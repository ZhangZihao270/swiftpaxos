#!/bin/bash
# Phase 31.5: Client Parallelism Scaling Test
# Tests throughput with different client/thread configurations

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/docs/phase-31-profiles"
ITERATIONS=3
REQUESTS=10000  # 10K ops per client (short test for fast iteration)

mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULT_FILE="$OUTPUT_DIR/client-parallelism-results-$TIMESTAMP.txt"

echo "=== Phase 31.5: Client Parallelism Scaling Test ===" | tee "$RESULT_FILE"
echo "Test configurations:" | tee -a "$RESULT_FILE"
echo "  - Requests per client: $REQUESTS" | tee -a "$RESULT_FILE"
echo "  - Iterations per config: $ITERATIONS" | tee -a "$RESULT_FILE"
echo "  - Pendings: 10 (fixed)" | tee -a "$RESULT_FILE"
echo "  - Timestamp: $TIMESTAMP" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

cd "$PROJECT_ROOT"

# Function to run test with specific client/thread configuration
run_test() {
    local num_clients=$1
    local threads_per_client=$2
    local total_streams=$((num_clients * threads_per_client))

    echo "=== Testing: $num_clients clients × $threads_per_client threads = $total_streams streams ===" | tee -a "$RESULT_FILE"

    # Backup original config
    cp multi-client.conf multi-client.conf.backup

    # Update config
    sed -i "s/^reqs:.*/reqs:        $REQUESTS/" multi-client.conf
    sed -i "s/^clientThreads:.*/clientThreads: $threads_per_client/" multi-client.conf

    # Enable/disable clients based on num_clients
    if [ "$num_clients" -eq 2 ]; then
        sed -i 's/^client0/client0/' multi-client.conf
        sed -i 's/^client1/client1/' multi-client.conf
        sed -i 's/^client2/# client2/' multi-client.conf
        sed -i 's/^client3/# client3/' multi-client.conf
    elif [ "$num_clients" -eq 3 ]; then
        sed -i 's/^# client0/client0/' multi-client.conf
        sed -i 's/^# client1/client1/' multi-client.conf
        sed -i 's/^# client2/client2/' multi-client.conf
        sed -i 's/^# client3/# client3/' multi-client.conf
    elif [ "$num_clients" -eq 4 ]; then
        sed -i 's/^# client0/client0/' multi-client.conf
        sed -i 's/^# client1/client1/' multi-client.conf
        sed -i 's/^# client2/client2/' multi-client.conf
        sed -i 's/^# client3/client3/' multi-client.conf
    fi

    # Arrays to store results
    declare -a throughputs
    declare -a weak_medians
    declare -a strong_medians

    # Run iterations
    for i in $(seq 1 $ITERATIONS); do
        echo "  Iteration $i/$ITERATIONS..." | tee -a "$RESULT_FILE"

        OUTPUT=$(timeout 120 ./run-multi-client.sh -c multi-client.conf 2>&1)

        # Extract metrics
        THROUGHPUT=$(echo "$OUTPUT" | grep -oP 'Aggregate throughput:\s+\K[0-9.]+' | tail -1)
        WEAK_MED=$(echo "$OUTPUT" | grep "Weak Operations:" -A 2 | grep -oP 'Avg median:\s+\K[0-9.]+' | head -1)
        STRONG_MED=$(echo "$OUTPUT" | grep "Strong Operations:" -A 2 | grep -oP 'Avg median:\s+\K[0-9.]+' | head -1)

        throughputs+=("$THROUGHPUT")
        weak_medians+=("$WEAK_MED")
        strong_medians+=("$STRONG_MED")

        echo "    Throughput: $THROUGHPUT ops/sec, Weak: ${WEAK_MED}ms, Strong: ${STRONG_MED}ms" | tee -a "$RESULT_FILE"

        sleep 2
    done

    # Calculate average
    local sum=0
    for val in "${throughputs[@]}"; do
        sum=$(echo "$sum + $val" | bc -l)
    done
    local avg=$(echo "scale=2; $sum / $ITERATIONS" | bc -l)

    local weak_sum=0
    for val in "${weak_medians[@]}"; do
        weak_sum=$(echo "$weak_sum + $val" | bc -l)
    done
    local weak_avg=$(echo "scale=2; $weak_sum / $ITERATIONS" | bc -l)

    echo "  Average: $avg ops/sec (weak median: ${weak_avg}ms)" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"

    # Restore original config
    mv multi-client.conf.backup multi-client.conf
}

# Test configurations
echo "Starting tests..." | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Baseline: 2 clients × 2 threads = 4 streams
run_test 2 2

# Scale threads: 2 clients × 4 threads = 8 streams (2x parallelism)
run_test 2 4

# Scale threads: 2 clients × 6 threads = 12 streams (3x parallelism)
run_test 2 6

# Scale threads: 2 clients × 8 threads = 16 streams (4x parallelism)
run_test 2 8

# Alternative: 4 clients × 2 threads = 8 streams (different topology)
# Commented out by default - requires enabling client2 and client3 in config
# run_test 4 2

# Alternative: 4 clients × 3 threads = 12 streams (different topology)
# run_test 4 3

echo "=== Summary ===" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Results saved to: $RESULT_FILE" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Next steps:" | tee -a "$RESULT_FILE"
echo "  1. Review results in $RESULT_FILE" | tee -a "$RESULT_FILE"
echo "  2. Plot throughput vs streams (4, 8, 12, 16)" | tee -a "$RESULT_FILE"
echo "  3. Identify optimal configuration (max throughput, weak median < 2ms)" | tee -a "$RESULT_FILE"
echo "  4. Document findings in docs/phase-31.5-client-parallelism.md" | tee -a "$RESULT_FILE"
