// Package mcpsession implements the signed X-Session-ID header used between
// AI workers and the MCP server.
//
// Format: "v1:<userId>:<providerType>:<expUnix>:<hexHMAC-SHA256>".
// The HMAC covers "v1:<userId>:<providerType>:<expUnix>" with a shared secret.
package mcpsession

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
)

// Version is the signed header version prefix.
const Version = "v1"

// TTL is the default lifetime of a signed session header.
const TTL = 5 * time.Minute

// ErrExpired is returned when a signed session header is past its expiry.
var ErrExpired = errors.New("session token expired")

// Sign builds a signed X-Session-ID header value.
func Sign(secret string, userID uint64, provider comdom.ProviderType, now time.Time) string {
	exp := now.Add(TTL).Unix()
	msg := fmt.Sprintf("%s:%d:%d:%d", Version, userID, provider, exp)
	return msg + ":" + signature(secret, msg)
}

// Parse validates a signed X-Session-ID header value.
func Parse(secret, header string, now time.Time) (uint64, comdom.ProviderType, error) {
	parts := strings.Split(header, ":")
	if len(parts) != 5 || parts[0] != Version {
		return 0, 0, fmt.Errorf("invalid signed session format")
	}

	uid, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid userId: %w", err)
	}
	pt, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid providerType: %w", err)
	}
	exp, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid expiry: %w", err)
	}

	provided, err := hex.DecodeString(parts[4])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid signature: %w", err)
	}

	msg := strings.Join(parts[:4], ":")
	expected, err := hex.DecodeString(signature(secret, msg))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid signature: %w", err)
	}
	if !hmac.Equal(expected, provided) {
		return 0, 0, fmt.Errorf("invalid session signature")
	}
	if now.Unix() > exp {
		return 0, 0, ErrExpired
	}

	return uid, comdom.ProviderType(pt), nil
}

// IsSigned reports whether the header looks like a signed session value.
func IsSigned(header string) bool {
	return strings.HasPrefix(header, Version+":")
}

func signature(secret, msg string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}
