#!/bin/bash
#
# Setup AWS instance IPs for all experiment configs and eval scripts.
#
# Usage:
#   bash scripts/setup-aws-ips.sh <r0> <r1> <r2> <r3> <r4> [<c0> <c1> <c2>]
#
# Arguments:
#   r0..r4  — IP addresses of the 5 replica machines
#   c0..c2  — (optional) IP addresses of 3 client machines
#             Default: c0=r0, c1=r1, c2=r2 (co-located with replicas)
#
# Example (5 replicas, clients co-located):
#   bash scripts/setup-aws-ips.sh 10.0.1.1 10.0.2.1 10.0.3.1 10.0.4.1 10.0.5.1
#
# Example (separate client machines):
#   bash scripts/setup-aws-ips.sh 10.0.1.1 10.0.2.1 10.0.3.1 10.0.4.1 10.0.5.1 \
#                                  10.0.1.2 10.0.2.2 10.0.3.2

set -euo pipefail

if [[ $# -lt 5 ]]; then
    echo "Usage: $0 <r0> <r1> <r2> <r3> <r4> [<c0> <c1> <c2>]"
    exit 1
fi

R0=$1; R1=$2; R2=$3; R3=$4; R4=$5
C0=${6:-$R0}; C1=${7:-$R1}; C2=${8:-$R2}
MASTER=$R1  # master on replica1 (same as current setup)

WORK_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "Replica IPs: $R0 $R1 $R2 $R3 $R4"
echo "Client IPs:  $C0 $C1 $C2"
echo "Master IP:   $MASTER"
echo ""

# --- Update config files ---
for conf in "$WORK_DIR"/configs/exp*.conf; do
    echo "Updating $(basename $conf)..."
    sed -i \
        -e "s|^replica0 .*|replica0 $R0|" \
        -e "s|^replica1 .*|replica1 $R1|" \
        -e "s|^replica2 .*|replica2 $R2|" \
        -e "s|^replica3 .*|replica3 $R3|" \
        -e "s|^replica4 .*|replica4 $R4|" \
        -e "s|^client0 .*|client0 $C0|" \
        -e "s|^client1 .*|client1 $C1|" \
        -e "s|^client2 .*|client2 $C2|" \
        -e "s|^master0 .*|master0 $MASTER|" \
        "$conf"
done

# --- Update eval scripts ---
ALL_HOSTS="ALL_HOSTS=($R0 $R1 $R2 $R3 $R4)"
for script in "$WORK_DIR"/scripts/eval-exp*.sh; do
    echo "Updating $(basename $script)..."
    sed -i "s|^ALL_HOSTS=(.*)$|$ALL_HOSTS|" "$script"
done

# --- Update benchmark.conf (used for local testing) ---
if [[ -f "$WORK_DIR/benchmark.conf" ]]; then
    echo "Updating benchmark.conf..."
    sed -i \
        -e "s|^replica0 .*|replica0 $R0|" \
        -e "s|^replica1 .*|replica1 $R1|" \
        -e "s|^replica2 .*|replica2 $R2|" \
        -e "s|^replica3 .*|replica3 $R3|" \
        -e "s|^replica4 .*|replica4 $R4|" \
        -e "s|^client0 .*|client0 $C0|" \
        -e "s|^client1 .*|client1 $C1|" \
        -e "s|^client2 .*|client2 $C2|" \
        -e "s|^master0 .*|master0 $MASTER|" \
        "$WORK_DIR/benchmark.conf"
fi

echo ""
echo "Done! All IPs updated. Verify with:"
echo "  grep 'replica0\|client0\|master0' configs/exp1.1-base.conf"
