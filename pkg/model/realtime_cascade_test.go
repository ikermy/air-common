package model

import (
	"context"
	"testing"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/elevenlabs"
)

func TestCascadeChunker(t *testing.T) {
	var c cascadeChunker

	got := c.Push("Привет, мир. Как дела? Всё ", false)
	if len(got) != 2 || got[0] != "Привет, мир." || got[1] != "Как дела?" {
		t.Fatalf("sentences=%v", got)
	}

	got = c.Push("хорошо!", true)
	if len(got) != 1 || got[0] != "Всё хорошо!" {
		t.Fatalf("tail sentences=%v", got)
	}

	if rest := c.Push("", true); len(rest) != 0 {
		t.Fatalf("expected no tail after flush, got %v", rest)
	}
}

func TestCascadeChunkerMultibyte(t *testing.T) {
	var c cascadeChunker
	got := c.Push("слово… дальше", false)
	if len(got) != 1 || got[0] != "слово…" {
		t.Fatalf("multibyte sentences=%v", got)
	}
}

func TestCascadeSessionTurnGuard(t *testing.T) {
	s := &cascadeSession{
		ctx:      context.Background(),
		audioOut: make(chan []byte, 4),
		drain:    make(chan struct{}, 1),
	}

	turn1 := s.beginTurn()
	if !s.isCurrentTurn(turn1) {
		t.Fatalf("turn1 must be current")
	}
	if !s.publishAudio(turn1, []byte{1, 2, 3}) {
		t.Fatalf("audio for current turn must be published")
	}

	turn2 := s.beginTurn()
	if s.isCurrentTurn(turn1) {
		t.Fatalf("turn1 must be invalidated by turn2")
	}
	if s.publishAudio(turn1, []byte{4}) {
		t.Fatalf("audio for stale turn must be rejected")
	}
	if !s.publishAudio(turn2, []byte{5}) {
		t.Fatalf("audio for current turn must be published")
	}

	s.interrupt()
	if s.isCurrentTurn(turn2) {
		t.Fatalf("turn2 must be interrupted")
	}
	select {
	case <-s.drain:
	default:
		t.Fatalf("interrupt must signal drain")
	}
}

func TestCascadeSessionAcceptFinalDedup(t *testing.T) {
	s := &cascadeSession{}
	if !s.acceptFinal("привет") {
		t.Fatalf("first final must be accepted")
	}
	if s.acceptFinal("привет") {
		t.Fatalf("duplicate final must be rejected")
	}
	if s.acceptFinal("  ") {
		t.Fatalf("empty final must be rejected")
	}
	if !s.acceptFinal("пока") {
		t.Fatalf("new final must be accepted")
	}
}

func TestCascadeGreetingFromModel(t *testing.T) {
	str := func(s string) *string { return &s }
	boolean := func(b bool) *bool { return &b }

	if got := cascadeGreetingFromModel(nil); got != "" {
		t.Fatalf("nil model=%q", got)
	}
	if got := cascadeGreetingFromModel(&comdom.UniversalModelData{}); got != "" {
		t.Fatalf("nil vad=%q", got)
	}
	if got := cascadeGreetingFromModel(&comdom.UniversalModelData{
		RealtimeVAD: &comdom.RealtimeVAD{InitialGreeting: boolean(false), Greeting: str("hi")},
	}); got != "" {
		t.Fatalf("disabled greeting=%q", got)
	}
	if got := cascadeGreetingFromModel(&comdom.UniversalModelData{
		RealtimeVAD: &comdom.RealtimeVAD{Greeting: str("  Привет!  ")},
	}); got != "Привет!" {
		t.Fatalf("explicit greeting=%q", got)
	}
	if got := cascadeGreetingFromModel(&comdom.UniversalModelData{
		RealtimeVAD: &comdom.RealtimeVAD{InitialGreeting: boolean(true)},
	}); got != "" {
		t.Fatalf("no explicit phrase must not auto-generate, got %q", got)
	}
}

type fakeCascadeTTS struct {
	texts []string
}

func (f *fakeCascadeTTS) SynthesizeStream(_ context.Context, req elevenlabs.SynthesizeRequest) (<-chan []byte, error) {
	f.texts = append(f.texts, req.Text)
	ch := make(chan []byte, 1)
	ch <- []byte{1, 2, 3, 4}
	close(ch)
	return ch, nil
}

type fakeDialogSaver struct {
	messages []string
}

func (f *fakeDialogSaver) SaveDialog(_ comdom.CreatorType, _ uint64, resp *AssistResponse) {
	f.messages = append(f.messages, resp.Message)
}

func TestCascadeStartGreetingSpeaks(t *testing.T) {
	tts := &fakeCascadeTTS{}
	saver := &fakeDialogSaver{}
	p := &cascadeProvider{router: &Router{dialogSaver: saver}}
	s := &cascadeSession{
		ctx:       context.Background(),
		audioOut:  make(chan []byte, 8),
		drain:     make(chan struct{}, 1),
		events:    make(map[chan RealtimeEvent]struct{}),
		tts:       tts,
		ttsModel:  "eleven_flash_v2",
		ttsVoice:  "voice",
		ttsFormat: "pcm_24000",
		greeting:  "Привет! Чем помочь?",
	}
	events := make(chan RealtimeEvent, 8)
	s.events[events] = struct{}{}

	p.startGreeting(s)

	if len(tts.texts) == 0 || tts.texts[0] != "Привет!" {
		t.Fatalf("greeting sentences=%v", tts.texts)
	}
	select {
	case chunk := <-s.audioOut:
		if len(chunk) == 0 {
			t.Fatal("empty greeting audio chunk")
		}
	default:
		t.Fatal("greeting audio must be published")
	}
	if len(saver.messages) != 1 || saver.messages[0] != "Привет! Чем помочь?" {
		t.Fatalf("saved transcripts=%v", saver.messages)
	}

	var gotDelta, gotDone bool
	for len(events) > 0 {
		ev := <-events
		switch ev.Type {
		case "response_text_delta":
			gotDelta = true
		case "response_text_done":
			gotDone = true
		}
	}
	if !gotDelta || !gotDone {
		t.Fatalf("greeting events delta=%v done=%v", gotDelta, gotDone)
	}
}

func TestCascadeStartGreetingSkippedWhenUserSpeaking(t *testing.T) {
	tts := &fakeCascadeTTS{}
	p := &cascadeProvider{router: &Router{dialogSaver: &fakeDialogSaver{}}}
	s := &cascadeSession{
		ctx:       context.Background(),
		audioOut:  make(chan []byte, 4),
		drain:     make(chan struct{}, 1),
		events:    make(map[chan RealtimeEvent]struct{}),
		tts:       tts,
		ttsModel:  "eleven_flash_v2",
		ttsVoice:  "voice",
		ttsFormat: "pcm_24000",
		greeting:  "Привет!",
	}
	s.beginTurn() // пользователь уже начал говорить

	p.startGreeting(s)

	if len(tts.texts) != 0 {
		t.Fatalf("greeting must not be spoken when user already speaking, got %v", tts.texts)
	}
}
