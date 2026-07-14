package service

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
	"sort"
	"strings"
	"time"
)

func (s *service) persistWorkflowRunEventBestEffort(ctx context.Context, prepared *PreparedChat, eventType string, payload map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(payload) == 0 {
		return
	}
	metadata := mergeWorkflowRunMetadata(prepared.Message.Metadata, eventType, payload)
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat workflow run metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func (s *service) persistWorkflowApprovalPending(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	metadata, _ := s.persistWorkflowApprovalPendingResult(ctx, prepared, payload, usage)
	return metadata
}

func (s *service) persistWorkflowApprovalPendingResult(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) (map[string]interface{}, error) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return map[string]interface{}{}, nil
	}
	pendingPayload := copyStringAnyMap(payload)
	if pendingPayload == nil {
		pendingPayload = map[string]interface{}{}
	}
	pendingPayload["conversation_id"] = prepared.Conversation.ID.String()
	pendingPayload["message_id"] = prepared.Message.ID.String()
	metadata := mergeWorkflowRunMetadata(prepared.Message.Metadata, "approval_requested", pendingPayload)
	metadata = preparedResultMetadata(metadata, usage)
	metadata["agent_workflow_continuation"] = compactWorkflowRun(map[string]interface{}{
		"status":          "waiting_approval",
		"workflow_run_id": firstNonEmptyString(pendingPayload["workflow_run_id"]),
		"workflow_id":     firstNonEmptyString(pendingPayload["workflow_id"]),
		"agent_id":        firstNonEmptyString(pendingPayload["agent_id"]),
		"agent_type":      firstNonEmptyString(pendingPayload["agent_type"]),
		"binding_id":      firstNonEmptyString(pendingPayload["binding_id"]),
		"original_query":  prepared.Message.Query,
		"approval_token":  firstNonEmptyString(pendingPayload["approval_token"]),
		"approval_url":    firstNonEmptyString(pendingPayload["approval_url"]),
		"resume_policy":   "same_message",
	})
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return metadata, nil
	}
	err := s.persistPendingMessageAndFinishConversationBestEffort(
		ctx,
		prepared,
		"workflow approval",
		func(repo repository.MessageRepository) error {
			return repo.UpdateWaitingApproval(ctx, prepared.Message.ID, metadata)
		},
		func(repo repository.ConversationRepository) error {
			return repo.FinishWaitingApprovalMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
		},
	)
	return metadata, err
}

func (s *service) persistWorkflowQuestionPending(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	metadata, _ := s.persistWorkflowQuestionPendingResult(ctx, prepared, payload, usage)
	return metadata
}

func (s *service) persistWorkflowQuestionPendingResult(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) (map[string]interface{}, error) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return map[string]interface{}{}, nil
	}
	pendingPayload := copyStringAnyMap(payload)
	if pendingPayload == nil {
		pendingPayload = map[string]interface{}{}
	}
	pendingPayload["conversation_id"] = prepared.Conversation.ID.String()
	pendingPayload["message_id"] = prepared.Message.ID.String()
	metadata := mergeWorkflowRunMetadata(prepared.Message.Metadata, "workflow_paused", pendingPayload)
	metadata = mergeWorkflowRunMetadata(metadata, "question_answer_requested", pendingPayload)
	metadata = preparedResultMetadata(metadata, usage)
	metadata["agent_workflow_continuation"] = compactWorkflowRun(map[string]interface{}{
		"status":          "waiting_question",
		"workflow_run_id": firstNonEmptyString(pendingPayload["workflow_run_id"]),
		"workflow_id":     firstNonEmptyString(pendingPayload["workflow_id"]),
		"agent_id":        firstNonEmptyString(pendingPayload["agent_id"]),
		"agent_type":      firstNonEmptyString(pendingPayload["agent_type"]),
		"binding_id":      firstNonEmptyString(pendingPayload["binding_id"]),
		"original_query":  prepared.Message.Query,
		"resume_policy":   "same_message",
	})
	if request := workflowQuestionUserInputRequest(prepared.Conversation.ID.String(), prepared.Message.ID.String(), pendingPayload); len(request) > 0 {
		metadata["user_input_request"] = request
	}
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return metadata, nil
	}
	err := s.persistPendingMessageAndFinishConversationBestEffort(
		ctx,
		prepared,
		"workflow question",
		func(repo repository.MessageRepository) error {
			return repo.UpdateWaitingQuestion(ctx, prepared.Message.ID, metadata)
		},
		func(repo repository.ConversationRepository) error {
			return repo.FinishWaitingApprovalMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
		},
	)
	return metadata, err
}

