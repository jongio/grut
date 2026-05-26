# Installer for grüt — a terminal file explorer with Git integration.
#
# Usage:
#   irm https://raw.githubusercontent.com/jongio/grut/main/install.ps1 | iex
#   $v="v0.1.0"; irm https://raw.githubusercontent.com/jongio/grut/main/install.ps1 | iex
#   $env:VERSION = "v0.1.0"; irm https://raw.githubusercontent.com/jongio/grut/main/install.ps1 | iex
#   .\install.ps1 -Version v0.1.0
#
# Parameters:
#   -Version     Version to install (e.g. v0.1.0, 0.1.0). Defaults to latest.
#
# Environment variables:
#   VERSION  Override the version to install (e.g. v0.1.0). Defaults to latest.
#            The -Version parameter and $v variable take precedence.

$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
$Script:Repo            = 'jongio/grut'
$Script:BinaryName      = 'grut.exe'
$Script:ChecksumsFile   = 'grut_checksums.txt'
$Script:GitHubDownload  = "https://github.com/$Script:Repo/releases/download"

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------
function Write-Status  { param([string]$Message) Write-Host "  -> $Message" -ForegroundColor Cyan }
function Write-Success { param([string]$Message) Write-Host "  OK $Message" -ForegroundColor Green }
function Write-Warn    { param([string]$Message) Write-Host "  !! $Message" -ForegroundColor Yellow }
function Write-Fail    { param([string]$Message) throw $Message }

