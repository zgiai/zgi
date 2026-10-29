package skillloop

import (
	"context"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
)

func firstNativeRequest(t *testing.T, history []adapter.Message) *adapter.ChatRequest {
	t.Helper()
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "ok"}, FinishReason: "stop"}}},
	}}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := nativeCalculatorResolvedSkills()
	catalog := skills.BuildNativeSkillCatalog(resolved, nil, skills.DefaultNativeSkillCatalogBudgetChars, 0, nil)
	session := skills.NewNativeSkillSession(runtime, resolved, catalog, skills.NativeToolSetOptions{BudgetChars: 10000})
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-prefix", "msg-prefix", "", "auto", &adapter.ChatRequest{Messages: history})
	if _, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                 prepared,
		Resolved:                 resolved,
		NativeSkillSession:       session,
		NativeAgentLoop:          true,
		SystemPromptAdditions:    []adapter.Message{{Role: "system", Content: "Memory rules."}},
		AdditionalSystemMessages: []adapter.Message{{Role: "system", Content: "Page context for this turn."}},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return fakeLLM.appChatRequests[0]
}

func TestNativeLoopKeepsUnchangingGuidanceInTheSystemPrompt(t *testing.T) {
	rules := messageContent(nativeAgentLoopSystemMessage().Content)
	first := firstNativeRequest(t, []adapter.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "hi"},
	})
	prompt := messageContent(first.Messages[0].Content)
	// The calculator is the only candidate, so the directory lists every
	// candidate and is the same in every turn.
	catalog := skills.BuildNativeSkillCatalog(nativeCalculatorResolvedSkills(), nil, skills.DefaultNativeSkillCatalogBudgetChars, 0, nil)
	directory := messageContent(catalog.Message.Content)
	if !catalog.Stable || first.Messages[0].Role != "system" || prompt != "You are a helpful assistant.\n\nMemory rules.\n\n"+directory+"\n\n"+rules {
		t.Fatalf("system prompt = %q, want the persona, the memory rules, the skill directory and the loop rules", prompt)
	}
	later := first.Messages[1:]
	if runnerTestMessagesContain(later, "Memory rules.") || runnerTestMessagesContain(later, rules[:60]) || runnerTestMessagesContain(later, directory[:60]) {
		t.Fatalf("messages after the system prompt repeat its guidance: %#v", later)
	}
	if !runnerTestMessagesContain(later, "Page context for this turn.") {
		t.Fatalf("per-turn guidance left the turn: %#v", later)
	}

	next := firstNativeRequest(t, []adapter.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "and now?"},
	})
	if messageContent(next.Messages[0].Content) != prompt {
		t.Fatal("the system prompt changed between turns")
	}
}

func TestSystemPromptAdditionsBecomeTheSystemPromptWhenThereIsNone(t *testing.T) {
	messages := withSystemPromptAdditions([]adapter.Message{{Role: "user", Content: "hi"}}, []adapter.Message{{Role: "system", Content: "Rules."}})
	if len(messages) != 2 || messages[0].Role != "system" || messages[0].Content != "Rules." {
		t.Fatalf("messages = %#v, want a leading system prompt", messages)
	}
	if got := withSystemPromptAdditions(messages, nil); len(got) != 2 {
		t.Fatalf("no additions changed the messages: %#v", got)
	}
}
