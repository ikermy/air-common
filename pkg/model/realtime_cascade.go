package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/elevenlabs"
)

// cascadeProvider реализует RealtimeProvider.
var _ RealtimeProvider = (*cascadeProvider)(nil)

// cascadeAudioBuffer — размер буферов аудио каскада.
const cascadeAudioBuffer = 100

// cascadeSTT / cascadeTTS — узкие контракты стадий каскада (позволяют
// подменять реализации в тестах).
type cascadeSTT interface {
	Run(ctx context.Context, audio <-chan []byte, onTranscript func(text string, final bool) error) error
}

type cascadeTTS interface {
	SynthesizeStream(ctx context.Context, req elevenlabs.SynthesizeRequest) (<-chan []byte, error)
}

// cascadeProvider — провайдер-агностичный realtime-каскад:
//
//	STT (ElevenLabs Scribe) → LLM (активный провайдер) → TTS (ElevenLabs)
//
// Он реализует RealtimeProvider и используется, когда у активной модели
// пользователя Voice.RealtimeBackend = elevenlabs. Нативный audio-to-audio
// провайдера (OpenAI/Google/Mistral) в этом режиме не задействуется.
type cascadeProvider struct {
	router *Router

	mu       sync.Mutex
	sessions map[uint64]*cascadeSession

	// Фабрики стадий; nil → реальные ElevenLabs-реализации.
	sttFactory func(apiKey, model, language string) cascadeSTT
	ttsFactory func(apiKey string) cascadeTTS
}

func defaultCascadeSTTFactory(apiKey, model, language string) cascadeSTT {
	stt := elevenlabs.NewRealtimeSTT(apiKey)
	stt.Model = model
	stt.Language = language
	// Клиенты голосового каскада шлют PCM16 24 kHz (см. audio-capture/player).
	stt.SampleRate = elevenlabs.RealtimeSTTSampleRate
	return stt
}

func defaultCascadeTTSFactory(apiKey string) cascadeTTS {
	return elevenlabs.NewClient(apiKey)
}

type cascadeSession struct {
	ctx    context.Context
	cancel context.CancelFunc

	userID   uint32
	dialogID uint64
	respID   uint64

	apiKey string
	stt    cascadeSTT
	tts    cascadeTTS

	ttsModel  string
	ttsVoice  string
	ttsFormat string
	language  string

	audioRx  chan []byte
	audioOut chan []byte
	drain    chan struct{}

	eventsMu sync.RWMutex
	events   map[chan RealtimeEvent]struct{}

	generating atomic.Bool
	closed     atomic.Bool

	turnMu     sync.Mutex
	turnCtx    context.Context
	turnCancel context.CancelFunc
	turnID     uint64

	transcriptMu sync.Mutex
	lastFinal    string

	callbackMu sync.RWMutex
	disconnect func(uint64)
}

func (r *Router) cascade() *cascadeProvider {
	r.cascadeOnce.Do(func() {
		r.cascadeProvider = &cascadeProvider{
			router:     r,
			sessions:   make(map[uint64]*cascadeSession),
			sttFactory: defaultCascadeSTTFactory,
			ttsFactory: defaultCascadeTTSFactory,
		}
	})
	return r.cascadeProvider
}

// hasCascadeSession сообщает, есть ли активная каскадная сессия с данным respId.
func (p *cascadeProvider) hasCascadeSession(respId uint64) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.sessions[respId]
	return ok
}

// cascadeVoiceConfig возвращает Voice-конфигурацию активной модели, если для неё
// включён ElevenLabs realtime-каскад.
func (r *Router) cascadeVoiceConfig(userID uint32) (*comdom.UniversalModelData, *comdom.VoiceConfig, bool) {
	if r.modelsManager == nil {
		return nil, nil, false
	}
	data, err := r.modelsManager.GetActiveUserModel(userID)
	if err != nil || data == nil || data.Voice == nil {
		return nil, nil, false
	}
	if !data.Voice.UsesElevenLabsRealtime() {
		return nil, nil, false
	}
	return data, data.Voice, true
}

