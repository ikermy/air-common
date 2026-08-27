package model

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/ikermy/air-common/pkg/com"
	"github.com/ikermy/air-common/pkg/comdb"
	"github.com/ikermy/air-common/pkg/comdom"
)

// DB алиас для интерфейса БД
type DB = comdb.Exterior

// Inter интерфейс для работы с моделями Assistant
type Inter interface {
	NewMessage(operator Operator, msgType string, content *AssistResponse, name *string, files ...FileUpload) Message
	GetFileAsReader(userID uint32, url string) (io.Reader, error)
	GetOrSetRespGPT(assist Assistant, dialogID, respId uint64, respName string) (*RespModel, error)
	GetCh(respId uint64) (*Ch, error)
	GetRespIdByDialogID(dialogID uint64) (uint64, error)
	SaveAllContextDuringExit()
	Request(userID uint32, dialogID uint64, text string, files ...FileUpload) (AssistResponse, error)
	RequestStreaming(userID uint32, dialogID uint64, text string, onDelta func(delta string, done bool) error, files ...FileUpload) error
	CleanDialogData(dialogID uint64)
	DeleteTempFile(fileID string) error
	TranscribeAudio(userID uint32, audioData []byte, fileName string) (string, error)
	CleanUp()
	DisconnectUser(userID uint32)
	InvalidateUserAgentConfigCache(userID uint32)
	Shutdown(shutCh chan<- com.LogMsg)
	UpdateModelsListByProvider(ctx context.Context, union comdom.Union, apiKey string) ([]comdom.ProviderModel, error)
}

// RouterInterface минимальный интерфейс для доступа к методам роутера
type RouterInterface interface {
	ProvidersWithApiKeys(userID uint32) comdom.ProvidersAvailability
	RevokeUserAPIKey(userID uint32, provider comdom.ProviderType) error
}

// OpenAIManager расширяет Inter методами управления моделями OpenAI
type OpenAIManager interface {
	Inter
	CreateModel(userID uint32, provider comdom.ProviderType, modelData *comdom.UniversalModelData, fileIDs []comdom.Ids) (comdom.UMCR, error)
	UploadDocumentWithEmbedding(userID uint32, docName, content string, metadata comdom.DocumentMetadata) (string, error)
	SearchSimilarDocuments(userID uint32, query string, limit int) ([]comdom.VectorDocument, error)
	DeleteDocument(userID uint32, docID string) error
	ListUserDocuments(userID uint32) ([]comdom.VectorDocument, error)
}

// MistralManager расширяет Inter для Mistral-специфичных методов работы с библиотеками
type MistralManager interface {
	Inter
	CreateModel(userID uint32, provider comdom.ProviderType, modelData *comdom.UniversalModelData, fileIDs []comdom.Ids) (comdom.UMCR, error)
	UploadFileToProvider(userID uint32, fileName string, fileData []byte) (string, error)
	DeleteDocumentFromLibrary(userID uint32, documentID string) error
	AddFileToLibrary(userID uint32, fileID, fileName string) error
	CreateVoice(userID uint32, request comdom.CreateVoiceRequest) (comdom.Voice, error)
	ListVoices(userID uint32, limit, offset int, voiceType string) (comdom.VoiceList, error)
	GetVoice(userID uint32, voiceID string) (comdom.Voice, error)
	UpdateVoice(userID uint32, voiceID string, request comdom.UpdateVoiceRequest) (comdom.Voice, error)
	DeleteVoice(userID uint32, voiceID string) (comdom.Voice, error)
	GetVoiceSample(userID uint32, voiceID string) (io.ReadCloser, string, error)
}

// GoogleManager расширяет Inter для Google-специфичных методов
type GoogleManager interface {
	Inter
	CreateModel(userID uint32, provider comdom.ProviderType, modelData *comdom.UniversalModelData, fileIDs []comdom.Ids) (comdom.UMCR, error)
	UploadDocumentWithEmbedding(userID uint32, docName, content string, metadata comdom.DocumentMetadata) (string, error)
	SearchSimilarDocuments(userID uint32, query string, limit int) ([]comdom.VectorDocument, error)
	DeleteDocument(userID uint32, docID string) error
	ListUserDocuments(userID uint32) ([]comdom.VectorDocument, error)
}

