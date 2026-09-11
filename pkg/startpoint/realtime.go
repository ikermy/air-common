package startpoint

import (
	"context"
	"fmt"

	"github.com/ikermy/air-common/pkg/comdom"
	"github.com/ikermy/air-common/pkg/model"
)

// sessionEntry — запись реестра активных realtime-сессий.
type sessionEntry struct {
	cancel    context.CancelFunc         // останавливает bridge и provider через ctx
	channel   comdom.ChannelType         // для логирования
	rp        model.RealtimeProvider     // для UnsubscribeEvents при закрытии
	events    <-chan model.RealtimeEvent // подписка транспорта
	errEvents <-chan model.RealtimeEvent // подписка моста ошибок
}

// registerSession добавляет запись в реестр realtime-сессий (по respId).
func (s *Start) registerSession(respId uint64, e *sessionEntry) {
	s.sessions.Store(respId, e)
}

// runRealtimeSession запускает realtime/hybrid-сессию через Router.
//
// Возвращает канал ошибок сессии. Владелец канала — эта функция: на успешном
// пути его закрывает отдельная goroutine после остановки единственного
// писателя (bridgeRealtimeErrors); на пути ошибки — сама функция.
func (s *Start) runRealtimeSession(start *model.StartCh) <-chan error {
	errCh := make(chan error, 1)

	if s.RT == nil {
		s.sendError(errCh, fmt.Errorf("runRealtimeSession: realtime не инициализирован (Router отсутствует)"))
		close(errCh)
		return errCh
	}
	if start.Model == nil {
		s.sendError(errCh, fmt.Errorf("runRealtimeSession: start.Model is nil for respId %d", start.RespId))
		close(errCh)
		return errCh
	}

	rp, ok := s.RT.GetRealtimeProvider(start.Model.Assist.UserID)
	if !ok {
		s.sendError(errCh, fmt.Errorf("runRealtimeSession: модель пользователя %d не поддерживает Realtime",
			start.Model.Assist.UserID))
		close(errCh)
		return errCh
	}

	ctx, cancel := context.WithCancel(s.ctx)

	// start.ThreadId == dialogs.Id, передаётся как dialogID.
	if err := rp.StartRealtimeSession(start.Model.Assist.UserID, start.ThreadId, start.RespId); err != nil {
		cancel()
		s.sendError(errCh, err)
		close(errCh)
		return errCh
	}

	// rollback на частичном сбое: иначе сессия останется в realtimeSessions.
	rollback := func(err error) {
		rp.CloseRealtimeSession(start.RespId)
		cancel()
		s.sendError(errCh, err)
		close(errCh)
	}

	audioTx, err := rp.GetRealtimeAudio(start.RespId)
	if err != nil {
		rollback(err)
		return errCh
	}
	drainCh, err := rp.GetRealtimeDrain(start.RespId)
	if err != nil {
		rollback(err)
		return errCh
	}

	// ДВА независимых подписчика: events — транспорту, errEvents — мосту ошибок.
	// Один канал на двух потребителей привёл бы к «краже» событий.
	events, err := rp.SubscribeEvents(start.RespId)
	if err != nil {
		rollback(err)
		return errCh
	}
	errEvents, err := rp.SubscribeEvents(start.RespId)
	if err != nil {
		rp.UnsubscribeEvents(start.RespId, events)
		rollback(err)
		return errCh
	}

	start.Realtime = &model.RealtimeChannels{
		AudioTx: audioTx,
		Drain:   drainCh,
		Events:  events,
	}

	// Единственный писатель в errCh на успешном пути — bridge. Он НЕ закрывает
	// канал: владелец (эта функция) закрывает его только после остановки
	// писателя — гонки «закрыли, пока пишут» нет.
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		s.bridgeRealtimeErrors(ctx, start.RespId, errEvents, errCh)
	}()
	go func() {
		<-bridgeDone
		// Идемпотентно: CloseSession уже сделал LoadAndDelete, здесь снимаем
		// запись, если провайдер завершил сессию сам (errEvents закрыт).
		s.sessions.Delete(start.RespId)
		close(errCh)
	}()

	s.registerSession(start.RespId, &sessionEntry{
		cancel:    cancel,
		channel:   start.Channel,
		rp:        rp,
		events:    events,
		errEvents: errEvents,
	})

	return errCh
}

// bridgeRealtimeErrors мостит RealtimeEvent{Type:"error"} в errCh.
// Писатель: не закрывает errCh (владелец — runRealtimeSession). Завершается
// по отмене ctx (CloseSession) или закрытию errEvents провайдером.
func (s *Start) bridgeRealtimeErrors(ctx context.Context, respId uint64, events <-chan model.RealtimeEvent, errCh chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Type != "error" || (ev.Err == nil && ev.Text == "") {
				continue
			}
			msg := ev.Text
			if ev.Err != nil {
				msg = ev.Err.Error()
			}
			s.sendError(errCh, &NonCriticalError{
				Err: fmt.Errorf("realtime error [respId=%d]: %s", respId, msg),
			})
		}
	}
}

// CloseSession завершает realtime-сессию по respId: отписывает подписки,
// отменяет контекст (останавливает bridge и provider) и удаляет сессию.
// Для text-сессий не применяется (они не регистрируются в s.sessions).
func (s *Start) CloseSession(respId uint64) {
	if entry, ok := s.sessions.LoadAndDelete(respId); ok {
		e := entry.(*sessionEntry)
		if e.rp != nil {
			if e.events != nil {
				e.rp.UnsubscribeEvents(respId, e.events)
			}
			if e.errEvents != nil {
				e.rp.UnsubscribeEvents(respId, e.errEvents)
			}
		}
		if e.cancel != nil {
			e.cancel()
		}
	}
	if s.RT != nil {
		s.RT.DisconnectRealtimeSession(respId)
	}
}
