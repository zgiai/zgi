package agents

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
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
	if correlationID == "" || len(correlationID) > 256 || c.ShouldBindJSON(&req) != nil || (req.Action != "approve" && req.Action != "reject") {
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
		if !consolePlatformApprovalAvailable(runtimeCtx.RunConfig, prepared) {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}
	setupAgentSSE(c)
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
	_ = writeAgentSSEEvent(c, "", "error", gin.H{
		"conversation_id": conversationID.String(), "message_id": messageID.String(),
		"message": response.ErrSystemError.Message, "code": response.ErrSystemError.Code,
	})
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
