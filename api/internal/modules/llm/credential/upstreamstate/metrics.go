package upstreamstate

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	upstreamMeter           = otel.Meter("github.com/zgiai/zgi/api/llm/upstreamstate")
	upstreamChecks, _       = upstreamMeter.Int64Counter("llm_upstream_checks_total")
	upstreamBacklog, _      = upstreamMeter.Int64Gauge("llm_upstream_check_backlog")
	upstreamOldestDueAge, _ = upstreamMeter.Float64Gauge("llm_upstream_oldest_due_age_seconds")
	upstreamWouldGuard, _   = upstreamMeter.Int64Counter("llm_upstream_would_guard_total")
	upstreamGuard, _        = upstreamMeter.Int64Counter("llm_upstream_guard_total")
	upstreamHalfOpen, _     = upstreamMeter.Int64Counter("llm_upstream_half_open_total")
	upstreamNoCandidate, _  = upstreamMeter.Int64Counter("llm_upstream_no_candidate_total")
)

func recordCheckMetric(ctx context.Context, provider, outcome string) {
	upstreamChecks.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("outcome", outcome),
	))
}

func recordBacklogMetrics(ctx context.Context, stats DueStats) {
	upstreamBacklog.Record(ctx, stats.Backlog)
	if stats.OldestDueAge > 0 {
		upstreamOldestDueAge.Record(ctx, stats.OldestDueAge.Seconds())
	} else {
		upstreamOldestDueAge.Record(ctx, 0)
	}
}

func RecordWouldGuardMetric(ctx context.Context, provider string, reason GuardReason) {
	upstreamWouldGuard.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("reason", string(reason)),
	))
}

func RecordGuardMetric(ctx context.Context, provider string, reason GuardReason) {
	upstreamGuard.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("reason", string(reason)),
	))
}

func RecordHalfOpenMetric(ctx context.Context, provider, outcome string) {
	upstreamHalfOpen.Add(ctx, 1, metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("outcome", outcome),
	))
}

func RecordNoCandidateMetric(ctx context.Context, routeMix string) {
	upstreamNoCandidate.Add(ctx, 1, metric.WithAttributes(attribute.String("route_mix", routeMix)))
}
