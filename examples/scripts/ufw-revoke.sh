#!/bin/bash
# EntryGuard revoke script for UFW
# Usage: revoke.sh <CIDR> <description>
set -euo pipefail

CIDR="$1"

# Delete the matching rule
ufw delete allow from "$CIDR" to any port 443 || true
echo "Revoked $CIDR on port 443"
