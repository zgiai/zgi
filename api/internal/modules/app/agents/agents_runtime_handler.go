package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
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

type agentWorkflowContinuationRequest struct {
	Type          string                 `json:"type"`
	Inputs        map[string]interface{} `json:"inputs"`
	Action        string                 `json:"action"`
	ApprovalToken string                 `json:"approval_token"`
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
	req, err := readAgentWorkflowContinuationRequest(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	questionContinuation := isAgentWorkflowQuestionContinuation(req)
	approvalContinuation := isAgentWorkflowApprovalContinuation(req)
	var resumeInputs map[string]interface{}
	if questionContinuation || approvalContinuation {
		if h.workflowContinuationRunner == nil {
			h.failRuntime(c, fmt.Errorf("%w: workflow continuation runner is not configured", runtimeservice.ErrInvalidInput))
			return
		}
	}
	if questionContinuation {
		resumeInputs = normalizeAgentWorkflowQuestionInputs(req.Inputs)
		if len(resumeInputs) == 0 {
			h.failRuntime(c, fmt.Errorf("%w: question answer continuation inputs are required", runtimeservice.ErrInvalidInput))
			return
		}
	}
	if approvalContinuation {
		if strings.TrimSpace(req.ApprovalToken) == "" || strings.TrimSpace(req.Action) == "" {
			h.failRuntime(c, fmt.Errorf("%w: approval token and action are required", runtimeservice.ErrInvalidInput))
			return
		}
	}
	continuation, err := h.chatRuntimeService.BeginWorkflowApprovalContinuation(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, conversationID, messageID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	setupAgentSSE(c)
	emit := func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	}
	h.emitAgentWorkflowContinuationStreamEvent(c.Request.Context(), continuation, "message_start", gin.H{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
		"workflow_run_id": continuation.WorkflowRunID,
		"created_at":      time.Now().Unix(),
		"continuation":    true,
	}, emit)
	if continuation.Completed {
		h.emitAgentWorkflowContinuationStreamEvent(c.Request.Context(), continuation, "message_end", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"status":          runtimemodel.MessageStatusCompleted,
			"metadata":        continuation.Metadata,
		}, emit)
		return
	}
	if approvalContinuation && h.finishAgentWorkflowContinuationIfRunTerminal(
		c.Request.Context(),
		runtimeCtx.Scope,
		continuation,
		"",
		false,
		emit,
	) {
		return
	}
	if h.streamWorkflowApprovalContinuationDirect(c, runtimeCtx.Scope, continuation, req, resumeInputs, approvalContinuation, questionContinuation) {
		return
	}
	afterSequence := 0
	if questionContinuation || approvalContinuation {
		run, err := h.loadAgentWorkflowRunLog(c.Request.Context(), continuation.WorkflowRunID)
		if err != nil {
			h.failRuntime(c, err)
			return
		}
		afterSequence = h.latestAgentWorkflowContinuationSequence(c.Request.Context(), run.TenantID, continuation.WorkflowRunID)
	}
	resumeErrCh := make(chan error, 1)
	if approvalContinuation {
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), agentWorkflowContinuationMaxDuration)
			defer cancel()
			if err := h.resumeAgentWorkflowApproval(ctx, runtimeCtx.Scope, continuation, req); err != nil {
				resumeErrCh <- err
			}
		}()
	}
	if questionContinuation {
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), agentWorkflowContinuationMaxDuration)
			defer cancel()
			if err := h.workflowContinuationRunner.ResumeQuestionAnswerWorkflow(ctx, continuation.WorkflowRunID, resumeInputs); err != nil {
				resumeErrCh <- err
			}
		}()
	}
	h.streamWorkflowApprovalContinuation(c, runtimeCtx.Scope, continuation, afterSequence, resumeErrCh)
}

func readAgentWorkflowContinuationRequest(c *gin.Context) (agentWorkflowContinuationRequest, error) {
	var req agentWorkflowContinuationRequest
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return req, nil
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return req, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return req, fmt.Errorf("invalid workflow continuation request")
	}
	return req, nil
}

func isAgentWorkflowQuestionContinuation(req agentWorkflowContinuationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Type), "question_answer")
}

func isAgentWorkflowApprovalContinuation(req agentWorkflowContinuationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Type), "approval")
}

