# Runbook: выпуск `air-common` и bump сервисов

`air-common` — библиотека. Сервисы (`air_orchestrator`, `air_lead-hunter`, ...) собираются
с опубликованной версией; `go.work` используется только локально. Поэтому после изменений
в библиотеке нужен релиз и bump во всех сервисах.

## 1. Проверка перед релизом

```powershell
pwsh ./scripts/verify-all.ps1
pwsh ./scripts/verify-all.ps1 -Race
```

```bash
./scripts/verify-all.sh
```

- [ ] Все модули: `All modules OK`.
- [ ] CI `air-common` зелёный (build/vet/test/race/govulncheck).
- [ ] `go test -race ./pkg/idgen/... ./pkg/dbrouter/... ./pkg/telemetry/...` — PASS.
- [ ] Нет незакоммиченных изменений в `air-common` и сервисах.

## 2. Тег и публикация

```bash
git -C air-common checkout new-arch
git -C air-common tag v1.61.0
git -C air-common push origin v1.61.0
```

Версия — semver. Для breaking changes в публичных API — major/minor по политике проекта.
После push модуль доступен как `github.com/ikermy/air-common@v1.61.0`.

Проверка:

```bash
GOWORK=off go list -m github.com/ikermy/air-common@v1.61.0
```

## 3. Bump сервисов

```powershell
pwsh ./scripts/bump-air-common.ps1 -Version v1.61.0
```

Скрипт:

- находит все `go.mod`, где есть `github.com/ikermy/air-common`;
- выполняет `go mod edit -require=github.com/ikermy/air-common@<version>`;
- `go mod tidy`;
- запускает `scripts/verify-all.ps1`.

Проверить, что все сервисы на одной версии:

```bash
grep -h "ikermy/air-common" air_*/go.mod marusia_crm/go.mod
```

## 4. Standalone-сборка (без go.work)

```bash
GOWORK=off go -C air_orchestrator build ./...
GOWORK=off go -C air_lead-hunter build ./...
```

Если падает `no required module provides package ...` — версия не опубликована
или bump не выполнен.

## 5. Docker/CI

- Dockerfile сервиса собирается с опубликованной версией (`go mod download`).
- Локальный образ можно собрать из `go.work` (см. `docs/versions.md`);
  это не заменяет релиз.
- CI сервисов должен использовать опубликованную версию или checkout `air-common`
  в workspace с явным `ref`.

## 6. Откат релиза

- вернуть `-require` на предыдущую версию во всех сервисах;
- `go mod tidy` + `verify-all`;
- тег не удалять (immutable), при необходимости выпустить patch-версию.
