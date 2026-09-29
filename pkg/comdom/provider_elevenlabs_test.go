package comdom

import "testing"

func TestProviderElevenLabsRoundTrip(t *testing.T) {
	if got := ProviderElevenLabs.String(); got != "elevenlabs" {
		t.Fatalf("String()=%q, want elevenlabs", got)
	}
	p, err := FromString("elevenlabs")
	if err != nil {
		t.Fatalf("FromString(elevenlabs) error: %v", err)
	}
	if p != ProviderElevenLabs {
		t.Fatalf("FromString(elevenlabs)=%d, want %d", p, ProviderElevenLabs)
	}
	if !p.IsValid() {
		t.Fatalf("ProviderElevenLabs must be valid")
	}
	if p != 4 {
		t.Fatalf("ProviderElevenLabs=%d, want 4 (model_providers.Id)", p)
	}
}

func TestProviderRoles(t *testing.T) {
	if !ProviderElevenLabs.IsVoiceOnly() {
		t.Fatalf("ElevenLabs must be voice-only")
	}
	if ProviderElevenLabs.IsLLM() {
		t.Fatalf("ElevenLabs must not be LLM")
	}
	for _, p := range AllLLMProviders {
		if !p.IsLLM() || p.IsVoiceOnly() {
			t.Fatalf("provider %s must be LLM and not voice-only", p)
		}
	}
	if !ProviderElevenLabs.Supports(CapabilityTTS) || !ProviderElevenLabs.Supports(CapabilitySTT) {
		t.Fatalf("ElevenLabs must support TTS and STT")
	}
	if ProviderElevenLabs.Supports(CapabilityLLM) {
		t.Fatalf("ElevenLabs must not support LLM capability")
	}
}

func TestVoiceKindValidation(t *testing.T) {
	for _, k := range []VoiceKind{VoiceKindTTS, VoiceKindSTT, VoiceKindMusic, VoiceKindSTS} {
		if !k.IsValid() {
			t.Fatalf("kind %s must be valid", k)
		}
		if _, err := VoiceKindFromString(string(k)); err != nil {
			t.Fatalf("VoiceKindFromString(%s) error: %v", k, err)
		}
	}
	if VoiceKind("bad").IsValid() {
		t.Fatalf("bad kind must be invalid")
	}
	if _, err := VoiceKindFromString("bad"); err == nil {
		t.Fatalf("VoiceKindFromString(bad) must error")
	}
}
