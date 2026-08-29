package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

const (
	workflowStartReasonInitial    = "initial"
	workflowStartReasonResumption = "resumption"
	workflowRunEventAppendTimeout = 5 * time.Second
)

type workflowRunEventRecord struct {
	eventType string
	data      map[string]interface{}
}

type workflowRunEventHandler func(eventType string, data map[string]interface{}, stored *workflowpause.RunEventPayload) error

type workflowRunEventDispatcher struct {
	mu              sync.Mutex
	tenantID        string
	appID           string
	workflowRunID   string
	persistMessages bool
	onEvent         workflowRunEventHandler
	containers      map[string]workflowRunContainerState
	pending         []workflowRunEventRecord
	batcher         *RuntimeEventBatcher
	closed          bool
}

type workflowRunContainerState struct {
	started bool
	rounds  map[int]bool
}

func newWorkflowRunEventDispatcher(tenantID, appID, workflowRunID string, persistMessages bool, onEvent workflowRunEventHandler) *workflowRunEventDispatcher {
	if workflowRunID == "" {
		return nil
	}
	dispatcher := &workflowRunEventDispatcher{
		tenantID:        tenantID,
		appID:           appID,
		workflowRunID:   workflowRunID,
		persistMessages: persistMessages,
		onEvent:         onEvent,
		containers:      map[string]workflowRunContainerState{},
	}
	dispatcher.batcher = newRuntimeEventBatcher(dispatcher.dispatchBatchNow)
	return dispatcher
}

func (d *workflowRunEventDispatcher) Dispatch(ctx context.Context, eventType string, data map[string]interface{}) error {
	if d == nil || eventType == "" {
		return nil
	}
	storedSequence := firstWorkflowValue(data["__stored_sequence"])
	storedEventID := firstWorkflowValue(data["__stored_event_id"])
	storedEventPayload := firstWorkflowValue(data["__stored_event_payload"])
	data = normalizeWorkflowExecutionEventData(eventType, data)
	if storedSequence != nil {
		data["__stored_sequence"] = storedSequence
	}
	if storedEventID != nil {
		data["__stored_event_id"] = storedEventID
	}
	if storedEventPayload != nil {
		data["__stored_event_payload"] = storedEventPayload
	}
	if workflowEventString(data["workflow_run_id"]) == "" {
		data["workflow_run_id"] = d.workflowRunID
	}
	if workflowEventString(data["correlation_id"]) == "" {
		data["correlation_id"] = d.workflowRunID
	}
	record := workflowRunEventRecord{eventType: eventType, data: data}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return errWorkflowRuntimeEventBatcherClosed
	}
	if d.shouldBuffer(record) {
		// Do not allocate a durable sequence before the missing container/round
		// lifecycle event. Publishing it later would make live order differ from
		// replay order. Once the parent arrives, flushPending persists and emits
		// the child in the same sequence order.
		d.pending = append(d.pending, record)
		d.mu.Unlock()
		return nil
	}
	records := make([]workflowRunEventRecord, 0, 1+len(d.pending))
	if isWorkflowRunTerminalEvent(record.eventType) {
		// No buffered execution event may become visible after a terminal event.
		// Buffered events were already persisted at arrival time, so flushing here
		// preserves both live ordering and replay sequence.
		records = append(records, d.takeReadyPendingLocked(true)...)
	}
	if isWorkflowContainerTerminalEvent(record.eventType) {
		// A container has no "next" event after its final round. Confirm the
		// authoritative completed range before publishing the terminal event so
		// buffered child events keep their real execution order.
		d.observeContainerLifecycle(record)
		records = append(records, d.takeReadyPendingLocked(false)...)
	}
	records = append(records, record)
	if !isWorkflowContainerTerminalEvent(record.eventType) {
		d.observeContainerLifecycle(record)
		records = append(records, d.takeReadyPendingLocked(false)...)
	}
	barrier := isWorkflowRuntimeEventBarrier(record.eventType)
	done, err := d.enqueueBatchLocked(ctx, records, barrier)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	return <-done
}

