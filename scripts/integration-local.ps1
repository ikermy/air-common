# Runs the self-contained repository integration tests against the local dev DB.
# Usage:
#   pwsh ./scripts/integration-local.ps1
#   pwsh ./scripts/integration-local.ps1 -Run "TestFinalUserMigrationLocal"
#
# Prerequisites: air_db container (build/air-db.yml) is running on 127.0.0.1:3307
# and the air_orchestrator repository is checked out next to air-common.
param(
    [string]$Run = "TestFinalUserMigrationLocal|TestRetentionLocal|TestSoftDeleteAndPurgeLocal|TestDBVerifySelfLocal",
    [string]$DbHost = "127.0.0.1:3307",
    [string]$DbName = "air",
    [string]$DbUser = "root",
    [string]$DbPassword = "lenin2k"
)

$ErrorActionPreference = "Stop"
$airCommon = Split-Path -Parent $PSScriptRoot
$orchestrator = Join-Path (Split-Path -Parent $airCommon) "air_orchestrator"

if (-not (Test-Path $orchestrator)) {
    throw "air_orchestrator not found at $orchestrator"
}
if (-not (Test-Path (Join-Path $env:GOROOT "src\runtime")) -and (Test-Path "C:\Program Files\Go\src\runtime")) {
    $env:GOROOT = "C:\Program Files\Go"
}
$env:Path = "$env:GOROOT\bin;$env:Path"

$container = docker ps --format "{{.Names}}" | Select-String -SimpleMatch "air_db"
if (-not $container) {
    throw "air_db container is not running. Start it: docker compose -f $orchestrator/build/air-db.yml up -d"
}

$env:DB_HOST = $DbHost
$env:DB_NAME = $DbName
$env:DB_USER = $DbUser
$env:DB_PASSWORD = $DbPassword
$env:IDGEN_ENABLED = "true"
$env:USER_MIGRATION_DEADLINE = "2099-01-01T00:00:00Z"

go test -C $orchestrator -tags integration -count=1 -run $Run -v ./internal/repository/mysql/
exit $LASTEXITCODE
