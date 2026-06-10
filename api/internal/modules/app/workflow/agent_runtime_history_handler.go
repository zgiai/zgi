package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

type agentRuntimeLookupRepository interface {
	GetByID(ctx context.Context, id string) (*agents.Agent, error)
}

type AgentHistoryDispatchHandler struct {
	agentsRepo      agentRuntimeLookupRepository
	workflowRuns    *WorkflowHandler
	workflowHistory *AgentWorkflowHistoryHandler
	workflowRuntime *RuntimeLogHandler
	chatRuntime     runtimeservice.Service
}

func NewAgentHistoryDispatchHandler(
	agentsRepo agentRuntimeLookupRepository,
	workflowRuns *WorkflowHandler,
	workflowHistory *AgentWorkflowHistoryHandler,
	workflowRuntime *RuntimeLogHandler,
	chatRuntime runtimeservice.Service,
) *AgentHistoryDispatchHandler {
	return &AgentHistoryDispatchHandler{
		agentsRepo:      agentsRepo,
		workflowRuns:    workflowRuns,
		workflowHistory: workflowHistory,
		workflowRuntime: workflowRuntime,
		chatRuntime:     chatRuntime,
	}
}

func (h *AgentHistoryDispatchHandler) GetWorkflowRuns(c *gin.Context) {
	if h.isRuntimeAgent(c) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	h.workflowRuns.GetWorkflowRuns(c)
}

func (h *AgentHistoryDispatchHandler) GetWorkflowRunDetail(c *gin.Context) {
	if h.isRuntimeAgent(c) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	h.workflowRuns.GetWorkflowRunDetail(c)
}

