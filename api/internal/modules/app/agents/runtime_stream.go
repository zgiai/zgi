package agents

import (
	"errors"
	"github.com/gin-gonic/gin"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

func (h *AgentsHandler) regenerateRuntimeMessage(c *gin.Context, runtimeCtx agentRuntimeContext) {
	messageID, ok := uuidParam(c, "message_id")
	if !ok {
		return
	}
	var req runtimedto.RegenerateMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	prepared, err := h.chatRuntimeService.PrepareConfiguredRootRegeneration(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, runtimeCtx.RunConfig, messageID, req)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	h.runPreparedAgentStream(c, prepared)
}

func (h *AgentsHandler) runPreparedAgentStream(c *gin.Context, prepared *runtimeservice.PreparedChat) {
	setupAgentSSE(c)
	_ = writeAgentSSE(c, "message_start", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"parent_id":       uuidPtrToString(prepared.Message.ParentID),
		"title":           prepared.Conversation.Title,
		"model":           prepared.Message.ModelName,
		"replace":         prepared.ReplaceRoot,
		"created_at":      prepared.Message.CreatedAt.Unix(),
	})
	result, err := h.chatRuntimeService.RunPreparedStream(c.Request.Context(), prepared, func(chunk string) error {
		return writeAgentSSE(c, "message", gin.H{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"answer":          chunk,
		})
	}, func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		status := runtimemodel.MessageStatusError
		if errors.Is(err, runtimeservice.ErrMessageStopped) {
			status = runtimemodel.MessageStatusStopped
		}
		if runtimeservice.IsFinalizedStreamError(err) {
			return
		}
		_ = writeAgentSSE(c, "error", runtimeservice.BuildStreamErrorPayload(prepared, err))
		_ = writeAgentSSE(c, "message_end", gin.H{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"status":          status,
			"metadata":        gin.H{},
		})
		return
	}
	if agentWorkflowContinuationWaiting(result.Metadata) {
		return
	}
	_ = writeAgentSSE(c, "message_end", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          runtimemodel.MessageStatusCompleted,
		"metadata": gin.H{
			"usage": result.Metadata["usage"],
		},
	})
}
