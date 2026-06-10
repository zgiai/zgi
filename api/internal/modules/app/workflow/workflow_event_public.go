package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
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

type workflowRunEventHandler func(eventType string, data map[string]interface{}, stored *workflowpause.RunEventPayload) error

type workflowRunEventDispatcher struct {
	tenantID        string
	appID           string
	workflowRunID   string
	persistMessages bool
	onEvent         workflowRunEventHandler
	containers      map[string]workflowRunContainerState
	pending         []workflowRunEventRecord
}

type workflowRunContainerState struct {
	started bool
	rounds  map[int]bool
}

func newWorkflowRunEventDispatcher(tenantID, appID, workflowRunID string, persistMessages bool, onEvent workflowRunEventHandler) *workflowRunEventDispatcher {
	if workflowRunID == "" {
		return nil
	}
	return &workflowRunEventDispatcher{
		tenantID:        tenantID,
		appID:           appID,
		workflowRunID:   workflowRunID,
		persistMessages: persistMessages,
		onEvent:         onEvent,
		containers:      map[string]workflowRunContainerState{},
	}
}

func (d *workflowRunEventDispatcher) Dispatch(ctx context.Context, eventType string, data map[string]interface{}) {
	if d == nil || eventType == "" {
		return
	}
	record := workflowRunEventRecord{eventType: eventType, data: sanitizeWorkflowEventData(data)}
	if d.shouldBuffer(record) {
		d.pending = append(d.pending, record)
		return
	}
	d.dispatchNow(ctx, record)
	d.flushPending(ctx, false)
}

func (d *workflowRunEventDispatcher) Close(ctx context.Context) {
	if d == nil {
		return
	}
	d.flushPending(ctx, false)
	if len(d.pending) > 0 {
		logger.WarnContext(ctx, "dropping unmatched workflow container child events", "workflow_run_id", d.workflowRunID, "count", len(d.pending))
		d.pending = nil
	}
}

func (d *workflowRunEventDispatcher) shouldBuffer(record workflowRunEventRecord) bool {
	containerID, index, ok := workflowRunEventContainerContext(record)
	if !ok {
		return false
	}
	state, exists := d.containers[containerID]
	if !exists || !state.started {
		return true
	}
	if index == nil {
		return false
	}
	return !state.rounds[*index]
}

func (d *workflowRunEventDispatcher) flushPending(ctx context.Context, force bool) {
	if len(d.pending) == 0 {
		return
	}
	remaining := d.pending[:0]
	for _, record := range d.pending {
		if !force && d.shouldBuffer(record) {
			remaining = append(remaining, record)
			continue
		}
		d.dispatchNow(ctx, record)
	}
	d.pending = remaining
}

func (d *workflowRunEventDispatcher) dispatchNow(ctx context.Context, record workflowRunEventRecord) {
	if d == nil {
		return
	}
	d.observeContainerLifecycle(record)
	publicData := sanitizeWorkflowEventData(record.data)
	var stored *workflowpause.RunEventPayload
	if d.shouldPersist(record.eventType) {
		stored = appendWorkflowRunEventPayload(ctx, d.tenantID, d.appID, d.workflowRunID, record.eventType, publicData)
		if stored != nil {
			publicData = copyWorkflowEventDataWithSequence(publicData, stored.Sequence)
		}
	}
	if d.onEvent != nil {
		if err := d.onEvent(record.eventType, publicData, stored); err != nil {
			logger.WarnContext(ctx, "workflow run event handler failed", "workflow_run_id", d.workflowRunID, "event_type", record.eventType, err)
		}
	}
}

func (d *workflowRunEventDispatcher) shouldPersist(eventType string) bool {
	if eventType == workflowEventAnswerSnapshotReady {
		return false
	}
	if eventType == workflowEventMessage && !d.persistMessages {
		return false
	}
	return true
}

