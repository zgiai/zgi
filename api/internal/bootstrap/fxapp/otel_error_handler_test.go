package fxapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestOTelRuntimeErrorHandlerRateLimitsReports(t *testing.T) {
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	type report struct {
		err        string
		suppressed uint64
	}
	var reports []report
	handler := &otelRuntimeErrorHandler{
		log:      zap.NewNop(),
		now:      func() time.Time { return now },
		interval: time.Minute,
		report: func(err error, suppressed uint64) {
			reports = append(reports, report{err: err.Error(), suppressed: suppressed})
		},
	}

	handler.Handle(errors.New("export failed"))
	handler.Handle(errors.New("export failed again"))
	handler.Handle(errors.New("queue full"))
	handler.Handle(context.Canceled)
	now = now.Add(time.Minute)
	handler.Handle(errors.New("export still failing"))

	if len(reports) != 2 {
		t.Fatalf("reports = %#v, want two rate-limited reports", reports)
	}
	if reports[0].suppressed != 0 || reports[1].suppressed != 2 {
		t.Fatalf("suppressed counts = %d, %d; want 0, 2", reports[0].suppressed, reports[1].suppressed)
	}
}

func TestNormalizedSentryTraceSampleRate(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{input: -0.1, want: 0},
		{input: 0.025, want: 0.025},
		{input: 2, want: 1},
	}
	for _, test := range tests {
		if got := normalizedSentryTraceSampleRate(test.input); got != test.want {
			t.Fatalf("normalizedSentryTraceSampleRate(%v) = %v, want %v", test.input, got, test.want)
		}
	}
}
