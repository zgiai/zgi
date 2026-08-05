package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
)

func TestRunnerNativeAgentLoopCallsBusinessToolDirectly(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := nativeCalculatorResolvedSkills()
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{BudgetChars: 10000})
	arguments, _ := json.Marshal(map[string]interface{}{"operation": "multiply", "left": 6, "right": 7})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: "I’m calculating the requested values now and will return the exact result once the computation completes.", ToolCalls: []adapter.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: adapter.FunctionCall{
					Name:      "calculate",
					Arguments: string(arguments),
				},
			}, {
				ID:   "call-2",
				Type: "function",
				Function: adapter.FunctionCall{
					Name:      "calculate",
					Arguments: string(arguments),
				},
			}}},
			FinishReason: "tool_calls",
		}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "42"}, FinishReason: "stop"}}},
	}, appChatDelays: []time.Duration{30 * time.Millisecond}}
	events := make([]Event, 0)
	phases := make([]string, 0)
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		ModelProgressSchedule: ModelProgressSchedule{
			Initial:     5 * time.Millisecond,
			Extended:    15 * time.Millisecond,
			LongRunning: time.Second,
		},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
		OnModelInvocation: func(trace ModelInvocationTrace) {
			phases = append(phases, trace.Phase)
		},
	}
	prepared := NewPreparedChat("conv-native", "msg-native", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "What is 6 times 7?"}},
	})

	answer, usage, err := runner.Run(context.Background(), RunRequest{
		Prepared:                   prepared,
		Resolved:                   resolved,
		NativeAgentLoop:            true,
		NativeModelProgressEnabled: true,
		NativeToolSet:              &toolSet,
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: "org-1",
			UserID:         "user-1",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "42" || usage != nil {
		t.Fatalf("Run() = answer %q usage %#v, want 42 and nil usage", answer, usage)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want 2", fakeLLM.appChatCalls)
	}
	first := fakeLLM.appChatRequests[0]
	if !runnerTestMessagesContain(first.Messages, "Every assistant turn that calls any tool must contain tool calls only") {
		t.Fatalf("first request messages = %#v, want native tool-only assistant content policy", first.Messages)
	}
	for _, excluded := range []string{
		skills.MetaToolLoadSkill,
		skills.MetaToolCallSkillTool,
		skills.MetaToolIntermediateAnswer,
		skills.MetaToolFinalAnswer,
	} {
		if runnerTestHasTool(first.Tools, excluded) {
			t.Fatalf("native tools unexpectedly include %s: %#v", excluded, first.Tools)
		}
	}
	for _, required := range []string{"calculate", skills.MetaToolUpdatePlan, skills.MetaToolTurnState, skills.MetaToolRequestUserInput} {
		if !runnerTestHasTool(first.Tools, required) {
			t.Fatalf("native tools = %#v, missing %s", first.Tools, required)
		}
	}
	if len(phases) != 2 || phases[0] != "agent_tool_loop" || phases[1] != "agent_tool_loop" {
		t.Fatalf("phases = %#v, want agent_tool_loop for both calls", phases)
	}
	assertNativeCallEventsWithoutLoadEvents(t, events)
	assertNoNativeActionProgress(t, events)
	assertModelProgressBeforeSkillCall(t, events)
}

func TestNativeReferenceReadContinuationSystemMessage(t *testing.T) {
	message, ok := nativeReferenceReadContinuationSystemMessage(skills.SkillTrace{
		Kind:   "reference_read",
		Status: "success",
	})
	if !ok {
		t.Fatal("successful reference read should produce a continuation instruction")
	}
	content := messageContent(message.Content)
	for _, required := range []string{
		"call the relevant business function now",
		"leave ordinary assistant content empty",
		"complete artifact body only in the function arguments",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("continuation instruction %q missing %q", content, required)
		}
	}

	for _, trace := range []skills.SkillTrace{
		{Kind: "reference_read", Status: "error"},
		{Kind: "tool_call", Status: "success"},
	} {
		if message, ok := nativeReferenceReadContinuationSystemMessage(trace); ok || message.Role != "" {
			t.Fatalf("trace %#v unexpectedly produced continuation instruction %#v", trace, message)
		}
	}
}

