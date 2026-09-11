package startpoint

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/model"
)

// fakeRealtimeProvider реализует model.RealtimeProvider для юнит-тестов.
type fakeRealtimeProvider struct {
	started   bool
	unsubbed  int
	closed    int
	failAudio bool
	audioTx   chan []byte
	drain     chan struct{}
	subs      []chan model.RealtimeEvent
}

func newFakeRealtimeProvider() *fakeRealtimeProvider {
	return &fakeRealtimeProvider{
		audioTx: make(chan []byte, 1),
		drain:   make(chan struct{}, 1),
	}
}

func (f *fakeRealtimeProvider) StartRealtimeSession(userID uint32, dialogID, respId uint64) error {
	f.started = true
	return nil
}
func (f *fakeRealtimeProvider) CloseRealtimeSession(respId uint64) { f.closed++ }
func (f *fakeRealtimeProvider) SendRealtimeAudio(respId uint64, pcm16 []byte) error {
	return nil
}
func (f *fakeRealtimeProvider) SubscribeEvents(respId uint64) (<-chan model.RealtimeEvent, error) {
	ch := make(chan model.RealtimeEvent, 1)
	f.subs = append(f.subs, ch)
	return ch, nil
}
func (f *fakeRealtimeProvider) UnsubscribeEvents(respId uint64, sub <-chan model.RealtimeEvent) {
	f.unsubbed++
}
func (f *fakeRealtimeProvider) GetRealtimeAudio(respId uint64) (<-chan []byte, error) {
	if f.failAudio {
		return nil, errors.New("audio setup failed")
	}
	return f.audioTx, nil
}
func (f *fakeRealtimeProvider) GetRealtimeDrain(respId uint64) (<-chan struct{}, error) {
	return f.drain, nil
}
func (f *fakeRealtimeProvider) GetRealtimeGenerating(respId uint64) *atomic.Bool {
	return &atomic.Bool{}
}
func (f *fakeRealtimeProvider) SetRealtimeDisconnectCallback(respId uint64, callback func(respId uint64)) error {
	return nil
}

// fakeRouter реализует model.RealtimeRouter.
type fakeRouter struct {
	provider     model.RealtimeProvider
	disconnected uint64
}

func (r *fakeRouter) GetRealtimeProvider(userID uint32) (model.RealtimeProvider, bool) {
	if r.provider == nil {
		return nil, false
	}
	return r.provider, true
}
func (r *fakeRouter) DisconnectRealtimeSession(respId uint64) { r.disconnected = respId }

func TestRunRealtimeSession_SuccessWiresChannelsAndCloses(t *testing.T) {
	fp := newFakeRealtimeProvider()
	fr := &fakeRouter{provider: fp}
	s := &Start{RT: fr, ctx: context.Background()}

	start := &model.StartCh{
		RespId:   42,
		ThreadId: 7,
		Channel:  comdom.Telegram,
		Model:    &model.RespModel{Assist: model.Assistant{UserID: 5}},
		Realtime: &model.RealtimeChannels{},
	}

	errCh := s.StartSession(start)

	if !fp.started {
		t.Fatal("provider не запущен")
	}
	if start.Realtime == nil || start.Realtime.AudioTx == nil ||
		start.Realtime.Drain == nil || start.Realtime.Events == nil {
		t.Fatal("каналы start.Realtime не наполнены")
	}
	if _, ok := s.sessions.Load(uint64(42)); !ok {
		t.Fatal("сессия не зарегистрирована")
	}

	s.CloseSession(42)

	// После CloseSession bridge завершается, владелец закрывает errCh.
	closed := make(chan struct{})
	go func() {
		for range errCh {
		}
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("errCh не закрыт после CloseSession")
	}

	if fp.unsubbed != 2 {
		t.Fatalf("ожидалось 2 UnsubscribeEvents, получено %d", fp.unsubbed)
	}
	if fr.disconnected != 42 {
		t.Fatalf("DisconnectRealtimeSession не вызван для respId=42 (получено %d)", fr.disconnected)
	}
}

func TestRunRealtimeSession_ProviderInitiatedCloseCleansUp(t *testing.T) {
	fp := newFakeRealtimeProvider()
	s := &Start{RT: &fakeRouter{provider: fp}, ctx: context.Background()}
	start := &model.StartCh{
		RespId:   9,
		ThreadId: 1,
		Model:    &model.RespModel{Assist: model.Assistant{UserID: 5}},
		Realtime: &model.RealtimeChannels{},
	}
	errCh := s.StartSession(start)

	if len(fp.subs) != 2 {
		t.Fatalf("ожидалось 2 подписки, получено %d", len(fp.subs))
	}
	// Провайдер сам закрывает подписку моста (errEvents).
	close(fp.subs[1])

	closed := make(chan struct{})
	go func() {
		for range errCh {
		}
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("errCh не закрыт после provider-initiated close")
	}
	if _, ok := s.sessions.Load(uint64(9)); ok {
		t.Fatal("запись сессии должна быть удалена после provider-initiated close")
	}
}

func TestRunRealtimeSession_RollbackOnSetupFailure(t *testing.T) {
	fp := newFakeRealtimeProvider()
	fp.failAudio = true
	s := &Start{RT: &fakeRouter{provider: fp}, ctx: context.Background()}
	start := &model.StartCh{
		RespId:   3,
		ThreadId: 1,
		Model:    &model.RespModel{Assist: model.Assistant{UserID: 5}},
		Realtime: &model.RealtimeChannels{},
	}

	errs := drainErrCh(s.StartSession(start))
	if len(errs) != 1 {
		t.Fatalf("ожидалась 1 ошибка setup, получено %d", len(errs))
	}
	if fp.closed != 1 {
		t.Fatalf("ожидался rollback CloseRealtimeSession, closed=%d", fp.closed)
	}
	if _, ok := s.sessions.Load(uint64(3)); ok {
		t.Fatal("сессия не должна регистрироваться при сбое setup")
	}
}

func TestBridgeRealtimeErrors_ForwardsErrorAndStopsOnCancel(t *testing.T) {
	s := &Start{}
	evCh := make(chan model.RealtimeEvent, 1)
	errCh := make(chan error, 4)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.bridgeRealtimeErrors(ctx, 7, evCh, errCh)
	}()

	evCh <- model.RealtimeEvent{Type: "error", Text: "boom"}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("ожидалась непустая ошибка")
		}
		if !IsNonCriticalError(err) {
			t.Fatalf("ожидалась NonCriticalError, получено %T", err)
		}
	case <-time.After(time.Second):
		t.Fatal("таймаут ожидания ошибки из bridge")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("bridge не завершился по отмене ctx")
	}
}

func TestCloseSession_UnsubscribesAndCancels(t *testing.T) {
	s := &Start{}
	fp := &fakeRealtimeProvider{}
	cancelled := false
	s.registerSession(42, &sessionEntry{
		cancel:    func() { cancelled = true },
		rp:        fp,
		events:    make(chan model.RealtimeEvent),
		errEvents: make(chan model.RealtimeEvent),
	})

	s.CloseSession(42)

	if fp.unsubbed != 2 {
		t.Fatalf("ожидалось 2 UnsubscribeEvents, получено %d", fp.unsubbed)
	}
	if !cancelled {
		t.Fatal("ожидался вызов cancel")
	}
	if _, ok := s.sessions.Load(42); ok {
		t.Fatal("запись сессии должна быть удалена")
	}
}
