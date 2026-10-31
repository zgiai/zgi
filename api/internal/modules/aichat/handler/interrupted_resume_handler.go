package handler

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// interruptedTurnNotResumableCode marks a resume request for a turn that
// cannot be resumed: it did not stop with its process, it was resumed before,
// the conversation moved on, or a call that may have changed something has no
// recorded result. Agents report it with the same code.
const interruptedTurnNotResumableCode = "interrupted_turn_not_resumable"

// ResumeInterruptedTurn resumes an AI chat turn whose server process stopped
// in the middle of it (ADR 0042). The runtime decides whether it can resume.
func (h *Handler) ResumeInterruptedTurn(c *gin.Context) {
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

	defer setupSSE(c).Close()
	_, err = h.service.RunInterruptedTurnResumeStream(c.Request.Context(), scope, conversationID, messageID, func(event runtimeservice.StreamEvent) error {
		return writeStreamEvent(c, event)
	})
	if err == nil || errors.Is(err, runtimeservice.ErrMessageStopped) || runtimeservice.IsFinalizedStreamError(err) {
		return
	}
	if errors.Is(err, runtimeservice.ErrInterruptedTurnNotResumable) {
		_ = writeSSEEvent(c, "", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"message":         err.Error(),
			"code":            interruptedTurnNotResumableCode,
			"error_code":      interruptedTurnNotResumableCode,
		})
		return
	}
	logger.WarnContext(c.Request.Context(), "aichat interrupted turn resume failed", "conversation_id", conversationID.String(), "message_id", messageID.String(), err)
	_ = writeSSEEvent(c, "", "error", gin.H{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
		"message":         response.ErrSystemError.Message,
		"code":            response.ErrSystemError.Code,
	})
}
