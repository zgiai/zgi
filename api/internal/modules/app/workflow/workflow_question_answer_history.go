package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const questionAnswerTranscriptMetadataKey = "questionAnswerTranscript"
const questionAnswerPromptMetadataKey = "questionAnswerPrompt"

type questionAnswerHistory struct {
	Transcript []map[string]interface{}
	Prompt     map[string]interface{}
}

type questionAnswerTranscriptEntry struct {
	Key      string
	NodeID   string
	Round    int
	Question string
	Answer   string
}

func conversationMessageMetadataWithQuestionAnswer(ctx context.Context, message *conversation.AgentMessage) map[string]interface{} {
	if message == nil {
		return nil
	}
	metadata := conversationMessageMetadata(ctx, message)
	if message.WorkflowRunID == nil {
		return metadata
	}

	history, err := loadQuestionAnswerHistory(ctx, message.WorkflowRunID.String())
	if err != nil {
		logger.WarnContext(ctx, "failed to load question answer transcript", "workflow_run_id", message.WorkflowRunID.String(), err)
		return metadata
	}
	if len(history.Transcript) == 0 && len(history.Prompt) == 0 {
		return metadata
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	if len(history.Transcript) > 0 {
		metadata[questionAnswerTranscriptMetadataKey] = history.Transcript
	}
	if message.Status == conversation.AgentMessageStatusPendingQuestion && len(history.Prompt) > 0 {
		metadata[questionAnswerPromptMetadataKey] = history.Prompt
	}
	return metadata
}

func conversationMessageMetadata(ctx context.Context, message *conversation.AgentMessage) map[string]interface{} {
	metadata, err := message.GetMessageMetadataAsMap()
	if err != nil {
		logger.WarnContext(ctx, "failed to parse conversation message metadata", "message_id", message.ID.String(), err)
		return nil
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func loadQuestionAnswerHistory(ctx context.Context, workflowRunID string) (questionAnswerHistory, error) {
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" {
		return questionAnswerHistory{}, nil
	}
	service := workflowpause.NewService(database.GetDB())
	payload, err := service.ListEvents(ctx, "", workflowRunID, 0, 200)
	if err != nil {
		return questionAnswerHistory{}, err
	}
	return buildQuestionAnswerHistoryFromEvents(payload.Events), nil
}

func buildQuestionAnswerHistoryFromEvents(events []workflowpause.RunEventPayload) questionAnswerHistory {
	return questionAnswerHistory{
		Transcript: buildQuestionAnswerTranscriptFromEvents(events),
		Prompt:     buildQuestionAnswerPromptFromEvents(events),
	}
}

func buildQuestionAnswerTranscriptFromEvents(events []workflowpause.RunEventPayload) []map[string]interface{} {
	entries := make([]questionAnswerTranscriptEntry, 0)
	for _, event := range events {
		switch event.Event {
		case workflowpause.EventQuestionAnswerRequested:
			entries = upsertQuestionAnswerTranscriptQuestion(entries, event.Data)
		case workflowpause.EventQuestionAnswerSubmitted:
			entries = applyQuestionAnswerTranscriptAnswer(entries, event.Data)
		case workflowpause.EventWorkflowPaused:
			entries = upsertQuestionAnswerTranscriptPausedQuestions(entries, event.Data)
		}
	}
	return questionAnswerTranscriptEntriesToMaps(entries)
}

func buildQuestionAnswerPromptFromEvents(events []workflowpause.RunEventPayload) map[string]interface{} {
	var prompt map[string]interface{}
	for _, event := range events {
		switch event.Event {
		case workflowpause.EventQuestionAnswerRequested:
			prompt = questionAnswerPromptFromData(event.Data)
		case workflowpause.EventWorkflowPaused:
			if pausedPrompt := questionAnswerPromptFromPausedReasons(event.Data); len(pausedPrompt) > 0 {
				prompt = pausedPrompt
			}
		case workflowpause.EventQuestionAnswerSubmitted, workflowpause.EventWorkflowFinished:
			prompt = nil
		}
	}
	return prompt
}

func questionAnswerPromptFromPausedReasons(data map[string]interface{}) map[string]interface{} {
	reasons, ok := data["reasons"].([]interface{})
	if !ok {
		return nil
	}
	for _, reason := range reasons {
		record, ok := reason.(map[string]interface{})
		if !ok || record["type"] != workflowpause.ReasonTypeQuestionAnswerRequired {
			continue
		}
		if prompt := questionAnswerPromptFromData(record); len(prompt) > 0 {
			return prompt
		}
	}
	return nil
}

func questionAnswerPromptFromData(data map[string]interface{}) map[string]interface{} {
	question := workflowQuestionAnswerString(data["question"])
	if question == "" {
		return nil
	}
	prompt := map[string]interface{}{
		"question": question,
		"choices":  []interface{}{},
	}
	if choices, ok := data["choices"]; ok {
		prompt["choices"] = choices
	}
	if round := workflowQuestionAnswerNumber(data["round"]); round > 0 {
		prompt["round"] = round
	}
	if nodeID := workflowQuestionAnswerString(data["node_id"]); nodeID != "" {
		prompt["node_id"] = nodeID
	}
	if nodeTitle := workflowQuestionAnswerString(data["node_title"]); nodeTitle != "" {
		prompt["node_title"] = nodeTitle
	}
	return prompt
}

func upsertQuestionAnswerTranscriptPausedQuestions(entries []questionAnswerTranscriptEntry, data map[string]interface{}) []questionAnswerTranscriptEntry {
	reasons, ok := data["reasons"].([]interface{})
	if !ok {
		return entries
	}
	for _, reason := range reasons {
		record, ok := reason.(map[string]interface{})
		if !ok || record["type"] != workflowpause.ReasonTypeQuestionAnswerRequired {
			continue
		}
		entries = upsertQuestionAnswerTranscriptQuestion(entries, record)
	}
	return entries
}

func upsertQuestionAnswerTranscriptQuestion(entries []questionAnswerTranscriptEntry, data map[string]interface{}) []questionAnswerTranscriptEntry {
	question := workflowQuestionAnswerString(data["question"])
	if question == "" {
		return entries
	}
	nodeID := workflowQuestionAnswerString(data["node_id"])
	round := workflowQuestionAnswerNumber(data["round"])
	key := workflowQuestionAnswerTranscriptKey(nodeID, round, question)

	for i := range entries {
		if entries[i].Key != key {
			continue
		}
		entries[i].NodeID = nodeID
		entries[i].Round = round
		entries[i].Question = question
		return entries
	}
	for i := range entries {
		if !samePendingQuestionAnswerEntry(entries[i], nodeID, question) {
			continue
		}
		entries[i].Key = key
		entries[i].NodeID = nodeID
		entries[i].Round = round
		entries[i].Question = question
		return entries
	}
	return append(entries, questionAnswerTranscriptEntry{
		Key:      key,
		NodeID:   nodeID,
		Round:    round,
		Question: question,
	})
}

func samePendingQuestionAnswerEntry(entry questionAnswerTranscriptEntry, nodeID, question string) bool {
	if entry.Answer != "" || entry.Question != question {
		return false
	}
	if entry.NodeID != "" && nodeID != "" && entry.NodeID != nodeID {
		return false
	}
	return true
}

func applyQuestionAnswerTranscriptAnswer(entries []questionAnswerTranscriptEntry, data map[string]interface{}) []questionAnswerTranscriptEntry {
	answer := workflowQuestionAnswerSubmittedDisplayAnswer(data)
	if answer == "" {
		return entries
	}
	nodeID := workflowQuestionAnswerString(data["node_id"])
	round := workflowQuestionAnswerNumber(data["round"])

	if nodeID != "" && round > 0 {
		for i := range entries {
			if entries[i].NodeID == nodeID && entries[i].Round == round {
				entries[i].Answer = answer
				return entries
			}
		}
	}
	if nodeID != "" {
		for i := len(entries) - 1; i >= 0; i-- {
			if entries[i].NodeID == nodeID && entries[i].Answer == "" {
				entries[i].Answer = answer
				return entries
			}
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Answer == "" {
			entries[i].Answer = answer
			return entries
		}
	}

	return append(entries, questionAnswerTranscriptEntry{
		Key:    workflowQuestionAnswerTranscriptKey(nodeID, round, answer),
		NodeID: nodeID,
		Round:  round,
		Answer: answer,
	})
}

func questionAnswerTranscriptEntriesToMaps(entries []questionAnswerTranscriptEntry) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		if entry.Question == "" && entry.Answer == "" {
			continue
		}
		item := map[string]interface{}{
			"key": entry.Key,
		}
		if entry.Question != "" {
			item["question"] = entry.Question
		}
		if entry.NodeID != "" {
			item["nodeId"] = entry.NodeID
		}
		if entry.Round > 0 {
			item["round"] = entry.Round
		}
		if entry.Answer != "" {
			item["answer"] = entry.Answer
		}
		items = append(items, item)
	}
	return items
}

func workflowQuestionAnswerSubmittedDisplayAnswer(data map[string]interface{}) string {
	for _, key := range []string{"choice_label", "choice_value", "answer"} {
		if value := workflowQuestionAnswerString(data[key]); value != "" {
			return value
		}
	}
	return ""
}

func workflowQuestionAnswerString(value interface{}) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func workflowQuestionAnswerNumber(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}

func workflowQuestionAnswerTranscriptKey(nodeID string, round int, fallback string) string {
	if nodeID != "" && round > 0 {
		return fmt.Sprintf("%s:%d", nodeID, round)
	}
	if nodeID != "" {
		return nodeID + ":" + fallback
	}
	return fallback
}
