package observability_test

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestLangfuseTraceAttributeSpanProcessorCopiesContextAttributes(t *testing.T) {
	restoreConfig := setOpenTelemetryConfigForTest(t, true, true)
	defer restoreConfig()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(observability.NewLangfuseTraceAttributeSpanProcessor()),
		sdktrace.WithSpanProcessor(recorder),
	)
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Fatalf("tp.Shutdown() error = %v", err)
		}
	}()

	ctx := observability.WithLangfuseTraceAttributes(context.Background(),
		attribute.String("langfuse.user.id", "user-1"),
		attribute.String("langfuse.session.id", "session-1"),
		attribute.String("http.method", "POST"),
		attribute.StringSlice("langfuse.trace.tags", []string{"zgi", "llm"}),
	)

	_, span := tp.Tracer("test").Start(ctx, "provider.call")
	span.End()

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("len(recorder.Ended()) = %d, want 1", len(ended))
	}

	attrs := attributesByKey(ended[0].Attributes())
	if attrs["langfuse.user.id"].Value.AsString() != "user-1" {
		t.Fatalf("langfuse.user.id = %q, want user-1", attrs["langfuse.user.id"].Value.AsString())
	}
	if attrs["langfuse.session.id"].Value.AsString() != "session-1" {
		t.Fatalf("langfuse.session.id = %q, want session-1", attrs["langfuse.session.id"].Value.AsString())
	}
	if _, ok := attrs["http.method"]; ok {
		t.Fatal("http.method was copied, want only langfuse.* attributes")
	}
	if got := attrs["langfuse.trace.tags"].Value.AsStringSlice(); len(got) != 2 {
		t.Fatalf("langfuse.trace.tags = %v, want two tags", got)
	}
}

func TestLangfuseTraceAttributesDisabled(t *testing.T) {
	restoreConfig := setOpenTelemetryConfigForTest(t, true, false)
	defer restoreConfig()

	ctx := observability.WithLangfuseTraceAttributes(context.Background(),
		attribute.String("langfuse.user.id", "user-1"),
	)

	if attrs := observability.LangfuseTraceAttributesFromContext(ctx); len(attrs) != 0 {
		t.Fatalf("len(LangfuseTraceAttributesFromContext()) = %d, want 0", len(attrs))
	}
}

func TestLangfuseTraceAttributesSanitizeInvalidUTF8(t *testing.T) {
	restoreConfig := setOpenTelemetryConfigForTest(t, true, true)
	defer restoreConfig()

	invalid := "bad-" + string([]byte{0xff}) + "-value"
	ctx := observability.WithLangfuseTraceAttributes(context.Background(),
		attribute.String("langfuse.user.id", invalid),
		attribute.StringSlice("langfuse.trace.tags", []string{"zgi", invalid}),
	)

	attrs := observability.LangfuseTraceAttributesFromContext(ctx)
	if len(attrs) == 0 {
		t.Fatal("LangfuseTraceAttributesFromContext returned no attributes")
	}

	for _, attr := range attrs {
		switch attr.Value.Type() {
		case attribute.STRING:
			if !utf8.ValidString(attr.Value.AsString()) {
				t.Fatalf("%s has invalid UTF-8 value: %q", attr.Key, attr.Value.AsString())
			}
		case attribute.STRINGSLICE:
			for _, value := range attr.Value.AsStringSlice() {
				if !utf8.ValidString(value) {
					t.Fatalf("%s has invalid UTF-8 slice value: %q", attr.Key, value)
				}
			}
		}
	}
}

func setOpenTelemetryConfigForTest(t *testing.T, enabled bool, langfuseEnabled bool) func() {
	t.Helper()
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		OpenTelemetry: config.OpenTelemetryConfig{
			Enabled:               enabled,
			LLMLangfuseAttributes: langfuseEnabled,
		},
	}
	return func() {
		config.GlobalConfig = previous
	}
}

func attributesByKey(attrs []attribute.KeyValue) map[string]attribute.KeyValue {
	out := make(map[string]attribute.KeyValue, len(attrs))
	for _, attr := range attrs {
		out[string(attr.Key)] = attr
	}
	return out
}