func TestRunnerNativeAgentLoopActivatesSkillBeforeDirectBusinessCall(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := nativeCalculatorResolvedSkills()
	catalog := skills.BuildNativeSkillCatalog(resolved, nil, skills.DefaultNativeSkillCatalogBudgetChars, 0, nil)
	session := skills.NewNativeSkillSession(runtime, resolved, catalog, skills.NativeToolSetOptions{BudgetChars: 10000})
	activateArguments, _ := json.Marshal(map[string]interface{}{"skill_ids": []string{skills.SkillCalculator}})
	calculateArguments, _ := json.Marshal(map[string]interface{}{"operation": "multiply", "left": 6, "right": 7})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: "I will load the calculator skill.", ToolCalls: []adapter.ToolCall{{
				ID: "activate-1", Type: "function", Function: adapter.FunctionCall{Name: skills.MetaToolActivateSkills, Arguments: string(activateArguments)},
			}}},
			FinishReason: "tool_calls",
		}}},
		{Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{
				ID: "calculate-1", Type: "function", Function: adapter.FunctionCall{Name: "calculate", Arguments: string(calculateArguments)},
			}}},
			FinishReason: "tool_calls",
		}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "42"}, FinishReason: "stop"}}},
	}}
	events := make([]Event, 0)
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-progressive", "msg-progressive", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "What is 6 times 7?"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:           prepared,
		Resolved:           resolved,
		NativeAgentLoop:    true,
		NativeSkillSession: session,
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: "org-1",
			UserID:         "user-1",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "42" {
		t.Fatalf("Run() answer = %q, want 42", answer)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want activate, business call, final answer", fakeLLM.appChatCalls)
	}
	first := fakeLLM.appChatRequests[0]
	if !runnerTestHasTool(first.Tools, skills.MetaToolActivateSkills) || runnerTestHasTool(first.Tools, "calculate") {
		t.Fatalf("first request tools = %#v, want activate_skills without calculate", first.Tools)
	}
	second := fakeLLM.appChatRequests[1]
	if !runnerTestHasTool(second.Tools, "calculate") {
		t.Fatalf("second request tools = %#v, want activated calculate", second.Tools)
	}
	if !runnerTestMessagesContain(second.Messages, "Use the calculator for exact arithmetic.") {
		t.Fatalf("second request messages = %#v, want complete calculator instructions", second.Messages)
	}
	if runnerTestEventsContainText(events, "load the calculator skill") {
		t.Fatalf("activation narration leaked into events: %#v", events)
	}
	assertNativeCallEventsWithoutLoadEvents(t, events)
	assertNoNativeActionProgress(t, events)
}

func TestRunnerNativeToolNarrationDoesNotBlockBusinessTool(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := nativeCalculatorResolvedSkills()
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{BudgetChars: 10000})
	arguments, _ := json.Marshal(map[string]interface{}{"operation": "multiply", "left": 6, "right": 7})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{
			Message: adapter.Message{
				Role:    "assistant",
				Content: strings.Repeat("This is an intentionally overlong action update. ", 30),
				ToolCalls: []adapter.ToolCall{{
					ID: "call-overlong", Type: "function", Function: adapter.FunctionCall{Name: "calculate", Arguments: string(arguments)},
				}},
			},
			FinishReason: "tool_calls",
		}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "42"}, FinishReason: "stop"}}},
	}}
	events := make([]Event, 0)
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-native-overlong", "msg-native-overlong", "", "auto", &adapter.ChatRequest{
		Model:    "deepseek-chat",
		Messages: []adapter.Message{{Role: "user", Content: "What is 6 times 7?"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:        prepared,
		Resolved:        resolved,
		NativeAgentLoop: true,
		NativeToolSet:   &toolSet,
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: "org-1",
			UserID:         "user-1",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "42" || fakeLLM.appChatCalls != 2 {
		t.Fatalf("Run() = %q with %d model calls, want 42 with two calls", answer, fakeLLM.appChatCalls)
	}
	assertNativeCallEventsWithoutLoadEvents(t, events)
	assertNoNativeActionProgress(t, events)
}

func TestRunnerNativeAgentLoopDoesNotRetryLengthTermination(t *testing.T) {
	runtime := skills.NewRuntime(nil, nil)
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: "prompt-only", RuntimeType: skills.SkillRuntimeTypePrompt},
		Instructions: "Answer concisely.",
	}}}
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{BudgetChars: 10000})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{
			Message:      adapter.Message{Role: "assistant", ReasoningContent: "unfinished reasoning"},
			FinishReason: "length",
		}},
		Usage: &adapter.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
	}}}
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-length", "msg-length", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "Do a long task"}},
	})

	_, usage, err := runner.Run(context.Background(), RunRequest{
		Prepared:        prepared,
		Resolved:        resolved,
		NativeAgentLoop: true,
		NativeToolSet:   &toolSet,
	})
	if !errors.Is(err, ErrAgentOutputTruncated) {
		t.Fatalf("Run() error = %v, want ErrAgentOutputTruncated", err)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want exactly 1", fakeLLM.appChatCalls)
	}
	if usage == nil || usage.TotalTokens != 30 {
		t.Fatalf("usage = %#v, want actual single-call usage", usage)
	}
}