func (h *AgentsHandler) resumeAgentWorkflowApproval(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, req agentWorkflowContinuationRequest) error {
	if h.db == nil {
		return errors.New("database is not available")
	}
	if h.workflowContinuationRunner == nil {
		return fmt.Errorf("%w: workflow continuation runner is not configured", runtimeservice.ErrInvalidInput)
	}
	approvalService := approvalruntime.NewService(h.db)
	accountID := scope.AccountID.String()
	form, err := approvalService.SubmitByToken(ctx, strings.TrimSpace(req.ApprovalToken), approvalruntime.SubmitRequest{
		Inputs: copyMapForAgentWorkflowContinuation(req.Inputs),
		Action: strings.TrimSpace(req.Action),
	}, &accountID, nil)
	if err != nil {
		return err
	}
	resumeReady, err := approvalService.ActivePauseApprovalFormsSubmitted(ctx, form.WorkflowRunID)
	if err != nil {
		return err
	}
	if !resumeReady {
		if err := approvalService.AppendApprovalResultFilledEvent(ctx, form); err != nil {
			logger.WarnContext(ctx, "failed to append agent workflow approval result filled event", "form_id", form.ID, err)
		}
		return fmt.Errorf("workflow run %s is waiting for additional approvals", continuation.WorkflowRunID)
	}
	return h.workflowContinuationRunner.ResumeApprovalWorkflow(ctx, form)
}

func (h *AgentsHandler) resumeAgentWorkflowApprovalStream(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, req agentWorkflowContinuationRequest, runner workflowContinuationStreamRunner, onEvent func(string, map[string]interface{}) error) error {
	if h.db == nil {
		return errors.New("database is not available")
	}
	approvalService := approvalruntime.NewService(h.db)
	accountID := scope.AccountID.String()
	form, err := approvalService.SubmitByToken(ctx, strings.TrimSpace(req.ApprovalToken), approvalruntime.SubmitRequest{
		Inputs: copyMapForAgentWorkflowContinuation(req.Inputs),
		Action: strings.TrimSpace(req.Action),
	}, &accountID, nil)
	if err != nil {
		return err
	}
	resumeReady, err := approvalService.ActivePauseApprovalFormsSubmitted(ctx, form.WorkflowRunID)
	if err != nil {
		return err
	}
	if !resumeReady {
		if err := approvalService.AppendApprovalResultFilledEvent(ctx, form); err != nil {
			logger.WarnContext(ctx, "failed to append agent workflow approval result filled event", "form_id", form.ID, err)
		}
		return fmt.Errorf("workflow run %s is waiting for additional approvals", continuation.WorkflowRunID)
	}
	return runner.ResumeApprovalWorkflowStream(ctx, form, onEvent)
}

func (h *AgentsHandler) streamWorkflowApprovalContinuationDirect(c *gin.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, req agentWorkflowContinuationRequest, resumeInputs map[string]interface{}, approvalContinuation bool, questionContinuation bool) bool {
	if !approvalContinuation && !questionContinuation {
		return false
	}
	streamRunner, ok := h.workflowContinuationRunner.(workflowContinuationStreamRunner)
	if !ok {
		return false
	}
	workCtx, cancelWork := context.WithTimeout(context.WithoutCancel(c.Request.Context()), agentWorkflowContinuationMaxDuration)
	defer cancelWork()
	state := &agentWorkflowContinuationStreamState{}
	emit := func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	}
	onWorkflowEvent := func(eventType string, payload map[string]interface{}) error {
		result := h.handleAgentWorkflowContinuationEvent(workCtx, continuation, eventType, payload, emit)
		state.apply(result)
		return nil
	}
	var err error
	if approvalContinuation {
		err = h.resumeAgentWorkflowApprovalStream(workCtx, scope, continuation, req, streamRunner, onWorkflowEvent)
	} else {
		err = streamRunner.ResumeQuestionAnswerWorkflowStream(workCtx, continuation.WorkflowRunID, resumeInputs, onWorkflowEvent)
	}
	if err != nil {
		h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, err, emit)
		return true
	}
	if state.WaitingStatus != "" {
		h.pauseAgentWorkflowContinuation(workCtx, continuation, state.WaitingStatus, emit)
		return true
	}
	if state.Terminal {
		h.finishAgentWorkflowContinuation(workCtx, scope, continuation, state.WorkflowMessageText, state.HasWorkflowMessage, emit)
		return true
	}
	if h.finishAgentWorkflowContinuationIfRunTerminal(workCtx, scope, continuation, state.WorkflowMessageText, state.HasWorkflowMessage, emit) {
		return true
	}
	h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, errors.New("workflow continuation ended without terminal event"), emit)
	return true
}

