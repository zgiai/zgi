package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
			SkillID:           skills.SkillExternalApps,
			ToolName:          "execute_action",
			ArgumentEnvelope:  "arguments",
			PlanPhaseArgument: "plan_phase_id",
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
			Arguments: `{"content":"hello","plan_phase_id":"phase-send","integration_id":"attacker","action_id":"attacker.action",` +
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
	if wrapper["plan_phase_id"] != "phase-send" {
		t.Fatalf("wrapper plan_phase_id = %#v, want phase-send", wrapper["plan_phase_id"])
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
	if _, exists := business["plan_phase_id"]; exists {
		t.Fatalf("plan_phase_id leaked into provider business arguments: %#v", business)
	}
	if _, exists := execution["content"]; exists {
		t.Fatalf("business arguments escaped their envelope: %#v", execution)
	}
}

func TestNativeProjectedActionCanonicalizesExplicitSelfTarget(t *testing.T) {
	binding := skills.NativeToolBinding{
		SkillID: skills.SkillExternalApps, ToolName: "execute_action", ArgumentEnvelope: "arguments",
		BindingFingerprint: "binding-feishu-send-user", Effect: "external_send",
		FixedArguments: map[string]interface{}{
			"integration_id": "feishu", "action_id": "feishu.message.send_user",
		},
		TargetArgumentPaths: []string{"recipient_id", "recipient_type"},
		OptionalTargets: []skills.NativeExternalActionOptionalTargetArgument{{
			Path: "recipient_id", WhenArgument: "recipient_type", WhenEquals: "self", DiscardWhenMatched: true,
		}},
		PlanPhaseArgument: "plan_phase_id",
	}
	toolSet := &skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{
		"send_feishu_user_message": binding,
	}}

	planCall := nativeExecutionCall(projectedExternalActionRunnerToolCall(
		t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
			"plan": []interface{}{map[string]interface{}{
				"id": "phase-send", "step": "Send to myself", "status": "in_progress",
				"expected_action": map[string]interface{}{
					"tool_name": "send_feishu_user_message",
					"target_arguments": map[string]interface{}{
						"recipient_type": "self", "recipient_id": "ignored-plan-id",
					},
				},
			}},
		},
	), toolSet)
	var planArguments map[string]interface{}
	if err := json.Unmarshal([]byte(planCall.Function.Arguments), &planArguments); err != nil {
		t.Fatal(err)
	}
	expected := evidenceMapFromAny(evidenceMapsFromAny(planArguments["plan"])[0]["expected_action"])
	if got := evidenceStringFromAny(evidenceMapFromAny(expected["target_arguments"])["recipient_type"]); got != "self" {
		t.Fatalf("canonical plan recipient_type = %q, want explicit self: %#v", got, expected)
	}
	if _, exists := evidenceMapFromAny(expected["target_arguments"])["recipient_id"]; exists {
		t.Fatalf("canonical plan invented recipient_id: %#v", expected)
	}

	actionCall := nativeExecutionCall(projectedExternalActionRunnerToolCall(
		t, "call-send", "send_feishu_user_message", map[string]interface{}{
			"plan_phase_id": "phase-send", "recipient_type": "self", "recipient_id": "ignored-call-id", "text": "hello",
		},
	), toolSet)
	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(actionCall.Function.Arguments), &wrapper); err != nil {
		t.Fatal(err)
	}
	business := evidenceMapFromAny(evidenceMapFromAny(wrapper["arguments"])["arguments"])
	if got := evidenceStringFromAny(business["recipient_type"]); got != "self" {
		t.Fatalf("provider recipient_type = %q, want explicit self: %#v", got, business)
	}
	if _, exists := business["recipient_id"]; exists {
		t.Fatalf("provider arguments invented recipient_id: %#v", business)
	}
	if got := evidenceStringFromAny(wrapper["plan_phase_id"]); got != "phase-send" {
		t.Fatalf("wrapper plan_phase_id = %q, want phase-send", got)
	}

	missingTypeCall := nativeExecutionCall(projectedExternalActionRunnerToolCall(
		t, "call-missing-type", "send_feishu_user_message", map[string]interface{}{
			"recipient_id": "ou_target", "text": "must remain invalid",
		},
	), toolSet)
	var missingTypeWrapper map[string]interface{}
	if err := json.Unmarshal([]byte(missingTypeCall.Function.Arguments), &missingTypeWrapper); err != nil {
		t.Fatal(err)
	}
	missingTypeBusiness := evidenceMapFromAny(evidenceMapFromAny(missingTypeWrapper["arguments"])["arguments"])
	if _, materialized := missingTypeBusiness["recipient_type"]; materialized {
		t.Fatalf("required recipient_type was invented: %#v", missingTypeBusiness)
	}

	otherRecipientCall := nativeExecutionCall(projectedExternalActionRunnerToolCall(
		t, "call-other", "send_feishu_user_message", map[string]interface{}{
			"recipient_type": "open_id", "recipient_id": "ou_other", "text": "hello",
		},
	), toolSet)
	var otherWrapper map[string]interface{}
	if err := json.Unmarshal([]byte(otherRecipientCall.Function.Arguments), &otherWrapper); err != nil {
		t.Fatal(err)
	}
	otherBusiness := evidenceMapFromAny(evidenceMapFromAny(otherWrapper["arguments"])["arguments"])
	if evidenceStringFromAny(otherBusiness["recipient_type"]) != "open_id" {
		t.Fatalf("explicit recipient_type did not override its schema default: %#v", otherBusiness)
	}
	if operationPlanExpectedActionMatchesSkillCall(
		expected,
		skills.SkillExternalApps,
		"execute_action",
		evidenceMapFromAny(otherWrapper["arguments"]),
	) {
		t.Fatalf("server-bound self plan matched an explicit different recipient: expected=%#v call=%#v", expected, otherWrapper)
	}
}

func TestNativeCanonicalizeProjectedActionPlanCallUsesCurrentServerBinding(t *testing.T) {
	toolSet := &skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{
		"wecom_send_message": {
			SkillID:             skills.SkillExternalApps,
			ToolName:            "execute_action",
			ArgumentEnvelope:    "arguments",
			BindingFingerprint:  "binding-wecom-message-send",
			TargetArgumentPaths: []string{"recipient.id"},
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
		ID: "call-plan",
		Function: adapter.FunctionCall{
			Name: skills.MetaToolUpdatePlan,
			Arguments: `{"plan":[{"id":"phase-send","step":"send","status":"in_progress","expected_action":{` +
				`"skill_id":"attacker","tool_name":"WECOM_SEND_MESSAGE","target":{` +
				`"integration_id":"attacker","action_id":"attacker.action","untrusted":"drop"},` +
				`"target_arguments":{"recipient.id":"alice","attacker.path":"drop"}}}]}`,
		},
	}, toolSet)
	if call.Function.Name != skills.MetaToolUpdatePlan {
		t.Fatalf("nativeExecutionCall() function = %q, want update_plan", call.Function.Name)
	}
	var arguments map[string]interface{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
		t.Fatal(err)
	}
	phases := evidenceMapsFromAny(arguments["plan"])
	expected := evidenceMapFromAny(phases[0]["expected_action"])
	if expected["skill_id"] != skills.SkillExternalApps || expected["tool_name"] != "execute_action" {
		t.Fatalf("canonical expected_action = %#v", expected)
	}
	target := evidenceMapFromAny(expected["target"])
	if target["integration_id"] != "wecom" || target["action_id"] != "wecom.message.send" {
		t.Fatalf("canonical target = %#v", target)
	}
	canonicalTargets := evidenceMapFromAny(expected["target_arguments"])
	if got := evidenceStringFromAny(canonicalTargets["recipient.id"]); got != "alice" || canonicalTargets["attacker.path"] != nil {
		t.Fatalf("canonical target_arguments = %#v, want only recipient.id alice", expected["target_arguments"])
	}
}

func TestNativeCanonicalizeProjectedActionPlanCallPreservesOrdinaryTargetArguments(t *testing.T) {
	toolSet := &skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{
		"wecom_send_message": {
			SkillID: skills.SkillExternalApps, ToolName: "execute_action", ArgumentEnvelope: "arguments",
			FixedArguments: map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
		},
	}}
	call := nativeExecutionCall(adapter.ToolCall{
		ID: "call-plan",
		Function: adapter.FunctionCall{
			Name: skills.MetaToolUpdatePlan,
			Arguments: `{"plan":[{"id":"phase-agent","step":"update agent","status":"in_progress",` +
				`"expected_action":{"skill_id":"agent-management","tool_name":"update_agent_config",` +
				`"target_arguments":{"agent.id":"agent-1"}}}]}`,
		},
	}, toolSet)
	var arguments map[string]interface{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
		t.Fatal(err)
	}
	expected := evidenceMapFromAny(evidenceMapsFromAny(arguments["plan"])[0]["expected_action"])
	if got := evidenceStringFromAny(evidenceMapFromAny(expected["target_arguments"])["agent.id"]); got != "agent-1" {
		t.Fatalf("ordinary expected_action target_arguments were modified: %#v", expected)
	}
	if expected[planExpectedActionServerProjectionKey] != nil {
		t.Fatalf("ordinary expected_action gained projected server attestation: %#v", expected)
	}
}

