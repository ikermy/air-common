package elevenlabs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Voice — профиль голоса ElevenLabs.
type Voice struct {
	VoiceID     string            `json:"voice_id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Description string            `json:"description"`
	PreviewURL  string            `json:"preview_url"`
	Labels      map[string]string `json:"labels"`
}

type voiceListResponse struct {
	Voices []Voice `json:"voices"`
}

// VoiceSample — аудио-образец для клонирования.
type VoiceSample struct {
	FileName    string
	ContentType string
	Data        []byte
}

// ListVoices возвращает голоса аккаунта (GET /v2/voices).
func (c *Client) ListVoices(ctx context.Context, limit, offset int) ([]Voice, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 100
	}
	params := url.Values{}
	params.Set("page_size", strconv.Itoa(limit))
	if offset > 0 {
		params.Set("start_after", strconv.Itoa(offset))
	}
	endpoint := c.baseURL() + "/voices?" + params.Encode()

	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var out voiceListResponse
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("ошибка разбора списка голосов ElevenLabs: %w", err)
	}
	return out.Voices, nil
}

// GetVoice возвращает голос по ID (GET /v1/voices/{voice_id}).
func (c *Client) GetVoice(ctx context.Context, voiceID string) (Voice, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var out Voice
	err := c.doJSON(ctx, http.MethodGet, c.baseURL()+"/voices/"+url.PathEscape(voiceID), nil, &out)
	return out, err
}

// DeleteVoice удаляет голос (DELETE /v1/voices/{voice_id}).
func (c *Client) DeleteVoice(ctx context.Context, voiceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(voiceID) == "" {
		return fmt.Errorf("voice_id обязателен")
	}
	req, err := c.newRequest(ctx, http.MethodDelete, c.baseURL()+"/voices/"+url.PathEscape(voiceID), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// CreateVoice создаёт мгновенный клон голоса (POST /v1/voices/add, multipart).
func (c *Client) CreateVoice(ctx context.Context, name, description string, samples []VoiceSample) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("name обязателен")
	}
	if len(samples) == 0 {
		return "", fmt.Errorf("нужен хотя бы один аудио-образец")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("name", name); err != nil {
		return "", err
	}
	if description != "" {
		if err := writer.WriteField("description", description); err != nil {
			return "", err
		}
	}
	for i, sample := range samples {
		fileName := sample.FileName
		if fileName == "" {
			fileName = fmt.Sprintf("sample_%d.wav", i)
		}
		part, err := writer.CreateFormFile("files", fileName)
		if err != nil {
			return "", err
		}
		if _, err := part.Write(sample.Data); err != nil {
			return "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, c.baseURL()+"/voices/add", &buf)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		VoiceID string `json:"voice_id"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return "", fmt.Errorf("ошибка разбора ответа создания голоса: %w", err)
	}
	return out.VoiceID, nil
}

// EditVoice обновляет имя и описание голоса (POST /v1/voices/{voice_id}/edit).
func (c *Client) EditVoice(ctx context.Context, voiceID, name, description string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(voiceID) == "" {
		return fmt.Errorf("voice_id обязателен")
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if name != "" {
		if err := writer.WriteField("name", name); err != nil {
			return err
		}
	}
	if description != "" {
		if err := writer.WriteField("description", description); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, c.baseURL()+"/voices/"+url.PathEscape(voiceID)+"/edit", &buf)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// GetVoiceSample возвращает превью-аудио голоса.
func (c *Client) GetVoiceSample(ctx context.Context, voiceID string) (io.ReadCloser, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := c.newRequest(ctx, http.MethodGet, c.baseURL()+"/voices/"+url.PathEscape(voiceID)+"/sample", nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "audio/mpeg")
	resp, err := c.do(req)
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	return resp.Body, contentType, nil
}
