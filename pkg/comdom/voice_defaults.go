package comdom

import "strings"

// DefaultVoiceModel возвращает дефолтный model_id для вида голосовой модели.
// Используется как fallback, если пользователь не выбрал модель или выбранная
// отсутствует в каталоге voice_models.
func DefaultVoiceModel(kind VoiceKind) string {
	switch kind {
	case VoiceKindTTS:
		return "eleven_flash_v2_5"
	case VoiceKindSTT:
		return "scribe_v2_realtime"
	case VoiceKindMusic:
		return "eleven_music"
	case VoiceKindSTS:
		return "eleven_multilingual_sts_v2"
	default:
		return ""
	}
}

// ResolveVoiceModel возвращает выбранное имя модели или дефолт для вида.
func ResolveVoiceModel(kind VoiceKind, name string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return DefaultVoiceModel(kind)
}

// TTSModelName возвращает выбранную TTS-модель или дефолт.
func (v *VoiceConfig) TTSModelName() string {
	if v != nil && v.TTS != nil && v.TTS.Model != nil {
		return ResolveVoiceModel(VoiceKindTTS, *v.TTS.Model)
	}
	return DefaultVoiceModel(VoiceKindTTS)
}

// STTModelName возвращает выбранную STT-модель или дефолт.
func (v *VoiceConfig) STTModelName() string {
	if v != nil && v.STT != nil && v.STT.Model != nil {
		return ResolveVoiceModel(VoiceKindSTT, *v.STT.Model)
	}
	return DefaultVoiceModel(VoiceKindSTT)
}

// UsesElevenLabsVoice сообщает, что хотя бы одна голосовая стадия использует
// ElevenLabs (используется для выбора cascade/hybrid-пути).
func (v *VoiceConfig) UsesElevenLabsVoice() bool {
	if v == nil {
		return false
	}
	for _, backend := range []*ProviderType{v.TTSBackend, v.STTBackend, v.RealtimeBackend} {
		if backend != nil && *backend == ProviderElevenLabs {
			return true
		}
	}
	return false
}

// UsesElevenLabsRealtime сообщает, что realtime-стадию обслуживает ElevenLabs
// (каскад STT → LLM → TTS вместо нативного audio-to-audio провайдера).
func (v *VoiceConfig) UsesElevenLabsRealtime() bool {
	return v != nil && v.RealtimeBackend != nil && *v.RealtimeBackend == ProviderElevenLabs
}
