package skillloop

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
)

func runUnifiedCalls(t *testing.T, calls ...adapter.ToolCall) *runnerTestLLMClient {
	t.Helper()
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := nativeActivatableMathResolvedSkills()
	catalog := skills.BuildNativeSkillCatalog(resolved, nil, skills.DefaultNativeSkillCatalogBudgetChars, 0, nil)
	session := skills.NewNativeSkillSession(runtime, resolved, catalog, skills.NativeToolSetOptions{BudgetChars: 10000, UnifiedDispatch: true})
	responses := make([]*adapter.ChatResponse, 0, len(calls)+1)
	for _, call := range calls {
		responses = append(responses, &adapter.ChatResponse{Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{call}}, FinishReason: "tool_calls",
		}}})
	}
	responses = append(responses, &adapter.ChatResponse{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "42"}, FinishReason: "stop"}}})
	fakeLLM := &runnerTestLLMClient{appChatResponses: responses}
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-unified", "msg-unified", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "What is 6 times 7?"}},
	})
	if _, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:             prepared,
		Resolved:             resolved,
		NativeAgentLoop:      true,
		NativeSkillSession:   session,
		UnifiedSkillDispatch: true,
		ExecutionContext:     skills.ExecutionContext{OrganizationID: "org-1", UserID: "user-1"},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return fakeLLM
}

func unifiedCall(id string, command string, input map[string]interface{}) adapter.ToolCall {
	arguments, _ := json.Marshal(map[string]interface{}{"command": command, "input": input})
	return adapter.ToolCall{ID: id, Type: "function", Function: adapter.FunctionCall{Name: skills.UnifiedSkillToolName, Arguments: string(arguments)}}
}

// A unified call runs the tool it names, activating its skill, and the tool
// list does not change (ADR 0044).
func TestUnifiedCallRunsTheNamedToolWithAStableToolList(t *testing.T) {
	fakeLLM := runUnifiedCalls(t, unifiedCall("calc-1", nativeActivatableMathSkillID+" calculate", map[string]interface{}{"operation": "multiply", "left": 6, "right": 7}))
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("model calls = %d, want the call and the answer", fakeLLM.appChatCalls)
	}
	first, second := fakeLLM.appChatRequests[0], fakeLLM.appChatRequests[1]
	if !runnerTestHasTool(first.Tools, skills.UnifiedSkillToolName) || runnerTestHasTool(first.Tools, "calculate") {
		t.Fatalf("first tools = %#v, want zgi without calculate", first.Tools)
	}
	if !reflect.DeepEqual(first.Tools, second.Tools) {
		t.Fatalf("tool list changed after activation:\n%#v\n%#v", first.Tools, second.Tools)
	}
	if result := toolResultFor(second.Messages, "calc-1"); !strings.Contains(result, "42") {
		t.Fatalf("calculate result = %s, want 42", result)
	}
	if !runnerTestMessagesContain(second.Messages, "calculate(left: number, operation: add|subtract|multiply|divide|power|mod, right: number, precision?: number)") {
		t.Fatalf("second request messages = %#v, want the skill's tools listed in its instructions", second.Messages)
	}
	for _, message := range second.Messages {
		for _, call := range message.ToolCalls {
			if call.ID == "calc-1" && call.Function.Name != skills.UnifiedSkillToolName {
				t.Fatalf("stored call = %#v, want the model's own zgi call", call)
			}
		}
	}
}

func TestUnifiedCallThatNamesNoToolIsRecoverable(t *testing.T) {
	fakeLLM := runUnifiedCalls(t, unifiedCall("bad-1", "nothing here", map[string]interface{}{}))
	result := toolResultFor(fakeLLM.appChatRequests[1].Messages, "bad-1")
	if !strings.Contains(result, "zgi command must name") {
		t.Fatalf("result = %s, want the unified command error", result)
	}
}
