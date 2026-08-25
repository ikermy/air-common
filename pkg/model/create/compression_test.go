package create

import (
	"testing"

	"github.com/ikermy/air-common/pkg/comdom"
)

func TestModelDataCompressionRoundTripPreservesRealtimeAndVectorIDs(t *testing.T) {
	data := &comdom.UniversalModelData{
		Name:         "mistral-voice",
		Provider:     comdom.ProviderMistral,
		Realtime:     true,
		RealtimeVAD:  &comdom.RealtimeVAD{Mistral: &comdom.MistralRealtimeVAD{}},
		UseModelName: &comdom.UseModelName{GptType: &comdom.GptType{ID: 11}, Realtime: &comdom.Realtime{ID: 12}},
	}
	compressed, err := compressModelData(data)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := decompressModelData(compressed, &comdom.VecIds{FileIds: []comdom.Ids{{ID: "file-1"}}, VectorId: []string{"vec-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Name != data.Name || restored.Provider != comdom.ProviderMistral || len(restored.FileIds) != 1 || len(restored.VecIds.VectorId) != 1 {
		t.Fatalf("unexpected restored model: %+v", restored)
	}
	if restored.RealtimeVAD.Mistral.SpeechFormat == nil {
		t.Fatal("Mistral realtime defaults were not applied")
	}
}
