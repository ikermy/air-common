package comdom

// VoiceConfig описывает выбор голосовых backend'ов и моделей для модели
// пользователя. Провайдер LLM и голосовой backend выбираются независимо:
// ElevenLabs может озвучивать/распознавать речь для диалога OpenAI/Mistral/Google.
type VoiceConfig struct {
	// Выбор backend на каждую голосовую стадию.
	TTSBackend      *ProviderType `json:"tts_backend,omitempty"`      // elevenlabs | mistral | openai | google
	STTBackend      *ProviderType `json:"stt_backend,omitempty"`      // elevenlabs | mistral | openai | google
	RealtimeBackend *ProviderType `json:"realtime_backend,omitempty"` // native (nil) | elevenlabs (cascade)
	MusicBackend    *ProviderType `json:"music_backend,omitempty"`    // elevenlabs

	TTS   *TTSConfig   `json:"tts,omitempty"`
	STT   *STTConfig   `json:"stt,omitempty"`
	STS   *STSConfig   `json:"sts,omitempty"`
	Music *MusicConfig `json:"music,omitempty"`

	// Общая привязка к голосовому профилю.
	VoiceID   *string `json:"voice_id,omitempty"`
	VoiceName *string `json:"voice_name,omitempty"`

	ElevenLabs *ElevenLabsVoiceConfig `json:"elevenlabs,omitempty"`
}

// TTSConfig — параметры синтеза речи.
type TTSConfig struct {
	Model        *string  `json:"model,omitempty"`    // eleven_flash_v2_5 | eleven_turbo_v2_5 | eleven_v3 | eleven_multilingual_v2 | ...
	Format       *string  `json:"format,omitempty"`   // pcm_16000 | mp3_44100_128 | ulaw_8000 | ...
	Language     *string  `json:"language,omitempty"` // language_id из /v1/models.languages
	Speed        *float64 `json:"speed,omitempty"`
	Stability    *float64 `json:"stability,omitempty"`
	Similarity   *float64 `json:"similarity_boost,omitempty"`
	Style        *float64 `json:"style,omitempty"`             // только если can_use_style
	SpeakerBoost *bool    `json:"use_speaker_boost,omitempty"` // только если can_use_speaker_boost
}

// STTConfig — параметры распознавания речи.
type STTConfig struct {
	Model         *string  `json:"model,omitempty"`          // batch: scribe_v2 | scribe_v1
	RealtimeModel *string  `json:"realtime_model,omitempty"` // realtime-каскад: scribe_v2_realtime
	Language      *string  `json:"language,omitempty"`
	Keyterms      []string `json:"keyterms,omitempty"`
}

// STSConfig — speech-to-speech / voice conversion (eleven_*_sts_v2).
type STSConfig struct {
	Model   *string `json:"model,omitempty"`
	VoiceID *string `json:"voice_id,omitempty"`
}

// MusicConfig — параметры генерации музыки (tool generate_music).
//
// Соответствует POST /v1/music: prompt и composition_plan взаимоисключающие.
type MusicConfig struct {
	Model             *string               `json:"model,omitempty"`              // music_v1 | music_v2 | music_v2_5
	Format            *string               `json:"format,omitempty"`             // output_format (query), напр. mp3_44100_128
	LengthMs          *int                  `json:"length_ms,omitempty"`          // 3000..600000, только с prompt
	ForceInstrumental *bool                 `json:"force_instrumental,omitempty"` // только с prompt
	Seed              *int                  `json:"seed,omitempty"`               // 0..2147483647, только с composition_plan
	CompositionPlan   *MusicCompositionPlan `json:"composition_plan,omitempty"`
}

// MusicCompositionPlan — детализированный план композиции (chunks).
// Использовать вместо prompt; длина считается как сумма секций.
type MusicCompositionPlan struct {
	Chunks []MusicChunk `json:"chunks"` // 1..30
}

// MusicChunk — секция композиции.
type MusicChunk struct {
	Text           string   `json:"text"`        // текст/секция, до 30 строк по 200 символов
	DurationMs     int      `json:"duration_ms"` // 3000..120000
	PositiveStyles []string `json:"positive_styles,omitempty"`
	NegativeStyles []string `json:"negative_styles,omitempty"`
}

// ElevenLabsVoiceConfig — специфичные для ElevenLabs настройки голоса.
type ElevenLabsVoiceConfig struct {
	VoiceSettings *TTSConfig `json:"voice_settings,omitempty"`
}
