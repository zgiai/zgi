package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
	workflowbuiltin "github.com/zgiai/zgi/api/internal/modules/tools/builtin/workflow"
)

func TestControlToolsForRoundHidesUnneededPlanAndFileIntermediateTools(t *testing.T) {
	input := []adapter.Tool{
		{Type: "function", Function: adapter.Function{Name: skills.MetaToolUpdatePlan}},
		{Type: "function", Function: adapter.Function{Name: skills.MetaToolIntermediateAnswer}},
		{Type: "function", Function: adapter.Function{Name: skills.MetaToolCallSkillTool}},
	}
	filtered := controlToolsForRound(input, false, false)
	if len(filtered) != 1 || filtered[0].Function.Name != skills.MetaToolCallSkillTool {
		t.Fatalf("filtered tools = %#v, want only call_skill_tool", filtered)
	}
}

func TestFileDeliveryRequiresArtifactOnlyUnlessInlineCopyRequested(t *testing.T) {
	state := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id": "phase-file", "status": "in_progress",
				"expected_action": map[string]interface{}{"skill_id": skills.SkillFileGenerator, "tool_name": "generate_file"},
			}},
		},
	}
	fileOnly := RunRequest{Prepared: &PreparedChat{Query: "生成 Markdown 文件并保存"}}
	if !fileDeliveryRequiresArtifactOnly(fileOnly, state) {
		t.Fatal("file delivery should hide full intermediate answer")
	}
	inline := RunRequest{Prepared: &PreparedChat{Query: "生成 Markdown 文件，同时在聊天中展示全文"}}
	if fileDeliveryRequiresArtifactOnly(inline, state) {
		t.Fatal("explicit inline copy should keep intermediate answer available")
	}
}

func TestRunnerInjectsRuntimeInputBeforeModelRoundInOrder(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "updated answer"}}},
	}}}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-runtime-input", "msg-runtime-input", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "original request"}},
	})
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillFileManager},
	}}}
	hookCalls := 0
	afterHookCalls := 0
	modelAccepted := false

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		BeforeModelRound: func(_ context.Context, round int) ([]adapter.Message, error) {
			hookCalls++
			if round != 0 {
				return nil, nil
			}
			return []adapter.Message{
				{Role: "system", Content: "runtime input instruction"},
				{Role: "user", Content: "first supplement"},
				{Role: "user", Content: "second supplement"},
			}, nil
		},
		AfterModelRound: func(_ context.Context, round int, accepted bool) error {
			afterHookCalls++
			if round != 0 {
				t.Fatalf("AfterModelRound() round = %d, want 0", round)
			}
			modelAccepted = accepted
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "updated answer" || hookCalls != 1 || afterHookCalls != 1 || !modelAccepted {
		t.Fatalf("answer = %q, before calls = %d, after calls = %d, accepted = %v", answer, hookCalls, afterHookCalls, modelAccepted)
	}
	if len(fakeLLM.appChatRequests) != 1 {
		t.Fatalf("model requests = %d, want 1", len(fakeLLM.appChatRequests))
	}
	messages := fakeLLM.appChatRequests[0].Messages
	var transcript strings.Builder
	for _, message := range messages {
		transcript.WriteString(messageContent(message.Content))
		transcript.WriteString("\n")
	}
	text := transcript.String()
	positions := map[string]int{}
	for _, content := range []string{"original request", "runtime input instruction", "first supplement", "second supplement"} {
		positions[content] = strings.Index(text, content)
		if positions[content] < 0 {
			t.Fatalf("messages = %#v, missing %q", messages, content)
		}
	}
	// The instruction stays where the runtime input arrived, after the
	// original request and before the supplements it introduces (ADR 0032).
	if !(positions["original request"] < positions["runtime input instruction"] &&
		positions["runtime input instruction"] < positions["first supplement"] &&
		positions["first supplement"] < positions["second supplement"]) {
		t.Fatalf("runtime input order = %#v", positions)
	}
}

func TestRunnerReportsRejectedRuntimeInputModelRound(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatErrors: []error{errors.New("provider unavailable")}}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-runtime-input-error", "msg-runtime-input-error", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "original request"}},
	})
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillFileManager},
	}}}
	afterHookCalls := 0
	modelAccepted := true

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		BeforeModelRound: func(context.Context, int) ([]adapter.Message, error) {
			return []adapter.Message{{Role: "user", Content: "supplement"}}, nil
		},
		AfterModelRound: func(_ context.Context, _ int, accepted bool) error {
			afterHookCalls++
			modelAccepted = accepted
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected model error")
	}
	if afterHookCalls != 1 || modelAccepted {
		t.Fatalf("after calls = %d, accepted = %v; want one rejected round", afterHookCalls, modelAccepted)
	}
}

func TestRunnerTerminalOnlyUsesOneFinalAnswerModelCall(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "The approved update was completed."}}},
	}}}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Instructions: strings.Repeat("large instructions ", 2000),
		Tools:        []skills.SkillToolDefinition{{Name: "update_agent_config"}},
	}}}
	prepared := NewPreparedChat("conv-terminal", "msg-terminal", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "update the Agent"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:     prepared,
		Resolved:     resolved,
		TerminalOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "The approved update was completed." {
		t.Fatalf("answer = %q", answer)
	}
	if fakeLLM.appChatCalls != 1 || len(fakeLLM.appChatRequests) != 1 {
		t.Fatalf("model calls = %d, requests = %d; want 1", fakeLLM.appChatCalls, len(fakeLLM.appChatRequests))
	}
	request := fakeLLM.appChatRequests[0]
	if len(request.Tools) != 0 || request.ToolChoice != nil {
		t.Fatalf("tools = %#v, tool_choice = %#v; want a tool-free terminal request", request.Tools, request.ToolChoice)
	}
	for _, message := range request.Messages {
		if strings.Contains(messageContent(message.Content), "large instructions") {
			t.Fatal("terminal-only request included restored business skill instructions")
		}
	}
}

func TestRunnerTerminalOnlyDoesNotRetryContentFilterTermination(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{
			Choices: []adapter.Choice{{
				FinishReason: "content_filter",
				Message:      adapter.Message{Role: "assistant"},
			}},
		},
		{
			Choices: []adapter.Choice{{Message: adapter.Message{
				Role:    "assistant",
				Content: "must not be used",
			}}},
		},
	}}
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: skills.NewRuntime(nil, nil), AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-terminal-content-filter", "msg-terminal-content-filter", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "finish"}},
	})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:     prepared,
		Resolved:     &skills.ResolvedSkills{},
		TerminalOnly: true,
	})
	var terminationErr *PlanningTerminationError
	if !errors.As(err, &terminationErr) || terminationErr == nil || terminationErr.Reason != "content_filter" {
		t.Fatalf("error = %v, want content_filter PlanningTerminationError", err)
	}
	if errors.Is(err, ErrFinalAnswerUnavailable) {
		t.Fatalf("error = %v, want the original non-retryable error", err)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("model calls = %d, want no retry for content filtering", fakeLLM.appChatCalls)
	}
}

func TestRunnerTerminalOnlyProjectsOnlyGoalAndLatestEvidence(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "done"}}},
	}}}
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: skills.NewRuntime(nil, nil), AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-terminal-projected", "msg-terminal-projected", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{
			{Role: "system", Content: "PAGE_INSTRUCTIONS FULL_PLAN_MARKER"},
			{Role: "user", Content: "USER_GOAL_MARKER"},
			{Role: "system", Content: "SKILL_INSTRUCTIONS_MARKER"},
		},
	})
	prepared.Query = "USER_GOAL_MARKER"

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:     prepared,
		Resolved:     &skills.ResolvedSkills{},
		TerminalOnly: true,
		AdditionalSystemMessages: []adapter.Message{{
			Role: "system", Content: "SKILL_ADDITIONAL_MARKER",
		}},
		CurrentMetadata: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"full_plan": "FULL_PLAN_MARKER"},
				"skill_invocations": []interface{}{map[string]interface{}{
					"kind": "tool_call", "skill_id": "agent-management", "tool_name": "update_agent_config", "status": "success",
					"result": map[string]interface{}{
						"message":       "LATEST_EVIDENCE_MARKER",
						"content":       "TOOL_RESULT_BODY_MUST_NOT_LEAK",
						"system_prompt": "TOOL_RESULT_SYSTEM_PROMPT_MUST_NOT_LEAK",
					},
				}},
				"tool_governance": map[string]interface{}{
					"status": "approved", "decision": "allowed", "message": "GOVERNANCE_SUMMARY_MARKER",
					"arguments":         map[string]interface{}{"content": "GOVERNANCE_ARGUMENT_BODY_MUST_NOT_LEAK"},
					"frozen_invocation": map[string]interface{}{"system_prompt": "FROZEN_SYSTEM_PROMPT_MUST_NOT_LEAK"},
					"system_prompt":     "GOVERNANCE_SYSTEM_PROMPT_MUST_NOT_LEAK",
					"content":           "GOVERNANCE_CONTENT_MUST_NOT_LEAK",
				},
				"generated_files": []interface{}{map[string]interface{}{
					"file_id": "file-1", "filename": "FILE_REF_MARKER.md", "content_sha256": "sha256:file",
				}},
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fakeLLM.appChatRequests) != 1 {
		t.Fatalf("requests = %d, want 1", len(fakeLLM.appChatRequests))
	}
	request := fakeLLM.appChatRequests[0]
	for _, forbidden := range []string{
		"PAGE_INSTRUCTIONS", "SKILL_INSTRUCTIONS", "SKILL_ADDITIONAL", "FULL_PLAN_MARKER",
		"TOOL_RESULT_BODY_MUST_NOT_LEAK", "TOOL_RESULT_SYSTEM_PROMPT_MUST_NOT_LEAK",
		"GOVERNANCE_ARGUMENT_BODY_MUST_NOT_LEAK", "FROZEN_SYSTEM_PROMPT_MUST_NOT_LEAK",
		"GOVERNANCE_SYSTEM_PROMPT_MUST_NOT_LEAK", "GOVERNANCE_CONTENT_MUST_NOT_LEAK",
	} {
		if runnerTestRequestContains(request, forbidden) {
			t.Fatalf("terminal-only request leaked %q: %#v", forbidden, request.Messages)
		}
	}
	for _, required := range []string{"USER_GOAL_MARKER", "LATEST_EVIDENCE_MARKER", "GOVERNANCE_SUMMARY_MARKER", "FILE_REF_MARKER.md"} {
		if !runnerTestRequestContains(request, required) {
			t.Fatalf("terminal-only request missing %q: %#v", required, request.Messages)
		}
	}
}

func TestTerminalProjectionCompactValuePrioritizesStableKeysDeterministically(t *testing.T) {
	input := map[string]interface{}{
		"status": "success", "code": "ok", "message": "done", "summary": "stable",
		"agent_id": "agent-1", "file_id": "file-1", "system_prompt_digest": "sha256:prompt",
		"operation_status": "completed", "provider_success_confirmed": true,
		"provider_result": map[string]interface{}{
			"event": map[string]interface{}{"event_id": "event-1", "summary": "未来七天日程"},
		},
		"updated_fields": []interface{}{"system_prompt", "description"},
	}
	for index := 0; index < 40; index++ {
		input[fmt.Sprintf("extra_%02d", index)] = index
	}
	first := terminalProjectionCompactValue(input, 0)
	for attempt := 0; attempt < 20; attempt++ {
		if next := terminalProjectionCompactValue(input, 0); !reflect.DeepEqual(first, next) {
			t.Fatalf("projection changed across attempts: first=%#v next=%#v", first, next)
		}
	}
	projected := evidenceMapFromAny(first)
	for _, key := range []string{"status", "code", "message", "summary", "agent_id", "file_id", "system_prompt_digest", "updated_fields"} {
		if _, ok := projected[key]; !ok {
			t.Fatalf("projection omitted priority key %q: %#v", key, projected)
		}
	}
	stable := terminalProjectionStableSummary(input)
	for _, key := range []string{"operation_status", "provider_success_confirmed", "provider_result"} {
		if _, ok := stable[key]; !ok {
			t.Fatalf("stable projection omitted provider key %q: %#v", key, stable)
		}
	}
}

