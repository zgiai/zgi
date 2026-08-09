package fxapp

import (
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/zgiai/zgi/api/internal/observability"
	"go.uber.org/zap"
)

const otelRuntimeErrorReportInterval = time.Minute

// otelRuntimeErrorHandler keeps exporter/queue failures visible without
// feeding them back into the OTel reporter and creating a recursive failure.
type otelRuntimeErrorHandler struct {
	log      *zap.Logger
	now      func() time.Time
	report   func(error, uint64)
	interval time.Duration

	mu         sync.Mutex
	lastReport time.Time
	suppressed uint64
}

func newOTelRuntimeErrorHandler(log *zap.Logger, sentryEnabled bool) *otelRuntimeErrorHandler {
	if log == nil {
		log = zap.NewNop()
	}
	handler := &otelRuntimeErrorHandler{
		log:      log,
		now:      time.Now,
		interval: otelRuntimeErrorReportInterval,
	}
	if sentryEnabled {
		handler.report = captureOTelRuntimeError
	}
	return handler
}

func (h *otelRuntimeErrorHandler) Handle(err error) {
	if h == nil || err == nil || observability.IsExpectedCancellation(err) {
		return
	}

	now := h.now()
	h.mu.Lock()
	if !h.lastReport.IsZero() && now.Sub(h.lastReport) < h.interval {
		h.suppressed++
		h.mu.Unlock()
		return
	}
	suppressed := h.suppressed
	h.suppressed = 0
	h.lastReport = now
	h.mu.Unlock()

	h.log.Warn("OpenTelemetry runtime error", zap.Error(err), zap.Uint64("suppressed_count", suppressed))
	if h.report != nil {
		h.report(err, suppressed)
	}
}

func captureOTelRuntimeError(err error, suppressed uint64) {
	hub := sentry.CurrentHub()
	if hub == nil {
		return
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelError)
		scope.SetTag("zgi.event", "observability.otel.runtime_failed")
		scope.SetTag("zgi.kind", string(observability.EventKindError))
		scope.SetTag("otel.signal", "traces")
		scope.SetTag("error.category", string(observability.ErrorCategoryDependency))
		scope.SetTag("error.source", string(observability.ErrorSourceInfrastructure))
		scope.SetTag("error.code", "otel_export_failed")
		scope.SetTag("error.retryable", "true")
		scope.SetFingerprint([]string{"zgi.event", "observability.otel.runtime_failed"})
		scope.SetExtra("error", observability.SanitizeError(err).Error())
		scope.SetExtra("suppressed_count", suppressed)
		hub.CaptureMessage("OpenTelemetry trace exporter failed")
	})
}
