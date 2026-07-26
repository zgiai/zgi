package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/workflowevents"
)

func TestListAgentWorkflowsReturnsRuntimeBindingsOnly(t *testing.T) {
	runtimeTool := workflowRuntimeTool(t, ToolListAgentWorkflows, &fakeWorkflowRunner{})

	messages, err := runtimeTool.Invoke(context.Background(), "caller-1", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	workflows, ok := messages[0].Data["workflows"].([]map[string]interface{})
	if !ok {
		t.Fatalf("workflows type = %T", messages[0].Data["workflows"])
	}
	if len(workflows) != 1 || workflows[0]["binding_id"] != "approval-flow" {
		t.Fatalf("workflows = %#v, want approval-flow only", workflows)
	}
	if workflows[0]["workflow_id"] != nil {
		t.Fatalf("workflow_id leaked in list payload: %#v", workflows[0])
	}
	if _, exists := workflows[0]["default_input_key"]; exists {
		t.Fatalf("default_input_key = %#v, want omitted for zero-input task workflow", workflows[0]["default_input_key"])
	}
	schema, ok := workflows[0]["input_schema"].(map[string]interface{})
	if !ok || schema["type"] != "object" {
		t.Fatalf("input_schema = %#v, want object schema", workflows[0]["input_schema"])
	}
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok || len(properties) != 0 {
		t.Fatalf("input_schema properties = %#v, want empty", schema["properties"])
	}
	required, ok := workflows[0]["required_inputs"].([]string)
	if !ok || len(required) != 0 {
		t.Fatalf("required_inputs = %#v, want empty", workflows[0]["required_inputs"])
	}
}

func TestListAgentWorkflowsRequiresQueryForConversationalWorkflow(t *testing.T) {
	runtimeTool := workflowRuntimeToolWithBinding(t, ToolListAgentWorkflows, &fakeWorkflowRunner{}, map[string]interface{}{
		"binding_id": "chat-flow", "agent_id": "agent-1", "workflow_id": "workflow-1",
		"agent_type": "CONVERSATIONAL_WORKFLOW", "version_strategy": "latest_published",
	})

	messages, err := runtimeTool.Invoke(context.Background(), "caller-1", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	workflows := messages[0].Data["workflows"].([]map[string]interface{})
	if workflows[0]["default_input_key"] != "query" {
		t.Fatalf("default_input_key = %#v, want query", workflows[0]["default_input_key"])
	}
	required := workflows[0]["required_inputs"].([]string)
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("required_inputs = %#v, want [query]", required)
	}
	properties := workflows[0]["input_schema"].(map[string]interface{})["properties"].(map[string]interface{})
	if _, exists := properties["query"]; !exists {
		t.Fatalf("input_schema properties = %#v, want query", properties)
	}
}

func TestRunAgentTaskWorkflowWithNoStartInputsAcceptsEmptyInputs(t *testing.T) {
	runner := &fakeWorkflowRunner{}
	runtimeTool := workflowRuntimeTool(t, ToolRunAgentWorkflow, runner)

	_, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "approval-flow",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if _, exists := runner.lastReq.Inputs["query"]; exists {
		t.Fatalf("task workflow inputs = %#v, want no query", runner.lastReq.Inputs)
	}
	if _, exists := runner.lastReq.Inputs["sys.query"]; exists {
		t.Fatalf("task workflow inputs = %#v, want no sys.query", runner.lastReq.Inputs)
	}
}

func TestRunAgentConversationalWorkflowRejectsMissingQuery(t *testing.T) {
	runtimeTool := workflowRuntimeToolWithBinding(t, ToolRunAgentWorkflow, &fakeWorkflowRunner{}, map[string]interface{}{
		"binding_id": "chat-flow", "agent_id": "agent-1", "workflow_id": "workflow-1",
		"agent_type": "CONVERSATIONAL_WORKFLOW", "version_strategy": "latest_published",
	})

	_, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "chat-flow",
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "inputs.query is required") {
		t.Fatalf("Invoke() error = %v, want missing query rejection", err)
	}
}

func TestRunAgentWorkflowRejectsUnknownBindingID(t *testing.T) {
	runtimeTool := workflowRuntimeTool(t, ToolRunAgentWorkflow, &fakeWorkflowRunner{})

	_, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "missing-flow",
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown workflow binding_id") {
		t.Fatalf("Invoke() error = %v, want unknown binding rejection", err)
	}
}

