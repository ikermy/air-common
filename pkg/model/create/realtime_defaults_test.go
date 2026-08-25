package create

import (
	"testing"

	"github.com/ikermy/air-common/pkg/comdom"
)

func TestApplyRealtimeVADDefaultsMistral(t *testing.T) {
	vad := applyRealtimeVADDefaults(&comdom.RealtimeVAD{Mistral: &comdom.MistralRealtimeVAD{}})
	if vad.Mistral == nil {
		t.Fatal("Mistral defaults are missing")
	}
	if vad.Mistral.SpeechFormat == nil || *vad.Mistral.SpeechFormat != "wav" {
		t.Fatalf("Mistral speech format = %v, want wav", vad.Mistral.SpeechFormat)
	}
}

func TestApplyRealtimeVADDefaultsPreservesMistralValues(t *testing.T) {
	format := "mp3"
	vad := applyRealtimeVADDefaults(&comdom.RealtimeVAD{Mistral: &comdom.MistralRealtimeVAD{
		SpeechFormat: &format,
	}})
	if *vad.Mistral.SpeechFormat != "mp3" {
		t.Fatal("explicit Mistral values were overwritten")
	}
}
