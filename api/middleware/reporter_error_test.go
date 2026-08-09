package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/observability"
)

type middlewareRecordingReporter struct {
	events []observability.Event
}

func (r *middlewareRecordingReporter) Name() string { return "test" }

func (r *middlewareRecordingReporter) Report(_ context.Context, event observability.Event) error {
	r.events = append(r.events, event)
	return nil
}

func (*middlewareRecordingReporter) Flush(context.Context) error { return nil }

func TestZGIErrorReporterUsesInjectedReporter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/failed", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/failed", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 1 {
		t.Fatalf("reported events = %d, want 1", len(adapter.events))
	}
	event := adapter.events[0]
	if event.Name != "http.request.failed" || event.Tags["http.route"] != "/failed" {
		t.Fatalf("reported event = %#v", event)
	}
	if event.Tags["error.category"] != "application" || event.Tags["error.source"] != "zgi" {
		t.Fatalf("error classification = %#v", event.Tags)
	}
}

func TestZGIErrorReporterDisabledKeepsRequestContextFastPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reporter := observability.NewZGIReporter()
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	var handlerContext context.Context
	engine.GET("/ok", func(c *gin.Context) {
		handlerContext = c.Request.Context()
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/ok", nil)
	originalContext := request.Context()
	engine.ServeHTTP(httptest.NewRecorder(), request)
	if handlerContext != originalContext {
		t.Fatal("disabled reporter should not wrap the request context")
	}
}

func TestZGIErrorReporterSkipsExpectedClientErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/missing", func(c *gin.Context) {
		c.Status(http.StatusNotFound)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 0 {
		t.Fatalf("reported events = %d, want 0", len(adapter.events))
	}
}

func TestZGIErrorReporterSkipsSameConcreteSemanticError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	previous := observability.DefaultReporter()
	observability.SetDefaultReporter(reporter)
	t.Cleanup(func() { observability.SetDefaultReporter(previous) })
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/provider", func(c *gin.Context) {
		providerErr := fmt.Errorf("provider stream failed")
		observability.CaptureError(c.Request.Context(), "llm.provider.stream_failed", providerErr)
		_ = c.Error(providerErr)
		c.Status(http.StatusBadGateway)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/provider", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 1 || adapter.events[0].Name != "llm.provider.stream_failed" {
		t.Fatalf("events = %#v, want exactly one semantic report", adapter.events)
	}
}

func TestZGIErrorReporterKeepsUnknownFallbackAfterUnrelatedWarning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	previous := observability.DefaultReporter()
	observability.SetDefaultReporter(reporter)
	t.Cleanup(func() { observability.SetDefaultReporter(previous) })
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/failed-after-warning", func(c *gin.Context) {
		observability.CaptureError(
			c.Request.Context(),
			"database.query.slow",
			fmt.Errorf("slow query detected"),
			observability.WithLevel(observability.LevelWarning),
		)
		c.Status(http.StatusInternalServerError)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/failed-after-warning", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 2 || adapter.events[0].Name != "database.query.slow" || adapter.events[1].Name != "http.request.failed" {
		t.Fatalf("events = %#v, want warning followed by request fallback", adapter.events)
	}
}

func TestZGIErrorReporterSkipsUnknownFallbackAfterErrorReport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	previous := observability.DefaultReporter()
	observability.SetDefaultReporter(reporter)
	t.Cleanup(func() { observability.SetDefaultReporter(previous) })
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/failed-after-database-error", func(c *gin.Context) {
		observability.CaptureError(c.Request.Context(), "database.operation.failed", fmt.Errorf("database unavailable"))
		c.Status(http.StatusInternalServerError)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/failed-after-database-error", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 1 || adapter.events[0].Name != "database.operation.failed" {
		t.Fatalf("events = %#v, want exactly one semantic database report", adapter.events)
	}
}

func TestZGIErrorReporterReportsConcreteStreamErrorAfterHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/stream", func(c *gin.Context) {
		c.Status(http.StatusOK)
		_ = c.Error(fmt.Errorf("billing settlement failed"))
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/stream", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 1 || adapter.events[0].Name != "http.stream.failed" {
		t.Fatalf("events = %#v, want one stream failure report", adapter.events)
	}
	if adapter.events[0].Tags["error.code"] != "stream_failed_after_headers" {
		t.Fatalf("classification = %#v", adapter.events[0].Tags)
	}
}

func TestZGIErrorReporterUsesExplicitStreamFailureHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/provider-stream", func(c *gin.Context) {
		c.Status(http.StatusOK)
		ginErr := c.Error(fmt.Errorf("upstream connection reset"))
		ginErr.SetMeta(observability.FailureReportHint{
			EventName: "llm.stream.failed",
			Classification: observability.ErrorClassification{
				Category:  observability.ErrorCategoryDependency,
				Source:    observability.ErrorSourceProvider,
				Code:      "upstream_stream_failed",
				Retryable: true,
			},
		})
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/provider-stream", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 1 || adapter.events[0].Name != "llm.stream.failed" || adapter.events[0].Tags["error.source"] != "provider" || adapter.events[0].Tags["error.code"] != "upstream_stream_failed" {
		t.Fatalf("events = %#v, want provider-classified stream failure", adapter.events)
	}
}

