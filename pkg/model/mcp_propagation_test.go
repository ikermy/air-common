package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ikermy/air-common/v2/pkg/comdom"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestMCPPropagatesTraceContext(t *testing.T) {
	var gotTraceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("traceparent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"tools":[]}}`))
	}))
	defer srv.Close()

	t.Setenv("MCP_URL", srv.URL)
	t.Setenv("MCP_SESSION_SECRET", "")

	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer otel.SetTextMapPropagator(prevProp)

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("span id: %v", err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	handler := NewUniversalActionHandler(ctx)
	if _, err := handler.FetchToolsList(ctx, 1, comdom.ProviderType(1)); err != nil {
		t.Fatalf("FetchToolsList: %v", err)
	}

	want := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if gotTraceparent != want {
		t.Fatalf("traceparent=%q, want %q", gotTraceparent, want)
	}
}

func TestMCPPropagatesServiceKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Service-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"tools":[]}}`))
	}))
	defer srv.Close()

	t.Setenv("MCP_URL", srv.URL)
	t.Setenv("MCP_SESSION_SECRET", "")
	t.Setenv("MCP_SERVICE_KEY", "service-test-key")

	handler := NewUniversalActionHandler(context.Background())
	if _, err := handler.FetchToolsList(context.Background(), 1, comdom.ProviderType(1)); err != nil {
		t.Fatalf("FetchToolsList: %v", err)
	}
	if gotKey != "service-test-key" {
		t.Fatalf("X-Service-Key=%q, want service-test-key", gotKey)
	}
}
