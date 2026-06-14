package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	modelInvocationSchema                  = "zgi.model_invocation.v2"
	modelInvocationRequestSummarySchema    = "zgi.model_invocation.request_summary.v1"
	modelInvocationResponseSummarySchema   = "zgi.model_invocation.response_summary.v1"
	modelInvocationDebugSchema             = "zgi.model_invocation.debug_trace.v1"
	modelInvocationTraceLevelSummary       = "summary"
	modelInvocationTraceLevelRawDebug      = "raw_debug"
	debugModelInvocationsMetadataKey       = "debug_model_invocations"
	modelInvocationRawDebugEnv             = "ZGI_AICHAT_DEBUG_RAW_MODEL_INVOCATIONS"
	maxModelInvocationMetadataRecords      = 100
	maxDebugModelInvocationMetadataRecords = 100
	modelInvocationDebugRetention          = 7 * 24 * time.Hour
)

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
	metadata = mergeDebugModelInvocationMetadata(metadata, debugModelInvocationFromTrace(trace, runtimeUserSystemPrompt(prepared)), time.Now())
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

func mergeDebugModelInvocationMetadata(source map[string]interface{}, invocation map[string]interface{}, now time.Time) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	stored := pruneDebugModelInvocations(modelInvocationsFromMetadata(metadata[debugModelInvocationsMetadataKey]), now)
	if len(invocation) > 0 {
		stored = upsertSkillInvocation(stored, invocation)
	}
	if len(stored) > maxDebugModelInvocationMetadataRecords {
		stored = stored[len(stored)-maxDebugModelInvocationMetadataRecords:]
	}
	if len(stored) == 0 {
		delete(metadata, debugModelInvocationsMetadataKey)
		delete(metadata, "debug_model_invocation_count")
		return metadata
	}
	metadata[debugModelInvocationsMetadataKey] = skillInvocationsToInterfaceSlice(stored)
	metadata["debug_model_invocation_count"] = len(stored)
	return metadata
}

func pruneDebugModelInvocations(invocations []map[string]interface{}, now time.Time) []map[string]interface{} {
	if len(invocations) == 0 {
		return invocations
	}
	if now.IsZero() {
		now = time.Now()
	}
	out := make([]map[string]interface{}, 0, len(invocations))
	nowUnix := now.Unix()
	for _, invocation := range invocations {
		expiresAt := intValueFromAny(invocation["expires_at"])
		if expiresAt > 0 && int64(expiresAt) < nowUnix {
			continue
		}
		out = append(out, invocation)
	}
	return out
}

// DebugModelInvocationTrace returns one unexpired raw debug model invocation by runtime ID.
func DebugModelInvocationTrace(metadata map[string]interface{}, runtimeID string, now time.Time) (map[string]interface{}, bool) {
	runtimeID = strings.TrimSpace(runtimeID)
	if len(metadata) == 0 || runtimeID == "" {
		return nil, false
	}
	for _, invocation := range pruneDebugModelInvocations(modelInvocationsFromMetadata(metadata[debugModelInvocationsMetadataKey]), now) {
		if strings.TrimSpace(stringFromAny(invocation["runtime_id"])) != runtimeID {
			continue
		}
		return copyStringAnyMap(invocation), true
	}
	return nil, false
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
		"schema":      modelInvocationSchema,
		"trace_level": modelInvocationTraceLevelSummary,
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
		invocation["user_system_prompt"] = textSummaryPayload(userSystemPrompt)
	}
	return compactSkillInvocation(invocation)
}

