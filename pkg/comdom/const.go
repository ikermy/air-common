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

// AllChannels is defined in channels.go from the canonical channel registry.

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
	if info, ok := ChannelBySlug(s); ok {
		return info.Type, nil
	}
	return 0, fmt.Errorf("unknown channel: %s", s)
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
