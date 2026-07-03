package workflow

import (
	"encoding/json"
	"strings"
)

const (
	agentRuntimeHiddenInstructionsPlaceholder = "__ZGI_HIDDEN_SKILL_INSTRUCTIONS__"
)

func agentRuntimeEventInput(event map[string]interface{}) interface{} {
	switch agentRuntimeEventType(event) {
	case "model_call":
		return sanitizeAgentRuntimeModelRequest(runtimeMap(event["request"]), runtimeString(event["user_system_prompt"]))
	case "tool_call":
		return sanitizeAgentRuntimeToolArguments(runtimeMap(event["arguments"]))
	case "skill_load":
		return map[string]interface{}{"skill_id": runtimeString(event["skill_id"])}
	case "reference_read":
		return map[string]interface{}{
			"skill_id": runtimeString(event["skill_id"]),
			"path":     runtimeString(event["path"]),
		}
	case "intermediate_answer":
		return map[string]interface{}{"answer_id": runtimeString(event["answer_id"])}
	case "workflow_run":
		return sanitizeAgentRuntimeSensitiveValue(runtimeMap(event["inputs"]))
	case "workflow_node":
		return sanitizeAgentRuntimeSensitiveValue(runtimeMap(event["inputs"]))
	case "workflow_approval":
		return sanitizeAgentRuntimeSensitiveValue(runtimeMap(event["approval_form"]))
	case "workflow_approval_submitted":
		return sanitizeAgentRuntimeSensitiveValue(compactAgentRuntimeMap(map[string]interface{}{
			"approval_form_id": runtimeString(event["approval_form_id"]),
			"action":           runtimeString(event["action"]),
			"action_label":     runtimeString(event["action_label"]),
			"inputs":           event["inputs"],
		}))
	case "workflow_approval_expired":
		return compactAgentRuntimeMap(map[string]interface{}{
			"approval_form_id": runtimeString(event["approval_form_id"]),
			"reason":           runtimeString(event["reason"]),
		})
	case "workflow_question":
		return sanitizeAgentRuntimeSensitiveValue(compactAgentRuntimeMap(map[string]interface{}{
			"question": runtimeString(event["question"]),
			"choices":  event["choices"],
			"round":    event["round"],
		}))
	case "workflow_message":
		return compactAgentRuntimeMap(map[string]interface{}{
			"event": runtimeString(event["event"]),
		})
	default:
		if arguments := runtimeMap(event["arguments"]); len(arguments) > 0 {
			return sanitizeAgentRuntimeToolArguments(arguments)
		}
		return compactAgentRuntimeMap(map[string]interface{}{
			"skill_id":  runtimeString(event["skill_id"]),
			"tool_name": runtimeString(event["tool_name"]),
			"path":      runtimeString(event["path"]),
			"answer_id": runtimeString(event["answer_id"]),
		})
	}
}

func agentRuntimeEventOutput(event map[string]interface{}) interface{} {
	if agentRuntimeEventType(event) == "model_call" {
		output := runtimeMap(event["response"])
		if len(output) == 0 {
			output["status"] = runtimeString(event["status"])
		}
		return output
	}
	if agentRuntimeEventType(event) == "workflow_run" || agentRuntimeEventType(event) == "workflow_node" {
		output := runtimeMap(event["outputs"])
		if len(output) == 0 {
			output["status"] = runtimeString(event["status"])
		}
		return sanitizeAgentRuntimeResultValue(output)
	}
	if agentRuntimeEventType(event) == "workflow_approval" {
		return sanitizeAgentRuntimeResultValue(compactAgentRuntimeMap(map[string]interface{}{
			"approval_form_id": runtimeString(event["approval_form_id"]),
			"approval_url":     runtimeString(event["approval_url"]),
			"status":           runtimeString(event["status"]),
		}))
	}
	if agentRuntimeEventType(event) == "workflow_approval_submitted" || agentRuntimeEventType(event) == "workflow_approval_expired" {
		return sanitizeAgentRuntimeResultValue(compactAgentRuntimeMap(map[string]interface{}{
			"status": runtimeString(event["status"]),
			"reason": runtimeString(event["reason"]),
		}))
	}
	if agentRuntimeEventType(event) == "workflow_question" {
		return sanitizeAgentRuntimeResultValue(compactAgentRuntimeMap(map[string]interface{}{
			"answer":       runtimeString(event["answer"]),
			"choice_id":    runtimeString(event["choice_id"]),
			"choice_label": runtimeString(event["choice_label"]),
			"choice_value": runtimeString(event["choice_value"]),
			"status":       runtimeString(event["status"]),
		}))
	}
	if agentRuntimeEventType(event) == "workflow_message" {
		return sanitizeAgentRuntimeResultValue(compactAgentRuntimeMap(map[string]interface{}{
			"answer":   runtimeString(event["answer"]),
			"data":     event["data"],
			"metadata": event["metadata"],
			"status":   runtimeString(event["status"]),
		}))
	}
	output := map[string]interface{}{}
	if result := runtimeMap(event["result"]); len(result) > 0 {
		output["result"] = sanitizeAgentRuntimeResultValue(result)
	}
	if text := runtimeString(event["message"]); text != "" {
		output["message"] = text
	}
	if path := runtimeString(event["path"]); path != "" && agentRuntimeEventType(event) == "reference_read" {
		output["path"] = path
	}
	if len(output) == 0 {
		output["status"] = runtimeString(event["status"])
	}
	return output
}

