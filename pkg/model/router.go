package model

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ikermy/air-common/pkg/com"
	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/elevenlabs"
	"github.com/ikermy/air-common/pkg/mode"
	"github.com/ikermy/air-common/pkg/model/create"
	"github.com/ikermy/air-common/pkg/model/provider_catalog"
)

// ============================================================================
// MODEL ROUTER
// ============================================================================

// Router маршрутизирует запросы к разным провайдерам моделей
type Router struct {
	openai        Inter
	mistral       Inter
	google        Inter
	modelsManager *create.UniversalModel
	ctx           context.Context
	db            DB
	dialogSaver   DialogSaver
	catalog       catalogThrottle

	cascadeOnce     sync.Once
	cascadeProvider *cascadeProvider
}

// catalogTTL — период, в течение которого каталог моделей считается свежим и
// повторный запрос к провайдеру не выполняется (in-memory, per-instance).
const catalogTTL = 24 * time.Hour

// catalogThrottle — простой in-memory throttle без внешних зависимостей.
type catalogThrottle struct {
	mu   sync.Mutex
	last map[string]time.Time
	busy map[string]bool
}

// claim возвращает true, если вызывающий должен выполнить обновление
// (каталог устарел и никто другой его сейчас не обновляет).
func (t *catalogThrottle) claim(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ts, ok := t.last[key]; ok && time.Since(ts) < catalogTTL {
		return false
	}
	if t.busy[key] {
		return false
	}
	if t.busy == nil {
		t.busy = map[string]bool{}
	}
	t.busy[key] = true
	return true
}

// release снимает флаг обновления; success=true фиксирует свежесть каталога.
func (t *catalogThrottle) release(key string, success bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.busy, key)
	if success {
		if t.last == nil {
			t.last = map[string]time.Time{}
		}
		t.last[key] = time.Now()
	}
}

// fresh сообщает, что каталог по ключу обновлялся менее catalogTTL назад.
func (t *catalogThrottle) fresh(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	ts, ok := t.last[key]
	return ok && time.Since(ts) < catalogTTL
}

// mark фиксирует успешное обновление каталога по ключу.
func (t *catalogThrottle) mark(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		t.last = map[string]time.Time{}
	}
	t.last[key] = time.Now()
}

// DialogSaver принимает сообщения для пакетного сохранения диалогов.
// endpoint.Endpoint реализует этот интерфейс без зависимости model от endpoint.
type DialogSaver interface {
	SaveDialog(creator comdom.CreatorType, dialogID uint64, resp *AssistResponse)
}

func (r *Router) DialogSaver() DialogSaver {
	if r == nil {
		return nil
	}
	return r.dialogSaver
}

// RouterOption определяет опцию для настройки Router
type RouterOption func(*Router, context.Context, DB) error

func (r *Router) mistralVoiceManager() (MistralManager, error) {
	if r == nil || r.mistral == nil {
		return nil, fmt.Errorf("Mistral провайдер не инициализирован")
	}
	manager, ok := r.mistral.(MistralManager)
	if !ok {
		return nil, fmt.Errorf("Mistral провайдер не поддерживает voice profiles")
	}
	return manager, nil
}

func (r *Router) CreateMistralVoice(userID uint32, request comdom.CreateVoiceRequest) (comdom.Voice, error) {
	m, err := r.mistralVoiceManager()
	if err != nil {
		return comdom.Voice{}, err
	}
	return m.CreateVoice(userID, request)
}
func (r *Router) ListMistralVoices(userID uint32, limit, offset int, voiceType string) (comdom.VoiceList, error) {
	m, err := r.mistralVoiceManager()
	if err != nil {
		return comdom.VoiceList{}, err
	}
	return m.ListVoices(userID, limit, offset, voiceType)
}
func (r *Router) GetMistralVoice(userID uint32, voiceID string) (comdom.Voice, error) {
	m, err := r.mistralVoiceManager()
	if err != nil {
		return comdom.Voice{}, err
	}
	return m.GetVoice(userID, voiceID)
}
func (r *Router) UpdateMistralVoice(userID uint32, voiceID string, request comdom.UpdateVoiceRequest) (comdom.Voice, error) {
	m, err := r.mistralVoiceManager()
	if err != nil {
		return comdom.Voice{}, err
	}
	return m.UpdateVoice(userID, voiceID, request)
}
func (r *Router) DeleteMistralVoice(userID uint32, voiceID string) (comdom.Voice, error) {
	m, err := r.mistralVoiceManager()
	if err != nil {
		return comdom.Voice{}, err
	}
	return m.DeleteVoice(userID, voiceID)
}
func (r *Router) GetMistralVoiceSample(userID uint32, voiceID string) (io.ReadCloser, string, error) {
	m, err := r.mistralVoiceManager()
	if err != nil {
		return nil, "", err
	}
	return m.GetVoiceSample(userID, voiceID)
}

// ─── Generic voice CRUD (провайдер-агностичный) ─────────────────────────────
// Mistral обслуживается своим менеджером, ElevenLabs — leaf-клиентом.

func (r *Router) elevenLabsVoiceClient(userID uint32) (*elevenlabs.Client, error) {
	if r.db == nil {
		return nil, fmt.Errorf("БД не инициализирована")
	}
	apiKey, err := r.db.GetUserAPIKey(userID, comdom.ProviderElevenLabs)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("API-ключ ElevenLabs не настроен")
	}
	return elevenlabs.NewClient(apiKey), nil
}

// ListVoices возвращает голоса выбранного voice-backend.
func (r *Router) ListVoices(userID uint32, backend comdom.ProviderType, limit, offset int, voiceType string) (comdom.VoiceList, error) {
	switch backend {
	case comdom.ProviderMistral:
		return r.ListMistralVoices(userID, limit, offset, voiceType)
	case comdom.ProviderElevenLabs:
		client, err := r.elevenLabsVoiceClient(userID)
		if err != nil {
			return comdom.VoiceList{}, err
		}
		voices, err := client.ListVoices(r.ctx, limit, offset)
		if err != nil {
			return comdom.VoiceList{}, err
		}
		items := make([]comdom.Voice, 0, len(voices))
		for _, v := range voices {
			items = append(items, mapElevenLabsVoice(v))
		}
		page := 1
		if limit > 0 {
			page = offset/limit + 1
		}
		return comdom.VoiceList{Items: items, Page: page, PageSize: limit, Total: len(items), TotalPages: 1}, nil
	default:
		return comdom.VoiceList{}, fmt.Errorf("провайдер %s не поддерживает голоса", backend)
	}
}

// GetVoice возвращает голос по ID.
func (r *Router) GetVoice(userID uint32, backend comdom.ProviderType, voiceID string) (comdom.Voice, error) {
	switch backend {
	case comdom.ProviderMistral:
		return r.GetMistralVoice(userID, voiceID)
	case comdom.ProviderElevenLabs:
		client, err := r.elevenLabsVoiceClient(userID)
		if err != nil {
			return comdom.Voice{}, err
		}
		voice, err := client.GetVoice(r.ctx, voiceID)
		if err != nil {
			return comdom.Voice{}, err
		}
		return mapElevenLabsVoice(voice), nil
	default:
		return comdom.Voice{}, fmt.Errorf("провайдер %s не поддерживает голоса", backend)
	}
}

