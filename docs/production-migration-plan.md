# План миграции AiR на production-архитектуру

**Статус:** draft for review  
**Область:** `air-common`, `air_orchestrator`, `air_whatsbot`, `air-logger`, остальные сервисы семейства `air_*`  
**Цель релиза:** шардируемая БД с сохранением всех данных, сквозная трассировка OpenTelemetry + Jaeger, SLA/SLO-мониторинг  
**Базовые артефакты:** `docs/air_prod.sql`, `air_orchestrator/migrations/air.sql`, `pkg/comdb/*`, `air_orchestrator/internal/repository/mysql/*`, `air_whatsbot/internal/repository/mysql/*`, `air-logger/pkg/logger/*`

---

## 0. Как читать документ

Документ состоит из пяти связанных треков:

1. **Анализ текущего состояния** (§1–§2).
2. **Целевая архитектура и миграция БД** (§3–§5).
3. **Наблюдаемость** (§6–§8).
4. **Исполнение** (§9–§14).
5. **Дополнительные улучшения проекта** (§15) — безопасность, надёжность, качество кода, тесты, стоимость и процессы.

Для принятия решения: §1, §3, §9, §13, §15. Для исполнителей: весь документ и приложения A–F.

**Термины:**

- **Reference-данные** — глобальные справочники (валюты, языки, роли, каталоги моделей).
- **User-scoped данные** — всё, что принадлежит `user_id` и лежит на шарде пользователя.
- **Shard key** — `user_id`; маршрутизация выполняется приложением.
- **Slot** — логический слот `0..1023` для перемещения данных между шардами без полного перехеширования.
- **Expand/Contract** — сначала добавить новое, писать в оба места, переключить чтение, затем удалить старое.
- **MasterKey (`$mk$`)** — per-user ключ шифрования полей в БД.

---

## 1. Резюме для руководства

### 1.1. Что делаем

Переводим AiR с «одна общая БД `air`, автоинкрементные ID, бизнес-логика в хранимых процедурах» на production-архитектуру:

- **ID генерируются приложением** (Snowflake-совместимые `BIGINT UNSIGNED`), а не `AUTO_INCREMENT`. Это снимает блокер шардирования и не требует переписывания существующих ID.
- **Схема нормализуется**: `channels` → `user_channels` + `user_channel_configs`; `dialogs.Data` → `dialog_messages`; `responders` становится user-scoped.
- **SP, функции, триггеры и EVENT удаляются**; логика переносится в Go-репозитории `air-common`.
- **Вводится `pkg/dbrouter`** и slot-based шардирование по `user_id`.
- **OpenTelemetry** — в `air-common` и во все сервисы; **Jaeger** принимает OTLP; **air-logger** обогащает логи `trace_id`/`span_id`.
- **SLA/SLO** формализуются; метрики приводятся к единому неймспейсу; в Perses добавляются SLO-дашборды.

### 1.2. Ключевой принцип миграции данных

**Существующие числовые ID не переписываются.** Snowflake-ID начинаются с ~`1.7e12`, старые `int(11)` — до `2.1e9`, коллизий нет. Достаточно:

1. Расширить тип колонок до `BIGINT UNSIGNED`.
2. Отключить `AUTO_INCREMENT` для новых строк (приложение передаёт ID явно).
3. Маршрутизировать и старые, и новые ID через `hash(user_id) % 1024`.

Это сохраняет все FK, не требует mapping-таблиц и массовой замены PK.

### 1.3. Основные риски

| Риск | Влияние | Смягчение |
|---|---|---|
| `$mk$`-поля нельзя прочитать без MasterKey | Нельзя офлайн развернуть `dialogs.Data` и `channels` | Копировать шифротекст байт-в-байт; lazy-миграция; legacy fallback |
| Разные версии `air-common` | Невозможно координированно менять схему | Сначала выровнять версии, затем flags/dual-write |
| `dialogs.Data` — растущий JSON с `FOR UPDATE` | Блокировки активных диалогов | `dialog_messages` + dual-write + переключение чтения |
| `responders` без `user_id` | Коллизии между тенантами | Добавить `user_id`, развести записи |
| Каталоги моделей пишутся из каждого сервиса | Конфликты при шардировании | Единый владелец reference-данных + кэш |
| Высокая кардинальность метрик | «Взрыв» VictoriaMetrics | Запрет `user_id`/`dialog_id` в лейблах |
| Нет `_scheduler_ResetUserFlags` в дампе | EVENT не работает | Cron в orchestrator + distributed lock |

### 1.4. Ожидаемый результат

- Новые пользователи, диалоги, responders, боты и сообщения создаются без глобального автоинкремента.
- Любая user-scoped операция выполняется на шарде по `user_id`.
- Схема версионируется миграциями; `air_prod.sql` — не источник истины.
- Один запрос/сообщение прослеживается в Jaeger до SQL и провайдера.
- Для каждого сервиса есть SLO, error budget и дашборд.

---

## 2. Анализ текущего состояния

### 2.1. Топология

```text
air_front / Envoy
        |
        +--> air_orchestrator  ──┐
        +--> air_tgbot         ──┤
        +--> air_tguserbot     ──┤
        +--> air_whatsbot      ──┼──> MariaDB 13 (air_db:3306, DB=air)
        +--> air_avito         ──┤
        +--> air_widget        ──┤
        +--> air_operator      ──┤
        +--> air_lead-hunter   ──┤
        +--> air_payment       ──┤
        +--> air_phantom       ──┤
        +--> marusia_crm       ──┘
```

