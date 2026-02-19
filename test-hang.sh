#!/bin/bash
# Run CURP-HO benchmark N times, detect hangs via timeout
TOTAL=10
PASS=0
FAIL=0
TIMEOUT=120  # seconds

for i in $(seq 1 $TOTAL); do
    echo "=== Run $i/$TOTAL ==="
    # Run benchmark with timeout
    timeout $TIMEOUT ./run-multi-client.sh -c benchmark.conf -d > /tmp/bench-run-$i.log 2>&1
    EXIT=$?
    
    # Find the latest results directory
    LATEST=$(ls -td results/benchmark-* 2>/dev/null | head -1)
    
    if [ $EXIT -eq 124 ]; then
        echo "  HUNG (timeout after ${TIMEOUT}s)"
        # Check which clients finished
        for f in $LATEST/client*.log; do
            CLIENT=$(basename $f .log)
            if grep -q "Test took" "$f" 2>/dev/null; then
                echo "    $CLIENT: OK"
            else
                echo "    $CLIENT: HUNG"
            fi
        done
        FAIL=$((FAIL+1))
    elif [ $EXIT -ne 0 ]; then
        echo "  ERROR (exit code $EXIT)"
        FAIL=$((FAIL+1))
    elif [ -f "$LATEST/summary.txt" ]; then
        THROUGHPUT=$(grep "Aggregate throughput" "$LATEST/summary.txt" | grep -oP '[\d.]+')
        echo "  OK - ${THROUGHPUT} ops/sec"
        PASS=$((PASS+1))
    else
        echo "  FAIL (no summary)"
        FAIL=$((FAIL+1))
    fi
    
    # Small delay between runs
    sleep 3
done

echo ""
echo "=== Results: $PASS/$TOTAL passed, $FAIL/$TOTAL failed ==="