func TestRunAgentWorkflowReturnsSucceededOutputs(t *testing.T) {
	runner := &fakeWorkflowRunner{
		result: &automationaction.WorkflowRunResult{
			WorkflowRunID: "run-1",
			WorkflowID:    "workflow-1",
			AgentID:       "agent-1",
			Version:       "v1",
			Status:        "succeeded",
			Outputs:       map[string]interface{}{"answer": "done"},
		},
	}
	runtimeTool := workflowRuntimeTool(t, ToolRunAgentWorkflow, runner)

	messages, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "approval-flow",
		"inputs":     map[string]interface{}{},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if runner.lastReq.AccountID != "binder-1" {
		t.Fatalf("runner account = %q, want binder-1", runner.lastReq.AccountID)
	}
	if runner.lastReq.WorkflowRef.WorkflowID != "workflow-1" || runner.lastReq.WorkflowRef.AgentID != "agent-1" {
		t.Fatalf("workflow ref = %#v, want bound workflow", runner.lastReq.WorkflowRef)
	}
	if _, exists := runner.lastReq.Inputs["query"]; exists {
		t.Fatalf("workflow inputs = %#v, want no implicit query", runner.lastReq.Inputs)
	}
	if _, exists := runner.lastReq.Inputs["sys.query"]; exists {
		t.Fatalf("workflow inputs = %#v, want no implicit sys.query", runner.lastReq.Inputs)
	}
	payload := messages[0].Data
	if payload["status"] != "succeeded" || payload["workflow_run_id"] != "run-1" {
		t.Fatalf("payload = %#v, want succeeded run-1", payload)
	}
	if payload["primary_output"] != "done" {
		t.Fatalf("primary_output = %#v, want done", payload["primary_output"])
	}
	outputs, _ := payload["outputs"].(map[string]interface{})
	if outputs["answer"] != "done" {
		t.Fatalf("outputs = %#v, want answer done", outputs)
	}
}

func TestRunAgentWorkflowUsesTargetBindingAuthorizationActor(t *testing.T) {
	runner := &fakeWorkflowRunner{}
	provider := NewProvider(func() automationaction.AutomationWorkflowRunner { return runner })
	tool, err := provider.GetTool(ToolRunAgentWorkflow)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	runtimeTool := tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "workspace-1",
		InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"organization_id":              "org-1",
			"workspace_id":                 "workspace-1",
			"workflow_bound_by_account_id": "category-editor",
			"workflow_bindings": []map[string]interface{}{{
				"binding_id": "approval-flow", "agent_id": "agent-1", "workflow_id": "workflow-1", "version_strategy": "latest_published",
			}},
			"agent_binding_authorizations": []map[string]interface{}{{
				"binding_type": "workflow", "parent_resource_id": "agent-1", "resource_id": "approval-flow", "access_mode": "execute", "bound_by_account_id": "binding-owner", "bound_at_unix": int64(200),
			}},
		},
	})

	_, err = runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "approval-flow",
		"inputs":     map[string]interface{}{},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if runner.lastReq.AccountID != "binding-owner" {
		t.Fatalf("runner account = %q, want binding-owner", runner.lastReq.AccountID)
	}
}

func TestRunAgentTaskWorkflowUsesDeclaredStartInputWithoutQueryAlias(t *testing.T) {
	runner := &fakeWorkflowRunner{
		result: &automationaction.WorkflowRunResult{
			WorkflowRunID: "run-1",
			WorkflowID:    "workflow-1",
			AgentID:       "agent-1",
			Status:        "succeeded",
			Outputs:       map[string]interface{}{"output": "done"},
		},
	}
	runtimeTool := workflowRuntimeToolWithBinding(t, ToolRunAgentWorkflow, runner, map[string]interface{}{
		"binding_id":        "task-flow",
		"label":             "Task flow",
		"agent_id":          "agent-1",
		"workflow_id":       "workflow-1",
		"agent_type":        "WORKFLOW",
		"version_strategy":  "latest_published",
		"timeout_seconds":   60,
		"required_inputs":   []string{"input"},
		"default_input_key": "input",
		"start_inputs": []map[string]interface{}{
			{"variable": "input", "label": "用户输入", "type": "paragraph", "required": true},
		},
	})

	_, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "task-flow",
		"inputs":     map[string]interface{}{"input": "write a summer poem"},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if runner.lastReq.Inputs["input"] != "write a summer poem" {
		t.Fatalf("workflow inputs = %#v, want declared input", runner.lastReq.Inputs)
	}
	if _, exists := runner.lastReq.Inputs["sys.query"]; exists {
		t.Fatalf("workflow inputs kept conversational sys.query: %#v", runner.lastReq.Inputs)
	}
}

