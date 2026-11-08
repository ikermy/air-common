# Runbook: миграции и rollout

## 1. Preflight

- [ ] Свежий backup БД + проверка restore на отдельной копии.
- [ ] `schema_migrations.dirty = 0` для всех строк.
- [ ] Все сервисы на одной версии `air-common` (ветка `new-arch`).
- [ ] `go test ./...` зелёный в затронутых модулях.

Проверка:

```sql
SELECT version, dirty FROM schema_migrations ORDER BY version;
```

## 2. Expand (миграции)

Миграции применяются автоматически при `RUN_MIGRATIONS=true` (idempotent runner).

```bash
RUN_MIGRATIONS=true go test -tags integration -run TestApplyMigrationsLocal ./internal/repository/mysql/...
```

Проверить ожидаемую версию (`000013` = soft-delete/audit, `000012` = user_directory, ...):

```sql
SELECT MAX(version) FROM schema_migrations;
```

Если `dirty=1`: остановить rollout, выяснить причину, исправить и только потом продолжать.

## 3. Backfill

Порядок и флаги:

| Данные | Флаг | Проверка |
|---|---|---|
| channels → user_channels/configs | `RUN_BACKFILL=true` | 12 каналов/12 конфигов |
| dialogs → dialog_messages/contexts | `RUN_BACKFILL=true` | 47 диалогов/1595 сообщений |
| responders.user_id | `RUN_BACKFILL_RESPONDERS=true` | нет дублей `(user_id, Type, RealId)` |
| service_*bot* → user_bots | `RUN_BACKFILL_USERBOTS=true` | `{Bots,Events,States,Skipped*}` |
| user_auth → user_directory | `RUN_BACKFILL_USERDIRECTORY=true` | `issues: 0` |

Все backfill идемпотентны: повторный прогон не меняет данные (`Total:0`, `Updated:0` и т.п.).

Проверка user_directory:

```bash
DB_HOST=... DB_NAME=air DB_USER=... DB_PASSWORD=... \
  go run ./cmd/userdir -action verify
```

## 4. Включение v2 read-path

Поэтапно, с наблюдением:

1. `CHANNELS_V2_READ_ENABLED=true` — смотреть ошибки/латентность 24–48 ч.
2. `DIALOG_MESSAGES_READ_ENABLED=true` — то же.
3. `USER_DELETE_SOFT=true` (опционально) — soft-delete + purge.

Откат: выставить флаг в `false` и перезапустить сервис (см. `rollback.md`).

## 5. Contract-фаза (после стабильного rollout)

Только после того, как все сервисы читают/пишут только v2 и v2-флаги стабильны.

1. Проверить предусловия в `phase5_contract.sql` (dirty, unmigrated dialogs, unowned responders).
2. Прогнать скрипт на копии схемы:

```bash
mariadb-dump -u... --no-data --routines --triggers --events air > schema.sql
mariadb -u... -e "CREATE DATABASE air_contract_test"
mariadb -u... air_contract_test < schema.sql
mariadb -u... air_contract_test < docs/sql/phase5_contract.sql
```

3. Применить в prod в окно обслуживания:

```bash
mariadb --default-character-set=utf8mb4 < docs/sql/phase5_contract.sql
```

4. Проверить: нет `service_*bot*`, routine и EVENT; новые колонки/ключи на месте.

## 6. Права и TLS

`docs/sql/phase6_privileges.sql`:

- заменить `__PASSWORD__` из secret store;
- применить при подготовке инфраструктуры;
- включить `require_secure_transport=ON` и TLS-сертификаты в `my.cnf`;
- проверить подключение каждого сервиса по TLS.

## 7. Пост-проверки

- `/readyz` = 200 во всех сервисах.
- `airorc_http_requests_total` не растёт по 5xx.
- `air_common_db_router_errors_total` не растёт.
- Backfill-статистика совпадает с ожидаемой.
