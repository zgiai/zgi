package skillloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestSkillPlanningStreamStreamsExplicitFinalAnswerWithoutRetract(t *testing.T) {
	index := 0
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{
			Choices: []adapter.StreamChoice{{Delta: adapter.Message{ToolCalls: []adapter.ToolCall{{
				Index: &index,
				ID:    "final-1",
				Type:  "function",
				Function: adapter.FunctionCall{
					Name:      skills.MetaToolFinalAnswer,
					Arguments: `{"answer":"\u5df2`,
				},
			}}}}},
		},
		{
			Choices: []adapter.StreamChoice{{Delta: adapter.Message{ToolCalls: []adapter.ToolCall{{
				Index: &index,
				Function: adapter.FunctionCall{
					Arguments: `\u5b8c\u6210"}`,
				},
			}}}}},
		},
		{Choices: []adapter.StreamChoice{{FinishReason: "tool_calls"}}, Done: true},
	}}}
	events := make([]Event, 0)
	runner := &Runner{
		LLMClient: client,
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})

	result, ok, err := runner.runSkillPlanningStream(context.Background(), prepared, prepared.LLMRequest, 0, nil, true, true)
	if err != nil {
		t.Fatalf("runSkillPlanningStream() error = %v", err)
	}
	if !ok {
		t.Fatal("runSkillPlanningStream() ok = false, want true")
	}
	if !result.answerStreamed {
		t.Fatal("runSkillPlanningStream() answerStreamed = false, want true")
	}
	var answer strings.Builder
	for _, event := range events {
		switch event.Type {
		case EventMessage:
			answer.WriteString(stringFromInterface(event.Payload["answer"]))
		case EventMessageRetract:
			t.Fatalf("unexpected message retract: %#v", event.Payload)
		}
	}
	if got := answer.String(); got != "\u5df2\u5b8c\u6210" {
		t.Fatalf("streamed answer = %q, want %q", got, "\u5df2\u5b8c\u6210")
	}
	if len(result.message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(result.message.ToolCalls))
	}
	args, err := skills.ParseArguments(result.message.ToolCalls[0].Function.Arguments)
	if err != nil {
		t.Fatalf("ParseArguments() error = %v", err)
	}
	if !boolArg(args, streamedFinalAnswerArg) {
		t.Fatalf("%s = %#v, want true", streamedFinalAnswerArg, args[streamedFinalAnswerArg])
	}
}

func TestSkillPlanningStreamClassifiesToolTurnBodyAsProgress(t *testing.T) {
	index := 0
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "\u6211\u5148\u786e\u8ba4\u5f53\u524d\u6587\u4ef6\u3002", ToolCalls: []adapter.ToolCall{{
			Index: &index,
			ID:    "load-1",
			Type:  "function",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolLoadSkill,
				Arguments: `{"skill_id":"file-reader"}`,
			},
		}}}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "tool_calls"}}},
		{Done: true},
	}}}
	events := make([]Event, 0)
	runner := &Runner{LLMClient: client, OnEvent: func(event Event) error {
		events = append(events, event)
		return nil
	}}
	prepared := NewPreparedChat("conv-1", "msg-1", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})
	prepared.Query = "\u8bfb\u53d6\u6587\u4ef6"

	result, ok, err := runner.runSkillPlanningStream(context.Background(), prepared, prepared.LLMRequest, 0, nil, true, true)
	if err != nil || !ok {
		t.Fatalf("runSkillPlanningStream() = ok %v, error %v", ok, err)
	}
	if result.answerStreamed {
		t.Fatal("tool-turn process body was marked as a streamed answer")
	}
	progressCount := 0
	for _, event := range events {
		switch event.Type {
		case EventMessage, EventMessageRetract:
			t.Fatalf("tool-turn process body emitted %s: %#v", event.Type, event.Payload)
		case EventAgentProgress:
			if strings.TrimSpace(stringFromInterface(event.Payload["content"])) != "" {
				progressCount++
			}
		}
	}
	if progressCount == 0 {
		t.Fatal("tool-turn process body did not produce user-visible progress")
	}
}

func TestSkillPlanningStreamRejectsLengthTermination(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "partial"}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "length"}}},
		{Done: true},
	}}}
	var trace ModelInvocationTrace
	runner := &Runner{
		LLMClient: client,
		OnModelInvocation: func(current ModelInvocationTrace) {
			trace = current
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})

	_, ok, err := runner.runSkillPlanningStream(context.Background(), prepared, prepared.LLMRequest, 0, nil, true, true)
	if err == nil || !ok {
		t.Fatalf("runSkillPlanningStream() = ok %v, error %v; want terminal error", ok, err)
	}
	if trace.FinishReason != "length" || trace.Error == "" {
		t.Fatalf("trace = %#v, want length termination recorded as error", trace)
	}
}

