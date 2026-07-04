package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const maxModelInvocationMetadataRecords = 100

func (s *service) persistSkillTracesBestEffort(ctx context.Context, prepared *PreparedChat, traces []skills.SkillTrace) {
	if prepared == nil || prepared.Message == nil {
		return
	}
	metadata := mergeSkillTraceMetadata(prepared.Message.Metadata, traces)
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	_ = s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata)
}

func (s *service) persistGeneratedArtifactBestEffort(ctx context.Context, prepared *PreparedChat, artifact map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(artifact) == 0 {
		return
	}
	metadata := mergeGeneratedArtifactMetadata(prepared.Message.Metadata, artifact)
	prepared.Message.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat generated artifact metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func (s *service) persistModelInvocationBestEffort(ctx context.Context, prepared *PreparedChat, trace skillloop.ModelInvocationTrace) {
	if prepared == nil || prepared.Message == nil {
		return
	}
	invocation := modelInvocationFromTrace(trace, runtimeUserSystemPrompt(prepared), shouldRedactModelInvocationRequest(prepared))
	if len(invocation) == 0 {
		return
	}
	metadata := mergeModelInvocationMetadata(prepared.Message.Metadata, invocation)
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat model invocation metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func mergeGeneratedArtifactMetadata(source map[string]interface{}, artifact map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata = mergeConversationArtifactMetadata(metadata, artifact)
	storedArtifact := persistentGeneratedArtifact(artifact)
	files := generatedFilesFromMetadata(metadata["generated_files"])
	fileID := stringFromAny(storedArtifact["file_id"])
	for idx, item := range files {
		if fileID != "" && stringFromAny(item["file_id"]) == fileID {
			files[idx] = storedArtifact
			metadata["generated_files"] = files
			metadata["generated_file_count"] = len(files)
			applyOperationPlanArtifactState(metadata, files)
			return metadata
		}
	}
	files = append(files, storedArtifact)
	metadata["generated_files"] = files
	metadata["generated_file_count"] = len(files)
	applyOperationPlanArtifactState(metadata, files)
	return metadata
}

func mergeSkillTraceMetadata(source map[string]interface{}, traces []skills.SkillTrace) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(traces) == 0 {
		return metadata
	}
	invocations := sanitizeSkillInvocationsForMetadata(skillInvocationsFromMetadata(metadata["skill_invocations"]))
	for index, trace := range traces {
		if !visibleSkillInvocationKind(trace.Kind) {
			continue
		}
		invocation := skillInvocationFromTrace(trace, index)
		if internalPlannerFeedbackInvocation(invocation) {
			continue
		}
		invocations = upsertSkillInvocation(invocations, invocation)
	}
	applySkillInvocationSummary(metadata, invocations)
	applyOperationPlanInvocationState(metadata, invocations)
	applyOperationPlanPlannerFeedbackState(metadata, traces)
	return metadata
}

func mergeSkillInvocationMetadata(source map[string]interface{}, invocations []map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(invocations) == 0 {
		return metadata
	}
	stored := sanitizeSkillInvocationsForMetadata(skillInvocationsFromMetadata(metadata["skill_invocations"]))
	for _, invocation := range invocations {
		invocation = sanitizeSkillInvocationForMetadata(invocation)
		if !visibleSkillInvocationKind(stringFromAny(invocation["kind"])) {
			continue
		}
		if internalPlannerFeedbackInvocation(invocation) {
			continue
		}
		stored = upsertSkillInvocation(stored, invocation)
	}
	applySkillInvocationSummary(metadata, stored)
	applyOperationPlanInvocationState(metadata, stored)
	return metadata
}

func mergeModelInvocationMetadata(source map[string]interface{}, invocation map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(invocation) == 0 {
		return metadata
	}
	stored := modelInvocationsFromMetadata(metadata["model_invocations"])
	runtimeID := strings.TrimSpace(stringFromAny(invocation["runtime_id"]))
	replaced := false
	if runtimeID != "" {
		for index, item := range stored {
			if strings.TrimSpace(stringFromAny(item["runtime_id"])) == runtimeID {
				stored[index] = mergeInvocation(item, invocation)
				replaced = true
				break
			}
		}
	}
	if !replaced {
		stored = append(stored, compactSkillInvocation(invocation))
	}
	if len(stored) > maxModelInvocationMetadataRecords {
		stored = stored[len(stored)-maxModelInvocationMetadataRecords:]
	}
	metadata["model_invocations"] = skillInvocationsToInterfaceSlice(stored)
	metadata["model_invocation_count"] = len(stored)
	return metadata
}

func modelInvocationFromTrace(trace skillloop.ModelInvocationTrace, userSystemPrompt string, redactRequest bool) map[string]interface{} {
	phase := strings.TrimSpace(trace.Phase)
	if phase == "" {
		phase = "model_call"
	}
	startedAt := trace.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	status := "success"
	if strings.TrimSpace(trace.Error) != "" {
		status = "error"
	}
	invocation := map[string]interface{}{
		"kind":        "model_call",
		"phase":       phase,
		"round":       trace.Round,
		"streaming":   trace.Streaming,
		"status":      status,
		"title":       modelInvocationTitle(phase, trace.Round),
		"created_at":  startedAt.Unix(),
		"duration_ms": trace.DurationMS,
		"runtime_id":  fmt.Sprintf("model_call:%s:%d:%d", phase, trace.Round, startedAt.UnixNano()),
		"request":     modelInvocationRequestPayload(trace.Request, redactRequest),
		"response":    modelInvocationResponsePayload(trace.Response, trace.Usage),
		"usage":       usageMetadata(trace.Usage),
		"error":       strings.TrimSpace(trace.Error),
	}
	if trace.Request != nil {
		invocation["model"] = trace.Request.Model
		invocation["provider"] = trace.Request.Provider
	}
	if trace.Usage != nil {
		invocation["prompt_tokens"] = trace.Usage.PromptTokens
		invocation["completion_tokens"] = trace.Usage.CompletionTokens
		invocation["total_tokens"] = trace.Usage.TotalTokens
	}
	if strings.TrimSpace(userSystemPrompt) != "" {
		invocation["user_system_prompt"] = strings.TrimSpace(userSystemPrompt)
	}
	return compactSkillInvocation(invocation)
}

func shouldRedactModelInvocationRequest(prepared *PreparedChat) bool {
	if prepared == nil {
		return true
	}
	return normalizeCallerType(prepared.Caller.Type) != runtimemodel.ConversationCallerAgent
}

func runtimeUserSystemPrompt(prepared *PreparedChat) string {
	if prepared == nil || prepared.parts == nil {
		return ""
	}
	return strings.TrimSpace(prepared.parts.SystemPrompt)
}

func modelInvocationTitle(phase string, round int) string {
	switch phase {
	case "final_answer":
		return "Model call: final answer"
	case "skill_planning":
		if round >= 0 {
			return fmt.Sprintf("Model call: skill planning #%d", round+1)
		}
		return "Model call: skill planning"
	default:
		return "Model call"
	}
}

func modelInvocationRequestPayload(req *adapter.ChatRequest, redactToolContent bool) map[string]interface{} {
	if req == nil {
		return map[string]interface{}{}
	}
	payload := jsonObjectPayload(req)
	if redactToolContent {
		payload = sanitizeModelInvocationRequestPayload(payload)
	}
	if strings.TrimSpace(req.Provider) != "" {
		payload["provider"] = req.Provider
	}
	if len(req.AdditionalParameters) > 0 {
		payload["additional_parameters"] = copyStringAnyMap(req.AdditionalParameters)
	}
	return payload
}

func sanitizeModelInvocationRequestPayload(payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return payload
	}
	messages, ok := payload["messages"].([]interface{})
	if !ok {
		return payload
	}
	for _, raw := range messages {
		message, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(stringFromAny(message["role"])))
		switch role {
		case "tool", "function":
			content, exists := message["content"]
			if !exists {
				continue
			}
			message["content"] = modelInvocationToolContentSummary(content)
			message["content_redacted"] = true
		case "user":
			content, exists := message["content"]
			if !exists {
				continue
			}
			if summary, ok := modelInvocationUserContentSummary(content); ok {
				message["content"] = summary
				message["content_redacted"] = true
			}
		}
	}
	return payload
}

