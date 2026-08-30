package skillloop

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const runtimeStateNativeExternalActionProjectionsKey = "_native_external_action_projections"
const runtimeStateNativeExternalActionIntentMatchedKey = "_native_external_action_intent_matched"
const runtimeStateNativeExternalActionIntentKeysKey = "_native_external_action_intent_keys"
const runtimeStateNativeExternalActionCandidatesKey = "_native_external_action_candidates"

func runtimeStateForRun(req RunRequest) map[string]interface{} {
	state := map[string]interface{}{}
	if req.RuntimeStateSnapshot != nil {
		state = copyStringAnyMap(req.RuntimeStateSnapshot())
	}
	if state == nil {
		state = map[string]interface{}{}
	}
	for _, key := range []string{
		"operation_plan",
		"operation_result_summary",
		"evidence_ledger",
		"turn_state",
		"execution_summary",
		"execution_ledger",
		"agent_create_progress",
		"generated_files",
		"client_actions",
		"pending_approval",
		"pending_client_action",
		"pending_question",
		"pending_user_input",
	} {
		if _, exists := state[key]; exists {
			continue
		}
		if value, ok := currentMetadataForRun(req)[key]; ok && value != nil {
			state[key] = value
		}
	}
	if plan := evidenceMapFromAny(state["operation_plan"]); len(evidenceMapsFromAny(plan["evidence_ledger"])) > 0 {
		delete(state, "evidence_ledger")
		if execution := evidenceMapFromAny(state["execution_ledger"]); len(execution) > 0 {
			compacted := copyStringAnyMap(execution)
			delete(compacted, "evidence_ledger")
			state["execution_ledger"] = compacted
		}
	}
	if text := strings.TrimSpace(latestUserRequestText(req)); text != "" {
		state["latest_user_request"] = text
	}
	// This request-scoped list is rebuilt from server-owned bindings on every
	// snapshot. Never accept a persisted/model-provided value as authority for
	// deciding which external Action identities may be retried.
	delete(state, runtimeStateNativeExternalActionProjectionsKey)
	delete(state, runtimeStateNativeExternalActionIntentMatchedKey)
	delete(state, runtimeStateNativeExternalActionIntentKeysKey)
	delete(state, runtimeStateNativeExternalActionCandidatesKey)
	if projections := nativeExternalActionProjectionEvidence(req); len(projections) > 0 {
		state[runtimeStateNativeExternalActionProjectionsKey] = projections
	}
	if nativeExternalActionIntentMatched(req) {
		state[runtimeStateNativeExternalActionIntentMatchedKey] = true
	}
	if candidates := nativeExternalActionCandidateEvidence(req); len(candidates) > 0 {
		state[runtimeStateNativeExternalActionCandidatesKey] = candidates
	}
	return state
}

func nativeExternalActionIntentKeys(req RunRequest) []string {
	if req.NativeSkillSession != nil {
		return append([]string(nil), req.NativeSkillSession.ToolSet().ExternalActionIntentKeys...)
	}
	if req.NativeToolSet == nil {
		return nil
	}
	return append([]string(nil), req.NativeToolSet.ExternalActionIntentKeys...)
}

func nativeExternalActionIntentMatched(req RunRequest) bool {
	if req.NativeSkillSession != nil {
		return req.NativeSkillSession.ToolSet().ExternalActionIntentMatched
	}
	return req.NativeToolSet != nil && req.NativeToolSet.ExternalActionIntentMatched
}

