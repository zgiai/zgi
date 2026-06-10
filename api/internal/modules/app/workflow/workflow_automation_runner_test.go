package workflow

import (
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
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

func TestEmitAutomationWorkflowNodeFinishedIncludesElapsedMilliseconds(t *testing.T) {
	startedAt := time.Unix(1700000000, 0)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	var got automationaction.WorkflowRunEvent

	emitAutomationWorkflowNodeFinished(func(event automationaction.WorkflowRunEvent) {
		got = event
	}, automationWorkflowEventTestRequest(), automationWorkflowEventTestWorkflow(), "run-1", automationWorkflowNodeMeta{
		NodeID:   "node-1",
		NodeType: "llm",
		Title:    "LLM",
	}, graph_engine.NodeFinishedEvent{
		NodeID:      "node-1",
		NodeType:    "llm",
		Status:      "succeeded",
		Outputs:     map[string]any{"text": "ok"},
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
		ElapsedTime: finishedAt.Sub(startedAt),
	})

	if got.Type != automationWorkflowEventNodeFinished {
		t.Fatalf("event type = %q, want %q", got.Type, automationWorkflowEventNodeFinished)
	}
	if elapsed, ok := got.Payload["elapsed_time"].(float64); !ok || elapsed != 1500 {
		t.Fatalf("elapsed_time = %#v, want 1500 ms", got.Payload["elapsed_time"])
	}
	if got.Payload["created_at"] != startedAt.Unix() || got.Payload["finished_at"] != finishedAt.Unix() {
		t.Fatalf("timestamps = %#v, want started/finished unix seconds", got.Payload)
	}
}

func TestEmitAutomationWorkflowIterationCompletedIncludesElapsedMilliseconds(t *testing.T) {
	startedAt := time.Unix(1700000000, 0)
	finishedAt := startedAt.Add(2200 * time.Millisecond)
	var got automationaction.WorkflowRunEvent

	emitAutomationWorkflowIterationEvent(func(event automationaction.WorkflowRunEvent) {
		got = event
	}, automationWorkflowEventTestRequest(), automationWorkflowEventTestWorkflow(), "run-1", map[string]automationWorkflowNodeMeta{
		"iter-1": {NodeID: "iter-1", NodeType: "iteration", Title: "Iteration"},
	}, &graph_engine.IterationEvent{
		Type:      "completed",
		NodeID:    "iter-1",
		Index:     2,
		StartedAt: startedAt,
		Timestamp: finishedAt,
		Outputs:   map[string]any{"items": 2},
	})

	if got.Type != automationWorkflowEventIterationFinished {
		t.Fatalf("event type = %q, want %q", got.Type, automationWorkflowEventIterationFinished)
	}
	if elapsed, ok := got.Payload["elapsed_time"].(float64); !ok || elapsed != 2200 {
		t.Fatalf("elapsed_time = %#v, want 2200 ms", got.Payload["elapsed_time"])
	}
}

func automationWorkflowEventTestRequest() automationaction.WorkflowRunRequest {
	return automationaction.WorkflowRunRequest{
		WorkflowRef: automationaction.WorkflowRef{
			AgentID:    "agent-1",
			WorkflowID: "workflow-1",
		},
	}
}

func automationWorkflowEventTestWorkflow() *Workflow {
	return &Workflow{
		ID:      "workflow-1",
		AgentID: "agent-1",
		Version: "v1",
	}
}
