#!/bin/bash
# Phase 32.4 Quick Test: Test key batch delay values only
# Test delays: 0, 100, 150, 200μs (4 values × 3 iterations = 12 runs, ~24 minutes)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/docs"
CONFIG_TEMPLATE="$PROJECT_ROOT/curpht-baseline.conf"
CONFIG_TEMP="$PROJECT_ROOT/curpht-batch-test.conf"
ITERATIONS="${1:-3}"

mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULT_FILE="$OUTPUT_DIR/phase-32.4-quick-test-$TIMESTAMP.txt"

# Key batch delays to test (fewer values for faster testing)
DELAYS=(0 100 150 200)

echo "========================================" | tee "$RESULT_FILE"
echo "Phase 32.4: CURP-HT Batch Delay Quick Test" | tee -a "$RESULT_FILE"
echo "========================================" | tee -a "$RESULT_FILE"
echo "Configuration:" | tee -a "$RESULT_FILE"
echo "  - Protocol: curpht" | tee -a "$RESULT_FILE"
echo "  - MaxDescRoutines: 200" | tee -a "$RESULT_FILE"
echo "  - Pendings: 20" | tee -a "$RESULT_FILE"
echo "  - Iterations per delay: $ITERATIONS" | tee -a "$RESULT_FILE"
echo "  - Delays to test: ${DELAYS[*]} μs (quick test)" | tee -a "$RESULT_FILE"
echo "  - Timestamp: $TIMESTAMP" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Store results for all delays
declare -A avg_throughputs
declare -A max_throughputs
declare -A min_throughputs
declare -A avg_strong_medians
declare -A avg_weak_medians

for delay in "${DELAYS[@]}"; do
    echo "========================================" | tee -a "$RESULT_FILE"
    echo "Testing batchDelayUs = $delay μs" | tee -a "$RESULT_FILE"
    echo "========================================" | tee -a "$RESULT_FILE"

    # Create temporary config with this batch delay
    cp "$CONFIG_TEMPLATE" "$CONFIG_TEMP"

    # Insert batchDelayUs after maxDescRoutines line
    if [ "$delay" -gt 0 ]; then
        sed -i "/maxDescRoutines:/a batchDelayUs: $delay      // Network batching delay (Phase 32.4)" "$CONFIG_TEMP"
    fi

    # Arrays for this delay's iterations
    declare -a throughputs
    declare -a strong_medians
    declare -a weak_medians

    for i in $(seq 1 $ITERATIONS); do
        echo "  Iteration $i/$ITERATIONS (delay=${delay}μs)..." | tee -a "$RESULT_FILE"

        # Ensure clean state before running
        killall -9 server master client 2>/dev/null || true
        sleep 3

        # Run benchmark and save to temp file
        cd "$PROJECT_ROOT"
        TEMP_OUTPUT="/tmp/curpht-batch-test-${delay}-${i}.txt"
        timeout 180 ./run-multi-client.sh -c "$CONFIG_TEMP" > "$TEMP_OUTPUT" 2>&1 || true

        # Extract metrics from temp file
        THROUGHPUT=$(grep "Aggregate throughput:" "$TEMP_OUTPUT" | grep -oP '[0-9]+\.[0-9]+' | head -1)
        STRONG_MEDIAN=$(grep "Strong Operations:" -A 2 "$TEMP_OUTPUT" | grep "Avg median:" | grep -oP '[0-9]+\.[0-9]+' | head -1)
        WEAK_MEDIAN=$(grep "Weak Operations:" -A 2 "$TEMP_OUTPUT" | grep "Avg median:" | grep -oP '[0-9]+\.[0-9]+' | head -1)

        # Clean up temp file
        rm -f "$TEMP_OUTPUT"

        # Store results (default to 0 if extraction failed)
        throughputs+=("${THROUGHPUT:-0}")
        strong_medians+=("${STRONG_MEDIAN:-0}")
        weak_medians+=("${WEAK_MEDIAN:-0}")

        echo "    Throughput: ${THROUGHPUT:-0} ops/sec" | tee -a "$RESULT_FILE"
        echo "    Strong median: ${STRONG_MEDIAN:-0}ms, Weak median: ${WEAK_MEDIAN:-0}ms" | tee -a "$RESULT_FILE"

        # Longer sleep between iterations to avoid resource contention/segfaults
        sleep 5
    done

    # Calculate statistics for this delay
    min_tput=${throughputs[0]}
    max_tput=${throughputs[0]}
    sum_tput=0
    sum_strong=0
    sum_weak=0

    for tput in "${throughputs[@]}"; do
        sum_tput=$(echo "$sum_tput + $tput" | bc)
        if (( $(echo "$tput < $min_tput" | bc -l) )); then min_tput=$tput; fi
        if (( $(echo "$tput > $max_tput" | bc -l) )); then max_tput=$tput; fi
    done
    for lat in "${strong_medians[@]}"; do
        sum_strong=$(echo "$sum_strong + $lat" | bc)
    done
    for lat in "${weak_medians[@]}"; do
        sum_weak=$(echo "$sum_weak + $lat" | bc)
    done

    avg_tput=$(echo "scale=2; $sum_tput / $ITERATIONS" | bc)
    avg_strong=$(echo "scale=2; $sum_strong / $ITERATIONS" | bc)
    avg_weak=$(echo "scale=2; $sum_weak / $ITERATIONS" | bc)

    # Store summary stats
    avg_throughputs[$delay]=$avg_tput
    max_throughputs[$delay]=$max_tput
    min_throughputs[$delay]=$min_tput
    avg_strong_medians[$delay]=$avg_strong
    avg_weak_medians[$delay]=$avg_weak

    echo "" | tee -a "$RESULT_FILE"
    echo "  Summary for delay=$delay μs:" | tee -a "$RESULT_FILE"
    echo "    Throughput: min=$min_tput, avg=$avg_tput, max=$max_tput ops/sec" | tee -a "$RESULT_FILE"
    echo "    Latency: strong=${avg_strong}ms, weak=${avg_weak}ms (median avg)" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"

    # Clean up temp config
    rm -f "$CONFIG_TEMP"