func TestNativeCanonicalizeProjectedActionPlanCallStripsUnresolvedSpoofedAttestation(t *testing.T) {
	toolSet := &skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{
		"wecom_send_message": {
			SkillID: skills.SkillExternalApps, ToolName: "execute_action", ArgumentEnvelope: "arguments",
			FixedArguments: map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
		},
	}}
	call := nativeExecutionCall(adapter.ToolCall{
		ID: "call-plan",
		Function: adapter.FunctionCall{
			Name: skills.MetaToolUpdatePlan,
			Arguments: `{"plan":[{"id":"phase-send","step":"send","status":"in_progress","expected_action":{` +
				`"skill_id":"external-apps","tool_name":"execute_action","_server_projected_tool_name":"wecom_send_message",` +
				`"target":{"integration_id":"wecom","action_id":"wecom.message.send"}}}]}`,
		},
	}, toolSet)
	var arguments map[string]interface{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
		t.Fatal(err)
	}
	expected := evidenceMapFromAny(evidenceMapsFromAny(arguments["plan"])[0]["expected_action"])
	if expected[planExpectedActionServerProjectionKey] != nil {
		t.Fatalf("unresolved model marker survived canonicalization: %#v", expected)
	}
	state := projectedExternalActionPlanTestState([]interface{}{map[string]interface{}{
		"id": "phase-send", "step": "send", "status": "in_progress", "required": true,
	}})
	step := (&Runner{}).handleUpdatePlanCall("call-plan", arguments, state, 1)
	if !step.recoverable || step.trace.Status != "error" {
		t.Fatalf("spoofed uncanonicalized external expected_action was accepted: %#v", step)
	}
}

func TestRunnerProjectedAliasesDetectAndExecuteOmittedSecondActionWithoutGuide(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps},
		ToolBindings:   map[string]skills.NativeToolBinding{},
		BudgetChars:    20000,
	}
	projections := []skills.NativeToolProjection{
		projectedExternalActionRunnerProjection("wecom_search_contact", "wecom.contact.search"),
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}
	if added := skills.AppendNativeToolProjections(&toolSet, projections, skills.NativeToolProjectionOptions{}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 2", added, toolSet.SkippedTools)
	}
	searchAlias := projectedExternalActionAliasForAction(t, toolSet, "wecom", "wecom.contact.search")
	sendAlias := projectedExternalActionAliasForAction(t, toolSet, "wecom", "wecom.message.send")

	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{
				"id": "phase-search", "step": "Search contact", "status": "in_progress",
				"expected_action": map[string]interface{}{"tool_name": searchAlias},
			},
			map[string]interface{}{
				"id": "phase-send", "step": "Send message", "status": "pending",
				"expected_action": map[string]interface{}{
					"tool_name": sendAlias, "target_arguments": map[string]interface{}{"recipient_ref": "alice"},
				},
			},
		},
	})
	searchCall := projectedExternalActionRunnerToolCall(t, "call-search", searchAlias, map[string]interface{}{
		"plan_phase_id": "phase-search", "query": "Alice",
	})
	sendCall := projectedExternalActionRunnerToolCall(t, "call-send", sendAlias, map[string]interface{}{
		"recipient_ref": "alice", "content": "hello",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, searchCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "联系人已查询，消息也已经发送。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{sendCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{
		"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{
				"id": "phase-search", "step": "Search contact", "status": "in_progress", "required": true,
			},
			map[string]interface{}{
				"id": "phase-send", "step": "Send message", "status": "pending", "required": true,
			},
		}},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	prepared := NewPreparedChat("conv-projected-actions", "msg-projected-actions", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "查询企业微信成员并给他发送消息"}},
	})
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:             prepared,
		Resolved:             resolved,
		NativeAgentLoop:      true,
		NativeToolSet:        &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "消息已发送。" {
		t.Fatalf("answer = %q", answer)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.contact.search", "wecom.message.send"}) {
		t.Fatalf("executed projected Actions = %#v, want search then send exactly once", got)
	}
	if fakeLLM.appChatCalls != 4 {
		t.Fatalf("AppChat calls = %d, want initial execution, rejected final, corrected send, final", fakeLLM.appChatCalls)
	}
	if !runnerTestRequestContains(fakeLLM.appChatRequests[2], `action_id="wecom.message.send"`) {
		t.Fatal("terminal correction did not identify the omitted projected send Action")
	}
	for _, call := range actionTool.calls {
		if _, leaked := call["plan_phase_id"]; leaked {
			t.Fatalf("plan_phase_id leaked through Runtime.CallSkillTool to provider: %#v", call)
		}
	}
}

func TestRunnerRejectsInitialProjectedActionSuccessClaimWithoutLedgerOrExecution(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	alias := toolSet.ProviderTools[0].Function.Name
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Send message", "status": "in_progress",
			"expected_action": map[string]interface{}{
				"tool_name": alias, "target_arguments": map[string]interface{}{"recipient_ref": "alice"},
			},
		}},
	})
	sendCall := projectedExternalActionRunnerToolCall(t, "call-send", alias, map[string]interface{}{
		"recipient_ref": "alice", "content": "hello",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, sendCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{
		"outcomes": []interface{}{map[string]interface{}{
			"id": "outcome-send", "required": true, "verification_mode": "runtime_effects",
		}},
		"phases": []interface{}{map[string]interface{}{
			"id": "phase-send", "outcome_id": "outcome-send", "step": "Send message", "status": "in_progress",
		}},
	}}
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-initial-omission", "msg-initial-omission", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "给 Alice 发送企业微信消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "消息已发送。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.message.send"}) {
		t.Fatalf("external Skill runtime Actions = %#v, want one send after terminal correction", got)
	}
	if fakeLLM.appChatCalls != 3 || !runnerTestRequestContains(fakeLLM.appChatRequests[1], "server-owned operation plan") {
		t.Fatalf("terminal ledger correction was not issued before execution; calls=%d", fakeLLM.appChatCalls)
	}
}

func TestRunnerRejectsNewCompletedProjectedPhaseWithoutExecution(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	alias := toolSet.ProviderTools[0].Function.Name
	completedPlanCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Send message", "status": "completed",
			"expected_action": map[string]interface{}{
				"tool_name": alias, "target_arguments": map[string]interface{}{"recipient_ref": "alice"},
			},
		}},
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{completedPlanCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{}}}
	planBlocked := false
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status != "success" {
				planBlocked = true
			}
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-new-completed-phase", "msg-new-completed-phase", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "给 Alice 发送企业微信消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if !planBlocked {
		t.Fatal("new completed projected phase was not rejected by the plan validator")
	}
	if len(actionTool.calls) != 0 {
		t.Fatalf("provider calls = %#v, want none without an explicit projected Action execution", actionTool.calls)
	}
	if err != nil {
		t.Fatalf("Run() returned an error instead of a safe non-execution answer: %v", err)
	}
	if answer == "消息已发送。" || !strings.Contains(answer, "外部操作未完成") {
		t.Fatalf("unexecuted projected Action did not produce a safe answer: %q", answer)
	}
	if fakeLLM.appChatCalls != 3 || !runnerTestRequestContains(fakeLLM.appChatRequests[2], "server-owned operation plan") {
		t.Fatalf("missing-ledger correction was not issued exactly once; calls=%d", fakeLLM.appChatCalls)
	}
}

func TestProjectedReadAliasIsBlockedBeforeRuntimeWithoutCanonicalOutcomeLedger(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjection("wecom_search_contact", "wecom.contact.search"),
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 2", added, toolSet.SkippedTools)
	}
	state := runtimeStateForRun(RunRequest{
		NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
				"id": "phase-send", "step": "Send message", "status": "in_progress", "required": true,
			}}}}
		},
	})
	state[runtimeStateAllowPlanUpdateKey] = true
	state[runtimeStateNativeAgentLoopKey] = true
	call := nativeExecutionCall(projectedExternalActionRunnerToolCall(
		t, "call-search", toolSet.ProviderTools[0].Function.Name, map[string]interface{}{"query": "Alice"},
	), &toolSet)
	step := (&Runner{SkillRuntime: runtime}).handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-no-ledger", "msg-no-ledger", "", "auto", &adapter.ChatRequest{}),
		resolved,
		call,
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{skills.SkillExternalApps: {}},
		state,
		1,
		nil,
	)
	if !step.recoverable || step.trace.Arguments["code"] != projectedExternalActionPlanIncompleteCode {
		t.Fatalf("handleProgressiveSkillCall() = %#v, want recoverable canonical-ledger rejection", step)
	}
	if len(actionTool.calls) != 0 {
		t.Fatalf("provider calls = %#v, projected prerequisite executed before outcome ledger", actionTool.calls)
	}
}

