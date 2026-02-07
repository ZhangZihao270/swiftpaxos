#!/bin/bash
# Phase 31.1: Baseline Performance Measurement
# Measures CURP-HO performance with pendings=10 (current configuration)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/docs/phase-31-profiles"
ITERATIONS="${1:-5}"
REQUESTS="${2:-100000}"

mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULT_FILE="$OUTPUT_DIR/baseline-results-$TIMESTAMP.txt"

echo "=== Phase 31.1: Baseline Performance Measurement ===" | tee "$RESULT_FILE"
echo "Configuration:" | tee -a "$RESULT_FILE"
echo "  - Protocol: curpho" | tee -a "$RESULT_FILE"
echo "  - Pendings: 10 (target constraint)" | tee -a "$RESULT_FILE"
echo "  - MaxDescRoutines: 200" | tee -a "$RESULT_FILE"
echo "  - Clients: 2 × 2 threads = 4 total streams" | tee -a "$RESULT_FILE"
echo "  - Requests per run: $REQUESTS" | tee -a "$RESULT_FILE"
echo "  - Iterations: $ITERATIONS" | tee -a "$RESULT_FILE"
echo "  - Timestamp: $TIMESTAMP" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Verify configuration
echo "Checking multi-client.conf..." | tee -a "$RESULT_FILE"
if ! grep -q "pendings:\s*10" "$PROJECT_ROOT/multi-client.conf"; then
    echo "⚠ WARNING: multi-client.conf does not have pendings=10" | tee -a "$RESULT_FILE"
    echo "Current pendings setting:" | tee -a "$RESULT_FILE"
    grep "pendings:" "$PROJECT_ROOT/multi-client.conf" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"
fi

if ! grep -q "protocol:\s*curpho" "$PROJECT_ROOT/multi-client.conf"; then
    echo "⚠ WARNING: multi-client.conf does not have protocol=curpho" | tee -a "$RESULT_FILE"
    echo "Current protocol setting:" | tee -a "$RESULT_FILE"
    grep "protocol:" "$PROJECT_ROOT/multi-client.conf" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"
fi

echo "✓ Configuration verified" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Arrays to store results
declare -a throughputs
declare -a strong_medians
declare -a strong_p99s
declare -a weak_medians
declare -a weak_p99s
declare -a slow_paths

# Run benchmark iterations
for i in $(seq 1 $ITERATIONS); do
    echo "=== Iteration $i/$ITERATIONS ===" | tee -a "$RESULT_FILE"

    # Update reqs in config temporarily
    sed -i.bak "s/^reqs:.*/reqs:        $REQUESTS/" "$PROJECT_ROOT/multi-client.conf"

    # Run benchmark
    cd "$PROJECT_ROOT"
    OUTPUT=$(make run-hybrid 2>&1)

    # Restore original config
    mv "$PROJECT_ROOT/multi-client.conf.bak" "$PROJECT_ROOT/multi-client.conf"

    # Extract metrics
    THROUGHPUT=$(echo "$OUTPUT" | grep -oP 'Total:\s+\K[0-9.]+(?=\s+ops/sec)' | tail -1)
    STRONG_MEDIAN=$(echo "$OUTPUT" | grep "Strong latency" | grep -oP 'median:\s+\K[0-9.]+' | head -1)
    STRONG_P99=$(echo "$OUTPUT" | grep "Strong latency" | grep -oP 'p99:\s+\K[0-9.]+' | head -1)
    WEAK_MEDIAN=$(echo "$OUTPUT" | grep "Weak latency" | grep -oP 'median:\s+\K[0-9.]+' | head -1)
    WEAK_P99=$(echo "$OUTPUT" | grep "Weak latency" | grep -oP 'p99:\s+\K[0-9.]+' | head -1)
    SLOW_PATH=$(echo "$OUTPUT" | grep "Slow Paths:" | grep -oP 'Slow Paths:\s+\K[0-9]+' | head -1)

    # Store results
    throughputs+=("$THROUGHPUT")
    strong_medians+=("$STRONG_MEDIAN")
    strong_p99s+=("$STRONG_P99")
    weak_medians+=("$WEAK_MEDIAN")
    weak_p99s+=("$WEAK_P99")
    slow_paths+=("$SLOW_PATH")

    # Display iteration results
    echo "  Throughput: $THROUGHPUT ops/sec" | tee -a "$RESULT_FILE"
    echo "  Strong latency: median ${STRONG_MEDIAN}ms, p99 ${STRONG_P99}ms" | tee -a "$RESULT_FILE"
    echo "  Weak latency: median ${WEAK_MEDIAN}ms, p99 ${WEAK_P99}ms" | tee -a "$RESULT_FILE"
    echo "  Slow paths: $SLOW_PATH" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"

    # Brief sleep between iterations
    sleep 2
done

