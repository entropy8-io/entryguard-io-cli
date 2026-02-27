#!/bin/bash
# EntryGuard revoke script for iptables
# Usage: revoke.sh <CIDR> <description>
set -euo pipefail

CIDR="$1"

# Remove all matching rules for this CIDR on port 443
while iptables -D INPUT -s "$CIDR" -p tcp --dport 443 -j ACCEPT 2>/dev/null; do
    true
done
echo "Removed iptables rules for $CIDR on port 443"