func (p *cascadeProvider) StartRealtimeSession(userID uint32, dialogID, respID uint64) error {
	if p == nil || p.router == nil {
		return fmt.Errorf("realtime cascade не инициализирован")
	}
	_, voiceCfg, ok := p.router.cascadeVoiceConfig(userID)
	if !ok {
		return fmt.Errorf("ElevenLabs realtime не включён для userID=%d", userID)
	}

	if p.router.db == nil {
		return fmt.Errorf("БД не инициализирована")
	}
	apiKey, err := p.router.db.GetUserAPIKey(userID, comdom.ProviderElevenLabs)
	if err != nil {
		return err
	}
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("API-ключ ElevenLabs не настроен")
	}

	voiceID := ""
	if voiceCfg.VoiceID != nil {
		voiceID = strings.TrimSpace(*voiceCfg.VoiceID)
	}
	if voiceID == "" {
		return fmt.Errorf("не задан голос ElevenLabs (Voice.VoiceID)")
	}

	// Realtime-транспорт всегда сырой PCM16 24 kHz (совпадает с плеером клиента).
	// Voice.TTS.Format относится к batch-озвучке и в каскаде игнорируется: mp3/иная
	// частота в PCM-плеере дают резкие искажения.
	const format = "pcm_24000"
	language := ""
	if voiceCfg.TTS != nil && voiceCfg.TTS.Language != nil {
		language = *voiceCfg.TTS.Language
	}
	if voiceCfg.STT != nil && voiceCfg.STT.Language != nil && language == "" {
		language = *voiceCfg.STT.Language
	}

	parent := p.router.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)

	session := &cascadeSession{
		ctx:       ctx,
		cancel:    cancel,
		userID:    userID,
		dialogID:  dialogID,
		respID:    respID,
		apiKey:    apiKey,
		stt:       p.sttFactory(apiKey, voiceCfg.RealtimeSTTModelName(), language),
		tts:       p.ttsFactory(apiKey),
		ttsModel:  voiceCfg.TTSModelName(),
		ttsVoice:  voiceID,
		ttsFormat: format,
		language:  language,
		audioRx:   make(chan []byte, cascadeAudioBuffer),
		audioOut:  make(chan []byte, cascadeAudioBuffer),
		drain:     make(chan struct{}, 1),
		events:    make(map[chan RealtimeEvent]struct{}),
	}

	p.mu.Lock()
	if old, exists := p.sessions[respID]; exists {
		p.mu.Unlock()
		old.close()
		p.mu.Lock()
	}
	p.sessions[respID] = session
	p.mu.Unlock()

	go p.runSTT(session)
	p.publishEvent(session, RealtimeEvent{Type: "session_started", ResponseID: fmt.Sprint(respID)})

	return nil
}

func (p *cascadeProvider) runSTT(s *cascadeSession) {
	err := s.stt.Run(s.ctx, s.audioRx, func(text string, final bool) error {
		if !final || strings.TrimSpace(text) == "" {
			return nil
		}
		if !s.acceptFinal(text) {
			return nil
		}
		// Новый пользовательский turn прерывает предыдущий ответ/TTS.
		if s.currentTurn() != 0 {
			s.interrupt()
		}
		p.publishEvent(s, RealtimeEvent{Type: "input_transcript_done", Text: text})
		p.saveTranscript(s, comdom.SpeechRealTimeUser, text)
		p.runTurn(s, text)
		return nil
	})
	if err != nil && s.ctx.Err() == nil {
		p.publishEvent(s, RealtimeEvent{Type: "error", Text: "ElevenLabs STT завершился с ошибкой", Err: err})
		s.callbackMu.RLock()
		cb := s.disconnect
		s.callbackMu.RUnlock()
		if cb != nil {
			cb(s.respID)
		}
	}
}

// runTurn запускает LLM-ответ активного провайдера и озвучивает его ElevenLabs.
func (p *cascadeProvider) runTurn(s *cascadeSession, text string) {
	turnID := s.beginTurn()
	manager, err := p.router.GetActiveUserManager(s.userID)
	if err != nil {
		p.publishEvent(s, RealtimeEvent{Type: "error", Text: "LLM провайдер недоступен", Err: err})
		return
	}

	s.generating.Store(true)
	go func() {
		defer s.generating.Store(false)
		chunker := &cascadeChunker{}
		extractor := &cascadeTextExtractor{}
		var full strings.Builder
		finalized := false

		err := manager.RequestStreaming(s.userID, s.dialogID, text, func(delta string, done bool) error {
			if !s.isCurrentTurn(turnID) {
				return nil
			}
			text := extractor.Push(delta)
			if done {
				text += extractor.Flush()
			}
			if text != "" {
				full.WriteString(text)
				p.publishEvent(s, RealtimeEvent{Type: "response_text_delta", Text: text, Delta: text})
				for _, sentence := range chunker.Push(text, done) {
					if err := p.speak(s, turnID, sentence); err != nil {
						return err
					}
				}
			}
			if done && !finalized {
				finalized = true
				assistant := full.String()
				p.saveTranscript(s, comdom.SpeechRealTimeAI, assistant)
				p.publishEvent(s, RealtimeEvent{Type: "response_text_done", Text: assistant})
			}
			return nil
		})
		if err != nil && s.ctx.Err() == nil && s.isCurrentTurn(turnID) {
			p.publishEvent(s, RealtimeEvent{Type: "error", Text: "ошибка LLM в realtime-каскаде", Err: err})
		}
	}()
}

