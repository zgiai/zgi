package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

const (
	workflowEventsPollInterval = 10 * time.Second
	workflowEventsPingInterval = 10 * time.Second
	workflowEventsBatchLimit   = 100
)

func (h *WorkflowHandler) GetWorkflowRunEvents(c *gin.Context) {
	workflowRunID := strings.TrimSpace(c.Param("workflow_run_id"))
	if workflowRunID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	workflowService, ok := h.workflowService.(*WorkflowService)
	if !ok || workflowService == nil || workflowService.workflowRunLogRepo == nil {
		response.FailWithMessage(c, response.ErrSystemError, "workflow service is not available")
		return
	}
	run, err := workflowService.workflowRunLogRepo.GetByID(c.Request.Context(), workflowRunID)
	if err != nil {
		response.Fail(c, response.ErrNotFound)
		return
	}
	if !h.requireWorkflowRunEventAccess(c, workflowService, run) {
		return
	}

	afterSequence, hasAfter, err := parseWorkflowEventsAfter(c.Query("after"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if !hasAfter {
		afterSequence, hasAfter, err = parseWorkflowEventsAfter(c.GetHeader("Last-Event-ID"))
		if err != nil {
			response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
			return
		}
	}
	includeSnapshot := strings.EqualFold(c.Query("include_snapshot"), "true")
	includeMessageEvents := strings.EqualFold(c.Query("include_message_events"), "true")
	continueOnPause := strings.EqualFold(c.Query("continue_on_pause"), "true")

	prepareWorkflowEventsSSE(c)

	service := workflowpause.NewService(database.GetDB())
	eventSignal, cancelEventSignal := subscribeWorkflowRuntimeEvents(database.GetDB(), run.ID)
	defer cancelEventSignal()
	lastSequence := afterSequence
	if includeSnapshot && run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
		snapshotStartedAt := time.Now()
		snapshot, snapshotErr := h.buildWorkflowRunSnapshot(c.Request.Context(), run.ID)
		recordWorkflowSnapshotLatency(c.Request.Context(), float64(time.Since(snapshotStartedAt).Microseconds())/1000)
		if snapshotErr != nil {
			logger.WarnContext(c.Request.Context(), "failed to build workflow run snapshot", "workflow_run_id", run.ID, snapshotErr)
			return
		}
		lastSequence = snapshot.Sequence
		sendWorkflowSSEStoredEventForInvocation(
			c.Request.Context(),
			c.Writer,
			workflowRunEventProjectionInvokeFrom(run),
			snapshot,
		)
		if workflowSnapshotIsTerminal(snapshot) {
			return
		}
		includeSnapshot = false
		hasAfter = true
	}
	messageReplayCutoff := 0
	latest, err := service.LatestEventSequence(c.Request.Context(), run.TenantID, run.ID)
	if err != nil {
		logger.WarnContext(c.Request.Context(), "failed to load latest workflow event sequence", "workflow_run_id", run.ID, err)
	} else {
		if !includeMessageEvents && run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
			messageReplayCutoff = latest
		}
		logger.DebugContext(c.Request.Context(), "workflow events replay boundary loaded",
			"workflow_run_id", run.ID,
			"replay_cutoff", messageReplayCutoff,
			"include_message_events", includeMessageEvents,
		)
		if !includeSnapshot && !hasAfter {
			lastSequence = latest
		}
	}

	if includeSnapshot || hasAfter {
		sequence, terminal := h.sendWorkflowRunEvents(c, service, run, lastSequence, continueOnPause, includeMessageEvents, messageReplayCutoff)
		lastSequence = sequence
		if terminal {
			return
		}
	}

	pollTicker := time.NewTicker(workflowEventsPollInterval)
	defer pollTicker.Stop()
	pingTicker := time.NewTicker(workflowEventsPingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-pingTicker.C:
			sendWorkflowSSEPing(c.Request.Context(), c.Writer)
		case <-eventSignal:
			sequence, terminal := h.sendWorkflowRunEvents(c, service, run, lastSequence, continueOnPause, includeMessageEvents, messageReplayCutoff)
			lastSequence = sequence
			if terminal {
				return
			}
		case <-pollTicker.C:
			sequence, terminal := h.sendWorkflowRunEvents(c, service, run, lastSequence, continueOnPause, includeMessageEvents, messageReplayCutoff)
			lastSequence = sequence
			if terminal {
				return
			}
		}
	}
}

