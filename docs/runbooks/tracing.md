# Runbook: OpenTelemetry

## 1. Включение

Трассировка включается только при заданном endpoint:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
# или
OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://otel-collector:4318/v1/traces
```

Без endpoint `telemetry.Init` возвращает no-op, сервис работает как раньше.
Propagators (W3C `traceparent`) устанавливаются всегда.

## 2. Что создаётся

| Сервис | Спаны |
|---|---|
| `air_orchestrator` | HTTP server span (`<METHOD> <route>`), `mcp.request`, `mcp.tool.call` |
| воркеры (air-common) | исходящий MCP-запрос с `traceparent` |

Атрибуты:

- HTTP: `http.method`, `http.route`, `http.status_code`;
- MCP: `tool.name`, `provider`, `user.id`, `tool.error`;
- `/metrics`, `/healthz`, `/readyz` не трассируются.

PII в спаны не пишется: только route, tool, user ID; аргументы инструментов и тела запросов — нет.

## 3. Проверка

1. Включить endpoint и перезапустить сервис.
2. Сделать запрос, получить `trace_id` из лога:
   `[trace=... span=...] MCP audit: ...`
3. Найти trace в Jaeger/UI.
4. Проверить, что `mcp.tool.call` — дочерний к HTTP-спану.

## 4. Collector и retention

- Collector: OTLP HTTP (`4318`) → backend (Jaeger/Tempo).
- Рекомендуемый retention: 7–14 дней для hot-хранилища, 30 дней — выборочно.
- Сэмплирование: `parentbased_traceidratio`, старт с 10–20%, при стабильности — выше.

## 5. Отключение

Убрать `OTEL_EXPORTER_OTLP_*` и перезапустить сервис. Логи останутся без `[trace=...]`,
но функциональность не меняется.