func debugModelInvocationFromTrace(trace skillloop.ModelInvocationTrace, userSystemPrompt string) map[string]interface{} {
	if !modelInvocationRawDebugEnabled() {
		return nil
	}
	startedAt := trace.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	invocation := map[string]interface{}{
		"schema":      modelInvocationDebugSchema,
		"trace_level": modelInvocationTraceLevelRawDebug,
		"kind":        "model_call",
		"phase":       firstNonEmptyString(trace.Phase, "model_call"),
		"round":       trace.Round,
		"streaming":   trace.Streaming,
		"created_at":  startedAt.Unix(),
		"expires_at":  startedAt.Add(modelInvocationDebugRetention).Unix(),
		"duration_ms": trace.DurationMS,
		"runtime_id":  fmt.Sprintf("model_call:%s:%d:%d", firstNonEmptyString(trace.Phase, "model_call"), trace.Round, startedAt.UnixNano()),
		"request":     modelInvocationRawRequestPayload(trace.Request),
		"response":    modelInvocationRawResponsePayload(trace.Response, trace.Usage),
		"usage":       usageMetadata(trace.Usage),
		"error":       strings.TrimSpace(trace.Error),
	}
	if trace.Request != nil {
		invocation["model"] = trace.Request.Model
		invocation["provider"] = trace.Request.Provider
	}
	if strings.TrimSpace(userSystemPrompt) != "" {
		invocation["user_system_prompt"] = strings.TrimSpace(userSystemPrompt)
	}
	return compactSkillInvocation(invocation)
}

func modelInvocationRawDebugEnabled() bool {
	return ModelInvocationRawDebugEnabled()
}

// ModelInvocationRawDebugEnabled reports whether short-lived raw model traces are enabled.
func ModelInvocationRawDebugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(modelInvocationRawDebugEnv))) {
	case "1", "true", "yes", "on", "debug", "raw":
		return true
	default:
		return false
	}
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
	return modelInvocationRequestSummaryPayload(modelInvocationRawRequestPayload(req), req.Provider)
}

func modelInvocationRawRequestPayload(req *adapter.ChatRequest) map[string]interface{} {
	if req == nil {
		return map[string]interface{}{}
	}
	request := jsonObjectPayload(req)
	if strings.TrimSpace(req.Provider) != "" {
		request["provider"] = req.Provider
	}
	if len(req.AdditionalParameters) > 0 {
		request["additional_parameters"] = copyStringAnyMap(req.AdditionalParameters)
	}
	return request
}

func modelInvocationRequestSummaryPayload(request map[string]interface{}, provider string) map[string]interface{} {
	payload := map[string]interface{}{
		"schema":      modelInvocationRequestSummarySchema,
		"trace_level": modelInvocationTraceLevelSummary,
	}
	if model := strings.TrimSpace(stringFromAny(request["model"])); model != "" {
		payload["model"] = model
	}
	if provider = strings.TrimSpace(provider); provider != "" {
		payload["provider"] = provider
	} else if provider = strings.TrimSpace(stringFromAny(request["provider"])); provider != "" {
		payload["provider"] = provider
	}
	copySafeModelRequestParameter(payload, request, "temperature")
	copySafeModelRequestParameter(payload, request, "top_p")
	copySafeModelRequestParameter(payload, request, "max_tokens")
	copySafeModelRequestParameter(payload, request, "presence_penalty")
	copySafeModelRequestParameter(payload, request, "frequency_penalty")
	copySafeModelRequestParameter(payload, request, "seed")
	copySafeModelRequestParameter(payload, request, "n")
	copySafeModelRequestParameter(payload, request, "stream")
	if stopCount := len(interfaceSliceFromAny(request["stop"])); stopCount > 0 {
		payload["stop_count"] = stopCount
	}
	if toolsCount := len(interfaceSliceFromAny(request["tools"])); toolsCount > 0 {
		payload["tool_count"] = toolsCount
		payload["has_tools"] = true
	}
	if functionsCount := len(interfaceSliceFromAny(request["functions"])); functionsCount > 0 {
		payload["function_count"] = functionsCount
	}
	if responseFormat := mapFromAny(request["response_format"]); len(responseFormat) > 0 {
		if formatType := strings.TrimSpace(stringFromAny(responseFormat["type"])); formatType != "" {
			payload["response_format_type"] = formatType
		}
		if len(mapFromAny(responseFormat["schema"])) > 0 {
			payload["has_response_schema"] = true
		}
	}
	if additional := mapFromAny(request["additional_parameters"]); len(additional) > 0 {
		payload["additional_parameter_keys"] = sortedMapKeys(additional)
	}
	mergeModelMessagesSummary(payload, request["messages"])
	payload["prompt_hash"] = hashAnyPayload(request)
	return compactSkillInvocation(payload)
}

