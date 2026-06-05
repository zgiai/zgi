package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/internal/modules/tools"
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
		"inputs":     map[string]interface{}{"query": "approve"},
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
	payload := messages[0].Data
	if payload["status"] != "succeeded" || payload["workflow_run_id"] != "run-1" {
		t.Fatalf("payload = %#v, want succeeded run-1", payload)
	}
	outputs, _ := payload["outputs"].(map[string]interface{})
	if outputs["answer"] != "done" {
		t.Fatalf("outputs = %#v, want answer done", outputs)
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
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := messages[0].Data
	if payload["status"] != "failed" || !strings.Contains(stringValue(payload, "error"), "node failed") {
		t.Fatalf("payload = %#v, want failed summary", payload)
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

func workflowRuntimeTool(t *testing.T, name string, runner *fakeWorkflowRunner) tools.Tool {
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
			"workflow_bound_by_account_id": "binder-1",
			"workflow_bindings": []map[string]interface{}{
				{
					"binding_id":       "approval-flow",
					"label":            "Approval flow",
					"description":      "Approves work",
					"agent_id":         "agent-1",
					"workflow_id":      "workflow-1",
					"version_strategy": "latest_published",
					"timeout_seconds":  60,
				},
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
}

func (f *fakeWorkflowRunner) RunAutomationWorkflow(ctx context.Context, req automationaction.WorkflowRunRequest) (*automationaction.WorkflowRunResult, error) {
	_ = ctx
	f.lastReq = req
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
