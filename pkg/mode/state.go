package mode

import (
	"time"

	"github.com/ikermy/air-common/pkg/com"
)

var (
	testAnswerMode   = false                     // Тестовый режим, когда текстовый ответ на вопрос возвращается сразу, без обращения к модели
	textMode         = false                     // Разрешает принимать текстовые сообщения в диалоге
	audioMode        = false                     // Разрешает принимать аудио сообщения в диалоге
	voiceCallMode    = false                     // Разрешает принимать голосовые вызовы
	sendErrMsgToUser = false                     // Отправлять сообщение о невозможности получить ответ модели в канал пользователя
	carpinteroCh     = make(chan com.CarpCh, 1)  // Канал для отправки уведомлений о событиях пользователям
	endDialogCh      = make(chan uint64, 1)      // Канал для передачи Id диалога при отключении клиента, в реализациях где яно можно вызвать например SSE
	instantCh        = make(chan com.InstMsg, 1) // Канал для передачи мгновенных сообщений пользователю в панель управления фронтенда
	realHost         string

	// Operator settings
	// Таймаут ожидания ПЕРВОГО ответа оператора в секундах
	// После первого ответа операторский режим становится постоянным (без таймера)
	operatorResponseTimeout = 120 * time.Second

	// Тайм-аут на операции с БД (в секундах)
	sqlTimeToCancel = 5 * time.Second
	userModelTTl    = 60 * time.Minute

	// Логирование — инициализируются через InitFromEnv()
	validLevels = map[string]struct{}{
		"info":  {},
		"error": {},
		"debug": {},
		"warn":  {},
	}
	logLevel = "info" // LOG_LEVEL: debug | info | warn | error
	logPath  = ""     // LOG_PATH: путь к файлу лога, не используется в режиме logger.StdOut()
)
