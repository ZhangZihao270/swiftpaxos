#!/bin/bash
# Sweep clientThreads values for peak throughput testing
# Usage: ./sweep-throughput.sh <protocol> [thread_values...]
# Example: ./sweep-throughput.sh curpho 2 4 8 16 32
cd /home/users/zihao/swiftpaxos

PROTOCOL=${1:-curpho}
shift
THREADS="${@:-2 4 8 16 32}"
CONFIG="multi-client.conf"
TIMEOUT=300

echo "============================================"
echo "  Peak Throughput Sweep: $PROTOCOL"
echo "============================================"
echo "Thread values: $THREADS"
echo ""

for T in $THREADS; do
    echo "=== clientThreads=$T (total=${T}*3=$((T*3))) ==="

    # Update config file
    sed -i "s/^clientThreads:.*$/clientThreads: $T/" "$CONFIG"
    sed -i "s/^protocol:.*$/protocol: $PROTOCOL/" "$CONFIG"

    # Run benchmark with timeout
    timeout ${TIMEOUT}s ./run-multi-client.sh -c "$CONFIG" -d > /tmp/sweep-${PROTOCOL}-${T}.log 2>&1
    EXIT_CODE=$?

    if [[ $EXIT_CODE -eq 124 ]]; then
        echo "  HUNG (timeout after ${TIMEOUT}s)"
    else
        # Extract results
        THROUGHPUT=$(grep "Aggregate throughput:" /tmp/sweep-${PROTOCOL}-${T}.log | awk '{print $3}')
        STRONG_AVG=$(grep "Strong Operations:" -A2 /tmp/sweep-${PROTOCOL}-${T}.log | grep "Avg:" | sed 's/.*Avg: \([^ ]*\).*/\1/')
        STRONG_MED=$(grep "Strong Operations:" -A2 /tmp/sweep-${PROTOCOL}-${T}.log | grep "Avg:" | sed 's/.*Avg median: \([^ ]*\).*/\1/')
        STRONG_P99=$(grep "Strong Operations:" -A2 /tmp/sweep-${PROTOCOL}-${T}.log | grep "Avg:" | sed 's/.*Max P99: \([^ ]*\).*/\1/')
        WEAK_AVG=$(grep "Weak Operations:" -A2 /tmp/sweep-${PROTOCOL}-${T}.log | grep "Avg:" | sed 's/.*Avg: \([^ ]*\).*/\1/')
        WEAK_MED=$(grep "Weak Operations:" -A2 /tmp/sweep-${PROTOCOL}-${T}.log | grep "Avg:" | sed 's/.*Avg median: \([^ ]*\).*/\1/')
        WEAK_P99=$(grep "Weak Operations:" -A2 /tmp/sweep-${PROTOCOL}-${T}.log | grep "Avg:" | sed 's/.*Max P99: \([^ ]*\).*/\1/')
        ZERO_CLIENT=$(grep "0.00 ops/sec" /tmp/sweep-${PROTOCOL}-${T}.log)

        if [[ -n "$ZERO_CLIENT" ]]; then
            echo "  PARTIAL - $THROUGHPUT ops/sec (some clients failed)"
        elif [[ -n "$THROUGHPUT" ]]; then
            echo "  Throughput: $THROUGHPUT ops/sec"
            echo "  Strong: avg=$STRONG_AVG median=$STRONG_MED p99=$STRONG_P99"
            echo "  Weak:   avg=$WEAK_AVG median=$WEAK_MED p99=$WEAK_P99"
        else
            echo "  ERROR (no throughput data)"
        fi
    fi
    echo ""
    sleep 3
done

# Restore original config
sed -i "s/^clientThreads:.*$/clientThreads: 2/" "$CONFIG"
echo "Config restored to clientThreads: 2"
