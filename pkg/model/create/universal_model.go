package create

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/mode"
)

// New creates the provider-aware UniversalModel facade.
func New(ctx context.Context, db DB) *UniversalModel {
	m := &UniversalModel{
		ctx: ctx,
		db:  db,
	}

	// Инициализируем OpenAI клиент БЕЗ глобального ключа — глобальные ключи из конфига
	// должны игнорироваться полностью. Персональный ключ читается из БД через keyResolver.
	m.openaiClient = &OpenAIAgentClient{
		url: mode.OpenAIAgentsURL,
		ctx: ctx,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		universalModel: m, // Передаем ссылку на universalModel
	}
	m.openaiClient.SetKeyResolver(func(userID uint32) string {
		if key, err := db.GetUserAPIKey(userID, comdom.ProviderOpenAI); err == nil {
			return key
		}
		return ""
	})

	// Инициализируем Mistral клиент БЕЗ глобального ключа — глобальные ключи из конфига
	// должны игнорироваться полностью. Персональный ключ читается из БД через keyResolver.
	m.mistralClient = &MistralAgentClient{
		url:            mode.MistralAgentsURL,
		ctx:            ctx,
		universalModel: m,
	}
	m.mistralClient.SetKeyResolver(func(userID uint32) string {
		if key, err := db.GetUserAPIKey(userID, comdom.ProviderMistral); err == nil {
			return key
		}
		return ""
	})

	// Инициализируем google клиент БЕЗ глобального ключа — глобальные ключи из конфига
	// должны игнорироваться полностью. Персональный ключ читается из БД через keyResolver.
	m.googleClient = &GoogleAgentClient{
		url:            mode.GoogleAgentsURL,
		ctx:            ctx,
		universalModel: m,
	}
	m.googleClient.SetKeyResolver(func(userID uint32) string {
		if key, err := db.GetUserAPIKey(userID, comdom.ProviderGoogle); err == nil {
			return key
		}
		return ""
	})

	return m
}

// CreateModel creates provider resources and returns the database references.
func (m *UniversalModel) CreateModel(userID uint32, provider comdom.ProviderType, modelData *comdom.UniversalModelData, fileIDs []comdom.Ids) (comdom.UMCR, error) {
	if modelData == nil {
		return comdom.UMCR{}, fmt.Errorf("modelData не может быть nil")
	}

	if modelData.UseModelName == nil {
		return comdom.UMCR{}, fmt.Errorf("modelData.UseModelName не может быть пустым")
	}

	switch provider {
	case comdom.ProviderOpenAI:
		return m.createModel(userID, modelData, fileIDs)
	case comdom.ProviderMistral:
		return m.createMistralModel(userID, modelData, fileIDs)
	case comdom.ProviderGoogle:
		return m.createGoogleModel(userID, modelData, fileIDs)
	default:
		return comdom.UMCR{}, fmt.Errorf("неизвестный провайдер: %s", provider)
	}
}

// SaveModel сохраняет модель в БД в универсальном формате
// Работает для любого провайдера (OpenAI, Mistral..)
// Автоматически устанавливает модель как активную если это первая модель пользователя
func (m *UniversalModel) SaveModel(userID uint32, umcr comdom.UMCR, data *comdom.UniversalModelData) error {
	if data == nil {
		return fmt.Errorf("не указана модель провайдера")
	}
	if data.UseModelName == nil {
		data.UseModelName = &comdom.UseModelName{}
	}

	// При частичном обновлении клиент может прислать только часть UseModelName.
	// Восстанавливаем отсутствующие ссылки на модели из актуальных данных БД.
	if data.UseModelName.GptType == nil || data.UseModelName.GptType.ID == 0 {
		existingModels, lookupErr := m.db.GetAllUserModels(userID)
		if lookupErr == nil {
			for _, existing := range existingModels {
				if existing.Provider != umcr.Provider {
					continue
				}
				if (data.UseModelName.GptType == nil || data.UseModelName.GptType.ID == 0) && existing.GptType != nil && existing.GptType.ID != 0 {
					data.UseModelName.GptType = existing.GptType
				}
				break
			}
		}
	}

	// При обновлении конфигурации клиент может прислать только имя модели.
	// Model в user_gpt — это FK на gpt_models.Id, поэтому нельзя сохранять ID=0.
	// Восстанавливаем ID из уже существующей записи для любого провайдера.
	if data.UseModelName.GptType == nil || data.UseModelName.GptType.ID == 0 {
		return fmt.Errorf("не указан корректный ID модели gpt_models для провайдера %s", umcr.Provider)
	}
	if data.Provider != comdom.ProviderMistral {
		if data.Realtime && (data.UseModelName.Realtime == nil || data.UseModelName.Realtime.ID == 0) {
			return fmt.Errorf("не указан корректный ID realtime-модели для провайдера %s", umcr.Provider)
		}
	} else {
		// Для мистраль структура RealTime другая {
		if data.Realtime && (data.RealtimeVAD.Mistral.STTModel == nil || data.RealtimeVAD.Mistral.TTSModel == nil) {
			return fmt.Errorf("не указан корректный ID realtime-модели для провайдера %s", umcr.Provider)
		}
	}

	compressed, err := compressModelData(data)
	if err != nil {
		return err
	}

	// Realtime is optional for ordinary models, so its descriptor may be nil.
	var realtimeModelID uint
	if data.UseModelName.Realtime != nil {
		realtimeModelID = data.UseModelName.Realtime.ID
	}

	err = m.db.SaveUserModel(
		userID,
		umcr.Provider,
		data.Name,
		umcr.AssistID,
		compressed,
		comdom.DefaultProvidersModels{
			GeneralModelID:  data.UseModelName.GptType.ID,
			RealTimeModelID: realtimeModelID,
		},
		umcr.AllIds,
		data.Operator,
	)
	if err != nil {
		return fmt.Errorf("ошибка сохранения модели в БД: %w", err)
	}

	return nil
}

// SetMistralMCPFetchers устанавливает MCP-fetchers на mistralClient.
// Вызывается из mistral/model.go после инициализации UniversalModel.
func (m *UniversalModel) SetMistralMCPFetchers(promptFetcher GooglePromptHintFetcher, toolsFetcher GoogleFunctionDeclarationsFetcher) {
	if m.mistralClient != nil {
		m.mistralClient.SetMCPConfigFetchers(promptFetcher, toolsFetcher)
	}
}
