package workflow

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

type runtimeLogAgentResolver interface {
	GetByID(ctx context.Context, id string) (*agents.Agent, error)
}

type runtimeLogWorkspacePermissionChecker interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error)
}

type RuntimeLogHandlerOption func(*RuntimeLogHandler)

// RuntimeLogHandler handles runtime log query operations
type RuntimeLogHandler struct {
	workflowRunLogRepo         WorkflowRunLogRepository
	workflowNodeRuntimeLogRepo WorkflowNodeRuntimeLogRepository
	agentsRepo                 runtimeLogAgentResolver
	enterpriseService          runtimeLogWorkspacePermissionChecker
}

// NewRuntimeLogHandler creates a new RuntimeLogHandler
func NewRuntimeLogHandler(workflowRunLogRepo WorkflowRunLogRepository, workflowNodeRuntimeLogRepo WorkflowNodeRuntimeLogRepository, opts ...RuntimeLogHandlerOption) *RuntimeLogHandler {
	handler := &RuntimeLogHandler{
		workflowRunLogRepo:         workflowRunLogRepo,
		workflowNodeRuntimeLogRepo: workflowNodeRuntimeLogRepo,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func WithRuntimeLogAuthorization(agentsRepo runtimeLogAgentResolver, enterpriseService runtimeLogWorkspacePermissionChecker) RuntimeLogHandlerOption {
	return func(handler *RuntimeLogHandler) {
		handler.agentsRepo = agentsRepo
		handler.enterpriseService = enterpriseService
	}
}

// RuntimeLogsRequest represents the request body for runtime logs query
type RuntimeLogsRequest struct {
	TriggeredFrom string   `json:"triggered_from,omitempty"`
	DateRange     []string `json:"date_range,omitempty"` // [start_date, end_date] format: ["2025-08-08", "2025-10-10"]
	Page          int      `json:"page,omitempty"`
	Limit         int      `json:"limit,omitempty"`
}

// GetRuntimeLogs handles POST /agents/:agent_id/runtime-logs
// @Summary Get runtime logs
// @Description Get runtime execution logs for an agent (excluding debugging logs)
// @Tags RuntimeLog
// @Accept json
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param request body RuntimeLogsRequest false "Query parameters (date_range: [start_date, end_date] format: ['2025-08-08', '2025-10-10'])"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/runtime-logs [post]
func (h *RuntimeLogHandler) GetRuntimeLogs(c *gin.Context) {
	agentID := strings.TrimSpace(c.Param("agent_id"))
	accountID := strings.TrimSpace(c.GetString("account_id"))

	filter, ok := h.runtimeLogListAccessFilter(c, agentID, accountID)
	if !ok {
		return
	}

	// Parse request body
	var req RuntimeLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// If no body provided, use default values
		req = RuntimeLogsRequest{
			Page:  1,
			Limit: 20,
		}
	}

	// Set defaults
	page := req.Page
	limit := req.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	triggeredFrom := req.TriggeredFrom

	// Extract start and end dates from date_range array
	var startDateStr, endDateStr string
	if len(req.DateRange) >= 2 {
		startDateStr = req.DateRange[0]
		endDateStr = req.DateRange[1]
	} else if len(req.DateRange) == 1 {
		startDateStr = req.DateRange[0]
	}

	logger.Info("Getting runtime logs", "agentID", agentID, "accountID", accountID, "triggeredFrom", triggeredFrom, "dateRange", req.DateRange)

	// Supported date formats
	dateFormats := []string{
		"2006-01-02",           // YYYY-MM-DD
		time.RFC3339,           // 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04:05Z", // ISO 8601
	}

	// Parse date range
	var startDate, endDate *time.Time
	if startDateStr != "" {
		for _, format := range dateFormats {
			if t, err := time.Parse(format, startDateStr); err == nil {
				startDate = &t
				break
			}
		}
	}
	if endDateStr != "" {
		for _, format := range dateFormats {
			if t, err := time.Parse(format, endDateStr); err == nil {
				endOfDay := t.Truncate(24 * time.Hour).Add(24*time.Hour - time.Nanosecond)
				endDate = &endOfDay
				break
			}
		}
	}

	filter.TriggeredFrom = triggeredFrom
	filter.StartDate = startDate
	filter.EndDate = endDate
	filter.ExcludeDebug = true // Exclude debugging logs

	// Get runtime logs
	logs, total, err := h.workflowRunLogRepo.GetRuntimeLogs(c.Request.Context(), filter, page, limit)
	if err != nil {
		logger.Error("Failed to get runtime logs", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Build response
	items := make([]map[string]interface{}, 0, len(logs))
	for _, log := range logs {
		item := map[string]interface{}{
			"id":               log.ID,
			"workflow_id":      log.WorkflowID,
			"type":             log.Type,
			"triggered_from":   log.TriggeredFrom,
			"version":          log.Version,
			"status":           log.Status,
			"elapsed_time":     workflowRunElapsedMilliseconds(log),
			"total_tokens":     log.TotalTokens,
			"total_steps":      log.TotalSteps,
			"created_by_role":  log.CreatedByRole,
			"created_at":       log.CreatedAt.Unix(),
			"exceptions_count": log.ExceptionsCount,
		}

		if log.WebAppID != nil {
			item["web_app_id"] = *log.WebAppID
		}
		if log.FinishedAt != nil {
			item["finished_at"] = log.FinishedAt.Unix()
		}
		if log.Error != nil {
			item["error"] = *log.Error
		}

		// Parse outputs if available
		if log.Outputs != nil && *log.Outputs != "" {
			item["outputs"] = log.GetOutputsDict()
		}

		items = append(items, item)
	}

	hasMore := int64(page*limit) < total

	response.Success(c, map[string]interface{}{
		"data":     items,
		"page":     page,
		"limit":    limit,
		"total":    total,
		"has_more": hasMore,
	})
}

func (h *RuntimeLogHandler) runtimeLogListAccessFilter(c *gin.Context, agentID, accountID string) (WorkflowRunLogFilter, bool) {
	filter := WorkflowRunLogFilter{AgentID: agentID}
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return filter, false
	}
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return filter, false
	}

	if h.agentsRepo == nil || h.enterpriseService == nil {
		return filter, true
	}

	agent, err := h.agentsRepo.GetByID(c.Request.Context(), agentID)
	if err != nil || agent == nil {
		response.Fail(c, response.ErrAppNotFound)
		return filter, false
	}

	workspaceID := agent.TenantID.String()
	if isSystemWorkflowTenantID(workspaceID) {
		filter.CreatedBy = accountID
		return filter, true
	}

	hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
		c.Request.Context(),
		util.GetOrganizationID(c),
		workspaceID,
		accountID,
		workspace_model.WorkspacePermissionAgentView,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return filter, false
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return filter, false
	}

	return filter, true
}