// CreateVoice создаёт клон голоса. Поддерживает два режима ElevenLabs:
// instant (IVC, голос готов сразу) и professional (PVC, асинхронное обучение).
func (r *Router) CreateVoice(userID uint32, backend comdom.ProviderType, request comdom.CreateVoiceRequest) (comdom.Voice, error) {
	switch backend {
	case comdom.ProviderMistral:
		return r.CreateMistralVoice(userID, request)
	case comdom.ProviderElevenLabs:
		client, err := r.elevenLabsVoiceClient(userID)
		if err != nil {
			return comdom.Voice{}, err
		}
		samples, err := decodeVoiceSamples(request)
		if err != nil {
			return comdom.Voice{}, err
		}
		description := ""
		if request.Description != nil {
			description = *request.Description
		}

		mode := comdom.CloneMode(strings.TrimSpace(request.CloneMode))
		if mode == "" {
			mode = comdom.CloneModeInstant
		}
		if !mode.IsValid() {
			return comdom.Voice{}, fmt.Errorf("неизвестный режим клонирования: %s", mode)
		}

		if mode == comdom.CloneModeProfessional {
			language := strings.TrimSpace(request.Language)
			if language == "" {
				return comdom.Voice{}, fmt.Errorf("language обязателен для профессионального клонирования (PVC)")
			}
			voiceID, err := client.CreatePVCVoice(r.ctx, request.Name, language, description)
			if err != nil {
				return comdom.Voice{}, err
			}
			if err := client.AddPVCSamples(r.ctx, voiceID, samples); err != nil {
				// голос-контейнер уже создан; возвращаем ошибку, чтобы клиент повторил обучение
				return comdom.Voice{}, fmt.Errorf("PVC voice %s создан, но образцы не загружены: %w", voiceID, err)
			}
			if err := client.TrainPVCVoice(r.ctx, voiceID, request.ModelID); err != nil {
				return comdom.Voice{}, fmt.Errorf("PVC voice %s создан, но обучение не запущено: %w", voiceID, err)
			}
			return comdom.Voice{
				ID:              voiceID,
				Name:            request.Name,
				CloneMode:       string(comdom.CloneModeProfessional),
				FineTuningState: "queued",
			}, nil
		}

		// instant (IVC)
		voiceID, err := client.CreateVoice(r.ctx, request.Name, description, samples)
		if err != nil {
			return comdom.Voice{}, err
		}
		return comdom.Voice{ID: voiceID, Name: request.Name, CloneMode: string(comdom.CloneModeInstant)}, nil
	default:
		return comdom.Voice{}, fmt.Errorf("провайдер %s не поддерживает клонирование голоса", backend)
	}
}

// decodeVoiceSamples принимает образцы из Samples (несколько base64) или
// из устаревшего SampleAudio (один base64).
func decodeVoiceSamples(request comdom.CreateVoiceRequest) ([]elevenlabs.VoiceSample, error) {
	fileName := "sample.wav"
	if request.SampleFilename != nil && strings.TrimSpace(*request.SampleFilename) != "" {
		fileName = *request.SampleFilename
	}

	raw := request.Samples
	if len(raw) == 0 && strings.TrimSpace(request.SampleAudio) != "" {
		raw = []string{request.SampleAudio}
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("нужен хотя бы один аудио-образец")
	}

	samples := make([]elevenlabs.VoiceSample, 0, len(raw))
	for i, encoded := range raw {
		audio, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(audio) == 0 {
			return nil, fmt.Errorf("некорректный аудио-образец голоса #%d", i+1)
		}
		name := fileName
		if len(raw) > 1 {
			name = fmt.Sprintf("sample_%d.wav", i+1)
		}
		samples = append(samples, elevenlabs.VoiceSample{FileName: name, Data: audio})
	}
	return samples, nil
}

// UpdateVoice обновляет голос.
func (r *Router) UpdateVoice(userID uint32, backend comdom.ProviderType, voiceID string, request comdom.UpdateVoiceRequest) (comdom.Voice, error) {
	switch backend {
	case comdom.ProviderMistral:
		return r.UpdateMistralVoice(userID, voiceID, request)
	case comdom.ProviderElevenLabs:
		client, err := r.elevenLabsVoiceClient(userID)
		if err != nil {
			return comdom.Voice{}, err
		}
		var name, description string
		if request.Name != nil {
			name = *request.Name
		}
		if request.Description != nil {
			description = *request.Description
		}
		if err := client.EditVoice(r.ctx, voiceID, name, description); err != nil {
			return comdom.Voice{}, err
		}
		return r.GetVoice(userID, backend, voiceID)
	default:
		return comdom.Voice{}, fmt.Errorf("провайдер %s не поддерживает голоса", backend)
	}
}

// DeleteVoice удаляет голос.
func (r *Router) DeleteVoice(userID uint32, backend comdom.ProviderType, voiceID string) (comdom.Voice, error) {
	switch backend {
	case comdom.ProviderMistral:
		return r.DeleteMistralVoice(userID, voiceID)
	case comdom.ProviderElevenLabs:
		client, err := r.elevenLabsVoiceClient(userID)
		if err != nil {
			return comdom.Voice{}, err
		}
		voice, getErr := client.GetVoice(r.ctx, voiceID)
		if err := client.DeleteVoice(r.ctx, voiceID); err != nil {
			return comdom.Voice{}, err
		}
		if getErr != nil {
			return comdom.Voice{ID: voiceID}, nil
		}
		return mapElevenLabsVoice(voice), nil
	default:
		return comdom.Voice{}, fmt.Errorf("провайдер %s не поддерживает голоса", backend)
	}
}

// GetVoiceSample возвращает аудио-превью голоса.
func (r *Router) GetVoiceSample(userID uint32, backend comdom.ProviderType, voiceID string) (io.ReadCloser, string, error) {
	switch backend {
	case comdom.ProviderMistral:
		return r.GetMistralVoiceSample(userID, voiceID)
	case comdom.ProviderElevenLabs:
		client, err := r.elevenLabsVoiceClient(userID)
		if err != nil {
			return nil, "", err
		}
		return client.GetVoiceSample(r.ctx, voiceID)
	default:
		return nil, "", fmt.Errorf("провайдер %s не поддерживает голоса", backend)
	}
}

func mapElevenLabsVoice(v elevenlabs.Voice) comdom.Voice {
	out := comdom.Voice{ID: v.VoiceID, Name: v.Name}
	if v.Description != "" {
		desc := v.Description
		out.Description = &desc
	}
	if v.Category != "" {
		out.Tags = []string{v.Category}
	}
	if lang, ok := v.Labels["language"]; ok && lang != "" {
		out.Languages = strings.Split(lang, ",")
	}
	if v.FineTuning != nil {
		out.FineTuningState = v.FineTuning.State
		out.FineTuningProgress = v.FineTuning.Progress
		if v.FineTuning.State != "" && v.FineTuning.State != "fine_tuned" {
			out.CloneMode = string(comdom.CloneModeProfessional)
		}
	}
	return out
}

