package mistral

import "testing"

func TestHasMistralSpeechText(t *testing.T) {
	for _, text := range []string{"😏", "!!!", "...   ", "*улыбается*"} {
		if hasMistralSpeechText(text) {
			t.Errorf("hasMistralSpeechText(%q) = true, want false", text)
		}
	}
	for _, text := range []string{"привет", "hello", "42", "😏 хорошо"} {
		if !hasMistralSpeechText(text) {
			t.Errorf("hasMistralSpeechText(%q) = false, want true", text)
		}
	}
}
