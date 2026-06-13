<#
.SYNOPSIS
    End-to-end TalkGo network test automation (PowerShell/Windows).

.DESCRIPTION
    1. (Optional) Apply network profile via simulate-4g.ps1
    2. Build and start the TalkGo server in background
    3. Run loadgen against the server for the specified duration
    4. Parse server logs for chunk_latency metrics
    5. Generate consolidated JSON report
    6. Clean up (kill server, reset network)

.PARAMETER Profile
    Network profile name (default: wifi-home). Use -ShowProfiles to list.

.PARAMETER Duration
    Test duration (default: 60s). Format: 30s, 60s, 120s.

.PARAMETER Output
    Report output path (default: ./report-<timestamp>.json).

.PARAMETER SkipSimulation
    Skip network profile application (baseline test).

.PARAMETER ShowProfiles
    List available network profiles and exit.

.EXAMPLE
    PS> .\run-test-session.ps1 -Profile 4g -Duration 30s
    30-second test with 4G network simulation.

.EXAMPLE
    PS> .\run-test-session.ps1 -Duration 10s -SkipSimulation
    Baseline test without any network simulation.

.EXAMPLE
    PS> .\run-test-session.ps1 -ShowProfiles
    List available network profiles.

.NOTES
    Requires:
      - Go 1.23+ in PATH
      - PowerShell 7+ (for proper JSON parsing)
      - Administrator rights (if applying network simulation)
    For full network simulation (latency, loss, jitter), use WSL2 or Linux.
#>

param(
    [string]$Profile = "wifi-home",
    [string]$Duration = "60s",
    [string]$Output = "",
    [switch]$SkipSimulation,
    [switch]$ShowProfiles
)

# ── Help text ───────────────────────────────────────────────────────────────
$usage = @"
TalkGo Run Test Session — End-to-end network test automation (PowerShell)

USAGE:
  .\run-test-session.ps1 [flags]

FLAGS:
  -Profile <name>       Network profile (default: wifi-home)
  -Duration <duration>  Test duration (default: 60s, e.g. 30s, 120s)
  -Output <path>        Report output path
  -SkipSimulation       Skip network simulation (baseline test)
  -ShowProfiles         List available network profiles
  -Help                 Show this help

EXAMPLES:
  .\run-test-session.ps1 -Profile 4g -Duration 30s
  .\run-test-session.ps1 -Duration 10s -SkipSimulation
  .\run-test-session.ps1 -ShowProfiles

REQUIREMENTS:
  - Go 1.23+ in PATH
  - PowerShell 7+ (recommended)
  - Administrator rights (if using -Profile)
"@

if ($ShowProfiles) {
    & "$PSScriptRoot\simulate-4g.ps1" -ShowProfiles
    exit 0
}

# ── Colours (PowerShell 7+) ─────────────────────────────────────────────────
function Write-Info  { Write-Host "[INFO]  $($args -join ' ')" -ForegroundColor Cyan }
function Write-Ok    { Write-Host "[OK]    $($args -join ' ')" -ForegroundColor Green }
function Write-Warn  { Write-Host "[WARN]  $($args -join ' ')" -ForegroundColor Yellow }
function Write-Error { Write-Host "[ERROR] $($args -join ' ')" -ForegroundColor Red }

# ── Paths ───────────────────────────────────────────────────────────────────
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectDir = Resolve-Path "$scriptDir/../.."
$timestamp = Get-Date -Format "yyyyMMddTHHmmss"
if (-not $Output) {
    $Output = "report-$timestamp.json"
}
$serverBin = "$env:TEMP\talkgo-server-$timestamp.exe"
$loadgenBin = "$env:TEMP\loadgen-$timestamp.exe"
$serverLog = "$env:TEMP\talkgo-server-$timestamp.log"
$loadgenLog = "$env:TEMP\loadgen-$timestamp.json"

# ── Ensure Go is available ─────────────────────────────────────────────────
$goPath = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $goPath) {
    Write-Error "Go is required. Install Go 1.23+ and ensure it is in PATH."
    exit 1
}
Write-Info "Using Go: $goPath"

# ── Timer helper ───────────────────────────────────────────────────────────
function Convert-DurationToSeconds {
    param([string]$Dur)
    if ($Dur -match '^(\d+)s$') {
        return [int]$matches[1]
    }
    return 60
}
$durationSec = Convert-DurationToSeconds $Duration

# ── Cleanup ────────────────────────────────────────────────────────────────
$serverProcess = $null
$script:exitCode = 0

