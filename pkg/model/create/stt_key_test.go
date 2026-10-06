package create

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAITranscribeAudioUsesPersonalKey проверяет, что batch-STT OpenAI
// подставляет персональный API-ключ пользователя (а не пустой глобальный).
func TestOpenAITranscribeAudioUsesPersonalKey(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hello"}`))
	}))
	defer server.Close()

	client := &OpenAIAgentClient{url: server.URL, httpClient: server.Client()}
	client.SetKeyResolver(func(userID uint32) string {
		if userID == 42 {
			return "personal-openai"
		}
		return ""
	})

	text, err := client.TranscribeAudio(context.Background(), 42, []byte("audio"), "a.wav")
	if err != nil {
		t.Fatalf("TranscribeAudio() error: %v", err)
	}
	if text != "hello" {
		t.Fatalf("TranscribeAudio() = %q, want hello", text)
	}
	if gotAuth != "Bearer personal-openai" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer personal-openai")
	}
}

// TestGoogleTranscribeAudioUsesPersonalKey проверяет, что batch-STT Google
// подставляет персональный API-ключ пользователя в query-параметр key.
func TestGoogleTranscribeAudioUsesPersonalKey(t *testing.T) {
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`))
	}))
	defer server.Close()

	client := &GoogleAgentClient{url: server.URL, ctx: context.Background()}
	client.SetKeyResolver(func(userID uint32) string {
		if userID == 42 {
			return "personal-google"
		}
		return ""
	})

	text, err := client.TranscribeAudio(42, []byte("audio"), "audio/wav")
	if err != nil {
		t.Fatalf("TranscribeAudio() error: %v", err)
	}
	if text != "hello" {
		t.Fatalf("TranscribeAudio() = %q, want hello", text)
	}
	if gotKey != "personal-google" {
		t.Fatalf("query key = %q, want %q", gotKey, "personal-google")
	}
}

// TestGoogleTranscribeAudioNoGlobalFallback проверяет, что при userID == 0
// глобального ключа нет: резолвер не вызывается и key пустой.
func TestGoogleTranscribeAudioNoGlobalFallback(t *testing.T) {
	var gotKey = "unset"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`))
	}))
	defer server.Close()

	client := &GoogleAgentClient{url: server.URL, ctx: context.Background()}
	client.SetKeyResolver(func(uint32) string { return "personal-google" })

	if _, err := client.TranscribeAudio(0, []byte("audio"), "audio/wav"); err != nil {
		t.Fatalf("TranscribeAudio() error: %v", err)
	}
	if gotKey != "" {
		t.Fatalf("query key = %q, want empty (no global fallback)", gotKey)
	}
}
