package service

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	agentLogContextMaxBytes        = 32 * 1024
	agentLogContextValueMaxBytes   = 8 * 1024
	agentLogContextMaxToolNames    = 64
	agentLogHiddenInstructions     = "__ZGI_HIDDEN_SKILL_INSTRUCTIONS__"
	agentLogRedactedSensitiveValue = "[REDACTED]"
)

func shouldPersistAgentLogContext(prepared *PreparedChat) bool {
	return prepared != nil && normalizeCallerType(prepared.Caller.Type) == runtimemodel.ConversationCallerAgent
}

func modelInvocationLogContext(req *adapter.ChatRequest, userSystemPrompt string) map[string]interface{} {
	if req == nil {
		return nil
	}
	context := map[string]interface{}{
		"provider":      strings.TrimSpace(req.Provider),
		"model":         strings.TrimSpace(req.Model),
		"stream":        req.Stream,
		"message_count": len(req.Messages),
		"tool_count":    len(req.Tools),
	}
	copyAgentLogRequestParameters(context, req)
	if names, omitted := agentLogToolNames(req); len(names) > 0 {
		context["tool_names"] = names
		if omitted > 0 {
			context["omitted_tool_name_count"] = omitted
		}
	}

	messages := make([]interface{}, 0, len(req.Messages)+1)
	truncated := false
	if prompt := strings.TrimSpace(userSystemPrompt); prompt != "" {
		value, valueTruncated := sanitizeAgentLogValue(prompt, "content")
		messages = append(messages, map[string]interface{}{"role": "system", "content": value})
		truncated = truncated || valueTruncated
	}
	for _, message := range req.Messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			continue
		}
		payload := agentLogMessagePayload(message)
		value, valueTruncated := sanitizeAgentLogValue(payload, "")
		value, messageTruncated := boundAgentLogMessage(value)
		messages = append(messages, value)
		truncated = truncated || valueTruncated || messageTruncated
	}
	originalVisibleCount := len(messages)
	omittedMessageCount := 0
	context["messages"] = messages
	for serializedAgentLogContextSize(context, truncated, originalVisibleCount, omittedMessageCount) > agentLogContextMaxBytes && canOmitAgentLogMessage(messages) {
		removeAt := 0
		if message, ok := messages[0].(map[string]interface{}); ok && strings.EqualFold(stringFromAny(message["role"]), "system") {
			removeAt = 1
		}
		messages = append(messages[:removeAt], messages[removeAt+1:]...)
		omittedMessageCount++
		truncated = true
		context["messages"] = messages
	}
	if serializedAgentLogContextSize(context, truncated, originalVisibleCount, omittedMessageCount) > agentLogContextMaxBytes {
		for _, key := range []string{"tool_choice", "response_format", "stop", "additional_parameter_keys", "tool_names"} {
			if _, ok := context[key]; !ok {
				continue
			}
			delete(context, key)
			context["parameters_truncated"] = true
			truncated = true
			if serializedAgentLogContextSize(context, truncated, originalVisibleCount, omittedMessageCount) <= agentLogContextMaxBytes {
				break
			}
		}
	}
	context["snapshot_meta"] = compactSkillInvocation(map[string]interface{}{
		"truncated":              truncated,
		"original_message_count": originalVisibleCount,
		"omitted_message_count":  omittedMessageCount,
		"max_bytes":              agentLogContextMaxBytes,
	})
	return compactSkillInvocation(context)
}

func agentLogMessagePayload(message adapter.Message) map[string]interface{} {
	payload := jsonObjectPayload(message)
	delete(payload, "reasoning_content")
	role := strings.ToLower(strings.TrimSpace(message.Role))
	switch role {
	case "tool", "function":
		if serializedAgentLogValueSize(message.Content) > agentLogContextValueMaxBytes {
			payload["content"] = modelInvocationToolContentSummary(message.Content)
			payload["content_truncated"] = true
		}
	case "user":
		if summary, ok := modelInvocationUserContentSummary(message.Content); ok {
			payload["content"] = summary
			payload["content_truncated"] = true
		}
	}
	return payload
}

func serializedAgentLogValueSize(value interface{}) int {
	if text, ok := value.(string); ok {
		return len(text)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return agentLogContextValueMaxBytes + 1
	}
	return len(data)
}

