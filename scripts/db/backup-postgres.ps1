#Requires -Version 5.1
<#
.SYNOPSIS
    Syntopica PostgreSQL cold backup script.
.DESCRIPTION
    Flow: stop container -> 7z pack PGDATA -> (always) start container
    Target:  data\18\docker\  (the real PGDATA)
    Output:  backups\data-<timestamp>.7z
.NOTES
    Usage: powershell -ExecutionPolicy Bypass -File backup-postgres.ps1
#>

# ===== Config (edit as needed) =====
$ProjectRoot = 'D:\project\Syntopica'
$ComposeFile = Join-Path $ProjectRoot 'docker-compose.pg.yml'
$PgDataDir   = Join-Path $ProjectRoot 'data\18\docker'
$SevenZip    = 'D:\tool\7-Zip\7z.exe'
$Container   = 'syntopica-postgres'
$BackupDir   = Join-Path $ProjectRoot 'backups'
$WaitTimeout = 30   # max seconds to wait for container stop/start
# ===================================

$ErrorActionPreference = 'Continue'

# Invoke a native command, streaming its combined output without letting
# PowerShell's NativeCommandError (stderr -> Stop) abort the script.
function Invoke-Native {
    param([scriptblock]$Block)
    & $Block 2>&1 | ForEach-Object {
        if ($_ -is [System.Management.Automation.ErrorRecord]) { $_.Exception.Message }
        else { $_ }
    } | ForEach-Object { Write-Host "      $_" }
    return $LASTEXITCODE
}

function Write-Step($msg) { Write-Host "`n[STEP] $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "      [OK] $msg" -ForegroundColor Green }
function Write-Warn2($msg){ Write-Host "      [!!] $msg" -ForegroundColor Yellow }
function Write-Err2($msg) { Write-Host "      [XX] $msg" -ForegroundColor Red }

function Get-ContainerState {
    param([string]$Name)
    try {
        $state = docker inspect $Name --format '{{.State.Status}}' 2>$null
        return ($state -replace '"', '').Trim()
    } catch { return '' }
}

function Wait-ContainerState {
    param([string]$Name, [string]$Target, [int]$Timeout)
    $elapsed = 0
    while ($elapsed -lt $Timeout) {
        $s = Get-ContainerState -Name $Name
        if ($s -eq $Target) { return $true }
        Start-Sleep -Seconds 1
        $elapsed++
    }
    return $false
}

# ===== Timestamp =====
$ts = Get-Date -Format 'yyyyMMdd-HHmmss'
$archive = Join-Path $BackupDir "data-$ts.7z"

Write-Host "`n============================================================" -ForegroundColor White
Write-Host " Syntopica PostgreSQL Backup  $ts" -ForegroundColor White
Write-Host "============================================================" -ForegroundColor White
Write-Host " PGDATA  : $PgDataDir"
Write-Host " Archive : $archive"
Write-Host "============================================================"

# ===== Pre-flight checks =====
Write-Step 'Pre-flight checks'
$abort = $false
foreach ($p in @($SevenZip, $ComposeFile)) {
    if (-not (Test-Path $p)) { Write-Err2 "Not found: $p"; $abort = $true }
}
if (-not (Test-Path (Join-Path $PgDataDir 'PG_VERSION'))) {
    Write-Err2 "Invalid PGDATA (missing PG_VERSION): $PgDataDir"; $abort = $true
}
if ($abort) { exit 1 }
if (-not (Test-Path $BackupDir)) { New-Item -ItemType Directory -Path $BackupDir | Out-Null }
Write-Ok 'Checks passed'

# ===== Step 1: stop container =====
Write-Step "Stopping container $Container (timeout ${WaitTimeout}s)"
$null = Invoke-Native { docker compose -f $ComposeFile stop postgres }
$state = Get-ContainerState -Name $Container
if ($state -ne 'exited') {
    Write-Warn2 "compose stop did not exit (state=$state), falling back to docker stop"
    $null = Invoke-Native { docker stop $Container }
}
if (-not (Wait-ContainerState -Name $Container -Target 'exited' -Timeout $WaitTimeout)) {
    Write-Err2 "Container not stopped within ${WaitTimeout}s (state=$(Get-ContainerState -Name $Container))"
    Write-Err2 'DB may still be running. Aborting backup for safety. Inspect manually and retry.'
    exit 2
}
Write-Ok 'Container stopped'

# ===== Steps 2-3: 7z pack, then (always) start container =====
$backupOk = $false
try {
    Write-Step "Packing PGDATA -> $archive"
    # 7z exit codes: 0=ok 1=warning(acceptable) 2+=fatal
    $rc = Invoke-Native { & $SevenZip a -t7z -mx=5 -ms=on $archive "$PgDataDir\*" }
    if ($rc -ge 2) {
        Write-Err2 "7z packing failed (exit code $rc)"
    } else {
        $backupOk = $true
        $size = (Get-Item $archive).Length
        $sizeMB = [math]::Round($size / 1MB, 1)
        Write-Ok "Done: $archive ($sizeMB MB)"
        if ($rc -eq 1) { Write-Warn2 '7z returned warning code 1, archive is usually still usable' }
    }
} catch {
    Write-Err2 "Packing exception: $_"
} finally {
    # ===== Step 3: start container (always) =====
    Write-Step "Starting container $Container"
    $null = Invoke-Native { docker compose -f $ComposeFile start postgres }
    if (-not (Wait-ContainerState -Name $Container -Target 'running' -Timeout $WaitTimeout)) {
        Write-Warn2 "Container not running within ${WaitTimeout}s, inspect manually"
    } else {
        Write-Ok 'Container started, waiting for healthcheck'
        $null = Invoke-Native { docker compose -f $ComposeFile ps postgres }
    }
}

# ===== Summary =====
Write-Host "`n============================================================" -ForegroundColor White
if ($backupOk) {
    Write-Host ' RESULT: backup succeeded' -ForegroundColor Green
    Write-Host " Archive: $archive"
} else {
    Write-Host ' RESULT: backup FAILED - check log above' -ForegroundColor Red
}
Write-Host ' Container:' -ForegroundColor White
docker ps -a --filter "name=$Container" --format '       {{.Names}}  {{.Status}}'
Write-Host '============================================================' -ForegroundColor White
