package workflow

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

type AgentHistoryDispatchHandler struct {
	agentsRepo      agents.AgentsRepository
	workflowHistory *AgentWorkflowHistoryHandler
	workflowRuntime *RuntimeLogHandler
	chatRuntime     runtimeservice.Service
}

func NewAgentHistoryDispatchHandler(
	agentsRepo agents.AgentsRepository,
	workflowHistory *AgentWorkflowHistoryHandler,
	workflowRuntime *RuntimeLogHandler,
	chatRuntime runtimeservice.Service,
) *AgentHistoryDispatchHandler {
	return &AgentHistoryDispatchHandler{
		agentsRepo:      agentsRepo,
		workflowHistory: workflowHistory,
		workflowRuntime: workflowRuntime,
		chatRuntime:     chatRuntime,
	}
}

func (h *AgentHistoryDispatchHandler) GetConversations(c *gin.Context) {
	if h.isRuntimeAgent(c) {
		h.getRuntimeConversations(c)
		return
	}
	h.workflowHistory.GetConversations(c)
}

func (h *AgentHistoryDispatchHandler) GetConversationDetail(c *gin.Context) {
	if h.isRuntimeAgent(c) {
		h.getRuntimeConversationDetail(c)
		return
	}
	h.workflowHistory.GetConversationDetail(c)
}

func (h *AgentHistoryDispatchHandler) GetChatMessages(c *gin.Context) {
	if h.isRuntimeAgent(c) {
		h.getRuntimeChatMessages(c)
		return
	}
	h.workflowHistory.GetChatMessages(c)
}

func (h *AgentHistoryDispatchHandler) GetRuntimeLogs(c *gin.Context) {
	if h.isRuntimeAgent(c) {
		h.getRuntimeLogs(c)
		return
	}
	h.workflowRuntime.GetRuntimeLogs(c)
}

func (h *AgentHistoryDispatchHandler) isRuntimeAgent(c *gin.Context) bool {
	if h == nil || h.agentsRepo == nil || h.chatRuntime == nil {
		return false
	}
	agentID := strings.TrimSpace(c.Param("agent_id"))
	if agentID == "" {
		return false
	}
	ag, err := h.agentsRepo.GetByID(c.Request.Context(), agentID)
	if err != nil {
		return false
	}
	return ag.AgentsType == "AGENT"
}

func (h *AgentHistoryDispatchHandler) runtimeScope(c *gin.Context) (runtimeservice.Scope, uuid.UUID, bool) {
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return runtimeservice.Scope{}, uuid.Nil, false
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return runtimeservice.Scope{}, uuid.Nil, false
	}
	agentID, err := uuid.Parse(strings.TrimSpace(c.Param("agent_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return runtimeservice.Scope{}, uuid.Nil, false
	}

	var workspaceID *uuid.UUID
	if h.agentsRepo != nil {
		if ag, err := h.agentsRepo.GetByID(c.Request.Context(), agentID.String()); err == nil {
			value := ag.TenantID
			workspaceID = &value
		}
	}
	if workspaceID == nil {
		if raw := strings.TrimSpace(util.GetWorkspaceID(c)); raw != "" {
			if parsed, err := uuid.Parse(raw); err == nil {
				workspaceID = &parsed
			}
		}
	}

	return runtimeservice.Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    workspaceID,
	}, agentID, true
}

func (h *AgentHistoryDispatchHandler) runtimeCaller(agentID uuid.UUID) runtimeservice.Caller {
	return runtimeservice.Caller{
		Type: runtimemodel.ConversationCallerAgent,
		ID:   &agentID,
	}
}

func (h *AgentHistoryDispatchHandler) getRuntimeConversations(c *gin.Context) {
	scope, agentID, ok := h.runtimeScope(c)
	if !ok {
		return
	}
	page := parsePositiveInt(c.Query("page"), 1)
	limit := parsePositiveInt(c.Query("limit"), 20)
	conversations, total, err := h.chatRuntime.ListConversationsByCaller(c.Request.Context(), scope, h.runtimeCaller(agentID), page, limit)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	items := make([]AgentConversationListItem, 0, len(conversations))
	for _, conversation := range conversations {
		items = append(items, buildRuntimeConversationListItem(conversation))
	}
	response.Success(c, AgentConversationListResponse{
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: int64(page*limit) < total,
		Data:    items,
	})
}

