package comdom

import "testing"

func TestDefaultAndResolveVoiceModel(t *testing.T) {
	if got := DefaultVoiceModel(VoiceKindTTS); got != "eleven_flash_v2_5" {
		t.Fatalf("TTS default=%q", got)
	}
	if got := DefaultVoiceModel(VoiceKindSTT); got != "scribe_v2_realtime" {
		t.Fatalf("STT default=%q", got)
	}
	if got := ResolveVoiceModel(VoiceKindTTS, "  "); got != "eleven_flash_v2_5" {
		t.Fatalf("empty must fall back to default, got %q", got)
	}
	if got := ResolveVoiceModel(VoiceKindTTS, "eleven_v3"); got != "eleven_v3" {
		t.Fatalf("explicit model must be kept, got %q", got)
	}
}

func TestVoiceConfigHelpers(t *testing.T) {
	tts := "eleven_v3"
	stt := "scribe_v2"
	el := ProviderElevenLabs
	cfg := &VoiceConfig{TTS: &TTSConfig{Model: &tts}, STT: &STTConfig{Model: &stt}, TTSBackend: &el}
	if cfg.TTSModelName() != "eleven_v3" {
		t.Fatalf("TTSModelName=%q", cfg.TTSModelName())
	}
	if cfg.STTModelName() != "scribe_v2" {
		t.Fatalf("STTModelName=%q", cfg.STTModelName())
	}
	if !cfg.UsesElevenLabsVoice() {
		t.Fatalf("UsesElevenLabsVoice must be true")
	}
	if (*VoiceConfig)(nil).UsesElevenLabsVoice() {
		t.Fatalf("nil VoiceConfig must not use ElevenLabs")
	}
	if (*VoiceConfig)(nil).TTSModelName() != "eleven_flash_v2_5" {
		t.Fatalf("nil TTS must return default")
	}
}

func TestMusicModelName(t *testing.T) {
	if (*VoiceConfig)(nil).MusicModelName() != "music_v1" {
		t.Fatalf("nil Music must return default music_v1")
	}
	model := "music_v2_5"
	cfg := &VoiceConfig{Music: &MusicConfig{Model: &model}}
	if cfg.MusicModelName() != "music_v2_5" {
		t.Fatalf("MusicModelName=%q", cfg.MusicModelName())
	}
}