func TestRunnerProjectedAliasCollisionRoutesThroughExternalSkillRuntime(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	projection := projectedExternalActionRunnerProjection("execute_action", "wecom.message.send")
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{projection}, skills.NativeToolProjectionOptions{
		ReservedToolNames: []string{"EXECUTE_ACTION"},
	}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	alias := toolSet.ProviderTools[0].Function.Name
	if strings.EqualFold(alias, "execute_action") {
		t.Fatalf("projection retained runtime-colliding name %q", alias)
	}
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Send message", "status": "in_progress",
			"expected_action": map[string]interface{}{
				"tool_name": alias, "target_arguments": map[string]interface{}{"recipient_ref": "alice"},
			},
		}},
	})
	actionCall := projectedExternalActionRunnerToolCall(t, "call-action", alias, map[string]interface{}{
		"recipient_ref": "alice", "content": "hello",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, actionCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
		"id": "phase-send", "step": "Send message", "status": "in_progress", "required": true,
	}}}}
	runtimeRouteCalls := 0
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-alias-collision", "msg-alias-collision", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "给 Alice 发送企业微信消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeTools: []RuntimeTool{{
			Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: "EXECUTE_ACTION"}},
			Handler: func(context.Context, adapter.ToolCall) RuntimeToolResult {
				runtimeRouteCalls++
				return RuntimeToolResult{Status: "error", Error: errors.New("wrong runtime route")}
			},
		}},
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "消息已发送。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if runtimeRouteCalls != 0 {
		t.Fatalf("runtime collision handler calls = %d, projected alias was misrouted", runtimeRouteCalls)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.message.send"}) {
		t.Fatalf("external Skill runtime Actions = %#v, want one send", got)
	}
}

func TestRunnerProjectedForbiddenProtocolNameIsAliasedAndRoutesThroughExternalSkillRuntime(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	projection := projectedExternalActionRunnerProjection("CALL_SKILL_TOOL", "wecom.message.send")
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{projection}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	alias := toolSet.ProviderTools[0].Function.Name
	if strings.EqualFold(alias, skills.MetaToolCallSkillTool) {
		t.Fatalf("projection retained forbidden protocol name %q", alias)
	}
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Send message", "status": "in_progress",
			"expected_action": map[string]interface{}{
				"tool_name": alias, "target_arguments": map[string]interface{}{"recipient_ref": "alice"},
			},
		}},
	})
	actionCall := projectedExternalActionRunnerToolCall(t, "call-action", alias, map[string]interface{}{
		"recipient_ref": "alice", "content": "hello",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, actionCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
		"id": "phase-send", "step": "Send message", "status": "in_progress", "required": true,
	}}}}
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-forbidden-alias", "msg-forbidden-alias", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "给 Alice 发送企业微信消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "消息已发送。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.message.send"}) {
		t.Fatalf("external Skill runtime Actions = %#v, want one send", got)
	}
}

func TestRunnerProjectedSameActionSameTargetUsesPhaseIDWithoutProviderLeak(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	alias := toolSet.ProviderTools[0].Function.Name
	expected := func() map[string]interface{} {
		return map[string]interface{}{
			"tool_name":        alias,
			"target_arguments": map[string]interface{}{"recipient_ref": "alice"},
		}
	}
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{"id": "phase-first", "step": "Send first", "status": "in_progress", "expected_action": expected()},
			map[string]interface{}{"id": "phase-second", "step": "Send second", "status": "pending", "expected_action": expected()},
		},
	})
	firstCall := projectedExternalActionRunnerToolCall(t, "call-first", alias, map[string]interface{}{
		"plan_phase_id": "phase-first", "recipient_ref": "alice", "content": "first",
	})
	secondCall := projectedExternalActionRunnerToolCall(t, "call-second", alias, map[string]interface{}{
		"plan_phase_id": "phase-second", "recipient_ref": "alice", "content": "second",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, firstCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "两条消息都已发送。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{secondCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "两条消息均已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
		map[string]interface{}{"id": "phase-first", "step": "Send first", "status": "in_progress", "required": true},
		map[string]interface{}{"id": "phase-second", "step": "Send second", "status": "pending", "required": true},
	}}}
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-repeated-projection", "msg-repeated-projection", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "给 Alice 连续发送两条不同的企业微信消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "两条消息均已发送。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if got := actionTool.businessStrings("content"); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("provider message contents = %#v, want each payload exactly once", got)
	}
	if fakeLLM.appChatCalls != 4 {
		t.Fatalf("AppChat calls = %d, want omitted second phase corrected once", fakeLLM.appChatCalls)
	}
	for _, call := range actionTool.calls {
		if _, leaked := call["plan_phase_id"]; leaked {
			t.Fatalf("plan_phase_id leaked to provider outer arguments: %#v", call)
		}
		if _, leaked := evidenceMapFromAny(call["arguments"])["plan_phase_id"]; leaked {
			t.Fatalf("plan_phase_id leaked to provider business arguments: %#v", call)
		}
	}
}

func TestRunnerProjectedEquivalentProviderActionsAreAnyOfWithoutProviderSelection(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 20000,
	}
	projections := []skills.NativeToolProjection{
		projectedExternalActionRunnerProjectionFor("wecom", "wecom_send_message", "wecom.message.send"),
		projectedExternalActionRunnerProjectionFor("feishu", "feishu_send_message", "feishu.message.send"),
		projectedExternalActionRunnerProjectionFor("dingtalk", "dingtalk_send_message", "dingtalk.message.send"),
	}
	for index := range projections {
		projections[index].Binding.IntentGroup = "message.send"
		projections[index].Binding.IntentTokens = []string{"send", "message", "发送", "消息"}
	}
	if added := skills.AppendNativeToolProjections(&toolSet, projections, skills.NativeToolProjectionOptions{}); added != 3 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 3", added, toolSet.SkippedTools)
	}
	alias := projectedExternalActionAliasForIntegration(t, toolSet, "feishu")
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Send message", "status": "in_progress",
			"expected_action": map[string]interface{}{"tool_name": alias, "target_arguments": map[string]interface{}{"recipient_ref": "alice"}},
		}},
	})
	actionCall := projectedExternalActionRunnerToolCall(t, "call-send", alias, map[string]interface{}{
		"plan_phase_id": "phase-send", "recipient_ref": "alice", "content": "hello",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, actionCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
		"id": "phase-send", "step": "Send message", "status": "in_progress", "required": true,
	}}}}
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-provider-any-of", "msg-provider-any-of", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "给 Alice 发送消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "消息已发送。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if got := actionTool.integrationIDs(); !reflect.DeepEqual(got, []string{"feishu"}) {
		t.Fatalf("provider calls = %#v, want exactly one selected provider", got)
	}
}

func TestRunnerProjectedExplicitProvidersRequireIndependentActionGroups(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 20000,
	}
	wecom := projectedExternalActionRunnerProjectionFor("wecom", "wecom_send_message", "wecom.message.send")
	wecom.Binding.IntentGroup = "wecom:message.send"
	wecom.Binding.IntentTokens = []string{"企业微信", "send", "message", "发送", "消息"}
	feishu := projectedExternalActionRunnerProjectionFor("feishu", "feishu_send_message", "feishu.message.send")
	feishu.Binding.IntentGroup = "feishu:message.send"
	feishu.Binding.IntentTokens = []string{"飞书", "send", "message", "发送", "消息"}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{wecom, feishu}, skills.NativeToolProjectionOptions{}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 2", added, toolSet.SkippedTools)
	}
	wecomAlias := projectedExternalActionAliasForIntegration(t, toolSet, "wecom")
	feishuAlias := projectedExternalActionAliasForIntegration(t, toolSet, "feishu")
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{
				"id": "phase-wecom", "step": "Send with 企业微信", "status": "in_progress",
				"expected_action": map[string]interface{}{"tool_name": wecomAlias, "target_arguments": map[string]interface{}{"recipient_ref": "alice"}},
			},
			map[string]interface{}{
				"id": "phase-feishu", "step": "Send with 飞书", "status": "pending",
				"expected_action": map[string]interface{}{"tool_name": feishuAlias, "target_arguments": map[string]interface{}{"recipient_ref": "alice"}},
			},
		},
	})
	wecomCall := projectedExternalActionRunnerToolCall(t, "call-wecom", wecomAlias, map[string]interface{}{
		"plan_phase_id": "phase-wecom", "recipient_ref": "alice", "content": "hello",
	})
	feishuCall := projectedExternalActionRunnerToolCall(t, "call-feishu", feishuAlias, map[string]interface{}{
		"plan_phase_id": "phase-feishu", "recipient_ref": "alice", "content": "hello",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, wecomCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "两边都已发送。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{feishuCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "企业微信和飞书均已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
		map[string]interface{}{"id": "phase-wecom", "step": "Send with 企业微信", "status": "in_progress", "required": true},
		map[string]interface{}{"id": "phase-feishu", "step": "Send with 飞书", "status": "pending", "required": true},
	}}}
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-provider-explicit", "msg-provider-explicit", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "在企业微信和飞书分别给 Alice 发送消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "企业微信和飞书均已发送。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if got := actionTool.integrationIDs(); !reflect.DeepEqual(got, []string{"wecom", "feishu"}) {
		t.Fatalf("provider calls = %#v, want independent WeCom then Feishu calls", got)
	}
	if fakeLLM.appChatCalls != 4 {
		t.Fatalf("AppChat calls = %d, want omitted provider corrected once", fakeLLM.appChatCalls)
	}
}

