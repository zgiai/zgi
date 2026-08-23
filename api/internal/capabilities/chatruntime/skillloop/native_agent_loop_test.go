package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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
	if answer != "42" || usage != nil {
		t.Fatalf("Run() = answer %q usage %#v, want 42 and nil usage", answer, usage)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want 2", fakeLLM.appChatCalls)
	}
	first := fakeLLM.appChatRequests[0]
	if !runnerTestMessagesContain(first.Messages, "before the first business stage include one brief user-visible process note") {
		t.Fatalf("first request messages = %#v, want native semantic process-note policy", first.Messages)
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
	assertNativeProcessCommentaryBeforeSkillCall(t, events)
}

func TestRunnerNativeStreamConvertsProcessCommentaryBeforeBusinessTool(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := nativeCalculatorResolvedSkills()
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{BudgetChars: 10000})
	index := 0
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{
		{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{ReasoningContent: "private reasoning"}}}},
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "The relevant values are available; "}}}},
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "the next step will produce the exact result."}}}},
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{ToolCalls: []adapter.ToolCall{{
				Index: &index,
				ID:    "business-stream-1",
				Type:  "function",
				Function: adapter.FunctionCall{
					Name:      "calculate",
					Arguments: `{"operation":"multiply","left":6,"right":7}`,
				},
			}}}}}},
			{Choices: []adapter.StreamChoice{{FinishReason: "tool_calls"}}, Done: true},
		},
		{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "4"}}}},
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "2"}}}},
			{Choices: []adapter.StreamChoice{{FinishReason: "stop"}}, Done: true},
		},
	}}
	events := make([]Event, 0)
	runner := &Runner{
		LLMClient:    client,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-native-stream-run", "msg-native-stream-run", "qwen", "auto", &adapter.ChatRequest{
		Model:    "qwen-plus",
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
	if answer != "42" || client.appChatStreamCalls != 2 || client.appChatCalls != 0 {
		t.Fatalf("Run() = %q with stream=%d nonstream=%d, want 42 with two stream calls", answer, client.appChatStreamCalls, client.appChatCalls)
	}
	assertNativeProcessCommentaryBeforeSkillCall(t, events)
	for _, event := range events {
		if event.Type == EventMessage && strings.Contains(stringFromInterface(event.Payload["answer"]), "private reasoning") {
			t.Fatalf("reasoning leaked into message events: %#v", events)
		}
	}
	if len(client.appChatStreamRequests) < 2 {
		t.Fatalf("stream requests = %d, want follow-up request", len(client.appChatStreamRequests))
	}
	toolTurnFound := false
	for _, message := range client.appChatStreamRequests[1].Messages {
		if len(message.ToolCalls) == 0 {
			continue
		}
		toolTurnFound = true
		if strings.TrimSpace(messageContent(message.Content)) != "" || strings.TrimSpace(message.ReasoningContent) != "" {
			t.Fatalf("tool turn carried presentation-only content into next request: %#v", message)
		}
	}
	if !toolTurnFound {
		t.Fatalf("follow-up messages = %#v, want prior tool turn", client.appChatStreamRequests[1].Messages)
	}
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
		"brief process note is allowed",
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
			Message: adapter.Message{Role: "assistant", Content: "I will prepare the exact calculation first, then return the verified result.", ToolCalls: []adapter.ToolCall{{
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
	assertNativeProcessCommentaryBeforeSkillCall(t, events)
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
	assertNativeDiscardedCommentary(t, events, "intentionally overlong action update")
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

func TestNativeAgentStreamKeepsActivationNarrationProvisionalForRunnerClassification(t *testing.T) {
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
	messageCount := 0
	for _, event := range events {
		if event.Type == EventMessage {
			messageCount++
		}
		if event.Type == EventMessageRetract {
			t.Fatalf("stream layer classified activation narration before the complete tool turn: %#v", events)
		}
		if event.Type == EventAgentProgress && event.Payload["meta_tool_name"] == skills.MetaToolActivateSkills {
			t.Fatalf("activation leaked as tool planning progress: %#v", events)
		}
	}
	if messageCount != 1 || !result.provisionalStreamed || result.provisionalContent != "I am loading a skill." {
		t.Fatalf("provisional activation narration = %#v, want one provisional candidate", result)
	}
}

func TestNativeAgentStreamExposesBusinessNarrationProvisionally(t *testing.T) {
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
	progress := runner.startModelProgressTracker(context.Background(), prepared, 0, "qwen-plus", nil)

	result, ok, err := runner.runModelToolRoundStream(context.Background(), prepared, prepared.LLMRequest, 0, nil, false, true, false, "agent_tool_loop", progress)
	progress.Stop()
	if err != nil {
		t.Fatalf("runModelToolRoundStream() error = %v", err)
	}
	if !ok || len(result.message.ToolCalls) != 1 || strings.TrimSpace(messageContent(result.message.Content)) == "" {
		t.Fatalf("stream result = %#v ok=%v, want retained narration and business call", result, ok)
	}
	messageCount := 0
	messageSeen := false
	progressBeforeMessage := false
	for _, event := range events {
		if event.Type == EventMessage {
			messageCount++
			messageSeen = true
		}
		if event.Type == EventMessageRetract {
			t.Fatalf("stream layer classified business narration before the complete tool turn: %#v", events)
		}
		if event.Type == EventAgentProgress && event.Payload["phase"] == "tool_planning" {
			t.Fatalf("native tool_planning progress was not suppressed: %#v", events)
		}
		if event.Type == EventAgentProgress && event.Payload["phase"] == modelProgressPhase {
			if messageSeen {
				t.Fatalf("model progress resumed after visible narration: %#v", events)
			}
			progressBeforeMessage = true
		}
	}
	if messageCount != 1 || !progressBeforeMessage || !result.provisionalStreamed {
		t.Fatalf("provisional business narration = %#v progress_before_message=%v, want one provisional candidate after progress", result, progressBeforeMessage)
	}
}

func TestNativeAgentStreamStillEmitsOrdinaryFinalContent(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "com"}}}},
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "pleted"}}}},
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
	chunks := make([]string, 0)
	for _, event := range events {
		if event.Type == EventMessage {
			chunks = append(chunks, stringFromInterface(event.Payload["answer"]))
		}
	}
	if got, want := chunks, []string{"com", "pleted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("streamed answer chunks = %#v, want %#v", got, want)
	}
	if got := messageContent(result.message.Content); got != "completed" {
		t.Fatalf("completed message content = %q, want completed", got)
	}
}