func workflowSnapshotIsTerminal(snapshot workflowpause.RunEventPayload) bool {
	run, _ := snapshot.Data["workflow_run"].(map[string]interface{})
	status, _ := run["status"].(string)
	switch dto.WorkflowRunStatus(strings.ToLower(strings.TrimSpace(status))) {
	case dto.WorkflowRunStatusSucceeded, dto.WorkflowRunStatusFailed, dto.WorkflowRunStatusStopped:
		return true
	default:
		return false
	}
}

func (h *WorkflowHandler) buildWorkflowRunSnapshot(ctx context.Context, workflowRunID string) (workflowpause.RunEventPayload, error) {
	db := database.GetDB()
	var payload workflowpause.RunEventPayload
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run WorkflowRunLog
		if err := tx.Where("id = ? AND deleted_at IS NULL", workflowRunID).First(&run).Error; err != nil {
			return err
		}
		var message conversation.AgentMessage
		messageErr := tx.Where("workflow_run_id = ? AND deleted_at IS NULL", workflowRunID).First(&message).Error
		if messageErr != nil && !errors.Is(messageErr, gorm.ErrRecordNotFound) {
			return messageErr
		}
		var nodes []WorkflowNodeRuntimeLog
		if err := tx.Where("workflow_run_id = ? AND deleted_at IS NULL", workflowRunID).
			Order("index ASC, created_at ASC, id ASC").Find(&nodes).Error; err != nil {
			return err
		}
		pauseData := map[string]interface{}(nil)
		pauseRecord, reasons, _, pauseErr := workflowpause.NewService(tx).GetActiveByWorkflowRunID(ctx, workflowRunID)
		if pauseErr == nil {
			pauseReasons, err := workflowSnapshotPauseReasons(tx, pauseRecord, reasons)
			if err != nil {
				return err
			}
			pauseData = map[string]interface{}{
				"pause": map[string]interface{}{
					"id":              pauseRecord.ID,
					"workflow_run_id": pauseRecord.WorkflowRunID,
					"node_id":         pauseRecord.NodeID,
					"reason":          pauseRecord.Reason,
					"generation":      pauseRecord.Generation,
					"status":          pauseRecord.Status,
					"revision":        pauseRecord.Revision,
					"created_at":      pauseRecord.CreatedAt.Unix(),
					"conversation_id": pauseRecord.ConversationID,
				},
				"reasons": pauseReasons,
			}
		} else if !errors.Is(pauseErr, workflowpause.ErrPauseNotFound) {
			return pauseErr
		}
		messageData := map[string]interface{}(nil)
		if messageErr == nil {
			messageData = map[string]interface{}{
				"id":                   message.ID,
				"answer":               message.Answer,
				"status":               message.Status,
				"projection_revision":  message.ProjectionRevision,
				"execution_generation": message.ExecutionGeneration,
			}
		}
		executionID := ""
		if run.ActiveExecutionID != nil {
			executionID = *run.ActiveExecutionID
		}
		runData := map[string]interface{}{
			"id":                       run.ID,
			"workflow_id":              run.WorkflowID,
			"status":                   run.Status,
			"inputs":                   run.GetInputsDict(),
			"outputs":                  run.GetOutputsDict(),
			"elapsed_time":             run.ElapsedTime,
			"total_tokens":             run.TotalTokens,
			"total_steps":              run.TotalSteps,
			"exceptions_count":         run.ExceptionsCount,
			"created_at":               run.CreatedAt.Unix(),
			"runtime_protocol_version": run.RuntimeProtocolVersion,
			"execution_generation":     run.ExecutionGeneration,
			"state_revision":           run.StateRevision,
		}
		if run.FinishedAt != nil {
			runData["finished_at"] = run.FinishedAt.Unix()
		}
		if run.Error != nil {
			runData["error"] = *run.Error
		}
		nodeData := make([]map[string]interface{}, 0, len(nodes))
		for i := range nodes {
			nodeData = append(nodeData, workflowRunSnapshotNode(nodes[i]))
		}
		payload = workflowpause.RunEventPayload{
			Sequence:       int(run.NextEventSequence),
			Event:          "workflow_snapshot",
			Category:       workflowpause.EventCategoryControl,
			SchemaVersion:  2,
			PayloadVersion: 1,
			ExecutionID:    executionID,
			Data: map[string]interface{}{
				"workflow_run":  runData,
				"message":       messageData,
				"nodes":         nodeData,
				"active_pause":  pauseData,
				"last_sequence": run.NextEventSequence,
			},
			CreatedAt:    time.Now().Unix(),
			OccurredAtMS: time.Now().UnixMilli(),
			RecordedAtMS: time.Now().UnixMilli(),
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return workflowpause.RunEventPayload{}, err
	}
	return payload, nil
}