func runnerTestHasTool(tools []adapter.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return true
		}
	}
	return false
}

func TestNormalizeDirectLoadedSkillToolCallWrapsUniqueLoadedTool(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Tools: []skills.SkillToolDefinition{{
			Name: "get_agent_config",
		}},
	}}}
	call := adapter.ToolCall{
		ID:   "direct-get-config",
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      "get_agent_config",
			Arguments: `{"agent_id":"agent-1"}`,
		},
	}
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		t.Fatalf("ParseArguments() error = %v", err)
	}

	normalizedCall, normalizedArgs, ok := normalizeDirectLoadedSkillToolCall(resolved, map[string]struct{}{
		skills.SkillAgentManagement: {},
	}, call, args)

	if !ok {
		t.Fatal("normalizeDirectLoadedSkillToolCall() ok = false, want true")
	}
	if got := normalizedCall.Function.Name; got != skills.MetaToolCallSkillTool {
		t.Fatalf("normalized function = %q, want %q", got, skills.MetaToolCallSkillTool)
	}
	if got := normalizedArgs["skill_id"]; got != skills.SkillAgentManagement {
		t.Fatalf("skill_id = %#v, want %q", got, skills.SkillAgentManagement)
	}
	if got := normalizedArgs["tool_name"]; got != "get_agent_config" {
		t.Fatalf("tool_name = %#v, want get_agent_config", got)
	}
	toolArgs := mapArg(normalizedArgs, "arguments")
	if got := stringArg(toolArgs, "agent_id"); got != "agent-1" {
		t.Fatalf("arguments.agent_id = %q, want agent-1", got)
	}
	wrappedArgs, err := skills.ParseArguments(normalizedCall.Function.Arguments)
	if err != nil {
		t.Fatalf("wrapped ParseArguments() error = %v", err)
	}
	if got := stringArg(wrappedArgs, "tool_name"); got != "get_agent_config" {
		t.Fatalf("wrapped tool_name = %q, want get_agent_config", got)
	}
}

func TestNormalizeDirectLoadedSkillToolCallIgnoresUnloadedTool(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Tools: []skills.SkillToolDefinition{{
			Name: "get_agent_config",
		}},
	}}}
	call := adapter.ToolCall{
		ID:   "direct-get-config",
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      "get_agent_config",
			Arguments: `{"agent_id":"agent-1"}`,
		},
	}
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		t.Fatalf("ParseArguments() error = %v", err)
	}

	normalizedCall, normalizedArgs, ok := normalizeDirectLoadedSkillToolCall(resolved, nil, call, args)

	if ok {
		t.Fatal("normalizeDirectLoadedSkillToolCall() ok = true, want false for unloaded skill")
	}
	if normalizedCall.Function.Name != call.Function.Name {
		t.Fatalf("function name changed to %q, want %q", normalizedCall.Function.Name, call.Function.Name)
	}
	if stringArg(normalizedArgs, "agent_id") != "agent-1" {
		t.Fatalf("arguments changed unexpectedly: %#v", normalizedArgs)
	}
}

func TestSkillToolCallIdentityForCallResolvesDirectLoadedTool(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Tools: []skills.SkillToolDefinition{{
			Name: "list_agent_workflow_binding_candidates",
		}},
	}}}
	call := adapter.ToolCall{
		ID:   "direct-list-workflows",
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      "list_agent_workflow_binding_candidates",
			Arguments: `{"agent_id":"agent-1"}`,
		},
	}

	skillID, toolName, toolArgs, key := skillToolCallIdentityForCall(resolved, map[string]struct{}{
		skills.SkillAgentManagement: {},
	}, call)

	if skillID != skills.SkillAgentManagement {
		t.Fatalf("skillID = %q, want %q", skillID, skills.SkillAgentManagement)
	}
	if toolName != "list_agent_workflow_binding_candidates" {
		t.Fatalf("toolName = %q, want list_agent_workflow_binding_candidates", toolName)
	}
	if got := stringArg(toolArgs, "agent_id"); got != "agent-1" {
		t.Fatalf("toolArgs.agent_id = %q, want agent-1", got)
	}
	if key == "" {
		t.Fatal("key = empty, want stable duplicate key")
	}
}

func TestGovernedReadFileTargetSystemMessageAnchorsResolvedAsset(t *testing.T) {
	message, ok := governedReadFileTargetSystemMessage(skills.SkillTrace{
		SkillID:  skills.SkillFileReader,
		ToolName: "read_file",
		Governance: &toolgovernance.Decision{
			Status: toolgovernance.DecisionStatusAllowed,
			Manifest: toolgovernance.Manifest{
				Effect:    toolgovernance.EffectRead,
				AssetType: "file",
			},
			ExpectedAssets: []toolgovernance.AssetRef{{
				ID:   "file-expected",
				Name: "second.xlsx",
				Type: "file",
			}},
		},
	})
	if !ok {
		t.Fatal("governedReadFileTargetSystemMessage() ok = false, want true")
	}
	content := fmt.Sprint(message.Content)
	for _, want := range []string{
		`resolved file target "second.xlsx"`,
		"Any earlier assistant progress text",
		"Do not mention this correction",
		"Simply answer the user's request",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("message missing %q in:\n%s", want, content)
		}
	}
}

func TestRunnerSkipsEmptyIntermediateAnswerWithoutUserVisibleError(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "empty-intermediate",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolIntermediateAnswer,
								Arguments: `{"content":""}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Query completed without changes."},
				}},
			},
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, "")
	var traces []skills.SkillTrace
	var events []Event
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			traces = append(traces, trace)
		},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "read only; do not change config"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Query completed without changes." {
		t.Fatalf("answer = %q, want final answer after skipped intermediate answer", answer)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want skipped intermediate answer plus final answer", fakeLLM.appChatCalls)
	}
	for _, trace := range traces {
		if trace.Kind == "intermediate_answer" {
			t.Fatalf("trace = %#v, want empty intermediate answer omitted from trace timeline", trace)
		}
		if strings.Contains(trace.Error, "intermediate answer content is required") {
			t.Fatalf("trace error leaked empty intermediate answer failure: %#v", trace)
		}
	}
	for _, event := range events {
		if event.Type == EventSkillCallError || event.Type == EventIntermediateAnswer {
			t.Fatalf("event = %#v, want no user-visible error/intermediate event for empty intermediate answer", event)
		}
	}
}

func TestRunnerRecordsTurnStateWithoutUserVisibleEvent(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "turn-state-1",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolTurnState,
								Arguments: `{"items":[{"kind":"working_fact","visibility":"model_only","key":"agent_theme","value":"water fee confirmation","source":"file-reader/read_file","used_for":["agent.name"]}]}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "State recorded."},
				}},
			},
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, "")
	var traces []skills.SkillTrace
	var events []Event
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			traces = append(traces, trace)
		},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "read a file, then use its theme to configure an agent"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "State recorded." {
		t.Fatalf("answer = %q, want final answer", answer)
	}
	var found *skills.SkillTrace
	for index := range traces {
		if traces[index].Kind == "turn_state" {
			found = &traces[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("traces = %#v, want turn_state trace", traces)
	}
	items := mapSliceFromAny(found.Result["items"])
	if len(items) != 1 || stringFromInterface(items[0]["key"]) != "agent_theme" || stringFromInterface(items[0]["value"]) != "water fee confirmation" {
		t.Fatalf("turn_state items = %#v, want agent_theme fact", items)
	}
	for _, event := range events {
		if event.Type == EventIntermediateAnswer {
			t.Fatalf("event = %#v, want no user-visible intermediate answer for model_only turn state", event)
		}
	}
}

func TestRunnerAcceptsOrdinaryTextFinalWhenPlanIsOpen(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:    "assistant",
						Content: "Only the file save has completed.",
					},
				}},
			},
		},
	}
	var traces []skills.SkillTrace
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntimeWithCatalog(nil, nil, ""),
		AppContext:   &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			traces = append(traces, trace)
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "save the generated file and update the agent"}},
	})
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{map[string]interface{}{
				"id":        "tool:file-manager/save_file_to_management",
				"status":    "completed",
				"skill_id":  "file-manager",
				"tool_name": "save_file_to_management",
			}},
			"step_status": map[string]interface{}{
				"tool:file-manager/save_file_to_management": "completed",
			},
		},
		"operation_result_summary": map[string]interface{}{
			"plan_status": "running",
			"status":      "running",
		},
		"evidence_ledger": []interface{}{map[string]interface{}{
			"skill_id":  "file-manager",
			"tool_name": "save_file_to_management",
			"status":    "completed",
		}},
	}

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared:             prepared,
		Resolved:             runnerTestResolvedSkills(),
		RuntimeStateSnapshot: func() map[string]interface{} { return evidence },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Only the file save has completed." {
		t.Fatalf("answer = %q, want model final answer", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one terminal decision", fakeLLM.appChatCalls)
	}
	for _, trace := range traces {
		if trace.Kind == "final_answer" && trace.Status == "success" {
			t.Fatalf("ordinary final answer unexpectedly produced final_answer trace: %#v", trace)
		}
	}
}

func TestRunnerKeepsContextualSidebarModelOnlyTurnStateHidden(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "turn-state-sidebar",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolTurnState,
								Arguments: `{"items":[{"kind":"working_fact","visibility":"model_only","key":"agent_theme","value":"雪是主角的妹妹，外向但带有怪谈般的不确定性。","source":"file-reader/read_file","used_for":["agent.prompt"]},{"kind":"decision","visibility":"model_only","key":"selected_model","value":"deepseek-v4-flash","source":"agent-management/list_models"}]}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "State recorded."},
				}},
			},
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, "")
	var events []Event
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "读取文件后用主题创建智能体"}},
	})
	prepared.Query = "读取文件后用主题创建智能体"
	prepared.Surface = "contextual_sidebar"

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "State recorded." {
		t.Fatalf("answer = %q, want final answer", answer)
	}
	for _, event := range events {
		if event.Type == EventIntermediateAnswer {
			t.Fatalf("event = %#v, want model_only turn state hidden on contextual sidebar", event)
		}
	}
}

func TestRunnerTreatsInjectedPageContextPseudoToolAsPlannerFeedback(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_page_context",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      "get_current_page_context",
								Arguments: `{}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "I used the injected page context."},
				}},
			},
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, "")
	var traces []skills.SkillTrace
	var events []Event
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			traces = append(traces, trace)
		},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "what is on this page?"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "I used the injected page context." {
		t.Fatalf("answer = %q, want final answer after planner feedback", answer)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want pseudo-tool feedback plus final answer", fakeLLM.appChatCalls)
	}
	if len(fakeLLM.appChatRequests) < 2 || !runnerTestRequestContains(fakeLLM.appChatRequests[1], "current page context is already injected") {
		t.Fatalf("second planning request missing injected-context feedback")
	}
	foundFeedback := false
	for _, trace := range traces {
		if trace.Kind == "planner_feedback" && trace.ToolName == "get_current_page_context" {
			foundFeedback = true
		}
		if trace.Status == "error" && strings.Contains(trace.ToolName, "get_current_page_context") {
			t.Fatalf("trace = %#v, want pseudo-tool as advisory feedback", trace)
		}
	}
	if !foundFeedback {
		t.Fatalf("traces = %#v, want planner feedback for pseudo page context tool", traces)
	}
	for _, event := range events {
		if event.Type == EventSkillCallError || event.Type == EventSkillLoadStart {
			t.Fatalf("event = %#v, want no user-visible tool/load event for pseudo page context tool", event)
		}
	}
}

