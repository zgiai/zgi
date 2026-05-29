package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	agentMemoryToolUpdate = "update_agent_memory"
	agentMemoryToolClear  = "clear_agent_memory"

	maxNativeAgentMemoryRounds  = 3
	maxNativeAgentMemoryRetries = 1

	minAgentMemoryDecisionConfidence = 0.55
)

type nativeAgentMemoryDecision = AgentMemoryPlannerDecision

func (s *service) runNativeAgentMemoryPreflight(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	onEvent func(StreamEvent) error,
) (*adapter.Usage, error) {
	state, skipStatus := agentMemoryPreflightState(prepared, s.agentMemoryService, s.llmClient != nil)
	if skipStatus != "" {
		if prepared != nil && prepared.parts != nil && prepared.parts.AgentMemoryEnabled {
			trace := nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: skipStatus}, skipStatus, nil)
			s.recordNativeAgentMemoryTrace(ctx, persistCtx, prepared, []skills.SkillTrace{}, trace)
			s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"planner_status": skipStatus,
				"planner_action": "none",
			})
		}
		return nil, nil
	}

	if !shouldRunNativeAgentMemoryDecision(prepared.parts.Query) {
		trace := nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: "empty latest user message"}, "skipped_empty_query", nil)
		s.recordNativeAgentMemoryTrace(ctx, persistCtx, prepared, []skills.SkillTrace{}, trace)
		s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
			"planner_status": "skipped_empty_query",
			"planner_action": "none",
		})
		prepared.LLMRequest.Messages = append(prepared.LLMRequest.Messages, nativeAgentMemoryGuardNote("skipped_empty_query"))
		return nil, nil
	}

	baseMessages := append([]adapter.Message{}, prepared.LLMRequest.Messages...)
	slots := state.EnabledSlots
	messages := nativeAgentMemoryPlannerMessages(state, prepared.parts.Query, baseMessages)
	var usage *adapter.Usage
	retries := 0
	traces := []skills.SkillTrace{}
	recordTrace := func(trace skills.SkillTrace) {
		traces = s.recordNativeAgentMemoryTrace(ctx, persistCtx, prepared, traces, trace)
	}

	for round := 0; round < maxNativeAgentMemoryRounds; round++ {
		req := cloneChatRequest(prepared.LLMRequest)
		req.Messages = messages
		req.Stream = false
		req.Tools = nil
		req.ToolChoice = nil
		temperature := 0.0
		req.Temperature = &temperature
		if shouldUseAgentMemoryPlannerJSONMode(prepared) {
			req.ResponseFormat = &adapter.ResponseFormat{Type: "json_object"}
		} else {
			req.ResponseFormat = nil
		}
		maxTokens := 700
		req.MaxTokens = &maxTokens

		resp, err := s.llmClient.AppChat(ctx, newBillingAppContext(prepared), req)
		if err != nil {
			trace := nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: "planner llm error"}, "error_llm", err)
			recordTrace(trace)
			s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"planner_status": "error_llm",
				"planner_action": "none",
			})
			prepared.LLMRequest.Messages = append(baseMessages, nativeAgentMemoryGuardNote("error_llm"))
			return usage, nil
		}
		usage = mergeUsage(usage, planningRespUsage(resp))
		message := firstPlanningMessage(resp)
		decision, err := parseNativeAgentMemoryDecision(assistantMessageText(message))
		if err != nil {
			retries++
			if retries > maxNativeAgentMemoryRetries {
				recordTrace(nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: "planner returned invalid JSON"}, "error_parse", err))
				s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
					"planner_status": "error_parse",
					"planner_action": "none",
				})
				prepared.LLMRequest.Messages = append(baseMessages, nativeAgentMemoryGuardNote("error_parse"))
				break
			}
			recordTrace(nativeAgentMemoryPlannerTrace(nativeAgentMemoryDecision{Action: "none", Reason: "planner returned invalid JSON"}, "error_parse", err))
			messages = append(messages, nativeAgentMemoryDecisionRetryMessage(err))
			continue
		}
		if decisionNoop(decision) {
			recordTrace(nativeAgentMemoryPlannerTrace(decision, "success_none", nil))
			s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"planner_status": "success_none",
				"planner_action": "none",
			})
			prepared.LLMRequest.Messages = append(baseMessages, nativeAgentMemoryGuardNote("success_none"))
			break
		}
		plannerStatus := nativeAgentMemoryPlannerSuccessStatus(decision)
		recordTrace(nativeAgentMemoryPlannerTrace(decision, plannerStatus, nil))
		s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
			"planner_status": plannerStatus,
			"planner_action": decision.Action,
			"planner_key":    decision.Key,
		})
		result, trace, err := s.applyNativeAgentMemoryDecision(ctx, prepared, slots, decision, onEvent)
		recordTrace(trace)
		if err != nil {
			s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"mutation_status": trace.Status,
				"mutation_key":    decision.Key,
			})
			prepared.LLMRequest.Messages = append(baseMessages, nativeAgentMemoryGuardNote(trace.Status))
			break
		}
		finalMessages := append([]adapter.Message{}, baseMessages...)
		finalMessages = append(finalMessages, nativeAgentMemorySuccessNote(decision, result))
		prepared.LLMRequest.Messages = finalMessages
		s.updateAgentMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
			"mutation_status": "success",
			"mutation_key":    stringResultValue(result, "key"),
		})
		break
	}
	return usage, nil
}

