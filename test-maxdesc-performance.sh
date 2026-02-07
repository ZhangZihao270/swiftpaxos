#!/bin/bash
# Test MaxDescRoutines impact on throughput

set -e

echo "=== MaxDescRoutines Performance Test ==="
echo "Testing values: 100, 500, 1000, 2000, 5000, 10000, 20000"
echo ""

# Test each value
for value in 100 500 1000 2000 5000 10000 20000; do
    echo "========================================="
    echo "Testing MaxDescRoutines = $value"
    echo "========================================="

    # Create temp config
    TEMP_CONF="/tmp/test-maxdesc-${value}.conf"
    cp multi-client.conf "$TEMP_CONF"

    # Add/update maxDescRoutines line
    if grep -q "^maxDescRoutines:" "$TEMP_CONF"; then
        sed -i "s/^maxDescRoutines:.*/maxDescRoutines: $value/" "$TEMP_CONF"
    else
        # Add after protocol line
        sed -i "/^protocol:/a maxDescRoutines: $value" "$TEMP_CONF"
    fi

    echo "Config: maxDescRoutines = $value"
    echo "Running benchmark..."

    # Run benchmark (shorter for faster testing)
    # Modify reqs to 5000 for faster testing
    sed -i "s/^reqs:.*/reqs:        5000/" "$TEMP_CONF"

    OUTPUT_FILE="results-maxdesc-${value}.txt"

    # Run and capture output
    ./run-multi-client.sh -c "$TEMP_CONF" 2>&1 | tee "$OUTPUT_FILE"

    # Extract throughput
    THROUGHPUT=$(grep "Aggregate throughput:" "$OUTPUT_FILE" | awk '{print $3}')
    echo ""
    echo ">>> MaxDescRoutines=$value → Throughput=$THROUGHPUT ops/sec"
    echo ""

    # Clean up
    rm -f "$TEMP_CONF"

    # Wait between runs
    sleep 5
done

echo ""
echo "=== Summary ==="
echo "MaxDescRoutines | Throughput"
echo "----------------|------------"
for value in 100 500 1000 2000 5000 10000 20000; do
    FILE="results-maxdesc-${value}.txt"
    if [ -f "$FILE" ]; then
        TP=$(grep "Aggregate throughput:" "$FILE" | awk '{print $3}')
        printf "%15s | %s ops/sec\n" "$value" "$TP"
    fi
done
