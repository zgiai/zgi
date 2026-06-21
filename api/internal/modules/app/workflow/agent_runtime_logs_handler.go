package workflow

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

// AgentRuntimeLogsHandler exposes dedicated AGENT runtime log APIs.
type AgentRuntimeLogsHandler struct {
	agentsRepo        agentRuntimeLookupRepository
	chatRuntime       runtimeservice.Service
	enterpriseService runtimeLogWorkspacePermissionChecker
}

func NewAgentRuntimeLogsHandler(
	agentsRepo agentRuntimeLookupRepository,
	chatRuntime runtimeservice.Service,
	enterpriseService runtimeLogWorkspacePermissionChecker,
) *AgentRuntimeLogsHandler {
	return &AgentRuntimeLogsHandler{
		agentsRepo:        agentsRepo,
		chatRuntime:       chatRuntime,
		enterpriseService: enterpriseService,
	}
}

func (h *AgentRuntimeLogsHandler) GetRuntimeRuns(c *gin.Context) {
	scope, agentID, ok := h.runtimeScope(c)
	if !ok {
		return
	}

	var req dto.AgentRuntimeRunsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	source, sourceOK := normalizeAgentRuntimeLogSource(req.Source)
	if !sourceOK {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if source == "" {
		if strings.TrimSpace(req.TriggeredFrom) != "" && req.TriggeredFrom != string(CreatedFromWebApp) {
			response.Success(c, emptyAgentRuntimeRunsResponse(page, limit))
			return
		}
		source = runtimemodel.ConversationSourceWebApp
	}
	if source == runtimemodel.ConversationSourceWebApp && strings.TrimSpace(req.TriggeredFrom) != "" && req.TriggeredFrom != string(CreatedFromWebApp) {
		response.Success(c, emptyAgentRuntimeRunsResponse(page, limit))
		return
	}

	var (
		conversationID *uuid.UUID
		messages       []*runtimemodel.Message
		total          int64
		err            error
	)
	conversationIDText := strings.TrimSpace(req.ConversationID)
	if conversationIDText != "" {
		parsedConversationID, parseErr := uuid.Parse(conversationIDText)
		if parseErr != nil || parsedConversationID == uuid.Nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		conversationID = &parsedConversationID
	}
	messages, total, err = h.chatRuntime.ListMessagesByCallerRuntimeLogFilters(
		c.Request.Context(),
		scope,
		runtimeCaller(agentID),
		source,
		conversationID,
		req.Query,
		page,
		limit,
	)
	if err != nil {
		failAgentRuntimeLog(c, err)
		return
	}

	items := make([]dto.AgentRuntimeRunItem, 0, len(messages))
	for _, message := range messages {
		items = append(items, buildAgentRuntimeRunItem(message, nil, source))
	}
	response.Success(c, dto.AgentRuntimeRunsResponse{
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: int64(page*limit) < total,
		Data:    items,
	})
}

func normalizeAgentRuntimeLogSource(source string) (string, bool) {
	switch strings.TrimSpace(source) {
	case "", "web-app", runtimemodel.ConversationSourceWebApp:
		return runtimemodel.ConversationSourceWebApp, true
	case runtimemodel.ConversationSourceConsole:
		return runtimemodel.ConversationSourceConsole, true
	default:
		return "", false
	}
}

func emptyAgentRuntimeRunsResponse(page, limit int) dto.AgentRuntimeRunsResponse {
	return dto.AgentRuntimeRunsResponse{
		Page:    page,
		Limit:   limit,
		Data:    []dto.AgentRuntimeRunItem{},
		HasMore: false,
	}
}

func (h *AgentRuntimeLogsHandler) GetRuntimeRunDetail(c *gin.Context) {
	message, conversation, ok := h.runtimeMessage(c)
	if !ok {
		return
	}
	response.Success(c, buildAgentRuntimeRunDetail(message, conversation))
}

func (h *AgentRuntimeLogsHandler) GetRuntimeRunSteps(c *gin.Context) {
	message, conversation, ok := h.runtimeMessage(c)
	if !ok {
		return
	}
	if !isRuntimeLogConversation(conversation) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.Success(c, dto.AgentRuntimeStepsResponse{
		Data: buildAgentRuntimeSteps(message),
	})
}

func (h *AgentRuntimeLogsHandler) runtimeScope(c *gin.Context) (runtimeservice.Scope, uuid.UUID, bool) {
	if h == nil || h.agentsRepo == nil || h.chatRuntime == nil {
		response.Fail(c, response.ErrNotFound)
		return runtimeservice.Scope{}, uuid.Nil, false
	}

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

	agent, err := h.agentsRepo.GetByID(c.Request.Context(), agentID.String())
	if err != nil || agent == nil || agent.AgentsType != "AGENT" {
		response.Fail(c, response.ErrNotFound)
		return runtimeservice.Scope{}, uuid.Nil, false
	}
	workspaceID := agent.TenantID
	if h.enterpriseService == nil {
		response.Fail(c, response.ErrSystemError)
		return runtimeservice.Scope{}, uuid.Nil, false
	}
	hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
		c.Request.Context(),
		organizationID.String(),
		workspaceID.String(),
		accountID.String(),
		workspace_model.WorkspacePermissionAgentView,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return runtimeservice.Scope{}, uuid.Nil, false
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return runtimeservice.Scope{}, uuid.Nil, false
	}
	return runtimeservice.Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
	}, agentID, true
}

