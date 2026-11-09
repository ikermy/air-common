package comdb

import (
	"encoding/json"

	"github.com/ikermy/air-common/pkg/comdom"
	"golang.org/x/oauth2"
)

// Exterior is the composed repository surface. New code should depend on the
// narrow interfaces below instead of the whole Exterior.
type Exterior interface {
	DialogStore
	ChannelStore
	UserModelStore
	EmbeddingStore
	ContactAvailabilityStore
	GoogleTokenStore
	UserInfoStore
	UserAPIKeyStore
}

// DialogStore — диалоги, контексты и респондеры.
type DialogStore interface {
	GetOrSetTreadAndResponder(userID uint64, responderRealId uint64, responderName string, chatType comdom.ChannelType) (uint64, error)
	SaveDialog(treadId uint64, message json.RawMessage) error
	ReadDialog(dialogId uint64, limit ...uint8) (json.RawMessage, error)
	DeleteDialog(userID uint64, dialogId uint64) error
	UpdateDialogsMeta(dialogId uint64, meta string) error
	ReadContext(dialogId uint64, provider comdom.ProviderType) (json.RawMessage, error)
	SaveContext(threadId uint64, provider comdom.ProviderType, dialogContext json.RawMessage) error
}

// ChannelStore — каналы пользователя и уведомления.
type ChannelStore interface {
	DisableAllUserChannel(userID uint64) error
	GetNotificationChannel(userID uint64) (json.RawMessage, error)
	SetChannelEnabled(userID uint64, chName string, status bool) error
}

// UserModelStore — модели пользователя, провайдеры и голоса.
type UserModelStore interface {
	GetActiveProvider(userID uint64) (comdom.ProviderType, error)
	GetAllUserModels(userID uint64) ([]comdom.UserModelRecord, error)
	UpdateUserGPT(userID uint64, modelId uint64, assistId string, allIds []byte) error
	GetUserVectorStorage(userID uint64) (string, error)
	SaveUserModel(userID uint64, provider comdom.ProviderType, name, assistantId string, data []byte, def comdom.DefaultProvidersModels, ids json.RawMessage, operator bool) error
	SyncProviderModels(union comdom.Union, modelNames []string) (comdom.ProviderModelsSyncResult, error)
	SyncVoiceModels(provider comdom.ProviderType, kind comdom.VoiceKind, modelNames []string) (comdom.ProviderModelsSyncResult, error)
	GetVoiceModels(provider comdom.ProviderType) ([]comdom.ProviderModel, error)
	GetProviderModels(provider comdom.ProviderType, modelType comdom.ModelType) ([]comdom.ProviderModel, error)
	ReadUserModel(userID uint64) ([]byte, *comdom.VecIds, error)
	DefaultProvidersModels(providerName string) (comdom.DefaultProvidersModels, error)
	ReadUserModelByProvider(userID uint64, provider comdom.ProviderType) ([]byte, *comdom.VecIds, error)
	GetActiveModel(userID uint64) (*comdom.UserModelRecord, error)
	GetModelByProvider(userID uint64, provider comdom.ProviderType) (*comdom.UserModelRecord, error)
	GetModelByProviderAnyStatus(userID uint64, provider comdom.ProviderType) (*comdom.UserModelRecord, error)
	SetActiveModel(userID uint64, modelId uint64) error
	SetActiveModelByProvider(userID uint64, provider comdom.ProviderType) error
	RemoveModelFromUser(userID uint64, modelId uint64) error
}

// EmbeddingStore — векторные эмбеддинги.
type EmbeddingStore interface {
	SaveEmbedding(userID uint64, modelId uint64, provider comdom.ProviderType, docID, docName, content string, embedding []float32, metadata comdom.DocumentMetadata) error
	GetEmbedding(modelId uint64, docID string) ([]float32, error)
	DeleteEmbedding(modelId uint64, docID string) error
	DeleteAllModelEmbeddings(modelId uint64) error
	CountModelEmbeddings(modelId uint64) (int, error)
	ListModelEmbeddings(modelId uint64, provider comdom.ProviderType) ([]comdom.VectorDocument, error)
	SearchSimilarEmbeddings(modelId uint64, provider comdom.ProviderType, queryEmbedding []float32, limit int) ([]comdom.VectorDocument, error)
}

// ContactAvailabilityStore — доступность контактов по провайдерам.
type ContactAvailabilityStore interface {
	SetContactAvailability(userID uint64, contact, provider string, isAvailable bool) error
	GetContactAvailability(userID uint64, contact string) (map[string]bool, error)
	GetContactsAvailableIn(userID uint64, provider string) ([]string, error)
	GetContactsInBothProviders(userID uint64, provider1, provider2 string) ([]string, error)
}

// GoogleTokenStore — OAuth-токены Google (один на пользователя).
type GoogleTokenStore interface {
	SaveGoogleToken(userID uint64, googleEmail string, token *oauth2.Token) error
	GetGoogleToken(userID uint64) (*oauth2.Token, string, error)
	RefreshGoogleTokenIfNeeded(userID uint64, oauthConfig *oauth2.Config) error
	DeleteGoogleToken(userID uint64) error
}

// UserInfoStore — базовые данные пользователя и лимиты.
type UserInfoStore interface {
	GetUserSubscriptionLimites(userID uint64) (json.RawMessage, error)
	GetOrSetUserStorageLimit(userID uint64, setStorage int64) (remaining uint64, totalLimit uint64, err error)
	SetUserSubscriptionNotified(user uint64) error
	GetAnyUserAPIKey(provider comdom.ProviderType) (string, error)
	UserTimeZone(userID uint64) (string, error)
	UserLanguage(userID uint64) string
}

// UserAPIKeyStore — персональные API-ключи провайдеров.
type UserAPIKeyStore interface {
	GetUserAPIKey(userID uint64, provider comdom.ProviderType) (string, error)
	SetUserAPIKey(userId uint64, provider comdom.ProviderType, apiKey string) error
	DeleteUserAPIKey(userID uint64, provider comdom.ProviderType) error
}
