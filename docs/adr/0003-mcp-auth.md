# ADR 0003: защита `/int/mcp`

## Контекст

`/int/mcp` принимал сырой `X-Session-ID` вида `userId:provider`, без allowlist на
`tools/call`, без проверки владельца `resp_id` и без лимитов.

## Решение

- **Service-auth:** `X-Service-Key` сверяется constant-time с `MCP_SERVICE_KEY`.
  Пустой ключ отключает проверку (обратная совместимость).
- **User-token:** `X-Session-ID` может быть подписан HMAC-SHA256
  (`pkg/mcpsession`, `v1:<uid>:<provider>:<exp>:<hex>`, TTL 5 мин).
  `MCP_REQUIRE_SIGNED_SESSION=true` запрещает legacy-формат.
- **Allowlist:** `tools/call` сверяет инструмент с `buildToolsList(user, provider)`.
- **Anti-IDOR:** `lead_target` проверяет `UserOwnsResponder(user, resp_id)`.
- **Rate limit/audit/timeouts:** token bucket `(user, tool)`, durable `audit_log`
  (`action='mcp.tool.call'`), 30 c / 120 c (`generate_music`).
- **Идемпотентность:** необязательный `idempotency_key` для инструментов с побочными эффектами.

## Последствия

- Требуется обновить клиентов `air-common` (заголовок и подпись).
- mTLS остаётся инфраструктурным усилением; service-key уже включён.
- Scope-токены не вводятся, пока allowlist строится по флагам модели в БД.
