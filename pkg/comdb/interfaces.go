package comdb

import (
	"encoding/json"

	"github.com/ikermy/air-common/pkg/comdom"
	"golang.org/x/oauth2"
)

type Exterior interface {
	GetOrSetTreadAndResponder(userID uint32, responderRealId uint64, responderName string, chatType ChatType) (uint64, error)
	DisableAllUserChannel(userID uint32) error
	GetNotificationChannel(userID uint32) (json.RawMessage, error)
	GetUserSubscriptionLimites(userID uint32) (json.RawMessage, error)
	SaveDialog(treadId uint64, message json.RawMessage) error
	ReadDialog(dialogId uint64, limit ...uint8) (json.RawMessage, error)
	DeleteDialog(userID uint32, dialogId uint64) error
	UpdateDialogsMeta(dialogId uint64, meta string) error
	ReadContext(dialogId uint64, provider comdom.ProviderType) (json.RawMessage, error)
	SaveContext(threadId uint64, provider comdom.ProviderType, dialogContext json.RawMessage) error
	GetActiveProvider(userID uint32) (comdom.ProviderType, error)
	GetAllUserModels(userID uint32) ([]comdom.UserModelRecord, error)
	UpdateUserGPT(userID uint32, modelId uint64, assistId string, allIds []byte) error
	GetUserVectorStorage(userID uint32) (string, error)
	SetChannelEnabled(userID uint32, chName string, status bool) error
	SaveUserModel(userID uint32, provider comdom.ProviderType, name, assistantId string, data []byte, def comdom.DefaultProvidersModels, ids json.RawMessage, operator bool) error
	SyncProviderModels(union comdom.Union, modelNames []string) (comdom.ProviderModelsSyncResult, error)
	GetOrSetUserStorageLimit(userID uint32, setStorage int64) (remaining uint64, totalLimit uint64, err error)
	ReadUserModel(userID uint32) ([]byte, *comdom.VecIds, error)
	SetUserSubscriptionNotified(user uint32) error
	DefaultProvidersModels(providerName string) (comdom.DefaultProvidersModels, error)

	// User Model Management - методы для управления моделями пользователя (для comdom.DB)
	ReadUserModelByProvider(userID uint32, provider comdom.ProviderType) ([]byte, *comdom.VecIds, error)
	GetActiveModel(userID uint32) (*comdom.UserModelRecord, error)
	GetModelByProvider(userID uint32, provider comdom.ProviderType) (*comdom.UserModelRecord, error)
	GetModelByProviderAnyStatus(userID uint32, provider comdom.ProviderType) (*comdom.UserModelRecord, error)
	SetActiveModel(userID uint32, modelId uint64) error
	SetActiveModelByProvider(userID uint32, provider comdom.ProviderType) error
	RemoveModelFromUser(userID uint32, modelId uint64) error

	// Vector Embeddings - методы для работы с эмбеддингами в MariaDB
	SaveEmbedding(userID uint32, modelId uint64, provider comdom.ProviderType, docID, docName, content string, embedding []float32, metadata comdom.DocumentMetadata) error
	GetEmbedding(modelId uint64, docID string) ([]float32, error)
	DeleteEmbedding(modelId uint64, docID string) error
	DeleteAllModelEmbeddings(modelId uint64) error
	CountModelEmbeddings(modelId uint64) (int, error)
	ListModelEmbeddings(modelId uint64, provider comdom.ProviderType) ([]comdom.VectorDocument, error)
	SearchSimilarEmbeddings(modelId uint64, provider comdom.ProviderType, queryEmbedding []float32, limit int) ([]comdom.VectorDocument, error)

	// Contact Availability - методы для работы с доступностью контактов в разных провайдерах
	SetContactAvailability(userID uint32, contact, provider string, isAvailable bool) error
	GetContactAvailability(userID uint32, contact string) (map[string]bool, error)
	GetContactsAvailableIn(userID uint32, provider string) ([]string, error)
	GetContactsInBothProviders(userID uint32, provider1, provider2 string) ([]string, error)

	// Google OAuth методы (токен единый для пользователя, не зависит от провайдера/модели)
	SaveGoogleToken(userID uint32, googleEmail string, token *oauth2.Token) error
	GetGoogleToken(userID uint32) (*oauth2.Token, string, error)
	RefreshGoogleTokenIfNeeded(userID uint32, oauthConfig *oauth2.Config) error
	DeleteGoogleToken(userID uint32) error

	// UserInfo методы
	UserTimeZone(userID uint32) (string, error)
	UserLanguage(userID uint32) string

	// UserAPIKey — персональные API-ключи провайдеров для каждого пользователя.
	GetUserAPIKey(userID uint32, provider ProviderType) (string, error)
	SetUserAPIKey(userId uint32, provider ProviderType, apiKey string) error
	DeleteUserAPIKey(userID uint32, provider ProviderType) error
}
