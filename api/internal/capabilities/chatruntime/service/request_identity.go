package service

import (
	"context"
	"strings"

	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// streamRequestIDKey carries the transport request identity from the HTTP edge
// into the stream so error frames can be correlated with the request that
// produced them.
type streamRequestIDKey struct{}

// WithStreamRequestID records the inbound transport request id on the context.
// The runtime derives its persistence context with context.WithoutCancel, which
// keeps values, so the id survives into the error finalization path.
func WithStreamRequestID(ctx context.Context, requestID string) context.Context {
	trimmed := strings.TrimSpace(requestID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, streamRequestIDKey{}, trimmed)
}

// StreamRequestID returns the transport request id recorded on the context, or
// an empty string when the caller did not supply one.
func StreamRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(streamRequestIDKey{}).(string)
	return strings.TrimSpace(requestID)
}

// streamLocaleKey carries the caller's Accept-Language from the HTTP edge into
// the stream so error frames are rendered in the caller's language.
type streamLocaleKey struct{}

// WithStreamLocale records the locale negotiated from an Accept-Language
// header. An unsupported or empty header records nothing, and the catalog's
// fallback language applies.
func WithStreamLocale(ctx context.Context, acceptLanguage string) context.Context {
	locale := apptransport.LocaleFromAcceptLanguage(acceptLanguage)
	if locale == "" {
		return ctx
	}
	return context.WithValue(ctx, streamLocaleKey{}, locale)
}

// StreamLocale returns the locale recorded on the context, or the zero Locale.
func StreamLocale(ctx context.Context) appcatalog.Locale {
	if ctx == nil {
		return ""
	}
	locale, _ := ctx.Value(streamLocaleKey{}).(appcatalog.Locale)
	return locale
}
