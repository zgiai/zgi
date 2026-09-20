package agents

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// ContinueAPIKeyAgentRuntimeTokenBudget resumes an external Agent task that
// paused because its API-provided token budget was exhausted.
func (h *AgentsHandler) ContinueAPIKeyAgentRuntimeTokenBudget(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	runtimeCtx, ok := h.apiKeyAgentContinuationRuntimeContext(c)
	if !ok {
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
	requestID := strings.TrimSpace(c.Param("request_id"))
	if requestID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	var req runtimedto.TaskTokenBudgetContinuationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	req.Surface = "external_api"
	setupAgentSSE(c)
	_, err := h.chatRuntimeService.RunConfiguredTaskTokenBudgetContinuationStream(
		c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, runtimeCtx.RunConfig,
		conversationID, messageID, requestID, req,
		func(event runtimeservice.StreamEvent) error {
			return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
		},
	)
	if err == nil || errors.Is(err, runtimeservice.ErrMessageStopped) || runtimeservice.IsFinalizedStreamError(err) {
		return
	}
	logger.WarnContext(c.Request.Context(), "agent token budget continuation failed",
		"conversation_id", conversationID.String(), "message_id", messageID.String(), "request_id", requestID, err)
	_ = writeAgentSSEEvent(c, "", "error", gin.H{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
		"message":         response.ErrSystemError.Message,
		"code":            response.ErrSystemError.Code,
	})
}