func workflowSnapshotPauseReasons(tx *gorm.DB, pauseRecord *workflowpause.RunPause, reasons []workflowpause.RunPauseReason) ([]map[string]interface{}, error) {
	requestedByNode := make(map[string]map[string]interface{})
	if pauseRecord != nil {
		var events []workflowpause.RunEvent
		if err := tx.Where("workflow_run_id = ? AND pause_id = ? AND event_type IN ?", pauseRecord.WorkflowRunID, pauseRecord.ID,
			[]string{workflowpause.EventApprovalRequested, workflowpause.EventQuestionAnswerRequested}).
			Order("sequence ASC").Find(&events).Error; err != nil {
			return nil, err
		}
		for _, event := range events {
			data := map[string]interface{}{}
			if err := json.Unmarshal([]byte(event.EventData), &data); err != nil {
				continue
			}
			nodeID, _ := data["node_id"].(string)
			if nodeID != "" {
				requestedByNode[nodeID] = data
			}
		}
	}
	result := make([]map[string]interface{}, 0, len(reasons))
	for _, reason := range reasons {
		item := map[string]interface{}{
			"id":      reason.ID,
			"type":    reason.Type,
			"node_id": reason.NodeID,
			"form_id": reason.FormID,
			"status":  reason.Status,
		}
		for key, value := range requestedByNode[reason.NodeID] {
			item[key] = value
		}
		result = append(result, item)
	}
	return result, nil
}

func workflowRunSnapshotNode(node WorkflowNodeRuntimeLog) map[string]interface{} {
	item := map[string]interface{}{
		"id":            node.ID,
		"node_id":       node.NodeID,
		"node_type":     node.NodeType,
		"title":         node.Title,
		"index":         node.Index,
		"status":        node.Status,
		"elapsed_time":  workflowNodeElapsedMilliseconds(node),
		"created_at":    node.CreatedAt.Unix(),
		"created_at_ms": node.CreatedAt.UnixMilli(),
		"attempt":       node.Attempt,
	}
	if node.NodeExecutionID != nil {
		item["node_execution_id"] = *node.NodeExecutionID
	}
	if node.PredecessorNodeID != nil {
		item["predecessor_node_id"] = *node.PredecessorNodeID
	}
	if node.ParentExecutionID != nil {
		item["parent_execution_id"] = *node.ParentExecutionID
	}
	if node.ContainerID != nil {
		item["container_id"] = *node.ContainerID
	}
	if node.ContainerType != nil {
		item["container_type"] = *node.ContainerType
	}
	if node.RoundIndex != nil {
		item["round_index"] = *node.RoundIndex
	}
	if node.StartedEventSequence != nil {
		item["started_event_sequence"] = *node.StartedEventSequence
		item["sequence"] = *node.StartedEventSequence
	}
	if node.FinishedEventSequence != nil {
		item["finished_event_sequence"] = *node.FinishedEventSequence
		if _, ok := item["sequence"]; !ok {
			item["sequence"] = *node.FinishedEventSequence
		}
	}
	if node.FinishedAt != nil {
		item["finished_at"] = node.FinishedAt.Unix()
	}
	if node.Error != nil {
		item["error"] = *node.Error
	}
	if inputs, err := node.GetInputsDict(); err == nil && len(inputs) > 0 {
		item["inputs"] = FilterFrontendInputs(node.NodeType, inputs)
	}
	if outputs, err := node.GetOutputsDict(); err == nil && len(outputs) > 0 {
		item["outputs"] = FilterFrontendOutputs(node.NodeType, outputs)
	}
	if processData, err := node.GetProcessDataDict(); err == nil && len(processData) > 0 {
		item["process_data"] = processData
	}
	if metadata, err := node.GetExecutionMetadataDict(); err == nil && len(metadata) > 0 {
		item["execution_metadata"] = metadata
	}
	return item
}

