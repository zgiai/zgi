package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

func TestInitialLoadedSkillsForRunRestoresMetadataState(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillFileGenerator}},
	}}
	loaded := initialLoadedSkillsForRun(RunRequest{
		CurrentMetadata: func() map[string]interface{} {
			return map[string]interface{}{
				"loaded_skill_ids": []interface{}{"AGENT-MANAGEMENT", "missing-skill"},
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":     "skill_load",
						"status":   "success",
						"skill_id": skills.SkillFileGenerator,
					},
				},
			}
		},
	}, resolved)

	for _, skillID := range []string{skills.SkillAgentManagement, skills.SkillFileGenerator} {
		if _, ok := loaded[skillID]; !ok {
			t.Fatalf("loaded[%q] missing; got %#v", skillID, loaded)
		}
	}
	if _, ok := loaded["missing-skill"]; ok {
		t.Fatalf("loaded includes unresolved skill: %#v", loaded)
	}
}

func TestInitialLoadedSkillsForRunPreloadsLegacyToolChatSkills(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillFileManager}},
		{Metadata: skills.SkillMetadata{ID: skills.SkillCalculator}},
	}}
	loaded := initialLoadedSkillsForRun(RunRequest{LegacyToolChat: true}, resolved)
	for _, skillID := range resolved.SkillIDs() {
		if _, ok := loaded[skillID]; !ok {
			t.Fatalf("loaded[%q] missing; got %#v", skillID, loaded)
		}
	}
}

func TestLegacyToolChatToolsExcludeAgentPlanningProtocol(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillFileManager},
		Tools:    []skills.SkillToolDefinition{{Name: "delete_file"}},
	}}}
	loaded := initialLoadedSkillsForRun(RunRequest{LegacyToolChat: true}, resolved)
	tools := legacyToolChatTools(metaToolsForRun(resolved, loaded, false, false), false)
	if !runnerTestHasTool(tools, skills.MetaToolCallSkillTool) {
		t.Fatalf("tools = %#v, want %s", tools, skills.MetaToolCallSkillTool)
	}
	for _, excluded := range []string{
		skills.MetaToolLoadSkill,
		skills.MetaToolTurnState,
		skills.MetaToolUpdatePlan,
		skills.MetaToolIntermediateAnswer,
		skills.MetaToolFinalAnswer,
	} {
		if runnerTestHasTool(tools, excluded) {
			t.Fatalf("tools = %#v, legacy tool chat should exclude %s", tools, excluded)
		}
	}
}

func TestLegacyToolChatToolsAllowReloadOnlyWhenRestorationRequiresIt(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: skills.SkillFileManager},
		Instructions: strings.Repeat("instruction ", restoredSkillInstructionsPerSkillBudgetChars),
		Tools:        []skills.SkillToolDefinition{{Name: "delete_file"}},
	}}}
	loaded := initialLoadedSkillsForRun(RunRequest{LegacyToolChat: true}, resolved)
	state := restoredLoadedSkillInstructionState(resolved, loaded)
	if len(state.reloadRequired) != 1 {
		t.Fatalf("reloadRequired = %#v, want one oversized skill", state.reloadRequired)
	}

	tools := legacyToolChatTools(metaToolsForRun(resolved, state.activeLoaded, false, false), true)
	if !runnerTestHasTool(tools, skills.MetaToolLoadSkill) {
		t.Fatalf("tools = %#v, want %s recovery path", tools, skills.MetaToolLoadSkill)
	}
	if runnerTestHasTool(legacyToolChatTools(tools, false), skills.MetaToolLoadSkill) {
		t.Fatalf("tools = %#v, load_skill should remain hidden without a reload requirement", tools)
	}
}

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

func TestOperationPlanModelRevisionRequiredOnlyForStalePlan(t *testing.T) {
	req := RunRequest{CurrentMetadata: func() map[string]interface{} { return map[string]interface{}{} }}
	if operationPlanModelRevisionRequired(req, map[string]interface{}{
		"operation_plan": map[string]interface{}{"plan_sync_status": "current"},
	}) {
		t.Fatal("current plan unexpectedly requires model revision")
	}
	if !operationPlanModelRevisionRequired(req, map[string]interface{}{
		"operation_plan": map[string]interface{}{"plan_sync_status": "stale"},
	}) {
		t.Fatal("stale plan should require model revision")
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

func TestRunnerLegacyToolChatUsesSimpleToolContract(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "plain answer"}}},
	}}}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: skills.SkillFileManager},
		Instructions: "# File Manager\nUse exact file IDs.",
		Tools:        []skills.SkillToolDefinition{{Name: "delete_file"}},
	}}}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:       prepared,
		Resolved:       resolved,
		LegacyToolChat: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "plain answer" {
		t.Fatalf("answer = %q, want plain answer", answer)
	}
	if len(fakeLLM.appChatRequests) != 1 {
		t.Fatalf("AppChat requests = %d, want 1", len(fakeLLM.appChatRequests))
	}
	request := fakeLLM.appChatRequests[0]
	if !runnerTestHasTool(request.Tools, skills.MetaToolCallSkillTool) {
		t.Fatalf("tools = %#v, want %s", request.Tools, skills.MetaToolCallSkillTool)
	}
	if runnerTestHasTool(request.Tools, skills.MetaToolUpdatePlan) || runnerTestHasTool(request.Tools, skills.MetaToolLoadSkill) {
		t.Fatalf("tools = %#v, want no Agent planning or dynamic-loading tools", request.Tools)
	}
	foundLegacyPrompt := false
	for _, message := range request.Messages {
		if strings.Contains(evidenceStringFromAny(message.Content), "Use the already available tools only when they are needed") {
			foundLegacyPrompt = true
			break
		}
	}
	if !foundLegacyPrompt {
		t.Fatalf("messages = %#v, want legacy tool-chat system prompt", request.Messages)
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

func TestRunnerTerminalOnlyFallsBackFromEmptyModelResponseWhenOperationCompleted(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant"}}},
	}}}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-terminal-empty", "msg-terminal-empty", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "finish"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:          prepared,
		Resolved:          &skills.ResolvedSkills{},
		ProtocolToolsOnly: true,
		TerminalOnly:      true,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{"operation_plan": map[string]interface{}{"status": "completed"}}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "The operation was completed." {
		t.Fatalf("answer = %q, want deterministic completed-operation fallback", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("model calls = %d, want 1", fakeLLM.appChatCalls)
	}
}

func TestRunnerTerminalOnlyFallsBackFromInventedToolAfterCompletedOperation(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{
			ID:   "invented-file-list",
			Type: "function",
			Function: adapter.FunctionCall{
				Name:      "file_list",
				Arguments: `{}`,
			},
		}}}}},
	}}}
	var traces []skills.SkillTrace
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			traces = append(traces, trace)
		},
	}
	prepared := NewPreparedChat("conv-terminal-tool", "msg-terminal-tool", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "生成第十一章文件并更新智能体提示词"}},
	})
	prepared.Query = "生成第十一章文件并更新智能体提示词"

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:          prepared,
		Resolved:          &skills.ResolvedSkills{},
		ProtocolToolsOnly: true,
		TerminalOnly:      true,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "completed"},
			}
		},
		CurrentMetadata: func() map[string]interface{} {
			return map[string]interface{}{
				"generated_files": []interface{}{map[string]interface{}{
					"file_id": "file-11", "filename": "第十一章.md", "target": "managed_file",
				}},
				"skill_invocations": []interface{}{map[string]interface{}{
					"kind": "tool_call", "skill_id": "agent-management", "tool_name": "update_agent_config", "status": "success",
					"result": map[string]interface{}{
						"status": "completed", "agent_name": "灵澜学院说书人", "updated_fields": []interface{}{"system_prompt"},
					},
				}},
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "文件「第十一章.md」已生成并保存；智能体「灵澜学院说书人」的系统提示词已更新。"
	if answer != want {
		t.Fatalf("answer = %q, want %q", answer, want)
	}
	if len(fakeLLM.appChatRequests) != 1 || len(fakeLLM.appChatRequests[0].Tools) != 0 {
		t.Fatalf("requests = %#v, want one tool-free terminal request", fakeLLM.appChatRequests)
	}
	foundFallback := false
	for _, trace := range traces {
		if trace.Kind == "final_answer" && trace.Status == "success" && trace.Arguments["fallback"] == true {
			foundFallback = true
			if trace.Arguments["fallback_reason"] != "unexpected_tool_call" {
				t.Fatalf("fallback trace = %#v, want unexpected_tool_call reason", trace)
			}
		}
	}
	if !foundFallback {
		t.Fatalf("traces = %#v, want successful fallback final_answer trace", traces)
	}
}

func TestRunnerTerminalOnlyDoesNotRetryTruncatedModelResponse(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{}}
	fakeLLM.appChatResponses = append(fakeLLM.appChatResponses, &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			FinishReason: "length",
			Message:      adapter.Message{Role: "assistant", Content: "partial"},
		}},
	})
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: skills.NewRuntime(nil, nil), AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-terminal-length", "msg-terminal-length", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "finish once"}},
	})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:     prepared,
		Resolved:     &skills.ResolvedSkills{},
		TerminalOnly: true,
	})
	if err == nil || !strings.Contains(err.Error(), "length") {
		t.Fatalf("error = %v, want terminal length error", err)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("model calls = %d, want exactly one terminal call", fakeLLM.appChatCalls)
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
}

