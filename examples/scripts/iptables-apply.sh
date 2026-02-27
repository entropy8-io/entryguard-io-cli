#!/bin/bash
# EntryGuard apply script for iptables
# Usage: apply.sh <CIDR> <description>
set -euo pipefail

CIDR="$1"
DESC="${2:-entryguard}"

# Add rule to accept traffic from CIDR on port 443
iptables -A INPUT -s "$CIDR" -p tcp --dport 443 -m comment --comment "entryguard: $DESC" -j ACCEPT
echo "Added iptables rule for $CIDR on port 443"