func (h *AgentHistoryDispatchHandler) GetWorkflowRunNodeExecutions(c *gin.Context) {
	if h.isRuntimeAgent(c) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	h.workflowRuns.GetWorkflowRunNodeExecutions(c)
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

func (h *AgentHistoryDispatchHandler) getRuntimeWorkflowRuns(c *gin.Context) {
	scope, agentID, ok := h.runtimeScope(c)
	if !ok {
		return
	}
	req := dto.WorkflowRunsRequest{Page: 1, Limit: 20}
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	page := req.Page
	limit := req.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	source := runtimeConversationSourceFromTriggeredFrom(req.TriggeredFrom)
	messages, total, err := h.chatRuntime.ListMessagesByCallerSource(c.Request.Context(), scope, h.runtimeCaller(agentID), source, page, limit)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	items := make([]dto.WorkflowRunLogResponse, 0, len(messages))
	for _, message := range messages {
		items = append(items, buildRuntimeWorkflowRunLogResponse(message))
	}
	response.Success(c, dto.WorkflowRunsResponse{
		Limit:   limit,
		HasMore: int64(page*limit) < total,
		Data:    items,
	})
}

func (h *AgentHistoryDispatchHandler) getRuntimeWorkflowRunDetail(c *gin.Context) {
	scope, agentID, ok := h.runtimeScope(c)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("run_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	message, conversation, err := h.chatRuntime.GetMessageByCaller(c.Request.Context(), scope, h.runtimeCaller(agentID), messageID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	if !isRuntimeWebAppConversation(conversation) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.Success(c, buildRuntimeWorkflowRunDetailResponse(message))
}

func (h *AgentHistoryDispatchHandler) getRuntimeWorkflowRunNodeExecutions(c *gin.Context) {
	scope, agentID, ok := h.runtimeScope(c)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("run_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	message, conversation, err := h.chatRuntime.GetMessageByCaller(c.Request.Context(), scope, h.runtimeCaller(agentID), messageID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	if !isRuntimeWebAppConversation(conversation) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.Success(c, dto.WorkflowRunNodeExecutionListResponse{
		Data: buildRuntimeNodeExecutionResponses(message),
	})
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
	source := runtimeConversationSourceFromTriggeredFrom(req.TriggeredFrom)
	messages, total, err := h.chatRuntime.ListMessagesByCallerSource(c.Request.Context(), scope, h.runtimeCaller(agentID), source, page, limit)
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

func buildRuntimeWorkflowRunLogResponse(message *runtimemodel.Message) dto.WorkflowRunLogResponse {
	metadata := runtimeMetadata(message)
	createdAt := message.CreatedAt.Unix()
	finishedAt := runtimeFinishedAtUnix(message)
	conversationID := message.ConversationID.String()
	messageID := message.ID.String()
	return dto.WorkflowRunLogResponse{
		ID:              message.ID.String(),
		SequenceNumber:  0,
		Version:         metadataString(metadata, "system_prompt_version", "chat_runtime"),
		TriggeredFrom:   string(CreatedFromWebApp),
		Status:          runtimeWorkflowStatus(message.Status),
		ElapsedTime:     runtimeElapsedTime(message),
		TotalTokens:     int64(metadataTotalTokens(metadata)),
		TotalSteps:      runtimeTotalSteps(metadata),
		ConversationID:  &conversationID,
		MessageID:       &messageID,
		CreatedAt:       createdAt,
		FinishedAt:      finishedAt,
		ExceptionsCount: runtimeExceptionCount(message),
		RetryIndex:      0,
	}
}

func buildRuntimeWorkflowRunDetailResponse(message *runtimemodel.Message) dto.WorkflowRunDetailResponse {
	metadata := runtimeMetadata(message)
	conversationID := message.ConversationID.String()
	messageID := message.ID.String()
	return dto.WorkflowRunDetailResponse{
		ID:              message.ID.String(),
		SequenceNumber:  0,
		Version:         metadataString(metadata, "system_prompt_version", "chat_runtime"),
		Graph:           map[string]interface{}{},
		Features:        map[string]interface{}{},
		Inputs:          map[string]interface{}{"query": message.Query},
		Status:          runtimeWorkflowStatus(message.Status),
		Outputs:         map[string]interface{}{"answer": message.Answer},
		Error:           runtimeErrorString(message),
		ElapsedTime:     runtimeElapsedTime(message),
		TotalTokens:     int64(metadataTotalTokens(metadata)),
		TotalSteps:      runtimeTotalSteps(metadata),
		ConversationID:  &conversationID,
		MessageID:       &messageID,
		CreatedByRole:   string(CreatedByRoleAccount),
		CreatedAt:       message.CreatedAt.Unix(),
		FinishedAt:      runtimeFinishedAtUnix(message),
		ExceptionsCount: runtimeExceptionCount(message),
	}
}

func buildRuntimeNodeExecutionResponses(message *runtimemodel.Message) []dto.WorkflowRunNodeExecutionResponse {
	metadata := runtimeMetadata(message)
	invocations := runtimeSkillInvocations(metadata["skill_invocations"])
	items := make([]dto.WorkflowRunNodeExecutionResponse, 0, len(invocations)+1)
	for index, invocation := range invocations {
		items = append(items, runtimeInvocationNodeExecution(message, invocation, index+1))
	}
	items = append(items, runtimeFinalAnswerNodeExecution(message, len(items)+1))
	return items
}

func runtimeInvocationNodeExecution(message *runtimemodel.Message, invocation map[string]interface{}, index int) dto.WorkflowRunNodeExecutionResponse {
	status := runtimeInvocationStatus(runtimeString(invocation["status"]))
	errText := runtimeString(invocation["error"])
	if errText == "" && status == string(dto.NodeStatusFailed) {
		errText = runtimeString(invocation["message"])
	}
	outputs := map[string]interface{}{}
	if result := runtimeMap(invocation["result"]); len(result) > 0 {
		outputs["result"] = result
	}
	if text := runtimeString(invocation["message"]); text != "" {
		outputs["message"] = text
	}
	if outputs["result"] == nil && outputs["message"] == nil {
		outputs["status"] = runtimeString(invocation["status"])
	}
	return dto.WorkflowRunNodeExecutionResponse{
		ID:                runtimeInvocationID(message.ID.String(), invocation, index),
		Index:             index,
		NodeID:            runtimeInvocationID(message.ID.String(), invocation, index),
		NodeType:          runtimeInvocationNodeType(invocation),
		Title:             runtimeInvocationTitle(invocation),
		TriggeredFrom:     string(dto.TriggeredFromWorkflowRun),
		Inputs:            runtimeRawMessage(runtimeMap(invocation["arguments"])),
		ProcessData:       runtimeRawMessage(map[string]interface{}{"kind": runtimeString(invocation["kind"]), "skill_id": runtimeString(invocation["skill_id"]), "tool_name": runtimeString(invocation["tool_name"])}),
		Outputs:           runtimeRawMessage(outputs),
		Status:            status,
		Error:             errText,
		ElapsedTime:       metadataNumber(invocation, "duration_ms"),
		ExecutionMetadata: runtimeRawMessage(map[string]interface{}{}),
		CreatedAt:         runtimeInvocationCreatedAt(message, invocation),
		CreatedByRole:     string(CreatedByRoleAccount),
		FinishedAt:        runtimeInvocationFinishedAt(message, invocation, status),
	}
}

func runtimeFinalAnswerNodeExecution(message *runtimemodel.Message, index int) dto.WorkflowRunNodeExecutionResponse {
	metadata := runtimeMetadata(message)
	status := runtimeWorkflowStatus(message.Status)
	errText := runtimeErrorString(message)
	outputs := map[string]interface{}{"answer": message.Answer}
	processData := map[string]interface{}{
		"model":          message.ModelName,
		"model_provider": message.ModelProvider,
		"usage":          metadata["usage"],
	}
	return dto.WorkflowRunNodeExecutionResponse{
		ID:                message.ID.String() + ":answer",
		Index:             index,
		NodeID:            "answer",
		NodeType:          "answer",
		Title:             "Final Answer",
		TriggeredFrom:     string(dto.TriggeredFromWorkflowRun),
		Inputs:            runtimeRawMessage(map[string]interface{}{"query": message.Query}),
		ProcessData:       runtimeRawMessage(processData),
		Outputs:           runtimeRawMessage(outputs),
		Status:            status,
		Error:             errText,
		ElapsedTime:       runtimeElapsedTime(message),
		ExecutionMetadata: runtimeRawMessage(map[string]interface{}{"total_tokens": metadataTotalTokens(metadata)}),
		CreatedAt:         message.CreatedAt,
		CreatedByRole:     string(CreatedByRoleAccount),
		FinishedAt:        runtimeFinishedAtTime(message),
	}
}

func runtimeConversationSourceFromTriggeredFrom(triggeredFrom string) string {
	switch strings.TrimSpace(triggeredFrom) {
	case string(CreatedFromWebApp):
		return runtimemodel.ConversationSourceWebApp
	default:
		return ""
	}
}

func isRuntimeWebAppConversation(conversation *runtimemodel.Conversation) bool {
	return conversation != nil &&
		conversation.Source == runtimemodel.ConversationSourceWebApp &&
		conversation.SourceWebAppID != nil &&
		*conversation.SourceWebAppID != uuid.Nil
}

func runtimeMetadata(message *runtimemodel.Message) map[string]interface{} {
	if message == nil || message.Metadata == nil {
		return map[string]interface{}{}
	}
	return message.Metadata
}

func runtimeWorkflowStatus(status string) string {
	switch strings.TrimSpace(status) {
	case runtimemodel.MessageStatusPending, runtimemodel.MessageStatusStreaming:
		return string(dto.WorkflowRunStatusRunning)
	case runtimemodel.MessageStatusWaitingApproval:
		return "pending_approval"
	case runtimemodel.MessageStatusWaitingQuestion:
		return "pending_question"
	case runtimemodel.MessageStatusCompleted:
		return string(dto.WorkflowRunStatusSucceeded)
	case runtimemodel.MessageStatusStopped:
		return string(dto.WorkflowRunStatusStopped)
	case runtimemodel.MessageStatusError:
		return string(dto.WorkflowRunStatusFailed)
	default:
		return status
	}
}

func runtimeInvocationStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "loading", "running", "streaming":
		return string(dto.NodeStatusRunning)
	case "success", "succeeded", "completed":
		return string(dto.NodeStatusSucceeded)
	case "pending_approval", "waiting_approval":
		return "pending_approval"
	case "pending_question", "waiting_question":
		return "pending_question"
	case "paused":
		return string(dto.NodeStatusPaused)
	case "error", "failed", "blocked":
		return string(dto.NodeStatusFailed)
	default:
		if status == "" {
			return string(dto.NodeStatusSucceeded)
		}
		return status
	}
}

