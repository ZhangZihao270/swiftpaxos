#!/bin/bash
#
# Initialize all AWS instances: install Go, setup SSH keys.
# Run this AFTER setup-aws-ips.sh has been run.
#
# Usage:
#   bash scripts/aws-init.sh [ssh-key-path]

set -euo pipefail

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"
KEY="${1:-$HOME/.ssh/swiftpaxos.pem}"
SSH_USER="ubuntu"
SSH_OPTS="-i $KEY -o StrictHostKeyChecking=no -o ConnectTimeout=10"

# Read IPs from the first config file
CONF="$WORK_DIR/configs/exp1.1-base.conf"
HOSTS=$(grep -E "^(replica|client|master)" "$CONF" | awk '{print $2}' | sort -u)

if [[ -z "$HOSTS" ]]; then
    echo "Error: No hosts found in $CONF. Run setup-aws-ips.sh first."
    exit 1
fi

echo "Hosts to initialize:"
echo "$HOSTS" | tr '\n' ' '
echo ""
echo ""

# ============================================================================
# Step 1: Test SSH connectivity
# ============================================================================
echo "Testing SSH connectivity..."
for host in $HOSTS; do
    if ssh $SSH_OPTS "$SSH_USER@$host" "echo ok" &>/dev/null; then
        echo "  $host: OK"
    else
        echo "  $host: FAILED"
        echo "    Check: ssh $SSH_OPTS $SSH_USER@$host"
        exit 1
    fi
done
echo ""

# ============================================================================
# Step 2: Install Go on all machines (parallel)
# ============================================================================
echo "Installing Go on all machines..."
for host in $HOSTS; do
    ssh $SSH_OPTS "$SSH_USER@$host" bash -s <<'REMOTE' &
        if command -v go &>/dev/null; then
            echo "Go already installed: $(go version)"
        else
            sudo apt-get update -qq
            sudo apt-get install -y -qq golang-go
            echo "Go installed: $(go version)"
        fi
        # Create work directory
        mkdir -p ~/swiftpaxos
REMOTE
    echo "  Started: $host"
done
wait
echo "  All done!"
echo ""

# ============================================================================
# Step 3: Generate SSH key on this machine and distribute
# ============================================================================
# The eval scripts SSH from this machine to the remote hosts.
# run-multi-client.sh also SSHes between hosts (master -> clients).
# We need to setup SSH keys so hosts can reach each other.

echo "Setting up inter-host SSH..."
# Generate a temporary key for inter-host communication
TMPKEY="/tmp/swiftpaxos-inter-$$"
ssh-keygen -t ed25519 -f "$TMPKEY" -N "" -q

PUB=$(cat "${TMPKEY}.pub")

for host in $HOSTS; do
    # Add the inter-host key and authorize it
    ssh $SSH_OPTS "$SSH_USER@$host" bash -s <<REMOTE
        mkdir -p ~/.ssh
        echo '$PUB' >> ~/.ssh/authorized_keys
        sort -u -o ~/.ssh/authorized_keys ~/.ssh/authorized_keys
REMOTE
    # Copy private key to each host
    scp $SSH_OPTS "$TMPKEY" "$SSH_USER@$host:~/.ssh/id_ed25519"
    scp $SSH_OPTS "${TMPKEY}.pub" "$SSH_USER@$host:~/.ssh/id_ed25519.pub"
    ssh $SSH_OPTS "$SSH_USER@$host" "chmod 600 ~/.ssh/id_ed25519"
    echo "  $host: SSH key deployed"
done

rm -f "$TMPKEY" "${TMPKEY}.pub"
echo ""

# ============================================================================
# Step 4: Build and sync binary
# ============================================================================
echo "Building swiftpaxos..."
cd "$WORK_DIR"
go build -o swiftpaxos .
echo "  Built: $(ls -lh swiftpaxos | awk '{print $5}')"
echo ""

echo "============================================"
echo "  Initialization Complete!"
echo "============================================"
echo ""
echo "Ready to run experiments:"
echo "  bash scripts/eval-exp1.1.sh"
echo "  bash scripts/eval-exp2.1-final.sh"
echo "  bash scripts/eval-exp2.2-final.sh"
echo "  bash scripts/eval-exp3.1-final.sh"
echo "  bash scripts/eval-exp3.2-final.sh"