func TestRunnerAdvisoryEvidenceDoesNotForceMissingAgentCreation(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, skills.SkillAgentManagement, `---
name: agent-management
description: Manage Agent assets.
when_to_use: Use when testing agent creation.
provider_type: builtin
provider_id: agent_management
runtime_type: tool
tools:
  - create_agent
max_calls_per_turn: 20
---

# Agent Management

Use create_agent to create an Agent.
`)
	createTool := &runnerAgentManagementCreateAgentTool{}
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Only Agent One was created; Agent Two is still missing."},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall(
								"call_create_agent_two",
								skills.SkillAgentManagement,
								"create_agent",
								map[string]interface{}{
									"name":        "Agent Two",
									"description": "Smoke agent",
								},
							),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "model final answer should not be used"},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{createTool: createTool}); err != nil {
		t.Fatalf("register agent management provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillAgentManagement})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "create two draft agents named Agent One and Agent Two"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			invocations := []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"status":    "success",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
					"result": map[string]interface{}{
						"status":     "completed",
						"effect":     "created",
						"agent_id":   "agent-1",
						"agent_name": "Agent One",
					},
				},
			}
			actions := []interface{}{
				map[string]interface{}{
					"status":    "succeeded",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
				},
			}
			progress := map[string]interface{}{
				"operation":         "agent.create",
				"status":            "partial",
				"requested_count":   2,
				"completed_count":   1,
				"completed_targets": []string{"Agent One"},
				"missing_count":     1,
				"missing_targets":   []string{"Agent Two"},
			}
			if createTool.calls > 0 {
				invocations = append(invocations, map[string]interface{}{
					"kind":      "tool_call",
					"status":    "success",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
					"result": map[string]interface{}{
						"status":     "completed",
						"effect":     "created",
						"agent_id":   "agent-2",
						"agent_name": "Agent Two",
					},
				})
				actions = append(actions, map[string]interface{}{
					"status":    "succeeded",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
				})
				progress = map[string]interface{}{
					"operation":         "agent.create",
					"status":            "completed",
					"requested_count":   2,
					"completed_count":   2,
					"completed_targets": []string{"Agent One", "Agent Two"},
					"missing_count":     0,
				}
			}
			return map[string]interface{}{
				"user_request":          "create two draft agents named Agent One and Agent Two",
				"skill_invocations":     invocations,
				"client_actions":        actions,
				"agent_create_progress": progress,
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if createTool.calls != 0 {
		t.Fatalf("create_agent calls = %d, want no backend-forced creation", createTool.calls)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want the main-model answer only", fakeLLM.appChatCalls)
	}
	if answer != "Only Agent One was created; Agent Two is still missing." {
		t.Fatalf("answer = %q, want the main model's truthful partial answer", answer)
	}
}

func TestRunnerAdvisoryPlanDoesNotForcePendingTool(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, skills.SkillAgentManagement, `---
name: agent-management
description: Manage Agent assets.
when_to_use: Use when testing agent management.
provider_type: builtin
provider_id: agent_management
runtime_type: tool
tools:
  - delete_agents
max_calls_per_turn: 20
---

# Agent Management

Use delete_agents to delete several agents in one operation.
`)
	deleteTool := &runnerAgentManagementDeleteAgentsTool{}
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "I deleted the two Agents."},
				}},
			},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"needs_action","reason":"there is no deletion evidence","unsupported_claims":["I deleted the two Agents"]}`}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I do not have evidence that the two Agents were deleted."}}}},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall(
								"call_delete_agents",
								skills.SkillAgentManagement,
								"delete_agents",
								map[string]interface{}{
									"agents": []interface{}{
										map[string]interface{}{"agent_id": "agent-1", "name": "Agent One"},
										map[string]interface{}{"agent_id": "agent-2", "name": "Agent Two"},
									},
								},
							),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "model final answer should not be used"},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{tool: deleteTool}); err != nil {
		t.Fatalf("register agent management provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillAgentManagement})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "delete the first two visible test Agents"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			planStepStatus := "pending"
			planStatus := "running"
			pendingNextAction := "agent-management/delete_agents"
			evidence := map[string]interface{}{
				"user_request": "delete the first two visible test Agents",
				"operation_plan": map[string]interface{}{
					"status":              planStatus,
					"pending_next_action": pendingNextAction,
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:agent-management/delete_agents",
							"status":    planStepStatus,
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "delete_agents",
							"asset_target": map[string]interface{}{
								"effect":         "delete",
								"asset_type":     "agent",
								"operation_mode": "batch",
							},
						},
					},
					"step_status": map[string]interface{}{
						"tool:agent-management/delete_agents": planStepStatus,
					},
				},
			}
			if deleteTool.calls > 0 {
				planStepStatus = "completed"
				planStatus = "completed"
				pendingNextAction = "none"
				result := map[string]interface{}{
					"status":        "completed",
					"skill_id":      skills.SkillAgentManagement,
					"tool_name":     "delete_agents",
					"target_count":  2,
					"deleted_count": 2,
					"item_results": []interface{}{
						map[string]interface{}{"status": "succeeded", "agent_name": "Agent One"},
						map[string]interface{}{"status": "succeeded", "agent_name": "Agent Two"},
					},
				}
				evidence["operation_plan"].(map[string]interface{})["status"] = planStatus
				evidence["operation_plan"].(map[string]interface{})["pending_next_action"] = pendingNextAction
				evidence["operation_plan"].(map[string]interface{})["step_status"] = map[string]interface{}{
					"tool:agent-management/delete_agents": planStepStatus,
				}
				evidence["operation_plan"].(map[string]interface{})["steps"] = []interface{}{
					map[string]interface{}{
						"id":        "tool:agent-management/delete_agents",
						"status":    planStepStatus,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agents",
					},
				}
				evidence["operation_result_summary"] = map[string]interface{}{
					"latest_tool_result": result,
				}
			}
			return evidence
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if deleteTool.calls != 0 {
		t.Fatalf("delete_agents calls = %d, want no backend-forced tool execution", deleteTool.calls)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one main-model call", fakeLLM.appChatCalls)
	}
	if answer != "I deleted the two Agents." {
		t.Fatalf("answer = %q, want the main-model answer", answer)
	}
}

func TestRunnerAdvisoryPlanDoesNotForceSkillLoading(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, skills.SkillAgentManagement, `---
name: agent-management
description: Manage Agent assets.
when_to_use: Use when testing agent management.
provider_type: builtin
provider_id: agent_management
runtime_type: tool
tools:
  - delete_agents
max_calls_per_turn: 20
---

# Agent Management

Use delete_agents to delete several agents in one operation.
`)
	deleteTool := &runnerAgentManagementDeleteAgentsTool{}
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "I deleted the two Agents."},
				}},
			},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"needs_action","reason":"there is no deletion evidence","unsupported_claims":["I deleted the two Agents"]}`}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I do not have evidence that the two Agents were deleted."}}}},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall(
								"call_delete_agents",
								skills.SkillAgentManagement,
								"delete_agents",
								map[string]interface{}{
									"agents": []interface{}{
										map[string]interface{}{"agent_id": "agent-1", "name": "Agent One"},
										map[string]interface{}{"agent_id": "agent-2", "name": "Agent Two"},
									},
								},
							),
						},
					},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{tool: deleteTool}); err != nil {
		t.Fatalf("register agent management provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillAgentManagement})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "delete the first two visible test Agents"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			planStepStatus := "pending"
			planStatus := "running"
			pendingNextAction := "agent-management/delete_agents"
			evidence := map[string]interface{}{
				"user_request": "delete the first two visible test Agents",
				"operation_plan": map[string]interface{}{
					"status":              planStatus,
					"pending_next_action": pendingNextAction,
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:agent-management/delete_agents",
							"status":    planStepStatus,
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "delete_agents",
						},
					},
					"step_status": map[string]interface{}{
						"tool:agent-management/delete_agents": planStepStatus,
					},
				},
			}
			if deleteTool.calls > 0 {
				result := map[string]interface{}{
					"status":        "completed",
					"skill_id":      skills.SkillAgentManagement,
					"tool_name":     "delete_agents",
					"target_count":  2,
					"deleted_count": 2,
					"item_results": []interface{}{
						map[string]interface{}{"status": "succeeded", "agent_name": "Agent One"},
						map[string]interface{}{"status": "succeeded", "agent_name": "Agent Two"},
					},
				}
				evidence["operation_plan"].(map[string]interface{})["status"] = "completed"
				evidence["operation_plan"].(map[string]interface{})["pending_next_action"] = "none"
				evidence["operation_plan"].(map[string]interface{})["step_status"] = map[string]interface{}{
					"tool:agent-management/delete_agents": "completed",
				}
				evidence["operation_plan"].(map[string]interface{})["steps"] = []interface{}{
					map[string]interface{}{
						"id":        "tool:agent-management/delete_agents",
						"status":    "completed",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agents",
					},
				}
				evidence["operation_result_summary"] = map[string]interface{}{
					"latest_tool_result": result,
				}
			}
			return evidence
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if deleteTool.calls != 0 {
		t.Fatalf("delete_agents calls = %d, want no backend-forced tool execution", deleteTool.calls)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one main-model call", fakeLLM.appChatCalls)
	}
	if answer != "I deleted the two Agents." {
		t.Fatalf("answer = %q, want the main-model answer", answer)
	}
}

func TestShouldStreamSkillPlanningIncludesQwenProvider(t *testing.T) {
	prepared := NewPreparedChat("conversation-1", "message-1", " qWeN ", "auto", &adapter.ChatRequest{Model: "qwq-plus"})

	if !shouldStreamSkillPlanning(prepared) {
		t.Fatal("shouldStreamSkillPlanning(qwen) = false, want true")
	}
}

// Synced Kimi models have the provider "moonshot"; their Agent runs used to
// fall back to waiting for the whole answer.
func TestShouldStreamSkillPlanningIncludesMoonshotProvider(t *testing.T) {
	prepared := NewPreparedChat("conversation-1", "message-1", "moonshot", "auto", &adapter.ChatRequest{Model: "kimi-k2.6"})

	if !shouldStreamSkillPlanning(prepared) {
		t.Fatal("shouldStreamSkillPlanning(moonshot) = false, want true")
	}
}

func TestShouldStreamSkillPlanningIncludesQwQModelWithoutProvider(t *testing.T) {
	prepared := NewPreparedChat("conversation-1", "message-1", "", "auto", &adapter.ChatRequest{Model: " qwen/qwq-plus "})

	if !shouldStreamSkillPlanning(prepared) {
		t.Fatal("shouldStreamSkillPlanning(qwq-plus) = false, want true")
	}
}

