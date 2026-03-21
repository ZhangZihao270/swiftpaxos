#!/bin/bash
# Phase 31.10: Comprehensive Validation of 23K Target Achievement
# Extended testing with statistical analysis

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/docs/phase-31-profiles"
ITERATIONS=10  # Extended validation
REQUESTS=10000  # 10K ops per client

mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULT_FILE="$OUTPUT_DIR/validation-23k-$TIMESTAMP.txt"

echo "==================================================================" | tee "$RESULT_FILE"
echo "   Phase 31.10: 23K Target Validation" | tee -a "$RESULT_FILE"
echo "==================================================================" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Configuration:" | tee -a "$RESULT_FILE"
echo "  - Protocol: CURP-HO (curpho)" | tee -a "$RESULT_FILE"
echo "  - Pendings: 15" | tee -a "$RESULT_FILE"
echo "  - MaxDescRoutines: 500" | tee -a "$RESULT_FILE"
echo "  - BatchDelayUs: 150" | tee -a "$RESULT_FILE"
echo "  - Clients: 2 × 2 threads = 4 streams" | tee -a "$RESULT_FILE"
echo "  - Requests per run: $REQUESTS per client ($(($REQUESTS * 2)) total)" | tee -a "$RESULT_FILE"
echo "  - Iterations: $ITERATIONS" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Target: 23,000 ops/sec with weak median < 2.0ms" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Starting validation at $(date)" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

cd "$PROJECT_ROOT"

# Verify configuration
echo "Verifying configuration..." | tee -a "$RESULT_FILE"
if ! grep -q "^pendings:    15" multi-client.conf; then
    echo "ERROR: pendings not set to 15" | tee -a "$RESULT_FILE"
    exit 1
fi
if ! grep -q "^batchDelayUs: 150" multi-client.conf; then
    echo "ERROR: batchDelayUs not set to 150" | tee -a "$RESULT_FILE"
    exit 1
fi
if ! grep -q "^maxDescRoutines: 500" multi-client.conf; then
    echo "ERROR: maxDescRoutines not set to 500" | tee -a "$RESULT_FILE"
    exit 1
fi
echo "✓ Configuration verified" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Arrays to collect results
declare -a throughputs
declare -a weak_medians
declare -a weak_p99s
declare -a strong_medians
declare -a strong_p99s

# Run validation iterations
for i in $(seq 1 $ITERATIONS); do
    echo "=== Iteration $i/$ITERATIONS ===" | tee -a "$RESULT_FILE"

    OUTPUT=$(timeout 120 ./run-multi-client.sh -c multi-client.conf 2>&1)

    # Extract metrics
    THROUGHPUT=$(echo "$OUTPUT" | grep -oP 'Aggregate throughput:\s+\K[0-9.]+' | tail -1)
    WEAK_MED=$(echo "$OUTPUT" | grep "Weak Operations:" -A 2 | grep -oP 'Avg median:\s+\K[0-9.]+' | head -1)
    WEAK_P99=$(echo "$OUTPUT" | grep "Weak Operations:" -A 2 | grep -oP 'Max P99:\s+\K[0-9.]+' | head -1)
    STRONG_MED=$(echo "$OUTPUT" | grep "Strong Operations:" -A 2 | grep -oP 'Avg median:\s+\K[0-9.]+' | head -1)
    STRONG_P99=$(echo "$OUTPUT" | grep "Strong Operations:" -A 2 | grep -oP 'Max P99:\s+\K[0-9.]+' | head -1)

    throughputs+=("$THROUGHPUT")
    weak_medians+=("$WEAK_MED")
    weak_p99s+=("$WEAK_P99")
    strong_medians+=("$STRONG_MED")
    strong_p99s+=("$STRONG_P99")

    # Check if iteration meets target
    meets_target=""
    if (( $(echo "$THROUGHPUT >= 23000" | bc -l) )) && (( $(echo "$WEAK_MED < 2.0" | bc -l) )); then
        meets_target=" ✓ TARGET MET"
    else
        meets_target=" ✗ Target not met"
    fi

    echo "  Throughput: $THROUGHPUT ops/sec" | tee -a "$RESULT_FILE"
    echo "  Weak:   median=${WEAK_MED}ms, P99=${WEAK_P99}ms" | tee -a "$RESULT_FILE"
    echo "  Strong: median=${STRONG_MED}ms, P99=${STRONG_P99}ms" | tee -a "$RESULT_FILE"
    echo "  $meets_target" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"

    sleep 2
done