// speak синтезирует одну фразу и публикует PCM-чанки текущего turn.
func (p *cascadeProvider) speak(s *cascadeSession, turnID uint64, text string) error {
	ctx, ok := s.turnContext(turnID)
	if !ok {
		return nil
	}
	chunks, err := s.tts.SynthesizeStream(ctx, elevenlabs.SynthesizeRequest{
		Model:   s.ttsModel,
		Text:    text,
		VoiceID: s.ttsVoice,
		Format:  s.ttsFormat,
	})
	if err != nil {
		return err
	}
	for chunk := range chunks {
		if !s.isCurrentTurn(turnID) {
			return nil
		}
		s.publishAudio(turnID, chunk)
	}
	return nil
}

func (p *cascadeProvider) saveTranscript(s *cascadeSession, creator comdom.CreatorType, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	resp := &AssistResponse{Message: text, Action: Action{SendFiles: []File{}}}
	if p.router.dialogSaver != nil {
		p.router.dialogSaver.SaveDialog(creator, s.dialogID, resp)
		return
	}
	raw, _ := json.Marshal(map[string]any{
		"creator":   creator,
		"message":   resp,
		"timestamp": time.Now(),
	})
	_ = p.router.db.SaveDialog(s.dialogID, raw)
}

func (p *cascadeProvider) publishEvent(s *cascadeSession, event RealtimeEvent) {
	if s == nil {
		return
	}
	s.eventsMu.RLock()
	defer s.eventsMu.RUnlock()
	for ch := range s.events {
		select {
		case ch <- event:
		default:
		}
	}
}

func (p *cascadeProvider) CloseRealtimeSession(respID uint64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	session, ok := p.sessions[respID]
	delete(p.sessions, respID)
	p.mu.Unlock()
	if ok {
		session.close()
	}
}

func (p *cascadeProvider) SendRealtimeAudio(respID uint64, pcm16 []byte) error {
	if p == nil || len(pcm16) == 0 {
		return nil
	}
	p.mu.Lock()
	session, ok := p.sessions[respID]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("cascade session %d не найдена", respID)
	}
	if session.closed.Load() {
		return fmt.Errorf("cascade session %d закрыта", respID)
	}
	copied := append([]byte(nil), pcm16...)
	select {
	case session.audioRx <- copied:
	case <-session.ctx.Done():
		return session.ctx.Err()
	default:
		// очередь переполнена — кадр отбрасывается, как в Mistral
	}
	return nil
}

func (p *cascadeProvider) SubscribeEvents(respID uint64) (<-chan RealtimeEvent, error) {
	p.mu.Lock()
	session, ok := p.sessions[respID]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("cascade session %d не найдена", respID)
	}
	ch := make(chan RealtimeEvent, 64)
	session.eventsMu.Lock()
	session.events[ch] = struct{}{}
	session.eventsMu.Unlock()
	return ch, nil
}

func (p *cascadeProvider) UnsubscribeEvents(respID uint64, sub <-chan RealtimeEvent) {
	p.mu.Lock()
	session, ok := p.sessions[respID]
	p.mu.Unlock()
	if !ok {
		return
	}
	session.eventsMu.Lock()
	defer session.eventsMu.Unlock()
	for ch := range session.events {
		if ch == sub {
			delete(session.events, ch)
			close(ch)
			return
		}
	}
}

func (p *cascadeProvider) GetRealtimeAudio(respID uint64) (<-chan []byte, error) {
	p.mu.Lock()
	session, ok := p.sessions[respID]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("cascade session %d не найдена", respID)
	}
	return session.audioOut, nil
}

func (p *cascadeProvider) GetRealtimeDrain(respID uint64) (<-chan struct{}, error) {
	p.mu.Lock()
	session, ok := p.sessions[respID]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("cascade session %d не найдена", respID)
	}
	return session.drain, nil
}

func (p *cascadeProvider) GetRealtimeGenerating(respID uint64) *atomic.Bool {
	p.mu.Lock()
	session, ok := p.sessions[respID]
	p.mu.Unlock()
	if !ok {
		return nil
	}
	return &session.generating
}

func (p *cascadeProvider) SetRealtimeDisconnectCallback(respID uint64, callback func(uint64)) error {
	p.mu.Lock()
	session, ok := p.sessions[respID]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("cascade session %d не найдена", respID)
	}
	session.callbackMu.Lock()
	session.disconnect = callback
	session.callbackMu.Unlock()
	return nil
}

