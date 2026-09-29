package provider_catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ikermy/air-common/pkg/comdom"
)

func TestFetchElevenLabsModels(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		gotKey = r.Header.Get("xi-api-key")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"model_id": "m_tts", "can_do_text_to_speech": true},
			{"model_id": "m_sts", "can_do_voice_conversion": true},
			{"model_id": "m_alpha", "can_do_text_to_speech": true, "requires_alpha_access": true},
			{"model_id": "m_neither"},
		})
	}))
	defer srv.Close()

	client := &Client{HTTPClient: srv.Client(), ElevenLabsBaseURL: srv.URL}

	tts, err := client.FetchElevenLabsModels(context.Background(), "test-key", comdom.VoiceKindTTS)
	if err != nil {
		t.Fatalf("TTS fetch error: %v", err)
	}
	if len(tts) != 1 || tts[0] != "m_tts" {
		t.Fatalf("TTS models=%v, want [m_tts]", tts)
	}

	sts, err := client.FetchElevenLabsModels(context.Background(), "test-key", comdom.VoiceKindSTS)
	if err != nil {
		t.Fatalf("STS fetch error: %v", err)
	}
	if len(sts) != 1 || sts[0] != "m_sts" {
		t.Fatalf("STS models=%v, want [m_sts]", sts)
	}

	if gotKey != "test-key" {
		t.Fatalf("xi-api-key header=%q, want test-key", gotKey)
	}

	stt, err := client.FetchElevenLabsModels(context.Background(), "test-key", comdom.VoiceKindSTT)
	if err != nil {
		t.Fatalf("STT fetch error: %v", err)
	}
	if len(stt) == 0 || stt[0] != "scribe_v2_realtime" {
		t.Fatalf("STT models=%v, want static list starting with scribe_v2_realtime", stt)
	}

	music, err := client.FetchElevenLabsModels(context.Background(), "test-key", comdom.VoiceKindMusic)
	if err != nil {
		t.Fatalf("Music fetch error: %v", err)
	}
	if len(music) != 1 || music[0] != "eleven_music" {
		t.Fatalf("Music models=%v, want [eleven_music]", music)
	}
}

func TestFetchElevenLabsModelsEmptyKey(t *testing.T) {
	client := &Client{}
	if _, err := client.FetchElevenLabsModels(context.Background(), "", comdom.VoiceKindTTS); err == nil {
		t.Fatalf("expected error for empty api key")
	}
}
