package agentmemoryruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func ApplyDecision(ctx context.Context, req MutationRequest, decision Decision) (map[string]interface{}, skills.SkillTrace, error) {
	toolName, args, err := ValidateDecision(decision, req.Slots)
	if err != nil {
		return nil, DecisionTrace(decision, "rejected_validation", nil, err), err
	}
	started := time.Now()
	argumentSummary := SummarizeArguments(toolName, args)
	if req.OnToolCallStart != nil {
		req.OnToolCallStart(toolName, argumentSummary)
	}
	result, err := ExecuteTool(ctx, req, toolName, args)
	trace := DecisionTrace(decision, "success", result, nil)
	trace.Kind = "tool_call"
	trace.ToolName = toolName
	trace.Arguments = argumentSummary
	trace.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		trace.Status = "mutation_error"
		trace.Error = err.Error()
		return nil, trace, err
	}
	if req.OnToolCallEnd != nil {
		req.OnToolCallEnd(trace)
	}
	return result, trace, nil
}

func ExecuteTool(ctx context.Context, req MutationRequest, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	if req.MemoryService == nil || req.AgentID == zeroUUID || req.WorkspaceID == zeroUUID {
		return nil, fmt.Errorf("%w: agent memory runtime scope is invalid", ErrInvalidInput)
	}
	key := stringArg(args, "key")
	if key == "" {
		return nil, fmt.Errorf("%w: memory key is required", ErrInvalidInput)
	}
	slots := RuntimeSlots(req.Slots)
	switch toolName {
	case ToolUpdate:
		content := strings.TrimSpace(stringArg(args, "content"))
		if content == "" {
			return nil, fmt.Errorf("%w: memory content is required", ErrInvalidInput)
		}
		value, err := req.MemoryService.UpdateValue(ctx, req.WorkspaceID, req.AgentID, slots, req.UserScope, req.UserID, agentmemory.UpdateValueRequest{
			Key:     key,
			Content: content,
		}, req.MutationMetadata)
		if err != nil {
			return nil, err
		}
		return Result("updated", value), nil
	case ToolClear:
		value, err := req.MemoryService.ClearValue(ctx, req.WorkspaceID, req.AgentID, slots, req.UserScope, req.UserID, key, req.MutationMetadata)
		if err != nil {
			return nil, err
		}
		return Result("cleared", value), nil
	default:
		return nil, fmt.Errorf("%w: unsupported agent memory tool %s", ErrInvalidInput, toolName)
	}
}

func Result(action string, value *agentmemory.SlotValueResponse) map[string]interface{} {
	result := map[string]interface{}{"status": "success", "action": action}
	if value == nil {
		return result
	}
	result["key"] = value.Key
	result["content"] = value.Content
	result["max_chars"] = value.MaxChars
	result["updated_at"] = value.UpdatedAt
	return result
}

func StringResultValue(result map[string]interface{}, key string) string {
	if len(result) == 0 {
		return ""
	}
	value, ok := result[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func SummarizeArguments(toolName string, args map[string]interface{}) map[string]interface{} {
	summary := map[string]interface{}{}
	if key := stringArg(args, "key"); key != "" {
		summary["key"] = key
	}
	if toolName == ToolUpdate {
		content := strings.TrimSpace(stringArg(args, "content"))
		if content != "" {
			summary["content_preview"] = TruncateRunes(content, 160)
			summary["content_chars"] = len([]rune(content))
		}
	}
	return summary
}

func ValidateDecision(decision Decision, slots []Slot) (string, map[string]interface{}, error) {
	action := strings.ToLower(strings.TrimSpace(decision.Action))
	if action == "" || action == "none" {
		return "", nil, fmt.Errorf("%w: memory decision has no mutation", ErrInvalidInput)
	}
	slot, ok := findEnabledSlot(slots, decision.Key)
	if !ok {
		return "", nil, fmt.Errorf("%w: memory key is not enabled", ErrInvalidInput)
	}
	args := map[string]interface{}{"key": slot.Key}
	switch action {
	case "update":
		content := strings.TrimSpace(decision.Content)
		if content == "" {
			return "", nil, fmt.Errorf("%w: memory content is required", ErrInvalidInput)
		}
		if slot.MaxChars > 0 && len([]rune(content)) > slot.MaxChars {
			return "", nil, fmt.Errorf("%w: memory content exceeds slot limit", ErrInvalidInput)
		}
		if ContainsSensitiveContent(content) {
			return "", nil, fmt.Errorf("%w: sensitive content cannot be saved to agent memory", ErrInvalidInput)
		}
		args["content"] = content
		return ToolUpdate, args, nil
	case "clear":
		return ToolClear, args, nil
	default:
		return "", nil, fmt.Errorf("%w: unsupported memory decision action", ErrInvalidInput)
	}
}

func findEnabledSlot(slots []Slot, key string) (Slot, bool) {
	key = strings.TrimSpace(key)
	for _, slot := range slots {
		if slot.Enabled && strings.TrimSpace(slot.Key) == key {
			return slot, true
		}
	}
	return Slot{}, false
}

func ContainsSensitiveContent(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return false
	}
	if containsLongDigitRun(normalized, 12) {
		return true
	}
	return containsAny(normalized, []string{
		"password", "passwd", "passcode", "credential", "credentials", "secret", "api key", "apikey", "access token", "refresh token", "private key", "credit card", "bank card", "card number", "ssn",
		"\u5bc6\u7801", "\u53e3\u4ee4", "\u51ed\u636e", "\u4ee4\u724c", "\u79d8\u94a5", "\u94f6\u884c\u5361", "\u4fe1\u7528\u5361", "\u8eab\u4efd\u8bc1", "\u8bc1\u4ef6\u53f7", "\u9a8c\u8bc1\u7801", "\u652f\u4ed8",
	})
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func containsLongDigitRun(value string, limit int) bool {
	if limit <= 0 {
		return false
	}
	run := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			run++
			if run >= limit {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

func stringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