// GetWorkflowRunNodeLogs handles POST /agents/:agent_id/workflow-runs/:run_id/nodes
// @Summary Get workflow run node logs
// @Description Get node execution logs for a specific workflow run
// @Tags RuntimeLog
// @Accept json
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param run_id path string true "Workflow Run ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflow-runs/{run_id}/nodes [post]
func (h *RuntimeLogHandler) GetWorkflowRunNodeLogs(c *gin.Context) {
	agentID := c.Param("agent_id")
	runID := c.Param("run_id")
	accountID := c.GetString("account_id")

	logger.Info("Getting workflow run node logs", "agentID", agentID, "runID", runID, "accountID", accountID)

	if !h.requireWorkflowRunAccess(c, agentID, runID) {
		return
	}

	// Get node logs for this workflow run
	nodeLogs, err := h.workflowNodeRuntimeLogRepo.GetByWorkflowRunID(c.Request.Context(), runID)
	if err != nil {
		logger.Error("Failed to get node logs", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Build response
	items := make([]map[string]interface{}, 0, len(nodeLogs))
	for _, nodeLog := range nodeLogs {
		item := map[string]interface{}{
			"id":           nodeLog.ID,
			"node_id":      nodeLog.NodeID,
			"node_type":    nodeLog.NodeType,
			"title":        nodeLog.Title,
			"index":        nodeLog.Index,
			"status":       nodeLog.Status,
			"elapsed_time": workflowNodeElapsedMilliseconds(nodeLog),
			"created_at":   nodeLog.CreatedAt.Unix(),
		}

		if nodeLog.PredecessorNodeID != nil {
			item["predecessor_node_id"] = *nodeLog.PredecessorNodeID
		}
		if nodeLog.NodeExecutionID != nil {
			item["node_execution_id"] = *nodeLog.NodeExecutionID
		}
		if nodeLog.WebAppID != nil {
			item["web_app_id"] = *nodeLog.WebAppID
		}
		if nodeLog.FinishedAt != nil {
			item["finished_at"] = nodeLog.FinishedAt.Unix()
		}
		if nodeLog.Error != nil {
			item["error"] = *nodeLog.Error
		}

		// Parse inputs, outputs, process_data if available
		if inputs, err := nodeLog.GetInputsDict(); err == nil && len(inputs) > 0 {
			item["inputs"] = FilterFrontendInputs(nodeLog.NodeType, inputs)
		}
		if outputs, err := nodeLog.GetOutputsDict(); err == nil && len(outputs) > 0 {
			item["outputs"] = FilterFrontendOutputs(nodeLog.NodeType, outputs)
		}
		if processData, err := nodeLog.GetProcessDataDict(); err == nil && len(processData) > 0 {
			item["process_data"] = processData
		}
		if metadata, err := nodeLog.GetExecutionMetadataDict(); err == nil && len(metadata) > 0 {
			item["execution_metadata"] = metadata
		}

		items = append(items, item)
	}

	response.Success(c, map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}

func (h *RuntimeLogHandler) requireWorkflowRunAccess(c *gin.Context, agentID, runID string) bool {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return false
	}
	if agentID == "" || runID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return false
	}

	if h.agentsRepo == nil || h.enterpriseService == nil {
		if h.workflowRunLogRepo != nil {
			run, err := h.workflowRunLogRepo.GetByID(c.Request.Context(), runID)
			if err != nil || run == nil || run.AgentID != agentID {
				response.Fail(c, response.ErrNotFound)
				return false
			}
		}
		return true
	}

	agent, err := h.agentsRepo.GetByID(c.Request.Context(), agentID)
	if err != nil || agent == nil {
		response.Fail(c, response.ErrAppNotFound)
		return false
	}

	workspaceID := agent.TenantID.String()
	if isSystemWorkflowTenantID(workspaceID) {
		return h.requireOwnSystemWorkflowRun(c, agentID, runID, accountID)
	}

	hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
		c.Request.Context(),
		util.GetOrganizationID(c),
		workspaceID,
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
	if h.workflowRunLogRepo != nil {
		run, err := h.workflowRunLogRepo.GetByID(c.Request.Context(), runID)
		if err != nil || run == nil || run.AgentID != agentID {
			response.Fail(c, response.ErrNotFound)
			return false
		}
	}
	return true
}

func (h *RuntimeLogHandler) requireOwnSystemWorkflowRun(c *gin.Context, agentID, runID, accountID string) bool {
	if h.workflowRunLogRepo == nil {
		return true
	}
	run, err := h.workflowRunLogRepo.GetByID(c.Request.Context(), runID)
	if err != nil || run == nil || run.AgentID != agentID {
		response.Fail(c, response.ErrNotFound)
		return false
	}
	if run.CreatedBy != accountID {
		response.Fail(c, response.ErrPermissionDenied)
		return false
	}
	return true
}
