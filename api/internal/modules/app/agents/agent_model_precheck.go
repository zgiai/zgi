package agents

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

const agentModelPrecheckAppType = "agent"

type agentModelPrecheckRequest struct {
	Provider string `json:"provider" binding:"required"`
	Model    string `json:"model" binding:"required"`
}

type agentModelPrecheckWarning struct {
	Kind   llmclient.AppModelPrecheckWarningKind  `json:"kind"`
	Reason string                                 `json:"reason,omitempty"`
	Scope  llmclient.AppModelPrecheckWarningScope `json:"scope,omitempty"`
}

type agentModelPrecheckResponse struct {
	Status   llmclient.AppModelPrecheckStatus `json:"status"`
	Warnings []agentModelPrecheckWarning      `json:"warnings"`
}

// PrecheckAgentDraftModel reports risks for the currently selected draft model.
func (h *AgentsHandler) PrecheckAgentDraftModel(c *gin.Context) {
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(c.Param("agent_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	ctx, ok := h.requireAgentManageAccess(c, accountID.String())
	if !ok {
		return
	}

	var req agentModelPrecheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	req.Provider = strings.TrimSpace(req.Provider)
	req.Model = strings.TrimSpace(req.Model)
	if req.Provider == "" || req.Model == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	draft, err := h.appService.GetAgentDraftRuntimeConfig(ctx, agentID.String(), accountID.String())
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	workspaceID, err := uuid.Parse(strings.TrimSpace(draft.WorkspaceID))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	h.respondAgentModelPrecheck(c, &llmclient.AppContext{
		OrganizationID:     organizationID.String(),
		WorkspaceID:        workspaceID.String(),
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              agentID.String(),
		AppType:            agentModelPrecheckAppType,
		AccountID:          accountID.String(),
		ModelUseCase:       agentModelSelectionUseCase,
	}, req.Provider, req.Model)
}

// PrecheckPublishedAgentModel reports risks for a published agent's configured model.
func (h *AgentsHandler) PrecheckPublishedAgentModel(c *gin.Context) {
	if !h.requireWebAppRuntimeAccess(c) {
		return
	}
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	published, err := h.appService.GetPublishedAgentWebAppConfig(c.Request.Context(), c.Param("web_app_id"))
	if err != nil {
		h.failWebAppRuntime(c, err)
		return
	}

	h.respondAgentModelPrecheck(c, &llmclient.AppContext{
		OrganizationID:     strings.TrimSpace(published.OrganizationID),
		WorkspaceID:        strings.TrimSpace(published.WorkspaceID),
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              strings.TrimSpace(published.AgentID),
		AppType:            agentModelPrecheckAppType,
		AccountID:          accountID.String(),
		ModelUseCase:       agentModelSelectionUseCase,
	}, published.Config.ModelProvider, published.Config.Model)
}

func (h *AgentsHandler) respondAgentModelPrecheck(c *gin.Context, appCtx *llmclient.AppContext, provider, model string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if h.modelPrechecker == nil || provider == "" || model == "" {
		response.Success(c, unknownAgentModelPrecheckResponse())
		return
	}

	result, err := h.modelPrechecker.PrecheckAppModels(c.Request.Context(), appCtx, []llmclient.AppModelRef{{
		Provider: provider,
		Model:    model,
	}})
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to precheck agent model", err)
		response.Success(c, unknownAgentModelPrecheckResponse())
		return
	}
	response.Success(c, agentModelPrecheckResult(result))
}

func agentModelPrecheckResult(result *llmclient.AppModelPrecheckResult) agentModelPrecheckResponse {
	if result == nil {
		return unknownAgentModelPrecheckResponse()
	}
	warnings := make([]agentModelPrecheckWarning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, agentModelPrecheckWarning{
			Kind:   warning.Kind,
			Reason: warning.Reason,
			Scope:  warning.Scope,
		})
	}
	return agentModelPrecheckResponse{Status: result.Status, Warnings: warnings}
}

func unknownAgentModelPrecheckResponse() agentModelPrecheckResponse {
	return agentModelPrecheckResponse{
		Status:   llmclient.AppModelPrecheckStatusUnknown,
		Warnings: []agentModelPrecheckWarning{},
	}
}
