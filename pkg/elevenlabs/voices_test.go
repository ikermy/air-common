package elevenlabs

import (
	"encoding/json"
	"testing"
)

func TestVoiceFineTuning_UnmarshalJSON_StringState(t *testing.T) {
	var ft VoiceFineTuning
	if err := json.Unmarshal([]byte(`{"state":"fine_tuned","progress":100}`), &ft); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ft.State != "fine_tuned" {
		t.Fatalf("state=%q", ft.State)
	}
	if ft.Progress == nil || *ft.Progress != 100 {
		t.Fatalf("progress=%v", ft.Progress)
	}
}

func TestVoiceFineTuning_UnmarshalJSON_ObjectState(t *testing.T) {
	payload := `{"state":{"status":"fine_tuning","progress":42},"message":"ok"}`
	var ft VoiceFineTuning
	if err := json.Unmarshal([]byte(payload), &ft); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ft.State != "fine_tuning" {
		t.Fatalf("state=%q", ft.State)
	}
	if ft.Progress == nil || *ft.Progress != 42 {
		t.Fatalf("progress=%v", ft.Progress)
	}
	if ft.Message != "ok" {
		t.Fatalf("message=%q", ft.Message)
	}
}

func TestVoiceFineTuning_UnmarshalJSON_NullState(t *testing.T) {
	var ft VoiceFineTuning
	if err := json.Unmarshal([]byte(`{"state":null}`), &ft); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ft.State != "" {
		t.Fatalf("state=%q", ft.State)
	}
}