func modelInvocationUserContentSummary(content interface{}) (map[string]interface{}, bool) {
	summary := map[string]interface{}{
		"redacted": true,
		"reason":   "user_payload_file_content_omitted_from_model_invocation_metadata",
	}
	if content == nil {
		return nil, false
	}
	switch typed := content.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		var parsed interface{}
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			return nil, false
		}
		jsonSummary, ok := modelInvocationUserJSONSummary(parsed)
		if !ok {
			return nil, false
		}
		summary["original_type"] = "string"
		summary["content_chars"] = len([]rune(typed))
		summary["json"] = jsonSummary
	case map[string]interface{}:
		jsonSummary, ok := modelInvocationUserJSONSummary(typed)
		if !ok {
			return nil, false
		}
		summary["original_type"] = "object"
		summary["json"] = jsonSummary
	default:
		return nil, false
	}
	return summary, true
}

func modelInvocationUserJSONSummary(value interface{}) (map[string]interface{}, bool) {
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, false
	}
	files := modelInvocationFileItemsFromAny(object["files"])
	if len(files) == 0 {
		return nil, false
	}
	fileSummaries, redacted := modelInvocationPayloadFilesSummary(files)
	if !redacted {
		return nil, false
	}
	fields := map[string]interface{}{
		"files": fileSummaries,
	}
	if userQuery := strings.TrimSpace(stringFromAny(object["user_query"])); userQuery != "" {
		fields["user_query"] = userQuery
	}
	if postprocess, ok := modelInvocationPayloadSafeValue(object["postprocess"]); ok {
		fields["postprocess"] = postprocess
	}
	if fallback := strings.TrimSpace(stringFromAny(object["fallback_answer"])); fallback != "" {
		fields["fallback_answer_chars"] = len([]rune(fallback))
		fields["fallback_answer_redacted"] = true
	}
	return map[string]interface{}{
		"type":      "object",
		"json_keys": sortedStringKeys(object),
		"fields":    fields,
	}, true
}

