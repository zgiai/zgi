package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/observability"
)

// ZGIErrorReporter reports unexpected HTTP failures through the provider-neutral
// ZGI Reporter facade. Common client and business errors remain excluded.
func ZGIErrorReporter(reporter *observability.ZGIReporter) gin.HandlerFunc {
	if reporter == nil || !reporter.Enabled() {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return func(c *gin.Context) {
		requestCtx, reportState := observability.WithReportState(c.Request.Context())
		c.Request = c.Request.WithContext(requestCtx)
		c.Next()

		statusCode := c.Writer.Status()
		reportErr := errors.New(http.StatusText(statusCode))
		hasConcreteError := false
		var reportHint observability.FailureReportHint
		if len(c.Errors) > 0 && c.Errors.Last().Err != nil {
			lastError := c.Errors.Last()
			reportErr = lastError.Err
			hasConcreteError = true
			reportHint, _ = lastError.Meta.(observability.FailureReportHint)
			if reportHint.Suppress {
				return
			}
		}
		if hasConcreteError {
			if reportState.ReportedError(reportErr) {
				return
			}
		} else if reportState.Reported() {
			// Without a concrete final error, an earlier error-level semantic
			// report is the best available identity for this failure. Warning
			// diagnostics deliberately do not enter this branch.
			return
		}
		if statusCode < http.StatusBadRequest && !hasConcreteError {
			return
		}
		providerRateLimit := false
		switch statusCode {
		case http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusForbidden,
			http.StatusNotFound,
			http.StatusConflict,
			http.StatusUnprocessableEntity:
			return
		case http.StatusTooManyRequests:
			// A 429 without domain metadata is an expected tenant/client limit.
			// Explicit provider hints from Gateway protocols remain actionable.
			if reportHint.Classification.Source != observability.ErrorSourceProvider {
				return
			}
			providerRateLimit = true
		}
		if observability.IsExpectedCancellation(reportErr) {
			return
		}

		level := observability.LevelWarning
		eventName := "http.request.failed"
		classification := httpErrorClassification(statusCode)
		if statusCode >= http.StatusInternalServerError {
			level = observability.LevelError
		}
		if providerRateLimit {
			level = observability.LevelError
		}
		if statusCode < http.StatusBadRequest {
			// Streaming protocols may already have flushed a successful HTTP
			// status before an asynchronous provider or billing error arrives.
			level = observability.LevelError
			eventName = "http.stream.failed"
			classification = observability.ErrorClassification{
				Category:  observability.ErrorCategoryDependency,
				Source:    observability.ErrorSourceInfrastructure,
				Code:      "stream_failed_after_headers",
				Retryable: true,
			}
		}
		if reportHint.EventName != "" {
			eventName = reportHint.EventName
		}
		if reportHint.Classification.Category != "" || reportHint.Classification.Source != "" || reportHint.Classification.Code != "" {
			classification = reportHint.Classification
		}
		event := observability.Event{
			Name:  eventName,
			Kind:  observability.EventKindError,
			Level: level,
			Err:   reportErr,
			Tags: map[string]string{
				"http.method":      c.Request.Method,
				"http.route":       c.FullPath(),
				"http.status_code": strconv.Itoa(statusCode),
			},
			Attributes: map[string]any{
				"request_id": c.GetString("request_id"),
				"user_id":    c.GetString("user_id"),
				"tenant_id":  c.GetString("tenant_id"),
			},
		}
		observability.WithErrorClassification(classification)(&event)
		_ = reporter.Report(c.Request.Context(), event)
	}
}

func httpErrorClassification(statusCode int) observability.ErrorClassification {
	classification := observability.ErrorClassification{
		Category: observability.ErrorCategoryApplication,
		Source:   observability.ErrorSourceZGI,
		Code:     "http_" + strconv.Itoa(statusCode),
	}
	if statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout {
		classification.Category = observability.ErrorCategoryDependency
		classification.Source = observability.ErrorSourceInfrastructure
		classification.Retryable = true
	}
	return classification
}