// ─── Voice gateway: TTS/STT поверх выбранного backend'а ──────────────────────

// activeVoiceConfig возвращает Voice-конфигурацию активной модели пользователя.
func (r *Router) activeVoiceConfig(userID uint32) *comdom.VoiceConfig {
	if r.modelsManager == nil {
		return nil
	}
	data, err := r.modelsManager.GetActiveUserModel(userID)
	if err != nil || data == nil {
		return nil
	}
	return data.Voice
}

// SynthesizeSpeech синтезирует речь выбранным backend'ом (batch).
func (r *Router) SynthesizeSpeech(userID uint32, backend comdom.ProviderType, req elevenlabs.SynthesizeRequest) (io.ReadCloser, string, error) {
	switch backend {
	case comdom.ProviderElevenLabs:
		client, err := r.elevenLabsVoiceClient(userID)
		if err != nil {
			return nil, "", err
		}
		return client.Synthesize(r.ctx, req)
	default:
		return nil, "", fmt.Errorf("backend %s не поддерживает TTS", backend)
	}
}

// SynthesizeSpeechStream синтезирует речь выбранным backend'ом (streaming).
func (r *Router) SynthesizeSpeechStream(userID uint32, backend comdom.ProviderType, req elevenlabs.SynthesizeRequest) (<-chan []byte, error) {
	switch backend {
	case comdom.ProviderElevenLabs:
		client, err := r.elevenLabsVoiceClient(userID)
		if err != nil {
			return nil, err
		}
		return client.SynthesizeStream(r.ctx, req)
	default:
		return nil, fmt.Errorf("backend %s не поддерживает TTS", backend)
	}
}

// TranscribeAudioWithBackend распознаёт аудио выбранным backend'ом.
func (r *Router) TranscribeAudioWithBackend(userID uint32, backend comdom.ProviderType, req elevenlabs.TranscribeRequest) (string, error) {
	switch backend {
	case comdom.ProviderElevenLabs:
		client, err := r.elevenLabsVoiceClient(userID)
		if err != nil {
			return "", err
		}
		return client.Transcribe(r.ctx, req)
	default:
		manager, err := r.GetActiveUserManager(userID)
		if err != nil {
			return "", err
		}
		return manager.TranscribeAudio(userID, req.Audio, req.FileName)
	}
}

// GenerateMusic генерирует музыку. Параметры, не заданные в req, берутся из
// Voice.Music активной модели пользователя (model по умолчанию eleven_music).
func (r *Router) GenerateMusic(userID uint32, req elevenlabs.MusicRequest) (elevenlabs.MusicResult, error) {
	client, err := r.elevenLabsVoiceClient(userID)
	if err != nil {
		return elevenlabs.MusicResult{}, err
	}
	voiceCfg := r.activeVoiceConfig(userID)
	if strings.TrimSpace(req.Model) == "" {
		if voiceCfg != nil {
			req.Model = voiceCfg.MusicModelName()
		} else {
			req.Model = comdom.DefaultVoiceModel(comdom.VoiceKindMusic)
		}
	}
	if voiceCfg != nil && voiceCfg.Music != nil {
		if req.LengthMs == nil {
			req.LengthMs = voiceCfg.Music.LengthMs
		}
		if req.Format == "" && voiceCfg.Music.Format != nil {
			req.Format = *voiceCfg.Music.Format
		}
		if req.ForceInstrumental == nil {
			req.ForceInstrumental = voiceCfg.Music.ForceInstrumental
		}
	}
	return client.GenerateMusic(r.ctx, req)
}

// NewModelRouter создаёт новый маршрутизатор с опциями.
//
//	router, err := model.NewModelRouter(ctx, conf, db,
//	    model.WithMasterKeyProvider(orcClient), // должна идти первой, если используется
//	    openai.NewAsRouterOption(),
//	    mistral.NewAsRouterOption())
func NewModelRouter(ctx context.Context, db DB, options ...RouterOption) *Router {
	router := &Router{
		ctx: ctx,
		db:  db,
	}

	// Применяем опции ПЕРЕД созданием modelsManager, чтобы WithMasterKeyProvider
	// успел обернуть router.db до того, как его используют провайдеры и modelsManager.
	// Каждая опция получает актуальный router.db (возможно уже обёрнутый предыдущей опцией).
	for _, option := range options {
		if err := option(router, ctx, router.db); err != nil {
			log.Fatalf("ошибка применения опции: %v", err)
		}
	}

	if managerDB, ok := router.db.(create.DB); ok {
		router.modelsManager = create.New(ctx, managerDB)
	} else {
		log.Fatalf("DB не реализует comdom.DB, невозможна инициализация ModelRouter")
	}

	if router.google != nil {
		if googleModel, ok := router.google.(interface{ SetUniversalModel(*create.UniversalModel) }); ok {
			if router.modelsManager == nil {
				log.Fatal("КРИТИЧЕСКАЯ ОШИБКА: modelsManager == nil, не можем установить UniversalModel!")
			}
			googleModel.SetUniversalModel(router.modelsManager)
		} else {
			log.Fatal("КРИТИЧЕСКАЯ ОШИБКА: Google модель не реализует метод SetUniversalModel!")
		}
	}

	if router.openai == nil && router.mistral == nil && router.google == nil {
		log.Fatal("не инициализирован ни один провайдер моделей " +
			"(используйте openai.NewAsRouterOption(), mistral.NewAsRouterOption() или google.NewAsRouterOption())")
	}

	return router
}

// WithOpenAIModel добавляет реализацию OpenAI модели
func WithOpenAIModel(model Inter) RouterOption {
	return func(r *Router, _ context.Context, _ DB) error {
		if model == nil {
			return fmt.Errorf("OpenAI модель не может быть nil")
		}
		r.openai = model
		return nil
	}
}

// WithMistralModel добавляет реализацию Mistral модели
func WithMistralModel(model Inter) RouterOption {
	return func(r *Router, _ context.Context, _ DB) error {
		if model == nil {
			return fmt.Errorf("Mistral модель не может быть nil")
		}
		r.mistral = model
		return nil
	}
}

// WithDialogSaver подключает общий batch-writer диалогов, обычно endpoint.Endpoint.
func WithDialogSaver(saver DialogSaver) RouterOption {
	return func(r *Router, _ context.Context, _ DB) error {
		if saver == nil {
			return fmt.Errorf("DialogSaver не может быть nil")
		}
		r.dialogSaver = saver
		return nil
	}
}

// WithMasterKeyProvider подключает Landing-сервис для расшифровки API-ключей,
// зашифрованных MasterKey пользователя ($mk$ префикс).
//
// ВАЖНО: передавайте эту опцию ПЕРВОЙ в NewModelRouter — она оборачивает DB,
// и все последующие опции (провайдеры) автоматически получат обёрнутую версию.
//
//	router := model.NewModelRouter(ctx, db,
//	    model.WithMasterKeyProvider(orcClient),
//	    openai.NewAsRouterOption(),
//	    ...)
//
// rpc.Client из пакета orc удовлетворяет интерфейсу MasterKeyProvider без изменений.
// Если пользователь запрашивает $mk$-зашифрованный ключ, а Landing недоступен —
// пользователю автоматически отправляется уведомление "reauth-userkey" и
// вызывающий сервис получает ErrMasterKeyUnavailable.
func WithMasterKeyProvider(mkProvider MasterKeyProvider) RouterOption {
	return func(r *Router, ctx context.Context, db DB) error {
		r.db = WrapDBWithMasterKeyDecryption(ctx, db, mkProvider)
		return nil
	}
}

