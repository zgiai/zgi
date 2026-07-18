package observability

import (
	"context"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

// SentryReporter adapts ZGI Reporter events to the Sentry SDK.
type SentryReporter struct{}

func NewSentryReporter() Reporter {
	return SentryReporter{}
}

func (SentryReporter) Name() string { return "sentry" }

func (SentryReporter) Report(ctx context.Context, event Event) error {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}

	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentryLevel(event.Level))
		scope.SetTag("zgi.event", event.Name)
		scope.SetTag("zgi.kind", string(event.Kind))
		for key, value := range event.Tags {
			scope.SetTag(key, value)
		}
		for key, value := range event.Attributes {
			scope.SetExtra(key, value)
		}

		if event.Err != nil {
			hub.CaptureException(sanitizedError{message: sanitizeReporterString(event.Err.Error())})
			return
		}
		hub.CaptureMessage(event.Name)
	})
	return nil
}

func (SentryReporter) Flush(ctx context.Context) error {
	timeout := 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ctx.Err()
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	if sentry.Flush(timeout) {
		return nil
	}
	return context.DeadlineExceeded
}

func sentryLevel(level Level) sentry.Level {
	switch level {
	case LevelDebug:
		return sentry.LevelDebug
	case LevelInfo:
		return sentry.LevelInfo
	case LevelWarning:
		return sentry.LevelWarning
	case LevelFatal:
		return sentry.LevelFatal
	default:
		return sentry.LevelError
	}
}

// SanitizeSentryEvent applies the same privacy policy to events produced by
// automatic Sentry integrations, not only events created through ZGIReporter.
func SanitizeSentryEvent(event *sentry.Event) *sentry.Event {
	if event == nil {
		return nil
	}
	event.Message = sanitizeReporterString(event.Message)
	for i := range event.Exception {
		event.Exception[i].Value = sanitizeReporterString(event.Exception[i].Value)
	}
	event.Tags = sanitizeReporterTags(event.Tags)
	event.Extra = SanitizeReporterAttributes(event.Extra)
	for name, values := range event.Contexts {
		event.Contexts[name] = SanitizeReporterAttributes(values)
	}
	for _, breadcrumb := range event.Breadcrumbs {
		if breadcrumb != nil {
			breadcrumb.Message = sanitizeReporterString(breadcrumb.Message)
			breadcrumb.Data = SanitizeReporterAttributes(breadcrumb.Data)
		}
	}
	if event.Request != nil {
		event.Request.URL = sanitizeReporterURL(event.Request.URL)
		event.Request.Data = ""
		event.Request.QueryString = ""
		event.Request.Cookies = ""
		event.Request.Headers = sanitizeSentryRequestHeaders(event.Request.Headers)
		event.Request.Env = nil
	}
	event.User.Email = ""
	event.User.IPAddress = ""
	event.User.Username = ""
	event.User.Name = ""
	event.User.Data = nil
	return event
}

func sanitizeSentryRequestHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"accept":         {},
		"content-length": {},
		"content-type":   {},
		"user-agent":     {},
		"x-request-id":   {},
	}
	result := make(map[string]string, len(allowed))
	for key, value := range headers {
		if _, ok := allowed[strings.ToLower(key)]; ok {
			result[key] = sanitizeReporterString(value)
		}
	}
	return result
}