func modelInvocationPayloadSafeValue(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		return text, true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if safe, ok := modelInvocationPayloadSafeValue(item); ok {
				out = append(out, safe)
			}
		}
		return out, len(out) > 0
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for _, key := range sortedStringKeys(typed) {
			if safe, ok := modelInvocationPayloadSafeValue(typed[key]); ok {
				out[key] = safe
			}
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

func modelInvocationToolContentSummary(content interface{}) map[string]interface{} {
	summary := map[string]interface{}{
		"redacted": true,
		"reason":   "tool_result_content_omitted_from_model_invocation_metadata",
	}
	if content == nil {
		return summary
	}
	switch typed := content.(type) {
	case string:
		summary["original_type"] = "string"
		summary["content_chars"] = len([]rune(typed))
		if strings.TrimSpace(typed) != "" {
			var parsed interface{}
			if err := json.Unmarshal([]byte(typed), &parsed); err == nil {
				summary["json"] = modelInvocationToolJSONSummary(parsed)
			}
		}
	case map[string]interface{}:
		summary["original_type"] = "object"
		summary["json"] = modelInvocationToolJSONSummary(typed)
	case []interface{}:
		summary["original_type"] = "array"
		summary["item_count"] = len(typed)
		summary["json"] = modelInvocationToolJSONSummary(typed)
	default:
		summary["original_type"] = fmt.Sprintf("%T", content)
	}
	return summary
}

func modelInvocationToolJSONSummary(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return modelInvocationToolObjectSummary(typed)
	case []interface{}:
		return map[string]interface{}{
			"type":       "array",
			"item_count": len(typed),
		}
	default:
		return map[string]interface{}{"type": fmt.Sprintf("%T", value)}
	}
}