func mergeWorkflowRunMetadata(source map[string]interface{}, eventType string, payload map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	run := workflowRunFromEvent(eventType, payload)
	if len(run) == 0 {
		return metadata
	}
	runs := workflowRunsFromMetadata(metadata["workflow_runs"])
	runs = upsertWorkflowRun(runs, run)
	metadata["has_trace"] = true
	metadata["workflow_runs"] = workflowRunsToInterfaceSlice(runs)
	metadata["workflow_run_count"] = len(runs)
	return metadata
}

func workflowRunFromEvent(eventType string, payload map[string]interface{}) map[string]interface{} {
	runID := firstNonEmptyString(payload["workflow_run_id"], payload["task_id"], payload["id"])
	if runID == "" {
		return nil
	}
	run := map[string]interface{}{
		"workflow_run_id": runID,
		"status":          workflowRunStatusFromEvent(eventType, payload),
	}
	copyWorkflowFields(run, payload, "workflow_id", "agent_id", "version", "inputs", "outputs", "elapsed_time", "error", "created_at")
	if createdAt := workflowCreatedAt(payload); createdAt != nil {
		run["created_at"] = createdAt
	}
	if approval := workflowApprovalFromEvent(payload); len(approval) > 0 {
		run["approval"] = approval
	}
	if approvalResult := workflowApprovalResultFromEvent(eventType, payload); len(approvalResult) > 0 {
		run["approval_result"] = approvalResult
		run["approval_results"] = []interface{}{approvalResult}
		run["approval"] = workflowApprovalWithStatus(run["approval"], "submitted")
	}
	if approvalExpired := workflowApprovalExpiredFromEvent(eventType, payload); len(approvalExpired) > 0 {
		run["approval_expired"] = approvalExpired
		run["approval"] = workflowApprovalWithStatus(run["approval"], "expired")
	}
	if question := workflowQuestionAnswerFromEvent(eventType, payload); len(question) > 0 {
		run["question_answer"] = question
		run["question_answers"] = []interface{}{question}
	}
	if message := workflowMessageFromEvent(eventType, payload); len(message) > 0 {
		run["messages"] = []interface{}{message}
	}
	switch strings.TrimSpace(eventType) {
	case "node_started":
		run["nodes"] = workflowNodesForEvent(payload, false)
	case "node_finished":
		run["nodes"] = workflowNodesForEvent(payload, true)
	case "iteration_started", "loop_started":
		run["nodes"] = []interface{}{workflowContainerNodeFromEvent(payload, false)}
	case "iteration_next", "loop_next":
		run["nodes"] = []interface{}{workflowContainerRoundNodeFromEvent(payload)}
	case "iteration_completed", "iteration_succeeded", "iteration_failed", "loop_completed", "loop_succeeded", "loop_failed":
		run["nodes"] = []interface{}{workflowContainerNodeFromEvent(payload, true)}
	case "workflow_paused":
		if node := workflowNodeFromEvent(payload, true); len(node) > 0 {
			node["status"] = "paused"
			run["nodes"] = []interface{}{node}
		}
	}
	return compactWorkflowRun(run)
}

func workflowNodesForEvent(payload map[string]interface{}, finished bool) []interface{} {
	node := workflowNodeFromEvent(payload, finished)
	if len(node) == 0 {
		return nil
	}
	if container := workflowContainerNodeForChild(payload, node); len(container) > 0 {
		return []interface{}{container}
	}
	return []interface{}{node}
}

