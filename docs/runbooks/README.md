# Runbooks

Операционные инструкции для миграции AiR на новую архитектуру.

| Runbook | Когда использовать |
|---|---|
| [migrations.md](migrations.md) | Применение миграций, backfill, включение v2-флагов, contract-фаза |
| [rollback.md](rollback.md) | Откат флагов, slot-map, миграций, восстановление из бэкапа |
| [incident.md](incident.md) | Деградация/инцидент: readiness, логи, трейсы, БД, MCP |
| [tracing.md](tracing.md) | Включение и проверка OpenTelemetry, collector, retention |
| [release.md](release.md) | Тег/публикация `air-common` и bump всех сервисов |
| [cutover.md](cutover.md) | Перенос слота на новый шард, сверка, переключение и откат |
| [loadtest.md](loadtest.md) | Нагрузочное тестирование staging и критерии приёмки |

Связанные артефакты:

- `docs/sql/phase5_contract.sql` — contract-фаза (не запускать до rollout).
- `docs/sql/phase6_privileges.sql` — DB-пользователи, grants, TLS.
- `docs/ops/prometheus-rules.yml` — recording rules и алерты.
- `docs/ops/loadtest.js` — k6-сценарии для `/readyz` и MCP.
- `scripts/bump-air-common.ps1` — bump версии библиотеки во всех сервисах + verify.
- `docs/production-migration-plan.md` — полный план и статус.
