#!/bin/bash
# add-route.sh — Add a host route through a specific network interface.
# Called by the Phish & Discover backend (network.SetupRoute) or manually by the operator.
#
# Usage: ./scripts/add-route.sh <cidr> <interface>
#
# Examples:
#   ./scripts/add-route.sh 192.168.1.0/24 tun0
#   ./scripts/add-route.sh 10.10.0.0/16   tun0

set -euo pipefail

CIDR="${1:?Usage: $0 <cidr> <interface>}"
IFACE="${2:?Usage: $0 <cidr> <interface>}"

echo "[Phish & Discover-route] Adding route: $CIDR via $IFACE"

# Check if the interface is up
if ! ip link show "$IFACE" | grep -q "UP"; then
    echo "[Phish & Discover-route] WARNING: Interface $IFACE does not appear to be UP."
fi

# Add route — fail gracefully if it already exists
if ip route show "$CIDR" dev "$IFACE" &>/dev/null; then
    echo "[Phish & Discover-route] Route $CIDR via $IFACE already exists — skipping."
else
    ip route add "$CIDR" dev "$IFACE"
    echo "[Phish & Discover-route] Route added: $CIDR via $IFACE"
fi

# Verify
echo "[Phish & Discover-route] Verification:"
ip route get "${CIDR%%/*}" || echo "[Phish & Discover-route] Could not verify route."