func normalizeAgentWorkflowQuestionInputs(inputs map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	query := strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["query"]))
	if query == "" {
		query = strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["sys.query"]))
	}
	if query != "" {
		out["query"] = query
		out["sys.query"] = query
	}
	optionID := strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["question_answer_option_id"]))
	if optionID == "" {
		optionID = strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["option_id"]))
	}
	if optionID != "" {
		out["question_answer_option_id"] = optionID
	}
	return out
}

func (h *AgentsHandler) streamWorkflowApprovalContinuation(c *gin.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, afterSequence int, resumeErrCh <-chan error) {
	emit := func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	}
	if h.db == nil {
		h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, errors.New("database is not available"), emit)
		return
	}
	workBaseCtx := context.WithoutCancel(c.Request.Context())
	workCtx, cancelWork := context.WithTimeout(workBaseCtx, agentWorkflowContinuationMaxDuration)
	defer cancelWork()
	run, err := h.loadAgentWorkflowRunLog(workCtx, continuation.WorkflowRunID)
	if err != nil {
		h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, err, emit)
		return
	}
	pauseService := workflowpause.NewService(h.db)
	lastSequence := afterSequence
	passthroughAnswer := strings.Builder{}
	hasPassthroughAnswer := false
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		drained := h.drainAgentWorkflowContinuationEvents(workCtx, continuation, pauseService, run.TenantID, lastSequence, emit)
		lastSequence = drained.NextSequence
		if drained.HasWorkflowMessage {
			hasPassthroughAnswer = true
			passthroughAnswer.WriteString(drained.WorkflowMessageText)
		}
		if drained.Terminal {
			h.finishAgentWorkflowContinuation(workCtx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, emit)
			return
		}
		if drained.WaitingStatus != "" {
			h.pauseAgentWorkflowContinuation(workCtx, continuation, drained.WaitingStatus, emit)
			return
		}
		if h.finishAgentWorkflowContinuationIfRunTerminal(workCtx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, emit) {
			return
		}
		select {
		case <-c.Request.Context().Done():
			h.startWorkflowApprovalContinuationBackground(workBaseCtx, scope, continuation, run.TenantID, lastSequence, passthroughAnswer.String(), hasPassthroughAnswer, resumeErrCh)
			return
		case resumeErr := <-resumeErrCh:
			drained := h.drainAgentWorkflowContinuationEvents(workCtx, continuation, pauseService, run.TenantID, lastSequence, emit)
			lastSequence = drained.NextSequence
			if drained.HasWorkflowMessage {
				hasPassthroughAnswer = true
				passthroughAnswer.WriteString(drained.WorkflowMessageText)
			}
			if drained.Terminal {
				h.finishAgentWorkflowContinuation(workCtx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, emit)
				return
			}
			h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, resumeErr, emit)
			return
		case <-workCtx.Done():
			h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, workCtx.Err(), emit)
			return
		case <-ticker.C:
		}
	}
}

func (h *AgentsHandler) latestAgentWorkflowContinuationSequence(ctx context.Context, tenantID string, workflowRunID string) int {
	if h.db == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(workflowRunID) == "" {
		return 0
	}
	pauseService := workflowpause.NewService(h.db)
	payload, err := pauseService.ListEvents(ctx, tenantID, workflowRunID, 0, 1000)
	if err != nil {
		logger.WarnContext(ctx, "failed to list current workflow continuation events", "workflow_run_id", workflowRunID, err)
		return 0
	}
	latest := 0
	for _, event := range payload.Events {
		if event.Sequence > latest {
			latest = event.Sequence
		}
	}
	return latest
}

