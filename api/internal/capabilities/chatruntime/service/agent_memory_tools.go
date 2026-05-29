package service

import (
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

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
