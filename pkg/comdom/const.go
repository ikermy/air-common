package comdom

// ChatType определяет тип чата (используется в БД)
type ChatType uint8

const (
	TelegramBot ChatType = 0
	Web         ChatType = 1
	Telegram    ChatType = 2
	Avito       ChatType = 3
	Widget      ChatType = 4
	WhatsApp    ChatType = 5
	Instagram   ChatType = 6
)

type CreatorType uint8

const (
	AI                 CreatorType = 1 // Право
	User               CreatorType = 2 // Лево
	UserVoice          CreatorType = 3 // Лево
	Operator           CreatorType = 4 // Прав
	SpeechRealTimeAI   CreatorType = 5 // Прав
	SpeechRealTimeUser CreatorType = 6 // Лево
)
