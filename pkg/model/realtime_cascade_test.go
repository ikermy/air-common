package model

import (
	"context"
	"testing"
)

func TestCascadeChunker(t *testing.T) {
	var c cascadeChunker

	got := c.Push("Привет, мир. Как дела? Всё ", false)
	if len(got) != 2 || got[0] != "Привет, мир." || got[1] != "Как дела?" {
		t.Fatalf("sentences=%v", got)
	}

	got = c.Push("хорошо!", true)
	if len(got) != 1 || got[0] != "Всё хорошо!" {
		t.Fatalf("tail sentences=%v", got)
	}

	if rest := c.Push("", true); len(rest) != 0 {
		t.Fatalf("expected no tail after flush, got %v", rest)
	}
}

func TestCascadeChunkerMultibyte(t *testing.T) {
	var c cascadeChunker
	got := c.Push("слово… дальше", false)
	if len(got) != 1 || got[0] != "слово…" {
		t.Fatalf("multibyte sentences=%v", got)
	}
}

func TestCascadeSessionTurnGuard(t *testing.T) {
	s := &cascadeSession{
		ctx:      context.Background(),
		audioOut: make(chan []byte, 4),
		drain:    make(chan struct{}, 1),
	}

	turn1 := s.beginTurn()
	if !s.isCurrentTurn(turn1) {
		t.Fatalf("turn1 must be current")
	}
	if !s.publishAudio(turn1, []byte{1, 2, 3}) {
		t.Fatalf("audio for current turn must be published")
	}

	turn2 := s.beginTurn()
	if s.isCurrentTurn(turn1) {
		t.Fatalf("turn1 must be invalidated by turn2")
	}
	if s.publishAudio(turn1, []byte{4}) {
		t.Fatalf("audio for stale turn must be rejected")
	}
	if !s.publishAudio(turn2, []byte{5}) {
		t.Fatalf("audio for current turn must be published")
	}

	s.interrupt()
	if s.isCurrentTurn(turn2) {
		t.Fatalf("turn2 must be interrupted")
	}
	select {
	case <-s.drain:
	default:
		t.Fatalf("interrupt must signal drain")
	}
}

func TestCascadeSessionAcceptFinalDedup(t *testing.T) {
	s := &cascadeSession{}
	if !s.acceptFinal("привет") {
		t.Fatalf("first final must be accepted")
	}
	if s.acceptFinal("привет") {
		t.Fatalf("duplicate final must be rejected")
	}
	if s.acceptFinal("  ") {
		t.Fatalf("empty final must be rejected")
	}
	if !s.acceptFinal("пока") {
		t.Fatalf("new final must be accepted")
	}
}
