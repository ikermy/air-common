package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
)

// TranscribeRequest — запрос batch-распознавания (Scribe).
type TranscribeRequest struct {
	Model    string
	Language string
	Keyterms []string
	FileName string
	Audio    []byte
}

type sttResponse struct {
	Text string `json:"text"`
}

// Transcribe выполняет POST /v1/speech-to-text (multipart/form-data).
func (c *Client) Transcribe(ctx context.Context, req TranscribeRequest) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(req.Audio) == 0 {
		return "", fmt.Errorf("audio не может быть пустым")
	}
	model := req.Model
	if strings.TrimSpace(model) == "" {
		model = "scribe_v2"
	}
	fileName := req.FileName
	if strings.TrimSpace(fileName) == "" {
		fileName = "audio.wav"
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("model_id", model); err != nil {
		return "", err
	}
	if req.Language != "" {
		if err := writer.WriteField("language_code", req.Language); err != nil {
			return "", err
		}
	}
	for _, term := range req.Keyterms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if err := writer.WriteField("keyterms", term); err != nil {
			return "", err
		}
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(req.Audio); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, c.baseURL()+"/speech-to-text", &buf)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var out sttResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("ошибка разбора ответа ElevenLabs STT: %w", err)
	}
	return strings.TrimSpace(out.Text), nil
}
