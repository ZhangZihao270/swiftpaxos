#!/bin/bash
#
# Terminate all SwiftPaxos AWS instances.
#
# Usage:
#   bash scripts/aws-teardown.sh [--stop]
#
# Options:
#   --stop    Stop instances (can restart later) instead of terminate

set -euo pipefail

ACTION="terminate"
if [[ "${1:-}" == "--stop" ]]; then
    ACTION="stop"
fi

IDS=$(aws ec2 describe-instances \
    --filters "Name=tag:Project,Values=swiftpaxos" "Name=instance-state-name,Values=running,stopped" \
    --query 'Reservations[].Instances[].InstanceId' --output text)

if [[ -z "$IDS" ]]; then
    echo "No swiftpaxos instances found."
    exit 0
fi

COUNT=$(echo "$IDS" | wc -w)
echo "Found $COUNT instances: $IDS"
echo ""

if [[ "$ACTION" == "stop" ]]; then
    echo "Stopping instances..."
    aws ec2 stop-instances --instance-ids $IDS --output text
    echo "  Stopped. Use 'aws ec2 start-instances' to restart."
else
    echo "Terminating instances (this is permanent!)..."
    read -p "Are you sure? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        aws ec2 terminate-instances --instance-ids $IDS --output text
        echo "  Terminated."
    else
        echo "  Cancelled."
    fi
fi
