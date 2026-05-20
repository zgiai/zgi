package workflow

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

func (h *WorkflowHandler) PrecheckDraftWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")
	callerOrganizationID := util.GetOrganizationID(c)

	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	appWorkspaceID, err := h.workflowService.GetAgentWorkspaceID(c.Request.Context(), appID)
	if err != nil {
		logger.Error("Failed to get agent workspace id", err)
		if err.Error() == "agent not found" {
			response.Fail(c, response.ErrAppNotFound)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	if h.enterpriseService != nil {
		hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
			c.Request.Context(),
			callerOrganizationID,
			appWorkspaceID,
			accountID,
			workspace_model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	workflow, err := h.workflowService.GetDraftWorkflow(c.Request.Context(), appID, true)
	if err != nil {
		logger.Error("Failed to get draft workflow for precheck", err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	agent, err := h.getAgentForPrecheck(c.Request.Context(), appID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	h.respondWorkflowRunPrecheck(c, workflow, agent, appID, accountID, callerOrganizationID, "", appWorkspaceID, nil)
}

func (h *WorkflowHandler) PrecheckAdvancedChatDraftWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")
	callerOrganizationID := util.GetOrganizationID(c)

	appWorkspaceID, ok := h.requireAgentWorkspacePermission(c, appID, workspace_model.WorkspacePermissionAgentManage)
	if !ok {
		return
	}

	workflow, err := h.workflowService.GetDraftWorkflow(c.Request.Context(), appID, true)
	if err != nil {
		logger.Error("Failed to get draft workflow for precheck", err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	agent, err := h.getAgentForPrecheck(c.Request.Context(), appID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	h.respondWorkflowRunPrecheck(c, workflow, agent, appID, accountID, callerOrganizationID, "", appWorkspaceID, nil)
}

func (h *WorkflowHandler) PrecheckAdvancedChatWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")
	callerOrganizationID := util.GetOrganizationID(c)

	invokeFrom := c.GetString("invoke_from")
	if invokeFrom == "" {
		invokeFrom = string(InvokeFromWebApp)
	}
	createdFrom := c.GetString("created_from")
	if createdFrom == "" {
		createdFrom = "web-app"
	}
	createdByRole := c.GetString("created_by_role")
	if createdByRole == "" {
		createdByRole = "account"
	}

	ctx := context.WithValue(c.Request.Context(), "invoke_from", invokeFrom)
	ctx = context.WithValue(ctx, "created_from", createdFrom)
	ctx = context.WithValue(ctx, "created_by_role", createdByRole)

	workflow, err := h.workflowService.GetLatestPublishedWorkflow(ctx, callerOrganizationID, appID, true)
	if err != nil {
		logger.Error("Failed to get published workflow for precheck", err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	appWorkspaceID, err := h.workflowService.GetAgentWorkspaceID(c.Request.Context(), appID)
	if err != nil {
		logger.Error("Failed to get agent workspace id", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	agent, err := h.getAgentForPrecheck(c.Request.Context(), appID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	h.respondWorkflowRunPrecheck(c, workflow, agent, appID, accountID, callerOrganizationID, "", appWorkspaceID, nil)
}

func (h *WorkflowHandler) PrecheckPublishedWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")
	callerOrganizationID := util.GetOrganizationID(c)

	invokeFrom := c.GetString("invoke_from")
	if invokeFrom == "" {
		invokeFrom = string(InvokeFromWebApp)
	}
	createdFrom := c.GetString("created_from")
	if createdFrom == "" {
		createdFrom = "web-app"
	}
	createdByRole := c.GetString("created_by_role")
	if createdByRole == "" {
		createdByRole = "account"
	}

	ctx := context.WithValue(c.Request.Context(), "invoke_from", invokeFrom)
	ctx = context.WithValue(ctx, "created_from", createdFrom)
	ctx = context.WithValue(ctx, "created_by_role", createdByRole)

	workflow, err := h.workflowService.GetLatestPublishedWorkflow(ctx, callerOrganizationID, appID, true)
	if err != nil {
		logger.Error("Failed to get published workflow for precheck", err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	appWorkspaceID, err := h.workflowService.GetAgentWorkspaceID(c.Request.Context(), appID)
	if err != nil {
		logger.Error("Failed to get agent workspace id", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	agent, err := h.getAgentForPrecheck(c.Request.Context(), appID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	h.respondWorkflowRunPrecheck(c, workflow, agent, appID, accountID, callerOrganizationID, "", appWorkspaceID, nil)
}

func (h *WorkflowHandler) PrecheckWorkflowByWebAppID(c *gin.Context) {
	webAppID := c.Param("web_app_id")
	accountID := c.GetString("account_id")

	agent, workspaceID, err := h.resolveWebAppPrecheckAgent(c)
	if err != nil {
		return
	}

	agentID := agent.ID.String()
	workflow, err := h.workflowService.GetLatestPublishedWorkflow(c.Request.Context(), workspaceID, agentID, true)
	if err != nil {
		logger.Error("Failed to get latest published workflow", err)
		response.FailWithMessage(c, response.ErrAppNotFound, "workflow not found")
		return
	}

	appWorkspaceID := ""
	if agent.TenantID.String() != "00000000-0000-0000-0000-000000000000" {
		appWorkspaceID = agent.TenantID.String()
	}

	_ = webAppID
	callerOrganizationID := resolveCallerOrganizationForWebAppPrecheck(c.Request.Context(), h.accountService, accountID, agent)
	h.respondWorkflowRunPrecheck(c, workflow, agent, agentID, accountID, callerOrganizationID, workspaceID, appWorkspaceID, nil)
}

func (h *WorkflowHandler) getAgentForPrecheck(ctx context.Context, appID string) (*agents.Agent, error) {
	ws, ok := h.workflowService.(*WorkflowService)
	if !ok || ws.agentsRepo == nil {
		logger.Error("Workflow service or agents repository not available", nil)
		return nil, context.Canceled
	}
	return ws.agentsRepo.GetByID(ctx, appID)
}

func resolveWorkflowPrecheckSubjects(ctx context.Context, resolver workspaceOrganizationResolver, agent *agents.Agent, callerOrganizationID, callerWorkspaceID, appWorkspaceID string, userInputs map[string]any) (organizationID, workspaceID, billingSubjectType string) {
	billingSubjectType = resolveWorkflowBillingSubjectType(agent)
	if billingSubjectType == llmclient.BillingSubjectTypeOrganization {
		organizationID = strings.TrimSpace(callerOrganizationID)
		if organizationID == "" {
			organizationID = resolveRunOrganizationID(ctx, resolver, strings.TrimSpace(callerWorkspaceID), userInputs)
		}
		return organizationID, "", billingSubjectType
	}

	workspaceID = strings.TrimSpace(appWorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(callerWorkspaceID)
	}
	organizationID = resolveRunOrganizationID(ctx, resolver, workspaceID, userInputs)
	return organizationID, workspaceID, billingSubjectType
}

type currentOrganizationEnsurer interface {
	EnsureCurrentOrganizationID(ctx context.Context, accountID string) (string, error)
}

func resolveCallerOrganizationForWebAppPrecheck(ctx context.Context, accountService currentOrganizationEnsurer, accountID string, agent *agents.Agent) string {
	if resolveWorkflowBillingSubjectType(agent) != llmclient.BillingSubjectTypeOrganization {
		return ""
	}
	if accountService == nil || accountID == "" {
		return ""
	}

	organizationID, err := accountService.EnsureCurrentOrganizationID(ctx, accountID)
	if err != nil {
		logger.WarnContext(ctx, "failed to resolve caller organization for web app workflow precheck", "account_id", accountID, err)
		return ""
	}
	return organizationID
}

func webAppPrecheckRequiresWorkspace(agent *agents.Agent) bool {
	return resolveWorkflowBillingSubjectType(agent) != llmclient.BillingSubjectTypeOrganization
}

func (h *WorkflowHandler) respondWorkflowRunPrecheck(c *gin.Context, workflow any, agent *agents.Agent, appID, accountID, callerOrganizationID, callerWorkspaceID, appWorkspaceID string, userInputs map[string]any) {
	ws, ok := h.workflowService.(*WorkflowService)
	if !ok {
		response.Fail(c, response.ErrSystemError)
		return
	}

	organizationID, workspaceID, billingSubjectType := resolveWorkflowPrecheckSubjects(
		c.Request.Context(),
		h.enterpriseService,
		agent,
		callerOrganizationID,
		callerWorkspaceID,
		appWorkspaceID,
		userInputs,
	)
	if billingSubjectType != llmclient.BillingSubjectTypeOrganization && workspaceID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	appCtx := &llmclient.AppContext{
		OrganizationID:     organizationID,
		WorkspaceID:        workspaceID,
		BillingSubjectType: billingSubjectType,
		AppID:              appID,
		AppType:            "agent",
		AccountID:          accountID,
	}

	result, err := ws.PrecheckWorkflowRun(c.Request.Context(), workflow, appCtx, userInputs)
	if err != nil {
		logger.Error("Failed to precheck workflow run", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

func (h *WorkflowHandler) resolveWebAppPrecheckAgent(c *gin.Context) (*agents.Agent, string, error) {
	webAppID := c.Param("web_app_id")
	accountID := c.GetString("account_id")

	ws, ok := h.workflowService.(*WorkflowService)
	if !ok || ws.agentsRepo == nil {
		logger.Error("Workflow service or agents repository not available", nil)
		response.Fail(c, response.ErrSystemError)
		return nil, "", context.Canceled
	}

	agent, err := ws.agentsRepo.GetByWebAppID(c.Request.Context(), webAppID)
	if err != nil {
		logger.Error("Failed to get agent by web_app_id", err)
		response.Fail(c, response.ErrAppNotFound)
		return nil, "", err
	}
	if rejectInactiveWebApp(c, agent, webAppID) {
		return nil, "", context.Canceled
	}

	fallbackWorkspaceID := ""
	if !isUnsetWorkflowWorkspaceID(agent.TenantID.String()) {
		fallbackWorkspaceID = agent.TenantID.String()
	}

	workspaceID, err := resolveWebAppRunWorkspaceID(c.Request.Context(), h.accountService, accountID, fallbackWorkspaceID)
	if err != nil {
		logger.Error("Failed to resolve workspace for web app workflow precheck", err)
		response.Fail(c, response.ErrSystemError)
		return nil, "", err
	}
	if workspaceID == "" && webAppPrecheckRequiresWorkspace(agent) {
		logger.Error("Caller has no available workspace for web app workflow precheck", nil)
		response.Fail(c, response.ErrWorkspaceNotFound)
		return nil, "", context.Canceled
	}

	return agent, workspaceID, nil
}