func isWorkflowRunTerminalEvent(eventType string) bool {
	switch eventType {
	case workflowpause.EventWorkflowFinished, "workflow_failed", "workflow_stopped", "workflow_succeeded", "workflow_completed":
		return true
	default:
		return false
	}
}

func (d *workflowRunEventDispatcher) Close(ctx context.Context) error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	records := d.takeReadyPendingLocked(true)
	done, err := d.enqueueBatchLocked(ctx, records, true)
	batcher := d.batcher
	d.mu.Unlock()
	if err == nil {
		err = <-done
	}
	if batcher != nil {
		batcher.close()
	}
	return err
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

func (d *workflowRunEventDispatcher) takeReadyPendingLocked(force bool) []workflowRunEventRecord {
	if len(d.pending) == 0 {
		return nil
	}
	remaining := make([]workflowRunEventRecord, 0, len(d.pending))
	ready := make([]workflowRunEventRecord, 0, len(d.pending))
	for _, record := range d.pending {
		if !force && d.shouldBuffer(record) {
			remaining = append(remaining, record)
			continue
		}
		ready = append(ready, record)
	}
	d.pending = remaining
	return ready
}

func (d *workflowRunEventDispatcher) enqueueBatchLocked(ctx context.Context, records []workflowRunEventRecord, barrier bool) (<-chan error, error) {
	if len(records) == 0 {
		done := make(chan error, 1)
		done <- nil
		return done, nil
	}
	if d.batcher == nil {
		done := make(chan error, 1)
		done <- d.dispatchBatchNow(ctx, records)
		return done, nil
	}
	return d.batcher.enqueue(ctx, records, barrier)
}

func (d *workflowRunEventDispatcher) dispatchBatchNow(ctx context.Context, records []workflowRunEventRecord) error {
	if d == nil {
		return nil
	}
	if len(records) == 0 {
		return nil
	}
	type dispatchResult struct {
		record     workflowRunEventRecord
		publicData map[string]interface{}
		stored     *workflowpause.RunEventPayload
	}
	results := make([]dispatchResult, len(records))
	persistRecords := make([]workflowRunEventRecord, 0, len(records))
	persistResultIndexes := make([]int, 0, len(records))
	for index, record := range records {
		publicData := sanitizeWorkflowEventData(record.data)
		results[index] = dispatchResult{record: record, publicData: publicData}
		if storedSequence, ok := workflowEventInt(record.data["__stored_sequence"]); ok && storedSequence > 0 {
			delete(publicData, "__stored_sequence")
			delete(publicData, "__stored_event_id")
			delete(publicData, "__stored_event_payload")
			if storedPayload, exists := record.data["__stored_event_payload"].(*workflowpause.RunEventPayload); exists && storedPayload != nil {
				payload := *storedPayload
				payload.Data = publicData
				results[index].stored = &payload
			} else {
				now := time.Now()
				results[index].stored = &workflowpause.RunEventPayload{
					EventID: workflowEventString(record.data["__stored_event_id"]), Sequence: storedSequence,
					Event: record.eventType, Category: workflowEventCategory(record.eventType), SchemaVersion: 2, PayloadVersion: 1,
					Data: publicData, CreatedAt: now.Unix(), OccurredAtMS: now.UnixMilli(), RecordedAtMS: now.UnixMilli(),
				}
			}
			continue
		}
		if d.shouldPersist(record.eventType) {
			persistRecords = append(persistRecords, workflowRunEventRecord{eventType: record.eventType, data: publicData})
			persistResultIndexes = append(persistResultIndexes, index)
		}
	}
	if len(persistRecords) > 0 {
		storedEvents, err := appendWorkflowRunEventBatchPayloadResult(ctx, d.tenantID, d.appID, d.workflowRunID, persistRecords)
		if err != nil {
			return fmt.Errorf("persist workflow event batch: %w", err)
		}
		for index, stored := range storedEvents {
			resultIndex := persistResultIndexes[index]
			results[resultIndex].stored = stored
			if stored != nil {
				results[resultIndex].publicData = copyWorkflowEventDataWithSequence(results[resultIndex].publicData, stored.Sequence)
			}
		}
	}
	committed := make([]*workflowpause.RunEventPayload, 0, len(results))
	for _, result := range results {
		if result.stored != nil && result.stored.Sequence > 0 {
			committed = append(committed, result.stored)
		}
	}
	publishWorkflowCommittedTail(ctx, d.workflowRunID, committed...)
	if d.onEvent == nil {
		return nil
	}
	for _, result := range results {
		recordWorkflowCommitToSSELatency(ctx, result.stored)
		if err := d.onEvent(result.record.eventType, result.publicData, result.stored); err != nil {
			logger.WarnContext(ctx, "workflow run event handler failed", "workflow_run_id", d.workflowRunID, "event_type", result.record.eventType, err)
		}
	}
	return nil
}