function Invoke-Cleanup {
    # Kill server if running.
    if ($serverProcess -and (-not $serverProcess.HasExited)) {
        Write-Info "Stopping server..."
        $serverProcess.Kill()
        $serverProcess.Dispose()
        Write-Ok "Server stopped."
    }

    # Reset network if applied.
    if (-not $SkipSimulation) {
        Write-Info "Resetting network simulation..."
        & "$scriptDir\simulate-4g.ps1" -Reset 2>$null
        Write-Ok "Network simulation reset."
    }

    # Clean temp files.
    if (Test-Path $serverLog) { Remove-Item $serverLog -Force -ErrorAction SilentlyContinue }
    if (Test-Path $serverBin) { Remove-Item $serverBin -Force -ErrorAction SilentlyContinue }
    if (Test-Path $loadgenBin) { Remove-Item $loadgenBin -Force -ErrorAction SilentlyContinue }
}

$cleanupRegistered = $false
Register-EngineEvent -SourceIdentifier PowerShell.Exiting -Action {
    Invoke-Cleanup
} | Out-Null

# ── Step 0: Apply network profile ─────────────────────────────────────────
if (-not $SkipSimulation) {
    Write-Info "Applying network profile '$Profile'..."
    $simResult = & "$scriptDir\simulate-4g.ps1" -Profile $Profile
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to apply network profile '$Profile'."
        exit 1
    }
    Write-Ok "Network profile '$Profile' applied."
} else {
    Write-Info "Skipping network simulation (baseline mode)."
}