func nativeExternalActionCandidateEvidence(req RunRequest) []interface{} {
	var candidates []skills.NativeExternalActionCandidate
	if req.NativeSkillSession != nil {
		candidates = req.NativeSkillSession.ToolSet().ExternalActionCandidates
	} else if req.NativeToolSet != nil {
		candidates = req.NativeToolSet.ExternalActionCandidates
	}
	out := make([]interface{}, 0, len(candidates))
	for _, candidate := range candidates {
		integrationID := strings.ToLower(strings.TrimSpace(candidate.IntegrationID))
		actionID := strings.ToLower(strings.TrimSpace(candidate.ActionID))
		fingerprint := strings.TrimSpace(candidate.BindingFingerprint)
		if integrationID == "" || actionID == "" || fingerprint == "" {
			continue
		}
		value := map[string]interface{}{
			"integration_id":      integrationID,
			"action_id":           actionID,
			"binding_fingerprint": fingerprint,
		}
		if connectionBinding := strings.TrimSpace(candidate.ConnectionBinding); connectionBinding != "" {
			value[operationPlanServerProjectedConnectionBindingKey] = connectionBinding
		}
		if group := strings.ToLower(strings.TrimSpace(candidate.IntentGroup)); group != "" {
			value["intent_group"] = group
		}
		if candidate.IntentMatched {
			value["intent_matched"] = true
		}
		if candidate.Pinned {
			value["pinned"] = true
		}
		if effect := strings.ToLower(strings.TrimSpace(candidate.Effect)); effect != "" {
			value["effect"] = effect
		}
		if tokens := compactStringSlice(candidate.IntentTokens, 24, 120); len(tokens) > 0 {
			value["intent_tokens"] = stringSliceInterfaces(tokens)
		}
		if paths := compactStringSlice(candidate.TargetArgumentPaths, 16, 160); len(paths) > 0 {
			value["target_argument_paths"] = stringSliceInterfaces(paths)
		}
		if defaults := nativeExternalActionDefaultArgumentEvidence(candidate.DefaultArguments); len(defaults) > 0 {
			value["default_arguments"] = defaults
		}
		if optionalTargets := nativeExternalActionOptionalTargetEvidence(candidate.OptionalTargets); len(optionalTargets) > 0 {
			value["optional_targets"] = optionalTargets
		}
		if ids := compactStringSlice(candidate.PreparationActionIDs, 16, 160); len(ids) > 0 {
			keys := make([]string, 0, len(ids))
			for _, id := range ids {
				keys = append(keys, integrationID+":"+strings.ToLower(strings.TrimSpace(id)))
			}
			value["preparation_action_keys"] = stringSliceInterfaces(keys)
		}
		if hints := nativeExternalActionPreparationHintEvidence(integrationID, candidate.PreparationHints); len(hints) > 0 {
			value["preparation_hints"] = hints
		}
		if argument := strings.TrimSpace(candidate.ResultLimitArgument); argument != "" && candidate.ResultLimitDefault > 0 {
			value["result_limit_argument"] = argument
			value["result_limit_default"] = candidate.ResultLimitDefault
		}
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := evidenceMapFromAny(out[i])
		right := evidenceMapFromAny(out[j])
		return evidenceStringFromAny(left["integration_id"])+":"+evidenceStringFromAny(left["action_id"]) <
			evidenceStringFromAny(right["integration_id"])+":"+evidenceStringFromAny(right["action_id"])
	})
	return out
}

func stringSliceInterfaces(values []string) []interface{} {
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func nativeExternalActionProjectionEvidence(req RunRequest) []interface{} {
	var toolSet skills.NativeToolSet
	if req.NativeSkillSession != nil {
		toolSet = req.NativeSkillSession.ToolSet()
	} else if req.NativeToolSet != nil {
		toolSet = *req.NativeToolSet
	} else {
		return nil
	}
	type projectionEvidence struct {
		toolName string
		value    map[string]interface{}
	}
	items := make([]projectionEvidence, 0)
	for toolName, binding := range toolSet.ToolBindings {
		integrationID, actionID, ok := nativeProjectedExternalActionIdentity(binding)
		if !ok {
			continue
		}
		value := map[string]interface{}{
			"tool_name":      strings.TrimSpace(toolName),
			"integration_id": integrationID,
			"action_id":      actionID,
		}
		if effect := strings.ToLower(strings.TrimSpace(binding.Effect)); effect != "" {
			value["effect"] = effect
		}
		if binding.IntentMatched {
			value["intent_matched"] = true
		}
		if group := strings.ToLower(strings.TrimSpace(binding.IntentGroup)); group != "" {
			value["intent_group"] = group
		}
		if fingerprint := strings.TrimSpace(binding.BindingFingerprint); fingerprint != "" {
			value["binding_fingerprint"] = fingerprint
		}
		connectionBinding := strings.TrimSpace(binding.ConnectionBinding)
		if connectionBinding == "" {
			connectionBinding = skills.NativeExternalActionConnectionBindingHash(evidenceStringFromAny(binding.FixedArguments["connection_id"]))
		}
		if connectionBinding != "" {
			value[operationPlanServerProjectedConnectionBindingKey] = connectionBinding
		}
		for _, key := range []string{"action_schema_hash", "action_schema_revision", "catalog_revision"} {
			if revision := strings.TrimSpace(evidenceStringFromAny(binding.FixedArguments[key])); revision != "" {
				value[key] = revision
			}
		}
		if len(binding.TargetArgumentPaths) > 0 {
			paths := make([]interface{}, 0, len(binding.TargetArgumentPaths))
			for _, path := range binding.TargetArgumentPaths {
				if path = strings.TrimSpace(path); path != "" {
					paths = append(paths, path)
				}
			}
			if len(paths) > 0 {
				value["target_argument_paths"] = paths
			}
		}
		if defaults := nativeExternalActionDefaultArgumentEvidence(binding.DefaultArguments); len(defaults) > 0 {
			value["default_arguments"] = defaults
		}
		if optionalTargets := nativeExternalActionOptionalTargetEvidence(binding.OptionalTargets); len(optionalTargets) > 0 {
			value["optional_targets"] = optionalTargets
		}
		if len(binding.PreparationActionIDs) > 0 {
			keys := make([]interface{}, 0, len(binding.PreparationActionIDs))
			seen := map[string]struct{}{}
			for _, preparationActionID := range binding.PreparationActionIDs {
				preparationActionID = strings.ToLower(strings.TrimSpace(preparationActionID))
				if preparationActionID == "" {
					continue
				}
				key := integrationID + ":" + preparationActionID
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				keys = append(keys, key)
			}
			sort.SliceStable(keys, func(i, j int) bool {
				return evidenceStringFromAny(keys[i]) < evidenceStringFromAny(keys[j])
			})
			if len(keys) > 0 {
				value["preparation_action_keys"] = keys
			}
		}
		if hints := nativeExternalActionPreparationHintEvidence(integrationID, binding.PreparationHints); len(hints) > 0 {
			value["preparation_hints"] = hints
		}
		items = append(items, projectionEvidence{toolName: strings.TrimSpace(toolName), value: value})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].toolName) < strings.ToLower(items[j].toolName)
	})
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, item.value)
	}
	return out
}