func copyAgentLogRequestParameters(target map[string]interface{}, req *adapter.ChatRequest) {
	if req.Temperature != nil {
		target["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		target["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		target["max_tokens"] = *req.MaxTokens
	}
	if req.PresencePenalty != nil {
		target["presence_penalty"] = *req.PresencePenalty
	}
	if req.FrequencyPenalty != nil {
		target["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.Seed != nil {
		target["seed"] = *req.Seed
	}
	if req.N != nil {
		target["n"] = *req.N
	}
	if req.ToolChoice != nil {
		value, _ := sanitizeAgentLogValue(req.ToolChoice, "tool_choice")
		target["tool_choice"] = value
	}
	if req.ResponseFormat != nil {
		target["response_format"] = compactSkillInvocation(map[string]interface{}{
			"type":       req.ResponseFormat.Type,
			"has_schema": len(req.ResponseFormat.Schema) > 0,
		})
	}
	if len(req.Stop) > 0 {
		stop := req.Stop
		if len(stop) > 16 {
			stop = stop[:16]
			target["omitted_stop_count"] = len(req.Stop) - len(stop)
		}
		value, _ := sanitizeAgentLogValue(stop, "stop")
		target["stop"] = value
	}
	if len(req.AdditionalParameters) > 0 {
		keys := make([]string, 0, len(req.AdditionalParameters))
		for key := range req.AdditionalParameters {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 32 {
			keys = keys[:32]
		}
		target["additional_parameter_keys"] = keys
	}
}

func agentLogToolNames(req *adapter.ChatRequest) ([]interface{}, int) {
	names := make([]interface{}, 0, len(req.Tools)+len(req.Functions))
	for _, tool := range req.Tools {
		if name := strings.TrimSpace(tool.Function.Name); name != "" {
			names = append(names, truncateAgentLogText(name))
		}
	}
	for _, function := range req.Functions {
		if name := strings.TrimSpace(function.Name); name != "" {
			names = append(names, truncateAgentLogText(name))
		}
	}
	if len(names) <= agentLogContextMaxToolNames {
		return names, 0
	}
	return names[:agentLogContextMaxToolNames], len(names) - agentLogContextMaxToolNames
}

func sanitizeAgentLogValue(value interface{}, key string) (interface{}, bool) {
	if isAgentLogSensitiveKey(key) {
		return agentLogRedactedSensitiveValue, false
	}
	if strings.EqualFold(strings.TrimSpace(key), "instructions") {
		return agentLogHiddenInstructions, false
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		truncated := false
		for itemKey, item := range typed {
			sanitized, itemTruncated := sanitizeAgentLogValue(item, itemKey)
			out[itemKey] = sanitized
			truncated = truncated || itemTruncated
		}
		sanitizeModelInvocationInlineImageDataURLs(out)
		return out, truncated
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		truncated := false
		for _, item := range typed {
			sanitized, itemTruncated := sanitizeAgentLogValue(item, key)
			out = append(out, sanitized)
			truncated = truncated || itemTruncated
		}
		return out, truncated
	case []string:
		out := make([]interface{}, 0, len(typed))
		truncated := false
		for _, item := range typed {
			sanitized, itemTruncated := sanitizeAgentLogValue(item, key)
			out = append(out, sanitized)
			truncated = truncated || itemTruncated
		}
		return out, truncated
	case string:
		return sanitizeAgentLogString(typed)
	default:
		payload := jsonObjectPayload(typed)
		if len(payload) == 1 && payload["value"] != nil {
			return typed, false
		}
		return sanitizeAgentLogValue(payload, key)
	}
}

func sanitizeAgentLogString(value string) (interface{}, bool) {
	trimmed := strings.TrimSpace(value)
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var decoded interface{}
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			sanitized, truncated := sanitizeAgentLogValue(decoded, "")
			encoded, err := json.Marshal(sanitized)
			if err == nil {
				text, textTruncated := truncateAgentLogTextWithStatus(string(encoded))
				return text, truncated || textTruncated
			}
		}
	}
	redacted, _, changed := redactEmbeddedModelInvocationImageDataURLs(value)
	if changed {
		value = redacted
	}
	text, truncated := truncateAgentLogTextWithStatus(value)
	return text, truncated
}

func truncateAgentLogText(value string) string {
	text, _ := truncateAgentLogTextWithStatus(value)
	return text
}

func truncateAgentLogTextWithStatus(value string) (string, bool) {
	if len(value) <= agentLogContextValueMaxBytes {
		return value, false
	}
	limit := agentLogContextValueMaxBytes - len("...")
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit] + "...", true
}

func boundAgentLogMessage(value interface{}) (interface{}, bool) {
	data, err := json.Marshal(value)
	if err == nil && len(data) <= agentLogContextValueMaxBytes {
		return value, false
	}
	message, ok := value.(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"content":        truncateAgentLogText(string(data)),
			"truncated":      true,
			"original_bytes": len(data),
		}, true
	}
	bounded := compactSkillInvocation(map[string]interface{}{
		"role":           message["role"],
		"name":           message["name"],
		"tool_call_id":   message["tool_call_id"],
		"truncated":      true,
		"original_bytes": len(data),
	})
	if content, ok := message["content"]; ok {
		contentData, _ := json.Marshal(content)
		contentText := string(contentData)
		if text, ok := content.(string); ok {
			contentText = text
		}
		bounded["content"] = truncateAgentLogTextToBytes(contentText, 6*1024)
	}
	if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
		names := make([]interface{}, 0, len(toolCalls))
		for _, item := range toolCalls {
			call := jsonObjectPayload(item)
			function := jsonObjectPayload(call["function"])
			if name := strings.TrimSpace(stringFromAny(function["name"])); name != "" {
				names = append(names, truncateAgentLogText(name))
			}
		}
		if len(names) > 0 {
			bounded["tool_call_names"] = names
		}
	}
	return bounded, true
}

func truncateAgentLogTextToBytes(value string, maxBytes int) string {
	if maxBytes <= 3 || len(value) <= maxBytes {
		return value
	}
	limit := maxBytes - len("...")
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit] + "..."
}

func canOmitAgentLogMessage(messages []interface{}) bool {
	if len(messages) <= 1 {
		return false
	}
	first, ok := messages[0].(map[string]interface{})
	return !ok || !strings.EqualFold(stringFromAny(first["role"]), "system") || len(messages) > 2
}

func serializedAgentLogContextSize(context map[string]interface{}, truncated bool, originalCount, omittedCount int) int {
	candidate := copyStringAnyMap(context)
	candidate["snapshot_meta"] = map[string]interface{}{
		"truncated":              truncated,
		"original_message_count": originalCount,
		"omitted_message_count":  omittedCount,
		"max_bytes":              agentLogContextMaxBytes,
	}
	data, err := json.Marshal(candidate)
	if err != nil {
		return agentLogContextMaxBytes + 1
	}
	return len(data)
}

func isAgentLogSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch normalized {
	case "password", "passwd", "pwd", "secret", "token", "api_key", "apikey", "access_key",
		"secret_key", "private_key", "access_token", "refresh_token", "authorization",
		"auth_token", "bearer", "cookie", "credential", "credentials", "client_secret",
		"x_api_key":
		return true
	default:
		return false
	}
}
