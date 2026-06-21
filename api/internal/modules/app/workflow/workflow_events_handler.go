package workflow

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	workflowEventsPollInterval = time.Second
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
	includeSnapshot := strings.EqualFold(c.Query("include_snapshot"), "true")
	includeMessageEvents := strings.EqualFold(c.Query("include_message_events"), "true")
	continueOnPause := strings.EqualFold(c.Query("continue_on_pause"), "true")

	prepareWorkflowEventsSSE(c)

	service := workflowpause.NewService(database.GetDB())
	lastSequence := afterSequence
	messageReplayCutoff := 0
	latest, err := service.LatestEventSequence(c.Request.Context(), run.TenantID, run.ID)
	if err != nil {
		logger.WarnContext(c.Request.Context(), "failed to load latest workflow event sequence", "workflow_run_id", run.ID, err)
	} else {
		if !includeMessageEvents {
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
		case <-pollTicker.C:
			sequence, terminal := h.sendWorkflowRunEvents(c, service, run, lastSequence, continueOnPause, includeMessageEvents, messageReplayCutoff)
			lastSequence = sequence
			if terminal {
				return
			}
		}
	}
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
			workspace_model.WorkspacePermissionAgentView,
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
	payload, err := service.ListEvents(c.Request.Context(), run.TenantID, run.ID, afterSequence, workflowEventsBatchLimit)
	if err != nil {
		logger.WarnContext(c.Request.Context(), "failed to load workflow run events", "workflow_run_id", run.ID, err)
		return afterSequence, false
	}

	lastSequence := afterSequence
	for _, event := range payload.Events {
		lastSequence = event.Sequence
		if shouldSkipWorkflowReplayMessage(event, includeMessageEvents, messageReplayCutoff) {
			continue
		}
		event.Data = h.workflowStoredEventData(c.Request.Context(), run, event)
		sendWorkflowSSEStoredEvent(c.Request.Context(), c.Writer, event)
		if event.Event == workflowpause.EventWorkflowFinished {
			return lastSequence, true
		}
		if event.Event == workflowpause.EventWorkflowPaused && !continueOnPause {
			return lastSequence, true
		}
	}
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