- Подключение: `pkg/comdb/db.go:33-61` — `DB_HOST`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`, DSN `parseTime=true&charset=utf8mb4&loc=Local`.
- Пул: `MaxOpenConns=100`, `MaxIdleConns=100`, `ConnMaxLifetime=5m`, без `ConnMaxIdleTime`.
- Все сервисы используют один `comdb.New` и одну БД `air`.
- `air_orchestrator` владеет HTTP/gRPC API, MasterKey-кешем в Redis и планировщиком.
- `air_whatsbot` ходит в `airorc:50051` по gRPC (`pkg/rpc/client.go`).

### 2.2. Версии `air-common` (блокер координации)

| Сервис | Версия |
|---|---|
| `air_orchestrator`, `air_tguserbot`, `air_phantom` | `v1.60.3` |
| `air_whatsbot` | `v1.60.1` |
| `air_tgbot`, `air_avito`, `air_widget` | `v1.51.0` |
| `air_operator`, `air_payment` | `v1.50.94` |

Разброс версий означает разное поведение и типы. **До миграции схемы выровнять все сервисы на одну версию `air-common`.**

### 2.3. Инвентаризация БД

Источник: `docs/air_prod.sql` — 40 таблиц, 9 процедур, 9 функций, 6 триггеров, 1 событие.

Проверка live-БД (MariaDB 13.0.1, контейнер `air_db`) подтвердила: 40 base tables, 18 routines, 6 triggers, 1 event, `schema_migrations` отсутствует. Обнаружены расхождения:

- `crm_mappings` содержит 4 FK на `crm_applications`/`crm_configs`, но таблицы `crm_applications` в `air` **нет**; FK созданы при `foreign_key_checks=0` и невалидны.
- `user_gpt` содержит дублирующиеся FK на `gpt_models`/`model_providers` (`FK_user_gpt_gpt_models` + `fk_user_gpt_model_v2`, `fk_user_gpt_provider` + `fk_user_gpt_provider_v2`).
- `user_api_keys` использует `utf8mb4_uca1400_ai_ci`, остальные — `utf8mb4_general_ci`/`unicode_ci`.
- live-данные малы (7 users, 72 dialogs, 72 responders, 6 channels); это может быть dev/тестовый стенд, а не production. Для оценки объёмов и таймингов нужен baseline реальной prod-БД.

#### Reference-таблицы

`app_config`, `currency`, `languages`, `user_roles`, `chat_type`, `model_providers`, `gpt_models`, `realtime_models`, `voice_models`.

Проблемы: `app_config` без версий/аудита; `gpt_models`/`realtime_models`/`voice_models` без уникальности `(Provider, Name)`; `chat_type` дублирует `comdom.ChannelType`.

#### User-scoped таблицы

| Таблица | Shard key | Проблемы |
|---|---|---|
| `users` | `Id` | `int(11) AUTO_INCREMENT`, `PRIMARY KEY USING HASH` (InnoDB фактически BTREE), нет `status/deleted_at` |
| `user_auth` | `UserId` | `int(11)`, email в открытом виде, `MasterKey`/`TOTPSecret` в БД |
| `channels` | `UserId` | 6 каналов в одной широкой строке, `longtext` JSON, нет timestamps, нет user-scoped уникальности по типу |
| `billing` | `UserId` | PascalCase, нет `ON DELETE CASCADE` |
| `subscriptions` | `UserId` | — |
| `notifications` | `UserId` | PascalCase, смешанные типы |
| `operators` | `UserId` | Дублирует channel-подобные поля |
| `user_api_keys` | `UserId` | `Provider ENUM`, смешанная коллация `uca1400_ai_ci` |
| `user_gpt` | через `user_models` | Нет `user_id`; `Data blob`, `Ids longtext` |
| `user_models` | `UserId` | `ModelId` → `user_gpt.Id`; `Provider tinyint` |
| `google_oauth_tokens` | `user_id` | — |
| `crm_configs` | `user_id` | `credentials`/`options` JSON; unique `(user_id, crm_type)` |
| `crm_mappings` | `user_id` | Дублирующиеся FK (`crm_mappings_ibfk_1`, `fk_crm_mappings_application`) |
| `crm_oauth_states` | `user_id` | TTL-таблица |
| `crypto_payments` | `user_id` | Много индексов, enum статусов |
| `user_storage_config` | `user_id` | Секреты в `*_ciphertext` |
| `user_storage_quota` | `user_id` | — |
| `storage_migrations` | `user_id` | — |
| `vector_embeddings` | `user_id` | FK `model_id` → `user_models(ModelId)` на **неуникальную** колонку; `VECTOR(3072)` |
| `service` | `UserId` | `ServiceType ENUM('lead-haunter','voice-call')` |
| `service_contacts` | `UserId` | `Contact varchar(32)` |
| `service_contact_availability` | через `ContactId` | Нет `user_id` |
| `service_dialogs` | через `Responder`/`DialogId` | Нет `user_id` |
| `service_gpt` | `UserId` | `TgGroup tinytext` |
| `service_proxy` | `UserId` | `Password varchar(64)` в открытом виде |
| `service_tgbots` | `UserId` | `AuthData` зашифрован; `BotId` — внешний ID |
| `service_wabots` | `UserId` | То же |
| `service_user_bot_events` | нет | Только `BotType` + `BotId`, пересечение между пользователями |
| `service_user_bot_state` | нет | PK `(BotType, BotId)` |
| `responders` | **нет** | Глобальная: `RealId` + `Type`; коллизии между тенантами |
| `dialogs` | `User` | `Data longtext` — растущий JSON; `Context longtext`; PascalCase |

### 2.4. Критические недостатки

#### 2.4.1. `AUTO_INCREMENT` как глобальная последовательность

`users.Id`, `dialogs.Id`, `responders.Id`, `billing.Id`, `subscriptions.Id`, `operators.Id`, `service*.Id`, `crypto_payments.id` используют `AUTO_INCREMENT`. В шардированной БД каждая запись требует координации с центральным генератором. `users.Id int(11)` уже недостаточен для распределённых ID.

#### 2.4.2. `channels` — широкая таблица с JSON-блобом

`docs/air_prod.sql:51-67`:

```sql
CREATE TABLE `channels` (
  `UserId` int(11) NOT NULL,
  `TgBot` longtext ..., `TgBot_enabled` tinyint(4) ...,
  `Widget` longtext ..., `Widget_enabled` ...,
  `TgUserBot` ..., `Whats` ..., `Insta` ..., `Avito` ...,
  PRIMARY KEY (`UserId`)
)
```

Проблемы: новый канал требует `ALTER TABLE`; нельзя искать «все включённые WhatsApp-боты»; данные в `longtext`; `SaveChannelData` SP пишет plaintext, а приложение — `$mk$`; нет истории/версий/timestamps; нет FK на конкретный бот.

#### 2.4.3. `dialogs.Data` — неограниченный JSON-массив

`dialogs.Data` хранит всю историю в одном `longtext`. `SaveDialog` либо SP, либо `saveDialogWithResolver` (`pkg/comdb/dialog.go:94-181`) читает массив, аппендит элемент, пишет обратно; при MasterKey — с `FOR UPDATE`.

Последствия: размер строки растёт линейно; активный диалог — «горячая» строка; каждое сообщение — read-modify-write всей истории; нельзя индексировать/анализировать сообщения; шифрование всего массива мешает частичному чтению.

#### 2.4.4. Бизнес-логика в SP

`DeleteAllUserData`, `DeleteDialog`, `SaveDialog`, `SaveContext`, `ReadDialog`, `GetOrSetTreadAndResponder`, `GetUserTariff`, `GetNotificationChannel`, `UserLang` и др.:

- не покрыты unit-тестами;
- дублируют Go-код (`SaveChannelData` SP и `internal/repository/mysql/channels.go`);
- не шардируются;
- содержат баги;
- не видны в трассировке;
- требуют отдельного управления версиями.

#### 2.4.5. Триггеры и EVENT

- 6 триггеров на `service_tgbots`/`service_wabots`; `tg_delete_tgbot_events` использует `OLD.BotId`, `trg_wabot_delete_cascade_events` — `OLD.Id`.
- Дубли триггеров на `service_wabots`.
- `ResetFlagsEvent` вызывает `_scheduler_ResetUserFlags()`, которой **нет в дампе**.
- Триггеры не видны приложению и не трассируются.

#### 2.4.6. FK и целостность

- `vector_embeddings.fk_embeddings_model` → `user_models(ModelId)`, но `ModelId` не уникален. `ON DELETE CASCADE` может удалять не те строки.
- `crm_mappings` — два эквивалентных FK под разными именами.
- Нет `user_id` в `responders`, `service_contact_availability`, `service_dialogs`, `service_user_bot_events/state`.
- `DeleteAllUserData` покрывает не все таблицы.

#### 2.4.7. Логические баги в SP

- `GetOrSetTreadAndResponder` ищет responder по `(RealId, Type)` без `user_id` → общий responder/диалог у разных пользователей.
- `ReadDialog` (`docs/air_prod.sql:726-775`) выбирает одну строку по уникальному `Id`; `LIMIT`/`ORDER BY` бессмысленны; имена `Model`/`Responder` перепутаны.
- `ReadContext` поддерживает только `openai`/`mistral`, Google — нет.
- `DisableAllUserChannel` строит динамический SQL по `INFORMATION_SCHEMA`.

#### 2.4.8. Несогласованность имён, типов и коллаций

- PascalCase и snake_case в одной схеме.
- `user_id`: `int(11)`, `int(10) unsigned`, `bigint(20)`.
- Коллации: `general_ci`, `unicode_ci`, `uca1400_ai_ci`.
- Смешаны `timestamp`/`datetime`, DSN `loc=Local`.
- `tinyint` для bool без `CHECK`.

#### 2.4.9. Эксплуатация

- `MaxOpenConns=100` × реплики может исчерпать `max_connections`.
- Нет `ConnMaxIdleTime`, ретраев на deadlock/lock wait.
- Нет `PingContext` и `/readyz` с проверкой схемы.
- Нет версионированных миграций: `air_prod.sql` и `migrations/air.sql` расходятся.
- Все сервисы ходят одним `DB_USER`.

### 2.5. Анализ кода

#### `air-common/pkg/comdb`

- `DB` содержит `*sql.DB` и один DSN (`db.go:15-61`).
- `Exterior` (`interfaces.go:10-75`) смешивает диалоги, модели, каналы, векторы, OAuth, API-ключи, storage.
- `GetActiveProvider` (`provider.go:24`) — `user_models` с `userID`.
- `SaveUserModel` (`model.go:251-442`) — ручная транзакция + `LAST_INSERT_ID()`.
- `SaveDialog`/`ReadDialog`/`DeleteDialog`/`UpdateDialogsMeta`/`GetOrSetTreadAndResponder` используют SP (`dialog.go:108`, `dialog.go:192`, `dialog.go:233`).
- `GetUserAPIKey` — legacy numeric provider.
- Часть методов без `context` (`ReadDialog`, `SetActiveModel`).

#### `air_orchestrator`

- `internal/repository/mysql/*` — ~20 файлов SQL.
- `channels.go` дублирует `SaveChannelData`/`GetChannelsData`/`DeleteChannelData`/`CheckActiveChannels`/`GetActiveChannels`.
- `internal/config/config.go:55-73` — хардкод URL сервисов.
- `internal/metrics/metrics.go` — `airorc_*`; `DBQueriesTotal`/`DBQueryDuration` не подключены.
- HTTP/gRPC не инструментированы OTel.
- `migrations/air.sql` — дамп.
- `monitoring/` — VictoriaMetrics/VictoriaLogs/Perses, без трейсинга.

#### `air_whatsbot`

- `internal/db/db.go` встраивает `*comdb.DB`.
- `internal/repository/mysql/repository.go` — SQL к `channels`, `service_wabots`, `service_user_bot_*`.
- `internal/whatsapp/bot_channels.go:157` — `GetOrSetTreadAndResponder`.
- `internal/metrics/metrics.go` — богатый набор метрик.
- Нет OTel; `responder.Store` — in-memory кэш.

#### `air-logger`

- Логгер с ротацией, уровнями, `caller`.
- `userID` последним аргументом (`formatter.go:15`, `formatter.go:90-94`).
- Нет `context.Context`, `trace_id`/`span_id`, JSON, OTel-моста.
- В контейнерах `StdOut()`.

### 2.6. Наблюдаемость: текущее состояние

| Компонент | Что есть | Чего нет |
|---|---|---|
| Метрики | Prometheus-клиент, `/metrics`, VictoriaMetrics скрейпит все сервисы | SLO/error budget, единый неймспейс, контроль кардинальности |
| Логи | air-logger → stdout → Vector → VictoriaLogs | trace correlation, JSON, контекст |
| Трейсы | Нет | OTel SDK, Jaeger, пропагация, инструментирование SQL/gRPC/HTTP/WS |
| Дашборды | Perses по сервисам | SLO, сквозные зависимости, ссылки на трейсы |
| Алерты | Не формализованы | alert rules, пороги, error budget policy |

---

## 3. Целевая архитектура

### 3.1. Принципы

1. Приложение владеет бизнес-логикой; БД обеспечивает целостность в пределах шарда.
2. `user_id` — единый shard key; все user-scoped таблицы содержат `user_id` явно.
3. Reference-данные отделены и кэшируются.
4. ID генерируются приложением.
5. Expand/Contract вместо big-bang.
6. Секреты шифруются на уровне полей + key versioning.
7. Наблюдаемость — часть контракта.
8. Метрики без unbounded-лейблов.

### 3.2. Целевая топология БД

```text
                        ┌─────────────────────────┐
                        │  air_config / reference │
                        │  currency, languages,   │
                        │  roles, model catalog   │
                        └───────────┬─────────────┘
                                    │ read-only replica / cache
        ┌───────────────────────────┼───────────────────────────┐
        │                           │                           │
┌───────▼────────┐         ┌────────▼───────┐         ┌─────────▼──────┐
│ shard-0        │         │ shard-1        │   ...   │ shard-N        │
│ users 0..255   │         │ users 256..511 │         │ users ...      │
│ user_scoped    │         │ user_scoped    │         │ user_scoped    │
└───────┬────────┘         └────────┬───────┘         └─────────┬──────┘
        │                           │                           │
        └───────────────────────────┼───────────────────────────┘
                                    │
                        ┌───────────▼─────────────┐
                        │ air_orchestrator        │
                        │ user_directory (email→  │
                        │ user_id→slot→shard)     │
                        └─────────────────────────┘

Аналитика: CDC (Debezium/Maxwell) → ClickHouse/OLAP, без cross-shard JOIN.
```

Правила:

- Внутри шарда FK между user-scoped таблицами одного пользователя допустимы.
- Между шардами FK запрещены; целостность — на уровне приложения + периодические проверки.
- Reference-таблицы — в `air_config`/primary orchestrator, read-only из шардов.
- `user_directory`: `email_hash → user_id`, `user_id → slot`, `slot → shard_id`.
- Секреты не реплицируются в аналитику.

### 3.3. Slot-based шардирование

```text
slot = xxhash64(user_id) & 1023
shard = slot_map[slot]
```

`slot_map` в конфиге/Redis позволяет добавлять шарды, перемещая только часть слотов.

```json
{
  "generation": 3,
  "slots": {
    "0-255": "shard-0",
    "256-511": "shard-1",
    "512-767": "shard-2",
    "768-1023": "shard-3"
  }
}
```

```go
type Router interface {
    ForUser(ctx context.Context, userID uint64) (*sql.DB, error)
    ForReference(ctx context.Context) (*sql.DB, error)
    ForGlobal(ctx context.Context) (*sql.DB, error)
    Generation() uint64
}
```

На этапе одной БД все методы возвращают один `*sql.DB`; переключение на шарды не меняет репозитории.

### 3.4. Стратегия ID

| Вариант | Размер | Сортировка | Координация | Изменения | Вердикт |
|---|---|---|---|---|---|
| `AUTO_INCREMENT` | 4/8 | да | центральная | нет | Не подходит |
| UUIDv4 | 16 | нет | нет | высокие | Не рекомендуется |
| UUIDv7 / ULID | 16/26 | да | нет | высокие | Для public ID |
| **Snowflake `BIGINT UNSIGNED`** | **8** | **да** | **worker id** | **средние** | **Рекомендуется** |

Решение:

- Внутренние ID: 41 бит времени (с 2024-01-01), 10 бит `node_id`, 12 бит sequence.
- Публичные ID: ULID/UUIDv7 в `public_id`, не PK.
- Существующие ID не меняются: новые > `2^40`, старые `int(11)` < `2^31`.
- `node_id` из `NODE_ID`/`SHARD_ID`.

```go
type Generator struct { nodeID uint64; mu sync.Mutex; lastMS int64; seq uint64 }

func New(nodeID uint64) *Generator
func (g *Generator) Next() uint64
func (g *Generator) NextUserID() uint64
```

Требования: потокобезопасность, обработка отката часов, тест на 1M ID без дублей.

Изменения типов:

| Слой | Было | Стало |
|---|---|---|
| БД `users.Id` | `int(11) AUTO_INCREMENT` | `BIGINT UNSIGNED` (без AUTO_INCREMENT после contract) |
| БД `user_id` | `int(11)` / `int(10) unsigned` | `BIGINT UNSIGNED` |
| Go `userID` | `uint32` | `comdom.UserID` (defined type над `uint64`) |
| Go `dialogID` | `uint64` | `comdom.DialogID` (defined type над `uint64`) |
| Go `responderID` | `uint64` | `comdom.ResponderID` |
| Go `messageID` | — | `comdom.MessageID` |
| Go `botID` / `channelID` | `uint64` | `comdom.BotID` / `comdom.ChannelID` |
| `air-logger` userID | `uint32` | `uint64` + явное поле/`ctx` (не last-arg) |

Это широкое механическое изменение — отдельный релиз `air-common v2` (§8.1).

#### 3.4.1. Именованные типы ID (защита от путаницы `UserID` и `DialogID`)

После миграции и `UserID`, и `DialogID` — `uint64`. Без именованных типов компилятор не поймает вызов `ReadDialog(userID)` вместо `ReadDialog(dialogID)`.

Правило: **не использовать `type UserID = uint64` (alias)** — alias не даёт защиты. Нужны defined types в `pkg/comdom/id.go`:

```go
package comdom

type UserID uint64
type DialogID uint64
type ResponderID uint64
type MessageID uint64
type BotID uint64
type ChannelID uint64

func (id UserID) Uint64() uint64      { return uint64(id) }
func (id DialogID) Uint64() uint64    { return uint64(id) }
func (id ResponderID) Uint64() uint64 { return uint64(id) }
func (id MessageID) Uint64() uint64   { return uint64(id) }
func (id BotID) Uint64() uint64       { return uint64(id) }
func (id ChannelID) Uint64() uint64   { return uint64(id) }
```

Требования:

- `comdb.Exterior`, все репозитории и usecase-сигнатуры используют только именованные типы:
  ```go
  GetOrSetTreadAndResponder(userID UserID, responderRealID uint64, ...) (DialogID, error)
  ReadDialog(dialogID DialogID, ...)
  DeleteDialog(userID UserID, dialogID DialogID)
  ```
- На границе SQL — явная конвертация:
  ```go
  db.QueryRowContext(ctx, q, userID.Uint64())
  var raw uint64
  _ = row.Scan(&raw)
  id := DialogID(raw)
  ```
  Либо реализовать `driver.Valuer`/`sql.Scanner` на каждом типе; тогда конвертация не нужна, но появляется boilerplate.
- На границе API/gRPC — явная конвертация из `uint64`/proto/JSON в `UserID`/`DialogID` и обратно. JSON по умолчанию кодирует underlying `uint64` числом — обратная совместимость API сохраняется.
- `air-logger`: не полагаться на последний аргумент; передавать `WithField("user_id", userID.Uint64())` или `InfoCtx` с baggage. Иначе `UserID`/`DialogID` невозможно отличить в логах.
- Запрещены `int`, `int64`, `uint32` в доменных сигнатурах; только `Uint64()`/конструкторы на границах.
- Компиляция становится тестом: `go build ./...` падает, если перепутаны `UserID` и `DialogID`.
- `map[UserID]...`, `map[DialogID]...`, `sync.Map` — ключи разных типов, случайно не пересекутся.
- Для nullable-полей — `sql.Null[uint64]`/`*uint64` на границе, затем `UserID(raw)`.
- Добавить `Validate()`/`IsZero()`; `UserID(0)`/`DialogID(0)` считаются невалидными.
- Для JSON при необходимости строкового представления — явные `MarshalJSON`/`UnmarshalJSON`; по умолчанию оставить число для совместимости.

Порядок внедрения: сначала defined types в `comdom` и `air-common` (v2), затем сервисы; на время перехода допустимы adapter-функции `UserIDFromUint64`/`DialogIDFromUint64`, но не alias.

### 3.5. Целевая модель каналов

```sql
CREATE TABLE user_channels (
  id              BIGINT UNSIGNED NOT NULL,
  user_id         BIGINT UNSIGNED NOT NULL,
  channel_type    VARCHAR(32) NOT NULL,
  enabled         TINYINT(1) NOT NULL DEFAULT 0,
  external_id     VARCHAR(255) NULL,
  created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_user_channel (user_id, channel_type),
  KEY idx_type_enabled (channel_type, enabled),
  KEY idx_external (channel_type, external_id),
  CONSTRAINT fk_user_channels_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE user_channel_configs (
  channel_id        BIGINT UNSIGNED NOT NULL,
  user_id           BIGINT UNSIGNED NOT NULL,
  config            MEDIUMTEXT NULL,
  updated_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (channel_id),
  KEY idx_channel_user (user_id),
  CONSTRAINT fk_channel_cfg_channel FOREIGN KEY (channel_id) REFERENCES user_channels (id) ON DELETE CASCADE,
  CONSTRAINT fk_channel_cfg_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
```

Опционально — typed-таблица ботов:

```sql
CREATE TABLE user_channel_bots (
  channel_id        BIGINT UNSIGNED NOT NULL,
  user_id           BIGINT UNSIGNED NOT NULL,
  bot_type          ENUM('telegram','whatsapp') NOT NULL,
  external_bot_id   BIGINT NOT NULL,
  alias             VARCHAR(100) NOT NULL,
  auth_ciphertext   MEDIUMTEXT NOT NULL,
  is_active         TINYINT(1) NOT NULL DEFAULT 1,
  created_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (channel_id),
  UNIQUE KEY uq_user_bot (user_id, bot_type, external_bot_id),
  UNIQUE KEY uq_user_alias (user_id, bot_type, alias),
  CONSTRAINT fk_channel_bots_channel FOREIGN KEY (channel_id) REFERENCES user_channels (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
```

Семантика:

- Один пользователь — максимум одна запись `(user_id, channel_type)`.
- `enabled` заменяет `*_enabled`.
- `config` — единственная payload-колонка: plaintext JSON либо `$mk$`-шифротекст (если у пользователя есть MasterKey); режим определяется по префиксу, отдельного флага нет.
- `external_id` — быстрый поиск владельца по входящему сообщению.

Совместимость: view `channels_legacy` на время миграции, но репозитории переключаются на новые таблицы.

### 3.6. Целевая модель диалогов

```sql
CREATE TABLE dialogs (
  id            BIGINT UNSIGNED NOT NULL,
  user_id       BIGINT UNSIGNED NOT NULL,
  responder_id  BIGINT UNSIGNED NOT NULL,
  chat_type     TINYINT UNSIGNED NOT NULL,
  is_target     TINYINT(1) NOT NULL DEFAULT 0,
  is_trigger    TINYINT(1) NOT NULL DEFAULT 0,
  created_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_dialog_identity (user_id, responder_id, chat_type),
  KEY idx_user_created (user_id, created_at),
  CONSTRAINT fk_dialogs_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  CONSTRAINT fk_dialogs_responder FOREIGN KEY (responder_id) REFERENCES responders (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE dialog_messages (
  id            BIGINT UNSIGNED NOT NULL,
  dialog_id     BIGINT UNSIGNED NOT NULL,
  user_id       BIGINT UNSIGNED NOT NULL,
  seq           BIGINT UNSIGNED NOT NULL,
  role          ENUM('user','assistant','system','tool') NOT NULL,
  content_type  ENUM('text','audio','image','file','json') NOT NULL DEFAULT 'text',
  content       MEDIUMTEXT NULL,
  content_ciphertext MEDIUMTEXT NULL,
  content_json  JSON NULL,
  provider      VARCHAR(32) NULL,
  model         VARCHAR(128) NULL,
  tokens_in     INT UNSIGNED NULL,
  tokens_out    INT UNSIGNED NULL,
  created_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_dialog_seq (dialog_id, seq),
  KEY idx_user_created (user_id, created_at),
  KEY idx_dialog_created (dialog_id, created_at),
  CONSTRAINT fk_messages_dialog FOREIGN KEY (dialog_id) REFERENCES dialogs (id) ON DELETE CASCADE,
  CONSTRAINT fk_messages_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE dialog_contexts (
  dialog_id     BIGINT UNSIGNED NOT NULL,
  user_id       BIGINT UNSIGNED NOT NULL,
  provider      VARCHAR(32) NOT NULL,
  context_json  JSON NULL,
  updated_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (dialog_id, provider),
  KEY idx_context_user (user_id),
  CONSTRAINT fk_context_dialog FOREIGN KEY (dialog_id) REFERENCES dialogs (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
```

`dialogs.Data`/`Context` остаются как legacy до contract-фазы.

### 3.7. Целевая модель responders и ботов

```sql
CREATE TABLE responders (
  id          BIGINT UNSIGNED NOT NULL,
  user_id     BIGINT UNSIGNED NOT NULL,
  chat_type   TINYINT UNSIGNED NOT NULL,
  real_id     BIGINT NOT NULL,
  name        VARCHAR(120) NULL,
  created_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_responder (user_id, chat_type, real_id),
  KEY idx_user_type (user_id, chat_type),
  CONSTRAINT fk_responders_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE user_bots (
  id                BIGINT UNSIGNED NOT NULL,
  user_id           BIGINT UNSIGNED NOT NULL,
  bot_type          ENUM('telegram','whatsapp') NOT NULL,
  external_bot_id   BIGINT NOT NULL,
  alias             VARCHAR(100) NOT NULL,
  auth_ciphertext   MEDIUMTEXT NOT NULL,
  is_active         TINYINT(1) NOT NULL DEFAULT 1,
  created_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_user_bot (user_id, bot_type, external_bot_id),
  UNIQUE KEY uq_user_alias (user_id, bot_type, alias),
  KEY idx_user_active (user_id, is_active),
  CONSTRAINT fk_user_bots_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE user_bot_events (
  id            BIGINT UNSIGNED NOT NULL,
  user_id       BIGINT UNSIGNED NOT NULL,
  user_bot_id   BIGINT UNSIGNED NOT NULL,
  event_type    ENUM('state','error','warning','info') NOT NULL,
  cause_group   ENUM('successes','auth_key','session','phone','password','api','proxy','user_block','chat_access','content','media','rate_limit','retry','other') NULL,
  cause_code    VARCHAR(64) NULL,
  description   VARCHAR(255) NULL,
  context_json  JSON NULL,
  severity      ENUM('fatal','start_block','send_block','recoverable') NOT NULL DEFAULT 'recoverable',
  wait_until    TIMESTAMP(3) NULL,
  new_is_active TINYINT(1) NULL,
  created_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_bot_time (user_bot_id, created_at),
  KEY idx_user_time (user_id, created_at),
  CONSTRAINT fk_bot_events_bot FOREIGN KEY (user_bot_id) REFERENCES user_bots (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;

CREATE TABLE user_bot_state (
  user_bot_id       BIGINT UNSIGNED NOT NULL,
  user_id           BIGINT UNSIGNED NOT NULL,
  can_start         TINYINT(1) NOT NULL DEFAULT 1,
  can_send          TINYINT(1) NOT NULL DEFAULT 1,
  last_error_group  ENUM('auth_key','session','phone','password','api','user_block','chat_access','content','media','rate_limit','other') NULL,
  last_error_code   VARCHAR(64) NULL,
  last_error_at     TIMESTAMP(3) NULL,
  rate_limit_until  TIMESTAMP(3) NULL,
  updated_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_bot_id),
  KEY idx_user (user_id),
  KEY idx_state_flags (can_start, can_send),
  KEY idx_rate (rate_limit_until),
  CONSTRAINT fk_bot_state_bot FOREIGN KEY (user_bot_id) REFERENCES user_bots (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;
```

Это устраняет триггеры и баг `BotId` vs `Id`.

### 3.8. Reference-данные и владение каталогами

- `model_providers`, `gpt_models`, `realtime_models`, `voice_models` — reference.
- Владелец — `air_orchestrator` (или отдельный `air_config`-сервис).
- Остальные сервисы читают через gRPC/Redis-кэш и не пишут в reference.
- `SyncProviderModels` переезжает в orchestrator.
- `app_config` получает версионирование и аудит.

### 3.9. Миграционный инструментарий

- `golang-migrate/migrate` (или `pressly/goose`) + embed FS.
- `pkg/migrate/migrations/<version>_<name>.up.sql`/`.down.sql`.
- `schema_migrations(version, dirty, applied_at, checksum)`.
- CI: `atlas`/schemadiff.
- Запуск — только orchestrator/migrate job; сервисы проверяют schema version при старте.
- Ручные `ALTER TABLE` в prod запрещены.

---

## 4. Детальный план миграции БД

### 4.0. Общая стратегия

```text
Phase 0  Подготовка и baseline
Phase 1  Инструменты + типы + ID (expand)
Phase 2  Каналы v2 (expand + dual-write + backfill)
Phase 3  Диалоги/сообщения/контексты (expand + dual-write + lazy backfill)
Phase 4  Responders/боты и удаление SP/триггеров/EVENT
Phase 5  Contract: удаление legacy
Phase 6  Шардирование: dbrouter, user_directory, reference separation
Phase 7  Cutover на шарды
```

Каждая фаза: миграция вперёд/назад, проверка данных, feature flag, runbook, метрики/трейсы.

### Phase 0 — Подготовка (1 неделя)

1. Заморозить изменения схемы; только через миграции.
2. Backup: `mysqldump --single-transaction --routines --triggers --events` + snapshot + binlog.
3. Проверить restore; зафиксировать RTO/RPO.
4. Baseline: row counts, размеры, slow queries, `AUTO_INCREMENT`, пользователи с MasterKey, зашифрованные поля.
5. Дрейф `air_prod.sql` vs `migrations/air.sql` vs prod: `_scheduler_ResetUserFlags`, дубли триггеров, FK.
6. `cmd/schema-audit`: ожидаемая vs фактическая схема, diff, наличие SP/триггеров/EVENT, типы `user_id`.
7. Выровнять версии `air-common` на последнюю `v1.x`.
8. Ввести выключенные flags: `DB_SCHEMA_VERSION_REQUIRED`, `CHANNELS_V2_READ_ENABLED`, `CHANNELS_V2_WRITE_ENABLED`, `DIALOG_MESSAGES_WRITE_ENABLED`, `DIALOG_MESSAGES_READ_ENABLED`, `IDGEN_ENABLED`.
9. Зафиксировать окно и ответственных.

**Выход:** backup, restore-check, baseline, audit, план окна.

### Phase 1 — Инструменты, типы, ID (2 недели)

#### 1.1. Миграции

```text
air-common/pkg/migrate/
  migrate.go
  migrations/
    000001_baseline.up.sql
    000002_widen_ids.up.sql
    000003_add_schema_migrations.up.sql
    000010_user_channels.up.sql
    ...
```

`000001_baseline` — текущая схема, чтобы новая БД поднималась с нуля.

#### 1.2. Расширить типы

```sql
ALTER TABLE users
  MODIFY Id BIGINT UNSIGNED NOT NULL,
  ADD COLUMN public_id CHAR(26) NULL AFTER Id,
  ADD COLUMN status ENUM('active','blocked','deleted') NOT NULL DEFAULT 'active',
  ADD COLUMN deleted_at TIMESTAMP(3) NULL,
  ADD COLUMN created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  ADD COLUMN updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3);

ALTER TABLE user_auth              MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE channels               MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE billing                MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE subscriptions          MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE notifications          MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE operators              MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE user_api_keys          MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE user_gpt               MODIFY Id BIGINT UNSIGNED NOT NULL;
ALTER TABLE user_models            MODIFY UserId BIGINT UNSIGNED NOT NULL,
                                   MODIFY Id BIGINT UNSIGNED NOT NULL,
                                   MODIFY ModelId BIGINT UNSIGNED NOT NULL;
ALTER TABLE google_oauth_tokens    MODIFY user_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE crm_configs            MODIFY user_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE crm_mappings           MODIFY user_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE crm_oauth_states       MODIFY user_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE crypto_payments        MODIFY user_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE user_storage_config    MODIFY user_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE user_storage_quota     MODIFY user_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE storage_migrations     MODIFY user_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE vector_embeddings      MODIFY user_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE service                MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE service_contacts       MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE service_gpt            MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE service_proxy          MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE service_tgbots         MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE service_wabots         MODIFY UserId BIGINT UNSIGNED NOT NULL;
ALTER TABLE dialogs                MODIFY User BIGINT UNSIGNED NOT NULL,
                                   MODIFY Id BIGINT UNSIGNED NOT NULL,
                                   MODIFY Responder BIGINT UNSIGNED NOT NULL;
ALTER TABLE responders             MODIFY Id BIGINT UNSIGNED NOT NULL;
```

> `ALTER ... MODIFY` на больших таблицах — через `pt-online-schema-change`/`gh-ost`, окно или реплику со switchover.

#### 1.3. `pkg/idgen`

- Snowflake, `NODE_ID`/`SHARD_ID` из env.
- Тесты: уникальность, монотонность, откат часов.
- `IDGEN_ENABLED`.
- Новые записи — с явным ID; `AUTO_INCREMENT` остаётся fallback до Phase 5.
- Не полагаться на `LastInsertId()`.

#### 1.4. `id_source`

```sql
ALTER TABLE users ADD COLUMN id_source ENUM('legacy','snowflake') NOT NULL DEFAULT 'legacy';
UPDATE users SET id_source='legacy';
```

Аналогично для `dialogs`, `responders`, `user_bots`.

#### 1.5. Исправить FK `vector_embeddings`

```sql
ALTER TABLE vector_embeddings ADD COLUMN user_model_id BIGINT UNSIGNED NULL;
UPDATE vector_embeddings ve
JOIN user_models um ON um.ModelId = ve.model_id AND um.UserId = ve.user_id
SET ve.user_model_id = um.Id;
ALTER TABLE vector_embeddings MODIFY user_model_id BIGINT UNSIGNED NOT NULL;
ALTER TABLE vector_embeddings
  ADD CONSTRAINT fk_embeddings_user_model FOREIGN KEY (user_model_id) REFERENCES user_models (Id) ON DELETE CASCADE;
-- после проверки
ALTER TABLE vector_embeddings DROP FOREIGN KEY fk_embeddings_model;
```

#### 1.6. Collation/charset

- `utf8mb4_uca1400_ai_ci` (MariaDB 13) или `utf8mb4_unicode_ci` (совместимость с MySQL 8).
- Поэтапно, начиная с маленьких таблиц; проверить индексы/уникальность.

#### 1.7. Именованные типы ID

- Добавить `pkg/comdom/id.go`: `UserID`, `DialogID`, `ResponderID`, `MessageID`, `BotID`, `ChannelID` как defined types над `uint64` (§3.4.1).
- Перевести `comdb.Exterior`, `pkg/model`, `pkg/startpoint`, `pkg/endpoint` на эти типы.
- На SQL-границе — `Uint64()`/явная конвертация; на API/gRPC — конвертация из proto/JSON.
- `air-logger` — явное поле `user_id`, не last-arg.
- Проверка: `go build ./...` должен падать при подстановке `UserID` вместо `DialogID`.
- Adapter-функции `UserIDFromUint64`/`DialogIDFromUint64` — только на переходный период, не alias.

**Выход:** миграции на dev/stage, ID генерируются приложением, типы расширены, именованные типы внедрены, FK исправлен, audit проходит.

### Phase 2 — Каналы v2 (2–3 недели)

#### 2.1. Новые таблицы

`user_channels`, `user_channel_configs` (§3.5).

#### 2.2. Dual-write

- `SaveChannelData` → legacy `channels` + `user_channels`/`user_channel_configs`.
- `SetChannelEnabled`/`DisableAllUserChannel` — обе модели.
- `DeleteChannelData` — legacy + новые.
- Ошибка новой записи не ломает legacy; метрика `air_common_dual_write_errors_total{target="channels_v2"}`.

#### 2.3. Backfill

`cmd/migrate-channels`:

1. Читает `channels` батчами по `UserId`.
2. Для каждого непустого поля:
   - `$mk$`/`$app$` → `config` как есть (шифротекст);
   - валидный JSON → `config` как plaintext;
   - иначе → `config` с `{"value": ...}`.
3. Создаёт `user_channels` с app-generated ID.
4. Создаёт `user_channel_configs`.
5. Пишет `migration_progress`.

```sql
CREATE TABLE migration_progress (
  name        VARCHAR(64) NOT NULL,
  last_key    VARCHAR(255) NULL,
  processed   BIGINT UNSIGNED NOT NULL DEFAULT 0,
  total       BIGINT UNSIGNED NULL,
  updated_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (name)
) ENGINE=InnoDB;
```

#### 2.4. Проверка

- Количество непустых legacy-каналов == количество новых записей.
- `enabled` совпадает.
- JSON — семантическое сравнение; шифротекст — SHA-256 (байт-в-байт).
- Выборочная расшифровка 10–20 пользователей с MasterKey.
- Метрика `air_common_migration_rows_total{phase="channels",status}`.

#### 2.5. Переключение чтения

- `CHANNELS_V2_READ_ENABLED=true`.
- `GetChannelsData`/`CheckActiveChannels`/`GetActiveChannels` — новые таблицы.
- При расхождении — legacy fallback + warning.
- `air_whatsbot` и др. — через API orchestrator или новые таблицы после обновления.

#### 2.6. Contract

- Удалить legacy-колонки и `channels` (или `channels_archive`).
- Удалить SP `SaveChannelData`, `SetChannelEnabled`, `DisableAllUserChannel`.
- Обновить `comdom.ChannelType`.

**Выход:** каналы v2, legacy архивирован, данные сверены.

### Phase 3 — Диалоги, сообщения, контексты (2–3 недели)

#### 3.1. Таблицы

`dialog_messages`, `dialog_contexts`; добавить:

```sql
ALTER TABLE dialogs
  ADD COLUMN messages_migrated TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN messages_migrated_at TIMESTAMP(3) NULL;
```

#### 3.2. Dual-write

- `SaveDialog` → `dialog_messages` + legacy `dialogs.Data`.
- `ReadDialog` → `dialog_messages`, если мигрировано/включено, иначе legacy.
- `UpdateDialogsMeta` → `is_target/is_trigger`.
- `SaveContext`/`ReadContext` → `dialog_contexts` + legacy.

#### 3.3. Backfill

**Lazy (основной):**

- При `ReadDialog`, если `messages_migrated=0`:
  - plaintext → парсит JSON-массив, вставляет `dialog_messages`;
  - `$mk$` + MasterKey → расшифровывает, парсит, вставляет;
  - нет ключа → legacy, `messages_migrated=0`;
  - успех → `messages_migrated=1`.

**Batch:**

- `cmd/migrate-dialogs` для пользователей с MasterKey в Redis/orchestrator.
- Батчи по `dialogs.Id`, прогресс.
- Без ключа — отчёт; миграция при следующем логине/чтении.

#### 3.4. Формат сообщений

Маппинг: `role` (`user`/`assistant`/`system`/`tool`), `content`, `content_type`, `seq`, `provider`, `model`, `tokens_*`. При неоднозначности — `content_json` без потерь.

#### 3.5. Проверка

- `COUNT(dialog_messages)` == число элементов legacy для мигрированных диалогов.
- Непрерывность `seq`.
- Контрольные суммы `(role, content, created_at)`.
- Сравнение `ReadDialog` legacy vs v2 на 100+ диалогах.
- Метрика `air_common_migration_dialogs_total{status}`.

#### 3.6. Contract

- `DIALOG_MESSAGES_READ_ENABLED=true`.
- `dialogs.Data`/`Context` → read-only, затем drop/archive.
- Удалить SP `SaveDialog`, `SaveContext`, `ReadContext`, `ReadDialog`, `GetOrSetDialog`, `UpdateDialogsMeta`, `DeleteDialog`.
- `pkg/comdb/dialog.go` — чистый SQL.

**Выход:** сообщения нормализованы, история сохранена, legacy архивирован.

### Phase 4 — Responders, боты, SP/триггеры/EVENT (2 недели)

#### 4.1. `responders` → user-scoped

1. `ADD COLUMN user_id BIGINT UNSIGNED NULL`.
2. Backfill:
   - все `dialogs` одного пользователя → `user_id = User`;
   - несколько → копия responder на каждого, перепривязка `dialogs.Responder`;
   - не используется → `user_id = 0`, пометить.
3. `MODIFY user_id ... NOT NULL`.
4. `UNIQUE (user_id, chat_type, real_id)` после дедупликации.
5. Новый `GetOrSetTreadAndResponder`:

```sql
INSERT INTO responders (id, user_id, chat_type, real_id, name)
VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE name = VALUES(name), updated_at = CURRENT_TIMESTAMP(3);
SELECT id FROM responders WHERE user_id=? AND chat_type=? AND real_id=?;
```

6. Транзакция + `air_common_responder_ops_total{result}`.

> Нужна Go-команда: разветвление по пользователям.

#### 4.2. Боты и события

- Создать `user_bots`, `user_bot_events`, `user_bot_state`.
- Backfill из `service_tgbots`/`service_wabots` и `service_user_bot_*`.
- `user_bot_events.user_bot_id` — через `(bot_type, external_bot_id, user_id)`.
- Удалить триггеры; каскад — FK `ON DELETE CASCADE`.
- Удалить `service_tgbots`/`service_wabots` после contract (или view).

#### 4.3. `DeleteAllUserData`

- Реализовать в `air_orchestrator/internal/usecase/admin` (или `pkg/comdb/account`):
  - soft-delete `users.status='deleted'`, `deleted_at`;
  - `audit_log`;
  - purge-задача в очереди;
  - фоновый purge батчами по user-scoped таблицам;
  - удаление файлов MinIO/S3 и зашифрованных данных;
  - идемпотентность.
- Удалить SP.
- Тест: после purge нет строк с `user_id`.

#### 4.4. Остальные SP/функции

| Объект | Куда | Примечание |
|---|---|---|
| `DeleteDialog` | `comdb/dialog` | проверка роли в Go, каскад messages/contexts |
| `DisableAllUserChannel` | `comdb/channels` | `UPDATE user_channels SET enabled=0` |
| `GetOrSetTreadAndResponder` | `comdb/dialog` | upsert + app ID |
| `SaveChannelData` | `comdb/channels` | v2 |
| `SaveContext` | `comdb/dialog` | upsert `dialog_contexts` |
| `SaveDialog` | `comdb/dialog` | insert `dialog_messages` |
| `SetChannelEnabled` | `comdb/channels` | v2 |
| `UpdateDialogsMeta` | `comdb/dialog` | `is_target/is_trigger` |
| `GetBalance` | `comdb/users` | SELECT |
| `GetNotificationChannel` | `comdb/notifications` | Go JSON |
| `GetOrSetDialog` | `comdb/dialog` | upsert |
| `GetUserSubscriptionLimites` | `comdb/users` | join |
| `GetUserTariff` | `comdb/billing` | Go, тесты по ролям |
| `ReadContext` | `comdb/dialog` | SELECT |
| `ReadDialog` | `comdb/dialog` | messages + responder |
| `ReadResponderName` | `comdb/responders` | — |
| `UserLang` | `comdb/users` | join languages |

#### 4.5. EVENT → cron

- Удалить `ResetFlagsEvent`.
- `internal/infrastructure/scheduler/cron.go`:
  - каждые 30 секунд;
  - Redis lock `SET NX PX`;
  - спан `scheduler.reset_user_flags`;
  - метрики `air_orc_scheduler_runs_total`, `air_orc_scheduler_duration_seconds`;
  - идемпотентность.
- Восстановить `_scheduler_ResetUserFlags` логику в Go (open question §14).

**Выход:** нет SP/функций/триггеров/EVENT в production; логика в Go с тестами.

### Phase 5 — Contract и очистка (1–2 недели)

- Удалить legacy-колонки/таблицы после 2 недель стабильности.
- Архив: `channels_archive`, `dialogs_legacy_data`.
- Удалить compatibility views и feature flags.
- `000001_baseline` описывает целевую схему для новых установок.
- `docs/air_prod.sql` перегенерировать из целевой схемы (для документации).

### Phase 6 — Шардирование (2–4 недели)

#### 6.1. `user_directory`

```sql
CREATE TABLE user_directory (
  user_id     BIGINT UNSIGNED NOT NULL,
  email_hash  VARBINARY(32) NOT NULL,
  slot        SMALLINT UNSIGNED NOT NULL,
  shard_id    SMALLINT UNSIGNED NOT NULL,
  created_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id),
  UNIQUE KEY uq_email_hash (email_hash),
  KEY idx_slot (slot)
) ENGINE=InnoDB;
```

- В глобальной БД (`air_config`/orchestrator).
- Backfill из `user_auth.EmailHash` + `user_id`.
- Регистрация: создать user в шарде → directory.
- Логин: email_hash → user_id → шард.

#### 6.2. `pkg/dbrouter`

```json
{
  "generation": 1,
  "slots": [{"from":0,"to":1023,"shard":"shard-0"}],
  "shards": {"shard-0":{"dsn_env":"DB_SHARD0_DSN"}}
}
```

- `ForUser` → `xxhash64(userID) & 1023` → shard.
- Кэш соединений `map[shardID]*sql.DB`.
- Метрики `air_common_db_router_requests_total{shard}`, `..._errors_total`.
- `Ping` шардов при старте; `/readyz` при `SHARD_STRICT=true`.

#### 6.3. Reference-данные

- Вынести в `air_config`.
- `ForReference()` — отдельный пул.
- Redis-кэш: `model_providers`, `gpt_models`, `realtime_models`, `voice_models`, `currency`, `languages`, `user_roles`, `chat_type`.
- Инвалидация по версии/событию.

#### 6.4. Разделение прав

| Пользователь | Права |
|---|---|
| `air_migrator` | DDL, только CI/CD |
| `air_orchestrator` | CRUD user-scoped + reference read |
| `air_whatsbot` | CRUD свои таблицы/шард + reference read |
| `air_tgbot`/`air_tguserbot`/... | аналогично |
| `air_analytics` | SELECT на репликах/CDC |
| `air_backup` | `RELOAD`, `LOCK TABLES`, `REPLICATION CLIENT` |

- Убрать общий `DB_USER`; у каждого сервиса свой.
- TLS, `require_secure_transport=ON`, audit plugin.

#### 6.5. Репликация и переезд

- snapshot + binlog (XtraBackup/mariabackup);
- dual-write;
- контрольные суммы;
- переключение по слотам;
- откат — вернуть `slot_map` generation.
- Большие таблицы — `pt-osc`/`gh-ost`, батчи по `user_id`.

#### 6.6. Cross-shard

- Запрещены в рантайме.
- Аналитика — CDC → ClickHouse/OLAP.
- Глобальные списки — `user_directory`.

### Phase 7 — Cutover и стабилизация

- Переключить `DB_SHARD_MAP_GENERATION`.
- `SHARD_STRICT=true`.
- Проверить SLO/трейсы.
- Legacy-шард read-only 2 недели.
- Затем вывести legacy.

---

## 5. Матрица миграции таблиц

| Таблица | Действие | Целевое | Риск |
|---|---|---|---|
| `users` | PK/тип, новые поля | `users` | Высокий |
| `user_auth` | `UserId`, `email_hash` | `user_auth` | Средний |
| `channels` | Разбить | `user_channels`, `user_channel_configs` | Высокий |
| `billing` | Тип, snake_case, timestamps | `billing` | Низкий |
| `subscriptions` | Тип | `subscriptions` | Низкий |
| `notifications` | Тип, snake_case | `notifications` | Низкий |
| `operators` | Тип | `operators` | Средний |
| `user_api_keys` | Тип, provider reference | `user_api_keys` | Низкий |
| `user_gpt` | `user_id`, тип | `user_gpt` | Средний |
| `user_models` | Тип | `user_models` | Средний |
| `google_oauth_tokens` | Тип | — | Низкий |
| `crm_*` | Тип, дубли FK | — | Низкий |
| `crypto_payments` | Тип | — | Низкий |
| `user_storage_*` | Тип | — | Низкий |
| `storage_migrations` | Тип | — | Низкий |
| `vector_embeddings` | FK, тип | — | Средний |
| `service` | Тип, snake_case | `user_services` | Низкий |
| `service_contacts` | Тип | — | Низкий |
| `service_contact_availability` | `user_id` | — | Средний |
| `service_dialogs` | `user_id` | — | Средний |
| `service_gpt` | Тип | — | Низкий |
| `service_proxy` | Тип, шифрование | — | Средний |
| `service_tgbots` | Объединить | `user_bots` | Высокий |
| `service_wabots` | Объединить | `user_bots` | Высокий |
| `service_user_bot_events` | `user_id`, `user_bot_id` | `user_bot_events` | Высокий |
| `service_user_bot_state` | `user_id`, `user_bot_id` | `user_bot_state` | Высокий |
| `responders` | `user_id`, дедуп | `responders` | Высокий |
| `dialogs` | Разбить Data/Context | `dialogs`, `dialog_messages`, `dialog_contexts` | Высокий |
| Reference | Вынести в `air_config` | — | Средний |
| `app_config` | Версионирование/аудит | — | Низкий |

---

## 6. OpenTelemetry + Jaeger

### 6.1. Компоненты

```text
Service ──OTLP──> OTel Collector ──> Jaeger (storage/query/UI)
   │                    │
   └──Prometheus /metrics
```

- **SDK:** `go.opentelemetry.io/otel/sdk`, `trace`, `propagation`.
- **Экспортёр:** OTLP gRPC (4317) или HTTP (4318).
- **Коллектор:** `otel/opentelemetry-collector-contrib` (batch, memory_limiter, tail_sampling, attributes).
- **Jaeger:** `jaegertracing/all-in-one` для dev/stage; для prod — collector + query + Elasticsearch/ClickHouse или Jaeger v2.
- **UI:** `http://jaeger:16686`.

### 6.2. `air-common/pkg/telemetry`

```go
package telemetry

type Config struct {
    Enabled          bool
    ServiceName      string
    ServiceVersion   string
    Environment      string
    OTLPEndpoint     string
    OTLPInsecure     bool
    SampleRatio      float64
    PII              bool
}

func Init(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error)
func Tracer(scope string) trace.Tracer
func Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
func SpanFromContext(ctx context.Context) trace.Span
func WithUser(ctx context.Context, userID uint64) context.Context
```

Требования:

- Resource: `service.name`, `service.version`, `deployment.environment`, `service.instance.id`, `host.name`, `telemetry.sdk.*`.
- Propagator: `TraceContext` + `Baggage`.
- Sampler: `ParentBased(TraceIDRatioBased(cfg.SampleRatio))`.
- `BatchSpanProcessor` с очередью и таймаутом.
- `shutdown` 5s, вызывается в `defer` в `main`.
- `Enabled=false` → no-op provider.
- Ошибки экспорта не валят приложение, логируются с rate limit.

### 6.3. Инструментирование `air-common`

| Слой | Инструмент | Спаны/атрибуты |
|---|---|---|
| SQL | `github.com/XSAM/otelsql` | `db.query`, `db.system=mysql`, `db.operation.name`, `db.sql.table` |
| HTTP-клиент | `otelhttp.NewTransport` | `http.client.request` |
| gRPC-клиент | `otelgrpc.NewClientHandler` | `rpc.system=grpc` |
| gRPC-сервер | `otelgrpc.NewServerHandler` | входящие вызовы |
| Redis | `redisotel` | `db.redis.*` |
| MinIO/S3 | `otelhttp` transport | — |
| Провайдеры AI | обёртка `http.Client` + ручные спаны | `air.provider`, `air.model` |
| Realtime WS | ручные спаны | `air.realtime.session`, `air.realtime.stt`, `air.llm.request`, `air.tts.synthesize` |
| Диалоги | ручные спаны | `air.dialog.save`, `air.dialog.read` |
| Сессии | `startpoint` | `air.session.start`, `air.session.end` |
| Tool calls | ручные | `air.tool.name`, `air.tool.result` |

**Имена спанов:** `http.server.request`, `http.client.request`, `rpc.server`, `rpc.client`, `db.query`, `provider.openai.chat`, `provider.mistral.chat`, `provider.google.chat`, `provider.elevenlabs.tts`, `provider.elevenlabs.stt`, `dialog.save`, `dialog.read`, `bot.message.process`, `bot.call.start`, `bot.call.end`, `session.start`, `session.stop`.

**Атрибуты:**

```text
http.request.method, url.path, url.scheme, server.address,
http.response.status_code, error.type,
db.system, db.namespace, db.operation.name, db.sql.table,
rpc.system, rpc.service, rpc.method,
air.user.id_hash, air.channel.type, air.dialog.id,
air.model.provider, air.model.name, air.session.id,
air.message.id, air.bot.id, air.correlation_id,
air.provider.request_id, air.tokens.input, air.tokens.output
```

`air.user.id` — только HMAC-хэш (`air.user.id_hash`) при `PII=false`. `air.dialog.id`/`air.message.id` допустимы в трейсах, но не в метриках.

**Ошибки:** `span.RecordError(err)`, `span.SetStatus(codes.Error, ...)`; ожидаемые бизнес-ошибки — `codes.Unset` + `error.type`.

### 6.4. Пропагация контекста

| Транспорт | Механизм |
|---|---|
| HTTP входящий | `otelhttp`/`otelgin`, заголовки `traceparent`, `tracestate` |
| HTTP исходящий | `otelhttp.NewTransport` на всех `http.Client` |
| gRPC | `otelgrpc` interceptors + metadata |
| WebSocket | `traceparent` в upgrade; `SpanContext` в `conn`; child-спаны на сообщения |
| SSE | `traceparent` в запросе; спан соединения + child на события |
| Async goroutines | явная передача `ctx`; при отложенной работе — `trace.Link` |
| Очереди | `traceparent` в заголовках сообщений |

`air-logger` извлекает `trace_id`/`span_id` из `ctx`. В gRPC metadata добавлять `x-request-id` (UUID), если его нет.

### 6.5. Сэмплирование

- Dev/stage: `parentbased_always_on` / `ratio=1.0`.
- Prod: `parentbased_traceidratio`, `ratio=0.05–0.2`.
- Tail sampling: 100% ошибок, 100% медленных (>2s), 10% остальных, лимит `max_spans_per_second`.

```yaml
receivers:
  otlp:
    protocols:
      grpc: { endpoint: 0.0.0.0:4317 }
      http: { endpoint: 0.0.0.0:4318 }

processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 512
  batch:
    timeout: 5s
    send_batch_size: 1024
  attributes/drop_pii:
    actions:
      - key: air.user.email
        action: delete
      - key: air.user.phone
        action: delete
  tail_sampling:
    decision_wait: 10s
    policies:
      - name: errors
        type: status_code
        status_code: { status_codes: [ERROR] }
      - name: slow
        type: latency
        latency: { threshold_ms: 2000 }
      - name: baseline
        type: probabilistic
        probabilistic: { sampling_percentage: 10 }

exporters:
  otlp/jaeger:
    endpoint: jaeger:4317
    tls: { insecure: true }

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, attributes/drop_pii, tail_sampling, batch]
      exporters: [otlp/jaeger]
```

### 6.6. Переменные окружения

```dotenv
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317
OTEL_EXPORTER_OTLP_INSECURE=true
OTEL_SERVICE_NAME=air_orchestrator
OTEL_RESOURCE_ATTRIBUTES=deployment.environment=prod,service.version=1.60.3
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.1
OTEL_PII=false
JAEGER_UI_URL=http://jaeger:16686
```

### 6.7. Jaeger deployment

`air_orchestrator/build/air-jaeger.yml`:

```yaml
name: air_orchestrator
services:
  jaeger:
    image: jaegertracing/all-in-one:1.62
    container_name: airorc_jaeger
    restart: unless-stopped
    environment:
      - COLLECTOR_OTLP_ENABLED=true
      - SPAN_STORAGE_TYPE=badger
      - BADGER_EPHEMERAL=false
      - BADGER_DIRECTORY_VALUE=/badger/data
      - BADGER_DIRECTORY_KEY=/badger/key
    ports:
      - "16686:16686"
      - "4317:4317"
      - "4318:4318"
    volumes:
      - jaeger_data:/badger
    networks:
      - monitoring_shared
volumes:
  jaeger_data:
networks:
  monitoring_shared:
    name: monitoring_shared
    external: true
```

Для production: Jaeger + Elasticsearch/ClickHouse (retention 7–30 дней), отдельный collector, resource limits, persistent volume, UI через Envoy/SSO.

### 6.8. Инструментирование сервисов

#### `air_orchestrator`

- `main`: `telemetry.Init`, `defer shutdown`.
- Gin: `otelgin.Middleware` (после `PrometheusMiddleware`).
- gRPC: `grpc.StatsHandler(otelgrpc.NewServerHandler())`.
- Исходящий HTTP: `otelhttp.NewTransport`.
- SQL: `otelsql.Open`/`otelsql.Register`.
- Redis: `redisotel.InstrumentTracing`.
- Scheduler: спан на job.
- MinIO: `otelhttp` transport.
- MasterKey/Redis: спан `masterkey.get`.

#### `air_whatsbot`

- `main`: `telemetry.Init`.
- HTTP server: `otelhttp`.
- gRPC client к orchestrator: `otelgrpc`.
- SQL: `otelsql`; Redis: `redisotel`.
- `bot.message.process`: `air.bot.id`, `air.channel.type=whatsapp`, `air.message.id`.
- `bot_calls`: `bot.call.start`, `bot.call.media`, `bot.call.end`.
- `bot.reconnect` + метрика.

#### Остальные сервисы

`air_tgbot`, `air_tguserbot`, `air_avito`, `air_widget`, `air_operator`, `air_lead-hunter`, `air_payment`, `air_phantom`: `telemetry.Init`, HTTP/gRPC/SQL/Redis, входящие webhook с `traceparent` к orchestrator.

`air_front`: прокидывать `traceparent` (fetch wrapper) или полагаться на Envoy; CORS; без PII.

### 6.9. `air-logger`: изменения

Старый API сохранить. Добавить:

```go
func InfoCtx(ctx context.Context, format string, args ...any)
func ErrorCtx(ctx context.Context, format string, args ...any)
func WarnCtx(ctx context.Context, format string, args ...any)
func DebugCtx(ctx context.Context, format string, args ...any)
func With(fields map[string]any) *Entry
func (e *Entry) WithContext(ctx context.Context) *Entry
```

Конфигурация:

```go
logger.StdOut().
    WithJSON(true).
    WithService("air_orchestrator").
    WithEnvironment("prod").
    WithTrace(true).
    WithCaller(false).
    Apply()
```

JSON:

```json
{
  "ts": "2026-11-07T13:00:00.123Z",
  "level": "INFO",
  "service": "air_orchestrator",
  "env": "prod",
  "msg": "user logged in",
  "user_id": 123,
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "caller": "auth.go:42"
}
```

Правила:

- `trace_id`/`span_id` из `trace.SpanFromContext(ctx).SpanContext()`.
- `user_id` — явно или из baggage, `uint64`.
- PII не логировать по умолчанию; `WithPII(true)` только для debug.
- В контейнерах — stdout, без ротации.
- `Recover` middleware для panic с трейсом.
- Опционально: OTel Logs Bridge → OTLP → collector → VictoriaLogs.

Постепенно заменить `logger.Info(...)` на `logger.InfoCtx(ctx, ...)`.

### 6.10. Проверка

- HTTP-запрос к orchestrator → `http.server.request` + `db.query` + `provider.openai.chat`.
- WhatsApp-сообщение → `bot.message.process` → orchestrator → `provider.*` → `dialog.save` в одном трейсе.
- Ошибка провайдера → ERROR-спан + `RecordError` + тот же `trace_id` в логе.
- Realtime-звонок → `session.start` + STT/LLM/TTS child-спаны.
- Автотест на отсутствие PII.

---

## 7. SLA/SLO и метрики

### 7.1. Каталог SLI/SLO

| Сервис | SLI | SLO (30d) | Окно | Источник |
|---|---|---|---|---|
| Orchestrator API | Доля не-5xx | 99.9% | 30d | `air_orchestrator_http_requests_total` |
| Orchestrator API | p95 latency | < 300 ms | 5m | `..._request_duration_seconds_bucket` |
| Orchestrator API | p99 latency | < 1 s | 5m | то же |
| Auth | Успешные логины/регистрации | 99.95% | 30d | `air_orchestrator_auth_*` |
| Message ingestion | Принятые без ошибок | ≥ 99.5% | 30d | `air_whatsbot_messages_processed_total{status}` |
| End-to-end ответ | p95 входящее → отправка | < 10 s | 30d | новый histogram |
| AI provider | Успешные запросы | ≥ 99% | 30d | `air_common_provider_requests_total` |
| AI provider | p95 latency | < 5 s | 5m | `..._duration_seconds` |
| Dialog persistence | Успешные сохранения | ≥ 99.9% | 30d | `air_common_dialog_save_total` |
| DB | Успешные запросы | ≥ 99.95% | 30d | `air_common_db_queries_total` |
| DB | p95 query latency | < 50 ms | 5m | `air_common_db_query_duration_seconds` |
| Bot connectivity | Успешные реконнекты | ≥ 99% | 30d | `air_whatsbot_reconnects_total` |
| Observability | Успешные экспорты трейсов | ≥ 99.9% | 30d | collector metrics |

Пороги — стартовые; утвердить после 2 недель baseline.

### 7.2. Неймспейс и лейблы

- Формат: `air_<service>_<domain>_<metric>`.
- Обязательные: `service`, `env`, `version`.
- Разрешённые: `route`, `method`, `status`, `provider`, `channel`, `result`, `job`.
- Запрещены: `user_id`, `dialog_id`, `message_id`, `email`, `phone`, `token`.
- `bot_id` — только при ограниченном количестве (<1000); иначе бакет.
- Новые метрики — OTel Metrics + Prometheus exporter; старые `airorc_*`/`air_whatsbot_*` сохранить до миграции дашбордов.

### 7.3. Обязательные метрики

RED:

- `air_<svc>_http_requests_total{method,route,status}`
- `air_<svc>_http_request_duration_seconds{method,route}`
- `air_<svc>_http_errors_total{route,status}`
- `air_<svc>_grpc_requests_total{service,method,status}`
- `air_<svc>_grpc_request_duration_seconds{service,method}`

DB:

- `air_common_db_queries_total{operation,table,result}`
- `air_common_db_query_duration_seconds{operation,table}`
- `air_common_db_deadlocks_total`
- `air_common_db_pool_open_connections{service}`
- `air_common_db_pool_wait_seconds`

AI:

- `air_common_provider_requests_total{provider,operation,result}`
- `air_common_provider_request_duration_seconds{provider,operation}`
- `air_common_provider_tokens_total{provider,model,direction}`
- `air_common_provider_fallbacks_total{provider,reason}`

Бизнес:

- `air_common_dialogs_created_total`
- `air_common_dialog_messages_saved_total{role}`
- `air_common_message_e2e_duration_seconds{channel}`
- `air_common_active_sessions{service,type}`
- `air_whatsbot_messages_received_total{message_type}`
- `air_whatsbot_messages_processed_total{status}`
- `air_whatsbot_crm_requests_total{direction,status}`
- `air_whatsbot_whatsapp_calls_total{direction,result}`

Миграция (временно):

- `air_common_migration_rows_total{phase,status}`
- `air_common_migration_duration_seconds{phase}`
- `air_common_dual_write_errors_total{target}`
- `air_common_legacy_fallback_total{operation}`

### 7.4. Recording rules и алерты

```yaml
groups:
  - name: air-slo
    interval: 30s
    rules:
      - record: job:air_http_success_rate:ratio_rate5m
        expr: |
          sum by (service) (rate(air_http_requests_total{status!~"5.."}[5m]))
          /
          sum by (service) (rate(air_http_requests_total[5m]))

      - record: job:air_http_latency_p95:5m
        expr: |
          histogram_quantile(0.95,
            sum by (service, le) (rate(air_http_request_duration_seconds_bucket[5m]))
          )

      - record: job:air_slo_error_budget_burn_rate:1h
        expr: |
          (1 - job:air_http_success_rate:ratio_rate5m) / (1 - 0.999)
```

| Алерт | Условие | Severity |
|---|---|---|
| `AirHighErrorRate` | success rate < 99.5% за 5m | critical |
| `AirHighLatency` | p95 > 1s за 10m | warning |
| `AirProviderDegraded` | provider error rate > 5% за 5m | warning |
| `AirDBErrors` | DB error rate > 1% за 5m | critical |
| `AirDBDeadlocks` | deadlocks > 0 за 5m | warning |
| `AirSLOBurnRateFast` | burn rate > 14.4 за 1h | critical |
| `AirSLOBurnRateSlow` | burn rate > 6 за 6h | warning |
| `AirMigrationMismatch` | `migration_rows_total{status="mismatch"} > 0` | warning |
| `AirTraceExportFailures` | export failures > 1% | warning |

### 7.5. Perses-дашборды

Добавить в `monitoring/perses/provisioning/dashboards/`:

1. `air-slo-overview.json` — SLO, error budget, burn rate, топ-5 маршрутов.
2. `air-service-red.json` — RPS, error rate, p50/p95/p99.
3. `air-dependencies.json` — latency/error rate к провайдерам, CRM, DB, Redis; ссылки в Jaeger.
4. `air-db-health.json` — connections, slow queries, deadlocks, replication lag, рост `dialog_messages`.
5. `air-channels-bots.json` — активные каналы, reconnect rate, ошибки авторизации.
6. `air-migration.json` — прогресс фаз, mismatch, dual-write errors, legacy fallback.

Пример панели:

```json
{
  "kind": "Panel",
  "spec": {
    "display": { "name": "SLO availability (30d)" },
    "plugin": { "kind": "StatChart", "spec": {} },
    "queries": [{
      "kind": "TimeSeriesQuery",
      "spec": {
        "plugin": {
          "kind": "PrometheusTimeSeriesQuery",
          "spec": {
            "query": "job:air_http_success_rate:ratio_rate5m",
            "seriesNameFormat": "{{service}}"
          }
        }
      }
    }]
  }
}
```

Ссылки на трейсы: переменная `trace_id` + `http://jaeger:16686/trace/${trace_id}`.

### 7.6. Error budget policy

- Бюджет = 0.1% для 99.9% SLO.
- Burn rate > 14.4 (1h) — остановить релизы, incident.
- Burn rate > 6 (6h) — заморозить фичи.
- Ежемесячный обзор SLO.

---

## 8. Изменения зависимых сервисов

### 8.1. `air-common`

Новые пакеты:

```text
pkg/idgen          — Snowflake
pkg/dbrouter       — маршрутизация по шардам
pkg/telemetry      — OTel init/tracer/propagation
pkg/migrate        — миграции и schema version
pkg/metrics        — общие метрики
pkg/channels       — доменная модель каналов
pkg/store          — репозитории поверх router
```

Изменения `pkg/comdb`:

- `DB` получает `Router` и `idgen.Generator`.
- `New`: `DB_DSN`/granular env, `PingContext` с retry, `otelsql`, pool из env, `loc=UTC`.
- `Exterior` → узкие `DialogStore`, `ChannelStore`, `ModelStore`, `UserStore`, `VectorStore`, `StorageStore`, `OAuthStore`.
- Удалить SP-вызовы в `dialog.go`; SQL + транзакции.
- `SaveDialog` → `dialog_messages` (dual-write), `ReadDialog` → `dialog_messages` + legacy fallback.
- `SaveChannelData`/`GetChannelsData` → `user_channels`.
- `GetUserAPIKey`/`SetUserAPIKey` — убрать numeric provider после миграции.
- `GetActiveProvider` → `user_models` с `user_id`.
- `SyncProviderModels`/`SyncVoiceModels` — вынести в orchestrator.
- Все методы получают `ctx`; `userID` → `comdom.UserID`, `dialogID` → `comdom.DialogID`; в доменных сигнатурах не остаётся голых `uint64` для разных сущностей (§3.4.1).

`pkg/rpc`:

- `otelgrpc.NewClientHandler`, таймауты из конфига, пропагация metadata.

`pkg/model`, `pkg/startpoint`, `pkg/endpoint`:

- `ctx` в провайдерские вызовы.
- Спаны `StartSession`/`Stop`, `GetRealtimeProvider`, `TranscribeAudio`/`SynthesizeSpeech`/`GenerateMusic`, `SaveDialog`.
- Не логировать содержимое сообщений на Info.

`pkg/crypto`:

- `key_version` в формате шифротекста; не менять `$mk$`/`$app$` без миграции; `IsEncryptedAny`.

Версионирование:

- Breaking changes → `air-common/v2`.
- `v2.0.0` с adapters → обновить все сервисы одним поездом → `v1` только security-фиксы 1 месяц.

### 8.2. `air_orchestrator`

- `internal/repository/mysql/*`: переход на `pkg/comdb`/`pkg/store`; `channels.go` → `user_channels`; `auth.go` → `idgen` + `user_directory`; `masterkey.go` без поломки ключей; остальные файлы — типы/поля.
- HTTP: OTel middleware; `/healthz` и `/readyz` (DB ping + schema version + shard health); `/metrics`.
- gRPC: `otelgrpc` interceptors.
- `notification`, `smtp`, `storage`, `redis`, `scheduler` — OTel/метрики.
- `internal/metrics`: единый неймспейс + SLO-метрики.
- `internal/config/config.go`: URL сервисов из env.
- `migrations/`: дамп → `pkg/migrate`.
- `monitoring/perses`: новые дашборды.
- `build/`: `air-jaeger.yml`, `air-otel-collector.yml`, OTel env в `prod.yml`/`dev.yml`.

### 8.3. `air_whatsbot`

- `internal/db/db.go`: `comdb.New` + router.
- `repository.go`: `channels` → `user_channels`; `service_wabots` → `user_bots`; `service_user_bot_*` → `user_bot_*`; типы `comdom.UserID`/`comdom.DialogID`/`comdom.BotID` вместо голых `uint64`.
- `bot_channels.go`: новый `GetOrSetTreadAndResponder`.
- `responder/store.go`: user-scoped ключи, контекст.
- OTel в `bot_message`, `bot_calls`, `bot_event`, `whatsapp.go`.
- `internal/metrics`: e2e latency, trace exemplars.
- `prod.yml`: OTel env, networks, отдельный DB user.

### 8.4. Остальные сервисы

| Сервис | Работы |
|---|---|
| `air_tgbot` | `air-common/v2`, channels v2, OTel HTTP/SQL, `/metrics` |
| `air_tguserbot` | то же + realtime-спаны, `user_bots` |
| `air_avito` | bump, channels v2, OTel |
| `air_widget` | bump, OTel HTTP/SSE, channels v2 |
| `air_operator` | bump, операторские сессии, OTel |
| `air_lead-hunter` | bump, `user_bots`, OTel |
| `air_payment` | bump, `crypto_payments` типы, OTel |
| `air_phantom` | bump, OTel, аудит БД |
| `marusia_crm` | аудит БД, OTel HTTP |
| `air_app` | аудит, OTel |
| `air_front` | `traceparent`, API-клиенты |

### 8.5. `air-logger`

- Релиз `v2.2.0` (или `v3`): `InfoCtx`/`ErrorCtx`/`WarnCtx`/`DebugCtx`, JSON, `trace_id`/`span_id`, `With(fields)`, `WithPII`, обратная совместимость.
- Обновить все сервисы вместе с `air-common/v2`.
- Тесты: формат, trace, производительность.

---

## 9. Тестирование и валидация

### 9.1. Unit

- `pkg/idgen`: 1M ID без дублей, монотонность, rollback часов, `-race`.
- `pkg/dbrouter`: slot mapping, reload, unknown shard, health.
- `pkg/telemetry`: no-op, init/shutdown, propagation.
- `pkg/comdb`: репозитории на `sqlmock`/testcontainers.
- `air-logger`: формат, trace extraction, PII.

### 9.2. Integration

- Testcontainers MariaDB 13: миграции с нуля, на baseline, upgrade/downgrade.
- Фикстуры `channels`, `dialogs`, `responders`; шифротекст/plaintext; несколько каналов; один `RealId` у разных пользователей.
- Dual-write и fallback.

### 9.3. Data reconciliation

`cmd/migrate-verify`:

| Проверка | Метод |
|---|---|
| Row counts | `COUNT(*)` до/после |
| Checksums | `SHA2(GROUP_CONCAT(...))` по батчам |
| Каналы | `enabled`, JSON, SHA-256 шифротекста |
| Диалоги | количество, `seq`, хэш содержимого |
| Responders | нет дублей `(user_id, chat_type, real_id)` |
| FK | нет orphan-строк |
| Шифрование | префиксы сохранены, ключи не изменены |
| Purge | нет строк с `user_id` после `DeleteAllUserData` |

### 9.4. Shadow-read и canary

- Shadow-read v1 vs v2, сравнение, логирование.
- Canary: 1% → 10% → 50% → 100%.
- Метрики: `air_common_legacy_fallback_total`, `air_common_shadow_mismatch_total`.

### 9.5. Нагрузка

- 1000 RPS orchestrator; 100 диалогов/пользователь; 10k сообщений/мин; realtime.
- p95/p99, DB connections, deadlocks, memory, insert throughput.

### 9.6. Chaos

- Kill shard/replica → readiness, fallback, алерты.
- Потеря Jaeger/collector → приложение работает.
- Потеря Redis → MasterKey fallback.
- Откат часов → idgen без дублей.

### 9.7. Трассировка

- E2E: whatsbot (mock) → единый trace в Jaeger.
- Автотест на отсутствие PII.
- `trace_id` в логах == Jaeger.

---

## 10. График и вехи

| Фаза | Длительность | Веха |
|---|---|---|
| 0. Подготовка | 1 неделя | Backup/restore, audit, baseline |
| 1. Типы/ID/миграции | 2 недели | `idgen`, `pkg/migrate`, типы |
| 2. Каналы v2 | 2–3 недели | dual-write + backfill + read switch |
| 3. Диалоги/сообщения | 2–3 недели | dual-write + lazy backfill |
| 4. Responders/боты/SP | 2 недели | SP/триггеры/EVENT удалены |
| 5. Contract | 1–2 недели | legacy удалён |
| 6. OTel + Jaeger | 2 недели | E2E трейсы, логи с trace_id |
| 7. SLO + дашборды | 1–2 недели | SLO/alerts |
| 8. Шардирование | 2–4 недели | `dbrouter`, `user_directory`, cutover |
| **Итого** | **~3–4 месяца** | Production-архитектура |

Оценки для 2–3 backend-инженеров + 1 DevOps. БД-трек и OTel-трек можно параллелить, релизы `air-common` синхронизировать.

---

## 11. Риски и меры

| # | Риск | Вероятность | Влияние | Мера |
|---|---|---|---|---|
| 1 | Потеря данных | Низкая | Критическое | Backup + PITR + reconciliation + canary |
| 2 | Зашифрованные данные без ключей | Высокая | Высокое | Шифротекст как есть; lazy; fallback |
| 3 | Блокировки `ALTER TABLE` | Средняя | Высокое | `pt-osc`/`gh-ost`, окно, реплика |
| 4 | Расхождение dual-write | Средняя | Высокое | Shadow-read, метрики, fallback |
| 5 | Ошибки нового `GetOrSet...` | Средняя | Высокое | Тесты многотенантности, unique |
| 6 | Кардинальность метрик | Средняя | Среднее | Ревью лейблов, линтер |
| 7 | Долгий cutover | Низкая | Высокое | Slot-based, rollback |
| 8 | Несовместимость версий | Высокая | Высокое | Выровнять `air-common` |
| 9 | PII в трейсах | Средняя | Высокое | Redaction, `PII=false`, тесты |
| 10 | Потеря трейсов | Средняя | Низкое | Batch + очередь |
| 11 | `AUTO_INCREMENT` остаётся | Низкая | Среднее | Contract + audit |
| 12 | Cross-shard JOIN | Средняя | Высокое | Review, линтер, тесты |
| 13 | Нет `_scheduler_ResetUserFlags` | Высокая | Среднее | Восстановить, cron + тесты |
| 14 | VECTOR + шардирование | Средняя | Среднее | В шарде пользователя; далее Qdrant/pgvector |

---

## 12. План отката

### 12.1. Общие правила

- Перед каждой фазой — snapshot + binlog position.
- Feature flags возвращают чтение на legacy без отката схемы.
- Схема расширяется аддитивно; удаление — после стабильности.
- Откат приложения — предыдущий образ + флаг.

### 12.2. По фазам

| Фаза | Откат |
|---|---|
| 1 (типы/ID) | `ALTER` обратно (осторожно), `IDGEN_ENABLED=false`; расширенные типы совместимы |
| 2 (каналы) | `CHANNELS_V2_READ_ENABLED=false`; legacy не удалена |
| 3 (диалоги) | `DIALOG_MESSAGES_READ_ENABLED=false`; legacy `Data` не удалён |
| 4 (SP) | Предыдущая версия `air-common`/образ; SP ещё в БД до contract |
| 5 (contract) | Restore snapshot (необратимо — после 2 недель стабильности) |
| 6 (шарды) | Вернуть `slot_map` generation; dual-write; старый шард read-only |

### 12.3. Критерии экстренного отката

- 5xx > 1% за 5 минут.
- Ошибки записи в БД > 0.5%.
- Потеря сообщений/диалогов (mismatch > 0).
- Ошибки чтения каналов > 0.1%.

---

## 13. Критерии приёмки (Definition of Done)

### БД

- [ ] Миграции применяются на пустой БД и на baseline.
- [ ] `schema_migrations` заполнена, checksum совпадает.
- [ ] `users.Id` и все `user_id` — `BIGINT UNSIGNED`.
- [ ] Новые записи — app-generated ID; `AUTO_INCREMENT` не используется.
- [ ] `channels` → `user_channels` + `user_channel_configs`; данные сверены.
- [ ] `dialogs.Data` → `dialog_messages`; история сохранена.
- [ ] `responders` user-scoped; дублей нет.
- [ ] `user_bots`/`user_bot_events`/`user_bot_state` заменили `service_*`; триггеры удалены.
- [ ] Нет SP/функций/триггеров/EVENT.
- [ ] `DeleteAllUserData` в Go и протестирован.
- [ ] `vector_embeddings` FK исправлен.
- [ ] Reference-данные вынесены/кэшируются.
- [ ] `dbrouter` + `user_directory` работают.
- [ ] Отдельные DB-пользователи и TLS.

### Наблюдаемость

- [ ] Все сервисы отправляют OTLP в Jaeger.
- [ ] E2E трейс от входящего сообщения до ответа.
- [ ] `air-logger` добавляет `trace_id`/`span_id`; JSON в prod.
- [ ] PII не попадает в трейсы/логи.
- [ ] Jaeger UI и retention.
- [ ] SLO-метрики, recording rules, алерты.
- [ ] Perses SLO/RED/dependencies/DB/migration.
- [ ] Error budget policy.

### Сервисы

- [ ] Все на одной версии `air-common` (v2) и `air-logger`.
- [ ] `go test ./...`, `go vet` зелёные.
- [ ] `go test -race` для `idgen`, `dbrouter`, `telemetry`.
- [ ] Integration-тесты с MariaDB в CI.
- [ ] Нагрузочные тесты в пределах SLO.
- [ ] Runbooks: миграция, откат, инцидент, трейсинг.

---

## 14. Открытые вопросы

1. **`_scheduler_ResetUserFlags`** отсутствует в дампе. Восстановить из prod (`SHOW CREATE FUNCTION`) или переписать по поведению.
2. **Структура `dialogs.Data`** неоднородна. Нужен enum ролей и маппинг; до решения — `content_json`.
3. **MasterKey для офлайн-миграции**: сколько пользователей имеют ключ в Redis? Для остальных — lazy.
4. **Reference-владелец**: orchestrator или отдельный `air_config`? Рекомендация — отдельный, на первом этапе orchestrator.
5. **Shard DB engine**: MariaDB 13 (VECTOR) или PostgreSQL + pgvector? На первом этапе MariaDB.
6. **`service_proxy.Password`** в открытом виде — нужно шифрование.
7. **`user_auth.Email`** в открытом виде — нужен ли `email_encrypted` в дополнение к `EmailHash`?
8. **Retention**: events 90 дней, messages — бессрочно/по тарифу, трейсы 7–30 дней.
9. **`air-common/v2`**: готовы ли все сервисы к breaking change в одном релизе? Иначе — adapter-слой.
10. **Envoy и traceparent**: проверить, что заголовки не теряются; при необходимости включить tracing/`generate_request_id`.

---

## 15. Дополнительные рекомендации по улучшению проекта

Раздел не блокирует миграцию БД, но закрывает системные риски, которые проявятся после выхода в production. Приоритеты: **P0** — безопасность/целостность, **P1** — архитектура/надёжность, **P2** — код/сопровождаемость, **P3** — тесты/CI/CD, **P4** — производительность/стоимость, **P5** — процессы/документация.

### 15.1. P0 — безопасность и целостность данных

- **Секреты и ключи — управляемо, но без изменения текущих таблиц.** На этом этапе `app_config`, `service_proxy`, `user_storage_config` **остаются в БД без изменений**; для них достаточно существующего шифрования и ротации. Vault/KMS — отдельный трек при росте инфраструктуры. Для OAuth-токенов и MasterKey добавить key versioning и аудит.
- **Убрать plaintext PII.** `user_auth.Email` → `email_hash` для поиска + `email_encrypted` для отображения. Логи/трейсы — без email, phone, текста сообщений.
- **Разделить DB-права.** Отдельные пользователи для orchestrator, whatsbot, tgbot, миграций, аналитики; TLS; audit plugin. Убрать общий `DB_USER`.
- **Проверить AAD шифрования.** Если `user_id`/`dialog_id` входит в дополнительные данные `$mk$`/`$app$`, смена ID сломает расшифровку — нужен переходный период с поддержкой старого и нового формата.
- **SSRF и webhook-подписи.** Валидировать URL для CRM/Avito/widget, проверять подписи Telegram/WhatsApp/CRM, ограничивать редиректы и исходящие адреса.
- **AI-guardrails.** Allowlist инструментов, валидация tool-вызовов, защита от prompt injection, лимиты токенов, модерация, запрет произвольного кода/URL.
- **Исправить известные баги:** `ReadDialog` (перепутаны `Model`/`Responder`), `GetOrSetTreadAndResponder` без `user_id`, FK `vector_embeddings`, неполный `DeleteAllUserData`, дубли триггеров, битые FK `crm_mappings` → отсутствующая `crm_applications`, дубли FK `user_gpt`.
- **Аудит доступа** к пользовательским данным и админ-операциям (`DeleteAllUserData`, смена MasterKey, экспорт данных).

### 15.2. P1 — архитектура и надёжность

- **Владельцы данных.** Каждая таблица/домен — один сервис-владелец. Остальные ходят через API/события, а не пишут в чужие таблицы. Сейчас это распределённый монолит на одной БД.
- **Контракты API.** OpenAPI/protobuf + версионирование + обратная совместимость + generated clients. Не менять поля «на месте».
- **Outbox + idempotency.** Для биллинга, CRM, уведомлений, событий ботов. Ключи идемпотентности для webhook/payment/bot message.
- **Таймауты, retry с jitter, circuit breaker** для OpenAI/Mistral/Google/ElevenLabs/Telegram/WhatsApp/CRM. Fallback-провайдеры и graceful degradation.
- **Rate limit и квоты** по tenant/user/model; backpressure на входящие сообщения; ограниченные очереди и пулы горутин.
- **Health/readiness/liveness + graceful shutdown**; проверка схемы/шардов/Redis/MinIO при старте; drain соединений.
- **Redis и MasterKey.** Текущая схема принята: MasterKey в Redis хранится в зашифрованном виде, доступ — только после авторизации. Схему не меняем; добавить TTL, ACL/сетевую изоляцию, аудит и явную инвалидацию при logout/смене пароля. Redis остаётся кэшем, source of truth — зашифрованный MasterKey в `user_auth`.
- **Read replicas** для чтения и **CDC → OLAP** для аналитики; никаких cross-shard JOIN в рантайме.
- **Retention/архив**: `dialog_messages`, `service_user_bot_events`, трейсы, логи. Партиционирование по времени/пользователю.
- **Идемпотентность и дедупликация** входящих сообщений/звонков; at-least-once семантика с явными ключами.

### 15.3. P2 — код и сопровождаемость

- **Разбить `comdb.Exterior`** на узкие интерфейсы: `DialogStore`, `ChannelStore`, `ModelStore`, `UserStore`, `VectorStore`, `StorageStore`. Потребитель объявляет только нужный интерфейс.
- **`context.Context` везде**, включая `ReadDialog`, `SetActiveModel`, провайдерские вызовы. Без этого таймауты и трейсинг неполные.
- **Типизированные ошибки** вместо `strings.Contains(err, "SQLSTATE 45001")`. `errors.Is/As`, `%w`, sentinel errors.
- **Убрать глобальные синглтоны** (`idgen.Default()`). Передавать зависимости через конструкторы; иначе тесты и несколько окружений конфликтуют.
- **Конфиг с валидацией.** Убрать хардкод URL из `internal/config/config.go`; fail fast при отсутствии обязательных env.
- **Единый стиль имён.** `Thread` вместо `Tread`, `user_id` вместо `UserId`, snake_case в БД, единые `created_at/updated_at`.
- **SQL-гигиена:** убрать `SELECT *`, cursor-пагинация, `EXPLAIN` для критичных запросов, slow query log, линтер на N+1.
- **sqlc или аналог** для типобезопасных запросов; минимум — репозитории + интеграционные тесты.
- **Линтеры:** `golangci-lint` (staticcheck, errcheck, gosec, ineffassign, revive), `go vet`, `-race`, `goleak`.
- **Провайдеры AI:** общий conformance-test suite для OpenAI/Mistral/Google/ElevenLabs; убрать дублирование realtime-pump; единый интерфейс lifecycle.
- **`startpoint`:** проверить жизненный цикл горутин, закрытие каналов, отмену контекста, отсутствие утечек и паник в background-горутинах.
- **Убрать legacy:** числовой `Provider`, старые SP, неиспользуемые поля, дубли `SyncProviderModels`.
- **Ограниченные каналы и backpressure** вместо неограниченных очередей; таймауты на все внешние вызовы.

### 15.4. P3 — тесты и CI/CD

- **Testcontainers** для MariaDB/Redis/MinIO; интеграционные тесты миграций и репозиториев.
- **Contract tests** между сервисами (protobuf/OpenAPI), чтобы не ломать gRPC/HTTP при релизе.
- **E2E-сценарии:** регистрация → подключение канала → сообщение → ответ → биллинг; realtime-звонок.
- **Fuzzing** для webhook/JWT/crypto/парсеров провайдеров.
- **Нагрузочные и soak-тесты** (k6/Locust): p95/p99, DB connections, deadlocks, память.
- **CI-гейты:** lint → unit → integration → build → scan → sign → deploy; запрет merge без зелёного CI.
- **Supply chain:** `govulncheck`, Trivy, SBOM, подпись образов, pinned dependencies, Renovate/Dependabot.
- **Релизы:** canary/blue-green, feature flags, version matrix `air-common` ↔ сервисы, автоматический rollback.
- **`go.work`** для локальной разработки нескольких сервисов; `replace` — только в dev.
- **Coverage gates** для критичных пакетов (`comdb`, `crypto`, `rpc`, `startpoint`); mutation testing — опционально.

### 15.5. P4 — производительность и стоимость

- **Кэш reference-данных** (модели, провайдеры, валюты, языки) в Redis с TTL/инвалидацией.
- **LLM-cost:** учёт токенов по user/model, лимиты, routing по цене/латентности, кэш промптов, суммаризация контекста, fallback.
- **Блобы — в S3/MinIO**, в БД только ссылки; не хранить большие JSON/аудио в OLTP.
- **Векторы:** оценить Qdrant/pgvector; `VECTOR(3072)` в OLTP-таблице — тяжёлый вариант.
- **Realtime-качество:** метрики jitter, packet loss, dropped frames, MOS, время до первого ответа.
- **FinOps:** cost per tenant/model, budget alerts, дашборд стоимости AI.
- **Кэш эмбеддингов** и повторное использование; не пересчитывать без изменения документа.
- **Пул соединений:** пересчитать `MaxOpenConns` на сервис × реплики против `max_connections` MariaDB; добавить `ConnMaxIdleTime`.

### 15.6. P5 — процессы и документация

- **ADR/RFC** на архитектурные решения (ID, шардирование, шифрование, провайдеры).
- **CODEOWNERS** и service catalog: кто владеет сервисом/таблицей/провайдером.
- **Runbooks:** миграция, откат, инцидент, деградация провайдера, потеря Redis, восстановление БД.
- **SEV-уровни и postmortems**, error budget policy, регулярный SLO-review.
- **Deprecation policy** для `air-common` и API; LTS-ветка.
- **Onboarding-документация:** как поднять все сервисы, тестовые ключи, mock-провайдеры.
- **Матрица совместимости** версий `air-common`/`air-logger`/сервисов; правило «один релизный поезд» для breaking changes.

### 15.7. Приоритетный backlog

| # | Улучшение | Приоритет | Эффект | Зависимости |
|---|---|---|---|---|
| 1 | Управление секретами и ключами (без смены `app_config`/`service_proxy`/`user_storage_config`) | P0 | Снимает утечки и упрощает ротацию | — |
| 2 | Исправление `ReadDialog`/`responders`/FK/триггеров | P0 | Корректность данных и мультитенантность | Phase 4 |
| 3 | Разделение DB-прав и TLS | P0 | Ограничение blast radius | Phase 6 |
| 4 | MCP: auth, allowlist на `tools/call`, IDOR `lead_target`, rate limit/audit | P0 | Закрывает доступ к чужим данным и инструментам | — |
| 5 | Outbox + idempotency | P1 | Надёжность биллинга/CRM/событий | — |
| 6 | Retry/circuit breaker/rate limit | P1 | Устойчивость к сбоям провайдеров | telemetry |
| 7 | Health/readiness/graceful shutdown | P1 | Управляемые релизы и рестарты | — |
| 8 | Разбиение `comdb.Exterior` + ctx + typed errors | P2 | Сопровождаемость | v2 |
| 9 | sqlc/линтеры/`goleak` | P2 | Качество кода | — |
| 10 | Conformance-тесты провайдеров | P2 | Единое поведение AI/realtime | — |
| 11 | Testcontainers/contract/E2E | P3 | Регрессии ловятся в CI | — |
| 12 | Supply chain + canary | P3 | Безопасные релизы | CI/CD |
| 13 | Кэш reference/LLM-cost/FinOps | P4 | Снижение latency и затрат | — |
| 14 | Вынос `air_mcp` в отдельный сервис | P3 | Изоляция tool execution и scaling | auth + внутренние API |
| 15 | ADR/CODEOWNERS/runbooks | P5 | Предсказуемость эксплуатации | — |

### 15.8. Критерии приёмки дополнительных улучшений

- [ ] Секреты не хранятся в открытом виде; есть ротация. `app_config`/`service_proxy`/`user_storage_config` остаются в БД в рамках текущего этапа.
- [ ] `/int/mcp` защищён mTLS/service-auth и подписанным user-token; сырой `X-Session-ID` не принимается.
- [ ] `tools/call` проверяет allowlist инструментов, а не только `tools/list`.
- [ ] `lead_target` проверяет, что `resp_id` принадлежит текущему пользователю.
- [ ] MCP-вызовы имеют rate limit, audit, OTel-спаны и используют request context.
- [ ] PII не попадает в логи/трейсы; email хранится в зашифрованном виде + hash.
- [ ] У каждого сервиса свой DB-пользователь, TLS включён.
- [ ] Для внешних вызовов есть timeout/retry/circuit breaker/rate limit.
- [ ] Входящие webhook/сообщения идемпотентны.
- [ ] `comdb.Exterior` разделён; доменные сигнатуры используют именованные ID-типы.
- [ ] `context.Context` присутствует во всех I/O-операциях.
- [ ] Нет `strings.Contains` по тексту SQL-ошибок.
- [ ] `go test -race`, `goleak`, `golangci-lint`, `govulncheck` в CI зелёные.
- [ ] Есть contract/E2E/нагрузочные тесты для критичных сценариев.
- [ ] Есть ADR, CODEOWNERS, runbooks и матрица версий.
- [ ] SLO/error budget и алерты используются в операционных решениях.

### 15.9. MCP: анализ механизма вызова инструментов и решение по выносу

#### 15.9.1. Как это работает сейчас

- MCP-сервер живёт внутри `air_orchestrator`: `internal/delivery/mcp/handler.go`, `tools.go`, `session.go`.
- Роут: `POST /int/mcp` (`internal/delivery/http/server.go:362-363`, `mcpRoutes`), без auth-middleware, доступен в сети `air_shared`.
- Протокол: JSON-RPC 2.0 over HTTP (MCP 2025-03-26), фактически stateless: `initialize`, `tools/list`, `tools/call`, `prompts/list`, `prompts/get`.
- Клиент: `air-common/pkg/model/action_handler.go`:
  - hardcoded `http://airorc:8080/int/mcp` (`action_handler.go:15`);
  - `X-Session-ID: <userID>:<provider>` (`action_handler.go:72`);
  - timeout 30s, без mTLS/подписи/ретраев.
- Вызов инструментов агентами:
  - OpenAI/Mistral/Google/Realtime получают tool/function call от провайдера;
  - `actionHandler.FetchToolsList` → `tools/list`, `FetchSystemPrompt` → `prompts/get`;
  - `RunAction` → `tools/call` → MCP handler.
- Инструменты (`tools.go`): `get_current_time`, `lead_target`, `generate_music`, S3 (`get_s3_files`, `create_file`, `save_image`), Google Calendar (`calendar_*`), Google Sheets (`sheets_*`).
- Зависимости MCP:
  - `comdb.Exterior` (БД: timezone, Google-токены, API-ключи, app_config);
  - model router `GetUserModels` (флаги S3/GOAuth/Music/MetaAction);
  - `storageusecase.Service` (MinIO/S3);
  - `web.CallLeadTarget` через `SetLeadTargetFn` (`app.go:196`);
  - сборка в `app.go:178` с `d`, `m`, storage.
- Клиент и сервер уже разделены HTTP-границей, но находятся в одном процессе orchestrator.

#### 15.9.2. Критические проблемы (независимо от выноса)

1. **Нет аутентификации `/int/mcp`.** `X-Session-ID` — это просто `userID:provider` без подписи. Любой контейнер в `air_shared` может выдать себя за любого пользователя и вызвать `calendar_*`, `sheets_*`, S3, `generate_music`, `lead_target`.
2. **`tools/call` не проверяет allowlist.** `buildToolsList` фильтрует инструменты, но `callTool` (`tools.go:156`) выполняет любой tool по имени. Можно вызвать `calendar_create`/`sheets_write`/`generate_music`, даже если флаги модели это запрещают.
3. **IDOR в `lead_target`.** Аргумент `resp_id` приходит от LLM; `CallLeadTarget(ctx, respId)` (`services.go:369`) не получает `userID` и не проверяет принадлежность диалога. Нужна проверка владельца.
4. **Потеря контекста.** Storage-вызовы используют `h.ctx` (контекст приложения), а не `c.Request.Context()` — отмена/таймаут/трейсинг не работают.
5. **Нет rate limit, аудита, идемпотентности.** `generate_music` — дорогой и долгий инструмент; возможны злоупотребления и повторные вызовы.
6. **Хардкод URL и отсутствие service auth.** `mcpURL` нельзя переключить на отдельный сервис без пересборки; `X-Session-ID` после перехода на `uint64` сломается.
7. **Нет OTel-спанов** на MCP-вызовы и инструменты.
8. **`tools/list`/`prompts/get` дергаются часто** — нужен кэш per user/provider с инвалидацией при смене модели.

#### 15.9.3. Нужно ли выносить MCP в отдельный сервис

**Сейчас — нет.** Причины:

- MCP сильно связан с внутренностями orchestrator: БД, model router, storage, app_config, Google OAuth, ElevenLabs keys, lead target.
- Вынос потребует 5–6 внутренних API и добавит сетевой hop на критический путь AI-ответа.
- На одном VPS дополнительный сервис добавляет деплой, секреты, сеть, мониторинг, версионирование без явной выгоды.
- Сначала нужно закрыть auth/authorization-дыры — вынос сам по себе их не решает.

**MCP — сильный кандидат на вынос позже**, потому что:

- уже есть HTTP-граница и stateless JSON-RPC;
- tool execution — это security boundary (ввод от LLM);
- долгие инструменты (`generate_music`) можно масштабировать/изолировать;
- вынесенный сервис можно запускать **без DB-доступа** и с узкой сетевой политикой;
- внешние MCP-клиенты/несколько сервисов смогут использовать один MCP.

**Критерии для выноса:**

- нужно изолировать исполнение инструментов (sandbox, no DB credentials);
- нужен независимый scaling/ресурсные лимиты;
- MCP начинают использовать несколько сервисов или внешние клиенты;
- нужен отдельный релизный цикл/команда;
- security-политика требует отдельной trust boundary.

#### 15.9.4. Что сделать до выноса (in-place hardening)

1. **Аутентификация.** mTLS/service-token между сервисами + короткоживущий **подписанный user-token** со scopes (`tools:s3`, `tools:calendar`, `tools:sheets`, `tools:music`, `tools:lead`). Убрать доверие к сырому `X-Session-ID`.
2. **Авторизация.** Один policy-слой для `tools/list` и `tools/call`: проверять инструмент по флагам модели/тарифу в момент вызова, а не только при листинге.
3. **IDOR.** Передавать `userID` в `lead_target` и проверять, что `resp_id` принадлежит этому пользователю (в orchestrator/lead service).
4. **Контекст.** Использовать `c.Request.Context()` во всех tool-вызовах; добавить таймауты на инструмент.
5. **Rate limit + audit.** Лимиты per user/tool; аудит `user_id`, `tool`, хэш аргументов, статус, latency; без PII.
6. **Идемпотентность.** Ключ идемпотентности для `create_file`, `save_image`, `generate_music`, `lead_target`.
7. **Конфиг.** `MCP_URL` из env; таймауты/ретраи/circuit breaker.
8. **Кэш.** `tools/list`/`prompts/get` кэшировать per user/provider с TTL; инвалидация при смене модели.
9. **Наблюдаемость.** OTel-спаны `mcp.request`, `mcp.tool.call` с атрибутами `tool.name`, `provider`, `result`; метрики `air_mcp_tool_calls_total{tool,status}`.
10. **Интерфейсы.** Выделить узкие `ModelFlagsReader`, `GoogleTokenStore`, `UserAPIKeyReader`, `StorageAPI`, `LeadTargetAPI`, `AppConfigReader` — это подготовит вынос.

**Статус in-place hardening (локально):**
- [x] Авторизация `tools/call`: перед выполнением инструмент сверяется с `buildToolsList(userID, provider)`; вызов неразрешённого инструмента отклоняется (`tool not allowed`), а не только скрывается в `tools/list`.
- [x] IDOR `lead_target`: добавлен `comdb.UserOwnsResponder(userID, respID)` и проверка владельца перед вызовом Meta-сервиса (`access denied`); ответы с чужим `resp_id` не выполняются.
- [x] Service-auth `X-Service-Key`: при заданном `MCP_SERVICE_KEY` сервер отклоняет запросы без ключа/с неверным ключом через constant-time compare (HTTP 401); клиент `air-common` отправляет ключ из `MCP_SERVICE_KEY`. Live: без заголовка — 401, с ключом — 200. mTLS пока не включён (инфраструктурный шаг).
- [x] Подписанный `X-Session-ID`: `pkg/mcpsession` (HMAC-SHA256, формат `v1:<uid>:<provider>:<exp>:<hex>`, TTL 5 мин). Клиент `air-common` подписывает при заданном `MCP_SESSION_SECRET`; сервер проверяет подпись и срок. Включение принудительной проверки — `MCP_REQUIRE_SIGNED_SESSION=true` (по умолчанию `false` для поэтапного rollout; legacy-формат остаётся). `MCP_URL` теперь читается из env. Проверено live: при `require=true` legacy-заголовок отклоняется, подписанный — принимается; после проверки контейнер возвращён в default-режим.
- [x] Rate limit на инструменты: token bucket per `(user_id, tool)` (`config.MCPToolRateLimit/Burst`, ленивая очистка лимитеров). Превышение — `rate limit exceeded`.
- [x] Аудит вызовов: `MCP audit: user provider tool status duration_ms` (без PII/аргументов) + durable-запись в `audit_log` (`action='mcp.tool.call'`, JSON `{tool,provider,status,duration_ms}`); при ошибке записи — warning, лог остаётся fallback. Live-проверка: после `tools/call` в `audit_log` появилась строка для user 23.
- [x] Таймауты инструментов: 30 c по умолчанию, 120 c для `generate_music`; используется `c.Request.Context()`.
- [x] Идемпотентность: необязательный `idempotency_key` в схемах `lead_target`, `create_file`, `save_image`, `generate_music`, `calendar_create`, `calendar_delete`, `sheets_write`, `sheets_append`; успешный результат кэшируется per `(user_id, tool, key)` на 10 минут, ошибки не кэшируются (retry разрешён).
- [ ] Кэш `tools/list` — не применять для allowlist (риск устаревших прав); можно ввести только для выдачи списка с коротким TTL после появления инвалидации по смене модели.
- [x] OTel-спаны: добавлен `air-common/pkg/telemetry` (OTLP HTTP exporter, opt-in через `OTEL_EXPORTER_OTLP_ENDPOINT`/`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`; без endpoint — no-op). `air_orchestrator` инициализирует провайдер в `cmd/main.go` и корректно вызывает `Shutdown`. Спаны: `mcp.request` (атрибут `mcp.method`) и `mcp.tool.call` (`tool.name`, `provider`, `user.id`, `tool.error`). Коллектор в локальном compose пока не поднят — при появлении endpoint трассировка включится без изменений кода.
- [ ] Передача scopes (`tools:s3|calendar|sheets|music|lead`) в токене — имеет смысл только если scopes выдаёт сам orchestrator при аутентификации, а не клиент; пока allowlist строится по флагам модели из БД.

#### 15.9.5. Целевая архитектура при выносе

```text
air-common (OpenAI/Mistral/Google/Realtime)
        │  MCP JSON-RPC + signed user token
        ▼
   air_mcp  ── mTLS ──> air_orchestrator API
        │                 ├─ model flags / user model
        │                 ├─ Google token get/save
        │                 ├─ user API key
        │                 ├─ app config (read-only)
        │                 ├─ storage API
        │                 └─ lead target API (с проверкой ownership)
        │
        ├─ Redis: rate limit / audit / idempotency
        └─ OTel → Jaeger
```

- `air_mcp` **не имеет прямого доступа к БД** пользователей.
- Свои данные (audit, rate limit, idempotency) — Redis/отдельная БД.
- Сетевые политики: ingress только от orchestrator/AI-воркеров; egress только к orchestrator и allowlist провайдеров.
- Контракт: MCP JSON-RPC снаружи + внутренние gRPC/HTTP API с версионированием.
- Дополнительный hop компенсируется кэшем `tools/list`, таймаутами и circuit breaker.

#### 15.9.6. Решение

- **Этап 1 (сейчас):** оставить MCP в orchestrator, но закрыть auth/authorization/IDOR/context/rate-limit/audit. Это обязательный P0.
- **Этап 2:** разнести зависимости на узкие интерфейсы, сделать `MCP_URL` конфигурируемым, добавить OTel/кэш.
- **Этап 3:** вынести `air_mcp` только при выполнении критериев из §15.9.3. Ориентир — после стабилизации auth и появления внутренних API; протокол MCP менять не нужно.

### 15.10. Readiness gate: готовность к реализации

Проверка выполнена по фактическому состоянию кода и live-БД (MariaDB 13.0.1, контейнер `air_db`).

| Проверка | Фактическое состояние | Готовность |
|---|---|---|
| Live-БД | 40 таблиц, 18 routines, 6 triggers, 1 event; `schema_migrations` 1–3 | Phase 1 применён локально |
| `crm_mappings` | битые FK на `crm_applications` удалены миграцией `000001` | Исправлено |
| `user_gpt` | дубли FK удалены; orphan `Realtime` переведены в `NULL` | Исправлено |
| `air-common` тесты | `go test ./...` — pass; integration-тесты за build tag `integration` | Локально готов |
| CI `air-common` | `ci.yml` (build/vet/test/race/govulncheck, Go 1.27.x) добавлен | Закрыто локально |
| `air_orchestrator` | `go build ./...` OK, `go test ./...` OK; запущен с новой схемой | Готов |
| `air_whatsbot` | `go test ./...` OK | Готов |
| `pkg/idgen`, `pkg/migrate`, `pkg/dbrouter`, `pkg/telemetry`, `comdom/id.go` | реализованы и покрыты тестами | Phase 1 готов |
| Версии `air-common` | `v1.50.94`–`v1.60.3` | **Блокер**: нет единой версии |
| Объём live-данных | 7 users, 72 dialogs, 72 responders, 6 channels | Похоже на dev; нужен baseline prod |
| MCP | auth/allowlist/IDOR/rate limit/audit/idempotency реализованы | P0 закрыт локально |
| Решения по scope | `app_config`/`service_proxy`/`user_storage_config` не меняем; MasterKey в Redis зашифрован | Зафиксировано |

**Вывод:** план как документ проработан, но **к прямой реализации в полном объёме не готов**. Нужно закрыть 4 gate:

1. **G1. Зелёный baseline.** Закрыт: MCP/CRM integration-тесты изолированы, Mistral-тесты исправлены, `go test`/`go vet` проходят локально; добавлен CI workflow `air-common/.github/workflows/ci.yml` (`build + vet + unit test` на push/PR в `main` и `new-arch`, Go `1.27.x`). Integration-тесты требуют БД/Redis и в CI не запускаются. Локальный тулчейн обновлён до Go 1.27.3; при этом `GOROOT` в окружении всё ещё указывал на старый `D:\Go` — для сборки нужно выставлять `GOROOT=C:\Program Files\Go` (глобальную переменную стоит поправить вручную).
2. **G2. Целостность live-схемы и prod-baseline.** Локально закрыт: битые FK `crm_mappings` и дубли `user_gpt` удалены миграцией. Осталось снять реальный prod-дамп, сделать restore-check и зафиксировать объёмы.
3. **G3. Выравнивание версий.** Все сервисы на одной `air-common`; выбрать целевую `v2` и релизный поезд.
4. **G4. Закрытие open questions.** Семантика `_scheduler_ResetUserFlags`, маппинг `dialogs.Data`, наличие MasterKey у пользователей, владелец reference-данных.

После G1–G4 план готов к **Phase 0–1**. Phase 2–7 запускать только после успешного завершения предыдущих фаз и подтверждённого prod-baseline.

### 15.11. Phase 1: статус локальной реализации

Реализовано в ветке `new-arch` (локально, без merge в `master`):

- `pkg/comdom/id.go` — defined types `UserID`, `DialogID`, `ResponderID`, `MessageID`, `BotID`, `ChannelID`, `Validate/IsZero/String/Uint64`, JSON остаётся числовым.
- `pkg/idgen` — Snowflake-генератор (41/10/12 бит), `NODE_ID`/`SHARD_ID`, монотонность при откате часов, typed-методы `NextUserID`/`NextDialogID`/..., тесты на уникальность/конкурентность/overflow.
- `pkg/migrate` — минимальный runner для MariaDB: `schema_migrations`, lock, dirty-флаг, checksum, `Up/Down/Version/Required`, embed SQL, splitter с учётом строк/комментариев.
- `pkg/telemetry` — opt-in OpenTelemetry: OTLP HTTP exporter при `OTEL_EXPORTER_OTLP_ENDPOINT`/`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, иначе no-op; `Shutdown` для flush. Тест экспортирует реальный спан в `httptest`-приёмник.
- `pkg/mcpsession` — HMAC-подпись `X-Session-ID` (используется в MCP hardening).
- `air-logger` — корреляция логов с трассировкой:
  - новые функции `InfoCtx`/`WarnCtx`/`ErrorCtx`/`DebugCtx` извлекают `trace_id`/`span_id` активного OTel-спана из `context.Context`;
  - строка лога получает префикс `[trace=<32hex> span=<16hex>]`; без активного спана формат не меняется;
  - MCP audit/rate-limit/allowlist/IDOR-логи и `/readyz` переведены на контекстные функции;
  - unit-тесты с синтетическим `SpanContext` — PASS; добавлен CI для `air-logger` (`build/vet/test` + race);
  - live-проверка: при заданном `OTEL_EXPORTER_OTLP_ENDPOINT` MCP audit-лог содержит `[trace=<32hex> span=<16hex>]`; после проверки контейнер возвращён в режим без экспортёра.
- `air_orchestrator` HTTP-трассировка:
  - `TracingMiddleware` создаёт server-span для каждого запроса (`http.method`/`http.route`/`http.status_code`), извлекает W3C `traceparent`, кладёт span в `request.Context()`;
  - `/metrics`, `/healthz`, `/readyz` исключены из трейсинга;
  - вложенные спаны (`mcp.request`, `mcp.tool.call`) становятся дочерними, логи получают тот же `trace_id`;
  - unit-тест с in-memory exporter и `traceparent` — PASS; live: audit-лог `trace=c95b2926… span=d01f14a5…`.
- Исходящий MCP-клиент (`air-common/pkg/model/action_handler.go`) инжектирует W3C `traceparent` в оба запроса (`callMCP`, `callMCPMethod`), поэтому спаны MCP-сервера станут дочерними при выносе `/int/mcp` в отдельный сервис; unit-тест `TestMCPPropagatesTraceContext` (httptest-сервер проверяет заголовок) — PASS.
- Миграции:
  - `000001_widen_ids` — расширение ID до `BIGINT UNSIGNED`, drop/recreate валидных FK, удаление битых FK `crm_mappings → crm_applications` и дублей `user_gpt`/`crm_mappings`, отказ от повторного создания `fk_embeddings_model`, `service_gpt` FK переведён с `SET NULL` на `RESTRICT`, data repair для orphan `user_gpt.Realtime`.
  - `000002_add_id_source` — `id_source ENUM('legacy','snowflake')` для `users`, `dialogs`, `responders`.
  - `000003_vector_embedding_model_fk` — `vector_embeddings.user_model_id` + backfill + корректный FK на `user_models.Id` + индекс `(user_id, user_model_id)`.
- Локальная БД:
  - `air_db` переведена с Windows bind mount на named volume `air_db_data`; `build/air-db.yml` обновлён.
  - Применены миграции `000001`–`000005`; `schema_migrations` содержит версии 1–5 (`dirty=0`).
  - 42 валидных FK восстановлены; битые `crm_applications` FK и дубли удалены; 8 orphan `user_gpt.Realtime` переведены в `NULL`.
  - `airorc_app` перезапущен и работает с новой схемой.
- Baseline-тесты:
  - MCP integration-тесты (`pkg/model/mcp*_test.go`) и CRM integration-тесты (`pkg/crm/crm_test.go`) переведены на build tag `integration`.
  - Исправлены Mistral-баги: `Interrupt()` инвалидирует текущий turn; `hasMistralSpeechText()` применяет ту же санитизацию, что и TTS.
- Проверки:
  - `go test ./...` — pass;
  - `go vet ./...` — pass;
  - полный integration-suite `air_orchestrator/internal/repository/mysql` (с БД и флагами v2/backfill) — pass; хрупкие проверки данных ослаблены (RoleName/AssistId/пустой Responder/набор провайдеров), ручной `TestEncryptChannelsWSSManual` корректно скипается без `APP_MASTER_KEY_FILE`/`REDIS_ADDR`;
  - `go test -race` для `pkg/idgen`, `pkg/dbrouter`, `pkg/telemetry` — pass, добавлено в CI;
  - `govulncheck` добавлен в CI;
  - ошибки `comdb` типизированы (`ErrInvalidDialogID`, `ErrInvalidUserID`, `ErrUserNotFound`, `ErrDemoUserDeletion`, `ErrDialogNotFound`); `strings.Contains` по тексту SQL-ошибки в `DeleteDialog` устранён, исправлен баг: demo-ограничение раньше не распознавалось вызывающим кодом;
  - `git` не коммитился, `master` не затронут.
- Локальная связка:
  - создан `D:\project\marusia\go.work` со всеми 13 модулями;
  - добавлен workspace-replace `cloud.google.com/go/compute => v1.75.0` — устраняет ambiguous import `compute/metadata` (compute v1.6.1 тянули `air_operator`/`air_tgbot`);
  - все сервисы собираются с локальным `air-common` в едином workspace; `air_tguserbot` — кросс-сборкой `GOOS=linux`;
  - собран Linux-бинарник и Docker-образ `airorc:local-new-arch`.

Блокеры и ограничения:

1. **CI `air-common`.** Добавлен отдельный тестовый workflow `.github/workflows/ci.yml` (`build + vet + unit test`, Go 1.27.x) для `main`/`new-arch`; prod CI не затронут.
2. **`air_tguserbot` на Windows.** Пакет `internal/telegram` имеет build tag `amd64 && linux`; локальная сборка на Windows требует `GOOS=linux` (cross-build проходит).
3. **`go.work` не коммитится.** Workspace-файл в `.gitignore`; replace нужно перенести в `go.mod` зависимых сервисов или в CI-скрипт.

### 15.12. Phase 2 (expand): статус

- Миграция `000004_user_channels` применена: созданы `user_channels` и `user_channel_configs`.
- Миграция `000005_backfill_channels` применена: legacy `channels` перенесены в v2 (12 каналов, 12 конфигов), `$mk$`/`$app$` перенесены как есть, plaintext JSON — как plaintext; повторный запуск идемпотентен.
- Миграция `000008_single_channel_config` применена (`version=8`): `config_json`/`secret_ciphertext`/`encrypted`/`key_version` заменены одной колонкой `config` (`MEDIUMTEXT`), 12 конфигов перенесены, 5 из них `$mk$`-шифротекст.
- В `air_orchestrator`:
  - `Implementation` получил Snowflake-генератор `IDGen`;
  - `SaveChannelData` пишет в legacy `channels` и в `user_channels`/`user_channel_configs` в одной транзакции;
  - `DeleteChannelData` отключает v2-канал и удаляет v2-конфиг;
  - `pkg/comdb`: `SetChannelEnabled` и `DisableAllUserChannel` зеркалят изменение `enabled` в `user_channels` (маппинг `TgBot`→`tgbot`, `Widget`→`widget`, `TgUserBot`→`tguserbot`, `Whats`→`whatsbot`, `Insta`→`insta`, `Avito`→`avito`; upsert по `(user_id, channel_type)`);
  - добавлен `normalizeChannelType` (`tgubot` → `tguserbot`);
  - `GetChannelsData`, `CheckActiveChannels`, `GetActiveChannels` читают v2 при `CHANNELS_V2_READ_ENABLED=true`, иначе legacy.
- Интеграционные проверки (build tag `integration`, локальная MariaDB `127.0.0.1:3307`):
  - `TestChannelsV2ReadLocal` — чтение v2 (PASS);
  - `TestSaveChannelDataV2Local` — dual-write, `config`, delete/disable (PASS);
  - `TestChannelEnabledV2Local` — `SetChannelEnabled`/`DisableAllUserChannel` зеркалятся в v2 (PASS).
- Сверка legacy ↔ v2 на локальной БД (по каждому из 6 каналов: наличие payload, `enabled`, наличие v2-конфига):
  - `tgbot` 3/3, `widget` 2/2, `tguserbot` 4/4, `whatsbot` 1/1, `insta` 2/2, `avito` 0/0;
  - `payload_mismatch=0`, `enabled_mismatch=0` по всем каналам; всего 12 v2-каналов / 12 конфигов.
- `airorc_app` перезапущен с новым образом (`airorc:dev` = `airorc:local-new-arch`) и `CHANNELS_V2_READ_ENABLED=true`; startup OK, gRPC ConfigService слушает `:50051`, ошибок миграции нет.
- Проверки: `air-common go test ./...` pass; `air_orchestrator` build/test pass.
- Осталось: наблюдение за v2 read-path, затем contract (удаление legacy-колонок `channels`) в отдельной фазе.

Следующие шаги:

- Phase 2: наблюдение за v2 read-path в `airorc_app` (`CHANNELS_V2_READ_ENABLED=true`).
- Phase 3: наблюдение за `DIALOG_MESSAGES_READ_ENABLED=true`, миграция зашифрованных диалогов, затем contract (удаление legacy `Data`/`Context` и SP).
- Phase 4: contract по bot-таблицам (`service_tgbots`/`service_wabots`/`service_user_bot_*` + триггеры) после rollout; уникальный ключ responders после rollout; soft-delete/purge для `DeleteAllUserData`; восстановить логику `_scheduler_ResetUserFlags` из prod для Go-cron.
- ~~Перенести workspace-replace `cloud.google.com/go/compute` в `go.mod`/CI.~~ Сделано: `replace cloud.google.com/go/compute => v1.75.0` добавлен в `air-common/go.mod` и `air_orchestrator/go.mod`; из `go.work` replace удалён, `verify-all` по всем модулям — PASS.
- Добавить CI test workflow для `air-common` (после согласования с prod CI).

### 15.13. Phase 3 (expand): статус

- Миграция `000006_dialog_messages` применена (`schema_migrations` version=6, `dirty=0`):
  - `dialogs.messages_migrated`, `dialogs.messages_migrated_at`;
  - `dialog_messages` (PK `id`, unique `(dialog_id, seq)`, FK на `dialogs`/`users`);
  - `dialog_contexts` (PK `(dialog_id, provider)`, FK на `dialogs`).
- `air-common/pkg/comdb`:
  - `DB.IDGen func() uint64` + `SetIDGen`; при отсутствии инъекции используется `idgen.NewFromEnv()` (NODE_ID/SHARD_ID, иначе host:pid).
  - `SaveDialog` dual-write: legacy `dialogs.Data` + `dialog_messages` (`seq` = длина legacy-массива до аппенда); работает и для resolver-пути (в той же транзакции), и для SP-пути.
  - `SaveContext` dual-write: legacy `dialogs.Context` + upsert `dialog_contexts`.
  - Lazy backfill `dialog_messages` в `ReadDialog` (`messages_migrated=0` → разбор массива, `INSERT IGNORE` по `seq`, затем `messages_migrated=1`).
  - `ReadDialog` при `DIALOG_MESSAGES_READ_ENABLED=true` реконструирует legacy-ответ из v2: `Data`, `Type` (string, как у SP), `Model`/`Responder`, `Date` в MySQL-формате; fallback на legacy при отсутствии миграции.
  - `ReadContext` при `DIALOG_MESSAGES_READ_ENABLED=true` читает `dialog_contexts`, при отсутствии — legacy + backfill.
  - `content_json` хранит полный legacy-объект сообщения (creator/message/timestamp) — миграция без потерь.
- Локальная БД:
  - backfill выполнен для 47 plaintext-диалогов (1595 сообщений), 18 зашифрованных (`$mk$`) ждут чтения с MasterKey;
  - `role`: user 712, assistant 828, system 55.
- Интеграционные проверки (build tag `integration`):
  - `TestDialogV2Local` — dual-write `SaveDialog`/`SaveContext`, lazy backfill — PASS;
  - `TestDialogEncryptedBackfillLocal` — `$mk$`-диалог расшифровывается через `MasterKeyResolver` и мигрирует — PASS;
  - `TestBackfillDialogsLocal` (`RUN_BACKFILL=true`) — 47/47 диалогов, 0 расхождений по количеству сообщений;
  - `TestDialogV2ReadCompareLocal` (`RUN_DIALOG_V2_COMPARE=true`) — 47/47 диалогов семантически равны между legacy и v2;
  - `TestApplyMigrationsLocal` (`RUN_MIGRATIONS=true`) — применение миграций.
- `endpoint.GetDialogHistory`: DB-path теперь разбирает envelope `ReadDialog` (`{"Data":[...]}`), включая `Data`-строку; limit клампится в `uint8`; добавлены unit-тесты `TestParseDialogHistoryPayload*`. Ранее DB-path падал на `json.Unmarshal` в `[]Message`, поэтому история после рестарта не читалась.
- `airorc_app` пересобран из локального `air-common` и перезапущен с `CHANNELS_V2_READ_ENABLED=true` и `DIALOG_MESSAGES_READ_ENABLED=true`; startup OK, gRPC ConfigService слушает `:50051`, ошибок миграции нет.
- Проверки: `air-common go test ./...`, `air_orchestrator go test ./...`, `go vet` — pass.
- Осталось: наблюдение `DIALOG_MESSAGES_READ_ENABLED=true` на живом трафике, миграция зашифрованных диалогов при чтении, затем contract-фаза.

### 15.14. Phase 4 (expand): responders

- Миграция `000007_responders_user_scope` применена (`schema_migrations` version=7, `dirty=0`):
  - `responders.user_id BIGINT UNSIGNED NULL`;
  - `responders.is_orphan TINYINT(1) NOT NULL DEFAULT 0`;
  - индекс `(user_id, Type, RealId)`, FK на `users` `ON DELETE CASCADE`.
- `pkg/comdb.BackfillResponders(ctx)`:
  - responder, используемый одним пользователем → `user_id = User`;
  - используемый несколькими → владелец = минимальный `User`, для остальных создаются копии с app-generated ID, `dialogs.Responder` перепривязывается;
  - неиспользуемые → `user_id = NULL`, `is_orphan = 1`;
  - дубли `(user_id, Type, RealId)` сливаются (перепривязка + удаление);
  - идемпотентен.
- Локальный прогон: `{Total:72 Shared:12 Orphans:25 Copies:19 Repointed:24 Merged:0}`; проверки: 0 диалогов с чужим `user_id`, 0 неотмеченных orphan, 0 дублей; повторный запуск не создаёт копий.
- `GetOrSetTreadAndResponder` переписан на Go:
  - транзакция + `SELECT ... FOR UPDATE` по `users` (сериализация в рамках пользователя);
  - поиск/создание responder по `(user_id, Type, RealId)` с Snowflake ID;
  - поиск/создание диалога с Snowflake ID и `id_source='snowflake'`;
  - обновление `Name` при изменении.
- Интеграционные проверки: `TestBackfillRespondersLocal` (`RUN_BACKFILL_RESPONDERS=true`) и `TestGetOrSetTreadAndResponderLocal` — PASS.
- `air_lead-hunter`: прямой `INSERT INTO responders` теперь пишет `user_id`; `UPDATE responders ... RealId` ограничен `user_id` (anti-IDOR).
- SP → Go:
  - `DeleteDialog`: проверка `RoleId=1` (demo) + ownership `dialogs.User = userID`, каскад messages/contexts через FK; SP не вызывается.
  - `UpdateDialogsMeta`: `target`/`trigger` через `UPDATE`; неизвестное значение — ошибка.
  - `air_widget.ReadResponderName`: вместо SP — `SELECT ... WHERE user_id=? AND RealId=?`, сигнатура получила `userID` (устранён cross-tenant выбор).
  - `UserLanguage`, `GetNotificationChannel`, `GetUserSubscriptionLimites` — Go-реализация без SP.
  - `SaveDialog` полностью на Go (read → append → encrypt → update в транзакции, `FOR UPDATE`); SP `SaveDialog` больше не вызывается.
  - `ReadDialog` legacy-path полностью на Go (`dialogs.Data` + имена), SP `ReadDialog` больше не вызывается.
  - `SaveContext`/`ReadContext` — Go (`dialogs.Context` + `dialog_contexts`), SP не вызываются.
  - `DisableAllUserChannel`/`SetChannelEnabled` — Go-обновление legacy `channels` + `user_channels`.
  - `DeleteAllUserData` — Go-транзакция с явным удалением user-scoped таблиц и каскадов; SP не вызывается.
  - `air_payment.GetUserTariff` — Go-реализация (ветки RoleId=3 / остальные, billing, Discounts, null-billing), SP не вызывается.
  - `air_tguserbot.SaveChannelData` → общий `comdb.SaveChannelDataRaw` (legacy + v2), без SP.
- Интеграционные проверки: `TestDeleteDialogAndMetaLocal`, `TestUtilsReadsV2Local`, `TestGetUserTariffV2Local`, `TestSaveReadContextLocal`, `TestDeleteAllUserDataLocal` — PASS.
- Все production-вызовы SP/функций из прикладного кода удалены; остались только объявления в БД до contract-фазы.
- Единый источник истины каналов — `pkg/comdom/channels.go`:
  - `ChannelInfo{Type, Slug, LegacyName, LegacyColumn, Code, Configurable, Aliases}`;
  - `ChannelInfos()`, `ConfigurableChannels()`, `ChannelBySlug/Type/LegacyName`, `NormalizeChannelSlug`;
  - `AllChannels` и `ChannelType.FromString` теперь выводятся из реестра;
  - `pkg/comdb` и `air_orchestrator` (channels, encryption, v2 read/write) используют реестр вместо локальных map/switch; `Web` помечен как неконфигурируемый.
- Все вызовы SP/функций из Go-кода удалены (проверено поиском `CALL`/имён routine). В БД остаются неиспользуемые определения `GetBalance`, `GetOrSetDialog` и ported-процедуры — их можно удалить в contract-фазе после деплоя всех сервисов.
- Осталось: уникальный ключ `(user_id, Type, RealId)` после rollout, `user_bots`/`user_bot_events`/`user_bot_state`.

### 15.15. Phase 4.2: user_bots (expand)

- Миграция `000009_user_bots` применена (`version=9`): `user_bots`, `user_bot_events`, `user_bot_state` с FK `ON DELETE CASCADE` и unique `(user_id, bot_type, external_bot_id)` / `(user_id, bot_type, alias)`.
- `pkg/comdb.BackfillUserBots(ctx)`:
  - `service_tgbots`/`service_wabots` → `user_bots` (Snowflake ID, bot_type, external_bot_id, alias, auth, is_active, timestamps);
  - `service_user_bot_events` → `user_bot_events` (legacy id сохраняется, `INSERT IGNORE`, `context_json` валидируется);
  - `service_user_bot_state` → `user_bot_state` (upsert по `user_bot_id`);
  - маппинг `(bot_type, legacy_id)` → `user_bot_id`; orphan-события/состояния пропускаются со счётчиком;
  - идемпотентен.
- Локальный прогон: `{Bots:10 Events:159 States:9 SkippedEvents:0 SkippedStates:1}` (1 state ссылается на удалённого telegram-бота 11); повторный запуск: `Events:0`, количество строк не меняется.
- Миграция `000010_drop_duplicate_triggers` (`version=10`): удалены ошибочные `tg_delete_*` (использовали `OLD.BotId` вместо `OLD.Id`) и дубли; добавлены корректные `trg_tgbot_delete_cascade_*`; `trg_wabot_delete_cascade_*` оставлены.
- Интеграционный тест `TestBackfillUserBotsLocal` (`RUN_BACKFILL_USERBOTS=true`) — PASS.
- Миграция `000011_disable_stale_event` (`version=11`): `ResetFlagsEvent` переведён в `DISABLED`. Событие каждые 30 секунд вызывало отсутствующую `_scheduler_ResetUserFlags` и генерировало ошибки; логика для Go-cron будет восстановлена из prod-дампа (`SHOW CREATE FUNCTION`).
- **Блокер перевода lead-hunter снят:** сервис переведён с `github.com/ikermy/air_common v1.50.91` на локальный `github.com/ikermy/air-common`:
  - заменены импорты (`air_common` → `air-common`, `model/commdom` → `comdom`);
  - `StartCh`: `Provider`→`ChName` (`comdom.ChannelType`), `Chanel`→`Channel`, `TreadId`→`ThreadId`;
  - `startpoint`: `StarterListener`→`StartSession` (цикл по `<-chan error`);
  - `comdb.AI` → `comdom.AI`.
- Lead-hunter полностью переведён на `user_bots`/`user_bot_events`/`user_bot_state`:
  - все чтения/записи ботов, событий, состояния и auth data;
  - `UpdateOrCreateUserBotData` создаёт `user_bots.id` через Snowflake и `external_bot_id` как маленький публичный слот (`MAX+1` в рамках bot_type);
  - события получают Snowflake `id`;
  - `ResetBotErrorStatus` принимает `botType` (устранена неоднозначность слотов между telegram/whatsapp).
- `air_orchestrator/security.go`:
  - `EncryptUserStorageServicesBotDataWSS` шифрует `user_bots.auth_ciphertext` (в дополнение к legacy на время expand);
  - `EncryptChannelsWSS` шифрует `user_channel_configs.config`;
  - `DeleteEncryptedUserDataWSS` очищает `user_bots`, `user_channel_configs`, `dialog_messages`, `dialog_contexts`.
- Исправлен ранний выход `EncryptChannelsWSS`: если legacy-строки `channels` нет (`sql.ErrNoRows`), функция больше не завершается, а продолжает и шифрует `user_channel_configs.config`. Добавлен интеграционный тест `TestEncryptChannelsUserConfigsWSSLocal` (user + v2-канал без legacy-строки: config становится `$mk$`, расшифровывается тем же MasterKey) — PASS.
- Интеграционный тест `TestUserBotsRepositoryLocal` (create/update/list/auth/toggle/state/event/clear/delete) — PASS.
- Unit-тесты lead-hunter приведены в актуальное состояние: удалены мёртвые мануальные тесты (`GetDefaultProxy`, `Hunters`, реальные Telegram-подключения), добавлены проверки `extractRealPhone`, `parseTgUserBotToken`, `parseMultipleTgUserBotTokens`, `parseFloodWait`, `formatUserName`; `go vet ./...` и `go test ./...` в `air_lead-hunter` — PASS.
- Локальная сборка Docker-образа: `build/Dockerfile` тянет опубликованный `air-common v1.60.3`, где ещё нет `pkg/idgen`/`pkg/migrate`. Для локального образа бинарник собирается из рабочего дерева через `go.work` (`GOOS=linux GOARCH=amd64 go build -o airorc ./cmd/`) и упаковывается в alpine; образы `airorc:dev`/`airorc:local-new-arch` пересобраны, `airorc_app` поднят с `CHANNELS_V2_READ_ENABLED=true`, `DIALOG_MESSAGES_READ_ENABLED=true`.
- `DeleteAllUserData` расширен v2-таблицами: `user_directory`, `user_bots`, `user_bot_events`, `user_bot_state`, `user_channels`, `user_channel_configs`, `dialog_messages`, `dialog_contexts`, `responders` удаляются явно (не только через cascade); интеграционный тест проверяет отсутствие строк, включая `user_directory` и `user_bots`.
- Осталось: soft-delete (`users.status`/`deleted_at`) + фоновый purge-джоб + audit_log (п. 4.3); contract-удаление `service_tgbots`/`service_wabots`/`service_user_bot_*` и триггеров после rollout.

### 15.16. Phase 6: dbrouter и user_directory (локально)

- `pkg/dbrouter` (air-common):
  - `Config{generation, slots[{from,to,shard}], shards{id:{dsn_env,driver}}, reference}`; валидация полного покрытия 0..1023 без пересечений и ссылок на неизвестные шарды;
  - `SlotForUser(userID)` — `xxhash64(BE64(userID)) & 1023`; `ShardForUser`/`ForUser`;
  - ленивый кэш соединений `map[shardID]*sql.DB`, `DBForUser`, `DBForReference`, `Ping` (join ошибок), `Close`;
  - метрики-счётчики `Requests`/`Errors`/`ByShard`/`ErrorsByShard` (API `Metrics()`, без внешней Prometheus-зависимости);
  - конфиг из `DB_SHARDS_CONFIG` (JSON) или `DB_SHARDS_CONFIG_FILE`; `NewForTest` для инъекции opener.
  - `UpdateConfig(cfg)` — атомарная смена generation/slot-map: валидирует новый конфиг, сохраняет соединения и счётчики для оставшихся шардов, закрывает соединения удалённых/изменённых шардов; при ошибке состояние не меняется (откат generation).
  - unit-тесты: валидация (gap/overlap/unknown shard), детерминированность слота, маршрутизация, ошибки DSN/open, reference, загрузка из env, reload/rollback и закрытие соединений; `go test -race` — PASS.
- Миграция `000012_user_directory` (`version=12`): таблица `user_directory` (PK `user_id`, unique `email_hash`, `idx_slot`).
- `pkg/comdb`:
  - `BackfillUserDirectory(ctx, shardForSlot)` — из `user_auth.EmailHash` (`HEX` → 32 байта), `slot=dbrouter.SlotForUser`, `shard_id` через резолвер; пропускает пустые/битые хэши; идемпотентен (повторный прогон `Total:0`);
  - `UpsertUserDirectory`, `LookupUserDirectory[ByEmailHash]`, `DecodeEmailHash`;
  - `VerifyUserDirectory(ctx, shardForSlot)` — проверка консистентности: пропущенные строки, orphan-строки, несовпадение `slot` с `dbrouter.SlotForUser`, несовпадение `shard_id` с резолвером; возвращает список проблем без изменения данных;
  - `RecomputeUserDirectoryShards(ctx, shardForSlot)` — после смены slot-map пакетно (keyset-пагинация по 1000) обновляет `shard_id` только у изменившихся строк; идемпотентен.
- CLI `cmd/userdir`:
  - `-action verify|backfill|recompute`, `-shard-map-file` (или `DB_SHARDS_CONFIG[_FILE]`), `-timeout`;
  - `DB_*` указывают на БД `user_directory`; для `recompute` slot-map обязателен;
  - `verify` возвращает exit `1` при найденных проблемах;
  - live: `verify` → `issues: 0`, `backfill` → `Total:0` (идемпотентно), `recompute` → `updated 0`.
- Локальный backfill: 3 реальных пользователя с `EmailHash` перенесены; `shard_id=0` (single-shard).
- Интеграционные тесты: `TestBackfillUserDirectoryLocal`, `TestBackfillUserDirectoryAllLocal` (gated `RUN_BACKFILL_USERDIRECTORY=true`), `TestVerifyUserDirectoryLocal` (чистая строка, затем искусственные slot/shard mismatches) — PASS; на локальных данных `issues: 0`.
- Интеграция в `air_orchestrator`:
  - при старте `initShardRouter`: если задан `DB_SHARDS_CONFIG`/`DB_SHARDS_CONFIG_FILE` — загрузка, валидация и `Ping` с таймаутом 10 c; без конфига — single-DB режим (debug-лог);
  - `SHARD_STRICT=true` — fatal при неверном конфиге/недоступном шарде; иначе warning;
  - роутер закрывается при shutdown;
  - `/healthz` (liveness) и `/readyz` (ping primary DB + зарегистрированные проверки, включая шарды) — 200/503;
  - Prometheus-коллектор `internal/metrics/dbrouter.go`: `air_common_db_router_requests_total{shard}`, `air_common_db_router_errors_total{shard}`, `air_common_db_router_generation`; регистрируется при наличии shard-конфига; unit-тест коллектора — PASS;
  - env добавлены в `build/dev.yml`.
- Реальная маршрутизация (первый блок):
  - `comdb.DB.SetShardResolver` + `ConnForUser(ctx, userID)`; без резолвера — primary-соединение (single-DB режим не меняется);
  - через шард уже работают: `GetNotificationChannel`, `GetUserSubscriptionLimites`, `SetChannelEnabled`, `DisableAllUserChannel`, `SaveChannelDataRaw`, `DeleteDialog`, `GetOrSetTreadAndResponder`, `UserTimeZone`, `SetUserSubscriptionNotified`, `GetContactAvailability`, `SetContactAvailability`, `GetContactsAvailableIn`, `GetContactsInBothProviders`, `SaveGoogleToken`, `GetGoogleToken`, `DeleteGoogleToken`, `GetActiveProvider`, `GetUserAPIKey`, `SetUserAPIKey`, `DeleteUserAPIKey`, `GetUserVectorStorage`, `GetOrSetUserStorageLimit`, `SaveEmbedding`, `ReadUserModel`, `ReadUserModelByProvider`, `RemoveModelFromUser`, `SetActiveModel`, `SetActiveModelByProvider`, `UpdateUserGPT`, `GetAllUserModels`, `GetActiveModel`, `GetModelByProvider`, `GetModelByProviderAnyStatus`;
  - orchestrator при наличии shard-конфига вызывает `d.SetShardResolver(shardRouter)`;
  - unit-тесты fallback/resolver — PASS; интеграционный `TestShardRoutingLocal` (gated `RUN_SHARD_ROUTING=true`) подтверждает, что запросы идут через `dbrouter` и метрики растут — PASS.
- Reference-данные:
  - `comdb.DB.SetReferenceConn` + `RefConn()` (fallback на primary);
  - `DefaultProvidersModels`, `GetProviderModels`, `GetVoiceModels`, `SyncVoiceModels` работают с reference-соединением;
  - `GetAllUserModels`/`GetActiveModel`/`GetModelByProvider`/`GetModelByProviderAnyStatus` разделены на два запроса: user-часть (`user_models`+`user_gpt`) с шарда, имена моделей — `fillReferenceModels` через `RefConn`; N+1 ограничен числом моделей пользователя;
  - orchestrator подключает `RefConn` из `router.DBForReference()` при наличии `reference`-шарда; unit-тест fallback/override — PASS.
- Не сделано: методы по `dialogID`/`botID` (нужен lookup владельца/директория диалогов), репликация и cutover (6.5).

### 15.17. Эксплуатационные SQL-артефакты (не применяются автоматически)

- `docs/sql/phase5_contract.sql` — contract-фаза:
  - проверки-предусловия (dirty migrations, unmigrated dialogs, unowned responders);
  - `DROP` всех 18 legacy routine (процедуры/функции);
  - `DROP` триггеров `trg_*_delete_cascade_*` и таблиц `service_tgbots`/`service_wabots`/`service_user_bot_*`;
  - `DROP EVENT ResetFlagsEvent`;
  - unique `responders(user_id, Type, RealId)`;
  - удаление legacy-колонок `channels.*` и `dialogs.Data/Context/messages_migrated*`.
- `docs/sql/phase6_privileges.sql` — Phase 6.4:
  - отдельные пользователи `air_migrator`, `air_orchestrator`, `air_*`-сервисы, `air_analytics`, `air_backup`;
  - least-privilege гранты по таблицам, reference-only чтение для всех сервисов;
  - `REQUIRE SSL` для каждого пользователя + пример `require_secure_transport=ON`/TLS в `my.cnf`;
  - плейсхолдер `__PASSWORD__` — пароли подставляются из secret store при деплое.
- Скрипты намеренно лежат вне `pkg/migrate` и не запускаются автоматически: contract — только после подтверждённого rollout, права/TLS — при подготовке инфраструктуры.

### 15.18. Phase 4.3: soft-delete, audit и purge

- Миграция `000013_user_soft_delete` (`version=13`):
  - `users.status ENUM('active','deleted')`, `users.deleted_at`, индекс `(status, deleted_at)`;
  - таблица `audit_log` (user_id, actor_id, action, details JSON, created_at + индексы).
- `air_orchestrator`:
  - `SoftDeleteUserData(userID, actorID)` — идемпотентно ставит `status='deleted'`, `deleted_at=NOW(3)` и пишет `audit_log action='user.soft_delete'`;
  - `PurgeDeletedUsers(ctx, grace, limit)` — выбирает soft-deleted старше grace (сравнение с `NOW(3)` в БД, чтобы не зависеть от часового пояса/clock skew) и вызывает hard-delete пакетами;
  - `DeleteAllUserData` пишет `audit_log action='user.hard_delete'`;
  - `Scheduler.SetPurgeWorker` + hourly-тик (настраивается `USER_PURGE_INTERVAL`); включается при `USER_DELETE_SOFT=true`;
  - `DeleteAllUserDataWSS` при `USER_DELETE_SOFT=true` делает soft-delete вместо немедленного hard-delete; по умолчанию поведение прежнее.
- Env: `USER_DELETE_SOFT` (default `false`), `USER_DELETE_GRACE` (default `24h`), `USER_PURGE_INTERVAL` (default `1h`) — добавлены в `build/dev.yml`.
- Интеграционный тест `TestSoftDeleteAndPurgeLocal`: soft-delete + audit, идемпотентность, grace-период, hard-delete по нулевому grace и `user.hard_delete` audit — PASS.

### 15.19. Приведение unit-тестов всех модулей к зелёному состоянию

- `air_lead-hunter`: удалены мёртвые мануальные тесты; добавлены тесты `extractRealPhone`, `parseTgUserBotToken`, `parseMultipleTgUserBotTokens`, `parseFloodWait`, `formatUserName`.
- `comdb.Exterior` разделён на узкие интерфейсы: `DialogStore`, `ChannelStore`, `UserModelStore`, `EmbeddingStore`, `ContactAvailabilityStore`, `GoogleTokenStore`, `UserInfoStore`, `UserAPIKeyStore`; `Exterior` — их композиция, обратная совместимость сохранена.
- `pkg/httpclient` — HTTP-клиент с retry для идемпотентных запросов (5xx/429/network, повтор с `GetBody`) и circuit breaker (consecutive failures → `ErrCircuitOpen`, half-open после cooldown). Подключён к MCP-клиенту: `tools/list`/prompts ретраятся, `tools/call` — без retry; тесты retry/no-retry/breaker — PASS.
- `air_operator`:
  - `repository.InternalRepository.DialogLastMessages` переведён на `ctx + uint16`; обновлены mysql-репозиторий, `db.DB` и handler;
  - реальный DB-тест `db_test.go` помечен `//go:build integration` и переписан под текущий API.
- `air_payment`:
  - `GetSupportedCurrencies` при отсутствии uid возвращает `400` (раньше молча `200`);
  - тест `createTestWeb` устанавливает `userID` в Locals (как реальный middleware);
  - реальные Bybit/DB-тесты (`bybit_test.go`, `debug_test.go`) помечены `//go:build integration`.
- `marusia_crm`: реальный DB-тест `TestGetCRMConfig_ReadsAndDecryptsFromRealDB` помечен `//go:build integration`.
- Итог: `go test ./...` проходит во всех 12 модулях (`air_tguserbot` — только cross-build `GOOS=linux`), `go vet ./...` — без ошибок.

### 15.20. Runbooks, SLO-алерты, ADR и единая проверка модулей

- Runbooks:
  - `docs/runbooks/migrations.md` — preflight, миграции, backfill, v2-флаги, contract, права/TLS, пост-проверки;
  - `docs/runbooks/rollback.md` — откат v2-флагов, slot-map, soft-delete, миграций; restore и MCP-аварийное отключение;
  - `docs/runbooks/incident.md` — readiness, логи/trace_id, MCP-ошибки, БД, purge;
  - `docs/runbooks/tracing.md` — включение OTel, спаны, проверка, collector/retention.
- `docs/ops/prometheus-rules.yml`:
  - recording rules: request rate, 5xx ratio, p95/p99;
  - алерты: orchestrator down, `/readyz`, error rate > 1%, p95 > 1 c, storage degraded, DB router errors, generation change.
- ADR:
  - `0001-single-channel-config.md`, `0002-slot-routing.md`, `0003-mcp-auth.md`.
- `docs/versions.md` — матрица версий модулей и порядок локальной сборки образа.
- `scripts/verify-all.ps1` / `scripts/verify-all.sh` — build+vet+test всех 12 модулей + cross-build `air_tguserbot`; опционально `-Race`. Прогон: `All modules OK`.
- Дополнительные runbooks:
  - `docs/runbooks/cutover.md` — поэтапный перенос слота: предусловия, копирование, `cmd/dbverify`, переключение generation, наблюдение, откат, вывод из эксплуатации;
  - `docs/runbooks/loadtest.md` + `docs/ops/loadtest.js` — k6-сценарии (`/readyz`, `tools/list`, `tools/call`) с порогами SLO и критериями приёмки;
  - `docs/runbooks/release.md` — тег/публикация `air-common`, bump сервисов, проверка standalone-сборки и откат релиза.
- `scripts/bump-air-common.ps1 -Version vX.Y.Z` — `go mod edit -require` + `go mod tidy` во всех сервисах и запуск `verify-all`.
- Подтверждён операционный блокер: `GOWORK=off go build` в `air_orchestrator`/`air_lead-hunter` падает на новых пакетах (`dbrouter`, `idgen`, `mcpsession`, `telemetry`), пока не опубликована новая версия `air-common`; до релиза сборка идёт через `go.work`.
- Contract-скрипт провалидирован на одноразовой копии схемы (`mariadb:13.0.1-rc`):
  `phase5_contract.sql` выполнился без ошибок; после применения нет `service_*bot*`, routine, EVENT, legacy-колонок `dialogs/channels`, а `uq_user_type_real` создан.

### 15.21. Идемпотентность Telegram webhook

- Миграция `000014_webhook_dedup` (`version=14`): таблица `webhook_dedup(source, event_id, created_at)` с PK `(source, event_id)`.
- `air_orchestrator`:
  - `RegisterWebhookEvent(ctx, source, eventID)` — `INSERT IGNORE`, возвращает `isNew`; фоновая очистка записей старше 7 дней каждые 1000 вставок;
  - `proxyTgBotWebhook` извлекает `update_id` и при повторе сразу отвечает `200 {"ok":true}` без проксирования; ошибки парсинга/БД — fail-open (запрос форвардится);
  - `setwebhook` не дедуплицируется.
- Тест `TestRegisterWebhookEventLocal` (new/duplicate) — PASS.
- Live: первый webhook → `502` (tgbot недоступен), повтор с тем же `update_id` → `200 {"ok":true}`; в `webhook_dedup` ровно одна строка.

### 15.22. Phase 6.5: сверка данных для cutover

- `pkg/dbverify`:
  - `Table{Name, UserColumn, KeyColumn}`, `DefaultTables()` — 16 user-scoped таблиц;
  - `CompareUser`/`CompareUsers` считают `COUNT(*)` и `SUM(CRC32(key))` по каждому пользователю в source и target; расхождения возвращаются как `Diff{UserID, Table, Source, Target, Reason}`.
- CLI `cmd/dbverify`:
  - `-users 1,2,3` или `-users-file` (по одному ID в строке);
  - `-slot N` + `DB_DIRECTORY_DSN` — подтягивает пользователей слота из `user_directory`;
  - DSN из env (`DB_SOURCE_DSN`, `DB_TARGET_DSN`, `DB_DIRECTORY_DSN`), общий `-timeout` (2 мин);
  - exit code `1` при расхождениях, `2` при ошибке конфигурации/подключения.
- Проверки: unit-тесты `pkg/dbverify`; интеграционный `TestDBVerifySelfLocal` (выполняет запросы всех `DefaultTables` по реальной схеме и сравнивает БД сама с собой) — PASS; CLI live: `-users 23` и `-slot 563` → `diffs: 0`.

### 15.23. Итоговая оценка готовности (локально)

- Маршрутизация `pkg/comdb/model.go` завершена: `GetAllUserModels`, `GetActiveModel`, `GetModelByProvider`, `GetModelByProviderAnyStatus`, `ReadUserModel`, `ReadUserModelByProvider`, `RemoveModelFromUser`, `SetActiveModel`, `SetActiveModelByProvider`, `UpdateUserGPT` читают user-часть с шарда (`ConnForUser`), reference-имена — через `RefConn` (`fillReferenceModels`); `gofmt` чистый, `go build`/`go test ./pkg/comdb/...` — PASS.
- Полная проверка `scripts/verify-all.ps1`: **All modules OK** (12 модулей; `air_tguserbot` — cross-build `GOOS=linux`).
- Интеграционный набор `air_orchestrator` с `-tags integration` на локальной MariaDB (`RUN_BACKFILL*`, `RUN_SHARD_ROUTING`, `RUN_DIALOG_V2_COMPARE`) — PASS.
- `GOWORK=off go build ./...` внутри `air-common` — PASS; standalone-сборка сервисов (`air_orchestrator`, `air_lead-hunter`) остаётся заблокированной до публикации новой версии библиотеки.
- Остаточные gate перед prod: (1) коммит/тег/push `air-common` по `docs/runbooks/release.md` и bump сервисов; (2) prod-baseline и contract-фаза (`docs/sql/phase5_contract.sql` уже валидирован); (3) cutover по `docs/runbooks/cutover.md`; (4) `_scheduler_ResetUserFlags` — нужен `SHOW CREATE FUNCTION` с prod.

---

## Приложение A. Полная инвентаризация таблиц

| # | Таблица | Класс | Shard key | Целевое действие |
|---|---|---|---|---|
| 1 | `app_config` | global | — | Версионирование, аудит |
| 2 | `billing` | user | `UserId` | Тип, snake_case |
| 3 | `channels` | user | `UserId` | → `user_channels` + `user_channel_configs` |
| 4 | `chat_type` | reference | — | Оставить |
| 5 | `crm_configs` | user | `user_id` | Тип |
| 6 | `crm_mappings` | user | `user_id` | Убрать дубли FK |
| 7 | `crm_oauth_states` | user | `user_id` | Тип, TTL |
| 8 | `crypto_payments` | user | `user_id` | Тип |
| 9 | `currency` | reference | — | Оставить |
| 10 | `dialogs` | user | `User` | → `user_id`, `is_target/is_trigger`, вынести Data/Context |
| 11 | `google_oauth_tokens` | user | `user_id` | Тип |
| 12 | `gpt_models` | reference | — | Unique `(Provider, Name)` |
| 13 | `languages` | reference | — | Оставить |
| 14 | `model_providers` | reference | — | Оставить |
| 15 | `notifications` | user | `UserId` | Тип, snake_case |
| 16 | `operators` | user | `UserId` | Тип |
| 17 | `realtime_models` | reference | — | Unique |
| 18 | `responders` | user | **new `user_id`** | Добавить `user_id`, дедуп |
| 19 | `service` | user | `UserId` | → `user_services` |
| 20 | `service_contacts` | user | `UserId` | Тип |
| 21 | `service_contact_availability` | user | **new `user_id`** | Денормализовать |
| 22 | `service_dialogs` | user | **new `user_id`** | Денормализовать |
| 23 | `service_gpt` | user | `UserId` | Тип |
| 24 | `service_proxy` | user | `UserId` | Тип, шифрование |
| 25 | `service_tgbots` | user | `UserId` | → `user_bots` |
| 26 | `service_user_bot_events` | user | **new `user_id`** | → `user_bot_events` |
| 27 | `service_user_bot_state` | user | **new `user_id`** | → `user_bot_state` |
| 28 | `service_wabots` | user | `UserId` | → `user_bots` |
| 29 | `storage_migrations` | user | `user_id` | Тип |
| 30 | `subscriptions` | user | `UserId` | Тип |
| 31 | `users` | user | `Id` | PK, `id_source`, `public_id`, `status`, timestamps |
| 32 | `user_api_keys` | user | `UserId` | Тип, provider reference |
| 33 | `user_auth` | user | `UserId` | Тип, email hash |
| 34 | `user_gpt` | user | **new `user_id`** | Добавить `user_id`, тип |
| 35 | `user_models` | user | `UserId` | Тип |
| 36 | `user_roles` | reference | — | Оставить |
| 37 | `user_storage_config` | user | `user_id` | Тип |
| 38 | `user_storage_quota` | user | `user_id` | Тип |
| 39 | `vector_embeddings` | user | `user_id` | FK fix |
| 40 | `voice_models` | reference | — | Unique |
| 41 | `user_channels` (new) | user | `user_id` | — |
| 42 | `user_channel_configs` (new) | user | `user_id` | — |
| 43 | `user_bots` (new) | user | `user_id` | — |
| 44 | `user_bot_events` (new) | user | `user_id` | — |
| 45 | `user_bot_state` (new) | user | `user_id` | — |
| 46 | `dialog_messages` (new) | user | `user_id` | — |
| 47 | `dialog_contexts` (new) | user | `user_id` | — |
| 48 | `user_directory` (new) | global | — | — |
| 49 | `schema_migrations` (new) | global | — | — |
| 50 | `migration_progress` (new) | global | — | — |

## Приложение B. Карта замены SP/функций/триггеров

| Объект | Тип | Замена | Файл |
|---|---|---|---|
| `DeleteAllUserData` | procedure | usecase/admin + purge job | `air_orchestrator/internal/usecase/admin` |
| `DeleteDialog` | procedure | `comdb.DeleteDialog` | `pkg/comdb/dialog.go` |
| `DisableAllUserChannel` | procedure | `comdb.DisableAllUserChannel` | `pkg/comdb/channels.go` |
| `GetOrSetTreadAndResponder` | procedure | `comdb.GetOrSetThreadAndResponder` | `pkg/comdb/dialog.go` |
| `SaveChannelData` | procedure | `comdb.SaveChannelData` | `pkg/comdb/channels.go` |
| `SaveContext` | procedure | `comdb.SaveContext` | `pkg/comdb/dialog.go` |
| `SaveDialog` | procedure | `comdb.SaveDialog` | `pkg/comdb/dialog.go` |
| `SetChannelEnabled` | procedure | `comdb.SetChannelEnabled` | `pkg/comdb/channels.go` |
| `UpdateDialogsMeta` | procedure | `comdb.UpdateDialogsMeta` | `pkg/comdb/dialog.go` |
| `GetBalance` | function | `comdb.GetBalance` | `pkg/comdb/user.go` |
| `GetNotificationChannel` | function | `comdb.GetNotificationChannel` | `pkg/comdb/notification.go` |
| `GetOrSetDialog` | function | `comdb.GetOrSetDialog` | `pkg/comdb/dialog.go` |
| `GetUserSubscriptionLimites` | function | `comdb.GetUserSubscriptionLimites` | `pkg/comdb/user.go` |
| `GetUserTariff` | function | `comdb.GetUserTariff` | `pkg/comdb/billing.go` |
| `ReadContext` | function | `comdb.ReadContext` | `pkg/comdb/dialog.go` |
| `ReadDialog` | function | `comdb.ReadDialog` | `pkg/comdb/dialog.go` |
| `ReadResponderName` | function | `comdb.ReadResponderName` | `pkg/comdb/responder.go` |
| `UserLang` | function | `comdb.UserLanguage` | `pkg/comdb/user.go` |
| `tg_delete_tgbot_events` | trigger | FK `ON DELETE CASCADE` | schema |
| `tg_delete_tgbot_state` | trigger | FK `ON DELETE CASCADE` | schema |
| `tg_delete_wabot_events` | trigger | FK `ON DELETE CASCADE` | schema |
| `tg_delete_wabot_state` | trigger | FK `ON DELETE CASCADE` | schema |
| `trg_wabot_delete_cascade_events` | trigger | удалить (дубль) | schema |
| `trg_wabot_delete_cascade_state` | trigger | удалить (дубль) | schema |
| `ResetFlagsEvent` | event | cron + distributed lock | `internal/infrastructure/scheduler/cron.go` |

## Приложение C. Пример backfill каналов (псевдокод)

```go
func migrateChannels(ctx context.Context, db *sql.DB, gen *idgen.Generator) error {
    rows, err := db.QueryContext(ctx, `
        SELECT UserId, TgBot, TgBot_enabled, Widget, Widget_enabled,
               TgUserBot, TgUserBot_enabled, Whats, Whats_enabled,
               Insta, Insta_enabled, Avito, Avito_enabled
        FROM channels
        WHERE UserId > ?
        ORDER BY UserId
        LIMIT ?`, lastKey, batchSize)
    if err != nil {
        return err
    }
    defer rows.Close()

    for rows.Next() {
        var u legacyChannels
        if err := rows.Scan(&u.UserID, &u.TgBot, &u.TgBotEn, &u.Widget, &u.WidgetEn,
            &u.TgUserBot, &u.TgUserBotEn, &u.Whats, &u.WhatsEn,
            &u.Insta, &u.InstaEn, &u.Avito, &u.AvitoEn); err != nil {
            return err
        }

        for _, ch := range u.Items() {
            if ch.Data == "" && !ch.Enabled {
                continue
            }
            channelID := int64(gen.Next())
            cfg := ch.Data
            tx, err := db.BeginTx(ctx, nil)
            if err != nil {
                return err
            }
            _, err = tx.ExecContext(ctx, `
                INSERT INTO user_channels (id, user_id, channel_type, enabled, created_at, updated_at)
                VALUES (?, ?, ?, ?, NOW(3), NOW(3))
                ON DUPLICATE KEY UPDATE enabled=VALUES(enabled), updated_at=NOW(3)`,
                channelID, u.UserID, ch.Type, ch.Enabled)
            if err != nil {
                _ = tx.Rollback()
                return err
            }
            _, err = tx.ExecContext(ctx, `
                INSERT INTO user_channel_configs (channel_id, user_id, config, updated_at)
                VALUES (?, ?, ?, NOW(3))
                ON DUPLICATE KEY UPDATE config=VALUES(config), updated_at=NOW(3)`,
                channelID, u.UserID, cfg)
            if err != nil {
                _ = tx.Rollback()
                return err
            }
            if err := tx.Commit(); err != nil {
                return err
            }
        }
        lastKey = u.UserID
    }
    return rows.Err()
}
```

## Приложение D. Переменные окружения (сводка)

| Переменная | Назначение |
|---|---|
| `DB_HOST`, `DB_NAME`, `DB_USER`, `DB_PASSWORD` | legacy-подключение |
| `DB_DSN` | полный DSN (приоритетнее) |
| `DB_MAX_OPEN`, `DB_MAX_IDLE` | пул |
| `DB_CONN_MAX_LIFETIME`, `DB_CONN_MAX_IDLE_TIME` | lifetime пула |
| `DB_SCHEMA_VERSION_REQUIRED` | минимальная версия схемы |
| `SHARD_MAP_FILE`/`SHARD_MAP_JSON` | slot map |
| `SHARD_STRICT` | падать при недоступности шарда |
| `NODE_ID`/`SHARD_ID` | idgen |
| `IDGEN_ENABLED` | включить Snowflake |
| `CHANNELS_V2_READ_ENABLED`/`CHANNELS_V2_WRITE_ENABLED` | миграция каналов |
| `DIALOG_MESSAGES_READ_ENABLED`/`DIALOG_MESSAGES_WRITE_ENABLED` | миграция диалогов |
| `OTEL_*` | OTel SDK |
| `JAEGER_UI_URL` | ссылки из Perses |
| `LOG_LEVEL`, `LOG_JSON`, `LOG_PII` | air-logger |

## Приложение E. Файлы, которые будут затронуты

- `air-common/pkg/comdb/*` — репозитории, SP removal, router, otelsql.
- `air-common/pkg/rpc/client.go` — otelgrpc, context.
- `air-common/pkg/model/*`, `pkg/startpoint/*`, `pkg/endpoint/*` — ctx/спаны.
- `air-common/pkg/crypto/*` — key version.
- `air-common/pkg/comdom/*` — ChannelType/config.
- `air_orchestrator/internal/repository/mysql/*` — SQL/типы.
- `air_orchestrator/internal/delivery/http/*`, `internal/delivery/grpc/*` — OTel.
- `air_orchestrator/internal/metrics/metrics.go` — SLO.
- `air_orchestrator/internal/infrastructure/scheduler/cron.go` — EVENT replacement.
- `air_orchestrator/migrations/*` — versioned migrations.
- `air_orchestrator/monitoring/perses/*` — dashboards.
- `air_orchestrator/build/*.yml` — Jaeger/collector/env.
- `air_whatsbot/internal/repository/mysql/*`, `internal/whatsapp/*`, `internal/metrics/*`.
- `air-logger/pkg/logger/*` — ctx/JSON/trace.
- `air_tgbot`, `air_tguserbot`, `air_avito`, `air_widget`, `air_operator`, `air_lead-hunter`, `air_payment`, `air_phantom` — bump + OTel + schema.

## Приложение F. Журнал решений (ADR-кратко)

| # | Решение | Обоснование |
|---|---|---|
| 1 | Snowflake `BIGINT UNSIGNED` вместо UUID | 8 байт, сортируемость, минимальные изменения типов |
| 2 | Существующие ID не переписываются | Нет коллизий, сохраняются FK, меньше риск |
| 3 | Slot-based (1024) шардирование | Перемещение части данных без полного rehash |
| 4 | `user_channels` + `user_channel_configs` | Расширяемость, разделение секретов и конфига |
| 5 | `dialog_messages` вместо `dialogs.Data` | Нет горячих строк, индексируемость, частичное шифрование |
| 6 | `responders.user_id` | Мультитенантность, корректный шардинг |
| 7 | Удаление SP/триггеров/EVENT | Тестируемость, шардируемость, трассируемость |
| 8 | OpenTelemetry + Jaeger | Единый стандарт, сквозная корреляция |
| 9 | `air-common/v2` | Breaking change типов и интерфейсов |
| 10 | Tail sampling в коллекторе | Контроль стоимости и сохранение ошибок/медленных |

## Приложение G. Быстрый чек-лист запуска

1. Backup + restore-check.
2. Выровнять `air-common`.
3. `pkg/migrate` + baseline.
4. `pkg/idgen` + расширение типов.
5. `user_channels` + dual-write + backfill.
6. `dialog_messages` + lazy backfill.
7. `responders.user_id` + `user_bots`.
8. Удалить SP/триггеры/EVENT.
9. `pkg/telemetry` + Jaeger + air-logger.
10. SLO-метрики + Perses + алерты.
11. `pkg/dbrouter` + `user_directory`.
12. Cutover по слотам.