func (h *AgentRuntimeLogsHandler) runtimeMessage(c *gin.Context) (*runtimemodel.Message, *runtimemodel.Conversation, bool) {
	scope, agentID, ok := h.runtimeScope(c)
	if !ok {
		return nil, nil, false
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return nil, nil, false
	}
	message, conversation, err := h.chatRuntime.GetMessageByCallerRuntimeLog(c.Request.Context(), scope, runtimeCaller(agentID), messageID, runtimemodel.ConversationSourceWebApp)
	if err != nil {
		message, conversation, err = h.chatRuntime.GetMessageByCallerRuntimeLog(c.Request.Context(), scope, runtimeCaller(agentID), messageID, "")
	}
	if err != nil {
		failAgentRuntimeLog(c, err)
		return nil, nil, false
	}
	if !isRuntimeLogConversation(conversation) {
		response.Fail(c, response.ErrNotFound)
		return nil, nil, false
	}
	return message, conversation, true
}

func runtimeCaller(agentID uuid.UUID) runtimeservice.Caller {
	return runtimeservice.Caller{
		Type: runtimemodel.ConversationCallerAgent,
		ID:   &agentID,
	}
}

func buildAgentRuntimeRunItem(message *runtimemodel.Message, conversation *runtimemodel.Conversation, fallbackSource string) dto.AgentRuntimeRunItem {
	metadata := runtimeMetadata(message)
	return dto.AgentRuntimeRunItem{
		ID:             message.ID.String(),
		ConversationID: message.ConversationID.String(),
		Status:         runtimeWorkflowStatus(message.Status),
		Query:          message.Query,
		AnswerPreview:  truncateAgentRuntimeText(message.Answer, 160),
		ModelName:      message.ModelName,
		ModelProvider:  message.ModelProvider,
		ElapsedTime:    runtimeElapsedTime(message),
		TotalTokens:    int64(metadataTotalTokens(metadata)),
		TotalSteps:     agentRuntimeTotalSteps(metadata),
		CreatedAt:      message.CreatedAt.Unix(),
		FinishedAt:     runtimeFinishedAtUnix(message),
		Error:          runtimeErrorString(message),
		Source:         runtimeConversationSource(conversation, fallbackSource),
		SourceWebAppID: runtimeSourceWebAppID(conversation),
	}
}

func buildAgentRuntimeRunDetail(message *runtimemodel.Message, conversation *runtimemodel.Conversation) dto.AgentRuntimeRunDetail {
	metadata := runtimeMetadata(message)
	return dto.AgentRuntimeRunDetail{
		ID:              message.ID.String(),
		ConversationID:  message.ConversationID.String(),
		Status:          runtimeWorkflowStatus(message.Status),
		Query:           message.Query,
		Answer:          message.Answer,
		ModelName:       message.ModelName,
		ModelProvider:   message.ModelProvider,
		ModelParameters: message.ModelParameters,
		Usage:           metadata["usage"],
		ElapsedTime:     runtimeElapsedTime(message),
		TotalTokens:     int64(metadataTotalTokens(metadata)),
		TotalSteps:      agentRuntimeTotalSteps(metadata),
		CreatedAt:       message.CreatedAt.Unix(),
		FinishedAt:      runtimeFinishedAtUnix(message),
		Error:           runtimeErrorString(message),
		Source:          runtimeConversationSource(conversation, runtimemodel.ConversationSourceWebApp),
		SourceWebAppID:  runtimeSourceWebAppID(conversation),
	}
}