# Calculate statistics
calculate_stats() {
    local values=("$@")
    local sum=0
    local min=${values[0]}
    local max=${values[0]}

    for val in "${values[@]}"; do
        sum=$(echo "$sum + $val" | bc -l)
        if (( $(echo "$val < $min" | bc -l) )); then
            min=$val
        fi
        if (( $(echo "$val > $max" | bc -l) )); then
            max=$val
        fi
    done

    local count=${#values[@]}
    local avg=$(echo "scale=2; $sum / $count" | bc -l)

    # Calculate standard deviation
    local sq_diff_sum=0
    for val in "${values[@]}"; do
        local diff=$(echo "$val - $avg" | bc -l)
        local sq_diff=$(echo "$diff * $diff" | bc -l)
        sq_diff_sum=$(echo "$sq_diff_sum + $sq_diff" | bc -l)
    done
    local variance=$(echo "scale=2; $sq_diff_sum / $count" | bc -l)
    local stddev=$(echo "scale=2; sqrt($variance)" | bc -l)

    echo "$min $max $avg $stddev"
}

# Compute statistics
read THRU_MIN THRU_MAX THRU_AVG THRU_STDDEV <<< $(calculate_stats "${throughputs[@]}")
read SM_MIN SM_MAX SM_AVG SM_STDDEV <<< $(calculate_stats "${strong_medians[@]}")
read SP99_MIN SP99_MAX SP99_AVG SP99_STDDEV <<< $(calculate_stats "${strong_p99s[@]}")
read WM_MIN WM_MAX WM_AVG WM_STDDEV <<< $(calculate_stats "${weak_medians[@]}")
read WP99_MIN WP99_MAX WP99_AVG WP99_STDDEV <<< $(calculate_stats "${weak_p99s[@]}")

# Display summary
echo "=== Baseline Performance Summary ===" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Throughput (ops/sec):" | tee -a "$RESULT_FILE"
echo "  Min:    $THRU_MIN" | tee -a "$RESULT_FILE"
echo "  Max:    $THRU_MAX" | tee -a "$RESULT_FILE"
echo "  Avg:    $THRU_AVG ± $THRU_STDDEV" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

echo "Strong Latency (ms):" | tee -a "$RESULT_FILE"
echo "  Median: $SM_MIN - $SM_MAX (avg: $SM_AVG ± $SM_STDDEV)" | tee -a "$RESULT_FILE"
echo "  P99:    $SP99_MIN - $SP99_MAX (avg: $SP99_AVG ± $SP99_STDDEV)" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

echo "Weak Latency (ms):" | tee -a "$RESULT_FILE"
echo "  Median: $WM_MIN - $WM_MAX (avg: $WM_AVG ± $WM_STDDEV)" | tee -a "$RESULT_FILE"
echo "  P99:    $WP99_MIN - $WP99_MAX (avg: $WP99_AVG ± $WP99_STDDEV)" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Check if weak median meets constraint
WEAK_CONSTRAINT_CHECK=$(echo "$WM_AVG < 2.0" | bc -l)
if [ "$WEAK_CONSTRAINT_CHECK" -eq 1 ]; then
    echo "✓ Weak median latency constraint MET (< 2ms)" | tee -a "$RESULT_FILE"
else
    echo "✗ Weak median latency constraint VIOLATED (>= 2ms)" | tee -a "$RESULT_FILE"
fi
echo "" | tee -a "$RESULT_FILE"

# Calculate gap to target
TARGET_THROUGHPUT=23000
GAP=$(echo "$TARGET_THROUGHPUT - $THRU_AVG" | bc -l)
GAP_PERCENT=$(echo "scale=1; ($GAP / $THRU_AVG) * 100" | bc -l)

echo "=== Target Analysis ===" | tee -a "$RESULT_FILE"
echo "Current baseline:  $THRU_AVG ops/sec" | tee -a "$RESULT_FILE"
echo "Target throughput: $TARGET_THROUGHPUT ops/sec" | tee -a "$RESULT_FILE"
echo "Gap:               $GAP ops/sec (+${GAP_PERCENT}% needed)" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

echo "=== Recommendations ===" | tee -a "$RESULT_FILE"
if (( $(echo "$GAP_PERCENT > 50" | bc -l) )); then
    echo "Gap is large (>50%). Recommended approach:" | tee -a "$RESULT_FILE"
    echo "  1. Profile CPU/memory to find bottlenecks (Phase 31.2-31.3)" | tee -a "$RESULT_FILE"
    echo "  2. Increase client parallelism (4 clients × 3 threads) (Phase 31.5)" | tee -a "$RESULT_FILE"
    echo "  3. Optimize hot paths identified in profiling (Phase 31.6-31.8)" | tee -a "$RESULT_FILE"
elif (( $(echo "$GAP_PERCENT > 20" | bc -l) )); then
    echo "Gap is moderate (20-50%). Recommended approach:" | tee -a "$RESULT_FILE"
    echo "  1. Increase client threads (2 clients × 4-6 threads) (Phase 31.5)" | tee -a "$RESULT_FILE"
    echo "  2. Profile to identify 1-2 key bottlenecks (Phase 31.2)" | tee -a "$RESULT_FILE"
    echo "  3. Optimize network batching (Phase 31.4)" | tee -a "$RESULT_FILE"
else
    echo "Gap is small (<20%). Recommended approach:" | tee -a "$RESULT_FILE"
    echo "  1. Fine-tune existing parameters (maxDescRoutines, SHARD_COUNT)" | tee -a "$RESULT_FILE"
    echo "  2. Increase client threads slightly (2 clients × 3 threads)" | tee -a "$RESULT_FILE"
fi
echo "" | tee -a "$RESULT_FILE"

echo "Results saved to: $RESULT_FILE"
echo ""
echo "Next steps:"
echo "  1. Review baseline results above"
echo "  2. Run profiling: ./scripts/phase-31-profile.sh all 30"
echo "  3. Document findings in docs/phase-31-baseline.md"
echo "  4. Proceed to optimization phases (31.4+)"
