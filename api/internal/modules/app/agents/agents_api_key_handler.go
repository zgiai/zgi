package agents

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
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

type apiKeyAgentRegenerateRequest struct {
	Query *string `json:"query,omitempty"`
}

type apiKeyAgentRuntimeConfigResponse struct {
	AgentID              string   `json:"agent_id"`
	WebAppID             string   `json:"web_app_id"`
	AgentType            string   `json:"agent_type"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Icon                 string   `json:"icon"`
	IconType             string   `json:"icon_type"`
	IconURL              string   `json:"icon_url"`
	HomeTitle            string   `json:"home_title"`
	OpeningStatement     string   `json:"opening_statement"`
	InputPlaceholder     string   `json:"input_placeholder"`
	ThemeColor           string   `json:"theme_color"`
	SuggestedQuestions   []string `json:"suggested_questions"`
	FileUploadEnabled    bool     `json:"file_upload_enabled"`
	SupportsVision       bool     `json:"supports_vision"`
	AgentMemoryEnabled   bool     `json:"agent_memory_enabled"`
	MemoryRequiresUser   bool     `json:"memory_requires_user"`
	ExternalUserRequired bool     `json:"external_user_required"`
	Version              string   `json:"version"`
	VersionUUID          string   `json:"version_uuid"`
}

// GetAPIKeyAgentRuntimeConfig returns the public, published presentation
// configuration needed by a server-side external Agent integration.
func (h *AgentsHandler) GetAPIKeyAgentRuntimeConfig(c *gin.Context) {
	runtimeCtx, ok := h.apiKeyAgentRuntimeContext(c)
	if !ok {
		return
	}
	if runtimeCtx.Caller.ID == nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	published, err := h.appService.GetPublishedAgentRuntimeConfig(c.Request.Context(), runtimeCtx.Caller.ID.String())
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, apiKeyAgentRuntimeConfigResponse{
		AgentID:              published.AgentID,
		WebAppID:             published.WebAppID,
		AgentType:            published.AgentType,
		Name:                 published.Name,
		Description:          published.Description,
		Icon:                 published.Icon,
		IconType:             published.IconType,
		IconURL:              published.IconURL,
		HomeTitle:            published.Config.HomeTitle,
		OpeningStatement:     published.Config.OpeningStatement,
		InputPlaceholder:     published.Config.InputPlaceholder,
		ThemeColor:           published.Config.ThemeColor,
		SuggestedQuestions:   published.Config.SuggestedQuestions,
		FileUploadEnabled:    published.Config.FileUpload,
		SupportsVision:       published.Config.SupportsVision,
		AgentMemoryEnabled:   published.Config.AgentMemoryEnabled,
		MemoryRequiresUser:   publishedAgentConfigRequiresExternalUser(published.Config),
		ExternalUserRequired: true,
		Version:              published.Version,
		VersionUUID:          published.VersionUUID,
	})
}

// ChatAPIKeyAgent handles external API-key authenticated chat for published Agent runtime.
func (h *AgentsHandler) ChatAPIKeyAgent(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
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

	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, req.User)
	if !ok {
		return
	}
	if runtimeCtx.Caller.ID == nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	agentID := *runtimeCtx.Caller.ID
	ctx := c.Request.Context()

	published, err := h.appService.GetPublishedAgentRuntimeConfig(ctx, agentID.String())
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	scope := runtimeCtx.Scope
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
		runtimeCtx.Caller,
		runConfig,
		chatReq,
	)
	if err != nil {
		h.failRuntime(c, err)
		return
	}

	setupPreparedAgentSSE(c, prepared)
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

func externalAgentInvocationContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, "invoke_from", runtimemodel.ConversationSourceExternalAPI)
}

// StreamAPIKeyAgentRuntimeEvents resumes a recoverable external Agent stream.
func (h *AgentsHandler) StreamAPIKeyAgentRuntimeEvents(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
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
	err = h.chatRuntimeService.StreamConversationEventsForCaller(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID, messageID, c.Query("after_id"), func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		logger.WarnContext(c.Request.Context(), "external Agent event stream failed", "conversation_id", conversationID.String(), "message_id", messageID.String(), err)
		_ = writeAgentSSEEvent(c, "", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"message":         response.ErrSystemError.Message,
		})
	}
}

// StopAPIKeyAgentRuntimeConversation cancels the active model, Skill, and
// workflow-continuation execution for an external API caller. The caller scope
// is derived exclusively from the validated API key context so one key cannot
// stop another Agent's conversation.
func (h *AgentsHandler) StopAPIKeyAgentRuntimeConversation(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
	if !ok {
		return
	}
	h.stopRuntimeConversation(c, runtimeCtx)
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

// ContinueAPIKeyAgentRuntimeUserInput resumes a Skill ask-user pause for an
// external API caller. This is distinct from workflow approval/question nodes.
func (h *AgentsHandler) ContinueAPIKeyAgentRuntimeUserInput(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	runtimeCtx, ok := h.apiKeyAgentContinuationRuntimeContext(c)
	if !ok {
		return
	}
	h.continueRuntimeUserInput(c, runtimeCtx)
}

func (h *AgentsHandler) ListAPIKeyAgentRuntimeConversations(c *gin.Context) {
	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
	if !ok {
		return
	}
	h.listRuntimeConversations(c, runtimeCtx)
}

func (h *AgentsHandler) SearchAPIKeyAgentRuntimeConversations(c *gin.Context) {
	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
	if !ok {
		return
	}
	h.searchRuntimeConversations(c, runtimeCtx)
}

func (h *AgentsHandler) GetAPIKeyAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
	if !ok {
		return
	}
	h.getRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) UpdateAPIKeyAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
	if !ok {
		return
	}
	h.updateRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) DeleteAPIKeyAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
	if !ok {
		return
	}
	h.deleteRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) ListAPIKeyAgentRuntimeMessages(c *gin.Context) {
	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
	if !ok {
		return
	}
	h.listRuntimeMessages(c, runtimeCtx)
}

// RegenerateAPIKeyAgentRuntimeMessage preserves the published Agent's model
// and parameters; external callers may only replace the original query.
func (h *AgentsHandler) RegenerateAPIKeyAgentRuntimeMessage(c *gin.Context) {
	runtimeCtx, ok := h.apiKeyAgentContinuationRuntimeContext(c)
	if !ok {
		return
	}
	messageID, ok := uuidParam(c, "message_id")
	if !ok {
		return
	}
	var req apiKeyAgentRegenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	prepared, err := h.chatRuntimeService.PrepareConfiguredRootRegeneration(
		c.Request.Context(),
		runtimeCtx.Scope,
		runtimeCtx.Caller,
		runtimeCtx.RunConfig,
		messageID,
		runtimedto.RegenerateMessageRequest{Query: req.Query},
	)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	h.runPreparedAgentStream(c, prepared)
}

func (h *AgentsHandler) apiKeyAgentRuntimeContext(c *gin.Context) (agentRuntimeContext, bool) {
	c.Set(externalAgentAPIContextKey, true)
	c.Request = c.Request.WithContext(externalAgentInvocationContext(c.Request.Context()))
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
	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
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

func (h *AgentsHandler) apiKeyAgentUserRuntimeContext(c *gin.Context, legacyUser string) (agentRuntimeContext, bool) {
	runtimeCtx, ok := h.apiKeyAgentRuntimeContext(c)
	if !ok {
		return agentRuntimeContext{}, false
	}
	if runtimeCtx.Scope.WorkspaceID == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return agentRuntimeContext{}, false
	}
	if runtimeCtx.Caller.ID == nil {
		response.Fail(c, response.ErrInvalidParam)
		return agentRuntimeContext{}, false
	}
	externalUser, ok := externalUserFromRequest(c, legacyUser)
	if !ok {
		return agentRuntimeContext{}, false
	}
	externalUserID := DeriveExternalUserID(*runtimeCtx.Scope.WorkspaceID, *runtimeCtx.Caller.ID, externalUser)
	runtimeCtx.Scope.ExternalUserID = uuidPtr(externalUserID)
	runtimeCtx.Scope.AgentMemoryUserID = uuidPtr(externalUserID)
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
