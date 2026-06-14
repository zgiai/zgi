package agents

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"strconv"
	"strings"
)

type agentRuntimeContext struct {
	Scope     runtimeservice.Scope
	Caller    runtimeservice.Caller
	RunConfig runtimeservice.RunConfig
}

func (h *AgentsHandler) ListAgentRuntimeConversations(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.listRuntimeConversations(c, runtimeCtx)
}

func (h *AgentsHandler) GetAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.getRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) UpdateAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.updateRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) DeleteAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.deleteRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) ListAgentRuntimeMessages(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.listRuntimeMessages(c, runtimeCtx)
}

func (h *AgentsHandler) StopAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.stopRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) StreamAgentRuntimeEvents(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.streamRuntimeEvents(c, runtimeCtx)
}

func (h *AgentsHandler) RegenerateAgentRuntimeMessage(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.regenerateRuntimeMessage(c, runtimeCtx)
}

func (h *AgentsHandler) ListWebAppAgentRuntimeConversations(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.listRuntimeConversations(c, runtimeCtx)
}

func (h *AgentsHandler) GetWebAppAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.getRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) UpdateWebAppAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.updateRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) DeleteWebAppAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.deleteRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) ListWebAppAgentRuntimeMessages(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.listRuntimeMessages(c, runtimeCtx)
}

func (h *AgentsHandler) StopWebAppAgentRuntimeConversation(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.stopRuntimeConversation(c, runtimeCtx)
}

func (h *AgentsHandler) StreamWebAppAgentRuntimeEvents(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.streamRuntimeEvents(c, runtimeCtx)
}

func (h *AgentsHandler) RegenerateWebAppAgentRuntimeMessage(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.regenerateRuntimeMessage(c, runtimeCtx)
}

func (h *AgentsHandler) ContinueWebAppAgentRuntimeWorkflowApproval(c *gin.Context) {
	runtimeCtx, ok := h.webAppAgentRuntimeContext(c)
	if !ok {
		return
	}
	h.continueRuntimeWorkflowApproval(c, runtimeCtx)
}

func (h *AgentsHandler) agentRuntimeContext(c *gin.Context) (agentRuntimeContext, bool) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return agentRuntimeContext{}, false
	}
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return agentRuntimeContext{}, false
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return agentRuntimeContext{}, false
	}
	agentID, err := uuid.Parse(strings.TrimSpace(c.Param("agent_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return agentRuntimeContext{}, false
	}
	ctx := agentRuntimeRequestContext(c, accountID.String())
	draft, err := h.appService.GetAgentDraftRuntimeConfig(ctx, agentID.String(), accountID.String())
	if err != nil {
		h.failRuntime(c, err)
		return agentRuntimeContext{}, false
	}
	agentWorkspaceID, err := uuid.Parse(strings.TrimSpace(draft.WorkspaceID))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return agentRuntimeContext{}, false
	}
	scope := runtimeservice.Scope{OrganizationID: organizationID, AccountID: accountID, WorkspaceID: &agentWorkspaceID}
	runConfig, err := h.agentRunConfig(ctx, scope, agentID.String(), "agent.draft", draft.Config, "account")
	if err != nil {
		h.failRuntime(c, err)
		return agentRuntimeContext{}, false
	}
	return agentRuntimeContext{
		Scope: scope,
		Caller: runtimeservice.Caller{
			Type:   runtimemodel.ConversationCallerAgent,
			ID:     &agentID,
			Source: runtimemodel.ConversationSourceConsole,
		},
		RunConfig: runConfig,
	}, true
}

