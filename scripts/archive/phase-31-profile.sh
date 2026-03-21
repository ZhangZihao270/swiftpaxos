#!/bin/bash
# Phase 31.2: CPU and Memory Profiling Script for CURP-HO
# Usage: ./scripts/phase-31-profile.sh [cpu|mem|mutex|all]

set -e

PROFILE_TYPE="${1:-all}"
DURATION="${2:-30}"
OUTPUT_DIR="docs/phase-31-profiles"
# Replica runs on port 7070+1000 = 8070 (see run.go)
# Using replica0 at 127.0.0.1:8070
REPLICA_PPROF="127.0.0.1:8070"
# Client profiling not currently supported (would need client pprof setup)
CLIENT_PPROF="127.0.0.4:8070"

# Create output directory
mkdir -p "$OUTPUT_DIR"

echo "=== Phase 31.2: Performance Profiling ==="
echo "Profile type: $PROFILE_TYPE"
echo "Duration: ${DURATION}s"
echo "Output directory: $OUTPUT_DIR"
echo ""

# Function to collect CPU profile
collect_cpu_profile() {
    echo "Collecting CPU profile (${DURATION}s)..."

    # Replica CPU profile
    echo "  - Replica CPU profile..."
    curl -s "$REPLICA_PPROF/debug/pprof/profile?seconds=$DURATION" > "$OUTPUT_DIR/replica-cpu.prof" &
    REPLICA_PID=$!

    # Client CPU profile
    echo "  - Client CPU profile..."
    curl -s "$CLIENT_PPROF/debug/pprof/profile?seconds=$DURATION" > "$OUTPUT_DIR/client-cpu.prof" &
    CLIENT_PID=$!

    # Wait for both to complete
    wait $REPLICA_PID
    wait $CLIENT_PID

    echo "  ✓ CPU profiles saved"
    echo ""

    # Analyze replica CPU profile
    echo "=== Top CPU consumers (Replica) ==="
    go tool pprof -top -nodecount=15 "$OUTPUT_DIR/replica-cpu.prof" 2>/dev/null || echo "Failed to analyze replica CPU profile"
    echo ""

    echo "=== Top CPU consumers (Client) ==="
    go tool pprof -top -nodecount=15 "$OUTPUT_DIR/client-cpu.prof" 2>/dev/null || echo "Failed to analyze client CPU profile"
    echo ""
}

# Function to collect memory profile
collect_memory_profile() {
    echo "Collecting memory allocation profile..."

    # Replica memory profile
    echo "  - Replica memory profile..."
    curl -s "$REPLICA_PPROF/debug/pprof/allocs" > "$OUTPUT_DIR/replica-allocs.prof"

    # Client memory profile
    echo "  - Client memory profile..."
    curl -s "$CLIENT_PPROF/debug/pprof/allocs" > "$OUTPUT_DIR/client-allocs.prof"

    echo "  ✓ Memory profiles saved"
    echo ""

    # Analyze replica memory profile
    echo "=== Top memory allocations (Replica) ==="
    go tool pprof -top -nodecount=15 -alloc_space "$OUTPUT_DIR/replica-allocs.prof" 2>/dev/null || echo "Failed to analyze replica memory profile"
    echo ""

    echo "=== Top memory allocations (Client) ==="
    go tool pprof -top -nodecount=15 -alloc_space "$OUTPUT_DIR/client-allocs.prof" 2>/dev/null || echo "Failed to analyze client memory profile"
    echo ""
}

# Function to collect mutex profile
collect_mutex_profile() {
    echo "Collecting mutex contention profile..."

    # Replica mutex profile
    echo "  - Replica mutex profile..."
    curl -s "$REPLICA_PPROF/debug/pprof/mutex" > "$OUTPUT_DIR/replica-mutex.prof"

    # Client mutex profile
    echo "  - Client mutex profile..."
    curl -s "$CLIENT_PPROF/debug/pprof/mutex" > "$OUTPUT_DIR/client-mutex.prof"

    echo "  ✓ Mutex profiles saved"
    echo ""

    # Analyze replica mutex profile
    echo "=== Mutex contention (Replica) ==="
    go tool pprof -top -nodecount=15 "$OUTPUT_DIR/replica-mutex.prof" 2>/dev/null || echo "Failed to analyze replica mutex profile"
    echo ""

    echo "=== Mutex contention (Client) ==="
    go tool pprof -top -nodecount=15 "$OUTPUT_DIR/client-mutex.prof" 2>/dev/null || echo "Failed to analyze client mutex profile"
    echo ""
}

# Function to collect heap profile
collect_heap_profile() {
    echo "Collecting heap profile..."

    # Replica heap profile
    echo "  - Replica heap profile..."
    curl -s "$REPLICA_PPROF/debug/pprof/heap" > "$OUTPUT_DIR/replica-heap.prof"

    # Client heap profile
    echo "  - Client heap profile..."
    curl -s "$CLIENT_PPROF/debug/pprof/heap" > "$OUTPUT_DIR/client-heap.prof"

    echo "  ✓ Heap profiles saved"
    echo ""

    # Analyze replica heap profile
    echo "=== Heap usage (Replica) ==="
    go tool pprof -top -nodecount=15 -inuse_space "$OUTPUT_DIR/replica-heap.prof" 2>/dev/null || echo "Failed to analyze replica heap profile"
    echo ""
}

# Main execution
case "$PROFILE_TYPE" in
    cpu)
        collect_cpu_profile
        ;;
    mem|memory)
        collect_memory_profile
        collect_heap_profile
        ;;
    mutex)
        collect_mutex_profile
        ;;
    all)
        collect_cpu_profile
        collect_memory_profile
        collect_mutex_profile
        collect_heap_profile
        ;;
    *)
        echo "Error: Invalid profile type '$PROFILE_TYPE'"
        echo "Usage: $0 [cpu|mem|mutex|all] [duration_seconds]"
        exit 1
        ;;
esac

echo "=== Profile Collection Complete ==="
echo ""
echo "Profile files saved to: $OUTPUT_DIR/"
echo ""
echo "To analyze interactively:"
echo "  CPU:    go tool pprof -http=:8080 $OUTPUT_DIR/replica-cpu.prof"
echo "  Memory: go tool pprof -http=:8080 $OUTPUT_DIR/replica-allocs.prof"
echo "  Mutex:  go tool pprof -http=:8080 $OUTPUT_DIR/replica-mutex.prof"
echo "  Heap:   go tool pprof -http=:8080 $OUTPUT_DIR/replica-heap.prof"
echo ""
echo "Next steps:"
echo "  1. Review profile analysis above"
echo "  2. Identify top 3-5 CPU/memory hotspots"
echo "  3. Document findings in docs/phase-31.2-cpu-profile.md"
echo "  4. Plan optimizations for Phase 31.4+"