func workflowRunStatusFromEvent(eventType string, payload map[string]interface{}) string {
	switch strings.TrimSpace(eventType) {
	case "workflow_started":
		return "running"
	case "workflow_paused":
		if workflowPausedHasQuestionReason(payload) {
			return "pending_question"
		}
		return "pending_approval"
	case "approval_requested":
		return "pending_approval"
	case "approval_result_filled":
		return "running"
	case "approval_expired":
		return "expired"
	case "question_answer_requested":
		return "pending_question"
	case "question_answer_submitted":
		return "running"
	case "workflow_stopped":
		return "stopped"
	case "workflow_finished":
		if status := firstNonEmptyString(payload["status"]); status != "" {
			return status
		}
		return "completed"
	case "workflow_failed":
		return "error"
	default:
		if status := firstNonEmptyString(payload["status"]); status != "" {
			return status
		}
		return "running"
	}
}

func workflowPausedHasQuestionReason(payload map[string]interface{}) bool {
	reasons, ok := payload["reasons"].([]interface{})
	if !ok {
		return strings.EqualFold(firstNonEmptyString(payload["reason"], payload["type"]), "question_answer_required")
	}
	for _, item := range reasons {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.EqualFold(firstNonEmptyString(record["type"], record["reason"]), "question_answer_required") {
			return true
		}
	}
	return false
}

func workflowNodeFromEvent(payload map[string]interface{}, finished bool) map[string]interface{} {
	nodeID := firstNonEmptyString(payload["node_id"], payload["execution_id"])
	nodeType := firstNonEmptyString(payload["node_type"], payload["type"])
	title := firstNonEmptyString(payload["title"], payload["node_title"], payload["name"], payload["label"])
	if nodeID == "" && nodeType == "" && title == "" {
		return nil
	}
	status := firstNonEmptyString(payload["status"])
	if status == "" {
		if finished {
			status = "succeeded"
		} else {
			status = "running"
		}
	}
	node := map[string]interface{}{
		"node_id":   nodeID,
		"node_type": nodeType,
		"title":     title,
		"status":    status,
	}
	copyWorkflowFields(node, payload, "inputs", "outputs", "elapsed_time", "error", "created_at")
	if createdAt := workflowCreatedAt(payload); createdAt != nil {
		node["created_at"] = createdAt
	}
	return compactWorkflowRun(node)
}

func workflowContainerNodeFromEvent(payload map[string]interface{}, finished bool) map[string]interface{} {
	node := workflowNodeFromEvent(payload, finished)
	if len(node) == 0 {
		return nil
	}
	nodeType := strings.TrimSpace(stringFromAny(node["node_type"]))
	if nodeType != "iteration" && nodeType != "loop" {
		return node
	}
	if steps := numericValueFromMap(payload, "steps"); steps != nil {
		node["steps"] = steps
	}
	if finished {
		if rounds := workflowContainerRoundsFromExecutionMetadata(nodeType, payload); len(rounds) > 0 {
			if nodeType == "loop" {
				node["loop_rounds"] = rounds
			} else {
				node["iteration_rounds"] = rounds
			}
		}
	}
	return node
}

func workflowContainerRoundNodeFromEvent(payload map[string]interface{}) map[string]interface{} {
	node := workflowContainerNodeFromEvent(payload, false)
	if len(node) == 0 {
		return nil
	}
	nodeType := strings.TrimSpace(stringFromAny(node["node_type"]))
	index := workflowContainerRoundIndexFromPayload(nodeType, payload)
	round := map[string]interface{}{"index": index}
	if nodeType == "loop" {
		if value, ok := payload["pre_loop_output"]; ok {
			round["variables"] = value
		}
		node["loop_rounds"] = []interface{}{compactWorkflowRun(round)}
	} else {
		node["iteration_rounds"] = []interface{}{compactWorkflowRun(round)}
	}
	return node
}

func workflowContainerRoundsFromExecutionMetadata(nodeType string, payload map[string]interface{}) []interface{} {
	metadata := workflowRecordFromAny(payload["execution_metadata"])
	if len(metadata) == 0 {
		metadata = workflowRecordFromAny(payload["metadata"])
	}
	var durations map[int]interface{}
	switch strings.TrimSpace(nodeType) {
	case "iteration":
		durations = workflowRoundDurationMapFromMetadata(metadata, "iteration_duration_map", "iteration_duration_list")
	case "loop":
		durations = workflowRoundDurationMapFromMetadata(metadata, "loop_duration_map", "")
	default:
		return nil
	}
	if len(durations) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(durations))
	for index := range durations {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	rounds := make([]interface{}, 0, len(durations))
	for _, index := range indexes {
		round := map[string]interface{}{
			"index":        index,
			"elapsed_time": durations[index],
		}
		rounds = append(rounds, compactWorkflowRun(round))
	}
	return rounds
}

