#!/bin/bash
# EntryGuard revoke script for nginx allow-list
# Usage: revoke.sh <CIDR> <description>
# Manages /etc/nginx/conf.d/entryguard-allowlist.conf
set -euo pipefail

CIDR="$1"
ALLOWLIST="/etc/nginx/conf.d/entryguard-allowlist.conf"

# Remove the CIDR from the allowlist
if [ -f "$ALLOWLIST" ]; then
    sed -i "/allow ${CIDR//\//\\/};/d" "$ALLOWLIST"
    nginx -t && nginx -s reload
    echo "Removed $CIDR from nginx allowlist"
else
    echo "Allowlist file not found, nothing to revoke"
fi
