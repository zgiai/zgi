package service

import "strings"

func upsertWorkflowRun(current []map[string]interface{}, incoming map[string]interface{}) []map[string]interface{} {
	runID := strings.TrimSpace(stringFromAny(incoming["workflow_run_id"]))
	if runID == "" {
		return current
	}
	for index, run := range current {
		if strings.TrimSpace(stringFromAny(run["workflow_run_id"])) == runID {
			current[index] = mergeWorkflowRun(run, incoming)
			return current
		}
	}
	return append(current, incoming)
}

func mergeWorkflowRun(existing map[string]interface{}, incoming map[string]interface{}) map[string]interface{} {
	merged := copyStringAnyMap(existing)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	preserveTerminalFields := workflowStatusIsTerminal(stringFromAny(existing["status"])) &&
		workflowStatusIsActive(stringFromAny(incoming["status"]))
	for key, value := range incoming {
		if value == nil {
			continue
		}
		switch key {
		case "nodes":
			merged["nodes"] = mergeWorkflowNodes(workflowNodesFromMetadata(merged["nodes"]), workflowNodesFromMetadata(value))
		case "approval":
			merged["approval"] = mergeWorkflowMap(workflowRecordFromAny(merged["approval"]), workflowRecordFromAny(value))
		case "approval_result":
			merged["approval_result"] = mergeWorkflowMap(workflowRecordFromAny(merged["approval_result"]), workflowRecordFromAny(value))
			merged["approval"] = workflowApprovalWithStatus(merged["approval"], "submitted")
		case "approval_expired":
			merged["approval_expired"] = mergeWorkflowMap(workflowRecordFromAny(merged["approval_expired"]), workflowRecordFromAny(value))
			merged["approval"] = workflowApprovalWithStatus(merged["approval"], "expired")
		case "question_answer":
			merged["question_answer"] = mergeWorkflowMap(workflowRecordFromAny(merged["question_answer"]), workflowRecordFromAny(value))
		case "approval_results", "question_answers", "messages":
			merged[key] = appendWorkflowRecordList(merged[key], value)
		default:
			if preserveTerminalFields && workflowRunTerminalPreservedField(key) {
				continue
			}
			merged[key] = value
		}
	}
	return compactWorkflowRun(merged)
}

func workflowApprovalWithStatus(value interface{}, status string) map[string]interface{} {
	approval := workflowRecordFromAny(value)
	if approval == nil {
		approval = map[string]interface{}{}
	}
	approval["status"] = strings.TrimSpace(status)
	return compactWorkflowRun(approval)
}

func workflowRunTerminalPreservedField(key string) bool {
	switch key {
	case "status", "outputs", "elapsed_time", "error":
		return true
	default:
		return false
	}
}

func appendWorkflowRecordList(existing interface{}, incoming interface{}) []interface{} {
	out := workflowRecordListFromAny(existing)
	for _, item := range workflowRecordListFromAny(incoming) {
		out = append(out, item)
	}
	return out
}

func workflowRecordListFromAny(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return append([]interface{}{}, typed...)
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case map[string]interface{}:
		return []interface{}{typed}
	default:
		return []interface{}{}
	}
}

func workflowRecordFromAny(value interface{}) map[string]interface{} {
	if record, ok := value.(map[string]interface{}); ok {
		return record
	}
	return map[string]interface{}{}
}

