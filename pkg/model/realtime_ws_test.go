package model

import (
	"encoding/json"
	"errors"
	"testing"
)

var errTestRealtime = errors.New("transport failed")

func TestNormalizeRealtimeEventTranscriptDeltaFallback(t *testing.T) {
	msg := NormalizeRealtimeEvent(RealtimeEvent{
		Type: "input_transcript_delta",
		Text: "привет",
	})

	if msg.Type != "transcript" || msg.Role != "user" || msg.Phase != "delta" {
		t.Fatalf("unexpected message identity: %+v", msg)
	}
	if msg.Delta != "привет" || msg.Text != "привет" {
		t.Fatalf("delta fallback was not applied: %+v", msg)
	}
}

func TestNormalizeRealtimeEventPreservesLifecycleAndMetadata(t *testing.T) {
	files := []File{{Type: Photo, URL: "https://example.test/image.jpg", FileName: "image.jpg"}}
	msg := NormalizeRealtimeEvent(RealtimeEvent{
		Type:       "response_done",
		ResponseID: "resp_123",
		Files:      files,
	})

	if msg.Type != "response" || msg.Phase != "done" || msg.ResponseID != "resp_123" {
		t.Fatalf("unexpected response message: %+v", msg)
	}
	if len(msg.Files) != 1 || msg.Files[0].URL != files[0].URL {
		t.Fatalf("files were not preserved: %+v", msg.Files)
	}
}

func TestNormalizeRealtimeEventUsage(t *testing.T) {
	msg := NormalizeRealtimeEvent(RealtimeEvent{
		Type: "token_usage",
		Data: []byte(`{"type":"token_usage","usage":{"input_tokens":3,"output_tokens":2}}`),
	})

	usage, ok := msg.Usage.(map[string]any)
	if !ok || usage["input_tokens"] != float64(3) || usage["output_tokens"] != float64(2) {
		t.Fatalf("usage was not extracted: %#v", msg.Usage)
	}
}

func TestNormalizeRealtimeEventUnknownTypeIsWrapped(t *testing.T) {
	msg := NormalizeRealtimeEvent(RealtimeEvent{Type: "provider.secret.event", Text: "payload"})

	if msg.Type != "realtime_event" || msg.Event != "provider.secret.event" {
		t.Fatalf("unknown event leaked or was dropped: %+v", msg)
	}
	if msg.Payload != "payload" {
		t.Fatalf("unknown event payload was not preserved: %#v", msg.Payload)
	}

	if _, err := json.Marshal(msg); err != nil {
		t.Fatalf("message is not serializable: %v", err)
	}
}

func TestNormalizeRealtimeEventMistralLegacyTranscript(t *testing.T) {
	msg := NormalizeRealtimeEvent(RealtimeEvent{Type: "transcript", Text: "готово"})

	if msg.Type != "transcript" || msg.Role != "user" || msg.Phase != "done" || msg.Text != "готово" {
		t.Fatalf("legacy transcript was not normalized: %+v", msg)
	}
}

func TestNormalizeRealtimeEventErrorUsesUnderlyingError(t *testing.T) {
	msg := NormalizeRealtimeEvent(RealtimeEvent{Type: "error", Err: errTestRealtime})

	if msg.Type != "error" || msg.Error != "transport failed" || msg.Text != "transport failed" {
		t.Fatalf("error was not preserved: %+v", msg)
	}
}

func TestNormalizeRealtimeEventLifecycle(t *testing.T) {
	tests := []struct {
		event RealtimeEvent
		type_ string
		phase string
	}{
		{RealtimeEvent{Type: "response_started"}, "response", "started"},
		{RealtimeEvent{Type: "response_done"}, "response", "done"},
		{RealtimeEvent{Type: "speech_started"}, "speech", "started"},
		{RealtimeEvent{Type: "speech_stopped"}, "speech", "stopped"},
	}
	for _, test := range tests {
		msg := NormalizeRealtimeEvent(test.event)
		if msg.Type != test.type_ || msg.Phase != test.phase {
			t.Errorf("%s normalized to %+v, want type=%q phase=%q", test.event.Type, msg, test.type_, test.phase)
		}
	}
}

func TestNormalizeRealtimeEventJSONContract(t *testing.T) {
	msg := NormalizeRealtimeEvent(RealtimeEvent{
		Type:       "response_text_delta",
		Text:       "Здрав",
		Delta:      "Здрав",
		ResponseID: "resp_1",
	})

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got["type"] != "transcript" || got["role"] != "assistant" || got["phase"] != "delta" {
		t.Fatalf("unexpected public identity: %s", data)
	}
	if got["delta"] != "Здрав" || got["text"] != "Здрав" || got["response_id"] != "resp_1" {
		t.Fatalf("text fields were not serialized: %s", data)
	}
	if _, ok := got["files"]; ok {
		t.Fatalf("empty files field must be omitted: %s", data)
	}
}

func TestNormalizeRealtimeEventDoneOmitsDelta(t *testing.T) {
	msg := NormalizeRealtimeEvent(RealtimeEvent{Type: "response_text_done", Text: "полный ответ"})
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got["phase"] != "done" || got["text"] != "полный ответ" {
		t.Fatalf("unexpected done message: %s", data)
	}
	if _, ok := got["delta"]; ok {
		t.Fatalf("done message must not contain delta: %s", data)
	}
}

func TestNormalizeRealtimeEventFunctionPayloadRemainsJSON(t *testing.T) {
	msg := NormalizeRealtimeEvent(RealtimeEvent{
		Type: "function_result",
		Data: []byte(`{"name":"get_weather","result":{"ok":true}}`),
	})

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	payload, ok := got["payload"].(map[string]any)
	if !ok || payload["name"] != "get_weather" {
		t.Fatalf("function payload was not preserved as JSON: %s", data)
	}
}

func TestNormalizeRealtimeEventSessionStarted(t *testing.T) {
	msg := NormalizeRealtimeEvent(RealtimeEvent{Type: "session_started"})
	if msg.Type != "session" || msg.Phase != "started" {
		t.Fatalf("session event was not normalized: %+v", msg)
	}
}

func TestMarshalRealtimeEventUsesPublicContract(t *testing.T) {
	data, err := MarshalRealtimeEvent(RealtimeEvent{
		Type:  "input_transcript_delta",
		Delta: "hello",
	})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["type"] != "transcript" || got["role"] != "user" || got["phase"] != "delta" {
		t.Fatalf("internal event leaked into public JSON: %s", data)
	}
}