func TestNativeAgentStreamDefersCandidateDispositionUntilToolCallsAreComplete(t *testing.T) {
	index := 0
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "I will check the current values."}}}},
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{ToolCalls: []adapter.ToolCall{{
			Index: &index,
			ID:    "calculate-after-content",
			Type:  "function",
			Function: adapter.FunctionCall{
				Name:      "calculate",
				Arguments: `{"operation":"multiply","left":6,"right":7}`,
			},
		}}}}}},
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "This tool-turn text stays hidden."}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "tool_calls"}}, Done: true},
	}}}
	events := make([]Event, 0)
	runner := &Runner{LLMClient: client, OnEvent: func(event Event) error {
		events = append(events, event)
		return nil
	}}
	prepared := NewPreparedChat("conv-native-retract", "msg-native-retract", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})

	result, ok, err := runner.runModelToolRoundStream(context.Background(), prepared, prepared.LLMRequest, 0, nil, false, true, false, "agent_tool_loop", nil)
	if err != nil {
		t.Fatalf("runModelToolRoundStream() error = %v", err)
	}
	if !ok || len(result.message.ToolCalls) != 1 {
		t.Fatalf("stream result = %#v ok=%v, want one tool call", result, ok)
	}
	if result.answerStreamed {
		t.Fatal("retracted candidate answer was marked as a final streamed answer")
	}

	answer := ""
	for _, event := range events {
		switch event.Type {
		case EventMessage:
			answer += stringFromInterface(event.Payload["answer"])
		case EventMessageRetract:
			t.Fatalf("candidate was retracted before runner classification: %#v", events)
		}
	}
	const candidate = "I will check the current values."
	if answer != candidate {
		t.Fatalf("streamed candidate answer = %q, want %q", answer, candidate)
	}
	if !result.provisionalStreamed || result.provisionalContent != candidate {
		t.Fatalf("provisional result = %#v, want %q", result, candidate)
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

func assertNativeProcessCommentaryBeforeSkillCall(t *testing.T, events []Event) {
	t.Helper()
	messageIndex := -1
	retractIndex := -1
	skillCallIndex := -1
	processRetracts := 0
	for index, event := range events {
		switch event.Type {
		case EventMessage:
			if messageIndex < 0 {
				messageIndex = index
			}
		case EventMessageRetract:
			if stringFromInterface(event.Payload["presentation_disposition"]) == nativeCommentaryDispositionProcess {
				processRetracts++
				if retractIndex < 0 {
					retractIndex = index
				}
			}
		case EventSkillCallStart:
			if skillCallIndex < 0 {
				skillCallIndex = index
			}
		}
	}
	if processRetracts != 1 || messageIndex < 0 || retractIndex <= messageIndex || skillCallIndex <= retractIndex {
		t.Fatalf("events = %#v, want one message -> process retract -> skill call sequence", events)
	}
}

func assertNativeDiscardedCommentary(t *testing.T, events []Event, text string) {
	t.Helper()
	messageFound := false
	discardFound := false
	processFound := false
	for _, event := range events {
		switch event.Type {
		case EventMessage:
			if strings.Contains(stringFromInterface(event.Payload["answer"]), text) {
				messageFound = true
			}
		case EventMessageRetract:
			if !strings.Contains(stringFromInterface(event.Payload["content"]), text) {
				continue
			}
			switch stringFromInterface(event.Payload["presentation_disposition"]) {
			case nativeCommentaryDispositionDiscard:
				discardFound = true
			case nativeCommentaryDispositionProcess:
				processFound = true
			}
		}
	}
	if !messageFound || !discardFound || processFound {
		t.Fatalf("events = %#v, want provisional text retracted with discard only", events)
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

func TestNativeAgentToolsForRunIncludesRuntimeToolWithoutSkills(t *testing.T) {
	runtimeTool := RuntimeTool{
		Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: "mutate_agent_memory"}},
		Handler: func(context.Context, adapter.ToolCall) RuntimeToolResult {
			return RuntimeToolResult{Status: "success", Result: map[string]interface{}{"status": "success"}}
		},
	}
	tools := nativeAgentToolsForRun(&skills.ResolvedSkills{}, nil, nil, []RuntimeTool{runtimeTool})
	if !runnerTestHasTool(tools, "mutate_agent_memory") {
		t.Fatalf("tools = %#v, want runtime memory tool", tools)
	}
	step := handleRuntimeToolCall(context.Background(), adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Name: "mutate_agent_memory"}}, runtimeTool)
	if step.trace.Status != "success" || !step.usedTool || step.recoverable {
		t.Fatalf("runtime step = %#v", step)
	}
}

