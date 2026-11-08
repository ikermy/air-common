package comdom

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDomainIDConversions(t *testing.T) {
	if got := UserIDFromUint64(42).Uint64(); got != 42 {
		t.Fatalf("UserID round-trip = %d, want 42", got)
	}
	if got := DialogIDFromUint64(43).Uint64(); got != 43 {
		t.Fatalf("DialogID round-trip = %d, want 43", got)
	}
	if got := ResponderIDFromUint64(44).Uint64(); got != 44 {
		t.Fatalf("ResponderID round-trip = %d, want 44", got)
	}
	if got := MessageIDFromUint64(45).Uint64(); got != 45 {
		t.Fatalf("MessageID round-trip = %d, want 45", got)
	}
	if got := BotIDFromUint64(46).Uint64(); got != 46 {
		t.Fatalf("BotID round-trip = %d, want 46", got)
	}
	if got := ChannelIDFromUint64(47).Uint64(); got != 47 {
		t.Fatalf("ChannelID round-trip = %d, want 47", got)
	}
}

func TestDomainIDValidate(t *testing.T) {
	if err := UserID(0).Validate(); !errors.Is(err, ErrZeroID) {
		t.Fatalf("zero UserID Validate() = %v, want ErrZeroID", err)
	}
	if err := DialogID(1).Validate(); err != nil {
		t.Fatalf("non-zero DialogID Validate() = %v", err)
	}
	if !BotID(0).IsZero() || BotID(1).IsZero() {
		t.Fatal("BotID IsZero() is incorrect")
	}
}

func TestDomainIDString(t *testing.T) {
	if got := UserID(123).String(); got != "123" {
		t.Fatalf("UserID.String() = %q, want 123", got)
	}
}

func TestDomainIDJSONStaysNumeric(t *testing.T) {
	payload, err := json.Marshal(struct {
		User UserID   `json:"user"`
		Dlg  DialogID `json:"dialog"`
	}{User: 7, Dlg: 8})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"user":7,"dialog":8}` {
		t.Fatalf("JSON = %s, want numeric ids", payload)
	}
}
