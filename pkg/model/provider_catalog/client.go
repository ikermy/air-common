package provider_catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ikermy/air-common/v2/pkg/comdom"
	"github.com/ikermy/air-common/v2/pkg/comerrors"
	"github.com/ikermy/air-common/v2/pkg/mode"
)

// Client fetches provider model lists from external provider APIs.
// It is intentionally kept outside pkg/comdb so that the DB layer remains database-only.
type Client struct {
	HTTPClient *http.Client
	// ElevenLabsBaseURL переопределяет базовый URL ElevenLabs (для тестов).
	ElevenLabsBaseURL string
	// GoogleBaseURL переопределяет базовый URL Google Generative Language API (для тестов).
	GoogleBaseURL string
}

// DefaultMistralSTTModel is the production default Voxtral realtime
// transcription model. STT models are not stored in realtime_models because
// that table has a single realtime-model role in the legacy schema.
const DefaultMistralSTTModel = "voxtral-mini-transcribe-realtime-2602"

func NewClient() *Client {
	return &Client{HTTPClient: &http.Client{}}
}

type Syncer interface {
	SyncProviderModels(union comdom.Union, modelNames []string) (comdom.ProviderModelsSyncResult, error)
}

func SyncProviderModels(ctx context.Context, syncer Syncer, union comdom.Union, apiKey string) (comdom.ProviderModelsSyncResult, error) {
	result := comdom.ProviderModelsSyncResult{Provider: union.Provider}
	if ctx == nil {
		ctx = context.Background()
	}

	client := NewClient()
	modelNames, err := client.FetchModelNames(ctx, union, apiKey)
	if err != nil {
		return result, fmt.Errorf("не удалось получить каталог моделей провайдера %s: %w", union.Provider, err)
	}

	if union.ModelType.IsGeneral() || union.ModelType.IsRealtime() {
		result, err = syncer.SyncProviderModels(union, modelNames)
	}
	if err != nil {
		return result, fmt.Errorf("не удалось синхронизировать каталог моделей провайдера %s: %w", union.Provider, err)
	}
	return result, nil
}

// FetchModelNames получает актуальный список моделей провайдера из внешнего API.
func (c *Client) FetchModelNames(
	ctx context.Context,
	union comdom.Union,
	apiKey string,
) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	client := c
	if client == nil || client.HTTPClient == nil {
		client = NewClient()
	}

	if !union.Provider.IsValid() {
		return nil, fmt.Errorf("некорректный provider: %d", union.Provider)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("пустой API-ключ для провайдера %s", union.Provider.String())
	}

	switch union.Provider {
	case comdom.ProviderOpenAI:
		switch {
		case union.ModelType.IsGeneral():
			return client.generalOpenAIModels(ctx, apiKey)
		case union.ModelType.IsRealtime():
			return client.realtimeOpenAIModels(ctx, apiKey)
		default:
			return client.fetchOpenAIModels(ctx, apiKey)
		}
	case comdom.ProviderMistral:
		switch {
		case union.ModelType.IsGeneral():
			return client.generalMistralModels(ctx, apiKey)
		case union.ModelType.IsRealtime():
			return client.realtimeMistralModels(ctx, apiKey)
		default:
			return client.fetchMistralModels(ctx, apiKey)
		}
	case comdom.ProviderGoogle:
		switch {
		case union.ModelType.IsGeneral():
			return client.generalGoogleModels(ctx, apiKey)
		case union.ModelType.IsRealtime():
			return client.realtimeGoogleModels(ctx, apiKey)
		default:
			return client.fetchGoogleModels(ctx, apiKey)
		}
	default:
		return nil, fmt.Errorf("неподдерживаемый провайдер: %s", union.Provider.String())
	}
}

// FetchMistralVoiceModels returns the specialized models used by the
// server-side realtime voice pipeline. They are auxiliary fields of a
// realtime model response, not independent public ModelType values.
func (c *Client) FetchMistralVoiceModels(ctx context.Context, apiKey string) (stt, tts []string, err error) {
	stt, err = c.sttMistralModels(ctx, apiKey)
	if err != nil {
		return nil, nil, err
	}
	tts, err = c.ttsMistralModels(ctx, apiKey)
	if err != nil {
		return nil, nil, err
	}
	return stt, tts, nil
}

func (c *Client) fetchOpenAIModels(ctx context.Context, apiKey string) ([]string, error) {
	return c.fetchListModels(ctx, mode.OpenAIAgentsURL+"/models", apiKey, comdom.ProviderOpenAI, func(body []byte) ([]string, error) {
		var payload struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("ошибка разбора ответа OpenAI: %w", err)
		}
		result := make([]string, 0, len(payload.Data))
		for _, item := range payload.Data {
			if name := strings.TrimSpace(item.ID); name != "" {
				result = append(result, name)
			}
		}
		return result, nil
	})
}