func TestRunnerAllowsBatchRecoverableSkillToolFailures(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a low per-turn success limit.
when_to_use: Use when testing tool call limits.
provider_type: builtin
provider_id: calculator
runtime_type: tool
tools:
  - evaluate_expression
max_calls_per_turn: 20
---

# Limited Calculator

Use the calculator tool.
`)
	toolCalls := make([]adapter.ToolCall, 0, 10)
	for i := 0; i < 10; i++ {
		toolCalls = append(toolCalls, runnerTestSkillToolCall(
			fmt.Sprintf("call_bad_%d", i),
			"limited-calculator",
			"evaluate_expression",
			map[string]interface{}{"expression": "1/"},
		))
	}
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"limited-calculator"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:      "assistant",
						ToolCalls: toolCalls,
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "replanned after batch failures"},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{"limited-calculator"})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "calculate several expressions"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want batch failures to be returned before replanning", err)
	}
	if answer != "replanned after batch failures" {
		t.Fatalf("answer = %q, want final answer after batch failure round", answer)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want 3", fakeLLM.appChatCalls)
	}
}

func TestRunnerPreservesBatchToolResponseOrderingWithoutInjectedContinuation(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	skillID := "protocol-batch"
	writeRunnerTestSkill(t, catalogDir, skillID, `---
name: protocol-batch
description: Exercise multiple tool calls in one assistant message.
when_to_use: Use when testing chat protocol ordering.
provider_type: builtin
provider_id: protocol_batch
runtime_type: tool
tools:
  - echo_value
max_calls_per_turn: 20
---

# Protocol Batch

Use echo_value to echo values.
`)
	echoTool := &runnerProtocolEchoTool{}
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: fmt.Sprintf(`{"skill_id":%q}`, skillID),
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall("call_echo_1", skillID, "echo_value", map[string]interface{}{"value": "one"}),
							runnerTestSkillToolCall("call_echo_2", skillID, "echo_value", map[string]interface{}{"value": "two"}),
							runnerTestSkillToolCall("call_echo_3", skillID, "echo_value", map[string]interface{}{"value": "three"}),
						},
					},
				}},
			},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "done"}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"pass","reason":"tool results were handled in order"}`}}}},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerProtocolEchoProvider{tool: echoTool}); err != nil {
		t.Fatalf("register protocol provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skillID})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "echo three values"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			if echoTool.calls < 3 {
				return map[string]interface{}{
					"operation_plan": map[string]interface{}{
						"status": "running",
						"steps": []interface{}{map[string]interface{}{
							"id":        "tool:" + skillID + "/echo_value",
							"skill_id":  skillID,
							"tool_name": "echo_value",
							"status":    "pending",
						}},
					},
				}
			}
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status": "completed",
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "done" {
		t.Fatalf("answer = %q, want done", answer)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want planning, tools, and the main-model final", fakeLLM.appChatCalls)
	}
	if len(fakeLLM.appChatRequests) < 3 {
		t.Fatalf("captured requests = %d, want at least 3", len(fakeLLM.appChatRequests))
	}
	reqAfterBatchTools := fakeLLM.appChatRequests[2]
	assistantIndex := -1
	for i, message := range reqAfterBatchTools.Messages {
		if message.Role == "assistant" && len(message.ToolCalls) == 3 {
			assistantIndex = i
			break
		}
	}
	if assistantIndex < 0 {
		t.Fatalf("third request messages = %#v, want assistant message with 3 tool calls", reqAfterBatchTools.Messages)
	}
	wantToolIDs := []string{"call_echo_1", "call_echo_2", "call_echo_3"}
	for offset, wantID := range wantToolIDs {
		messageIndex := assistantIndex + 1 + offset
		if messageIndex >= len(reqAfterBatchTools.Messages) {
			t.Fatalf("missing tool response %q after assistant tool calls", wantID)
		}
		message := reqAfterBatchTools.Messages[messageIndex]
		if message.Role != "tool" || message.ToolCallID != wantID {
			t.Fatalf("message after assistant at offset %d = role %q tool_call_id %q, want tool %q", offset, message.Role, message.ToolCallID, wantID)
		}
	}
}

func TestRecoverableSkillFailureMadeProgress(t *testing.T) {
	trace := func(skillID string, toolName string, category string, argumentFingerprint string, actualType string, missingFields ...string) skills.SkillTrace {
		return skills.SkillTrace{
			Kind:     "tool_call",
			SkillID:  skillID,
			ToolName: toolName,
			Status:   "error",
			Arguments: map[string]interface{}{
				"failure_category":     category,
				"argument_fingerprint": argumentFingerprint,
				"actual_type":          actualType,
				"missing_fields":       missingFields,
			},
		}
	}
	providerFailure := func(argumentFingerprint string) skills.SkillTrace {
		value := trace("ordinary-skill", "perform_task", "arguments", argumentFingerprint, "object")
		value.Arguments["recovery_kind"] = "provider_validation"
		value.Arguments["provider_error_code"] = "99992402"
		value.Arguments["invalid_fields"] = []string{"page_size"}
		return value
	}

	tests := []struct {
		name     string
		previous skills.SkillTrace
		current  skills.SkillTrace
		want     bool
	}{
		{
			name:     "identical failure is not progress",
			previous: trace("file-generator", "generate_docx", "arguments", "sha256:same", "object", "document"),
			current:  trace("file-generator", "generate_docx", "arguments", "sha256:same", "object", "document"),
		},
		{
			name:     "changed arguments are progress",
			previous: trace("file-generator", "generate_docx", "arguments", "sha256:before", "object", "document"),
			current:  trace("file-generator", "generate_docx", "arguments", "sha256:after", "object", "document"),
			want:     true,
		},
		{
			name:     "fewer missing fields are progress",
			previous: trace("file-generator", "generate_docx", "arguments", "", "object", "document", "filename"),
			current:  trace("file-generator", "generate_docx", "arguments", "", "object", "document"),
			want:     true,
		},
		{
			name:     "corrected argument type is progress",
			previous: trace("file-generator", "generate_docx", "arguments", "", "string"),
			current:  trace("file-generator", "generate_docx", "arguments", "", "object", "document"),
			want:     true,
		},
		{
			name:     "reasonable tool switch is progress",
			previous: trace("file-generator", "generate_docx", "tool", "sha256:before", "object"),
			current:  trace("file-generator", "generate_file", "tool", "sha256:after", "object"),
			want:     true,
		},
		{
			name:     "transient failure does not claim argument progress",
			previous: trace("file-generator", "generate_file", "transient", "sha256:before", "object"),
			current:  trace("file-generator", "generate_file", "transient", "sha256:after", "object"),
		},
		{
			name:     "same provider validation is not progress despite changed arguments",
			previous: providerFailure("sha256:before"),
			current:  providerFailure("sha256:after"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := recoverableSkillFailureMadeProgress(test.previous, test.current); got != test.want {
				t.Fatalf("recoverableSkillFailureMadeProgress() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRepeatedProviderValidationFailureRequiresSameCodeAndFields(t *testing.T) {
	trace := func(code string, fields []string) skills.SkillTrace {
		return skills.SkillTrace{
			Kind: "tool_call", SkillID: "ordinary-skill", ToolName: "perform_task", Status: "error",
			Arguments: map[string]interface{}{
				"recovery_kind": "provider_validation", "provider_error_code": code, "invalid_fields": fields,
			},
		}
	}
	previous := trace("99992402", []string{"page_size"})
	if !repeatedProviderValidationFailure(previous, trace("99992402", []string{"page_size"})) {
		t.Fatal("identical provider validation failure was not detected")
	}
	if repeatedProviderValidationFailure(previous, trace("99992402", []string{"start_time"})) {
		t.Fatal("different invalid field was treated as an identical provider failure")
	}
	if repeatedProviderValidationFailure(previous, trace("other", []string{"page_size"})) {
		t.Fatal("different provider error code was treated as identical")
	}
}

func TestRunnerToolFailureExplanationRetriesEmptyResponseOnce(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant"}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{
				Role:    "assistant",
				Content: "I could not create the file because the required document content was missing. Please retry or provide the content.",
			}}}},
		},
	}
	runner := &Runner{
		LLMClient:  fakeLLM,
		AppContext: &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "Create a Word document."}},
	})

	answer, _, err := runner.runToolFailureExplanation(
		context.Background(),
		RunRequest{Prepared: prepared},
		prepared,
		skills.SkillTrace{
			Kind:     "tool_call",
			SkillID:  "file-generator",
			ToolName: "generate_docx",
			Status:   "error",
			Error:    "missing required argument: document",
			Arguments: map[string]interface{}{
				"reason_code":    skillToolArgumentsMissingCode,
				"missing_fields": []string{"document"},
			},
		},
		nil,
		"repeated tool failure without progress",
		2,
	)
	if err != nil {
		t.Fatalf("runToolFailureExplanation() error = %v", err)
	}
	if !strings.Contains(answer, "could not create the file") {
		t.Fatalf("answer = %q, want model-generated failure explanation", answer)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want one controlled retry after empty response", fakeLLM.appChatCalls)
	}
	for index, request := range fakeLLM.appChatRequests {
		if len(request.Tools) != 0 || request.ToolChoice != nil {
			t.Fatalf("request %d tools = %#v choice = %#v, want explanation-only request", index, request.Tools, request.ToolChoice)
		}
	}
}

func TestRunnerToolFailureExplanationPreservesModelError(t *testing.T) {
	wantErr := errors.New("model provider unavailable")
	fakeLLM := &runnerTestLLMClient{
		appChatErrors: []error{wantErr},
	}
	runner := &Runner{
		LLMClient:  fakeLLM,
		AppContext: &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "Create a file."}},
	})

	answer, _, err := runner.runToolFailureExplanation(
		context.Background(),
		RunRequest{Prepared: prepared},
		prepared,
		skills.SkillTrace{
			Kind:     "tool_call",
			SkillID:  "file-generator",
			ToolName: "generate_file",
			Status:   "error",
			Error:    "tool execution failed",
		},
		nil,
		"tool execution failed",
		1,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runToolFailureExplanation() error = %v, want %v", err, wantErr)
	}
	if answer != "" {
		t.Fatalf("answer = %q, want no system-generated fallback answer", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want infrastructure failure returned immediately", fakeLLM.appChatCalls)
	}
}

