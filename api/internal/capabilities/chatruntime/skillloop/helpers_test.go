package skillloop

import (
	"encoding/json"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestNormalizeToolCallsRepairsBareQuotesInArguments(t *testing.T) {
	raw := `{"skill_id":"agent-management","tool_name":"update_agent_config","arguments":{"agent_id":"agent-1","system_prompt":"午夜走廊尽头通往"家"的神秘之门，疑似怪谈具象化的"影子"。"}}`

	calls := normalizeToolCalls([]adapter.ToolCall{{
		Function: adapter.FunctionCall{
			Name:      "call_skill_tool",
			Arguments: raw,
		},
	}})

	if len(calls) != 1 {
		t.Fatalf("normalizeToolCalls len = %d, want 1", len(calls))
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("normalized arguments are not JSON: %v\n%s", err, calls[0].Function.Arguments)
	}
	toolArgs, ok := args["arguments"].(map[string]interface{})
	if !ok {
		t.Fatalf("arguments = %#v, want object", args["arguments"])
	}
	if got, want := toolArgs["system_prompt"], `午夜走廊尽头通往"家"的神秘之门，疑似怪谈具象化的"影子"。`; got != want {
		t.Fatalf("system_prompt = %#v, want %#v", got, want)
	}
}

func TestNormalizeToolCallsReplacesUnrepairableArgumentsWithJSONFeedback(t *testing.T) {
	calls := normalizeToolCalls([]adapter.ToolCall{{
		Function: adapter.FunctionCall{
			Name:      "call_skill_tool",
			Arguments: `{"skill_id":"agent-management","arguments":{"system_prompt":"unterminated}`,
		},
	}})

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("fallback arguments are not JSON: %v\n%s", err, calls[0].Function.Arguments)
	}
	if got := args["_invalid_tool_arguments"]; got != true {
		t.Fatalf("_invalid_tool_arguments = %#v, want true", got)
	}
	if got := args["next_action"]; got == "" {
		t.Fatalf("next_action = %#v, want recovery guidance", got)
	}
}

func TestCloneChatRequestSanitizesHistoricalToolProtocolMessages(t *testing.T) {
	raw := `{"skill_id":"agent-management","tool_name":"update_agent_config","arguments":{"system_prompt":"通往"家"的门"}}`
	source := &adapter.ChatRequest{Messages: []adapter.Message{
		{
			Role: "assistant",
			ToolCalls: []adapter.ToolCall{{
				ID: "call-1",
				Function: adapter.FunctionCall{
					Name:      "call_skill_tool",
					Arguments: raw,
				},
			}},
		},
		{
			Role:    "tool",
			Content: map[string]interface{}{"error": "bad arguments"},
		},
	}}

	cloned := cloneChatRequest(source)
	if err := json.Unmarshal([]byte(cloned.Messages[0].ToolCalls[0].Function.Arguments), &map[string]interface{}{}); err != nil {
		t.Fatalf("cloned historical tool arguments are not JSON: %v", err)
	}
	if _, ok := cloned.Messages[1].Content.(string); !ok {
		t.Fatalf("tool content type = %T, want string", cloned.Messages[1].Content)
	}
}