func isWorkflowRuntimeEventBarrier(eventType string) bool {
	if isWorkflowRunTerminalEvent(eventType) || isWorkflowContainerTerminalEvent(eventType) {
		return true
	}
	switch eventType {
	case workflowpause.EventWorkflowPaused, workflowpause.EventWorkflowResumed,
		workflowpause.EventApprovalRequested, workflowpause.EventApprovalResultFilled, workflowpause.EventApprovalExpired,
		workflowpause.EventQuestionAnswerRequested, workflowpause.EventQuestionAnswerSubmitted,
		workflowpause.EventError, workflowEventMessageEnd:
		return true
	default:
		return false
	}
}

func workflowEventCategory(eventType string) string {
	switch eventType {
	case workflowpause.EventWorkflowStarted, workflowpause.EventWorkflowPaused, workflowpause.EventWorkflowResumed, workflowpause.EventWorkflowFinished,
		"workflow_failed", "workflow_stopped", "workflow_succeeded", "workflow_completed",
		workflowpause.EventError, workflowEventMessageEnd:
		return workflowpause.EventCategoryControl
	case workflowpause.EventApprovalRequested, workflowpause.EventApprovalResultFilled, workflowpause.EventApprovalExpired,
		workflowpause.EventQuestionAnswerRequested, workflowpause.EventQuestionAnswerSubmitted:
		return workflowpause.EventCategoryInteraction
	case workflowEventMessage:
		return workflowpause.EventCategoryAnswerCheckpoint
	default:
		return workflowpause.EventCategoryExecution
	}
}

func (d *workflowRunEventDispatcher) shouldPersist(eventType string) bool {
	// A dispatcher without a run identity is used by pure ordering tests and
	// transient compatibility adapters. Durable V2 dispatchers always carry a
	// workflow run ID.
	if strings.TrimSpace(d.workflowRunID) == "" {
		return false
	}
	if eventType == workflowEventAnswerSnapshotReady {
		return false
	}
	switch eventType {
	case "text_chunk", "text_replace", "heartbeat", "ping", "keepalive", "workflow_events_open":
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
	case "iteration_completed", "iteration_succeeded", "iteration_failed",
		"loop_completed", "loop_succeeded", "loop_failed":
		state.started = true
		if steps, ok := workflowEventInt(record.data["steps"]); ok && steps > 0 {
			for index := range steps {
				state.rounds[index] = true
			}
		}
	}
	d.containers[id] = state
}