func (h *AgentsHandler) webAppAgentRuntimeContext(c *gin.Context) (agentRuntimeContext, bool) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return agentRuntimeContext{}, false
	}
	published, err := h.appService.GetPublishedAgentWebAppConfig(c.Request.Context(), c.Param("web_app_id"))
	if err != nil {
		h.failWebAppRuntime(c, err)
		return agentRuntimeContext{}, false
	}
	if !requireAuthenticatedWebAppAgentWhenMemoryEnabled(c, published) {
		return agentRuntimeContext{}, false
	}
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return agentRuntimeContext{}, false
	}
	agentID, err := uuid.Parse(published.AgentID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return agentRuntimeContext{}, false
	}
	workspaceID, err := uuid.Parse(published.WorkspaceID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return agentRuntimeContext{}, false
	}
	organizationID, err := uuid.Parse(published.OrganizationID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return agentRuntimeContext{}, false
	}
	webAppID, err := uuid.Parse(published.WebAppID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return agentRuntimeContext{}, false
	}
	scope := runtimeservice.Scope{
		OrganizationID:  organizationID,
		AccountID:       accountID,
		WorkspaceID:     &workspaceID,
		SkipAccessCheck: true,
	}
	runConfig, err := h.agentRunConfig(c.Request.Context(), scope, published.AgentID, "agent.published."+published.Version, published.Config, webAppAgentMemoryUserScope(c))
	if err != nil {
		h.failRuntime(c, err)
		return agentRuntimeContext{}, false
	}
	return agentRuntimeContext{
		Scope: scope,
		Caller: runtimeservice.Caller{
			Type:           runtimemodel.ConversationCallerAgent,
			ID:             &agentID,
			Source:         runtimemodel.ConversationSourceWebApp,
			SourceWebAppID: &webAppID,
		},
		RunConfig: runConfig,
	}, true
}

func agentRunConfig(agentID, systemPromptVersion string, cfg dto.AgentConfigResponse, agentMemoryUserScope string) runtimeservice.RunConfig {
	return runtimeservice.RunConfig{
		SystemPrompt:              cfg.SystemPrompt,
		SystemPromptVersion:       systemPromptVersion,
		ModelProvider:             cfg.ModelProvider,
		Model:                     cfg.Model,
		ModelParameters:           cfg.ModelParameters,
		EnabledSkillIDs:           cfg.EnabledSkillIDs,
		KnowledgeDatasetIDs:       cfg.KnowledgeDatasetIDs,
		KnowledgeBoundByAccountID: cfg.KnowledgeBoundByAccountID,
		KnowledgeBoundAtUnix:      cfg.KnowledgeBoundAtUnix,
		KnowledgeRetrievalConfig:  cfg.KnowledgeRetrievalConfig,
		DatabaseBindings:          agentDatabaseRuntimeBindings(cfg.DatabaseBindings),
		DatabaseBoundByAccountID:  cfg.DatabaseBoundByAccountID,
		DatabaseBoundAtUnix:       cfg.DatabaseBoundAtUnix,
		WorkflowBindings:          agentWorkflowRuntimeBindings(cfg.WorkflowBindings),
		WorkflowBoundByAccountID:  cfg.WorkflowBoundByAccountID,
		WorkflowBoundAtUnix:       cfg.WorkflowBoundAtUnix,
		UseMemory:                 false,
		AgentMemoryEnabled:        cfg.AgentMemoryEnabled,
		AgentMemorySlots:          agentMemoryRuntimeSlots(cfg.AgentMemorySlots),
		AgentMemoryUserScope:      agentMemoryUserScope,
		BillingAppID:              agentID,
		BillingAppType:            runtimemodel.ConversationCallerAgent,
	}
}

func agentDatabaseRuntimeBindings(bindings []dto.AgentDatabaseBinding) []runtimeservice.AgentDatabaseBinding {
	out := make([]runtimeservice.AgentDatabaseBinding, 0, len(bindings))
	for _, binding := range bindings {
		if strings.TrimSpace(binding.DataSourceID) == "" || len(binding.TableIDs) == 0 {
			continue
		}
		out = append(out, runtimeservice.AgentDatabaseBinding{
			DataSourceID:     strings.TrimSpace(binding.DataSourceID),
			TableIDs:         append([]string(nil), binding.TableIDs...),
			WritableTableIDs: append([]string(nil), binding.WritableTableIDs...),
		})
	}
	return out
}

