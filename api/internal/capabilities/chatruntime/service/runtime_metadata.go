package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

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
	invocation := modelInvocationFromTrace(trace, runtimeUserSystemPrompt(prepared))
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
	storedArtifact := persistentGeneratedArtifact(artifact)
	files := generatedFilesFromMetadata(metadata["generated_files"])
	fileID := stringFromAny(storedArtifact["file_id"])
	for idx, item := range files {
		if fileID != "" && stringFromAny(item["file_id"]) == fileID {
			files[idx] = storedArtifact
			metadata["generated_files"] = files
			metadata["generated_file_count"] = len(files)
			return metadata
		}
	}
	files = append(files, storedArtifact)
	metadata["generated_files"] = files
	metadata["generated_file_count"] = len(files)
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
	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for index, trace := range traces {
		if !visibleSkillInvocationKind(trace.Kind) {
			continue
		}
		invocations = upsertSkillInvocation(invocations, skillInvocationFromTrace(trace, index))
	}
	applySkillInvocationSummary(metadata, invocations)
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
	stored := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for _, invocation := range invocations {
		if !visibleSkillInvocationKind(stringFromAny(invocation["kind"])) {
			continue
		}
		stored = upsertSkillInvocation(stored, invocation)
	}
	applySkillInvocationSummary(metadata, stored)
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

func modelInvocationFromTrace(trace skillloop.ModelInvocationTrace, userSystemPrompt string) map[string]interface{} {
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
		"request":     modelInvocationRequestPayload(trace.Request),
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

func modelInvocationRequestPayload(req *adapter.ChatRequest) map[string]interface{} {
	if req == nil {
		return map[string]interface{}{}
	}
	payload := jsonObjectPayload(req)
	if strings.TrimSpace(req.Provider) != "" {
		payload["provider"] = req.Provider
	}
	if len(req.AdditionalParameters) > 0 {
		payload["additional_parameters"] = copyStringAnyMap(req.AdditionalParameters)
	}
	return payload
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
	return compactSkillInvocation(invocation)
}

func newSkillInvocation(kind, skillID, toolName, status string, values map[string]interface{}) map[string]interface{} {
	invocation := map[string]interface{}{
		"kind":       strings.TrimSpace(kind),
		"skill_id":   strings.TrimSpace(skillID),
		"tool_name":  strings.TrimSpace(toolName),
		"status":     strings.TrimSpace(status),
		"created_at": time.Now().Unix(),
	}
	for key, value := range values {
		invocation[key] = value
	}
	if strings.TrimSpace(stringFromAny(invocation["runtime_id"])) == "" {
		invocation["runtime_id"] = invocationRuntimeIdentity(invocation)
	}
	return compactSkillInvocation(invocation)
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
				current[index] = mergeInvocation(invocation, incoming)
				return current
			}
		}
	}
	for index, invocation := range current {
		if sameInvocationIdentity(invocation, incoming) && isOpenInvocation(invocation) {
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
	case "skill_load", "reference_read", "tool_call", "intermediate_answer", "user_input_request", "guardrail":
		return true
	default:
		return false
	}
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
