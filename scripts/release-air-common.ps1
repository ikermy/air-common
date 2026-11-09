# Prepares a local air-common release without touching master or prod.
# Usage:
#   pwsh ./scripts/release-air-common.ps1 -Version v1.61.0
#   pwsh ./scripts/release-air-common.ps1 -Version v1.61.0 -Push
#
# The script:
#   1. refuses to run on master/main and on a dirty tree (unless -AllowDirty);
#   2. validates that the requested major matches the module path (/v2 for v2+);
#   3. runs scripts/verify-all.ps1;
#   4. creates an annotated local tag (and optionally pushes it with -Push).
# It never deploys to prod and never bumps service go.mod files — use
# scripts/bump-air-common.ps1 after the tag is published.
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v\d+\.\d+\.\d+$')]
    [string]$Version,
    [string]$Branch = "new-arch",
    [switch]$Push,
    [switch]$AllowDirty,
    [switch]$SkipVerify
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot

if (-not (Test-Path (Join-Path $env:GOROOT "src\runtime"))) {
    if (Test-Path "C:\Program Files\Go\src\runtime") {
        $env:GOROOT = "C:\Program Files\Go"
    }
}
$env:Path = "$env:GOROOT\bin;$env:Path"

$current = (git -C $repo branch --show-current).Trim()
if ($current -in @("master", "main")) {
    throw "Refusing to release from '$current'. Switch to '$Branch'."
}
if ($current -ne $Branch) {
    throw "Expected branch '$Branch', current branch is '$current'."
}

$status = git -C $repo status --porcelain
if ($status -and -not $AllowDirty) {
    throw "Working tree is dirty. Commit/stash changes or pass -AllowDirty."
}

$module = (Select-String -Path (Join-Path $repo "go.mod") -Pattern '^module\s+(.+)$').Matches[0].Groups[1].Value.Trim()
$major = [int]($Version.TrimStart('v').Split('.')[0])
if ($major -ge 2 -and $module -notmatch "/v$major$") {
    throw "Module path '$module' has no /v$major suffix; Go will reject tag $Version. Use a v1.x tag or change the module path."
}

if (git -C $repo tag --list $Version) {
    throw "Tag $Version already exists locally."
}

if (-not $SkipVerify) {
    Write-Host "=== verify-all ===" -ForegroundColor Cyan
    & (Join-Path $PSScriptRoot "verify-all.ps1")
    if ($LASTEXITCODE -ne 0) {
        throw "verify-all failed; tag not created."
    }
}

git -C $repo tag -a $Version -m "air-common $Version"
if ($LASTEXITCODE -ne 0) {
    throw "git tag failed."
}

Write-Host "Created local tag $Version ($module)." -ForegroundColor Green
Write-Host "Next steps:"
Write-Host "  1. Review the tag: git show $Version"
Write-Host "  2. Push when ready: git push origin $Branch --follow-tags   (or re-run with -Push)"
Write-Host "  3. Bump services: pwsh ./scripts/bump-air-common.ps1 -Version $Version"
Write-Host "  4. Deploy only through the normal prod release process (not this script)."

if ($Push) {
    git -C $repo push origin $Branch --follow-tags
    if ($LASTEXITCODE -ne 0) {
        throw "git push failed."
    }
    Write-Host "Pushed $Branch and tag $Version." -ForegroundColor Green
}
