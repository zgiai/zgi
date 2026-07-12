package service

import (
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func currentTurnAuthoritativeStateMessage(message *runtimemodel.Message) *adapter.Message {
	state := currentTurnAuthoritativeStatePayload(message)
	if len(state) == 0 {
		return nil
	}
	sections := []string{
		"You are continuing the same assistant turn. The JSON below is authoritative same-turn state assembled from persisted runtime metadata.",
		"Continue only unfinished work from this state. Do not restart the whole task, repeat completed side-effecting operations, or navigate back only to re-derive a fact that is already recorded here.",
		"Use turn_state exact values for later tool arguments, summaries, names, prompts, and final answers. If later tool/page evidence contradicts a value, update turn_state before continuing.",
		"Treat current_turn_execution_state completed_operations, completed_client_actions, and operation_result_summary as completed in this same user request, not as previous conversation history.",
		"Current assistant turn authoritative state JSON:\n" + compactJSONForPrompt(state, 7000),
	}
	result := adapter.Message{Role: "system", Content: strings.Join(sections, "\n")}
	return &result
}

func currentTurnAuthoritativeStatePayload(message *runtimemodel.Message) map[string]interface{} {
	if message == nil {
		return nil
	}
	payload := map[string]interface{}{}
	if query := strings.TrimSpace(message.Query); query != "" {
		payload["original_user_request"] = compactForPrompt(query, 800)
	}
	if contract := mapFromOperationContext(metadataValue(message.Metadata, "turn_task_contract")); len(contract) > 0 {
		payload["turn_task_contract"] = contract
	}
	if plan := compactOperationPlanForPrompt(mapFromOperationContext(metadataValue(message.Metadata, "operation_plan"))); len(plan) > 0 {
		payload["operation_plan"] = plan
	}
	if result := mapFromOperationContext(metadataValue(message.Metadata, "operation_result_summary")); len(result) > 0 {
		payload["operation_result_summary"] = result
	}
	if executionState := currentTurnExecutionStateSummary(message); len(executionState) > 0 {
		payload["current_turn_execution_state"] = executionState
	}
	if turnState := turnStateContinuationSummary(message); len(turnState) > 0 {
		payload["turn_state"] = turnState
	}
	if artifacts := currentTurnGeneratedArtifactsSummary(message); len(artifacts) > 0 {
		payload["generated_artifacts"] = mapsToInterfaceSlice(artifacts)
		payload["generated_artifact_count"] = len(artifacts)
	}
	if completedActions := completedClientActionsForContinuation(message); len(completedActions) > 0 {
		payload["completed_client_actions"] = mapsToInterfaceSlice(completedActions)
	}
	if len(payload) <= 1 {
		if _, hasQuery := payload["original_user_request"]; hasQuery {
			return nil
		}
	}
	return payload
}

func currentTurnGeneratedArtifactsSummary(message *runtimemodel.Message) []map[string]interface{} {
	if message == nil || len(message.Metadata) == 0 {
		return nil
	}
	items := conversationArtifactsFromMetadata(metadataValue(message.Metadata, "conversation_artifacts"))
	if len(items) == 0 {
		items = generatedFilesFromMetadata(metadataValue(message.Metadata, "generated_files"))
	}
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, min(len(items), 8))
	seen := map[string]struct{}{}
	for idx := len(items) - 1; idx >= 0 && len(out) < 8; idx-- {
		item := items[idx]
		if len(item) == 0 {
			continue
		}
		compact := compactGeneratedArtifactForPrompt(item)
		if len(compact) == 0 {
			continue
		}
		key := strings.TrimSpace(firstNonEmptyString(compact["artifact_id"], compact["tool_file_id"], compact["file_id"], compact["filename"]))
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, compact)
	}
	return out
}

func compactGeneratedArtifactForPrompt(item map[string]interface{}) map[string]interface{} {
	if len(item) == 0 {
		return nil
	}
	compact := map[string]interface{}{}
	for _, key := range []string{
		"artifact_id",
		"artifact_type",
		"status",
		"lifecycle",
		"target",
		"filename",
		"name",
		"extension",
		"mime_type",
		"file_type",
		"transfer_method",
		"skill_id",
		"tool_name",
	} {
		if value := strings.TrimSpace(stringFromAny(item[key])); value != "" {
			compact[key] = truncateRunes(value, 160)
		}
	}
	for _, key := range []string{"tool_file_id", "file_id", "source_tool_file_id", "upload_file_id"} {
		if value := strings.TrimSpace(stringFromAny(item[key])); value != "" {
			compact[key] = value
		}
	}
	for _, key := range []string{"size", "created_at"} {
		if value, ok := item[key]; ok && value != nil {
			compact[key] = value
		}
	}
	if strings.TrimSpace(stringFromAny(compact["filename"])) == "" {
		if filename := strings.TrimSpace(firstNonEmptyString(item["file_name"], item["title"])); filename != "" {
			compact["filename"] = truncateRunes(filename, 160)
		}
	}
	if strings.TrimSpace(stringFromAny(compact["lifecycle"])) == "" {
		if isManagedFileArtifact(item) {
			compact["lifecycle"] = conversationArtifactLifecycleManaged
		} else {
			compact["lifecycle"] = conversationArtifactLifecycleTemp
		}
	}
	if strings.TrimSpace(stringFromAny(compact["status"])) == "" {
		if isManagedFileArtifact(item) {
			compact["status"] = conversationArtifactStatusSaved
		} else {
			compact["status"] = conversationArtifactStatusAvailable
		}
	}
	if strings.TrimSpace(stringFromAny(compact["target"])) == "" && !isManagedFileArtifact(item) {
		compact["target"] = "chat_artifact"
	}
	if strings.TrimSpace(stringFromAny(compact["artifact_type"])) == "" {
		compact["artifact_type"] = conversationArtifactTypeFile
	}
	return compact
}

