package workflow

import "testing"

// A digital human's task confirmation shares the workflow continuation
// record but must not be logged as a workflow run or approval.
func TestTaskConfirmationIsNotAWorkflowApproval(t *testing.T) {
	events := runtimeWorkflowRunEvents([]interface{}{
		map[string]interface{}{
			"kind": "digital_human_task", "status": "pending_approval", "task_id": "task-1",
			"approval": map[string]interface{}{"task_name": "Water reminder", "approval_form": map[string]interface{}{"id": "form-1"}},
		},
		map[string]interface{}{"workflow_run_id": "run-1", "workflow_id": "wf-1", "status": "succeeded"},
	})
	if len(events) != 2 {
		t.Fatalf("events = %#v", events)
	}
	task := events[0]
	if agentRuntimeEventType(task) != "task_approval" || agentRuntimeEventTitle(task) != "Task confirmation: Water reminder" || task["task_id"] != "task-1" {
		t.Fatalf("task confirmation event = %#v", task)
	}
	if agentRuntimeEventType(events[1]) != "workflow_run" {
		t.Fatalf("workflow run event = %#v", events[1])
	}
}
