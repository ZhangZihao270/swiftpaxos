#!/bin/bash

# Simple local CURP-HT benchmark runner
# Usage: ./run-local.sh [config_file]

CONFIG="${1:-local.conf}"
STARTUP_DELAY=3

echo "=== CURP-HT Local Benchmark ==="
echo "Config: $CONFIG"
echo ""

# Build if needed
if [[ ! -f ./swiftpaxos ]]; then
    echo "Building swiftpaxos..."
    go build -o swiftpaxos . || exit 1
fi

# Cleanup function
cleanup() {
    echo ""
    echo "Stopping all processes..."
    pkill -f "swiftpaxos" 2>/dev/null
    exit 0
}
trap cleanup EXIT INT TERM

# Kill any existing processes
pkill -f "swiftpaxos" 2>/dev/null
sleep 1

# Start master
echo "Starting master..."
./swiftpaxos -run master -config "$CONFIG" -alias master0 &
sleep 1

# Start replicas
echo "Starting replicas..."
./swiftpaxos -run server -config "$CONFIG" -alias replica0 &
./swiftpaxos -run server -config "$CONFIG" -alias replica1 &
./swiftpaxos -run server -config "$CONFIG" -alias replica2 &

echo "Waiting ${STARTUP_DELAY}s for replicas to connect..."
sleep $STARTUP_DELAY

# Run client
echo ""
echo "=== Running Benchmark ==="
./swiftpaxos -run client -config "$CONFIG" -alias client0

echo ""
echo "=== Done ==="
