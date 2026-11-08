package comdom

import "strings"

// ChannelInfo is the canonical description of a channel.
//
// It is the single source of truth for:
//   - canonical storage/API slug (user_channels.channel_type, HTTP/API)
//   - legacy stored-procedure name (SetChannelEnabled/DisableAllUserChannel)
//   - legacy channels column (channels.TgBot, ...)
//   - deterministic v2 id suffix (user_id*10+code)
type ChannelInfo struct {
	Type         ChannelType
	Slug         string
	LegacyName   string
	LegacyColumn string
	Code         uint16
	Configurable bool
	Aliases      []string
}

var channelInfos = []ChannelInfo{
	{
		Type:         TelegramBot,
		Slug:         "tgbot",
		LegacyName:   "TgBot",
		LegacyColumn: "TgBot",
		Code:         1,
		Configurable: true,
		Aliases:      []string{"TelegramBot", "telegram_bot", "telegrambot", "tg_bot", "tg"},
	},
	{
		Type:         Widget,
		Slug:         "widget",
		LegacyName:   "Widget",
		LegacyColumn: "Widget",
		Code:         2,
		Configurable: true,
		Aliases:      []string{"Widget", "widg"},
	},
	{
		Type:         Telegram,
		Slug:         "tguserbot",
		LegacyName:   "TgUserBot",
		LegacyColumn: "TgUserBot",
		Code:         3,
		Configurable: true,
		Aliases:      []string{"TelegramUserBot", "telegram_user_bot", "telegramuserbot", "tgubot", "tg_user_bot"},
	},
	{
		Type:         WhatsApp,
		Slug:         "whatsbot",
		LegacyName:   "Whats",
		LegacyColumn: "Whats",
		Code:         4,
		Configurable: true,
		Aliases:      []string{"WhatsApp", "whatsapp", "whats"},
	},
	{
		Type:         Instagram,
		Slug:         "insta",
		LegacyName:   "Insta",
		LegacyColumn: "Insta",
		Code:         5,
		Configurable: true,
		Aliases:      []string{"Instagram", "instagram"},
	},
	{
		Type:         Avito,
		Slug:         "avito",
		LegacyName:   "Avito",
		LegacyColumn: "Avito",
		Code:         6,
		Configurable: true,
		Aliases:      []string{"Avito"},
	},
	{
		// Web is a chat type used by web sessions, not a configurable channel.
		Type:         Web,
		Slug:         "web",
		LegacyName:   "Web",
		LegacyColumn: "",
		Code:         0,
		Configurable: false,
		Aliases:      []string{"Web"},
	},
}

var (
	channelBySlug   = make(map[string]ChannelInfo, len(channelInfos)*3)
	channelByType   = make(map[ChannelType]ChannelInfo, len(channelInfos))
	channelByLegacy = make(map[string]ChannelInfo, len(channelInfos))
)

func init() {
	for _, info := range channelInfos {
		channelByType[info.Type] = info
		channelByLegacy[strings.ToLower(info.LegacyName)] = info
		channelByLegacy[strings.ToLower(info.Slug)] = info

		keys := append([]string{info.Slug, info.LegacyName, info.Type.String()}, info.Aliases...)
		for _, key := range keys {
			if key == "" {
				continue
			}
			channelBySlug[strings.ToLower(key)] = info
		}
	}
}

// AllChannels — все известные ChannelType, включая неконфигурируемые (Web).
var AllChannels = func() []ChannelType {
	out := make([]ChannelType, 0, len(channelInfos))
	for _, info := range channelInfos {
		out = append(out, info.Type)
	}
	return out
}()

// ChannelInfos returns all known channels in canonical order.
func ChannelInfos() []ChannelInfo {
	out := make([]ChannelInfo, len(channelInfos))
	copy(out, channelInfos)
	return out
}

// ConfigurableChannels returns channels that can be configured per user, in
// the canonical UI/order and deterministic-id order.
func ConfigurableChannels() []ChannelInfo {
	out := make([]ChannelInfo, 0, len(channelInfos))
	for _, info := range channelInfos {
		if info.Configurable {
			out = append(out, info)
		}
	}
	return out
}

// ChannelBySlug resolves a canonical slug or any known alias.
func ChannelBySlug(slug string) (ChannelInfo, bool) {
	info, ok := channelBySlug[strings.ToLower(strings.TrimSpace(slug))]
	return info, ok
}

// ChannelByType resolves a channel by its ChannelType value.
func ChannelByType(t ChannelType) (ChannelInfo, bool) {
	info, ok := channelByType[t]
	return info, ok
}

// ChannelByLegacyName resolves a legacy stored-procedure name or slug.
func ChannelByLegacyName(name string) (ChannelInfo, bool) {
	info, ok := channelByLegacy[strings.ToLower(strings.TrimSpace(name))]
	return info, ok
}

// NormalizeChannelSlug returns the canonical slug for any known alias.
func NormalizeChannelSlug(value string) (string, bool) {
	info, ok := ChannelBySlug(value)
	if !ok {
		return "", false
	}
	return info.Slug, true
}

// Slug returns the canonical storage/API slug for the channel type.
func (c ChannelType) Slug() string {
	if info, ok := ChannelByType(c); ok {
		return info.Slug
	}
	return "unknown"
}

// LegacyName returns the legacy stored-procedure name for the channel type.
func (c ChannelType) LegacyName() string {
	if info, ok := ChannelByType(c); ok {
		return info.LegacyName
	}
	return "unknown"
}

// V2Code returns the deterministic user_channels id suffix for the channel type.
func (c ChannelType) V2Code() uint16 {
	if info, ok := ChannelByType(c); ok {
		return info.Code
	}
	return 0
}
