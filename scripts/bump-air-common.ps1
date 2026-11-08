# Bumps github.com/ikermy/air-common in every service and verifies the workspace.
# Usage: pwsh ./scripts/bump-air-common.ps1 -Version v1.61.0
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v\d+\.\d+\.\d+$')]
    [string]$Version,
    [switch]$SkipVerify
)

$ErrorActionPreference = "Continue"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
if (-not $root) { $root = "D:\project\marusia" }

if (-not (Test-Path (Join-Path $env:GOROOT "src\runtime"))) {
    if (Test-Path "C:\Program Files\Go\src\runtime") {
        $env:GOROOT = "C:\Program Files\Go"
    }
}
$env:Path = "$env:GOROOT\bin;$env:Path"

$modules = @(
    "air_orchestrator", "air_whatsbot", "air_avito", "air_lead-hunter",
    "air_operator", "air_payment", "air_phantom", "air_tgbot", "air_tguserbot",
    "air_widget", "marusia_crm"
)

$updated = @()
foreach ($module in $modules) {
    $dir = Join-Path $root $module
    $gomod = Join-Path $dir "go.mod"
    if (-not (Test-Path $gomod)) { continue }
    if (-not (Select-String -Path $gomod -Pattern "github.com/ikermy/air-common" -Quiet)) { continue }

    Write-Host "=== $module -> $Version ===" -ForegroundColor Cyan
    & go -C $dir mod edit "-require=github.com/ikermy/air-common@$Version"
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAILED: go mod edit in $module" -ForegroundColor Red
        exit 1
    }
    & go -C $dir mod tidy
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAILED: go mod tidy in $module" -ForegroundColor Red
        exit 1
    }
    $updated += $module
}

if ($updated.Count -eq 0) {
    Write-Host "No modules reference github.com/ikermy/air-common" -ForegroundColor Yellow
    exit 0
}

Write-Host "Updated: $($updated -join ', ')" -ForegroundColor Green
if ($SkipVerify) { exit 0 }

& pwsh -NoProfile -File (Join-Path $PSScriptRoot "verify-all.ps1")
exit $LASTEXITCODE
