#!/bin/bash

# Benchmark CURP-HT with Phase 19 Optimizations
# Tests performance of CURP-HT with all optimizations from Phase 19.1-19.4

CONFIG_FILE="curpht-optimized.conf"
RESULTS_DIR="results/phase-19.5-curpht-optimized"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
ITERATIONS=3

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "============================================"
echo "  Phase 19.5: CURP-HT Optimization Benchmark"
echo "============================================"
echo "Config: $CONFIG_FILE"
echo "Iterations: $ITERATIONS"
echo "Results: $RESULTS_DIR"
echo ""

# Display configuration
echo "Optimization Settings:"
grep "^protocol:" "$CONFIG_FILE"
grep "^maxDescRoutines:" "$CONFIG_FILE"
grep "^pendings:" "$CONFIG_FILE"
grep "^clientThreads:" "$CONFIG_FILE"
grep "^reqs:" "$CONFIG_FILE"
echo ""

# Create results directory
mkdir -p "$RESULTS_DIR"

# Summary file
SUMMARY_FILE="$RESULTS_DIR/benchmark-summary-$TIMESTAMP.txt"
echo "CURP-HT Optimization Benchmark - $TIMESTAMP" > "$SUMMARY_FILE"
echo "===========================================" >> "$SUMMARY_FILE"
echo "" >> "$SUMMARY_FILE"
echo "Configuration:" >> "$SUMMARY_FILE"
grep "^protocol:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
grep "^maxDescRoutines:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
grep "^pendings:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
grep "^reqs:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
grep "^clientThreads:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
grep "^weakRatio:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
echo "" >> "$SUMMARY_FILE"
echo "Optimizations Applied:" >> "$SUMMARY_FILE"
echo "  - Phase 19.1: String caching (sync.Map)" >> "$SUMMARY_FILE"
echo "  - Phase 19.2: Pre-allocated closed channel" >> "$SUMMARY_FILE"
echo "  - Phase 19.3: Faster spin-wait (10μs polling)" >> "$SUMMARY_FILE"
echo "  - Phase 19.4: Config optimizations (maxDescRoutines=200, pendings=20)" >> "$SUMMARY_FILE"
echo "" >> "$SUMMARY_FILE"
echo "Benchmark Results:" >> "$SUMMARY_FILE"
echo "==================" >> "$SUMMARY_FILE"

# Arrays to collect statistics
declare -a THROUGHPUTS
declare -a DURATIONS
declare -a STRONG_MEDIANS
declare -a STRONG_P99S
declare -a WEAK_MEDIANS
declare -a WEAK_P99S

# Run benchmark iterations
for i in $(seq 1 $ITERATIONS); do
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}Iteration $i of $ITERATIONS${NC}"
    echo -e "${BLUE}========================================${NC}"

    OUTPUT_FILE="$RESULTS_DIR/iteration-$i-$TIMESTAMP.log"
    echo "Running benchmark... (output: $OUTPUT_FILE)"

    timeout 180 ./run-multi-client.sh -c "$CONFIG_FILE" > "$OUTPUT_FILE" 2>&1
    EXIT_CODE=$?

    if [ $EXIT_CODE -eq 124 ]; then
        echo -e "${YELLOW}Benchmark timed out after 180s${NC}"
        echo "Iteration $i: TIMEOUT" >> "$SUMMARY_FILE"
        continue
    elif [ $EXIT_CODE -ne 0 ]; then
        echo -e "${YELLOW}Benchmark failed with exit code $EXIT_CODE${NC}"
        echo "Iteration $i: FAILED (exit $EXIT_CODE)" >> "$SUMMARY_FILE"
        continue
    fi

    # Extract results
    THROUGHPUT=$(grep "Aggregate throughput:" "$OUTPUT_FILE" | awk '{print $3}')
    DURATION=$(grep "Max duration:" "$OUTPUT_FILE" | awk '{print $3}')
    STRONG_MEDIAN=$(grep "Avg median:" "$OUTPUT_FILE" | head -1 | awk '{print $3}')
    STRONG_P99=$(grep "Max P99:" "$OUTPUT_FILE" | head -1 | awk '{print $3}')
    WEAK_MEDIAN=$(grep "Avg median:" "$OUTPUT_FILE" | tail -1 | awk '{print $3}')
    WEAK_P99=$(grep "Max P99:" "$OUTPUT_FILE" | tail -1 | awk '{print $3}')

    # Store in arrays
    THROUGHPUTS+=("$THROUGHPUT")
    DURATIONS+=("$DURATION")
    STRONG_MEDIANS+=("$STRONG_MEDIAN")
    STRONG_P99S+=("$STRONG_P99")
    WEAK_MEDIANS+=("$WEAK_MEDIAN")
    WEAK_P99S+=("$WEAK_P99")

    echo -e "${GREEN}Results for iteration $i:${NC}"
    echo "  Throughput: $THROUGHPUT ops/sec"
    echo "  Duration: $DURATION"
    echo "  Strong - Median: $STRONG_MEDIAN, P99: $STRONG_P99"
    echo "  Weak   - Median: $WEAK_MEDIAN, P99: $WEAK_P99"
    echo ""

    # Append to summary
    echo "Iteration $i:" >> "$SUMMARY_FILE"
    echo "  Throughput: $THROUGHPUT ops/sec" >> "$SUMMARY_FILE"
    echo "  Duration: $DURATION" >> "$SUMMARY_FILE"
    echo "  Strong - Median: $STRONG_MEDIAN, P99: $STRONG_P99" >> "$SUMMARY_FILE"
    echo "  Weak   - Median: $WEAK_MEDIAN, P99: $WEAK_P99" >> "$SUMMARY_FILE"
    echo "" >> "$SUMMARY_FILE"

    # Wait between runs
    sleep 3