func agentWorkflowRuntimeBindings(bindings []dto.AgentWorkflowBinding) []runtimeservice.AgentWorkflowBinding {
	out := make([]runtimeservice.AgentWorkflowBinding, 0, len(bindings))
	for _, binding := range bindings {
		if strings.TrimSpace(binding.BindingID) == "" || strings.TrimSpace(binding.AgentID) == "" || strings.TrimSpace(binding.WorkflowID) == "" {
			continue
		}
		out = append(out, runtimeservice.AgentWorkflowBinding{
			BindingID:       strings.TrimSpace(binding.BindingID),
			Label:           strings.TrimSpace(binding.Label),
			Description:     strings.TrimSpace(binding.Description),
			AgentID:         strings.TrimSpace(binding.AgentID),
			WorkflowID:      strings.TrimSpace(binding.WorkflowID),
			AgentType:       strings.TrimSpace(binding.AgentType),
			VersionStrategy: strings.TrimSpace(binding.VersionStrategy),
			VersionUUID:     strings.TrimSpace(binding.VersionUUID),
			TimeoutSeconds:  binding.TimeoutSeconds,
			StartInputs:     agentWorkflowRuntimeStartInputs(binding.StartInputs),
			RequiredInputs:  append([]string(nil), binding.RequiredInputs...),
			DefaultInputKey: strings.TrimSpace(binding.DefaultInputKey),
		})
	}
	return out
}

func agentWorkflowRuntimeStartInputs(inputs []dto.AgentWorkflowStartInput) []runtimeservice.AgentWorkflowStartInput {
	out := make([]runtimeservice.AgentWorkflowStartInput, 0, len(inputs))
	for _, input := range inputs {
		variable := strings.TrimSpace(input.Variable)
		if variable == "" {
			continue
		}
		out = append(out, runtimeservice.AgentWorkflowStartInput{
			Variable: variable,
			Label:    strings.TrimSpace(input.Label),
			Type:     strings.TrimSpace(input.Type),
			Required: input.Required,
		})
	}
	return out
}

func webAppAgentMemoryUserScope(c *gin.Context) string {
	if c.GetBool("is_authenticated") {
		return "account"
	}
	return "end_user"
}

func agentMemoryRuntimeSlots(slots []dto.AgentMemorySlotConfig) []runtimeservice.AgentMemorySlotConfig {
	out := make([]runtimeservice.AgentMemorySlotConfig, 0, len(slots))
	for _, slot := range slots {
		if !slot.Enabled {
			continue
		}
		out = append(out, runtimeservice.AgentMemorySlotConfig{
			Key:         slot.Key,
			Description: slot.Description,
			MaxChars:    slot.MaxChars,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
		})
	}
	return out
}

func (h *AgentsHandler) listRuntimeConversations(c *gin.Context, runtimeCtx agentRuntimeContext) {
	page := positiveQueryInt(c, "page", 1)
	limit := positiveQueryInt(c, "limit", 20)
	conversations, total, err := h.chatRuntimeService.ListConversationsByCaller(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, page, limit)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	items := make([]runtimedto.ConversationResponse, 0, len(conversations))
	for _, conversation := range conversations {
		items = append(items, runtimeConversationResponse(conversation))
	}
	response.Success(c, runtimedto.ListResponse[runtimedto.ConversationResponse]{
		Data:    items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: int64(page*limit) < total,
	})
}

func (h *AgentsHandler) getRuntimeConversation(c *gin.Context, runtimeCtx agentRuntimeContext) {
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
		return
	}
	conversation, err := h.chatRuntimeService.GetConversationByCaller(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, runtimeConversationResponse(conversation))
}

