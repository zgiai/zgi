package workflow

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"gorm.io/gorm"
)

var (
	workflowRuntimeMetricsMeter       = otel.Meter("github.com/zgiai/zgi/api/workflow/runtime")
	workflowAnswerCheckpointWrites, _ = workflowRuntimeMetricsMeter.Int64Counter("answer_checkpoint_writes")
	workflowProjectionConflicts, _    = workflowRuntimeMetricsMeter.Int64Counter("message_projection_conflicts")
	workflowReplayEventCount, _       = workflowRuntimeMetricsMeter.Int64Histogram("replay_event_count")
	workflowReplayBytes, _            = workflowRuntimeMetricsMeter.Int64Histogram("replay_event_bytes")
	workflowSnapshotBuildLatency, _   = workflowRuntimeMetricsMeter.Float64Histogram("snapshot_build_latency_ms")
	workflowDBStatementsPerRun, _     = workflowRuntimeMetricsMeter.Int64Histogram("db_statements_per_run")
	workflowDBStatementsByKind, _     = workflowRuntimeMetricsMeter.Int64Histogram("db_statements_by_operation_per_run")
	workflowDBTransactionsPerRun, _   = workflowRuntimeMetricsMeter.Int64Histogram("db_transactions_per_run")
	workflowDBTransactionLatency, _   = workflowRuntimeMetricsMeter.Float64Histogram("db_transaction_duration_ms")
	workflowDBRowsWrittenPerRun, _    = workflowRuntimeMetricsMeter.Int64Histogram("db_rows_written_per_run")
	workflowLeaseRenewalFailures, _   = workflowRuntimeMetricsMeter.Int64Counter("lease_renewal_failure")
	workflowDurableAppendFailures, _  = workflowRuntimeMetricsMeter.Int64Counter("durable_append_failure")
	workflowV1ContinuationTraffic, _  = workflowRuntimeMetricsMeter.Int64Counter("v1_continuation_traffic")
	workflowRedisTailPublishes, _     = workflowRuntimeMetricsMeter.Int64Counter("redis_tail_publish")
	workflowRedisTailEnqueues, _      = workflowRuntimeMetricsMeter.Int64Counter("redis_tail_publish_enqueue")
	workflowRedisTailReads, _         = workflowRuntimeMetricsMeter.Int64Counter("redis_tail_read")
	workflowRedisTailLatency, _       = workflowRuntimeMetricsMeter.Float64Histogram("redis_tail_publish_latency_ms")
	workflowCommitToSSELatency, _     = workflowRuntimeMetricsMeter.Float64Histogram("commit_to_sse_latency_ms")
	workflowDBMetricsCallbacksOnce    sync.Once
)

type workflowDBStatementMetricsContextKey struct{}

type workflowDBStatementMetrics struct {
	runID        string
	count        atomic.Int64
	transactions atomic.Int64
	rowsWritten  atomic.Int64
	mu           sync.Mutex
	byKind       map[string]int64
}

func withWorkflowDBStatementMetrics(ctx context.Context, workflowRunID string) context.Context {
	if ctx == nil || workflowRunID == "" {
		return ctx
	}
	if existing, ok := ctx.Value(workflowDBStatementMetricsContextKey{}).(*workflowDBStatementMetrics); ok && existing != nil {
		return ctx
	}
	registerWorkflowDBMetricsCallbacks(database.GetDB())
	return context.WithValue(ctx, workflowDBStatementMetricsContextKey{}, &workflowDBStatementMetrics{
		runID:  workflowRunID,
		byKind: make(map[string]int64),
	})
}

func registerWorkflowDBMetricsCallbacks(db *gorm.DB) {
	if db == nil {
		return
	}
	workflowDBMetricsCallbacksOnce.Do(func() {
		callback := func(operation string) func(*gorm.DB) {
			return func(tx *gorm.DB) {
				if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
					return
				}
				if metrics, ok := tx.Statement.Context.Value(workflowDBStatementMetricsContextKey{}).(*workflowDBStatementMetrics); ok && metrics != nil {
					metrics.count.Add(1)
					if operation != "query" && tx.RowsAffected > 0 {
						metrics.rowsWritten.Add(tx.RowsAffected)
					}
					table := workflowDBTableCategory(tx.Statement.Table)
					metrics.mu.Lock()
					metrics.byKind[table+":"+operation]++
					metrics.mu.Unlock()
				}
			}
		}
		_ = db.Callback().Create().After("gorm:create").Register("workflow:count_create", callback("insert"))
		_ = db.Callback().Query().After("gorm:query").Register("workflow:count_query", callback("query"))
		_ = db.Callback().Update().After("gorm:update").Register("workflow:count_update", callback("update"))
		_ = db.Callback().Delete().After("gorm:delete").Register("workflow:count_delete", callback("delete"))
		_ = db.Callback().Raw().After("gorm:raw").Register("workflow:count_raw", callback("raw"))
		_ = db.Callback().Row().After("gorm:row").Register("workflow:count_row", callback("row"))
	})
}