func shouldRunNativeAgentMemoryPreflight(prepared *PreparedChat, memoryService AgentMemoryContextService, llmConfigured bool) bool {
	if prepared == nil || prepared.parts == nil || prepared.LLMRequest == nil || !llmConfigured || memoryService == nil {
		return false
	}
	if !prepared.parts.AgentMemoryEnabled || len(enabledAgentMemorySlots(prepared.parts.AgentMemorySlots)) == 0 {
		return false
	}
	if prepared.Scope.WorkspaceID == nil || *prepared.Scope.WorkspaceID == uuid.Nil {
		return false
	}
	agentID, err := uuid.Parse(strings.TrimSpace(prepared.parts.AgentMemoryAgentID))
	return err == nil && agentID != uuid.Nil
}

func agentMemoryPreflightState(prepared *PreparedChat, memoryService AgentMemoryContextService, llmConfigured bool) (*AgentMemoryRuntimeState, string) {
	if prepared == nil || prepared.parts == nil || prepared.LLMRequest == nil {
		return nil, ""
	}
	if !prepared.parts.AgentMemoryEnabled {
		return nil, ""
	}
	slots := enabledAgentMemorySlots(prepared.parts.AgentMemorySlots)
	if len(slots) == 0 {
		return nil, "skipped_scope"
	}
	if memoryService == nil || !llmConfigured {
		return nil, "skipped_scope"
	}
	state := prepared.parts.AgentMemoryRuntimeState
	if state == nil {
		state = &AgentMemoryRuntimeState{
			Enabled:      true,
			UserScope:    strings.TrimSpace(prepared.parts.AgentMemoryUserScope),
			EnabledSlots: slots,
		}
		prepared.parts.AgentMemoryRuntimeState = state
	}
	if len(state.EnabledSlots) == 0 {
		state.EnabledSlots = slots
	}
	if prepared.Scope.WorkspaceID == nil || *prepared.Scope.WorkspaceID == uuid.Nil {
		return state, "skipped_scope"
	}
	agentID, err := uuid.Parse(strings.TrimSpace(prepared.parts.AgentMemoryAgentID))
	if err != nil || agentID == uuid.Nil {
		return state, "skipped_scope"
	}
	state.AgentID = agentID
	return state, ""
}

func nativeAgentMemoryDecisionStateMessage(state *AgentMemoryRuntimeState, latestUserMessage string) adapter.Message {
	return nativeAgentMemoryDecisionStateMessageWithHistory(state, latestUserMessage, nil)
}

func nativeAgentMemoryPlannerMessages(state *AgentMemoryRuntimeState, latestUserMessage string, sourceMessages []adapter.Message) []adapter.Message {
	return []adapter.Message{
		nativeAgentMemoryDecisionStateMessageWithHistory(state, latestUserMessage, sourceMessages),
		{Role: "user", Content: "Return exactly one Agent memory decision JSON object for the structured payload. Do not answer the user."},
	}
}