func (h *AgentsHandler) startWorkflowApprovalContinuationBackground(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, tenantID string, afterSequence int, initialPassthroughAnswer string, initialHasPassthroughAnswer bool, resumeErrCh <-chan error) {
	go func() {
		ctx, cancel := context.WithTimeout(ctx, agentWorkflowContinuationMaxDuration)
		defer cancel()
		pauseService := workflowpause.NewService(h.db)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		lastSequence := afterSequence
		passthroughAnswer := strings.Builder{}
		passthroughAnswer.WriteString(initialPassthroughAnswer)
		hasPassthroughAnswer := initialHasPassthroughAnswer
		for {
			drained := h.drainAgentWorkflowContinuationEvents(ctx, continuation, pauseService, tenantID, lastSequence, nil)
			lastSequence = drained.NextSequence
			if drained.HasWorkflowMessage {
				hasPassthroughAnswer = true
				passthroughAnswer.WriteString(drained.WorkflowMessageText)
			}
			if drained.Terminal {
				h.finishAgentWorkflowContinuation(ctx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, nil)
				return
			}
			if drained.WaitingStatus != "" {
				h.pauseAgentWorkflowContinuation(ctx, continuation, drained.WaitingStatus, nil)
				return
			}
			if h.finishAgentWorkflowContinuationIfRunTerminal(ctx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, nil) {
				return
			}
			select {
			case resumeErr := <-resumeErrCh:
				drained := h.drainAgentWorkflowContinuationEvents(ctx, continuation, pauseService, tenantID, lastSequence, nil)
				lastSequence = drained.NextSequence
				if drained.HasWorkflowMessage {
					hasPassthroughAnswer = true
					passthroughAnswer.WriteString(drained.WorkflowMessageText)
				}
				if drained.Terminal {
					h.finishAgentWorkflowContinuation(ctx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, nil)
					return
				}
				h.failAgentWorkflowContinuation(context.WithoutCancel(ctx), continuation, resumeErr, nil)
				return
			case <-ctx.Done():
				h.failAgentWorkflowContinuation(context.WithoutCancel(ctx), continuation, ctx.Err(), nil)
				return
			case <-ticker.C:
			}
		}
	}()
}

type agentWorkflowContinuationDrainResult struct {
	Terminal            bool
	WaitingStatus       string
	NextSequence        int
	WorkflowMessageText string
	HasWorkflowMessage  bool
}

type agentWorkflowContinuationStreamState struct {
	Terminal            bool
	WaitingStatus       string
	WorkflowMessageText string
	HasWorkflowMessage  bool
}

func (s *agentWorkflowContinuationStreamState) apply(result agentWorkflowContinuationDrainResult) {
	if result.Terminal {
		s.Terminal = true
	}
	if result.WaitingStatus != "" {
		s.WaitingStatus = result.WaitingStatus
	}
	if result.HasWorkflowMessage {
		s.HasWorkflowMessage = true
		s.WorkflowMessageText += result.WorkflowMessageText
	}
}

func (h *AgentsHandler) drainAgentWorkflowContinuationEvents(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, pauseService *workflowpause.Service, tenantID string, afterSequence int, emit func(runtimeservice.StreamEvent) error) agentWorkflowContinuationDrainResult {
	result := agentWorkflowContinuationDrainResult{NextSequence: afterSequence}
	payload, err := pauseService.ListEvents(ctx, tenantID, continuation.WorkflowRunID, afterSequence, 100)
	if err != nil {
		logger.WarnContext(ctx, "failed to list workflow continuation events", "workflow_run_id", continuation.WorkflowRunID, err)
		return result
	}
	messageText := strings.Builder{}
	for _, event := range payload.Events {
		result.NextSequence = event.Sequence
		eventResult := h.handleAgentWorkflowContinuationEvent(ctx, continuation, event.Event, event.Data, emit)
		if eventResult.HasWorkflowMessage {
			result.HasWorkflowMessage = true
			messageText.WriteString(eventResult.WorkflowMessageText)
		}
		if eventResult.Terminal {
			result.Terminal = true
		}
		if eventResult.WaitingStatus != "" {
			result.WaitingStatus = eventResult.WaitingStatus
		}
	}
	result.WorkflowMessageText = messageText.String()
	return result
}

