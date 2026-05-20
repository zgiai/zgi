package workflow

import (
	"errors"
	"testing"
	"time"
)

func TestReceiveWorkflowStreamSelection_PrioritizesErrorOverClosedDone(t *testing.T) {
	resultChan := make(chan *WorkflowStreamEvent, 1)
	errorChan := make(chan error, 1)
	doneChan := make(chan map[string]interface{})

	wantErr := errors.New("node failed")
	errorChan <- wantErr
	close(doneChan)

	selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, nil)

	if selection.kind != workflowStreamSelectionError {
		t.Fatalf("selection.kind = %v, want %v", selection.kind, workflowStreamSelectionError)
	}
	if !errors.Is(selection.err, wantErr) {
		t.Fatalf("selection.err = %v, want %v", selection.err, wantErr)
	}
}

func TestReceiveWorkflowStreamSelection_ReturnsDonePayloadWhenAvailable(t *testing.T) {
	resultChan := make(chan *WorkflowStreamEvent, 1)
	errorChan := make(chan error, 1)
	doneChan := make(chan map[string]interface{}, 1)

	doneChan <- map[string]interface{}{"status": "succeeded"}

	selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, nil)

	if selection.kind != workflowStreamSelectionDone {
		t.Fatalf("selection.kind = %v, want %v", selection.kind, workflowStreamSelectionDone)
	}
	if selection.outputs["status"] != "succeeded" {
		t.Fatalf("selection.outputs[status] = %v, want %v", selection.outputs["status"], "succeeded")
	}
	if !selection.ok {
		t.Fatal("selection.ok = false, want true")
	}
}

func TestReceiveWorkflowStreamSelection_DrainsResultBeforeDone(t *testing.T) {
	resultChan := make(chan *WorkflowStreamEvent, 1)
	errorChan := make(chan error, 1)
	doneChan := make(chan map[string]interface{}, 1)

	resultChan <- &WorkflowStreamEvent{EventType: "node_finished"}
	doneChan <- map[string]interface{}{"status": "succeeded"}

	selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, nil)

	if selection.kind != workflowStreamSelectionResult {
		t.Fatalf("selection.kind = %v, want %v", selection.kind, workflowStreamSelectionResult)
	}
	if selection.event == nil || selection.event.EventType != "node_finished" {
		t.Fatalf("selection.event = %#v, want node_finished", selection.event)
	}
}

func TestReceiveWorkflowStreamSelection_DrainsResultBeforeHeartbeat(t *testing.T) {
	oldInterval := workflowStreamHeartbeatInterval
	workflowStreamHeartbeatInterval = time.Nanosecond
	t.Cleanup(func() {
		workflowStreamHeartbeatInterval = oldInterval
	})

	resultChan := make(chan *WorkflowStreamEvent, 1)
	errorChan := make(chan error, 1)
	doneChan := make(chan map[string]interface{}, 1)

	resultChan <- &WorkflowStreamEvent{EventType: "node_finished"}

	selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, nil)

	if selection.kind != workflowStreamSelectionResult {
		t.Fatalf("selection.kind = %v, want %v", selection.kind, workflowStreamSelectionResult)
	}
	if selection.event == nil || selection.event.EventType != "node_finished" {
		t.Fatalf("selection.event = %#v, want node_finished", selection.event)
	}
}

func TestReceiveWorkflowStreamSelection_ReturnsHeartbeatAfterIdleInterval(t *testing.T) {
	oldInterval := workflowStreamHeartbeatInterval
	workflowStreamHeartbeatInterval = time.Millisecond
	t.Cleanup(func() {
		workflowStreamHeartbeatInterval = oldInterval
	})

	resultChan := make(chan *WorkflowStreamEvent)
	errorChan := make(chan error)
	doneChan := make(chan map[string]interface{})

	selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, nil)

	if selection.kind != workflowStreamSelectionHeartbeat {
		t.Fatalf("selection.kind = %v, want %v", selection.kind, workflowStreamSelectionHeartbeat)
	}
}

func TestReceiveWorkflowStreamSelection_ReturnsClosedDoneWithoutPayload(t *testing.T) {
	resultChan := make(chan *WorkflowStreamEvent, 1)
	errorChan := make(chan error, 1)
	doneChan := make(chan map[string]interface{})
	close(doneChan)

	selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, nil)

	if selection.kind != workflowStreamSelectionDone {
		t.Fatalf("selection.kind = %v, want %v", selection.kind, workflowStreamSelectionDone)
	}
	if selection.ok {
		t.Fatal("selection.ok = true, want false for closed done channel")
	}
}
