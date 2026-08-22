package startpoint

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ikermy/air_common/pkg/com"
	"github.com/ikermy/air_common/pkg/comerrors"
	"github.com/ikermy/air_common/pkg/mode"
	"github.com/ikermy/air_common/pkg/model"
	"github.com/ikermy/air_common/pkg/model/commdom"
)

// RetryableError представляет временную ошибку, которую можно повторить
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// FatalError представляет критическую ошибку, требующую завершения
type FatalError struct {
	Err error
}

func (e *FatalError) Error() string {
	return e.Err.Error()
}

func (e *FatalError) Unwrap() error {
	return e.Err
}

// NonCriticalError представляет некритическую ошибку, не требующую завершения
type NonCriticalError struct {
	Err error
}

func (e *NonCriticalError) Error() string {
	return e.Err.Error()
}

func (e *NonCriticalError) Unwrap() error {
	return e.Err
}

// ProviderLimitError представляет ошибку превышения лимита/квоты/подписки AI-провайдера
// (429 Too Many Requests, rate limit exceeded, quota exceeded, billing errors и т.п.)
type ProviderLimitError = comerrors.ProviderLimitError

// IsFatalError проверяет, является ли ошибка критической
func IsFatalError(err error) bool {
	var fatalErr *FatalError
	return errors.As(err, &fatalErr)
}

// IsNonCriticalError проверяет, является ли ошибка некритической
func IsNonCriticalError(err error) bool {
	var nonCritErr *NonCriticalError
	return errors.As(err, &nonCritErr)
}

// IsProviderLimitError проверяет, является ли ошибка лимитной ошибкой AI-провайдера
func IsProviderLimitError(err error) bool {
	var limitErr *ProviderLimitError
	return errors.As(err, &limitErr) || isProviderLimitError(err)
}

// isProviderLimitError проверяет, связана ли ошибка с превышением лимита/квоты/подписки AI-провайдера
// (429 Too Many Requests, rate limit exceeded, quota exceeded, billing errors и т.п.)
func isProviderLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	limitPatterns := []string{
		"429 too many requests",
		"status 429",
		"http 429",
		"error 429",
		"статус 429",
		"http статус 429",
		"rate limit",
		"rate_limit",
		"quota exceeded",
		"quota_exceeded",
		"insufficient_quota",
		"insufficient quota",
		"billing issue",
		"billing error",
		"payment required",
		"subscription required",
		"resource exhausted",
	}
	for _, pattern := range limitPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

func isProviderResponseError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, pattern := range []string{"status 400", "status 401", "status 403", "status 408", "status 429", "status 500", "status 502", "status 503", "status 504", "http 400", "http 401", "http 403", "http 408", "http 429", "http 500", "http 502", "http 503", "http 504", "invalid api key", "unauthorized", "forbidden", "timeout", "safety", "content policy", "rate limit", "quota", "billing"} {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

// isFatalErrorPattern проверяет паттерны критических ошибок (auth)
func isFatalErrorPattern(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	fatalPatterns := []string{
		"401", "403",
		"Unauthorized",
		"Forbidden",
		"invalid API key",
	}
	for _, pattern := range fatalPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// isRetryableErrorPattern проверяет паттерны временных ошибок (5xx, сетевые)
func isRetryableErrorPattern(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	retryablePatterns := []string{
		"500", "502", "503", "504",
		"Service Unavailable",
		"Bad Gateway",
		"Gateway Timeout",
		"Internal Server Error",
		"upstream connect error",
		"connection reset",
		"connection refused",
		"connection termination",
		"timeout",
		"temporary failure",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// AskWithRetry выполняет запрос к модели с retry-логикой
func (s *Start) AskWithRetry(userID uint32, provider commdom.ProviderType, respId, dialogID uint64, arrAsk []string, files ...model.FileUpload) (model.AssistResponse, error) {
	var lastErr error

	for attempt := 0; attempt < mode.RetryMaxAttempts; attempt++ {
		response, err := s.ask(userID, respId, dialogID, arrAsk, files...)

		if err == nil {
			return response, nil
		}

		lastErr = err

		// Лимитная ошибка провайдера (429, rate limit, quota, billing) — немедленный возврат
		if isProviderResponseError(err) {
			//logger.Warn("Лимитная ошибка провайдера для диалога %d: %v", dialogID, err)
			return response, &ProviderLimitError{
				Provider: provider,
				Kind:     comerrors.ClassifyProviderError(0, err.Error()),
				Message:  err.Error(),
			}
		}

		// Критическая ошибка — немедленный возврат
		if isFatalErrorPattern(err) {
			message := s.End.TranslateMessageWithUserID(userID, "event.model-fatal")
			select {
			case mode.GetCarpinteroChannel() <- com.CarpCh{
				Event:  "model-fatal",
				Target: message,
				UserID: userID,
			}:
			default:
			}
			//logger.Warn("Критическая ошибка для диалога %d: %v", dialogID, err)
			return response, &FatalError{Err: fmt.Errorf("критическая ошибка: %w", err)}
		}

		// Временная ошибка — retry
		if isRetryableErrorPattern(err) {
			if attempt == mode.RetryMaxAttempts-1 {
				message := s.End.TranslateMessageWithUserID(userID, "event.model-retry-exhausted")
				select {
				case mode.GetCarpinteroChannel() <- com.CarpCh{
					Event:  "model-retry-exhausted",
					Target: message,
					UserID: userID,
				}:
				default:
				}
				break
			}

			delay := time.Duration(mode.RetryBaseDelay) * time.Second * time.Duration(math.Pow(2, float64(attempt)))
			//logger.Debug("Retry attempt %d/%d for dialog %d, waiting %v", attempt+1, mode.RetryMaxAttempts, dialogID, delay)

			select {
			case <-s.ctx.Done():
				return model.AssistResponse{}, &NonCriticalError{Err: s.ctx.Err()}
			case <-time.After(delay):
			}
			continue
		}

		// Некритическая ошибка (400, 404, context canceled и др.) — сразу возвращаем
		//logger.Debug("Non-critical error for dialog %d: %v", dialogID, err)
		return response, &NonCriticalError{Err: err}
	}

	// Все retry исчерпаны
	//logger.Warn("Все %d попыток неуспешны для диалога %d", mode.RetryMaxAttempts, dialogID)
	return model.AssistResponse{}, &NonCriticalError{Err: fmt.Errorf("все %d попыток неуспешны: %w", mode.RetryMaxAttempts, lastErr)}
}