func workflowRoundDurationMapFromMetadata(metadata map[string]interface{}, mapKey, listKey string) map[int]interface{} {
	durations := map[int]interface{}{}
	if len(metadata) == 0 {
		return durations
	}
	mergeWorkflowRoundDurationMap(durations, metadata[mapKey])
	if listKey == "" {
		return durations
	}
	mergeWorkflowRoundDurationList(durations, metadata[listKey])
	return durations
}

func mergeWorkflowRoundDurationMap(target map[int]interface{}, value interface{}) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			if elapsed := numericValueFromAny(item); elapsed != nil {
				target[intValueFromAny(key)] = elapsed
			}
		}
	case map[string]float64:
		for key, item := range typed {
			target[intValueFromAny(key)] = item
		}
	case map[string]int:
		for key, item := range typed {
			target[intValueFromAny(key)] = item
		}
	case map[string]int64:
		for key, item := range typed {
			target[intValueFromAny(key)] = item
		}
	}
}

func mergeWorkflowRoundDurationList(target map[int]interface{}, value interface{}) {
	switch typed := value.(type) {
	case []interface{}:
		for index, item := range typed {
			if elapsed := numericValueFromAny(item); elapsed != nil {
				target[index] = elapsed
			}
		}
	case []float64:
		for index, item := range typed {
			target[index] = item
		}
	case []int:
		for index, item := range typed {
			target[index] = item
		}
	case []int64:
		for index, item := range typed {
			target[index] = item
		}
	}
}

func workflowContainerRoundIndexFromPayload(nodeType string, payload map[string]interface{}) int {
	switch strings.TrimSpace(nodeType) {
	case "loop":
		if _, ok := payload["loop_index"]; ok {
			return intValueFromAny(payload["loop_index"])
		}
		index := intValueFromAny(payload["index"])
		if index > 0 {
			return index - 1
		}
		return index
	case "iteration":
		if _, ok := payload["iteration_index"]; ok {
			return intValueFromAny(payload["iteration_index"])
		}
		return intValueFromAny(payload["index"])
	default:
		return intValueFromAny(payload["index"])
	}
}

func workflowContainerNodeForChild(payload map[string]interface{}, child map[string]interface{}) map[string]interface{} {
	if loopID := strings.TrimSpace(stringFromAny(payload["loop_id"])); loopID != "" {
		return workflowContainerNodeWithChild("loop", loopID, payload["loop_index"], child)
	}
	if iterationID := strings.TrimSpace(stringFromAny(payload["iteration_id"])); iterationID != "" {
		return workflowContainerNodeWithChild("iteration", iterationID, payload["iteration_index"], child)
	}
	return nil
}

func workflowContainerNodeWithChild(nodeType string, nodeID string, indexValue interface{}, child map[string]interface{}) map[string]interface{} {
	round := map[string]interface{}{
		"index": intValueFromAny(indexValue),
		"nodes": []interface{}{child},
	}
	node := map[string]interface{}{
		"node_id":   nodeID,
		"node_type": nodeType,
		"title":     nodeType,
		"status":    "running",
	}
	if nodeType == "loop" {
		node["loop_rounds"] = []interface{}{round}
	} else {
		node["iteration_rounds"] = []interface{}{round}
	}
	return compactWorkflowRun(node)
}

func workflowApprovalFromEvent(payload map[string]interface{}) map[string]interface{} {
	approval := map[string]interface{}{}
	copyWorkflowFields(approval, payload, "approval_form_id", "approval_token", "approval_url", "approval_form")
	return compactWorkflowRun(approval)
}

func workflowApprovalResultFromEvent(eventType string, payload map[string]interface{}) map[string]interface{} {
	if strings.TrimSpace(eventType) != "approval_result_filled" {
		return nil
	}
	result := map[string]interface{}{}
	copyWorkflowFields(result, payload, "form_id", "approval_form_id", "action", "action_id", "action_label", "inputs", "submitted_data", "submitted_at", "created_at")
	return compactWorkflowRun(result)
}

