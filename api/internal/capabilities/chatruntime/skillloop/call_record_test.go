package skillloop

import (
	"context"
	"errors"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func callRecordTestRun(t *testing.T, onEvent func(Event) error, tools ...RuntimeTool) (*runnerTestLLMClient, error) {
	t.Helper()
	calls := make([]adapter.ToolCall, 0, len(tools))
	for _, tool := range tools {
		name := tool.Definition.Function.Name
		calls = append(calls, adapter.ToolCall{ID: "call-" + name, Type: "function", Function: adapter.FunctionCall{Name: name, Arguments: "{}"}})
	}
	client := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: calls}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "done"}, FinishReason: "stop"}}},
	}}
	runner := &Runner{LLMClient: client, SkillRuntime: skills.NewRuntime(nil, nil), AppContext: &llmclient.AppContext{}, OnEvent: onEvent}
	prepared := NewPreparedChat("conv-record", "msg-record", "", "auto", &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "work"}}})
	_, _, err := runner.Run(context.Background(), RunRequest{Prepared: prepared, Resolved: &skills.ResolvedSkills{}, RuntimeTools: tools})
	return client, err
}

func callRecordTestTool(name string, readOnly bool, ran *bool) RuntimeTool {
	return RuntimeTool{
		SkillID:    "digital-human-memory",
		Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: name}},
		ReadOnly:   readOnly,
		Handler: func(context.Context, adapter.ToolCall) RuntimeToolResult {
			*ran = true
			return RuntimeToolResult{Status: "success", Result: map[string]interface{}{"ok": true}}
		},
	}
}

// A call runs only after its start is recorded: if the process stopped while
// an unrecorded call ran, nothing would show later that it may have changed
// something (ADR 0035).
func TestCallDoesNotRunWhenItsStartCannotBeRecorded(t *testing.T) {
	recordFailed := errors.New("database unavailable")
	ran := false
	client, err := callRecordTestRun(t, func(event Event) error {
		if event.Type == EventSkillCallStart {
			return recordFailed
		}
		return nil
	}, callRecordTestTool("memory_edit", false, &ran))
	if !errors.Is(err, recordFailed) {
		t.Fatalf("Run() error = %v, want the recording failure", err)
	}
	if ran {
		t.Fatal("the tool ran although its start was not recorded")
	}
	if client.appChatCalls != 1 {
		t.Fatalf("model calls = %d, want the run to stop after the first", client.appChatCalls)
	}
}

func TestCallStartRecordsWhetherARuntimeToolIsReadOnly(t *testing.T) {
	var viewRan, editRan bool
	starts := map[string]map[string]interface{}{}
	_, err := callRecordTestRun(t, func(event Event) error {
		if event.Type == EventSkillCallStart {
			starts[stringFromAny(event.Payload["tool_name"])] = event.Payload
		}
		return nil
	}, callRecordTestTool("memory_view", true, &viewRan), callRecordTestTool("memory_edit", false, &editRan))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if starts["memory_view"][skillCallReadOnlyKey] != true {
		t.Fatalf("memory_view start = %#v, want it marked read-only", starts["memory_view"])
	}
	if _, marked := starts["memory_edit"][skillCallReadOnlyKey]; marked {
		t.Fatalf("memory_edit start = %#v, want no read-only mark", starts["memory_edit"])
	}
}