func nativeAgentMemoryDecisionStateMessageWithHistory(state *AgentMemoryRuntimeState, latestUserMessage string, sourceMessages []adapter.Message) adapter.Message {
	slots := []AgentMemorySlotConfig{}
	values := []agentmemory.SlotValueResponse{}
	if state != nil {
		slots = state.EnabledSlots
		values = state.SavedValues
	}
	slotLines := make([]string, 0, len(slots))
	for _, slot := range slots {
		description := strings.TrimSpace(slot.Description)
		if description == "" {
			description = "No description provided."
		}
		slotLines = append(slotLines, fmt.Sprintf("- %s: %s (max %d chars)", slot.Key, description, slot.MaxChars))
	}
	payload := map[string]interface{}{
		"latest_user_message": strings.TrimSpace(latestUserMessage),
		"recent_messages":     nativeAgentMemoryRecentMessagePayload(sourceMessages),
		"enabled_slots":       nativeAgentMemorySlotPayload(slots),
		"saved_memory":        nativeAgentMemorySavedValuePayload(values),
	}
	rawPayload, _ := json.Marshal(payload)
	lines := []string{
		"You are the internal Agent memory decision pass. Decide whether the latest user message should update or clear one configured Agent memory slot.",
		"Use the structured payload below plus the preceding conversation messages to resolve references such as \"this way\", \"the above approach\", or \"do this from now on\".",
		"Return exactly one JSON object and no prose.",
		`Schema: {"action":"none|update|clear","key":"enabled slot key or empty","content":"complete merged slot content for update, empty otherwise","confidence":0.0,"reason":"short internal reason"}`,
		"Choose action=none for ordinary questions, transient small talk, one-off facts, temporary emotions, passwords, credentials, payment data, government IDs, banking details, secrets, or unsupported keys.",
		"Choose action=none for capability questions or one-off task requests such as asking whether the assistant can draw charts or asking for one chart now.",
		"Choose action=update when the latest user message provides stable profile facts, durable answer preferences, standing collaboration/interaction rules, assistant persona/addressing rules, or ongoing project context.",
		"Choose action=clear only when the latest user message explicitly asks to forget, delete, remove, or clear saved Agent memory.",
		"Slot routing guidance:",
		"- profile: the user's own name, preferred name, job, team role, or stable identity. Never store assistant persona or what the user calls the assistant here.",
		"- preferences: answer language, style, examples, length, formatting, tone, and output format preferences.",
		"- standing_instructions: durable procedures, collaboration rules, assistant persona, how the user addresses the assistant, how the assistant must address the user, and ongoing interaction rules.",
		"- project_context: ongoing projects, goals, workstreams, background, and long-running responsibilities.",
		"When updating, write complete merged content for the chosen slot. Preserve still-valid saved content and replace stale facts in that same slot.",
		"Use exactly one of the enabled keys below. Do not invent keys.",
		"Enabled memory slots:",
		strings.Join(slotLines, "\n"),
		"Structured memory planner payload:",
		string(rawPayload),
	}
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
}

func nativeAgentMemoryRecentMessagePayload(messages []adapter.Message) []map[string]interface{} {
	const maxRecentPlannerMessages = 8
	out := make([]map[string]interface{}, 0, maxRecentPlannerMessages)
	start := 0
	if len(messages) > maxRecentPlannerMessages {
		start = len(messages) - maxRecentPlannerMessages
	}
	for _, message := range messages[start:] {
		role := strings.TrimSpace(message.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := truncateNativeAgentMemoryRunes(messageTextForAgentMemoryPlanner(message.Content), 1200)
		if content == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}
	return out
}

func messageTextForAgentMemoryPlanner(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []adapter.MessageContentPart:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, strings.TrimSpace(part.Text))
			}
		}
		return strings.Join(parts, "\n")
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if text, ok := itemMap["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, strings.TrimSpace(text))
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func nativeAgentMemorySlotPayload(slots []AgentMemorySlotConfig) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(slots))
	for _, slot := range slots {
		out = append(out, map[string]interface{}{
			"key":         slot.Key,
			"description": slot.Description,
			"max_chars":   slot.MaxChars,
		})
	}
	return out
}

func nativeAgentMemorySavedValuePayload(values []agentmemory.SlotValueResponse) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(values))
	for _, value := range values {
		content := strings.TrimSpace(value.Content)
		key := strings.TrimSpace(value.Key)
		if key == "" || content == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"key":     key,
			"content": content,
		})
	}
	return out
}

