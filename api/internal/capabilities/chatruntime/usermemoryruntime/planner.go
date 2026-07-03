package usermemoryruntime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/memory"
)

func PlannerMessages(state *State, latestUserMessage string, sourceMessages []adapter.Message) []adapter.Message {
	return []adapter.Message{
		DecisionStateMessage(state, latestUserMessage, sourceMessages),
		{Role: "user", Content: "Return exactly one user memory decision JSON object for the structured payload. Do not answer the user."},
	}
}

func DecisionStateMessage(state *State, latestUserMessage string, sourceMessages []adapter.Message) adapter.Message {
	entries := []memory.MemoryEntryResponse{}
	if state != nil {
		entries = state.Entries
	}
	payload := map[string]interface{}{
		"latest_user_message": strings.TrimSpace(latestUserMessage),
		"recent_messages":     recentMessagePayload(sourceMessages),
		"saved_memory":        savedMemoryPayload(entries),
		"current_time":        time.Now().UTC().Format(time.RFC3339),
		"current_date_policy": "Resolve relative dates into absolute RFC3339 expires_at before choosing temporary memory. If date/time cannot be resolved, choose ask_confirmation or none.",
	}
	rawPayload, _ := json.Marshal(payload)
	lines := []string{
		"You are the internal AIChat user-memory decision pass. Decide whether the latest user message should create, update, delete, or ask confirmation for account-level memory.",
		"Return exactly one JSON object and no prose.",
		`Schema: {"action":"none|create|update|delete|ask_confirmation","entry_id":"existing memory id or empty","content":"complete concise memory content","category":"profile|preference|instruction|fact|other","memory_type":"long_term|temporary","expires_at":"RFC3339 timestamp or empty","confidence":0.0,"reason":"short internal reason"}`,
		"Choose action=none for ordinary questions, transient small talk, one-off task requests, secrets, credentials, payment data, government IDs, banking details, or facts about other people that are not explicitly memory-worthy.",
		"Choose action=create when the user provides new durable account-level profile facts, preferences, standing instructions, habits, address forms, or explicitly asks to remember useful short-term context.",
		"Choose action=update when related memory already exists. Write complete merged content and replace stale facts in that entry.",
		"Choose action=delete only when the user explicitly asks to forget, delete, remove, or clear a saved memory.",
		"Choose action=ask_confirmation when new information conflicts with saved memory, is ambiguous, crosses a sensitive boundary, or project/workspace facts should not become account-wide memory without explicit confirmation.",
		"Categories: profile=user identity/name/role/location; preference=language/style/format/tone; instruction=standing behavior rules; fact=stable background or project facts explicitly meant to persist; other=only when no category fits.",
		"Use memory_type=temporary only for date-bounded context and include absolute RFC3339 expires_at. Otherwise use long_term and leave expires_at empty.",
		"Never claim memory was saved here. This is an internal decision only.",
		"Structured memory planner payload:",
		string(rawPayload),
	}
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
}

func ParseDecision(raw string) (Decision, error) {
	raw = strings.TrimSpace(stripJSONCodeFence(raw))
	if raw == "" {
		return Decision{}, fmt.Errorf("empty decision")
	}
	if start := strings.Index(raw, "{"); start > 0 {
		raw = raw[start:]
	}
	if end := strings.LastIndex(raw, "}"); end >= 0 && end < len(raw)-1 {
		raw = raw[:end+1]
	}
	var decision Decision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return Decision{}, fmt.Errorf("parse decision json: %w", err)
	}
	decision.Action = strings.ToLower(strings.TrimSpace(decision.Action))
	decision.EntryID = strings.TrimSpace(decision.EntryID)
	decision.Content = strings.TrimSpace(decision.Content)
	decision.Category = strings.ToLower(strings.TrimSpace(decision.Category))
	decision.MemoryType = strings.ToLower(strings.TrimSpace(decision.MemoryType))
	decision.ExpiresAt = normalizeDecisionExpiresAt(decision.ExpiresAt)
	decision.Reason = strings.TrimSpace(decision.Reason)
	switch decision.Action {
	case "", ActionNone:
		decision.Action = ActionNone
	case ActionCreate, ActionUpdate, ActionDelete, ActionAskConfirmation:
	default:
		return Decision{}, fmt.Errorf("unsupported action %q", decision.Action)
	}
	if decision.Confidence != nil && *decision.Confidence > 0 && *decision.Confidence < 0.55 {
		return Decision{Action: ActionNone, Reason: "decision confidence below threshold"}, nil
	}
	return decision, nil
}

func normalizeDecisionExpiresAt(raw string) string {
	value := strings.TrimSpace(raw)
	switch strings.ToLower(value) {
	case "", "null", "nil", "none", "n/a", "na", "never", "no expiration", "long_term", "long-term", "permanent", "empty", "\u7a7a", "\u65e0", "\u65e0\u9700", "\u957f\u671f", "\u6c38\u4e45":
		return ""
	default:
		return value
	}
}

func DecisionNoop(decision Decision) bool {
	action := strings.TrimSpace(decision.Action)
	return action == "" || action == ActionNone || action == ActionAskConfirmation
}

func PlannerSuccessStatus(decision Decision) string {
	switch decision.Action {
	case ActionCreate:
		return "success_create"
	case ActionUpdate:
		return "success_update"
	case ActionDelete:
		return "success_delete"
	case ActionAskConfirmation:
		return "requires_confirmation"
	default:
		return "success_none"
	}
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

func recentMessagePayload(messages []adapter.Message) []map[string]interface{} {
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
		content := truncateRunes(messageText(message.Content), 1200)
		if content == "" {
			continue
		}
		out = append(out, map[string]interface{}{"role": role, "content": content})
	}
	return out
}

func savedMemoryPayload(entries []memory.MemoryEntryResponse) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		if !entry.Enabled || strings.TrimSpace(entry.Content) == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":          entry.ID,
			"content":     entry.Content,
			"category":    entry.Category,
			"memory_type": entry.MemoryType,
			"expires_at":  entry.ExpiresAt,
			"status":      entry.Status,
		})
	}
	return out
}

func messageText(content interface{}) string {
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
	default:
		return ""
	}
}