func TestNativeSessionActivationAttemptTraceIsDiagnosticOnly(t *testing.T) {
	trace := nativeSessionActivationAttemptTrace(skills.NativeSkillActivationAttempt{
		SkillID: "file-generator",
		Source:  "runtime_preload",
		Outcome: "activated",
	})
	if trace.Kind != "skill_load_attempt" || trace.Status != "success" {
		t.Fatalf("trace = %#v, want successful diagnostic load attempt", trace)
	}
	if trace.Arguments["source"] != "runtime_preload" {
		t.Fatalf("trace source = %#v, want runtime_preload", trace.Arguments["source"])
	}
}

func TestNativeAgentStreamSuppressesActivationNarration(t *testing.T) {
	index := 0
	client := &runnerTestLLMClient{
		appChatStreams: [][]adapter.StreamResponse{{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{
				Content: "I am loading a skill.",
				ToolCalls: []adapter.ToolCall{{
					Index: &index,
					ID:    "activate-stream",
					Type:  "function",
					Function: adapter.FunctionCall{
						Name:      skills.MetaToolActivateSkills,
						Arguments: `{"skill_ids":["calculator"]}`,
					},
				}},
			}}}},
			{Choices: []adapter.StreamChoice{{FinishReason: "tool_calls"}}, Done: true},
		}}}
	events := make([]Event, 0)
	runner := &Runner{LLMClient: client, OnEvent: func(event Event) error {
		events = append(events, event)
		return nil
	}}
	prepared := NewPreparedChat("conv-native-stream", "msg-native-stream", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})

	result, ok, err := runner.runModelToolRoundStream(context.Background(), prepared, prepared.LLMRequest, 0, nil, false, true, false, "agent_tool_loop", nil)
	if err != nil {
		t.Fatalf("runModelToolRoundStream() error = %v", err)
	}
	if !ok || len(result.message.ToolCalls) != 1 {
		t.Fatalf("stream result = %#v ok=%v, want activation call", result, ok)
	}
	for _, event := range events {
		if event.Type == EventMessage || event.Type == EventMessageRetract {
			t.Fatalf("activation narration leaked as message event: %#v", events)
		}
		if event.Type == EventAgentProgress && event.Payload["meta_tool_name"] == skills.MetaToolActivateSkills {
			t.Fatalf("activation leaked as tool planning progress: %#v", events)
		}
	}
}

func TestNativeAgentStreamKeepsBusinessNarrationHidden(t *testing.T) {
	index := 0
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{
			Content: "I’m calculating the requested values now and will return the exact result.",
			ToolCalls: []adapter.ToolCall{{
				Index: &index,
				ID:    "calculate-stream",
				Type:  "function",
				Function: adapter.FunctionCall{
					Name:      "calculate",
					Arguments: `{"operation":"multiply","left":6,"right":7}`,
				},
			}},
		}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "tool_calls"}}, Done: true},
	}},
		appChatStreamResponseDelays: [][]time.Duration{{15 * time.Millisecond, 0}},
	}
	events := make([]Event, 0)
	runner := &Runner{
		LLMClient: client,
		ModelProgressSchedule: ModelProgressSchedule{
			Initial:     5 * time.Millisecond,
			Extended:    time.Second,
			LongRunning: 2 * time.Second,
		},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-native-business-stream", "msg-native-business-stream", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})
	progress := runner.startModelProgressTracker(context.Background(), prepared, 0, "qwen-plus", nil, true)

	result, ok, err := runner.runModelToolRoundStream(context.Background(), prepared, prepared.LLMRequest, 0, nil, false, true, false, "agent_tool_loop", progress)
	progress.Stop()
	if err != nil {
		t.Fatalf("runModelToolRoundStream() error = %v", err)
	}
	if !ok || len(result.message.ToolCalls) != 1 || strings.TrimSpace(messageContent(result.message.Content)) == "" {
		t.Fatalf("stream result = %#v ok=%v, want retained narration and business call", result, ok)
	}
	for _, event := range events {
		if event.Type == EventMessage || event.Type == EventMessageRetract {
			t.Fatalf("business narration leaked through ordinary message events: %#v", events)
		}
		if event.Type == EventAgentProgress && event.Payload["phase"] == "tool_planning" {
			t.Fatalf("native tool_planning progress was not suppressed: %#v", events)
		}
	}
	assertModelProgressActivity(t, events, modelProgressActivityPreparingAction, modelProgressSourceProviderSignal)
}