func TestRunnerProjectedContinuationRebindsPinnedFingerprintAfterAliasAndTopKChange(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	const fingerprint = "stable-wecom-send-binding"
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 20000,
	}
	projections := make([]skills.NativeToolProjection, 0, 30)
	for index := 0; index < 29; index++ {
		projection := projectedExternalActionRunnerProjectionFor(
			"wecom", fmt.Sprintf("unrelated_action_%02d", index), fmt.Sprintf("wecom.unrelated.%02d", index),
		)
		projection.Binding.IntentMatched = false
		projection.Binding.IntentGroup = ""
		projection.Binding.IntentTokens = nil
		projections = append(projections, projection)
	}
	pinned := projectedExternalActionRunnerProjectionFor("wecom", "wecom_send_message_v2", "wecom.message.send")
	pinned.Binding.BindingFingerprint = fingerprint
	pinned.Binding.Pinned = true
	projections = append(projections, pinned)
	if added := skills.AppendNativeToolProjections(&toolSet, projections, skills.NativeToolProjectionOptions{MaxTools: 1}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want one pinned projection", added, toolSet.SkippedTools)
	}
	alias := projectedExternalActionAliasForIntegration(t, toolSet, "wecom")
	if alias != "wecom_send_message_v2" {
		t.Fatalf("exposed alias = %q, want pinned continuation alias after Top-K change", alias)
	}
	state := projectedExternalActionContinuationState("wecom_send_message_v1", fingerprint)
	actionCall := projectedExternalActionRunnerToolCall(t, "call-send", alias, map[string]interface{}{
		"plan_phase_id": "phase-send", "recipient_ref": "alice", "content": "continued",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{actionCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-alias-continuation", "msg-alias-continuation", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "继续给 Alice 发送消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "消息已发送。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.message.send"}) {
		t.Fatalf("external Actions = %#v, want one rebound send", got)
	}
	if got := evidenceStringFromAny(actionTool.calls[0]["connection_id"]); got != "connection-wecom" {
		t.Fatalf("connection_id = %q, want pinned server connection", got)
	}
}

func TestProjectedContinuationBindingDriftBlocksBeforeProviderAndRevocationStaysSafe(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	const oldFingerprint = "stable-wecom-send-binding"
	projection := projectedExternalActionRunnerProjectionFor("wecom", "wecom_send_message_v2", "wecom.message.send")
	projection.Binding.BindingFingerprint = "drifted-schema-or-connection-binding"
	projection.Binding.FixedArguments["connection_id"] = "connection-wecom-replacement"
	projection.Binding.Pinned = true
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{projection}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	stateSnapshot := projectedExternalActionContinuationState("wecom_send_message_v1", oldFingerprint)
	state := runtimeStateForRun(RunRequest{
		NativeToolSet: &toolSet,
		Prepared: NewPreparedChat("conv-binding-drift", "msg-binding-drift", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "继续给 Alice 发送消息"}},
		}),
		RuntimeStateSnapshot: func() map[string]interface{} { return stateSnapshot },
	})
	state[runtimeStateAllowPlanUpdateKey] = true
	state[runtimeStateNativeAgentLoopKey] = true
	alias := projectedExternalActionAliasForIntegration(t, toolSet, "wecom")
	call := nativeExecutionCall(projectedExternalActionRunnerToolCall(t, "call-send", alias, map[string]interface{}{
		"plan_phase_id": "phase-send", "recipient_ref": "alice", "content": "continued",
	}), &toolSet)
	step := (&Runner{SkillRuntime: runtime}).handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-binding-drift", "msg-binding-drift", "", "auto", &adapter.ChatRequest{}),
		resolved,
		call,
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{skills.SkillExternalApps: {}},
		state,
		1,
		nil,
	)
	if !step.recoverable || step.trace.Arguments["code"] != projectedExternalActionPlanIncompleteCode {
		t.Fatalf("drifted binding call = %#v, want recoverable pre-provider rejection", step)
	}
	if len(actionTool.calls) != 0 {
		t.Fatalf("provider calls after fingerprint/connection drift = %#v, want none", actionTool.calls)
	}

	revokedState := runtimeStateForRun(RunRequest{
		NativeToolSet:        &skills.NativeToolSet{ToolBindings: map[string]skills.NativeToolBinding{}},
		RuntimeStateSnapshot: func() map[string]interface{} { return stateSnapshot },
	})
	decision := terminalStateGuardEvaluate(revokedState, "消息已发送。")
	if decision.PendingExternalAction != nil || !strings.Contains(decision.FinalAnswer, "未完成") || decision.FinalAnswer == "消息已发送。" {
		t.Fatalf("revoked projected binding final decision = %#v, want deterministic safe non-replay answer", decision)
	}
}

func TestRunnerProjectedMutationWithEmptyCanonicalTargetIsBlockedBeforeProvider(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	const fingerprint = "stable-wecom-send-binding"
	projection := projectedExternalActionRunnerProjectionFor("wecom", "wecom_send_message", "wecom.message.send")
	projection.Binding.BindingFingerprint = fingerprint
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{projection}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	stateSnapshot := projectedExternalActionContinuationState("wecom_send_message", fingerprint)
	phase := evidenceMapsFromAny(evidenceMapFromAny(stateSnapshot["operation_plan"])["phases"])[0]
	delete(evidenceMapFromAny(phase["expected_action"]), "target_arguments")
	state := runtimeStateForRun(RunRequest{
		NativeToolSet: &toolSet,
		Prepared: NewPreparedChat("conv-empty-mutation-target", "msg-empty-mutation-target", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "给 Alice 发送消息"}},
		}),
		RuntimeStateSnapshot: func() map[string]interface{} { return stateSnapshot },
	})
	state[runtimeStateAllowPlanUpdateKey] = true
	state[runtimeStateNativeAgentLoopKey] = true
	alias := projectedExternalActionAliasForIntegration(t, toolSet, "wecom")
	call := nativeExecutionCall(projectedExternalActionRunnerToolCall(t, "call-send", alias, map[string]interface{}{
		"plan_phase_id": "phase-send", "recipient_ref": "alice", "content": "must-not-send",
	}), &toolSet)
	step := (&Runner{SkillRuntime: runtime}).handleProgressiveSkillCall(
		context.Background(),
		NewPreparedChat("conv-empty-mutation-target", "msg-empty-mutation-target", "", "auto", &adapter.ChatRequest{}),
		resolved,
		call,
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{skills.SkillExternalApps: {}},
		state,
		1,
		nil,
	)
	if !step.recoverable || step.trace.Arguments["code"] != projectedExternalActionPlanIncompleteCode {
		t.Fatalf("empty-target mutation call = %#v, want recoverable canonical-target rejection", step)
	}
	if len(actionTool.calls) != 0 {
		t.Fatalf("provider calls = %#v, empty-target mutation must not execute", actionTool.calls)
	}
	decision := terminalStateGuardEvaluate(state, "消息已发送。")
	if decision.Path != terminalStateGuardBlocked || terminalStateGuardCanStream(state) {
		t.Fatalf("empty-target mutation terminal decision = %#v, want blocked and non-streaming", decision)
	}
	if got := evidenceStringFromAny(phase["status"]); got != "in_progress" {
		t.Fatalf("phase status = %q, want still open", got)
	}
}