func (h *AgentsHandler) updateRuntimeConversation(c *gin.Context, runtimeCtx agentRuntimeContext) {
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
		return
	}
	var req runtimedto.UpdateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, err := h.chatRuntimeService.GetConversationByCaller(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID); err != nil {
		h.failRuntime(c, err)
		return
	}
	conversation, err := h.chatRuntimeService.UpdateConversation(c.Request.Context(), runtimeCtx.Scope, conversationID, req)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, runtimeConversationResponse(conversation))
}

func (h *AgentsHandler) deleteRuntimeConversation(c *gin.Context, runtimeCtx agentRuntimeContext) {
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
		return
	}
	if _, err := h.chatRuntimeService.GetConversationByCaller(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID); err != nil {
		h.failRuntime(c, err)
		return
	}
	if err := h.chatRuntimeService.DeleteConversation(c.Request.Context(), runtimeCtx.Scope, conversationID); err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success"})
}

func (h *AgentsHandler) listRuntimeMessages(c *gin.Context, runtimeCtx agentRuntimeContext) {
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
		return
	}
	if _, err := h.chatRuntimeService.GetConversationByCaller(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID); err != nil {
		h.failRuntime(c, err)
		return
	}
	page := positiveQueryInt(c, "page", 1)
	limit := positiveQueryInt(c, "limit", 100)
	messages, total, err := h.chatRuntimeService.ListMessages(c.Request.Context(), runtimeCtx.Scope, conversationID, page, limit)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	items := make([]runtimedto.MessageResponse, 0, len(messages))
	for _, message := range messages {
		items = append(items, runtimeMessageResponse(message))
	}
	response.Success(c, runtimedto.ListResponse[runtimedto.MessageResponse]{
		Data:    items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: int64(page*limit) < total,
	})
}

func (h *AgentsHandler) stopRuntimeConversation(c *gin.Context, runtimeCtx agentRuntimeContext) {
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
		return
	}
	if _, err := h.chatRuntimeService.GetConversationByCaller(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID); err != nil {
		h.failRuntime(c, err)
		return
	}
	result, err := h.chatRuntimeService.StopConversation(c.Request.Context(), runtimeCtx.Scope, conversationID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	if result != nil && result.Message != nil && h.workflowContinuationRunner != nil {
		if workflowRunID := agentWorkflowContinuationRunIDFromMetadata(result.Message.Metadata); workflowRunID != "" {
			if err := h.workflowContinuationRunner.StopWorkflowContinuation(c.Request.Context(), workflowRunID, runtimeCtx.Scope.AccountID.String()); err != nil {
				logger.WarnContext(c.Request.Context(), "failed to stop agent workflow continuation", "workflow_run_id", workflowRunID, err)
			}
		}
	}
	response.Success(c, runtimeStopConversationResponse(result))
}

func (h *AgentsHandler) streamRuntimeEvents(c *gin.Context, runtimeCtx agentRuntimeContext) {
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
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
		logger.WarnContext(c.Request.Context(), "agent runtime event stream failed", "conversation_id", conversationID.String(), "message_id", messageID.String(), err)
		_ = writeAgentSSEEvent(c, "", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"message":         err.Error(),
		})
	}
}

func (h *AgentsHandler) failRuntime(c *gin.Context, err error) {
	switch {
	case errors.Is(err, runtimeservice.ErrNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, runtimeservice.ErrInvalidInput):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	case errors.Is(err, runtimeservice.ErrConversationWaitingApproval):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	case errors.Is(err, agentmemory.ErrInvalidInput):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	case errors.Is(err, runtimeservice.ErrUnauthorized):
		response.Fail(c, response.ErrUnauthorized)
	case errors.Is(err, runtimeservice.ErrPermissionDenied):
		response.SpecialFail(c, gin.H{"code": "403001", "message": "Permission denied"})
	case errors.Is(err, errAgentWebAppOffline):
		response.Fail(c, response.ErrWebAppOffline)
	case errors.Is(err, errAgentWebAppNotPublished):
		response.Fail(c, response.ErrWebAppNotPublished)
	case errors.Is(err, errAgentPromptTooLong):
		response.Fail(c, response.ErrAgentPromptTooLong)
	default:
		logger.ErrorContext(c.Request.Context(), "agent runtime request failed", err)
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
	}
}

