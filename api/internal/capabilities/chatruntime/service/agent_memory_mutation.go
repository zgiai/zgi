package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func (s *service) applyNativeAgentMemoryDecision(
	ctx context.Context,
	prepared *PreparedChat,
	slots []AgentMemorySlotConfig,
	decision nativeAgentMemoryDecision,
	onEvent func(StreamEvent) error,
) (map[string]interface{}, skills.SkillTrace, error) {
	toolName, args, err := validateNativeAgentMemoryDecision(decision, slots)
	if err != nil {
		return nil, nativeAgentMemoryDecisionTrace(decision, "rejected_validation", nil, err), err
	}
	started := time.Now()
	argumentSummary := summarizeNativeAgentMemoryArguments(toolName, args)
	result, err := s.executeNativeAgentMemoryTool(ctx, prepared, toolName, args)
	trace := nativeAgentMemoryDecisionTrace(decision, "success", result, nil)
	trace.Kind = "tool_call"
	trace.ToolName = toolName
	trace.Arguments = argumentSummary
	trace.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		trace.Status = "mutation_error"
		trace.Error = err.Error()
		return nil, trace, err
	}
	s.emitPreparedEvent(ctx, prepared, streamEventSkillCallStart, skillCallStartPayload(prepared, skills.SkillAgentMemory, toolName, argumentSummary), onEvent)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillCallEnd, skillCallEndPayload(prepared, trace), onEvent)
	return result, trace, nil
}

func (s *service) handleNativeAgentMemoryToolCall(
	ctx context.Context,
	prepared *PreparedChat,
	call adapter.ToolCall,
	onEvent func(StreamEvent) error,
) (skills.SkillTrace, adapter.Message, error) {
	started := time.Now()
	toolName := strings.TrimSpace(call.Function.Name)
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		trace := failedSkillTrace("tool_call", toolName, err)
		trace.SkillID = skills.SkillAgentMemory
		trace.DurationMS = time.Since(started).Milliseconds()
		s.emitSkillError(ctx, prepared, trace, onEvent)
		return trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "fix the JSON arguments and retry the same memory tool call")), err
	}
	argumentSummary := summarizeNativeAgentMemoryArguments(toolName, args)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillCallStart, skillCallStartPayload(prepared, skills.SkillAgentMemory, toolName, argumentSummary), onEvent)

	result, err := s.executeNativeAgentMemoryTool(ctx, prepared, toolName, args)
	trace := skills.SkillTrace{
		Kind:       "tool_call",
		SkillID:    skills.SkillAgentMemory,
		ToolName:   toolName,
		Status:     "success",
		DurationMS: time.Since(started).Milliseconds(),
		Arguments:  argumentSummary,
		Result:     result,
	}
	if err != nil {
		trace.Status = "error"
		trace.Error = err.Error()
		s.emitSkillError(ctx, prepared, trace, onEvent)
		return trace, skills.ToolResultMessage(call.ID, recoverableSkillToolErrorPayload(err, "fix the tool arguments based on the error and retry", skills.SkillAgentMemory, toolName)), err
	}
	s.emitPreparedEvent(ctx, prepared, streamEventSkillCallEnd, skillCallEndPayload(prepared, trace), onEvent)
	return trace, skills.ToolResultMessage(call.ID, result), nil
}