try {
    # ── Step 1: Build server ───────────────────────────────────────────────
    Write-Info "Building server binary..."
    $buildResult = & go build -o $serverBin ./cmd/server 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Server build failed: $buildResult"
        exit 1
    }
    Write-Ok "Server binary built: $serverBin"

    # ── Step 2: Build loadgen ──────────────────────────────────────────────
    Write-Info "Building loadgen binary..."
    $buildResult = & go build -o $loadgenBin ./cmd/loadgen 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Loadgen build failed: $buildResult"
        exit 1
    }
    Write-Ok "Loadgen binary built: $loadgenBin"

    # ── Step 3: Start server in background ─────────────────────────────────
    Write-Info "Starting server (log: $serverLog)..."
    $serverProcess = Start-Process -FilePath $serverBin -NoNewWindow -RedirectStandardOutput $serverLog -RedirectStandardError $serverLog -PassThru
    Write-Info "Server PID: $($serverProcess.Id)"

    # Wait for server to be ready (up to 15 seconds).
    Write-Info "Waiting for server to be ready..."
    $ready = $false
    for ($i = 1; $i -le 30; $i++) {
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:8080/health" -UseBasicParsing -TimeoutSec 1 -ErrorAction Stop
            if ($response.StatusCode -eq 200) {
                Write-Ok "Server is ready (attempt $i)."
                $ready = $true
                break
            }
        } catch {
            # Server not ready yet.
        }
        if ($i -eq 30) {
            Write-Error "Server did not start within 15 seconds."
            exit 1
        }
        Start-Sleep -Milliseconds 500
    }

    # ── Step 4: Run loadgen ────────────────────────────────────────────────
    Write-Info "Running loadgen for $Duration ..."
    $loadgenResult = & $loadgenBin -server localhost:8080 -duration $Duration -profile $Profile -output $loadgenLog 2>&1
    $loadgenExit = $LASTEXITCODE

    if ($loadgenExit -ne 0) {
        Write-Warn "Loadgen exited with code $loadgenExit."
        if (Test-Path $loadgenLog) {
            $loadgenReportRaw = Get-Content $loadgenLog -Raw -ErrorAction SilentlyContinue
        } else {
            $loadgenReportRaw = "{}"
        }
    } else {
        Write-Ok "Loadgen completed."
        if (Test-Path $loadgenLog) {
            $loadgenReportRaw = Get-Content $loadgenLog -Raw
        } else {
            $loadgenReportRaw = "{}"
        }
    }

    # Parse loadgen report.
    $loadgenReport = $loadgenReportRaw | ConvertFrom-Json -ErrorAction SilentlyContinue
    if (-not $loadgenReport) { $loadgenReport = @{} }

    # ── Step 5: Parse server logs ──────────────────────────────────────────
    $serverStats = @{
        total_chunks     = 0
        chunks_ok        = 0
        chunks_error     = 0
        error_rate_pct   = 0.0
        latency_p50_ms   = 0
        latency_p90_ms   = 0
        min_chunk_ms     = 0
        max_chunk_ms     = 0
        total_chunks_AtoB = 0
        total_chunks_BtoA = 0
    }

    if (Test-Path $serverLog) {
        Write-Info "Parsing server logs..."

        $logLines = Get-Content $serverLog -ErrorAction SilentlyContinue
        $chunkLatencies = @()
        $chunksOk = 0
        $chunksError = 0
        $chunksAtoB = 0
        $chunksBtoA = 0

        foreach ($line in $logLines) {
            try {
                $entry = $line | ConvertFrom-Json -ErrorAction SilentlyContinue
                if (-not $entry) { continue }

                if ($entry.msg -eq "chunk_latency") {
                    $totalMs = [int]($entry.total_ms -as [int])
                    if ($totalMs -gt 0) { $chunkLatencies += $totalMs }

                    if ($entry.status -eq "ok") { $chunksOk++ }
                    elseif ($entry.status -eq "error") { $chunksError++ }

                    if ($entry.half -eq "AtoB") { $chunksAtoB++ }
                    elseif ($entry.half -eq "BtoA") { $chunksBtoA++ }
                }
            } catch {
                # Skip malformed log lines.
            }
        }

        $totalChunks = $chunkLatencies.Count
        if ($totalChunks -gt 0) {
            $sorted = $chunkLatencies | Sort-Object
            $serverStats.total_chunks = $totalChunks
            $serverStats.chunks_ok = $chunksOk
            $serverStats.chunks_error = $chunksError
            $serverStats.error_rate_pct = [math]::Round(($chunksError * 100.0 / $totalChunks), 2)
            $serverStats.min_chunk_ms = $sorted[0]
            $serverStats.max_chunk_ms = $sorted[-1]
            $serverStats.latency_p50_ms = $sorted[[math]::Floor($totalChunks * 50 / 100)]
            $serverStats.latency_p90_ms = $sorted[[math]::Floor($totalChunks * 90 / 100)]
            $serverStats.total_chunks_AtoB = $chunksAtoB
            $serverStats.total_chunks_BtoA = $chunksBtoA
        }
        Write-Ok "Server logs parsed ($totalChunks chunk_latency entries)."
    } else {
        Write-Warn "Server log file not found: $serverLog"
    }

    # ── Step 6: Determine status ───────────────────────────────────────────
    $overallStatus = "ok"
    $notes = @()

    # Extract loadgen values.
    $lgAvgRtt = [double]($loadgenReport.avg_rtt_ms -as [double])
    $lgP50Rtt = [double]($loadgenReport.p50_rtt_ms -as [double])
    $lgP90Rtt = [double]($loadgenReport.p90_rtt_ms -as [double])
    $lgLoss = [double]($loadgenReport.packet_loss_pct -as [double])
    $lgStatus = [string]($loadgenReport.status -as [string])

    if ($lgStatus -eq "failed") {
        $overallStatus = "failed"
        $notes += "loadgen reported failure"
    }

    if ($serverStats.error_rate_pct -gt 15) {
        $overallStatus = "failed"
        $notes += "server error rate $($serverStats.error_rate_pct)% exceeds 15% threshold"
    } elseif ($serverStats.latency_p90_ms -gt 2500) {
        $overallStatus = "failed"
        $notes += "server p90 latency $($serverStats.latency_p90_ms)ms exceeds 2500ms threshold"
    } elseif ($serverStats.error_rate_pct -gt 5) {
        if ($overallStatus -ne "failed") { $overallStatus = "degraded" }
        $notes += "server error rate $($serverStats.error_rate_pct)% exceeds 5% threshold"
    } elseif ($serverStats.latency_p90_ms -gt 1500) {
        if ($overallStatus -ne "failed") { $overallStatus = "degraded" }
        $notes += "server p90 latency $($serverStats.latency_p90_ms)ms exceeds 1500ms threshold"
    }

    # ── Step 7: Generate consolidated report ────────────────────────────────
    $report = [PSCustomObject]@{
        timestamp   = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
        profile     = $Profile
        duration_sec = $durationSec
        server_logs = $serverStats
        loadgen     = @{
            avg_rtt_ms     = $lgAvgRtt
            p50_rtt_ms     = $lgP50Rtt
            p90_rtt_ms     = $lgP90Rtt
            packet_loss_pct = $lgLoss
        }
        status      = $overallStatus
        notes       = $notes
    }

    $reportJson = $report | ConvertTo-Json -Depth 10
    $reportJson | Out-File -FilePath $Output -Encoding utf8
    Write-Ok "Report written to $Output"
    Write-Host ""
    Write-Host $reportJson
    Write-Host ""

    Write-Info "Results: status=$overallStatus, profile=$Profile, duration=$Duration"
    Write-Info "Server: $($serverStats.chunks_ok) ok / $($serverStats.chunks_error) error chunks, p50=$($serverStats.latency_p50_ms)ms, p90=$($serverStats.latency_p90_ms)ms"
    Write-Info "Loadgen: avg_rtt=${lgAvgRtt}ms, p50=${lgP50Rtt}ms, p90=${lgP90Rtt}ms, loss=${lgLoss}%"

    if ($overallStatus -eq "failed") {
        $script:exitCode = 1
    }

    # Cleanup before exit.
    Invoke-Cleanup
} catch {
    Write-Error "Test session failed: $_"
    Invoke-Cleanup
    exit 1
}

exit $script:exitCode