func TestRunnerMovesAndMergesSystemMessagesBeforePlanning(t *testing.T) {
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "done"}}},
	}}}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-system-order", "msg-system-order", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{
			{Role: "system", Content: "base system"},
			{Role: "user", Content: "question"},
			{Role: "assistant", Content: "earlier answer"},
		},
	})
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillFileManager},
		Tools:    []skills.SkillToolDefinition{{Name: "list_files"}},
	}}}

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:       prepared,
		Resolved:       resolved,
		LegacyToolChat: true,
		AdditionalSystemMessages: []adapter.Message{
			{Role: "system", Content: "continuation guidance"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fakeLLM.appChatRequests) != 1 {
		t.Fatalf("AppChat requests = %d, want 1", len(fakeLLM.appChatRequests))
	}

	messages := fakeLLM.appChatRequests[0].Messages
	if len(messages) < 3 || messages[0].Role != "system" {
		t.Fatalf("planning messages = %#v, want one leading system message", messages)
	}
	systemContent := messageContent(messages[0].Content)
	for _, want := range []string{
		"base system",
		"continuation guidance",
		"Use the already available tools only when they are needed",
	} {
		if !strings.Contains(systemContent, want) {
			t.Fatalf("system content missing %q:\n%s", want, systemContent)
		}
	}
	for index, message := range messages[1:] {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			t.Fatalf("planning message %d = %#v, want all system messages merged at the beginning", index+1, message)
		}
	}
	if messages[1].Role != "user" || messages[2].Role != "assistant" {
		t.Fatalf("conversation order = %#v, want user then assistant", messages[1:])
	}
}

func TestBuiltInToolSkillRequiresInstructionLoad(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: skills.SkillFileManager, Source: skills.SkillSourceSystem, RuntimeType: skills.SkillRuntimeTypeTool},
		Instructions: "# File Manager\nDo not invent file IDs.",
		Tools:        []skills.SkillToolDefinition{{Name: "delete_file"}},
	}}}
	loaded := initialLoadedSkillsForRun(RunRequest{}, resolved)
	if _, ok := loaded[skills.SkillFileManager]; ok {
		t.Fatalf("built-in skill was marked loaded without its instructions: %#v", loaded)
	}

	tools := metaToolsForRun(resolved, loaded, true, false)
	hasTool := func(name string) bool {
		for _, tool := range tools {
			if strings.EqualFold(strings.TrimSpace(tool.Function.Name), name) {
				return true
			}
		}
		return false
	}
	if !hasTool(skills.MetaToolLoadSkill) {
		t.Fatalf("tools = %#v, want %s", tools, skills.MetaToolLoadSkill)
	}
	if hasTool(skills.MetaToolCallSkillTool) {
		t.Fatalf("tools = %#v, should not expose %s before instructions load", tools, skills.MetaToolCallSkillTool)
	}
}

func TestInitialLoadedSkillsForRunTreatsSuccessfulToolCallAsLoaded(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
	}}}
	loaded := initialLoadedSkillsForRun(RunRequest{
		CurrentMetadata: func() map[string]interface{} {
			return map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"status":    "success",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "create_agent",
					},
				},
			}
		},
	}, resolved)
	if _, ok := loaded[skills.SkillAgentManagement]; !ok {
		t.Fatalf("loaded[%q] missing after successful tool_call; got %#v", skills.SkillAgentManagement, loaded)
	}
}

func TestRestoredLoadedSkillInstructionStateRehydratesCompleteContinuation(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{
			Metadata: skills.SkillMetadata{
				ID:          skills.SkillAgentManagement,
				Description: "Manage agents.",
				WhenToUse:   "Use for Agent configuration.",
			},
			Instructions: "# Agent Management\nPreserve unmentioned settings.",
		},
		{
			Metadata:     skills.SkillMetadata{ID: skills.SkillFileReader},
			Instructions: "# File Reader\nRead exact file content.",
		},
	}}
	state := restoredLoadedSkillInstructionState(resolved, map[string]struct{}{
		skills.SkillAgentManagement: {},
	})
	if state.message == nil {
		t.Fatal("restoredLoadedSkillInstructionState().message = nil")
	}
	content := messageContent(state.message.Content)
	for _, want := range []string{
		"loaded earlier in this same user turn",
		skills.SkillAgentManagement,
		"Preserve unmentioned settings.",
		"complete instructions appear below",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("restored message missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "Read exact file content.") {
		t.Fatalf("restored message included an unloaded skill:\n%s", content)
	}
	if _, ok := state.activeLoaded[skills.SkillAgentManagement]; !ok {
		t.Fatalf("activeLoaded[%q] missing: %#v", skills.SkillAgentManagement, state.activeLoaded)
	}
	if len(state.reloadRequired) != 0 {
		t.Fatalf("reloadRequired = %#v, want none", state.reloadRequired)
	}
}

func TestRestoredLoadedSkillInstructionStateReopensOversizedSkill(t *testing.T) {
	fullInstructions := "BEGIN\n" + strings.Repeat("authoritative contract\n", 1400) + "END"
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Instructions: fullInstructions,
		Tools:        []skills.SkillToolDefinition{{Name: "update_agent_config"}},
	}}}

	state := restoredLoadedSkillInstructionState(resolved, map[string]struct{}{
		skills.SkillAgentManagement: {},
	})

	if _, ok := state.activeLoaded[skills.SkillAgentManagement]; ok {
		t.Fatalf("activeLoaded[%q] present for oversized instructions", skills.SkillAgentManagement)
	}
	if !reflect.DeepEqual(state.reloadRequired, []string{skills.SkillAgentManagement}) {
		t.Fatalf("reloadRequired = %#v, want agent-management", state.reloadRequired)
	}
	content := messageContent(state.message.Content)
	if strings.Contains(content, "authoritative contract") {
		t.Fatalf("restored message included partial oversized instructions:\n%s", content)
	}
	if !strings.Contains(content, "requiring full reload") {
		t.Fatalf("restored message omitted reload guidance:\n%s", content)
	}
	tools := metaToolsForRun(resolved, state.activeLoaded, true, false)
	if !runnerTestHasTool(tools, skills.MetaToolLoadSkill) {
		t.Fatalf("tools = %#v, want load_skill for oversized restored skill", tools)
	}
	if runnerTestHasTool(tools, skills.MetaToolCallSkillTool) {
		t.Fatalf("tools = %#v, call_skill_tool exposed before full reload", tools)
	}
}

func TestRestoredLoadedSkillInstructionStateRestoresPreferredOversizedSkill(t *testing.T) {
	fullInstructions := "BEGIN\n" + strings.Repeat("authoritative continuation contract\n", 1400) + "END"
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillFileGenerator}, Instructions: strings.Repeat("small\n", 100)},
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}, Instructions: fullInstructions},
	}}

	state := restoredLoadedSkillInstructionStateForRun(resolved, map[string]struct{}{
		skills.SkillFileGenerator:   {},
		skills.SkillAgentManagement: {},
	}, skills.SkillAgentManagement)

	if _, ok := state.activeLoaded[skills.SkillAgentManagement]; !ok {
		t.Fatalf("activeLoaded = %#v, want preferred oversized skill restored", state.activeLoaded)
	}
	if !strings.Contains(messageContent(state.message.Content), fullInstructions) {
		t.Fatal("preferred oversized instructions were not restored completely")
	}
	if slices.Contains(state.reloadRequired, skills.SkillAgentManagement) {
		t.Fatalf("reloadRequired = %#v, preferred skill must not require reload", state.reloadRequired)
	}
}

