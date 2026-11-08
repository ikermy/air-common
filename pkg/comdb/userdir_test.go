package comdb

import (
	"strings"
	"testing"
)

func TestDecodeEmailHash(t *testing.T) {
	encoded := strings.Repeat("ab", 32)
	decoded, err := DecodeEmailHash(encoded)
	if err != nil {
		t.Fatalf("DecodeEmailHash: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("len=%d want 32", len(decoded))
	}

	if _, err := DecodeEmailHash("zz"); err == nil {
		t.Fatal("expected error for non-hex input")
	}
	if _, err := DecodeEmailHash("abcd"); err == nil {
		t.Fatal("expected error for short hash")
	}
}
