package model

import "testing"

func TestCascadeTextExtractor_MistralEnvelopeIncremental(t *testing.T) {
	var e cascadeTextExtractor
	if got := e.Push(`{"message":"Sa`); got != "Sa" {
		t.Fatalf("chunk1=%q", got)
	}
	if got := e.Push(`lut !`); got != "lut !" {
		t.Fatalf("chunk2=%q", got)
	}
	if got := e.Push(` Je vais bien"}`); got != " Je vais bien" {
		t.Fatalf("chunk3=%q", got)
	}
	if got := e.Push(` ignored`); got != "" {
		t.Fatalf("after done chunk must be ignored, got %q", got)
	}
}

func TestCascadeTextExtractor_FullEnvelopeWithSpace(t *testing.T) {
	var e cascadeTextExtractor
	got := e.Push(`{"message": "Salut ! Je vais super bien, merci de demander."}`)
	want := "Salut ! Je vais super bien, merci de demander."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCascadeTextExtractor_PlainTextPassthroughAndControlFrames(t *testing.T) {
	var e cascadeTextExtractor
	if got := e.Push("Bonjour"); got != "Bonjour" {
		t.Fatalf("plain text=%q", got)
	}
	if got := e.Push(" tout le monde"); got != " tout le monde" {
		t.Fatalf("plain text2=%q", got)
	}
	if got := e.Push(`{"type":"token_usage","usage":{"total_tokens":10}}`); got != "" {
		t.Fatalf("control frame must be ignored, got %q", got)
	}
}

func TestCascadeTextExtractor_Escapes(t *testing.T) {
	var e cascadeTextExtractor
	got := e.Push(`{"message":"ligne1\nligne2 \"citation\""}`)
	want := "ligne1\nligne2 \"citation\""
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCascadeTextExtractor_FlushUnclosed(t *testing.T) {
	var e cascadeTextExtractor
	if got := e.Push(`{"message":"abc`); got != "abc" {
		t.Fatalf("push=%q", got)
	}
	if got := e.Flush(); got != "" {
		t.Fatalf("flush must not duplicate, got %q", got)
	}
}

func TestCascadeTextExtractor_NonStringMessage(t *testing.T) {
	var e cascadeTextExtractor
	if got := e.Push(`{"message":123}`); got != "" {
		t.Fatalf("non-string message must be ignored, got %q", got)
	}
}

func TestCascadeTextExtractor_IgnoresActionEnvelope(t *testing.T) {
	var e cascadeTextExtractor
	payload := `{"message": "Привет-привет! У Василич всё муррр-мяу! \nДаже шерсть шевелится?","action":{"type":"send","text":"code"}}`
	want := "Привет-привет! У Василич всё муррр-мяу! \nДаже шерсть шевелится?"
	if got := e.Push(payload); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := e.Flush(); got != "" {
		t.Fatalf("flush must be empty after closed message, got %q", got)
	}
}

func TestCascadeTextExtractor_SplitAcrossChunksActionEnvelope(t *testing.T) {
	var e cascadeTextExtractor
	var got string
	for _, chunk := range []string{`{"message":"Привет`, `-привет! Всё `, `муррр","action":{}}`} {
		got += e.Push(chunk)
	}
	if got != "Привет-привет! Всё муррр" {
		t.Fatalf("got %q", got)
	}
}

func TestCascadeTextExtractor_EnvelopeWithoutLeadingBrace(t *testing.T) {
	var e cascadeTextExtractor
	var got string
	for _, chunk := range []string{`data: {"mess`, `age":"Привет"}`, `{"type":"token_usage"}`} {
		got += e.Push(chunk)
	}
	if got != "Привет" {
		t.Fatalf("got %q", got)
	}
}