func TestNativeExecutionCallWrapsActionArgumentsUnderFixedIdentity(t *testing.T) {
	toolSet := &skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{
		"wecom_send_message": {
			SkillID:          skills.SkillExternalApps,
			ToolName:         "execute_action",
			ArgumentEnvelope: "arguments",
			FixedArguments: map[string]interface{}{
				"integration_id":         "wecom",
				"action_id":              "wecom.message.send",
				"action_schema_hash":     "fixed-hash",
				"action_schema_revision": "fixed-schema",
				"catalog_revision":       "fixed-catalog",
			},
		},
	}}
	call := nativeExecutionCall(adapter.ToolCall{
		ID: "call-action",
		Function: adapter.FunctionCall{
			Name: "wecom_send_message",
			Arguments: `{"content":"hello","integration_id":"attacker","action_id":"attacker.action",` +
				`"action_schema_hash":"attacker-hash","catalog_revision":"attacker-catalog"}`,
		},
	}, toolSet)
	if call.Function.Name != skills.MetaToolCallSkillTool {
		t.Fatalf("nativeExecutionCall() function = %q", call.Function.Name)
	}
	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &wrapper); err != nil {
		t.Fatalf("wrapped arguments are invalid JSON: %v", err)
	}
	if wrapper["skill_id"] != skills.SkillExternalApps || wrapper["tool_name"] != "execute_action" {
		t.Fatalf("wrapper target = %#v", wrapper)
	}
	execution, _ := wrapper["arguments"].(map[string]interface{})
	for key, want := range map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.message.send",
		"action_schema_hash": "fixed-hash", "action_schema_revision": "fixed-schema", "catalog_revision": "fixed-catalog",
	} {
		if got := execution[key]; got != want {
			t.Fatalf("fixed execution identity %s = %#v, want %#v", key, got, want)
		}
	}
	business, _ := execution["arguments"].(map[string]interface{})
	if business["content"] != "hello" || business["integration_id"] != "attacker" || business["action_id"] != "attacker.action" {
		t.Fatalf("business arguments = %#v", business)
	}
	if _, exists := execution["content"]; exists {
		t.Fatalf("business arguments escaped their envelope: %#v", execution)
	}
}