func runtimeElapsedTime(message *runtimemodel.Message) float64 {
	if message == nil {
		return 0
	}
	if elapsed := metadataNumber(runtimeMetadata(message), "elapsed_time_ms"); elapsed > 0 {
		return elapsed
	}
	if runtimeFinishedAtTime(message) != nil {
		return durationMilliseconds(message.UpdatedAt.Sub(message.CreatedAt))
	}
	return 0
}

func runtimeFinishedAtUnix(message *runtimemodel.Message) *int64 {
	finishedAt := runtimeFinishedAtTime(message)
	if finishedAt == nil {
		return nil
	}
	unix := finishedAt.Unix()
	return &unix
}

func runtimeFinishedAtTime(message *runtimemodel.Message) *time.Time {
	if message == nil {
		return nil
	}
	switch message.Status {
	case runtimemodel.MessageStatusCompleted, runtimemodel.MessageStatusError, runtimemodel.MessageStatusStopped:
		finishedAt := message.UpdatedAt
		return &finishedAt
	default:
		return nil
	}
}

func runtimeErrorString(message *runtimemodel.Message) string {
	if message == nil || message.Error == nil {
		return ""
	}
	return *message.Error
}

func runtimeTotalSteps(metadata map[string]interface{}) int {
	count := metadataInt(metadata, "skill_step_count")
	if count == 0 {
		count = len(runtimeSkillInvocations(metadata["skill_invocations"]))
	}
	return count + 1
}