func workflowApprovalExpiredFromEvent(eventType string, payload map[string]interface{}) map[string]interface{} {
	if strings.TrimSpace(eventType) != "approval_expired" {
		return nil
	}
	expired := map[string]interface{}{}
	copyWorkflowFields(expired, payload, "form_id", "approval_form_id", "reason", "expired_at", "created_at")
	return compactWorkflowRun(expired)
}

func workflowQuestionAnswerFromEvent(eventType string, payload map[string]interface{}) map[string]interface{} {
	switch strings.TrimSpace(eventType) {
	case "question_answer_requested", "question_answer_submitted":
	default:
		if firstNonEmptyString(payload["question"], payload["answer"], payload["choice_id"], payload["choice_label"], payload["choice_value"]) == "" {
			return nil
		}
	}
	question := map[string]interface{}{}
	copyWorkflowFields(question, payload, "node_id", "node_title", "question", "round", "choices", "answer", "choice_id", "choice_label", "choice_value")
	return compactWorkflowRun(question)
}

func workflowMessageFromEvent(eventType string, payload map[string]interface{}) map[string]interface{} {
	switch strings.TrimSpace(eventType) {
	case "message", "text_chunk", "message_end":
	default:
		return nil
	}
	message := map[string]interface{}{"event": strings.TrimSpace(eventType)}
	copyWorkflowFields(message, payload, "answer", "text", "data", "metadata", "created_at")
	return compactWorkflowRun(message)
}

func workflowQuestionUserInputRequest(conversationID, messageID string, payload map[string]interface{}) map[string]interface{} {
	question := strings.TrimSpace(firstNonEmptyString(payload["question"]))
	if question == "" {
		return nil
	}
	workflowRunID := firstNonEmptyString(payload["workflow_run_id"])
	nodeID := firstNonEmptyString(payload["node_id"])
	round := firstNonEmptyString(payload["round"])
	requestID := strings.Join([]string{workflowRunID, nodeID, round}, ":")
	requestID = strings.Trim(requestID, ":")
	if requestID == "" {
		requestID = workflowRunID
	}
	item := map[string]interface{}{
		"id":       "answer",
		"question": question,
	}
	if options := workflowQuestionOptions(payload["choices"]); len(options) > 0 {
		item["options"] = options
	}
	return compactWorkflowRun(map[string]interface{}{
		"source":          "agent_workflow_question_answer",
		"request_id":      requestID,
		"workflow_run_id": workflowRunID,
		"node_id":         nodeID,
		"round":           payload["round"],
		"conversation_id": conversationID,
		"message_id":      messageID,
		"questions":       []interface{}{item},
		"created_at":      time.Now().Unix(),
	})
}

func workflowQuestionOptions(value interface{}) []interface{} {
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
	options := make([]interface{}, 0, len(items))
	for index, item := range items {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		label := firstNonEmptyString(record["label"], record["value"], record["id"])
		if strings.TrimSpace(label) == "" {
			continue
		}
		option := map[string]interface{}{
			"label": label,
			"value": firstNonEmptyString(record["value"], record["id"], label),
		}
		if id := firstNonEmptyString(record["id"], record["option_id"]); id != "" {
			option["option_id"] = id
		} else {
			option["option_id"] = fmt.Sprintf("option_%d", index+1)
		}
		if description := firstNonEmptyString(record["description"]); description != "" {
			option["description"] = description
		}
		options = append(options, option)
	}
	return options
}

func copyWorkflowFields(target map[string]interface{}, source map[string]interface{}, keys ...string) {
	for _, key := range keys {
		value, ok := source[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		target[key] = value
	}
}

func workflowCreatedAt(payload map[string]interface{}) interface{} {
	if createdAt := numericValueFromMap(payload, "created_at"); createdAt != nil {
		return createdAt
	}
	return time.Now().Unix()
}

func workflowRunsFromMetadata(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, copyStringAnyMap(item))
		}
		return out
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if run, ok := item.(map[string]interface{}); ok {
				out = append(out, copyStringAnyMap(run))
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func workflowRunsToInterfaceSlice(runs []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(runs))
	for _, run := range runs {
		out = append(out, run)
	}
	return out
}
