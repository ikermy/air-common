package provider_catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/comerrors"
	"github.com/ikermy/air-common/pkg/mode"
)

// Static catalog values for ElevenLabs capabilities that are not exposed via
// GET /v1/models (STT и Music). Актуализировать при выходе новых версий.
var (
	elevenLabsSTTModels   = []string{"scribe_v2_realtime", "scribe_v2", "scribe_v2_medical", "scribe_v1"}
	elevenLabsMusicModels = []string{"eleven_music"}
)

type elevenLabsModel struct {
	ModelID       string `json:"model_id"`
	Name          string `json:"name"`
	CanDoTTS      bool   `json:"can_do_text_to_speech"`
	CanDoVC       bool   `json:"can_do_voice_conversion"`
	RequiresAlpha bool   `json:"requires_alpha_access"`
}

// FetchElevenLabsModels возвращает актуальный список model_id для указанного
// вида голосовой модели ElevenLabs. TTS/STS берутся из GET /v1/models, STT/Music —
// из статических списков (их нет в /v1/models).
func (c *Client) FetchElevenLabsModels(ctx context.Context, apiKey string, kind comdom.VoiceKind) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	client := c
	if client == nil || client.HTTPClient == nil {
		client = NewClient()
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("пустой API-ключ ElevenLabs")
	}

	switch kind {
	case comdom.VoiceKindSTT:
		return append([]string(nil), elevenLabsSTTModels...), nil
	case comdom.VoiceKindMusic:
		return append([]string(nil), elevenLabsMusicModels...), nil
	case comdom.VoiceKindTTS, comdom.VoiceKindSTS:
		models, err := client.fetchElevenLabsModels(ctx, apiKey)
		if err != nil {
			return nil, err
		}
		result := make([]string, 0, len(models))
		for _, m := range models {
			if m.RequiresAlpha {
				continue
			}
			switch kind {
			case comdom.VoiceKindTTS:
				if m.CanDoTTS {
					result = append(result, m.ModelID)
				}
			case comdom.VoiceKindSTS:
				if m.CanDoVC {
					result = append(result, m.ModelID)
				}
			}
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("получено 0 моделей ElevenLabs для kind=%s", kind)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("неподдерживаемый kind ElevenLabs: %s", kind)
	}
}

func (c *Client) fetchElevenLabsModels(ctx context.Context, apiKey string) ([]elevenLabsModel, error) {
	baseURL := c.ElevenLabsBaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = mode.ElevenLabsBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, comerrors.NewProviderError(comdom.ProviderElevenLabs, resp.StatusCode, strings.TrimSpace(string(body)), nil)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var models []elevenLabsModel
	if err := json.Unmarshal(body, &models); err != nil {
		return nil, fmt.Errorf("ошибка разбора ответа ElevenLabs: %w", err)
	}
	return models, nil
}
