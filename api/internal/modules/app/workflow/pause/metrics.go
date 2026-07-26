package pause

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	workflowRuntimeMeter, _     = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Counter("runtime_events_written")
	workflowResumeConflicts, _  = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Counter("resume_claim_conflicts")
	workflowLeaseTakeovers, _   = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Counter("lease_takeovers")
	workflowRuntimeOutboxLag, _ = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Float64Histogram("outbox_lag_seconds")
	workflowStaleAppends, _     = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Counter("stale_append_rejected")
	workflowOrphansFinalized, _ = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Counter("orphan_finalized")
	workflowActiveV1Runs, _     = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Histogram("active_v1_runs")
	workflowEventBatchSize, _   = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Histogram("event_batch_size")
	workflowEventBatchBytes, _  = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Histogram("event_batch_bytes")
	workflowSequenceAllocs, _   = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Counter("event_sequence_allocations")
	workflowEventNotifies, _    = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Counter("event_notify_count")
	workflowIdempotencyHits, _  = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Counter("event_idempotency_hits")
	workflowEventWALBytes, _    = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Int64Histogram("event_wal_estimate_bytes")
	workflowEventLockWait, _    = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Float64Histogram("event_run_lock_wait_ms")
	workflowSequencePerEvent, _ = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Float64Histogram("sequence_allocations_per_event")
	workflowNotifyPerEvent, _   = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime").Float64Histogram("notify_per_event")
)

func recordRuntimeEventWritten(ctx context.Context, category string) {
	workflowRuntimeMeter.Add(ctx, 1, metric.WithAttributes(attribute.String("category", category)))
}

func recordResumeClaimConflict(ctx context.Context, reason string) {
	workflowResumeConflicts.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func recordLeaseTakeover(ctx context.Context) {
	workflowLeaseTakeovers.Add(ctx, 1)
}

func recordStaleAppendRejected(ctx context.Context, reason string) {
	workflowStaleAppends.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func recordOrphanFinalized(ctx context.Context) {
	workflowOrphansFinalized.Add(ctx, 1)
}

func recordActiveV1Runs(ctx context.Context, count int64) {
	workflowActiveV1Runs.Record(ctx, count)
}

func recordOutboxLag(ctx context.Context, createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	workflowRuntimeOutboxLag.Record(ctx, max(0, time.Since(createdAt).Seconds()))
}

func recordEventBatch(ctx context.Context, requested, payloadBytes, inserted int, flushReason string) {
	attributes := metric.WithAttributes(attribute.Int("inserted", inserted), attribute.String("flush_reason", flushReason))
	workflowEventBatchSize.Record(ctx, int64(requested), attributes)
	workflowEventBatchBytes.Record(ctx, int64(payloadBytes), metric.WithAttributes(attribute.String("flush_reason", flushReason)))
	workflowEventWALBytes.Record(ctx, int64(payloadBytes+inserted*256), metric.WithAttributes(attribute.String("flush_reason", flushReason)))
	if inserted > 0 {
		workflowSequencePerEvent.Record(ctx, 1/float64(inserted), metric.WithAttributes(attribute.String("flush_reason", flushReason)))
	}
	if requested > inserted {
		workflowIdempotencyHits.Add(ctx, int64(requested-inserted))
	}
}

func recordEventRunLockWait(ctx context.Context, elapsed time.Duration) {
	workflowEventLockWait.Record(ctx, float64(elapsed.Microseconds())/1000)
}

func recordEventSequenceAllocation(ctx context.Context) {
	workflowSequenceAllocs.Add(ctx, 1)
}

func recordEventNotify(ctx context.Context, eventCount int, flushReason string) {
	workflowEventNotifies.Add(ctx, 1)
	if eventCount > 0 {
		workflowNotifyPerEvent.Record(ctx, 1/float64(eventCount), metric.WithAttributes(attribute.String("flush_reason", flushReason)))
	}
}
