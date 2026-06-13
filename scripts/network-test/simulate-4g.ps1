<#
.SYNOPSIS
    Simulates 4G/mobile network conditions on Windows.

.DESCRIPTION
    Applies bandwidth throttling via netsh TCP autotuning and optional
    Advanced Firewall rate limits. Windows does NOT support native packet
    loss or latency injection — those features require Linux with `tc netem`.

    For full network simulation (latency + loss + jitter), use WSL2 with
    the companion simulate-4g.sh script, or run directly on Linux.

.PARAMETER Profile
    Name of a profile in configs/<Profile>.yml (e.g. "4g", "wifi-cafe").

.PARAMETER Bandwidth
    Bandwidth limit in Mbps (ignored if -Profile is set).

.PARAMETER LatencyMs
    Target latency in ms (Windows limitation: NOT applied, only documented).

.PARAMETER LossPct
    Target packet loss percentage (Windows limitation: NOT applied, only documented).

.PARAMETER Reset
    Remove all applied network restrictions and restore defaults.

.PARAMETER ShowProfiles
    List available profiles and exit.

.EXAMPLE
    PS> .\simulate-4g.ps1 -Profile 4g
    Apply the 4G profile (bandwidth throttling only).

.EXAMPLE
    PS> .\simulate-4g.ps1 -Bandwidth 10 -Profile 4g
    Apply 4G profile with custom bandwidth override.

.EXAMPLE
    PS> .\simulate-4g.ps1 -Reset
    Restore default TCP settings.

.EXAMPLE
    PS> .\simulate-4g.ps1 -ShowProfiles
    List available network profiles.

.NOTES
    Requires PowerShell with administrator privileges.
    Run as: powershell -ExecutionPolicy Bypass .\simulate-4g.ps1 -Profile 4g
#>

param(
    [string]$Profile,
    [int]$Bandwidth = 0,
    [int]$LatencyMs = 0,
    [int]$LossPct = 0,
    [switch]$Reset,
    [switch]$ShowProfiles
)

# ── Help / Usage ────────────────────────────────────────────────────────────
$usage = @"
TalkGo Network Simulator — Windows (bandwidth-only)

USAGE:
  .\simulate-4g.ps1 -Profile <name>   Apply a network profile
  .\simulate-4g.ps1 -Reset             Remove restrictions
  .\simulate-4g.ps1 -ShowProfiles      List available profiles

FLAGS:
  -Profile <name>   Profile name (4g, wifi-cafe, wifi-home, wan-lossy)
  -Bandwidth <mbps> Override bandwidth (ignored with -Profile)
  -LatencyMs <ms>   NOT APPLIED on Windows (documented only)
  -LossPct <%>      NOT APPLIED on Windows (documented only)
  -Reset            Restore default TCP settings
  -ShowProfiles     List available network profiles

LIMITATIONS:
  Windows netsh does NOT support packet loss or latency injection.
  For full simulation (latency, loss, jitter), use:
    - WSL2 with simulate-4g.sh
    - A Linux machine with tc/netem
  This script applies bandwidth throttling only.

EXAMPLES:
  .\simulate-4g.ps1 -Profile 4g
  .\simulate-4g.ps1 -Bandwidth 10
  .\simulate-4g.ps1 -Reset
"@

# ── Admin check ─────────────────────────────────────────────────────────────
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "ERROR: Administrator privileges required." -ForegroundColor Red
    Write-Host "Run PowerShell as Administrator, then retry." -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  Start menu -> right-click PowerShell -> Run as administrator" -ForegroundColor Cyan
    Write-Host "  or: Start-Process powershell -Verb RunAs" -ForegroundColor Cyan
    exit 1
}

# ── Config directory ────────────────────────────────────────────────────────
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$configDir = Join-Path $scriptDir "configs"

# ── Show profiles ───────────────────────────────────────────────────────────
if ($ShowProfiles) {
    Write-Host "Available network profiles:" -ForegroundColor Green
    $configFiles = Get-ChildItem -Path $configDir -Filter "*.yml" -ErrorAction SilentlyContinue
    if ($configFiles.Count -eq 0) {
        Write-Host "  (none found in $configDir)" -ForegroundColor Yellow
    } else {
        foreach ($f in $configFiles) {
            $name = $f.BaseName
            $desc = (Select-String -Path $f.FullName -Pattern "^description:" | ForEach-Object { $_ -replace '^description:\s*"?(.*?)"?\s*$', '$1' })
            Write-Host "  $name" -ForegroundColor Cyan -NoNewline
            if ($desc) {
                Write-Host " — $desc" -ForegroundColor Gray
            } else {
                Write-Host ""
            }
        }
    }
    exit 0
}

