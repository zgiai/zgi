package digitalhuman

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

type fakeWorkflowContinuationRuntime struct {
	recorded       []string
	paused         []string
	completed      []string
	failed         []string
	summarized     []chatruntime.WorkflowContinuationSummaryRequest
	summaryResult  *chatruntime.ChatResult
	appendedEvents []string
}

func (f *fakeWorkflowContinuationRuntime) BeginWorkflowApprovalContinuation(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.RunConfig, uuid.UUID, uuid.UUID) (*chatruntime.WorkflowApprovalContinuation, error) {
	return nil, errors.New("not used")
}

func (f *fakeWorkflowContinuationRuntime) RecordWorkflowApprovalContinuationEvent(_ context.Context, _ *chatruntime.WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*chatruntime.StreamEvent, error) {
	f.recorded = append(f.recorded, eventType)
	return &chatruntime.StreamEvent{EventType: eventType, Payload: payload}, nil
}

func (f *fakeWorkflowContinuationRuntime) AppendWorkflowApprovalContinuationStreamEvent(_ context.Context, _ *chatruntime.WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*chatruntime.StreamEvent, error) {
	f.appendedEvents = append(f.appendedEvents, eventType)
	return &chatruntime.StreamEvent{EventType: eventType, Payload: payload}, nil
}

func (f *fakeWorkflowContinuationRuntime) PauseWorkflowApprovalContinuation(_ context.Context, _ *chatruntime.WorkflowApprovalContinuation, status string) (map[string]interface{}, error) {
	f.paused = append(f.paused, status)
	return map[string]interface{}{}, nil
}

func (f *fakeWorkflowContinuationRuntime) CompleteWorkflowApprovalContinuation(_ context.Context, _ *chatruntime.WorkflowApprovalContinuation, _ string, status string) (map[string]interface{}, error) {
	f.completed = append(f.completed, status)
	return map[string]interface{}{}, nil
}

func (f *fakeWorkflowContinuationRuntime) SummarizeWorkflowApprovalContinuation(_ context.Context, _ chatruntime.Scope, _ *chatruntime.WorkflowApprovalContinuation, req chatruntime.WorkflowContinuationSummaryRequest, _ func(chatruntime.StreamEvent) error) (*chatruntime.ChatResult, error) {
	f.summarized = append(f.summarized, req)
	if f.summaryResult != nil {
		return f.summaryResult, nil
	}
	return &chatruntime.ChatResult{Status: runtimemodel.MessageStatusCompleted, Metadata: map[string]interface{}{}}, nil
}

func (f *fakeWorkflowContinuationRuntime) FailWorkflowApprovalContinuation(_ context.Context, _ *chatruntime.WorkflowApprovalContinuation, message string) (map[string]interface{}, error) {
	f.failed = append(f.failed, message)
	return map[string]interface{}{}, nil
}

func newTestWorkflowContinuationLifecycle(runtime *fakeWorkflowContinuationRuntime, emitted *[]chatruntime.StreamEvent) *digitalHumanWorkflowContinuation {
	return &digitalHumanWorkflowContinuation{
		ctx:     context.Background(),
		runtime: runtime,
		continuation: &chatruntime.WorkflowApprovalContinuation{
			ConversationID: uuid.New(), MessageID: uuid.New(), WorkflowRunID: "run-1",
			MessageStatus: runtimemodel.MessageStatusWaitingApproval,
		},
		onEvent: func(event chatruntime.StreamEvent) error {
			*emitted = append(*emitted, event)
			return nil
		},
	}
}

func lastEmittedStatus(t *testing.T, events []chatruntime.StreamEvent) string {
	t.Helper()
	if len(events) == 0 || events[len(events)-1].EventType != "message_end" {
		t.Fatalf("last emitted event is not message_end: %#v", events)
	}
	status, _ := events[len(events)-1].Payload["status"].(string)
	return status
}

func TestDigitalHumanWorkflowContinuationCompletesFinishedWorkflow(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
	outputs := map[string]interface{}{"answer": "done"}
	_ = lifecycle.handle("node_finished", map[string]interface{}{})
	_ = lifecycle.handle("workflow_finished", map[string]interface{}{"status": "succeeded", "outputs": outputs})

	if err := lifecycle.finish(workflowContinuationStreamed, nil); err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if strings.Join(runtime.recorded, ",") != "node_finished,workflow_finished" {
		t.Fatalf("recorded events = %v", runtime.recorded)
	}
	if len(runtime.summarized) != 1 || runtime.summarized[0].Status != "succeeded" || runtime.summarized[0].Outputs["answer"] != "done" {
		t.Fatalf("summary requests = %#v", runtime.summarized)
	}
	if len(runtime.failed) != 0 || len(runtime.paused) != 0 {
		t.Fatalf("unexpected failed=%v paused=%v", runtime.failed, runtime.paused)
	}
	if status := lastEmittedStatus(t, emitted); status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("message_end status = %q", status)
	}
}

func TestDigitalHumanWorkflowContinuationPausesOnNextApproval(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
	_ = lifecycle.handle("approval_requested", map[string]interface{}{})

	if err := lifecycle.finish(workflowContinuationStreamed, nil); err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if len(runtime.paused) != 1 || runtime.paused[0] != runtimemodel.MessageStatusWaitingApproval {
		t.Fatalf("paused = %v", runtime.paused)
	}
	if status := lastEmittedStatus(t, emitted); status != runtimemodel.MessageStatusWaitingApproval {
		t.Fatalf("message_end status = %q", status)
	}
}

func TestDigitalHumanWorkflowContinuationPausesOnNextQuestion(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
	_ = lifecycle.handle("question_answer_requested", map[string]interface{}{})

	if err := lifecycle.finish(workflowContinuationStreamed, nil); err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if len(runtime.paused) != 1 || runtime.paused[0] != runtimemodel.MessageStatusWaitingQuestion {
		t.Fatalf("paused = %v", runtime.paused)
	}
}

