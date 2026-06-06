package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

type agentRuntimeContext struct {
	Scope     runtimeservice.Scope
	Caller    runtimeservice.Caller
	RunConfig runtimeservice.RunConfig
}

const agentWorkflowContinuationMaxDuration = 35 * time.Minute

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

func (h *AgentsHandler) ContinueAgentRuntimeWorkflowApproval(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.continueRuntimeWorkflowApproval(c, runtimeCtx)
}

func (h *AgentsHandler) continueRuntimeWorkflowApproval(c *gin.Context, runtimeCtx agentRuntimeContext) {
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
		return
	}
	messageID, ok := uuidParam(c, "message_id")
	if !ok {
		return
	}
	continuation, err := h.chatRuntimeService.BeginWorkflowApprovalContinuation(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID, messageID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	setupAgentSSE(c)
	_ = writeAgentSSE(c, "message_start", gin.H{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
		"workflow_run_id": continuation.WorkflowRunID,
		"created_at":      time.Now().Unix(),
		"continuation":    true,
	})
	if continuation.Completed {
		_ = writeAgentSSE(c, "message_end", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"status":          runtimemodel.MessageStatusCompleted,
			"metadata":        continuation.Metadata,
		})
		return
	}
	h.streamWorkflowApprovalContinuation(c, runtimeCtx.Scope, continuation)
}

func (h *AgentsHandler) streamWorkflowApprovalContinuation(c *gin.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation) {
	if h.db == nil {
		h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, errors.New("database is not available"), func(eventType string, payload gin.H) error {
			_ = writeAgentSSE(c, eventType, payload)
			return nil
		})
		return
	}
	workBaseCtx := context.WithoutCancel(c.Request.Context())
	workCtx, cancelWork := context.WithTimeout(workBaseCtx, agentWorkflowContinuationMaxDuration)
	defer cancelWork()
	run, err := h.loadAgentWorkflowRunLog(workCtx, continuation.WorkflowRunID)
	if err != nil {
		h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, err, func(eventType string, payload gin.H) error {
			_ = writeAgentSSE(c, eventType, payload)
			return nil
		})
		return
	}
	pauseService := workflowpause.NewService(h.db)
	lastSequence := 0
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		terminal, nextSequence := h.drainAgentWorkflowContinuationEvents(workCtx, continuation, pauseService, run.TenantID, lastSequence, func(eventType string, payload gin.H) error {
			return writeAgentSSE(c, eventType, payload)
		})
		lastSequence = nextSequence
		if terminal {
			h.finishAgentWorkflowContinuation(workCtx, scope, continuation, func(eventType string, payload gin.H) error {
				_ = writeAgentSSE(c, eventType, payload)
				return nil
			})
			return
		}
		if h.finishAgentWorkflowContinuationIfRunTerminal(workCtx, scope, continuation, func(eventType string, payload gin.H) error {
			_ = writeAgentSSE(c, eventType, payload)
			return nil
		}) {
			return
		}
		select {
		case <-c.Request.Context().Done():
			h.startWorkflowApprovalContinuationBackground(workBaseCtx, scope, continuation, run.TenantID, lastSequence)
			return
		case <-workCtx.Done():
			h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, workCtx.Err(), func(eventType string, payload gin.H) error {
				_ = writeAgentSSE(c, eventType, payload)
				return nil
			})
			return
		case <-ticker.C:
		}
	}
}

func (h *AgentsHandler) startWorkflowApprovalContinuationBackground(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, tenantID string, afterSequence int) {
	go func() {
		ctx, cancel := context.WithTimeout(ctx, agentWorkflowContinuationMaxDuration)
		defer cancel()
		pauseService := workflowpause.NewService(h.db)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		lastSequence := afterSequence
		for {
			terminal, nextSequence := h.drainAgentWorkflowContinuationEvents(ctx, continuation, pauseService, tenantID, lastSequence, nil)
			lastSequence = nextSequence
			if terminal {
				h.finishAgentWorkflowContinuation(ctx, scope, continuation, nil)
				return
			}
			if h.finishAgentWorkflowContinuationIfRunTerminal(ctx, scope, continuation, nil) {
				return
			}
			select {
			case <-ctx.Done():
				h.failAgentWorkflowContinuation(context.WithoutCancel(ctx), continuation, ctx.Err(), nil)
				return
			case <-ticker.C:
			}
		}
	}()
}