func modelInvocationResponsePayload(message *adapter.Message, usage *adapter.Usage) map[string]interface{} {
	return modelInvocationResponseSummaryPayload(modelInvocationRawResponsePayload(message, usage))
}

func modelInvocationRawResponsePayload(message *adapter.Message, usage *adapter.Usage) map[string]interface{} {
	payload := map[string]interface{}{}
	if message != nil {
		payload["message"] = jsonObjectPayload(message)
	}
	if usageMap := usageMetadata(usage); len(usageMap) > 0 {
		payload["usage"] = usageMap
	}
	return payload
}

func modelInvocationResponseSummaryPayload(response map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"schema":      modelInvocationResponseSummarySchema,
		"trace_level": modelInvocationTraceLevelSummary,
	}
	if message := mapFromAny(response["message"]); len(message) > 0 {
		payload["message"] = summarizeModelMessageMap(message, 0)
	}
	if usageMap := mapFromAny(response["usage"]); len(usageMap) > 0 {
		payload["usage"] = usageMap
	}
	payload["response_hash"] = hashAnyPayload(response)
	return compactSkillInvocation(payload)
}

// PublicMessageMetadata returns metadata safe for webapp and console API responses.
func PublicMessageMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	out := copyStringAnyMap(metadata)
	delete(out, debugModelInvocationsMetadataKey)
	delete(out, "debug_model_invocation_count")
	if invocations := publicModelInvocations(out["model_invocations"]); len(invocations) > 0 {
		out["model_invocations"] = invocations
		out["model_invocation_count"] = len(invocations)
	}
	return out
}

// PublicModelInvocationEvent returns a single model invocation safe for webapp logs.
func PublicModelInvocationEvent(event map[string]interface{}) map[string]interface{} {
	if len(event) == 0 {
		return map[string]interface{}{}
	}
	out := copyStringAnyMap(event)
	out["schema"] = modelInvocationSchema
	out["trace_level"] = modelInvocationTraceLevelSummary
	if request := mapFromAny(event["request"]); len(request) > 0 {
		out["request"] = PublicModelInvocationRequest(request, stringFromAny(event["user_system_prompt"]))
	}
	if response := mapFromAny(event["response"]); len(response) > 0 {
		out["response"] = PublicModelInvocationResponse(response)
	}
	if prompt := strings.TrimSpace(stringFromAny(event["user_system_prompt"])); prompt != "" {
		out["user_system_prompt"] = textSummaryPayload(prompt)
	}
	delete(out, debugModelInvocationsMetadataKey)
	return compactSkillInvocation(out)
}

// PublicModelInvocationRequest summarizes a model request without exposing prompt text.
func PublicModelInvocationRequest(request map[string]interface{}, userSystemPrompt string) map[string]interface{} {
	if isModelInvocationSummaryPayload(request, modelInvocationRequestSummarySchema) {
		payload := copyStringAnyMap(request)
		if strings.TrimSpace(userSystemPrompt) != "" {
			payload["user_system_prompt"] = textSummaryPayload(userSystemPrompt)
		}
		return compactSkillInvocation(payload)
	}
	payload := modelInvocationRequestSummaryPayload(request, "")
	if strings.TrimSpace(userSystemPrompt) != "" {
		payload["user_system_prompt"] = textSummaryPayload(userSystemPrompt)
	}
	return compactSkillInvocation(payload)
}