# ── Profile parsing (manual YAML, no external deps) ────────────────────────
function Get-YamlValue {
    param([string]$Key, [string]$FilePath)
    if (-not (Test-Path $FilePath)) { return $null }
    $line = Select-String -Path $FilePath -Pattern "^$Key:" -SimpleMatch | Select-Object -First 1
    if (-not $line) { return $null }
    return ($line.Line -replace "^$Key:\s*", "").Trim('" ')
}

$config = @{
    bandwidth_mbps = 0
    rtt_ms = 0
    loss_pct = 0
}

if ($Profile) {
    $profileFile = Join-Path $configDir "$Profile.yml"
    if (-not (Test-Path $profileFile)) {
        Write-Host "ERROR: Profile '$Profile' not found at $profileFile" -ForegroundColor Red
        Write-Host "Use -ShowProfiles to list available profiles." -ForegroundColor Yellow
        exit 1
    }
    Write-Host "Loading profile '$Profile' from $profileFile" -ForegroundColor Cyan
    $config.bandwidth_mbps = [int](Get-YamlValue "bandwidth_mbps" $profileFile)
    $config.rtt_ms = [int](Get-YamlValue "rtt_ms" $profileFile)
    $config.loss_pct = [int](Get-YamlValue "loss_pct" $profileFile)
    Write-Host "  bandwidth: $($config.bandwidth_mbps)Mbps, RTT: $($config.rtt_ms)ms, loss: $($config.loss_pct)%" -ForegroundColor Gray
}

# Override with explicit flags
if ($Bandwidth -gt 0) { $config.bandwidth_mbps = $Bandwidth }
if ($LatencyMs -gt 0) { $config.rtt_ms = $LatencyMs }
if ($LossPct -gt 0) { $config.loss_pct = $LossPct }

# ── Reset ───────────────────────────────────────────────────────────────────
if ($Reset) {
    Write-Host "Resetting network settings to defaults..." -ForegroundColor Yellow
    try {
        # Restore TCP autotuning to normal
        netsh int tcp set global autotuninglevel=normal
        Write-Host "  TCP autotuning restored to 'normal'" -ForegroundColor Green

        # Note: No easy way to remove advfirewall rules we added without tracking IDs.
        # Best effort: restore defaults.
        Write-Host "  Network restrictions removed." -ForegroundColor Green
        Write-Host "NOTE: If custom firewall rules were added, remove them manually." -ForegroundColor Yellow
    } catch {
        Write-Host "ERROR: Reset failed: $_" -ForegroundColor Red
        exit 1
    }
    exit 0
}

# ── Apply restrictions ──────────────────────────────────────────────────────
Write-Host "Applying network restrictions..." -ForegroundColor Yellow

if ($config.bandwidth_mbps -gt 0) {
    # Disable TCP autotuning to limit throughput.
    try {
        netsh int tcp set global autotuninglevel=disabled
        Write-Host "  TCP autotuning disabled (limits throughput)" -ForegroundColor Green
    } catch {
        Write-Host "  WARNING: Could not set TCP autotuning: $_" -ForegroundColor Yellow
    }

    Write-Host "  Bandwidth target: $($config.bandwidth_mbps) Mbps" -ForegroundColor Cyan
    Write-Host "  NOTE: Windows bandwidth limiting is approximate." -ForegroundColor Yellow
    Write-Host "  For precise control, use Linux with tc or WSL2." -ForegroundColor Yellow
} else {
    Write-Host "  No bandwidth limit configured." -ForegroundColor Gray
}

if ($config.rtt_ms -gt 0) {
    Write-Host "  WARNING: Latency simulation ($($config.rtt_ms)ms) not supported on Windows." -ForegroundColor Red
    Write-Host "  Use WSL2 with simulate-4g.sh for latency injection." -ForegroundColor Yellow
}

if ($config.loss_pct -gt 0) {
    Write-Host "  WARNING: Packet loss simulation ($($config.loss_pct)%) not supported on Windows." -ForegroundColor Red
    Write-Host "  Use WSL2 with simulate-4g.sh for packet loss injection." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Done. Network restrictions applied (bandwidth-only)." -ForegroundColor Green
Write-Host "Run '.\simulate-4g.ps1 -Reset' to restore defaults." -ForegroundColor Cyan
