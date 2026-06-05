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
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

type apiKeyAgentChatRequest struct {
	ConversationID string                 `json:"conversation_id,omitempty"`
	ParentID       string                 `json:"parent_id,omitempty"`
	Query          string                 `json:"query" binding:"required"`
	FileIDs        []string               `json:"file_ids,omitempty"`
	ResponseMode   string                 `json:"response_mode,omitempty"`
	Parameters     map[string]interface{} `json:"parameters,omitempty"`
	UseMemory      bool                   `json:"use_memory,omitempty"`
	User           string                 `json:"user,omitempty"`
}

// ChatAPIKeyAgent handles external API-key authenticated chat for published Agent runtime.
func (h *AgentsHandler) ChatAPIKeyAgent(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	agentID, err := uuid.Parse(strings.TrimSpace(c.GetString("agent_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}
	workspaceID, err := uuid.Parse(strings.TrimSpace(util.GetWorkspaceID(c)))
	if err != nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	var req apiKeyAgentChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	responseMode := strings.TrimSpace(req.ResponseMode)
	if responseMode == "" {
		responseMode = "streaming"
	}
	if responseMode != "streaming" {
		response.FailWithMessage(c, response.ErrInvalidParam, "response_mode must be streaming")
		return
	}

	ctx := c.Request.Context()
	if user := strings.TrimSpace(req.User); user != "" {
		ctx = context.WithValue(ctx, "external_user", user)
		c.Request = c.Request.WithContext(ctx)
	}

	published, err := h.appService.GetPublishedAgentRuntimeConfig(ctx, agentID.String())
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}

	scope := runtimeservice.Scope{
		OrganizationID:  organizationID,
		AccountID:       accountID,
		WorkspaceID:     &workspaceID,
		SkipAccessCheck: true,
	}
	runConfig, err := h.agentRunConfig(ctx, scope, published.AgentID, "agent.published."+published.Version, published.Config, "end_user")
	if err != nil {
		h.failRuntime(c, err)
		return
	}

	chatReq := runtimedto.ChatRequest{
		ConversationID: req.ConversationID,
		ParentID:       req.ParentID,
		Query:          req.Query,
		FileIDs:        req.FileIDs,
		ResponseMode:   "streaming",
		Parameters:     req.Parameters,
		UseMemory:      req.UseMemory,
	}
	prepared, err := h.chatRuntimeService.PrepareConfiguredChat(
		ctx,
		scope,
		runtimeservice.Caller{
			Type:   runtimemodel.ConversationCallerAgent,
			ID:     &agentID,
			Source: runtimemodel.ConversationSourceExternalAPI,
		},
		runConfig,
		chatReq,
	)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}

	setupAgentSSE(c)
	_ = writeAgentSSE(c, "message_start", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"parent_id":       uuidPtrToString(prepared.Message.ParentID),
		"title":           prepared.Conversation.Title,
		"model":           prepared.Message.ModelName,
		"created_at":      prepared.Message.CreatedAt.Unix(),
	})
	result, err := h.chatRuntimeService.RunPreparedStream(c.Request.Context(), prepared, func(chunk string) error {
		return writeAgentSSE(c, "message", gin.H{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"answer":          chunk,
		})
	}, func(event runtimeservice.StreamEvent) error {
		if event.EventType != "message" && event.EventType != "error" {
			return nil
		}
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		status := runtimemodel.MessageStatusError
		if errors.Is(err, runtimeservice.ErrMessageStopped) {
			status = runtimemodel.MessageStatusStopped
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
	_ = writeAgentSSE(c, "message_end", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          runtimemodel.MessageStatusCompleted,
		"metadata": gin.H{
			"usage": result.Metadata["usage"],
		},
	})
}
