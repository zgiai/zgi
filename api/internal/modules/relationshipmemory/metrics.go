package relationshipmemory

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	relationshipMemoryMeter, _        = otel.Meter("github.com/zgiai/zgi/api/relationshipmemory").Int64Counter("relationship_memory_operations_total")
	relationshipMemoryAccess, _       = otel.Meter("github.com/zgiai/zgi/api/relationshipmemory").Int64Counter("relationship_memory_access_decisions_total")
	relationshipMemoryContextItems, _ = otel.Meter("github.com/zgiai/zgi/api/relationshipmemory").Int64Histogram("relationship_memory_context_items")
	relationshipMemoryContextChars, _ = otel.Meter("github.com/zgiai/zgi/api/relationshipmemory").Int64Histogram("relationship_memory_context_chars")
	relationshipMemoryJobs, _         = otel.Meter("github.com/zgiai/zgi/api/relationshipmemory").Int64Counter("relationship_memory_extraction_jobs_total")
)

func RecordAccessDecision(ctx context.Context, consumerType string, identity InvocationIdentity, decision AccessDecision) {
	result := "denied"
	if decision.Read || decision.Mutate || decision.Tools || decision.AutoCapture {
		result = "allowed"
	}
	relationshipMemoryAccess.Add(ctx, 1, metric.WithAttributes(
		attribute.String("surface", string(identity.Surface)),
		attribute.String("conversation_type", string(identity.ConversationType)),
		attribute.String("consumer_type", consumerType),
		attribute.String("decision", result),
		attribute.String("reason", decision.Reason),
	))
}

func recordOperation(ctx context.Context, consumerType, operation string, err error) {
	relationshipMemoryMeter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("consumer_type", consumerType),
		attribute.String("operation", operation),
		attribute.String("result", memoryResult(err)),
	))
}

func recordRenderedContext(ctx context.Context, consumerType, rendered string, itemCount int) {
	attributes := metric.WithAttributes(attribute.String("consumer_type", consumerType))
	relationshipMemoryContextItems.Record(ctx, int64(itemCount), attributes)
	relationshipMemoryContextChars.Record(ctx, int64(utf8.RuneCountInString(rendered)), attributes)
}

// RecordContextInjection lets runtime adapters report the final post-budget
// relationship-memory block without exposing content as metric attributes.
func RecordContextInjection(ctx context.Context, consumerType, rendered string, itemCount int) {
	recordRenderedContext(ctx, consumerType, rendered, itemCount)
}

func recordExtractionJob(ctx context.Context, consumerType, result string) {
	relationshipMemoryJobs.Add(ctx, 1, metric.WithAttributes(
		attribute.String("consumer_type", consumerType),
		attribute.String("result", result),
	))
}

func memoryResult(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, ErrRevisionConflict):
		return "conflict"
	case errors.Is(err, ErrPolicyRejected), errors.Is(err, usersubject.ErrInvalidUser):
		return "rejected"
	case errors.Is(err, ErrAccessDenied):
		return "denied"
	case errors.Is(err, ErrCapacityExceeded):
		return "capacity_exceeded"
	case errors.Is(err, ErrUndoUnavailable):
		return "undo_unavailable"
	default:
		return "error"
	}
}
