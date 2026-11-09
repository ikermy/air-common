# Матрица версий

Единая точка правды по модулям и зависимостям на ветке `new-arch`.

| Модуль | Go | `air-common` | `air-logger` | Примечание |
|---|---|---|---|---|
| `air-common` | 1.27.3 (локально) / CI 1.27.x | v2.0.3 (`.../v2`) | v2 | библиотека, module path `github.com/ikermy/air-common/v2` |
| `air-logger` | 1.27.x | — | — | CI: build/vet/test/race |
| `air_orchestrator` | 1.27.3 | локально через `go.work` (в go.mod — опубликованная версия) | v2 | реальный образ собирается из workspace |
| `air_lead-hunter` | 1.27.3 | локально через `go.work` | v2 | тесты зелёные |
| `air_whatsbot` | 1.27.3 | локально/опубликованная | v2 | — |
| `air_avito` | 1.27.3 | локально/опубликованная | v2 | — |
| `air_tgbot` | 1.27.3 | локально/опубликованная | v2 | — |
| `air_tguserbot` | 1.27.3 | локально/опубликованная | v2 | только `GOOS=linux` |
| `air_operator` | 1.27.3 | локально/опубликованная | v2 | — |
| `air_payment` | 1.27.3 | локально/опубликованная | v2 | — |
| `air_phantom` | 1.27.3 | локально/опубликованная | v2 | — |
| `air_widget` | 1.27.3 | локально/опубликованная | v2 | — |
| `marusia_crm` | 1.27.3 | локально/опубликованная | v2 | — |

## Проверка

```powershell
pwsh ./air-common/scripts/verify-all.ps1
pwsh ./air-common/scripts/verify-all.ps1 -Race
```

```bash
./air-common/scripts/verify-all.sh
./air-common/scripts/verify-all.sh --race
```

## Локальная сборка образа orchestrator

`build/Dockerfile` использует опубликованный `air-common`, где может не быть новых
пакетов (`idgen`, `migrate`, `dbrouter`, `telemetry`, `mcpsession`). Для локального
образа:

```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go -C air_orchestrator build -trimpath -ldflags="-s -w" -o airorc.exe ./cmd/
```

и упаковать бинарник в alpine (см. историю §15.15 в плане).

До публикации новой версии `air-common` прод-сборка должна либо использовать
workspace, либо дождаться релиза с этими пакетами.
