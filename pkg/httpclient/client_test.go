package httpclient

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewBufferString(`{"x":1}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestRetryOnServerError(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(Options{Retries: 2, Backoff: time.Millisecond})
	resp, err := c.DoWithPolicy(newTestRequest(t, srv.URL), true)
	if err != nil {
		t.Fatalf("DoWithPolicy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts=%d, want 2", got)
	}
}

func TestNoRetryWhenNotAllowed(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(Options{Retries: 2, Backoff: time.Millisecond})
	resp, err := c.DoWithPolicy(newTestRequest(t, srv.URL), false)
	if err != nil {
		t.Fatalf("DoWithPolicy: %v", err)
	}
	defer resp.Body.Close()

	if got := attempts.Load(); got != 1 {
		t.Fatalf("attempts=%d, want 1", got)
	}
}

func TestCircuitBreakerOpens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(Options{Retries: 0, FailureThreshold: 2, OpenFor: time.Hour})
	for i := 0; i < 2; i++ {
		resp, err := c.Do(newTestRequest(t, srv.URL))
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		resp.Body.Close()
	}

	if _, err := c.Do(newTestRequest(t, srv.URL)); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
}