// WithGoogleModel добавляет реализацию Google модели
func WithGoogleModel(model Inter) RouterOption {
	return func(r *Router, _ context.Context, _ DB) error {
		if model == nil {
			return fmt.Errorf("Google модель не может быть nil")
		}
		r.google = model
		return nil
	}
}

// HasOpenAI проверяет, инициализирован ли провайдер OpenAI
func (r *Router) HasOpenAI() bool { return r.openai != nil }

// HasMistral проверяет, инициализирован ли провайдер Mistral
func (r *Router) HasMistral() bool { return r.mistral != nil }

// HasGoogle проверяет, инициализирован ли провайдер Google
func (r *Router) HasGoogle() bool { return r.google != nil }

// GetAvailableProviders возвращает список доступных провайдеров
func (r *Router) GetAvailableProviders() []string {
	providers := make([]string, 0, 3)
	if r.openai != nil {
		providers = append(providers, "OpenAI")
	}
	if r.mistral != nil {
		providers = append(providers, "Mistral")
	}
	if r.google != nil {
		providers = append(providers, "Google")
	}
	return providers
}

// forEachProvider вызывает fn для каждого инициализированного провайдера.
// Порядок: OpenAI → Mistral → Google.
func (r *Router) forEachProvider(fn func(Inter)) {
	for _, p := range []Inter{r.openai, r.mistral, r.google} {
		if p != nil {
			fn(p)
		}
	}
}

// getModel возвращает модель по типу провайдера
func (r *Router) getModel(provider comdom.ProviderType) (Inter, error) {
	switch provider {
	case comdom.ProviderOpenAI:
		if r.openai == nil {
			return nil, fmt.Errorf("модель OpenAI не инициализирована")
		}
		return r.openai, nil
	case comdom.ProviderMistral:
		if r.mistral == nil {
			return nil, fmt.Errorf("модель Mistral не инициализирована")
		}
		return r.mistral, nil
	case comdom.ProviderGoogle:
		if r.google == nil {
			return nil, fmt.Errorf("модель Google не инициализирована")
		}
		return r.google, nil
	default:
		return nil, fmt.Errorf("неизвестный провайдер: %v", provider)
	}
}

// GetProviderModel возвращает модель конкретного провайдера (для тестирования)
func (r *Router) GetProviderModel(provider comdom.ProviderType) any {
	switch provider {
	case comdom.ProviderOpenAI:
		return r.openai
	case comdom.ProviderMistral:
		return r.mistral
	case comdom.ProviderGoogle:
		return r.google
	default:
		return nil
	}
}

// ============================================================================
// ДЕЛЕГИРУЮЩИЕ МЕТОДЫ
// ============================================================================

// NewMessage делегирует к первому доступному провайдеру
func (r *Router) NewMessage(operator Operator, msgType string, content *AssistResponse, name *string, files ...FileUpload) Message {
	if r.openai != nil {
		return r.openai.NewMessage(operator, msgType, content, name, files...)
	}
	if r.mistral != nil {
		return r.mistral.NewMessage(operator, msgType, content, name, files...)
	}
	if r.google != nil {
		return r.google.NewMessage(operator, msgType, content, name, files...)
	}
	// Fallback — только если ни один провайдер не инициализирован
	return Message{
		Operator:  operator,
		Type:      msgType,
		Content:   *content,
		Name:      *name,
		Timestamp: time.Now(),
		Files:     files,
	}
}

// GetFileAsReader делегирует к активному провайдеру пользователя
func (r *Router) GetFileAsReader(userID uint32, url string) (io.Reader, error) {
	manager, err := r.GetActiveUserManager(userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения активного менеджера для UserID %d: %w", userID, err)
	}
	return manager.GetFileAsReader(userID, url)
}

// GetOrSetRespGPT делегирует к модели на основе Provider из Assistant
func (r *Router) GetOrSetRespGPT(assist Assistant, dialogID, respId uint64, respName string) (*RespModel, error) {
	if assist.Provider == 0 {
		return nil, fmt.Errorf("провайдер не установлен для UserID=%d: у пользователя не создана модель ассистента. "+
			"Создайте модель через API или панель управления", assist.UserID)
	}
	m, err := r.getModel(assist.Provider)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить модель для провайдера %s (UserID=%d): %w",
			assist.Provider, assist.UserID, err)
	}
	return m.GetOrSetRespGPT(assist, dialogID, respId, respName)
}

// GetCh ищет канал по respId во всех провайдерах
func (r *Router) GetCh(respId uint64) (*Ch, error) {
	for _, p := range []Inter{r.openai, r.mistral, r.google} {
		if p == nil {
			continue
		}
		if ch, err := p.GetCh(respId); err == nil {
			return ch, nil
		}
	}
	return nil, fmt.Errorf("канал не найден для respId %d", respId)
}

// GetRespIdBydialogID ищет respId по dialogID во всех провайдерах
func (r *Router) GetRespIdByDialogID(dialogID uint64) (uint64, error) {
	for _, p := range []Inter{r.openai, r.mistral, r.google} {
		if p == nil {
			continue
		}
		if id, err := p.GetRespIdByDialogID(dialogID); err == nil {
			return id, nil
		}
	}
	return 0, fmt.Errorf("RespId не найден для DialogID %d", dialogID)
}

// SaveAllContextDuringExit сохраняет контексты всех провайдеров
func (r *Router) SaveAllContextDuringExit() {
	r.forEachProvider(func(p Inter) { p.SaveAllContextDuringExit() })
}

// Request направляет запрос к провайдеру, которому принадлежит диалог
func (r *Router) Request(userID uint32, dialogID uint64, text string, files ...FileUpload) (AssistResponse, error) {
	for _, p := range []Inter{r.openai, r.mistral, r.google} {
		if p == nil {
			continue
		}
		if _, err := p.GetRespIdByDialogID(dialogID); err == nil {
			return p.Request(userID, dialogID, text, files...)
		}
	}
	return AssistResponse{}, fmt.Errorf("модель не найдена для DialogID %d", dialogID)
}

// tryProviderStreaming пытается выполнить streaming запрос к провайдеру.
// Возвращает (true, err) если провайдер найден, (false, nil) если нет.
func (r *Router) tryProviderStreaming(provider Inter, userID uint32, dialogID uint64, text string,
	onDelta func(delta string, done bool) error, files ...FileUpload) (bool, error) {
	if provider == nil {
		return false, nil
	}
	if _, err := provider.GetRespIdByDialogID(dialogID); err != nil {
		return false, nil
	}
	if streamer, ok := provider.(interface {
		RequestStreaming(userID uint32, dialogID uint64, text string,
			onDelta func(delta string, done bool) error, files ...FileUpload) error
	}); ok {
		return true, streamer.RequestStreaming(userID, dialogID, text, onDelta, files...)
	}
	// Fallback: буферизуем через Request
	response, err := provider.Request(userID, dialogID, text, files...)
	if err != nil {
		return true, err
	}
	jsonData, _ := json.Marshal(response)
	if onDelta != nil {
		if err := onDelta(string(jsonData), true); err != nil {
			return true, err
		}
	}
	return true, nil
}

