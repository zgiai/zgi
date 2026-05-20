package sentry

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
)

// CaptureError captures an error with optional context
func CaptureError(err error, tags map[string]string, extras map[string]interface{}) {
	if err == nil {
		return
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		// Add tags
		for key, value := range tags {
			scope.SetTag(key, value)
		}

		// Add extras
		for key, value := range extras {
			scope.SetExtra(key, value)
		}

		sentry.CaptureException(err)
	})
}

// CaptureErrorWithContext captures an error with context
func CaptureErrorWithContext(ctx context.Context, err error, tags map[string]string, extras map[string]interface{}) {
	if err == nil {
		return
	}

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}

	hub.WithScope(func(scope *sentry.Scope) {
		// Add tags
		for key, value := range tags {
			scope.SetTag(key, value)
		}

		// Add extras
		for key, value := range extras {
			scope.SetExtra(key, value)
		}

		hub.CaptureException(err)
	})
}

// CaptureMessage captures a message with level
func CaptureMessage(message string, level sentry.Level, tags map[string]string) {
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(level)

		for key, value := range tags {
			scope.SetTag(key, value)
		}

		sentry.CaptureMessage(message)
	})
}

// CaptureLLMError captures LLM-related errors
func CaptureLLMError(err error, provider, model, tenantID string, extras map[string]interface{}) {
	if err == nil {
		return
	}

	tags := map[string]string{
		"error_type": "llm_error",
		"provider":   provider,
		"model":      model,
		"tenant_id":  tenantID,
	}

	CaptureError(err, tags, extras)
}

// CaptureWorkflowError captures workflow-related errors
func CaptureWorkflowError(err error, workflowID, nodeID, nodeType string, extras map[string]interface{}) {
	if err == nil {
		return
	}

	tags := map[string]string{
		"error_type":  "workflow_error",
		"workflow_id": workflowID,
		"node_id":     nodeID,
		"node_type":   nodeType,
	}

	CaptureError(err, tags, extras)
}

// CaptureHTTPError captures HTTP-related errors
func CaptureHTTPError(err error, method, path string, statusCode int, extras map[string]interface{}) {
	if err == nil {
		return
	}

	tags := map[string]string{
		"error_type":  "http_error",
		"http_method": method,
		"http_path":   path,
		"status_code": fmt.Sprintf("%d", statusCode),
	}

	CaptureError(err, tags, extras)
}

// CaptureDBError captures database-related errors
func CaptureDBError(err error, operation, table string, extras map[string]interface{}) {
	if err == nil {
		return
	}

	tags := map[string]string{
		"error_type":   "database_error",
		"db_operation": operation,
		"db_table":     table,
	}

	CaptureError(err, tags, extras)
}

// CaptureEmbeddingError captures embedding-related errors
func CaptureEmbeddingError(err error, provider, model, datasetID, documentID string, extras map[string]interface{}) {
	if err == nil {
		return
	}

	tags := map[string]string{
		"error_type":  "embedding_error",
		"provider":    provider,
		"model":       model,
		"dataset_id":  datasetID,
		"document_id": documentID,
	}

	CaptureError(err, tags, extras)
}
