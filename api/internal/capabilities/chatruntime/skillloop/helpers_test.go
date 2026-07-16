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

func TestMergeStreamUsageSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		snapshots []*adapter.Usage
		want      adapter.Usage
	}{
		{
			name: "repeated cumulative snapshot",
			snapshots: []*adapter.Usage{
				{PromptTokens: 5078, CompletionTokens: 239, TotalTokens: 5317},
				{PromptTokens: 5078, CompletionTokens: 239, TotalTokens: 5317},
			},
			want: adapter.Usage{PromptTokens: 5078, CompletionTokens: 239, TotalTokens: 5317},
		},
		{
			name: "monotonic cumulative growth",
			snapshots: []*adapter.Usage{
				{PromptTokens: 5078, CompletionTokens: 50, TotalTokens: 5128},
				{PromptTokens: 5078, CompletionTokens: 239, TotalTokens: 5317},
			},
			want: adapter.Usage{PromptTokens: 5078, CompletionTokens: 239, TotalTokens: 5317},
		},
		{
			name: "components arrive separately",
			snapshots: []*adapter.Usage{
				{PromptTokens: 5078, TotalTokens: 5078},
				{CompletionTokens: 239, TotalTokens: 239},
			},
			want: adapter.Usage{PromptTokens: 5078, CompletionTokens: 239, TotalTokens: 5317},
		},
		{
			name: "total is normalized from components",
			snapshots: []*adapter.Usage{
				{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 10},
			},
			want: adapter.Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *adapter.Usage
			for _, snapshot := range tt.snapshots {
				got = mergeStreamUsageSnapshot(got, snapshot)
			}
			if got == nil {
				t.Fatal("mergeStreamUsageSnapshot() = nil")
			}
			if *got != tt.want {
				t.Fatalf("mergeStreamUsageSnapshot() = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestMergeUsageAddsDistinctInvocations(t *testing.T) {
	got := mergeUsage(
		&adapter.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
		&adapter.Usage{PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60},
	)
	want := adapter.Usage{PromptTokens: 150, CompletionTokens: 30, TotalTokens: 180}
	if got == nil || *got != want {
		t.Fatalf("mergeUsage() = %+v, want %+v", got, want)
	}
}

func TestRecordModelInvocationEstimatesCompleteFinalRequest(t *testing.T) {
	var captured ModelInvocationTrace
	runner := &Runner{
		OnModelInvocation: func(trace ModelInvocationTrace) {
			captured = trace
		},
	}
	request := &adapter.ChatRequest{
		Model: "deepseek-chat",
		Messages: adapter.NormalizeSystemMessages([]adapter.Message{
			{Role: "system", Content: "first instruction"},
			{Role: "user", Content: "do the task"},
			{Role: "system", Content: "second instruction"},
		}),
		Tools: []adapter.Tool{{
			Type: "function",
			Function: adapter.Function{
				Name:        "perform_task",
				Description: "Performs the requested task",
				Parameters:  map[string]interface{}{"type": "object"},
			},
		}},
		ToolChoice:     "auto",
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	}

	runner.recordModelInvocation(ModelInvocationTrace{Request: request})

	if captured.EstimatedPromptTokens <= 0 {
		t.Fatalf("estimated prompt tokens = %d, want positive", captured.EstimatedPromptTokens)
	}
	if captured.RequestChars <= captured.PromptChars {
		t.Fatalf("request chars = %d, want greater than legacy prompt chars %d", captured.RequestChars, captured.PromptChars)
	}
	for _, name := range []string{"messages", "tools", "tool_choice", "response_format"} {
		if captured.PromptComponentTokens[name] <= 0 {
			t.Fatalf("prompt component tokens[%q] = %d, want positive; all=%#v", name, captured.PromptComponentTokens[name], captured.PromptComponentTokens)
		}
		if captured.PromptComponentChars[name] <= 0 {
			t.Fatalf("prompt component chars[%q] = %d, want positive; all=%#v", name, captured.PromptComponentChars[name], captured.PromptComponentChars)
		}
	}
	if captured.PromptEstimator == "" {
		t.Fatal("prompt estimator is empty")
	}
	if got := len(captured.Request.Messages); got != 2 {
		t.Fatalf("captured request messages = %d, want normalized system plus user", got)
	}
}
