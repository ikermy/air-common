// Package elevenlabs is a leaf HTTP client for the ElevenLabs API.
//
// It intentionally does not import pkg/model so that it can be reused by the
// MCP server (air_orchestrator) and other services that need voice/music
// capabilities without pulling the whole model layer.
package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/comerrors"
	"github.com/ikermy/air-common/pkg/mode"
)

// Client is a minimal ElevenLabs REST client authenticated with a user API key.
type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient создаёт клиент для конкретного API-ключа.
func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:  strings.TrimSpace(apiKey),
		BaseURL: mode.ElevenLabsBaseURL,
		// Без общего Timeout: продолжительные операции (TTS streaming, Music)
		// должны ограничиваться контекстом вызывающей стороны.
		HTTPClient: &http.Client{
			Transport: &http.Transport{ResponseHeaderTimeout: 30 * time.Second},
		},
	}
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return mode.ElevenLabsBaseURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 30 * time.Second}}
}

func (c *Client) newRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, comerrors.NewProviderError(comdom.ProviderElevenLabs, resp.StatusCode, strings.TrimSpace(string(body)), nil)
	}
	return resp, nil
}

// doJSON выполняет запрос и разбирает JSON-ответ в out.
func (c *Client) doJSON(ctx context.Context, method, url string, payload any, out any) error {
	var (
		body io.Reader
		ct   string
	)
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
		ct = "application/json"
	}
	req, err := c.newRequest(ctx, method, url, body)
	if err != nil {
		return err
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := decodeJSON(resp.Body, out); err != nil {
		return fmt.Errorf("ошибка разбора ответа ElevenLabs: %w", err)
	}
	return nil
}

func decodeJSON(r io.Reader, out any) error {
	return json.NewDecoder(r).Decode(out)
}
