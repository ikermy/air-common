package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func TestInitNoEndpointIsNoop(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	shutdown, enabled, err := Init(context.Background(), "test-service", "test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if enabled {
		t.Fatal("tracing must be disabled without an endpoint")
	}
	if shutdown == nil {
		t.Fatal("shutdown must not be nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestInitEndpointConfiguresProvider(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")

	shutdown, enabled, err := Init(context.Background(), "test-service", "test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !enabled {
		t.Fatal("tracing must be enabled when an endpoint is set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	// Shutdown may return a flush error because no collector is running;
	// the important part is that it is wired and callable.
	_ = shutdown(ctx)
}

func TestInitExportsSpan(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)

	shutdown, enabled, err := Init(context.Background(), "test-service", "test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !enabled {
		t.Fatal("tracing must be enabled")
	}

	_, span := otel.Tracer("test").Start(context.Background(), "test-span")
	span.End()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if atomic.LoadInt32(&requests) == 0 {
		t.Fatal("no OTLP export request received")
	}
}
