package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ikermy/air-common/pkg/mode"
)

// RealtimeSTT — серверный streaming-распознаватель Scribe v2 Realtime.
//
// Аудио: PCM 16 kHz mono s16le LE. Протокол:
//
//	отправка  {"message_type":"input_audio_chunk","audio_base_64":"...","commit":false,"sample_rate":16000}
//	финал     {"message_type":"input_audio_chunk","audio_base_64":"","commit":true,"sample_rate":16000}
//	приём     session_started | partial_transcript | committed_transcript | input_error
type RealtimeSTT struct {
	Model    string
	Language string
	APIKey   string
	BaseURL  string

	// Dialer позволяет переопределить websocket-соединение в тестах.
	Dialer *websocket.Dialer
}

// NewRealtimeSTT создаёт серверный realtime STT клиент.
func NewRealtimeSTT(apiKey string) *RealtimeSTT {
	return &RealtimeSTT{APIKey: strings.TrimSpace(apiKey)}
}

func (s *RealtimeSTT) url() string {
	base := strings.TrimSpace(s.BaseURL)
	if base == "" {
		base = mode.ElevenLabsRealtimeSTTURL
	}
	model := strings.TrimSpace(s.Model)
	if model == "" {
		model = "scribe_v2_realtime"
	}
	params := url.Values{}
	params.Set("model_id", model)
	if strings.TrimSpace(s.Language) != "" {
		params.Set("language_code", s.Language)
	}
	return base + "?" + params.Encode()
}

type realtimeSTTMessage struct {
	MessageType string `json:"message_type"`
	Text        string `json:"text"`
	Error       string `json:"error"`
}

// Run подключается к ElevenLabs и транскрибирует аудио из канала.
// onTranscript вызывается для partial (final=false) и committed (final=true)
// транскриптов. Возврат управляется контекстом или закрытием audio-канала.
func (s *RealtimeSTT) Run(ctx context.Context, audio <-chan []byte, onTranscript func(text string, final bool) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(s.APIKey) == "" {
		return fmt.Errorf("пустой API-ключ ElevenLabs")
	}

	header := http.Header{}
	header.Set("xi-api-key", s.APIKey)

	dialer := s.Dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	conn, resp, err := dialer.DialContext(ctx, s.url(), header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return fmt.Errorf("подключение к ElevenLabs realtime STT (status=%d): %w", status, err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	readErr := make(chan error, 1)
	go func() {
		readErr <- s.readLoop(ctx, conn, onTranscript)
	}()

	writeErr := make(chan error, 1)
	go func() {
		writeErr <- s.writeLoop(ctx, conn, audio)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-readErr:
		return err
	case err := <-writeErr:
		return err
	}
}

func (s *RealtimeSTT) readLoop(ctx context.Context, conn *websocket.Conn, onTranscript func(string, bool) error) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("чтение ElevenLabs realtime STT: %w", err)
		}
		var msg realtimeSTTMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.MessageType {
		case "partial_transcript":
			if onTranscript != nil && strings.TrimSpace(msg.Text) != "" {
				if err := onTranscript(msg.Text, false); err != nil {
					return err
				}
			}
		case "committed_transcript", "committed_transcript_with_timestamps":
			if onTranscript != nil && strings.TrimSpace(msg.Text) != "" {
				if err := onTranscript(msg.Text, true); err != nil {
					return err
				}
			}
		case "input_error", "error":
			if strings.TrimSpace(msg.Error) != "" {
				return fmt.Errorf("ElevenLabs realtime STT: %s", msg.Error)
			}
		}
	}
}

func (s *RealtimeSTT) writeLoop(ctx context.Context, conn *websocket.Conn, audio <-chan []byte) error {
	write := func(payload []byte, commit bool) error {
		frame := map[string]any{
			"message_type":  "input_audio_chunk",
			"audio_base_64": base64.StdEncoding.EncodeToString(payload),
			"commit":        commit,
			"sample_rate":   16000,
		}
		raw, err := json.Marshal(frame)
		if err != nil {
			return err
		}
		return conn.WriteMessage(websocket.TextMessage, raw)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-audio:
			if !ok {
				// Конец потока: финальный commit и немного ждём committed-транскрипт.
				if err := write(nil, true); err != nil {
					return err
				}
				select {
				case <-ctx.Done():
				case <-time.After(2 * time.Second):
				}
				return nil
			}
			if len(chunk) == 0 {
				continue
			}
			if err := write(chunk, false); err != nil {
				return err
			}
		}
	}
}
