#!/bin/bash
# Phase 32.1: CURP-HT Baseline Measurement
# Measure baseline performance before adding network batching

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/docs"
CONFIG="$PROJECT_ROOT/curpht-baseline.conf"
ITERATIONS="${1:-5}"

mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULT_FILE="$OUTPUT_DIR/phase-32-baseline-$TIMESTAMP.txt"

echo "========================================" | tee "$RESULT_FILE"
echo "Phase 32.1: CURP-HT Baseline Measurement" | tee -a "$RESULT_FILE"
echo "========================================" | tee -a "$RESULT_FILE"
echo "Configuration:" | tee -a "$RESULT_FILE"
echo "  - Protocol: curpht" | tee -a "$RESULT_FILE"
echo "  - MaxDescRoutines: 200" | tee -a "$RESULT_FILE"
echo "  - Pendings: 20" | tee -a "$RESULT_FILE"
echo "  - BatchDelayUs: N/A (not implemented yet)" | tee -a "$RESULT_FILE"
echo "  - Iterations: $ITERATIONS" | tee -a "$RESULT_FILE"
echo "  - Timestamp: $TIMESTAMP" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Arrays to store results
declare -a throughputs
declare -a strong_medians
declare -a strong_p99s
declare -a weak_medians
declare -a weak_p99s

echo "Running $ITERATIONS iterations..." | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

for i in $(seq 1 $ITERATIONS); do
    echo "========================================" | tee -a "$RESULT_FILE"
    echo "Iteration $i/$ITERATIONS" | tee -a "$RESULT_FILE"
    echo "========================================" | tee -a "$RESULT_FILE"

    # Run benchmark
    cd "$PROJECT_ROOT"
    OUTPUT=$(timeout 180 ./run-multi-client.sh -c "$CONFIG" 2>&1 || true)

    # Extract metrics (from run-multi-client.sh merged output)
    THROUGHPUT=$(echo "$OUTPUT" | grep -oP 'Aggregate throughput:\s+\K[0-9.]+' | tail -1)
    STRONG_MEDIAN=$(echo "$OUTPUT" | grep "Strong Operations:" -A 2 | grep -oP 'Avg median:\s+\K[0-9.]+' | head -1)
    STRONG_P99=$(echo "$OUTPUT" | grep "Strong Operations:" -A 2 | grep -oP 'Max P99:\s+\K[0-9.]+' | head -1)
    WEAK_MEDIAN=$(echo "$OUTPUT" | grep "Weak Operations:" -A 2 | grep -oP 'Avg median:\s+\K[0-9.]+' | head -1)
    WEAK_P99=$(echo "$OUTPUT" | grep "Weak Operations:" -A 2 | grep -oP 'Max P99:\s+\K[0-9.]+' | head -1)

    # Store results
    throughputs+=("$THROUGHPUT")
    strong_medians+=("$STRONG_MEDIAN")
    strong_p99s+=("$STRONG_P99")
    weak_medians+=("$WEAK_MEDIAN")
    weak_p99s+=("$WEAK_P99")

    # Display iteration results
    echo "  Throughput: $THROUGHPUT ops/sec" | tee -a "$RESULT_FILE"
    echo "  Strong latency: median ${STRONG_MEDIAN}ms, P99 ${STRONG_P99}ms" | tee -a "$RESULT_FILE"
    echo "  Weak latency: median ${WEAK_MEDIAN}ms, P99 ${WEAK_P99}ms" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"

    # Brief sleep between iterations
    sleep 2
done

# Calculate statistics
echo "========================================" | tee -a "$RESULT_FILE"
echo "Summary Statistics ($ITERATIONS iterations)" | tee -a "$RESULT_FILE"
echo "========================================" | tee -a "$RESULT_FILE"

# Calculate min/max/avg for throughput
min_tput=${throughputs[0]}
max_tput=${throughputs[0]}
sum_tput=0
for tput in "${throughputs[@]}"; do
    sum_tput=$(echo "$sum_tput + $tput" | bc)
    if (( $(echo "$tput < $min_tput" | bc -l) )); then min_tput=$tput; fi
    if (( $(echo "$tput > $max_tput" | bc -l) )); then max_tput=$tput; fi
done
avg_tput=$(echo "scale=2; $sum_tput / $ITERATIONS" | bc)

echo "Throughput:" | tee -a "$RESULT_FILE"
echo "  Min: $min_tput ops/sec" | tee -a "$RESULT_FILE"
echo "  Max: $max_tput ops/sec" | tee -a "$RESULT_FILE"
echo "  Avg: $avg_tput ops/sec" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Calculate avg for latencies
sum_strong=0
sum_weak=0
for lat in "${strong_medians[@]}"; do
    sum_strong=$(echo "$sum_strong + $lat" | bc)
done
for lat in "${weak_medians[@]}"; do
    sum_weak=$(echo "$sum_weak + $lat" | bc)
done
avg_strong=$(echo "scale=2; $sum_strong / $ITERATIONS" | bc)
avg_weak=$(echo "scale=2; $sum_weak / $ITERATIONS" | bc)

echo "Latency (median, avg):" | tee -a "$RESULT_FILE"
echo "  Strong: ${avg_strong}ms" | tee -a "$RESULT_FILE"
echo "  Weak: ${avg_weak}ms" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

echo "========================================" | tee -a "$RESULT_FILE"
echo "Baseline measurement complete!" | tee -a "$RESULT_FILE"
echo "Results saved in $RESULT_FILE" | tee -a "$RESULT_FILE"
echo "========================================" | tee -a "$RESULT_FILE"