func isWorkflowContainerTerminalEvent(eventType string) bool {
	switch eventType {
	case "iteration_completed", "iteration_succeeded", "iteration_failed",
		"loop_completed", "loop_succeeded", "loop_failed":
		return true
	default:
		return false
	}
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

func appendWorkflowRunEvent(ctx context.Context, tenantID, appID, workflowRunID, eventType string, data map[string]interface{}) {
	appendWorkflowRunEventPayload(ctx, tenantID, appID, workflowRunID, eventType, data)
}

func appendWorkflowRunEventPayload(ctx context.Context, tenantID, appID, workflowRunID, eventType string, data map[string]interface{}) *workflowpause.RunEventPayload {
	stored, err := appendWorkflowRunEventPayloadResult(ctx, tenantID, appID, workflowRunID, eventType, data)
	if err != nil {
		logger.WarnContext(ctx, "failed to append workflow run event", "workflow_run_id", workflowRunID, "event_type", eventType, err)
		return nil
	}
	return stored
}

func appendWorkflowRunEventPayloadResult(ctx context.Context, tenantID, appID, workflowRunID, eventType string, data map[string]interface{}) (*workflowpause.RunEventPayload, error) {
	stored, err := appendWorkflowRunEventBatchPayloadResult(ctx, tenantID, appID, workflowRunID, []workflowRunEventRecord{{
		eventType: eventType,
		data:      data,
	}})
	if err != nil || len(stored) == 0 {
		return nil, err
	}
	return stored[0], nil
}

func appendWorkflowRunEventBatchPayloadResult(ctx context.Context, tenantID, appID, workflowRunID string, records []workflowRunEventRecord) ([]*workflowpause.RunEventPayload, error) {
	if workflowRunID == "" || len(records) == 0 {
		return nil, nil
	}
	request := workflowpause.AppendEventBatchRequest{
		TenantID: tenantID, AppID: appID, WorkflowRunID: workflowRunID,
		FlushReason: "execution_microbatch",
		Events:      make([]workflowpause.EventDraft, 0, len(records)),
	}
	var owner workflowExecutionOwner
	var hasOwner bool
	if loadedOwner, ok := workflowExecutionOwnerFromContext(ctx); ok {
		owner = loadedOwner
		hasOwner = true
		request.Fence.ExpectedExecutionID = owner.ExecutionID
		request.Fence.ExpectedExecutionGeneration = owner.Generation
	}
	for _, record := range records {
		if strings.TrimSpace(record.eventType) == "" {
			continue
		}
		data := normalizeWorkflowExecutionEventData(record.eventType, record.data)
		draft := workflowpause.EventDraft{EventType: record.eventType, EventData: data, Category: workflowEventCategory(record.eventType)}
		if hasOwner {
			draft.ExecutionID = owner.ExecutionID
			draft.IdempotencyKey = workflowExecutionEventIdempotencyKey(record.eventType, data, owner)
			if owner.PauseID != "" {
				draft.PauseID = owner.PauseID
				pauseGeneration := owner.PauseGeneration
				draft.PauseGeneration = &pauseGeneration
			}
		}
		request.Events = append(request.Events, draft)
	}
	if len(request.Events) == 0 {
		return []*workflowpause.RunEventPayload{}, nil
	}
	finishTransactionMetric := beginWorkflowDBTransaction(ctx, "event_batch")
	stored := make([]workflowpause.StoredEvent, 0, len(request.Events))
	err := database.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		stored, err = workflowpause.NewService(tx).AppendEventBatchTx(ctx, tx, request)
		if err != nil {
			return err
		}
		projections := make([]internalNodeProjectionEvent, 0, len(stored))
		for index := range stored {
			projections = append(projections, internalNodeProjectionEvent{
				eventType: request.Events[index].EventType,
				data:      request.Events[index].EventData,
				stored:    stored[index].Payload,
			})
		}
		return projectInternalNodeExecutions(ctx, tx, workflowRunID, projections)
	})
	finishTransactionMetric()
	if err != nil {
		for _, record := range records {
			recordWorkflowDurableAppendFailure(ctx, record.eventType)
		}
		return nil, err
	}
	publishWorkflowRuntimeEventSignal(workflowRunID)
	result := make([]*workflowpause.RunEventPayload, len(stored))
	for index := range stored {
		result[index] = stored[index].Payload
	}
	return result, nil
}