func TestRunnerProjectedUnresolvedMutationAllowsDeclaredPreparationThenBindsExactTarget(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 20000,
	}
	searchProjection := projectedExternalActionRunnerProjection("wecom_search_contact", "wecom.contact.search")
	searchProjection.Binding.IntentMatched = false
	searchProjection.Binding.IntentGroup = ""
	searchProjection.Binding.IntentTokens = nil
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		searchProjection,
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 2", added, toolSet.SkippedTools)
	}
	searchAlias := toolSet.ProviderTools[0].Function.Name
	sendAlias := toolSet.ProviderTools[1].Function.Name
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Resolve Alice and send message", "status": "in_progress",
			"expected_action": map[string]interface{}{"tool_name": sendAlias},
		}},
	})
	searchCall := projectedExternalActionRunnerToolCall(t, "call-search", searchAlias, map[string]interface{}{
		"plan_phase_id": "phase-send", "query": "Alice",
	})
	bindTargetCall := projectedExternalActionRunnerToolCall(t, "call-bind-target", skills.MetaToolUpdatePlan, map[string]interface{}{
		"phase_updates": []interface{}{map[string]interface{}{
			"id": "phase-send",
			"expected_action": map[string]interface{}{
				"tool_name": sendAlias, "target_arguments": map[string]interface{}{"recipient_ref": "wm-alice"},
			},
		}},
	})
	sendCall := projectedExternalActionRunnerToolCall(t, "call-send", sendAlias, map[string]interface{}{
		"plan_phase_id": "phase-send", "recipient_ref": "wm-alice", "content": "hello",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, searchCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{bindTargetCall, sendCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
		map[string]interface{}{"id": "phase-send", "step": "Resolve Alice and send message", "status": "in_progress", "required": true},
	}}}
	firstLedgerEpoch := ""
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				phases := evidenceMapsFromAny(trace.Result["plan"])
				if firstLedgerEpoch == "" && len(phases) > 0 {
					firstLedgerEpoch = evidenceStringFromAny(phases[0][operationPlanServerProjectedLedgerEpochKey])
				}
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-resolve-target", "msg-resolve-target", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "查询 Alice 的企业微信联系人并发送消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "消息已发送。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.contact.search", "wecom.message.send"}) {
		t.Fatalf("external Actions = %#v, want declared read then exactly one send", got)
	}
	if got := actionTool.businessStrings("recipient_ref"); !reflect.DeepEqual(got, []string{"", "wm-alice"}) {
		t.Fatalf("provider recipient targets = %#v, write target was not bound exactly after preparation", got)
	}
	if got := actionTool.businessStrings("max_results"); !reflect.DeepEqual(got, []string{"10", ""}) {
		t.Fatalf("provider result limits = %#v, omitted search default was not materialized server-side", got)
	}
	finalPhase := evidenceMapsFromAny(evidenceMapFromAny(state["operation_plan"])["phases"])[0]
	if finalEpoch := evidenceStringFromAny(finalPhase[operationPlanServerProjectedLedgerEpochKey]); firstLedgerEpoch == "" || finalEpoch == "" || finalEpoch == firstLedgerEpoch {
		t.Fatalf("target binding did not rotate the server ledger epoch: first=%q final=%q", firstLedgerEpoch, finalEpoch)
	}
	if fakeLLM.appChatCalls != 4 {
		t.Fatalf("AppChat calls = %d, unresolved target final claim was not corrected", fakeLLM.appChatCalls)
	}
}

func TestRunnerProjectedForgedUnobservedTargetNeverReachesMutationProvider(t *testing.T) {
	actionTool, state, err := runProjectedObservedTargetRejection(t, []string{"wm-alice"}, "wm-mallory")
	if err == nil || !strings.Contains(err.Error(), "unexpected AppChat call") {
		t.Fatalf("Run() error = %v, want the deliberately bounded model script to stop after runtime rejection", err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.contact.search"}) {
		t.Fatalf("external Actions = %#v, forged target reached the mutation provider", got)
	}
	decision := terminalStateGuardEvaluate(runtimeStateForRun(RunRequest{
		NativeToolSet:        state.toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state.snapshot },
	}), "消息已发送。")
	if decision.Path != terminalStateGuardBlocked {
		t.Fatalf("forged-target terminal decision = %#v, want blocked", decision)
	}
}

func TestRunnerProjectedAmbiguousPreparationTargetNeverReachesMutationProvider(t *testing.T) {
	actionTool, _, err := runProjectedObservedTargetRejection(t, []string{"wm-alice-1", "wm-alice-2"}, "wm-alice-1")
	if err == nil || !strings.Contains(err.Error(), "unexpected AppChat call") {
		t.Fatalf("Run() error = %v, want the deliberately bounded model script to stop after ambiguity rejection", err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.contact.search"}) {
		t.Fatalf("external Actions = %#v, ambiguous target reached the mutation provider", got)
	}
}

func TestRunnerProjectedPreparationFromDifferentConnectionNeverReachesMutationProvider(t *testing.T) {
	actionTool, _, err := runProjectedObservedTargetRejectionWithSearchConnection(
		t, []string{"wm-alice"}, "wm-alice", "connection-wecom-other",
	)
	if err == nil || !strings.Contains(err.Error(), "unexpected AppChat call") {
		t.Fatalf("Run() error = %v, want the deliberately bounded model script to stop after connection rejection", err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.contact.search"}) {
		t.Fatalf("external Actions = %#v, cross-connection target reached the mutation provider", got)
	}
}

func TestRunnerProjectedFullSingleResultPageNeverBindsOrCallsMutationProvider(t *testing.T) {
	actionTool, state, err := runProjectedObservedTargetRejectionWithObservation(
		t, []string{"wm-alice"}, "wm-alice", "connection-wecom",
		map[string]interface{}{"max_results": 1}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "unexpected AppChat call") {
		t.Fatalf("Run() error = %v, want the deliberately bounded model script to stop after incomplete-page rejection", err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"wecom.contact.search"}) {
		t.Fatalf("external Actions = %#v, a full max_results=1 page reached the mutation provider", got)
	}
	decision := terminalStateGuardEvaluate(runtimeStateForRun(RunRequest{
		NativeToolSet:        state.toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state.snapshot },
	}), "消息已发送。")
	if decision.Path != terminalStateGuardBlocked {
		t.Fatalf("full-page terminal decision = %#v, want blocked", decision)
	}
}

func TestRunnerProjectedRefinedPreparationSupersedesEarlierAmbiguousResultInSameEpoch(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	actionTool.searchRecipientRefBatches = [][]string{{"wm-alice-1", "wm-alice-2"}, {"wm-alice-2"}}
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 20000,
	}
	searchProjection := projectedExternalActionRunnerProjection("wecom_search_contact", "wecom.contact.search")
	searchProjection.Binding.IntentMatched = false
	searchProjection.Binding.IntentGroup = ""
	searchProjection.Binding.IntentTokens = nil
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		searchProjection,
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 2", added, toolSet.SkippedTools)
	}
	searchAlias := toolSet.ProviderTools[0].Function.Name
	sendAlias := toolSet.ProviderTools[1].Function.Name
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Resolve Alice and send message", "status": "in_progress",
			"expected_action": map[string]interface{}{"tool_name": sendAlias},
		}},
	})
	broadSearchCall := projectedExternalActionRunnerToolCall(t, "call-search-broad", searchAlias, map[string]interface{}{
		"plan_phase_id": "phase-send", "query": "Alice",
	})
	refinedSearchCall := projectedExternalActionRunnerToolCall(t, "call-search-refined", searchAlias, map[string]interface{}{
		"plan_phase_id": "phase-send", "query": "Alice in product team",
	})
	bindTargetCall := projectedExternalActionRunnerToolCall(t, "call-bind-target", skills.MetaToolUpdatePlan, map[string]interface{}{
		"phase_updates": []interface{}{map[string]interface{}{
			"id": "phase-send", "expected_action": map[string]interface{}{
				"tool_name": sendAlias, "target_arguments": map[string]interface{}{"recipient_ref": "wm-alice-2"},
			},
		}},
	})
	sendCall := projectedExternalActionRunnerToolCall(t, "call-send", sendAlias, map[string]interface{}{
		"plan_phase_id": "phase-send", "recipient_ref": "wm-alice-2", "content": "hello",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, broadSearchCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{refinedSearchCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{bindTargetCall, sendCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
		map[string]interface{}{"id": "phase-send", "step": "Resolve Alice and send message", "status": "in_progress", "required": true},
	}}}
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-refined-target", "msg-refined-target", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "给 Alice 发送企业微信消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "消息已发送。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{
		"wecom.contact.search", "wecom.contact.search", "wecom.message.send",
	}) {
		t.Fatalf("external Actions = %#v, refined result did not safely supersede the broad search", got)
	}
}