func nativeExternalActionDefaultArgumentEvidence(defaults map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(defaults))
	for key, value := range defaults {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		switch value.(type) {
		case string, bool,
			int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64,
			float32, float64, json.Number:
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func nativeExternalActionOptionalTargetEvidence(
	targets []skills.NativeExternalActionOptionalTargetArgument,
) []interface{} {
	out := make([]interface{}, 0, len(targets))
	for _, target := range targets {
		path := strings.TrimSpace(target.Path)
		whenArgument := strings.TrimSpace(target.WhenArgument)
		if path == "" || whenArgument == "" {
			continue
		}
		whenEquals := nativeExternalActionDefaultArgumentEvidence(map[string]interface{}{
			whenArgument: target.WhenEquals,
		})
		value, safe := whenEquals[whenArgument]
		if !safe {
			continue
		}
		out = append(out, map[string]interface{}{
			"path": path, "when_argument": whenArgument, "when_equals": value,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func nativeExternalActionPreparationHintEvidence(
	integrationID string,
	hints []skills.NativeExternalActionPreparationHint,
) []interface{} {
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	out := make([]interface{}, 0, len(hints))
	for _, hint := range hints {
		actionID := strings.ToLower(strings.TrimSpace(hint.ActionID))
		relation := strings.ToLower(strings.TrimSpace(hint.Relation))
		targetArguments := compactStringSlice(hint.TargetArguments, 8, 160)
		resultPaths := compactStringSlice(hint.ResultPaths, 16, 256)
		if integrationID == "" || actionID == "" || relation == "" || len(targetArguments) == 0 || len(resultPaths) == 0 {
			continue
		}
		item := map[string]interface{}{
			"action_key":       integrationID + ":" + actionID,
			"relation":         relation,
			"target_arguments": stringSliceInterfaces(targetArguments),
			"result_paths":     stringSliceInterfaces(resultPaths),
		}
		if transform := strings.ToLower(strings.TrimSpace(hint.ResultTransform)); transform != "" {
			item["result_transform"] = transform
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := evidenceMapFromAny(out[i])
		right := evidenceMapFromAny(out[j])
		return evidenceStringFromAny(left["action_key"])+":"+evidenceStringFromAny(left["relation"]) <
			evidenceStringFromAny(right["action_key"])+":"+evidenceStringFromAny(right["relation"])
	})
	return out
}

func nativeProjectedExternalActionIdentity(binding skills.NativeToolBinding) (string, string, bool) {
	if !strings.EqualFold(strings.TrimSpace(binding.SkillID), skills.SkillExternalApps) ||
		!strings.EqualFold(strings.TrimSpace(binding.ToolName), "execute_action") ||
		!strings.EqualFold(strings.TrimSpace(binding.ArgumentEnvelope), "arguments") {
		return "", "", false
	}
	integrationID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(binding.FixedArguments["integration_id"])))
	actionID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(binding.FixedArguments["action_id"])))
	if integrationID == "" || actionID == "" {
		return "", "", false
	}
	return integrationID, actionID, true
}

func runtimeStateWithSuccessfulToolCalls(req RunRequest, successful []SkillToolCallRef) map[string]interface{} {
	state := runtimeStateForRun(req)
	if len(successful) == 0 {
		return state
	}
	invocations := evidenceSliceFromAny(state["skill_invocations"])
	existing := map[string]struct{}{}
	for _, raw := range invocations {
		invocation := evidenceMapFromAny(raw)
		if len(invocation) == 0 || !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["kind"])), "tool_call") {
			continue
		}
		signature := skillToolCallEvidenceSignature(
			evidenceStringFromAny(invocation["skill_id"]),
			evidenceStringFromAny(invocation["tool_name"]),
			evidenceMapFromAny(invocation["arguments"]),
			evidenceMapFromAny(invocation["result"]),
		)
		if signature != "" {
			existing[signature] = struct{}{}
		}
	}
	for _, call := range successful {
		skillID := strings.TrimSpace(call.SkillID)
		toolName := strings.TrimSpace(call.ToolName)
		if skillID == "" || toolName == "" {
			continue
		}
		signature := skillToolCallEvidenceSignature(skillID, toolName, call.Arguments, call.Result)
		if _, ok := existing[signature]; ok && signature != "" {
			continue
		}
		invocation := map[string]interface{}{
			"kind":      "tool_call",
			"status":    "success",
			"skill_id":  skillID,
			"tool_name": toolName,
		}
		if len(call.Arguments) > 0 {
			invocation["arguments"] = copyStringAnyMap(call.Arguments)
		}
		if len(call.Result) > 0 {
			invocation["result"] = copyStringAnyMap(call.Result)
		}
		invocations = append(invocations, invocation)
		if signature != "" {
			existing[signature] = struct{}{}
		}
	}
	if len(invocations) > 0 {
		state["skill_invocations"] = invocations
	}
	return state
}