func TestRunAgentTaskWorkflowRejectsQueryAliasForDifferentStartInput(t *testing.T) {
	runtimeTool := workflowRuntimeToolWithBinding(t, ToolRunAgentWorkflow, &fakeWorkflowRunner{}, map[string]interface{}{
		"binding_id": "task-flow", "agent_id": "agent-1", "workflow_id": "workflow-1",
		"agent_type": "WORKFLOW", "version_strategy": "latest_published",
		"start_inputs": []map[string]interface{}{
			{"variable": "input", "label": "Input", "type": "paragraph", "required": true},
		},
	})

	_, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "task-flow",
		"inputs":     map[string]interface{}{"query": "write a summer poem"},
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "inputs.input is required") {
		t.Fatalf("Invoke() error = %v, want declared input rejection", err)
	}
}

func TestRunAgentTaskWorkflowAllowsExplicitQueryStartInput(t *testing.T) {
	runner := &fakeWorkflowRunner{}
	runtimeTool := workflowRuntimeToolWithBinding(t, ToolRunAgentWorkflow, runner, map[string]interface{}{
		"binding_id": "task-flow", "agent_id": "agent-1", "workflow_id": "workflow-1",
		"agent_type": "WORKFLOW", "version_strategy": "latest_published",
		"start_inputs": []map[string]interface{}{
			{"variable": "query", "label": "Search query", "type": "string", "required": true},
		},
	})

	_, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "task-flow",
		"inputs":     map[string]interface{}{"query": "explicit business input"},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if runner.lastReq.Inputs["query"] != "explicit business input" {
		t.Fatalf("workflow inputs = %#v, want explicit query start input", runner.lastReq.Inputs)
	}
	if _, exists := runner.lastReq.Inputs["sys.query"]; exists {
		t.Fatalf("workflow inputs = %#v, want no conversational sys.query", runner.lastReq.Inputs)
	}
}

func TestRunAgentWorkflowUsesTaskInvocationWithoutConversationHistory(t *testing.T) {
	runner := &fakeWorkflowRunner{}
	runtimeTool := workflowRuntimeToolWithBinding(t, ToolRunAgentWorkflow, runner, map[string]interface{}{
		"binding_id": "task-flow", "agent_id": "agent-1", "workflow_id": "workflow-1",
		"agent_type": "WORKFLOW", "version_strategy": "latest_published",
	})
	conversationID := "conversation-1"
	messageID := "message-1"
	_, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "task-flow", "inputs": map[string]interface{}{},
	}, &conversationID, nil, &messageID)
	if err != nil {
		t.Fatal(err)
	}
	if runner.lastReq.Invocation == nil || runner.lastReq.Invocation.Mode != automationaction.WorkflowInvocationModeAgentTaskTool {
		t.Fatalf("invocation = %#v, want task tool mode", runner.lastReq.Invocation)
	}
	if runner.lastReq.Invocation.ParentConversationID != conversationID || runner.lastReq.Invocation.ParentMessageID != messageID {
		t.Fatalf("invocation parent = %#v", runner.lastReq.Invocation)
	}
	if _, exists := runner.lastReq.Inputs["sys.conversation_history"]; exists {
		t.Fatalf("task workflow received conversation history: %#v", runner.lastReq.Inputs)
	}
}

