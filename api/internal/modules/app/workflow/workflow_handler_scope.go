package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/app/agents"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/response"
)

type currentWorkspaceGetter interface {
	GetCurrentWorkspace(ctx context.Context, accountID string) (*workspace_model.Workspace, error)
}

type workspaceOrganizationResolver interface {
	GetOrganizationByWorkspaceID(ctx context.Context, workspaceID string) (*workspace_model.Organization, error)
}

type shadowWorkspaceEnsurer interface {
	GetShadowWorkspaceByID(ctx context.Context, organizationID string) (*workspace_model.Workspace, error)
}

type webAppRunScope struct {
	WorkspaceID        string
	OrganizationID     string
	BillingSubjectType string
}

func conversationHistoryCount(history any) int {
	switch v := history.(type) {
	case []map[string]interface{}:
		return len(v)
	case []interface{}:
		return len(v)
	default:
		return 0
	}
}

func workflowOutputKeys(outputs map[string]interface{}) []string {
	return stringMapKeys(outputs)
}

func stringMapKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func isOrganizationScopedWorkflowAgent(agent *agents.Agent) bool {
	return agent != nil && isUnsetWorkflowWorkspaceID(agent.TenantID.String())
}

func resolveWorkflowBillingSubjectType(agent *agents.Agent) string {
	if isOrganizationScopedWorkflowAgent(agent) {
		return "organization"
	}
	return ""
}

func resolveRunStreamWorkspaceID(agent *agents.Agent, callerWorkspaceID, appWorkspaceID string) string {
	if isOrganizationScopedWorkflowAgent(agent) {
		return strings.TrimSpace(callerWorkspaceID)
	}
	return strings.TrimSpace(appWorkspaceID)
}

func resolveWebAppRunWorkspaceID(ctx context.Context, accountService currentWorkspaceGetter, accountID, fallbackWorkspaceID string) (string, error) {
	if accountService != nil && accountID != "" {
		currentWorkspace, err := accountService.GetCurrentWorkspace(ctx, accountID)
		if err != nil {
			if fallbackWorkspaceID == "" {
				return "", err
			}
			return fallbackWorkspaceID, nil
		}
		if currentWorkspace != nil && currentWorkspace.ID != "" {
			return currentWorkspace.ID, nil
		}
	}

	return fallbackWorkspaceID, nil
}

func resolveWebAppRunScope(ctx context.Context, accountService interface {
	currentWorkspaceGetter
	currentOrganizationEnsurer
}, organizationResolver workspaceOrganizationResolver, shadowWorkspaceService shadowWorkspaceEnsurer, accountID string, agent *agents.Agent, fallbackWorkspaceID string) (webAppRunScope, error) {
	scope := webAppRunScope{
		BillingSubjectType: resolveWorkflowBillingSubjectType(agent),
	}

	if scope.BillingSubjectType != "organization" {
		workspaceID, err := resolveWebAppRunWorkspaceID(ctx, accountService, accountID, fallbackWorkspaceID)
		if err != nil {
			return scope, err
		}
		scope.WorkspaceID = workspaceID
		return scope, nil
	}

	workspaceID := strings.TrimSpace(fallbackWorkspaceID)
	if accountService != nil && accountID != "" {
		currentWorkspace, err := accountService.GetCurrentWorkspace(ctx, accountID)
		if err == nil && currentWorkspace != nil && strings.TrimSpace(currentWorkspace.ID) != "" {
			workspaceID = strings.TrimSpace(currentWorkspace.ID)
		} else if err != nil {
			logger.WarnContext(ctx, "failed to resolve current workspace for system web app workflow run, falling back to shadow workspace", "account_id", accountID, err)
		}
	}
	scope.WorkspaceID = workspaceID

	organizationID := ""
	if accountService != nil && accountID != "" {
		if resolvedOrganizationID, err := accountService.EnsureCurrentOrganizationID(ctx, accountID); err == nil {
			organizationID = strings.TrimSpace(resolvedOrganizationID)
		} else {
			logger.WarnContext(ctx, "failed to resolve caller organization for web app workflow run", "account_id", accountID, err)
		}
	}
	if organizationID == "" && workspaceID != "" {
		organizationID = resolveRunOrganizationID(ctx, organizationResolver, workspaceID, nil)
	}
	if organizationID == "" {
		return scope, fmt.Errorf("organization_id is required for system web app workflow run")
	}
	scope.OrganizationID = organizationID

	if scope.WorkspaceID != "" {
		return scope, nil
	}
	if shadowWorkspaceService == nil {
		return scope, fmt.Errorf("shadow workspace service is unavailable")
	}

	shadowWorkspace, err := shadowWorkspaceService.GetShadowWorkspaceByID(ctx, organizationID)
	if err != nil {
		return scope, fmt.Errorf("failed to ensure shadow workspace: %w", err)
	}
	if shadowWorkspace == nil || strings.TrimSpace(shadowWorkspace.ID) == "" {
		return scope, fmt.Errorf("shadow workspace not found")
	}

	scope.WorkspaceID = strings.TrimSpace(shadowWorkspace.ID)
	return scope, nil
}