func modelInvocationToolObjectSummary(value map[string]interface{}) map[string]interface{} {
	summary := map[string]interface{}{
		"type":      "object",
		"json_keys": sortedStringKeys(value),
	}
	fields := map[string]interface{}{}
	for _, key := range []string{
		"status",
		"content_status",
		"content_chars",
		"content_truncated",
		"from_cache",
		"file_id",
		"file_ids",
		"name",
		"extension",
		"mime_type",
		"workspace_id",
		"count",
		"selected_count",
	} {
		if sanitized, ok := modelInvocationSafeSummaryValue(value[key]); ok {
			fields[key] = sanitized
		}
	}
	if _, exists := value["content"]; exists {
		fields["content_redacted"] = true
	}
	if errorText := strings.TrimSpace(stringFromAny(value["content_error"])); errorText != "" {
		fields["content_error_chars"] = len([]rune(errorText))
	}
	if files := modelInvocationFileItemsFromAny(value["files"]); len(files) > 0 {
		fields["files"] = modelInvocationToolFilesSummary(files)
	}
	if len(fields) > 0 {
		summary["fields"] = fields
	}
	return summary
}

func modelInvocationSafeSummaryValue(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		return text, true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if safe, ok := modelInvocationSafeSummaryValue(item); ok {
				out = append(out, safe)
			}
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

func modelInvocationToolFilesSummary(files []interface{}) []map[string]interface{} {
	summaries, _ := modelInvocationPayloadFilesSummary(files)
	return summaries
}

func modelInvocationFileItemsFromAny(value interface{}) []interface{} {
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

func modelInvocationPayloadFilesSummary(files []interface{}) ([]map[string]interface{}, bool) {
	out := make([]map[string]interface{}, 0, min(len(files), 20))
	redacted := false
	for idx, raw := range files {
		if idx >= 20 {
			break
		}
		file, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		item := map[string]interface{}{}
		for _, key := range []string{"visible_index", "id", "file_id", "name", "extension", "mime_type", "file_type", "workspace_id", "size", "content_status", "content_chars", "filtered_reason", "content_truncated", "from_cache", "selected"} {
			if safe, ok := modelInvocationSafeSummaryValue(file[key]); ok {
				item[key] = safe
			}
		}
		if preview := strings.TrimSpace(stringFromAny(file["content_preview"])); preview != "" {
			item["content_preview_chars"] = len([]rune(preview))
			item["content_preview_redacted"] = true
			redacted = true
		}
		if content := strings.TrimSpace(stringFromAny(file["content"])); content != "" {
			setRedactedTextSummary(item, "content", content)
			redacted = true
		}
		if contentError := strings.TrimSpace(stringFromAny(file["content_error"])); contentError != "" {
			item["content_error_chars"] = len([]rune(contentError))
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
	return out, redacted
}

func sortedStringKeys(value map[string]interface{}) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func modelInvocationResponsePayload(message *adapter.Message, usage *adapter.Usage) map[string]interface{} {
	payload := map[string]interface{}{}
	if message != nil {
		payload["message"] = jsonObjectPayload(message)
	}
	if usageMap := usageMetadata(usage); len(usageMap) > 0 {
		payload["usage"] = usageMap
	}
	return payload
}

func jsonObjectPayload(value interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]interface{}{"value": fmt.Sprintf("%v", value)}
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err == nil && payload != nil {
		return payload
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]interface{}{"value": string(data)}
	}
	return map[string]interface{}{"value": raw}
}

func applySkillInvocationSummary(metadata map[string]interface{}, invocations []map[string]interface{}) {
	invocations = sortSkillInvocationsForMetadata(invocations)
	selected := make([]interface{}, 0)
	loaded := make([]interface{}, 0)
	toolsUsed := make([]interface{}, 0)
	selectedSeen := map[string]struct{}{}
	loadedSeen := map[string]struct{}{}
	toolSeen := map[string]struct{}{}
	toolCallCount := 0
	guardrailCount := 0
	addConfiguredSkillIDs(metadata, selectedSeen, &selected)

	for _, invocation := range invocations {
		skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
		kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
		status := strings.TrimSpace(stringFromAny(invocation["status"]))
		toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
		if !visibleSkillInvocationKind(kind) {
			continue
		}
		if skillID != "" {
			if _, exists := selectedSeen[skillID]; !exists {
				selectedSeen[skillID] = struct{}{}
				selected = append(selected, skillID)
			}
		}
		if kind == "skill_load" && status == "success" {
			if _, exists := loadedSeen[skillID]; skillID != "" && !exists {
				loadedSeen[skillID] = struct{}{}
				loaded = append(loaded, skillID)
			}
		}
		if kind == "tool_call" {
			toolCallCount++
			if _, exists := toolSeen[toolName]; toolName != "" && !exists {
				toolSeen[toolName] = struct{}{}
				toolsUsed = append(toolsUsed, toolName)
			}
		}
		if kind == "guardrail" {
			guardrailCount++
		}
	}
	metadata["has_trace"] = true
	metadata["selected_skill_ids"] = selected
	metadata["loaded_skill_ids"] = loaded
	actionTraceCount := countSkillActionInvocations(invocations)
	metadata["skill_step_count"] = actionTraceCount
	metadata["skill_call_count"] = actionTraceCount
	metadata["tool_call_count"] = toolCallCount
	metadata["guardrail_count"] = guardrailCount
	metadata["skill_names"] = selected
	metadata["tool_names"] = toolsUsed
	metadata["skill_invocations"] = skillInvocationsToInterfaceSlice(invocations)
}

func sortSkillInvocationsForMetadata(invocations []map[string]interface{}) []map[string]interface{} {
	if len(invocations) <= 1 {
		return invocations
	}
	sort.SliceStable(invocations, func(i, j int) bool {
		leftAt, leftOK := skillInvocationTimelineMilliseconds(invocations[i])
		rightAt, rightOK := skillInvocationTimelineMilliseconds(invocations[j])
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return false
		}
		return leftAt < rightAt
	})
	return invocations
}

func skillInvocationTimelineMilliseconds(invocation map[string]interface{}) (int64, bool) {
	if at, ok := unixMillisecondsFromAny(invocation["created_at_ms"]); ok {
		return at, true
	}
	at, ok := skillInvocationTimelineUnix(invocation)
	if !ok {
		return 0, false
	}
	return at * 1000, true
}

func skillInvocationTimelineUnix(invocation map[string]interface{}) (int64, bool) {
	for _, key := range []string{"created_at", "resolved_at", "updated_at"} {
		if at, ok := unixSecondsFromAny(invocation[key]); ok {
			return at, true
		}
	}
	return 0, false
}

func unixSecondsFromAny(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
		return 0, false
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return parsed, true
		}
		if timestamp, err := time.Parse(time.RFC3339, text); err == nil {
			return timestamp.Unix(), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func unixMillisecondsFromAny(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return parsed, true
		}
		return 0, false
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return parsed, true
		}
		if timestamp, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return timestamp.UnixMilli(), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func skillInvocationsFromMetadata(value interface{}) []map[string]interface{} {
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
			if invocation, ok := item.(map[string]interface{}); ok {
				out = append(out, copyStringAnyMap(invocation))
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func modelInvocationsFromMetadata(value interface{}) []map[string]interface{} {
	return skillInvocationsFromMetadata(value)
}

func skillInvocationsToInterfaceSlice(invocations []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(invocations))
	for _, invocation := range invocations {
		out = append(out, invocation)
	}
	return out
}

func skillInvocationFromTrace(trace skills.SkillTrace, index int) map[string]interface{} {
	invocation := map[string]interface{}{
		"kind":        trace.Kind,
		"skill_id":    trace.SkillID,
		"tool_name":   trace.ToolName,
		"title":       trace.Title,
		"status":      trace.Status,
		"duration_ms": trace.DurationMS,
		"arguments":   trace.Arguments,
		"result":      trace.Result,
		"message":     trace.Message,
		"error":       trace.Error,
		"runtime_id":  traceRuntimeID(trace, index),
	}
	if trace.Governance != nil {
		invocation["governance"] = trace.Governance
	}
	if path := firstNonEmptyString(valueFromMap(trace.Arguments, "path"), valueFromMap(trace.Result, "path")); path != "" {
		invocation["path"] = path
	}
	if answerID := firstNonEmptyString(valueFromMap(trace.Arguments, "answer_id"), valueFromMap(trace.Result, "answer_id")); answerID != "" {
		invocation["answer_id"] = answerID
	}
	if createdAt := numericValueFromMap(trace.Arguments, "created_at"); createdAt != nil {
		invocation["created_at"] = createdAt
	} else if createdAt := numericValueFromMap(trace.Result, "created_at"); createdAt != nil {
		invocation["created_at"] = createdAt
	}
	if createdAtMS := numericValueFromMap(trace.Arguments, "created_at_ms"); createdAtMS != nil {
		invocation["created_at_ms"] = createdAtMS
	} else if createdAtMS := numericValueFromMap(trace.Result, "created_at_ms"); createdAtMS != nil {
		invocation["created_at_ms"] = createdAtMS
	}
	normalizeSkillInvocationTimelineFields(invocation)
	return sanitizeSkillInvocationForMetadata(compactSkillInvocation(invocation))
}

func sanitizeSkillInvocationsForMetadata(invocations []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(invocations))
	for _, invocation := range invocations {
		if internalPlannerFeedbackInvocation(invocation) {
			continue
		}
		out = append(out, sanitizeSkillInvocationForMetadata(invocation))
	}
	return out
}

func sanitizeSkillInvocationForMetadata(invocation map[string]interface{}) map[string]interface{} {
	if len(invocation) == 0 {
		return invocation
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	if skillID != skills.SkillFileReader && skillID != skills.SkillFileManager {
		return invocation
	}
	result, ok := invocation["result"].(map[string]interface{})
	if !ok || len(result) == 0 {
		return invocation
	}
	sanitizedResult, changed := sanitizeFileReaderResultForMetadata(result)
	if !changed {
		return invocation
	}
	out := copyStringAnyMap(invocation)
	out["result"] = sanitizedResult
	return out
}

func sanitizeFileReaderResultForMetadata(result map[string]interface{}) (map[string]interface{}, bool) {
	out := copyStringAnyMap(result)
	changed := false
	if redactTextFieldForMetadata(out, "content", "content") {
		changed = true
	}
	if redactTextFieldForMetadata(out, "content_preview", "content_preview") {
		changed = true
	}
	if redactTextFieldForMetadata(out, "content_error", "content_error") {
		changed = true
	}
	if file, ok := out["file"].(map[string]interface{}); ok && len(file) > 0 {
		out["file"] = fileReaderMetadataFileSummary(file)
		changed = true
	}
	if files := modelInvocationFileItemsFromAny(out["files"]); len(files) > 0 {
		fileSummaries, redacted := modelInvocationPayloadFilesSummary(files)
		if len(fileSummaries) > 0 {
			out["files"] = fileSummaries
		}
		if redacted {
			out["files_content_redacted"] = true
		}
		changed = changed || redacted
	}
	return out, changed
}

func redactTextFieldForMetadata(payload map[string]interface{}, key string, prefix string) bool {
	if payload == nil {
		return false
	}
	value, exists := payload[key]
	if !exists {
		return false
	}
	delete(payload, key)
	setRedactedTextSummary(payload, prefix, strings.TrimSpace(stringFromAny(value)))
	return true
}

func setRedactedTextSummary(payload map[string]interface{}, prefix string, text string) {
	if payload == nil {
		return
	}
	payload[prefix+"_redacted"] = true
	if text == "" {
		return
	}
	charsKey := prefix + "_chars"
	if prefix == "content" {
		if _, exists := payload["content_chars"]; exists {
			charsKey = "content_returned_chars"
		}
	}
	payload[charsKey] = len([]rune(text))
}

func fileReaderMetadataFileSummary(file map[string]interface{}) map[string]interface{} {
	item := map[string]interface{}{}
	for _, key := range []string{"visible_index", "id", "file_id", "name", "extension", "mime_type", "file_type", "workspace_id", "size", "selected", "content_status", "content_chars", "content_truncated", "from_cache"} {
		if safe, ok := modelInvocationSafeSummaryValue(file[key]); ok {
			item[key] = safe
		}
	}
	if preview := strings.TrimSpace(stringFromAny(file["content_preview"])); preview != "" {
		item["content_preview_chars"] = len([]rune(preview))
		item["content_preview_redacted"] = true
	}
	if content := strings.TrimSpace(stringFromAny(file["content"])); content != "" {
		setRedactedTextSummary(item, "content", content)
	}
	if contentError := strings.TrimSpace(stringFromAny(file["content_error"])); contentError != "" {
		item["content_error_chars"] = len([]rune(contentError))
		item["content_error_redacted"] = true
	}
	return item
}

func newSkillInvocation(kind, skillID, toolName, status string, values map[string]interface{}) map[string]interface{} {
	if values == nil {
		values = map[string]interface{}{}
	}
	now := time.Now()
	invocation := map[string]interface{}{
		"kind":          strings.TrimSpace(kind),
		"skill_id":      strings.TrimSpace(skillID),
		"tool_name":     strings.TrimSpace(toolName),
		"status":        strings.TrimSpace(status),
		"created_at":    now.Unix(),
		"created_at_ms": now.UnixMilli(),
	}
	_, hasProvidedCreatedAt := values["created_at"]
	_, hasProvidedCreatedAtMS := values["created_at_ms"]
	for key, value := range values {
		invocation[key] = value
	}
	if hasProvidedCreatedAt && !hasProvidedCreatedAtMS {
		delete(invocation, "created_at_ms")
	}
	normalizeSkillInvocationTimelineFields(invocation)
	if strings.TrimSpace(stringFromAny(invocation["runtime_id"])) == "" {
		invocation["runtime_id"] = invocationRuntimeIdentity(invocation)
	}
	return compactSkillInvocation(invocation)
}

func normalizeSkillInvocationTimelineFields(invocation map[string]interface{}) {
	if len(invocation) == 0 {
		return
	}
	if createdAtMS, ok := unixMillisecondsFromAny(invocation["created_at_ms"]); ok && createdAtMS > 0 {
		invocation["created_at_ms"] = createdAtMS
		if _, ok := unixSecondsFromAny(invocation["created_at"]); !ok {
			invocation["created_at"] = createdAtMS / 1000
		}
		return
	}
	if createdAt, ok := unixSecondsFromAny(invocation["created_at"]); ok && createdAt > 0 {
		invocation["created_at"] = createdAt
		invocation["created_at_ms"] = createdAt * 1000
	}
}

func compactSkillInvocation(invocation map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(invocation))
	for key, value := range invocation {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		if number, ok := value.(int64); ok && number == 0 && key == "duration_ms" {
			continue
		}
		out[key] = value
	}
	return out
}

func upsertSkillInvocation(current []map[string]interface{}, incoming map[string]interface{}) []map[string]interface{} {
	if len(incoming) == 0 {
		return current
	}
	if runtimeID := strings.TrimSpace(stringFromAny(incoming["runtime_id"])); runtimeID != "" {
		for index, invocation := range current {
			if strings.TrimSpace(stringFromAny(invocation["runtime_id"])) == runtimeID {
				if shouldKeepExistingInvocation(invocation, incoming) {
					return current
				}
				current[index] = mergeInvocation(invocation, incoming)
				return current
			}
		}
	}
	for index, invocation := range current {
		if sameInvocationIdentity(invocation, incoming) && isOpenInvocation(invocation) {
			if shouldKeepExistingInvocation(invocation, incoming) {
				return current
			}
			current[index] = mergeInvocation(invocation, incoming)
			return current
		}
	}
	for index, invocation := range current {
		if sameInvocationIdentity(invocation, incoming) && shouldMergeClosedInvocation(invocation, incoming) {
			if shouldKeepExistingInvocation(invocation, incoming) {
				return current
			}
			current[index] = mergeInvocation(invocation, incoming)
			return current
		}
	}
	for _, invocation := range current {
		if reflect.DeepEqual(invocation, incoming) {
			return current
		}
	}
	return append(current, incoming)
}

func shouldKeepExistingInvocation(existing map[string]interface{}, incoming map[string]interface{}) bool {
	if strings.EqualFold(strings.TrimSpace(stringFromAny(existing["kind"])), "skill_load") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(incoming["kind"])), "skill_load") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(existing["status"])), "success") {
		return true
	}
	return false
}

