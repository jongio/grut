<#
.SYNOPSIS
    Generate screenshot PNGs for the grut website.

.DESCRIPTION
    Builds and runs the screenshot generator which drives the TUI through
    every visual state using demo data, captures the ANSI output,
    converts it to styled HTML, then uses Playwright to render each HTML
    file as a PNG image.

    Output goes to web/public/screenshots/ by default.

.PARAMETER OutDir
    Override the output directory (default: web/public/screenshots).

.EXAMPLE
    .\screenshots.ps1
    .\screenshots.ps1 -OutDir .\my-shots
#>

param(
    [string]$OutDir
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Push-Location "$PSScriptRoot\.."
try {
    # Step 1: Build the Go screenshot capture tool.
    Write-Host "Building screenshot generator..." -ForegroundColor Cyan
    go build -tags screenshots -o "$env:TEMP\grut-screenshots.exe" ./cmd/screenshots/
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Build failed."
        exit 1
    }

    $goArgs = @()
    if ($OutDir) { $goArgs += "--out", $OutDir }

    # Step 2: Run the Go binary to capture TUI states as HTML.
    Write-Host "Capturing TUI states as HTML..." -ForegroundColor Cyan
    & "$env:TEMP\grut-screenshots.exe" @goArgs
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Capture failed."
        exit 1
    }

    # Step 3: Render HTML to PNG with Playwright.
    $renderDir = if ($OutDir) { $OutDir } else { "web\public\screenshots" }

    # Install npm dependencies if needed.
    if (-not (Test-Path "cmd\screenshots\node_modules")) {
        Write-Host "Installing Playwright dependencies..." -ForegroundColor Cyan
        Push-Location "cmd\screenshots"
        npm install
        Pop-Location
    }

    Write-Host "Rendering PNGs with Playwright..." -ForegroundColor Cyan
    node cmd/screenshots/render.mjs $renderDir
    if ($LASTEXITCODE -ne 0) {
        Write-Error "PNG rendering failed."
        exit 1
    }

    # Clean up the temporary binary.
    Remove-Item "$env:TEMP\grut-screenshots.exe" -ErrorAction SilentlyContinue

    Write-Host "Done." -ForegroundColor Green
}
finally {
    Pop-Location
}