func (c *Client) generalOpenAIModels(ctx context.Context, apiKey string) ([]string, error) {
	allModels, err := c.fetchOpenAIModels(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения общих моделей OpenIA: %v", err)
	}

	result := make([]string, 0, len(allModels))
	for _, name := range allModels {
		if isGeneralOpenAIModel(name) {
			result = append(result, name)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("получено 0 общих моделей OpenAI")
	}

	return result, nil
}

func (c *Client) realtimeOpenAIModels(ctx context.Context, apiKey string) ([]string, error) {
	allModels, err := c.fetchOpenAIModels(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения моделей OpenAI: %v", err)
	}

	result := make([]string, 0, len(allModels))
	for _, name := range allModels {
		if isRealtimeModel(name) {
			result = append(result, name)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("получено 0 realtime моделей OpenAI")
	}

	return result, nil
}

func (c *Client) generalGoogleModels(ctx context.Context, apiKey string) ([]string, error) {
	allModels, err := c.fetchGoogleModels(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения общих моделей Google: %v", err)
	}

	result := make([]string, 0, len(allModels))
	for _, name := range allModels {
		if isGeneralGoogleModel(name) {
			result = append(result, name)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("получено 0 общих моделей Google")
	}

	return result, nil
}

// isGoogleLiveModelName — в live-каталог попадают только модели, в имени
// которых есть "live" (например, gemini-3.1-flash-live-preview).
func isGoogleLiveModelName(modelName string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(modelName)), "live")
}

// realtimeGoogleModels возвращает live-модели Google для Multimodal Live API:
// только записи с "live" в имени. Если API вернул supportedGenerationMethods,
// дополнительно требуется bidiGenerateContent — иначе Google отклонит сессию
// с close code=1008.
func (c *Client) realtimeGoogleModels(ctx context.Context, apiKey string) ([]string, error) {
	infos, err := c.fetchGoogleModelInfos(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения live-моделей Google: %w", err)
	}

	// Если API не вернул supportedGenerationMethods ни для одной модели,
	// фильтруем только по имени (live).
	hasMethods := false
	for _, info := range infos {
		if len(info.Methods) > 0 {
			hasMethods = true
			break
		}
	}

	result := make([]string, 0, len(infos))
	for _, info := range infos {
		if !isGoogleLiveModelName(info.Name) {
			continue
		}
		if hasMethods {
			supportsBidi := false
			for _, method := range info.Methods {
				if method == "bidiGenerateContent" {
					supportsBidi = true
					break
				}
			}
			if !supportsBidi {
				continue
			}
		}
		result = append(result, info.Name)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("получено 0 live-моделей Google (live + bidiGenerateContent)")
	}
	return result, nil
}

func isRealtimeModel(modelName string) bool {
	if strings.HasPrefix(modelName, "-realtime-") ||
		strings.HasPrefix(modelName, "-tts-") {
		return true
	}
	return strings.Contains(modelName, "realtime")
}

// isMistralRealtimeLLMModel selects only the realtime language models that
// belong in realtime_models. Mistral exposes STT and TTS models from the same
// /models endpoint, but they have different roles and must not be synced into
// the realtime-model table.
func isMistralRealtimeLLMModel(modelName string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(lower, "realtime") &&
		!strings.Contains(lower, "transcribe") &&
		!strings.Contains(lower, "-tts")
}

func isGeneralOpenAIModel(modelName string) bool {
	// исключаем realtime, tts, transcribe, embedding, moderation
	exclude := []string{"realtime", "tts", "transcribe", "embedding", "moderation", "audio"}
	for _, bad := range exclude {
		if strings.Contains(modelName, bad) {
			return false
		}
	}
	return true
}

// isGeneralGoogleModel оставляет только текстовые/чат-модели Gemini.
// Google models.list отдаёт десятки моделей (image, video, music, TTS,
// deep-research, robotics, omni, embeddings и т.п.) — в выборе языковой
// модели они не нужны.
func isGeneralGoogleModel(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "" || !isGeneralOpenAIModel(name) {
		return false
	}
	if !strings.HasPrefix(name, "gemini-") {
		return false
	}

	exclude := []string{
		"live",
		"image",
		"imagen",
		"veo",
		"embedding",
		"tts",
		"audio",
		"omni",
		"robotics",
		"computer-use",
		"deep-research",
		"antigravity",
		"nano-banana",
		"lyria",
		"gemma",
		"aqa",
	}
	for _, bad := range exclude {
		if strings.Contains(name, bad) {
			return false
		}
	}
	return true
}

func (c *Client) fetchMistralModels(ctx context.Context, apiKey string) ([]string, error) {
	return c.fetchListModels(ctx, mode.MistralBaseURL+"/models", apiKey, comdom.ProviderMistral, func(body []byte) ([]string, error) {
		var payload struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("ошибка разбора ответа Mistral: %w", err)
		}
		result := make([]string, 0, len(payload.Data))
		for _, item := range payload.Data {
			if name := strings.TrimSpace(item.ID); name != "" {
				result = append(result, name)
			}
		}
		return result, nil
	})
}

