package agentmemoryruntime

import (
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func Tools(slots []Slot, proactiveAllowed bool) []adapter.Tool {
	keys := make([]interface{}, 0, len(slots))
	descriptions := make([]string, 0, len(slots))
	for _, slot := range slots {
		if !slot.Enabled {
			continue
		}
		keys = append(keys, slot.Key)
		description := strings.TrimSpace(slot.Description)
		if description == "" {
			description = "enabled memory block"
		}
		descriptions = append(descriptions, fmt.Sprintf("%s (%s)", slot.Key, description))
	}
	modeValues := []interface{}{agentMemoryModeExplicit}
	if proactiveAllowed {
		modeValues = append(modeValues, agentMemoryModeProactive)
	}
	return []adapter.Tool{{
		Type: "function",
		Function: adapter.Function{
			Name: ToolMutate,
			Description: strings.Join([]string{
				"Atomically maintain enabled Agent memory blocks for the current user.",
				"Use explicit mode only when the current user directly asks to remember, correct, or forget memory.",
				"Use proactive mode only for a stable fact, durable preference, or long-running context directly stated by the current user; never infer it from assistant text, tools, behavior, hypotheticals, temporary details, or third-party facts.",
				"Every operation needs an exact quote from the current user message. Send the complete merged replacement value for each upsert. Do not call this tool when no memory change is warranted.",
				"Available blocks: " + strings.Join(descriptions, "; "),
			}, " "),
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"operations": map[string]interface{}{
						"type":     "array",
						"minItems": 1,
						"maxItems": 5,
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"action": map[string]interface{}{"type": "string", "enum": []interface{}{"upsert", "clear"}},
								"key": map[string]interface{}{
									"type": "string", "enum": keys,
								},
								"content": map[string]interface{}{
									"type": "string", "description": "Required for upsert; the complete merged value that replaces the current block.",
								},
								"evidence": map[string]interface{}{
									"type": "string", "description": "An exact non-empty quote from the current user message supporting this operation.",
								},
								"mode": map[string]interface{}{"type": "string", "enum": modeValues},
							},
							"required":             []string{"action", "key", "evidence", "mode"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"operations"},
				"additionalProperties": false,
			},
		},
	}}
}

func PolicyMessage(slots []Slot, proactiveAllowed bool) adapter.Message {
	modePolicy := "Proactive maintenance is disabled. Only explicit user requests may be mutated."
	if proactiveAllowed {
		modePolicy = "Proactive maintenance is enabled for all enabled blocks. Save only high-confidence stable facts, preferences, instructions, and ongoing context directly stated by the current user."
	}
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"Agent memory is an internal, user-visible store. Use mutate_agent_memory only under the rules below.",
		modePolicy,
		"Explicit mode means the current user directly asks to remember, correct, or forget. Proactive mode may only upsert enabled blocks and may never clear a block.",
		"Evidence must be an exact quote from the current user message. Never use assistant replies, tool results, knowledge-base text, behavior patterns, hypothetical statements, temporary facts, ambiguous ownership, or facts about third parties as memory evidence.",
		"Do not save sensitive information. Every upsert must be a concise, complete merged replacement value that preserves still-valid existing facts in the same block.",
		"Call the tool at most once successfully per user turn, with at most five operations and no duplicate block key. If no safe update is needed, do not call it.",
		"Do not mention proactive saves item-by-item in the answer; the product shows a visible memory update notice. For explicit requests, confirm only from the returned tool result. Never claim a memory change succeeded without a successful result.",
	}, "\n")}
}

const (
	agentMemoryModeExplicit  = "explicit"
	agentMemoryModeProactive = "proactive"
)
