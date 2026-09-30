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
	"time"
)

// MusicChunk — секция composition_plan (v2/v2_5).
type MusicChunk struct {
	Text           string   `json:"text"`
	DurationMs     int      `json:"duration_ms"`
	PositiveStyles []string `json:"positive_styles,omitempty"`
	NegativeStyles []string `json:"negative_styles,omitempty"`
}

// MusicCompositionPlan — детализированный план композиции.
type MusicCompositionPlan struct {
	Chunks []MusicChunk `json:"chunks"`
}

// MusicRequest — запрос генерации музыки (POST /v1/music).
//
// Prompt и CompositionPlan взаимоисключающие: должен быть задан ровно один.
// - LengthMs / ForceInstrumental — только с Prompt;
// - Seed — только с CompositionPlan.
type MusicRequest struct {
	Prompt            string
	CompositionPlan   *MusicCompositionPlan
	LengthMs          *int
	ForceInstrumental *bool
	Seed              *int
	Model             string // music_v1 | music_v2 | music_v2_5
	Format            string // output_format (query), напр. mp3_44100_128
}

// MusicResult — результат генерации музыки.
type MusicResult struct {
	Audio       io.ReadCloser
	ContentType string
	FileName    string
}

const (
	musicPromptMaxLen     = 4100
	musicLengthMinMs      = 3000
	musicLengthMaxMs      = 600000
	musicChunkMinMs       = 3000
	musicChunkMaxMs       = 120000
	musicChunkMaxCount    = 30
	musicSeedMax          = 2147483647
	musicPollInterval     = 5 * time.Second
	musicPollDefaultLimit = 5 * time.Minute
)

// ValidateMusicRequest проверяет запрос по правилам ElevenLabs Music API.
func ValidateMusicRequest(req MusicRequest) error {
	hasPrompt := strings.TrimSpace(req.Prompt) != ""
	hasPlan := req.CompositionPlan != nil && len(req.CompositionPlan.Chunks) > 0
	switch {
	case hasPrompt && hasPlan:
		return fmt.Errorf("prompt и composition_plan взаимоисключающие")
	case !hasPrompt && !hasPlan:
		return fmt.Errorf("нужен prompt или composition_plan")
	}
	if hasPrompt {
		if len([]rune(req.Prompt)) > musicPromptMaxLen {
			return fmt.Errorf("prompt превышает %d символов", musicPromptMaxLen)
		}
		if req.Seed != nil {
			return fmt.Errorf("seed нельзя использовать вместе с prompt")
		}
		if req.LengthMs != nil && (*req.LengthMs < musicLengthMinMs || *req.LengthMs > musicLengthMaxMs) {
			return fmt.Errorf("music_length_ms должен быть в диапазоне %d..%d", musicLengthMinMs, musicLengthMaxMs)
		}
	} else {
		chunks := req.CompositionPlan.Chunks
		if len(chunks) > musicChunkMaxCount {
			return fmt.Errorf("composition_plan: не более %d chunks", musicChunkMaxCount)
		}
		for i, chunk := range chunks {
			if chunk.DurationMs < musicChunkMinMs || chunk.DurationMs > musicChunkMaxMs {
				return fmt.Errorf("composition_plan: duration_ms chunk #%d должен быть %d..%d", i+1, musicChunkMinMs, musicChunkMaxMs)
			}
		}
		if req.LengthMs != nil || req.ForceInstrumental != nil {
			return fmt.Errorf("music_length_ms/force_instrumental нельзя использовать с composition_plan")
		}
	}
	if req.Seed != nil && (*req.Seed < 0 || *req.Seed > musicSeedMax) {
		return fmt.Errorf("seed должен быть в диапазоне 0..%d", musicSeedMax)
	}
	return nil
}

