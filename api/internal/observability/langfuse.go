package observability

import (
	"context"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/config"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	langfuseAttributePrefix = "langfuse."
	langfuseTraceTagsKey    = attribute.Key("langfuse.trace.tags")
)

type langfuseTraceAttributesContextKey struct{}

// LangfuseTraceAttributeSpanProcessor copies process-local Langfuse trace
// attributes from context to spans without using outgoing OTel baggage headers.
type LangfuseTraceAttributeSpanProcessor struct{}

func NewLangfuseTraceAttributeSpanProcessor() *LangfuseTraceAttributeSpanProcessor {
	return &LangfuseTraceAttributeSpanProcessor{}
}

func (p *LangfuseTraceAttributeSpanProcessor) OnStart(ctx context.Context, span sdktrace.ReadWriteSpan) {
	if span == nil || !LangfuseTraceAttributesEnabled() {
		return
	}
	attrs := SanitizeAttributes(LangfuseTraceAttributesFromContext(ctx))
	if len(attrs) == 0 {
		return
	}
	span.SetAttributes(attrs...)
}

func (p *LangfuseTraceAttributeSpanProcessor) OnEnd(sdktrace.ReadOnlySpan) {}

func (p *LangfuseTraceAttributeSpanProcessor) Shutdown(context.Context) error {
	return nil
}

func (p *LangfuseTraceAttributeSpanProcessor) ForceFlush(context.Context) error {
	return nil
}

func LangfuseTraceAttributesEnabled() bool {
	cfg := otelConfig()
	return cfg.Enabled && cfg.LLMLangfuseAttributes
}

func WithLangfuseTraceAttributes(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if !LangfuseTraceAttributesEnabled() {
		return ctx
	}

	filtered := SanitizeAttributes(filterLangfuseTraceAttributes(attrs))
	if len(filtered) == 0 {
		return ctx
	}

	merged := SanitizeAttributes(mergeLangfuseTraceAttributes(LangfuseTraceAttributesFromContext(ctx), filtered))
	trace.SpanFromContext(ctx).SetAttributes(merged...)
	return context.WithValue(ctx, langfuseTraceAttributesContextKey{}, merged)
}

func LangfuseTraceAttributesFromContext(ctx context.Context) []attribute.KeyValue {
	if ctx == nil {
		return nil
	}
	attrs, ok := ctx.Value(langfuseTraceAttributesContextKey{}).([]attribute.KeyValue)
	if !ok || len(attrs) == 0 {
		return nil
	}
	return append([]attribute.KeyValue(nil), attrs...)
}

func LangfuseRuntimeAttributes() []attribute.KeyValue {
	cfg := config.GlobalConfig
	if cfg == nil {
		return nil
	}

	attrs := make([]attribute.KeyValue, 0, 3)
	if env := strings.TrimSpace(cfg.Server.Environment); env != "" {
		attrs = append(attrs, attribute.String("langfuse.environment", SanitizeString(env)))
	}
	if release := strings.TrimSpace(cfg.Sentry.Release); release != "" {
		attrs = append(attrs,
			attribute.String("langfuse.release", SanitizeString(release)),
			attribute.String("langfuse.version", SanitizeString(release)),
		)
	}
	return attrs
}

func filterLangfuseTraceAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		if !strings.HasPrefix(string(attr.Key), langfuseAttributePrefix) {
			continue
		}
		out = append(out, sanitizeAttribute(attr))
	}
	return out
}

func mergeLangfuseTraceAttributes(existing []attribute.KeyValue, incoming []attribute.KeyValue) []attribute.KeyValue {
	byKey := make(map[attribute.Key]attribute.KeyValue, len(existing)+len(incoming))
	tags := make([]string, 0)

	for _, attr := range existing {
		if attr.Key == langfuseTraceTagsKey {
			tags = appendStringSet(tags, attr.Value.AsStringSlice())
			continue
		}
		byKey[attr.Key] = attr
	}

	for _, attr := range incoming {
		if attr.Key == langfuseTraceTagsKey {
			tags = appendStringSet(tags, attr.Value.AsStringSlice())
			continue
		}
		byKey[attr.Key] = attr
	}

	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)

	out := make([]attribute.KeyValue, 0, len(byKey)+1)
	for _, key := range keys {
		out = append(out, byKey[attribute.Key(key)])
	}
	if len(tags) > 0 {
		sort.Strings(tags)
		out = append(out, attribute.StringSlice(string(langfuseTraceTagsKey), tags))
	}
	return out
}

func appendStringSet(base []string, values []string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range base {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
