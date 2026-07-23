package observability

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// Reporter is the provider-neutral extension point used by ZGIReporter.
// Implementations must not mutate the supplied event.
type Reporter interface {
	Name() string
	Report(context.Context, Event) error
	Flush(context.Context) error
}

// EventKind identifies the semantic type of a ZGI observability event.
type EventKind string

const (
	EventKindError EventKind = "error"
	EventKindEvent EventKind = "event"
)

// Level is a provider-neutral severity level.
type Level string

const (
	LevelDebug   Level = "debug"
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
	LevelFatal   Level = "fatal"
)

// Event is the stable ZGI Reporter envelope. Name should be a low-cardinality,
// dot-separated identifier such as "llm.provider.selection_failed".
type Event struct {
	Name       string
	Kind       EventKind
	Level      Level
	Err        error
	Tags       map[string]string
	Attributes map[string]any
	OccurredAt time.Time
}

// EventOption enriches an Event without exposing any provider SDK types.
type EventOption func(*Event)

// WithLevel overrides the default event severity.
func WithLevel(level Level) EventOption {
	return func(event *Event) {
		event.Level = level
	}
}

// Tag adds one indexed, low-cardinality attribute.
func Tag(key, value string) EventOption {
	return func(event *Event) {
		if event.Tags == nil {
			event.Tags = make(map[string]string)
		}
		event.Tags[key] = value
	}
}

// Tags adds indexed, low-cardinality attributes.
func Tags(values map[string]string) EventOption {
	return func(event *Event) {
		if event.Tags == nil {
			event.Tags = make(map[string]string, len(values))
		}
		for key, value := range values {
			event.Tags[key] = value
		}
	}
}

// Attribute adds non-indexed diagnostic context.
func Attribute(key string, value any) EventOption {
	return func(event *Event) {
		if event.Attributes == nil {
			event.Attributes = make(map[string]any)
		}
		event.Attributes[key] = value
	}
}

// Attributes adds non-indexed diagnostic context.
func Attributes(values map[string]any) EventOption {
	return func(event *Event) {
		if event.Attributes == nil {
			event.Attributes = make(map[string]any, len(values))
		}
		for key, value := range values {
			event.Attributes[key] = value
		}
	}
}

// ZGIReporter is the branded provider-neutral facade. It fans each event out
// to every registered Reporter, so selecting multiple platforms is a native
// behavior rather than a special integration.
type ZGIReporter struct {
	reporters []Reporter
}

// NewZGIReporter builds a reporter facade. With no adapters it is a No-op.
func NewZGIReporter(reporters ...Reporter) *ZGIReporter {
	registered := make([]Reporter, 0, len(reporters))
	seen := make(map[string]struct{}, len(reporters))
	for _, reporter := range reporters {
		if reporter == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(reporter.Name()))
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		registered = append(registered, reporter)
	}
	return &ZGIReporter{reporters: registered}
}

// Enabled reports whether at least one platform adapter is registered.
func (r *ZGIReporter) Enabled() bool {
	return r != nil && len(r.reporters) > 0
}

// HasReporter reports whether a named platform adapter is registered.
func (r *ZGIReporter) HasReporter(name string) bool {
	if r == nil {
		return false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for _, reporter := range r.reporters {
		if strings.EqualFold(reporter.Name(), name) {
			return true
		}
	}
	return false
}

// ReporterNames returns the registered adapters in execution order.
func (r *ZGIReporter) ReporterNames() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.reporters))
	for _, reporter := range r.reporters {
		names = append(names, reporter.Name())
	}
	return names
}

// Report sends an event to every registered adapter. One adapter failure does
// not prevent delivery to the remaining adapters.
func (r *ZGIReporter) Report(ctx context.Context, event Event) (result error) {
	if r == nil || len(r.reporters) == 0 {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = errors.Join(result, fmt.Errorf("normalize ZGI Reporter event: %v", recovered))
		}
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	event = normalizeEvent(event)

	for _, reporter := range r.reporters {
		if err := reportSafely(ctx, reporter, event); err != nil {
			result = errors.Join(result, fmt.Errorf("report with %s: %w", reporter.Name(), err))
		}
	}
	return result
}

// Flush asks every adapter to deliver buffered events.
func (r *ZGIReporter) Flush(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var result error
	for _, reporter := range r.reporters {
		if err := flushSafely(ctx, reporter); err != nil {
			result = errors.Join(result, fmt.Errorf("flush %s: %w", reporter.Name(), err))
		}
	}
	return result
}

func reportSafely(ctx context.Context, reporter Reporter, event Event) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("reporter panic: %v", recovered)
		}
	}()
	return reporter.Report(ctx, event)
}

func flushSafely(ctx context.Context, reporter Reporter) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("reporter panic: %v", recovered)
		}
	}()
	return reporter.Flush(ctx)
}

func normalizeEvent(event Event) Event {
	event.Name = SanitizeString(strings.TrimSpace(event.Name))
	if event.Name == "" {
		event.Name = "zgi.event"
	}
	if event.Kind == "" {
		if event.Err != nil {
			event.Kind = EventKindError
		} else {
			event.Kind = EventKindEvent
		}
	}
	if event.Level == "" {
		if event.Err != nil || event.Kind == EventKindError {
			event.Level = LevelError
		} else {
			event.Level = LevelInfo
		}
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	event.Tags = sanitizeReporterTags(event.Tags)
	event.Attributes = SanitizeReporterAttributes(event.Attributes)
	if event.Err != nil {
		event.Err = sanitizedError{message: sanitizeReporterString(event.Err.Error())}
	}
	return event
}

type reporterHolder struct {
	reporter *ZGIReporter
}

var defaultReporter atomic.Pointer[reporterHolder]

func init() {
	defaultReporter.Store(&reporterHolder{reporter: NewZGIReporter()})
}

// SetDefaultReporter replaces the process-wide facade used by CaptureError and
// CaptureEvent. Passing nil restores the No-op behavior.
func SetDefaultReporter(reporter *ZGIReporter) {
	if reporter == nil {
		reporter = NewZGIReporter()
	}
	defaultReporter.Store(&reporterHolder{reporter: reporter})
}

// DefaultReporter returns the process-wide ZGIReporter.
func DefaultReporter() *ZGIReporter {
	holder := defaultReporter.Load()
	if holder == nil || holder.reporter == nil {
		return NewZGIReporter()
	}
	return holder.reporter
}

// CaptureError reports an error through the process-wide ZGIReporter. Delivery
// errors are deliberately isolated from business execution.
func CaptureError(ctx context.Context, name string, err error, options ...EventOption) {
	if err == nil {
		return
	}
	event := Event{Name: name, Kind: EventKindError, Level: LevelError, Err: err}
	for _, option := range options {
		if option != nil {
			option(&event)
		}
	}
	_ = DefaultReporter().Report(ctx, event)
}

// CaptureEvent reports a non-error event through the process-wide ZGIReporter.
func CaptureEvent(ctx context.Context, name string, options ...EventOption) {
	event := Event{Name: name, Kind: EventKindEvent, Level: LevelInfo}
	for _, option := range options {
		if option != nil {
			option(&event)
		}
	}
	_ = DefaultReporter().Report(ctx, event)
}

// NoopReporter is useful for tests and explicit custom compositions. A
// ZGIReporter with no adapters already has the same behavior.
type NoopReporter struct{}

func (NoopReporter) Name() string                        { return "noop" }
func (NoopReporter) Report(context.Context, Event) error { return nil }
func (NoopReporter) Flush(context.Context) error         { return nil }
