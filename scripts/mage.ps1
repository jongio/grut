<#
.SYNOPSIS
    Run mage targets using a project-local mage binary in bin/.

.DESCRIPTION
    Installs mage into the project's bin/ directory (not %TEMP%) and forwards
    all arguments to it.  This avoids Windows Defender false-positive blocks
    that occur when mage compiles its temporary binary into %TEMP%.

.EXAMPLE
    .\scripts\mage.ps1 install
    .\scripts\mage.ps1 preflight
    .\scripts\mage.ps1 -l              # List available targets
#>

$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Resolve paths
# ---------------------------------------------------------------------------
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$BinDir   = Join-Path $RepoRoot 'bin'
$MageBin  = Join-Path $BinDir 'mage.exe'

# ---------------------------------------------------------------------------
# Ensure mage is installed in bin/
# ---------------------------------------------------------------------------
if (-not (Test-Path $MageBin)) {
    Write-Host '  Installing mage to bin/...' -ForegroundColor Cyan

    if (-not (Test-Path $BinDir)) {
        New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    }

    $env:GOBIN = $BinDir
    & go install github.com/magefile/mage@latest
    if ($LASTEXITCODE -ne 0) {
        throw 'Failed to install mage'
    }

    if (-not (Test-Path $MageBin)) {
        throw "mage binary not found at $MageBin after install"
    }

    Write-Host "  Installed mage to $MageBin" -ForegroundColor Green
}

# ---------------------------------------------------------------------------
# Forward all arguments to mage
# ---------------------------------------------------------------------------
& $MageBin @args
exit $LASTEXITCODE
