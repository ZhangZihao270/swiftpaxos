#!/bin/bash
#
# Launch AWS EC2 instances for SwiftPaxos evaluation.
#
# Prerequisites:
#   1. AWS CLI installed: aws --version
#   2. AWS credentials configured: aws configure
#
# Usage:
#   bash scripts/aws-launch.sh [region]
#
# Default region: us-east-1

set -euo pipefail

REGION="${1:-us-east-1}"
KEY_NAME="swiftpaxos"
SG_NAME="swiftpaxos"
PROJECT_TAG="swiftpaxos"

# Ubuntu 22.04 LTS AMIs by region (x86_64, HVM, EBS)
declare -A AMIS=(
    ["us-east-1"]="ami-0c7217cdde317cfec"
    ["us-east-2"]="ami-05fb0b8c1424f266b"
    ["us-west-1"]="ami-0ce2cb35386fc22e9"
    ["us-west-2"]="ami-008fe2fc65df48dac"
    ["eu-west-1"]="ami-0905a3c97561e0b69"
)

AMI="${AMIS[$REGION]:-}"
if [[ -z "$AMI" ]]; then
    echo "Error: No AMI configured for region $REGION"
    echo "Supported regions: ${!AMIS[*]}"
    exit 1
fi

export AWS_DEFAULT_REGION="$REGION"

echo "Region: $REGION"
echo "AMI: $AMI"
echo ""

# ============================================================================
# Step 1: Key Pair
# ============================================================================
KEY_FILE="$HOME/.ssh/${KEY_NAME}.pem"
if aws ec2 describe-key-pairs --key-names "$KEY_NAME" &>/dev/null; then
    echo "Key pair '$KEY_NAME' already exists."
else
    echo "Creating key pair '$KEY_NAME'..."
    aws ec2 create-key-pair --key-name "$KEY_NAME" \
        --query 'KeyMaterial' --output text > "$KEY_FILE"
    chmod 400 "$KEY_FILE"
    echo "  Saved to $KEY_FILE"
fi

# ============================================================================
# Step 2: Security Group
# ============================================================================
SG_ID=$(aws ec2 describe-security-groups --group-names "$SG_NAME" \
    --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null || echo "")

if [[ -n "$SG_ID" && "$SG_ID" != "None" ]]; then
    echo "Security group '$SG_NAME' already exists: $SG_ID"
else
    echo "Creating security group '$SG_NAME'..."
    SG_ID=$(aws ec2 create-security-group --group-name "$SG_NAME" \
        --description "SwiftPaxos evaluation" \
        --query 'GroupId' --output text)
    echo "  Created: $SG_ID"

    # Allow SSH from anywhere
    aws ec2 authorize-security-group-ingress --group-id "$SG_ID" \
        --protocol tcp --port 22 --cidr 0.0.0.0/0
    echo "  Allowed: SSH (22) from 0.0.0.0/0"

    # Allow all TCP within the group (replica communication)
    aws ec2 authorize-security-group-ingress --group-id "$SG_ID" \
        --protocol tcp --port 0-65535 --source-group "$SG_ID"
    echo "  Allowed: All TCP within group"
fi

# ============================================================================
# Step 3: Launch Instances
# ============================================================================
echo ""
echo "Launching 5 replicas (c5.xlarge)..."
REPLICA_IDS=$(aws ec2 run-instances \
    --image-id "$AMI" \
    --instance-type c5.xlarge \
    --key-name "$KEY_NAME" \
    --security-group-ids "$SG_ID" \
    --count 5 \
    --tag-specifications "ResourceType=instance,Tags=[{Key=Project,Value=$PROJECT_TAG},{Key=Role,Value=replica}]" \
    --query 'Instances[].InstanceId' --output text)
echo "  Instance IDs: $REPLICA_IDS"

echo "Launching 3 clients (c5.2xlarge)..."
CLIENT_IDS=$(aws ec2 run-instances \
    --image-id "$AMI" \
    --instance-type c5.2xlarge \
    --key-name "$KEY_NAME" \
    --security-group-ids "$SG_ID" \
    --count 3 \
    --tag-specifications "ResourceType=instance,Tags=[{Key=Project,Value=$PROJECT_TAG},{Key=Role,Value=client}]" \
    --query 'Instances[].InstanceId' --output text)
echo "  Instance IDs: $CLIENT_IDS"

# ============================================================================
# Step 4: Wait for Running
# ============================================================================
ALL_IDS="$REPLICA_IDS $CLIENT_IDS"
echo ""
echo "Waiting for instances to start..."
aws ec2 wait instance-running --instance-ids $ALL_IDS
echo "  All instances running!"

# ============================================================================
# Step 5: Display IPs
# ============================================================================
echo ""
echo "============================================"
echo "  Instance Summary"
echo "============================================"
aws ec2 describe-instances \
    --instance-ids $ALL_IDS \
    --query 'Reservations[].Instances[].{ID:InstanceId,Role:Tags[?Key==`Role`].Value|[0],PublicIP:PublicIpAddress,PrivateIP:PrivateIpAddress}' \
    --output table

# Collect IPs
REPLICA_IPS=$(aws ec2 describe-instances --instance-ids $REPLICA_IDS \
    --query 'Reservations[].Instances[].PublicIpAddress' --output text)
CLIENT_IPS=$(aws ec2 describe-instances --instance-ids $CLIENT_IDS \
    --query 'Reservations[].Instances[].PublicIpAddress' --output text)

echo ""
echo "============================================"
echo "  Next Steps"
echo "============================================"
echo ""
echo "1. Configure IPs:"
echo "   bash scripts/setup-aws-ips.sh $REPLICA_IPS $CLIENT_IPS"
echo ""
echo "2. Initialize machines (install Go, setup SSH):"
echo "   bash scripts/aws-init.sh"
echo ""
echo "3. Run experiments:"
echo "   bash scripts/eval-exp1.1.sh"
echo "   bash scripts/eval-exp2.1-final.sh"
echo "   ..."
echo ""
echo "4. When done, terminate all instances:"
echo "   bash scripts/aws-teardown.sh"
