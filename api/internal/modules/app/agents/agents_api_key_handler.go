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
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

const externalAgentMemoryUserRequiredMessage = "user is required when Agent Memory is enabled"

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
	externalUser := strings.TrimSpace(req.User)

	published, err := h.appService.GetPublishedAgentRuntimeConfig(ctx, agentID.String())
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	if publishedAgentConfigRequiresExternalUser(published.Config) && externalUser == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, externalAgentMemoryUserRequiredMessage)
		return
	}
	if externalUser != "" {
		ctx = context.WithValue(ctx, "external_user", externalUser)
		c.Request = c.Request.WithContext(ctx)
	}

	scope := runtimeservice.Scope{
		OrganizationID:  organizationID,
		AccountID:       accountID,
		WorkspaceID:     &workspaceID,
		SkipAccessCheck: true,
	}
	if externalUser != "" {
		agentMemoryUserID := externalAgentMemoryUserID(workspaceID, agentID, externalUser)
		scope.AgentMemoryUserID = &agentMemoryUserID
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
		Parameters:     nil,
		UseMemory:      false,
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

// StreamAPIKeyAgentRuntimeEvents resumes a recoverable external Agent stream.
func (h *AgentsHandler) StreamAPIKeyAgentRuntimeEvents(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	runtimeCtx, ok := h.apiKeyAgentRuntimeContext(c)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(c.Param("conversation_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Query("message_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, err := h.chatRuntimeService.GetConversationByCaller(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID); err != nil {
		h.failRuntime(c, err)
		return
	}

	setupAgentSSE(c)
	err = h.chatRuntimeService.StreamConversationEvents(c.Request.Context(), runtimeCtx.Scope, conversationID, messageID, c.Query("after_id"), func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		_ = writeAgentSSEEvent(c, "", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"message":         err.Error(),
		})
	}
}

// ContinueAPIKeyAgentRuntimeWorkflowContinuation resumes an Agent workflow pause for an external API caller.
func (h *AgentsHandler) ContinueAPIKeyAgentRuntimeWorkflowContinuation(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	runtimeCtx, ok := h.apiKeyAgentContinuationRuntimeContext(c)
	if !ok {
		return
	}
	h.continueRuntimeWorkflowApproval(c, runtimeCtx)
}

func (h *AgentsHandler) apiKeyAgentRuntimeContext(c *gin.Context) (agentRuntimeContext, bool) {
	agentID, err := uuid.Parse(strings.TrimSpace(c.GetString("agent_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return agentRuntimeContext{}, false
	}
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return agentRuntimeContext{}, false
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return agentRuntimeContext{}, false
	}
	workspaceID, err := uuid.Parse(strings.TrimSpace(util.GetWorkspaceID(c)))
	if err != nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return agentRuntimeContext{}, false
	}

	scope := runtimeservice.Scope{
		OrganizationID:  organizationID,
		AccountID:       accountID,
		WorkspaceID:     &workspaceID,
		SkipAccessCheck: true,
	}
	caller := runtimeservice.Caller{
		Type:   runtimemodel.ConversationCallerAgent,
		ID:     &agentID,
		Source: runtimemodel.ConversationSourceExternalAPI,
	}
	return agentRuntimeContext{Scope: scope, Caller: caller}, true
}

func (h *AgentsHandler) apiKeyAgentContinuationRuntimeContext(c *gin.Context) (agentRuntimeContext, bool) {
	runtimeCtx, ok := h.apiKeyAgentRuntimeContext(c)
	if !ok {
		return agentRuntimeContext{}, false
	}
	if runtimeCtx.Caller.ID == nil {
		response.Fail(c, response.ErrInvalidParam)
		return agentRuntimeContext{}, false
	}
	published, err := h.appService.GetPublishedAgentRuntimeConfig(c.Request.Context(), runtimeCtx.Caller.ID.String())
	if err != nil {
		h.failRuntime(c, err)
		return agentRuntimeContext{}, false
	}
	runConfig, err := h.agentRunConfig(
		c.Request.Context(),
		runtimeCtx.Scope,
		published.AgentID,
		"agent.published."+published.Version,
		published.Config,
		"end_user",
	)
	if err != nil {
		h.failRuntime(c, err)
		return agentRuntimeContext{}, false
	}
	runtimeCtx.RunConfig = runConfig
	return runtimeCtx, true
}

func publishedAgentConfigRequiresExternalUser(cfg dto.AgentConfigResponse) bool {
	if !cfg.AgentMemoryEnabled {
		return false
	}
	for _, slot := range cfg.AgentMemorySlots {
		if slot.Enabled && strings.TrimSpace(slot.Key) != "" {
			return true
		}
	}
	return false
}

func externalAgentMemoryUserID(workspaceID, agentID uuid.UUID, externalUser string) uuid.UUID {
	seed := strings.Join([]string{
		"zgi",
		"external-agent-memory",
		workspaceID.String(),
		agentID.String(),
		strings.TrimSpace(externalUser),
	}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed))
}
