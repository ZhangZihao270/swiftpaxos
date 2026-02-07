#!/bin/bash

# Validation script for 20K throughput target
# Runs multiple benchmark iterations to verify consistent performance

CONFIG_FILE="multi-client.conf"
RESULTS_DIR="results/20k-validation"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
ITERATIONS=5

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "============================================"
echo "  20K Throughput Target Validation"
echo "============================================"
echo "Config: $CONFIG_FILE"
echo "Iterations: $ITERATIONS"
echo "Results: $RESULTS_DIR"
echo ""

# Display current configuration
echo "Current Optimization Settings:"
grep "^protocol:" "$CONFIG_FILE"
grep "^maxDescRoutines:" "$CONFIG_FILE"
grep "^pendings:" "$CONFIG_FILE"
echo ""

# Create results directory
mkdir -p "$RESULTS_DIR"

# Summary file
SUMMARY_FILE="$RESULTS_DIR/validation-summary-$TIMESTAMP.txt"
echo "20K Target Validation Results - $TIMESTAMP" > "$SUMMARY_FILE"
echo "==========================================" >> "$SUMMARY_FILE"
echo "" >> "$SUMMARY_FILE"
echo "Configuration:" >> "$SUMMARY_FILE"
grep "^protocol:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
grep "^maxDescRoutines:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
grep "^pendings:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
grep "^reqs:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
grep "^clientThreads:" "$CONFIG_FILE" >> "$SUMMARY_FILE"
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
    echo "  Throughput: $THROUGHPUT"
    echo "  Duration: $DURATION"
    echo "  Strong - Median: $STRONG_MEDIAN, P99: $STRONG_P99"
    echo "  Weak   - Median: $WEAK_MEDIAN, P99: $WEAK_P99"
    echo ""

    # Append to summary
    echo "Iteration $i:" >> "$SUMMARY_FILE"
    echo "  Throughput: $THROUGHPUT" >> "$SUMMARY_FILE"
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
echo "===================" >> "$SUMMARY_FILE"

if [ ${#THROUGHPUTS[@]} -gt 0 ]; then
    # Calculate min, max, avg for throughput
    MIN_TP=$(printf '%s\n' "${THROUGHPUTS[@]}" | sort -n | head -1)
    MAX_TP=$(printf '%s\n' "${THROUGHPUTS[@]}" | sort -n | tail -1)

    # Calculate average (using awk for floating point)
    AVG_TP=$(printf '%s\n' "${THROUGHPUTS[@]}" | awk '{sum+=$1; count++} END {printf "%.2f", sum/count}')

    echo "Throughput:" >> "$SUMMARY_FILE"
    echo "  Min: $MIN_TP ops/sec" >> "$SUMMARY_FILE"
    echo "  Max: $MAX_TP ops/sec" >> "$SUMMARY_FILE"
    echo "  Avg: $AVG_TP ops/sec" >> "$SUMMARY_FILE"
    echo "" >> "$SUMMARY_FILE"

    # Check if target achieved
    TARGET=20000
    if (( $(echo "$AVG_TP >= $TARGET" | bc -l) )); then
        echo "✅ 20K Target: ACHIEVED (avg: $AVG_TP ops/sec)" >> "$SUMMARY_FILE"
        echo -e "${GREEN}✅ 20K Target: ACHIEVED (avg: $AVG_TP ops/sec)${NC}"
    else
        echo "⚠️  20K Target: NOT ACHIEVED (avg: $AVG_TP ops/sec, need: 20000)" >> "$SUMMARY_FILE"
        echo -e "${YELLOW}⚠️  20K Target: NOT ACHIEVED (avg: $AVG_TP ops/sec)${NC}"
    fi
fi

# Display summary
echo ""
echo "============================================"
echo "  Validation Summary"
echo "============================================"
cat "$SUMMARY_FILE"

echo ""
echo "Full results saved to: $RESULTS_DIR"
echo "Summary: $SUMMARY_FILE"
echo "============================================"
