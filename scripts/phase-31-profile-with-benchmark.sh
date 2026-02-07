#!/bin/bash
# Phase 31.2: Run benchmark and collect profiles simultaneously
# This script starts a benchmark run and collects CPU/memory profiles during execution

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$PROJECT_ROOT/docs/phase-31-profiles"
BENCHMARK_DURATION=60  # Run benchmark for 60 seconds
PROFILE_DURATION=30    # Collect profile during middle 30 seconds

mkdir -p "$OUTPUT_DIR"

echo "=== Phase 31.2: Profiling with Active Benchmark ==="
echo "Benchmark duration: ${BENCHMARK_DURATION}s"
echo "Profile collection: ${PROFILE_DURATION}s"
echo "Output directory: $OUTPUT_DIR"
echo ""

# Step 1: Start benchmark in background (100K ops should take ~15-20 seconds)
# Use enough operations to sustain load during profile collection
REQS_PER_CLIENT=200000  # 400K total ops, should take ~60 seconds at 6.5K ops/sec
echo "Starting benchmark (400K total ops)..."
cd "$PROJECT_ROOT"

# Update config temporarily
cp multi-client.conf multi-client.conf.profiling-backup
sed -i "s/^reqs:.*/reqs:        $REQS_PER_CLIENT/" multi-client.conf

# Start benchmark in background
timeout $((BENCHMARK_DURATION + 30)) ./run-multi-client.sh -c multi-client.conf > "$OUTPUT_DIR/benchmark-output.log" 2>&1 &
BENCHMARK_PID=$!

echo "Benchmark started (PID: $BENCHMARK_PID)"
echo "Waiting 10 seconds for system to stabilize..."
sleep 10

# Step 2: Collect profiles while benchmark is running
echo ""
echo "Collecting profiles (${PROFILE_DURATION}s)..."
./scripts/phase-31-profile.sh all $PROFILE_DURATION

# Step 3: Wait for benchmark to complete
echo ""
echo "Waiting for benchmark to complete..."
wait $BENCHMARK_PID || true  # Don't fail if benchmark times out

# Restore original config
mv multi-client.conf.profiling-backup multi-client.conf

# Step 4: Extract benchmark results
echo ""
echo "=== Benchmark Results ==="
grep -E "(Aggregate throughput|Strong Operations|Weak Operations|Avg median|Max P99)" "$OUTPUT_DIR/benchmark-output.log" || echo "Benchmark may have failed - check log"

echo ""
echo "=== Profiling Complete ==="
echo "Profiles saved to: $OUTPUT_DIR/"
echo ""
echo "Profile files:"
ls -lh "$OUTPUT_DIR"/*.prof 2>/dev/null || echo "No .prof files found - pprof may not be working"
echo ""
echo "Next steps:"
echo "  1. Review profile analysis above"
echo "  2. Analyze interactively: go tool pprof -http=:8080 $OUTPUT_DIR/replica-cpu.prof"
echo "  3. Document findings in docs/phase-31.2-cpu-profile.md"