// PublicModelInvocationResponse summarizes a model response without exposing response text.
func PublicModelInvocationResponse(response map[string]interface{}) map[string]interface{} {
	if isModelInvocationSummaryPayload(response, modelInvocationResponseSummarySchema) {
		return compactSkillInvocation(copyStringAnyMap(response))
	}
	return modelInvocationResponseSummaryPayload(response)
}

func publicModelInvocations(value interface{}) []interface{} {
	invocations := modelInvocationsFromMetadata(value)
	if len(invocations) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(invocations))
	for _, invocation := range invocations {
		out = append(out, PublicModelInvocationEvent(invocation))
	}
	return out
}

func isModelInvocationSummaryPayload(payload map[string]interface{}, schema string) bool {
	return strings.TrimSpace(stringFromAny(payload["schema"])) == schema &&
		strings.TrimSpace(stringFromAny(payload["trace_level"])) == modelInvocationTraceLevelSummary
}

func copySafeModelRequestParameter(out map[string]interface{}, source map[string]interface{}, key string) {
	if value, ok := safeScalarValue(source[key]); ok {
		out[key] = value
	}
}

func safeScalarValue(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return typed, true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" || len([]rune(text)) > 120 {
			return nil, false
		}
		return text, true
	default:
		return nil, false
	}
}

func mergeModelMessagesSummary(payload map[string]interface{}, value interface{}) {
	messages := interfaceSliceFromAny(value)
	if len(messages) == 0 {
		return
	}
	roleCounts := map[string]interface{}{}
	roles := make([]interface{}, 0, len(messages))
	summaries := make([]interface{}, 0, len(messages))
	totalContentChars := 0
	runtimeContextChars := 0
	toolCallCount := 0
	toolMessageCount := 0
	imagePartCount := 0
	textPartCount := 0
	toolNamesSeen := map[string]struct{}{}
	toolNames := make([]interface{}, 0)
	hasRuntimeContext := false

	for index, item := range messages {
		message := mapFromAny(item)
		if len(message) == 0 {
			continue
		}
		summary := summarizeModelMessageMap(message, index)
		summaries = append(summaries, summary)
		role := strings.TrimSpace(stringFromAny(summary["role"]))
		if role != "" {
			roles = append(roles, role)
			roleCounts[role] = intValueFromAny(roleCounts[role]) + 1
		}
		totalContentChars += intValueFromAny(summary["content_chars"])
		runtimeContextChars += intValueFromAny(summary["runtime_context_char_count"])
		toolCallCount += intValueFromAny(summary["tool_call_count"])
		imagePartCount += intValueFromAny(summary["image_part_count"])
		textPartCount += intValueFromAny(summary["text_part_count"])
		if role == "tool" {
			toolMessageCount++
		}
		if value, ok := summary["has_runtime_context"].(bool); ok && value {
			hasRuntimeContext = true
		}
		for _, rawName := range interfaceSliceFromAny(summary["tool_call_names"]) {
			name := strings.TrimSpace(stringFromAny(rawName))
			if name == "" {
				continue
			}
			if _, exists := toolNamesSeen[name]; exists {
				continue
			}
			toolNamesSeen[name] = struct{}{}
			toolNames = append(toolNames, name)
		}
	}

	payload["message_count"] = len(messages)
	payload["message_roles"] = roles
	payload["message_role_counts"] = roleCounts
	payload["messages"] = summaries
	payload["total_content_chars"] = totalContentChars
	if toolCallCount > 0 {
		payload["tool_call_count"] = toolCallCount
		payload["tool_call_names"] = toolNames
	}
	if toolMessageCount > 0 {
		payload["tool_message_count"] = toolMessageCount
	}
	if imagePartCount > 0 {
		payload["image_part_count"] = imagePartCount
	}
	if textPartCount > 0 {
		payload["text_part_count"] = textPartCount
	}
	if hasRuntimeContext {
		payload["has_runtime_context"] = true
		payload["runtime_context_char_count"] = runtimeContextChars
	}
}

