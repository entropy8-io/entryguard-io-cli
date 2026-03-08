# EntryGuard - Windows Firewall Apply Script
# Adds an inbound allow rule for the given CIDR on port 443
param(
    [Parameter(Mandatory=$true)][string]$CIDR,
    [string]$Description = "entryguard"
)

$ErrorActionPreference = "Stop"

$RuleName = "EntryGuard-$CIDR"

# Remove existing rule if present (idempotent)
$existing = Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue
if ($existing) {
    Write-Output "$RuleName already exists, updating"
    Remove-NetFirewallRule -DisplayName $RuleName
}

New-NetFirewallRule `
    -DisplayName $RuleName `
    -Description $Description `
    -Direction Inbound `
    -Action Allow `
    -Protocol TCP `
    -LocalPort 443 `
    -RemoteAddress $CIDR `
    -Profile Any

Write-Output "Allowed $CIDR on port 443 (rule: $RuleName)"
