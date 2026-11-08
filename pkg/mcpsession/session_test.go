package mcpsession

import (
	"strings"
	"testing"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
)

func TestSignParseRoundTrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	header := Sign("secret", 42, comdom.ProviderType(3), now)
	if !IsSigned(header) {
		t.Fatalf("header is not signed: %s", header)
	}
	uid, provider, err := Parse("secret", header, now)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if uid != 42 || provider != comdom.ProviderType(3) {
		t.Fatalf("unexpected values: uid=%d provider=%d", uid, provider)
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	header := Sign("secret", 42, comdom.ProviderType(3), now)
	if _, _, err := Parse("other", header, now); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestParseRejectsTamperedUser(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	header := Sign("secret", 42, comdom.ProviderType(3), now)
	tampered := strings.Replace(header, ":42:", ":43:", 1)
	if _, _, err := Parse("secret", tampered, now); err == nil {
		t.Fatal("expected signature error for tampered user")
	}
}

func TestParseRejectsExpired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	header := Sign("secret", 42, comdom.ProviderType(3), now)
	if _, _, err := Parse("secret", header, now.Add(TTL+time.Second)); err != ErrExpired {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}