func workflowNodesFromMetadata(value interface{}) []map[string]interface{} {
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
			if node, ok := item.(map[string]interface{}); ok {
				out = append(out, copyStringAnyMap(node))
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func mergeWorkflowNodes(current []map[string]interface{}, incoming []map[string]interface{}) []interface{} {
	for _, node := range incoming {
		if len(node) == 0 {
			continue
		}
		key := workflowNodeIdentity(node)
		matched := false
		for index, existing := range current {
			if workflowNodeIdentity(existing) == key {
				current[index] = mergeWorkflowNode(existing, node)
				matched = true
				break
			}
		}
		if !matched {
			current = append(current, node)
		}
	}
	out := make([]interface{}, 0, len(current))
	for _, node := range current {
		out = append(out, compactWorkflowRun(node))
	}
	return out
}

func mergeWorkflowNode(existing map[string]interface{}, incoming map[string]interface{}) map[string]interface{} {
	merged := mergeWorkflowMap(existing, incoming)
	if rounds := mergeWorkflowRounds(workflowRecordListFromAny(existing["iteration_rounds"]), workflowRecordListFromAny(incoming["iteration_rounds"])); len(rounds) > 0 {
		merged["iteration_rounds"] = rounds
	}
	if rounds := mergeWorkflowRounds(workflowRecordListFromAny(existing["loop_rounds"]), workflowRecordListFromAny(incoming["loop_rounds"])); len(rounds) > 0 {
		merged["loop_rounds"] = rounds
	}
	if workflowIncomingHasGenericContainerTitle(existing, incoming) {
		merged["title"] = existing["title"]
	}
	if workflowStatusIsTerminal(stringFromAny(existing["status"])) &&
		workflowStatusIsActive(stringFromAny(incoming["status"])) {
		for _, key := range []string{"status", "outputs", "elapsed_time", "error"} {
			if value, ok := existing[key]; ok {
				merged[key] = value
			}
		}
	}
	return compactWorkflowRun(merged)
}

func workflowIncomingHasGenericContainerTitle(existing map[string]interface{}, incoming map[string]interface{}) bool {
	existingTitle := strings.TrimSpace(stringFromAny(existing["title"]))
	if existingTitle == "" {
		return false
	}
	nodeType := strings.TrimSpace(stringFromAny(incoming["node_type"]))
	if nodeType != "loop" && nodeType != "iteration" {
		return false
	}
	return strings.TrimSpace(stringFromAny(incoming["title"])) == nodeType
}

func workflowStatusIsTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "completed", "finished", "failed", "error", "stopped", "expired", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func workflowStatusIsActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "running", "pending", "pending_approval", "pending_question", "waiting_approval", "waiting_question", "paused", "submitting":
		return true
	default:
		return false
	}
}

func mergeWorkflowRounds(existing []interface{}, incoming []interface{}) []interface{} {
	current := make([]map[string]interface{}, 0, len(existing))
	for _, item := range existing {
		if record, ok := item.(map[string]interface{}); ok {
			current = append(current, copyStringAnyMap(record))
		}
	}
	for _, item := range incoming {
		round, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		index := intValueFromAny(round["index"])
		matched := false
		for currentIndex, existingRound := range current {
			if intValueFromAny(existingRound["index"]) != index {
				continue
			}
			merged := mergeWorkflowMap(existingRound, round)
			if nodes := mergeWorkflowNodes(workflowNodesFromMetadata(existingRound["nodes"]), workflowNodesFromMetadata(round["nodes"])); len(nodes) > 0 {
				merged["nodes"] = nodes
			}
			current[currentIndex] = compactWorkflowRun(merged)
			matched = true
			break
		}
		if !matched {
			current = append(current, copyStringAnyMap(round))
		}
	}
	out := make([]interface{}, 0, len(current))
	for _, round := range current {
		out = append(out, compactWorkflowRun(round))
	}
	return out
}

func workflowNodeIdentity(node map[string]interface{}) string {
	if nodeID := strings.TrimSpace(stringFromAny(node["node_id"])); nodeID != "" {
		return "node:" + nodeID
	}
	return strings.Join([]string{
		"type:" + strings.TrimSpace(stringFromAny(node["node_type"])),
		"title:" + strings.TrimSpace(stringFromAny(node["title"])),
	}, ":")
}

func mergeWorkflowMap(existing map[string]interface{}, incoming map[string]interface{}) map[string]interface{} {
	merged := copyStringAnyMap(existing)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	for key, value := range incoming {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		merged[key] = value
	}
	return compactWorkflowRun(merged)
}

func compactWorkflowRun(values map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		out[key] = value
	}
	return out
}
