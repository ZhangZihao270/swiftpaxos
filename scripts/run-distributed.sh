#!/bin/bash

# Distributed CURP-HT benchmark runner
# Usage: ./run-distributed.sh [config_file]
#
# Prerequisites:
#   1. SSH key-based authentication set up to all servers
#   2. swiftpaxos binary and config deployed to same path on all servers
#   3. Edit SERVER_MAP below to match your server hostnames/IPs

CONFIG="${1:-local.conf}"
STARTUP_DELAY=5
WORK_DIR="$(pwd)"

# ============================================================
# EDIT THIS: Map config IPs to actual server hostnames/IPs
# Format: CONFIG_IP -> SSH_HOST
# ============================================================
declare -A SERVER_MAP=(
    ["127.0.0.1"]="server1.example.com"  # replica0, master
    ["127.0.0.2"]="server2.example.com"  # replica1
    ["127.0.0.3"]="server3.example.com"  # replica2
    ["127.0.0.4"]="server1.example.com"  # client (can be same as replica0)
)

# SSH options (adjust as needed)
SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=5"
SSH_USER="${SSH_USER:-$(whoami)}"

# ============================================================

echo "=== CURP-HT Distributed Benchmark ==="
echo "Config: $CONFIG"
echo "Work dir: $WORK_DIR"
echo ""

# Parse replicas from config
get_replicas() {
    grep -E "^replica[0-9]+" "$CONFIG" | while read -r line; do
        alias=$(echo "$line" | awk '{print $1}')
        ip=$(echo "$line" | awk '{print $2}')
        echo "$alias $ip"
    done
}

get_master_ip() {
    grep -E "^master0" "$CONFIG" | awk '{print $2}'
}

get_client_ip() {
    grep -E "^client0" "$CONFIG" | awk '{print $2}'
}

# Get SSH host from config IP
get_ssh_host() {
    local config_ip="$1"
    echo "${SERVER_MAP[$config_ip]:-$config_ip}"
}

# Run command on remote server
run_remote() {
    local host="$1"
    shift
    local cmd="$@"
    ssh $SSH_OPTS "$SSH_USER@$host" "cd $WORK_DIR && $cmd"
}

# Run command on remote server (background)
run_remote_bg() {
    local host="$1"
    shift
    local cmd="$@"
    ssh $SSH_OPTS "$SSH_USER@$host" "cd $WORK_DIR && nohup $cmd > /dev/null 2>&1 &"
}

# Kill processes on remote server
kill_remote() {
    local host="$1"
    local pattern="$2"
    ssh $SSH_OPTS "$SSH_USER@$host" "pkill -f '$pattern'" 2>/dev/null || true
}

# Cleanup function
cleanup() {
    echo ""
    echo "Stopping all processes on remote servers..."

    # Kill on all servers
    for host in "${SERVER_MAP[@]}"; do
        kill_remote "$host" "swiftpaxos"
    done

    echo "Cleanup complete"
}
trap cleanup EXIT INT TERM

# Build locally first
echo "Building swiftpaxos..."
go build -o swiftpaxos . || exit 1

# Optionally sync binary to remote servers
echo ""
echo "Do you want to sync the binary to remote servers? (y/n)"
read -r sync_choice
if [[ "$sync_choice" == "y" ]]; then
    echo "Syncing binary and config to remote servers..."
    for host in $(printf '%s\n' "${SERVER_MAP[@]}" | sort -u); do
        echo "  -> $host"
        rsync -avz --progress swiftpaxos "$CONFIG" "$SSH_USER@$host:$WORK_DIR/" || {
            echo "Warning: Failed to sync to $host"
        }
    done
fi

# Stop any existing processes
echo ""
echo "Stopping any existing swiftpaxos processes..."
for host in $(printf '%s\n' "${SERVER_MAP[@]}" | sort -u); do
    kill_remote "$host" "swiftpaxos"
done
sleep 2

# Start master
master_ip=$(get_master_ip)
master_host=$(get_ssh_host "$master_ip")
echo ""
echo "Starting master on $master_host..."
run_remote_bg "$master_host" "./swiftpaxos -run master -config $CONFIG -alias master0"
sleep 2

# Start replicas
echo ""
echo "Starting replicas..."
while read -r alias ip; do
    host=$(get_ssh_host "$ip")
    echo "  Starting $alias on $host ($ip)..."
    run_remote_bg "$host" "./swiftpaxos -run server -config $CONFIG -alias $alias"
done < <(get_replicas)

echo ""
echo "Waiting ${STARTUP_DELAY}s for replicas to connect..."
sleep $STARTUP_DELAY

# Run client
client_ip=$(get_client_ip)
client_host=$(get_ssh_host "$client_ip")
echo ""
echo "=== Running Benchmark on $client_host ==="
run_remote "$client_host" "./swiftpaxos -run client -config $CONFIG -alias client0"

echo ""
echo "=== Done ==="