// RequestStreaming направляет streaming запрос к провайдеру диалога
func (r *Router) RequestStreaming(userID uint32, dialogID uint64, text string,
	onDelta func(delta string, done bool) error, files ...FileUpload) error {
	for _, p := range []Inter{r.openai, r.mistral, r.google} {
		if found, err := r.tryProviderStreaming(p, userID, dialogID, text, onDelta, files...); found {
			return err
		}
	}
	return fmt.Errorf("модель не найдена для DialogID %d", dialogID)
}

// CleanDialogData очищает данные диалога у всех провайдеров
func (r *Router) CleanDialogData(dialogID uint64) {
	r.forEachProvider(func(p Inter) { p.CleanDialogData(dialogID) })
}

// GetActiveUserModel получает активную модель пользователя
func (r *Router) GetActiveUserModel(userID uint32) (*comdom.UniversalModelData, error) {
	if r.modelsManager == nil {
		return nil, fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.GetActiveUserModel(userID)
}

// GetActiveUserManager возвращает менеджера активного провайдера пользователя.
// Использует comma-ok form для безопасного type assertion.
func (r *Router) GetActiveUserManager(userID uint32) (Inter, error) {
	provider, err := r.db.GetActiveProvider(userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения активного провайдера для UserID %d: %w", userID, err)
	}

	switch provider {
	case comdom.ProviderOpenAI:
		if r.openai == nil {
			return nil, fmt.Errorf("OpenAI провайдер не инициализирован")
		}
		manager, ok := r.openai.(OpenAIManager)
		if !ok {
			return nil, fmt.Errorf("OpenAI провайдер не реализует OpenAIManager")
		}
		return manager, nil

	case comdom.ProviderMistral:
		if r.mistral == nil {
			return nil, fmt.Errorf("Mistral провайдер не инициализирован")
		}
		manager, ok := r.mistral.(MistralManager)
		if !ok {
			return nil, fmt.Errorf("Mistral провайдер не реализует MistralManager")
		}
		return manager, nil

	case comdom.ProviderGoogle:
		if r.google == nil {
			return nil, fmt.Errorf("Google провайдер не инициализирован")
		}
		manager, ok := r.google.(GoogleManager)
		if !ok {
			return nil, fmt.Errorf("Google провайдер не реализует GoogleManager")
		}
		return manager, nil

	default:
		return nil, fmt.Errorf("неизвестный провайдер: %s", provider)
	}
}

// TranscribeAudio транскрибирует аудио. Если в конфигурации активной модели
// выбран ElevenLabs STT — используется он, иначе активный провайдер.
func (r *Router) TranscribeAudio(userID uint32, audioData []byte, fileName string) (string, error) {
	if voiceCfg := r.activeVoiceConfig(userID); voiceCfg != nil &&
		voiceCfg.STTBackend != nil && *voiceCfg.STTBackend == comdom.ProviderElevenLabs {
		req := elevenlabs.TranscribeRequest{FileName: fileName, Audio: audioData}
		if voiceCfg.STT != nil {
			if voiceCfg.STT.Model != nil {
				req.Model = *voiceCfg.STT.Model
			}
			if voiceCfg.STT.Language != nil {
				req.Language = *voiceCfg.STT.Language
			}
			req.Keyterms = voiceCfg.STT.Keyterms
		}
		if text, err := r.TranscribeAudioWithBackend(userID, comdom.ProviderElevenLabs, req); err == nil {
			return text, nil
		}
		// при ошибке ElevenLabs — fallback на активного провайдера
	}

	manager, err := r.GetActiveUserManager(userID)
	if err != nil {
		return "", fmt.Errorf("ошибка получения активного менеджера для UserID %d: %w", userID, err)
	}
	return manager.TranscribeAudio(userID, audioData, fileName)
}

// GetRealtimeProvider возвращает RealtimeProvider для активной модели пользователя.
// Если для модели включён ElevenLabs realtime-каскад (Voice.RealtimeBackend),
// возвращается cascade-провайдер; иначе — нативный realtime активного провайдера.
func (r *Router) GetRealtimeProvider(userID uint32) (RealtimeProvider, bool) {
	if _, _, ok := r.cascadeVoiceConfig(userID); ok {
		return r.cascade(), true
	}
	activeManager, err := r.GetActiveUserManager(userID)
	if err != nil {
		return nil, false
	}
	rp, ok := activeManager.(RealtimeProvider)
	return rp, ok
}

// getRealtimeProviderByRespId возвращает первый RealtimeProvider, у которого есть сессия с данным respId.
func (r *Router) getRealtimeProviderByRespId(respId uint64) (RealtimeProvider, bool) {
	if r.cascadeProvider != nil && r.cascadeProvider.hasCascadeSession(respId) {
		return r.cascadeProvider, true
	}
	for _, p := range []Inter{r.openai, r.mistral, r.google} {
		if p == nil {
			continue
		}
		rp, ok := p.(RealtimeProvider)
		if !ok {
			continue
		}
		// Используем GetRealtimeGenerating как зонд — если сессия существует, не вернёт nil
		if rp.GetRealtimeGenerating(respId) != nil {
			return rp, true
		}
	}
	return nil, false
}

// GetRealtimeGenerating возвращает указатель на флаг генерации Realtime-сессии
func (r *Router) GetRealtimeGenerating(respId uint64) *atomic.Bool {
	rp, ok := r.getRealtimeProviderByRespId(respId)
	if !ok {
		return nil
	}
	return rp.GetRealtimeGenerating(respId)
}

// DisconnectRealtimeSession завершает голосовую сессию
func (r *Router) DisconnectRealtimeSession(respId uint64) {
	rp, ok := r.getRealtimeProviderByRespId(respId)
	if !ok {
		return
	}
	rp.CloseRealtimeSession(respId)
}

// SetRealtimeDisconnectCallback устанавливает callback критического таймаута watchdog
func (r *Router) SetRealtimeDisconnectCallback(respId uint64, callback func(respId uint64)) error {
	rp, ok := r.getRealtimeProviderByRespId(respId)
	if !ok {
		return fmt.Errorf("SetRealtimeDisconnectCallback: Realtime сессия не найдена для respId=%d", respId)
	}
	return rp.SetRealtimeDisconnectCallback(respId, callback)
}

// Shutdown завершает работу всех провайдеров
func (r *Router) Shutdown(shutCh chan<- com.LogMsg) {
	if r.cascadeProvider != nil {
		r.cascadeProvider.closeAll()
	}
	r.forEachProvider(func(p Inter) { p.Shutdown(shutCh) })
}

// CleanUp запускает фоновую очистку у всех провайдеров
func (r *Router) CleanUp() {
	r.forEachProvider(func(p Inter) { go p.CleanUp() })
}

// ============================================================================
// УПРАВЛЕНИЕ МОДЕЛЯМИ
// ============================================================================

// CreateModel создаёт новую модель у указанного провайдера
func (r *Router) CreateModel(
	userID uint32,
	provider comdom.ProviderType,
	modelData *comdom.UniversalModelData,
	fileIDs []comdom.Ids,
) (comdom.UMCR, error) {
	if modelData.Realtime {
		go r.syncProviderModelsCatalog(userID, comdom.Union{
			Provider:  provider,
			ModelType: comdom.RealTime,
		})
	} else {
		go r.syncProviderModelsCatalog(userID, comdom.Union{
			Provider:  provider,
			ModelType: comdom.General,
		})
	}

	if _, err := r.getModel(provider); err != nil {
		return comdom.UMCR{}, err
	}

	if r.modelsManager == nil {
		return comdom.UMCR{}, fmt.Errorf("модельный менеджер не инициализирован")
	}
	umcr, err := r.modelsManager.CreateModel(userID, provider, modelData, fileIDs)
	if err != nil {
		return comdom.UMCR{}, err
	}

	return umcr, nil
}

func (r *Router) syncProviderModelsCatalog(userID uint32, union comdom.Union) {
	if r.db == nil || !union.Provider.IsValid() {
		return
	}
	if !union.ModelType.IsGeneral() && !union.ModelType.IsRealtime() {
		return
	}

	// Voice-only провайдер: обновляем только голосовой каталог.
	if union.Provider.IsVoiceOnly() {
		apiKey, _ := r.db.GetUserAPIKey(userID, union.Provider)
		r.ensureVoiceCatalogFresh(r.ctx, union.Provider, apiKey)
		return
	}

	// Глобальный in-memory throttle: не чаще одного обращения к провайдеру в 24 ч.
	key := fmt.Sprintf("llm:%d:%d", union.Provider, union.ModelType)
	if !r.catalog.claim(key) {
		return
	}
	success := false
	defer func() { r.catalog.release(key, success) }()

	apiKey, err := r.db.GetUserAPIKey(userID, union.Provider)
	if err != nil {
		return
	}

	if strings.TrimSpace(apiKey) == "" {
		return
	}

	syncCtx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	client := provider_catalog.NewClient()
	modelNames, err := client.FetchModelNames(syncCtx, union, apiKey)
	if err != nil {
		return
	}

	result, err := r.db.SyncProviderModels(union, modelNames)
	if err != nil {
		return
	}
	success = true

	// Голосовые модели провайдера (Mistral STT/TTS) — в voice_models.
	r.ensureVoiceCatalogFresh(r.ctx, union.Provider, apiKey)

	if len(result.AffectedUsers) == 0 {
		return
	}

	for _, affectedUser := range result.AffectedUsers {
		select {
		case mode.GetCarpinteroChannel() <- com.CarpCh{
			Event:      "model-removed",
			UserID:     affectedUser.UserID,
			Target:     union.Provider.String(),
			AssistName: affectedUser.ModelName,
		}:
		default:
			// канал переполнен — уведомление потеряно, но ошибка всё равно вернётся
		}
	}

	return
}

func (r *Router) UpdateModelsListByProvider(ctx context.Context, union comdom.Union, apiKey string) ([]comdom.ProviderModel, error) {
	// Voice-only провайдер (ElevenLabs): ModelType игнорируется, возвращаем все
	// голосовые модели. Каталог обновляется под in-memory throttle.
	if union.Provider.IsVoiceOnly() {
		r.ensureVoiceCatalogFresh(ctx, union.Provider, apiKey)
		return r.db.GetVoiceModels(union.Provider)
	}

	m, err := r.getModel(union.Provider)
	if err != nil {
		return nil, err
	}

	llmKey := fmt.Sprintf("llm:%d:%d", union.Provider, union.ModelType)
	var models []comdom.ProviderModel
	if r.catalog.fresh(llmKey) {
		if cached, cacheErr := r.db.GetProviderModels(union.Provider, union.ModelType); cacheErr == nil && len(cached) > 0 {
			models = cached
		}
	}
	if models == nil {
		models, err = m.UpdateModelsListByProvider(ctx, union, apiKey)
		if err != nil {
			return models, err
		}
		r.catalog.mark(llmKey)
	}

	// Дополняем голосовыми моделями провайдера (например Mistral STT/TTS).
	r.ensureVoiceCatalogFresh(ctx, union.Provider, apiKey)
	if voice, vErr := r.db.GetVoiceModels(union.Provider); vErr == nil && len(voice) > 0 {
		models = append(models, voice...)
	}
	return models, nil
}

// ensureVoiceCatalogFresh под in-memory throttle обновляет voice_models
// провайдера, если каталог устарел; повторный запрос к провайдеру в течение
// 24 часов не выполняется.
func (r *Router) ensureVoiceCatalogFresh(ctx context.Context, provider comdom.ProviderType, apiKey string) {
	if r.db == nil {
		return
	}
	key := fmt.Sprintf("voice:%d", provider)
	if !r.catalog.claim(key) {
		return
	}

	if strings.TrimSpace(apiKey) == "" {
		apiKey, _ = r.db.GetAnyUserAPIKey(provider)
	}
	if strings.TrimSpace(apiKey) == "" {
		r.catalog.release(key, false)
		return
	}

	syncCtx := ctx
	if syncCtx == nil {
		syncCtx = r.ctx
	}
	if syncCtx == nil {
		syncCtx = context.Background()
	}
	syncCtx, cancel := context.WithTimeout(syncCtx, 10*time.Second)
	defer cancel()

	err := r.syncVoiceModels(syncCtx, provider, apiKey)
	r.catalog.release(key, err == nil)
}

// syncVoiceModels получает актуальный голосовой каталог провайдера и пишет его
// в voice_models.
func (r *Router) syncVoiceModels(ctx context.Context, provider comdom.ProviderType, apiKey string) error {
	client := provider_catalog.NewClient()
	switch provider {
	case comdom.ProviderMistral:
		stt, tts, err := client.FetchMistralVoiceModels(ctx, apiKey)
		if err != nil {
			return err
		}
		if _, err := r.db.SyncVoiceModels(provider, comdom.VoiceKindSTT, stt); err != nil {
			return err
		}
		if _, err := r.db.SyncVoiceModels(provider, comdom.VoiceKindTTS, tts); err != nil {
			return err
		}
		return nil
	case comdom.ProviderElevenLabs:
		kinds := []comdom.VoiceKind{
			comdom.VoiceKindTTS,
			comdom.VoiceKindSTT,
			comdom.VoiceKindMusic,
			comdom.VoiceKindSTS,
		}
		var lastErr error
		for _, kind := range kinds {
			names, err := client.FetchElevenLabsModels(ctx, apiKey, kind)
			if err != nil {
				lastErr = err
				continue
			}
			if _, err := r.db.SyncVoiceModels(provider, kind, names); err != nil {
				lastErr = err
			}
		}
		return lastErr
	default:
		return nil
	}
}

// UploadFileToProvider загружает файл в указанный провайдер (только Mistral)
func (r *Router) UploadFileToProvider(userID uint32, provider comdom.ProviderType, fileName string, fileData []byte) (string, error) {
	switch provider {
	case comdom.ProviderOpenAI:
		return "", fmt.Errorf("OpenAI провайдер не поддерживает загрузку файлов")
	case comdom.ProviderMistral:
		if r.mistral == nil {
			return "", fmt.Errorf("Mistral провайдер не инициализирован")
		}
		if manager, ok := r.mistral.(MistralManager); ok {
			return manager.UploadFileToProvider(userID, fileName, fileData)
		}
		return "", fmt.Errorf("Mistral провайдер не поддерживает загрузку файлов")
	case comdom.ProviderGoogle:
		return "", fmt.Errorf("Google провайдер не поддерживает загрузку файлов")
	default:
		return "", fmt.Errorf("неизвестный провайдер: %s", provider)
	}
}

// DeleteTempFile удаляет загруженный временный файл через Mistral провайдер
func (r *Router) DeleteTempFile(fileID string) error {
	if r.mistral == nil {
		return fmt.Errorf("Mistral провайдер не инициализирован")
	}
	manager, ok := r.mistral.(MistralManager)
	if !ok {
		return fmt.Errorf("Mistral провайдер не поддерживает удаление временных файлов")
	}
	return manager.DeleteTempFile(fileID)
}

// DeleteFileFromProvider удаляет файл из указанного провайдера (только Mistral)
func (r *Router) DeleteFileFromProvider(userID uint32, provider comdom.ProviderType, fileID string) error {
	switch provider {
	case comdom.ProviderOpenAI:
		return fmt.Errorf("OpenAI провайдер не поддерживает удаление файлов")
	case comdom.ProviderMistral:
		if r.mistral == nil {
			return fmt.Errorf("Mistral провайдер не инициализирован")
		}
		if manager, ok := r.mistral.(MistralManager); ok {
			return manager.DeleteDocumentFromLibrary(userID, fileID)
		}
		return fmt.Errorf("Mistral провайдер не поддерживает удаление файлов")
	case comdom.ProviderGoogle:
		return fmt.Errorf("Google провайдер не поддерживает удаление файлов")
	default:
		return fmt.Errorf("неизвестный провайдер: %s", provider)
	}
}

// AddFileFromFromProvider добавляет файл в хранилище провайдера (только Mistral)
func (r *Router) AddFileFromFromProvider(provider comdom.ProviderType, userID uint32, fileID, fileName string) error {
	switch provider {
	case comdom.ProviderOpenAI:
		return fmt.Errorf("OpenAI провайдер не поддерживает добавление файлов")
	case comdom.ProviderMistral:
		if r.mistral == nil {
			return fmt.Errorf("Mistral провайдер не инициализирован")
		}
		if manager, ok := r.mistral.(MistralManager); ok {
			return manager.AddFileToLibrary(userID, fileID, fileName)
		}
		return fmt.Errorf("Mistral провайдер не поддерживает добавление файлов")
	case comdom.ProviderGoogle:
		return fmt.Errorf("Google провайдер не поддерживает добавление файлов")
	default:
		return fmt.Errorf("неизвестный провайдер: %s", provider)
	}
}

// ============================================================================
// VECTOR EMBEDDING МЕТОДЫ (OpenAI + Google)
// ============================================================================

// UploadDocumentWithEmbedding загружает документ с генерацией эмбеддинга
func (r *Router) UploadDocumentWithEmbedding(userID uint32, provider, docName, content string, metadata comdom.DocumentMetadata) (string, error) {
	providerType, err := comdom.FromString(provider)
	if err != nil {
		return "", fmt.Errorf("неверный provider: %w", err)
	}
	switch providerType {
	case comdom.ProviderGoogle:
		if r.google == nil {
			return "", fmt.Errorf("Google провайдер не инициализирован")
		}
		if manager, ok := r.google.(GoogleManager); ok {
			return manager.UploadDocumentWithEmbedding(userID, docName, content, metadata)
		}
		return "", fmt.Errorf("Google провайдер не поддерживает загрузку документов с эмбеддингами")
	case comdom.ProviderOpenAI:
		if r.openai == nil {
			return "", fmt.Errorf("OpenAI провайдер не инициализирован")
		}
		if manager, ok := r.openai.(OpenAIManager); ok {
			return manager.UploadDocumentWithEmbedding(userID, docName, content, metadata)
		}
		return "", fmt.Errorf("OpenAI провайдер не поддерживает загрузку документов с эмбеддингами")
	default:
		return "", fmt.Errorf("провайдер %s не поддерживает эмбеддинги", provider)
	}
}

// SearchSimilarDocuments ищет похожие документы в Vector Store
func (r *Router) SearchSimilarDocuments(userID uint32, provider, query string, limit int) ([]comdom.VectorDocument, error) {
	providerType, err := comdom.FromString(provider)
	if err != nil {
		return nil, fmt.Errorf("неверный provider: %w", err)
	}
	switch providerType {
	case comdom.ProviderGoogle:
		if r.google == nil {
			return nil, fmt.Errorf("Google провайдер не инициализирован")
		}
		if manager, ok := r.google.(GoogleManager); ok {
			return manager.SearchSimilarDocuments(userID, query, limit)
		}
		return nil, fmt.Errorf("Google провайдер не поддерживает поиск документов")
	case comdom.ProviderOpenAI:
		if r.openai == nil {
			return nil, fmt.Errorf("OpenAI провайдер не инициализирован")
		}
		if manager, ok := r.openai.(OpenAIManager); ok {
			return manager.SearchSimilarDocuments(userID, query, limit)
		}
		return nil, fmt.Errorf("OpenAI провайдер не поддерживает поиск документов")
	default:
		return nil, fmt.Errorf("провайдер %s не поддерживает эмбеддинги", provider)
	}
}

// DeleteDocument удаляет документ из Vector Store
func (r *Router) DeleteDocument(userID uint32, provider, docID string) error {
	providerType, err := comdom.FromString(provider)
	if err != nil {
		return fmt.Errorf("неверный provider: %w", err)
	}
	switch providerType {
	case comdom.ProviderGoogle:
		if r.google == nil {
			return fmt.Errorf("Google провайдер не инициализирован")
		}
		if manager, ok := r.google.(GoogleManager); ok {
			return manager.DeleteDocument(userID, docID)
		}
		return fmt.Errorf("Google провайдер не поддерживает удаление документов")
	case comdom.ProviderOpenAI:
		if r.openai == nil {
			return fmt.Errorf("OpenAI провайдер не инициализирован")
		}
		if manager, ok := r.openai.(OpenAIManager); ok {
			return manager.DeleteDocument(userID, docID)
		}
		return fmt.Errorf("OpenAI провайдер не поддерживает удаление документов")
	default:
		return fmt.Errorf("провайдер %s не поддерживает эмбеддинги", provider)
	}
}

// ListUserDocuments возвращает список документов пользователя.
// Если provider пустой — агрегирует документы всех провайдеров.
func (r *Router) ListUserDocuments(userID uint32, provider string) ([]comdom.VectorDocument, error) {
	if provider == "" {
		var allDocs []comdom.VectorDocument
		if r.google != nil {
			if manager, ok := r.google.(GoogleManager); ok {
				if docs, err := manager.ListUserDocuments(userID); err == nil && docs != nil {
					allDocs = append(allDocs, docs...)
				}
			}
		}
		if r.openai != nil {
			if manager, ok := r.openai.(OpenAIManager); ok {
				if docs, err := manager.ListUserDocuments(userID); err == nil && docs != nil {
					allDocs = append(allDocs, docs...)
				}
			}
		}
		return allDocs, nil
	}

	providerType, err := comdom.FromString(provider)
	if err != nil {
		return nil, fmt.Errorf("неверный provider: %w", err)
	}
	switch providerType {
	case comdom.ProviderGoogle:
		if r.google == nil {
			return nil, fmt.Errorf("Google провайдер не инициализирован")
		}
		if manager, ok := r.google.(GoogleManager); ok {
			return manager.ListUserDocuments(userID)
		}
		return nil, fmt.Errorf("Google провайдер не поддерживает список документов")
	case comdom.ProviderOpenAI:
		if r.openai == nil {
			return nil, fmt.Errorf("OpenAI провайдер не инициализирован")
		}
		if manager, ok := r.openai.(OpenAIManager); ok {
			return manager.ListUserDocuments(userID)
		}
		return nil, fmt.Errorf("OpenAI провайдер не поддерживает список документов")
	default:
		return nil, fmt.Errorf("провайдер %s не поддерживает эмбеддинги", provider)
	}
}

// ============================================================================
// ДЕЛЕГАТЫ К modelsManager
// ============================================================================

// SaveModel сохраняет модель в БД
func (r *Router) SaveModel(userID uint32, umcr comdom.UMCR, data *comdom.UniversalModelData) error {
	if r.modelsManager == nil {
		return fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.SaveModel(userID, umcr, data)
}

// ReadModel читает модель пользователя по провайдеру
func (r *Router) ReadModel(userID uint32, provider *comdom.ProviderType) (*comdom.UniversalModelData, error) {
	if r.modelsManager == nil {
		return nil, fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.ReadModel(userID, provider)
}

// GetAllModelAsJSON получает все модели пользователя в виде JSON
func (r *Router) GetAllModelAsJSON(userID uint32) ([]byte, error) {
	if r.modelsManager == nil {
		return nil, fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.GetModelAsJSON(userID)
}

// DeleteModel удаляет модель пользователя
func (r *Router) DeleteModel(userID uint32, provider comdom.ProviderType, deleteFiles bool, progressCallback func(string)) error {
	if r.modelsManager == nil {
		return fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.DeleteModel(userID, provider, deleteFiles, progressCallback)
}

// UpdateModelToDB обновляет модель в БД (без обновления у провайдера)
func (r *Router) UpdateModelToDB(userID uint32, data *comdom.UniversalModelData) error {
	if r.modelsManager == nil {
		return fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.UpdateModelToDB(userID, data)
}

// UpdateModelEveryWhere обновляет модель в БД и у провайдера
func (r *Router) UpdateModelEveryWhere(userID uint32, data *comdom.UniversalModelData) error {
	if r.modelsManager == nil {
		return fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.UpdateModelEveryWhere(userID, data)
}

// GetUserModels получает все модели пользователя
func (r *Router) GetUserModels(userID uint32) ([]comdom.UniversalModelData, error) {
	if r.modelsManager == nil {
		return nil, fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.GetUserModels(userID)
}

// GetUserModelsResponse получает все модели пользователя для API
func (r *Router) GetUserModelsResponse(userID uint32) (*comdom.UserModelsResponse, error) {
	if r.modelsManager == nil {
		return nil, fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.GetAllUserModelsResponse(userID)
}

// SetActiveUserModel переключает активную модель пользователя
func (r *Router) SetActiveUserModel(userID uint32, provider comdom.ProviderType) error {
	if r.modelsManager == nil {
		return fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.SetActiveModelByProvider(userID, provider)
}

// GetUserModelByProvider получает модель пользователя по провайдеру
func (r *Router) GetUserModelByProvider(userID uint32, provider comdom.ProviderType) (*comdom.UniversalModelData, error) {
	if r.modelsManager == nil {
		return nil, fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.GetUserModelByProvider(userID, provider)
}

// ProvidersWithApiKeys возвращает два списка провайдеров: с API-ключом и без.
func (r *Router) ProvidersWithApiKeys(userID uint32) comdom.ProvidersAvailability {
	if r.modelsManager == nil {
		return comdom.ProvidersAvailability{}
	}
	return r.modelsManager.ProvidersWithApiKeys(userID)
}

// GetProviderCapabilities возвращает карту возможностей провайдеров для фронтенда:
// {"elevenlabs":["tts","stt","voice_clone","music","speech_to_speech"], ...}.
func (r *Router) GetProviderCapabilities() map[string][]string {
	result := make(map[string][]string, len(comdom.AllProviders))
	for _, p := range comdom.AllProviders {
		caps := p.Capabilities()
		list := make([]string, 0, len(caps))
		for _, c := range caps {
			list = append(list, c.String())
		}
		result[p.String()] = list
	}
	return result
}

// InvalidateUserAgentConfigCache инвалидирует кэш конфигурации модели для пользователя
func (r *Router) InvalidateUserAgentConfigCache(userID uint32) {
	r.forEachProvider(func(p Inter) { p.InvalidateUserAgentConfigCache(userID) })
}

// DisconnectUser завершает активные сессии пользователя у всех инициализированных провайдеров:
// закрывает realtime-соединения, отменяет контексты респондентов, удаляет их из кэша.
// Используется при глобальном отключении пользователя (например, блокировка аккаунта).
// Для отключения конкретного провайдера используйте RevokeUserAPIKey.
func (r *Router) DisconnectUser(userID uint32) {
	if r.cascadeProvider != nil {
		r.cascadeProvider.closeUser(userID)
	}
	r.forEachProvider(func(p Inter) { p.DisconnectUser(userID) })
}

// RevokeUserAPIKey выполняет graceful завершение всех сессий пользователя
// для указанного провайдера и удаляет API-ключ из БД.
// Порядок: сначала DisconnectUser у конкретного провайдера, затем удаление ключа.
func (r *Router) RevokeUserAPIKey(userID uint32, provider comdom.ProviderType) error {
	// Завершаем сессии только у указанного провайдера
	if p, err := r.getModel(provider); err == nil {
		p.DisconnectUser(userID)
	} else if provider.IsVoiceOnly() {
		// У voice-only провайдера нет LLM-сессий: закрываем каскадные сессии и
		// инвалидируем кэш, чтобы новые сессии перестали использовать его голос.
		if r.cascadeProvider != nil {
			r.cascadeProvider.closeUser(userID)
		}
		r.InvalidateUserAgentConfigCache(userID)
	}

	if r.modelsManager == nil {
		return fmt.Errorf("модельный менеджер не инициализирован")
	}
	return r.modelsManager.DeleteUserAPIKey(userID, provider)
}