func shouldMergeClosedInvocation(existing map[string]interface{}, incoming map[string]interface{}) bool {
	if strings.EqualFold(strings.TrimSpace(stringFromAny(existing["kind"])), "skill_load") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(incoming["kind"])), "skill_load") {
		return true
	}
	if strings.TrimSpace(stringFromAny(existing["kind"])) != "guardrail" ||
		strings.TrimSpace(stringFromAny(incoming["kind"])) != "guardrail" {
		return false
	}
	return strings.TrimSpace(stringFromAny(existing["message"])) == strings.TrimSpace(stringFromAny(incoming["message"])) &&
		strings.TrimSpace(stringFromAny(existing["error"])) == strings.TrimSpace(stringFromAny(incoming["error"]))
}

func mergeInvocation(existing map[string]interface{}, incoming map[string]interface{}) map[string]interface{} {
	merged := copyStringAnyMap(existing)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	for key, value := range incoming {
		if value == nil {
			continue
		}
		merged[key] = value
	}
	return compactSkillInvocation(merged)
}

func sameInvocationIdentity(left map[string]interface{}, right map[string]interface{}) bool {
	return invocationRuntimeIdentity(left) == invocationRuntimeIdentity(right)
}

func invocationRuntimeIdentity(invocation map[string]interface{}) string {
	parts := []string{
		strings.TrimSpace(stringFromAny(invocation["kind"])),
		strings.TrimSpace(stringFromAny(invocation["skill_id"])),
		strings.TrimSpace(stringFromAny(invocation["tool_name"])),
		strings.TrimSpace(stringFromAny(invocation["path"])),
		strings.TrimSpace(stringFromAny(invocation["answer_id"])),
	}
	return strings.Join(parts, ":")
}

