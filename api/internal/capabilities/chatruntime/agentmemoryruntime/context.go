package agentmemoryruntime

import (
	"context"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func BuildContext(ctx context.Context, req ContextRequest) (ContextResult, error) {
	if !req.Enabled || len(enabledSlots(req.Slots)) == 0 {
		return ContextResult{SystemPrompt: req.SystemPrompt}, nil
	}
	slots := enabledSlots(req.Slots)
	metadata := map[string]interface{}{
		"agent_memory": map[string]interface{}{
			"enabled":        true,
			"available":      false,
			"injected":       false,
			"context_status": "skipped_scope",
		},
	}
	state := &State{
		Enabled:       true,
		AgentID:       req.AgentID,
		UserScope:     strings.TrimSpace(req.UserScope),
		EnabledSlots:  slots,
		ContextStatus: "skipped_scope",
	}
	if req.MemoryService == nil || req.WorkspaceID == zeroUUID || req.AgentID == zeroUUID {
		return ContextResult{SystemPrompt: req.SystemPrompt, Metadata: metadata, State: state}, nil
	}
	values, err := req.MemoryService.ReadUserMemory(ctx, req.WorkspaceID, req.AgentID, RuntimeSlots(slots), req.UserScope, req.UserID)
	if err != nil {
		state.ContextStatus = "error"
		state.ContextError = err.Error()
		metadata["agent_memory"] = map[string]interface{}{
			"enabled":        true,
			"available":      false,
			"injected":       false,
			"context_status": "error",
			"context_error":  err.Error(),
		}
		return ContextResult{SystemPrompt: req.SystemPrompt, Metadata: metadata, State: state}, nil
	}
	state.SavedValues = append([]agentmemory.SlotValueResponse(nil), values...)
	state.ContextStatus = "success"
	rendered, injectedCount := RenderContext(values, req.Budget)
	metadata["agent_memory"] = map[string]interface{}{
		"enabled":        true,
		"available":      injectedCount > 0,
		"injected":       strings.TrimSpace(rendered) != "",
		"value_count":    injectedCount,
		"context_status": "success",
	}
	if strings.TrimSpace(rendered) == "" {
		return ContextResult{SystemPrompt: req.SystemPrompt, Metadata: metadata, State: state}, nil
	}
	return ContextResult{SystemPrompt: strings.TrimSpace(req.SystemPrompt) + "\n\n" + rendered, Metadata: metadata, State: state}, nil
}

func AppendPolicy(systemPrompt string, enabled bool, slots []Slot) string {
	rendered := RenderPolicy(enabled, slots)
	if rendered == "" {
		return systemPrompt
	}
	base := strings.TrimSpace(systemPrompt)
	if base == "" {
		return rendered
	}
	return base + "\n\n" + rendered
}

func RenderPolicy(enabled bool, slots []Slot) string {
	if !enabled {
		return ""
	}
	slots = enabledSlots(slots)
	if len(slots) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Agent memory is enabled for this agent.\n")
	b.WriteString("Available memory keys configured by the organizer:\n")
	for _, slot := range slots {
		b.WriteString("- ")
		b.WriteString(slot.Key)
		if description := strings.TrimSpace(slot.Description); description != "" {
			b.WriteString(": ")
			b.WriteString(description)
		}
		if slot.MaxChars > 0 {
			b.WriteString(" (max ")
			b.WriteString(strconv.Itoa(slot.MaxChars))
			b.WriteString(" chars)")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nResponse style rules:\n")
	b.WriteString("- Do not describe internal memory operations to the user. Do not say you loaded memory, read memory, or called a tool.\n")
	b.WriteString("- Use saved memory naturally in the answer. Do not present a separate section saying memories were loaded.\n")
	b.WriteString("- Confirm a memory change only when the current message context contains an internal Agent memory success note for this turn.\n")
	b.WriteString("\nMemory management rules:\n")
	b.WriteString("- Saved Agent memory has already been injected into this system context when available. Use those saved values proactively without waiting for the user to remind you.\n")
	b.WriteString("- Agent memory writes and clears are handled by an internal memory planner before the final answer. Do not invent, simulate, or mention memory tools in the final answer.\n")
	b.WriteString("- If the user provides stable profile facts, preferences, standing instructions, or durable project context, answer normally and let the internal memory planner decide whether to persist it.\n")
	b.WriteString("- Choose keys by semantic fit: profile is only the user's own identity; preferences are response style, language, examples, tone, and formatting; standing_instructions are durable procedures, collaboration rules, interaction rules, user-addressing rules, and agent persona/roleplay instructions; project_context is ongoing project background.\n")
	b.WriteString("- Do not store agent identity, assistant persona, roleplay style, or what the user calls you in profile. Store those in standing_instructions when they are durable interaction rules, or preferences when they are only tone/style preferences.\n")
	b.WriteString("- Do not copy profile facts such as the user's real name, preferred name, job, or team into standing_instructions. If standing_instructions contains an addressing rule, keep it as the rule itself, not as a duplicate profile fact.\n")
	b.WriteString("- When the user changes their name, preferred address, job, or role, update profile only. Do not rewrite standing_instructions unless the user explicitly changes the collaboration rule, assistant persona, or addressing rule.\n")
	b.WriteString("- Do not infer project_context from a profile or job-role change. Update project_context only when the user describes an ongoing project, goal, workstream, or asks to change project background.\n")
	b.WriteString("- Do not save transient small talk, one-off events, secrets, credentials, payment data, government IDs, or other sensitive information. If asked to save sensitive information, politely decline.\n")
	b.WriteString("- Never say you remembered, recorded, updated, saved, cleared, or forgot memory unless an internal Agent memory success note says the change succeeded in this turn.\n")
	b.WriteString("- If there is no internal Agent memory success note, you may say you understand or will follow the user's request in this conversation, but do not claim any memory was saved or changed.\n")
	b.WriteString("- Do not say that you will remember something later. Either update memory successfully in this turn, or answer without claiming it was saved.\n")
	return b.String()
}

func RenderContext(values []agentmemory.SlotValueResponse, budget int) (string, int) {
	if budget <= 0 || len(values) == 0 {
		return "", 0
	}
	var b strings.Builder
	b.WriteString("Saved Agent memory for the current user:\n")
	b.WriteString("Use these saved memories proactively when answering. If the user's latest message conflicts with saved memory, prefer the latest message and update Agent memory when appropriate.\n")
	b.WriteString("Saved standing_instructions are binding interaction rules for this user. Follow them in every reply, including greetings, casual chat, and short turns, unless the latest user message explicitly changes or overrides them.\n")
	b.WriteString("Important: standing_instructions have higher priority than ordinary small talk. Even short turns must follow saved identity, addressing, tone, and interaction rules.\n")
	count := 0
	for _, value := range values {
		content := strings.TrimSpace(value.Content)
		key := strings.TrimSpace(value.Key)
		if content == "" || key == "" {
			continue
		}
		entryLabel := contextEntryLabel(key)
		entry := "- " + entryLabel + ":\n" + indentContent(content) + "\n"
		if b.Len()+len(entry) > budget {
			if count == 0 {
				prefix := "- " + entryLabel + ":\n"
				remaining := budget - b.Len() - len(prefix)
				if remaining > 0 {
					b.WriteString(prefix)
					b.WriteString(indentContent(truncateString(content, remaining)))
					count++
				}
			}
			break
		}
		b.WriteString(entry)
		count++
	}
	if count == 0 {
		return "", 0
	}
	return strings.TrimSpace(b.String()), count
}

func enabledSlots(input []Slot) []Slot {
	out := make([]Slot, 0, len(input))
	for _, slot := range input {
		if slot.Enabled {
			out = append(out, slot)
		}
	}
	return out
}

func contextEntryLabel(key string) string {
	if strings.EqualFold(strings.TrimSpace(key), "standing_instructions") {
		return "standing_instructions (binding interaction rules; follow every turn)"
	}
	return key
}

func indentContent(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	for i, line := range lines {
		lines[i] = "  " + strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

func truncateString(value string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	if maxChars <= 3 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-3]) + "..."
}
