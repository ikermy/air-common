# Runbook: откат

## 1. Отключение v2 read-path

Самый быстрый и безопасный откат: вернуть чтение на legacy-колонки.

```bash
CHANNELS_V2_READ_ENABLED=false
DIALOG_MESSAGES_READ_ENABLED=false
```

Перезапуск сервиса. Dual-write продолжает работать, данные не теряются.

## 2. Откат slot-map (Phase 6)

`dbrouter` поддерживает атомарную смену поколения:

- вернуть предыдущий `DB_SHARDS_CONFIG` (generation N-1) и перезапустить сервис;
- либо вызвать `Router.UpdateConfig(previousConfig)` без рестарта;
- проверить `air_common_db_router_generation` = N-1;
- пересчитать `user_directory.shard_id` под старое поколение:

```bash
DB_HOST=... DB_NAME=air DB_USER=... DB_PASSWORD=... \
  go run ./cmd/userdir -action recompute -shard-map-file previous-shards.json
```

- проверить `-action verify` → `issues: 0`.

## 3. Откат soft-delete

- `USER_DELETE_SOFT=false` — новые удаления снова hard-delete.
- Уже помеченные пользователи остаются `status='deleted'` до purge.
- Вернуть пользователя: `UPDATE users SET status='active', deleted_at=NULL WHERE Id=?;`
  (только если данные ещё не удалены purge-джобом).

## 4. Откат миграции

Runner поддерживает `Down` (см. `pkg/migrate`), но в prod применять только осознанно:

- для expand-миграций (`ADD COLUMN`, новые таблицы) откат обычно не нужен;
- для contract (`DROP`) откат невозможен без restore из бэкапа — поэтому contract только после подтверждённого rollout.

## 5. Восстановление из бэкапа

1. Остановить сервисы, пишущие в БД.
2. Restore в отдельную БД, проверить контрольные суммы: `go run ./cmd/dbverify -users ...`.
3. Переключить DSN/шард на восстановленную БД.
4. `RUN_BACKFILL_*` для восстановления derived-таблиц, если нужно.
5. `cmd/userdir -action verify`.
6. Запустить сервисы, проверить `/readyz` и MCP smoke.

## 6. MCP: аварийное отключение защиты

Если после включения новой проверки MCP недоступен:

- `MCP_SERVICE_KEY=""` — отключить service-auth;
- `MCP_REQUIRE_SIGNED_SESSION=false` — разрешить legacy `X-Session-ID`;
- `MCP_SESSION_SECRET=""` — не проверять подпись.

Это временная мера; после стабилизации вернуть защиту.
