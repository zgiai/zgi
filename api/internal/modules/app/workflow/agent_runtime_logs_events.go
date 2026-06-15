package workflow

import (
	"sort"
	"strconv"
	"strings"
)

func agentRuntimeTotalSteps(metadata map[string]interface{}) int {
	return len(agentRuntimeEvents(metadata)) + 1
}

func agentRuntimeEvents(metadata map[string]interface{}) []map[string]interface{} {
	modelEvents := sortAgentRuntimeEventsStable(runtimeSkillInvocations(metadata["model_invocations"]))
	workflowEvents := runtimeWorkflowRunEvents(metadata["workflow_runs"])
	activityEvents := sortAgentRuntimeEventsStable(append(
		filterAgentRuntimeWorkflowToolEvents(runtimeSkillInvocations(metadata["skill_invocations"]), len(workflowEvents) > 0),
		workflowEvents...,
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
	}
	return events
}

func runtimeWorkflowRunEvent(run map[string]interface{}, runIndex int) map[string]interface{} {
	event := compactAgentRuntimeMap(map[string]interface{}{
		"kind":             "workflow_run",
		"title":            workflowRunTitle(run),
		"status":           runtimeString(run["status"]),
		"duration_ms":      metadataNumber(run, "elapsed_time"),
		"created_at":       run["created_at"],
		"workflow_run_id":  runtimeString(run["workflow_run_id"]),
		"workflow_id":      runtimeString(run["workflow_id"]),
		"agent_id":         runtimeString(run["agent_id"]),
		"binding_id":       runtimeString(run["binding_id"]),
		"version":          run["version"],
		"inputs":           run["inputs"],
		"outputs":          run["outputs"],
		"nodes":            runtimeWorkflowNodeDetails(run, runIndex),
		"approvals":        runtimeWorkflowApprovalDetails(run),
		"question_answers": runtimeWorkflowQuestionDetails(run),
		"messages":         runtimeWorkflowMessageDetails(run),
		"invocation":       runtimeWorkflowInvocationDetails(run),
		"error":            runtimeString(run["error"]),
		"runtime_id":       workflowRuntimeID("workflow_run", run, nil, runIndex, 0),
	})
	return event
}

func filterAgentRuntimeWorkflowToolEvents(events []map[string]interface{}, hasWorkflowRuns bool) []map[string]interface{} {
	if !hasWorkflowRuns {
		return events
	}
	filtered := make([]map[string]interface{}, 0, len(events))
	for _, event := range events {
		if isAgentRuntimeWorkflowSkillEvent(event) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func isAgentRuntimeWorkflowSkillEvent(event map[string]interface{}) bool {
	skillID := runtimeString(event["skill_id"])
	if skillID != "agent-workflow" {
		return false
	}
	kind := runtimeString(event["kind"])
	toolName := runtimeString(event["tool_name"])
	return kind == "skill_load" || (kind == "tool_call" && toolName == "run_agent_workflow")
}

func runtimeWorkflowNodeDetails(run map[string]interface{}, runIndex int) []interface{} {
	nodes := runtimeSkillInvocations(run["nodes"])
	details := make([]interface{}, 0, len(nodes))
	for nodeIndex, node := range nodes {
		detail := copyRuntimeMap(node)
		if runtimeString(detail["title"]) == "" {
			detail["title"] = agentRuntimeWorkflowNodeTitle(node)
		}
		detail["runtime_id"] = workflowRuntimeID("workflow_node", run, node, runIndex, nodeIndex)
		details = append(details, detail)
	}
	return details
}

func runtimeWorkflowApprovalDetails(run map[string]interface{}) []interface{} {
	approval := runtimeMap(run["approval"])
	approvalResults := runtimeSkillInvocations(run["approval_results"])
	if len(approvalResults) == 0 {
		if approvalResult := runtimeMap(run["approval_result"]); len(approvalResult) > 0 {
			approvalResults = append(approvalResults, approvalResult)
		}
	}
	approvalExpired := runtimeMap(run["approval_expired"])
	if len(approval) == 0 && len(approvalResults) == 0 && len(approvalExpired) == 0 {
		return nil
	}

	status := runtimeString(approval["status"])
	if status == "" {
		status = "pending_approval"
	}
	if len(approvalResults) > 0 {
		status = "succeeded"
	}
	if len(approvalExpired) > 0 {
		status = "expired"
	}

	detail := compactAgentRuntimeMap(map[string]interface{}{
		"status":           status,
		"approval_form_id": firstRuntimeString(approval["approval_form_id"], approval["form_id"]),
		"approval_url":     runtimeString(approval["approval_url"]),
		"request":          approval,
		"results":          approvalResults,
		"expired":          approvalExpired,
	})
	return []interface{}{detail}
}

func runtimeWorkflowQuestionDetails(run map[string]interface{}) []interface{} {
	questions := runtimeSkillInvocations(run["question_answers"])
	if question := runtimeMap(run["question_answer"]); validRuntimeWorkflowQuestion(question) {
		questions = append(questions, question)
	}
	if len(questions) == 0 {
		return nil
	}
	details := make([]interface{}, 0, len(questions))
	for _, question := range questions {
		if !validRuntimeWorkflowQuestion(question) {
			continue
		}
		details = append(details, question)
	}
	if len(details) == 0 {
		return nil
	}
	return details
}

func validRuntimeWorkflowQuestion(question map[string]interface{}) bool {
	if len(question) == 0 {
		return false
	}
	if runtimeString(question["question"]) != "" {
		return true
	}
	if len(runtimeSkillInvocations(question["choices"])) > 0 {
		return true
	}
	if runtimeString(question["node_id"]) != "" && runtimeString(question["node_title"]) != "" {
		return true
	}
	return false
}

func runtimeWorkflowMessageDetails(run map[string]interface{}) map[string]interface{} {
	messages := runtimeSkillInvocations(run["messages"])
	if len(messages) == 0 {
		return nil
	}
	chunks := make([]interface{}, 0, len(messages))
	var text strings.Builder
	for _, message := range messages {
		chunks = append(chunks, message)
		if answer := firstRuntimeString(message["answer"], message["text"]); answer != "" {
			text.WriteString(answer)
		}
	}
	return compactAgentRuntimeMap(map[string]interface{}{
		"text":        text.String(),
		"chunks":      chunks,
		"chunk_count": len(chunks),
	})
}

func runtimeWorkflowInvocationDetails(run map[string]interface{}) map[string]interface{} {
	invocation := runtimeMap(run["invocation"])
	if len(invocation) > 0 {
		return invocation
	}
	return compactAgentRuntimeMap(map[string]interface{}{
		"binding_id": runtimeString(run["binding_id"]),
		"agent_id":   runtimeString(run["agent_id"]),
		"agent_type": runtimeString(run["agent_type"]),
	})
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

func runtimeWorkflowApprovalSubmittedEvent(run map[string]interface{}, approvalResult map[string]interface{}, runIndex, approvalIndex int) map[string]interface{} {
	event := compactAgentRuntimeMap(map[string]interface{}{
		"kind":             "workflow_approval_submitted",
		"title":            "Workflow approval submitted",
		"status":           "success",
		"created_at":       firstRuntimeValue(approvalResult["created_at"], run["created_at"]),
		"workflow_run_id":  runtimeString(run["workflow_run_id"]),
		"workflow_id":      runtimeString(run["workflow_id"]),
		"approval_form_id": firstRuntimeString(approvalResult["form_id"], approvalResult["approval_form_id"]),
		"action":           firstRuntimeString(approvalResult["action"], approvalResult["action_id"]),
		"action_label":     runtimeString(approvalResult["action_label"]),
		"inputs":           firstRuntimeValue(approvalResult["inputs"], approvalResult["submitted_data"]),
		"runtime_id":       workflowRuntimeID("workflow_approval_submitted", run, approvalResult, runIndex, approvalIndex),
	})
	return event
}

func runtimeWorkflowApprovalExpiredEvent(run map[string]interface{}, approvalExpired map[string]interface{}, runIndex int) map[string]interface{} {
	event := compactAgentRuntimeMap(map[string]interface{}{
		"kind":             "workflow_approval_expired",
		"title":            "Workflow approval expired",
		"status":           "expired",
		"created_at":       firstRuntimeValue(approvalExpired["created_at"], approvalExpired["expired_at"], run["created_at"]),
		"workflow_run_id":  runtimeString(run["workflow_run_id"]),
		"workflow_id":      runtimeString(run["workflow_id"]),
		"approval_form_id": firstRuntimeString(approvalExpired["form_id"], approvalExpired["approval_form_id"]),
		"reason":           runtimeString(approvalExpired["reason"]),
		"runtime_id":       workflowRuntimeID("workflow_approval_expired", run, approvalExpired, runIndex, 0),
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

func runtimeWorkflowMessageEvent(run map[string]interface{}, message map[string]interface{}, runIndex, messageIndex int) map[string]interface{} {
	event := compactAgentRuntimeMap(map[string]interface{}{
		"kind":            "workflow_message",
		"title":           "Workflow message",
		"status":          "success",
		"created_at":      firstRuntimeValue(message["created_at"], run["created_at"]),
		"workflow_run_id": runtimeString(run["workflow_run_id"]),
		"workflow_id":     runtimeString(run["workflow_id"]),
		"event":           runtimeString(message["event"]),
		"answer":          firstRuntimeString(message["answer"], message["text"]),
		"data":            message["data"],
		"metadata":        message["metadata"],
		"runtime_id":      workflowRuntimeID("workflow_message", run, message, runIndex, messageIndex),
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

func firstRuntimeString(values ...interface{}) string {
	for _, value := range values {
		if text := runtimeString(value); text != "" {
			return text
		}
	}
	return ""
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
	case "workflow_approval_submitted":
		return "workflow_approval_submitted"
	case "workflow_approval_expired":
		return "workflow_approval_expired"
	case "workflow_question":
		return "workflow_question"
	case "workflow_message":
		return "workflow_message"
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
	case "workflow_approval_submitted":
		return "Workflow approval submitted"
	case "workflow_approval_expired":
		return "Workflow approval expired"
	case "workflow_question":
		if title := runtimeString(event["node_title"]); title != "" {
			return title
		}
		return "Workflow question"
	case "workflow_message":
		return "Workflow message"
	default:
		return runtimeInvocationTitle(event)
	}
}
