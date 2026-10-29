package skillloop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
)

// runDirectCalculateCall runs a turn whose first round calls calculate without
// activating the calculator, as a model does with a tool it used in an
// earlier turn.
func runDirectCalculateCall(t *testing.T, catalogResolved *skills.ResolvedSkills) (*runnerTestLLMClient, string) {
	t.Helper()
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := nativeCalculatorResolvedSkills()
	catalog := skills.BuildNativeSkillCatalog(catalogResolved, nil, skills.DefaultNativeSkillCatalogBudgetChars, 0, nil)
	session := skills.NewNativeSkillSession(runtime, resolved, catalog, skills.NativeToolSetOptions{BudgetChars: 10000})
	calculateArguments, _ := json.Marshal(map[string]interface{}{"operation": "multiply", "left": 6, "right": 7})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{
				ID: "calculate-1", Type: "function", Function: adapter.FunctionCall{Name: "calculate", Arguments: string(calculateArguments)},
			}}},
			FinishReason: "tool_calls",
		}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "42"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "42"}, FinishReason: "stop"}}},
	}}
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-direct", "msg-direct", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "And 6 times 7?"}},
	})
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:           prepared,
		Resolved:           resolved,
		NativeAgentLoop:    true,
		NativeSkillSession: session,
		ExecutionContext:   skills.ExecutionContext{OrganizationID: "org-1", UserID: "user-1"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return fakeLLM, answer
}

func toolResultFor(messages []adapter.Message, callID string) string {
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == callID {
			return messageContent(message.Content)
		}
	}
	return ""
}

func TestNativeLoopActivatesACandidateSkillWhoseToolIsCalledDirectly(t *testing.T) {
	fakeLLM, answer := runDirectCalculateCall(t, nativeCalculatorResolvedSkills())
	if answer != "42" || fakeLLM.appChatCalls != 2 {
		t.Fatalf("answer = %q after %d model calls, want 42 after the call and the answer", answer, fakeLLM.appChatCalls)
	}
	second := fakeLLM.appChatRequests[1]
	result := toolResultFor(second.Messages, "calculate-1")
	if !strings.Contains(result, "42") || strings.Contains(result, "skill_not_active") {
		t.Fatalf("calculate result = %s, want 42 without a not-active error", result)
	}
	if !runnerTestHasTool(second.Tools, "calculate") {
		t.Fatalf("second request tools = %#v, want the activated calculate", second.Tools)
	}
	if !runnerTestMessagesContain(second.Messages, "Use the calculator for exact arithmetic.") {
		t.Fatalf("second request messages = %#v, want the calculator instructions", second.Messages)
	}
}

func TestNativeLoopKeepsRejectingAToolWhoseSkillIsNotACandidate(t *testing.T) {
	fakeLLM, _ := runDirectCalculateCall(t, &skills.ResolvedSkills{})
	if fakeLLM.appChatCalls < 2 {
		t.Fatalf("model calls = %d, want the rejected call fed back", fakeLLM.appChatCalls)
	}
	result := toolResultFor(fakeLLM.appChatRequests[1].Messages, "calculate-1")
	if !strings.Contains(result, "skill_not_active") {
		t.Fatalf("calculate result = %s, want skill_not_active", result)
	}
}
