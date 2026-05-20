package observability_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

func TestSanitizeStringReturnsValidUTF8(t *testing.T) {
	input := "prefix-" + string([]byte{0xff, 0xfe}) + "-suffix"

	got := observability.SanitizeString(input)

	if !utf8.ValidString(got) {
		t.Fatalf("SanitizeString returned invalid UTF-8: %q", got)
	}
	if !strings.Contains(got, "\uFFFD") {
		t.Fatalf("SanitizeString = %q, want replacement character", got)
	}
}

func TestSanitizeAttributesReturnsValidUTF8Values(t *testing.T) {
	invalid := "bad-" + string([]byte{0xff}) + "-value"
	attrs := []attribute.KeyValue{
		attribute.String("langfuse.observation.input", invalid),
		attribute.StringSlice("langfuse.trace.tags", []string{"zgi", invalid}),
		attribute.Int("gen_ai.usage.total_tokens", 12),
	}

	got := observability.SanitizeAttributes(attrs)

	assertAttributeValuesValidUTF8(t, got)
	if got[2].Value.AsInt64() != 12 {
		t.Fatalf("non-string attribute changed: got %d, want 12", got[2].Value.AsInt64())
	}
}

func TestSanitizeErrorReturnsValidUTF8Message(t *testing.T) {
	invalid := errors.New("trace export failed: " + string([]byte{0xff}))

	got := observability.SanitizeError(invalid)

	if got == nil {
		t.Fatal("SanitizeError returned nil")
	}
	if !utf8.ValidString(got.Error()) {
		t.Fatalf("SanitizeError returned invalid UTF-8 message: %q", got.Error())
	}
}

func assertAttributeValuesValidUTF8(t *testing.T, attrs []attribute.KeyValue) {
	t.Helper()

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