echo "" | tee -a "$RESULT_FILE"
echo "==================================================================" | tee -a "$RESULT_FILE"
echo "   Statistical Analysis" | tee -a "$RESULT_FILE"
echo "==================================================================" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Calculate statistics for throughput
sum=0
min=999999
max=0
for val in "${throughputs[@]}"; do
    sum=$(echo "$sum + $val" | bc -l)
    if (( $(echo "$val < $min" | bc -l) )); then min=$val; fi
    if (( $(echo "$val > $max" | bc -l) )); then max=$val; fi
done
avg=$(echo "scale=2; $sum / $ITERATIONS" | bc -l)

# Calculate standard deviation
sum_sq_diff=0
for val in "${throughputs[@]}"; do
    diff=$(echo "$val - $avg" | bc -l)
    sq_diff=$(echo "$diff * $diff" | bc -l)
    sum_sq_diff=$(echo "$sum_sq_diff + $sq_diff" | bc -l)
done
variance=$(echo "scale=2; $sum_sq_diff / $ITERATIONS" | bc -l)
stddev=$(echo "scale=2; sqrt($variance)" | bc -l)
cv=$(echo "scale=2; ($stddev / $avg) * 100" | bc -l)

echo "Throughput Statistics:" | tee -a "$RESULT_FILE"
echo "  Min:    $min ops/sec" | tee -a "$RESULT_FILE"
echo "  Max:    $max ops/sec" | tee -a "$RESULT_FILE"
echo "  Avg:    $avg ops/sec" | tee -a "$RESULT_FILE"
echo "  StdDev: $stddev ops/sec" | tee -a "$RESULT_FILE"
echo "  CV:     ${cv}%" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Calculate weak median statistics
weak_sum=0
weak_min=999
weak_max=0
for val in "${weak_medians[@]}"; do
    weak_sum=$(echo "$weak_sum + $val" | bc -l)
    if (( $(echo "$val < $weak_min" | bc -l) )); then weak_min=$val; fi
    if (( $(echo "$val > $weak_max" | bc -l) )); then weak_max=$val; fi
done
weak_avg=$(echo "scale=3; $weak_sum / $ITERATIONS" | bc -l)

echo "Weak Median Latency:" | tee -a "$RESULT_FILE"
echo "  Min: ${weak_min}ms" | tee -a "$RESULT_FILE"
echo "  Max: ${weak_max}ms" | tee -a "$RESULT_FILE"
echo "  Avg: ${weak_avg}ms" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Count successful runs
success_count=0
for i in "${!throughputs[@]}"; do
    if (( $(echo "${throughputs[$i]} >= 23000" | bc -l) )) && (( $(echo "${weak_medians[$i]} < 2.0" | bc -l) )); then
        success_count=$((success_count + 1))
    fi
done

echo "==================================================================" | tee -a "$RESULT_FILE"
echo "   Validation Results" | tee -a "$RESULT_FILE"
echo "==================================================================" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Runs meeting target: $success_count / $ITERATIONS ($(echo "scale=1; $success_count * 100 / $ITERATIONS" | bc -l)%)" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Overall assessment
if [ "$success_count" -ge 9 ]; then
    echo "✓✓✓ VALIDATION PASSED: 90%+ runs meet target" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"
    echo "Phase 31 target of 23K ops/sec with weak latency < 2ms is ACHIEVED!" | tee -a "$RESULT_FILE"
elif [ "$success_count" -ge 7 ]; then
    echo "✓✓ VALIDATION MOSTLY PASSED: 70%+ runs meet target" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"
    echo "Target achieved in majority of runs, but some variance present." | tee -a "$RESULT_FILE"
elif (( $(echo "$avg >= 23000" | bc -l) )); then
    echo "✓ VALIDATION PARTIALLY PASSED: Average meets throughput target" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"
    echo "Average throughput meets target but individual runs show variance." | tee -a "$RESULT_FILE"
else
    echo "✗ VALIDATION FAILED: Target not consistently achieved" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"
    echo "Gap to target: $(echo "23000 - $avg" | bc -l) ops/sec" | tee -a "$RESULT_FILE"
fi

echo "" | tee -a "$RESULT_FILE"
echo "Validation completed at $(date)" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Results saved to: $RESULT_FILE" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Next steps:" | tee -a "$RESULT_FILE"
echo "  1. Review detailed statistics above" | tee -a "$RESULT_FILE"
echo "  2. Document final configuration in docs/phase-31-final-config.md" | tee -a "$RESULT_FILE"
echo "  3. Create Phase 31 summary document" | tee -a "$RESULT_FILE"
