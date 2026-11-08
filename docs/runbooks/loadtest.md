# Runbook: нагрузочное тестирование

Цель: проверить SLO до/после миграции и перед cutover.

## Цели (стартовые)

| Сценарий | Нагрузка | SLO |
|---|---|---|
| `/readyz` | 50 rps | p95 < 100 ms, 0 ошибок |
| MCP `tools/list` | 50 rps | p95 < 200 ms, 0 5xx |
| MCP `tools/call` (`get_current_time`) | 20 rps | p95 < 300 ms, 5xx < 0.1% |
| Каналы (чтение/запись) | 20 rps | p95 < 400 ms |

Нагрузку подавать только на staging с тестовыми пользователями и отдельной БД.

## Подготовка

- [ ] Staging-стенд с тем же количеством реплик, что и prod.
- [ ] Тестовые пользователи и модели (MCP allowlist).
- [ ] `MCP_SERVICE_KEY`/`MCP_SESSION_SECRET` для нагрузочного клиента.
- [ ] Prometheus scraping включён; дашборд с `airorc_http_*` и `air_common_db_router_*`.

## Запуск

```bash
k6 run docs/ops/loadtest.js \
  -e BASE_URL=https://staging.example \
  -e SERVICE_KEY=... \
  -e SESSION_ID=... \
  -e TOOL=get_current_time
```

Во время прогона смотреть:

- `airorc_http_request_duration_seconds` (p95/p99);
- `airorc_http_requests_total{status_code=~"5.."}`;
- `air_common_db_router_errors_total`;
- CPU/RPS MySQL, slow query log, `SHOW PROCESSLIST`;
- `airorc_storage_healthy`.

## Критерии приёмки

- p95 в рамках SLO на всём прогоне;
- 5xx < 0.1%;
- отсутствие роста `air_common_db_router_errors_total`;
- отсутствие ошибок конкурентности (duplicate key, deadlock) в логах;
- после прогона backfill/verify: `cmd/userdir -action verify` → `issues: 0`,
  `cmd/dbverify` → `diffs: 0`.

## Регрессии

- Ошибки 5xx/таймауты → проверить пулы соединений (`dbrouter` кэш, `sql.DB` limits)
  и таймауты MCP (30 c/120 c).
- Рост latency → проверить индексы (`user_bot_events`, `dialog_messages`, `responders`)
  и размер батчей backfill.
- Deadlocks → проверить порядок блокировок в `GetOrSetTreadAndResponder` и purge/backfill.
