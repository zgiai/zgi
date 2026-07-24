package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	workChatModelPrecheckRoute   = "/work-chat/models/precheck"
	workChatPrecheckAppType      = "aichat"
	workChatPrecheckModelUseCase = "text-chat"
)

type workChatModelPrecheckRequest struct {
	Provider string `json:"provider" binding:"required"`
	Model    string `json:"model" binding:"required"`
}

type workChatModelPrecheckWarning struct {
	Kind   llmclient.AppModelPrecheckWarningKind  `json:"kind"`
	Reason string                                 `json:"reason,omitempty"`
	Scope  llmclient.AppModelPrecheckWarningScope `json:"scope,omitempty"`
}

type workChatModelPrecheckResponse struct {
	Status   llmclient.AppModelPrecheckStatus `json:"status"`
	Warnings []workChatModelPrecheckWarning   `json:"warnings"`
}

// PrecheckWorkChatModel reports model-level risks without blocking the selected model.
func (h *Handler) PrecheckWorkChatModel(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}

	var req workChatModelPrecheckRequest
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

	if h.modelPrechecker == nil {
		response.Success(c, unknownWorkChatModelPrecheckResponse())
		return
	}

	appCtx := &llmclient.AppContext{
		OrganizationID:     scope.OrganizationID.String(),
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              uuid.Nil.String(),
		AppType:            workChatPrecheckAppType,
		AccountID:          scope.AccountID.String(),
		ModelUseCase:       workChatPrecheckModelUseCase,
	}
	if scope.WorkspaceID != nil {
		appCtx.WorkspaceID = scope.WorkspaceID.String()
	}

	result, err := h.modelPrechecker.PrecheckAppModels(c.Request.Context(), appCtx, []llmclient.AppModelRef{{
		Provider: req.Provider,
		Model:    req.Model,
	}})
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to precheck work chat model", err)
		response.Success(c, unknownWorkChatModelPrecheckResponse())
		return
	}

	response.Success(c, workChatModelPrecheckResult(result))
}

func workChatModelPrecheckResult(result *llmclient.AppModelPrecheckResult) workChatModelPrecheckResponse {
	if result == nil {
		return unknownWorkChatModelPrecheckResponse()
	}

	warnings := make([]workChatModelPrecheckWarning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, workChatModelPrecheckWarning{
			Kind:   warning.Kind,
			Reason: warning.Reason,
			Scope:  warning.Scope,
		})
	}
	return workChatModelPrecheckResponse{Status: result.Status, Warnings: warnings}
}

func unknownWorkChatModelPrecheckResponse() workChatModelPrecheckResponse {
	return workChatModelPrecheckResponse{
		Status:   llmclient.AppModelPrecheckStatusUnknown,
		Warnings: []workChatModelPrecheckWarning{},
	}
}