func TestCompletionGateTerminalBlocksPendingModelPlan(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id":     "phase-1",
				"status": "pending",
			}},
		},
	}
	decision := completionGateEvaluateTerminal(evidence, "\u5df2\u5b8c\u6210\u3002")
	if decision.Path != completionGateNeedsAction {
		t.Fatalf("completionGateEvaluateTerminal().Path = %q, want %q", decision.Path, completionGateNeedsAction)
	}
	if len(decision.MissingFacts) != 1 || decision.MissingFacts[0] != "pending_phase:phase-1" {
		t.Fatalf("completionGateEvaluateTerminal().MissingFacts = %#v, want pending phase", decision.MissingFacts)
	}
}

func TestCompletionGateTerminalUsesSubmittedPlanSnapshot(t *testing.T) {
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id":     "phase-1",
				"step":   "update agent",
				"status": "pending",
			}},
		},
		"evidence_ledger": []interface{}{map[string]interface{}{
			"runtime_id": "tool-1",
			"kind":       "tool_call",
			"status":     "success",
		}},
	}
	snapshot := []map[string]interface{}{{
		"id":            "phase-1",
		"step":          "update agent",
		"status":        "completed",
		"evidence_refs": []string{"runtime_id:tool-1"},
	}}
	if completionGateCanBeginTerminalStream(evidence) {
		t.Fatal("terminal stream started before the stored plan was completed")
	}
	decision := completionGateEvaluateTerminal(completionEvidenceWithPlanSnapshot(evidence, snapshot), "updated")
	if decision.Path != completionGateDeterministicPass {
		t.Fatalf("completionGateEvaluateTerminal().Path = %q, want %q", decision.Path, completionGateDeterministicPass)
	}
}

func TestCompletionGateTerminalBlocksUnresolvedFailure(t *testing.T) {
	evidence := map[string]interface{}{
		"turn_state": map[string]interface{}{
			"open_items": []interface{}{map[string]interface{}{
				"status": "failed",
				"error":  "tool failed",
			}},
		},
	}
	decision := completionGateEvaluateTerminal(evidence, "\u5df2\u5b8c\u6210\u3002")
	if decision.Path != completionGateNeedsAction {
		t.Fatalf("completionGateEvaluateTerminal().Path = %q, want %q", decision.Path, completionGateNeedsAction)
	}
}