// ActionHandler интерфейс для обработки функций ассистента
type ActionHandler interface {
	RunAction(ctx context.Context, functionName, arguments string, provider comdom.ProviderType, userID uint32) string
}

// MCPToolDefinition описание инструмента от MCP сервера (tools/list).
// inputSchema не содержит user_id — он передаётся через X-Session-ID заголовок.
type MCPToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// MCPConfigProvider расширяет ActionHandler методами получения конфигурации от MCP-сервера.
// Реализуется UniversalActionHandler (pkg/model/action_handler.go).
type MCPConfigProvider interface {
	ActionHandler
	FetchToolsList(ctx context.Context, userID uint32, provider comdom.ProviderType) ([]MCPToolDefinition, error)
	FetchSystemPrompt(ctx context.Context, userID uint32, provider comdom.ProviderType) (string, error)
}

// RealtimeEvent — внутреннее событие голосовой realtime-сессии.
//
// Это не wire-формат WebSocket API. Перед отправкой клиенту событие должно
// быть преобразовано через NormalizeRealtimeEvent или MarshalRealtimeEvent.
//
// PCM-аудио не является RealtimeEvent: оно передаётся отдельно через
// RealtimeProvider.GetRealtimeAudio.
//
// Type может иметь значения:
//
//   - "session_started"        — realtime-сессия провайдера готова
//   - "speech_started"         — VAD обнаружил начало речи пользователя
//   - "speech_stopped"         — VAD обнаружил окончание речи пользователя
//   - "input_transcript_delta" — частичная транскрипция речи пользователя
//   - "input_transcript_done"  — транскрипция речи пользователя завершена
//   - "response_started"       — провайдер начал формировать ответ
//   - "response_text_delta"    — частичная текстовая транскрипция ответа
//   - "response_done"          — ответ провайдера завершён
//   - "interrupted"            — ответ прерван пользователем (barge-in)
//   - "function_result"        — результат вызова инструмента
//   - "token_usage"            — статистика использованных токенов
//   - "error"                  — ошибка realtime-сессии
type RealtimeEvent struct {
	Type       string
	Text       string
	Delta      string
	Err        error
	ResponseID string

	// Deprecated compatibility fields used by existing provider integrations.
	Data  []byte
	Files []File
}

// RealtimeProvider опциональный интерфейс для голосовых сессий реального времени.
// Реализуется OpenAIModel (OpenAI Realtime API) и GoogleModel (Google Multimodal Live API).
type RealtimeProvider interface {
	StartRealtimeSession(userID uint32, dialogID, respId uint64) error
	CloseRealtimeSession(respId uint64)
	SendRealtimeAudio(respId uint64, pcm16 []byte) error
	SubscribeEvents(respId uint64) (<-chan RealtimeEvent, error)
	UnsubscribeEvents(respId uint64, sub <-chan RealtimeEvent)
	GetRealtimeAudio(respId uint64) (<-chan []byte, error)
	GetRealtimeDrain(respId uint64) (<-chan struct{}, error)
	GetRealtimeGenerating(respId uint64) *atomic.Bool
	SetRealtimeDisconnectCallback(respId uint64, callback func(respId uint64)) error
}

// DeltaProcessor интерфейс унифицированной обработки стриминговых дельт.
// Реализуется Startpoint для клиентских каналов (Telegram/WhatsApp/Instagram и т.д.).
type StreamDeltaKind string

const (
	StreamDeltaKindText  StreamDeltaKind = "text"
	StreamDeltaKindEvent StreamDeltaKind = "event"
)

// StreamDeltaResult результат обработки входящей потоковой дельты.
// Для text-событий используется поле Text.
// Для function_call/service-событий сохраняется RawJSON и, если доступно, Arguments.
type StreamDeltaResult struct {
	Kind      StreamDeltaKind `json:"kind"`
	Text      string          `json:"text,omitempty"`
	Complete  bool            `json:"complete,omitempty"`
	EventType string          `json:"event_type,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	RawJSON   string          `json:"raw_json,omitempty"`
}

type DeltaProcessor interface {
	ProcessStreamDelta(respId uint64, rawChunk string) (StreamDeltaResult, error)
	GetStreamDisplayText(respId uint64) string
	ResetStreamAccumulator(respId uint64)
}
