package outbox

import (
	"context"
	"testing"
	"time"
)

func TestEventValidate(t *testing.T) {
	valid := Event{
		AggregateType: "billing",
		AggregateID:   "42",
		EventType:     "payment.created",
		Payload:       []byte(`{"amount":10}`),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid event: %v", err)
	}

	cases := []struct {
		name  string
		event Event
	}{
		{"aggregate_type", Event{AggregateID: "1", EventType: "x", Payload: []byte(`{}`)}},
		{"aggregate_id", Event{AggregateType: "a", EventType: "x", Payload: []byte(`{}`)}},
		{"event_type", Event{AggregateType: "a", AggregateID: "1", Payload: []byte(`{}`)}},
		{"payload empty", Event{AggregateType: "a", AggregateID: "1", EventType: "x"}},
		{"payload invalid", Event{AggregateType: "a", AggregateID: "1", EventType: "x", Payload: []byte(`{`)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.event.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestDispatchRequiresHandler(t *testing.T) {
	s := &Store{}
	if _, err := s.Dispatch(context.Background(), 10, time.Minute, time.Minute, nil); err == nil {
		t.Fatal("expected error for nil handler")
	}
}