func TestRunnerStreamsPlainAnswerWithoutExplicitFinalAnswerProtocol(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "\u4f60\u597d\uff01\u5f88\u9ad8\u5174\u89c1\u5230\u4f60\u3002"}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "stop"}}, Done: true},
	}}}
	events := make([]Event, 0)
	runner := &Runner{
		LLMClient:    client,
		SkillRuntime: skills.NewRuntime(nil, nil),
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-webapp", "msg-webapp", "deepseek", "auto", &adapter.ChatRequest{
		Model:    "deepseek-v4-flash",
		Messages: []adapter.Message{{Role: "user", Content: "\u4f60\u597d"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:           prepared,
		Resolved:           runnerTestResolvedSkills(),
		CompletionEvidence: func() map[string]interface{} { return map[string]interface{}{} },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "\u4f60\u597d\uff01\u5f88\u9ad8\u5174\u89c1\u5230\u4f60\u3002" {
		t.Fatalf("answer = %q, want plain streamed answer", answer)
	}
	if client.appChatStreamCalls != 1 {
		t.Fatalf("AppChatStream calls = %d, want 1", client.appChatStreamCalls)
	}
	if len(client.appChatStreamRequests) != 1 {
		t.Fatalf("stream requests = %d, want 1", len(client.appChatStreamRequests))
	}
	request := client.appChatStreamRequests[0]
	for _, tool := range request.Tools {
		if tool.Function.Name == skills.MetaToolFinalAnswer {
			t.Fatalf("plain agent chat unexpectedly exposed %s", skills.MetaToolFinalAnswer)
		}
	}
	for _, message := range request.Messages {
		content := messageContent(message.Content)
		if strings.Contains(content, "ordinary assistant content is always transient") || strings.Contains(content, "call submit_final_answer") {
			t.Fatalf("plain agent chat received explicit final-answer instructions: %q", content)
		}
	}
	messageEvents := 0
	for _, event := range events {
		if event.Type == EventMessage {
			messageEvents++
		}
		if event.Type == EventAgentProgress {
			t.Fatalf("plain answer was emitted as progress: %#v", event.Payload)
		}
	}
	if messageEvents == 0 {
		t.Fatal("plain streamed answer did not emit a message event")
	}
}

func TestRunnerUsesExplicitFinalAnswerProtocolWhenPreferred(t *testing.T) {
	index := 0
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{ToolCalls: []adapter.ToolCall{{
			Index: &index,
			ID:    "final-webapp-scope-test",
			Type:  "function",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolFinalAnswer,
				Arguments: `{"answer":"done"}`,
			},
		}}}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "tool_calls"}}, Done: true},
	}}}
	runner := &Runner{
		LLMClient:    client,
		SkillRuntime: skills.NewRuntime(nil, nil),
	}
	prepared := NewPreparedChat("conv-planned", "msg-planned", "deepseek", "auto", &adapter.ChatRequest{
		Model:    "deepseek-v4-flash",
		Messages: []adapter.Message{{Role: "user", Content: "complete the planned operation"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                  prepared,
		Resolved:                  runnerTestResolvedSkills(),
		PreferExplicitFinalAnswer: true,
		CompletionEvidence:        func() map[string]interface{} { return map[string]interface{}{} },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "done" {
		t.Fatalf("answer = %q, want explicit final answer", answer)
	}
	if client.appChatStreamCalls != 1 || len(client.appChatStreamRequests) != 1 {
		t.Fatalf("stream calls = %d, requests = %d; want one", client.appChatStreamCalls, len(client.appChatStreamRequests))
	}
	foundFinalTool := false
	foundTerminalPrompt := false
	for _, tool := range client.appChatStreamRequests[0].Tools {
		foundFinalTool = foundFinalTool || tool.Function.Name == skills.MetaToolFinalAnswer
	}
	for _, message := range client.appChatStreamRequests[0].Messages {
		foundTerminalPrompt = foundTerminalPrompt || strings.Contains(messageContent(message.Content), "call submit_final_answer")
	}
	if !foundFinalTool || !foundTerminalPrompt {
		t.Fatalf("explicit protocol request missing final tool or prompt: tool=%v prompt=%v", foundFinalTool, foundTerminalPrompt)
	}
}

func TestRunnerAcceptsPlainCandidateThroughGateWhenFinalToolPreferred(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "completed from evidence"}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "stop"}}, Done: true},
	}}}
	events := make([]Event, 0)
	runner := &Runner{
		LLMClient:    client,
		SkillRuntime: skills.NewRuntime(nil, nil),
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-fallback", "msg-fallback", "deepseek", "auto", &adapter.ChatRequest{
		Model:    "deepseek-v4-flash",
		Messages: []adapter.Message{{Role: "user", Content: "complete the planned operation"}},
	})
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id":            "phase-1",
				"step":          "complete operation",
				"status":        "completed",
				"evidence_refs": []interface{}{"runtime_id:tool-1"},
			}},
		},
		"evidence_ledger": []interface{}{map[string]interface{}{
			"runtime_id": "tool-1",
			"kind":       "tool_call",
			"status":     "success",
		}},
	}

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                  prepared,
		Resolved:                  runnerTestResolvedSkills(),
		PreferExplicitFinalAnswer: true,
		CompletionEvidence:        func() map[string]interface{} { return evidence },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "completed from evidence" {
		t.Fatalf("answer = %q, want gated plain candidate", answer)
	}
	if client.appChatStreamCalls != 1 {
		t.Fatalf("AppChatStream calls = %d, want 1", client.appChatStreamCalls)
	}
	for _, event := range events {
		if event.Type == EventAgentProgress {
			t.Fatalf("plain terminal candidate was emitted as progress: %#v", event.Payload)
		}
	}
}

func TestRunnerReturnsPendingPlanToMainModelInsteadOfRequiringFinalTool(t *testing.T) {
	client := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{
			Choices: []adapter.Choice{{
				Message: adapter.Message{Role: "assistant", Content: "current config summary"},
			}},
		},
		{
			Choices: []adapter.Choice{{
				Message: adapter.Message{
					Role: "assistant",
					ToolCalls: []adapter.ToolCall{{
						ID:   "clarify-pending-plan",
						Type: "function",
						Function: adapter.FunctionCall{
							Name:      skills.MetaToolRequestUserInput,
							Arguments: `{"message":"One decision is still needed.","questions":[{"id":"update_field","question":"Which field should be updated?","options":[{"label":"System prompt"},{"label":"Description"}]}]}`,
						},
					}},
				},
			}},
		},
	}}
	runner := &Runner{
		LLMClient:    client,
		SkillRuntime: skills.NewRuntime(nil, nil),
	}
	prepared := NewPreparedChat("conv-pending", "msg-pending", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "update the agent"}},
	})
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id":     "phase-update",
				"step":   "update agent",
				"status": "pending",
			}},
		},
	}

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                  prepared,
		Resolved:                  runnerTestResolvedSkills(),
		PreferExplicitFinalAnswer: true,
		CompletionEvidence:        func() map[string]interface{} { return evidence },
	})
	var pending *UserInputPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("Run() error = %v, want UserInputPendingError", err)
	}
	if client.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want candidate plus main-model recovery", client.appChatCalls)
	}
	request := client.appChatRequests[1]
	joined := ""
	for _, message := range request.Messages {
		joined += "\n" + messageContent(message.Content)
	}
	if !strings.Contains(joined, "pending_phase:phase-update") {
		t.Fatalf("recovery request missing pending phase feedback: %s", joined)
	}
	if strings.Contains(joined, "model did not use submit_final_answer") {
		t.Fatalf("recovery request contains obsolete final-tool failure: %s", joined)
	}
}

