package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const reporterTracerName = "zgi.reporter"

// OTelReporter records ZGI Reporter events on the active span, or creates a
// short standalone span when an event occurs outside an existing trace.
type OTelReporter struct{}

func NewOTelReporter() Reporter {
	return OTelReporter{}
}

func (OTelReporter) Name() string { return "otel" }

func (OTelReporter) Report(ctx context.Context, event Event) error {
	span := trace.SpanFromContext(ctx)
	standalone := !span.SpanContext().IsValid()
	if standalone {
		_, span = otel.Tracer(reporterTracerName).Start(ctx, event.Name)
		defer span.End()
	}

	attributes := reporterOTelAttributes(event)
	span.AddEvent(event.Name, trace.WithAttributes(attributes...))
	if event.Err != nil {
		span.RecordError(SanitizeError(event.Err), trace.WithAttributes(attributes...))
	}
	if event.Level == LevelError || event.Level == LevelFatal {
		description := event.Name
		if event.Err != nil {
			description = sanitizeReporterString(event.Err.Error())
		}
		span.SetStatus(codes.Error, description)
	}
	return nil
}

func (OTelReporter) Flush(context.Context) error { return nil }

func reporterOTelAttributes(event Event) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		attribute.String("zgi.event.name", event.Name),
		attribute.String("zgi.event.kind", string(event.Kind)),
		attribute.String("zgi.event.level", string(event.Level)),
	}
	for key, value := range event.Tags {
		attributes = append(attributes, attribute.String("zgi.tag."+key, value))
	}
	for key, value := range event.Attributes {
		attributes = append(attributes, reporterOTelAttribute("zgi.attribute."+key, value))
	}
	return SanitizeAttributes(attributes)
}

func reporterOTelAttribute(key string, value any) attribute.KeyValue {
	switch typed := value.(type) {
	case bool:
		return attribute.Bool(key, typed)
	case int:
		return attribute.Int(key, typed)
	case int64:
		return attribute.Int64(key, typed)
	case float64:
		return attribute.Float64(key, typed)
	case string:
		return attribute.String(key, typed)
	case []string:
		return attribute.StringSlice(key, typed)
	default:
		return attribute.String(key, fmt.Sprint(typed))
	}
}
