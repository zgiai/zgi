package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"strings"
	"time"
)

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