func TestInitialLoadedSkillsRejectsChangedInstructionDigest(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Instructions: "new instructions",
	}}}
	req := RunRequest{CurrentMetadata: func() map[string]interface{} {
		return map[string]interface{}{
			"loaded_skill_ids": []interface{}{skills.SkillAgentManagement},
			"loaded_skill_state": []interface{}{map[string]interface{}{
				"skill_id":           skills.SkillAgentManagement,
				"instruction_digest": skillInstructionDigest("old instructions"),
			}},
		}
	}}

	loaded := initialLoadedSkillsForRun(req, resolved)
	if _, ok := loaded[skills.SkillAgentManagement]; ok {
		t.Fatalf("loaded = %#v, changed instructions must require load_skill", loaded)
	}
}

func TestValidatedHistoricalLoadedSkillsRecordsVersionAndPolicyFailures(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement}, Instructions: "new instructions"},
		{Metadata: skills.SkillMetadata{ID: skills.SkillFileReader}, Instructions: "reader instructions"},
	}}
	req := RunRequest{
		CurrentMetadata: func() map[string]interface{} {
			return map[string]interface{}{
				"loaded_skill_ids": []interface{}{skills.SkillAgentManagement, skills.SkillFileReader, "disabled-skill"},
				"loaded_skill_state": []interface{}{
					map[string]interface{}{"skill_id": skills.SkillAgentManagement, "effective_version": skillInstructionDigest("old instructions")},
					map[string]interface{}{"skill_id": skills.SkillFileReader, "effective_version": skillInstructionDigest("reader instructions")},
					map[string]interface{}{"skill_id": "disabled-skill", "effective_version": "sha256:old"},
				},
			}
		},
		AuthorizeSkillStep: func(_ context.Context, skillID string) (bool, error) {
			return skillID != skills.SkillFileReader, nil
		},
	}

	loaded, traces := validatedHistoricalLoadedSkillsForRun(context.Background(), req, resolved)
	if len(loaded) != 0 {
		t.Fatalf("loaded = %#v, want all invalidated", loaded)
	}
	outcomes := map[string]string{}
	statuses := map[string]string{}
	for _, trace := range traces {
		outcomes[trace.SkillID] = evidenceStringFromAny(trace.Arguments["outcome"])
		statuses[trace.SkillID] = trace.Status
		if trace.Kind != "skill_load_attempt" {
			t.Fatalf("trace = %#v, want diagnostic load attempt", trace)
		}
	}
	if outcomes[skills.SkillAgentManagement] != "version_changed" || statuses[skills.SkillAgentManagement] != "reload_required" {
		t.Fatalf("agent-management diagnostic = %q/%q, want version_changed/reload_required", outcomes[skills.SkillAgentManagement], statuses[skills.SkillAgentManagement])
	}
	if outcomes[skills.SkillFileReader] != "policy_denied" || statuses[skills.SkillFileReader] != "blocked" {
		t.Fatalf("file-reader diagnostic = %q/%q, want policy_denied/blocked", outcomes[skills.SkillFileReader], statuses[skills.SkillFileReader])
	}
	if outcomes["disabled-skill"] != "not_exposed_current_surface" || statuses["disabled-skill"] != "skipped" {
		t.Fatalf("disabled-skill diagnostic = %q/%q, want not_exposed_current_surface/skipped", outcomes["disabled-skill"], statuses["disabled-skill"])
	}
}

func TestRestoredSkillLoadAttemptsHaveUniqueRuntimeIDs(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: skills.SkillAgentManagement},
		Instructions: "current instructions",
	}}}
	metadata := map[string]interface{}{
		"loaded_skill_state": []interface{}{map[string]interface{}{
			"skill_id": skills.SkillAgentManagement, "load_sequence": 7,
		}},
	}
	state := restoredSkillInstructionState{restored: []string{skills.SkillAgentManagement}}
	first := restoredSkillAttemptTraces(metadata, resolved, state)
	second := restoredSkillAttemptTraces(metadata, resolved, state)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("attempts = %#v / %#v, want one each", first, second)
	}
	firstID := evidenceStringFromAny(first[0].Arguments["runtime_id"])
	secondID := evidenceStringFromAny(second[0].Arguments["runtime_id"])
	if firstID == "" || secondID == "" || firstID == secondID {
		t.Fatalf("runtime ids = %q / %q, want unique non-empty ids", firstID, secondID)
	}
	if first[0].Status != "auto_restored" || first[0].Arguments["load_sequence"] != 7 {
		t.Fatalf("attempt = %#v, want auto_restored with prior load sequence", first[0])
	}
	deniedOne := restoredSkillValidationTrace(skills.SkillAgentManagement, "policy_denied", nil, "denied", "denied")
	deniedTwo := restoredSkillValidationTrace(skills.SkillAgentManagement, "policy_denied", nil, "denied", "denied")
	if evidenceStringFromAny(deniedOne.Arguments["runtime_id"]) == evidenceStringFromAny(deniedTwo.Arguments["runtime_id"]) {
		t.Fatalf("policy denial attempts reused runtime id: %#v / %#v", deniedOne, deniedTwo)
	}
}

func TestRestoredLoadedSkillInstructionStateReopensOnlySkillsBeyondTotalBudget(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: "skill-a"}, Instructions: strings.Repeat("a", 9000)},
		{Metadata: skills.SkillMetadata{ID: "skill-b"}, Instructions: strings.Repeat("b", 8000)},
	}}
	historical := map[string]struct{}{"skill-a": {}, "skill-b": {}}

	state := restoredLoadedSkillInstructionState(resolved, historical)

	if _, ok := state.activeLoaded["skill-a"]; !ok {
		t.Fatalf("activeLoaded = %#v, want skill-a fully restored", state.activeLoaded)
	}
	if _, ok := state.activeLoaded["skill-b"]; ok {
		t.Fatalf("activeLoaded = %#v, skill-b exceeded remaining total budget", state.activeLoaded)
	}
	if !reflect.DeepEqual(state.reloadRequired, []string{"skill-b"}) {
		t.Fatalf("reloadRequired = %#v, want [skill-b]", state.reloadRequired)
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

func TestRunNeverRequiresFinalPlanSnapshotForOutcomePlan(t *testing.T) {
	req := RunRequest{CurrentMetadata: func() map[string]interface{} {
		return map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"phases": []interface{}{map[string]interface{}{
					"id":     "phase-1",
					"step":   "Complete the task",
					"status": "in_progress",
				}},
			},
		}
	}}
	if runRequiresFinalPlanSnapshot(req) {
		t.Fatal("runRequiresFinalPlanSnapshot() = true, want optional audit snapshot")
	}
	req.CurrentMetadata = func() map[string]interface{} {
		return map[string]interface{}{"operation_plan": map[string]interface{}{"phases": []interface{}{}}}
	}
	if runRequiresFinalPlanSnapshot(req) {
		t.Fatal("runRequiresFinalPlanSnapshot() = true for empty phases")
	}
}

func TestShouldEmitNaturalProgressOnlyForExecutableBusinessTools(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{
			Metadata: skills.SkillMetadata{ID: skills.SkillAgentManagement},
			Tools:    []skills.SkillToolDefinition{{Name: "get_agent_config"}},
		},
		{
			Metadata: skills.SkillMetadata{ID: skills.SkillConsoleNavigator},
			Tools:    []skills.SkillToolDefinition{{Name: "navigate"}},
		},
	}}
	loaded := map[string]struct{}{skills.SkillAgentManagement: {}}
	loadCall := adapter.ToolCall{Function: adapter.FunctionCall{
		Name:      skills.MetaToolLoadSkill,
		Arguments: `{"skill_id":"agent-management"}`,
	}}
	if shouldEmitNaturalProgressForToolCalls(resolved, loaded, []adapter.ToolCall{loadCall}) {
		t.Fatal("load_skill-only turn was treated as user-visible business progress")
	}
	planCall := adapter.ToolCall{Function: adapter.FunctionCall{
		Name:      skills.MetaToolUpdatePlan,
		Arguments: `{"plan":[]}`,
	}}
	if shouldEmitNaturalProgressForToolCalls(resolved, loaded, []adapter.ToolCall{planCall}) {
		t.Fatal("update_plan-only turn was treated as user-visible business progress")
	}
	businessCall := runnerTestSkillToolCall("call-1", skills.SkillAgentManagement, "get_agent_config", map[string]interface{}{
		"agent_id": "agent-1",
	})
	if !shouldEmitNaturalProgressForToolCalls(resolved, loaded, []adapter.ToolCall{businessCall}) {
		t.Fatal("loaded business tool turn did not allow user-visible progress")
	}
	unsupportedNavigate := adapter.ToolCall{Function: adapter.FunctionCall{
		Name:      "navigate",
		Arguments: `{"href":"/console/files"}`,
	}}
	if shouldEmitNaturalProgressForToolCalls(resolved, loaded, []adapter.ToolCall{unsupportedNavigate}) {
		t.Fatal("unloaded direct navigate call was treated as executable progress")
	}
	loaded[skills.SkillConsoleNavigator] = struct{}{}
	if !shouldEmitNaturalProgressForToolCalls(resolved, loaded, []adapter.ToolCall{unsupportedNavigate}) {
		t.Fatal("direct tool call for a uniquely loaded skill did not allow progress")
	}
}