func TestRunAgentWorkflowDelegatesConversationWithStableInvocationAndDurableEnvelope(t *testing.T) {
	runner := &fakeWorkflowRunner{emitEvents: []automationaction.WorkflowRunEvent{{
		Type: "node_started", Payload: map[string]interface{}{"workflow_run_id": "run-1", "node_id": "node-1"},
		Sequence: 7, SchemaVersion: 2, PayloadVersion: 1, ExecutionID: "execution-1",
	}}}
	provider := NewProvider(func() automationaction.AutomationWorkflowRunner { return runner })
	tool, err := provider.GetTool(ToolRunAgentWorkflow)
	if err != nil {
		t.Fatal(err)
	}
	runtimeTool := tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID: "workspace-1", InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1", "workspace_id": "workspace-1",
			"workflow_parent_tool_call_id": "tool-call-1",
			"workflow_context":             map[string]interface{}{"conversation_history": []interface{}{"hello", "world"}},
			"workflow_bindings": []map[string]interface{}{{
				"binding_id": "chat-flow", "agent_id": "agent-1", "workflow_id": "workflow-1",
				"agent_type": "CONVERSATIONAL_WORKFLOW", "version_strategy": "latest_published",
			}},
		},
	})
	conversationID := "conversation-1"
	messageID := "message-1"
	var events []workflowevents.Event
	ctx := workflowevents.WithEmitter(context.Background(), func(event workflowevents.Event) { events = append(events, event) })
	invoke := func() string {
		_, invokeErr := runtimeTool.Invoke(ctx, "caller-1", map[string]interface{}{
			"binding_id": "chat-flow", "inputs": map[string]interface{}{"query": "continue"},
		}, &conversationID, nil, &messageID)
		if invokeErr != nil {
			t.Fatal(invokeErr)
		}
		return runner.lastReq.Invocation.InvocationID
	}
	firstInvocationID := invoke()
	secondInvocationID := invoke()
	if firstInvocationID == "" || firstInvocationID != secondInvocationID {
		t.Fatalf("invocation IDs = %q, %q, want stable non-empty value", firstInvocationID, secondInvocationID)
	}
	if runner.lastReq.Invocation.Mode != automationaction.WorkflowInvocationModeAgentDelegate {
		t.Fatalf("invocation mode = %q", runner.lastReq.Invocation.Mode)
	}
	if history, ok := runner.lastReq.Inputs["sys.conversation_history"].([]interface{}); !ok || len(history) != 2 {
		t.Fatalf("conversation history = %#v", runner.lastReq.Inputs["sys.conversation_history"])
	}
	if len(events) == 0 || events[0].Sequence != 7 || events[0].SchemaVersion != 2 || events[0].ExecutionID != "execution-1" {
		t.Fatalf("workflow events = %#v, want durable envelope", events)
	}
	if events[0].Payload["invocation_id"] != firstInvocationID || events[0].Payload["invocation_mode"] != automationaction.WorkflowInvocationModeAgentDelegate {
		t.Fatalf("workflow event invocation = %#v", events[0].Payload)
	}
}

