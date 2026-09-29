package model

import (
	"encoding/base64"
	"testing"

	"github.com/ikermy/air-common/pkg/comdom"
)

func TestDecodeVoiceSamplesMultiple(t *testing.T) {
	a := base64.StdEncoding.EncodeToString([]byte("audio-a"))
	b := base64.StdEncoding.EncodeToString([]byte("audio-b"))
	samples, err := decodeVoiceSamples(comdom.CreateVoiceRequest{Samples: []string{a, b}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples=%d, want 2", len(samples))
	}
	if samples[0].FileName != "sample_1.wav" || samples[1].FileName != "sample_2.wav" {
		t.Fatalf("unexpected file names: %s, %s", samples[0].FileName, samples[1].FileName)
	}
}

func TestDecodeVoiceSamplesFallbackSingle(t *testing.T) {
	a := base64.StdEncoding.EncodeToString([]byte("only-audio"))
	samples, err := decodeVoiceSamples(comdom.CreateVoiceRequest{SampleAudio: a})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 1 || string(samples[0].Data) != "only-audio" {
		t.Fatalf("unexpected samples: %+v", samples)
	}
}

func TestDecodeVoiceSamplesEmpty(t *testing.T) {
	if _, err := decodeVoiceSamples(comdom.CreateVoiceRequest{}); err == nil {
		t.Fatalf("expected error for empty samples")
	}
	if _, err := decodeVoiceSamples(comdom.CreateVoiceRequest{SampleAudio: "!!!not-base64!!!"}); err == nil {
		t.Fatalf("expected error for invalid base64")
	}
}