func TestHandleLoadSkillCallDoesNotEmitEventForAlreadyLoadedSkill(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{
			ID:          skills.SkillAgentManagement,
			Name:        "Agent Management",
			Description: "Manage agents.",
			RuntimeType: "prompt",
		},
		Instructions: "# Agent Management\n",
	}}}
	runner := &Runner{
		SkillRuntime: skills.NewRuntime(nil, nil),
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{})
	loaded := map[string]struct{}{skills.SkillAgentManagement: {}}
	events := []Event{}

	result := runner.handleLoadSkillCall(
		context.Background(),
		prepared,
		resolved,
		"call-1",
		map[string]interface{}{"skill_id": skills.SkillAgentManagement},
		loaded,
		func(event Event) error {
			events = append(events, event)
			return nil
		},
	)

	if len(events) != 0 {
		t.Fatalf("events = %#v, want no duplicated skill_load events", events)
	}
	if result.trace.Kind != "skill_load_attempt" || result.trace.Status != "already_loaded" {
		t.Fatalf("trace = %#v, want diagnostic already-loaded attempt", result.trace)
	}
	if _, ok := loaded[skills.SkillAgentManagement]; !ok {
		t.Fatalf("loaded skill was removed: %#v", loaded)
	}
	if !result.usedSkill {
		t.Fatal("usedSkill = false, want true for already loaded skill")
	}
	if result.toolMessage.Role == "" || result.toolMessage.ToolCallID == "" || result.toolMessage.Content == nil {
		t.Fatalf("toolMessage = %#v, want already-loaded tool message", result.toolMessage)
	}
	payload := messageContent(result.toolMessage.Content)
	if !strings.Contains(payload, "already_loaded") {
		t.Fatalf("toolMessage content = %q, want compact already-loaded status", payload)
	}
	if strings.Contains(payload, "# Agent Management") || strings.Contains(payload, `"instructions"`) {
		t.Fatalf("toolMessage content = %q, want no repeated skill instructions", payload)
	}
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