func workflowExecutionEventIdempotencyKey(eventType string, data map[string]interface{}, owner workflowExecutionOwner) string {
	if owner.WorkflowRunID == "" || owner.ExecutionID == "" || owner.Generation <= 0 {
		return ""
	}
	switch eventType {
	case workflowpause.EventWorkflowStarted:
		return fmt.Sprintf("run:%s:generation:%d:started", owner.WorkflowRunID, owner.Generation)
	case "node_started", "node_finished":
		nodeExecutionID := workflowEventString(firstWorkflowValue(data["node_execution_id"], data["id"], data["node_id"]))
		if nodeExecutionID == "" {
			return ""
		}
		attempt, _ := workflowEventInt(data["attempt"])
		return fmt.Sprintf("node:%s:%s:%d:%s", owner.ExecutionID, nodeExecutionID, attempt, eventType)
	case "iteration_started", "iteration_completed", "iteration_succeeded", "iteration_failed",
		"loop_started", "loop_completed", "loop_succeeded", "loop_failed":
		containerID := workflowEventString(firstWorkflowValue(data["node_id"], data["id"]))
		if containerID == "" {
			return ""
		}
		return fmt.Sprintf("container:%s:%s:%s", owner.ExecutionID, containerID, eventType)
	case "iteration_next", "loop_next":
		containerID := workflowEventString(firstWorkflowValue(data["node_id"], data["id"]))
		if containerID == "" {
			return ""
		}
		index := 0
		if eventType == "iteration_next" {
			index, _ = workflowEventInt(firstWorkflowValue(data["iteration_index"], data["index"]))
		} else {
			index, _ = workflowEventInt(firstWorkflowValue(data["loop_index"], data["index"]))
		}
		return fmt.Sprintf("round:%s:%s:%d:%s", owner.ExecutionID, containerID, index, eventType)
	default:
		return ""
	}
}

func normalizeWorkflowExecutionEventData(eventType string, input map[string]interface{}) map[string]interface{} {
	data := sanitizeWorkflowEventData(input)
	if eventType != workflowpause.EventNodeStarted && eventType != workflowpause.EventNodeFinished {
		return data
	}
	if strings.TrimSpace(workflowEventString(data["node_execution_id"])) != "" {
		return data
	}
	if nodeExecutionID := strings.TrimSpace(workflowEventString(data["id"])); nodeExecutionID != "" {
		data["node_execution_id"] = nodeExecutionID
	}
	return data
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
	data := sanitizeWorkflowEventData(event.Data)
	payload := map[string]interface{}{
		"event_id":        event.EventID,
		"event":           event.Event,
		"category":        event.Category,
		"schema_version":  event.SchemaVersion,
		"payload_version": event.PayloadVersion,
		"data":            data,
		"sequence":        event.Sequence,
		"created_at":      event.CreatedAt,
		"occurred_at_ms":  event.OccurredAtMS,
		"recorded_at_ms":  event.RecordedAtMS,
	}
	if event.ExecutionID != "" {
		payload["execution_id"] = event.ExecutionID
	}
	if event.PauseID != "" {
		payload["pause_id"] = event.PauseID
	}
	if event.PauseGeneration != nil {
		payload["pause_generation"] = *event.PauseGeneration
	}
	if event.IdempotencyKey != "" {
		payload["idempotency_key"] = event.IdempotencyKey
	}
	for _, key := range []string{"node_execution_id", "container_id", "round_execution_id", "round_index", "causation_id", "correlation_id"} {
		if value, exists := data[key]; exists && value != nil && value != "" {
			payload[key] = value
		}
	}
	if _, exists := payload["node_execution_id"]; !exists {
		if value := firstWorkflowValue(data["id"], data["node_id"]); value != nil {
			payload["node_execution_id"] = value
		}
	}
	fmt.Fprintf(w, "id: %d\n", event.Sequence)
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
	case []map[string]interface{}:
		output := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			output = append(output, sanitizeWorkflowEventData(item))
		}
		return output
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
		"__stored_sequence",
		"__stored_event_id",
		"__stored_event_payload",
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