func TestRunAgentTaskWorkflowDoesNotRelayAnswerTransportEvents(t *testing.T) {
	runner := &fakeWorkflowRunner{emitEvents: []automationaction.WorkflowRunEvent{
		{Type: "message", Payload: map[string]interface{}{"answer": "internal"}, Sequence: 1},
		{Type: "node_finished", Payload: map[string]interface{}{"node_id": "node-1"}, Sequence: 2},
	}}
	runtimeTool := workflowRuntimeToolWithBinding(t, ToolRunAgentWorkflow, runner, map[string]interface{}{
		"binding_id": "task-flow", "agent_id": "agent-1", "workflow_id": "workflow-1",
		"agent_type": "WORKFLOW", "version_strategy": "latest_published",
	})
	var events []workflowevents.Event
	ctx := workflowevents.WithEmitter(context.Background(), func(event workflowevents.Event) { events = append(events, event) })
	conversationID := "conversation-1"
	messageID := "message-1"
	_, err := runtimeTool.Invoke(ctx, "caller-1", map[string]interface{}{
		"binding_id": "task-flow", "inputs": map[string]interface{}{},
	}, &conversationID, nil, &messageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "node_finished" || events[0].Sequence != 2 {
		t.Fatalf("events = %#v, want only node_finished", events)
	}
}

func TestListAgentWorkflowsReturnsStartInputSchema(t *testing.T) {
	runtimeTool := workflowRuntimeToolWithBinding(t, ToolListAgentWorkflows, &fakeWorkflowRunner{}, map[string]interface{}{
		"binding_id":        "task-flow",
		"label":             "Task flow",
		"agent_id":          "agent-1",
		"workflow_id":       "workflow-1",
		"agent_type":        "WORKFLOW",
		"version_strategy":  "latest_published",
		"required_inputs":   []string{"input"},
		"default_input_key": "input",
		"start_inputs": []map[string]interface{}{
			{"variable": "input", "label": "用户输入", "type": "paragraph", "required": true},
		},
	})

	messages, err := runtimeTool.Invoke(context.Background(), "caller-1", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	workflows := messages[0].Data["workflows"].([]map[string]interface{})
	if workflows[0]["default_input_key"] != "input" {
		t.Fatalf("default_input_key = %#v, want input", workflows[0]["default_input_key"])
	}
	required, ok := workflows[0]["required_inputs"].([]string)
	if !ok || len(required) != 1 || required[0] != "input" {
		t.Fatalf("required_inputs = %#v, want [input]", workflows[0]["required_inputs"])
	}
	schema := workflows[0]["input_schema"].(map[string]interface{})
	properties := schema["properties"].(map[string]interface{})
	if _, ok := properties["input"]; !ok {
		t.Fatalf("input_schema properties = %#v, want input", properties)
	}
}

func TestRunAgentWorkflowReturnsPendingApprovalFields(t *testing.T) {
	runner := &fakeWorkflowRunner{
		result: &automationaction.WorkflowRunResult{
			WorkflowRunID: "run-approval",
			WorkflowID:    "workflow-1",
			AgentID:       "agent-1",
			Status:        "paused",
			Outputs: map[string]interface{}{
				"approval-node": map[string]interface{}{
					"outputs": map[string]interface{}{
						"__approval_form_id": "form-1",
						"__approval_token":   "token-1",
						"__approval_form":    map[string]interface{}{"id": "form-1", "token": "token-1"},
					},
				},
			},
		},
	}
	runtimeTool := workflowRuntimeTool(t, ToolRunAgentWorkflow, runner)

	messages, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "approval-flow",
		"inputs":     map[string]interface{}{},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := messages[0].Data
	if payload["status"] != "pending_approval" || payload["approval_form_id"] != "form-1" || payload["approval_token"] != "token-1" {
		t.Fatalf("payload = %#v, want pending approval fields", payload)
	}
	if payload["approval_form"] == nil {
		t.Fatalf("payload = %#v, want approval_form", payload)
	}
}

func TestWorkflowApprovalUIAccessFollowsAgentSurfaceAndWorkflowChannel(t *testing.T) {
	disabledPayload := map[string]interface{}{
		"approval_form": map[string]interface{}{
			"submit_methods": map[string]interface{}{
				"webapp": map[string]interface{}{"enabled": false},
			},
		},
	}
	consoleRuntime := &tools.ToolRuntime{RuntimeParameters: map[string]interface{}{
		agentRuntimeSourceParameter: "console",
	}}
	if !workflowApprovalUIAccessAllowed(consoleRuntime, disabledPayload) {
		t.Fatal("Agent draft preview should allow inline approval regardless of workflow channel")
	}

	webAppRuntime := &tools.ToolRuntime{RuntimeParameters: map[string]interface{}{
		agentRuntimeSourceParameter: "webapp",
	}}
	if workflowApprovalUIAccessAllowed(webAppRuntime, disabledPayload) {
		t.Fatal("Agent WebApp should not allow inline approval when workflow webapp channel is disabled")
	}
	enabledPayload := map[string]interface{}{
		"submit_methods": map[string]interface{}{
			"webapp": map[string]interface{}{"enabled": true},
		},
	}
	if !workflowApprovalUIAccessAllowed(webAppRuntime, enabledPayload) {
		t.Fatal("Agent WebApp should allow inline approval when workflow webapp channel is enabled")
	}

	externalAPIRuntime := &tools.ToolRuntime{RuntimeParameters: map[string]interface{}{
		agentRuntimeSourceParameter: "external-api",
	}}
	if workflowApprovalUIAccessAllowed(externalAPIRuntime, disabledPayload) {
		t.Fatal("Agent external API should not allow inline approval when workflow webapp channel is disabled")
	}
	if !workflowApprovalUIAccessAllowed(externalAPIRuntime, enabledPayload) {
		t.Fatal("Agent external API should allow inline approval when workflow webapp channel is enabled")
	}
}

func TestApplyWorkflowApprovalExposureRedactsCredentialsWhenUIApprovalIsUnavailable(t *testing.T) {
	payload := map[string]interface{}{
		"approval_token": "top-secret",
		"approval_form": map[string]interface{}{
			"id":    "form-1",
			"token": "form-secret",
		},
		"outputs": map[string]interface{}{
			"__approval_token": "nested-secret",
			"__approval_form": map[string]interface{}{
				"id":    "form-1",
				"token": "nested-form-secret",
			},
		},
	}
	applyWorkflowApprovalExposure(&tools.ToolRuntime{RuntimeParameters: map[string]interface{}{
		agentRuntimeSourceParameter: "external-api",
	}}, payload)

	if allowed, _ := payload[workflowApprovalUIAllowed].(bool); allowed {
		t.Fatalf("ui approval allowed = true, want false: %#v", payload)
	}
	if _, exists := payload["approval_token"]; exists {
		t.Fatalf("top-level approval token was exposed: %#v", payload)
	}
	form := payload["approval_form"].(map[string]interface{})
	if form["id"] != "form-1" {
		t.Fatalf("approval form identity was removed: %#v", form)
	}
	if _, exists := form["token"]; exists {
		t.Fatalf("approval form token was exposed: %#v", form)
	}
	outputs := payload["outputs"].(map[string]interface{})
	if _, exists := outputs["__approval_token"]; exists {
		t.Fatalf("nested approval token was exposed: %#v", outputs)
	}
	nestedForm := outputs["__approval_form"].(map[string]interface{})
	if _, exists := nestedForm["token"]; exists {
		t.Fatalf("nested approval form token was exposed: %#v", nestedForm)
	}
}

func TestRunAgentWorkflowReturnsFailedSummary(t *testing.T) {
	runner := &fakeWorkflowRunner{
		result: &automationaction.WorkflowRunResult{
			WorkflowRunID: "run-failed",
			WorkflowID:    "workflow-1",
			AgentID:       "agent-1",
			Status:        "failed",
		},
		err: errors.New("node failed"),
	}
	runtimeTool := workflowRuntimeTool(t, ToolRunAgentWorkflow, runner)

	messages, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"binding_id": "approval-flow",
		"inputs":     map[string]interface{}{},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := messages[0].Data
	if payload["status"] != "failed" || !strings.Contains(stringValue(payload, "error"), "node failed") {
		t.Fatalf("payload = %#v, want failed summary", payload)
	}
}

func TestRunAgentWorkflowForwardsWorkflowEvents(t *testing.T) {
	runner := &fakeWorkflowRunner{
		emitEvents: []automationaction.WorkflowRunEvent{
			{
				Type: "workflow_started",
				Payload: map[string]interface{}{
					"workflow_run_id": "run-event",
					"status":          "running",
				},
			},
			{
				Type: "node_started",
				Payload: map[string]interface{}{
					"workflow_run_id": "run-event",
					"node_id":         "node-1",
					"status":          "running",
				},
			},
		},
		result: &automationaction.WorkflowRunResult{
			WorkflowRunID: "run-event",
			WorkflowID:    "workflow-1",
			AgentID:       "agent-1",
			Status:        "succeeded",
		},
	}
	runtimeTool := workflowRuntimeTool(t, ToolRunAgentWorkflow, runner)
	var events []workflowevents.Event
	ctx := workflowevents.WithEmitter(context.Background(), func(event workflowevents.Event) {
		events = append(events, event)
	})

	_, err := runtimeTool.Invoke(ctx, "caller-1", map[string]interface{}{
		"binding_id": "approval-flow",
		"inputs":     map[string]interface{}{},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v, want 2 forwarded events", events)
	}
	if events[0].Type != "workflow_started" || events[0].Payload["workflow_run_id"] != "run-event" {
		t.Fatalf("first event = %#v, want workflow_started run-event", events[0])
	}
	if events[1].Type != "node_started" || events[1].Payload["node_id"] != "node-1" {
		t.Fatalf("second event = %#v, want node_started node-1", events[1])
	}
}

func TestGetWorkflowRunStatusReturnsBoundRunStatus(t *testing.T) {
	runner := &fakeWorkflowRunner{
		status: &automationaction.WorkflowRunStatusResult{
			WorkflowRunID: "run-1",
			WorkflowID:    "workflow-1",
			AgentID:       "agent-1",
			Status:        "succeeded",
			Outputs:       map[string]interface{}{"answer": "done"},
		},
	}
	runtimeTool := workflowRuntimeTool(t, ToolGetWorkflowRunStatus, runner)

	messages, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"workflow_run_id": "run-1",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if runner.lastStatusReq.WorkflowRunID != "run-1" || runner.lastStatusReq.AccountID != "binder-1" {
		t.Fatalf("status request = %#v, want run-1 with binder-1", runner.lastStatusReq)
	}
	payload := messages[0].Data
	if payload["status"] != "succeeded" {
		t.Fatalf("payload = %#v, want succeeded", payload)
	}
}

func TestGetWorkflowRunStatusRejectsUnboundRun(t *testing.T) {
	runner := &fakeWorkflowRunner{
		status: &automationaction.WorkflowRunStatusResult{
			WorkflowRunID: "run-2",
			WorkflowID:    "workflow-2",
			AgentID:       "agent-2",
			Status:        "succeeded",
		},
	}
	runtimeTool := workflowRuntimeTool(t, ToolGetWorkflowRunStatus, runner)

	_, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"workflow_run_id": "run-2",
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not part of the current Agent workflow bindings") {
		t.Fatalf("Invoke() error = %v, want unbound run rejection", err)
	}
}

func TestGetWorkflowRunStatusRejectsSameAgentDifferentWorkflow(t *testing.T) {
	runner := &fakeWorkflowRunner{
		status: &automationaction.WorkflowRunStatusResult{
			WorkflowRunID: "run-2",
			WorkflowID:    "workflow-2",
			AgentID:       "agent-1",
			Status:        "succeeded",
		},
	}
	runtimeTool := workflowRuntimeTool(t, ToolGetWorkflowRunStatus, runner)

	_, err := runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"workflow_run_id": "run-2",
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not part of the current Agent workflow bindings") {
		t.Fatalf("Invoke() error = %v, want different workflow rejection", err)
	}
}

func workflowRuntimeTool(t *testing.T, name string, runner *fakeWorkflowRunner) tools.Tool {
	return workflowRuntimeToolWithBinding(t, name, runner, map[string]interface{}{
		"binding_id":       "approval-flow",
		"label":            "Approval flow",
		"description":      "Approves work",
		"agent_id":         "agent-1",
		"workflow_id":      "workflow-1",
		"version_strategy": "latest_published",
		"timeout_seconds":  60,
	})
}

func workflowRuntimeToolWithBinding(t *testing.T, name string, runner *fakeWorkflowRunner, binding map[string]interface{}) tools.Tool {
	t.Helper()
	provider := NewProvider(func() automationaction.AutomationWorkflowRunner { return runner })
	tool, err := provider.GetTool(name)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	return tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "workspace-1",
		InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"organization_id":              "org-1",
			"workspace_id":                 "workspace-1",
			agentRuntimeSourceParameter:    "console",
			"workflow_bound_by_account_id": "binder-1",
			"workflow_bindings": []map[string]interface{}{
				binding,
			},
		},
	})
}

