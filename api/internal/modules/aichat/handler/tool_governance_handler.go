package handler

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

func (h *Handler) SubmitToolGovernanceDecision(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	correlationID := strings.TrimSpace(c.Param("correlation_id"))
	if correlationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req runtimedto.ToolGovernanceDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.service.SubmitToolGovernanceDecision(c.Request.Context(), scope, conversationID, messageID, correlationID, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) ContinueToolGovernanceDecision(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	correlationID := strings.TrimSpace(c.Param("correlation_id"))
	if correlationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req runtimedto.ToolGovernanceDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	setupSSE(c)
	_, err = h.service.RunToolGovernanceDecisionStream(c.Request.Context(), scope, conversationID, messageID, correlationID, req, func(event runtimeservice.StreamEvent) error {
		return writeSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		if errors.Is(err, runtimeservice.ErrMessageStopped) || runtimeservice.IsFinalizedStreamError(err) {
			return
		}
		logger.WarnContext(c.Request.Context(), "aichat tool governance continuation failed", "conversation_id", conversationID.String(), "message_id", messageID.String(), "correlation_id", correlationID, err)
		_ = writeSSEEvent(c, "", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"message":         err.Error(),
		})
	}
}

func (h *Handler) ContinueClientAction(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	actionID := strings.TrimSpace(c.Param("action_id"))
	if actionID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req runtimedto.ClientActionResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	setupSSE(c)
	_, err = h.service.RunClientActionContinuationStream(c.Request.Context(), scope, conversationID, messageID, actionID, req, func(event runtimeservice.StreamEvent) error {
		return writeSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		if errors.Is(err, runtimeservice.ErrMessageStopped) || runtimeservice.IsFinalizedStreamError(err) {
			return
		}
		logger.WarnContext(c.Request.Context(), "aichat client action continuation failed", "conversation_id", conversationID.String(), "message_id", messageID.String(), "action_id", actionID, err)
		_ = writeSSEEvent(c, "", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"message":         err.Error(),
		})
	}
}