func buildAgentRuntimeSteps(message *runtimemodel.Message) []dto.AgentRuntimeStep {
	metadata := runtimeMetadata(message)
	events := agentRuntimeEvents(metadata)
	steps := make([]dto.AgentRuntimeStep, 0, len(events)+1)
	for index, event := range events {
		steps = append(steps, buildAgentRuntimeEventStep(message, event, index+1))
	}
	steps = append(steps, buildAgentRuntimeAnswerStep(message, len(steps)+1))
	return steps
}

func buildAgentRuntimeEventStep(message *runtimemodel.Message, event map[string]interface{}, index int) dto.AgentRuntimeStep {
	status := runtimeInvocationStatus(runtimeString(event["status"]))
	errText := runtimeString(event["error"])
	if errText == "" && status == string(dto.NodeStatusFailed) {
		errText = runtimeString(event["message"])
	}
	createdAt := runtimeInvocationCreatedAt(message, event).Unix()
	return dto.AgentRuntimeStep{
		ID:          runtimeInvocationID(message.ID.String(), event, index-1),
		Index:       index,
		Type:        agentRuntimeEventType(event),
		Title:       agentRuntimeEventTitle(event),
		Status:      status,
		Input:       agentRuntimeEventInput(event),
		Output:      agentRuntimeEventOutput(event),
		Process:     agentRuntimeEventProcess(event),
		ElapsedTime: metadataNumber(event, "duration_ms"),
		CreatedAt:   &createdAt,
		FinishedAt:  timeUnixPtr(runtimeInvocationFinishedAt(message, event, status)),
		Error:       errText,
	}
}

func buildAgentRuntimeAnswerStep(message *runtimemodel.Message, index int) dto.AgentRuntimeStep {
	metadata := runtimeMetadata(message)
	createdAt := message.CreatedAt.Unix()
	return dto.AgentRuntimeStep{
		ID:          message.ID.String() + ":answer",
		Index:       index,
		Type:        "model_answer",
		Title:       "Model Answer",
		Status:      runtimeWorkflowStatus(message.Status),
		Output:      map[string]interface{}{"answer": message.Answer},
		Process:     map[string]interface{}{"model_name": message.ModelName, "model_provider": message.ModelProvider, "usage": metadata["usage"]},
		ElapsedTime: runtimeElapsedTime(message),
		CreatedAt:   &createdAt,
		FinishedAt:  runtimeFinishedAtUnix(message),
		Error:       runtimeErrorString(message),
	}
}

func compactAgentRuntimeMap(values map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		if nested, ok := value.(map[string]interface{}); ok && len(nested) == 0 {
			continue
		}
		if nested, ok := value.([]interface{}); ok && len(nested) == 0 {
			continue
		}
		out[key] = value
	}
	return out
}

func runtimeSourceWebAppID(conversation *runtimemodel.Conversation) *string {
	if conversation == nil || conversation.SourceWebAppID == nil {
		return nil
	}
	value := conversation.SourceWebAppID.String()
	return &value
}

func runtimeConversationSource(conversation *runtimemodel.Conversation, fallback string) string {
	if conversation != nil && strings.TrimSpace(conversation.Source) != "" {
		return conversation.Source
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return runtimemodel.ConversationSourceWebApp
}

func isRuntimeLogConversation(conversation *runtimemodel.Conversation) bool {
	return isRuntimeWebAppConversation(conversation) ||
		(conversation != nil && conversation.Source == runtimemodel.ConversationSourceConsole)
}

func timeUnixPtr(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	unix := value.Unix()
	return &unix
}

func truncateAgentRuntimeText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func failAgentRuntimeLog(c *gin.Context, err error) {
	switch {
	case errors.Is(err, runtimeservice.ErrNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, runtimeservice.ErrInvalidInput):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	default:
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	}
}
