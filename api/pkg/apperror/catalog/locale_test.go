package catalog_test

import (
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestParseLocale(t *testing.T) {
	t.Parallel()

	tests := map[string]catalog.Locale{
		"en":      catalog.LocaleEnglishUS,
		"EN_us":   catalog.LocaleEnglishUS,
		"zh":      catalog.LocaleChineseSimplified,
		"zh-CN":   catalog.LocaleChineseSimplified,
		"zh_Hans": catalog.LocaleChineseSimplified,
	}
	for input, want := range tests {
		got, ok := catalog.ParseLocale(input)
		if !ok || got != want {
			t.Fatalf("ParseLocale(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
	if _, ok := catalog.ParseLocale("fr-FR"); ok {
		t.Fatal("unsupported locale unexpectedly matched")
	}
}

func TestParseLegacyKey(t *testing.T) {
	t.Parallel()

	if got, err := catalog.ParseLegacyKey("llm.gateway:40101"); err != nil || got.String() != "llm.gateway:40101" {
		t.Fatalf("ParseLegacyKey() = %q, %v", got, err)
	}
	for _, invalid := range []string{"", "40101", "gateway:40101", "LLM.gateway:40101", "llm.gateway:", "llm.gateway:bad value"} {
		if _, err := catalog.ParseLegacyKey(invalid); err == nil {
			t.Fatalf("ParseLegacyKey(%q) succeeded", invalid)
		}
	}
}