func (h *AgentsHandler) handleAgentWorkflowContinuationEvent(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, rawEventType string, rawData map[string]interface{}, emit func(runtimeservice.StreamEvent) error) agentWorkflowContinuationDrainResult {
	result := agentWorkflowContinuationDrainResult{}
	eventType := agentWorkflowContinuationEventType(rawEventType)
	if eventType == "" {
		return result
	}
	data := copyMapForAgentWorkflowContinuation(rawData)
	data["workflow_run_id"] = continuation.WorkflowRunID
	data["conversation_id"] = continuation.ConversationID.String()
	data["message_id"] = continuation.MessageID.String()
	streamEvent, persistErr := h.chatRuntimeService.RecordWorkflowApprovalContinuationEvent(ctx, continuation, eventType, data)
	if persistErr != nil {
		logger.WarnContext(ctx, "failed to persist workflow continuation event", "workflow_run_id", continuation.WorkflowRunID, "event_type", eventType, persistErr)
		streamEvent, persistErr = h.chatRuntimeService.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, eventType, data)
		if persistErr != nil {
			logger.WarnContext(ctx, "failed to append fallback workflow continuation stream event", "workflow_run_id", continuation.WorkflowRunID, "event_type", eventType, persistErr)
		}
	}
	if streamEvent != nil {
		data = streamEvent.Payload
	}
	var userInput gin.H
	if eventType == workflowpause.EventQuestionAnswerRequested {
		userInput = agentWorkflowQuestionUserInputEvent(continuation, data)
		if len(userInput) > 0 {
			metadata := copyMapForAgentWorkflowContinuation(continuation.Metadata)
			metadata["user_input_request"] = map[string]interface{}(userInput)
			continuation.Metadata = metadata
		}
	}
	if emit != nil {
		emitAgentWorkflowContinuationEvent(emit, streamEvent)
		if len(userInput) > 0 {
			h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "user_input_requested", userInput, emit)
		}
	}
	if isAgentWorkflowPassthroughMessageEvent(eventType, continuation.AgentType) {
		chunk := agentWorkflowContinuationMessageChunk(data)
		if chunk != "" {
			result.HasWorkflowMessage = true
			result.WorkflowMessageText = chunk
			if eventType != "message" && emit != nil {
				h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message", gin.H{
					"conversation_id": continuation.ConversationID.String(),
					"message_id":      continuation.MessageID.String(),
					"answer":          chunk,
				}, emit)
			}
		}
	}
	if eventType == "workflow_finished" || eventType == "workflow_failed" {
		result.Terminal = true
	}
	if eventType == "approval_requested" {
		result.WaitingStatus = runtimemodel.MessageStatusWaitingApproval
	}
	if eventType == workflowpause.EventQuestionAnswerRequested {
		result.WaitingStatus = runtimemodel.MessageStatusWaitingQuestion
	}
	return result
}

func (h *AgentsHandler) finishAgentWorkflowContinuationIfRunTerminal(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, passthroughAnswer string, hasPassthroughAnswer bool, emit func(runtimeservice.StreamEvent) error) bool {
	run, err := h.loadAgentWorkflowRunLog(ctx, continuation.WorkflowRunID)
	if err != nil {
		logger.WarnContext(ctx, "failed to load workflow continuation run status", "workflow_run_id", continuation.WorkflowRunID, err)
		return false
	}
	if !agentWorkflowRunLogTerminal(run.Status) {
		return false
	}
	h.finishAgentWorkflowContinuation(ctx, scope, continuation, passthroughAnswer, hasPassthroughAnswer, emit)
	return true
}