done

# Final summary
echo "========================================" | tee -a "$RESULT_FILE"
echo "Phase 32.4 Quick Test Summary" | tee -a "$RESULT_FILE"
echo "========================================" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
printf "%-12s %-15s %-15s %-15s %-15s\n" "Delay (μs)" "Avg Tput" "Max Tput" "Strong Lat" "Weak Lat" | tee -a "$RESULT_FILE"
printf "%-12s %-15s %-15s %-15s %-15s\n" "----------" "----------" "----------" "----------" "----------" | tee -a "$RESULT_FILE"

for delay in "${DELAYS[@]}"; do
    printf "%-12s %-15s %-15s %-15s %-15s\n" \
        "$delay" \
        "${avg_throughputs[$delay]}" \
        "${max_throughputs[$delay]}" \
        "${avg_strong_medians[$delay]}ms" \
        "${avg_weak_medians[$delay]}ms" | tee -a "$RESULT_FILE"
done

echo "" | tee -a "$RESULT_FILE"

# Find optimal delay (highest average throughput)
max_avg_tput=0
optimal_delay=0
for delay in "${DELAYS[@]}"; do
    if (( $(echo "${avg_throughputs[$delay]} > $max_avg_tput" | bc -l) )); then
        max_avg_tput=${avg_throughputs[$delay]}
        optimal_delay=$delay
    fi
done

echo "========================================" | tee -a "$RESULT_FILE"
echo "Optimal Configuration" | tee -a "$RESULT_FILE"
echo "========================================" | tee -a "$RESULT_FILE"
echo "  Optimal batchDelayUs: $optimal_delay μs" | tee -a "$RESULT_FILE"
echo "  Average throughput: ${avg_throughputs[$optimal_delay]} ops/sec" | tee -a "$RESULT_FILE"
echo "  Peak throughput: ${max_throughputs[$optimal_delay]} ops/sec" | tee -a "$RESULT_FILE"
echo "  Strong latency (median avg): ${avg_strong_medians[$optimal_delay]}ms" | tee -a "$RESULT_FILE"
echo "  Weak latency (median avg): ${avg_weak_medians[$optimal_delay]}ms" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Calculate improvement over baseline (delay=0)
baseline_tput=${avg_throughputs[0]}
if (( $(echo "$baseline_tput > 0" | bc -l) )); then
    improvement=$(echo "scale=2; (($max_avg_tput - $baseline_tput) / $baseline_tput) * 100" | bc)
    echo "  Improvement over baseline: +${improvement}%" | tee -a "$RESULT_FILE"
fi

echo "" | tee -a "$RESULT_FILE"
echo "========================================" | tee -a "$RESULT_FILE"
echo "Quick test complete!" | tee -a "$RESULT_FILE"
echo "Results saved in $RESULT_FILE" | tee -a "$RESULT_FILE"
echo "========================================" | tee -a "$RESULT_FILE"