func (h *AgentsHandler) drainAgentWorkflowContinuationEvents(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, pauseService *workflowpause.Service, tenantID string, afterSequence int, emit func(string, gin.H) error) (bool, int) {
	payload, err := pauseService.ListEvents(ctx, tenantID, continuation.WorkflowRunID, afterSequence, 100)
	if err != nil {
		logger.WarnContext(ctx, "failed to list workflow continuation events", "workflow_run_id", continuation.WorkflowRunID, err)
		return false, afterSequence
	}
	lastSequence := afterSequence
	terminal := false
	for _, event := range payload.Events {
		lastSequence = event.Sequence
		eventType := agentWorkflowContinuationEventType(event.Event)
		if eventType == "" {
			continue
		}
		data := copyMapForAgentWorkflowContinuation(event.Data)
		data["workflow_run_id"] = continuation.WorkflowRunID
		data["conversation_id"] = continuation.ConversationID.String()
		data["message_id"] = continuation.MessageID.String()
		persistedPayload, persistErr := h.chatRuntimeService.RecordWorkflowApprovalContinuationEvent(ctx, continuation, eventType, data)
		if persistErr != nil {
			logger.WarnContext(ctx, "failed to persist workflow continuation event", "workflow_run_id", continuation.WorkflowRunID, "event_type", eventType, persistErr)
		}
		if persistedPayload != nil {
			data = persistedPayload
		}
		if emit != nil {
			_ = emit(eventType, data)
		}
		if eventType == "workflow_finished" || eventType == "workflow_failed" {
			terminal = true
		}
	}
	return terminal, lastSequence
}

func (h *AgentsHandler) finishAgentWorkflowContinuationIfRunTerminal(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, emit func(string, gin.H) error) bool {
	run, err := h.loadAgentWorkflowRunLog(ctx, continuation.WorkflowRunID)
	if err != nil {
		logger.WarnContext(ctx, "failed to load workflow continuation run status", "workflow_run_id", continuation.WorkflowRunID, err)
		return false
	}
	if !agentWorkflowRunLogTerminal(run.Status) {
		return false
	}
	h.finishAgentWorkflowContinuation(ctx, scope, continuation, emit)
	return true
}

func (h *AgentsHandler) finishAgentWorkflowContinuation(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, emit func(string, gin.H) error) {
	run, err := h.loadAgentWorkflowRunLog(ctx, continuation.WorkflowRunID)
	if err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	outputs := run.OutputsMap()
	if shouldSummarizeAgentWorkflowContinuation(continuation.AgentType, run.Status, outputs) {
		errorMessage := ""
		if run.Error != nil {
			errorMessage = *run.Error
		}
		result, summaryErr := h.chatRuntimeService.SummarizeWorkflowApprovalContinuation(ctx, scope, continuation, runtimeservice.WorkflowContinuationSummaryRequest{
			WorkflowRunID: continuation.WorkflowRunID,
			Status:        run.Status,
			Outputs:       outputs,
			Error:         errorMessage,
		}, func(chunk string) error {
			emitAgentWorkflowContinuationEvent(emit, "message", gin.H{
				"conversation_id": continuation.ConversationID.String(),
				"message_id":      continuation.MessageID.String(),
				"answer":          chunk,
			})
			return nil
		})
		if summaryErr != nil {
			h.failAgentWorkflowContinuation(ctx, continuation, summaryErr, emit)
			return
		}
		metadata := map[string]interface{}{}
		if result != nil {
			metadata = result.Metadata
		}
		emitAgentWorkflowContinuationEvent(emit, "message_end", gin.H{
			"conversation_id": continuation.ConversationID.String(),
			"message_id":      continuation.MessageID.String(),
			"status":          runtimemodel.MessageStatusCompleted,
			"metadata":        metadata,
		})
		return
	}
	status := "direct_output"
	if strings.EqualFold(strings.TrimSpace(run.Status), "failed") {
		status = "failed"
	}
	if _, err := h.chatRuntimeService.UpdateWorkflowApprovalContinuationStatus(ctx, continuation, status); err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	answer := agentWorkflowContinuationAnswer(continuation.AgentType, continuation.WorkflowRunID, run.Status, outputs, run.Error)
	if strings.TrimSpace(answer) != "" {
		emitAgentWorkflowContinuationEvent(emit, "message", gin.H{
			"conversation_id": continuation.ConversationID.String(),
			"message_id":      continuation.MessageID.String(),
			"answer":          answer,
		})
	}
	metadata, err := h.chatRuntimeService.CompleteWorkflowApprovalContinuation(ctx, continuation, answer, completionContinuationStatus(run.Status))
	if err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	emitAgentWorkflowContinuationEvent(emit, "message_end", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"status":          runtimemodel.MessageStatusCompleted,
		"metadata":        metadata,
	})
}

