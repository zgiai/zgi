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
	var pendingObservation *ClientActionPendingError
	if !errors.As(err, &pendingObservation) {
		t.Fatalf("Run() error = %v, want ClientActionPendingError for asset observation", err)
	}
	if answer != "" {
		t.Fatalf("answer = %q, want no final answer before asset observation", answer)
	}
	if len(deleteTool.calls) != 1 || deleteTool.calls[0] != "file-1" {
		t.Fatalf("delete calls = %#v, want one call for approved file-1", deleteTool.calls)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want load and delete before observation", fakeLLM.appChatCalls)
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
	if pendingObservation.Payload["action_id"] != clientActionEvent.Payload["action_id"] {
		t.Fatalf("pending action = %#v, event = %#v, want same action id", pendingObservation.Payload, clientActionEvent.Payload)
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
	appChatResponses []*adapter.ChatResponse
	appChatRequests  []*adapter.ChatRequest
	appChatCalls     int
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
	return nil, errors.New("not implemented")
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