func TestZGIErrorReporterUsesExplicitProviderHintForNonStreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/provider-request", func(c *gin.Context) {
		ginErr := c.Error(fmt.Errorf("upstream request failed"))
		ginErr.SetMeta(observability.FailureReportHint{
			EventName: "llm.request.failed",
			Classification: observability.ErrorClassification{
				Category:  observability.ErrorCategoryDependency,
				Source:    observability.ErrorSourceProvider,
				Code:      "upstream_request_failed",
				Retryable: true,
			},
		})
		c.Status(http.StatusBadGateway)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/provider-request", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 1 || adapter.events[0].Name != "llm.request.failed" || adapter.events[0].Tags["error.source"] != "provider" || adapter.events[0].Tags["error.code"] != "upstream_request_failed" {
		t.Fatalf("events = %#v, want provider-classified request failure", adapter.events)
	}
}

func TestZGIErrorReporterPreservesExplicitProviderRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/provider-rate-limit", func(c *gin.Context) {
		ginErr := c.Error(fmt.Errorf("provider rate limited"))
		ginErr.SetMeta(observability.FailureReportHint{
			EventName: "llm.request.failed",
			Classification: observability.ErrorClassification{
				Category:  observability.ErrorCategoryDependency,
				Source:    observability.ErrorSourceProvider,
				Code:      "upstream_request_failed",
				Retryable: true,
			},
		})
		c.Status(http.StatusTooManyRequests)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/provider-rate-limit", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 1 || adapter.events[0].Level != observability.LevelError || adapter.events[0].Tags["error.source"] != "provider" {
		t.Fatalf("events = %#v, want provider rate limit report", adapter.events)
	}
}

func TestZGIErrorReporterSuppressesExplicitStreamRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/rejected-stream", func(c *gin.Context) {
		c.Status(http.StatusOK)
		ginErr := c.Error(fmt.Errorf("content policy violation"))
		ginErr.SetMeta(observability.FailureReportHint{Suppress: true})
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/rejected-stream", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 0 {
		t.Fatalf("events = %#v, want deterministic rejection suppressed", adapter.events)
	}
}

func TestZGIErrorReporterDoesNotDeduplicateConcreteFailureAgainstWarning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	previous := observability.DefaultReporter()
	observability.SetDefaultReporter(reporter)
	t.Cleanup(func() { observability.SetDefaultReporter(previous) })
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/warning-becomes-failure", func(c *gin.Context) {
		requestErr := fmt.Errorf("query deadline exceeded")
		observability.CaptureError(
			c.Request.Context(),
			"database.query.slow",
			requestErr,
			observability.WithLevel(observability.LevelWarning),
		)
		_ = c.Error(requestErr)
		c.Status(http.StatusInternalServerError)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/warning-becomes-failure", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 2 || adapter.events[0].Name != "database.query.slow" || adapter.events[1].Name != "http.request.failed" {
		t.Fatalf("events = %#v, want warning followed by concrete request failure", adapter.events)
	}
}

func TestZGIErrorReporterKeepsDistinctLaterRequestError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	previous := observability.DefaultReporter()
	observability.SetDefaultReporter(reporter)
	t.Cleanup(func() { observability.SetDefaultReporter(previous) })
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/billing", func(c *gin.Context) {
		observability.CaptureError(c.Request.Context(), "llm.provider.failed", context.DeadlineExceeded)
		_ = c.Error(fmt.Errorf("billing settlement failed: %w", context.DeadlineExceeded))
		c.Status(http.StatusInternalServerError)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/billing", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 2 || adapter.events[0].Name != "llm.provider.failed" || adapter.events[1].Name != "http.request.failed" {
		t.Fatalf("events = %#v, want provider error followed by distinct request error", adapter.events)
	}
}

func TestHTTPErrorClassificationIdentifiesUpstreamFailures(t *testing.T) {
	classification := httpErrorClassification(http.StatusBadGateway)
	if classification.Category != observability.ErrorCategoryDependency || classification.Source != observability.ErrorSourceInfrastructure || !classification.Retryable {
		t.Fatalf("classification = %#v", classification)
	}
}

func TestZGIErrorReporterSkipsExpectedCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := &middlewareRecordingReporter{}
	reporter := observability.NewZGIReporter(adapter)
	engine := gin.New()
	engine.Use(ZGIErrorReporter(reporter))
	engine.GET("/canceled", func(c *gin.Context) {
		_ = c.Error(context.Canceled)
		c.Status(http.StatusInternalServerError)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/canceled", nil)
	engine.ServeHTTP(response, request)

	if len(adapter.events) != 0 {
		t.Fatalf("reported events = %d, want 0", len(adapter.events))
	}
}
