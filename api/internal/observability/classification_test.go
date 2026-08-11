package observability

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestWithErrorClassificationAddsStableTags(t *testing.T) {
	event := Event{}
	WithErrorClassification(ErrorClassification{
		Category:  ErrorCategoryDependency,
		Source:    ErrorSourceProvider,
		Code:      "upstream_timeout",
		Retryable: true,
	})(&event)

	want := map[string]string{
		"error.category":  "dependency",
		"error.source":    "provider",
		"error.code":      "upstream_timeout",
		"error.retryable": "true",
	}
	for key, value := range want {
		if event.Tags[key] != value {
			t.Fatalf("tag %q = %q, want %q", key, event.Tags[key], value)
		}
	}
}

func TestReportStateFollowsChildContexts(t *testing.T) {
	ctx, state := WithReportState(context.Background())
	child := context.WithValue(ctx, struct{}{}, "value")
	markErrorReported(child, context.DeadlineExceeded, LevelError)
	if !state.Reported() {
		t.Fatal("request report state was not marked")
	}
}

func TestReportStateDistinguishesLaterError(t *testing.T) {
	ctx, state := WithReportState(context.Background())
	markErrorReported(ctx, context.DeadlineExceeded, LevelError)
	if !state.ReportedError(context.DeadlineExceeded) {
		t.Fatal("reported error was not matched")
	}
	if state.ReportedError(errors.New("billing settlement failed")) {
		t.Fatal("distinct later error was incorrectly deduplicated")
	}
}

func TestReportStateDoesNotConflateFailuresSharingDeadlineCause(t *testing.T) {
	ctx, state := WithReportState(context.Background())
	markErrorReported(ctx, context.DeadlineExceeded, LevelError)
	billingTimeout := fmt.Errorf("billing settlement failed: %w", context.DeadlineExceeded)
	if state.ReportedError(billingTimeout) {
		t.Fatal("billing timeout was incorrectly deduplicated against provider timeout")
	}
}

func TestReportStateMatchesAWrapperOfTheSameConcreteError(t *testing.T) {
	ctx, state := WithReportState(context.Background())
	providerError := errors.New("provider unavailable")
	markErrorReported(ctx, providerError, LevelError)
	if !state.ReportedError(fmt.Errorf("request failed: %w", providerError)) {
		t.Fatal("wrapper of the same concrete provider error was not deduplicated")
	}
}

func TestWarningDoesNotMarkRequestAsErrorReported(t *testing.T) {
	ctx, state := WithReportState(context.Background())
	CaptureError(ctx, "database.query.slow", errors.New("slow query"), WithLevel(LevelWarning))
	if state.Reported() {
		t.Fatal("warning diagnostic should not suppress a later request failure")
	}
}

func TestCanceledErrorDoesNotMarkReportState(t *testing.T) {
	ctx, state := WithReportState(context.Background())
	CaptureError(ctx, "llm.provider.stream_failed", context.Canceled)
	if state.Reported() {
		t.Fatal("expected cancellation should not suppress a later actionable fallback error")
	}
}
