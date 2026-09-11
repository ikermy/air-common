package comdom

import "fmt"

// ChannelType определяет тип чата (используется в БД)
type ChannelType uint8

const (
	TelegramBot ChannelType = 0
	Web         ChannelType = 1
	Telegram    ChannelType = 2
	Avito       ChannelType = 3
	Widget      ChannelType = 4
	WhatsApp    ChannelType = 5
	Instagram   ChannelType = 6
)

// AllChannels — валидные каналы (значения 0–6 заняты в БД).
// Значение 255 не занято и трактуется как "unknown" (именованной константы нет).
var AllChannels = []ChannelType{TelegramBot, Web, Telegram, Avito, Widget, WhatsApp, Instagram}

func (c ChannelType) String() string {
	switch c {
	case TelegramBot:
		return "TelegramBot"
	case Web:
		return "Web"
	case Telegram:
		return "TelegramUserBot"
	case Avito:
		return "Avito"
	case Widget:
		return "Widget"
	case WhatsApp:
		return "WhatsApp"
	case Instagram:
		return "Instagram"
	default:
		return "unknown"
	}
}

func (c ChannelType) FromString(s string) (ChannelType, error) {
	switch s {
	case "TelegramBot", "telegram_bot", "telegrambot", "tg_bot", "tgbot":
		return TelegramBot, nil
	case "Web", "web":
		return Web, nil
	case "TelegramUserBot", "tgubot", "telegram_user_bot", "telegramuserbot":
		return Telegram, nil
	case "Avito", "avito":
		return Avito, nil
	case "Widget", "widget":
		return Widget, nil
	case "WhatsApp", "whatsapp":
		return WhatsApp, nil
	case "Instagram", "instagram", "insta":
		return Instagram, nil
	default:
		return 0, fmt.Errorf("unknown channel: %s", s)
	}
}

func (c ChannelType) FromUint8(value uint8) ChannelType { return ChannelType(value) }

func (c ChannelType) IsValid() bool {
	for _, known := range AllChannels {
		if c == known {
			return true
		}
	}
	return false
}

type CreatorType uint8

const (
	AI                 CreatorType = 1 // Право
	User               CreatorType = 2 // Лево
	UserVoice          CreatorType = 3 // Лево
	Operator           CreatorType = 4 // Право
	SpeechRealTimeAI   CreatorType = 5 // Право
	SpeechRealTimeUser CreatorType = 6 // Лево
)
