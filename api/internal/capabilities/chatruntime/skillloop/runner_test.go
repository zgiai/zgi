package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func TestRestoredLoadedSkillInstructionsMessageRehydratesContinuation(t *testing.T) {
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
	message := restoredLoadedSkillInstructionsMessage(resolved, map[string]struct{}{
		skills.SkillAgentManagement: {},
	})
	if message == nil {
		t.Fatal("restoredLoadedSkillInstructionsMessage() = nil")
	}
	content := messageContent(message.Content)
	for _, want := range []string{
		"loaded earlier in this same user turn",
		skills.SkillAgentManagement,
		"Preserve unmentioned settings.",
		"Do not call load_skill for these skills again",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("restored message missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "Read exact file content.") {
		t.Fatalf("restored message included an unloaded skill:\n%s", content)
	}
}

func TestCompactRestoredSkillInstructionsKeepsBoundariesWithinBudget(t *testing.T) {
	instructions := "BEGIN\n" + strings.Repeat("middle details\n", 2000) + "END"
	const budget = 600

	got := compactRestoredSkillInstructions(instructions, budget)

	if len([]rune(got)) > budget {
		t.Fatalf("compacted instruction runes = %d, want <= %d", len([]rune(got)), budget)
	}
	for _, want := range []string{"BEGIN", "Detailed middle section omitted", "END"} {
		if !strings.Contains(got, want) {
			t.Fatalf("compacted instructions missing %q in:\n%s", want, got)
		}
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
	if result.trace.Kind != "" {
		t.Fatalf("trace = %#v, want no timeline trace for already loaded skill", result.trace)
	}
	if _, ok := loaded[skills.SkillAgentManagement]; !ok {
		t.Fatalf("loaded skill was removed: %#v", loaded)
	}
	if !result.usedSkill {
		t.Fatal("usedSkill = false, want true for already loaded skill")
	}
	if result.toolMessage.Role == "" || result.toolMessage.ToolCallID == "" || result.toolMessage.Content == nil {
		t.Fatalf("toolMessage = %#v, want skill document tool message", result.toolMessage)
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

func TestRepeatedSuccessfulReadOnlyToolCallFeedbackStepUsesPreviousResult(t *testing.T) {
	args := map[string]interface{}{"agent_id": "agent-1"}
	result := repeatedSuccessfulReadOnlyToolCallFeedbackStep("call-2", skills.SkillAgentManagement, "list_agent_workflow_binding_candidates", args, map[string]SkillToolCallRef{
		failedToolCallKey(skills.SkillAgentManagement, "list_agent_workflow_binding_candidates", args): {
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "list_agent_workflow_binding_candidates",
			Arguments: copyStringAnyMap(args),
			Result: map[string]interface{}{
				"status":    "completed",
				"count":     0,
				"workflows": []interface{}{},
			},
		},
	}, nil)

	if result.trace.Kind != "planner_feedback" {
		t.Fatalf("trace.Kind = %q, want planner_feedback", result.trace.Kind)
	}
	if result.trace.Status != "advisory" {
		t.Fatalf("trace.Status = %q, want advisory", result.trace.Status)
	}
	if result.recoverable {
		t.Fatal("result.recoverable = true, want false for advisory feedback")
	}
	if result.usedTool {
		t.Fatal("result.usedTool = true, want false because no tool was executed")
	}
	if got := result.trace.Arguments["next_step"]; got != "answer_from_previous_result" {
		t.Fatalf("trace next_step = %#v, want answer_from_previous_result", got)
	}
	content, ok := result.toolMessage.Content.(string)
	if !ok {
		t.Fatalf("tool message content type = %T, want string", result.toolMessage.Content)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("tool message JSON error = %v; content = %s", err, content)
	}
	if got := payload["advisory"]; got != "same_read_only_tool_already_succeeded" {
		t.Fatalf("payload advisory = %#v, want same_read_only_tool_already_succeeded", got)
	}
	summary, ok := payload["previous_result_summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("previous_result_summary = %#v, want object", payload["previous_result_summary"])
	}
	if got := summary["count"]; got != float64(0) {
		t.Fatalf("summary count = %#v, want 0", got)
	}
	if got := summary["workflows_count"]; got != float64(0) {
		t.Fatalf("summary workflows_count = %#v, want 0", got)
	}
	if !strings.Contains(fmt.Sprint(payload["next_action"]), "previous tool result") {
		t.Fatalf("next_action = %#v, want previous-result guidance", payload["next_action"])
	}
}

func TestPlanToolGuardAdvisoryStepDoesNotReturnRecoverableError(t *testing.T) {
	result := planToolGuardRecoverableStep("call-1", skills.SkillAgentManagement, "list_agent_database_tables", map[string]interface{}{
		"agent_id": "agent-1",
	}, FinalAnswerGuardResult{
		SkillID:       skills.SkillAgentManagement,
		ToolName:      "get_agent_config",
		Message:       "agent-management/list_agent_database_tables already has successful evidence for this turn.",
		SystemMessage: "Continue with the next pending planned step: agent-management/get_agent_config.",
		Advisory:      true,
	})

	if result.recoverable {
		t.Fatal("result.recoverable = true, want false for advisory feedback")
	}
	if result.trace.Kind != "planner_feedback" {
		t.Fatalf("trace.Kind = %q, want planner_feedback", result.trace.Kind)
	}
	if result.trace.Status != "advisory" {
		t.Fatalf("trace.Status = %q, want advisory", result.trace.Status)
	}
	if result.trace.Error != "" {
		t.Fatalf("trace.Error = %q, want empty advisory error", result.trace.Error)
	}
	if got := result.trace.Arguments["next_step"]; got != "continue_with_next_planned_step" {
		t.Fatalf("trace next_step = %#v, want continue_with_next_planned_step", got)
	}
	content, ok := result.toolMessage.Content.(string)
	if !ok {
		t.Fatalf("tool message content type = %T, want string", result.toolMessage.Content)
	}
	if strings.Contains(content, "invalid input") {
		t.Fatalf("tool message content = %s, want no invalid input", content)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("tool message JSON error = %v; content = %s", err, content)
	}
	if got := payload["status"]; got != "advisory" {
		t.Fatalf("payload status = %#v, want advisory", got)
	}
	if got := payload["advisory"]; got != "planner_feedback" {
		t.Fatalf("payload advisory = %#v, want planner_feedback", got)
	}
	if _, ok := payload["error"]; ok {
		t.Fatalf("payload error = %#v, want no error field", payload["error"])
	}
	if !strings.Contains(fmt.Sprint(payload["next_action"]), "get_agent_config") {
		t.Fatalf("next_action = %#v, want next planned step guidance", payload["next_action"])
	}
}

func TestRepeatedSuccessfulReadOnlyToolCallFeedbackStepIgnoresDifferentArguments(t *testing.T) {
	previousArgs := map[string]interface{}{"agent_id": "agent-1"}
	result := repeatedSuccessfulReadOnlyToolCallFeedbackStep("call-2", skills.SkillAgentManagement, "list_agent_workflow_binding_candidates", map[string]interface{}{"agent_id": "agent-2"}, map[string]SkillToolCallRef{
		failedToolCallKey(skills.SkillAgentManagement, "list_agent_workflow_binding_candidates", previousArgs): {
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "list_agent_workflow_binding_candidates",
			Arguments: copyStringAnyMap(previousArgs),
			Result:    map[string]interface{}{"status": "completed"},
		},
	}, nil)

	if result.trace.Kind != "" {
		t.Fatalf("trace.Kind = %q, want empty for different arguments", result.trace.Kind)
	}
}

func TestRepeatedSuccessfulCandidateLookupWithDifferentArgumentsLetsModelDecide(t *testing.T) {
	previousArgs := map[string]interface{}{
		"agent_id":       "agent-1",
		"data_source_id": "database-1",
	}
	nextArgs := map[string]interface{}{
		"agent_id":       "agent-1",
		"data_source_id": "database-2",
	}
	result := repeatedSuccessfulReadOnlyToolCallFeedbackStep("call-2", skills.SkillAgentManagement, "list_agent_database_tables", nextArgs, nil, []SkillToolCallRef{
		{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "list_agent_database_tables",
			Arguments: copyStringAnyMap(previousArgs),
			Result: map[string]interface{}{
				"status": "completed",
				"count":  2,
				"binding_candidates": []interface{}{map[string]interface{}{
					"id":   "database-1:table-1",
					"name": "test1",
					"binding": map[string]interface{}{
						"data_source_id": "database-1",
						"table_ids":      []interface{}{"table-1"},
					},
				}},
			},
		},
	})

	if result.trace.Kind != "" {
		t.Fatalf("trace.Kind = %q, want no backend semantic redirect for different arguments", result.trace.Kind)
	}
}

func TestRepeatedSuccessfulReadOnlyToolCallFeedbackStepIgnoresMutations(t *testing.T) {
	args := map[string]interface{}{
		"agent_id": "agent-1",
		"name":     "Support Agent",
	}
	result := repeatedSuccessfulReadOnlyToolCallFeedbackStep("call-2", skills.SkillAgentManagement, "update_agent_identity", args, map[string]SkillToolCallRef{
		failedToolCallKey(skills.SkillAgentManagement, "update_agent_identity", args): {
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "update_agent_identity",
			Arguments: copyStringAnyMap(args),
			Result:    map[string]interface{}{"status": "completed"},
		},
	}, nil)

	if result.trace.Kind != "" {
		t.Fatalf("trace.Kind = %q, want empty for mutation tool", result.trace.Kind)
	}
}

func TestRepeatedSuccessfulReadOnlyToolCallFeedbackStepAllowsAgentConfigReadAfterMutation(t *testing.T) {
	args := map[string]interface{}{"agent_id": "agent-1"}
	key := failedToolCallKey(skills.SkillAgentManagement, "get_agent_config", args)
	result := repeatedSuccessfulReadOnlyToolCallFeedbackStep("call-3", skills.SkillAgentManagement, "get_agent_config", args, map[string]SkillToolCallRef{
		key: {
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "get_agent_config",
			Arguments: copyStringAnyMap(args),
			Result:    map[string]interface{}{"status": "completed"},
		},
	}, []SkillToolCallRef{
		{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "get_agent_config",
			Arguments: copyStringAnyMap(args),
			Result:    map[string]interface{}{"status": "completed"},
		},
		{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "update_agent_config",
			Arguments: copyStringAnyMap(args),
			Result:    map[string]interface{}{"status": "completed"},
		},
	})

	if result.trace.Kind != "" {
		t.Fatalf("trace.Kind = %q, want empty so post-update read can execute", result.trace.Kind)
	}
}

func TestRepeatedSuccessfulReadOnlyToolCallFeedbackStepDoesNotInterpretPendingPlan(t *testing.T) {
	args := map[string]interface{}{"agent_id": "agent-1"}
	key := failedToolCallKey(skills.SkillAgentManagement, "get_agent_config", args)
	result := repeatedSuccessfulReadOnlyToolCallFeedbackStep("call-4", skills.SkillAgentManagement, "get_agent_config", args, map[string]SkillToolCallRef{
		key: {
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "get_agent_config",
			Arguments: copyStringAnyMap(args),
			Result:    map[string]interface{}{"status": "completed", "agent_id": "agent-1"},
		},
	}, []SkillToolCallRef{
		{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "update_agent_identity",
			Arguments: copyStringAnyMap(args),
			Result:    map[string]interface{}{"status": "completed", "agent_id": "agent-1"},
		},
		{
			SkillID:   skills.SkillAgentManagement,
			ToolName:  "get_agent_config",
			Arguments: copyStringAnyMap(args),
			Result:    map[string]interface{}{"status": "completed", "agent_id": "agent-1"},
		},
	})

	if result.trace.Kind != "planner_feedback" {
		t.Fatalf("trace.Kind = %q, want planner_feedback", result.trace.Kind)
	}
	if got := stringFromInterface(result.trace.Arguments["next_step"]); got != "answer_from_previous_result" {
		t.Fatalf("trace next_step = %q, want answer_from_previous_result", got)
	}
	content := fmt.Sprint(result.toolMessage.Content)
	if strings.Contains(content, "update_agent_config") || strings.Contains(content, "pending asset-changing Agent step") {
		t.Fatalf("tool message = %s, want no backend-selected mutation", content)
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence:        func() map[string]interface{} { return map[string]interface{}{} },
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

func TestRunnerShowsContextualSidebarCheckpointForFileTurnState(t *testing.T) {
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
	var checkpoints []Event
	for _, event := range events {
		if event.Type == EventIntermediateAnswer {
			checkpoints = append(checkpoints, event)
		}
	}
	if len(checkpoints) != 1 {
		t.Fatalf("intermediate events = %#v, want exactly one file-derived checkpoint", checkpoints)
	}
	content := stringFromInterface(checkpoints[0].Payload["content"])
	if !strings.Contains(content, "已记录文件小结") || !strings.Contains(content, "雪是主角的妹妹") {
		t.Fatalf("checkpoint content = %q, want localized file summary", content)
	}
	if strings.Contains(content, "deepseek-v4-flash") {
		t.Fatalf("checkpoint content = %q, want selected model state hidden", content)
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
		if trace.Kind == "planner_feedback" && trace.SkillID == skills.SkillConsoleNavigator {
			foundFeedback = true
		}
		if trace.Kind == "skill_load" && trace.SkillID == skills.SkillConsoleNavigator {
			t.Fatalf("trace = %#v, want no skill_load trace for unavailable skill", trace)
		}
	}
	if !foundFeedback {
		t.Fatalf("traces = %#v, want planner feedback for unavailable skill", traces)
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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

func TestRunnerDoesNotForcePendingAgentConfigMutation(t *testing.T) {
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
		CompletionEvidence: func() map[string]interface{} {
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
	if state.configCalls != 1 {
		t.Fatalf("get_agent_config calls = %d, want one executed read despite repeated model call", state.configCalls)
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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

func TestRunnerCompletionEvidenceTurnsRepeatedRecoverableFailuresIntoTruthfulAnswer(t *testing.T) {
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
		CompletionEvidence: func() map[string]interface{} {
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

func TestRunnerCompletionEvidenceTurnsPlanningRoundExhaustionIntoTruthfulAnswer(t *testing.T) {
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
		CompletionEvidence: func() map[string]interface{} {
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

func TestRunnerFinalAnswerGuardForcesRequiredToolBeforeCompletion(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a required tool.
when_to_use: Use when testing final answer guards.
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
					Message: adapter.Message{Role: "assistant", Content: "The file has been deleted."},
				}},
			},
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
							runnerTestSkillToolCall("call_eval", "limited-calculator", "evaluate_expression", map[string]interface{}{
								"expression": "1+1",
							}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "tool-backed answer"},
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
	var traces []skills.SkillTrace
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnTrace: func(_ []skills.SkillTrace, trace skills.SkillTrace) {
			traces = append(traces, trace)
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "delete file-1"}},
	})
	guardCalls := 0
	sawSuccessfulToolArguments := false
	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		FinalAnswerGuard: func(req FinalAnswerGuardRequest) (FinalAnswerGuardResult, bool) {
			guardCalls++
			for _, call := range req.SuccessfulToolCalls {
				if call.SkillID == "limited-calculator" && call.ToolName == "evaluate_expression" {
					if summary, ok := call.Arguments["expression"].(map[string]interface{}); ok && summary["length"] == 3 {
						sawSuccessfulToolArguments = true
					}
					return FinalAnswerGuardResult{}, false
				}
			}
			return FinalAnswerGuardResult{
				SkillID:  "limited-calculator",
				ToolName: "evaluate_expression",
				Message:  "call evaluate_expression before claiming completion",
			}, true
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "tool-backed answer" {
		t.Fatalf("answer = %q, want tool-backed answer", answer)
	}
	if guardCalls != 2 {
		t.Fatalf("guard calls = %d, want 2", guardCalls)
	}
	if !sawSuccessfulToolArguments {
		t.Fatalf("final answer guard did not receive summarized tool arguments")
	}
	if fakeLLM.appChatCalls != 4 {
		t.Fatalf("AppChat calls = %d, want guard-triggered replan plus tool run", fakeLLM.appChatCalls)
	}
	if len(fakeLLM.appChatRequests) < 2 || !runnerTestRequestContains(fakeLLM.appChatRequests[1], "Runtime guardrail feedback") {
		t.Fatalf("second planning request did not include guardrail feedback")
	}
	foundGuardrail := false
	for _, trace := range traces {
		if trace.Kind == "guardrail" && trace.ToolName == "evaluate_expression" && strings.Contains(trace.Error, "call evaluate_expression") {
			foundGuardrail = true
			break
		}
	}
	if !foundGuardrail {
		t.Fatalf("traces = %#v, want final answer guardrail trace", traces)
	}
}

func TestFinalAnswerGuardSystemMessageUsesPrivateModelFeedbackWithoutLeakingTrace(t *testing.T) {
	result := FinalAnswerGuardResult{
		SkillID:       "file-reader",
		ToolName:      "delete_file",
		Message:       "Call delete_file for report.pdf before claiming completion.",
		SystemMessage: `Call delete_file with {"file_id":"file-internal-1"} before claiming completion.`,
	}

	trace := finalAnswerGuardrailTrace(result)
	if strings.Contains(trace.Error, "file-internal-1") {
		t.Fatalf("trace error exposed private model feedback: %q", trace.Error)
	}
	if !strings.Contains(trace.Error, "report.pdf") {
		t.Fatalf("trace error = %q, want display-safe message", trace.Error)
	}

	message := finalAnswerGuardSystemMessage(result, "Done.")
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("system message content type = %T, want string", message.Content)
	}
	if !strings.Contains(content, "file-internal-1") {
		t.Fatalf("system message missing private model feedback: %q", content)
	}
	if !strings.Contains(content, "Blocked candidate answer") {
		t.Fatalf("system message missing blocked candidate answer: %q", content)
	}
}

func TestRunnerUserInputGuardBlocksClarificationAndReplans(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a required tool.
when_to_use: Use when testing user input guards.
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
							ID:   "call_ask",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolRequestUserInput,
								Arguments: `{"message":"I found two candidate files and need your choice.","questions":[{"id":"file","question":"Which file should I read?","options":[{"label":"first.xlsx"},{"label":"second.xlsx"}]}]}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "continued after guard"},
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
		Messages: []adapter.Message{{Role: "user", Content: "read the resolved file"}},
	})
	guardCalls := 0
	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		UserInputGuard: func(req UserInputGuardRequest) (FinalAnswerGuardResult, bool) {
			guardCalls++
			if req.Message != "I found two candidate files and need your choice." || len(req.Questions) != 1 {
				t.Fatalf("guard request = %#v, want normalized user input request", req)
			}
			return FinalAnswerGuardResult{
				SkillID:  "file-reader",
				ToolName: "read_file",
				Message:  "target already resolved; call read_file instead of asking",
			}, true
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "continued after guard" {
		t.Fatalf("answer = %q, want replanned answer", answer)
	}
	if guardCalls != 1 {
		t.Fatalf("guard calls = %d, want 1", guardCalls)
	}
	if findRunnerTestEvent(events, EventUserInputRequested) != nil {
		t.Fatalf("events = %#v, want no user_input_requested event after guard block", events)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want guard-triggered replan", fakeLLM.appChatCalls)
	}
	if len(fakeLLM.appChatRequests) < 2 || !runnerTestRequestContains(fakeLLM.appChatRequests[1], "target already resolved; call read_file instead of asking") {
		t.Fatalf("second planning request did not include user input guard feedback")
	}
	foundGuardrail := false
	for _, trace := range traces {
		if trace.Kind == "guardrail" && trace.ToolName == "read_file" && strings.Contains(trace.Error, "target already resolved") {
			foundGuardrail = true
			break
		}
	}
	if !foundGuardrail {
		t.Fatalf("traces = %#v, want user input guardrail trace", traces)
	}
}

func TestRunnerCompletionEvidenceKeepsUserInputGuardForRedundantClarification(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a required tool.
when_to_use: Use when testing user input guard behavior.
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
							ID:   "call_ask",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolRequestUserInput,
								Arguments: `{"message":"I need your preferred target file before continuing.","questions":[{"id":"file","question":"Which file should I read?","options":[{"label":"first.xlsx"},{"label":"second.xlsx"}]}]}`,
							},
						}},
					},
				}},
			},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I will continue from the resolved evidence instead of asking again."}}}},
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
		Messages: []adapter.Message{{Role: "user", Content: "read a file after I choose it"}},
	})
	guardCalls := 0
	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		UserInputGuard: func(req UserInputGuardRequest) (FinalAnswerGuardResult, bool) {
			guardCalls++
			if req.Message != "I need your preferred target file before continuing." || len(req.Questions) != 1 {
				t.Fatalf("guard request = %#v, want normalized user input request", req)
			}
			return FinalAnswerGuardResult{
				SkillID:  "legacy-guard",
				ToolName: "read_file",
				Message:  "target already resolved; continue from evidence instead of asking",
			}, true
		},
		CompletionEvidence: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
				"evidence_ledger": []interface{}{map[string]interface{}{
					"kind": "tool_call", "status": "success", "skill_id": "file-reader", "tool_name": "read_file",
				}},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "I will continue from the resolved evidence instead of asking again." {
		t.Fatalf("answer = %q, want replanned answer", answer)
	}
	if guardCalls != 1 {
		t.Fatalf("user input guard calls = %d, want 1", guardCalls)
	}
	if findRunnerTestEvent(events, EventUserInputRequested) != nil {
		t.Fatalf("events = %#v, want no user_input_requested event after guard block", events)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want guard-triggered replan", fakeLLM.appChatCalls)
	}
	if len(fakeLLM.appChatRequests) < 2 || !runnerTestRequestContains(fakeLLM.appChatRequests[1], "target already resolved; continue from evidence instead of asking") {
		t.Fatalf("second planning request did not include user input guard feedback")
	}
}

func TestRunnerToolCallGuardBlocksToolBeforeExecutionAndReplans(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a required tool.
when_to_use: Use when testing tool call guards.
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
							runnerTestSkillToolCall("call_eval", "limited-calculator", "evaluate_expression", map[string]interface{}{
								"expression": "1+1",
							}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "continued after tool guard"},
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
		Messages: []adapter.Message{{Role: "user", Content: "calculate after navigation"}},
	})
	guardCalls := 0
	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		ToolCallGuard: func(req ToolCallGuardRequest) (FinalAnswerGuardResult, bool) {
			guardCalls++
			if req.SkillID != "limited-calculator" || req.ToolName != "evaluate_expression" {
				t.Fatalf("tool call guard request = %#v, want limited-calculator/evaluate_expression", req)
			}
			return FinalAnswerGuardResult{
				SkillID:       "console-navigator",
				ToolName:      "navigate",
				Message:       "navigate first",
				SystemMessage: `call console-navigator/navigate with href "/console/files" before evaluating`,
			}, true
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "continued after tool guard" {
		t.Fatalf("answer = %q, want replanned answer", answer)
	}
	if guardCalls != 1 {
		t.Fatalf("guard calls = %d, want 1", guardCalls)
	}
	if findRunnerTestEvent(events, EventSkillCallStart) != nil {
		t.Fatalf("events = %#v, want no skill_call_start for guarded tool call", events)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want guard-triggered replan", fakeLLM.appChatCalls)
	}
	if len(fakeLLM.appChatRequests) < 3 || !runnerTestRequestContains(fakeLLM.appChatRequests[2], "navigate first") {
		t.Fatalf("third planning request did not include tool guard feedback")
	}
	foundGuardrail := false
	for _, trace := range traces {
		if trace.Kind == "guardrail" && trace.ToolName == "navigate" && strings.Contains(trace.Error, "navigate first") {
			foundGuardrail = true
			break
		}
	}
	if !foundGuardrail {
		t.Fatalf("traces = %#v, want tool call guardrail trace", traces)
	}
}

func TestRunnerCompletionEvidenceDisablesLegacyToolCallGuard(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a required tool.
when_to_use: Use when testing tool call guards.
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
							runnerTestSkillToolCall("call_eval", "limited-calculator", "evaluate_expression", map[string]interface{}{
								"expression": "1+1",
							}),
						},
					},
				}},
			},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "The result is 2."}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"pass","reason":"calculator tool succeeded"}`}}}},
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
		Messages: []adapter.Message{{Role: "user", Content: "calculate 1+1"}},
	})
	guardCalls := 0
	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		ToolCallGuard: func(req ToolCallGuardRequest) (FinalAnswerGuardResult, bool) {
			guardCalls++
			return FinalAnswerGuardResult{
				SkillID:  "legacy-guard",
				ToolName: req.ToolName,
				Message:  "legacy tool-call guard should not run when post verification is configured",
			}, true
		},
		CompletionEvidence: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "completed"},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "The result is 2." {
		t.Fatalf("answer = %q, want verifier-approved tool result answer", answer)
	}
	if guardCalls != 0 {
		t.Fatalf("tool call guard calls = %d, want 0", guardCalls)
	}
	if findRunnerTestEvent(events, EventSkillCallStart) == nil {
		t.Fatalf("events = %#v, want skill call to execute", events)
	}
}

func TestRunnerCompletionEvidenceKeepsPlanToolGuard(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a required tool.
when_to_use: Use when testing plan tool guards.
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
							runnerTestSkillToolCall("call_eval", "limited-calculator", "evaluate_expression", map[string]interface{}{
								"expression": "1+1",
							}),
						},
					},
				}},
			},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "I cannot run that unplanned tool."}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"pass","reason":"reported the blocked tool honestly"}`}}}},
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
		Messages: []adapter.Message{{Role: "user", Content: "calculate 1+1"}},
	})
	blockedPlanCalls := 0
	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		PlanToolGuard: func(req ToolCallGuardRequest) (FinalAnswerGuardResult, bool) {
			if req.ToolName == "" {
				return FinalAnswerGuardResult{}, false
			}
			blockedPlanCalls++
			return FinalAnswerGuardResult{
				SkillID:       req.SkillID,
				ToolName:      req.ToolName,
				Message:       "tool is not part of the current operation plan",
				SystemMessage: "answer from existing evidence instead of running the unplanned tool",
			}, true
		},
		CompletionEvidence: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "running"},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "I cannot run that unplanned tool." {
		t.Fatalf("answer = %q, want plan-guard-aware answer", answer)
	}
	if blockedPlanCalls != 1 {
		t.Fatalf("blocked plan calls = %d, want 1", blockedPlanCalls)
	}
	if findRunnerTestEvent(events, EventSkillCallStart) != nil {
		t.Fatalf("events = %#v, want no skill tool execution for plan-blocked call", events)
	}
	if findRunnerTestEvent(events, EventSkillCallError) != nil {
		t.Fatalf("events = %#v, want no user-visible error for internal plan feedback", events)
	}
	foundPlannerFeedback := false
	for _, trace := range traces {
		if trace.Kind == "guardrail" {
			t.Fatalf("trace kind = guardrail for plan tool guard; traces=%#v", traces)
		}
		if trace.Kind == "planner_feedback" &&
			trace.SkillID == "limited-calculator" &&
			trace.ToolName == "evaluate_expression" {
			foundPlannerFeedback = true
		}
	}
	if !foundPlannerFeedback {
		t.Fatalf("traces = %#v, want internal planner feedback trace for blocked plan tool", traces)
	}
}

func TestRunnerFinalAnswerGuardAllowsAnswerAfterRequiredToolAttemptFails(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a required tool.
when_to_use: Use when testing final answer guards.
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
							runnerTestSkillToolCall("call_eval", "limited-calculator", "evaluate_expression", map[string]interface{}{
								"expression": "1/",
							}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "I tried the required tool, but it failed."},
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
		Messages: []adapter.Message{{Role: "user", Content: "calculate with the required tool"}},
	})
	guardCalls := 0
	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		FinalAnswerGuard: func(req FinalAnswerGuardRequest) (FinalAnswerGuardResult, bool) {
			guardCalls++
			for _, call := range req.AttemptedToolCalls {
				if call.SkillID == "limited-calculator" && call.ToolName == "evaluate_expression" {
					return FinalAnswerGuardResult{}, false
				}
			}
			return FinalAnswerGuardResult{
				SkillID:  "limited-calculator",
				ToolName: "evaluate_expression",
				Message:  "call evaluate_expression before claiming completion",
			}, true
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "I tried the required tool, but it failed." {
		t.Fatalf("answer = %q, want failed-tool explanation", answer)
	}
	if guardCalls != 1 {
		t.Fatalf("guard calls = %d, want 1 after failed tool attempt", guardCalls)
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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
		CompletionEvidence: func() map[string]interface{} {
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

func TestCompletionVerifierTreatsPendingPlanStepAsAdvisory(t *testing.T) {
	decision := completionVerificationApplyPlanOverride(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "running",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "pending",
					"skill_id":  "agent-management",
					"tool_name": "update_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_config": "pending",
			},
		},
	}, completionVerificationDecision{Status: completionVerificationStatusPass, Reason: "truthful incomplete answer"})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass; decision=%#v", got, decision)
	}
	if decision.NextActionHint != "" {
		t.Fatalf("NextActionHint = %q, want empty hint for advisory pending plan", decision.NextActionHint)
	}
	if len(decision.MissingSteps) != 0 {
		t.Fatalf("MissingSteps = %#v, want no forced missing steps", decision.MissingSteps)
	}
}

func TestCompletionVerifierKeepsPassForCompletedPlan(t *testing.T) {
	decision := completionVerificationApplyPlanOverride(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "completed",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    "completed",
					"skill_id":  "agent-management",
					"tool_name": "update_agent_config",
				},
			},
			"step_status": map[string]interface{}{
				"tool:agent-management/update_agent_config": "completed",
			},
		},
	}, completionVerificationDecision{Status: completionVerificationStatusPass, Reason: "done"})

	if got := decision.normalizedStatus(); got != completionVerificationStatusPass {
		t.Fatalf("decision status = %q, want pass; decision=%#v", got, decision)
	}
}

func TestRunnerCompletionEvidenceDisablesLegacyFinalAnswerGuard(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "The operation is complete."}}}},
			{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: `{"status":"pass","reason":"candidate is supported by evidence"}`}}}},
		},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "complete the operation"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
		FinalAnswerGuard: func(FinalAnswerGuardRequest) (FinalAnswerGuardResult, bool) {
			return FinalAnswerGuardResult{
				ToolName:      "legacy_guard",
				Message:       "legacy guard should not run when post verification is configured",
				SystemMessage: "legacy guard should not be visible",
			}, true
		},
		CompletionEvidence: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "completed"},
				"skill_invocations": []interface{}{map[string]interface{}{
					"kind": "tool_call", "status": "success", "skill_id": "test-skill", "tool_name": "test_tool",
				}},
			}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "The operation is complete." {
		t.Fatalf("answer = %q, want verifier-approved candidate answer", answer)
	}
	if fakeLLM.appChatCalls != 1 {
		t.Fatalf("AppChat calls = %d, want no routine verifier call", fakeLLM.appChatCalls)
	}
}

func TestRunnerTerminalGuardPreservesMainModelAnswer(t *testing.T) {
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
	var verificationResult CompletionVerificationResult

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
		CompletionEvidence: func() map[string]interface{} {
			return map[string]interface{}{
				"model_verifier_required": true,
				"operation_plan":          map[string]interface{}{"status": "completed"},
				"skill_invocations": []interface{}{map[string]interface{}{
					"kind": "tool_call", "status": "success", "skill_id": "test-skill", "tool_name": "test_tool",
				}},
			}
		},
		OnCompletionVerification: func(result CompletionVerificationResult) {
			verificationResult = result
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "\u6211\u8fd8\u4e0d\u80fd\u786e\u8ba4\u8fd9\u4e2a\u64cd\u4f5c\u5df2\u7ecf\u5b8c\u6210\u3002" {
		t.Fatalf("answer = %q, want the main-model candidate", answer)
	}
	if verificationResult.Status != completionVerificationStatusPass {
		t.Fatalf("completion verification status = %q, want pass", verificationResult.Status)
	}
	if verificationResult.FinalAnswer != answer {
		t.Fatalf("completion verification final answer = %q, want %q", verificationResult.FinalAnswer, answer)
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
	var verificationResult CompletionVerificationResult

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: runnerTestResolvedSkills(),
		CompletionEvidence: func() map[string]interface{} {
			return map[string]interface{}{
				"operation_plan": map[string]interface{}{"status": "failed"},
				"skill_invocations": []interface{}{map[string]interface{}{
					"kind": "tool_call", "skill_id": "agent-management", "tool_name": "update_agent_config", "status": "error",
				}},
			}
		},
		OnCompletionVerification: func(result CompletionVerificationResult) {
			verificationResult = result
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Done, the Agent was updated." {
		t.Fatalf("answer = %q, want initial main-model answer", answer)
	}
	if verificationResult.Status != completionVerificationStatusPass {
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
		CompletionEvidence: func() map[string]interface{} {
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
		Prepared:           prepared,
		Resolved:           runnerTestResolvedSkills(),
		CompletionEvidence: func() map[string]interface{} { return map[string]interface{}{} },
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
