package workflow

import (
	"context"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"

	"github.com/gin-gonic/gin"
)

type workflowStatisticService interface {
	GetAgentWorkspaceID(ctx context.Context, agentID string) (string, error)
	GetWorkflowDailyRuns(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetWorkflowDailyTerminals(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetWorkflowDailyTokenCost(ctx context.Context, tenantID, agentID string) (interface{}, error)
	GetWorkflowAverageAppInteraction(ctx context.Context, tenantID, agentID string) (interface{}, error)
}

type WorkflowStatisticHandlerOption func(*WorkflowStatisticHandler)

// WorkflowStatisticHandler handles workflow statistics requests
type WorkflowStatisticHandler struct {
	workflowService            workflowStatisticService
	workspacePermissionChecker workflowWorkspacePermissionChecker
}

// NewWorkflowStatisticHandler creates a new workflow statistic handler
func NewWorkflowStatisticHandler(workflowService workflowStatisticService, opts ...WorkflowStatisticHandlerOption) *WorkflowStatisticHandler {
	handler := &WorkflowStatisticHandler{
		workflowService: workflowService,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func WithWorkflowStatisticAuthorization(permissionChecker workflowWorkspacePermissionChecker) WorkflowStatisticHandlerOption {
	return func(handler *WorkflowStatisticHandler) {
		handler.workspacePermissionChecker = permissionChecker
	}
}

// GetWorkflowDailyRuns gets workflow daily runs statistics
func (sh *WorkflowStatisticHandler) GetWorkflowDailyRuns(c *gin.Context) {
	sh.handleWorkflowStatistic(c, func(ctx context.Context, workspaceID, agentID string) (interface{}, error) {
		return sh.workflowService.GetWorkflowDailyRuns(ctx, workspaceID, agentID)
	})
}

// GetWorkflowDailyTerminals gets workflow daily terminals statistics
func (sh *WorkflowStatisticHandler) GetWorkflowDailyTerminals(c *gin.Context) {
	sh.handleWorkflowStatistic(c, func(ctx context.Context, workspaceID, agentID string) (interface{}, error) {
		return sh.workflowService.GetWorkflowDailyTerminals(ctx, workspaceID, agentID)
	})
}

// GetWorkflowDailyTokenCost gets workflow daily token cost statistics
func (sh *WorkflowStatisticHandler) GetWorkflowDailyTokenCost(c *gin.Context) {
	sh.handleWorkflowStatistic(c, func(ctx context.Context, workspaceID, agentID string) (interface{}, error) {
		return sh.workflowService.GetWorkflowDailyTokenCost(ctx, workspaceID, agentID)
	})
}

// GetWorkflowAverageAppInteraction gets workflow average app interaction statistics
func (sh *WorkflowStatisticHandler) GetWorkflowAverageAppInteraction(c *gin.Context) {
	sh.handleWorkflowStatistic(c, func(ctx context.Context, workspaceID, agentID string) (interface{}, error) {
		return sh.workflowService.GetWorkflowAverageAppInteraction(ctx, workspaceID, agentID)
	})
}

func (sh *WorkflowStatisticHandler) handleWorkflowStatistic(
	c *gin.Context,
	load func(context.Context, string, string) (interface{}, error),
) {
	appID := c.Param("agent_id")
	workspaceID, ok := sh.requireAgentView(c, appID)
	if !ok {
		return
	}

	var req dto.WorkflowStatisticRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Get timezone from user account
	timezone := "UTC" // Default timezone, should get from user account
	if userTimezone := c.GetString("timezone"); userTimezone != "" {
		timezone = userTimezone
	}

	// Convert timezone format if needed
	if req.Start != nil {
		req.Start = convertToUTCWithTimezone(*req.Start, timezone)
	}
	if req.End != nil {
		req.End = convertToUTCWithTimezone(*req.End, timezone)
	}

	result, err := load(c.Request.Context(), workspaceID, appID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, result)
}

func (sh *WorkflowStatisticHandler) requireAgentView(c *gin.Context, agentID string) (string, bool) {
	accountID := strings.TrimSpace(c.GetString("account_id"))
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return "", false
	}
	if strings.TrimSpace(agentID) == "" {
		response.Fail(c, response.ErrInvalidParam)
		return "", false
	}
	if sh == nil || sh.workflowService == nil || sh.workspacePermissionChecker == nil {
		response.Fail(c, response.ErrSystemError)
		return "", false
	}

	workspaceID, err := sh.workflowService.GetAgentWorkspaceID(c.Request.Context(), agentID)
	if err != nil {
		if strings.Contains(err.Error(), "agent not found") {
			response.Fail(c, response.ErrAppNotFound)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return "", false
	}
	if strings.TrimSpace(workspaceID) == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return "", false
	}

	hasPermission, err := sh.workspacePermissionChecker.CheckWorkspacePermission(
		c.Request.Context(),
		util.GetOrganizationID(c),
		workspaceID,
		accountID,
		workspace_model.WorkspacePermissionWorkflowStatsView,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return "", false
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return "", false
	}
	return workspaceID, true
}

// convertToUTCWithTimezone converts time with timezone consideration
// This is a simplified version, in production you'd want proper timezone handling
func convertToUTCWithTimezone(t time.Time, timezone string) *time.Time {
	// Load the timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		// Default to UTC if timezone loading fails
		loc = time.UTC
	}

	// Convert the time to the specified timezone, then to UTC
	localTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
	utcTime := localTime.UTC()

	return &utcTime
}