func (h *AgentsHandler) failAgentWorkflowContinuation(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, cause error, emit func(string, gin.H) error) {
	message := "workflow continuation timed out before completion"
	if cause != nil && !errors.Is(cause, context.DeadlineExceeded) {
		message = fmt.Sprintf("workflow continuation stopped before completion: %v", cause)
	}
	metadata, err := h.chatRuntimeService.FailWorkflowApprovalContinuation(ctx, continuation, message)
	if err != nil {
		emitAgentWorkflowContinuationError(emit, err)
		return
	}
	emitAgentWorkflowContinuationEvent(emit, "error", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"message":         message,
	})
	emitAgentWorkflowContinuationEvent(emit, "message_end", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"status":          runtimemodel.MessageStatusError,
		"metadata":        metadata,
	})
}

func emitAgentWorkflowContinuationEvent(emit func(string, gin.H) error, eventType string, payload gin.H) {
	if emit == nil {
		return
	}
	_ = emit(eventType, payload)
}

func emitAgentWorkflowContinuationError(emit func(string, gin.H) error, err error) {
	if err == nil {
		return
	}
	emitAgentWorkflowContinuationEvent(emit, "error", gin.H{"message": err.Error()})
}

func shouldSummarizeAgentWorkflowContinuation(agentType, status string, outputs map[string]interface{}) bool {
	if !strings.EqualFold(strings.TrimSpace(agentType), "WORKFLOW") {
		return false
	}
	if agentWorkflowRunLogFailed(status) {
		return false
	}
	return len(outputs) > 0
}

func completionContinuationStatus(status string) string {
	if agentWorkflowRunLogFailed(status) {
		return "failed"
	}
	return "completed"
}

func agentWorkflowRunLogTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "failed", "stopped", "partial-succeeded":
		return true
	default:
		return false
	}
}

func agentWorkflowRunLogFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "stopped":
		return true
	default:
		return false
	}
}

type agentWorkflowRunLogRow struct {
	ID          string
	TenantID    string
	Status      string
	Outputs     *string
	Error       *string
	ElapsedTime float64
}

func (h *AgentsHandler) loadAgentWorkflowRunLog(ctx context.Context, workflowRunID string) (*agentWorkflowRunLogRow, error) {
	if strings.TrimSpace(workflowRunID) == "" {
		return nil, fmt.Errorf("workflow_run_id is required")
	}
	var row agentWorkflowRunLogRow
	err := h.db.WithContext(ctx).
		Table("workflow_run_logs").
		Select("id, tenant_id, status, outputs, error, elapsed_time").
		Where("id = ? AND deleted_at IS NULL", strings.TrimSpace(workflowRunID)).
		Take(&row).Error
	if err != nil {
		return nil, fmt.Errorf("load workflow run log: %w", err)
	}
	return &row, nil
}

func (r *agentWorkflowRunLogRow) OutputsMap() map[string]interface{} {
	if r == nil || r.Outputs == nil || strings.TrimSpace(*r.Outputs) == "" {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(*r.Outputs), &out); err != nil || out == nil {
		return map[string]interface{}{}
	}
	return out
}

func agentWorkflowContinuationEventType(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case workflowpause.EventWorkflowStarted:
		return "workflow_started"
	case workflowpause.EventNodeStarted:
		return "node_started"
	case workflowpause.EventNodeFinished:
		return "node_finished"
	case workflowpause.EventWorkflowPaused:
		return "workflow_paused"
	case workflowpause.EventApprovalRequested:
		return "approval_requested"
	case workflowpause.EventWorkflowFinished:
		return "workflow_finished"
	case workflowpause.EventError:
		return "workflow_failed"
	default:
		return ""
	}
}

