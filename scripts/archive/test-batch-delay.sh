#!/bin/bash
# Phase 31.4: Test Network Batching Delay Impact
# Measure throughput and latency with different batch delay values

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/docs/phase-31-profiles"
ITERATIONS=3
REQUESTS=10000  # 10K ops per client (short test)

mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULT_FILE="$OUTPUT_DIR/batch-delay-results-$TIMESTAMP.txt"

echo "=== Phase 31.4: Batch Delay Optimization Test ===" | tee "$RESULT_FILE"
echo "Goal: Find optimal batch delay to reach 23K ops/sec" | tee -a "$RESULT_FILE"
echo "Configuration: pendings=15, 4 streams (optimal from Phase 31.5c)" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Test parameters:" | tee -a "$RESULT_FILE"
echo "  - Requests per client: $REQUESTS" | tee -a "$RESULT_FILE"
echo "  - Iterations per config: $ITERATIONS" | tee -a "$RESULT_FILE"
echo "  - Timestamp: $TIMESTAMP" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

cd "$PROJECT_ROOT"

# Function to run test with specific batch delay
run_test() {
    local delay_us=$1

    echo "=== Testing: batchDelayUs=$delay_us ===" | tee -a "$RESULT_FILE"

    # Backup original config
    cp multi-client.conf multi-client.conf.backup

    # Update config
    sed -i "s/^reqs:.*/reqs:        $REQUESTS/" multi-client.conf
    sed -i "s/^clientThreads:.*/clientThreads: 2/" multi-client.conf
    sed -i "s/^pendings:.*/pendings:    15/" multi-client.conf

    # Add or update batchDelayUs parameter
    if grep -q "^batchDelayUs:" multi-client.conf; then
        sed -i "s/^batchDelayUs:.*/batchDelayUs: $delay_us/" multi-client.conf
    else
        # Add after maxDescRoutines
        sed -i "/^maxDescRoutines:/a batchDelayUs: $delay_us   // Batching delay in microseconds (Phase 31.4)" multi-client.conf
    fi

    # Ensure 2 clients
    sed -i 's/^# client0/client0/' multi-client.conf
    sed -i 's/^# client1/client1/' multi-client.conf
    sed -i 's/^client2/# client2/' multi-client.conf
    sed -i 's/^client3/# client3/' multi-client.conf

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

    local strong_sum=0
    for val in "${strong_medians[@]}"; do
        strong_sum=$(echo "$strong_sum + $val" | bc -l)
    done
    local strong_avg=$(echo "scale=2; $strong_sum / $ITERATIONS" | bc -l)

    # Check if target met
    local target_met=""
    if (( $(echo "$avg >= 23000" | bc -l) )) && (( $(echo "$weak_avg < 2.0" | bc -l) )); then
        target_met=" ✓✓✓ TARGET MET! ✓✓✓"
    elif (( $(echo "$weak_avg >= 2.0" | bc -l) )); then
        target_met=" ✗ Latency constraint violated"
    fi

    echo "  Average: $avg ops/sec (weak: ${weak_avg}ms, strong: ${strong_avg}ms)$target_met" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"

    # Restore original config
    mv multi-client.conf.backup multi-client.conf
}

echo "Starting batch delay sweep..." | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Test different delay values
run_test 0      # Baseline (zero-delay batching)
run_test 25     # Small delay
run_test 50     # Moderate delay
run_test 75     # Higher delay
run_test 100    # Maximum delay (from Phase 31.4 spec)
run_test 150    # Beyond spec (to see if more helps)

echo "=== Summary ===" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Target: 23,000 ops/sec with weak median < 2.0ms" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Results saved to: $RESULT_FILE" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Next steps:" | tee -a "$RESULT_FILE"
echo "  1. Review results and identify optimal batch delay" | tee -a "$RESULT_FILE"
echo "  2. If target met, update multi-client.conf with optimal value" | tee -a "$RESULT_FILE"
echo "  3. Document findings in docs/phase-31.4-network-batching.md" | tee -a "$RESULT_FILE"
