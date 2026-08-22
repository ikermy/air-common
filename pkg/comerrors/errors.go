package comerrors

import (
	"fmt"
	"strings"

	"github.com/ikermy/air_common/pkg/model/commdom"
)

// ProviderErrorKind identifies the user-facing category of a provider error.
type ProviderErrorKind string

const (
	ProviderLimitErrorKind          ProviderErrorKind = "ai-provider-limit"
	ProviderAuthErrorKind           ProviderErrorKind = "ai-provider-auth"
	ProviderPermissionErrorKind     ProviderErrorKind = "ai-provider-permission"
	ProviderRequestErrorKind        ProviderErrorKind = "ai-provider-request"
	ProviderUnavailableErrorKind    ProviderErrorKind = "ai-provider-unavailable"
	ProviderContentBlockedErrorKind ProviderErrorKind = "ai-provider-content-blocked"
	ProviderTimeoutErrorKind        ProviderErrorKind = "ai-provider-timeout"
)

// ProviderError is the common error format for all AI providers.
type ProviderError struct {
	Provider   commdom.ProviderType
	Kind       ProviderErrorKind
	StatusCode int
	Message    string
	Err        error
}

func (e *ProviderError) Error() string {
	message := e.Message
	if message == "" && e.Err != nil {
		message = e.Err.Error()
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("%s provider error (HTTP %d): %s", e.Provider.String(), e.StatusCode, message)
	}
	return fmt.Sprintf("%s provider error: %s", e.Provider.String(), message)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// ProviderLimitError is kept as a compatibility alias.
type ProviderLimitError = ProviderError

func NewProviderError(provider commdom.ProviderType, statusCode int, message string, err error) *ProviderError {
	return &ProviderError{Provider: provider, Kind: ClassifyProviderError(statusCode, message), StatusCode: statusCode, Message: message, Err: err}
}

func NewProviderTransportError(provider commdom.ProviderType, err error) *ProviderError {
	return &ProviderError{Provider: provider, Kind: ClassifyProviderError(0, err.Error()), Message: err.Error(), Err: err}
}

func ClassifyProviderError(statusCode int, message string) ProviderErrorKind {
	text := strings.ToLower(message)
	switch {
	case statusCode == 401 || strings.Contains(text, "invalid api key") || strings.Contains(text, "unauthorized"):
		return ProviderAuthErrorKind
	case statusCode == 403 || strings.Contains(text, "forbidden") || strings.Contains(text, "permission"):
		return ProviderPermissionErrorKind
	case statusCode == 408 || strings.Contains(text, "timeout") || strings.Contains(text, "deadline exceeded"):
		return ProviderTimeoutErrorKind
	case statusCode == 429 || strings.Contains(text, "rate limit") || strings.Contains(text, "quota") || strings.Contains(text, "billing") || strings.Contains(text, "insufficient balance"):
		return ProviderLimitErrorKind
	case statusCode >= 500 && statusCode <= 599 || strings.Contains(text, "temporarily unavailable") || strings.Contains(text, "service unavailable"):
		return ProviderUnavailableErrorKind
	case strings.Contains(text, "safety") || strings.Contains(text, "content policy") || strings.Contains(text, "blocked"):
		return ProviderContentBlockedErrorKind
	default:
		return ProviderRequestErrorKind
	}
}

/*
ProviderLimitError
ProviderAuthError
ProviderPermissionError
ProviderRequestError
ProviderUnavailableError
ProviderContentBlockedError
ProviderTimeoutError
*/
