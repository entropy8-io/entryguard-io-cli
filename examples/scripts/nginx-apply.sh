#!/bin/bash
# EntryGuard apply script for nginx allow-list
# Usage: apply.sh <CIDR> <description>
# Manages /etc/nginx/conf.d/entryguard-allowlist.conf
set -euo pipefail

CIDR="$1"
ALLOWLIST="/etc/nginx/conf.d/entryguard-allowlist.conf"

# Add the CIDR to the allowlist (idempotent)
if ! grep -q "allow $CIDR;" "$ALLOWLIST" 2>/dev/null; then
    echo "allow $CIDR;" >> "$ALLOWLIST"
    nginx -t && nginx -s reload
    echo "Added $CIDR to nginx allowlist"
else
    echo "$CIDR already in nginx allowlist"
fi
