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

func (h *Handler) ContinueUserInput(c *gin.Context) {
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

	setupSSE(c)
	_, err = h.service.RunUserInputContinuationStream(c.Request.Context(), scope, conversationID, messageID, requestID, req, func(event runtimeservice.StreamEvent) error {
		return writeStreamEvent(c, event)
	})
	if err != nil {
		if errors.Is(err, runtimeservice.ErrMessageStopped) || runtimeservice.IsFinalizedStreamError(err) {
			return
		}
		logger.WarnContext(c.Request.Context(), "aichat user input continuation failed", "conversation_id", conversationID.String(), "message_id", messageID.String(), "request_id", requestID, err)
		_ = writeSSEEvent(c, "", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"message":         err.Error(),
		})
	}
}
