# Bumps github.com/ikermy/air-common[/vN] in every service and verifies the workspace.
# Usage: pwsh ./scripts/bump-air-common.ps1 -Version v2.0.1
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

$major = [int]($Version.TrimStart('v').Split('.')[0])
$modulePath = if ($major -ge 2) { "github.com/ikermy/air-common/v$major" } else { "github.com/ikermy/air-common" }
$oldPattern = 'github\.com/ikermy/air-common(?:/v\d+)?'

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

    Write-Host "=== $module -> $modulePath@$Version ===" -ForegroundColor Cyan

    if ($major -ge 2) {
        Get-ChildItem $dir -Recurse -File -Include *.go,*.proto -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -notmatch '\\\.git\\' } |
            ForEach-Object {
                $text = [IO.File]::ReadAllText($_.FullName)
                $new = [regex]::Replace($text, $oldPattern, $modulePath)
                if ($new -ne $text) {
                    [IO.File]::WriteAllText($_.FullName, $new, (New-Object System.Text.UTF8Encoding($false)))
                }
            }
    }

    & go -C $dir mod edit "-require=$modulePath@$Version"
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAILED: go mod edit in $module" -ForegroundColor Red
        exit 1
    }
    # Drop a stale v1 requirement when switching to /v2.
    if ($major -ge 2) {
        & go -C $dir mod edit "-droprequire=github.com/ikermy/air-common"
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
