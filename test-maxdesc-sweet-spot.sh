#!/bin/bash

# Test script to find optimal MaxDescRoutines value with current optimizations
# Tests values in the sweet spot range: 100, 200, 500, 1000, 2000

CONFIG_FILE="multi-client.conf"
RESULTS_DIR="results/maxdesc-sweet-spot"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# MaxDescRoutines values to test
MAXDESC_VALUES=(100 200 500 1000 2000)

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "============================================"
echo "  MaxDescRoutines Sweet Spot Test"
echo "============================================"
echo "Config: $CONFIG_FILE"
echo "Testing MaxDescRoutines values: ${MAXDESC_VALUES[@]}"
echo "Results will be saved to: $RESULTS_DIR"
echo ""

# Create results directory
mkdir -p "$RESULTS_DIR"

# Save original maxDescRoutines value
ORIGINAL_MAXDESC=$(grep "^maxDescRoutines:" "$CONFIG_FILE" | awk '{print $2}')
echo "Original maxDescRoutines value: $ORIGINAL_MAXDESC"
echo ""

# Summary file
SUMMARY_FILE="$RESULTS_DIR/summary-$TIMESTAMP.txt"
echo "MaxDescRoutines Sweet Spot Test Results - $TIMESTAMP" > "$SUMMARY_FILE"
echo "==========================================" >> "$SUMMARY_FILE"
echo "Configuration: pendings=20, Protocol=CURP-HO" >> "$SUMMARY_FILE"
echo "" >> "$SUMMARY_FILE"

# Test each maxDescRoutines value
for MAXDESC in "${MAXDESC_VALUES[@]}"; do
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}Testing maxDescRoutines = $MAXDESC${NC}"
    echo -e "${BLUE}========================================${NC}"

    # Update config file
    sed -i "s/^maxDescRoutines:.*$/maxDescRoutines: $MAXDESC   \/\/ Sweet spot test/" "$CONFIG_FILE"

    # Verify update
    CURRENT_MAXDESC=$(grep "^maxDescRoutines:" "$CONFIG_FILE" | awk '{print $2}')
    if [ "$CURRENT_MAXDESC" != "$MAXDESC" ]; then
        echo -e "${YELLOW}Warning: Failed to update maxDescRoutines to $MAXDESC (got $CURRENT_MAXDESC)${NC}"
        continue
    fi

    echo "Updated maxDescRoutines to: $MAXDESC"
    echo ""

    # Run benchmark
    OUTPUT_FILE="$RESULTS_DIR/maxdesc-$MAXDESC-$TIMESTAMP.log"
    echo "Running benchmark... (output: $OUTPUT_FILE)"

    timeout 180 ./run-multi-client.sh -c "$CONFIG_FILE" > "$OUTPUT_FILE" 2>&1
    EXIT_CODE=$?

    if [ $EXIT_CODE -eq 124 ]; then
        echo -e "${YELLOW}Benchmark timed out after 180s${NC}"
        echo "maxDescRoutines=$MAXDESC: TIMEOUT" >> "$SUMMARY_FILE"
        echo "" >> "$SUMMARY_FILE"
        continue
    elif [ $EXIT_CODE -ne 0 ]; then
        echo -e "${YELLOW}Benchmark failed with exit code $EXIT_CODE${NC}"
        echo "maxDescRoutines=$MAXDESC: FAILED (exit $EXIT_CODE)" >> "$SUMMARY_FILE"
        echo "" >> "$SUMMARY_FILE"
        continue
    fi

    # Extract results
    THROUGHPUT=$(grep "Aggregate throughput:" "$OUTPUT_FILE" | awk '{print $3}')
    DURATION=$(grep "Max duration:" "$OUTPUT_FILE" | awk '{print $3}')
    STRONG_MEDIAN=$(grep "Avg median:" "$OUTPUT_FILE" | head -1 | awk '{print $3}')
    STRONG_P99=$(grep "Max P99:" "$OUTPUT_FILE" | head -1 | awk '{print $3}')
    WEAK_MEDIAN=$(grep "Avg median:" "$OUTPUT_FILE" | tail -1 | awk '{print $3}')
    WEAK_P99=$(grep "Max P99:" "$OUTPUT_FILE" | tail -1 | awk '{print $3}')

    echo -e "${GREEN}Results for maxDescRoutines=$MAXDESC:${NC}"
    echo "  Throughput: $THROUGHPUT"
    echo "  Duration: $DURATION"
    echo "  Strong - Median: $STRONG_MEDIAN, P99: $STRONG_P99"
    echo "  Weak   - Median: $WEAK_MEDIAN, P99: $WEAK_P99"
    echo ""

    # Append to summary
    echo "maxDescRoutines=$MAXDESC:" >> "$SUMMARY_FILE"
    echo "  Throughput: $THROUGHPUT" >> "$SUMMARY_FILE"
    echo "  Duration: $DURATION" >> "$SUMMARY_FILE"
    echo "  Strong - Median: $STRONG_MEDIAN, P99: $STRONG_P99" >> "$SUMMARY_FILE"
    echo "  Weak   - Median: $WEAK_MEDIAN, P99: $WEAK_P99" >> "$SUMMARY_FILE"
    echo "" >> "$SUMMARY_FILE"

    # Wait before next test
    sleep 5
done

# Restore original value
echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Restoring original configuration${NC}"
echo -e "${BLUE}========================================${NC}"
sed -i "s/^maxDescRoutines:.*$/maxDescRoutines: $ORIGINAL_MAXDESC   \/\/ Test: revert to old value/" "$CONFIG_FILE"
echo "Restored maxDescRoutines to: $ORIGINAL_MAXDESC"

# Display summary
echo ""
echo "============================================"
echo "  Summary"
echo "============================================"
cat "$SUMMARY_FILE"

echo ""
echo "Full results saved to: $RESULTS_DIR"
echo "Summary: $SUMMARY_FILE"
echo "============================================"
