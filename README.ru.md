# AiR_Common

![AiR_Common](air_common_logo.png)

[🇬🇧 English version](README.md)

`air_common` — базовая Go-библиотека для AI-микросервисов проекта `marusia_ai`, созданная в первую очередь для использования во внутренних микросервисах семейства `air_`.

![Go version](https://img.shields.io/badge/Go-1.25.8-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)
[![Telegram](https://img.shields.io/badge/Telegram-Join%20Chat-blue?logo=telegram)](https://t.me/marusia_dev)

## Связанные сервисы
- [air_common](https://github.com/ikermy/air_common) — общая библиотека для AI‑микросервисов
- [air_orchestrator](https://github.com/ikermy/air_orchestrator) — главный сервис оркестратор
- [air_tgbot](https://github.com/ikermy/air_tgbot) — Telegram Bot работа в режиме polling/webhook с возможностью стриминга дельт
- [air_tguserbot](https://github.com/ikermy/air_tguserbot) — Telegram пользовательский бот с возможностью принимать и совершать голосовые звонки
- [air_whatsbot](https://github.com/ikermy/air_whatsbot) — WhatsApp пользовательский бот без использования GraphAPI с возможностью принимать и совершать голосовые звонки
- [air_widget](https://github.com/ikermy/air_widget) — Widget виджет чат для интеграции на любые сайты
- [air_avito](https://github.com/ikermy/air_avito) — бот для ответов в чатах Авито
- [air_operator](https://github.com/ikermy/air_operator) — Сервис переадресации ответов на/от оператора AI работает для всех типов ботов
- [air_lead-hunter](https://github.com/ikermy/air_lead-hunter) — Сервис поиска лидов ботами в Telegram и WhatsApp, в том числе с исходязими голосовыми вызовами
- [air_payment](https://github.com/ikermy/air_payment) — Сервис приёма криптоплатежей от пользователей через Bybit
- [marusia_crm](https://github.com/ikermy/marusia_crm) — Сервис интеграции с внешними CRM системами
- [air_logger](https://github.com/ikermy/air_logger) — Вспомогательный сервис логирования событий с поддержкой многопользовательского режима и поддержкой сборщика логов loki

## Установка

Добавьте библиотеку в Go-модуль сервиса:

```bash
go get github.com/ikermy/air_common
```

## Использование

Базовая инициализация маршрутизатора моделей:

```go
package main

import (
	"context"

	"github.com/ikermy/air_common/pkg/model"
)

func main() {
	// Минимальный функционал
	ctx, cancel := context.WithCancel(parent)
	router := model.NewModelRouter(ctx, nil)
	
	// Полный функционал
	d, err := db.New(ctx)
	e := endpoint.New(ctx, d)
	router := model.NewModelRouter(ctx, d,
		model.WithDialogSaver(e),
		openai.NewAsRouterOption(),
		mistral.NewAsRouterOption(),
		google.NewAsRouterOption())
}
```

Конкретные AI-провайдеры подключаются через опции маршрутизатора и соответствующие пакеты `pkg/model/openai`, `pkg/model/mistral` и `pkg/model/google`.

Примеры практического использования:

[![Repo](https://img.shields.io/badge/github-air_orchestrator?logo=github)](https://github.com/ikermy/air_orchestrator)
[![Repo](https://img.shields.io/badge/github-air_tgbot-blue?logo=github)](https://github.com/ikermy/air_tgbot)
[![Repo](https://img.shields.io/badge/github-air_widget-blue?logo=github)](https://github.com/ikermy/air_widget)
[![Repo](https://img.shields.io/badge/github-air_avito-blue?logo=github)](https://github.com/ikermy/air_avito)
[![Repo](https://img.shields.io/badge/github-air_orchestrator-blue?logo=github)](https://github.com/ikermy/air_orchestrator)



## Функциональность

- единый интерфейс работы с OpenAI, Mistral и Google;
- маршрутизация запросов к AI-моделям;
- текстовые и realtime-сессии;
- потоковая обработка текста, аудио и событий;
- работа с файлами, документами и голосами;
- MCP-инструменты и обработка function calling;
- общие модели данных, интерфейсы, каналы и обработка ошибок;
- сохранение диалогов и интеграция с базой данных;
- RPC/gRPC-компоненты;
- интеграция с Google Calendar и Google Sheets;
- операторский режим, шифрование и работа с API-ключами.

## Архитектура

Библиотека предоставляет общие контракты и инфраструктурные компоненты для сервисов `air_`:

```text
air_-сервис
    |
    +--> model.Router
    |       |
    |       +--> OpenAI
    |       +--> Mistral
    |       +--> Google
    |
    +--> startpoint / channels / realtime events
    +--> endpoint / comdb
    +--> rpc / google_services / crypto
```

`air_common` не является самостоятельным конечным приложением. Микросервисы используют её пакеты, передавая собственные зависимости: базу данных, обработчики действий, провайдеры ключей и компоненты сохранения диалогов.

## Основные пакеты

| Пакет | Назначение |
| --- | --- |
| `pkg/model` | Общие модели, интерфейсы, роутер и AI-сессии |
| `pkg/model/openai` | Интеграция с OpenAI |
| `pkg/model/mistral` | Интеграция с Mistral и голосовыми сценариями |
| `pkg/model/google` | Интеграция с Google AI |
| `pkg/startpoint` | Запуск и жизненный цикл сессий |
| `pkg/endpoint` | Диалоги, уведомления и внешние endpoints |
| `pkg/comdb` | Контракты и операции хранения |
| `pkg/rpc` | RPC/gRPC-клиент и protobuf-контракты |
| `pkg/crypto` | Шифрование и работа с ключами |
| `pkg/google_services` | Google Calendar и Google Sheets |

## Конфигурация

Библиотека не задаёт единый обязательный набор переменных окружения: конфигурация передаётся вызывающим микросервисом через его зависимости и настройки.

В зависимости от подключённых компонентов могут потребоваться:

- API-ключи OpenAI, Mistral или Google;
- параметры подключения к базе данных;
- OAuth-настройки Google-сервисов;
- настройки MCP-серверов;
- Master Key Provider для расшифровки защищённых API-ключей.

Названия переменных окружения и формат конфигурации определяются конкретным `air_`-сервисом.

## Лицензия

Проект распространяется по лицензии [MIT](LICENSE). Она разрешает свободно использовать, копировать, изменять и распространять программное обеспечение при сохранении текста лицензии и уведомления об авторских правах.

Полный текст лицензии доступен в файле [`LICENSE`](LICENSE).

## Контакты
[![Telegram](https://img.shields.io/badge/Telegram-Contact-blue?logo=telegram)](https://t.me/ikermy)