func TestRunnerRepromptsMainModelAfterEmptyFinalAnswer(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, skills.SkillFileGenerator, `---
name: file-generator
description: Generate files.
when_to_use: Use for file generation.
provider_type: builtin
provider_id: file_generator
runtime_type: tool
tools:
  - generate_file
---

# File Generator

Use generate_file to create a temporary artifact.
`)
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: ""},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "The file empty-final.svg was generated."},
				}},
			},
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillFileGenerator})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "create an svg file"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"user_request": "create an svg file",
				"generated_files": []interface{}{
					map[string]interface{}{
						"target":       "temporary_artifact",
						"tool_file_id": "tool-file-1",
						"filename":     "empty-final.svg",
						"skill_id":     skills.SkillFileGenerator,
						"tool_name":    "generate_file",
					},
				},
				"operation_plan": map[string]interface{}{
					"status":              "completed",
					"pending_next_action": "message_file_card",
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "The file empty-final.svg was generated." {
		t.Fatalf("answer = %q, want repaired main model answer", answer)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want initial empty turn plus one main model repair", fakeLLM.appChatCalls)
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

func TestRunnerContinuesAfterMetaToolTextInsteadOfCoalescingFinalAnswer(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:    "assistant",
						Content: "Configuration updated successfully.",
						ToolCalls: []adapter.ToolCall{{
							ID:   "turn-state-terminal",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolTurnState,
								Arguments: `{"items":[{"kind":"verification","visibility":"model_only","key":"agent_updated","value":"true"}]}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "final-answer",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolFinalAnswer,
								Arguments: `{"answer":"Configuration updated successfully."}`,
							},
						}},
					},
				}},
			},
		},
	}
	var traces []skills.SkillTrace
	var events []Event
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntimeWithCatalog(nil, nil, ""),
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
		Messages: []adapter.Message{{Role: "user", Content: "update the agent"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared:                  prepared,
		Resolved:                  runnerTestResolvedSkills(),
		PreferExplicitFinalAnswer: true,
		RuntimeStateSnapshot:      func() map[string]interface{} { return map[string]interface{}{} },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Configuration updated successfully." {
		t.Fatalf("answer = %q, want explicit final answer from second round", answer)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want meta-tool round plus final-answer round", fakeLLM.appChatCalls)
	}
	foundTurnState := false
	foundFinalAnswer := false
	for _, trace := range traces {
		foundTurnState = foundTurnState || trace.Kind == "turn_state"
		foundFinalAnswer = foundFinalAnswer || trace.Kind == "final_answer"
	}
	if !foundTurnState || !foundFinalAnswer {
		t.Fatalf("traces = %#v, want turn_state and final_answer traces", traces)
	}
	for _, event := range events {
		if event.Type == EventAgentProgress {
			t.Fatalf("meta-tool text was emitted as progress: %#v", event.Payload)
		}
	}
}

func TestRunnerAcceptsFinalAnswerWithoutPlanSnapshotWhenCurrentPlanHasOpenPhases(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "final-answer-missing-plan",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolFinalAnswer,
								Arguments: `{"answer":"Done."}`,
							},
						}},
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
		Messages: []adapter.Message{{Role: "user", Content: "update the agent"}},
	})
	evidence := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id":     "phase-update",
				"step":   "Update the Agent",
				"status": "pending",
			}},
		},
		"evidence_ledger": []interface{}{map[string]interface{}{
			"skill_id":  "agent-management",
			"tool_name": "update_agent_config",
			"status":    "completed",
		}},
	}

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared:                  prepared,
		Resolved:                  runnerTestResolvedSkills(),
		PreferExplicitFinalAnswer: true,
		RuntimeStateSnapshot:      func() map[string]interface{} { return evidence },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Done." {
		t.Fatalf("answer = %q, want model final answer", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want one terminal decision", fakeLLM.appChatCalls)
	}
	failedFinalAnswers := 0
	successFinalAnswers := 0
	for _, trace := range traces {
		if trace.Kind != "final_answer" {
			continue
		}
		if trace.Status == "success" {
			successFinalAnswers++
		} else {
			failedFinalAnswers++
		}
	}
	if failedFinalAnswers != 0 || successFinalAnswers != 1 {
		t.Fatalf("final answer traces failed=%d success=%d; traces=%#v", failedFinalAnswers, successFinalAnswers, traces)
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
		Prepared:                  prepared,
		Resolved:                  runnerTestResolvedSkills(),
		PreferExplicitFinalAnswer: true,
		RuntimeStateSnapshot:      func() map[string]interface{} { return evidence },
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

func TestRunnerRecordsTurnStateWhenModelWrapsMetaToolInSkillToolCall(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "wrapped-turn-state",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolCallSkillTool,
								Arguments: `{"skill_id":"agent-management","tool_name":"submit_turn_state","arguments":{"items":[{"kind":"verification","visibility":"model_only","key":"agent_config_verified","value":"created agent uses deepseek-v4-flash and file-generator","source":"agent-management/get_agent_config"}]}}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Verified."},
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
		Messages: []adapter.Message{{Role: "user", Content: "verify the agent config"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Verified." {
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
	if len(items) != 1 || stringFromInterface(items[0]["key"]) != "agent_config_verified" {
		t.Fatalf("turn_state items = %#v, want agent_config_verified fact", items)
	}
	for _, event := range events {
		if event.Type == EventSkillCallError {
			t.Fatalf("event = %#v, want no visible skill call error", event)
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

func TestRunnerTreatsUnavailableSkillLoadAsPlannerFeedbackWithoutLoadEvent(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load_nav",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"console-navigator"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "I continued from the current page evidence."},
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
		Messages: []adapter.Message{{Role: "user", Content: "edit the current agent config"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "I continued from the current page evidence." {
		t.Fatalf("answer = %q, want final answer after unavailable skill feedback", answer)
	}
	if len(fakeLLM.appChatRequests) < 2 || !runnerTestRequestContains(fakeLLM.appChatRequests[1], "skill console-navigator is not enabled for this turn") {
		t.Fatalf("second planning request missing unavailable-skill feedback")
	}
	foundFeedback := false
	for _, trace := range traces {
		if trace.Kind == "skill_load_attempt" && trace.SkillID == skills.SkillConsoleNavigator && trace.Status == "blocked" {
			foundFeedback = true
		}
		if trace.Kind == "skill_load" && trace.SkillID == skills.SkillConsoleNavigator {
			t.Fatalf("trace = %#v, want no skill_load trace for unavailable skill", trace)
		}
	}
	if !foundFeedback {
		t.Fatalf("traces = %#v, want diagnostic blocked load attempt", traces)
	}
	for _, event := range events {
		if event.Type == EventSkillLoadStart || event.Type == EventSkillLoadEnd || event.Type == EventSkillCallError {
			t.Fatalf("event = %#v, want no user-visible skill load/error event for unavailable skill", event)
		}
	}
}

func TestRunnerUsesMainModelFinalAfterAgentBatchDelete(t *testing.T) {
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
								Arguments: `{"skill_id":"agent-management"}`,
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
					Message: adapter.Message{Role: "assistant", Content: "Deleted Agent One and Agent Two."},
				}},
			},
		},
	}
	deleteTool := &runnerAgentManagementDeleteAgentsTool{}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{tool: deleteTool}); err != nil {
		t.Fatalf("register agent management provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillAgentManagement})
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
		Messages: []adapter.Message{{Role: "user", Content: "delete the first two agents"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Deleted Agent One and Agent Two." {
		t.Fatalf("answer = %q, want main model final answer", answer)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want load, delete, and main model final rounds", fakeLLM.appChatCalls)
	}
	if deleteTool.calls != 1 {
		t.Fatalf("delete calls = %d, want one batch call", deleteTool.calls)
	}
	if findRunnerTestEvent(events, EventMessage) == nil {
		t.Fatalf("events = %#v, want final message event", events)
	}
}

func TestRunnerUsesMainModelFinalAfterReadOnlyAgentConfig(t *testing.T) {
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
  - get_agent_config
  - get_agent
max_calls_per_turn: 20
---

# Agent Management

Use get_agent_config and get_agent for read-only Agent configuration checks.
`)
	state := &runnerAgentIdentityState{
		agentID:     "agent-1",
		agentName:   "ReadOnly Agent",
		description: "Current Agent description",
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
								Arguments: `{"skill_id":"agent-management"}`,
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
							runnerTestSkillToolCall("call_config", skills.SkillAgentManagement, "get_agent_config", map[string]interface{}{"agent_id": "agent-1"}),
							runnerTestSkillToolCall("call_agent", skills.SkillAgentManagement, "get_agent", map[string]interface{}{"agent_id": "agent-1"}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "ReadOnly Agent uses openai/gpt-4o. Current Agent description. \u6570\u636e\u5e93\u8868 2 \u4e2a."},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{
		getConfigTool: &runnerAgentManagementGetAgentConfigTool{state: state},
		getAgentTool:  &runnerAgentManagementGetAgentTool{state: state},
	}); err != nil {
		t.Fatalf("register agent management provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillAgentManagement})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "read-only check the current Agent configuration: name, description, model/provider, and current bound resource counts; do not modify config"}},
	})
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"user_request": "read-only check the current Agent configuration: name, description, model/provider, and current bound resource counts; do not modify config",
				"operation_plan": map[string]interface{}{
					"status":             "running",
					"original_user_goal": "read-only check the current Agent configuration: name, description, model/provider, and current bound resource counts; do not modify config",
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:agent-management/get_agent_config",
							"status":    "pending",
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "get_agent_config",
						},
						map[string]interface{}{
							"id":        "tool:agent-management/get_agent",
							"status":    "pending",
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "get_agent",
						},
					},
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want load, read, and main model final rounds", fakeLLM.appChatCalls)
	}
	if state.configCalls != 1 || state.getCalls != 1 {
		t.Fatalf("config/get calls = %d/%d, want 1/1", state.configCalls, state.getCalls)
	}
	for _, want := range []string{
		"ReadOnly Agent",
		"Current Agent description",
		"openai/gpt-4o",
		"\u6570\u636e\u5e93\u8868 2 \u4e2a",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestRunnerUsesMainModelFinalAfterSplitReadOnlyAgentConfig(t *testing.T) {
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
  - get_agent_config
  - get_agent
  - list_available_models
max_calls_per_turn: 20
---

# Agent Management

Use get_agent_config and get_agent for read-only Agent configuration checks.
`)
	state := &runnerAgentIdentityState{
		agentID:     "agent-1",
		agentName:   "ReadOnly Agent",
		description: "Current Agent description",
	}
	readOnlyPrompt := "\u7b2c\u56db\u6b21\u53ea\u8bfb\u68c0\u67e5\u5f53\u524d\u667a\u80fd\u4f53\u914d\u7f6e\uff1a\u53ea\u786e\u8ba4\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u6a21\u578b/provider\u3001\u5f53\u524d\u5df2\u7ed1\u5b9a\u8d44\u6e90\u6570\u91cf\u3002\u4e0d\u8981\u5217\u5019\u9009\u8d44\u6e90\uff0c\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e\u3002"
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
								Arguments: `{"skill_id":"agent-management"}`,
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
							runnerTestSkillToolCall("call_config", skills.SkillAgentManagement, "get_agent_config", map[string]interface{}{"agent_id": "agent-1"}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall("call_agent", skills.SkillAgentManagement, "get_agent", map[string]interface{}{"agent_id": "agent-1"}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "ReadOnly Agent uses openai/gpt-4o. Current Agent description."},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{
		getConfigTool: &runnerAgentManagementGetAgentConfigTool{state: state},
		getAgentTool:  &runnerAgentManagementGetAgentTool{state: state},
	}); err != nil {
		t.Fatalf("register agent management provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillAgentManagement})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: readOnlyPrompt}},
	})
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fakeLLM.appChatCalls != 4 {
		t.Fatalf("AppChat calls = %d, want load, two reads, and main model final", fakeLLM.appChatCalls)
	}
	if state.configCalls != 1 || state.getCalls != 1 {
		t.Fatalf("config/get calls = %d/%d, want 1/1", state.configCalls, state.getCalls)
	}
	for _, want := range []string{"ReadOnly Agent", "Current Agent description", "openai/gpt-4o"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
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

func TestRunnerIgnoresExecutablePlanAndAllowsRepeatedReadBeforeMutation(t *testing.T) {
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
  - get_agent_config
  - update_agent_config
max_calls_per_turn: 20
---

# Agent Management

Use get_agent_config to inspect draft config and update_agent_config to patch selected config fields.
`)
	state := &runnerAgentIdentityState{agentID: "agent-1", agentName: "Config Agent"}
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
								Arguments: `{"skill_id":"agent-management"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:      "assistant",
						ToolCalls: []adapter.ToolCall{runnerTestSkillToolCall("call_get_1", skills.SkillAgentManagement, "get_agent_config", map[string]interface{}{"agent_id": "agent-1"})},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:      "assistant",
						ToolCalls: []adapter.ToolCall{runnerTestSkillToolCall("call_get_2", skills.SkillAgentManagement, "get_agent_config", map[string]interface{}{"agent_id": "agent-1"})},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{runnerTestSkillToolCall("call_update_config", skills.SkillAgentManagement, "update_agent_config", map[string]interface{}{
							"agent_id":            "agent-1",
							"home_title":          "Updated Home",
							"suggested_questions": []interface{}{"配置检查", "能力说明"},
						})},
					},
				}},
			},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "配置已经更新。"}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"pass","reason":"update_agent_config tool evidence confirms the requested config patch"}`}}}},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{
		getConfigTool:    &runnerAgentManagementGetAgentConfigTool{state: state},
		updateConfigTool: &runnerAgentManagementUpdateConfigTool{state: state},
	}); err != nil {
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
		Messages: []adapter.Message{{Role: "user", Content: "change the current Agent home title to Updated Home and suggested questions to 配置检查 and 能力说明"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			status := "running"
			stepStatus := "pending"
			pendingNextAction := "agent-management/update_agent_config"
			if state.configUpdateCalls > 0 {
				status = "completed"
				stepStatus = "completed"
				pendingNextAction = "none"
			}
			return map[string]interface{}{
				"user_request": "change the current Agent home title to Updated Home and suggested questions to 配置检查 and 能力说明",
				"operation_plan": map[string]interface{}{
					"status":              status,
					"pending_next_action": pendingNextAction,
					"steps": []interface{}{
						map[string]interface{}{
							"id":                      "tool:agent-management/update_agent_config",
							"status":                  stepStatus,
							"skill_id":                skills.SkillAgentManagement,
							"tool_name":               "update_agent_config",
							"expected_updated_fields": []interface{}{"home_title", "suggested_questions"},
							"arguments": map[string]interface{}{
								"agent_id": "agent-1",
							},
						},
					},
					"step_status": map[string]interface{}{
						"tool:agent-management/update_agent_config": stepStatus,
					},
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if state.configCalls != 2 {
		t.Fatalf("get_agent_config calls = %d, want both model-selected reads to execute", state.configCalls)
	}
	if state.configUpdateCalls != 1 {
		t.Fatalf("update_agent_config calls = %d, want one pending config mutation call", state.configUpdateCalls)
	}
	if got := runnerTestRequestToolChoiceName(fakeLLM.appChatRequests[2]); got != "" {
		t.Fatalf("third request forced tool_choice = %q, want model to choose from exposed tools", got)
	}
	if got := runnerTestRequestToolChoiceName(fakeLLM.appChatRequests[3]); got != "" {
		t.Fatalf("fourth request forced tool_choice = %q, want model to choose from exposed tools", got)
	}
	if answer != "配置已经更新。" {
		t.Fatalf("answer = %q, want the main-model final answer", answer)
	}
}

func TestRunnerUsesMainModelFinalAfterPostUpdateAgentConfigRead(t *testing.T) {
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
  - get_agent_config
  - list_agent_knowledge_candidates
max_calls_per_turn: 20
---

# Agent Management

Use get_agent_config to verify Agent configuration after a governed update.
`)
	state := &runnerAgentIdentityState{agentID: "agent-1", agentName: "Support Agent"}
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
								Arguments: `{"skill_id":"agent-management"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:      "assistant",
						ToolCalls: []adapter.ToolCall{runnerTestSkillToolCall("call_get_config", skills.SkillAgentManagement, "get_agent_config", map[string]interface{}{"agent_id": "agent-1"})},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Support Agent now includes KB Two and orders."},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{
		getConfigTool: &runnerAgentManagementGetAgentConfigTool{state: state},
	}); err != nil {
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
		Messages: []adapter.Message{{Role: "user", Content: "Bind KB Two and orders to Support Agent, then read config again after completion and verify the bindings."}},
	})
	updateResult := map[string]interface{}{
		"status":     "completed",
		"agent_name": "Support Agent",
		"updated_fields": []interface{}{
			"knowledge_dataset_ids",
			"database_bindings",
		},
		"binding_changes": []interface{}{
			map[string]interface{}{
				"field":                "knowledge_dataset_ids",
				"binding_kind":         "knowledge_base",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"KB Two"},
			},
			map[string]interface{}{
				"field":                "database_bindings",
				"binding_kind":         "database_table",
				"change_action":        "bind",
				"resource_count":       1,
				"added_resource_count": 1,
				"resource_names":       []interface{}{"orders"},
			},
		},
	}

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"user_request": "Bind KB Two and orders to Support Agent, then read config again after completion and verify the bindings.",
				"operation_plan": map[string]interface{}{
					"status":             "running",
					"original_user_goal": "Bind KB Two and orders to Support Agent, then read config again after completion and verify the bindings.",
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:agent-management/update_agent_config",
							"status":    "completed",
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "update_agent_config",
						},
						map[string]interface{}{
							"id":        "tool:agent-management/get_agent_config#post_update",
							"status":    "pending",
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "get_agent_config",
						},
					},
					"tool_result": map[string]interface{}{
						"status":         "success",
						"skill_id":       skills.SkillAgentManagement,
						"tool_name":      "update_agent_config",
						"result_summary": updateResult,
					},
				},
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"status":    "success",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "update_agent_config",
						"result":    updateResult,
					},
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want load, post-update read, and main model final", fakeLLM.appChatCalls)
	}
	if state.configCalls != 1 {
		t.Fatalf("get_agent_config calls = %d, want exactly one post-update verification read", state.configCalls)
	}
	for _, want := range []string{"Support Agent", "KB Two", "orders"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want %q", answer, want)
		}
	}
}

func TestRunnerDoesNotRewriteDuplicateAgentIdentityMutation(t *testing.T) {
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
  - update_agent_identity
  - get_agent
max_calls_per_turn: 20
---

# Agent Management

Use update_agent_identity for identity changes and get_agent to verify the updated Agent identity.
`)
	state := &runnerAgentIdentityState{agentID: "agent-1", agentName: "Before Agent"}
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load_agent_management",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"agent-management"}`,
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
							runnerTestSkillToolCall(
								"call_update_identity",
								skills.SkillAgentManagement,
								"update_agent_identity",
								map[string]interface{}{"agent_id": "agent-1", "name": "After Agent"},
							),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall(
								"call_duplicate_update_identity",
								skills.SkillAgentManagement,
								"update_agent_identity",
								map[string]interface{}{"agent_id": "agent-1", "name": "After Agent"},
							),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Updated and verified."},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerAgentManagementProvider{
		updateIdentityTool: &runnerAgentManagementUpdateIdentityTool{state: state},
		getAgentTool:       &runnerAgentManagementGetAgentTool{state: state},
	}); err != nil {
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
		Messages: []adapter.Message{{Role: "user", Content: "rename the current Agent, then confirm the page header updated"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			updateStatus := "pending"
			readStatus := "pending"
			planStatus := "running"
			pendingNextAction := "agent-management/update_agent_identity"
			if state.updateCalls > 0 {
				updateStatus = "completed"
				pendingNextAction = "agent-management/get_agent"
			}
			if state.getCalls > 0 {
				readStatus = "completed"
				planStatus = "completed"
				pendingNextAction = "none"
			}
			return map[string]interface{}{
				"user_request": "rename the current Agent, then confirm the page header updated",
				"operation_plan": map[string]interface{}{
					"status":              planStatus,
					"pending_next_action": pendingNextAction,
					"steps": []interface{}{
						map[string]interface{}{
							"id":        "tool:agent-management/update_agent_identity",
							"status":    updateStatus,
							"skill_id":  skills.SkillAgentManagement,
							"tool_name": "update_agent_identity",
						},
						map[string]interface{}{
							"id":                                "tool:agent-management/get_agent#post_update",
							"status":                            readStatus,
							"skill_id":                          skills.SkillAgentManagement,
							"tool_name":                         "get_agent",
							"wait_for":                          "tool:agent-management/update_agent_identity",
							"phase":                             "post_update_verification",
							"required_post_update_verification": true,
						},
					},
					"step_status": map[string]interface{}{
						"tool:agent-management/update_agent_identity": updateStatus,
						"tool:agent-management/get_agent#post_update": readStatus,
					},
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if state.updateCalls != 2 || state.getCalls != 0 {
		t.Fatalf("update/get calls = %d/%d, want the model-selected calls 2/0", state.updateCalls, state.getCalls)
	}
	if len(fakeLLM.appChatRequests) < 3 {
		t.Fatalf("AppChat request count = %d, want at least 3", len(fakeLLM.appChatRequests))
	}
	if got := runnerTestRequestToolChoiceName(fakeLLM.appChatRequests[2]); got != "" {
		t.Fatalf("third request forced tool_choice = %q, want model to choose from exposed tools", got)
	}
	if strings.TrimSpace(answer) == "" {
		t.Fatal("answer is empty, want the main-model final answer")
	}
}

func TestRunnerUsesMainModelFinalAfterFileManagementSave(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, skills.SkillFileManager, `---
name: file-manager
description: Manage files.
when_to_use: Use when testing file management saves.
provider_type: builtin
provider_id: files
runtime_type: tool
tools:
  - save_file_to_management
max_calls_per_turn: 20
---

# File Manager

Use save_file_to_management to save generated files into File Management.
`)
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
								Arguments: `{"skill_id":"file-manager"}`,
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
							runnerTestSkillToolCall(
								"call_save_file",
								skills.SkillFileManager,
								"save_file_to_management",
								map[string]interface{}{
									"source_type":  "tool_file",
									"tool_file_id": "tool-file-1",
									"filename":     "star-cat.svg",
								},
							),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Saved star-cat.svg to File Management."},
				}},
			},
		},
	}
	saveTool := &runnerFileManagerSaveTool{}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerFilesProvider{saveTool: saveTool}); err != nil {
		t.Fatalf("register files provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillFileManager})
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
		Messages: []adapter.Message{{Role: "user", Content: "save the generated SVG to file management"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Saved star-cat.svg to File Management." {
		t.Fatalf("answer = %q, want main model final answer", answer)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want load, save, and main model final rounds", fakeLLM.appChatCalls)
	}
	if saveTool.calls != 1 {
		t.Fatalf("save calls = %d, want one save call", saveTool.calls)
	}
	if findRunnerTestEvent(events, EventMessage) == nil {
		t.Fatalf("events = %#v, want final message event", events)
	}
}

func TestRunnerUsesMainModelFinalAfterFileManagementDelete(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, skills.SkillFileManager, `---
name: file-manager
description: Manage files.
when_to_use: Use when testing file management deletes.
provider_type: builtin
provider_id: files
runtime_type: tool
tools:
  - delete_file
max_calls_per_turn: 20
---

# File Manager

Use delete_file to delete a managed file.
`)
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
								Arguments: `{"skill_id":"file-manager"}`,
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
							runnerTestSkillToolCall(
								"call_delete_file",
								skills.SkillFileManager,
								"delete_file",
								map[string]interface{}{
									"file_id": "managed-file-1",
								},
							),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Deleted aichat-plan-smoke.md."},
				}},
			},
		},
	}
	deleteTool := &runnerFileManagerDeleteTool{}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerFilesProvider{deleteTool: deleteTool}); err != nil {
		t.Fatalf("register files provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{skills.SkillFileManager})
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
		Messages: []adapter.Message{{Role: "user", Content: "save aichat-plan-smoke.md to file management"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Deleted aichat-plan-smoke.md." {
		t.Fatalf("answer = %q, want main model final answer", answer)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want load, delete, and main model final rounds", fakeLLM.appChatCalls)
	}
	if deleteTool.calls != 1 {
		t.Fatalf("delete calls = %d, want one delete call", deleteTool.calls)
	}
	if findRunnerTestEvent(events, EventMessage) == nil {
		t.Fatalf("events = %#v, want final message event", events)
	}
}

func TestShouldStreamSkillPlanningIncludesQwenProvider(t *testing.T) {
	prepared := NewPreparedChat("conversation-1", "message-1", " qWeN ", "auto", &adapter.ChatRequest{Model: "qwq-plus"})

	if !shouldStreamSkillPlanning(prepared) {
		t.Fatal("shouldStreamSkillPlanning(qwen) = false, want true")
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

func TestRunnerBlocksRepeatedIdenticalFailedToolCallAndReplans(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a tool that can fail.
when_to_use: Use when testing repeated failed tool calls.
provider_type: builtin
provider_id: calculator
runtime_type: tool
tools:
  - evaluate_expression
---

# Limited Calculator

Use the calculator tool.
`)
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
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall("call_bad_1", "limited-calculator", "evaluate_expression", map[string]interface{}{
								"expression": "1/",
							}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall("call_bad_2", "limited-calculator", "evaluate_expression", map[string]interface{}{
								"expression": "1/",
							}),
						},
					},
				}},
			},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I cannot evaluate that expression without corrected input."}}}},
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
	var events []Event
	var traces []skills.SkillTrace
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			traces = append(traces, trace)
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "calculate an invalid expression"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "I cannot evaluate that expression without corrected input." {
		t.Fatalf("answer = %q, want replanned final answer", answer)
	}
	starts := 0
	for _, event := range events {
		if event.Type == EventSkillCallStart {
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("skill call start events = %d, want only the first failed tool execution", starts)
	}
	foundFeedback := false
	for _, trace := range traces {
		if trace.Kind == "planner_feedback" &&
			trace.SkillID == "limited-calculator" &&
			trace.ToolName == "evaluate_expression" &&
			strings.Contains(trace.Error, "same tool call with the same arguments already failed") {
			foundFeedback = true
			break
		}
	}
	if !foundFeedback {
		t.Fatalf("traces = %#v, want repeated failed tool call planner feedback", traces)
	}
	if fakeLLM.appChatCalls != 4 {
		t.Fatalf("AppChat calls = %d, want load, first failure, blocked retry, final answer", fakeLLM.appChatCalls)
	}
}

func TestRunnerAllowsCorrectedRetryAfterFailedToolCall(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a tool that can fail.
when_to_use: Use when testing corrected failed tool calls.
provider_type: builtin
provider_id: calculator
runtime_type: tool
tools:
  - evaluate_expression
---

# Limited Calculator

Use the calculator tool.
`)
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
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall("call_bad", "limited-calculator", "evaluate_expression", map[string]interface{}{
								"expression": "1/",
							}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall("call_retry", "limited-calculator", "evaluate_expression", map[string]interface{}{
								"expression": "1+1",
							}),
						},
					},
				}},
			},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "The corrected expression result is 2."}}}},
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
	var events []Event
	var traces []skills.SkillTrace
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			traces = append(traces, trace)
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "calculate after correcting a bad expression"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "The corrected expression result is 2." {
		t.Fatalf("answer = %q, want final answer after corrected retry", answer)
	}
	starts := 0
	for _, event := range events {
		if event.Type == EventSkillCallStart {
			starts++
		}
	}
	if starts != 2 {
		t.Fatalf("skill call start events = %d, want failed call and corrected retry to execute", starts)
	}
	for _, trace := range traces {
		if trace.Kind == "planner_feedback" &&
			trace.SkillID == "limited-calculator" &&
			trace.ToolName == "evaluate_expression" {
			t.Fatalf("traces = %#v, want corrected retry to execute without repeated-failure planner feedback", traces)
		}
	}
	if fakeLLM.appChatCalls != 4 {
		t.Fatalf("AppChat calls = %d, want load, first failure, corrected retry, final answer", fakeLLM.appChatCalls)
	}
}

