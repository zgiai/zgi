package observability

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/getsentry/sentry-go"
)

type recordingReporter struct {
	name   string
	events []Event
	err    error
	panic  bool
}

func (r *recordingReporter) Name() string { return r.name }

func (r *recordingReporter) Report(_ context.Context, event Event) error {
	if r.panic {
		panic("provider failed")
	}
	r.events = append(r.events, event)
	return r.err
}

func (r *recordingReporter) Flush(context.Context) error { return r.err }

func TestZGIReporterFansOutAndIsolatesProviderFailures(t *testing.T) {
	failing := &recordingReporter{name: "failing", err: errors.New("failed")}
	panicking := &recordingReporter{name: "panicking", panic: true}
	successful := &recordingReporter{name: "successful"}
	reporter := NewZGIReporter(failing, panicking, successful)

	err := reporter.Report(context.Background(), Event{Name: "file.parse.failed", Err: errors.New("parse")})
	if err == nil {
		t.Fatal("Report() error = nil, want aggregated provider errors")
	}
	if len(successful.events) != 1 {
		t.Fatalf("successful reporter received %d events, want 1", len(successful.events))
	}
	if successful.events[0].Kind != EventKindError || successful.events[0].Level != LevelError {
		t.Fatalf("normalized event = %#v, want error/error", successful.events[0])
	}
}

func TestZGIReporterWithoutAdaptersIsNoop(t *testing.T) {
	reporter := NewZGIReporter()
	if reporter.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	if err := reporter.Report(context.Background(), Event{Name: "ignored"}); err != nil {
		t.Fatalf("Report() error = %v, want nil", err)
	}
}

func TestZGIReporterDeduplicatesReporterNames(t *testing.T) {
	first := &recordingReporter{name: "sentry"}
	duplicate := &recordingReporter{name: "SENTRY"}
	reporter := NewZGIReporter(first, duplicate)

	if got, want := reporter.ReporterNames(), []string{"sentry"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ReporterNames() = %#v, want %#v", got, want)
	}
}

func TestReporterSanitizesAttributesBeforeDelivery(t *testing.T) {
	recording := &recordingReporter{name: "test"}
	reporter := NewZGIReporter(recording)

	if err := reporter.Report(context.Background(), Event{
		Name: "workflow.node.failed",
		Tags: map[string]string{"api_key": "secret", "node_type": "llm"},
		Attributes: map[string]any{
			"node_inputs": map[string]any{"prompt": "private"},
			"safe":        "value",
		},
	}); err != nil {
		t.Fatalf("Report() error = %v", err)
	}

	event := recording.events[0]
	if event.Tags["api_key"] != reporterRedactedValue {
		t.Fatalf("api_key tag = %q, want redacted", event.Tags["api_key"])
	}
	if event.Attributes["node_inputs"] != reporterRedactedValue {
		t.Fatalf("node_inputs = %#v, want redacted", event.Attributes["node_inputs"])
	}
	if event.Attributes["safe"] != "value" {
		t.Fatalf("safe = %#v, want value", event.Attributes["safe"])
	}
}

func TestReporterSanitizesErrorsAndURLQueriesBeforeEveryAdapter(t *testing.T) {
	recording := &recordingReporter{name: "custom"}
	reporter := NewZGIReporter(recording)

	err := errors.New("request https://example.com/files?token=secret failed with Bearer private-token")
	if reportErr := reporter.Report(context.Background(), Event{
		Name: "http.request.failed",
		Err:  err,
		Attributes: map[string]any{
			"request_url": "https://example.com/files?workspace_id=private#fragment",
		},
	}); reportErr != nil {
		t.Fatalf("Report() error = %v, want nil", reportErr)
	}

	event := recording.events[0]
	if got := event.Err.Error(); got != "request https://example.com/files failed with "+reporterRedactedValue {
		t.Fatalf("sanitized error = %q", got)
	}
	if got := event.Attributes["request_url"]; got != "https://example.com/files" {
		t.Fatalf("request_url = %#v, want query-free URL", got)
	}
}

func TestSanitizeSentryEventRemovesRequestAndUserPII(t *testing.T) {
	event := &sentry.Event{
		Exception: []sentry.Exception{{Value: "provider rejected sk-secretvalue123"}},
		Extra: map[string]any{
			"node_inputs": "private",
			"safe":        "value",
			"request_url": "https://example.com/files?workspace_id=private",
		},
		Request: &sentry.Request{
			Data:        "body",
			QueryString: "token=secret",
			Cookies:     "session=secret",
			Headers: map[string]string{
				"Authorization":   "Bearer secret",
				"X-API-Key":       "secret",
				"X-Forwarded-For": "192.0.2.1",
				"Content-Type":    "application/json",
			},
			Env: map[string]string{"REMOTE_ADDR": "192.0.2.1"},
		},
		User: sentry.User{
			ID:        "anonymous-id",
			Email:     "person@example.com",
			IPAddress: "192.0.2.1",
			Username:  "person",
		},
	}

	got := SanitizeSentryEvent(event)
	if got.Extra["node_inputs"] != reporterRedactedValue || got.Extra["safe"] != "value" {
		t.Fatalf("Extra = %#v, want redacted node inputs and safe value", got.Extra)
	}
	if got.Extra["request_url"] != "https://example.com/files" {
		t.Fatalf("request_url = %#v, want query-free URL", got.Extra["request_url"])
	}
	if got.Request.Data != "" || got.Request.QueryString != "" || got.Request.Cookies != "" {
		t.Fatalf("Request body/query/cookies were not removed: %#v", got.Request)
	}
	if _, exists := got.Request.Headers["Authorization"]; exists {
		t.Fatal("Authorization header was not removed")
	}
	if _, exists := got.Request.Headers["X-API-Key"]; exists {
		t.Fatal("X-API-Key header was not removed")
	}
	if _, exists := got.Request.Headers["X-Forwarded-For"]; exists {
		t.Fatal("X-Forwarded-For header was not removed")
	}
	if got.Request.Headers["Content-Type"] != "application/json" {
		t.Fatal("safe request header should be retained")
	}
	if got.Request.Env != nil {
		t.Fatalf("Request environment was not removed: %#v", got.Request.Env)
	}
	if got.User.ID != "anonymous-id" || got.User.Email != "" || got.User.IPAddress != "" || got.User.Username != "" {
		t.Fatalf("User = %#v, want only anonymous ID", got.User)
	}
	if got.Exception[0].Value != "provider rejected "+reporterRedactedValue {
		t.Fatalf("Exception value = %q, want credential redacted", got.Exception[0].Value)
	}
}
