package observer

import "testing"

func TestMetricsAggregatesExecutionEvents(t *testing.T) {
	recorder := NewRecorder(20)

	recorder.Record("exec.code", "sbx_1", "code executed", map[string]any{
		"status":      "success",
		"exit_code":   0,
		"duration_ms": int64(12),
		"truncated":   true,
		"backend":     "preview-process",
	})
	recorder.Record("exec.command", "sbx_1", "command executed", map[string]any{
		"status":      "success",
		"exit_code":   124,
		"duration_ms": int64(20),
		"backend":     "preview-process",
	})
	recorder.Record("exec.code.failed", "sbx_1", "code failed", map[string]any{
		"status":     "failure",
		"error_type": "execution_canceled",
	})
	recorder.Record("exec.command.failed", "sbx_1", "command failed", map[string]any{
		"status":     "failure",
		"error_type": "execution_error",
	})

	metrics := recorder.Metrics(20)

	if metrics.EventCount != 4 {
		t.Fatalf("expected four events, got %d", metrics.EventCount)
	}
	if metrics.ExecutionSuccessCount != 2 {
		t.Fatalf("expected two success events, got %d", metrics.ExecutionSuccessCount)
	}
	if metrics.ExecutionFailureCount != 2 {
		t.Fatalf("expected two failure events, got %d", metrics.ExecutionFailureCount)
	}
	if metrics.TimeoutCount != 1 {
		t.Fatalf("expected one timeout, got %d", metrics.TimeoutCount)
	}
	if metrics.CancellationCount != 1 {
		t.Fatalf("expected one cancellation, got %d", metrics.CancellationCount)
	}
	if metrics.OutputTruncationCount != 1 {
		t.Fatalf("expected one truncation, got %d", metrics.OutputTruncationCount)
	}
	if metrics.BackendErrorCount != 1 {
		t.Fatalf("expected one backend error, got %d", metrics.BackendErrorCount)
	}
	if metrics.ExecutionDurationCount != 2 {
		t.Fatalf("expected two duration samples, got %d", metrics.ExecutionDurationCount)
	}
	if metrics.ExecutionDurationTotalMS != 32 {
		t.Fatalf("expected duration total 32ms, got %d", metrics.ExecutionDurationTotalMS)
	}
	if metrics.ExecutionDurationMaxMS != 20 {
		t.Fatalf("expected max duration 20ms, got %d", metrics.ExecutionDurationMaxMS)
	}
	if metrics.ExecutionCountByBackend["preview-process"] != 2 {
		t.Fatalf("expected backend count 2, got %d", metrics.ExecutionCountByBackend["preview-process"])
	}
	if metrics.ExecutionFailureCountByReason["execution_canceled"] != 1 {
		t.Fatalf("expected cancellation reason count 1, got %d", metrics.ExecutionFailureCountByReason["execution_canceled"])
	}
}

func TestRecorderFiltersByRequestID(t *testing.T) {
	recorder := NewRecorder(20)
	recorder.Record("exec.command", "sbx_request", "match", map[string]any{"request_id": "req_match"})
	recorder.Record("exec.command", "sbx_request", "miss", map[string]any{"request_id": "req_miss"})

	events := recorder.Query(Query{SandboxID: "sbx_request", RequestID: "req_match", Limit: 10})
	if len(events) != 1 {
		t.Fatalf("expected one request-filtered event, got %d", len(events))
	}
	if events[0].Message != "match" {
		t.Fatalf("expected matching event, got %q", events[0].Message)
	}
}

func TestRecorderFiltersByTypePrefix(t *testing.T) {
	recorder := NewRecorder(20)
	recorder.Record("sandbox.created", "sbx_history", "lifecycle", nil)
	recorder.Record("exec.code", "sbx_history", "code", nil)
	recorder.Record("exec.command.failed", "sbx_history", "command failed", nil)

	events := recorder.Query(Query{SandboxID: "sbx_history", TypePrefix: "exec.", Limit: 10})
	if len(events) != 2 {
		t.Fatalf("expected two execution events, got %d", len(events))
	}
	if events[0].Message != "command failed" || events[1].Message != "code" {
		t.Fatalf("expected newest execution events only, got %#v", events)
	}
}
