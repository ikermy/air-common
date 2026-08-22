package create

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateResponseInternalFunctionCallOutput(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("OpenAI-Beta") != "" {
			t.Errorf("unexpected OpenAI-Beta header: %q", r.Header.Get("OpenAI-Beta"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, payload)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			_, _ = w.Write([]byte(strings.Join([]string{
				`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get_weather","arguments":""}}`,
				`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"city\":\"Buenos Aires\"}"}`,
				`data: {"type":"response.function_call_arguments.done","output_index":0,"arguments":"{\"city\":\"Buenos Aires\"}"}`,
				`data: {"type":"response.completed","response":{"usage":{"input_tokens":1}}}`,
				`data: [DONE]`,
			}, "\n")))
			return
		}
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"22C\"}\ndata: {\"type\":\"response.completed\",\"response\":{}}\ndata: [DONE]\n"))
	}))
	defer server.Close()

	client := &OpenAIAgentClient{url: server.URL, httpClient: server.Client()}
	var calls []map[string]any
	result, text, err := client.createResponseInternal(context.Background(), "weather?", map[string]any{"model_name": "test-model"}, nil,
		func(items []any) ([]any, error) {
			call := items[0].(map[string]any)
			calls = append(calls, call)
			return []any{map[string]any{"call_id": "call_1", "content": `{"temperature":"22C"}`}}, nil
		}, 0, 0)
	if err != nil {
		t.Fatalf("createResponseInternal: %v", err)
	}
	if result == nil || text != "22C" || len(calls) != 1 {
		t.Fatalf("unexpected result: result=%v text=%q calls=%d", result, text, len(calls))
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}

	input, ok := requests[1]["input"].([]any)
	if !ok || len(input) != 3 {
		t.Fatalf("second input = %#v, want 3 input items", requests[1]["input"])
	}
	callItem := input[1].(map[string]any)
	if callItem["arguments"] != `{"city":"Buenos Aires"}` {
		t.Errorf("function arguments = %v", callItem["arguments"])
	}
	outputItem := input[2].(map[string]any)
	if outputItem["type"] != "function_call_output" || outputItem["call_id"] != "call_1" {
		t.Errorf("output item = %#v", outputItem)
	}
}
