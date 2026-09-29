package comdom

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ProviderType uint8

const (
	ProviderOpenAI     ProviderType = 1
	ProviderMistral    ProviderType = 2
	ProviderGoogle     ProviderType = 3
	ProviderElevenLabs ProviderType = 4
)

// AllProviders — все провайдеры, для которых может храниться API-ключ
// (включая voice-only). Используется для provider availability и user_api_keys.
var AllProviders = []ProviderType{ProviderOpenAI, ProviderMistral, ProviderGoogle, ProviderElevenLabs}

// AllLLMProviders — провайдеры с LLM-контрактом (model.Inter). Voice-only
// провайдеры (ElevenLabs) сюда не входят: активная модель и getModel — только по ним.
var AllLLMProviders = []ProviderType{ProviderOpenAI, ProviderMistral, ProviderGoogle}

func (p ProviderType) String() string {
	switch p {
	case ProviderOpenAI:
		return "openai"
	case ProviderMistral:
		return "mistral"
	case ProviderGoogle:
		return "google"
	case ProviderElevenLabs:
		return "elevenlabs"
	default:
		return "unknown"
	}
}
func FromString(s string) (ProviderType, error) {
	switch s {
	case "openai":
		return ProviderOpenAI, nil
	case "mistral":
		return ProviderMistral, nil
	case "google":
		return ProviderGoogle, nil
	case "elevenlabs":
		return ProviderElevenLabs, nil
	default:
		return 0, fmt.Errorf("неизвестный провайдер: %s", s)
	}
}
func (p ProviderType) FromUint8(value uint8) ProviderType { return ProviderType(value) }

// UnmarshalJSON принимает как числовой ID провайдера (1..4), так и строковое имя
// ("openai", "mistral", "google", "elevenlabs"). Нужно, чтобы конфигурация
// Voice (tts_backend/stt_backend/realtime_backend/music_backend) читалась из БД
// и из API в обоих форматах: документация использует строки, а старые записи —
// числа. Без этого весь JSON модели не разбирался (json: cannot unmarshal
// string into ... ProviderType), Voice терялся и голос падал на провайдера по
// умолчанию (Mistral).
func (p *ProviderType) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}
		parsed, err := FromString(strings.TrimSpace(name))
		if err != nil {
			return err
		}
		*p = parsed
		return nil
	}
	var numeric uint8
	if err := json.Unmarshal(data, &numeric); err != nil {
		return err
	}
	*p = ProviderType(numeric)
	return nil
}
func (p ProviderType) IsValid() bool {
	for _, known := range AllProviders {
		if p == known {
			return true
		}
	}
	return false
}

// IsLLM сообщает, есть ли у провайдера LLM-контракт (model.Inter).
func (p ProviderType) IsLLM() bool {
	for _, known := range AllLLMProviders {
		if p == known {
			return true
		}
	}
	return false
}

// IsVoiceOnly сообщает, что провайдер предоставляет только голосовые
// возможности и не может быть активной LLM-моделью.
func (p ProviderType) IsVoiceOnly() bool {
	return p.IsValid() && !p.IsLLM()
}

// Capability — голосовые/LLM возможности провайдера.
type Capability uint8

const (
	CapabilityLLM Capability = iota + 1
	CapabilityRealtime
	CapabilityTTS
	CapabilitySTT
	CapabilityVoiceClone
	CapabilityMusic
	CapabilitySpeechToSpeech // voice conversion (eleven_*_sts_v2)
)

func (c Capability) String() string {
	switch c {
	case CapabilityLLM:
		return "llm"
	case CapabilityRealtime:
		return "realtime"
	case CapabilityTTS:
		return "tts"
	case CapabilitySTT:
		return "stt"
	case CapabilityVoiceClone:
		return "voice_clone"
	case CapabilityMusic:
		return "music"
	case CapabilitySpeechToSpeech:
		return "speech_to_speech"
	default:
		return "unknown"
	}
}

// Capabilities возвращает возможности провайдера.
func (p ProviderType) Capabilities() []Capability {
	switch p {
	case ProviderOpenAI, ProviderGoogle:
		return []Capability{CapabilityLLM, CapabilityRealtime, CapabilityTTS, CapabilitySTT}
	case ProviderMistral:
		return []Capability{CapabilityLLM, CapabilityRealtime, CapabilityTTS, CapabilitySTT, CapabilityVoiceClone}
	case ProviderElevenLabs:
		return []Capability{CapabilityTTS, CapabilitySTT, CapabilityVoiceClone, CapabilityMusic, CapabilitySpeechToSpeech}
	default:
		return nil
	}
}

// Supports сообщает, поддерживает ли провайдер указанную возможность.
func (p ProviderType) Supports(c Capability) bool {
	for _, known := range p.Capabilities() {
		if known == c {
			return true
		}
	}
	return false
}

// VoiceKind — назначение голосовой модели (колонка voice_models.Kind).
type VoiceKind string

const (
	VoiceKindTTS   VoiceKind = "tts"
	VoiceKindSTT   VoiceKind = "stt"
	VoiceKindMusic VoiceKind = "music"
	VoiceKindSTS   VoiceKind = "sts"
)

func (k VoiceKind) IsValid() bool {
	switch k {
	case VoiceKindTTS, VoiceKindSTT, VoiceKindMusic, VoiceKindSTS:
		return true
	default:
		return false
	}
}

func VoiceKindFromString(s string) (VoiceKind, error) {
	k := VoiceKind(s)
	if !k.IsValid() {
		return "", fmt.Errorf("неизвестный тип голосовой модели: %s", s)
	}
	return k, nil
}

type ModelType uint8

const (
	General  ModelType = 1
	RealTime ModelType = 2
)

func (m ModelType) IsRealtime() bool { return m == RealTime }
func (m ModelType) IsGeneral() bool  { return m == General }
func ModelTypeFromString(s string) (ModelType, error) {
	switch s {
	case "general":
		return General, nil
	case "realtime":
		return RealTime, nil
	default:
		return 0, fmt.Errorf("неизвестный тип модели: %s", s)
	}
}

type Union struct {
	Provider  ProviderType
	ModelType ModelType
}