func recordWorkflowDBStatements(ctx context.Context, lifecycle string) {
	if ctx == nil {
		return
	}
	metrics, ok := ctx.Value(workflowDBStatementMetricsContextKey{}).(*workflowDBStatementMetrics)
	if !ok || metrics == nil {
		return
	}
	workflowDBStatementsPerRun.Record(ctx, metrics.count.Load(), metric.WithAttributes(attribute.String("lifecycle", lifecycle)))
	workflowDBTransactionsPerRun.Record(ctx, metrics.transactions.Load(), metric.WithAttributes(attribute.String("lifecycle", lifecycle)))
	workflowDBRowsWrittenPerRun.Record(ctx, metrics.rowsWritten.Load(), metric.WithAttributes(attribute.String("lifecycle", lifecycle)))
	metrics.mu.Lock()
	counts := make(map[string]int64, len(metrics.byKind))
	for key, count := range metrics.byKind {
		counts[key] = count
	}
	metrics.mu.Unlock()
	for key, count := range counts {
		table, operation := splitWorkflowDBMetricKind(key)
		workflowDBStatementsByKind.Record(ctx, count, metric.WithAttributes(
			attribute.String("lifecycle", lifecycle),
			attribute.String("table", table),
			attribute.String("operation", operation),
		))
	}
}

func recordWorkflowDBTransaction(ctx context.Context) {
	if ctx == nil {
		return
	}
	if metrics, ok := ctx.Value(workflowDBStatementMetricsContextKey{}).(*workflowDBStatementMetrics); ok && metrics != nil {
		metrics.transactions.Add(1)
	}
}

func beginWorkflowDBTransaction(ctx context.Context, kind string) func() {
	recordWorkflowDBTransaction(ctx)
	startedAt := time.Now()
	return func() {
		workflowDBTransactionLatency.Record(ctx, float64(time.Since(startedAt).Microseconds())/1000,
			metric.WithAttributes(attribute.String("kind", kind)))
	}
}

func workflowDBTableCategory(table string) string {
	switch table {
	case "workflow_run_events":
		return "event"
	case "workflow_node_runtime_logs":
		return "node"
	case "agents_messages":
		return "message"
	case "workflow_run_logs":
		return "run"
	case "workflow_run_pauses", "workflow_run_pause_reasons":
		return "pause"
	case "workflow_runtime_outbox":
		return "outbox"
	case "":
		return "unknown"
	default:
		return "other"
	}
}

func splitWorkflowDBMetricKind(value string) (string, string) {
	for index := 0; index < len(value); index++ {
		if value[index] == ':' {
			return value[:index], value[index+1:]
		}
	}
	return value, "unknown"
}

func recordWorkflowAnswerCheckpoint(ctx context.Context) {
	workflowAnswerCheckpointWrites.Add(ctx, 1)
}

func recordWorkflowProjectionConflict(ctx context.Context, reason string) {
	workflowProjectionConflicts.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func recordWorkflowReplay(ctx context.Context, count, bytes int) {
	workflowReplayEventCount.Record(ctx, int64(count))
	workflowReplayBytes.Record(ctx, int64(bytes))
}

func recordWorkflowSnapshotLatency(ctx context.Context, milliseconds float64) {
	workflowSnapshotBuildLatency.Record(ctx, milliseconds)
}

func recordWorkflowLeaseRenewalFailure(ctx context.Context, reason string) {
	workflowLeaseRenewalFailures.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func recordWorkflowDurableAppendFailure(ctx context.Context, eventType string) {
	workflowDurableAppendFailures.Add(ctx, 1, metric.WithAttributes(attribute.String("event_type", eventType)))
}

func recordWorkflowV1Continuation(ctx context.Context, kind string) {
	workflowV1ContinuationTraffic.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", kind)))
}

func recordWorkflowRedisTailPublish(ctx context.Context, err error, elapsed time.Duration) {
	status := "success"
	if err != nil {
		status = "error"
	}
	workflowRedisTailPublishes.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
	workflowRedisTailLatency.Record(ctx, float64(elapsed.Microseconds())/1000, metric.WithAttributes(attribute.String("status", status)))
}

func recordWorkflowRedisTailPublishQueued(ctx context.Context, status string) {
	workflowRedisTailEnqueues.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
}

func recordWorkflowRedisTailRead(ctx context.Context, result string) {
	workflowRedisTailReads.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func recordWorkflowCommitToSSELatency(ctx context.Context, event *workflowpause.RunEventPayload) {
	if event == nil || event.RecordedAtMS <= 0 {
		return
	}
	elapsed := time.Since(time.UnixMilli(event.RecordedAtMS))
	if elapsed < 0 {
		return
	}
	workflowCommitToSSELatency.Record(ctx, float64(elapsed.Microseconds())/1000,
		metric.WithAttributes(attribute.String("event_type", event.Event)))
}