func (h *WorkflowHandler) resolveAgentWorkspaceID(c *gin.Context, agentID string) (string, bool) {
	workspaceID, err := h.workflowService.GetAgentWorkspaceID(c.Request.Context(), agentID)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to get agent workspace id", "agent_id", agentID, err)
		if strings.Contains(err.Error(), "agent not found") {
			response.Fail(c, response.ErrAppNotFound)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return "", false
	}

	if workspaceID == "" {
		logger.CriticalContext(c.Request.Context(), "agent workspace id not found", "agent_id", agentID, fmt.Errorf("workspace_id missing for agent %s", agentID))
		response.Fail(c, response.ErrWorkspaceNotFound)
		return "", false
	}

	return workspaceID, true
}

func (h *WorkflowHandler) requireAgentWorkspacePermission(c *gin.Context, agentID string, permissionCode workspace_model.WorkspacePermissionCode) (string, bool) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return "", false
	}

	workspaceID, ok := h.resolveAgentWorkspaceID(c, agentID)
	if !ok {
		return "", false
	}

	if h.enterpriseService == nil {
		return workspaceID, true
	}

	hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
		c.Request.Context(),
		util.GetOrganizationID(c),
		workspaceID,
		accountID,
		permissionCode,
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

func resolveRunOrganizationID(ctx context.Context, resolver workspaceOrganizationResolver, workspaceID string, inputs map[string]interface{}) string {
	if inputs != nil {
		if orgID, ok := inputs["sys.organization_id"].(string); ok && orgID != "" {
			return orgID
		}
	}

	if resolver == nil || workspaceID == "" {
		return ""
	}

	org, err := resolver.GetOrganizationByWorkspaceID(ctx, workspaceID)
	if err != nil || org == nil {
		return ""
	}

	return org.ID
}

func buildWorkflowStartedEventPayload(runType, workflowRunLogID, workflowID string, sequenceNumber int, systemInputs map[string]interface{}, createdAt int64, reasons ...string) map[string]interface{} {
	reason := workflowStartReasonInitial
	if len(reasons) > 0 && strings.TrimSpace(reasons[0]) != "" {
		reason = strings.TrimSpace(reasons[0])
	}
	payload := map[string]interface{}{
		"id":              workflowRunLogID,
		"workflow_id":     workflowID,
		"sequence_number": sequenceNumber,
		"inputs":          systemInputs,
		"created_at":      createdAt,
		"reason":          reason,
	}

	if runType != "CONVERSATION_WORKFLOW" {
		return payload
	}

	payload["message_id"] = workflowRunLogID
	if conversationID, ok := systemInputs["sys.conversation_id"].(string); ok && conversationID != "" {
		payload["conversation_id"] = conversationID
	}

	return payload
}