func (h *AgentHistoryDispatchHandler) getRuntimeConversationDetail(c *gin.Context) {
	scope, agentID, ok := h.runtimeScope(c)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(c.Param("conversation_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	conversation, err := h.chatRuntime.GetConversationByCaller(c.Request.Context(), scope, h.runtimeCaller(agentID), conversationID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, buildRuntimeConversationDetailResponse(conversation))
}

func (h *AgentHistoryDispatchHandler) getRuntimeChatMessages(c *gin.Context) {
	scope, agentID, ok := h.runtimeScope(c)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(c.Query("conversation_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, err := h.chatRuntime.GetConversationByCaller(c.Request.Context(), scope, h.runtimeCaller(agentID), conversationID); err != nil {
		h.failRuntime(c, err)
		return
	}
	page := parsePositiveInt(c.Query("page"), 1)
	limit := parsePositiveInt(c.Query("limit"), 20)
	messages, total, err := h.chatRuntime.ListMessages(c.Request.Context(), scope, conversationID, page, limit)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	response.Success(c, buildRuntimeChatMessagesResponse(messages, total, page, limit))
}

func (h *AgentHistoryDispatchHandler) getRuntimeLogs(c *gin.Context) {
	scope, agentID, ok := h.runtimeScope(c)
	if !ok {
		return
	}
	var req RuntimeLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = RuntimeLogsRequest{}
	}
	page := req.Page
	limit := req.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	messages, total, err := h.chatRuntime.ListMessagesByCaller(c.Request.Context(), scope, h.runtimeCaller(agentID), page, limit)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	items := make([]map[string]interface{}, 0, len(messages))
	for _, message := range messages {
		items = append(items, buildRuntimeLogItem(message))
	}
	response.Success(c, map[string]interface{}{
		"data":     items,
		"page":     page,
		"limit":    limit,
		"total":    total,
		"has_more": int64(page*limit) < total,
	})
}

func (h *AgentHistoryDispatchHandler) failRuntime(c *gin.Context, err error) {
	switch {
	case errors.Is(err, runtimeservice.ErrNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, runtimeservice.ErrInvalidInput):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	default:
		logger.ErrorContext(c.Request.Context(), "agent chat runtime history failed", err)
		response.Fail(c, response.ErrSystemError)
	}
}

func buildRuntimeConversationListItem(conversation *runtimemodel.Conversation) AgentConversationListItem {
	return AgentConversationListItem{
		ID:                 conversation.ID.String(),
		Status:             conversation.Status,
		FromSource:         conversation.Source,
		InvokeFrom:         stringPtr(conversation.Source),
		FromEndUserID:      nil,
		FromAccountID:      stringPtr(conversation.AccountID.String()),
		FromAccountName:    nil,
		Name:               conversation.Title,
		Summary:            nil,
		ReadAt:             nil,
		CreatedAt:          conversation.CreatedAt.Unix(),
		UpdatedAt:          conversation.UpdatedAt.Unix(),
		Annotated:          false,
		ModelConfig:        runtimeConversationModelConfig(conversation.Metadata),
		MessageCount:       conversation.DialogueCount,
		UserFeedbackStats:  defaultAgentConversationStats(),
		AdminFeedbackStats: defaultAgentConversationStats(),
	}
}

func buildRuntimeConversationDetailResponse(conversation *runtimemodel.Conversation) AgentConversationDetailResponse {
	return AgentConversationDetailResponse{
		ID:                 conversation.ID.String(),
		Status:             conversation.Status,
		FromSource:         conversation.Source,
		InvokeFrom:         stringPtr(conversation.Source),
		FromEndUserID:      nil,
		FromAccountID:      stringPtr(conversation.AccountID.String()),
		FromAccountName:    nil,
		Name:               conversation.Title,
		Summary:            nil,
		ReadAt:             nil,
		CreatedAt:          conversation.CreatedAt.Unix(),
		UpdatedAt:          conversation.UpdatedAt.Unix(),
		Annotated:          false,
		Introduction:       nil,
		ModelConfig:        runtimeConversationModelConfig(conversation.Metadata),
		MessageCount:       conversation.DialogueCount,
		UserFeedbackStats:  defaultAgentConversationStats(),
		AdminFeedbackStats: defaultAgentConversationStats(),
	}
}

func buildRuntimeChatMessagesResponse(messages []*runtimemodel.Message, total int64, page, limit int) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(messages))
	for _, message := range messages {
		items = append(items, map[string]interface{}{
			"id":                message.ID.String(),
			"conversation_id":   message.ConversationID.String(),
			"parent_message_id": uuidPointerString(message.ParentID),
			"query":             message.Query,
			"answer":            message.Answer,
			"status":            message.Status,
			"error":             message.Error,
			"model_provider":    message.ModelProvider,
			"model_name":        message.ModelName,
			"created_at":        message.CreatedAt.Unix(),
			"message_metadata":  message.Metadata,
		})
	}
	return map[string]interface{}{
		"data":     items,
		"page":     page,
		"limit":    limit,
		"total":    total,
		"has_more": int64(page*limit) < total,
	}
}