type fakeWorkflowRunner struct {
	result        *automationaction.WorkflowRunResult
	err           error
	status        *automationaction.WorkflowRunStatusResult
	statusErr     error
	lastReq       automationaction.WorkflowRunRequest
	lastStatusReq automationaction.WorkflowRunStatusRequest
	emitEvents    []automationaction.WorkflowRunEvent
}

func (f *fakeWorkflowRunner) RunAutomationWorkflow(ctx context.Context, req automationaction.WorkflowRunRequest) (*automationaction.WorkflowRunResult, error) {
	_ = ctx
	f.lastReq = req
	for _, event := range f.emitEvents {
		if req.EventSink != nil {
			req.EventSink(event)
		}
	}
	if f.result != nil || f.err != nil {
		return f.result, f.err
	}
	return &automationaction.WorkflowRunResult{
		WorkflowRunID: "run-default",
		WorkflowID:    req.WorkflowRef.WorkflowID,
		AgentID:       req.WorkflowRef.AgentID,
		Status:        "succeeded",
		Outputs:       map[string]interface{}{},
	}, nil
}

func (f *fakeWorkflowRunner) GetAutomationWorkflowRunStatus(ctx context.Context, req automationaction.WorkflowRunStatusRequest) (*automationaction.WorkflowRunStatusResult, error) {
	_ = ctx
	f.lastStatusReq = req
	if f.status != nil || f.statusErr != nil {
		return f.status, f.statusErr
	}
	return &automationaction.WorkflowRunStatusResult{
		WorkflowRunID: req.WorkflowRunID,
		WorkflowID:    "workflow-1",
		AgentID:       "agent-1",
		Status:        "succeeded",
		Outputs:       map[string]interface{}{},
	}, nil
}
