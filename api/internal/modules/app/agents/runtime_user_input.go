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

func (h *AgentsHandler) ContinueAgentRuntimeUserInput(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.continueRuntimeUserInput(c, runtimeCtx)
}

func (h *AgentsHandler) ContinueWebAppAgentRuntimeUserInput(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.continueRuntimeUserInput(c, runtimeCtx)
}

func (h *AgentsHandler) continueRuntimeUserInput(c *gin.Context, runtimeCtx agentRuntimeContext) {
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

	var req runtimedto.UserInputContinuationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	setupAgentSSE(c)
	_, err := h.chatRuntimeService.RunConfiguredUserInputContinuationStream(
		c.Request.Context(),
		runtimeCtx.Scope,
		runtimeCtx.Caller,
		runtimeCtx.RunConfig,
		conversationID,
		messageID,
		requestID,
		req,
		func(event runtimeservice.StreamEvent) error {
			return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
		},
	)
	if err == nil || errors.Is(err, runtimeservice.ErrMessageStopped) || runtimeservice.IsFinalizedStreamError(err) {
		return
	}
	logger.WarnContext(c.Request.Context(), "agent user input continuation failed",
		"conversation_id", conversationID.String(),
		"message_id", messageID.String(),
		"request_id", requestID,
		err,
	)
	_ = writeAgentSSEEvent(c, "", "error", gin.H{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
		"message":         err.Error(),
	})
}