func traceRuntimeID(trace skills.SkillTrace, index int) string {
	if runtimeID := firstNonEmptyString(valueFromMap(trace.Arguments, "runtime_id"), valueFromMap(trace.Result, "runtime_id")); runtimeID != "" {
		return runtimeID
	}
	return fmt.Sprintf("trace:%06d:%s", index, invocationRuntimeIdentity(map[string]interface{}{
		"kind":      trace.Kind,
		"skill_id":  trace.SkillID,
		"tool_name": trace.ToolName,
		"path":      firstNonEmptyString(valueFromMap(trace.Arguments, "path"), valueFromMap(trace.Result, "path")),
		"answer_id": firstNonEmptyString(valueFromMap(trace.Arguments, "answer_id"), valueFromMap(trace.Result, "answer_id")),
	}))
}

func isOpenInvocation(invocation map[string]interface{}) bool {
	switch strings.TrimSpace(stringFromAny(invocation["status"])) {
	case "loading", "running":
		return true
	default:
		return false
	}
}

func valueFromMap(values map[string]interface{}, key string) interface{} {
	if len(values) == 0 {
		return nil
	}
	return values[key]
}

func numericValueFromMap(values map[string]interface{}, key string) interface{} {
	return numericValueFromAny(valueFromMap(values, key))
}