func TestRunnerForwardsAgentWorkflowEvents(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "agent-workflow", `---
name: agent-workflow
description: Run Agent-bound workflows.
when_to_use: Use when testing Agent workflow event bridging.
provider_type: builtin
provider_id: workflow
runtime_type: tool
tools:
  - run_agent_workflow
---

# Agent Workflow

Use the workflow tool.
`)
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestDirectToolCall("call_workflow", "run_agent_workflow", map[string]interface{}{
								"binding_id": "approval-flow",
								"inputs":     map[string]interface{}{"query": "run workflow"},
							}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "workflow done"},
				}},
			},
		},
	}
	workflowRunner := &runnerTestWorkflowRunner{
		events: []automationaction.WorkflowRunEvent{
			{
				Type: EventWorkflowStarted,
				Payload: map[string]interface{}{
					"workflow_run_id": "run-1",
					"status":          "running",
				},
			},
			{
				Type: EventWorkflowNodeStarted,
				Payload: map[string]interface{}{
					"workflow_run_id": "run-1",
					"node_id":         "node-1",
					"status":          "running",
				},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(workflowbuiltin.NewProvider(func() automationaction.AutomationWorkflowRunner {
		return workflowRunner
	})); err != nil {
		t.Fatalf("register workflow provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{"agent-workflow"})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}

	var events []Event
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "run workflow"}},
	})

	toolSet := runtime.BuildNativeToolSet(ctx, resolved, skills.NativeToolSetOptions{BudgetChars: 100000})
	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared:      prepared,
		Resolved:      resolved,
		NativeToolSet: &toolSet,
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: "org-1",
			UserID:         "account-1",
			ConversationID: "conv-1",
			MessageID:      "msg-1",
			InvokeFrom:     tools.ToolInvokeFromAgent,
			RuntimeParameters: map[string]interface{}{
				"organization_id": "org-1",
				"workspace_id":    "workspace-1",
				"workflow_bindings": []map[string]interface{}{
					{
						"binding_id":       "approval-flow",
						"label":            "Approval flow",
						"agent_id":         "agent-1",
						"workflow_id":      "workflow-1",
						"version_strategy": "latest_published",
						"timeout_seconds":  60,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "workflow done" {
		t.Fatalf("answer = %q, want workflow done", answer)
	}
	workflowStarted := findRunnerTestEvent(events, EventWorkflowStarted)
	if workflowStarted == nil {
		t.Fatalf("events = %#v, want workflow_started", events)
	}
	if workflowStarted.Payload["conversation_id"] != "conv-1" || workflowStarted.Payload["message_id"] != "msg-1" {
		t.Fatalf("workflow_started payload = %#v, want conversation/message ids", workflowStarted.Payload)
	}
	nodeStarted := findRunnerTestEvent(events, EventWorkflowNodeStarted)
	if nodeStarted == nil || nodeStarted.Payload["node_id"] != "node-1" {
		t.Fatalf("events = %#v, want node_started node-1", events)
	}
}

func TestRunnerConversationalWorkflowSuccessTakesOverAnswer(t *testing.T) {
	ctx := t.Context()
	workflowRunner := &runnerTestWorkflowRunner{
		events: []automationaction.WorkflowRunEvent{
			{
				Type: EventWorkflowStarted,
				Payload: map[string]interface{}{
					"workflow_run_id": "run-delegate-success",
					"status":          "running",
				},
			},
			{
				Type: EventMessage,
				Payload: map[string]interface{}{
					"answer": "answer from ",
				},
			},
			{
				Type: EventMessage,
				Payload: map[string]interface{}{
					"answer": "workflow",
				},
			},
			{
				Type: EventWorkflowFinished,
				Payload: map[string]interface{}{
					"workflow_run_id": "run-delegate-success",
					"status":          "succeeded",
				},
			},
		},
		result: &automationaction.WorkflowRunResult{
			WorkflowRunID: "run-delegate-success",
			WorkflowID:    "workflow-1",
			AgentID:       "agent-1",
			Status:        "succeeded",
			Outputs:       map[string]interface{}{"answer": "answer from workflow"},
		},
	}
	runtime, resolved := newRunnerTestAgentWorkflowRuntime(t, ctx, workflowRunner)
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{
			runnerTestDirectToolCall("call_workflow", "run_agent_workflow", map[string]interface{}{
				"binding_id": "chat-flow", "inputs": map[string]interface{}{"query": "run workflow"},
			}),
		}}}}},
	}}
	var events []Event
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}

	answer, _, err := runner.Run(ctx, runnerTestAgentWorkflowRequest(runtime, resolved))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "answer from workflow" {
		t.Fatalf("answer = %q, want workflow answer", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want workflow to terminate Agent loop after tool call", fakeLLM.appChatCalls)
	}
	messageChunks := make([]string, 0, 2)
	unownedMessages := 0
	firstMessageIndex := -1
	workflowFinishedIndex := -1
	for index, event := range events {
		if event.Type == EventMessage {
			if event.Payload["presentation_role"] != agentWorkflowDelegatePresentationRole {
				unownedMessages++
				continue
			}
			if firstMessageIndex == -1 {
				firstMessageIndex = index
			}
			chunk, _ := event.Payload["answer"].(string)
			messageChunks = append(messageChunks, chunk)
		}
		if event.Type == EventWorkflowFinished {
			workflowFinishedIndex = index
		}
	}
	if got := strings.Join(messageChunks, ""); got != "answer from workflow" {
		t.Fatalf("streamed answer = %q, want exact workflow answer; events = %#v", got, events)
	}
	if len(messageChunks) != 2 {
		t.Fatalf("message event count = %d, want two live workflow chunks; events = %#v", len(messageChunks), events)
	}
	if unownedMessages != 0 {
		t.Fatalf("unowned message count = %d, want no duplicate parent answer; events = %#v", unownedMessages, events)
	}
	if firstMessageIndex < 0 || workflowFinishedIndex < 0 || firstMessageIndex >= workflowFinishedIndex {
		t.Fatalf("event order = %#v, want first workflow answer chunk before workflow_finished", events)
	}
}

func TestRunnerConversationalWorkflowStreamMismatchFallsBackWithoutDuplicate(t *testing.T) {
	ctx := t.Context()
	workflowRunner := &runnerTestWorkflowRunner{
		events: []automationaction.WorkflowRunEvent{
			{Type: EventMessage, Payload: map[string]interface{}{"answer": "stale workflow output"}},
			{Type: EventWorkflowFinished, Payload: map[string]interface{}{"workflow_run_id": "run-delegate-mismatch", "status": "succeeded"}},
		},
		result: &automationaction.WorkflowRunResult{
			WorkflowRunID: "run-delegate-mismatch",
			WorkflowID:    "workflow-1",
			AgentID:       "agent-1",
			Status:        "succeeded",
			Outputs:       map[string]interface{}{"answer": "authoritative workflow output"},
		},
	}
	runtime, resolved := newRunnerTestAgentWorkflowRuntime(t, ctx, workflowRunner)
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{
			runnerTestDirectToolCall("call_workflow", "run_agent_workflow", map[string]interface{}{
				"binding_id": "chat-flow", "inputs": map[string]interface{}{"query": "run workflow"},
			}),
		}}}}},
	}}
	var events []Event
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}

	answer, _, err := runner.Run(ctx, runnerTestAgentWorkflowRequest(runtime, resolved))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "authoritative workflow output" {
		t.Fatalf("answer = %q, want authoritative workflow output", answer)
	}
	retractIndex := -1
	finalIndex := -1
	for index, event := range events {
		if event.Type == EventMessageRetract && event.Payload["presentation_disposition"] == "discard" {
			retractIndex = index
		}
		if event.Type == EventMessage && event.Payload["answer"] == "authoritative workflow output" {
			finalIndex = index
		}
	}
	if retractIndex < 0 || finalIndex <= retractIndex {
		t.Fatalf("events = %#v, want stale stream retracted before authoritative fallback", events)
	}
}

func TestRunnerConversationalWorkflowFailureRetractsPartialBeforeAgentRecovery(t *testing.T) {
	ctx := t.Context()
	workflowRunner := &runnerTestWorkflowRunner{
		events: []automationaction.WorkflowRunEvent{
			{Type: EventMessage, Payload: map[string]interface{}{"answer": "partial workflow output"}},
			{Type: EventWorkflowFailed, Payload: map[string]interface{}{"workflow_run_id": "run-delegate-failed", "status": "failed"}},
		},
		result: &automationaction.WorkflowRunResult{
			WorkflowRunID: "run-delegate-failed", WorkflowID: "workflow-1", AgentID: "agent-1", Status: "failed",
		},
		err: errors.New("private provider failure"),
	}
	runtime, resolved := newRunnerTestAgentWorkflowRuntime(t, ctx, workflowRunner)
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{
			runnerTestDirectToolCall("call_workflow", "run_agent_workflow", map[string]interface{}{
				"binding_id": "chat-flow", "inputs": map[string]interface{}{"query": "run workflow"},
			}),
		}}}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "The workflow run failed."}}}},
	}}
	var events []Event
	runner := &Runner{
		LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}

	answer, _, err := runner.Run(ctx, runnerTestAgentWorkflowRequest(runtime, resolved))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "The workflow run failed." {
		t.Fatalf("answer = %q, want Agent recovery answer", answer)
	}
	retractIndex := -1
	recoveryIndex := -1
	for index, event := range events {
		if event.Type == EventMessageRetract && event.Payload["presentation_disposition"] == "discard" {
			retractIndex = index
		}
		if event.Type == EventMessage && event.Payload["answer"] == "The workflow run failed." {
			recoveryIndex = index
		}
	}
	if retractIndex < 0 || recoveryIndex <= retractIndex {
		t.Fatalf("events = %#v, want partial workflow output retracted before Agent recovery", events)
	}
}

func TestRunnerConversationalWorkflowAbnormalResultReturnsToAgentLoop(t *testing.T) {
	tests := []struct {
		name        string
		result      *automationaction.WorkflowRunResult
		err         error
		agentAnswer string
	}{
		{
			name: "failed workflow",
			result: &automationaction.WorkflowRunResult{
				WorkflowRunID: "run-delegate-failed", WorkflowID: "workflow-1", AgentID: "agent-1", Status: "failed", Outputs: map[string]interface{}{},
			},
			err:         errors.New("model route unavailable"),
			agentAnswer: "The workflow run failed.",
		},
		{
			name: "successful workflow without output",
			result: &automationaction.WorkflowRunResult{
				WorkflowRunID: "run-delegate-empty", WorkflowID: "workflow-1", AgentID: "agent-1", Status: "succeeded", Outputs: map[string]interface{}{},
			},
			agentAnswer: "The conversational workflow could not produce an answer.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			workflowRunner := &runnerTestWorkflowRunner{result: tt.result, err: tt.err}
			runtime, resolved := newRunnerTestAgentWorkflowRuntime(t, ctx, workflowRunner)
			fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
				{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{
					runnerTestDirectToolCall("call_workflow", "run_agent_workflow", map[string]interface{}{
						"binding_id": "chat-flow", "inputs": map[string]interface{}{"query": "run workflow"},
					}),
				}}}}},
				{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: tt.agentAnswer}}}},
			}}
			runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}

			answer, _, err := runner.Run(ctx, runnerTestAgentWorkflowRequest(runtime, resolved))
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if answer != tt.agentAnswer {
				t.Fatalf("answer = %q, want Agent model failure explanation", answer)
			}
			if fakeLLM.appChatCalls != 2 {
				t.Fatalf("AppChat calls = %d, want Agent loop to resume after abnormal workflow", fakeLLM.appChatCalls)
			}
			if tt.err != nil {
				if runnerTestRequestContains(fakeLLM.appChatRequests[1], "model route unavailable") {
					t.Fatal("published Agent model request exposed the workflow failure reason")
				}
				if !runnerTestRequestContains(fakeLLM.appChatRequests[1], "workflow run failed") {
					t.Fatal("published Agent model request did not contain the generic workflow failure")
				}
			}
		})
	}
}

func newRunnerTestAgentWorkflowRuntime(t *testing.T, ctx context.Context, workflowRunner automationaction.AutomationWorkflowRunner) (*skills.Runtime, *skills.ResolvedSkills) {
	t.Helper()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "agent-workflow", `---
name: agent-workflow
description: Run Agent-bound workflows.
when_to_use: Use when testing Agent workflow delegation.
provider_type: builtin
provider_id: workflow
runtime_type: tool
tools:
  - run_agent_workflow
---

# Agent Workflow

Use the workflow tool.
`)
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(workflowbuiltin.NewProvider(func() automationaction.AutomationWorkflowRunner {
		return workflowRunner
	})); err != nil {
		t.Fatalf("register workflow provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{"agent-workflow"})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	return runtime, resolved
}

func runnerTestAgentWorkflowRequest(runtime *skills.Runtime, resolved *skills.ResolvedSkills) RunRequest {
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{BudgetChars: 100000})
	return RunRequest{
		NativeToolSet: &toolSet,
		Prepared: NewPreparedChat("conv-delegate", "msg-delegate", "", "auto", &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "run workflow"}},
		}),
		Resolved: resolved,
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: "org-1", UserID: "account-1", ConversationID: "conv-delegate", MessageID: "msg-delegate", InvokeFrom: tools.ToolInvokeFromAgent,
			RuntimeParameters: map[string]interface{}{
				"organization_id": "org-1", "workspace_id": "workspace-1", "agent_runtime_source": "webapp",
				"workflow_bindings": []map[string]interface{}{{
					"binding_id": "chat-flow", "agent_id": "agent-1", "workflow_id": "workflow-1", "agent_type": "CONVERSATIONAL_WORKFLOW", "version_strategy": "latest_published", "timeout_seconds": 60,
				}},
			},
		},
	}
}