func latestUserRequestText(req RunRequest) string {
	if req.Prepared != nil {
		if text := strings.TrimSpace(req.Prepared.Query); text != "" {
			return text
		}
	}
	return latestUserMessageText(req)
}

func latestUserMessageText(req RunRequest) string {
	if req.Prepared == nil || req.Prepared.LLMRequest == nil {
		return ""
	}
	messages := req.Prepared.LLMRequest.Messages
	for index := len(messages) - 1; index >= 0; index-- {
		if !strings.EqualFold(strings.TrimSpace(messages[index].Role), "user") {
			continue
		}
		if text := strings.TrimSpace(messageContent(messages[index].Content)); text != "" {
			return text
		}
	}
	return ""
}

func skillToolCallEvidenceSignature(skillID string, toolName string, arguments map[string]interface{}, result map[string]interface{}) string {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return ""
	}
	payload := map[string]interface{}{"skill_id": skillID, "tool_name": toolName}
	if len(arguments) > 0 {
		payload["arguments"] = arguments
	}
	if len(result) > 0 {
		payload["result"] = result
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return skillID + "/" + toolName
	}
	return string(data)
}

func runtimeInvocationSucceeded(invocation map[string]interface{}) bool {
	result := evidenceMapFromAny(invocation["result"])
	if len(result) == 0 {
		result = evidenceMapFromAny(invocation["result_summary"])
	}
	if runtimeResultHasFailedItems(result) {
		return false
	}
	resultStatus := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["status"])))
	if resultStatus == "" {
		resultStatus = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["result_status"])))
	}
	switch resultStatus {
	case "error", "failed", "partial_failed", "partially_failed":
		return false
	}
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["status"]))) {
	case "success", "succeeded", "completed", "allowed", "approved":
		return true
	}
	switch resultStatus {
	case "success", "succeeded", "completed", "allowed", "approved":
		return true
	default:
		return false
	}
}

func runtimeResultHasFailedItems(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	for _, source := range []map[string]interface{}{result, evidenceMapFromAny(result["operation_group"])} {
		if numericValue(source["failed_count"]) > 0 {
			return true
		}
		for _, item := range evidenceMapsFromAny(source["item_results"]) {
			switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(item["status"]))) {
			case "failed", "error", "blocked", "rejected":
				return true
			}
		}
	}
	return false
}

func numericValue(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func evidenceMapFromAny(value interface{}) map[string]interface{} {
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return nil
}

func evidenceSliceFromAny(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func evidenceMapsFromAny(value interface{}) []map[string]interface{} {
	if typed, ok := value.([]map[string]interface{}); ok {
		return typed
	}
	values := evidenceSliceFromAny(value)
	out := make([]map[string]interface{}, 0, len(values))
	for _, value := range values {
		if item := evidenceMapFromAny(value); len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func evidenceStringFromAny(value interface{}) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	if typed, ok := value.(fmt.Stringer); ok {
		return typed.String()
	}
	return fmt.Sprint(value)
}

func evidenceValuePresent(value interface{}) bool {
	return len(evidenceMapFromAny(value)) > 0 || len(evidenceSliceFromAny(value)) > 0
}

func mapSliceFromAny(value interface{}) []map[string]interface{} {
	return evidenceMapsFromAny(value)
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
