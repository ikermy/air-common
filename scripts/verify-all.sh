#!/usr/bin/env bash
# Verifies every AiR module: build + vet + unit tests.
# Usage: ./scripts/verify-all.sh [--race]
set -uo pipefail

RACE=0
[[ "${1:-}" == "--race" ]] && RACE=1

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODULES=(
  air-common air-logger air_orchestrator
  air_whatsbot air_avito air_lead-hunter air_operator
  air_payment air_phantom air_tgbot air_widget marusia_crm
)

failed=()
for module in "${MODULES[@]}"; do
  dir="$ROOT/$module"
  [[ -f "$dir/go.mod" ]] || { echo "SKIP $module (no go.mod)"; continue; }
  echo "=== $module ==="

  (cd "$dir" && go build ./...) || { failed+=("$module:build"); continue; }
  (cd "$dir" && go vet ./...) || { failed+=("$module:vet"); continue; }
  (cd "$dir" && go test -count=1 -timeout 300s ./...) || { failed+=("$module:test"); continue; }
  if [[ "$RACE" == "1" ]]; then
    (cd "$dir" && go test -race -count=1 -timeout 300s ./...) || { failed+=("$module:race"); continue; }
  fi
  echo "OK $module"
done

if [[ -f "$ROOT/air_tguserbot/go.mod" ]]; then
  echo "=== air_tguserbot (GOOS=linux build) ==="
  (cd "$ROOT/air_tguserbot" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...) \
    || failed+=("air_tguserbot:build")
fi

if [[ ${#failed[@]} -gt 0 ]]; then
  echo "FAILED: ${failed[*]}"
  exit 1
fi
echo "All modules OK"