func (s *service) executeNativeAgentMemoryTool(ctx context.Context, prepared *PreparedChat, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	workspaceID := uuid.Nil
	if prepared.Scope.WorkspaceID != nil {
		workspaceID = *prepared.Scope.WorkspaceID
	}
	agentID, err := uuid.Parse(strings.TrimSpace(prepared.parts.AgentMemoryAgentID))
	if err != nil || agentID == uuid.Nil || workspaceID == uuid.Nil {
		return nil, fmt.Errorf("%w: agent memory runtime scope is invalid", ErrInvalidInput)
	}
	key := stringArg(args, "key")
	if key == "" {
		return nil, fmt.Errorf("%w: memory key is required", ErrInvalidInput)
	}
	meta := agentMemoryMutationMetadata(prepared)
	slots := agentMemoryRuntimeSlots(prepared.parts.AgentMemorySlots)
	switch toolName {
	case agentMemoryToolUpdate:
		content := strings.TrimSpace(stringArg(args, "content"))
		if content == "" {
			return nil, fmt.Errorf("%w: memory content is required", ErrInvalidInput)
		}
		value, err := s.agentMemoryService.UpdateValue(ctx, workspaceID, agentID, slots, prepared.parts.AgentMemoryUserScope, prepared.Scope.AccountID, agentmemory.UpdateValueRequest{
			Key:     key,
			Content: content,
		}, meta)
		if err != nil {
			return nil, err
		}
		return nativeAgentMemoryResult("updated", value), nil
	case agentMemoryToolClear:
		value, err := s.agentMemoryService.ClearValue(ctx, workspaceID, agentID, slots, prepared.parts.AgentMemoryUserScope, prepared.Scope.AccountID, key, meta)
		if err != nil {
			return nil, err
		}
		return nativeAgentMemoryResult("cleared", value), nil
	default:
		return nil, fmt.Errorf("%w: unsupported agent memory tool %s", ErrInvalidInput, toolName)
	}
}

func agentMemoryMutationMetadata(prepared *PreparedChat) agentmemory.MutationMetadata {
	meta := agentmemory.MutationMetadata{
		ActorType: agentmemory.EventActorModel,
		Source:    agentmemory.EventSourceAgent,
	}
	if prepared != nil && prepared.Conversation != nil {
		id := prepared.Conversation.ID
		meta.SourceConversationID = &id
	}
	if prepared != nil && prepared.Message != nil {
		id := prepared.Message.ID
		meta.SourceMessageID = &id
	}
	return meta
}

func nativeAgentMemoryResult(action string, value *agentmemory.SlotValueResponse) map[string]interface{} {
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

func stringResultValue(result map[string]interface{}, key string) string {
	if len(result) == 0 {
		return ""
	}
	value, ok := result[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func summarizeNativeAgentMemoryArguments(toolName string, args map[string]interface{}) map[string]interface{} {
	summary := map[string]interface{}{}
	if key := stringArg(args, "key"); key != "" {
		summary["key"] = key
	}
	if toolName == agentMemoryToolUpdate {
		content := strings.TrimSpace(stringArg(args, "content"))
		if content != "" {
			summary["content_preview"] = truncateNativeAgentMemoryRunes(content, 160)
			summary["content_chars"] = len([]rune(content))
		}
	}
	return summary
}

func validateNativeAgentMemoryDecision(decision nativeAgentMemoryDecision, slots []AgentMemorySlotConfig) (string, map[string]interface{}, error) {
	action := strings.ToLower(strings.TrimSpace(decision.Action))
	if action == "" || action == "none" {
		return "", nil, fmt.Errorf("%w: memory decision has no mutation", ErrInvalidInput)
	}
	slot, ok := findEnabledAgentMemorySlot(slots, decision.Key)
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
		if containsSensitiveAgentMemoryContent(content) {
			return "", nil, fmt.Errorf("%w: sensitive content cannot be saved to agent memory", ErrInvalidInput)
		}
		args["content"] = content
		return agentMemoryToolUpdate, args, nil
	case "clear":
		return agentMemoryToolClear, args, nil
	default:
		return "", nil, fmt.Errorf("%w: unsupported memory decision action", ErrInvalidInput)
	}
}

func findEnabledAgentMemorySlot(slots []AgentMemorySlotConfig, key string) (AgentMemorySlotConfig, bool) {
	key = strings.TrimSpace(key)
	for _, slot := range slots {
		if slot.Enabled && strings.TrimSpace(slot.Key) == key {
			return slot, true
		}
	}
	return AgentMemorySlotConfig{}, false
}

func containsSensitiveAgentMemoryContent(content string) bool {
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