func TestUserInputPlanRevisionPendingUsesStructuredContinuationState(t *testing.T) {
	metadata := map[string]interface{}{
		"user_input_continuation": map[string]interface{}{
			"status":      userInputContinuationAnswered,
			"next_action": userInputContinuationReplan,
		},
	}
	req := RunRequest{CurrentMetadata: func() map[string]interface{} { return metadata }}
	if !userInputPlanRevisionPending(req) {
		t.Fatal("userInputPlanRevisionPending() = false, want true")
	}
	if !planRevisionRequiredForTool(skills.SkillAgentManagement, "get_agent_config") {
		t.Fatal("business tool should require plan revision")
	}
	if planRevisionRequiredForTool(skills.SkillAgentManagement, skills.MetaToolUpdatePlan) {
		t.Fatal("update_plan must remain available while plan revision is pending")
	}
	metadata["user_input_continuation"] = map[string]interface{}{
		"status": "plan_updated",
	}
	if userInputPlanRevisionPending(req) {
		t.Fatal("userInputPlanRevisionPending() = true after plan update")
	}
}

func TestRunnerBlocksBusinessToolUntilClarificationPlanIsRevised(t *testing.T) {
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, skills.SkillAgentManagement, `---
name: agent-management
description: Manage Agent assets.
when_to_use: Use for Agent operations.
provider_type: builtin
provider_id: agent_management
runtime_type: tool
tools:
  - get_agent_config
max_calls_per_turn: 20
---

# Agent Management
`)
	state := &runnerAgentIdentityState{}
	getConfigTool := &runnerAgentManagementGetAgentConfigTool{state: state}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{getConfigTool: getConfigTool}); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{skills.SkillAgentManagement})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	client := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{
			Choices: []adapter.Choice{{Message: adapter.Message{
				Role: "assistant",
				ToolCalls: []adapter.ToolCall{{
					ID:   "read-before-replan",
					Type: "function",
					Function: adapter.FunctionCall{
						Name:      "get_agent_config",
						Arguments: `{"agent_id":"agent-1"}`,
					},
				}},
			}}},
		},
		{
			Choices: []adapter.Choice{{Message: adapter.Message{
				Role: "assistant",
				ToolCalls: []adapter.ToolCall{{
					ID:   "clarify-after-plan-feedback",
					Type: "function",
					Function: adapter.FunctionCall{
						Name:      skills.MetaToolRequestUserInput,
						Arguments: `{"message":"The requested update still needs one choice.","questions":[{"id":"field","question":"Which field should be updated?"}]}`,
					},
				}},
			}}},
		},
	}}
	metadata := map[string]interface{}{
		"loaded_skill_ids": []interface{}{skills.SkillAgentManagement},
		"user_input_continuation": map[string]interface{}{
			"status":      userInputContinuationAnswered,
			"next_action": userInputContinuationReplan,
		},
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id":     "phase-update",
				"step":   "revise update after clarification",
				"status": "pending",
			}},
		},
	}
	runner := &Runner{LLMClient: client, SkillRuntime: runtime}
	prepared := NewPreparedChat("conv-replan", "msg-replan", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "continue after clarification"}},
	})

	_, _, err = runner.Run(context.Background(), RunRequest{
		Prepared:           prepared,
		Resolved:           resolved,
		CurrentMetadata:    func() map[string]interface{} { return metadata },
		CompletionEvidence: func() map[string]interface{} { return metadata },
	})
	var pending *UserInputPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("Run() error = %v, want UserInputPendingError", err)
	}
	if state.configCalls != 0 {
		t.Fatalf("get_agent_config calls = %d, want 0 before update_plan", state.configCalls)
	}
	if client.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want blocked business call plus clarification", client.appChatCalls)
	}
}
