# air-common

![air-common](logo.png)

[🇬🇧 Английская версия](README.md)

`air-common` — базовая Go-библиотека для AI-микросервисов проекта `marusia_ai`, созданная в первую очередь для использования внутренними микросервисами семейства `air_`.

![Go version](https://img.shields.io/badge/Go-1.25.8-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)
[![Telegram](https://img.shields.io/badge/Telegram-Join%20Chat-blue?logo=telegram)](https://t.me/marusia_dev)

## Функциональность

### 🔌 Прозрачная абстракция провайдеров

- Единый интерфейс, скрывающий специфичные для провайдеров механизмы взаимодействия с моделями.
- Поддержка различных архитектур провайдеров, включая Mistral Agents & Conversations и API на основе запросов от OpenAI и Google.
- Специфичные для провайдера детали, такие как управление контекстом, состояние диалога, выполнение инструментов и потоковая обработка, обрабатываются внутри библиотеки.
- Последующие сервисы используют одинаковые контракты и порядок вызовов независимо от выбранного AI-провайдера.
- Общие Go-интерфейсы устраняют необходимость в специфичном для провайдера шаблонном коде на верхних слоях приложения.

### 👥 Нативная многопользовательская архитектура

- Полноценная многопользовательская работа с моделями, диалогами, сессиями, документами, API-ключами и realtime-соединениями.
- Строгое разделение данных по пользователям через `userID` на уровнях маршрутизации, клиентов провайдеров, хранения, инструментов и управления сессиями.
- Параллельная обработка независимых пользователей и их сессий.
- Определение API-ключа и доступа к провайдеру для каждого пользователя.
- Шифрование пользовательских данных с использованием Master Key.

### 💬 Текстовые, файловые и мультимодальные запросы

- Потоковая передача текстовых ответов через специфичные для провайдеров streaming API.
- Function calling и многошаговое выполнение инструментов.
- Загрузка, скачивание и удаление файлов, а также управление файлами у провайдеров.
- Транскрибация аудио и обработка голосовых сообщений.
- Поддержка текстовых документов, метаданных, embeddings и поиска по векторному сходству.

### 🎙️ Realtime и голосовые функции

- Нативная потоковая передача событий через WebSocket для интерактивных realtime-сессий с низкой задержкой.
- Единый жизненный цикл realtime-сессий для поддерживаемых провайдеров.
- Потоковая передача аудио, текста, транскрипций, событий прерывания и информации об использовании токенов.
- Нативная интеграция с Mistral Realtime API.
- Полная поддержка realtime-функций клонирования голоса Mistral.

### 🧑‍💼 Передача диалога оператору

- Режим оператора для передачи диалога от AI человеку-оператору.
- Синхронное и асинхронное взаимодействие с оператором.
- Операторские сессии с каналами сообщений, SSE-соединениями и тайм-аутами бездействия.
- Плавное возвращение из режима оператора к обработке AI.
- Управление операторскими сессиями в контексте пользователя и диалога.

### 🔐 Безопасность и межсервисная инфраструктура

- Шифрование на уровне приложения для API-ключей, OAuth-учётных данных, документов и защищённых пользовательских данных.
- RPC/gRPC-контракты для межсервисной конфигурации и получения пользовательского Master Key.

## Установка

Добавьте библиотеку в Go-модуль сервиса:

```bash
go get github.com/ikermy/air-common
```

## Использование

Базовая инициализация маршрутизатора моделей:

```go
package main

import (
	"context"

	"github.com/ikermy/air-common/pkg/model"
)

func main() {
	// Минимальная функциональность
	ctx, cancel := context.WithCancel(parent)
	router := model.NewModelRouter(ctx, nil)
	
	// Полная функциональность
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
[![Repo](https://img.shields.io/badge/github-air_tguserbot-blue?logo=github)](https://github.com/ikermy/air_tguserbot)
[![Repo](https://img.shields.io/badge/github-air_whatsbot-green?logo=github)](https://github.com/ikermy/air_whatsbot)
[![Repo](https://img.shields.io/badge/github-air_widget-purple?logo=github)](https://github.com/ikermy/air_widget)
[![Repo](https://img.shields.io/badge/github-air_avito-skyblue?logo=github)](https://github.com/ikermy/air_avito)
[![Repo](https://img.shields.io/badge/github-air_orchestrator-blue?logo=github)](https://github.com/ikermy/air_orchestrator)

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

`air-common` не является самостоятельным конечным приложением. Микросервисы используют её пакеты и передают собственные зависимости: базу данных, обработчики действий, провайдеры ключей и компоненты сохранения диалогов.

## Основные пакеты

| Пакет | Назначение |
| --- |---|
| `pkg/mode` | Параметры конфигурации библиотеки |
| `pkg/model` | Общие модели, интерфейсы, маршрутизатор и AI-сессии |
| `pkg/model/openai` | Интеграция с OpenAI |
| `pkg/model/mistral` | Интеграция с Mistral и голосовые сценарии |
| `pkg/model/google` | Интеграция с Google AI |
| `pkg/startpoint` | Запуск сессий и управление их жизненным циклом |
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

## Связанные сервисы

- [air-common](https://github.com/ikermy/air-common) — общая библиотека для AI-микросервисов
- [air_orchestrator](https://github.com/ikermy/air_orchestrator) — основной сервис оркестрации
- [air_tgbot](https://github.com/ikermy/air_tgbot) — Telegram-бот, работающий в режиме polling/webhook, с потоковой передачей дельт
- [air_tguserbot](https://github.com/ikermy/air_tguserbot) — Telegram-бот пользователя, способный принимать и совершать голосовые звонки
- [air_whatsbot](https://github.com/ikermy/air_whatsbot) — пользовательский WhatsApp-бот без Graph API, способный принимать и совершать голосовые звонки
- [air_widget](https://github.com/ikermy/air_widget) — чат-виджет для интеграции с любым веб-сайтом
- [air_avito](https://github.com/ikermy/air_avito) — бот для ответов в чатах Avito
- [air_operator](https://github.com/ikermy/air_operator) — сервис передачи ответов оператору и от оператора; AI работает со всеми типами ботов
- [air_lead-hunter](https://github.com/ikermy/air_lead-hunter) — сервис для поиска ботами лидов в Telegram и WhatsApp, включая исходящие голосовые звонки
- [air_payment](https://github.com/ikermy/air_payment) — сервис приёма криптовалютных платежей от пользователей через Bybit
- [marusia_crm](https://github.com/ikermy/marusia_crm) — сервис интеграции с внешними CRM-системами
- [air-logger](https://github.com/ikermy/air-logger) — вспомогательный сервис журналирования событий с поддержкой многопользовательского режима и сборщика логов Loki

## Лицензия

Проект распространяется по [лицензии MIT](LICENSE). Она разрешает свободно использовать, копировать, изменять и распространять программное обеспечение при условии сохранения текста лицензии и уведомления об авторских правах.

Полный текст лицензии доступен в файле [`LICENSE`](LICENSE).

## Контакты

[![Telegram](https://img.shields.io/badge/Telegram-Contact-blue?logo=telegram)](https://t.me/ikermy)