func (h *AgentsHandler) failWebAppRuntime(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errAgentWebAppOffline):
		response.Fail(c, response.ErrWebAppOffline)
	case errors.Is(err, errAgentWebAppNotPublished):
		response.Fail(c, response.ErrWebAppNotPublished)
	default:
		logger.ErrorContext(c.Request.Context(), "agent webapp runtime request failed", err)
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
	}
}

func uuidParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param(name)))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return uuid.Nil, false
	}
	return id, true
}

func positiveQueryInt(c *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(c.DefaultQuery(name, strconv.Itoa(fallback)))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func runtimeConversationResponse(conversation *runtimemodel.Conversation) runtimedto.ConversationResponse {
	resp := runtimedto.ConversationResponse{
		ID:             conversation.ID.String(),
		OrganizationID: conversation.OrganizationID.String(),
		AccountID:      conversation.AccountID.String(),
		Title:          conversation.Title,
		Status:         conversation.Status,
		RuntimeStatus:  conversation.RuntimeStatus,
		DialogueCount:  conversation.DialogueCount,
		Source:         conversation.Source,
		Metadata:       conversation.Metadata,
		CreatedAt:      conversation.CreatedAt.Unix(),
		UpdatedAt:      conversation.UpdatedAt.Unix(),
	}
	if resp.Metadata == nil {
		resp.Metadata = map[string]interface{}{}
	}
	if conversation.WorkspaceID != nil {
		resp.WorkspaceID = stringPtr(conversation.WorkspaceID.String())
	}
	if conversation.CurrentLeafMessageID != nil {
		resp.CurrentLeafMessageID = stringPtr(conversation.CurrentLeafMessageID.String())
	}
	if conversation.ActiveMessageID != nil {
		resp.ActiveMessageID = stringPtr(conversation.ActiveMessageID.String())
	}
	if conversation.SourceConversationID != nil {
		resp.SourceConversationID = stringPtr(conversation.SourceConversationID.String())
	}
	if conversation.SourceWebAppID != nil {
		resp.SourceWebAppID = stringPtr(conversation.SourceWebAppID.String())
	}
	return resp
}

func runtimeMessageResponse(message *runtimemodel.Message) runtimedto.MessageResponse {
	resp := runtimedto.MessageResponse{
		ID:                  message.ID.String(),
		ConversationID:      message.ConversationID.String(),
		Query:               message.Query,
		Answer:              message.Answer,
		Status:              message.Status,
		Error:               message.Error,
		ModelProvider:       message.ModelProvider,
		ModelName:           message.ModelName,
		BillingReasonSource: message.BillingReasonSource,
		ModelParameters:     message.ModelParameters,
		Metadata:            message.Metadata,
		CreatedAt:           message.CreatedAt.Unix(),
		UpdatedAt:           message.UpdatedAt.Unix(),
	}
	if message.ParentID != nil {
		resp.ParentID = stringPtr(message.ParentID.String())
	}
	if message.SourceMessageID != nil {
		resp.SourceMessageID = stringPtr(message.SourceMessageID.String())
	}
	return resp
}

func runtimeStopConversationResponse(result *runtimeservice.StopConversationResult) runtimedto.StopConversationResponse {
	resp := runtimedto.StopConversationResponse{
		Status: runtimemodel.ConversationRuntimeStatusIdle,
	}
	if result == nil || result.Conversation == nil {
		return resp
	}
	resp.ConversationID = result.Conversation.ID.String()
	resp.RuntimeStatus = result.Conversation.RuntimeStatus
	resp.Status = result.Conversation.RuntimeStatus
	if result.Message != nil {
		resp.MessageID = stringPtr(result.Message.ID.String())
		resp.Status = result.Message.Status
	}
	return resp
}
