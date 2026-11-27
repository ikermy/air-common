# Generates Go (and optionally TypeScript) from proto/air/v1/air.proto.
#
# Prerequisites:
#   protoc, protoc-gen-go, protoc-gen-go-grpc on PATH.
#   For TypeScript: npm i -D protoc-gen-js ts-protoc-gen (in air_front) and pass -TS.
param(
    [string]$Proto = "proto/air/v1/air.proto",
    [string]$GoOut = "gen/go",
    [string]$TSOut = "",
    [switch]$TS
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot

if (-not (Test-Path (Join-Path $env:GOROOT "src\runtime")) -and (Test-Path "C:\Program Files\Go\src\runtime")) {
    $env:GOROOT = "C:\Program Files\Go"
}
$env:Path = "$env:GOROOT\bin;$env:Path"

Push-Location $repo
try {
    if (-not (Get-Command protoc -ErrorAction SilentlyContinue)) { throw "protoc not found" }
    if (-not (Get-Command protoc-gen-go -ErrorAction SilentlyContinue)) { throw "protoc-gen-go not found" }
    if (-not (Get-Command protoc-gen-go-grpc -ErrorAction SilentlyContinue)) { throw "protoc-gen-go-grpc not found" }

    New-Item -ItemType Directory -Force -Path (Join-Path $repo $GoOut) | Out-Null

    & protoc -I proto `
        "--go_out=$GoOut" "--go_opt=paths=source_relative" `
        "--go-grpc_out=$GoOut" "--go-grpc_opt=paths=source_relative" `
        $Proto
    if ($LASTEXITCODE -ne 0) { throw "protoc (Go) failed" }

    if ($TS) {
        if (-not $TSOut) { $TSOut = "gen/ts" }
        New-Item -ItemType Directory -Force -Path (Join-Path $repo $TSOut) | Out-Null
        & protoc -I proto "--js_out=import_style=commonjs,binary:$TSOut" $Proto
        if ($LASTEXITCODE -ne 0) { throw "protoc (JS) failed" }
        & protoc -I proto "--ts_out=$TSOut" $Proto
        if ($LASTEXITCODE -ne 0) { throw "protoc (TS) failed" }
    }

    Write-Host "Generated: $Proto" -ForegroundColor Green
}
finally {
    Pop-Location
}
