package transport

import (
	"context"

	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

type localeContextKey struct{}

// WithLocale records the locale negotiated from a request's Accept-Language
// header. An unsupported or empty header records nothing.
func WithLocale(ctx context.Context, acceptLanguage string) context.Context {
	locale := LocaleFromAcceptLanguage(acceptLanguage)
	if ctx == nil || locale == "" {
		return ctx
	}
	return context.WithValue(ctx, localeContextKey{}, locale)
}

// LocaleFromContext returns the locale recorded on the context, or the zero
// Locale.
func LocaleFromContext(ctx context.Context) catalog.Locale {
	if ctx == nil {
		return ""
	}
	locale, _ := ctx.Value(localeContextKey{}).(catalog.Locale)
	return locale
}
