#!/bin/bash
# Run benchmark N times and collect pass/fail results
# Usage: ./run-10x.sh [N] [config]
cd /home/users/zihao/swiftpaxos
N=${1:-5}
CONFIG=${2:-multi-client.conf}
TIMEOUT=240
PASSED=0
FAILED=0

for i in $(seq 1 $N); do
    echo "=== Run $i/$N ==="

    # Run benchmark with timeout
    timeout ${TIMEOUT}s ./run-multi-client.sh -c "$CONFIG" -d > /tmp/bench-run-${i}.log 2>&1
    EXIT_CODE=$?

    if [[ $EXIT_CODE -eq 124 ]]; then
        echo "  HUNG (timeout after ${TIMEOUT}s)"
        # Check which clients completed
        RESULTS_DIR=$(grep "Results dir:" /tmp/bench-run-${i}.log | awk '{print $NF}')
        if [[ -n "$RESULTS_DIR" ]]; then
            for c in client0 client1 client2; do
                if [[ -f "$RESULTS_DIR/${c}.log" ]] && grep -q "Throughput:" "$RESULTS_DIR/${c}.log" 2>/dev/null; then
                    echo "    ${c}: OK"
                else
                    echo "    ${c}: HUNG"
                fi
            done
        fi
        FAILED=$((FAILED + 1))
    else
        # Check if all clients completed
        THROUGHPUT=$(grep "Aggregate throughput:" /tmp/bench-run-${i}.log | awk '{print $3}')
        ZERO_CLIENT=$(grep "0.00 ops/sec" /tmp/bench-run-${i}.log)
        if [[ -n "$ZERO_CLIENT" ]]; then
            echo "  PARTIAL - $THROUGHPUT ops/sec (some clients failed)"
            FAILED=$((FAILED + 1))
        elif [[ -n "$THROUGHPUT" ]]; then
            echo "  OK - $THROUGHPUT ops/sec"
            PASSED=$((PASSED + 1))
        else
            echo "  ERROR (no throughput data)"
            FAILED=$((FAILED + 1))
        fi
    fi

    # Brief pause between runs
    sleep 3
done

echo ""
echo "=== Results: $PASSED/$N passed, $FAILED/$N failed ==="