func TestProjectedObservedTargetBindingIgnoresWrongPreparationPhaseOrEpoch(t *testing.T) {
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	searchProjection := projectedExternalActionRunnerProjection("wecom_search_contact", "wecom.contact.search")
	searchProjection.Binding.IntentMatched = false
	searchProjection.Binding.IntentGroup = ""
	searchProjection.Binding.IntentTokens = nil
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		searchProjection,
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 2", added, toolSet.SkippedTools)
	}
	sendAlias := ""
	sendFingerprint := ""
	for alias, binding := range toolSet.ToolBindings {
		if strings.EqualFold(evidenceStringFromAny(binding.FixedArguments["action_id"]), "wecom.message.send") {
			sendAlias = alias
			sendFingerprint = binding.BindingFingerprint
			break
		}
	}
	if sendAlias == "" || sendFingerprint == "" {
		t.Fatal("missing projected send binding")
	}
	for _, testCase := range []struct {
		name        string
		phaseID     string
		ledgerEpoch string
	}{
		{name: "wrong phase", phaseID: "phase-other", ledgerEpoch: "ledger-current"},
		{name: "wrong epoch", phaseID: "phase-send", ledgerEpoch: "ledger-stale"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			snapshot := map[string]interface{}{
				"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
					"id": "phase-send", "step": "Resolve and send", "status": "in_progress", "required": true,
					operationPlanServerProjectedLedgerEpochKey: "ledger-current",
					"expected_action": map[string]interface{}{
						"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
						"projected_tool_name": sendAlias, planExpectedActionServerProjectionKey: sendAlias,
						planExpectedActionServerBindingFingerprintKey: sendFingerprint,
						"target": map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
					},
				}}},
				"skill_invocations": []interface{}{map[string]interface{}{
					"kind": "tool_call", "status": "success", "skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
					"arguments": map[string]interface{}{
						"plan_phase_id": testCase.phaseID,
						operationPlanServerProjectedLedgerEpochKey:       testCase.ledgerEpoch,
						planExpectedActionServerBindingFingerprintKey:    sendFingerprint,
						operationPlanServerProjectedConnectionBindingKey: skills.NativeExternalActionConnectionBindingHash("connection-wecom"),
					},
					"result": map[string]interface{}{
						"integration_id": "wecom", "action_id": "wecom.contact.search",
						projectedExternalObservedPreparationTargetsKey: map[string]interface{}{
							"recipient_ref": []interface{}{"wm-alice"},
						},
					},
				}},
			}
			state := runtimeStateForRun(RunRequest{
				NativeToolSet: &toolSet, RuntimeStateSnapshot: func() map[string]interface{} { return snapshot },
			})
			call := nativeExecutionCall(projectedExternalActionRunnerToolCall(t, "call-bind", skills.MetaToolUpdatePlan, map[string]interface{}{
				"phase_updates": []interface{}{map[string]interface{}{
					"id": "phase-send", "expected_action": map[string]interface{}{
						"tool_name": sendAlias, "target_arguments": map[string]interface{}{"recipient_ref": "wm-alice"},
					},
				}},
			}), &toolSet)
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
				t.Fatal(err)
			}
			step := (&Runner{}).handleUpdatePlanCall("call-bind", args, state, 2)
			if !step.recoverable || step.trace.Status != "error" {
				t.Fatalf("handleUpdatePlanCall() = %#v, want wrong phase/epoch observation rejected", step)
			}
		})
	}
}

type projectedObservedTargetRejectionState struct {
	toolSet  *skills.NativeToolSet
	snapshot map[string]interface{}
}

func runProjectedObservedTargetRejection(
	t *testing.T,
	searchRecipientRefs []string,
	proposedRecipientRef string,
) (*projectedExternalActionRunnerTool, projectedObservedTargetRejectionState, error) {
	return runProjectedObservedTargetRejectionWithSearchConnection(
		t, searchRecipientRefs, proposedRecipientRef, "connection-wecom",
	)
}

func runProjectedObservedTargetRejectionWithSearchConnection(
	t *testing.T,
	searchRecipientRefs []string,
	proposedRecipientRef string,
	searchConnectionID string,
) (*projectedExternalActionRunnerTool, projectedObservedTargetRejectionState, error) {
	return runProjectedObservedTargetRejectionWithObservation(
		t, searchRecipientRefs, proposedRecipientRef, searchConnectionID, nil, nil,
	)
}

func runProjectedObservedTargetRejectionWithObservation(
	t *testing.T,
	searchRecipientRefs []string,
	proposedRecipientRef string,
	searchConnectionID string,
	searchArguments map[string]interface{},
	searchResultMetadata map[string]interface{},
) (*projectedExternalActionRunnerTool, projectedObservedTargetRejectionState, error) {
	t.Helper()
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	actionTool.searchRecipientRefs = append([]string(nil), searchRecipientRefs...)
	actionTool.searchResultMetadata = copyStringAnyMap(searchResultMetadata)
	toolSet := &skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 20000,
	}
	searchProjection := projectedExternalActionRunnerProjection("wecom_search_contact", "wecom.contact.search")
	searchProjection.Binding.IntentMatched = false
	searchProjection.Binding.IntentGroup = ""
	searchProjection.Binding.IntentTokens = nil
	searchProjection.Binding.FixedArguments["connection_id"] = strings.TrimSpace(searchConnectionID)
	if added := skills.AppendNativeToolProjections(toolSet, []skills.NativeToolProjection{
		searchProjection,
		projectedExternalActionRunnerProjection("wecom_send_message", "wecom.message.send"),
	}, skills.NativeToolProjectionOptions{}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 2", added, toolSet.SkippedTools)
	}
	searchAlias := projectedExternalActionAliasForAction(t, *toolSet, "wecom", "wecom.contact.search")
	sendAlias := projectedExternalActionAliasForAction(t, *toolSet, "wecom", "wecom.message.send")
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Resolve Alice and send message", "status": "in_progress",
			"expected_action": map[string]interface{}{"tool_name": sendAlias},
		}},
	})
	searchCallArguments := map[string]interface{}{
		"plan_phase_id": "phase-send", "query": "Alice",
	}
	for key, value := range searchArguments {
		searchCallArguments[key] = value
	}
	searchCall := projectedExternalActionRunnerToolCall(t, "call-search", searchAlias, searchCallArguments)
	bindTargetCall := projectedExternalActionRunnerToolCall(t, "call-bind-target", skills.MetaToolUpdatePlan, map[string]interface{}{
		"phase_updates": []interface{}{map[string]interface{}{
			"id": "phase-send",
			"expected_action": map[string]interface{}{
				"tool_name": sendAlias, "target_arguments": map[string]interface{}{"recipient_ref": proposedRecipientRef},
			},
		}},
	})
	sendCall := projectedExternalActionRunnerToolCall(t, "call-send", sendAlias, map[string]interface{}{
		"plan_phase_id": "phase-send", "recipient_ref": proposedRecipientRef, "content": "must-not-send",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, searchCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{bindTargetCall, sendCall}}, FinishReason: "tool_calls"}}},
	}}
	snapshot := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{
		map[string]interface{}{"id": "phase-send", "step": "Resolve Alice and send message", "status": "in_progress", "required": true},
	}}}
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				snapshot["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-target-rejection", "msg-target-rejection", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "给 Alice 发送企业微信消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return snapshot },
	})
	return actionTool, projectedObservedTargetRejectionState{toolSet: toolSet, snapshot: snapshot}, err
}

func TestRunnerProjectedFeishuSendUserRequiresExplicitAndCanonicalizesSelfTarget(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedFeishuSendUserRunnerProjection(),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	alias := toolSet.ProviderTools[0].Function.Name
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Send message to myself", "status": "in_progress",
			"expected_action": map[string]interface{}{
				"tool_name": alias,
				"target_arguments": map[string]interface{}{
					"recipient_type": "self", "recipient_id": "ignored-plan-id",
				},
			},
		}},
	})
	sendCall := projectedExternalActionRunnerToolCall(t, "call-send", alias, map[string]interface{}{
		"plan_phase_id": "phase-send", "recipient_type": "self", "recipient_id": "ignored-call-id", "text": "hello self",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, sendCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "消息已发送给自己。"}, FinishReason: "stop"}}},
	}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
		"id": "phase-send", "step": "Send message to myself", "status": "in_progress", "required": true,
	}}}}
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-feishu-self", "msg-feishu-self", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "用飞书给我自己发一条消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err != nil || answer != "消息已发送给自己。" {
		t.Fatalf("Run() answer=%q error=%v", answer, err)
	}
	if got := actionTool.actionIDs(); !reflect.DeepEqual(got, []string{"feishu.message.send_user"}) {
		t.Fatalf("external Actions = %#v, want one Feishu send_user", got)
	}
	if got := actionTool.businessStrings("recipient_type"); !reflect.DeepEqual(got, []string{"self"}) {
		t.Fatalf("provider recipient types = %#v, want materialized self", got)
	}
	if _, exists := evidenceMapFromAny(actionTool.calls[0]["arguments"])["recipient_id"]; exists {
		t.Fatalf("provider arguments invented recipient_id: %#v", actionTool.calls[0])
	}
	phase := evidenceMapsFromAny(evidenceMapFromAny(state["operation_plan"])["phases"])[0]
	targets := evidenceMapFromAny(evidenceMapFromAny(phase["expected_action"])["target_arguments"])
	if evidenceStringFromAny(targets["recipient_type"]) != "self" || targets["recipient_id"] != nil {
		t.Fatalf("server-canonical self target = %#v, want recipient_type self only", targets)
	}
}

