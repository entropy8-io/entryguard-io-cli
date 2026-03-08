# EntryGuard - Windows Firewall Revoke Script
# Removes the inbound allow rule for the given CIDR
param(
    [Parameter(Mandatory=$true)][string]$CIDR,
    [string]$Description = "entryguard"
)

$ErrorActionPreference = "Stop"

$RuleName = "EntryGuard-$CIDR"

$existing = Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue
if ($existing) {
    Remove-NetFirewallRule -DisplayName $RuleName
    Write-Output "Revoked $CIDR (removed rule: $RuleName)"
} else {
    Write-Output "Rule $RuleName not found, nothing to revoke"
}