func (d *workflowRunEventDispatcher) observeContainerLifecycle(record workflowRunEventRecord) {
	id := workflowContainerLifecycleID(record)
	if id == "" {
		return
	}
	state := d.containers[id]
	if state.rounds == nil {
		state.rounds = map[int]bool{}
	}
	switch record.eventType {
	case "iteration_started", "loop_started":
		state.started = true
	case "iteration_next", "loop_next":
		state.started = true
		if index, ok := workflowLifecycleRoundIndex(record); ok {
			state.rounds[index] = true
		}
	}
	d.containers[id] = state
}

func workflowLifecycleRoundIndex(record workflowRunEventRecord) (int, bool) {
	switch record.eventType {
	case "iteration_next":
		if index, ok := workflowEventInt(record.data["iteration_index"]); ok {
			return index, true
		}
		return workflowEventInt(record.data["index"])
	case "loop_next":
		if index, ok := workflowEventInt(record.data["loop_index"]); ok {
			return index, true
		}
		index, ok := workflowEventInt(record.data["index"])
		if !ok {
			return 0, false
		}
		if index > 0 {
			return index - 1, true
		}
		return index, true
	default:
		return 0, false
	}
}

func workflowRunEventContainerContext(record workflowRunEventRecord) (string, *int, bool) {
	if record.eventType == "iteration_started" || record.eventType == "iteration_next" ||
		record.eventType == "iteration_completed" || record.eventType == "iteration_succeeded" ||
		record.eventType == "iteration_failed" || record.eventType == "loop_started" ||
		record.eventType == "loop_next" || record.eventType == "loop_completed" ||
		record.eventType == "loop_succeeded" || record.eventType == "loop_failed" {
		return "", nil, false
	}
	if id := workflowEventString(record.data["loop_id"]); id != "" {
		return "loop:" + id, workflowEventIndexPointer(record.data["loop_index"]), true
	}
	if id := workflowEventString(record.data["iteration_id"]); id != "" {
		return "iteration:" + id, workflowEventIndexPointer(record.data["iteration_index"]), true
	}
	return "", nil, false
}

func workflowEventIndexPointer(value interface{}) *int {
	index, ok := workflowEventInt(value)
	if !ok {
		return nil
	}
	return &index
}

func workflowContainerLifecycleID(record workflowRunEventRecord) string {
	switch record.eventType {
	case "iteration_started", "iteration_next", "iteration_completed", "iteration_succeeded", "iteration_failed":
		if id := workflowEventString(firstWorkflowValue(record.data["node_id"], record.data["id"])); id != "" {
			return "iteration:" + id
		}
	case "loop_started", "loop_next", "loop_completed", "loop_succeeded", "loop_failed":
		if id := workflowEventString(firstWorkflowValue(record.data["node_id"], record.data["id"])); id != "" {
			return "loop:" + id
		}
	}
	return ""
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
	appendWorkflowRunEventPayload(ctx, tenantID, appID, workflowRunID, eventType, data)
}

func appendWorkflowRunEventPayload(ctx context.Context, tenantID, appID, workflowRunID, eventType string, data map[string]interface{}) *workflowpause.RunEventPayload {
	if workflowRunID == "" || eventType == "" {
		return nil
	}
	service := workflowpause.NewService(database.GetDB())
	stored, err := service.AppendEventPayload(ctx, workflowpause.AppendEventParams{
		TenantID:      tenantID,
		AppID:         appID,
		WorkflowRunID: workflowRunID,
		EventType:     eventType,
		EventData:     sanitizeWorkflowEventData(data),
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to append workflow run event", "workflow_run_id", workflowRunID, "event_type", eventType, err)
		return nil
	}
	return stored
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

func copyWorkflowEventDataWithSequence(input map[string]interface{}, sequence int) map[string]interface{} {
	out := sanitizeWorkflowEventData(input)
	if sequence > 0 {
		out["sequence"] = sequence
	}
	return out
}

func firstWorkflowValue(values ...interface{}) interface{} {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func workflowEventString(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func workflowEventInt(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
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
