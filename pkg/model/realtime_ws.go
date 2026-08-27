package model

import "encoding/json"

// RealtimeWSMessage is the provider-independent realtime message sent to a
// WebSocket client. RealtimeEvent remains the internal provider-facing type.
type RealtimeWSMessage struct {
	Type       string `json:"type"`
	Role       string `json:"role,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Delta      string `json:"delta,omitempty"`
	Text       string `json:"text,omitempty"`
	ResponseID string `json:"response_id,omitempty"`
	Files      []File `json:"files,omitempty"`
	Usage      any    `json:"usage,omitempty"`
	Error      string `json:"error,omitempty"`
	Event      string `json:"event,omitempty"`
	Payload    any    `json:"payload,omitempty"`
}

// NormalizeRealtimeEvent converts an internal event to the stable client
// contract. Unknown types use a safe envelope instead of leaking provider
// event names into the public API.
func NormalizeRealtimeEvent(ev RealtimeEvent) RealtimeWSMessage {
	msg := RealtimeWSMessage{ResponseID: ev.ResponseID, Files: ev.Files}
	delta := ev.Delta
	if delta == "" {
		delta = ev.Text
	}

	switch ev.Type {
	case "session_started":
		msg.Type, msg.Phase = "session", "started"
	case "transcript":
		// Legacy Mistral STT event: it is a final user transcript.
		msg.Type, msg.Role, msg.Phase, msg.Text = "transcript", "user", "done", ev.Text
	case "input_transcript_delta":
		msg.Type, msg.Role, msg.Phase, msg.Delta, msg.Text = "transcript", "user", "delta", delta, ev.Text
	case "input_transcript_done":
		msg.Type, msg.Role, msg.Phase, msg.Text = "transcript", "user", "done", ev.Text
	case "response_text_delta":
		msg.Type, msg.Role, msg.Phase, msg.Delta, msg.Text = "transcript", "assistant", "delta", delta, ev.Text
	case "response_text_done":
		msg.Type, msg.Role, msg.Phase, msg.Text = "transcript", "assistant", "done", ev.Text
	case "response_started":
		msg.Type, msg.Phase = "response", "started"
	case "response_done":
		msg.Type, msg.Phase = "response", "done"
	case "speech_started":
		msg.Type, msg.Phase = "speech", "started"
	case "speech_stopped":
		msg.Type, msg.Phase = "speech", "stopped"
	case "error":
		msg.Type, msg.Error, msg.Text = "error", ev.Text, ev.Text
		if msg.Error == "" && ev.Err != nil {
			msg.Error, msg.Text = ev.Err.Error(), ev.Err.Error()
		}
		if len(ev.Data) > 0 {
			msg.Payload = json.RawMessage(ev.Data)
		}
	case "token_usage":
		msg.Type = "token_usage"
		var data struct {
			Usage any `json:"usage"`
		}
		if json.Unmarshal(ev.Data, &data) == nil {
			msg.Usage = data.Usage
		} else if len(ev.Data) > 0 {
			// Do not discard provider usage data merely because its envelope
			// differs from the currently supported shape.
			msg.Payload = json.RawMessage(ev.Data)
		}
	case "interrupted", "audio_start", "audio_end":
		msg.Type, msg.Text = ev.Type, ev.Text
	case "function_result":
		msg.Type, msg.Text = ev.Type, ev.Text
		if len(ev.Data) > 0 {
			msg.Payload = json.RawMessage(ev.Data)
		}
	default:
		msg.Type, msg.Event, msg.Payload = "realtime_event", ev.Type, ev.Text
		if len(ev.Data) > 0 {
			msg.Payload = json.RawMessage(ev.Data)
		}
	}
	return msg
}

// MarshalRealtimeEvent is the boundary helper for WebSocket handlers. It
// guarantees that the internal RealtimeEvent is normalized before encoding;
// callers must not marshal RealtimeEvent directly.
func MarshalRealtimeEvent(ev RealtimeEvent) ([]byte, error) {
	return json.Marshal(NormalizeRealtimeEvent(ev))
}
