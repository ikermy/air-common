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

// MusicRequest — запрос генерации музыки.
type MusicRequest struct {
	Model             string `json:"model_id,omitempty"`
	Prompt            string `json:"prompt"`
	LengthMs          *int   `json:"music_length_ms,omitempty"`
	ForceInstrumental *bool  `json:"force_instrumental,omitempty"`
	Format            string `json:"-"`
}

// MusicResult — результат генерации музыки.
type MusicResult struct {
	Audio       io.ReadCloser
	ContentType string
	FileName    string
}

// GenerateMusic вызывает POST /v1/music и возвращает аудиопоток.
// Вызывающий обязан закрыть MusicResult.Audio.
func (c *Client) GenerateMusic(ctx context.Context, req MusicRequest) (MusicResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return MusicResult{}, fmt.Errorf("prompt не может быть пустым")
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = "eleven_music"
	}
	if strings.TrimSpace(req.Format) == "" {
		req.Format = "mp3_44100_128"
	}

	params := url.Values{}
	params.Set("output_format", req.Format)
	endpoint := c.baseURL() + "/music?" + params.Encode()

	httpReq, err := c.newRequest(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return MusicResult{}, err
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return MusicResult{}, err
	}
	httpReq.Body = io.NopCloser(bytes.NewReader(raw))
	httpReq.ContentLength = int64(len(raw))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/mpeg")

	resp, err := c.do(httpReq)
	if err != nil {
		return MusicResult{}, err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	return MusicResult{
		Audio:       resp.Body,
		ContentType: contentType,
		FileName:    "music." + formatExt(req.Format),
	}, nil
}

func formatExt(format string) string {
	switch {
	case strings.HasPrefix(format, "pcm"):
		return "pcm"
	case strings.HasPrefix(format, "ulaw"):
		return "ulaw"
	default:
		return "mp3"
	}
}