func agentRuntimeEventProcess(event map[string]interface{}) map[string]interface{} {
	if agentRuntimeEventType(event) == "workflow_run" {
		return compactAgentRuntimeMap(map[string]interface{}{
			"event_type":       "workflow_run",
			"kind":             runtimeString(event["kind"]),
			"runtime_id":       runtimeString(event["runtime_id"]),
			"workflow_run_id":  runtimeString(event["workflow_run_id"]),
			"workflow_id":      runtimeString(event["workflow_id"]),
			"agent_id":         runtimeString(event["agent_id"]),
			"binding_id":       runtimeString(event["binding_id"]),
			"version":          event["version"],
			"node_count":       len(runtimeSkillInvocations(event["nodes"])),
			"approval_count":   len(runtimeSkillInvocations(event["approvals"])),
			"question_count":   len(runtimeSkillInvocations(event["question_answers"])),
			"message_count":    len(runtimeSkillInvocations(runtimeMap(event["messages"])["chunks"])),
			"invocation":       sanitizeAgentRuntimeSensitiveValue(event["invocation"]),
			"nodes":            sanitizeAgentRuntimeResultValue(event["nodes"]),
			"approvals":        sanitizeAgentRuntimeResultValue(event["approvals"]),
			"question_answers": sanitizeAgentRuntimeResultValue(event["question_answers"]),
			"messages":         sanitizeAgentRuntimeResultValue(event["messages"]),
			"raw_event":        sanitizeAgentRuntimeRawEvent(event),
		})
	}
	return compactAgentRuntimeMap(map[string]interface{}{
		"event_type":        agentRuntimeEventType(event),
		"kind":              runtimeString(event["kind"]),
		"phase":             runtimeString(event["phase"]),
		"round":             event["round"],
		"streaming":         event["streaming"],
		"model":             runtimeString(event["model"]),
		"provider":          runtimeString(event["provider"]),
		"usage":             event["usage"],
		"prompt_tokens":     event["prompt_tokens"],
		"completion_tokens": event["completion_tokens"],
		"total_tokens":      event["total_tokens"],
		"runtime_id":        runtimeString(event["runtime_id"]),
		"skill_id":          runtimeString(event["skill_id"]),
		"tool_name":         runtimeString(event["tool_name"]),
		"path":              runtimeString(event["path"]),
		"answer_id":         runtimeString(event["answer_id"]),
		"workflow_run_id":   runtimeString(event["workflow_run_id"]),
		"workflow_id":       runtimeString(event["workflow_id"]),
		"binding_id":        runtimeString(event["binding_id"]),
		"node_id":           runtimeString(event["node_id"]),
		"node_type":         runtimeString(event["node_type"]),
		"approval_form_id":  runtimeString(event["approval_form_id"]),
		"approval_url":      runtimeString(event["approval_url"]),
		"version":           event["version"],
		"raw_event":         sanitizeAgentRuntimeRawEvent(event),
	})
}

func sanitizeAgentRuntimeRawEvent(event map[string]interface{}) map[string]interface{} {
	raw := copyRuntimeMap(event)
	if agentRuntimeEventType(event) == "model_call" {
		raw["request"] = sanitizeAgentRuntimeModelRequest(runtimeMap(event["request"]), runtimeString(event["user_system_prompt"]))
	}
	if arguments := runtimeMap(event["arguments"]); len(arguments) > 0 {
		raw["arguments"] = sanitizeAgentRuntimeToolArguments(arguments)
	}
	if result := runtimeMap(event["result"]); len(result) > 0 {
		raw["result"] = sanitizeAgentRuntimeResultValue(result)
	}
	if agentRuntimeEventType(event) == "workflow_run" {
		raw["inputs"] = sanitizeAgentRuntimeSensitiveValue(raw["inputs"])
		raw["outputs"] = sanitizeAgentRuntimeResultValue(raw["outputs"])
		raw["nodes"] = sanitizeAgentRuntimeResultValue(raw["nodes"])
		raw["approvals"] = sanitizeAgentRuntimeResultValue(raw["approvals"])
		raw["question_answers"] = sanitizeAgentRuntimeResultValue(raw["question_answers"])
		raw["messages"] = sanitizeAgentRuntimeResultValue(raw["messages"])
		raw["invocation"] = sanitizeAgentRuntimeSensitiveValue(raw["invocation"])
	}
	return raw
}

