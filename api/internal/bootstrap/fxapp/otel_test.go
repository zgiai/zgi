package fxapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestBuildOTLPHTTPOptionsAppendsTracePathForBaseEndpoint(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotIngestionVersion string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotIngestionVersion = r.Header.Get("x-langfuse-ingestion-version")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts, _, insecure, err := buildOTLPHTTPOptions(server.URL+"/api/public/otel", "", map[string]string{
		"Authorization":                "Basic test",
		"x-langfuse-ingestion-version": "4",
	})
	if err != nil {
		t.Fatalf("buildOTLPHTTPOptions() error = %v, want nil", err)
	}
	if !insecure {
		t.Fatal("insecure = false, want true for http endpoint")
	}

	exportSpan(t, opts)

	if gotPath != "/api/public/otel/v1/traces" {
		t.Fatalf("export path = %q, want /api/public/otel/v1/traces", gotPath)
	}
	if gotAuth != "Basic test" {
		t.Fatalf("Authorization header = %q, want Basic test", gotAuth)
	}
	if gotIngestionVersion != "4" {
		t.Fatalf("x-langfuse-ingestion-version = %q, want 4", gotIngestionVersion)
	}
}

func TestBuildOTLPHTTPOptionsUsesSignalSpecificTraceEndpoint(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts, _, _, err := buildOTLPHTTPOptions("http://unused:4318", server.URL+"/custom/v1/traces", nil)
	if err != nil {
		t.Fatalf("buildOTLPHTTPOptions() error = %v, want nil", err)
	}

	exportSpan(t, opts)

	if gotPath != "/custom/v1/traces" {
		t.Fatalf("export path = %q, want /custom/v1/traces", gotPath)
	}
}

func TestBuildOTLPHTTPOptionsRequiresEndpoint(t *testing.T) {
	_, _, _, err := buildOTLPHTTPOptions("", "", nil)
	if !errors.Is(err, errMissingOTELEndpoint) {
		t.Fatalf("buildOTLPHTTPOptions() error = %v, want errMissingOTELEndpoint", err)
	}
}

func TestBuildOTELSamplerUsesConfiguredRate(t *testing.T) {
	if description := buildOTELSampler(1).Description(); !strings.Contains(description, "AlwaysOn") {
		t.Fatalf("sample rate 1 description = %q, want AlwaysOn", description)
	}
	if description := buildOTELSampler(0).Description(); !strings.Contains(description, "AlwaysOff") {
		t.Fatalf("sample rate 0 description = %q, want AlwaysOff", description)
	}
	if description := buildOTELSampler(0.25).Description(); !strings.Contains(description, "TraceIDRatioBased") {
		t.Fatalf("sample rate 0.25 description = %q, want TraceIDRatioBased", description)
	}
	if got := normalizedOTELSampleRate(2); got != 1 {
		t.Fatalf("normalizedOTELSampleRate(2) = %v, want 1", got)
	}
	if got := normalizedOTELSampleRate(-0.1); got != 0 {
		t.Fatalf("normalizedOTELSampleRate(-0.1) = %v, want 0", got)
	}
}

func exportSpan(t *testing.T, opts []otlptracehttp.Option) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		t.Fatalf("otlptracehttp.New() error = %v, want nil", err)
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	_, span := tp.Tracer("test").Start(ctx, "test-span")
	span.End()
	if err := tp.Shutdown(ctx); err != nil {
		t.Fatalf("TracerProvider.Shutdown() error = %v, want nil", err)
	}
}