func nativeAgentMemoryDecisionMessage(slots []AgentMemorySlotConfig) adapter.Message {
	slotLines := make([]string, 0, len(slots))
	for _, slot := range slots {
		description := strings.TrimSpace(slot.Description)
		if description == "" {
			description = "No description provided."
		}
		slotLines = append(slotLines, fmt.Sprintf("- %s: %s (max %d chars)", slot.Key, description, slot.MaxChars))
	}
	lines := []string{
		"You are the internal Agent memory decision pass. Decide whether the latest user message should update or clear one configured Agent memory slot.",
		"Saved memory values, if any, are already present in the previous system context. Use them to merge or replace stale facts.",
		"Return exactly one JSON object and no prose.",
		`Schema: {"action":"none|update|clear","key":"enabled slot key or empty","content":"complete merged slot content for update, empty otherwise","confidence":0.0,"reason":"short internal reason"}`,
		"Choose action=none for ordinary questions, transient small talk, one-off facts, temporary emotions, passwords, credentials, payment data, government IDs, banking details, secrets, or unsupported keys.",
		"Choose action=none for capability questions or one-off task requests such as asking whether the assistant can draw charts or asking for one chart now.",
		"Choose action=update when the latest user message provides stable profile facts, durable answer preferences, standing collaboration/interaction rules, assistant persona/addressing rules, or ongoing project context.",
		"Choose action=clear only when the latest user message explicitly asks to forget, delete, remove, or clear saved Agent memory.",
		"Slot routing guidance:",
		"- profile: the user's own name, preferred name, job, team role, or stable identity. Never store assistant persona or what the user calls the assistant here.",
		"- preferences: answer language, style, examples, length, formatting, tone, and output format preferences.",
		"- standing_instructions: durable procedures, collaboration rules, assistant persona, how the user addresses the assistant, how the assistant must address the user, and ongoing interaction rules. Chinese examples such as \"以后你是...\", \"我叫你...\", \"你要叫我...\", \"叫我主人\", or \"以后每次...\" should be update/standing_instructions when that key is enabled.",
		"- project_context: ongoing projects, goals, workstreams, background, and long-running responsibilities.",
		"Use exactly one of the enabled keys below. Do not invent keys.",
		"Enabled memory slots:",
		strings.Join(slotLines, "\n"),
	}
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
}

func nativeAgentMemoryDecisionRetryMessage(err error) adapter.Message {
	return adapter.Message{Role: "system", Content: "Return a valid Agent memory decision JSON object only. Previous decision was invalid: " + err.Error()}
}

func nativeAgentMemoryTools(slots []AgentMemorySlotConfig) []adapter.Tool {
	keys := make([]interface{}, 0, len(slots))
	descriptions := make([]string, 0, len(slots))
	for _, slot := range slots {
		keys = append(keys, slot.Key)
		if description := strings.TrimSpace(slot.Description); description != "" {
			descriptions = append(descriptions, fmt.Sprintf("%s: %s", slot.Key, description))
		} else {
			descriptions = append(descriptions, slot.Key)
		}
	}
	keyProperty := map[string]interface{}{
		"type":        "string",
		"description": "Enabled Agent memory key to operate on. Choose by semantic fit only.",
		"enum":        keys,
	}
	return []adapter.Tool{
		{
			Type: "function",
			Function: adapter.Function{
				Name: agentMemoryToolUpdate,
				Description: strings.Join([]string{
					"Update one enabled Agent memory slot for the current user.",
					"Use this only for stable profile facts, durable response preferences, standing collaboration rules, assistant persona/addressing rules, or ongoing project context from the latest user message.",
					"Do not use for transient small talk, one-off events, passwords, credentials, payment data, government IDs, or banking details.",
					"Available keys: " + strings.Join(descriptions, "; "),
				}, " "),
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"key": keyProperty,
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Concise complete memory content for this slot. Merge with existing injected memory and replace outdated facts in the same slot.",
						},
					},
					"required":             []string{"key", "content"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: adapter.Function{
				Name: agentMemoryToolClear,
				Description: strings.Join([]string{
					"Clear one enabled Agent memory slot for the current user.",
					"Use only when the latest user message explicitly asks to forget, delete, remove, or clear saved Agent memory.",
					"Available keys: " + strings.Join(descriptions, "; "),
				}, " "),
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"key": keyProperty,
					},
					"required":             []string{"key"},
					"additionalProperties": false,
				},
			},
		},
	}
}