// GenerateMusic генерирует музыку. API может ответить синхронно (200 + аудио)
// либо асинхронно (202 + request_id/polling_url) — поддерживаются оба варианта.
// Вызывающий обязан закрыть MusicResult.Audio.
func (c *Client) GenerateMusic(ctx context.Context, req MusicRequest) (MusicResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ValidateMusicRequest(req); err != nil {
		return MusicResult{}, err
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = "music_v1"
	}
	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = "auto"
	}

	body := map[string]any{"model_id": req.Model}
	if strings.TrimSpace(req.Prompt) != "" {
		body["prompt"] = req.Prompt
		if req.LengthMs != nil {
			body["music_length_ms"] = *req.LengthMs
		}
		if req.ForceInstrumental != nil {
			body["force_instrumental"] = *req.ForceInstrumental
		}
	}
	if req.CompositionPlan != nil {
		body["composition_plan"] = req.CompositionPlan
		if req.Seed != nil {
			body["seed"] = *req.Seed
		}
	}

	params := url.Values{}
	params.Set("output_format", format)
	endpoint := c.baseURL() + "/music?" + params.Encode()

	raw, err := json.Marshal(body)
	if err != nil {
		return MusicResult{}, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return MusicResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/*, application/json")

	resp, err := c.do(httpReq)
	if err != nil {
		return MusicResult{}, err
	}

	// Синхронный ответ с аудио.
	if isAudioResponse(resp) {
		return MusicResult{
			Audio:       resp.Body,
			ContentType: contentTypeOr(resp, "audio/mpeg"),
			FileName:    "music." + formatExt(format),
		}, nil
	}

	// Асинхронный ответ: JSON с request_id / polling_url.
	var job struct {
		RequestID  string `json:"request_id"`
		PollingURL string `json:"polling_url"`
		Status     string `json:"status"`
		AudioURL   string `json:"audio_url"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&job)
	_ = resp.Body.Close()

	if strings.TrimSpace(job.AudioURL) != "" && isTerminalCompleted(job.Status) {
		return c.fetchMusicAudio(ctx, job.AudioURL, format)
	}
	if strings.TrimSpace(job.PollingURL) == "" {
		return MusicResult{}, fmt.Errorf("ElevenLabs Music: неожиданный ответ без polling_url")
	}
	return c.pollMusic(ctx, job.PollingURL, format)
}

func (c *Client) pollMusic(ctx context.Context, pollingURL, format string) (MusicResult, error) {
	deadline := time.Now().Add(musicPollDefaultLimit)
	for {
		if ctx.Err() != nil {
			return MusicResult{}, ctx.Err()
		}
		status, audioURL, err := c.musicStatus(ctx, pollingURL)
		if err != nil {
			return MusicResult{}, err
		}
		switch strings.ToUpper(strings.TrimSpace(status)) {
		case "COMPLETED":
			if strings.TrimSpace(audioURL) != "" {
				return c.fetchMusicAudio(ctx, audioURL, format)
			}
			return MusicResult{}, fmt.Errorf("ElevenLabs Music: COMPLETED без audio_url")
		case "FAILED":
			return MusicResult{}, fmt.Errorf("ElevenLabs Music: генерация не удалась")
		}
		if time.Now().After(deadline) {
			return MusicResult{}, fmt.Errorf("ElevenLabs Music: превышено время ожидания генерации")
		}
		select {
		case <-ctx.Done():
			return MusicResult{}, ctx.Err()
		case <-time.After(musicPollInterval):
		}
	}
}

func (c *Client) musicStatus(ctx context.Context, pollingURL string) (status, audioURL string, err error) {
	endpoint := pollingURL
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = c.baseURL() + "/" + strings.TrimLeft(endpoint, "/")
	}
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := c.do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Status   string `json:"status"`
		AudioURL string `json:"audio_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("ElevenLabs Music: ошибка разбора статуса: %w", err)
	}
	return out.Status, out.AudioURL, nil
}

func (c *Client) fetchMusicAudio(ctx context.Context, audioURL, format string) (MusicResult, error) {
	endpoint := audioURL
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = c.baseURL() + "/" + strings.TrimLeft(endpoint, "/")
	}
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return MusicResult{}, err
	}
	req.Header.Set("Accept", "audio/*")
	resp, err := c.do(req)
	if err != nil {
		return MusicResult{}, err
	}
	return MusicResult{
		Audio:       resp.Body,
		ContentType: contentTypeOr(resp, "audio/mpeg"),
		FileName:    "music." + formatExt(format),
	}, nil
}

func isAudioResponse(resp *http.Response) bool {
	return strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "audio/")
}

func isTerminalCompleted(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "COMPLETED")
}

func contentTypeOr(resp *http.Response, fallback string) string {
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		return ct
	}
	return fallback
}

func formatExt(format string) string {
	switch {
	case strings.HasPrefix(format, "pcm"):
		return "pcm"
	case strings.HasPrefix(format, "ulaw"), strings.HasPrefix(format, "alaw"):
		return "ulaw"
	case strings.HasPrefix(format, "opus"):
		return "opus"
	default:
		return "mp3"
	}
}