func turnStateContinuationSummary(message *runtimemodel.Message) map[string]interface{} {
	if message == nil || len(message.Metadata) == 0 {
		return nil
	}
	state := mapFromOperationContext(metadataValue(message.Metadata, "turn_state"))
	items := mapSliceFromAny(state["items"])
	outItems := make([]map[string]interface{}, 0, 12)
	for _, item := range items {
		kind := strings.TrimSpace(stringFromAny(item["kind"]))
		visibility := strings.TrimSpace(stringFromAny(item["visibility"]))
		if kind == "" {
			continue
		}
		compact := map[string]interface{}{
			"kind":       kind,
			"visibility": firstNonEmptyString(visibility, "model_only"),
		}
		for _, key := range []string{"key", "value", "content", "title", "source"} {
			if value := strings.TrimSpace(stringFromAny(item[key])); value != "" {
				limit := 500
				if key == "key" {
					limit = 120
				}
				compact[key] = truncateRunes(value, limit)
			}
		}
		if usedFor := mapSliceOrStringListForPrompt(item["used_for"], 8, 120); len(usedFor) > 0 {
			compact["used_for"] = usedFor
		}
		if confidence, ok := floatValue(item["confidence"]); ok {
			compact["confidence"] = confidence
		}
		outItems = append(outItems, compact)
		if len(outItems) >= 12 {
			break
		}
	}
	out := map[string]interface{}{}
	if len(outItems) > 0 {
		out["items"] = mapsToInterfaceSlice(outItems)
	}
	for _, key := range []string{"steps", "tool_results", "assets", "navigations", "generated_artifacts", "open_items"} {
		if values := compactTurnStateStructuredListForPrompt(state[key], 12); len(values) > 0 {
			out[key] = mapsToInterfaceSlice(values)
		}
	}
	if len(out) == 0 {
		return nil
	}
	out["instructions"] = []string{
		"Treat these turn_state items, including user-visible deliverables, as authoritative state recorded earlier in this same assistant turn.",
		"Treat steps, tool_results, assets, navigations, and generated_artifacts as the current execution ledger for this same turn.",
		"Reuse exact working_fact values, tool result facts, and generated artifact filenames for later tool arguments and final answers instead of re-deriving placeholders.",
		"Reuse user_deliverable content when it is the only recorded summary for a later dependency; do not create a different private summary for the same source unless new evidence contradicts it.",
		"If a turn_state item or tool_result satisfies a later dependency, do not rerun the same earlier tool or navigate back to the same earlier page merely to rederive that fact.",
		"If later tool/page evidence contradicts a turn_state item, update the state with submit_turn_state before proceeding.",
	}
	return out
}

func compactTurnStateStructuredListForPrompt(value interface{}, limit int) []map[string]interface{} {
	if limit <= 0 {
		return nil
	}
	items := mapSliceFromAny(value)
	if len(items) == 0 {
		return nil
	}
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		compact := compactTurnStateStructuredMapForPrompt(item, 0)
		if len(compact) == 0 {
			continue
		}
		out = append(out, compact)
	}
	return out
}

func compactTurnStateStructuredMapForPrompt(item map[string]interface{}, depth int) map[string]interface{} {
	if len(item) == 0 || depth > 2 {
		return nil
	}
	out := map[string]interface{}{}
	for _, key := range sortedStringKeys(item) {
		value := item[key]
		switch typed := value.(type) {
		case map[string]interface{}:
			if nested := compactTurnStateStructuredMapForPrompt(typed, depth+1); len(nested) > 0 {
				out[key] = nested
			}
		case []interface{}:
			if depth >= 2 {
				continue
			}
			nestedOut := make([]interface{}, 0, min(len(typed), 6))
			for _, nestedItem := range typed {
				switch nested := nestedItem.(type) {
				case map[string]interface{}:
					if compact := compactTurnStateStructuredMapForPrompt(nested, depth+1); len(compact) > 0 {
						nestedOut = append(nestedOut, compact)
					}
				default:
					if text := strings.TrimSpace(stringFromAny(nestedItem)); text != "" {
						nestedOut = append(nestedOut, truncateRunes(text, 180))
					}
				}
				if len(nestedOut) >= 6 {
					break
				}
			}
			if len(nestedOut) > 0 {
				out[key] = nestedOut
			}
		default:
			if safe, ok := modelInvocationSafeSummaryValue(value); ok {
				if text, isText := safe.(string); isText {
					safe = truncateRunes(text, 500)
				}
				out[key] = safe
			}
		}
	}
	return out
}

func mapSliceOrStringListForPrompt(value interface{}, maxItems int, maxRunes int) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(stringFromAny(item))
			if text == "" {
				continue
			}
			out = append(out, truncateRunes(text, maxRunes))
			if len(out) >= maxItems {
				break
			}
		}
		return out
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(item)
			if text == "" {
				continue
			}
			out = append(out, truncateRunes(text, maxRunes))
			if len(out) >= maxItems {
				break
			}
		}
		return out
	default:
		return nil
	}
}
