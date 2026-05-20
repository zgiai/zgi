package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	graphentities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func persistQuestionAnswerPause(ctx context.Context, tenantID, appID, workflowRunID, nodeID string, reasons []workflowpause.Reason, state workflowpause.State) {
	service := workflowpause.NewService(database.GetDB())
	if _, err := service.Save(ctx, workflowpause.SaveParams{
		TenantID:       tenantID,
		AppID:          appID,
		WorkflowRunID:  workflowRunID,
		NodeID:         nodeID,
		Reason:         workflowpause.ReasonTypeQuestionAnswerRequired,
		ConversationID: questionAnswerStateConversationID(state),
		State:          state,
		Reasons:        reasons,
	}); err != nil {
		logger.WarnContext(ctx, "failed to save question answer pause state", "workflow_run_id", workflowRunID, err)
	}
}

func buildQuestionAnswerRequestedEvent(workflowRunID string, pausedNode workflowStreamPausedNode) map[string]interface{} {
	question, _ := pausedNode.Outputs["question"].(string)
	question = strings.TrimSpace(question)
	if question == "" {
		return nil
	}

	event := map[string]interface{}{
		"workflow_run_id": workflowRunID,
		"node_id":         pausedNode.NodeID,
		"node_title":      pausedNode.Title,
		"question":        question,
		"created_at":      time.Now().Unix(),
	}
	if round := questionAnswerEventRound(pausedNode.Outputs); round > 0 {
		event["round"] = round
	}
	if choices, ok := pausedNode.Outputs["choices"]; ok {
		event["choices"] = choices
	}
	return event
}

func buildQuestionAnswerSubmittedEvent(workflowRunID string, state *workflowpause.State, inputs map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"workflow_run_id": workflowRunID,
		"answer":          questionAnswerSubmittedAnswer(inputs),
		"created_at":      time.Now().Unix(),
	}
	if state == nil {
		return payload
	}

	nodeID := strings.TrimSpace(state.ExecutorState.PausedNodeID)
	if nodeID != "" {
		payload["node_id"] = nodeID
	}

	outputs := questionAnswerPausedOutputs(state, nodeID)
	if round := questionAnswerEventRound(outputs); round > 0 {
		payload["round"] = round
	}
	enrichQuestionAnswerSubmittedChoice(payload, outputs, inputs)
	return payload
}

func questionAnswerPausedOutputs(state *workflowpause.State, nodeID string) map[string]interface{} {
	if state == nil || nodeID == "" {
		return nil
	}
	if state.ExecutorState.ExecutionOutputs != nil {
		if outputs := state.ExecutorState.ExecutionOutputs[nodeID]; outputs != nil {
			return outputs
		}
	}
	if state.VariablePool.Variables != nil {
		return state.VariablePool.Variables[nodeID]
	}
	return nil
}

func questionAnswerEventRound(outputs map[string]interface{}) int {
	if outputs == nil {
		return 0
	}
	if answers, ok := questionAnswerAnswerRoundsLen(outputs["answers"]); ok {
		return answers + 1
	}
	if round, ok := questionAnswerInt(outputs["round"]); ok {
		return round + 1
	}
	return 0
}

func questionAnswerAnswerRoundsLen(value interface{}) (int, bool) {
	if value == nil {
		return 0, false
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return 0, false
	}
	var rounds []map[string]interface{}
	if err := json.Unmarshal(payload, &rounds); err != nil {
		return 0, false
	}
	return len(rounds), true
}

func questionAnswerInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	default:
		return 0, false
	}
}

type questionAnswerSubmittedChoice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value"`
}

func enrichQuestionAnswerSubmittedChoice(payload map[string]interface{}, outputs map[string]interface{}, inputs map[string]interface{}) {
	optionID := questionAnswerSubmittedOptionID(inputs)
	if optionID == "" {
		return
	}
	payload["choice_id"] = optionID
	for _, choice := range questionAnswerSubmittedChoices(outputs["choices"]) {
		if choice.ID != optionID {
			continue
		}
		if choice.Label != "" {
			payload["choice_label"] = choice.Label
		}
		if choice.Value != "" {
			payload["choice_value"] = choice.Value
		}
		return
	}
}