func (c *Client) generalMistralModels(ctx context.Context, apiKey string) ([]string, error) {
	allModels, err := c.fetchMistralModels(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения общих моделей Mistral: %v", err)
	}

	result := make([]string, 0, len(allModels))
	for _, name := range allModels {
		if isGeneralMistralModel(name) {
			result = append(result, name)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("получено 0 общих моделей Mistral")
	}
	return result, nil
}

func isGeneralMistralModel(modelName string) bool {
	exclude := []string{"embed", "moderation", "ocr", "realtime", "transcribe", "voxtral"}
	for _, bad := range exclude {
		if strings.Contains(modelName, bad) {
			return false
		}
	}
	return true
}

func (c *Client) realtimeMistralModels(ctx context.Context, apiKey string) ([]string, error) {
	allModels, err := c.fetchMistralModels(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения моделей Mistral: %v", err)
	}

	result := make([]string, 0, len(allModels))
	for _, name := range allModels {
		if isMistralRealtimeLLMModel(name) {
			result = append(result, name)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("получено 0 realtime моделей Mistral")
	}

	return result, nil
}

func filterMistralModels(models []string, predicate func(string) bool) []string {
	result := make([]string, 0, len(models))
	for _, name := range models {
		if predicate(name) {
			result = append(result, name)
		}
	}
	return result
}

func isMistralSTTModel(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "transcribe-realtime")
}

func isMistralTTSModel(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "-tts")
}

func (c *Client) sttMistralModels(ctx context.Context, apiKey string) ([]string, error) {
	allModels, err := c.fetchMistralModels(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения STT моделей Mistral: %w", err)
	}
	result := filterMistralModels(allModels, isMistralSTTModel)
	if len(result) == 0 {
		return nil, fmt.Errorf("получено 0 STT моделей Mistral")
	}
	return result, nil
}

func (c *Client) ttsMistralModels(ctx context.Context, apiKey string) ([]string, error) {
	allModels, err := c.fetchMistralModels(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения TTS моделей Mistral: %w", err)
	}
	result := filterMistralModels(allModels, isMistralTTSModel)
	if len(result) == 0 {
		return nil, fmt.Errorf("получено 0 TTS моделей Mistral")
	}
	return result, nil
}

// googleModelInfo — модель Google вместе со списком поддерживаемых методов
// генерации (supportedGenerationMethods), например "bidiGenerateContent".
type googleModelInfo struct {
	Name    string
	Methods []string
}

func (c *Client) googleBaseURL() string {
	if c != nil && c.GoogleBaseURL != "" {
		return c.GoogleBaseURL
	}
	return mode.GoogleAgentsURL
}

// fetchGoogleModelInfos получает полный список моделей Google с пагинацией.
// Google API expects the API key as a query parameter, not as a Bearer token.
func (c *Client) fetchGoogleModelInfos(ctx context.Context, apiKey string) ([]googleModelInfo, error) {
	base := strings.TrimRight(c.googleBaseURL(), "/")
	pageToken := ""
	var result []googleModelInfo

	for {
		u, err := url.Parse(base + "/models")
		if err != nil {
			return nil, fmt.Errorf("ошибка формирования URL Google: %w", err)
		}
		q := u.Query()
		q.Set("key", apiKey)
		q.Set("pageSize", "1000")
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		u.RawQuery = q.Encode()

		body, err := c.fetchBody(ctx, u.String(), comdom.ProviderGoogle)
		if err != nil {
			return nil, err
		}

		var payload struct {
			Models []struct {
				Name                       string   `json:"name"`
				SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("ошибка разбора ответа Google: %w", err)
		}

		for _, item := range payload.Models {
			name := strings.TrimSpace(strings.TrimPrefix(item.Name, "models/"))
			if name == "" {
				continue
			}
			result = append(result, googleModelInfo{Name: name, Methods: item.SupportedGenerationMethods})
		}

		if payload.NextPageToken == "" {
			break
		}
		pageToken = payload.NextPageToken
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("Google вернул пустой список моделей")
	}
	return result, nil
}

func (c *Client) fetchGoogleModels(ctx context.Context, apiKey string) ([]string, error) {
	infos, err := c.fetchGoogleModelInfos(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(infos))
	for _, info := range infos {
		result = append(result, info.Name)
	}
	return result, nil
}

func (c *Client) fetchListModels(ctx context.Context, url, apiKey string, provider comdom.ProviderType, parser func([]byte) ([]string, error)) ([]string, error) {
	body, err := c.fetchBody(ctx, url, provider, apiKey)
	if err != nil {
		return nil, err
	}
	return parser(body)
}

// fetchBody выполняет GET-запрос к provider API и возвращает тело ответа.
// apiKey передаётся через Bearer-заголовок; пустое значение допустимо.
func (c *Client) fetchBody(ctx context.Context, url string, provider comdom.ProviderType, apiKeys ...string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for _, apiKey := range apiKeys {
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			break
		}
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, comerrors.NewProviderError(provider, resp.StatusCode, strings.TrimSpace(string(body)), nil)
	}

	return io.ReadAll(resp.Body)
}