func buildRuntimeLogItem(message *runtimemodel.Message) map[string]interface{} {
	metadata := message.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	item := map[string]interface{}{
		"id":               message.ID.String(),
		"conversation_id":  message.ConversationID.String(),
		"type":             "agent",
		"triggered_from":   runtimemodel.ConversationCallerAgent,
		"version":          metadataString(metadata, "system_prompt_version", "chat_runtime"),
		"status":           message.Status,
		"elapsed_time":     metadataNumber(metadata, "elapsed_time_ms"),
		"total_tokens":     metadataTotalTokens(metadata),
		"total_steps":      metadataInt(metadata, "skill_step_count"),
		"created_by_role":  "account",
		"created_at":       message.CreatedAt.Unix(),
		"exceptions_count": runtimeExceptionCount(message),
		"outputs": map[string]interface{}{
			"answer": message.Answer,
		},
		"message_id":          message.ID.String(),
		"query":               message.Query,
		"skill_invocations":   metadata["skill_invocations"],
		"generated_files":     metadata["generated_files"],
		"message_metadata":    metadata,
		"billing_app_context": metadata["billing_app_context"],
	}
	if message.Error != nil {
		item["error"] = *message.Error
	}
	return item
}

func runtimeConversationModelConfig(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return defaultAgentConversationModelConfig()
	}
	return map[string]interface{}{
		"model":      metadata["model"],
		"pre_prompt": metadata["system_prompt_version"],
	}
}

func runtimeExceptionCount(message *runtimemodel.Message) int {
	if message.Error != nil && strings.TrimSpace(*message.Error) != "" {
		return 1
	}
	if message.Status == runtimemodel.MessageStatusError {
		return 1
	}
	return 0
}

func metadataString(metadata map[string]interface{}, key, fallback string) string {
	if raw, ok := metadata[key].(string); ok && strings.TrimSpace(raw) != "" {
		return raw
	}
	return fallback
}

func metadataNumber(metadata map[string]interface{}, key string) float64 {
	switch raw := metadata[key].(type) {
	case float64:
		return raw
	case int:
		return float64(raw)
	case int64:
		return float64(raw)
	case json.Number:
		value, _ := raw.Float64()
		return value
	default:
		return 0
	}
}

func metadataInt(metadata map[string]interface{}, key string) int {
	return int(metadataNumber(metadata, key))
}

func metadataTotalTokens(metadata map[string]interface{}) int {
	usage, ok := metadata["usage"].(map[string]interface{})
	if !ok {
		return 0
	}
	return metadataInt(usage, "total_tokens")
}

func uuidPointerString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	return stringPtr(value.String())
}

func stringPtr(value string) *string {
	return &value
}
