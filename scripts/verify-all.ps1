# Verifies every AiR module: build + vet + unit tests.
# Usage: pwsh ./scripts/verify-all.ps1 [-Race]
param(
    [switch]$Race
)

$ErrorActionPreference = "Continue"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
if (-not $root) { $root = "D:\project\marusia" }

# Go 1.27 may still see a stale GOROOT in the environment.
if (-not (Test-Path (Join-Path $env:GOROOT "src\runtime"))) {
    if (Test-Path "C:\Program Files\Go\src\runtime") {
        $env:GOROOT = "C:\Program Files\Go"
    }
}
$env:Path = "$env:GOROOT\bin;$env:Path"

$modules = @(
    "air-common", "air-logger", "air_orchestrator",
    "air_whatsbot", "air_avito", "air_lead-hunter", "air_operator",
    "air_payment", "air_phantom", "air_tgbot", "air_widget", "marusia_crm"
)

$failed = @()
foreach ($module in $modules) {
    $dir = Join-Path $root $module
    if (-not (Test-Path (Join-Path $dir "go.mod"))) {
        Write-Host "SKIP $module (no go.mod)" -ForegroundColor Yellow
        continue
    }
    Write-Host "=== $module ===" -ForegroundColor Cyan

    & go -C $dir build ./...
    if ($LASTEXITCODE -ne 0) { $failed += "$module:build"; continue }

    & go -C $dir vet ./...
    if ($LASTEXITCODE -ne 0) { $failed += "$module:vet"; continue }

    & go -C $dir test -count=1 -timeout 300s ./...
    if ($LASTEXITCODE -ne 0) { $failed += "$module:test"; continue }

    if ($Race) {
        & go -C $dir test -race -count=1 -timeout 300s ./...
        if ($LASTEXITCODE -ne 0) { $failed += "$module:race"; continue }
    }
    Write-Host "OK $module" -ForegroundColor Green
}

# Cross-compile the Linux-only service.
$tguserbot = Join-Path $root "air_tguserbot"
if (Test-Path (Join-Path $tguserbot "go.mod")) {
    Write-Host "=== air_tguserbot (GOOS=linux build) ===" -ForegroundColor Cyan
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    & go -C $tguserbot build ./...
    if ($LASTEXITCODE -ne 0) { $failed += "air_tguserbot:build" }
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

if ($failed.Count -gt 0) {
    Write-Host "FAILED: $($failed -join ', ')" -ForegroundColor Red
    exit 1
}
Write-Host "All modules OK" -ForegroundColor Green
