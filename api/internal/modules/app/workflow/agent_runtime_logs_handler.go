package workflow

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

// AgentRuntimeLogsHandler exposes dedicated AGENT runtime log APIs.
type AgentRuntimeLogsHandler struct {
	agentsRepo  agentRuntimeLookupRepository
	chatRuntime runtimeservice.Service
}

const (
	agentRuntimeHiddenInstructionsPlaceholder = "__ZGI_HIDDEN_SKILL_INSTRUCTIONS__"
)

func NewAgentRuntimeLogsHandler(
	agentsRepo agentRuntimeLookupRepository,
	chatRuntime runtimeservice.Service,
) *AgentRuntimeLogsHandler {
	return &AgentRuntimeLogsHandler{
		agentsRepo:  agentsRepo,
		chatRuntime: chatRuntime,
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
	messages, total, err = h.chatRuntime.ListMessagesByCallerLogFilters(
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
	message, conversation, err := h.chatRuntime.GetMessageByCaller(c.Request.Context(), scope, runtimeCaller(agentID), messageID)
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

func agentRuntimeTotalSteps(metadata map[string]interface{}) int {
	return len(agentRuntimeEvents(metadata)) + 1
}

func agentRuntimeEvents(metadata map[string]interface{}) []map[string]interface{} {
	modelEvents := sortAgentRuntimeEventsStable(runtimeSkillInvocations(metadata["model_invocations"]))
	activityEvents := sortAgentRuntimeEventsStable(append(
		runtimeSkillInvocations(metadata["skill_invocations"]),
		runtimeWorkflowRunEvents(metadata["workflow_runs"])...,
	))
	if len(modelEvents) > 0 && len(activityEvents) > 0 {
		return interleaveAgentRuntimeEvents(modelEvents, activityEvents)
	}
	events := append(modelEvents, activityEvents...)
	return sortAgentRuntimeEventsStable(events)
}

func runtimeWorkflowRunEvents(value interface{}) []map[string]interface{} {
	runs := runtimeSkillInvocations(value)
	events := make([]map[string]interface{}, 0, len(runs))
	for runIndex, run := range runs {
		events = append(events, runtimeWorkflowRunEvent(run, runIndex))
		for nodeIndex, node := range runtimeSkillInvocations(run["nodes"]) {
			events = append(events, runtimeWorkflowNodeEvent(run, node, runIndex, nodeIndex))
		}
		if approval := runtimeMap(run["approval"]); len(approval) > 0 {
			events = append(events, runtimeWorkflowApprovalEvent(run, approval, runIndex))
		}
		if question := runtimeMap(run["question_answer"]); len(question) > 0 {
			events = append(events, runtimeWorkflowQuestionEvent(run, question, runIndex))
		}
	}
	return events
}

func runtimeWorkflowRunEvent(run map[string]interface{}, runIndex int) map[string]interface{} {
	event := compactAgentRuntimeMap(map[string]interface{}{
		"kind":            "workflow_run",
		"title":           workflowRunTitle(run),
		"status":          runtimeString(run["status"]),
		"duration_ms":     metadataNumber(run, "elapsed_time"),
		"created_at":      run["created_at"],
		"workflow_run_id": runtimeString(run["workflow_run_id"]),
		"workflow_id":     runtimeString(run["workflow_id"]),
		"agent_id":        runtimeString(run["agent_id"]),
		"binding_id":      runtimeString(run["binding_id"]),
		"version":         run["version"],
		"inputs":          run["inputs"],
		"outputs":         run["outputs"],
		"error":           runtimeString(run["error"]),
		"runtime_id":      workflowRuntimeID("workflow_run", run, nil, runIndex, 0),
	})
	return event
}

func runtimeWorkflowNodeEvent(run map[string]interface{}, node map[string]interface{}, runIndex, nodeIndex int) map[string]interface{} {
	event := compactAgentRuntimeMap(map[string]interface{}{
		"kind":            "workflow_node",
		"title":           agentRuntimeWorkflowNodeTitle(node),
		"status":          runtimeString(node["status"]),
		"duration_ms":     metadataNumber(node, "elapsed_time"),
		"created_at":      firstRuntimeValue(node["created_at"], run["created_at"]),
		"workflow_run_id": runtimeString(run["workflow_run_id"]),
		"workflow_id":     runtimeString(run["workflow_id"]),
		"node_id":         runtimeString(node["node_id"]),
		"node_type":       runtimeString(node["node_type"]),
		"inputs":          node["inputs"],
		"outputs":         node["outputs"],
		"error":           runtimeString(node["error"]),
		"runtime_id":      workflowRuntimeID("workflow_node", run, node, runIndex, nodeIndex),
	})
	return event
}

func runtimeWorkflowApprovalEvent(run map[string]interface{}, approval map[string]interface{}, runIndex int) map[string]interface{} {
	event := compactAgentRuntimeMap(map[string]interface{}{
		"kind":             "workflow_approval",
		"title":            "Workflow approval",
		"status":           "pending_approval",
		"created_at":       run["created_at"],
		"workflow_run_id":  runtimeString(run["workflow_run_id"]),
		"workflow_id":      runtimeString(run["workflow_id"]),
		"approval_form_id": runtimeString(approval["approval_form_id"]),
		"approval_url":     runtimeString(approval["approval_url"]),
		"approval_form":    approval["approval_form"],
		"runtime_id":       workflowRuntimeID("workflow_approval", run, approval, runIndex, 0),
	})
	return event
}

func runtimeWorkflowQuestionEvent(run map[string]interface{}, question map[string]interface{}, runIndex int) map[string]interface{} {
	status := "pending_question"
	if runtimeString(question["answer"]) != "" || runtimeString(question["choice_id"]) != "" || runtimeString(question["choice_label"]) != "" {
		status = "success"
	}
	event := compactAgentRuntimeMap(map[string]interface{}{
		"kind":            "workflow_question",
		"title":           "Workflow question",
		"status":          status,
		"created_at":      run["created_at"],
		"workflow_run_id": runtimeString(run["workflow_run_id"]),
		"workflow_id":     runtimeString(run["workflow_id"]),
		"node_id":         runtimeString(question["node_id"]),
		"node_title":      runtimeString(question["node_title"]),
		"question":        runtimeString(question["question"]),
		"round":           question["round"],
		"choices":         question["choices"],
		"answer":          runtimeString(question["answer"]),
		"choice_id":       runtimeString(question["choice_id"]),
		"choice_label":    runtimeString(question["choice_label"]),
		"choice_value":    runtimeString(question["choice_value"]),
		"runtime_id":      workflowRuntimeID("workflow_question", run, question, runIndex, 0),
	})
	return event
}

func workflowRuntimeID(kind string, run map[string]interface{}, item map[string]interface{}, runIndex, itemIndex int) string {
	parts := []string{
		kind,
		runtimeString(run["workflow_run_id"]),
		runtimeString(run["workflow_id"]),
		runtimeString(item["node_id"]),
		runtimeString(item["node_type"]),
		strconv.Itoa(runIndex),
		strconv.Itoa(itemIndex),
	}
	return strings.Join(parts, ":")
}

func workflowRunTitle(run map[string]interface{}) string {
	if workflowID := runtimeString(run["workflow_id"]); workflowID != "" {
		return "Workflow run: " + workflowID
	}
	if runID := runtimeString(run["workflow_run_id"]); runID != "" {
		return "Workflow run: " + runID
	}
	return "Workflow run"
}

func agentRuntimeWorkflowNodeTitle(node map[string]interface{}) string {
	if title := runtimeString(node["title"]); title != "" {
		return "Workflow node: " + title
	}
	if nodeType := runtimeString(node["node_type"]); nodeType != "" {
		return "Workflow node: " + nodeType
	}
	return "Workflow node"
}

func firstRuntimeValue(values ...interface{}) interface{} {
	for _, value := range values {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		return value
	}
	return nil
}

func sortAgentRuntimeEventsStable(events []map[string]interface{}) []map[string]interface{} {
	sort.SliceStable(events, func(i, j int) bool {
		left := agentRuntimeEventSortValue(events[i])
		right := agentRuntimeEventSortValue(events[j])
		if left == 0 || right == 0 || left == right {
			return i < j
		}
		return left < right
	})
	return events
}

func agentRuntimeEventSortValue(event map[string]interface{}) int64 {
	if value := int64(metadataNumber(event, "created_at_ms")); value > 0 {
		return value
	}
	if value := runtimeIDTimestampMillis(runtimeString(event["runtime_id"])); value > 0 {
		return value
	}
	if value := int64(metadataNumber(event, "created_at")); value > 0 {
		return value * 1000
	}
	return 0
}

func runtimeIDTimestampMillis(runtimeID string) int64 {
	index := strings.LastIndex(runtimeID, ":")
	if index < 0 || index == len(runtimeID)-1 {
		return 0
	}
	value, err := strconv.ParseInt(runtimeID[index+1:], 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	if value > 1_000_000_000_000_000 {
		return value / 1_000_000
	}
	if value > 1_000_000_000_000 {
		return value
	}
	return value * 1000
}

func interleaveAgentRuntimeEvents(modelEvents []map[string]interface{}, skillEvents []map[string]interface{}) []map[string]interface{} {
	events := make([]map[string]interface{}, 0, len(modelEvents)+len(skillEvents))
	skillIndex := 0
	for _, modelEvent := range modelEvents {
		events = append(events, modelEvent)
		for range modelResponseToolCalls(modelEvent) {
			if skillIndex >= len(skillEvents) {
				break
			}
			events = append(events, skillEvents[skillIndex])
			skillIndex++
		}
	}
	for skillIndex < len(skillEvents) {
		events = append(events, skillEvents[skillIndex])
		skillIndex++
	}
	return events
}

func modelResponseToolCalls(event map[string]interface{}) []interface{} {
	response := runtimeMap(event["response"])
	message := runtimeMap(response["message"])
	if calls, ok := message["tool_calls"].([]interface{}); ok {
		return calls
	}
	return nil
}

func agentRuntimeEventType(event map[string]interface{}) string {
	switch kind := runtimeString(event["kind"]); kind {
	case "model_call":
		return "model_call"
	case "tool_call":
		return "tool_call"
	case "skill_load":
		return "skill_load"
	case "reference_read":
		return "reference_read"
	case "intermediate_answer":
		return "intermediate_answer"
	case "user_input_request":
		return "user_input_request"
	case "guardrail":
		return "guardrail"
	case "workflow_run":
		return "workflow_run"
	case "workflow_node":
		return "workflow_node"
	case "workflow_approval":
		return "workflow_approval"
	case "workflow_question":
		return "workflow_question"
	case "":
		return "agent_event"
	default:
		return kind
	}
}

func agentRuntimeEventTitle(event map[string]interface{}) string {
	if title := runtimeString(event["title"]); title != "" {
		return title
	}
	switch agentRuntimeEventType(event) {
	case "model_call":
		if phase := runtimeString(event["phase"]); phase != "" {
			return "Model call: " + phase
		}
		return "Model call"
	case "tool_call":
		return runtimeInvocationTitle(event)
	case "skill_load":
		if skillID := runtimeString(event["skill_id"]); skillID != "" {
			return "Load skill: " + skillID
		}
		return "Load skill"
	case "reference_read":
		if path := runtimeString(event["path"]); path != "" {
			return "Read reference: " + path
		}
		return "Read reference"
	case "intermediate_answer":
		return "Intermediate answer"
	case "user_input_request":
		return "User input requested"
	case "guardrail":
		return "Guardrail"
	case "workflow_run":
		return workflowRunTitle(event)
	case "workflow_node":
		return agentRuntimeWorkflowNodeTitle(event)
	case "workflow_approval":
		return "Workflow approval"
	case "workflow_question":
		if title := runtimeString(event["node_title"]); title != "" {
			return title
		}
		return "Workflow question"
	default:
		return runtimeInvocationTitle(event)
	}
}

func agentRuntimeEventInput(event map[string]interface{}) interface{} {
	switch agentRuntimeEventType(event) {
	case "model_call":
		return sanitizeAgentRuntimeModelRequest(runtimeMap(event["request"]), runtimeString(event["user_system_prompt"]))
	case "tool_call":
		return sanitizeAgentRuntimeToolArguments(runtimeMap(event["arguments"]))
	case "skill_load":
		return map[string]interface{}{"skill_id": runtimeString(event["skill_id"])}
	case "reference_read":
		return map[string]interface{}{
			"skill_id": runtimeString(event["skill_id"]),
			"path":     runtimeString(event["path"]),
		}
	case "intermediate_answer":
		return map[string]interface{}{"answer_id": runtimeString(event["answer_id"])}
	case "workflow_run":
		return sanitizeAgentRuntimeSensitiveValue(runtimeMap(event["inputs"]))
	case "workflow_node":
		return sanitizeAgentRuntimeSensitiveValue(runtimeMap(event["inputs"]))
	case "workflow_approval":
		return sanitizeAgentRuntimeSensitiveValue(runtimeMap(event["approval_form"]))
	case "workflow_question":
		return sanitizeAgentRuntimeSensitiveValue(compactAgentRuntimeMap(map[string]interface{}{
			"question": runtimeString(event["question"]),
			"choices":  event["choices"],
			"round":    event["round"],
		}))
	default:
		if arguments := runtimeMap(event["arguments"]); len(arguments) > 0 {
			return sanitizeAgentRuntimeToolArguments(arguments)
		}
		return compactAgentRuntimeMap(map[string]interface{}{
			"skill_id":  runtimeString(event["skill_id"]),
			"tool_name": runtimeString(event["tool_name"]),
			"path":      runtimeString(event["path"]),
			"answer_id": runtimeString(event["answer_id"]),
		})
	}
}

func agentRuntimeEventOutput(event map[string]interface{}) interface{} {
	if agentRuntimeEventType(event) == "model_call" {
		output := runtimeMap(event["response"])
		if len(output) == 0 {
			output["status"] = runtimeString(event["status"])
		}
		return output
	}
	if agentRuntimeEventType(event) == "workflow_run" || agentRuntimeEventType(event) == "workflow_node" {
		output := runtimeMap(event["outputs"])
		if len(output) == 0 {
			output["status"] = runtimeString(event["status"])
		}
		return sanitizeAgentRuntimeResultValue(output)
	}
	if agentRuntimeEventType(event) == "workflow_approval" {
		return sanitizeAgentRuntimeResultValue(compactAgentRuntimeMap(map[string]interface{}{
			"approval_form_id": runtimeString(event["approval_form_id"]),
			"approval_url":     runtimeString(event["approval_url"]),
			"status":           runtimeString(event["status"]),
		}))
	}
	if agentRuntimeEventType(event) == "workflow_question" {
		return sanitizeAgentRuntimeResultValue(compactAgentRuntimeMap(map[string]interface{}{
			"answer":       runtimeString(event["answer"]),
			"choice_id":    runtimeString(event["choice_id"]),
			"choice_label": runtimeString(event["choice_label"]),
			"choice_value": runtimeString(event["choice_value"]),
			"status":       runtimeString(event["status"]),
		}))
	}
	output := map[string]interface{}{}
	if result := runtimeMap(event["result"]); len(result) > 0 {
		output["result"] = sanitizeAgentRuntimeResultValue(result)
	}
	if text := runtimeString(event["message"]); text != "" {
		output["message"] = text
	}
	if path := runtimeString(event["path"]); path != "" && agentRuntimeEventType(event) == "reference_read" {
		output["path"] = path
	}
	if len(output) == 0 {
		output["status"] = runtimeString(event["status"])
	}
	return output
}

func agentRuntimeEventProcess(event map[string]interface{}) map[string]interface{} {
	return compactAgentRuntimeMap(map[string]interface{}{
		"event_type":        agentRuntimeEventType(event),
		"kind":              runtimeString(event["kind"]),
		"phase":             runtimeString(event["phase"]),
		"round":             event["round"],
		"streaming":         event["streaming"],
		"model":             runtimeString(event["model"]),
		"provider":          runtimeString(event["provider"]),
		"usage":             event["usage"],
		"prompt_tokens":     event["prompt_tokens"],
		"completion_tokens": event["completion_tokens"],
		"total_tokens":      event["total_tokens"],
		"runtime_id":        runtimeString(event["runtime_id"]),
		"skill_id":          runtimeString(event["skill_id"]),
		"tool_name":         runtimeString(event["tool_name"]),
		"path":              runtimeString(event["path"]),
		"answer_id":         runtimeString(event["answer_id"]),
		"workflow_run_id":   runtimeString(event["workflow_run_id"]),
		"workflow_id":       runtimeString(event["workflow_id"]),
		"binding_id":        runtimeString(event["binding_id"]),
		"node_id":           runtimeString(event["node_id"]),
		"node_type":         runtimeString(event["node_type"]),
		"approval_form_id":  runtimeString(event["approval_form_id"]),
		"approval_url":      runtimeString(event["approval_url"]),
		"version":           event["version"],
		"raw_event":         sanitizeAgentRuntimeRawEvent(event),
	})
}

func sanitizeAgentRuntimeRawEvent(event map[string]interface{}) map[string]interface{} {
	raw := copyRuntimeMap(event)
	if agentRuntimeEventType(event) == "model_call" {
		raw["request"] = sanitizeAgentRuntimeModelRequest(runtimeMap(event["request"]), runtimeString(event["user_system_prompt"]))
	}
	if arguments := runtimeMap(event["arguments"]); len(arguments) > 0 {
		raw["arguments"] = sanitizeAgentRuntimeToolArguments(arguments)
	}
	if result := runtimeMap(event["result"]); len(result) > 0 {
		raw["result"] = sanitizeAgentRuntimeResultValue(result)
	}
	return raw
}

func sanitizeAgentRuntimeModelRequest(request map[string]interface{}, userSystemPrompt string) map[string]interface{} {
	if len(request) == 0 {
		return request
	}
	sanitized := copyRuntimeMap(request)
	if messages, ok := sanitizeAgentRuntimeMessages(request["messages"], userSystemPrompt); ok {
		sanitized["messages"] = messages
	}
	return sanitized
}

func sanitizeAgentRuntimeMessages(value interface{}, userSystemPrompt string) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	if !ok {
		return nil, false
	}
	messages := make([]interface{}, 0, len(items))
	keptUserSystemPrompt := false
	for _, item := range items {
		message, ok := item.(map[string]interface{})
		if !ok {
			messages = append(messages, item)
			continue
		}
		if strings.EqualFold(runtimeString(message["role"]), "system") {
			if !keptUserSystemPrompt && strings.TrimSpace(userSystemPrompt) != "" {
				visible := copyRuntimeMap(message)
				visible["content"] = strings.TrimSpace(userSystemPrompt)
				messages = append(messages, visible)
				keptUserSystemPrompt = true
			}
			continue
		}
		messages = append(messages, sanitizeAgentRuntimeModelMessage(message))
	}
	return messages, true
}

func sanitizeAgentRuntimeModelMessage(message map[string]interface{}) map[string]interface{} {
	out := copyRuntimeMap(message)
	if toolCalls, ok := sanitizeAgentRuntimeToolCalls(out["tool_calls"]); ok {
		out["tool_calls"] = toolCalls
	}
	if strings.EqualFold(runtimeString(out["role"]), "tool") {
		out["content"] = sanitizeAgentRuntimeResultValue(out["content"])
	}
	return out
}

func sanitizeAgentRuntimeToolCalls(value interface{}) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		toolCall, ok := item.(map[string]interface{})
		if !ok {
			out = append(out, item)
			continue
		}
		sanitized := copyRuntimeMap(toolCall)
		if fn, ok := sanitized["function"].(map[string]interface{}); ok {
			sanitized["function"] = sanitizeAgentRuntimeToolCallFunction(fn)
		}
		if args, ok := sanitized["arguments"].(map[string]interface{}); ok {
			sanitized["arguments"] = sanitizeAgentRuntimeToolArguments(args)
		}
		out = append(out, sanitized)
	}
	return out, true
}

func sanitizeAgentRuntimeToolCallFunction(fn map[string]interface{}) map[string]interface{} {
	out := copyRuntimeMap(fn)
	switch raw := out["arguments"].(type) {
	case string:
		out["arguments"] = sanitizeAgentRuntimeToolArgumentsString(raw)
	case map[string]interface{}:
		out["arguments"] = sanitizeAgentRuntimeToolArguments(raw)
	}
	return out
}

func sanitizeAgentRuntimeToolArguments(args map[string]interface{}) map[string]interface{} {
	sanitized, ok := sanitizeAgentRuntimeSensitiveValue(args).(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return sanitized
}

func sanitizeAgentRuntimeToolArgumentsString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return raw
	}
	sanitized := sanitizeAgentRuntimeSensitiveValue(payload)
	data, err := json.Marshal(sanitized)
	if err != nil {
		return raw
	}
	return string(data)
}

func sanitizeAgentRuntimeSensitiveValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if isAgentRuntimeSensitiveKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = sanitizeAgentRuntimeSensitiveValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeAgentRuntimeSensitiveValue(item))
		}
		return out
	default:
		return value
	}
}

func sanitizeAgentRuntimeResultValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if strings.EqualFold(strings.TrimSpace(key), "instructions") {
				out[key] = agentRuntimeHiddenInstructionsPlaceholder
				continue
			}
			if isAgentRuntimeSensitiveKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = sanitizeAgentRuntimeResultValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeAgentRuntimeResultValue(item))
		}
		return out
	case string:
		sanitized, ok := sanitizeAgentRuntimeResultJSON(typed)
		if !ok {
			return typed
		}
		return sanitized
	default:
		return value
	}
}

func sanitizeAgentRuntimeResultJSON(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw, false
	}
	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return raw, false
	}
	sanitized := sanitizeAgentRuntimeResultValue(payload)
	data, err := json.Marshal(sanitized)
	if err != nil {
		return raw, false
	}
	return string(data), true
}

func isAgentRuntimeSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "password", "passwd", "pwd", "secret", "token", "api_key", "apikey", "access_key",
		"secret_key", "private_key", "access_token", "refresh_token", "authorization",
		"auth_token", "bearer", "cookie", "credential", "credentials", "client_secret",
		"x_api_key":
		return true
	}
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "access_token") ||
		strings.Contains(normalized, "refresh_token") ||
		strings.Contains(normalized, "api_key")
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
