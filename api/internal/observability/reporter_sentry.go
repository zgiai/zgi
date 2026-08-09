package observability

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/getsentry/sentry-go"
)

// SentryReporter adapts ZGI Reporter events to the Sentry SDK.
type SentryReporter struct{}

var sentryIssueTitles = map[string]string{
	"database.operation.failed":         "Database operation failed",
	"database.query.slow":               "Slow database query",
	"dataset.embedding.failed":          "Dataset embedding failed",
	"file.parse.failed":                 "File parsing failed",
	"http.request.failed":               "HTTP request failed",
	"http.stream.failed":                "Streamed response failed after start",
	"llm.stream.failed":                 "LLM stream failed after response started",
	"llm.provider.selection_failed":     "LLM provider selection failed",
	"llm.provider.request_failed":       "LLM provider request failed",
	"llm.provider.stream_failed":        "LLM provider stream failed",
	"llm.provider.unavailable":          "LLM provider unavailable",
	"llm.adapter.creation_failed":       "LLM adapter creation failed",
	"llm.billing.settlement_failed":     "LLM billing settlement failed",
	"llm.route.not_configured":          "LLM model route not configured",
	"observability.otel.runtime_failed": "OpenTelemetry trace exporter failed",
	"workflow.node.failed":              "Workflow node failed",
}

func NewSentryReporter() Reporter {
	return SentryReporter{}
}

func (SentryReporter) Name() string { return "sentry" }

func (SentryReporter) Report(ctx context.Context, event Event) error {
	hub := sentryHubForReport(ctx)

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

func sentryHubForReport(ctx context.Context) *sentry.Hub {
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		// A single request may report from concurrent branches. Clone its hub for
		// each event so WithScope cannot share a push/pop stack across goroutines;
		// the clone retains request, user, and trace context.
		return hub.Clone()
	}
	// Background jobs have no request hub. Clone the process hub before adding
	// event-specific scope data so concurrent reports cannot share its scope
	// stack or leak tenant/user attributes between events.
	return sentry.CurrentHub().Clone()
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
	if eventName := strings.TrimSpace(event.Tags["zgi.event"]); eventName != "" {
		// The provider-neutral reporter intentionally wraps errors after
		// sanitization. Restore semantic grouping and titles so unrelated ZGI
		// failures do not all appear as observability.sanitizedError.
		event.Fingerprint = []string{"zgi.event", eventName}
		for i := range event.Exception {
			event.Exception[i].Type = sentryIssueTitle(eventName)
			filterSentryReporterFrames(event.Exception[i].Stacktrace)
		}
	}
	return event
}

func filterSentryReporterFrames(stacktrace *sentry.Stacktrace) {
	if stacktrace == nil || len(stacktrace.Frames) == 0 {
		return
	}
	filtered := stacktrace.Frames[:0]
	for _, frame := range stacktrace.Frames {
		if isSentryReporterFrame(frame) {
			continue
		}
		filtered = append(filtered, frame)
	}
	stacktrace.Frames = filtered
}

func isSentryReporterFrame(frame sentry.Frame) bool {
	module := strings.ToLower(frame.Module)
	function := strings.ToLower(frame.Function)
	if strings.HasSuffix(module, "/internal/observability") {
		return true
	}
	if strings.HasSuffix(module, "/middleware") && strings.Contains(function, "zgierrorreporter") {
		return true
	}
	return strings.HasSuffix(module, "/pkg/database") && strings.Contains(function, "reporterplugin")
}

// sentryIssueTitle converts the stable machine-readable event name into the
// operator-facing title shown in Sentry. The event name remains available in
// the zgi.event tag and fingerprint for exact filtering and grouping.
func sentryIssueTitle(eventName string) string {
	if title, ok := sentryIssueTitles[eventName]; ok {
		return title
	}

	words := strings.FieldsFunc(eventName, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	if len(words) == 0 {
		return "ZGI application error"
	}
	for i, word := range words {
		switch strings.ToLower(word) {
		case "api":
			words[i] = "API"
		case "db":
			words[i] = "database"
		case "http":
			words[i] = "HTTP"
		case "llm":
			words[i] = "LLM"
		case "otel":
			words[i] = "OpenTelemetry"
		default:
			words[i] = strings.ToLower(word)
		}
	}
	if words[0] != "API" && words[0] != "HTTP" && words[0] != "LLM" {
		runes := []rune(words[0])
		runes[0] = unicode.ToUpper(runes[0])
		words[0] = string(runes)
	}
	return strings.Join(words, " ")
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