func TestRunnerRuntimeStateSnapshotTurnsRepeatedRecoverableFailuresIntoTruthfulAnswer(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a tool that can fail.
when_to_use: Use when testing repeated recoverable tool failures.
provider_type: builtin
provider_id: calculator
runtime_type: tool
tools:
  - evaluate_expression
---

# Limited Calculator

Use the calculator tool.
`)
	responses := []*adapter.ChatResponse{
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
	}
	for i := 0; i < defaultMaxConsecutiveRecoverableFailureRounds+1; i++ {
		responses = append(responses, &adapter.ChatResponse{
			Choices: []adapter.Choice{{
				Message: adapter.Message{
					Role: "assistant",
					ToolCalls: []adapter.ToolCall{
						runnerTestSkillToolCall(
							fmt.Sprintf("call_bad_%d", i),
							"limited-calculator",
							"evaluate_expression",
							map[string]interface{}{"expression": "1/"},
						),
					},
				},
			}},
		})
	}
	fakeLLM := &runnerTestLLMClient{appChatResponses: responses}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{"limited-calculator"})
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
		Messages: []adapter.Message{{Role: "user", Content: "calculate an invalid expression"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want truthful failed answer", err)
	}
	if strings.TrimSpace(answer) == "" || !strings.Contains(answer, "limited-calculator/evaluate_expression") {
		t.Fatalf("answer = %q, want failed-answer evidence for calculator tool", answer)
	}
	if findRunnerTestEvent(events, EventSkillCallError) == nil {
		t.Fatalf("events = %#v, want skill error event for failed tool evidence", events)
	}
}

func TestRunnerRuntimeStateSnapshotTurnsPlanningRoundExhaustionIntoTruthfulAnswer(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a tool.
when_to_use: Use when testing planning exhaustion.
provider_type: builtin
provider_id: calculator
runtime_type: tool
tools:
  - evaluate_expression
---

# Limited Calculator

Use the calculator tool.
`)
	responses := make([]*adapter.ChatResponse, 0, defaultMaxSkillPlanningRounds)
	for i := 0; i < defaultMaxSkillPlanningRounds; i++ {
		responses = append(responses, &adapter.ChatResponse{
			Choices: []adapter.Choice{{
				Message: adapter.Message{
					Role: "assistant",
					ToolCalls: []adapter.ToolCall{{
						ID:   fmt.Sprintf("call_load_%d", i),
						Type: "function",
						Function: adapter.FunctionCall{
							Name:      skills.MetaToolLoadSkill,
							Arguments: `{"skill_id":"limited-calculator"}`,
						},
					}},
				},
			}},
		})
	}
	fakeLLM := &runnerTestLLMClient{appChatResponses: responses}
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
		Messages: []adapter.Message{{Role: "user", Content: "calculate without finishing"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		RuntimeStateSnapshot: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
				"evidence_ledger": []interface{}{map[string]interface{}{
					"kind": "tool_call", "status": "success", "skill_id": "file-reader", "tool_name": "read_file",
				}},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want truthful exhausted-plan answer", err)
	}
	if !strings.Contains(answer, "too many skill planning rounds") {
		t.Fatalf("answer = %q, want planning-exhaustion failure answer", answer)
	}
	if fakeLLM.appChatCalls != defaultMaxSkillPlanningRounds {
		t.Fatalf("AppChat calls = %d, want %d", fakeLLM.appChatCalls, defaultMaxSkillPlanningRounds)
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
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"agent-workflow"}`,
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
							runnerTestSkillToolCall("call_workflow", "agent-workflow", "run_agent_workflow", map[string]interface{}{
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

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
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

func TestRunnerStopsForToolGovernanceApprovalPending(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "governed-files", `---
name: governed-files
description: Governed files test skill.
when_to_use: Use when testing tool governance feedback.
provider_type: builtin
provider_id: files
runtime_type: tool
tools:
  - delete_file
tool_governance:
  delete_file:
    tool_id: file.delete
    skill_id: governed-files
    domain: files
    effect: delete
    asset_type: file
    risk_level: high
    requires_asset_resolution: true
    permission_scopes:
      - file:manage
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
---

# Governed Files

Use governed file tools.
`)
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
								Arguments: `{"skill_id":"governed-files"}`,
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
							runnerTestSkillToolCall("call_delete", "governed-files", "delete_file", map[string]interface{}{
								"file_id": "file-1",
							}),
						},
					},
				}},
			},
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, catalogDir).
		WithToolGovernanceGateway(skills.NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()))
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{"governed-files"})
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
		Messages: []adapter.Message{{Role: "user", Content: "Delete the first file"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: "organization-1",
			UserID:         "user-1",
			ConversationID: "conv-1",
			MessageID:      "msg-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-1", "type": "file", "name": "report.pdf"},
					},
				},
			},
		},
	})
	var pending *ToolGovernancePendingError
	if !errors.As(err, &pending) {
		t.Fatalf("Run() error = %v, want ToolGovernancePendingError", err)
	}
	if answer != "" {
		t.Fatalf("answer = %q, want no final answer before approval", answer)
	}
	if pending.Payload["correlation_id"] == "" {
		t.Fatalf("pending payload = %#v, want correlation_id", pending.Payload)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want pause before governance replan", fakeLLM.appChatCalls)
	}
	event := findRunnerTestEvent(events, EventToolGovernanceDecision)
	if event == nil {
		t.Fatalf("events = %#v, want tool_governance_decision", events)
	}
	if event.Payload["decision"] != toolgovernance.DecisionStatusNeedsApproval {
		t.Fatalf("governance payload = %#v, want needs_approval", event.Payload)
	}
	if event.Payload["requires_approval"] != true {
		t.Fatalf("governance payload = %#v, want requires_approval", event.Payload)
	}
}

func TestRunnerApprovedGovernanceGrantExecutesDeleteTool(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "governed-files", `---
name: governed-files
description: Governed files test skill.
when_to_use: Use when testing approved tool governance execution.
provider_type: builtin
provider_id: governed_files_test
runtime_type: tool
tools:
  - delete_file
tool_governance:
  delete_file:
    tool_id: file.delete
    skill_id: governed-files
    domain: files
    effect: delete
    asset_type: file
    risk_level: high
    requires_asset_resolution: true
    permission_scopes:
      - file:manage
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
---

# Governed Files

Use governed file tools.
`)
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
								Arguments: `{"skill_id":"governed-files"}`,
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
							runnerTestSkillToolCall("call_delete", "governed-files", "delete_file", map[string]interface{}{
								"file_id": "file-1",
							}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Deleted report.pdf."},
				}},
			},
		},
	}
	deleteTool := &runnerGovernedFilesDeleteTool{}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&runnerGovernedFilesProvider{tool: deleteTool}); err != nil {
		t.Fatalf("register governed files provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir).
		WithToolGovernanceGateway(skills.NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()))
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{"governed-files"})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}

	pendingLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load_pending",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"governed-files"}`,
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
							runnerTestSkillToolCall("call_delete_pending", "governed-files", "delete_file", map[string]interface{}{
								"file_id": "file-1",
							}),
						},
					},
				}},
			},
		},
	}
	pendingRunner := &Runner{
		LLMClient:    pendingLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}
	pendingPrepared := NewPreparedChat("conv-1", "msg-pending", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "Delete report.pdf"}},
	})
	pendingAnswer, _, pendingErr := pendingRunner.Run(ctx, RunRequest{
		Prepared: pendingPrepared,
		Resolved: resolved,
		ExecutionContext: skills.ExecutionContext{
			ConversationID: "conv-1",
			MessageID:      "msg-pending",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-1", "type": "file", "name": "report.pdf"},
					},
				},
			},
		},
	})
	var pendingWithoutGrant *ToolGovernancePendingError
	if !errors.As(pendingErr, &pendingWithoutGrant) {
		t.Fatalf("pending Run() error = %v, want ToolGovernancePendingError", pendingErr)
	}
	if pendingAnswer != "" {
		t.Fatalf("pending answer = %q, want no final answer before approval", pendingAnswer)
	}
	if len(deleteTool.calls) != 0 {
		t.Fatalf("delete calls before approval = %#v, want none", deleteTool.calls)
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
		Messages: []adapter.Message{{Role: "user", Content: "Delete report.pdf"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: "organization-1",
			UserID:         "user-1",
			ConversationID: "conv-1",
			MessageID:      "msg-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-1", "type": "file", "name": "report.pdf"},
					},
					"session_grants": []map[string]interface{}{
						{
							"conversation_id":         "conv-1",
							"organization_id":         "organization-1",
							"user_id":                 "user-1",
							"skill_id":                "governed-files",
							"provider_type":           "builtin",
							"provider_id":             "governed_files_test",
							"tool_id":                 "file.delete",
							"effect":                  "delete",
							"asset_type":              "file",
							"assets":                  []map[string]interface{}{{"id": "file-1", "type": "file", "name": "report.pdf"}},
							"risk_level":              "high",
							"approval_correlation_id": "approval-corr-1",
							"expires_at":              time.Now().Add(time.Hour).Format(time.RFC3339),
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want final answer after record-only asset observation", err)
	}
	if answer != "Deleted report.pdf." {
		t.Fatalf("answer = %q, want final answer after asset observation event", answer)
	}
	if len(deleteTool.calls) != 1 || deleteTool.calls[0] != "file-1" {
		t.Fatalf("delete calls = %#v, want one call for approved file-1", deleteTool.calls)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want load, delete, and final answer", fakeLLM.appChatCalls)
	}
	event := findRunnerTestEvent(events, EventToolGovernanceDecision)
	if event == nil {
		t.Fatalf("events = %#v, want allowed tool governance decision", events)
	}
	if event.Payload["decision"] != toolgovernance.DecisionStatusAllowed {
		t.Fatalf("governance payload = %#v, want allowed", event.Payload)
	}
	decision, ok := event.Payload["governance"].(*toolgovernance.Decision)
	if !ok || decision.ApprovedByCorrelationID != "approval-corr-1" {
		t.Fatalf("governance payload = %#v, want approval correlation", event.Payload)
	}
	clientActionEvent := findRunnerTestEvent(events, EventClientActionRequired)
	if clientActionEvent == nil {
		t.Fatalf("events = %#v, want client action observation request", events)
	}
	if clientActionEvent.Payload["action_type"] != "asset_observation" ||
		clientActionEvent.Payload["effect"] != "delete" ||
		clientActionEvent.Payload["asset_type"] != "file" {
		t.Fatalf("client action payload = %#v, want file delete observation", clientActionEvent.Payload)
	}
	if clientActionEvent.Payload["continuation_policy"] != clientActionContinuationPolicyRecordOnly ||
		clientActionEvent.Payload["status"] != "succeeded" {
		t.Fatalf("client action payload = %#v, want record-only succeeded observation", clientActionEvent.Payload)
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
	appChatResponses      []*adapter.ChatResponse
	appChatRequests       []*adapter.ChatRequest
	appChatCalls          int
	appChatStreams        [][]adapter.StreamResponse
	appChatStreamRequests []*adapter.ChatRequest
	appChatStreamCalls    int
}

type runnerTestWorkflowRunner struct {
	events []automationaction.WorkflowRunEvent
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
	return &automationaction.WorkflowRunResult{
		WorkflowRunID: "run-1",
		WorkflowID:    req.WorkflowRef.WorkflowID,
		AgentID:       req.WorkflowRef.AgentID,
		Status:        "succeeded",
		Outputs:       map[string]interface{}{},
	}, nil
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
	if f.appChatCalls >= len(f.appChatResponses) {
		return nil, errors.New("unexpected AppChat call")
	}
	f.appChatRequests = append(f.appChatRequests, cloneChatRequest(req))
	resp := f.appChatResponses[f.appChatCalls]
	f.appChatCalls++
	return resp, nil
}

func (f *runnerTestLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	if f.appChatStreamCalls >= len(f.appChatStreams) {
		return nil, errors.New("unexpected AppChatStream call")
	}
	f.appChatStreamRequests = append(f.appChatStreamRequests, cloneChatRequest(req))
	responses := append([]adapter.StreamResponse(nil), f.appChatStreams[f.appChatStreamCalls]...)
	f.appChatStreamCalls++
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
