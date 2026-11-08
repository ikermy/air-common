package comdom

import "testing"

func TestChannelRegistryConsistency(t *testing.T) {
	slugs := make(map[string]ChannelType)
	codes := make(map[uint16]string)
	legacy := make(map[string]string)

	for _, info := range ChannelInfos() {
		if info.Slug == "" {
			t.Fatalf("channel %d has empty slug", info.Type)
		}
		if prev, ok := slugs[info.Slug]; ok {
			t.Fatalf("duplicate slug %q for types %d and %d", info.Slug, prev, info.Type)
		}
		slugs[info.Slug] = info.Type

		if prev, ok := codes[info.Code]; ok && info.Configurable {
			t.Fatalf("duplicate configurable code %d for %s and %s", info.Code, prev, info.Slug)
		}
		codes[info.Code] = info.Slug

		if info.LegacyName != "" {
			key := stringsLower(info.LegacyName)
			if prev, ok := legacy[key]; ok {
				t.Fatalf("duplicate legacy name %q for %s and %s", info.LegacyName, prev, info.Slug)
			}
			legacy[key] = info.Slug
		}

		if info.Configurable && info.LegacyColumn == "" {
			t.Fatalf("configurable channel %s has no legacy column", info.Slug)
		}
	}
}

func TestChannelRegistryLookups(t *testing.T) {
	for _, info := range ChannelInfos() {
		bySlug, ok := ChannelBySlug(info.Slug)
		if !ok || bySlug.Type != info.Type {
			t.Fatalf("ChannelBySlug(%q) = %+v, %v", info.Slug, bySlug, ok)
		}
		byType, ok := ChannelByType(info.Type)
		if !ok || byType.Slug != info.Slug {
			t.Fatalf("ChannelByType(%d) = %+v, %v", info.Type, byType, ok)
		}
		for _, alias := range append([]string{info.LegacyName}, info.Aliases...) {
			if alias == "" {
				continue
			}
			got, ok := ChannelBySlug(alias)
			if !ok || got.Type != info.Type {
				t.Fatalf("ChannelBySlug(%q) = %+v, %v; want %d", alias, got, ok, info.Type)
			}
		}
	}
}

func TestConfigurableChannelsOrder(t *testing.T) {
	want := []string{"tgbot", "widget", "tguserbot", "whatsbot", "insta", "avito"}
	got := ConfigurableChannels()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Slug != want[i] {
			t.Fatalf("channel[%d] = %s, want %s", i, got[i].Slug, want[i])
		}
		if got[i].Code == 0 {
			t.Fatalf("channel %s has zero code", got[i].Slug)
		}
	}
}

func TestFromStringUsesRegistry(t *testing.T) {
	for _, alias := range []string{"tgubot", "tguserbot", "TelegramUserBot", "whats", "WhatsApp", "widg", "insta", "Instagram"} {
		if _, err := ChannelType(0).FromString(alias); err != nil {
			t.Fatalf("FromString(%q): %v", alias, err)
		}
	}
	if _, err := ChannelType(0).FromString("unknown"); err == nil {
		t.Fatal("FromString(unknown) should fail")
	}
}

func stringsLower(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}
