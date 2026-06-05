package workflow

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestAutomationWorkflowOutputsUsesRuntimeOutputs(t *testing.T) {
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	runtimeState.UpdateOutputs(func(current map[string]any) map[string]any {
		current["answer"] = "春风入纸"
		current["extra"] = "visible"
		return current
	})
	result := &WorkflowExecutionResult{
		RuntimeState: runtimeState,
		NodeResults: map[string]interface{}{
			"answer-node": map[string]interface{}{
				"status": "succeeded",
			},
		},
	}

	outputs := automationWorkflowOutputs(result)

	if got := outputs["answer"]; got != "春风入纸" {
		t.Fatalf("outputs[answer] = %#v, want 春风入纸", got)
	}
	if got := outputs["extra"]; got != "visible" {
		t.Fatalf("outputs[extra] = %#v, want visible", got)
	}
	if _, ok := outputs["answer-node"]; ok {
		t.Fatalf("outputs[answer-node] contains node status summary")
	}
}

func TestAutomationWorkflowOutputsMergesApprovalFieldsFromNodeExecutions(t *testing.T) {
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	runtimeState.UpdateOutputs(func(current map[string]any) map[string]any {
		current["answer"] = "waiting"
		return current
	})
	result := &WorkflowExecutionResult{
		RuntimeState: runtimeState,
		NodeResults: map[string]interface{}{
			"approval": map[string]interface{}{
				"status": "paused",
			},
		},
		NodeExecutions: []graph_engine.NodeExecutionSnapshot{
			{
				NodeID:   "approval",
				NodeType: workflowshared.Approval,
				Outputs: map[string]interface{}{
					"__approval_form_id": "form-1",
					"__approval_token":   "token-1",
					"__approval_form": map[string]interface{}{
						"id":    "form-1",
						"token": "token-1",
						"url":   "/workflow/approval/form-1",
					},
				},
			},
		},
	}

	outputs := automationWorkflowOutputs(result)

	if got := outputs["__approval_form_id"]; got != "form-1" {
		t.Fatalf("outputs[__approval_form_id] = %#v, want form-1", got)
	}
	if got := outputs["__approval_token"]; got != "token-1" {
		t.Fatalf("outputs[__approval_token] = %#v, want token-1", got)
	}
	form, ok := outputs["__approval_form"].(map[string]interface{})
	if !ok {
		t.Fatalf("outputs[__approval_form] = %#v, want map", outputs["__approval_form"])
	}
	if got := form["url"]; got != "/workflow/approval/form-1" {
		t.Fatalf("outputs[__approval_form].url = %#v, want /workflow/approval/form-1", got)
	}
	if _, ok := outputs["approval"]; ok {
		t.Fatalf("outputs[approval] contains node status summary")
	}
}

func TestAutomationWorkflowOutputsKeepsExistingApprovalFields(t *testing.T) {
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	runtimeState.UpdateOutputs(func(current map[string]any) map[string]any {
		current["__approval_form_id"] = "existing-form"
		return current
	})
	result := &WorkflowExecutionResult{
		RuntimeState: runtimeState,
		NodeResults: map[string]interface{}{
			"__approval_form_id": "existing-form",
		},
		NodeExecutions: []graph_engine.NodeExecutionSnapshot{
			{
				NodeID:   "approval",
				NodeType: workflowshared.Approval,
				Outputs: map[string]interface{}{
					"__approval_form_id": "new-form",
				},
			},
		},
	}

	outputs := automationWorkflowOutputs(result)

	if got := outputs["__approval_form_id"]; got != "existing-form" {
		t.Fatalf("outputs[__approval_form_id] = %#v, want existing-form", got)
	}
}

func TestAutomationWorkflowOutputsDoesNotPromoteNonApprovalToken(t *testing.T) {
	result := &WorkflowExecutionResult{
		NodeExecutions: []graph_engine.NodeExecutionSnapshot{
			{
				NodeID:   "custom",
				NodeType: workflowshared.Code,
				Outputs: map[string]interface{}{
					"approval_token": "not-an-approval",
				},
			},
		},
	}

	outputs := automationWorkflowOutputs(result)

	if _, ok := outputs["approval_token"]; ok {
		t.Fatalf("outputs[approval_token] promoted from non-approval node")
	}
}