func TestRunnerProjectedFeishuNonSelfStillRequiresRecipientID(t *testing.T) {
	runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
	toolSet := skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
	}
	if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
		projectedFeishuSendUserRunnerProjection(),
	}, skills.NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
	}
	alias := toolSet.ProviderTools[0].Function.Name
	planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-send", "step": "Send message to a Feishu user", "status": "in_progress",
			"expected_action": map[string]interface{}{
				"tool_name": alias, "target_arguments": map[string]interface{}{"recipient_type": "open_id"},
			},
		}},
	})
	sendCall := projectedExternalActionRunnerToolCall(t, "call-send", alias, map[string]interface{}{
		"plan_phase_id": "phase-send", "recipient_type": "open_id", "text": "must not send",
	})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, sendCall}}, FinishReason: "tool_calls"}},
	}}}
	state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
		"id": "phase-send", "step": "Send message to a Feishu user", "status": "in_progress", "required": true,
	}}}}
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			if trace.Kind == "plan_update" && trace.Status == "success" {
				state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
			}
		},
	}
	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: NewPreparedChat("conv-feishu-nonself", "msg-feishu-nonself", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "用飞书给指定用户发消息"}},
		}),
		Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
		RuntimeStateSnapshot: func() map[string]interface{} { return state },
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected AppChat call") {
		t.Fatalf("Run() error=%v, want bounded script to stop after missing-recipient rejection", err)
	}
	if len(actionTool.calls) != 0 {
		t.Fatalf("provider calls = %#v, non-self recipient without recipient_id must be blocked", actionTool.calls)
	}
}

func TestRunnerProjectedFeishuRequiredRecipientTypeFailsBeforeProvider(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		arguments map[string]interface{}
	}{
		{name: "recipient_id_without_type", arguments: map[string]interface{}{
			"recipient_id": "ou_target", "text": "must not send",
		}},
		{name: "recipient_and_type_both_omitted", arguments: map[string]interface{}{
			"text": "must not send",
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runtime, resolved, actionTool := newProjectedExternalActionRunnerRuntime(t)
			toolSet := skills.NativeToolSet{
				ActiveSkillIDs: []string{skills.SkillExternalApps}, ToolBindings: map[string]skills.NativeToolBinding{}, BudgetChars: 10000,
			}
			if added := skills.AppendNativeToolProjections(&toolSet, []skills.NativeToolProjection{
				projectedFeishuSendUserRunnerProjection(),
			}, skills.NativeToolProjectionOptions{}); added != 1 {
				t.Fatalf("AppendNativeToolProjections() added=%d skipped=%#v, want 1", added, toolSet.SkippedTools)
			}
			alias := toolSet.ProviderTools[0].Function.Name
			planCall := projectedExternalActionRunnerToolCall(t, "call-plan", skills.MetaToolUpdatePlan, map[string]interface{}{
				"plan": []interface{}{map[string]interface{}{
					"id": "phase-send", "step": "Send a Feishu message", "status": "in_progress",
					"expected_action": map[string]interface{}{"tool_name": alias},
				}},
			})
			testCase.arguments["plan_phase_id"] = "phase-send"
			sendCall := projectedExternalActionRunnerToolCall(t, "call-send", alias, testCase.arguments)
			fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
				Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall, sendCall}}, FinishReason: "tool_calls"}},
			}}}
			state := map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
				"id": "phase-send", "step": "Send a Feishu message", "status": "in_progress", "required": true,
			}}}}
			runner := &Runner{
				LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
				OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
					if trace.Kind == "plan_update" && trace.Status == "success" {
						state["operation_plan"] = map[string]interface{}{"phases": trace.Result["plan"]}
					}
				},
			}
			_, _, err := runner.Run(context.Background(), RunRequest{
				Prepared: NewPreparedChat("conv-feishu-required-type-"+testCase.name, "msg-feishu-required-type", "", "auto", &adapter.ChatRequest{
					Messages: []adapter.Message{{Role: "user", Content: "用飞书发消息"}},
				}),
				Resolved: resolved, NativeAgentLoop: true, NativeToolSet: &toolSet,
				RuntimeStateSnapshot: func() map[string]interface{} { return state },
			})
			if err == nil || !strings.Contains(err.Error(), "unexpected AppChat call") {
				t.Fatalf("Run() error=%v, want bounded script to stop after required recipient_type rejection", err)
			}
			if len(actionTool.calls) != 0 {
				t.Fatalf("provider calls = %#v, missing required recipient_type must be blocked", actionTool.calls)
			}
		})
	}
}

func projectedFeishuSendUserRunnerProjection() skills.NativeToolProjection {
	return skills.NativeToolProjection{
		Name: "send_feishu_user_message", NameScope: "feishu/feishu.message.send_user",
		Description: "Send a Feishu message to a user or to myself.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"plan_phase_id": map[string]interface{}{"type": "string"},
				"recipient_type": map[string]interface{}{
					"type": "string", "enum": []interface{}{"self", "open_id", "user_id", "union_id"}, "default": "self",
				},
				"recipient_id": map[string]interface{}{
					"type": "string", "minLength": 1,
					"x-zgi-discard-when": map[string]interface{}{"argument": "recipient_type", "equals": "self"},
				},
				"text": map[string]interface{}{"type": "string", "minLength": 1},
			},
			"required": []interface{}{"recipient_type", "text"},
			"allOf": []interface{}{map[string]interface{}{
				"if": map[string]interface{}{
					"properties": map[string]interface{}{"recipient_type": map[string]interface{}{"const": "self"}},
					"required":   []interface{}{"recipient_type"},
				},
				"else": map[string]interface{}{"required": []interface{}{"recipient_id"}},
			}},
			"additionalProperties": false,
		},
		Binding: skills.NativeToolBinding{
			SkillID: skills.SkillExternalApps, ToolName: "execute_action", ArgumentEnvelope: "arguments",
			IntentMatched: true, IntentGroup: "feishu:message.send", IntentTokens: []string{"飞书", "发送", "消息"},
			BindingFingerprint: "binding-feishu-send-user", Effect: "external_send",
			FixedArguments: map[string]interface{}{
				"integration_id": "feishu", "action_id": "feishu.message.send_user", "connection_id": "connection-feishu",
				"action_schema_hash": "hash-feishu-send-user", "action_schema_revision": "schema-1", "catalog_revision": "catalog-1",
			},
			TargetArgumentPaths: []string{"recipient_id", "recipient_type"}, PlanPhaseArgument: "plan_phase_id",
		},
	}
}

func projectedExternalActionContinuationState(oldAlias string, fingerprint string) map[string]interface{} {
	return map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{
		"id": "phase-send", "step": "Send message", "status": "in_progress", "required": true,
		operationPlanServerProjectedLedgerEpochKey: "ledger-epoch-1",
		"expected_action": map[string]interface{}{
			"skill_id": skills.SkillExternalApps, "tool_name": "execute_action", "projected_tool_name": oldAlias,
			planExpectedActionServerProjectionKey:         oldAlias,
			planExpectedActionServerBindingFingerprintKey: fingerprint,
			"target":           map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
			"target_arguments": map[string]interface{}{"recipient_ref": "alice"},
		},
	}}}}
}

func newProjectedExternalActionRunnerRuntime(t *testing.T) (*skills.Runtime, *skills.ResolvedSkills, *projectedExternalActionRunnerTool) {
	t.Helper()
	actionTool := &projectedExternalActionRunnerTool{}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&projectedExternalActionRunnerProvider{tool: actionTool}); err != nil {
		t.Fatalf("register projected external Action provider: %v", err)
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{
			ID: skills.SkillExternalApps, Name: "External Apps", RuntimeType: skills.SkillRuntimeTypePrompt,
			MaxCallsPerTurn: 8,
		},
		Instructions: "Execute only the server-bound external Action requested by the direct function.",
		Tools: []skills.SkillToolDefinition{{
			Name: "execute_action", ProviderType: tools.ToolProviderTypeBuiltin, ProviderID: "projected_external_action_test",
			InputSchema: projectedExternalActionRunnerOuterSchema(),
		}},
	}}}
	return skills.NewRuntime(tools.NewToolEngine(manager), manager), resolved, actionTool
}

func projectedExternalActionRunnerProjection(name string, actionID string) skills.NativeToolProjection {
	return projectedExternalActionRunnerProjectionFor("wecom", name, actionID)
}