func TestRunnerRetriesPendingExternalActionAfterDifferentActionSucceeded(t *testing.T) {
	toolCall := func(callID string, actionID string) adapter.ToolCall {
		arguments, err := json.Marshal(map[string]interface{}{
			"integration_id": "wecom",
			"action_id":      actionID,
		})
		if err != nil {
			t.Fatalf("marshal tool arguments: %v", err)
		}
		return adapter.ToolCall{
			ID:   callID,
			Type: "function",
			Function: adapter.FunctionCall{
				Name:      "execute_action",
				Arguments: string(arguments),
			},
		}
	}

	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{
			Role: "assistant", ToolCalls: []adapter.ToolCall{toolCall("execute-search", "wecom.contact.search")},
		}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "查询和发送都已完成。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{
			Role: "assistant", ToolCalls: []adapter.ToolCall{toolCall("execute-send", "wecom.message.send")},
		}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}

	executedActions := make([]string, 0, 2)
	runtimeTool := RuntimeTool{
		Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: "execute_action"}},
		SkillID:    skills.SkillExternalApps,
		Handler: func(_ context.Context, call adapter.ToolCall) RuntimeToolResult {
			arguments := map[string]interface{}{}
			if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
				t.Fatalf("decode runtime tool arguments: %v", err)
			}
			actionID, _ := arguments["action_id"].(string)
			executedActions = append(executedActions, actionID)
			return RuntimeToolResult{
				Status: "success",
				Arguments: map[string]interface{}{
					"integration_id": "wecom",
					"action_id":      actionID,
				},
				Result: map[string]interface{}{
					"integration_id":   "wecom",
					"action_id":        actionID,
					"operation_status": "completed",
				},
			}
		},
	}

	runner := &Runner{LLMClient: fakeLLM, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-external-sequence", "msg-external-sequence", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "查询企业微信成员并给他发送消息"}},
	})
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:        prepared,
		Resolved:        &skills.ResolvedSkills{},
		NativeAgentLoop: true,
		RuntimeTools:    []RuntimeTool{runtimeTool},
		RuntimeStateSnapshot: func() map[string]interface{} {
			invocations := []interface{}{map[string]interface{}{
				"invocation_id": "guide-search", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "effect": "read", "can_execute": true,
				},
			}}
			if len(executedActions) > 0 {
				invocations = append(invocations, map[string]interface{}{
					"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
					"result": map[string]interface{}{
						"integration_id": "wecom", "action_id": "wecom.message.send", "effect": "external_send", "can_execute": true,
					},
				})
			}
			return map[string]interface{}{
				"latest_user_request": "查询企业微信成员并给他发送消息",
				"skill_invocations":   invocations,
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "消息已发送。" {
		t.Fatalf("answer = %q", answer)
	}
	if !reflect.DeepEqual(executedActions, []string{"wecom.contact.search", "wecom.message.send"}) {
		t.Fatalf("executed actions = %#v, want search then send exactly once", executedActions)
	}
	if fakeLLM.appChatCalls != 4 {
		t.Fatalf("AppChat calls = %d, want query, rejected final, corrected send, final", fakeLLM.appChatCalls)
	}
	if !runnerTestRequestContains(fakeLLM.appChatRequests[2], `action_id="wecom.message.send"`) {
		t.Fatal("external-action correction did not target the latest pending send action")
	}
}