func TestRunnerBlocksProfessionalToolWithoutPromptProfessionalizer(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, skills.SkillChartGenerator, `---
name: chart-generator
description: Generate charts.
when_to_use: Use for charts.
provider_type: builtin
provider_id: chart_generator
runtime_type: tool
tools:
  - generate_chart
---

# Chart Generator

Use the chart tool.
`)
	writeRunnerTestSkill(t, catalogDir, skills.SkillPromptProfessionalizer, `---
name: prompt-professionalizer
description: Optimize prompts.
when_to_use: Use before professional tools.
runtime_type: prompt
---

# Prompt Professionalizer

Prepare professional prompts.
`)
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load_chart",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"chart-generator"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall("call_chart", skills.SkillChartGenerator, "generate_chart", map[string]interface{}{
								"chart_type": "bar",
								"data":       map[string]interface{}{"categories": []string{"A"}, "series": []map[string]interface{}{{"name": "score", "values": []int{1}}}},
							}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "blocked and replanned"},
				}},
			},
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillChartGenerator})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	if _, ok := resolved.Get(skills.SkillPromptProfessionalizer); !ok {
		t.Fatalf("prompt professionalizer should be auto-included")
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "generate a chart"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "blocked and replanned" {
		t.Fatalf("answer = %q, want replanned answer", answer)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want 3", fakeLLM.appChatCalls)
	}
}

func TestRunnerDoesNotInvokeCompletionVerifierForNeedsActionEvidence(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I saved the file."}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"needs_action","reason":"save step missing","missing_steps":["save_file_to_management"],"next_action_hint":"call file-manager/save_file_to_management"}`}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I could not confirm the save yet, so I will not claim it is saved."}}}},
		},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "save the generated file"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":              "running",
					"pending_next_action": "save_file_to_management",
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "I saved the file." {
		t.Fatalf("answer = %q, want main-model answer", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one main-model call", fakeLLM.appChatCalls)
	}
}

func TestRunnerDoesNotCallUnparseableCompletionVerifier(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Deleted the first four agents."}}}},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:             "assistant",
						Content:          "",
						ReasoningContent: "I should verify the ledger first, but I never emitted JSON before the token budget ended.",
					},
				}},
				Usage: &adapter.Usage{PromptTokens: 2800, CompletionTokens: 700, TotalTokens: 3500},
			},
		},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "delete the first four agents on this page"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status": "completed",
					"steps": []interface{}{map[string]interface{}{
						"id":        "tool:agent-management/delete_agent",
						"status":    "completed",
						"skill_id":  "agent-management",
						"tool_name": "delete_agent",
					}},
				},
				"skill_invocations": []interface{}{map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  "agent-management",
					"tool_name": "delete_agent",
					"status":    "success",
				}},
				"model_verifier_required": true,
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Deleted the first four agents." {
		t.Fatalf("answer = %q, want main-model answer", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one main-model call", fakeLLM.appChatCalls)
	}
}

func TestRunnerDoesNotAuditMainModelAnswer(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I saved the file."}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"needs_action","reason":"save evidence is missing","missing_steps":["file-manager/save_file_to_management"],"unsupported_claims":["I saved the file"],"next_action_hint":"call file-manager/save_file_to_management"}`}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I saved the file now."}}}},
		},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "save the generated file"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_ledger": []interface{}{
					map[string]interface{}{
						"kind":   "generation_context",
						"status": "observed",
					},
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "I saved the file." {
		t.Fatalf("answer = %q, want initial main-model answer", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one main-model call", fakeLLM.appChatCalls)
	}
}

func TestRunnerTerminalGuardPreservesMainModelAnswerWithPendingAdvisoryPhase(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "\u6211\u8fd8\u4e0d\u80fd\u786e\u8ba4\u8fd9\u4e2a\u64cd\u4f5c\u5df2\u7ecf\u5b8c\u6210\u3002"}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"pass","reason":"tool evidence supports completion","final_answer":"\u5df2\u5b8c\u6210\uff1a\u667a\u80fd\u4f53\u5df2\u521b\u5efa\u5e76\u914d\u7f6e\u5b8c\u6210\u3002"}`}}}},
		},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "\u521b\u5efa\u4e00\u4e2a\u667a\u80fd\u4f53\u5e76\u5b8c\u6210\u914d\u7f6e"}},
	})
	var verificationResult TerminalCompletionResult

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"model_verifier_required": true,
				"operation_plan": map[string]interface{}{
					"status": "running",
					"phases": []interface{}{map[string]interface{}{
						"id": "phase-1", "step": "Optional model progress", "status": "pending",
					}},
				},
				"skill_invocations": []interface{}{map[string]interface{}{
					"kind": "tool_call", "status": "success", "skill_id": "test-skill", "tool_name": "test_tool",
				}},
			}
		},
		OnTerminalCompletion: func(result TerminalCompletionResult) {
			verificationResult = result
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "\u6211\u8fd8\u4e0d\u80fd\u786e\u8ba4\u8fd9\u4e2a\u64cd\u4f5c\u5df2\u7ecf\u5b8c\u6210\u3002" {
		t.Fatalf("answer = %q, want the main-model candidate", answer)
	}
	if verificationResult.Status != "pass" {
		t.Fatalf("completion verification status = %q, want pass", verificationResult.Status)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one main-model call", fakeLLM.appChatCalls)
	}
}

func TestRunnerDoesNotReplaceMainModelAnswerFromFailedEvidence(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Done, the Agent was updated."}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"failed","reason":"update_agent_config failed","unsupported_claims":["Agent was updated"],"final_answer":"\u6211\u6ca1\u6709\u786e\u8ba4\u667a\u80fd\u4f53\u914d\u7f6e\u66f4\u65b0\u6210\u529f\uff1aupdate_agent_config \u8c03\u7528\u5931\u8d25\u3002"}`}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I could not confirm the Agent update because the configuration call failed."}}}},
		},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "update agent config"}},
	})
	var verificationResult TerminalCompletionResult

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "failed"},
				"skill_invocations": []interface{}{map[string]interface{}{
					"kind": "tool_call", "skill_id": "agent-management", "tool_name": "update_agent_config", "status": "error",
				}},
			}
		},
		OnTerminalCompletion: func(result TerminalCompletionResult) {
			verificationResult = result
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Done, the Agent was updated." {
		t.Fatalf("answer = %q, want initial main-model answer", answer)
	}
	if verificationResult.Status != "pass" {
		t.Fatalf("completion verification status = %q, want repaired main-model pass", verificationResult.Status)
	}
	if verificationResult.Source != "main_model_final" {
		t.Fatalf("completion verification source = %q, want main_model_final", verificationResult.Source)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one main-model call", fakeLLM.appChatCalls)
	}
}

func TestRunnerTerminalGuardDoesNotGenerateFailedPlanReplacement(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Done, the file was saved to File Management."}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"pass","reason":"candidate appears supported"}`}}}},
		},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "save file to management"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status": "failed",
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:file-manager/save_file_to_management",
							"status":    "failed",
							"skill_id":  "file-manager",
							"tool_name": "save_file_to_management",
						},
					},
				},
				"execution_ledger": map[string]interface{}{
					"skill_invocations": []interface{}{map[string]interface{}{
						"kind":      "tool_call",
						"status":    "error",
						"skill_id":  "file-manager",
						"tool_name": "save_file_to_management",
						"error":     "permission denied",
					}},
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Done, the file was saved to File Management." {
		t.Fatalf("answer = %q, want verifier-approved main-model candidate", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one main-model call", fakeLLM.appChatCalls)
	}
}

func TestRunnerCompletionVerifierSkipsWithoutEvidence(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "plain answer"}}}},
		},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared:             prepared,
		Resolved:             runnerTestResolvedSkills(),
		RuntimeStateSnapshot: func() map[string]interface{} { return map[string]interface{}{} },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "plain answer" {
		t.Fatalf("answer = %q, want plain answer", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want no verifier call", fakeLLM.appChatCalls)
	}
}

func runnerTestResolvedSkills() *skills.ResolvedSkills {
	return &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{
			ID:          "test-skill",
			Name:        "Test Skill",
			Description: "Test skill metadata",
			WhenToUse:   "Use in runner tests.",
			RuntimeType: "prompt",
		},
		Instructions: "# Test Skill\n",
	}}}
}

func writeRunnerTestSkill(t *testing.T, catalogDir string, skillID string, content string) {
	t.Helper()

	root := filepath.Join(catalogDir, skillID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir skill root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

// runnerTestDirectToolCall is a native call of a business tool by its name.
func runnerTestDirectToolCall(callID string, toolName string, arguments map[string]interface{}) adapter.ToolCall {
	payload, _ := json.Marshal(arguments)
	return adapter.ToolCall{ID: callID, Type: "function", Function: adapter.FunctionCall{Name: toolName, Arguments: string(payload)}}
}

func runnerTestSkillToolCall(callID string, skillID string, toolName string, arguments map[string]interface{}) adapter.ToolCall {
	payload, _ := json.Marshal(map[string]interface{}{
		"skill_id":  skillID,
		"tool_name": toolName,
		"arguments": arguments,
	})
	return adapter.ToolCall{
		ID:   callID,
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      skills.MetaToolCallSkillTool,
			Arguments: string(payload),
		},
	}
}

type runnerTestLLMClient struct {
	appChatResponses            []*adapter.ChatResponse
	appChatErrors               []error
	appChatRequests             []*adapter.ChatRequest
	appChatCalls                int
	appChatDelays               []time.Duration
	appChatStreams              [][]adapter.StreamResponse
	appChatStreamOpenDelays     []time.Duration
	appChatStreamResponseDelays [][]time.Duration
	appChatStreamRequests       []*adapter.ChatRequest
	appChatStreamCalls          int
}

type runnerTestWorkflowRunner struct {
	events []automationaction.WorkflowRunEvent
	result *automationaction.WorkflowRunResult
	err    error
}

type runnerGovernedFilesProvider struct {
	tool *runnerGovernedFilesDeleteTool
}

func (p *runnerGovernedFilesProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity: tools.ToolProviderIdentity{
			Name:        "governed_files_test",
			Label:       tools.I18nText{"en_US": "Governed Files Test"},
			Description: tools.I18nText{"en_US": "Governed files test provider"},
		},
		ProviderType: tools.ToolProviderTypeBuiltin,
	}
}

func (p *runnerGovernedFilesProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (p *runnerGovernedFilesProvider) GetTool(name string) (tools.Tool, error) {
	if name != "delete_file" {
		return nil, tools.ErrToolNotFound
	}
	return p.tool, nil
}

func (p *runnerGovernedFilesProvider) GetTools() []tools.Tool {
	return []tools.Tool{p.tool}
}

func (p *runnerGovernedFilesProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerGovernedFilesDeleteTool struct {
	calls []string
}

func (t *runnerGovernedFilesDeleteTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "delete_file",
			Provider: "governed_files_test",
			Label:    tools.I18nText{"en_US": "Delete File"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Delete a file"},
			LLM:   "Delete the file identified by file_id.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:        "file_id",
				Label:       tools.I18nText{"en_US": "File ID"},
				Type:        tools.ToolParameterTypeString,
				Form:        tools.ToolParameterFormLLM,
				Required:    true,
				Placeholder: tools.I18nText{"en_US": "file id"},
			},
		},
	}
}

func (t *runnerGovernedFilesDeleteTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerGovernedFilesDeleteTool) GetTenantID() string {
	return ""
}

func (t *runnerGovernedFilesDeleteTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID
	fileID, _ := toolParameters["file_id"].(string)
	t.calls = append(t.calls, fileID)
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"deleted_count": 1,
			"file_id":       fileID,
		},
	}}, nil
}

func (t *runnerGovernedFilesDeleteTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerGovernedFilesDeleteTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerGovernedFilesDeleteTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerAgentManagementProvider struct {
	tool               *runnerAgentManagementDeleteAgentsTool
	createTool         *runnerAgentManagementCreateAgentTool
	updateIdentityTool *runnerAgentManagementUpdateIdentityTool
	updateConfigTool   *runnerAgentManagementUpdateConfigTool
	getAgentTool       *runnerAgentManagementGetAgentTool
	getConfigTool      *runnerAgentManagementGetAgentConfigTool
}

func (p *runnerAgentManagementProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity: tools.ToolProviderIdentity{
			Name:        "agent_management",
			Label:       tools.I18nText{"en_US": "Agent Management"},
			Description: tools.I18nText{"en_US": "Agent management test provider"},
		},
		ProviderType: tools.ToolProviderTypeBuiltin,
	}
}

func (p *runnerAgentManagementProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (p *runnerAgentManagementProvider) GetTool(name string) (tools.Tool, error) {
	switch name {
	case "delete_agents":
		if p.tool != nil {
			return p.tool, nil
		}
	case "create_agent":
		if p.createTool != nil {
			return p.createTool, nil
		}
	case "update_agent_identity":
		if p.updateIdentityTool != nil {
			return p.updateIdentityTool, nil
		}
	case "update_agent_config":
		if p.updateConfigTool != nil {
			return p.updateConfigTool, nil
		}
	case "get_agent":
		if p.getAgentTool != nil {
			return p.getAgentTool, nil
		}
	case "get_agent_config":
		if p.getConfigTool != nil {
			return p.getConfigTool, nil
		}
	}
	return nil, tools.ErrToolNotFound
}

func (p *runnerAgentManagementProvider) GetTools() []tools.Tool {
	out := make([]tools.Tool, 0, 6)
	if p.tool != nil {
		out = append(out, p.tool)
	}
	if p.createTool != nil {
		out = append(out, p.createTool)
	}
	if p.updateIdentityTool != nil {
		out = append(out, p.updateIdentityTool)
	}
	if p.updateConfigTool != nil {
		out = append(out, p.updateConfigTool)
	}
	if p.getAgentTool != nil {
		out = append(out, p.getAgentTool)
	}
	if p.getConfigTool != nil {
		out = append(out, p.getConfigTool)
	}
	return out
}

func (p *runnerAgentManagementProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerProtocolEchoProvider struct {
	tool *runnerProtocolEchoTool
}

func (p *runnerProtocolEchoProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity: tools.ToolProviderIdentity{
			Name:        "protocol_batch",
			Label:       tools.I18nText{"en_US": "Protocol Batch"},
			Description: tools.I18nText{"en_US": "Protocol ordering test provider"},
		},
		ProviderType: tools.ToolProviderTypeBuiltin,
	}
}

func (p *runnerProtocolEchoProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (p *runnerProtocolEchoProvider) GetTool(name string) (tools.Tool, error) {
	if name == "echo_value" && p.tool != nil {
		return p.tool, nil
	}
	return nil, tools.ErrToolNotFound
}

func (p *runnerProtocolEchoProvider) GetTools() []tools.Tool {
	if p.tool == nil {
		return nil
	}
	return []tools.Tool{p.tool}
}

func (p *runnerProtocolEchoProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerProtocolEchoTool struct {
	calls int
}

func (t *runnerProtocolEchoTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "echo_value",
			Provider: "protocol_batch",
			Label:    tools.I18nText{"en_US": "Echo Value"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Echo a value"},
			LLM:   "Echo a value.",
		},
		Parameters: []tools.ToolParameter{{
			Name:     "value",
			Label:    tools.I18nText{"en_US": "Value"},
			Type:     tools.ToolParameterTypeString,
			Form:     tools.ToolParameterFormLLM,
			Required: true,
		}},
	}
}

func (t *runnerProtocolEchoTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerProtocolEchoTool) GetTenantID() string {
	return ""
}

func (t *runnerProtocolEchoTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID
	t.calls++
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status": "ok",
			"value":  fmt.Sprint(toolParameters["value"]),
		},
	}}, nil
}

func (t *runnerProtocolEchoTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerProtocolEchoTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerProtocolEchoTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerAgentManagementDeleteAgentsTool struct {
	calls int
}

func (t *runnerAgentManagementDeleteAgentsTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "delete_agents",
			Provider: "agent_management",
			Label:    tools.I18nText{"en_US": "Delete Agents"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Delete several agents"},
			LLM:   "Delete several agents in one batch operation.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:     "agents",
				Label:    tools.I18nText{"en_US": "Agents"},
				Type:     tools.ToolParameterTypeString,
				Form:     tools.ToolParameterFormLLM,
				Required: true,
			},
		},
	}
}

func (t *runnerAgentManagementDeleteAgentsTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerAgentManagementDeleteAgentsTool) GetTenantID() string {
	return ""
}

func (t *runnerAgentManagementDeleteAgentsTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = toolParameters
	_ = conversationID
	_ = appID
	_ = messageID
	t.calls++
	itemResults := []map[string]interface{}{
		{"index": 0, "status": "succeeded", "agent_name": "Agent One", "effect": "deleted"},
		{"index": 1, "status": "succeeded", "agent_name": "Agent Two", "effect": "deleted"},
	}
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":             "completed",
			"effect":             "deleted",
			"operation_type":     "agent.delete.batch",
			"operation_group_id": "agent.delete.batch:test",
			"target_count":       2,
			"deleted_count":      2,
			"failed_count":       0,
			"item_results":       itemResults,
			"requires_refresh":   true,
			"refresh_target":     "/console/agents",
			"operation_group": map[string]interface{}{
				"id":            "agent.delete.batch:test",
				"type":          "batch",
				"operation":     "agent.delete",
				"asset_type":    "agent",
				"status":        "completed",
				"target_count":  2,
				"success_count": 2,
				"failed_count":  0,
				"item_results":  itemResults,
			},
		},
	}}, nil
}

func (t *runnerAgentManagementDeleteAgentsTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerAgentManagementDeleteAgentsTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerAgentManagementDeleteAgentsTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerAgentManagementCreateAgentTool struct {
	calls int
	names []string
}

func (t *runnerAgentManagementCreateAgentTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "create_agent",
			Provider: "agent_management",
			Label:    tools.I18nText{"en_US": "Create Agent"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Create an agent"},
			LLM:   "Create an Agent with the provided name and description.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:     "name",
				Label:    tools.I18nText{"en_US": "Name"},
				Type:     tools.ToolParameterTypeString,
				Form:     tools.ToolParameterFormLLM,
				Required: true,
			},
			{
				Name:  "description",
				Label: tools.I18nText{"en_US": "Description"},
				Type:  tools.ToolParameterTypeString,
				Form:  tools.ToolParameterFormLLM,
			},
		},
	}
}

func (t *runnerAgentManagementCreateAgentTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerAgentManagementCreateAgentTool) GetTenantID() string {
	return ""
}

func (t *runnerAgentManagementCreateAgentTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID
	t.calls++
	name, _ := toolParameters["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("Agent %d", t.calls)
	}
	t.names = append(t.names, name)
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":     "completed",
			"effect":     "created",
			"agent_id":   fmt.Sprintf("agent-created-%d", t.calls),
			"agent_name": name,
		},
	}}, nil
}

func (t *runnerAgentManagementCreateAgentTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerAgentManagementCreateAgentTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerAgentManagementCreateAgentTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerAgentIdentityState struct {
	updateCalls        int
	configUpdateCalls  int
	getCalls           int
	configCalls        int
	agentID            string
	agentName          string
	description        string
	homeTitle          string
	suggestedQuestions []string
}

type runnerAgentManagementUpdateIdentityTool struct {
	state *runnerAgentIdentityState
}

func (t *runnerAgentManagementUpdateIdentityTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "update_agent_identity",
			Provider: "agent_management",
			Label:    tools.I18nText{"en_US": "Update Agent Identity"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Update Agent identity"},
			LLM:   "Update an Agent name, description, icon, or icon background color.",
		},
		Parameters: []tools.ToolParameter{
			{Name: "agent_id", Label: tools.I18nText{"en_US": "Agent ID"}, Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true},
			{Name: "name", Label: tools.I18nText{"en_US": "Name"}, Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM},
		},
	}
}

func (t *runnerAgentManagementUpdateIdentityTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerAgentManagementUpdateIdentityTool) GetTenantID() string {
	return ""
}

func (t *runnerAgentManagementUpdateIdentityTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID
	if t.state == nil {
		t.state = &runnerAgentIdentityState{}
	}
	t.state.updateCalls++
	if agentID := strings.TrimSpace(stringArg(toolParameters, "agent_id")); agentID != "" {
		t.state.agentID = agentID
	}
	if name := strings.TrimSpace(stringArg(toolParameters, "name")); name != "" {
		t.state.agentName = name
	}
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":         "completed",
			"effect":         "updated",
			"agent_id":       firstNonEmptyString(t.state.agentID, "agent-1"),
			"agent_name":     firstNonEmptyString(t.state.agentName, "Updated Agent"),
			"updated_fields": []interface{}{"name"},
		},
	}}, nil
}

func (t *runnerAgentManagementUpdateIdentityTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerAgentManagementUpdateIdentityTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerAgentManagementUpdateIdentityTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerAgentManagementUpdateConfigTool struct {
	state *runnerAgentIdentityState
}

func (t *runnerAgentManagementUpdateConfigTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "update_agent_config",
			Provider: "agent_management",
			Label:    tools.I18nText{"en_US": "Update Agent Config"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Update Agent config"},
			LLM:   "Update selected Agent runtime configuration fields.",
		},
		Parameters: []tools.ToolParameter{
			{Name: "agent_id", Label: tools.I18nText{"en_US": "Agent ID"}, Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true},
			{Name: "home_title", Label: tools.I18nText{"en_US": "Home Title"}, Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM},
			{Name: "suggested_questions", Label: tools.I18nText{"en_US": "Suggested Questions"}, Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM},
		},
	}
}

func (t *runnerAgentManagementUpdateConfigTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerAgentManagementUpdateConfigTool) GetTenantID() string {
	return ""
}

func (t *runnerAgentManagementUpdateConfigTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID
	if t.state == nil {
		t.state = &runnerAgentIdentityState{}
	}
	t.state.configUpdateCalls++
	changedFields := []interface{}{}
	if value, ok := toolParameters["home_title"].(string); ok && strings.TrimSpace(value) != "" {
		t.state.homeTitle = value
		changedFields = append(changedFields, "home_title")
	}
	switch raw := toolParameters["suggested_questions"].(type) {
	case []interface{}:
		t.state.suggestedQuestions = nil
		for _, item := range raw {
			if question := strings.TrimSpace(fmt.Sprint(item)); question != "" {
				t.state.suggestedQuestions = append(t.state.suggestedQuestions, question)
			}
		}
		changedFields = append(changedFields, "suggested_questions")
	case []string:
		t.state.suggestedQuestions = nil
		for _, item := range raw {
			if question := strings.TrimSpace(item); question != "" {
				t.state.suggestedQuestions = append(t.state.suggestedQuestions, question)
			}
		}
		changedFields = append(changedFields, "suggested_questions")
	}
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":         "completed",
			"agent_id":       firstNonEmptyString(toolParameters["agent_id"], t.state.agentID, "agent-1"),
			"updated_fields": changedFields,
		},
	}}, nil
}

func (t *runnerAgentManagementUpdateConfigTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerAgentManagementUpdateConfigTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerAgentManagementUpdateConfigTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerAgentManagementGetAgentTool struct {
	state *runnerAgentIdentityState
}

func (t *runnerAgentManagementGetAgentTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "get_agent",
			Provider: "agent_management",
			Label:    tools.I18nText{"en_US": "Get Agent"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Get Agent"},
			LLM:   "Read the current Agent identity.",
		},
		Parameters: []tools.ToolParameter{
			{Name: "agent_id", Label: tools.I18nText{"en_US": "Agent ID"}, Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true},
		},
	}
}

func (t *runnerAgentManagementGetAgentTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerAgentManagementGetAgentTool) GetTenantID() string {
	return ""
}

func (t *runnerAgentManagementGetAgentTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = toolParameters
	_ = conversationID
	_ = appID
	_ = messageID
	if t.state == nil {
		t.state = &runnerAgentIdentityState{}
	}
	t.state.getCalls++
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status": "completed",
			"agent": map[string]interface{}{
				"id":          firstNonEmptyString(t.state.agentID, "agent-1"),
				"name":        firstNonEmptyString(t.state.agentName, "Updated Agent"),
				"description": firstNonEmptyString(t.state.description, "Current Agent description"),
			},
		},
	}}, nil
}

func (t *runnerAgentManagementGetAgentTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerAgentManagementGetAgentTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerAgentManagementGetAgentTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerAgentManagementGetAgentConfigTool struct {
	state *runnerAgentIdentityState
}

func (t *runnerAgentManagementGetAgentConfigTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "get_agent_config",
			Provider: "agent_management",
			Label:    tools.I18nText{"en_US": "Get Agent Config"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Get Agent config"},
			LLM:   "Read the current Agent draft runtime configuration.",
		},
		Parameters: []tools.ToolParameter{
			{Name: "agent_id", Label: tools.I18nText{"en_US": "Agent ID"}, Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true},
		},
	}
}

func (t *runnerAgentManagementGetAgentConfigTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerAgentManagementGetAgentConfigTool) GetTenantID() string {
	return ""
}

func (t *runnerAgentManagementGetAgentConfigTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = toolParameters
	_ = conversationID
	_ = appID
	_ = messageID
	if t.state == nil {
		t.state = &runnerAgentIdentityState{}
	}
	t.state.configCalls++
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":   "completed",
			"agent_id": firstNonEmptyString(t.state.agentID, "agent-1"),
			"config": map[string]interface{}{
				"model_provider":        "openai",
				"model":                 "gpt-4o",
				"system_prompt":         "be helpful",
				"enabled_skill_ids":     []interface{}{"chart-generator"},
				"knowledge_dataset_ids": []interface{}{"kb-1"},
				"database_bindings": []interface{}{
					map[string]interface{}{"table_ids": []interface{}{"table-1", "table-2"}},
				},
				"workflow_bindings":    []interface{}{map[string]interface{}{"workflow_id": "workflow-1"}},
				"agent_memory_enabled": true,
				"file_upload_enabled":  false,
				"suggested_questions":  []interface{}{"hello"},
			},
		},
	}}, nil
}

func (t *runnerAgentManagementGetAgentConfigTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerAgentManagementGetAgentConfigTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerAgentManagementGetAgentConfigTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerFilesProvider struct {
	saveTool   *runnerFileManagerSaveTool
	deleteTool *runnerFileManagerDeleteTool
}

func (p *runnerFilesProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity: tools.ToolProviderIdentity{
			Name:        "files",
			Label:       tools.I18nText{"en_US": "Files"},
			Description: tools.I18nText{"en_US": "File management test provider"},
		},
		ProviderType: tools.ToolProviderTypeBuiltin,
	}
}

func (p *runnerFilesProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (p *runnerFilesProvider) GetTool(name string) (tools.Tool, error) {
	switch name {
	case "save_file_to_management":
		if p.saveTool != nil {
			return p.saveTool, nil
		}
	case "delete_file":
		if p.deleteTool != nil {
			return p.deleteTool, nil
		}
	}
	return nil, tools.ErrToolNotFound
}

func (p *runnerFilesProvider) GetTools() []tools.Tool {
	out := make([]tools.Tool, 0, 2)
	if p.saveTool != nil {
		out = append(out, p.saveTool)
	}
	if p.deleteTool != nil {
		out = append(out, p.deleteTool)
	}
	return out
}

func (p *runnerFilesProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerFileManagerSaveTool struct {
	calls int
}

func (t *runnerFileManagerSaveTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "save_file_to_management",
			Provider: "files",
			Label:    tools.I18nText{"en_US": "Save File to Management"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Save generated file"},
			LLM:   "Save a generated file into File Management.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:     "source_type",
				Label:    tools.I18nText{"en_US": "Source type"},
				Type:     tools.ToolParameterTypeString,
				Form:     tools.ToolParameterFormLLM,
				Required: true,
			},
			{
				Name:     "tool_file_id",
				Label:    tools.I18nText{"en_US": "Tool file ID"},
				Type:     tools.ToolParameterTypeString,
				Form:     tools.ToolParameterFormLLM,
				Required: true,
			},
			{
				Name:     "filename",
				Label:    tools.I18nText{"en_US": "Filename"},
				Type:     tools.ToolParameterTypeString,
				Form:     tools.ToolParameterFormLLM,
				Required: true,
			},
		},
	}
}

func (t *runnerFileManagerSaveTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerFileManagerSaveTool) GetTenantID() string {
	return ""
}

func (t *runnerFileManagerSaveTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID
	t.calls++
	filename := strings.TrimSpace(fmt.Sprint(toolParameters["filename"]))
	if filename == "" {
		filename = "saved.svg"
	}
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":          "completed",
			"effect":          "created",
			"target":          "managed_file",
			"file_id":         "managed-file-1",
			"upload_file_id":  "managed-file-1",
			"filename":        filename,
			"transfer_method": "local_file",
		},
	}}, nil
}

func (t *runnerFileManagerSaveTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerFileManagerSaveTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerFileManagerSaveTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type runnerFileManagerDeleteTool struct {
	calls int
}

func (t *runnerFileManagerDeleteTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "delete_file",
			Provider: "files",
			Label:    tools.I18nText{"en_US": "Delete File"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Delete managed file"},
			LLM:   "Delete a managed file from File Management.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:     "file_id",
				Label:    tools.I18nText{"en_US": "File ID"},
				Type:     tools.ToolParameterTypeString,
				Form:     tools.ToolParameterFormLLM,
				Required: true,
			},
		},
	}
}

func (t *runnerFileManagerDeleteTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runnerFileManagerDeleteTool) GetTenantID() string {
	return ""
}

func (t *runnerFileManagerDeleteTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID
	t.calls++
	fileID, _ := toolParameters["file_id"].(string)
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":        "completed",
			"deleted_count": 1,
			"file": map[string]interface{}{
				"id":        fileID,
				"name":      "aichat-plan-smoke.md",
				"extension": "md",
			},
		},
	}}, nil
}

func (t *runnerFileManagerDeleteTool) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runnerFileManagerDeleteTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *runnerFileManagerDeleteTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

func (f *runnerTestWorkflowRunner) RunAutomationWorkflow(ctx context.Context, req automationaction.WorkflowRunRequest) (*automationaction.WorkflowRunResult, error) {
	_ = ctx
	for _, event := range f.events {
		if req.EventSink != nil {
			req.EventSink(event)
		}
	}
	if f.result != nil {
		result := *f.result
		return &result, f.err
	}
	return &automationaction.WorkflowRunResult{
		WorkflowRunID: "run-1",
		WorkflowID:    req.WorkflowRef.WorkflowID,
		AgentID:       req.WorkflowRef.AgentID,
		Status:        "succeeded",
		Outputs:       map[string]interface{}{},
	}, f.err
}

func findRunnerTestEvent(events []Event, eventType string) *Event {
	for i := range events {
		if events[i].Type == eventType {
			return &events[i]
		}
	}
	return nil
}

func runnerTestRequestContains(req *adapter.ChatRequest, text string) bool {
	if req == nil {
		return false
	}
	for _, message := range req.Messages {
		if strings.Contains(messageContent(message.Content), text) {
			return true
		}
	}
	return false
}

func runnerTestRequestHasTool(req *adapter.ChatRequest, toolName string) bool {
	if req == nil {
		return false
	}
	for _, tool := range req.Tools {
		if strings.EqualFold(strings.TrimSpace(tool.Function.Name), toolName) {
			return true
		}
	}
	return false
}

func runnerTestRequestToolChoiceName(req *adapter.ChatRequest) string {
	if req == nil || req.ToolChoice == nil {
		return ""
	}
	return functionToolChoiceName(req.ToolChoice)
}

func functionToolChoiceName(choice interface{}) string {
	root, ok := choice.(map[string]interface{})
	if !ok {
		return ""
	}
	fn, ok := root["function"].(map[string]interface{})
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(fn["name"]))
}

func (f *runnerTestLLMClient) Chat(ctx context.Context, organizationID string, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) ChatStream(ctx context.Context, organizationID string, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) CreateResponse(ctx context.Context, organizationID string, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) Embed(ctx context.Context, organizationID string, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) CreateImage(ctx context.Context, organizationID string, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) Rerank(ctx context.Context, organizationID string, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	callIndex := f.appChatCalls
	f.appChatRequests = append(f.appChatRequests, cloneChatRequest(req))
	f.appChatCalls++
	if callIndex < len(f.appChatDelays) && f.appChatDelays[callIndex] > 0 {
		timer := time.NewTimer(f.appChatDelays[callIndex])
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if callIndex < len(f.appChatErrors) && f.appChatErrors[callIndex] != nil {
		return nil, f.appChatErrors[callIndex]
	}
	if callIndex >= len(f.appChatResponses) {
		return nil, errors.New("unexpected AppChat call")
	}
	return f.appChatResponses[callIndex], nil
}

func (f *runnerTestLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	callIndex := f.appChatStreamCalls
	if callIndex >= len(f.appChatStreams) {
		return nil, errors.New("unexpected AppChatStream call")
	}
	if callIndex < len(f.appChatStreamOpenDelays) && f.appChatStreamOpenDelays[callIndex] > 0 {
		timer := time.NewTimer(f.appChatStreamOpenDelays[callIndex])
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	f.appChatStreamRequests = append(f.appChatStreamRequests, cloneChatRequest(req))
	responses := append([]adapter.StreamResponse(nil), f.appChatStreams[callIndex]...)
	delays := []time.Duration(nil)
	if callIndex < len(f.appChatStreamResponseDelays) {
		delays = append(delays, f.appChatStreamResponseDelays[callIndex]...)
	}
	f.appChatStreamCalls++
	if len(delays) > 0 {
		stream := make(chan adapter.StreamResponse)
		go func() {
			defer close(stream)
			for index, response := range responses {
				if index < len(delays) && delays[index] > 0 {
					timer := time.NewTimer(delays[index])
					select {
					case <-ctx.Done():
						if !timer.Stop() {
							<-timer.C
						}
						return
					case <-timer.C:
					}
				}
				select {
				case <-ctx.Done():
					return
				case stream <- response:
				}
			}
		}()
		return stream, nil
	}
	stream := make(chan adapter.StreamResponse, len(responses))
	for _, response := range responses {
		stream <- response
	}
	close(stream)
	return stream, nil
}

func (f *runnerTestLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) AppEmbed(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) AppRerank(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}