func runtimeSkillInvocations(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, copyRuntimeMap(item))
		}
		return out
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]interface{}); ok {
				out = append(out, copyRuntimeMap(mapped))
			}
		}
		return out
	default:
		return nil
	}
}

func runtimeInvocationID(messageID string, invocation map[string]interface{}, index int) string {
	if runtimeID := runtimeString(invocation["runtime_id"]); runtimeID != "" {
		return messageID + ":" + runtimeID
	}
	return fmt.Sprintf("%s:step-%d", messageID, index)
}

func runtimeInvocationNodeType(invocation map[string]interface{}) string {
	if kind := runtimeString(invocation["kind"]); kind != "" {
		return kind
	}
	return "agent_step"
}

func runtimeInvocationTitle(invocation map[string]interface{}) string {
	if title := runtimeString(invocation["title"]); title != "" {
		return title
	}
	skillID := runtimeString(invocation["skill_id"])
	toolName := runtimeString(invocation["tool_name"])
	switch {
	case skillID != "" && toolName != "":
		return skillID + " / " + toolName
	case toolName != "":
		return toolName
	case skillID != "":
		return skillID
	default:
		return runtimeInvocationNodeType(invocation)
	}
}

func runtimeInvocationCreatedAt(message *runtimemodel.Message, invocation map[string]interface{}) time.Time {
	if createdAt := metadataNumber(invocation, "created_at"); createdAt > 0 {
		return time.Unix(int64(createdAt), 0)
	}
	return message.CreatedAt
}

func runtimeInvocationFinishedAt(message *runtimemodel.Message, invocation map[string]interface{}, status string) *time.Time {
	if status == string(dto.NodeStatusRunning) {
		return nil
	}
	createdAt := runtimeInvocationCreatedAt(message, invocation)
	if durationMs := metadataNumber(invocation, "duration_ms"); durationMs > 0 {
		finishedAt := createdAt.Add(time.Duration(durationMs * float64(time.Millisecond)))
		return &finishedAt
	}
	finishedAt := createdAt
	return &finishedAt
}

func runtimeRawMessage(value interface{}) json.RawMessage {
	if value == nil {
		return json.RawMessage("{}")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

func runtimeMap(value interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	if mapped, ok := value.(map[string]interface{}); ok {
		return copyRuntimeMap(mapped)
	}
	return map[string]interface{}{"value": value}
}

func copyRuntimeMap(source map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func runtimeString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
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
	total := 0
	for _, invocation := range runtimeSkillInvocations(metadata["model_invocations"]) {
		if usage := runtimeMap(invocation["usage"]); len(usage) > 0 {
			total += metadataFirstInt(usage, "total_tokens", "totalTokens", "TotalTokens")
			continue
		}
		total += metadataFirstInt(invocation, "total_tokens", "totalTokens", "TotalTokens")
	}
	if total > 0 {
		return total
	}
	usage, ok := metadata["usage"].(map[string]interface{})
	if !ok {
		return 0
	}
	return metadataInt(usage, "total_tokens")
}

func metadataFirstInt(metadata map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if value := metadataInt(metadata, key); value > 0 {
			return value
		}
	}
	return 0
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