func projectedExternalActionRunnerProjectionFor(integrationID string, name string, actionID string) skills.NativeToolProjection {
	properties := map[string]interface{}{"plan_phase_id": map[string]interface{}{"type": "string"}}
	if strings.Contains(actionID, "search") {
		properties["query"] = map[string]interface{}{"type": "string"}
		properties["max_results"] = map[string]interface{}{"type": "integer", "minimum": 1, "default": 10}
		properties["page_size"] = map[string]interface{}{"type": "integer", "minimum": 1}
	} else {
		properties["recipient_ref"] = map[string]interface{}{"type": "string"}
		properties["content"] = map[string]interface{}{"type": "string"}
	}
	effect := "external_send"
	intentGroup := "message.send"
	intentTokens := []string{"send", "message", "发送", "消息"}
	if strings.Contains(actionID, "search") {
		effect = "read"
		intentGroup = "contact.search"
		intentTokens = []string{"search", "contact", "查询", "联系人", "成员"}
	}
	targetArgumentPaths := []string(nil)
	preparationActionIDs := []string(nil)
	preparationHints := []skills.NativeExternalActionPreparationHint(nil)
	if strings.Contains(actionID, "send") {
		targetArgumentPaths = []string{"recipient_ref"}
		preparationActionIDs = []string{integrationID + ".contact.search"}
		preparationHints = []skills.NativeExternalActionPreparationHint{{
			ActionID: integrationID + ".contact.search", Relation: "resolve_target",
			TargetArguments: []string{"recipient_ref"}, ResultPaths: []string{"members[].recipient_ref"},
		}}
	}
	return skills.NativeToolProjection{
		Name: name, NameScope: integrationID + "/" + actionID, Description: "Execute " + actionID,
		InputSchema: map[string]interface{}{
			"type": "object", "properties": properties, "additionalProperties": false,
		},
		Binding: skills.NativeToolBinding{
			SkillID: skills.SkillExternalApps, ToolName: "execute_action", ArgumentEnvelope: "arguments",
			IntentMatched:      true,
			IntentGroup:        intentGroup,
			IntentTokens:       intentTokens,
			BindingFingerprint: "binding-" + integrationID + "-" + actionID,
			Effect:             effect,
			FixedArguments: map[string]interface{}{
				"integration_id": integrationID, "action_id": actionID, "connection_id": "connection-" + integrationID,
				"action_schema_hash": "hash-" + actionID, "action_schema_revision": "schema-1", "catalog_revision": "catalog-1",
			},
			TargetArgumentPaths:  targetArgumentPaths,
			PreparationActionIDs: preparationActionIDs,
			PreparationHints:     preparationHints,
			PlanPhaseArgument:    "plan_phase_id",
		},
	}
}

func projectedExternalActionAliasForIntegration(t *testing.T, toolSet skills.NativeToolSet, integrationID string) string {
	t.Helper()
	for alias, binding := range toolSet.ToolBindings {
		if strings.EqualFold(evidenceStringFromAny(binding.FixedArguments["integration_id"]), integrationID) {
			return alias
		}
	}
	t.Fatalf("no projected alias for integration %q", integrationID)
	return ""
}

func projectedExternalActionAliasForAction(
	t *testing.T,
	toolSet skills.NativeToolSet,
	integrationID string,
	actionID string,
) string {
	t.Helper()
	for alias, binding := range toolSet.ToolBindings {
		if strings.EqualFold(evidenceStringFromAny(binding.FixedArguments["integration_id"]), integrationID) &&
			strings.EqualFold(evidenceStringFromAny(binding.FixedArguments["action_id"]), actionID) {
			return alias
		}
	}
	t.Fatalf("no projected alias for integration %q Action %q", integrationID, actionID)
	return ""
}

func projectedExternalActionRunnerToolCall(t *testing.T, callID string, name string, arguments map[string]interface{}) adapter.ToolCall {
	t.Helper()
	encoded, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	return adapter.ToolCall{ID: callID, Type: "function", Function: adapter.FunctionCall{Name: name, Arguments: string(encoded)}}
}

func projectedExternalActionRunnerOuterSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"integration_id":         map[string]interface{}{"type": "string"},
			"connection_id":          map[string]interface{}{"type": "string"},
			"action_id":              map[string]interface{}{"type": "string"},
			"action_schema_hash":     map[string]interface{}{"type": "string"},
			"action_schema_revision": map[string]interface{}{"type": "string"},
			"catalog_revision":       map[string]interface{}{"type": "string"},
			"arguments":              map[string]interface{}{"type": "object", "additionalProperties": true},
		},
		"required":             []string{"integration_id", "action_id", "arguments"},
		"additionalProperties": false,
	}
}

type projectedExternalActionRunnerProvider struct {
	tool *projectedExternalActionRunnerTool
}

func (p *projectedExternalActionRunnerProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity:     tools.ToolProviderIdentity{Name: "projected_external_action_test", Label: tools.I18nText{"en_US": "Projected external Action test"}},
		ProviderType: tools.ToolProviderTypeBuiltin,
	}
}

func (p *projectedExternalActionRunnerProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (p *projectedExternalActionRunnerProvider) GetTool(name string) (tools.Tool, error) {
	if name != "execute_action" || p.tool == nil {
		return nil, tools.ErrToolNotFound
	}
	return p.tool, nil
}

func (p *projectedExternalActionRunnerProvider) GetTools() []tools.Tool {
	if p.tool == nil {
		return nil
	}
	return []tools.Tool{p.tool}
}

func (p *projectedExternalActionRunnerProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type projectedExternalActionRunnerTool struct {
	calls                      []map[string]interface{}
	searchRecipientRefs        []string
	searchRecipientRefBatches  [][]string
	searchRecipientBatchCursor int
	searchResultMetadata       map[string]interface{}
}

func (t *projectedExternalActionRunnerTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity:    tools.ToolIdentity{Name: "execute_action", Provider: "projected_external_action_test", Label: tools.I18nText{"en_US": "Execute Action"}},
		Description: tools.ToolDescription{LLM: "Execute one server-bound Action."},
		InputSchema: projectedExternalActionRunnerOuterSchema(),
	}
}

func (t *projectedExternalActionRunnerTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *projectedExternalActionRunnerTool) GetTenantID() string { return "" }

func (t *projectedExternalActionRunnerTool) Invoke(
	_ context.Context,
	_ string,
	toolParameters map[string]interface{},
	_ *string,
	_ *string,
	_ *string,
) ([]tools.ToolInvokeMessage, error) {
	call := copyStringAnyMap(toolParameters)
	t.calls = append(t.calls, call)
	result := map[string]interface{}{}
	if strings.Contains(strings.ToLower(evidenceStringFromAny(toolParameters["action_id"])), "contact.search") {
		refs := t.searchRecipientRefs
		if t.searchRecipientBatchCursor < len(t.searchRecipientRefBatches) {
			refs = t.searchRecipientRefBatches[t.searchRecipientBatchCursor]
			t.searchRecipientBatchCursor++
		}
		if refs == nil {
			refs = []string{"wm-alice"}
		}
		members := make([]interface{}, 0, len(refs))
		for _, ref := range refs {
			members = append(members, map[string]interface{}{"recipient_ref": ref})
		}
		result["members"] = members
		for key, value := range t.searchResultMetadata {
			result[key] = value
		}
	}
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"integration_id":   evidenceStringFromAny(toolParameters["integration_id"]),
			"action_id":        evidenceStringFromAny(toolParameters["action_id"]),
			"operation_status": "completed",
			"result_count":     projectedExternalActionRunnerResultCount(result),
			"result":           result,
		},
	}}, nil
}

func projectedExternalActionRunnerResultCount(result map[string]interface{}) int {
	for _, key := range []string{"members", "users", "messages", "items"} {
		if values := evidenceSliceFromAny(result[key]); values != nil {
			return len(values)
		}
	}
	return 1
}

func (t *projectedExternalActionRunnerTool) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *projectedExternalActionRunnerTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool { return t }

func (t *projectedExternalActionRunnerTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

func (t *projectedExternalActionRunnerTool) actionIDs() []string {
	out := make([]string, 0, len(t.calls))
	for _, call := range t.calls {
		out = append(out, evidenceStringFromAny(call["action_id"]))
	}
	return out
}

func (t *projectedExternalActionRunnerTool) integrationIDs() []string {
	out := make([]string, 0, len(t.calls))
	for _, call := range t.calls {
		out = append(out, evidenceStringFromAny(call["integration_id"]))
	}
	return out
}

func (t *projectedExternalActionRunnerTool) businessStrings(key string) []string {
	out := make([]string, 0, len(t.calls))
	for _, call := range t.calls {
		out = append(out, evidenceStringFromAny(evidenceMapFromAny(call["arguments"])[key]))
	}
	return out
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
					"preparation_hints": []interface{}{map[string]interface{}{"action_id": "wecom.contact.search"}},
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