func (h *AgentsHandler) finishAgentWorkflowContinuation(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, passthroughAnswer string, hasPassthroughAnswer bool, emit func(runtimeservice.StreamEvent) error) {
	run, err := h.loadAgentWorkflowRunLog(ctx, continuation.WorkflowRunID)
	if err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	outputs := run.OutputsMap()
	if hasPassthroughAnswer && strings.EqualFold(strings.TrimSpace(continuation.AgentType), "CONVERSATIONAL_WORKFLOW") {
		metadata, err := h.chatRuntimeService.CompleteWorkflowApprovalContinuation(ctx, continuation, passthroughAnswer, completionContinuationStatus(run.Status))
		if err != nil {
			h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
			return
		}
		h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
			"conversation_id": continuation.ConversationID.String(),
			"message_id":      continuation.MessageID.String(),
			"status":          runtimemodel.MessageStatusCompleted,
			"metadata":        metadata,
		}, emit)
		return
	}
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
		}, func(event runtimeservice.StreamEvent) error {
			emitAgentWorkflowContinuationEvent(emit, &event)
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
		h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
			"conversation_id": continuation.ConversationID.String(),
			"message_id":      continuation.MessageID.String(),
			"status":          runtimemodel.MessageStatusCompleted,
			"metadata":        metadata,
		}, emit)
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
		h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message", gin.H{
			"conversation_id": continuation.ConversationID.String(),
			"message_id":      continuation.MessageID.String(),
			"answer":          answer,
		}, emit)
	}
	metadata, err := h.chatRuntimeService.CompleteWorkflowApprovalContinuation(ctx, continuation, answer, completionContinuationStatus(run.Status))
	if err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"status":          runtimemodel.MessageStatusCompleted,
		"metadata":        metadata,
	}, emit)
}

func (h *AgentsHandler) pauseAgentWorkflowContinuation(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, status string, emit func(runtimeservice.StreamEvent) error) {
	metadata, err := h.chatRuntimeService.PauseWorkflowApprovalContinuation(ctx, continuation, status)
	if err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"status":          status,
		"metadata":        metadata,
	}, emit)
}

func (h *AgentsHandler) failAgentWorkflowContinuation(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, cause error, emit func(runtimeservice.StreamEvent) error) {
	message := "workflow continuation timed out before completion"
	if cause != nil && !errors.Is(cause, context.DeadlineExceeded) {
		message = fmt.Sprintf("workflow continuation stopped before completion: %v", cause)
	}
	metadata, err := h.chatRuntimeService.FailWorkflowApprovalContinuation(ctx, continuation, message)
	if err != nil {
		emitAgentWorkflowContinuationError(emit, err)
		return
	}
	h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "error", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"message":         message,
	}, emit)
	h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"status":          runtimemodel.MessageStatusError,
		"metadata":        metadata,
	}, emit)
}

func (h *AgentsHandler) emitAgentWorkflowContinuationStreamEvent(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, eventType string, payload gin.H, emit func(runtimeservice.StreamEvent) error) *runtimeservice.StreamEvent {
	event, err := h.chatRuntimeService.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, eventType, payload)
	if err != nil {
		logger.WarnContext(ctx, "failed to append workflow continuation stream event", "workflow_run_id", continuation.WorkflowRunID, "event_type", eventType, err)
		event = &runtimeservice.StreamEvent{
			EventType: eventType,
			Payload:   payload,
			CreatedAt: time.Now().Unix(),
		}
	}
	emitAgentWorkflowContinuationEvent(emit, event)
	return event
}

func emitAgentWorkflowContinuationEvent(emit func(runtimeservice.StreamEvent) error, event *runtimeservice.StreamEvent) {
	if emit == nil || event == nil {
		return
	}
	_ = emit(*event)
}

func emitAgentWorkflowContinuationError(emit func(runtimeservice.StreamEvent) error, err error) {
	if err == nil {
		return
	}
	emitAgentWorkflowContinuationEvent(emit, &runtimeservice.StreamEvent{
		EventType: "error",
		Payload:   gin.H{"message": err.Error()},
		CreatedAt: time.Now().Unix(),
	})
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

func agentWorkflowContinuationRunIDFromMetadata(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	state, ok := metadata["agent_workflow_continuation"].(map[string]interface{})
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringFromAgentWorkflowContinuation(state["workflow_run_id"]))
}

func agentWorkflowRunLogTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "failed", "stopped", "expired", "partial-succeeded":
		return true
	default:
		return false
	}
}

func agentWorkflowRunLogFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "stopped", "expired":
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
	case workflowpause.EventApprovalResultFilled:
		return workflowpause.EventApprovalResultFilled
	case workflowpause.EventApprovalExpired:
		return workflowpause.EventApprovalExpired
	case workflowpause.EventQuestionAnswerRequested:
		return workflowpause.EventQuestionAnswerRequested
	case workflowpause.EventQuestionAnswerSubmitted:
		return workflowpause.EventQuestionAnswerSubmitted
	case workflowpause.EventWorkflowFinished:
		return "workflow_finished"
	case workflowpause.EventError:
		return "workflow_failed"
	case "iteration_started", "iteration_next", "iteration_completed", "iteration_succeeded", "iteration_failed",
		"loop_started", "loop_next", "loop_completed", "loop_succeeded", "loop_failed",
		"message", "text_chunk", "message_end", "workflow_stopped":
		return strings.TrimSpace(eventType)
	default:
		return ""
	}
}

