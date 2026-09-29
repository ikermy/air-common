package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SynthesizeRequest — запрос синтеза речи.
type SynthesizeRequest struct {
	Model         string
	Text          string
	VoiceID       string
	Format        string
	Language      string
	VoiceSettings map[string]any
	PreviousText  string
	NextText      string
	Seed          *int
}

type ttsBody struct {
	Text          string         `json:"text"`
	ModelID       string         `json:"model_id,omitempty"`
	LanguageCode  string         `json:"language_code,omitempty"`
	VoiceSettings map[string]any `json:"voice_settings,omitempty"`
	PreviousText  string         `json:"previous_text,omitempty"`
	NextText      string         `json:"next_text,omitempty"`
	Seed          *int           `json:"seed,omitempty"`
}

// Synthesize выполняет batch-синтез (POST /v1/text-to-speech/{voice_id}).
// Возвращает аудиопоток и content-type; вызывающий обязан закрыть поток.
func (c *Client) Synthesize(ctx context.Context, req SynthesizeRequest) (io.ReadCloser, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(req.VoiceID) == "" {
		return nil, "", fmt.Errorf("voice_id обязателен")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, "", fmt.Errorf("text обязателен")
	}
	format := req.Format
	if format == "" {
		format = "pcm_16000"
	}

	params := url.Values{}
	params.Set("output_format", format)
	endpoint := fmt.Sprintf("%s/text-to-speech/%s?%s", c.baseURL(), url.PathEscape(req.VoiceID), params.Encode())

	raw, err := json.Marshal(ttsBody{
		Text:          req.Text,
		ModelID:       req.Model,
		LanguageCode:  req.Language,
		VoiceSettings: req.VoiceSettings,
		PreviousText:  req.PreviousText,
		NextText:      req.NextText,
		Seed:          req.Seed,
	})
	if err != nil {
		return nil, "", err
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/*")

	resp, err := c.do(httpReq)
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return resp.Body, contentType, nil
}

// SynthesizeStream синтезирует речь через /stream и отдаёт аудио-чанки.
// Канал закрывается по завершении ответа или отмене контекста.
func (c *Client) SynthesizeStream(ctx context.Context, req SynthesizeRequest) (<-chan []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(req.VoiceID) == "" {
		return nil, fmt.Errorf("voice_id обязателен")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, fmt.Errorf("text обязателен")
	}
	format := req.Format
	if format == "" {
		format = "pcm_16000"
	}

	params := url.Values{}
	params.Set("output_format", format)
	endpoint := fmt.Sprintf("%s/text-to-speech/%s/stream?%s", c.baseURL(), url.PathEscape(req.VoiceID), params.Encode())

	raw, err := json.Marshal(ttsBody{
		Text:          req.Text,
		ModelID:       req.Model,
		LanguageCode:  req.Language,
		VoiceSettings: req.VoiceSettings,
		PreviousText:  req.PreviousText,
		NextText:      req.NextText,
		Seed:          req.Seed,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/*")

	resp, err := c.do(httpReq)
	if err != nil {
		return nil, err
	}

	out := make(chan []byte, 16)
	go func() {
		defer close(out)
		defer func() { _ = resp.Body.Close() }()
		buf := make([]byte, 8192)
		// PCM16-поток режем по границе 16-битного сэмпла: клиенты конвертируют
		// каждый чанк в Int16Array независимо, нечётная длина ломает выравнивание
		// и даёт щелчки/искажения.
		var carry byte
		hasCarry := false
		emit := func(data []byte) bool {
			if len(data) == 0 {
				return true
			}
			chunk := make([]byte, len(data))
			copy(chunk, data)
			select {
			case out <- chunk:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				data := buf[:n]
				if hasCarry {
					aligned := make([]byte, 0, len(data)+1)
					aligned = append(aligned, carry)
					aligned = append(aligned, data...)
					data = aligned
					hasCarry = false
				}
				if len(data)%2 != 0 {
					carry = data[len(data)-1]
					hasCarry = true
					data = data[:len(data)-1]
				}
				if !emit(data) {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()
	return out, nil
}