func sanitizeAgentRuntimeModelRequest(request map[string]interface{}, userSystemPrompt string) map[string]interface{} {
	if len(request) == 0 {
		return request
	}
	sanitized := copyRuntimeMap(request)
	if messages, ok := sanitizeAgentRuntimeMessages(request["messages"], userSystemPrompt); ok {
		sanitized["messages"] = messages
	}
	return sanitized
}

func sanitizeAgentRuntimeMessages(value interface{}, userSystemPrompt string) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	if !ok {
		return nil, false
	}
	messages := make([]interface{}, 0, len(items))
	keptUserSystemPrompt := false
	for _, item := range items {
		message, ok := item.(map[string]interface{})
		if !ok {
			messages = append(messages, item)
			continue
		}
		if strings.EqualFold(runtimeString(message["role"]), "system") {
			if !keptUserSystemPrompt && strings.TrimSpace(userSystemPrompt) != "" {
				visible := copyRuntimeMap(message)
				visible["content"] = strings.TrimSpace(userSystemPrompt)
				messages = append(messages, visible)
				keptUserSystemPrompt = true
			}
			continue
		}
		messages = append(messages, sanitizeAgentRuntimeModelMessage(message))
	}
	return messages, true
}

func sanitizeAgentRuntimeModelMessage(message map[string]interface{}) map[string]interface{} {
	out := copyRuntimeMap(message)
	if toolCalls, ok := sanitizeAgentRuntimeToolCalls(out["tool_calls"]); ok {
		out["tool_calls"] = toolCalls
	}
	if strings.EqualFold(runtimeString(out["role"]), "tool") {
		out["content"] = sanitizeAgentRuntimeResultValue(out["content"])
	}
	return out
}

func sanitizeAgentRuntimeToolCalls(value interface{}) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		toolCall, ok := item.(map[string]interface{})
		if !ok {
			out = append(out, item)
			continue
		}
		sanitized := copyRuntimeMap(toolCall)
		if fn, ok := sanitized["function"].(map[string]interface{}); ok {
			sanitized["function"] = sanitizeAgentRuntimeToolCallFunction(fn)
		}
		if args, ok := sanitized["arguments"].(map[string]interface{}); ok {
			sanitized["arguments"] = sanitizeAgentRuntimeToolArguments(args)
		}
		out = append(out, sanitized)
	}
	return out, true
}

func sanitizeAgentRuntimeToolCallFunction(fn map[string]interface{}) map[string]interface{} {
	out := copyRuntimeMap(fn)
	switch raw := out["arguments"].(type) {
	case string:
		out["arguments"] = sanitizeAgentRuntimeToolArgumentsString(raw)
	case map[string]interface{}:
		out["arguments"] = sanitizeAgentRuntimeToolArguments(raw)
	}
	return out
}

func sanitizeAgentRuntimeToolArguments(args map[string]interface{}) map[string]interface{} {
	sanitized, ok := sanitizeAgentRuntimeSensitiveValue(args).(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return sanitized
}

func sanitizeAgentRuntimeToolArgumentsString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return raw
	}
	sanitized := sanitizeAgentRuntimeSensitiveValue(payload)
	data, err := json.Marshal(sanitized)
	if err != nil {
		return raw
	}
	return string(data)
}

func sanitizeAgentRuntimeSensitiveValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if isAgentRuntimeSensitiveKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = sanitizeAgentRuntimeSensitiveValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeAgentRuntimeSensitiveValue(item))
		}
		return out
	default:
		return value
	}
}

func sanitizeAgentRuntimeResultValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if strings.EqualFold(strings.TrimSpace(key), "instructions") {
				out[key] = agentRuntimeHiddenInstructionsPlaceholder
				continue
			}
			if isAgentRuntimeSensitiveKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = sanitizeAgentRuntimeResultValue(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeAgentRuntimeResultValue(item))
		}
		return out
	case string:
		sanitized, ok := sanitizeAgentRuntimeResultJSON(typed)
		if !ok {
			return typed
		}
		return sanitized
	default:
		return value
	}
}

func sanitizeAgentRuntimeResultJSON(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw, false
	}
	var payload interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return raw, false
	}
	sanitized := sanitizeAgentRuntimeResultValue(payload)
	data, err := json.Marshal(sanitized)
	if err != nil {
		return raw, false
	}
	return string(data), true
}

func isAgentRuntimeSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "password", "passwd", "pwd", "secret", "token", "api_key", "apikey", "access_key",
		"secret_key", "private_key", "access_token", "refresh_token", "authorization",
		"auth_token", "bearer", "cookie", "credential", "credentials", "client_secret",
		"x_api_key":
		return true
	}
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "access_token") ||
		strings.Contains(normalized, "refresh_token") ||
		strings.Contains(normalized, "api_key")
}
