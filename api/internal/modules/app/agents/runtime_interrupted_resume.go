package agents

import (
	"errors"

	"github.com/gin-gonic/gin"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// agentInterruptedTurnNotResumableCode marks a resume request for a turn that
// cannot be resumed: it did not stop with its process, it was resumed before,
// the conversation moved on, or a call that may have changed something has no
// recorded result.
const agentInterruptedTurnNotResumableCode = "interrupted_turn_not_resumable"

// ResumeAgentRuntimeInterruptedTurn resumes a console Agent turn whose process
// stopped in the middle of it (ADR 0042).
func (h *AgentsHandler) ResumeAgentRuntimeInterruptedTurn(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.resumeRuntimeInterruptedTurn(c, runtimeCtx)
}

// ResumeWebAppAgentRuntimeInterruptedTurn resumes a web app Agent turn whose
// process stopped in the middle of it (ADR 0042).
func (h *AgentsHandler) ResumeWebAppAgentRuntimeInterruptedTurn(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.resumeRuntimeInterruptedTurn(c, runtimeCtx)
}

// ResumeAPIKeyAgentRuntimeInterruptedTurn resumes an external Agent turn whose
// process stopped in the middle of it (ADR 0042).
func (h *AgentsHandler) ResumeAPIKeyAgentRuntimeInterruptedTurn(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	runtimeCtx, ok := h.apiKeyAgentContinuationRuntimeContext(c)
	if !ok {
		return
	}
	h.resumeRuntimeInterruptedTurn(c, runtimeCtx)
}

func (h *AgentsHandler) resumeRuntimeInterruptedTurn(c *gin.Context, runtimeCtx agentRuntimeContext) {
	if runtimeCtx.ExecutionError != nil {
		h.failRuntime(c, runtimeCtx.ExecutionError)
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

	setupAgentSSE(c)
	_, err := h.chatRuntimeService.RunConfiguredInterruptedTurnResume(
		c.Request.Context(),
		runtimeCtx.Scope,
		runtimeCtx.Caller,
		runtimeCtx.RunConfig,
		conversationID,
		messageID,
		func(event runtimeservice.StreamEvent) error {
			return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
		},
	)
	if err == nil || errors.Is(err, runtimeservice.ErrMessageStopped) || runtimeservice.IsFinalizedStreamError(err) {
		return
	}
	if errors.Is(err, runtimeservice.ErrInterruptedTurnNotResumable) {
		_ = writeAgentSSEEvent(c, "", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"message":         err.Error(),
			"code":            agentInterruptedTurnNotResumableCode,
			"error_code":      agentInterruptedTurnNotResumableCode,
		})
		return
	}
	logger.WarnContext(c.Request.Context(), "agent interrupted turn resume failed",
		"conversation_id", conversationID.String(),
		"message_id", messageID.String(),
		err,
	)
	_ = writeAgentSSEEvent(c, "", "error", gin.H{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
		"message":         response.ErrSystemError.Message,
		"code":            response.ErrSystemError.Code,
	})
}
