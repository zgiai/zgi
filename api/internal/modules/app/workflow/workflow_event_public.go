package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	workflowpause "github.com/zgiai/ginext/internal/modules/app/workflow/pause"
	"github.com/zgiai/ginext/pkg/database"
	"github.com/zgiai/ginext/pkg/logger"
)

const (
	workflowStartReasonInitial    = "initial"
	workflowStartReasonResumption = "resumption"
	workflowRunEventAppendTimeout = 5 * time.Second
)

type workflowRunEventRecorder struct {
	tenantID      string
	appID         string
	workflowRunID string
	events        chan workflowRunEventRecord
}

type workflowRunEventRecord struct {
	eventType string
	data      map[string]interface{}
}

func newWorkflowRunEventRecorder(tenantID, appID, workflowRunID string) *workflowRunEventRecorder {
	if workflowRunID == "" {
		return nil
	}
	recorder := &workflowRunEventRecorder{
		tenantID:      tenantID,
		appID:         appID,
		workflowRunID: workflowRunID,
		events:        make(chan workflowRunEventRecord, 256),
	}
	go recorder.run()
	return recorder
}

func (r *workflowRunEventRecorder) Record(ctx context.Context, eventType string, data map[string]interface{}) {
	if r == nil || eventType == "" {
		return
	}
	record := workflowRunEventRecord{
		eventType: eventType,
		data:      sanitizeWorkflowEventData(data),
	}
	select {
	case r.events <- record:
	default:
		logger.WarnContext(ctx, "workflow run event recorder queue is full", "workflow_run_id", r.workflowRunID, "event_type", eventType)
	}
}

func (r *workflowRunEventRecorder) Close() {
	if r == nil {
		return
	}
	close(r.events)
}

func (r *workflowRunEventRecorder) run() {
	for record := range r.events {
		ctx, cancel := context.WithTimeout(context.Background(), workflowRunEventAppendTimeout)
		appendWorkflowRunEvent(ctx, r.tenantID, r.appID, r.workflowRunID, record.eventType, record.data)
		cancel()
	}
}

func appendWorkflowRunEvent(ctx context.Context, tenantID, appID, workflowRunID, eventType string, data map[string]interface{}) {
	if workflowRunID == "" || eventType == "" {
		return
	}
	service := workflowpause.NewService(database.GetDB())
	if err := service.AppendEvent(ctx, workflowpause.AppendEventParams{
		TenantID:      tenantID,
		AppID:         appID,
		WorkflowRunID: workflowRunID,
		EventType:     eventType,
		EventData:     sanitizeWorkflowEventData(data),
	}); err != nil {
		logger.WarnContext(ctx, "failed to append workflow run event", "workflow_run_id", workflowRunID, "event_type", eventType, err)
	}
}

func sendWorkflowSSEEvent(ctx context.Context, w http.ResponseWriter, eventType string, data map[string]interface{}) {
	if w == nil {
		logger.CriticalContext(ctx, "response writer is nil in send workflow sse event", "event_type", eventType)
		return
	}
	payload := map[string]interface{}{
		"event": eventType,
		"data":  sanitizeWorkflowEventData(data),
	}
	writeWorkflowSSEPayload(ctx, w, payload, eventType)
}

func sendWorkflowSSEStoredEvent(ctx context.Context, w http.ResponseWriter, event workflowpause.RunEventPayload) {
	payload := map[string]interface{}{
		"event":      event.Event,
		"data":       sanitizeWorkflowEventData(event.Data),
		"sequence":   event.Sequence,
		"created_at": event.CreatedAt,
	}
	writeWorkflowSSEPayload(ctx, w, payload, event.Event)
}

func sendWorkflowSSEPing(ctx context.Context, w http.ResponseWriter) {
	if w == nil {
		logger.CriticalContext(ctx, "response writer is nil in send workflow sse ping")
		return
	}
	fmt.Fprint(w, "event: ping\n\n")
	flushWorkflowSSE(ctx, w, "ping")
}

func sendWorkflowSSEKeepAlive(ctx context.Context, w http.ResponseWriter) {
	if w == nil {
		logger.CriticalContext(ctx, "response writer is nil in send workflow sse keepalive")
		return
	}
	fmt.Fprint(w, ": ping\n\n")
	flushWorkflowSSE(ctx, w, "keepalive")
}

func writeWorkflowSSEPayload(ctx context.Context, w http.ResponseWriter, payload map[string]interface{}, eventType string) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.ErrorContext(ctx, "failed to marshal workflow sse event", "event_type", eventType, err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
	flushWorkflowSSE(ctx, w, eventType)
}

func flushWorkflowSSE(ctx context.Context, w http.ResponseWriter, eventType string) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
		return
	}
	logger.CriticalContext(ctx, "response writer does not implement http flusher", "event_type", eventType)
}

func sanitizeWorkflowEventData(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		if isInternalWorkflowEventKey(key) {
			continue
		}
		if key == "elapsed_time" {
			output[key] = sanitizeWorkflowElapsedTime(value)
			continue
		}
		output[key] = sanitizeWorkflowEventValue(value)
	}
	return output
}

func sanitizeWorkflowEventValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return sanitizeWorkflowEventData(typed)
	case []interface{}:
		output := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			output = append(output, sanitizeWorkflowEventValue(item))
		}
		return output
	default:
		return value
	}
}

func sanitizeWorkflowElapsedTime(value interface{}) interface{} {
	elapsed, ok := workflowFloatValue(value)
	if !ok {
		return value
	}
	return elapsed
}

func workflowFloatValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func isInternalWorkflowEventKey(key string) bool {
	switch key {
	case "__timeout",
		"__action_id",
		"__rendered_content",
		"__approval_form",
		"__approval_form_id",
		"__approval_token",
		workflowInternalEdgeSourceHandle,
		"sys.workflow_resume_state",
		"sys.workflow_resume_pause_id",
		"workflow_resume_state",
		"workflow_resume_pause_id":
		return true
	default:
		return false
	}
}