func nativeAgentMemoryToolCalls(calls []adapter.ToolCall) []adapter.ToolCall {
	out := make([]adapter.ToolCall, 0, len(calls))
	for _, call := range calls {
		switch strings.TrimSpace(call.Function.Name) {
		case agentMemoryToolUpdate, agentMemoryToolClear:
			out = append(out, call)
		}
	}
	return out
}

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

func parseNativeAgentMemoryDecision(raw string) (nativeAgentMemoryDecision, error) {
	raw = strings.TrimSpace(stripJSONCodeFence(raw))
	if raw == "" {
		return nativeAgentMemoryDecision{}, fmt.Errorf("empty decision")
	}
	if start := strings.Index(raw, "{"); start > 0 {
		raw = raw[start:]
	}
	if end := strings.LastIndex(raw, "}"); end >= 0 && end < len(raw)-1 {
		raw = raw[:end+1]
	}
	var wrapper struct {
		Action     string                      `json:"action"`
		Key        string                      `json:"key"`
		Content    string                      `json:"content"`
		Confidence *float64                    `json:"confidence"`
		Reason     string                      `json:"reason"`
		Decisions  []nativeAgentMemoryDecision `json:"decisions"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return nativeAgentMemoryDecision{}, fmt.Errorf("parse decision json: %w", err)
	}
	var decision nativeAgentMemoryDecision
	if len(wrapper.Decisions) > 0 {
		decision = wrapper.Decisions[0]
	} else {
		decision = nativeAgentMemoryDecision{
			Action:     wrapper.Action,
			Key:        wrapper.Key,
			Content:    wrapper.Content,
			Confidence: wrapper.Confidence,
			Reason:     wrapper.Reason,
		}
	}
	decision.Action = strings.ToLower(strings.TrimSpace(decision.Action))
	decision.Key = strings.TrimSpace(decision.Key)
	decision.Content = strings.TrimSpace(decision.Content)
	decision.Reason = strings.TrimSpace(decision.Reason)
	switch decision.Action {
	case "", "none":
		decision.Action = "none"
		return decision, nil
	case "update", "clear":
	default:
		return nativeAgentMemoryDecision{}, fmt.Errorf("unsupported action %q", decision.Action)
	}
	if decision.Confidence != nil && *decision.Confidence > 0 && *decision.Confidence < minAgentMemoryDecisionConfidence {
		return nativeAgentMemoryDecision{Action: "none", Reason: "decision confidence below threshold"}, nil
	}
	if decision.Key == "" {
		return nativeAgentMemoryDecision{}, fmt.Errorf("key is required for %s", decision.Action)
	}
	if decision.Action == "update" && decision.Content == "" {
		return nativeAgentMemoryDecision{}, fmt.Errorf("content is required for update")
	}
	return decision, nil
}

func stripJSONCodeFence(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "```") {
		return raw
	}
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```JSON")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}

func decisionNoop(decision nativeAgentMemoryDecision) bool {
	return strings.TrimSpace(decision.Action) == "" || strings.EqualFold(decision.Action, "none")
}

func nativeAgentMemoryDecisionToolCall(decision nativeAgentMemoryDecision) (adapter.ToolCall, error) {
	args := map[string]interface{}{"key": decision.Key}
	toolName := ""
	switch strings.ToLower(strings.TrimSpace(decision.Action)) {
	case "update":
		toolName = agentMemoryToolUpdate
		args["content"] = decision.Content
	case "clear":
		toolName = agentMemoryToolClear
	default:
		return adapter.ToolCall{}, fmt.Errorf("unsupported decision action %q", decision.Action)
	}
	rawArgs, err := json.Marshal(args)
	if err != nil {
		return adapter.ToolCall{}, fmt.Errorf("marshal memory decision arguments: %w", err)
	}
	return adapter.ToolCall{
		ID:   "agent_memory_decision_1",
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      toolName,
			Arguments: string(rawArgs),
		},
	}, nil
}

func nativeAgentMemorySuccessNote(decision nativeAgentMemoryDecision, result map[string]interface{}) adapter.Message {
	action := strings.ToLower(strings.TrimSpace(decision.Action))
	key := strings.TrimSpace(decision.Key)
	if resultKey, ok := result["key"].(string); ok && strings.TrimSpace(resultKey) != "" {
		key = strings.TrimSpace(resultKey)
	}
	verb := "updated"
	if action == "clear" {
		verb = "clear"
	} else {
		verb = "update"
	}
	content := fmt.Sprintf("Internal Agent memory note: Agent memory %s succeeded for key %q. The final answer may briefly confirm this memory change if relevant. Do not mention tools, planner, or internal memory process.", verb, key)
	return adapter.Message{Role: "system", Content: content}
}

func nativeAgentMemoryGuardNote(status string) adapter.Message {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "none"
	}
	content := fmt.Sprintf("Internal Agent memory note: no Agent memory mutation succeeded in this turn (status: %s). The final answer must not say memory was remembered, recorded, saved, updated, cleared, or forgotten. It may acknowledge the user's request for the current conversation only.", status)
	return adapter.Message{Role: "system", Content: content}
}

func nativeAgentMemoryPlannerSuccessStatus(decision nativeAgentMemoryDecision) string {
	switch strings.ToLower(strings.TrimSpace(decision.Action)) {
	case "update":
		return "success_update"
	case "clear":
		return "success_clear"
	default:
		return "success_none"
	}
}

func shouldUseAgentMemoryPlannerJSONMode(_ *PreparedChat) bool {
	return false
}

func shouldRunNativeAgentMemoryDecision(query string) bool {
	return strings.TrimSpace(query) != ""
}

func (s *service) recordNativeAgentMemoryTrace(ctx context.Context, persistCtx context.Context, prepared *PreparedChat, traces []skills.SkillTrace, trace skills.SkillTrace) []skills.SkillTrace {
	if strings.TrimSpace(trace.Status) == "" {
		trace.Status = "success_none"
	}
	traces = append(traces, trace)
	s.persistSkillTracesBestEffort(persistCtx, prepared, traces)
	s.logSkillTrace(ctx, prepared, trace)
	return traces
}

func (s *service) updateAgentMemoryRuntimeMetadataBestEffort(ctx context.Context, prepared *PreparedChat, updates map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(updates) == 0 {
		return
	}
	metadata := copyStringAnyMap(prepared.Message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	contextControl := map[string]interface{}{}
	if prepared.parts != nil && prepared.parts.ContextControl != nil {
		contextControl = copyStringAnyMap(prepared.parts.ContextControl)
	}
	if existing, ok := metadata["context_control"].(map[string]interface{}); ok {
		for key, value := range existing {
			contextControl[key] = value
		}
	}
	agentMemory := map[string]interface{}{}
	if existing, ok := contextControl["agent_memory"].(map[string]interface{}); ok {
		agentMemory = copyStringAnyMap(existing)
	}
	for key, value := range updates {
		if stringValue, ok := value.(string); ok && strings.TrimSpace(stringValue) == "" {
			continue
		}
		agentMemory[key] = value
	}
	contextControl["agent_memory"] = agentMemory
	metadata["context_control"] = contextControl
	prepared.Message.Metadata = metadata
	if prepared.parts != nil {
		prepared.parts.ContextControl = contextControl
	}
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	_ = s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata)
}

func nativeAgentMemoryPlannerTrace(decision nativeAgentMemoryDecision, status string, err error) skills.SkillTrace {
	trace := nativeAgentMemoryDecisionTrace(decision, status, nil, err)
	trace.Kind = "memory_planner"
	trace.ToolName = "plan_agent_memory"
	return trace
}

func nativeAgentMemoryDecisionTrace(decision nativeAgentMemoryDecision, status string, result map[string]interface{}, err error) skills.SkillTrace {
	trace := skills.SkillTrace{
		Kind:      "agent_memory",
		SkillID:   skills.SkillAgentMemory,
		Status:    status,
		Arguments: map[string]interface{}{},
		Result:    map[string]interface{}{},
	}
	if action := strings.TrimSpace(decision.Action); action != "" {
		trace.Arguments["action"] = action
	}
	if key := strings.TrimSpace(decision.Key); key != "" {
		trace.Arguments["key"] = key
	}
	if decision.Confidence != nil {
		trace.Arguments["confidence"] = *decision.Confidence
	}
	if reason := strings.TrimSpace(decision.Reason); reason != "" {
		trace.Result["reason"] = truncateNativeAgentMemoryRunes(reason, 160)
	}
	for key, value := range result {
		trace.Result[key] = value
	}
	if err != nil {
		trace.Error = err.Error()
	}
	return trace
}

func truncateNativeAgentMemoryRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
