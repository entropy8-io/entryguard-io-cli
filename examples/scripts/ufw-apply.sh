#!/bin/bash
# EntryGuard apply script for UFW
# Usage: apply.sh <CIDR> <description>
set -euo pipefail

CIDR="$1"
DESC="${2:-entryguard}"

ufw allow from "$CIDR" to any port 443 comment "entryguard: $DESC"
echo "Allowed $CIDR on port 443"