# ---------------------------------------------------------------------------
# Architecture detection
# ---------------------------------------------------------------------------
function Get-GrutArch {
    # Prefer .NET RuntimeInformation (PS 6+, or PS 5.1 on .NET 4.7.1+).
    try {
        $procArch = [System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture
        switch ($procArch.ToString()) {
            'X64'   { return 'amd64' }
            'Arm64' { return 'arm64' }
            default { Write-Fail "Unsupported process architecture: $procArch" }
        }
    }
    catch {
        # Fallback for older .NET Framework — use environment variable.
        switch ($env:PROCESSOR_ARCHITECTURE) {
            'AMD64' { return 'amd64' }
            'ARM64' { return 'arm64' }
            'x86'   { Write-Fail '32-bit Windows is not supported. Please use 64-bit PowerShell.' }
            default { Write-Fail "Unsupported processor architecture: $env:PROCESSOR_ARCHITECTURE" }
        }
    }
}

# ---------------------------------------------------------------------------
# Version resolution
# ---------------------------------------------------------------------------
function Get-GrutVersion {
    # Priority: $v variable (one-liner friendly) > $env:VERSION > latest from GitHub.
    # $v is set by: $v="v0.1.0"; irm ... | iex
    $requestedVersion = if ($v) { $v } elseif ($env:VERSION) { $env:VERSION } else { $null }

    if ($requestedVersion) {
        $ver = $requestedVersion.ToString().Trim()
        # Normalise: ensure the tag starts with "v".
        if (-not $ver.StartsWith('v')) { $ver = "v$ver" }
        return $ver
    }

    Write-Status 'Querying GitHub for latest release...'

    # Use the releases/latest redirect - no auth required, no API rate limit.
    $redirectUrl = "https://github.com/$Script:Repo/releases/latest"
    try {
        $response = Invoke-WebRequest -Uri $redirectUrl -UseBasicParsing -MaximumRedirection 5
        $finalUrl = $response.BaseResponse.RequestMessage.RequestUri.ToString()
        if (-not $finalUrl) {
            # PS 5.1: ResponseUri is on BaseResponse directly.
            $finalUrl = $response.BaseResponse.ResponseUri.ToString()
        }
        if ($finalUrl -match '/releases/tag/(?<tag>v[^/?#]+)') {
            return $Matches.tag
        }
    }
    catch {}

    Write-Fail 'Could not determine the latest version. Set $env:VERSION and retry.'
}

# ---------------------------------------------------------------------------
# PATH management
# ---------------------------------------------------------------------------
function Add-ToUserPath {
    param([string]$Directory)

    $normalised = $Directory.TrimEnd('\')

    # --- Persistent user PATH (registry) ---
    $userPath = [System.Environment]::GetEnvironmentVariable('PATH', [System.EnvironmentVariableTarget]::User)
    if (-not $userPath) { $userPath = '' }

    $entries = $userPath -split ';' |
        ForEach-Object { $_.Trim().TrimEnd('\') } |
        Where-Object  { $_ -ne '' }

    if ($entries -and ($entries | Where-Object { $_ -ieq $normalised })) {
        Write-Status 'Install directory already in user PATH'
    }
    else {
        $newPath = if ($userPath.TrimEnd(';')) { "$($userPath.TrimEnd(';'));$Directory" } else { $Directory }
        [System.Environment]::SetEnvironmentVariable('PATH', $newPath, [System.EnvironmentVariableTarget]::User)
        Write-Success 'Added to user PATH'
    }

    # --- Current-session PATH ---
    $sessionEntries = $env:PATH -split ';' |
        ForEach-Object { $_.Trim().TrimEnd('\') } |
        Where-Object  { $_ -ne '' }

    if (-not ($sessionEntries | Where-Object { $_ -ieq $normalised })) {
        $env:PATH = "$env:PATH;$Directory"
    }
}

# ---------------------------------------------------------------------------
# Main installer
# ---------------------------------------------------------------------------
function Install-Grut {
    # Ensure TLS 1.2 — PowerShell 5.1 defaults to TLS 1.0 which GitHub rejects.
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    # Suppress the progress bar so Invoke-WebRequest doesn't crawl in PS 5.1.
    $prevProgressPref  = $ProgressPreference
    $ProgressPreference = 'SilentlyContinue'

    $installDir = Join-Path $env:LOCALAPPDATA 'Programs\grut'
    $tempDir    = Join-Path ([System.IO.Path]::GetTempPath()) "grut-install-$([guid]::NewGuid().ToString('N').Substring(0, 8))"

    try {
        Write-Host ''
        Write-Host '  grut Installer' -ForegroundColor White
        Write-Host ''

        # ---- Platform --------------------------------------------------------
        $arch = Get-GrutArch
        Write-Status "Detected platform: windows/$arch"

        # ---- Version ---------------------------------------------------------
        $tag     = Get-GrutVersion
        $version = $tag.TrimStart('v')          # strip leading "v" for filename
        Write-Success "Version: $tag"

        # ---- Build URLs ------------------------------------------------------
        $archiveName   = "grut_${version}_windows_${arch}.zip"
        $archiveUrl    = "$Script:GitHubDownload/$tag/$archiveName"
        $checksumsUrl  = "$Script:GitHubDownload/$tag/$Script:ChecksumsFile"

        # ---- Temp directory --------------------------------------------------
        New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
        $archivePath   = Join-Path $tempDir $archiveName
        $checksumsPath = Join-Path $tempDir $Script:ChecksumsFile

        # ---- Download --------------------------------------------------------
        Write-Status "Downloading $archiveName..."
        Invoke-WebRequest -Uri $archiveUrl -OutFile $archivePath -UseBasicParsing

        Write-Status "Downloading checksums..."
        Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing
        Write-Success 'Downloaded archive'

        # ---- Verify checksum -------------------------------------------------
        Write-Status 'Verifying SHA-256 checksum...'

        # Parse the checksums file for the matching archive entry.
        $checksumLine = Get-Content $checksumsPath |
            Where-Object {
                $fields = $_.Trim() -split '\s+'
                $fields.Count -ge 2 -and $fields[-1] -eq $archiveName
            } |
            Select-Object -First 1

        if (-not $checksumLine) {
            Write-Fail "Archive '$archiveName' not found in $($Script:ChecksumsFile)."
        }

        $expectedHash = ($checksumLine.Trim() -split '\s+')[0]
        $actualHash   = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash

        if ($actualHash -ine $expectedHash) {
            Write-Fail "Checksum mismatch!`n  Expected: $expectedHash`n  Got:      $actualHash"
        }
        Write-Success 'Checksum verified'

        # ---- Extract ---------------------------------------------------------
        Write-Status 'Extracting...'
        $extractDir = Join-Path $tempDir 'extract'
        Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force

        # Find the binary (handles both flat and nested archive layouts).
        $binary = Get-ChildItem -Path $extractDir -Filter $Script:BinaryName -Recurse |
            Select-Object -First 1

        if (-not $binary) {
            Write-Fail "$($Script:BinaryName) not found in the downloaded archive."
        }
        Write-Success "Extracted $($Script:BinaryName)"

        # ---- Install ---------------------------------------------------------
        if (-not (Test-Path $installDir)) {
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        }

        Copy-Item -Path $binary.FullName -Destination (Join-Path $installDir $Script:BinaryName) -Force
        Write-Success "Installed to $installDir"

        # ---- PATH ------------------------------------------------------------
        Add-ToUserPath -Directory $installDir

        # ---- Verify installation ---------------------------------------------
        $grutCmd = Get-Command grut -ErrorAction SilentlyContinue
        if ($grutCmd) {
            $installedVersion = & grut --version 2>$null
            if ($installedVersion) {
                Write-Success "Verified: $installedVersion"
            }
        }

        # ---- Done ------------------------------------------------------------
        Write-Host ''
        Write-Host "  grut $tag installed successfully!" -ForegroundColor Green
        Write-Host '  Run ''grut'' to get started.' -ForegroundColor Cyan
        Write-Host ''
        Write-Warn 'Restart your terminal if ''grut'' is not recognized.'
        Write-Host ''
    }
    catch {
        Write-Host ''
        Write-Host "  Error: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host ''
        # Re-throw so the caller sees a non-zero exit / terminating error.
        # Without this, 'irm | iex' silently succeeds even on failure.
        throw
    }
    finally {
        # Clean up temp files.
        if (Test-Path $tempDir) {
            Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        }
        # Restore progress preference.
        $ProgressPreference = $prevProgressPref
    }
}

# Entry point — supports both direct execution and piped (irm | iex) usage.
Install-Grut