func numericValueFromAny(value interface{}) interface{} {
	switch value.(type) {
	case int, int64, int32, float64, float32, uint, uint64, uint32:
		return value
	default:
		return nil
	}
}

func intValueFromAny(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
		return 0
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func countSkillActionInvocations(invocations []map[string]interface{}) int {
	count := 0
	for _, invocation := range invocations {
		if visibleSkillInvocationKind(stringFromAny(invocation["kind"])) {
			count++
		}
	}
	return count
}

func visibleSkillInvocationKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "skill_load", "reference_read", "tool_call", "tool_governance", "client_action", "intermediate_answer", "user_input_request", "guardrail":
		return true
	default:
		return false
	}
}

func internalPlannerFeedbackInvocation(invocation map[string]interface{}) bool {
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "guardrail" {
		return false
	}
	return operationPlanGuardrailIsPlanningFeedback(invocation)
}

func addConfiguredSkillIDs(metadata map[string]interface{}, seen map[string]struct{}, out *[]interface{}) {
	value, ok := metadata["configured_skill_ids"]
	if !ok {
		return
	}
	add := func(raw string) {
		id := strings.TrimSpace(raw)
		if id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		*out = append(*out, id)
	}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			add(item)
		}
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	}
}

func generatedFilesFromMetadata(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return append([]map[string]interface{}{}, typed...)
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if file, ok := item.(map[string]interface{}); ok {
				out = append(out, file)
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		text := strings.TrimSpace(stringFromAny(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func appendDownloadQuery(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if strings.Contains(rawURL, "download=") {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

func stringFromAny(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}
