# AiR_Common

![AiR_Common](air_common_logo.png)

[🇷🇺 Russian version](README.ru.md)

`air_common` is a foundational Go library for the AI microservices of the `marusia_ai` project, created primarily for use by the internal microservices of the `air_` family.

![Go version](https://img.shields.io/badge/Go-1.25.8-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)
[![Telegram](https://img.shields.io/badge/Telegram-Join%20Chat-blue?logo=telegram)](https://t.me/marusia_dev)

## Related services

- [air_common](https://github.com/ikermy/air_common) — shared library for AI microservices
- [air_orchestrator](https://github.com/ikermy/air_orchestrator) — main orchestration service
- [air_tgbot](https://github.com/ikermy/air_tgbot) — Telegram bot operating in polling/webhook mode with delta streaming
- [air_tguserbot](https://github.com/ikermy/air_tguserbot) — Telegram user bot that can receive and make voice calls
- [air_whatsbot](https://github.com/ikermy/air_whatsbot) — WhatsApp user bot without Graph API that can receive and make voice calls
- [air_widget](https://github.com/ikermy/air_widget) — chat widget for integration into any website
- [air_avito](https://github.com/ikermy/air_avito) — bot for replying in Avito chats
- [air_operator](https://github.com/ikermy/air_operator) — service for forwarding responses to and from an operator; AI works with all bot types
- [air_lead-hunter](https://github.com/ikermy/air_lead-hunter) — service for bots to find leads in Telegram and WhatsApp, including outgoing voice calls
- [air_payment](https://github.com/ikermy/air_payment) — service for receiving cryptocurrency payments from users through Bybit
- [marusia_crm](https://github.com/ikermy/marusia_crm) — service for integrating with external CRM systems
- [air_logger](https://github.com/ikermy/air_logger) — auxiliary event-logging service with multi-user support and Loki log collector support

## Installation

Add the library to the service's Go module:

```bash
go get github.com/ikermy/air_common
```

## Usage

Basic model router initialization:

```go
package main

import (
	"context"

	"github.com/ikermy/air_common/pkg/model"
)

func main() {
	// Minimal functionality
	ctx, cancel := context.WithCancel(parent)
	router := model.NewModelRouter(ctx, nil)
	
	// Full functionality
	d, err := db.New(ctx)
	e := endpoint.New(ctx, d)
	router := model.NewModelRouter(ctx, d,
		model.WithDialogSaver(e),
		openai.NewAsRouterOption(),
		mistral.NewAsRouterOption(),
		google.NewAsRouterOption())
}
```

Specific AI providers are connected through router options and the corresponding `pkg/model/openai`, `pkg/model/mistral`, and `pkg/model/google` packages.

Examples of practical usage:

[![Repo](https://img.shields.io/badge/github-air_orchestrator?logo=github)](https://github.com/ikermy/air_orchestrator)
[![Repo](https://img.shields.io/badge/github-air_tgbot-blue?logo=github)](https://github.com/ikermy/air_tgbot)
[![Repo](https://img.shields.io/badge/github-air_widget-blue?logo=github)](https://github.com/ikermy/air_widget)
[![Repo](https://img.shields.io/badge/github-air_avito-blue?logo=github)](https://github.com/ikermy/air_avito)
[![Repo](https://img.shields.io/badge/github-air_orchestrator-blue?logo=github)](https://github.com/ikermy/air_orchestrator)

## Features

- unified interface for working with OpenAI, Mistral, and Google;
- request routing to AI models;
- text and realtime sessions;
- streaming text, audio, and event processing;
- working with files, documents, and voice messages;
- MCP tools and function calling processing;
- shared data models, interfaces, channels, and error handling;
- dialog persistence and database integration;
- RPC/gRPC components;
- Google Calendar and Google Sheets integration;
- operator mode, encryption, and API-key handling.

## Architecture

The library provides shared contracts and infrastructure components for `air_` services:

```text
air_ service
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

`air_common` is not a standalone end-user application. Microservices use its packages and provide their own dependencies: a database, action handlers, key providers, and dialog persistence components.

## Main packages

| Package | Purpose                                            |
| --- |----------------------------------------------------|
| `pkg/mode` | Parameters for configuring library                 |
| `pkg/model` | Shared models, interfaces, router, and AI sessions |
| `pkg/model/openai` | OpenAI integration                                 |
| `pkg/model/mistral` | Mistral integration and voice workflows            |
| `pkg/model/google` | Google AI integration                              |
| `pkg/startpoint` | Session startup and lifecycle management           |
| `pkg/endpoint` | Dialogs, notifications, and external endpoints     |
| `pkg/comdb` | Storage contracts and operations                   |
| `pkg/rpc` | RPC/gRPC client and protobuf contracts             |
| `pkg/crypto` | Encryption and key handling                        |
| `pkg/google_services` | Google Calendar and Google Sheets                  |

## Configuration

The library does not define one mandatory set of environment variables: configuration is passed by the calling microservice through its dependencies and settings.

Depending on the connected components, the following may be required:

- OpenAI, Mistral, or Google API keys;
- database connection parameters;
- Google service OAuth settings;
- MCP server settings;
- a Master Key Provider for decrypting protected API keys.

Environment variable names and configuration format are defined by the specific `air_` service.

## License

The project is distributed under the [MIT License](LICENSE). It permits freely using, copying, modifying, and distributing the software provided that the license text and copyright notice are retained.

The full license text is available in the [`LICENSE`](LICENSE) file.

## Contacts

[![Telegram](https://img.shields.io/badge/Telegram-Contact-blue?logo=telegram)](https://t.me/ikermy)
