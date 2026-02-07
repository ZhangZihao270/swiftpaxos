#!/bin/bash
# Phase 31.4: Validate batchDelayUs=150 as optimal configuration
# Run comprehensive test to confirm 23K target achieved

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/docs/phase-31-profiles"
ITERATIONS=5  # More iterations for confidence
REQUESTS=10000

mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULT_FILE="$OUTPUT_DIR/batch-delay-150-validation-$TIMESTAMP.txt"

echo "=== Phase 31.4: Validate batchDelayUs=150 Configuration ===" | tee "$RESULT_FILE"
echo "Goal: Confirm 23K ops/sec target achievement" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

cd "$PROJECT_ROOT"

# Setup config
cp multi-client.conf multi-client.conf.backup
sed -i "s/^reqs:.*/reqs:        $REQUESTS/" multi-client.conf
sed -i "s/^clientThreads:.*/clientThreads: 2/" multi-client.conf
sed -i "s/^pendings:.*/pendings:    15/" multi-client.conf

# Add batchDelayUs
if grep -q "^batchDelayUs:" multi-client.conf; then
    sed -i "s/^batchDelayUs:.*/batchDelayUs: 150/" multi-client.conf
else
    sed -i "/^maxDescRoutines:/a batchDelayUs: 150   // Optimal from Phase 31.4" multi-client.conf
fi

# Ensure 2 clients
sed -i 's/^# client0/client0/' multi-client.conf
sed -i 's/^# client1/client1/' multi-client.conf
sed -i 's/^client2/# client2/' multi-client.conf
sed -i 's/^client3/# client3/' multi-client.conf

declare -a throughputs
declare -a weak_medians

echo "Running $ITERATIONS iterations..." | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

for i in $(seq 1 $ITERATIONS); do
    echo "Iteration $i/$ITERATIONS..." | tee -a "$RESULT_FILE"

    OUTPUT=$(timeout 120 ./run-multi-client.sh -c multi-client.conf 2>&1)

    THROUGHPUT=$(echo "$OUTPUT" | grep -oP 'Aggregate throughput:\s+\K[0-9.]+' | tail -1)
    WEAK_MED=$(echo "$OUTPUT" | grep "Weak Operations:" -A 2 | grep -oP 'Avg median:\s+\K[0-9.]+' | head -1)

    throughputs+=("$THROUGHPUT")
    weak_medians+=("$WEAK_MED")

    echo "  Throughput: $THROUGHPUT ops/sec, Weak median: ${WEAK_MED}ms" | tee -a "$RESULT_FILE"

    sleep 2
done

# Calculate statistics
sum=0
min=999999
max=0
for val in "${throughputs[@]}"; do
    sum=$(echo "$sum + $val" | bc -l)
    if (( $(echo "$val < $min" | bc -l) )); then min=$val; fi
    if (( $(echo "$val > $max" | bc -l) )); then max=$val; fi
done
avg=$(echo "scale=2; $sum / $ITERATIONS" | bc -l)

weak_sum=0
for val in "${weak_medians[@]}"; do
    weak_sum=$(echo "$weak_sum + $val" | bc -l)
done
weak_avg=$(echo "scale=2; $weak_sum / $ITERATIONS" | bc -l)

echo "" | tee -a "$RESULT_FILE"
echo "=== Results Summary ===" | tee -a "$RESULT_FILE"
echo "Configuration: pendings=15, 4 streams, batchDelayUs=150" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Throughput:" | tee -a "$RESULT_FILE"
echo "  Min: $min ops/sec" | tee -a "$RESULT_FILE"
echo "  Max: $max ops/sec" | tee -a "$RESULT_FILE"
echo "  Avg: $avg ops/sec" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"
echo "Weak Median Latency: ${weak_avg}ms" | tee -a "$RESULT_FILE"
echo "" | tee -a "$RESULT_FILE"

# Check target
if (( $(echo "$avg >= 23000" | bc -l) )) && (( $(echo "$weak_avg < 2.0" | bc -l) )); then
    echo "✓✓✓ SUCCESS: TARGET ACHIEVED! ✓✓✓" | tee -a "$RESULT_FILE"
    echo "" | tee -a "$RESULT_FILE"
    echo "Phase 31 goal of 23K ops/sec with weak latency < 2ms has been met!" | tee -a "$RESULT_FILE"
elif (( $(echo "$avg >= 23000" | bc -l) )); then
    echo "✗ Target throughput reached but latency constraint violated" | tee -a "$RESULT_FILE"
else
    echo "✗ Target not met (gap: $(echo "23000 - $avg" | bc -l) ops/sec)" | tee -a "$RESULT_FILE"
fi

echo "" | tee -a "$RESULT_FILE"
echo "Results saved to: $RESULT_FILE" | tee -a "$RESULT_FILE"

mv multi-client.conf.backup multi-client.conf