func TestNativeAgentStreamStillEmitsOrdinaryFinalContent(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "completed"}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "stop"}}, Done: true},
	}}}
	events := make([]Event, 0)
	runner := &Runner{LLMClient: client, OnEvent: func(event Event) error {
		events = append(events, event)
		return nil
	}}
	prepared := NewPreparedChat("conv-native-final", "msg-native-final", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})

	result, ok, err := runner.runModelToolRoundStream(context.Background(), prepared, prepared.LLMRequest, 0, nil, false, true, false, "agent_tool_loop", nil)
	if err != nil {
		t.Fatalf("runModelToolRoundStream() error = %v", err)
	}
	if !ok || !result.answerStreamed {
		t.Fatalf("stream result = %#v ok=%v, want streamed final content", result, ok)
	}
	if !runnerTestEventsContainText(events, "completed") {
		t.Fatalf("ordinary final content missing from events: %#v", events)
	}
}

func TestRunnerNativeAgentLoopDoesNotFallbackAfterEmptyStream(t *testing.T) {
	runtime := skills.NewRuntime(nil, nil)
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: "prompt-only", RuntimeType: skills.SkillRuntimeTypePrompt},
		Instructions: "Answer concisely.",
	}}}
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{BudgetChars: 10000})
	fakeLLM := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{}}}
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-empty-stream", "msg-empty-stream", "openai", "auto", &adapter.ChatRequest{
		Provider: "openai",
		Messages: []adapter.Message{{Role: "user", Content: "Do a task"}},
	})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:        prepared,
		Resolved:        resolved,
		NativeAgentLoop: true,
		NativeToolSet:   &toolSet,
	})
	if !errors.Is(err, ErrAgentOutputTruncated) {
		t.Fatalf("Run() error = %v, want ErrAgentOutputTruncated", err)
	}
	if fakeLLM.appChatStreamCalls != 1 || fakeLLM.appChatCalls != 0 {
		t.Fatalf("stream calls = %d AppChat calls = %d, want 1 and 0", fakeLLM.appChatStreamCalls, fakeLLM.appChatCalls)
	}
}

func nativeCalculatorResolvedSkills() *skills.ResolvedSkills {
	return &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{
			ID:          skills.SkillCalculator,
			Description: "Perform exact arithmetic.",
			RuntimeType: skills.SkillRuntimeTypeTool,
		},
		Instructions: "Use the calculator for exact arithmetic.",
		Tools: []skills.SkillToolDefinition{{
			Name:         "calculate",
			ProviderType: tools.ToolProviderTypeBuiltin,
			ProviderID:   "calculator",
		}},
	}}}
}

func assertNativeCallEventsWithoutLoadEvents(t *testing.T, events []Event) {
	t.Helper()
	want := map[string]bool{
		EventSkillCallStart: false,
		EventSkillCallEnd:   false,
	}
	for _, event := range events {
		if event.Type == EventSkillLoadStart || event.Type == EventSkillLoadEnd {
			t.Fatalf("events = %#v, native loop must not expose routine skill load events", events)
		}
		if _, ok := want[event.Type]; !ok {
			continue
		}
		want[event.Type] = true
	}
	for eventType, found := range want {
		if !found {
			t.Fatalf("events = %#v, missing %s", events, eventType)
		}
	}
}

func assertNoNativeActionProgress(t *testing.T, events []Event) {
	t.Helper()
	for _, event := range events {
		if event.Type == EventAgentProgress && event.Payload["phase"] == "action_progress" {
			t.Fatalf("native action progress was not removed: %#v", events)
		}
	}
}

func assertModelProgressBeforeSkillCall(t *testing.T, events []Event) {
	t.Helper()
	progressIndex := -1
	skillCallIndex := -1
	for index, event := range events {
		if event.Type == EventAgentProgress && event.Payload["phase"] == modelProgressPhase && progressIndex < 0 {
			progressIndex = index
		}
		if event.Type == EventSkillCallStart && skillCallIndex < 0 {
			skillCallIndex = index
		}
	}
	if progressIndex < 0 || skillCallIndex < 0 || progressIndex >= skillCallIndex {
		t.Fatalf("events = %#v, want model progress before skill_call_start", events)
	}
}

func runnerTestMessagesContain(messages []adapter.Message, target string) bool {
	for _, message := range messages {
		content, _ := message.Content.(string)
		if strings.Contains(content, target) {
			return true
		}
	}
	return false
}

func runnerTestEventsContainText(events []Event, target string) bool {
	for _, event := range events {
		encoded, _ := json.Marshal(event.Payload)
		if strings.Contains(string(encoded), target) {
			return true
		}
	}
	return false
}
