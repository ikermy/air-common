package comdom

import "testing"

func TestChannelTypeString(t *testing.T) {
	cases := map[ChannelType]string{
		TelegramBot:      "TelegramBot",
		Web:              "Web",
		Telegram:         "TelegramUserBot",
		Avito:            "Avito",
		Widget:           "Widget",
		WhatsApp:         "WhatsApp",
		Instagram:        "Instagram",
		ChannelType(255): "unknown",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("ChannelType(%d).String() = %q, want %q", c, got, want)
		}
	}
}

func TestChannelTypeFromString(t *testing.T) {
	for _, c := range AllChannels {
		got, err := ChannelType(0).FromString(c.String())
		if err != nil {
			t.Errorf("FromString(%q) вернул ошибку: %v", c.String(), err)
			continue
		}
		if got != c {
			t.Errorf("FromString(%q) = %d, want %d", c.String(), got, c)
		}
	}
	if _, err := ChannelType(0).FromString("unknown"); err == nil {
		t.Error("FromString(\"unknown\") должен вернуть ошибку")
	}
}

func TestChannelTypeFromUint8(t *testing.T) {
	if got := ChannelType(0).FromUint8(2); got != Telegram {
		t.Errorf("FromUint8(2) = %d, want %d", got, Telegram)
	}
}

func TestChannelTypeIsValid(t *testing.T) {
	for _, c := range AllChannels {
		if !c.IsValid() {
			t.Errorf("ChannelType(%d).IsValid() = false, want true", c)
		}
	}
	if ChannelType(255).IsValid() {
		t.Error("ChannelType(255).IsValid() = true, want false")
	}
}