// closeUser закрывает все каскадные сессии пользователя (например, при отзыве ключа).
func (p *cascadeProvider) closeUser(userID uint32) {
	if p == nil {
		return
	}
	p.mu.Lock()
	var toClose []*cascadeSession
	for respID, session := range p.sessions {
		if session.userID == userID {
			toClose = append(toClose, session)
			delete(p.sessions, respID)
		}
	}
	p.mu.Unlock()
	for _, session := range toClose {
		session.close()
	}
}

// closeAll закрывает все каскадные сессии (shutdown).
func (p *cascadeProvider) closeAll() {
	if p == nil {
		return
	}
	p.mu.Lock()
	toClose := make([]*cascadeSession, 0, len(p.sessions))
	for respID, session := range p.sessions {
		toClose = append(toClose, session)
		delete(p.sessions, respID)
	}
	p.mu.Unlock()
	for _, session := range toClose {
		session.close()
	}
}

// ─── session helpers ────────────────────────────────────────────────────────

func (s *cascadeSession) acceptFinal(text string) bool {
	s.transcriptMu.Lock()
	defer s.transcriptMu.Unlock()
	text = strings.TrimSpace(text)
	if text == "" || text == s.lastFinal {
		return false
	}
	s.lastFinal = text
	return true
}

func (s *cascadeSession) beginTurn() uint64 {
	s.turnMu.Lock()
	if s.turnCancel != nil {
		s.turnCancel()
	}
	s.turnCtx, s.turnCancel = context.WithCancel(s.ctx)
	s.turnID++
	id := s.turnID
	s.turnMu.Unlock()
	return id
}

func (s *cascadeSession) currentTurn() uint64 {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	return s.turnID
}

func (s *cascadeSession) isCurrentTurn(turnID uint64) bool {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	return s.turnID == turnID && s.turnCancel != nil
}

func (s *cascadeSession) turnContext(turnID uint64) (context.Context, bool) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if s.turnID != turnID || s.turnCtx == nil {
		return nil, false
	}
	return s.turnCtx, true
}

// interrupt отменяет текущий turn и опустошает очередь воспроизведения.
func (s *cascadeSession) interrupt() {
	s.turnMu.Lock()
	if s.turnCancel != nil {
		s.turnCancel()
		s.turnCancel = nil
		s.turnCtx = nil
	}
	s.turnMu.Unlock()
	s.generating.Store(false)
	// публикуем событие вне событийного лока
	s.eventsMu.RLock()
	for ch := range s.events {
		select {
		case ch <- RealtimeEvent{Type: "interrupted", Text: "пользователь перебил ответ"}:
		default:
		}
	}
	s.eventsMu.RUnlock()
	for {
		select {
		case <-s.audioOut:
		default:
			select {
			case s.drain <- struct{}{}:
			default:
			}
			return
		}
	}
}

func (s *cascadeSession) publishAudio(turnID uint64, pcm []byte) bool {
	if s == nil || len(pcm) == 0 || s.closed.Load() {
		return false
	}
	if !s.isCurrentTurn(turnID) {
		return false
	}
	copied := append([]byte(nil), pcm...)
	select {
	case s.audioOut <- copied:
		return true
	default:
		return false
	}
}

func (s *cascadeSession) close() {
	if s == nil {
		return
	}
	s.closed.Store(true)
	s.cancel()
	s.eventsMu.Lock()
	for ch := range s.events {
		close(ch)
		delete(s.events, ch)
	}
	s.eventsMu.Unlock()
}

// ─── sentence chunker ───────────────────────────────────────────────────────

// cascadeChunker нарезает потоковый текст LLM на фразы для TTS.
type cascadeChunker struct {
	buf strings.Builder
}

func (c *cascadeChunker) Push(delta string, final bool) []string {
	c.buf.WriteString(delta)
	text := c.buf.String()
	var out []string
	start := 0
	for i, r := range text {
		if r == '.' || r == '!' || r == '?' || r == '…' {
			sentence := strings.TrimSpace(text[start : i+utf8.RuneLen(r)])
			if sentence != "" {
				out = append(out, sentence)
			}
			start = i + utf8.RuneLen(r)
		}
	}
	if start > 0 {
		rest := text[start:]
		c.buf.Reset()
		c.buf.WriteString(rest)
	}
	if final {
		if tail := strings.TrimSpace(c.buf.String()); tail != "" {
			out = append(out, tail)
		}
		c.buf.Reset()
	}
	return out
}
