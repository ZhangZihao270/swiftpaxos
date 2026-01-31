#!/bin/bash

# CURP-HT Benchmark Runner Script
# Usage: ./run-curpht.sh [local|distributed] [config_file]
#
# Local mode: Runs all components on localhost (default)
# Distributed mode: SSHs to remote servers defined in config

set -e

# Configuration
MODE="${1:-local}"
CONFIG="${2:-local.conf}"
BINARY="./swiftpaxos"
STARTUP_DELAY=3        # Seconds to wait for replicas to start
MASTER_DELAY=1         # Seconds to wait for master to start

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Parse config file to extract replica/client addresses
parse_config() {
    # Extract replica addresses (format: "replicaN IP")
    REPLICA_ADDRS=()
    REPLICA_ALIASES=()
    while IFS= read -r line; do
        if [[ $line =~ ^replica([0-9]+)[[:space:]]+(.+)$ ]]; then
            REPLICA_ALIASES+=("replica${BASH_REMATCH[1]}")
            REPLICA_ADDRS+=("${BASH_REMATCH[2]}")
        fi
    done < "$CONFIG"

    # Extract client address
    CLIENT_ADDR=$(grep -E "^client0[[:space:]]+" "$CONFIG" | awk '{print $2}')

    # Extract master address
    MASTER_ADDR=$(grep -E "^master0[[:space:]]+" "$CONFIG" | awk '{print $2}')

    NUM_REPLICAS=${#REPLICA_ALIASES[@]}
    log_info "Found $NUM_REPLICAS replicas, master at $MASTER_ADDR, client at $CLIENT_ADDR"
}

# Cleanup function to kill all background processes
cleanup() {
    log_info "Cleaning up processes..."

    # Kill local background processes
    if [[ -n "${MASTER_PID:-}" ]]; then
        kill $MASTER_PID 2>/dev/null || true
    fi

    for pid in "${REPLICA_PIDS[@]:-}"; do
        kill $pid 2>/dev/null || true
    done

    # For distributed mode, kill remote processes
    if [[ "$MODE" == "distributed" ]]; then
        for i in "${!REPLICA_ADDRS[@]}"; do
            addr="${REPLICA_ADDRS[$i]}"
            ssh "$addr" "pkill -f 'swiftpaxos.*${REPLICA_ALIASES[$i]}'" 2>/dev/null || true
        done
        ssh "$MASTER_ADDR" "pkill -f 'swiftpaxos.*master'" 2>/dev/null || true
    else
        # Local mode: kill all swiftpaxos processes
        pkill -f "swiftpaxos" 2>/dev/null || true
    fi

    log_info "Cleanup complete"
}

# Set trap to cleanup on exit
trap cleanup EXIT INT TERM

# Check if binary exists
check_binary() {
    if [[ ! -f "$BINARY" ]]; then
        log_info "Binary not found, building..."
        go build -o swiftpaxos .
    fi

    if [[ ! -x "$BINARY" ]]; then
        chmod +x "$BINARY"
    fi
    log_info "Binary ready: $BINARY"
}

# Start master
start_master() {
    log_info "Starting master..."

    if [[ "$MODE" == "distributed" ]]; then
        ssh "$MASTER_ADDR" "cd $(pwd) && $BINARY -run master -config $CONFIG -alias master0" &
        MASTER_PID=$!
    else
        $BINARY -run master -config $CONFIG -alias master0 &
        MASTER_PID=$!
    fi

    log_info "Master started (PID: $MASTER_PID)"
    sleep $MASTER_DELAY
}

# Start replicas
start_replicas() {
    log_info "Starting $NUM_REPLICAS replicas..."
    REPLICA_PIDS=()

    for i in "${!REPLICA_ALIASES[@]}"; do
        alias="${REPLICA_ALIASES[$i]}"
        addr="${REPLICA_ADDRS[$i]}"

        if [[ "$MODE" == "distributed" ]]; then
            log_info "Starting $alias on $addr (SSH)..."
            ssh "$addr" "cd $(pwd) && $BINARY -run server -config $CONFIG -alias $alias" &
            REPLICA_PIDS+=($!)
        else
            log_info "Starting $alias locally..."
            $BINARY -run server -config $CONFIG -alias $alias &
            REPLICA_PIDS+=($!)
        fi
    done

    log_info "Waiting ${STARTUP_DELAY}s for replicas to initialize..."
    sleep $STARTUP_DELAY
}

# Run client benchmark
run_client() {
    log_info "Running client benchmark..."

    if [[ "$MODE" == "distributed" ]]; then
        ssh "$CLIENT_ADDR" "cd $(pwd) && $BINARY -run client -config $CONFIG -alias client0"
    else
        $BINARY -run client -config $CONFIG -alias client0
    fi

    log_info "Benchmark complete!"
}

# Main execution
main() {
    echo "========================================"
    echo "  CURP-HT Benchmark Runner"
    echo "  Mode: $MODE"
    echo "  Config: $CONFIG"
    echo "========================================"

    # Check config file exists
    if [[ ! -f "$CONFIG" ]]; then
        log_error "Config file not found: $CONFIG"
        exit 1
    fi

    parse_config
    check_binary

    # Kill any existing processes first
    log_info "Killing any existing swiftpaxos processes..."
    pkill -f "swiftpaxos" 2>/dev/null || true
    sleep 1

    start_master
    start_replicas
    run_client
}

main "$@"
