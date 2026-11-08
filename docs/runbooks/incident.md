# Runbook: инцидент

## 1. Быстрая диагностика

```bash
# liveness/readiness
curl -sS http://<host>:8080/healthz
curl -sS http://<host>:8080/readyz

# последние ошибки
docker logs --tail 200 <container>
```

`/readyz` проверяет primary DB и (если настроен) шарды. `503` означает проблему зависимостей, а не процесс.

## 2. Логи и трассировка

- Каждая запись при наличии спана содержит `[trace=<32hex> span=<16hex>]`.
- Найти все логи одного запроса:

```bash
docker logs <container> 2>&1 | grep '<trace_id>'
```

- В Jaeger/UI искать по `trace_id`, сервис `air_orchestrator`, операции:
  - `mcp.request`
  - `mcp.tool.call`
  - `<METHOD> <route>` (HTTP server span).

## 3. MCP

| Симптом | Причина | Действие |
|---|---|---|
| `401 invalid service key` | неверный/отсутствующий `X-Service-Key` | проверить `MCP_SERVICE_KEY` у сервиса и orchestrator |
| `Unauthorized: signed session required` | включён `MCP_REQUIRE_SIGNED_SESSION`, клиент старый | обновить worker или временно выключить |
| `Unauthorized: session token expired` | рассинхрон часов | проверить NTP, TTL 5 мин |
| `tool not allowed` | инструмент не включён в модели | проверить флаги модели (S3/GOAuth/MetaAction) |
| `rate limit exceeded` | >5 выз/с на `(user, tool)` | проверить цикл в LLM/клиенте |
| `access denied` (lead_target) | `resp_id` не принадлежит пользователю | проверить IDOR-попытку в логах |

Durable audit: `SELECT * FROM audit_log WHERE action='mcp.tool.call' ORDER BY id DESC LIMIT 20;`

## 4. База данных

```sql
SHOW PROCESSLIST;
SELECT * FROM schema_migrations ORDER BY version;
SHOW ENGINE INNODB STATUS;
```

- `dirty=1` → остановить миграции, разобрать причину.
- Долгие транзакции → проверить backfill/purge.

Метрики:

- `airorc_storage_healthy` = 0 → storage деградирован.
- `air_common_db_router_errors_total` растёт → недоступен шард/DSN.
- `airorc_http_request_duration_seconds` p95 > SLO.

## 5. Очередь purge

Если purge soft-deleted пользователей не идёт:

```sql
SELECT COUNT(*) FROM users WHERE status='deleted' AND deleted_at <= NOW(3) - INTERVAL 24 HOUR;
SELECT * FROM audit_log WHERE action IN ('user.soft_delete','user.hard_delete') ORDER BY id DESC LIMIT 20;
```

Проверить `USER_DELETE_SOFT`, `USER_DELETE_GRACE`, `USER_PURGE_INTERVAL` и логи cron.

## 6. Эскалация

1. Зафиксировать `trace_id`, время, затронутые сервисы.
2. Отключить проблемный v2-флаг (см. `rollback.md`).
3. При деградации БД — перевести чтение на legacy/предыдущий slot-map.
4. После стабилизации — добавить запись в `audit_log`/постмортем и алерт.
