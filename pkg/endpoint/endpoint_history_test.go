package endpoint

import (
	"testing"

	"github.com/ikermy/air-common/pkg/comdom"
)

func TestParseDialogHistoryPayload(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		wantLen  int
		wantText string
	}{
		{
			name:     "envelope with object messages",
			payload:  `{"Data":[{"creator":1,"message":{"message":"hello"},"timestamp":"2026-01-01T00:00:00Z"}],"Type":"0"}`,
			wantLen:  1,
			wantText: "hello",
		},
		{
			name:     "envelope with string messages",
			payload:  `{"Data":["{\"creator\":2,\"message\":\"hi\",\"timestamp\":\"2026-01-01T00:00:01Z\"}"]}`,
			wantLen:  1,
			wantText: "hi",
		},
		{
			name:     "bare array",
			payload:  `[{"creator":1,"message":{"message":"bare"},"timestamp":"2026-01-01T00:00:02Z"}]`,
			wantLen:  1,
			wantText: "bare",
		},
		{
			name:     "empty",
			payload:  `{"Data":[]}`,
			wantLen:  0,
			wantText: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDialogHistoryPayload([]byte(tc.payload))
			if err != nil {
				t.Fatalf("parseDialogHistoryPayload: %v", err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tc.wantLen)
			}
			if tc.wantLen > 0 && got[0].Message.Message != tc.wantText {
				t.Fatalf("message = %q, want %q", got[0].Message.Message, tc.wantText)
			}
		})
	}
}

func TestParseDialogHistoryPayloadCreator(t *testing.T) {
	got, err := parseDialogHistoryPayload([]byte(
		`{"Data":[{"creator":"2","message":"voice","timestamp":"2026-01-01T00:00:00Z"}]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 || got[0].Creator != comdom.User {
		t.Fatalf("creator = %v, want %v", got[0].Creator, comdom.User)
	}
}