func isAgentWorkflowPassthroughMessageEvent(eventType string, agentType string) bool {
	if !strings.EqualFold(strings.TrimSpace(agentType), "CONVERSATIONAL_WORKFLOW") {
		return false
	}
	switch strings.TrimSpace(eventType) {
	case "message", "text_chunk":
		return true
	default:
		return false
	}
}

func agentWorkflowContinuationMessageChunk(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	if answer := strings.TrimSpace(stringFromAgentWorkflowContinuation(payload["answer"])); answer != "" {
		return answer
	}
	if text := strings.TrimSpace(stringFromAgentWorkflowContinuation(payload["text"])); text != "" {
		return text
	}
	if data, ok := payload["data"].(map[string]interface{}); ok {
		if text := strings.TrimSpace(stringFromAgentWorkflowContinuation(data["text"])); text != "" {
			return text
		}
		if answer := strings.TrimSpace(stringFromAgentWorkflowContinuation(data["answer"])); answer != "" {
			return answer
		}
	}
	return ""
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
	status := strings.TrimSpace(fmt.Sprint(state["status"]))
	return strings.EqualFold(status, "waiting_approval") || strings.EqualFold(status, "waiting_question")
}

func agentWorkflowQuestionUserInputEvent(continuation *runtimeservice.WorkflowApprovalContinuation, data map[string]interface{}) gin.H {
	if continuation == nil || len(data) == 0 {
		return nil
	}
	question := strings.TrimSpace(stringFromAgentWorkflowContinuation(data["question"]))
	if question == "" {
		return nil
	}
	workflowRunID := continuation.WorkflowRunID
	if value := strings.TrimSpace(stringFromAgentWorkflowContinuation(data["workflow_run_id"])); value != "" {
		workflowRunID = value
	}
	nodeID := strings.TrimSpace(stringFromAgentWorkflowContinuation(data["node_id"]))
	round := data["round"]
	requestID := strings.Trim(strings.Join([]string{workflowRunID, nodeID, strings.TrimSpace(fmt.Sprint(round))}, ":"), ":")
	item := gin.H{
		"id":       "answer",
		"question": question,
	}
	if options := agentWorkflowQuestionOptions(data["choices"]); len(options) > 0 {
		item["options"] = options
	}
	return gin.H{
		"source":          "agent_workflow_question_answer",
		"request_id":      requestID,
		"workflow_run_id": workflowRunID,
		"node_id":         nodeID,
		"round":           round,
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"questions":       []gin.H{item},
		"created_at":      time.Now().Unix(),
	}
}

func agentWorkflowQuestionOptions(value interface{}) []gin.H {
	var items []interface{}
	switch typed := value.(type) {
	case []interface{}:
		items = typed
	case []map[string]interface{}:
		items = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
	default:
		return nil
	}
	options := make([]gin.H, 0, len(items))
	for index, item := range items {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		label := firstNonEmptyAgentWorkflowContinuationString(record["label"], record["value"], record["id"])
		if label == "" {
			continue
		}
		optionID := firstNonEmptyAgentWorkflowContinuationString(record["id"], record["option_id"])
		if optionID == "" {
			optionID = fmt.Sprintf("option_%d", index+1)
		}
		option := gin.H{
			"label":     label,
			"value":     firstNonEmptyAgentWorkflowContinuationString(record["value"], optionID, label),
			"option_id": optionID,
		}
		if description := firstNonEmptyAgentWorkflowContinuationString(record["description"]); description != "" {
			option["description"] = description
		}
		options = append(options, option)
	}
	return options
}

func firstNonEmptyAgentWorkflowContinuationString(values ...interface{}) string {
	for _, value := range values {
		if text := strings.TrimSpace(stringFromAgentWorkflowContinuation(value)); text != "" {
			return text
		}
	}
	return ""
}

func stringFromAgentWorkflowContinuation(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
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
