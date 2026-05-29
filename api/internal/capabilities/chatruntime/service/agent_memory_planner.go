package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

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
