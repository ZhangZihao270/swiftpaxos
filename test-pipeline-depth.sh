#!/bin/bash

# Test script to find optimal pipeline depth (pendings parameter)
# Tests different values and measures throughput/latency

CONFIG_FILE="multi-client.conf"
RESULTS_DIR="results/pipeline-depth-tuning"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# Pipeline depth values to test
PENDINGS_VALUES=(5 10 15 20 30)

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "============================================"
echo "  Pipeline Depth Optimization Test"
echo "============================================"
echo "Config: $CONFIG_FILE"
echo "Testing pendings values: ${PENDINGS_VALUES[@]}"
echo "Results will be saved to: $RESULTS_DIR"
echo ""

# Create results directory
mkdir -p "$RESULTS_DIR"

# Save original pendings value
ORIGINAL_PENDINGS=$(grep "^pendings:" "$CONFIG_FILE" | awk '{print $2}')
echo "Original pendings value: $ORIGINAL_PENDINGS"
echo ""

# Summary file
SUMMARY_FILE="$RESULTS_DIR/summary-$TIMESTAMP.txt"
echo "Pipeline Depth Tuning Results - $TIMESTAMP" > "$SUMMARY_FILE"
echo "==========================================" >> "$SUMMARY_FILE"
echo "" >> "$SUMMARY_FILE"

# Test each pendings value
for PENDINGS in "${PENDINGS_VALUES[@]}"; do
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}Testing pendings = $PENDINGS${NC}"
    echo -e "${BLUE}========================================${NC}"

    # Update config file
    sed -i "s/^pendings:.*$/pendings:    $PENDINGS       \/\/ Pipeline depth test/" "$CONFIG_FILE"

    # Verify update
    CURRENT_PENDINGS=$(grep "^pendings:" "$CONFIG_FILE" | awk '{print $2}')
    if [ "$CURRENT_PENDINGS" != "$PENDINGS" ]; then
        echo -e "${YELLOW}Warning: Failed to update pendings to $PENDINGS (got $CURRENT_PENDINGS)${NC}"
        continue
    fi

    echo "Updated pendings to: $PENDINGS"
    echo ""

    # Run benchmark
    OUTPUT_FILE="$RESULTS_DIR/pendings-$PENDINGS-$TIMESTAMP.log"
    echo "Running benchmark... (output: $OUTPUT_FILE)"

    timeout 180 ./run-multi-client.sh -c "$CONFIG_FILE" > "$OUTPUT_FILE" 2>&1
    EXIT_CODE=$?

    if [ $EXIT_CODE -eq 124 ]; then
        echo -e "${YELLOW}Benchmark timed out after 180s${NC}"
        echo "pendings=$PENDINGS: TIMEOUT" >> "$SUMMARY_FILE"
        echo "" >> "$SUMMARY_FILE"
        continue
    elif [ $EXIT_CODE -ne 0 ]; then
        echo -e "${YELLOW}Benchmark failed with exit code $EXIT_CODE${NC}"
        echo "pendings=$PENDINGS: FAILED (exit $EXIT_CODE)" >> "$SUMMARY_FILE"
        echo "" >> "$SUMMARY_FILE"
        continue
    fi

    # Extract results
    THROUGHPUT=$(grep "Aggregate throughput:" "$OUTPUT_FILE" | awk '{print $3}')
    STRONG_MEDIAN=$(grep "Avg median:" "$OUTPUT_FILE" | head -1 | awk '{print $3}')
    STRONG_P99=$(grep "Max P99:" "$OUTPUT_FILE" | head -1 | awk '{print $3}')
    WEAK_MEDIAN=$(grep "Avg median:" "$OUTPUT_FILE" | tail -1 | awk '{print $3}')
    WEAK_P99=$(grep "Max P99:" "$OUTPUT_FILE" | tail -1 | awk '{print $3}')

    echo -e "${GREEN}Results for pendings=$PENDINGS:${NC}"
    echo "  Throughput: $THROUGHPUT"
    echo "  Strong - Median: $STRONG_MEDIAN, P99: $STRONG_P99"
    echo "  Weak   - Median: $WEAK_MEDIAN, P99: $WEAK_P99"
    echo ""

    # Append to summary
    echo "pendings=$PENDINGS:" >> "$SUMMARY_FILE"
    echo "  Throughput: $THROUGHPUT" >> "$SUMMARY_FILE"
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
sed -i "s/^pendings:.*$/pendings:    $ORIGINAL_PENDINGS       \/\/ Max in-flight commands per thread/" "$CONFIG_FILE"
echo "Restored pendings to: $ORIGINAL_PENDINGS"

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
