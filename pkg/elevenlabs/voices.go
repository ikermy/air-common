package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// VoiceFineTuning — статус обучения PVC-голоса.
type VoiceFineTuning struct {
	State    string `json:"state"`
	Progress *int   `json:"progress,omitempty"`
	Message  string `json:"message,omitempty"`
}

// UnmarshalJSON терпимо разбирает fine_tuning: ElevenLabs может вернуть поля
// state/progress как строку/число, так и объектом (например
// {"state":{"status":"fine_tuned"},"progress":{"percent":42}}).
func (f *VoiceFineTuning) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		// null/строка/массив — считаем, что данных об обучении нет.
		return nil
	}
	raw := struct {
		State    json.RawMessage `json:"state"`
		Progress json.RawMessage `json:"progress"`
		Message  json.RawMessage `json:"message"`
	}{}
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil
	}
	f.Progress = decodeFineTuningProgress(raw.Progress)
	f.Message = decodeStringValue(raw.Message)
	f.State = decodeFineTuningState(raw.State, &f.Progress)
	return nil
}

// decodeStringValue извлекает строку из строки или объекта.
func decodeStringValue(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return ""
	}
	if trimmed[0] == '"' {
		var value string
		if json.Unmarshal(trimmed, &value) == nil {
			return value
		}
		return ""
	}
	var obj map[string]any
	if json.Unmarshal(trimmed, &obj) != nil {
		return ""
	}
	for _, key := range []string{"message", "text", "value", "status", "state"} {
		if value, ok := obj[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

// decodeFineTuningProgress извлекает число из числа, строки или объекта.
func decodeFineTuningProgress(raw json.RawMessage) *int {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	switch trimmed[0] {
	case '"':
		var str string
		if json.Unmarshal(trimmed, &str) == nil {
			if value, err := strconv.Atoi(strings.TrimSpace(str)); err == nil {
				return &value
			}
		}
		return nil
	case '{':
		var obj map[string]any
		if json.Unmarshal(trimmed, &obj) != nil {
			return nil
		}
		for _, key := range []string{"progress", "percent", "percentage", "value"} {
			switch value := obj[key].(type) {
			case float64:
				out := int(value)
				return &out
			case string:
				if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
					return &parsed
				}
			}
		}
		return nil
	default:
		var value float64
		if json.Unmarshal(trimmed, &value) == nil {
			out := int(value)
			return &out
		}
		return nil
	}
}

// decodeFineTuningState извлекает строковый статус из строки или объекта.
func decodeFineTuningState(raw json.RawMessage, progress **int) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return ""
	}
	if trimmed[0] == '"' {
		var value string
		if json.Unmarshal(trimmed, &value) == nil {
			return value
		}
		return ""
	}
	var obj map[string]any
	if json.Unmarshal(trimmed, &obj) != nil {
		return ""
	}
	if *progress == nil {
		if p, ok := obj["progress"].(float64); ok {
			value := int(p)
			*progress = &value
		}
	}
	for _, key := range []string{"status", "state", "value", "name", "result"} {
		if value, ok := obj[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

// Voice — профиль голоса ElevenLabs.
type Voice struct {
	VoiceID     string            `json:"voice_id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Description string            `json:"description"`
	PreviewURL  string            `json:"preview_url"`
	Labels      map[string]string `json:"labels"`
	FineTuning  *VoiceFineTuning  `json:"fine_tuning,omitempty"`
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

// CreatePVCVoice создаёт PVC-голос по метаданным (POST /v1/voices/pvc).
// language обязателен. Возвращает voice_id; образцы и обучение — отдельными вызовами.
func (c *Client) CreatePVCVoice(ctx context.Context, name, language, description string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("name обязателен")
	}
	if strings.TrimSpace(language) == "" {
		return "", fmt.Errorf("language обязателен для PVC")
	}
	payload := map[string]any{"name": name, "language": language}
	if description != "" {
		payload["description"] = description
	}
	var out struct {
		VoiceID string `json:"voice_id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL()+"/voices/pvc", payload, &out); err != nil {
		return "", err
	}
	return out.VoiceID, nil
}

// AddPVCSamples добавляет образцы к PVC-голосу (POST /v1/voices/pvc/{voice_id}/samples).
func (c *Client) AddPVCSamples(ctx context.Context, voiceID string, samples []VoiceSample) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(voiceID) == "" {
		return fmt.Errorf("voice_id обязателен")
	}
	if len(samples) == 0 {
		return fmt.Errorf("нужен хотя бы один аудио-образец")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for i, sample := range samples {
		fileName := sample.FileName
		if fileName == "" {
			fileName = fmt.Sprintf("sample_%d.wav", i)
		}
		part, err := writer.CreateFormFile("files", fileName)
		if err != nil {
			return err
		}
		if _, err := part.Write(sample.Data); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, c.baseURL()+"/voices/pvc/"+url.PathEscape(voiceID)+"/samples", &buf)
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

// TrainPVCVoice запускает обучение PVC-голоса (POST /v1/voices/pvc/{voice_id}/train).
func (c *Client) TrainPVCVoice(ctx context.Context, voiceID, modelID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(voiceID) == "" {
		return fmt.Errorf("voice_id обязателен")
	}
	payload := map[string]any{}
	if strings.TrimSpace(modelID) != "" {
		payload["model_id"] = modelID
	}
	return c.doJSON(ctx, http.MethodPost, c.baseURL()+"/voices/pvc/"+url.PathEscape(voiceID)+"/train", payload, nil)
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