func summarizeModelMessageMap(message map[string]interface{}, index int) map[string]interface{} {
	summary := map[string]interface{}{
		"index": index,
	}
	if role := strings.TrimSpace(stringFromAny(message["role"])); role != "" {
		summary["role"] = role
	}
	mergeContentSummary(summary, message["content"])
	if toolCalls := interfaceSliceFromAny(message["tool_calls"]); len(toolCalls) > 0 {
		toolNames := make([]interface{}, 0, len(toolCalls))
		argumentsChars := 0
		for _, item := range toolCalls {
			toolCall := mapFromAny(item)
			function := mapFromAny(toolCall["function"])
			if name := strings.TrimSpace(stringFromAny(function["name"])); name != "" {
				toolNames = append(toolNames, name)
			}
			argumentsChars += len([]rune(stringFromAny(function["arguments"])))
		}
		summary["tool_call_count"] = len(toolCalls)
		if len(toolNames) > 0 {
			summary["tool_call_names"] = toolNames
		}
		if argumentsChars > 0 {
			summary["tool_call_arguments_chars"] = argumentsChars
		}
	}
	if functionCall := mapFromAny(message["function_call"]); len(functionCall) > 0 {
		summary["function_call"] = compactSkillInvocation(map[string]interface{}{
			"name":            strings.TrimSpace(stringFromAny(functionCall["name"])),
			"arguments_chars": len([]rune(stringFromAny(functionCall["arguments"]))),
		})
	}
	return compactSkillInvocation(summary)
}

func mergeContentSummary(summary map[string]interface{}, content interface{}) {
	switch typed := content.(type) {
	case string:
		contentChars := len([]rune(typed))
		summary["content_type"] = "text"
		summary["content_chars"] = contentChars
		if hasRuntimeContextMarker(typed) {
			summary["has_runtime_context"] = true
			summary["runtime_context_char_count"] = contentChars
		}
	case []interface{}:
		textChars := 0
		textPartCount := 0
		imagePartCount := 0
		hasRuntimeContext := false
		runtimeChars := 0
		for _, item := range typed {
			part := mapFromAny(item)
			switch strings.TrimSpace(stringFromAny(part["type"])) {
			case "text", "input_text":
				text := stringFromAny(part["text"])
				textPartCount++
				textChars += len([]rune(text))
				if hasRuntimeContextMarker(text) {
					hasRuntimeContext = true
					runtimeChars += len([]rune(text))
				}
			case "image_url", "input_image":
				imagePartCount++
			}
		}
		summary["content_type"] = "parts"
		summary["content_chars"] = textChars
		if textPartCount > 0 {
			summary["text_part_count"] = textPartCount
		}
		if imagePartCount > 0 {
			summary["image_part_count"] = imagePartCount
		}
		if hasRuntimeContext {
			summary["has_runtime_context"] = true
			summary["runtime_context_char_count"] = runtimeChars
		}
	case nil:
		return
	default:
		summary["content_type"] = "structured"
		summary["content_chars"] = len([]rune(fmt.Sprintf("%v", typed)))
	}
}

func hasRuntimeContextMarker(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "transient zgi page context") ||
		strings.Contains(text, "current zgi page context") ||
		strings.Contains(text, "zgi page context")
}

func textSummaryPayload(text string) map[string]interface{} {
	text = strings.TrimSpace(text)
	if text == "" {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"trace_level": modelInvocationTraceLevelSummary,
		"char_count":  len([]rune(text)),
		"sha256":      hashString(text),
	}
}

func hashAnyPayload(value interface{}) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return hashString(fmt.Sprintf("%v", value))
	}
	return hashBytes(data)
}

func hashString(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return hashBytes([]byte(value))
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mapFromAny(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return copyStringAnyMap(typed)
	default:
		return map[string]interface{}{}
	}
}

func interfaceSliceFromAny(value interface{}) []interface{} {
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

func sortedMapKeys(values map[string]interface{}) []interface{} {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]interface{}, 0, len(keys))
	for _, key := range keys {
		out = append(out, key)
	}
	return out
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
