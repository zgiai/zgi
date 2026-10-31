package transport

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestLocaleContextRecordsSupportedLanguagesOnly(t *testing.T) {
	ctx := WithLocale(context.Background(), "zh-CN,zh;q=0.9,en;q=0.8")
	if got := LocaleFromContext(ctx); got != catalog.LocaleChineseSimplified {
		t.Fatalf("LocaleFromContext() = %q, want zh-Hans", got)
	}
	if got := LocaleFromContext(WithLocale(context.Background(), "fr-FR")); got != "" {
		t.Fatalf("unsupported language recorded as %q", got)
	}
}
