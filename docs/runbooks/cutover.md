# Runbook: cutover слота на новый шард

Цель: перенести пользователей слота (или диапазона слотов) с текущего шарда на новый
без потери данных и с возможностью отката.

## 0. Предусловия

- [ ] Новый шард поднят, доступен по TLS, схема применена (`schema_migrations` = актуальная).
- [ ] `user_directory` консистентен: `go run ./cmd/userdir -action verify` → `issues: 0`.
- [ ] Известны границы слота: `slot = xxhash64(BE64(user_id)) & 1023`.
- [ ] Есть свежий бэкап источника.

## 1. Заморозка записей (минимально)

Вариант A (рекомендуемый): перевести сервис в read-only для слота на время копирования.
Вариант B: копировать «на лету» и затем догнать дельту.

## 2. Копирование данных

Пользователи слота:

```sql
SELECT user_id FROM user_directory WHERE slot = ? ORDER BY user_id;
```

Скопировать user-scoped таблицы (`users`, `user_auth`, `user_models`, `user_api_keys`,
`user_channels`, `user_channel_configs`, `user_bots`, `user_bot_events`, `user_bot_state`,
`user_storage_config`, `user_storage_quota`, `dialogs`, `dialog_messages`, `dialog_contexts`,
`responders`, `subscriptions`, `notifications`, `billing`, `audit_log`).

Требования:

- `INSERT ... ON DUPLICATE KEY UPDATE` для идемпотентности;
- порядок: родители → дети (users → user_auth/…; dialogs → dialog_messages);
- `audit_log` копируется по `user_id`, id можно сохранять.

## 3. Сверка

```bash
DB_SOURCE_DSN=... DB_TARGET_DSN=... \
  go run ./cmd/dbverify -slot <N>
```

- exit `0` и `diffs: 0` — данные совпадают.
- при расхождениях — повторить копирование проблемных таблиц, не переключать слот.

Для точечной проверки:

```bash
go run ./cmd/dbverify -users 1,2,3
```

## 4. Переключение slot-map

1. Подготовить новый `shards.json` (generation N+1), где нужные слоты указывают на новый шард.
2. Проверить конфиг:

```bash
go run ./internal/... # либо unit/CI проверка Config
```

3. Применить:
   - обновить `DB_SHARDS_CONFIG`/`DB_SHARDS_CONFIG_FILE` и перезапустить сервис;
   - либо `Router.UpdateConfig(newCfg)` без рестарта (если есть админ-хук).
4. Пересчитать `user_directory.shard_id`:

```bash
go run ./cmd/userdir -action recompute -shard-map-file shards.json
go run ./cmd/userdir -action verify -shard-map-file shards.json
```

## 5. Наблюдение (30–60 мин)

- `/readyz` = 200;
- `air_common_db_router_generation` = N+1;
- `air_common_db_router_errors_total` не растёт;
- `airorc_http_error_ratio5m` и p95 в норме;
- выборочно: чтение/запись диалога, канала, бота для пользователей слота.

## 6. Откат

Если обнаружены расхождения/ошибки:

1. Вернуть предыдущий `shards.json` (generation N).
2. `UpdateConfig(previous)` или рестарт с прежним конфигом.
3. `cmd/userdir -action recompute -shard-map-file previous.json`.
4. `cmd/userdir -action verify`.
5. Проверить `/readyz` и ошибки.

Данные на новом шарде можно оставить (идемпотентное копирование) или очистить после инцидента.

## 7. Вывод из эксплуатации

После стабильного периода:

- удалить данные слота со старого шарда (после бэкапа);
- обновить документацию и inventory;
- зафиксировать generation в конфиге.