func questionAnswerSubmittedChoices(value interface{}) []questionAnswerSubmittedChoice {
	if value == nil {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var choices []questionAnswerSubmittedChoice
	if err := json.Unmarshal(payload, &choices); err != nil {
		return nil
	}
	return choices
}

func questionAnswerStateConversationID(state workflowpause.State) string {
	if state.VariablePool.SystemVariables != nil && state.VariablePool.SystemVariables.ConversationID != "" {
		return state.VariablePool.SystemVariables.ConversationID
	}
	if state.Request.Inputs != nil {
		if value, ok := state.Request.Inputs["sys.conversation_id"].(string); ok {
			return value
		}
		if value, ok := state.Request.Inputs["conversation_id"].(string); ok {
			return value
		}
	}
	return ""
}

func workflowPausedMessageStatus(eventData map[string]interface{}) string {
	for _, reason := range workflowPausedReasons(eventData) {
		if reason == workflowpause.ReasonTypeQuestionAnswerRequired {
			return conversation.AgentMessageStatusPendingQuestion
		}
	}
	return conversation.AgentMessageStatusPendingApproval
}

func workflowPausedReasons(eventData map[string]interface{}) []string {
	if eventData == nil {
		return nil
	}
	reasons, ok := eventData["reasons"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(reasons))
	for _, item := range reasons {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		reasonType, _ := record["type"].(string)
		if reasonType != "" {
			out = append(out, reasonType)
		}
	}
	return out
}

func (h *WorkflowHandler) workflowQuestionAnswerResumeState(ctx context.Context, tenantID, appID string, systemInputs map[string]interface{}) (*workflowpause.State, string, bool) {
	conversationID, _ := systemInputs["sys.conversation_id"].(string)
	if conversationID == "" {
		return nil, "", false
	}
	pauseService := workflowpause.NewService(database.GetDB())
	pauseRecord, _, state, err := pauseService.GetActiveByConversationID(ctx, tenantID, appID, conversationID, workflowpause.ReasonTypeQuestionAnswerRequired)
	if err != nil {
		if !errors.Is(err, workflowpause.ErrPauseNotFound) {
			logger.WarnContext(ctx, "failed to load question answer pause by conversation", "conversation_id", conversationID, err)
		}
		return nil, "", false
	}
	if state == nil || state.RunType != "CONVERSATION_WORKFLOW" {
		return nil, "", false
	}
	if err := pauseService.MarkResumed(ctx, state.WorkflowRunID); err != nil {
		logger.WarnContext(ctx, "failed to mark question answer pause resumed", "workflow_run_id", state.WorkflowRunID, err)
	}
	return state, pauseRecord.ID, true
}

func questionAnswerSubmittedAnswer(inputs map[string]interface{}) string {
	if inputs == nil {
		return ""
	}
	if value, ok := inputs["sys.query"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := inputs["query"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func questionAnswerSubmittedOptionID(inputs map[string]interface{}) string {
	if inputs == nil {
		return ""
	}
	if value, ok := inputs["question_answer_option_id"].(string); ok {
		return strings.TrimSpace(value)
	}
	if value, ok := inputs["inputs.question_answer_option_id"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func restoreQuestionAnswerResumeInputs(pool *graphentities.VariablePool, systemInputs map[string]interface{}, requestInputs map[string]interface{}) {
	if pool == nil {
		return
	}
	if pool.UserInputs == nil {
		pool.UserInputs = make(map[string]interface{})
	}
	optionID := questionAnswerResumeOptionID(requestInputs)
	query, hasQuery := questionAnswerResumeQuery(systemInputs, requestInputs)
	if !hasQuery && optionID != "" {
		query = optionID
		hasQuery = true
	}
	if hasQuery {
		pool.UserInputs["sys.query"] = query
		pool.UserInputs["query"] = query
		if pool.SystemVariables == nil {
			pool.SystemVariables = graphentities.SystemVariableEmpty()
		}
		pool.SystemVariables.Query = query
		pool.Add([]string{"sys", "query"}, query)
	} else {
		delete(pool.UserInputs, "sys.query")
		delete(pool.UserInputs, "query")
	}
	if optionID != "" {
		pool.UserInputs["question_answer_option_id"] = optionID
	}
}

func questionAnswerResumeQuery(systemInputs map[string]interface{}, requestInputs map[string]interface{}) (string, bool) {
	if value, ok := requestInputs["sys.query"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), true
	}
	if value, ok := requestInputs["query"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), true
	}
	if value, ok := systemInputs["sys.query"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), true
	}
	return "", false
}

func questionAnswerResumeOptionID(inputs map[string]interface{}) string {
	if inputs == nil {
		return ""
	}
	if value, ok := inputs["question_answer_option_id"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