func TestRunnerAcceptsTruthfulNonExecutionAfterEmptyExternalQuery(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "查询和发送都已完成。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "没有找到目标成员，因此未发送消息。"}, FinishReason: "stop"}}},
	}}
	runtimeState := map[string]interface{}{
		"latest_user_request": "查询企业微信成员并给他发送消息",
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search", "can_execute": true,
				},
			},
			map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "status": "success",
				"arguments": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.contact.search",
				},
				"result": map[string]interface{}{"operation_status": "completed", "result_count": 0},
			},
			map[string]interface{}{
				"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
				"result": map[string]interface{}{
					"integration_id": "wecom", "action_id": "wecom.message.send", "can_execute": true,
				},
			},
		},
	}

	runner := &Runner{LLMClient: fakeLLM, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-external-empty", "msg-external-empty", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "查询企业微信成员并给他发送消息"}},
	})
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:        prepared,
		Resolved:        &skills.ResolvedSkills{},
		NativeAgentLoop: true,
		RuntimeTools:    []RuntimeTool{runnerTestUnusedExternalActionTool(t)},
		RuntimeStateSnapshot: func() map[string]interface{} {
			return runtimeState
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "没有找到目标成员，因此未发送消息。" {
		t.Fatalf("answer = %q", answer)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want one correction and a truthful terminal answer", fakeLLM.appChatCalls)
	}
}

func TestRunnerReturnsSafeTerminalAnswerWhenExternalRetryIsExhausted(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已经发送。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息确实已经发送。"}, FinishReason: "stop"}}},
	}}
	runtimeState := map[string]interface{}{
		"latest_user_request": "发送企业微信消息",
		"skill_invocations": []interface{}{map[string]interface{}{
			"invocation_id": "guide-send", "skill_id": "external-apps", "tool_name": "get_action_guide", "status": "success",
			"result": map[string]interface{}{
				"integration_id": "wecom", "action_id": "wecom.message.send", "can_execute": true,
			},
		}},
	}

	runner := &Runner{LLMClient: fakeLLM, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-external-exhausted", "msg-external-exhausted", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "发送企业微信消息"}},
	})
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:        prepared,
		Resolved:        &skills.ResolvedSkills{},
		NativeAgentLoop: true,
		RuntimeTools:    []RuntimeTool{runnerTestUnusedExternalActionTool(t)},
		RuntimeStateSnapshot: func() map[string]interface{} {
			return runtimeState
		},
	})
	if err != nil {
		t.Fatalf("Run() exposed terminal guard error after retry exhaustion: %v", err)
	}
	if !strings.Contains(answer, "外部操作未完成") || !strings.Contains(answer, "不能视为已发送或已完成") {
		t.Fatalf("answer = %q, want safe non-execution terminal answer", answer)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want exactly one retry for the pending guide", fakeLLM.appChatCalls)
	}
}

func runnerTestUnusedExternalActionTool(t *testing.T) RuntimeTool {
	t.Helper()
	return RuntimeTool{
		Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: "execute_action"}},
		SkillID:    skills.SkillExternalApps,
		Handler: func(context.Context, adapter.ToolCall) RuntimeToolResult {
			t.Fatal("unexpected external action execution")
			return RuntimeToolResult{}
		},
	}
}