func (h *WorkflowHandler) requireWorkflowRunEventAccess(c *gin.Context, workflowService *WorkflowService, run *WorkflowRunLog) bool {
	if run == nil || strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.AgentID) == "" {
		response.Fail(c, response.ErrNotFound)
		return false
	}
	accountID := strings.TrimSpace(c.GetString("account_id"))
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return false
	}
	if run.TriggeredFrom == string(InvokeFromWebApp) || run.CreatedByRole == CreatedByRoleEndUser {
		virtualAccountID := strings.TrimSpace(c.GetString("virtual_account_id"))
		if strings.TrimSpace(run.CreatedBy) == "" || (run.CreatedBy != accountID && run.CreatedBy != virtualAccountID) {
			response.Fail(c, response.ErrPermissionDenied)
			return false
		}
		if err := workflowService.ValidateWorkflowRunAccess(c.Request.Context(), run.TenantID, run.AgentID, run.ID, run.CreatedBy); err != nil {
			h.failWorkflowRunAccess(c, err)
			return false
		}
		return true
	}

	if isSystemWorkflowTenantID(run.TenantID) {
		if strings.TrimSpace(run.CreatedBy) == "" || run.CreatedBy != accountID {
			response.Fail(c, response.ErrPermissionDenied)
			return false
		}
		if err := workflowService.ValidateWorkflowRunAccess(c.Request.Context(), run.TenantID, run.AgentID, run.ID, accountID); err != nil {
			h.failWorkflowRunAccess(c, err)
			return false
		}
		return true
	}

	permissionChecker := h.getWorkspacePermissionChecker()
	if permissionChecker != nil {
		hasPermission, err := permissionChecker.CheckWorkspacePermission(
			c.Request.Context(),
			util.GetOrganizationID(c),
			run.TenantID,
			accountID,
			workspace_model.WorkspacePermissionWorkflowRunDraft,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return false
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return false
		}
	}

	if err := workflowService.ValidateWorkflowRunAccess(c.Request.Context(), run.TenantID, run.AgentID, run.ID, accountID); err != nil {
		h.failWorkflowRunAccess(c, err)
		return false
	}
	return true
}

func prepareWorkflowEventsSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}); err != nil {
		logger.WarnContext(c.Request.Context(), "workflow events SSE write deadline is not configurable", "workflow_run_id", c.Param("workflow_run_id"), err)
	}
	flushWorkflowSSE(c.Request.Context(), c.Writer, "workflow_events_open")
}

