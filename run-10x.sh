#!/bin/bash
# Run benchmark 10 times and collect pass/fail results
cd /home/users/zihao/swiftpaxos
TIMEOUT=240
PASSED=0
FAILED=0

for i in $(seq 1 10); do
    echo "=== Run $i/10 ==="
    
    # Run benchmark with timeout
    timeout ${TIMEOUT}s ./run-multi-client.sh -c benchmark.conf -d > /tmp/bench-run-${i}.log 2>&1
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
echo "=== Results: $PASSED/10 passed, $FAILED/10 failed ==="