func TestDigitalHumanWorkflowContinuationRepausesWhenApprovalStillPending(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)

	if err := lifecycle.finish(workflowContinuationNotResumed, nil); err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if len(runtime.paused) != 1 || runtime.paused[0] != runtimemodel.MessageStatusWaitingApproval {
		t.Fatalf("paused = %v", runtime.paused)
	}
}

func TestDigitalHumanWorkflowContinuationFailsWithoutLeakingCause(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
	cause := errors.New("pq: relation workflow_secret_table does not exist")

	if err := lifecycle.finish(workflowContinuationStreamed, cause); !errors.Is(err, cause) {
		t.Fatalf("finish() error = %v, want cause for boundary projection", err)
	}
	if len(runtime.failed) != 1 || strings.Contains(runtime.failed[0], "workflow_secret_table") {
		t.Fatalf("failed messages = %v", runtime.failed)
	}
	if status := lastEmittedStatus(t, emitted); status != runtimemodel.MessageStatusError {
		t.Fatalf("message_end status = %q", status)
	}
}

func TestDigitalHumanWorkflowContinuationFailsWhenStreamEndsWithoutTerminalEvent(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
	_ = lifecycle.handle("node_started", map[string]interface{}{})

	if err := lifecycle.finish(workflowContinuationStreamed, nil); err == nil {
		t.Fatal("finish() succeeded for a stream without a terminal workflow event")
	}
	if len(runtime.failed) != 1 {
		t.Fatalf("failed = %v", runtime.failed)
	}
}

func TestDigitalHumanWorkflowContinuationStopsStoppedWorkflow(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
	_ = lifecycle.handle("workflow_stopped", map[string]interface{}{})
	_ = lifecycle.handle("workflow_finished", map[string]interface{}{"status": "succeeded"})

	if err := lifecycle.finish(workflowContinuationStreamed, nil); err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if len(runtime.completed) != 1 || runtime.completed[0] != runtimemodel.MessageStatusStopped || len(runtime.summarized) != 0 {
		t.Fatalf("completed=%v summarized=%v", runtime.completed, runtime.summarized)
	}
}

func TestDigitalHumanWorkflowContinuationSummarizesFailedWorkflow(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
	_ = lifecycle.handle("error", map[string]interface{}{"message": "node failed"})
	_ = lifecycle.handle("workflow_finished", map[string]interface{}{"status": "failed"})

	if err := lifecycle.finish(workflowContinuationStreamed, nil); err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if len(runtime.summarized) != 1 || runtime.summarized[0].Status != "failed" || runtime.summarized[0].Error != "node failed" {
		t.Fatalf("summary requests = %#v", runtime.summarized)
	}
	if runtime.recorded[0] != "workflow_failed" {
		t.Fatalf("error event recorded as %q, want workflow_failed", runtime.recorded[0])
	}
}

func TestDigitalHumanWorkflowContinuationIgnoresClientEmitFailure(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	lifecycle := &digitalHumanWorkflowContinuation{
		ctx:          context.Background(),
		runtime:      runtime,
		continuation: &chatruntime.WorkflowApprovalContinuation{ConversationID: uuid.New(), MessageID: uuid.New(), WorkflowRunID: "run-1"},
		onEvent:      func(chatruntime.StreamEvent) error { return errors.New("client disconnected") },
	}
	if err := lifecycle.handle("node_finished", map[string]interface{}{}); err != nil {
		t.Fatalf("handle() error = %v, want workflow to keep running after a client disconnect", err)
	}
	if len(runtime.recorded) != 1 {
		t.Fatalf("recorded = %v", runtime.recorded)
	}
}

func TestDigitalHumanWorkflowContinuationRestoresWaitingStateWhenNotResumed(t *testing.T) {
	for _, status := range []string{runtimemodel.MessageStatusWaitingApproval, runtimemodel.MessageStatusWaitingQuestion} {
		t.Run(status, func(t *testing.T) {
			runtime := &fakeWorkflowContinuationRuntime{}
			var emitted []chatruntime.StreamEvent
			lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
			lifecycle.continuation.MessageStatus = status
			cause := errors.New("invalid approval token")

			if err := lifecycle.finish(workflowContinuationNotResumed, cause); !errors.Is(err, cause) {
				t.Fatalf("finish() error = %v", err)
			}
			if len(runtime.paused) != 1 || runtime.paused[0] != status || len(runtime.failed) != 0 {
				t.Fatalf("paused=%v failed=%v, want the original %s state restored", runtime.paused, runtime.failed, status)
			}
		})
	}
}

func TestDigitalHumanWorkflowContinuationRestoresWaitingStateWhenMessageStartFails(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
	cause := errors.New("client disconnected")

	if err := lifecycle.abortBeforeResume(cause); !errors.Is(err, cause) {
		t.Fatalf("abortBeforeResume() error = %v", err)
	}
	if len(runtime.paused) != 1 || runtime.paused[0] != runtimemodel.MessageStatusWaitingApproval || len(runtime.failed) != 0 {
		t.Fatalf("paused=%v failed=%v", runtime.paused, runtime.failed)
	}
}

func TestDigitalHumanWorkflowContinuationFailsAbortedStreamingContinuation(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{}
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, &emitted)
	lifecycle.continuation.MessageStatus = runtimemodel.MessageStatusStreaming

	_ = lifecycle.abortBeforeResume(errors.New("client disconnected"))
	if len(runtime.failed) != 1 || len(runtime.paused) != 0 {
		t.Fatalf("paused=%v failed=%v, want a non-waiting continuation failed", runtime.paused, runtime.failed)
	}
}