func (h *WorkflowHandler) sendWorkflowRunEvents(c *gin.Context, service *workflowpause.Service, run *WorkflowRunLog, afterSequence int, continueOnPause bool, includeMessageEvents bool, messageReplayCutoff int) (int, bool) {
	payload := &workflowpause.RunEventsPayload{WorkflowRunID: run.ID}
	if cached, ok := readWorkflowCommittedTailAfter(c.Request.Context(), run.ID, afterSequence, workflowEventsBatchLimit); ok {
		payload.Events = cached
	} else {
		stored, err := service.ListEvents(c.Request.Context(), run.TenantID, run.ID, afterSequence, workflowEventsBatchLimit)
		if err != nil {
			logger.WarnContext(c.Request.Context(), "failed to load workflow run events", "workflow_run_id", run.ID, err)
			return afterSequence, false
		}
		payload = stored
	}

	lastSequence := afterSequence
	replayBytes := 0
	replayCount := 0
	for _, event := range payload.Events {
		lastSequence = event.Sequence
		if shouldSkipWorkflowReplayMessage(event, includeMessageEvents, messageReplayCutoff) {
			continue
		}
		event.Data = h.workflowStoredEventData(c.Request.Context(), run, event)
		if encoded, marshalErr := json.Marshal(event); marshalErr == nil {
			replayBytes += len(encoded)
		}
		replayCount++
		sendWorkflowSSEStoredEventForInvocation(
			c.Request.Context(),
			c.Writer,
			workflowRunEventProjectionInvokeFrom(run),
			event,
		)
		if event.Event == workflowpause.EventWorkflowFinished {
			recordWorkflowReplay(c.Request.Context(), replayCount, replayBytes)
			return lastSequence, true
		}
		if event.Event == workflowpause.EventWorkflowPaused && !continueOnPause {
			recordWorkflowReplay(c.Request.Context(), replayCount, replayBytes)
			return lastSequence, true
		}
	}
	recordWorkflowReplay(c.Request.Context(), replayCount, replayBytes)
	return lastSequence, false
}

func shouldSkipWorkflowReplayMessage(event workflowpause.RunEventPayload, includeMessageEvents bool, messageReplayCutoff int) bool {
	if includeMessageEvents || event.Event != workflowEventMessage {
		return false
	}
	return event.Sequence <= messageReplayCutoff
}

func (h *WorkflowHandler) workflowStoredEventData(ctx context.Context, run *WorkflowRunLog, event workflowpause.RunEventPayload) map[string]interface{} {
	data := sanitizeWorkflowEventData(event.Data)
	workflowService, ok := h.workflowService.(*WorkflowService)
	if !ok || workflowService == nil || workflowService.workflowNodeRuntimeLogRepo == nil {
		return filterStoredEventFrontendData(event.Event, data)
	}

	switch event.Event {
	case workflowpause.EventNodeFinished:
		nodeExecutionID, ok := data["node_execution_id"].(string)
		if !ok || nodeExecutionID == "" {
			return filterStoredEventFrontendData(event.Event, data)
		}
		nodeLog, err := workflowService.workflowNodeRuntimeLogRepo.GetByNodeExecutionID(ctx, nodeExecutionID)
		if err != nil || nodeLog == nil {
			return filterStoredEventFrontendData(event.Event, data)
		}
		data["elapsed_time"] = workflowNodeElapsedMilliseconds(*nodeLog)
	case workflowpause.EventWorkflowPaused, workflowpause.EventWorkflowFinished:
		if run == nil || run.ID == "" {
			return filterStoredEventFrontendData(event.Event, data)
		}
		if elapsed := workflowService.workflowRunNodeElapsedMilliseconds(ctx, run.ID); elapsed > 0 {
			data["elapsed_time"] = elapsed
		}
	}
	return filterStoredEventFrontendData(event.Event, data)
}

// filterStoredEventFrontendData applies frontend input/output filtering to
// persisted event data based on the event type and embedded node_type.
func filterStoredEventFrontendData(eventType string, data map[string]interface{}) map[string]interface{} {
	if eventType != workflowpause.EventNodeStarted && eventType != workflowpause.EventNodeFinished {
		return data
	}
	nodeType, _ := data["node_type"].(string)
	if nodeType == "" {
		return data
	}
	if inputs, ok := data["inputs"].(map[string]interface{}); ok {
		data["inputs"] = FilterFrontendInputs(nodeType, inputs)
	}
	if eventType == workflowpause.EventNodeFinished {
		if outputs, ok := data["outputs"].(map[string]interface{}); ok {
			data["outputs"] = FilterFrontendOutputs(nodeType, outputs)
		}
	}
	return data
}

func parseWorkflowEventsAfter(raw string) (int, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, true, err
	}
	if value < 0 {
		return 0, true, strconv.ErrSyntax
	}
	return value, true, nil
}
