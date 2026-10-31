package agents

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// ContinueAgentRuntimeToolGovernance is registered only on the authenticated
// console route. The service owns scope validation, frozen-call execution and
// duplicate continuation recovery; no original Shell job is replayed here.
func (h *AgentsHandler) ContinueAgentRuntimeToolGovernance(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.continueConsoleRuntimeToolGovernance(c, runtimeCtx)
}

func (h *AgentsHandler) continueConsoleRuntimeToolGovernance(c *gin.Context, runtimeCtx agentRuntimeContext) {
	if runtimeCtx.Caller.Type != runtimemodel.ConversationCallerAgent || runtimeCtx.Caller.Source != runtimemodel.ConversationSourceConsole || runtimeCtx.Caller.ID == nil {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	h.continueRuntimeToolGovernance(c, runtimeCtx, nil)
}

func (h *AgentsHandler) continueRuntimeToolGovernance(c *gin.Context, runtimeCtx agentRuntimeContext, request *runtimedto.ToolGovernanceDecisionRequest) {
	if runtimeCtx.Caller.Type != runtimemodel.ConversationCallerAgent || runtimeCtx.Caller.ID == nil ||
		(runtimeCtx.Caller.Source != "console" && runtimeCtx.Caller.Source != "webapp" && runtimeCtx.Caller.Source != "external-api") ||
		runtimeCtx.Scope.RuntimeWorkspaceID != nil || runtimeCtx.Scope.RuntimeSubjectID != nil {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
		return
	}
	messageID, ok := uuidParam(c, "message_id")
	if !ok {
		return
	}
	correlationID := strings.TrimSpace(c.Param("correlation_id"))
	var req runtimedto.ToolGovernanceDecisionRequest
	if request != nil {
		req = *request
	} else if c.ShouldBindJSON(&req) != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if correlationID == "" || len(correlationID) > 256 || (req.Action != "approve" && req.Action != "reject") {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	// Rejection remains available when runtime admission is revoked, so an old
	// pending decision can be closed without granting execution permissions.
	if req.Action == "approve" {
		if runtimeCtx.ExecutionError != nil {
			h.failRuntime(c, runtimeCtx.ExecutionError)
			return
		}
		prepared := &runtimeservice.PreparedChat{
			Scope: runtimeCtx.Scope, Caller: runtimeCtx.Caller,
			Conversation: &runtimemodel.Conversation{ID: conversationID},
			Message:      &runtimemodel.Message{ID: messageID},
		}
		available := agentExecutionEnvBool("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", false) && runtimeCtx.RunConfig.RuntimeToolFactory != nil
		if runtimeCtx.Caller.Source == "console" {
			available = consolePlatformApprovalAvailable(runtimeCtx.RunConfig, prepared)
		}
		if !available && runtimeCtx.Caller.Source == "console" {
			// A market write waits for a person, not for a published platform surface.
			if market, ok := h.chatRuntimeService.(interface {
				PendingMarketToolApproval(context.Context, runtimeservice.Scope, runtimeservice.Caller, uuid.UUID, uuid.UUID, string) bool
			}); ok {
				available = market.PendingMarketToolApproval(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID, messageID, correlationID)
			}
		}
		if !available {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}
	defer setupAgentSSE(c).Close()
	_, err := h.chatRuntimeService.RunConfiguredToolGovernanceDecisionStream(
		c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, runtimeCtx.RunConfig,
		conversationID, messageID, correlationID, req,
		func(event runtimeservice.StreamEvent) error {
			return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
		},
	)
	if err == nil || errors.Is(err, runtimeservice.ErrMessageStopped) || runtimeservice.IsFinalizedStreamError(err) {
		return
	}
	logger.WarnContext(c.Request.Context(), "agent tool governance continuation failed",
		"conversation_id", conversationID.String(), "message_id", messageID.String(), err)
	failure := response.ErrSystemError
	appCode := appcatalog.CodeInternal
	if errors.Is(err, runtimeservice.ErrNotFound) {
		failure = response.ErrNotFound
		appCode = appcatalog.CodeResourceNotFound
	}
	message := failure.Message
	if h.appErrorProjector != nil {
		locale, _ := appcatalog.ParseLocale(c.GetHeader("Accept-Language"))
		message = h.appErrorProjector.Project(apperror.Wrap(err, appCode), locale).Presentation.Message
	}
	_ = writeAgentSSEEvent(c, "", "error", gin.H{
		"conversation_id": conversationID.String(), "message_id": messageID.String(),
		"message": message, "code": failure.Code,
	})
}

// ContinueWebAppAgentRuntimeToolGovernance authenticates the login separately
// from Linux admission, so rejection remains possible after compute is disabled.
func (h *AgentsHandler) ContinueWebAppAgentRuntimeToolGovernance(c *gin.Context) {
	if !c.GetBool("is_authenticated") {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req runtimedto.ToolGovernanceDecisionRequest
	if c.ShouldBindJSON(&req) != nil || (req.Action != "approve" && req.Action != "reject") {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	var runtimeCtx agentRuntimeContext
	if req.Action == "approve" {
		var ok bool
		runtimeCtx, ok = h.webAppAgentRuntimeContext(c)
		if !ok {
			return
		}
		if !h.restorePublishedPlatformApproval(c, &runtimeCtx) {
			return
		}
	} else {
		scope, app, webApp, _, ok := h.webAppAgentRuntimeAccess(c)
		if !ok {
			return
		}
		runtimeCtx = agentRuntimeContext{Scope: scope, Caller: runtimeservice.Caller{Type: "agent", ID: &app, Source: "webapp", SourceWebAppID: &webApp}}
	}
	h.continueRuntimeToolGovernance(c, runtimeCtx, &req)
}

// ContinueAPIKeyAgentRuntimeToolGovernance uses the trusted backend's existing
// final-user contract. Runtime Workspace credentials never enter this route.
func (h *AgentsHandler) ContinueAPIKeyAgentRuntimeToolGovernance(c *gin.Context) {
	if _, exists := c.Get("runtime_workspace_principal"); exists || c.GetString("runtime_workspace_id") != "" || c.GetString("runtime_subject_id") != "" {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	var req runtimedto.ToolGovernanceDecisionRequest
	if c.ShouldBindJSON(&req) != nil || (req.Action != "approve" && req.Action != "reject") {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	var runtimeCtx agentRuntimeContext
	if req.Action == "approve" {
		var ok bool
		runtimeCtx, ok = h.apiKeyAgentContinuationRuntimeContext(c)
		if !ok {
			return
		}
		if !h.restorePublishedPlatformApproval(c, &runtimeCtx) {
			return
		}
	} else {
		var ok bool
		runtimeCtx, ok = h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
		if !ok || runtimeCtx.Caller.ID == nil {
			return
		}
		published, err := h.appService.GetPublishedAgentRuntimeConfig(c.Request.Context(), runtimeCtx.Caller.ID.String())
		if err != nil {
			h.failRuntime(c, err)
			return
		}
		if published == nil {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
		runtimeCtx.Scope, err = scopeWithAgentResourceWorkspace(runtimeCtx.Scope, published.WorkspaceID)
		if err != nil {
			h.failRuntime(c, err)
			return
		}
	}
	h.continueRuntimeToolGovernance(c, runtimeCtx, &req)
}

func consolePlatformApprovalAvailable(config runtimeservice.RunConfig, prepared *runtimeservice.PreparedChat) bool {
	if !agentExecutionEnvBool("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", false) || config.RuntimeToolFactory == nil {
		return false
	}
	for _, tool := range config.RuntimeToolFactory(prepared) {
		if tool.PlatformInteractiveApproval && tool.BindPlatform != nil && tool.Definition.Function.Name == "shell_exec" {
			return true
		}
	}
	return false
}
