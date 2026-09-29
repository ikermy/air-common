package comdom

import (
	"encoding/json"
	"testing"
)

func TestProviderTypeUnmarshalJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want ProviderType
	}{
		{"string elevenlabs", `"elevenlabs"`, ProviderElevenLabs},
		{"string mistral", `"mistral"`, ProviderMistral},
		{"numeric 4", `4`, ProviderElevenLabs},
		{"numeric 2", `2`, ProviderMistral},
		{"null", `null`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got ProviderType
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestProviderTypeUnmarshalJSON_Invalid(t *testing.T) {
	for _, in := range []string{`"unknown"`, `true`} {
		var got ProviderType
		if err := json.Unmarshal([]byte(in), &got); err == nil {
			t.Fatalf("expected error for %s", in)
		}
	}
}

func TestVoiceConfig_JSONAcceptsStringBackends(t *testing.T) {
	const payload = `{
		"name": "bot",
		"voice": {
			"stt_backend": "elevenlabs",
			"tts_backend": "elevenlabs",
			"realtime_backend": "elevenlabs",
			"voice_id": "abc",
			"stt": {"model": "scribe_v2_realtime", "language": "ru"},
			"tts": {"model": "eleven_flash_v2_5", "format": "pcm_16000"}
		}
	}`
	var data UniversalModelData
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Voice == nil {
		t.Fatal("Voice config was dropped")
	}
	if !data.Voice.UsesElevenLabsRealtime() || !data.Voice.UsesElevenLabsVoice() {
		t.Fatalf("string backends not resolved: %+v", data.Voice)
	}
	if data.Voice.VoiceID == nil || *data.Voice.VoiceID != "abc" {
		t.Fatalf("voice_id=%v", data.Voice.VoiceID)
	}
	if data.Voice.STTModelName() != "scribe_v2_realtime" {
		t.Fatalf("stt model=%q", data.Voice.STTModelName())
	}
}

func TestVoiceConfig_JSONAcceptsNumericBackends(t *testing.T) {
	const payload = `{"voice":{"realtime_backend":4,"stt_backend":4}}`
	var data UniversalModelData
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Voice == nil || !data.Voice.UsesElevenLabsRealtime() {
		t.Fatalf("numeric backends not resolved: %+v", data.Voice)
	}
}