func copyMapForAgentWorkflowContinuation(input map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(input)+2)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func agentWorkflowContinuationAnswer(agentType, workflowRunID, status string, outputs map[string]interface{}, errorMessage *string) string {
	if strings.EqualFold(strings.TrimSpace(status), "failed") {
		message := ""
		if errorMessage != nil {
			message = strings.TrimSpace(*errorMessage)
		}
		if message == "" {
			message = "unknown error"
		}
		return fmt.Sprintf("Workflow run failed. workflow_run_id: %s\n\nError: %s", workflowRunID, message)
	}
	primary := primaryAgentWorkflowOutput(outputs)
	if strings.EqualFold(strings.TrimSpace(agentType), "CONVERSATIONAL_WORKFLOW") {
		if primary != "" {
			return primary
		}
		return fmt.Sprintf("Workflow run completed, but no displayable output was returned. workflow_run_id: %s", workflowRunID)
	}
	if primary != "" {
		return primary
	}
	if len(outputs) == 0 {
		return fmt.Sprintf("Workflow run completed, but no displayable output was returned. workflow_run_id: %s", workflowRunID)
	}
	data, err := json.MarshalIndent(outputs, "", "  ")
	if err != nil {
		return fmt.Sprintf("Workflow run completed. workflow_run_id: %s", workflowRunID)
	}
	return fmt.Sprintf("Workflow run completed. Outputs:\n\n```json\n%s\n```", string(data))
}

func primaryAgentWorkflowOutput(outputs map[string]interface{}) string {
	if len(outputs) == 0 {
		return ""
	}
	if answer := strings.TrimSpace(fmt.Sprint(outputs["answer"])); answer != "" && answer != "<nil>" {
		return answer
	}
	if output := strings.TrimSpace(fmt.Sprint(outputs["output"])); output != "" && output != "<nil>" {
		return output
	}
	return ""
}

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

func agentWorkflowContinuationWaiting(metadata map[string]interface{}) bool {
	state, ok := metadata["agent_workflow_continuation"].(map[string]interface{})
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(state["status"])), "waiting_approval")
}

func (h *AgentsHandler) validateAgentRuntimeSkills(c *gin.Context, req dto.AgentConfigRequest) error {
	skillIDs := req.EnabledSkillIDs
	if h.chatRuntimeService == nil || len(skillIDs) == 0 {
		return nil
	}
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		return fmt.Errorf("unauthorized")
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		return fmt.Errorf("unauthorized")
	}
	scope := runtimeservice.Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}
	skillsMetadata, err := h.chatRuntimeService.ListSkills(c.Request.Context(), scope)
	if err != nil {
		return err
	}
	metadataByID := make(map[string]runtimedto.SkillResponse, len(skillsMetadata))
	for _, item := range skillsMetadata {
		metadataByID[strings.ToLower(strings.TrimSpace(item.ID))] = skillResponseFromMetadata(item)
	}
	for _, raw := range skillIDs {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if skills.IsHiddenSystemSkill(id) {
			continue
		}
		metadata, ok := metadataByID[id]
		if !ok {
			return fmt.Errorf("skill %s is not found", id)
		}
		if !skillResponseSupportsCaller(metadata, runtimemodel.ConversationCallerAgent) {
			return fmt.Errorf("skill %s is not available for agent", id)
		}
		if skillResponseRequires(metadata, "agent_knowledge") && len(req.KnowledgeDatasetIDs) == 0 {
			return fmt.Errorf("skill %s requires configured knowledge datasets", id)
		}
		if skillResponseRequires(metadata, "agent_database") && len(normalizeAgentDatabaseBindings(req.DatabaseBindings)) == 0 {
			return fmt.Errorf("skill %s requires configured database bindings", id)
		}
	}
	return nil
}

func skillResponseFromMetadata(metadata skills.SkillDiscoveryMetadata) runtimedto.SkillResponse {
	return runtimedto.SkillResponse{
		SkillID:          metadata.ID,
		SupportedCallers: metadata.SupportedCallers,
		RequiredConfig:   metadata.RequiredConfig,
	}
}

func skillResponseSupportsCaller(metadata runtimedto.SkillResponse, callerType string) bool {
	if len(metadata.SupportedCallers) == 0 {
		return true
	}
	for _, caller := range metadata.SupportedCallers {
		if strings.EqualFold(strings.TrimSpace(caller), callerType) {
			return true
		}
	}
	return false
}

func skillResponseRequires(metadata runtimedto.SkillResponse, requirement string) bool {
	for _, value := range metadata.RequiredConfig {
		if strings.EqualFold(strings.TrimSpace(value), requirement) {
			return true
		}
	}
	return false
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