done

# Calculate statistics
echo "" >> "$SUMMARY_FILE"
echo "Statistical Summary:" >> "$SUMMARY_FILE"
echo "====================" >> "$SUMMARY_FILE"

if [ ${#THROUGHPUTS[@]} -gt 0 ]; then
    # Calculate min, max, avg for throughput
    MIN_TP=$(printf '%s\n' "${THROUGHPUTS[@]}" | sort -n | head -1)
    MAX_TP=$(printf '%s\n' "${THROUGHPUTS[@]}" | sort -n | tail -1)

    # Calculate average (using awk for floating point)
    AVG_TP=$(printf '%s\n' "${THROUGHPUTS[@]}" | awk '{sum+=$1; count++} END {printf "%.2f", sum/count}')

    # Calculate average latencies
    AVG_STRONG_MED=$(printf '%s\n' "${STRONG_MEDIANS[@]}" | awk '{sum+=$1; count++} END {printf "%.2f", sum/count}')
    AVG_STRONG_P99=$(printf '%s\n' "${STRONG_P99S[@]}" | awk '{sum+=$1; count++} END {printf "%.2f", sum/count}')
    AVG_WEAK_MED=$(printf '%s\n' "${WEAK_MEDIANS[@]}" | awk '{sum+=$1; count++} END {printf "%.2f", sum/count}')
    AVG_WEAK_P99=$(printf '%s\n' "${WEAK_P99S[@]}" | awk '{sum+=$1; count++} END {printf "%.2f", sum/count}')

    echo "Throughput:" >> "$SUMMARY_FILE"
    echo "  Min: $MIN_TP ops/sec" >> "$SUMMARY_FILE"
    echo "  Max: $MAX_TP ops/sec" >> "$SUMMARY_FILE"
    echo "  Avg: $AVG_TP ops/sec" >> "$SUMMARY_FILE"
    echo "" >> "$SUMMARY_FILE"

    echo "Latency (Average across iterations):" >> "$SUMMARY_FILE"
    echo "  Strong - Median: ${AVG_STRONG_MED}ms, P99: ${AVG_STRONG_P99}ms" >> "$SUMMARY_FILE"
    echo "  Weak   - Median: ${AVG_WEAK_MED}ms, P99: ${AVG_WEAK_P99}ms" >> "$SUMMARY_FILE"
    echo "" >> "$SUMMARY_FILE"

    # Compare to baseline
    BASELINE=26000
    IMPROVEMENT=$(echo "scale=1; ($AVG_TP - $BASELINE) / $BASELINE * 100" | bc -l)

    echo "Comparison to Baseline:" >> "$SUMMARY_FILE"
    echo "  Baseline: ${BASELINE} ops/sec (CURP-HT before Phase 19)" >> "$SUMMARY_FILE"
    echo "  Optimized: ${AVG_TP} ops/sec" >> "$SUMMARY_FILE"
    echo "  Improvement: ${IMPROVEMENT}%" >> "$SUMMARY_FILE"
    echo "" >> "$SUMMARY_FILE"

    # Target validation
    TARGET=30000
    if (( $(echo "$AVG_TP >= $TARGET" | bc -l) )); then
        echo "✅ 30K Target: ACHIEVED (avg: $AVG_TP ops/sec)" >> "$SUMMARY_FILE"
        echo -e "${GREEN}✅ 30K Target: ACHIEVED (avg: $AVG_TP ops/sec)${NC}"
    else
        PERCENT=$(echo "scale=1; $AVG_TP / $TARGET * 100" | bc -l)
        echo "⚠️  30K Target: NOT ACHIEVED (avg: $AVG_TP ops/sec, ${PERCENT}% of target)" >> "$SUMMARY_FILE"
        echo -e "${YELLOW}⚠️  30K Target: NOT ACHIEVED (avg: $AVG_TP ops/sec, ${PERCENT}% of target)${NC}"
    fi
fi

# Display summary
echo ""
echo "============================================"
echo "  Benchmark Summary"
echo "============================================"
cat "$SUMMARY_FILE"

echo ""
echo "Full results saved to: $RESULTS_DIR"
echo "Summary: $SUMMARY_FILE"
echo "============================================"
