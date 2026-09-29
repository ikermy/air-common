package model

import "testing"

func TestCatalogThrottle(t *testing.T) {
	var th catalogThrottle
	key := "voice:4"

	if !th.claim(key) {
		t.Fatalf("first claim must succeed")
	}
	if th.claim(key) {
		t.Fatalf("second claim while busy must fail")
	}
	if th.fresh(key) {
		t.Fatalf("must not be fresh before success release")
	}

	th.release(key, true)
	if !th.fresh(key) {
		t.Fatalf("must be fresh after successful release")
	}
	if th.claim(key) {
		t.Fatalf("claim must fail within TTL")
	}

	// Неуспешное обновление не фиксирует свежесть.
	key2 := "voice:2"
	if !th.claim(key2) {
		t.Fatalf("claim key2 must succeed")
	}
	th.release(key2, false)
	if th.fresh(key2) {
		t.Fatalf("failed refresh must not mark fresh")
	}
	if !th.claim(key2) {
		t.Fatalf("failed refresh must allow retry")
	}
}

func TestCatalogThrottleMark(t *testing.T) {
	var th catalogThrottle
	th.mark("llm:1:2")
	if !th.fresh("llm:1:2") {
		t.Fatalf("mark must set freshness")
	}
	if th.claim("llm:1:2") {
		t.Fatalf("claim after mark must fail within TTL")
	}
}
